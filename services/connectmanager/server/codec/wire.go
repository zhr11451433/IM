package codec

import (
	"bytes"
	"errors"
	"fmt"

	"google.golang.org/protobuf/proto"
)

const (
	Version    int32 = 1
	Connect    int32 = 0
	ConnectAck int32 = 1
	Disconnect int32 = 2
	Ping       int32 = 8
	Pong       int32 = 9

	ProtoID = "jug9le1m"
	maskLen = 8 // 混淆码长度：8 字节循环 XOR
)

type Wire struct {
	mask   [maskLen]byte //密钥/掩码
	masked bool
}

// knownConnectPrefix 是 ConnectMsgBody 明文的前缀，只由 ProtoID 决定。
// 包级变量的初始化早于 init()，这里要先建好 protobuf 类型，否则 Marshal 会踩到空的 MessageInfo。
var knownConnectPrefix = func() []byte {
	file_connect_proto_init()
	fixed, err := proto.Marshal(&ConnectMsgBody{ProtoId: ProtoID})
	if err != nil {
		panic("codec: marshal connect prefix: " + err.Error())
	}
	return fixed
}()

func xorBytes(mask [maskLen]byte, data []byte) {
	for i := range data {
		data[i] ^= mask[i%maskLen]
	}
}

// deriveMask 用已知明文前缀反推混淆码，并把整段 payload 还原成明文返回。
// 校验通过前不写 w.mask，失败的 Connect 不能盖掉这条连接上已经生效的码。
func deriveMask(payload []byte) (mask [maskLen]byte, plain []byte, err error) {
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
	switch im.GetCmd() {
	case Connect:
		if len(im.GetPayload()) == 0 {
			if im.GetConnectMsgBody() == nil {
				return nil, errors.New("connect without body")
			}
			return &im, nil
		}
		// 混淆路径：不允许同时再带 oneof，否则两份 body 谁算数说不清
		if im.GetTestof() != nil {
			return nil, errors.New("connect carries both payload and oneof body")
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
			return nil, errors.New("disconnect carries both payload and oneof body")
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
// 先复制外层消息，避免清 oneof、写 payload 时改到调用者手里的那份。
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
