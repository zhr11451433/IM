package codec

import (
	"bytes"
	"errors"
	"fmt"

	"google.golang.org/protobuf/proto"
)

//外层 Cmd 声明消息类型，内层 one of 提供实际消息内容，两者必须匹配
//ImWebsocketMsg：整个快递包裹。
//Cmd：包裹上的物品类别标签，比如「易碎品」「液体」。
//Test of：包裹里面实际装的物品，以及对应的物品信息。
//ConnectMsgBody：具体的连接请求内容

//ConnectMsgBody解析好了的
//ImWebsocketMsg
//│
//├── Cmd = Connect
//│
//└── Test of
//     └── ConnectMsgBody
//          ├── ProtoId
//          ├── AppKey
//          └── Token

//ConnectMsgBody没有解析，发的原始payload，这时不能有test of
//ImWebsocketMsg
//├── Cmd = Connect
//├── Payload = 原始 ConnectMsgBody 字节
//└── Test of = ConnectMsgBody

const (
	Version    int32 = 1 //协议版本号
	Connect    int32 = 0 //客户端发起连接认证
	ConnectAck int32 = 1 //服务端回应连接认证
	Disconnect int32 = 2 //断开连接消息
	Ping       int32 = 8 //心跳请求
	Pong       int32 = 9 //心跳回应

	ProtoID = "jug9le1m" //协议标识字符串
	maskLen = 8          // 混淆码长度：8 字节循环 XOR
)

type Wire struct {
	mask   [maskLen]byte //密钥/掩码
	masked bool          //表示当前连接是否已经成功完成混淆码初始化
}

// 作用是：在程序启动、包初始化时，
// 提前生成一份固定的 ConnectMsgBody Protobuf 二进制数据，
// 作为后续推导 mask 的已知明文模板
var knownConnectPrefix = func() []byte {
	//告诉 Protobuf：ConnectMsgBody 这个类型的字段结构、序列化规则等信息已经准备好了
	file_connect_proto_init()
	fixed, err := proto.Marshal(&ConnectMsgBody{ProtoId: ProtoID})
	if err != nil {
		panic("codec: marshal connect prefix: " + err.Error())
	}
	return fixed
}()

// 原始数据 XOR mask = 混淆数据
// 混淆数据 XOR mask = 原始数据
func xorBytes(mask [maskLen]byte, data []byte) {
	for i := range data {
		data[i] ^= mask[i%maskLen]
	}
}

// deriveMask 用已知明文前缀（knownConnectPrefix）反推混淆码，
// 并把整段 payload 还原成明文返回。
// 校验通过前不写 w.mask，失败的 Connect 不能盖掉这条连接上已经生效的码。
func deriveMask(payload []byte) (mask [maskLen]byte, plain []byte, err error) {
	//检查客户端发送的数据长度是否足够
	if len(payload) < len(knownConnectPrefix) {
		return mask, nil, fmt.Errorf("masked Connect too short: %d < %d", len(payload), len(knownConnectPrefix))
	}
	for i := range maskLen {
		mask[i] = payload[i] ^ knownConnectPrefix[i]
	}
	plain = make([]byte, len(payload))
	copy(plain, payload)
	xorBytes(mask, plain)
	// 前 8 字节按构造必然还原；多出来的 protoId 字节用来确认混淆码确实在循环。
	if !bytes.Equal(plain[:len(knownConnectPrefix)], knownConnectPrefix) {
		return mask, nil, errors.New("connect payload does not match the expected protoId prefix")
	}
	return mask, plain, nil
}

//解码 把客户端发来的二进制数据，解析成 Go 能理解的消息结构体

