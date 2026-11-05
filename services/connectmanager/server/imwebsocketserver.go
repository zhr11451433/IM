package server

import (
	"IM/services/connectmanager/server/codec"
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	Log "log"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

//main：注入数据库查询与超时配置
//  ↓
//ImWsServer：升级连接，登记到连接集合
//  ↓
//首次读取：Wire.Decode → Connect 参数 → 查询应用 → ParseWithExpiry
//  ↓
//成功：生成 session → 登记身份 → Wire.Encode → ConnectAck
//失败：失败 ConnectAck → return
//  ↓
//已登录循环：Ping → Pong；Disconnect / 错误 / 超时 → return
//  ↓
//统一 defer：关闭连接，删除连接及 session
//进程退出：ws.Close → 关闭全部连接 → 等待处理函数退出

type ImWebsocketServer struct {
	lookup AppLookup
	opts   Options

	mu     sync.Mutex
	closed bool
	//因为你这里只关心某个 WebSocket 连接是否存在于集合中，不需要额外保存值
	//模拟set
	conns    map[*websocket.Conn]struct{}
	sessions map[*websocket.Conn]Session
	wg       sync.WaitGroup
}

var upgrader = websocket.Upgrader{
	ReadBufferSize:  4196,
	WriteBufferSize: 1124,
}

func New(lookup AppLookup, opts Options) (*ImWebsocketServer, error) {
	if lookup == nil {
		return nil, errors.New("AppLookup is required")
	}
	if opts.HeartbeatTimeout > 3600*time.Second {
		return nil, errors.New("HeartbeatTimeout must not exceed 3600 seconds")
	}
	//客户端建立 WebSocket 连接后，必须在多长时间内完成身份认证
	if opts.ConnectTimeout <= 0 {
		return nil, errors.New("ConnectTimeout must be greater than 0")
	}
	//客户端和服务端定期互相发送的：
	//你还活着吗？
	//我还活着！
	if opts.HeartbeatInterval < time.Second {
		return nil, errors.New("HeartbeatInterval must be at least 1 second")
	}
	//HeartbeatTimeout
	//如果客户端很久没有发送心跳，服务端应该等多久才认定它掉线
	if opts.HeartbeatTimeout <= opts.HeartbeatInterval {
		return nil, errors.New("HeartbeatTimeout must be greater than HeartbeatInterval")
	}
	//心跳秒数必须是整数，避免 ACK 向下截断
	if opts.HeartbeatInterval%time.Second != 0 {
		return nil, errors.New("HeartbeatInterval must be an integer number of seconds")
	}
	//心跳秒数必须是整数，避免 ACK 向下截断
	if opts.HeartbeatTimeout%time.Second != 0 {
		return nil, errors.New("HeartbeatTimeout must be an integer number of seconds")
	}
	return &ImWebsocketServer{
		lookup:   lookup,
		opts:     opts,
		conns:    make(map[*websocket.Conn]struct{}),
		sessions: make(map[*websocket.Conn]Session),
	}, nil
}

//为什么 WebSocket 函数还接收 HTTP 参数？
//因为 WebSocket 最开始需要通过 HTTP 握手建立连接，然后升级为 WebSocket

func (s *ImWebsocketServer) ImWsServer(w http.ResponseWriter, r *http.Request) {
	//它尝试将当前 HTTP 连接升级为 WebSocket 连接
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		Log.Printf("error upgrading websocket connection: %v", err)
		return
	}
	s.mu.Lock()
	//如果服务器已经关闭，就不允许新连接加入
	if s.closed {
		s.mu.Unlock()
		_ = conn.Close()
		return
	}
	s.conns[conn] = struct{}{}
	s.wg.Add(1)
	s.mu.Unlock()

	defer s.wg.Done()
	defer func() {
		_ = conn.Close()

		s.mu.Lock()
		delete(s.conns, conn)
		delete(s.sessions, conn)
		s.mu.Unlock()
	}()
	conn.SetReadLimit(64 * 1024)
	connectDeadline := time.Now().Add(s.opts.ConnectTimeout)
	if err := conn.SetReadDeadline(connectDeadline); err != nil {
		return
	}
	wire := &codec.Wire{}
	read := func() (*codec.ImWebsocketMsg, error) {
		msgType, data, err := conn.ReadMessage()
		if err != nil {
			return nil, err
		}
		// 只接受二进制消息
		if msgType != websocket.BinaryMessage {
			return nil, errors.New("expected binary protobuf message")
		}
		msg, err := wire.Decode(data)
		if err != nil {
			return nil, fmt.Errorf("decode websocket message: %w", err)
		}
		return msg, nil
	}
	write := func(msg *codec.ImWebsocketMsg) error {
		data, err := wire.Encode(msg)
		if err != nil {
			return fmt.Errorf("encode im websocket message: %w", err)
		}
		err = conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
		if err != nil {
			return fmt.Errorf("set write im websocket message: %w", err)
		}
		// 发送二进制消息
		if err := conn.WriteMessage(websocket.BinaryMessage, data); err != nil {
			return fmt.Errorf("write websocket message: %w", err)
		}
		return nil
	}
	msg, err := read()
	if err != nil || msg.Cmd != codec.Connect {
		return
	}
	ctx, cancel := context.WithDeadline(r.Context(), connectDeadline)
	userID, code, err := s.authenticate(ctx, msg.GetConnectMsgBody())
	ctxErr := ctx.Err()
	cancel()
	if err != nil || ctxErr != nil {
		return
	}
	sessionID := ""
	if code == 0 {
		p := make([]byte, 16)
		if _, err := rand.Read(p); err != nil {
			return
		}
		sessionID = hex.EncodeToString(p)
		s.mu.Lock()
		if s.closed {
			s.mu.Unlock()
			return
		}
		body := msg.GetConnectMsgBody()
		s.sessions[conn] = Session{
			AppKey:   body.Appkey,
			UserID:   userID,
			DeviceID: body.DeviceId,
			Platform: body.Platform,
		}
		s.mu.Unlock()
	}
	// 构造 ConnectAck
	ack := &codec.ImWebsocketMsg{
		Version: codec.Version,
		Cmd:     codec.ConnectAck,
		Testof: &codec.ImWebsocketMsg_ConnectAckMsgBody{
			ConnectAckMsgBody: &codec.ConnectAckMsgBody{
				Code:      code,
				UserId:    userID,
				Session:   sessionID,
				Timestamp: time.Now().UnixMilli(),
				Conf: &codec.ConnectConf{
					HeartbeatInterval: int32(s.opts.HeartbeatInterval / time.Second),
					HeartbeatTimeout:  int32(s.opts.HeartbeatTimeout / time.Second),
				},
			},
		},
	}
	// 发送 ConnectAck
	if err := write(ack); err != nil {
		return
	}

	// 鉴权失败，发送失败 ACK 后退出
	if code != 0 {
		return
	}
	// 进入已登录消息循环
	for {
		// 每次读取前刷新读取截止时间
		if err := conn.SetReadDeadline(time.Now().Add(s.opts.HeartbeatTimeout)); err != nil {
			return
		}

		msg, err := read()
		if err != nil {
			return
		}

		switch msg.GetCmd() {
		case codec.Ping:
			// Ping 不允许携带 one of 或 payload
			if msg.GetTestof() != nil || len(msg.GetPayload()) != 0 {
				return
			}

			// 回复 IM 层 Ping ACK
			ack := &codec.ImWebsocketMsg{
				Version: 1,
				Cmd:     9,
				Qos:     0,
			}

			if err := write(ack); err != nil {
				return
			}

		case codec.Disconnect:
			return

		default:
			// 暂不处理其他业务消息，也不接受再次 Connect
			return
		}
	}
}

type AuthApp struct {
	SecureKey        string
	EffectiveMinutes int
}

type AppLookup func(context.Context, string) (AuthApp, bool, error)

type Options struct {
	ConnectTimeout    time.Duration
	HeartbeatInterval time.Duration
	HeartbeatTimeout  time.Duration
}