func (w *Wire) Decode(raw []byte) (*ImWebsocketMsg, error) {
	var im ImWebsocketMsg
	err := proto.Unmarshal(raw, &im)
	if err != nil {
		return nil, errors.New("proto unmarshal err: " + err.Error())
	}
	if im.GetVersion() != Version {
		return nil, fmt.Errorf("version mismatch: got %d, want %d", im.GetVersion(), Version)
	}
	//cmd 这个包是干嘛用的 根据消息的 Cmd 命令编号选择不同处理方式
	switch im.GetCmd() {
	case Connect:
		//payload 这个包里真正要传输的业务数据
		if len(im.GetPayload()) == 0 {
			if im.GetTestof() != nil && im.GetConnectMsgBody() == nil {
				return nil, errors.New("connect carries an unexpected body")
			}
			//没有 Payload，但是已经有正常解析好的 ConnectMsgBody
			//有 Payload，需要走后面的混淆解码流程 说明还需要处理原始数据
			return &im, nil
		}
		// 混淆路径：不允许同时再带 one of，否则两份 body 谁算数说不清
		//one of 可以理解为：多个候选消息体中，只允许选择一个 表示这条消息具体属于哪一种类型
		//Payload 里携带的是尚未解析的 ConnectMsgBody 原始数据
		if im.GetTestof() != nil {
			return nil, errors.New("connect carries both payload and one of body")
		}
		mask, plain, err := deriveMask(im.GetPayload())
		if err != nil {
			return nil, err
		}
		var body ConnectMsgBody
		if err := proto.Unmarshal(plain, &body); err != nil {
			return nil, fmt.Errorf("unmarshal ConnectMsgBody: %w", err)
		}
		if body.GetProtoId() != ProtoID {
			return nil, fmt.Errorf("connect protoId mismatch: %q", body.GetProtoId())
		}
		//把内层消息放回 one of
		im.Testof = &ImWebsocketMsg_ConnectMsgBody{ConnectMsgBody: &body}
		im.Payload = nil
		w.mask = mask
		w.masked = true

	case Disconnect:
		if len(im.GetPayload()) == 0 {
			// Disconnect 的 body 是可选的（code/timestamp/ext 都可以不带）
			return &im, nil
		}
		if im.GetTestof() != nil {
			return nil, errors.New("disconnect carries both payload and one of body")
		}
		// 没有先收到带混淆的 Connect 就出现混淆 Disconnect，说明帧序被篡改或客户端状态错乱
		if !w.masked {
			return nil, errors.New("masked Disconnect before masked Connect")
		}
		plain := make([]byte, len(im.GetPayload()))
		copy(plain, im.GetPayload())
		xorBytes(w.mask, plain)
		var body DisconnectMsgBody
		if err := proto.Unmarshal(plain, &body); err != nil {
			return nil, fmt.Errorf("unmarshal DisconnectMsgBody: %w", err)
		}
		im.Testof = &ImWebsocketMsg_DisconnectMsgBody{DisconnectMsgBody: &body}
		im.Payload = nil

	default:
		// T05 只接受没有消息体的 Ping。其他命令是否合法交给服务循环。
		// payload 混淆只定义了 Connect / Disconnect，别的命令带 payload 无法解释。
		if im.GetCmd() == Ping {
			if len(im.GetPayload()) > 0 || im.GetTestof() != nil {
				return nil, errors.New("ping must not carry a body")
			}
			break
		}
		if len(im.GetPayload()) > 0 {
			return nil, fmt.Errorf("cmd %d: payload is not supported", im.GetCmd())
		}
	}
	return &im, nil
}

// Encode 把消息打成 WebSocket 二进制帧。
// 先复制外层消息，避免清 one of、写 payload 时改到调用者手里的那份。
func (w *Wire) Encode(msg *ImWebsocketMsg) ([]byte, error) {
	if msg == nil {
		return nil, errors.New("nil message")
	}
	out, ok := proto.Clone(msg).(*ImWebsocketMsg)
	if !ok || out == nil {
		return nil, errors.New("clone ImWebsocketMsg")
	}
	if !w.masked {
		return proto.Marshal(out)
	}

	var plain []byte
	switch out.GetCmd() {
	case ConnectAck:
		body := out.GetConnectAckMsgBody()
		if body == nil {
			return nil, errors.New("connect ack without body")
		}
		var err error
		plain, err = proto.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("marshal ConnectAckMsgBody: %w", err)
		}
	case Disconnect:
		if body := out.GetDisconnectMsgBody(); body != nil {
			var err error
			plain, err = proto.Marshal(body)
			if err != nil {
				return nil, fmt.Errorf("marshal DisconnectMsgBody: %w", err)
			}
		}
	case Pong:
		if len(out.GetPayload()) > 0 || out.GetTestof() != nil {
			return nil, errors.New("pong must not carry a body")
		}
	default:
		return nil, fmt.Errorf("cmd %d: obfuscated encode is not supported", out.GetCmd())
	}

	out.Payload = nil
	if len(plain) > 0 {
		xorBytes(w.mask, plain)
		out.Payload = plain
	}
	out.Testof = nil
	return proto.Marshal(out)
}
