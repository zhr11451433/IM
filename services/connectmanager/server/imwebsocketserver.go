package server

import (
	"IM/services/connectmanager/server/codec"
	"context"
	"errors"
	Log "log"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"google.golang.org/protobuf/proto"
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
	for {
		//ReadMessage()：读取客户端发给服务端的消息。
		//WriteMessage()：服务端向客户端发送消息
		//客户端之间不直接通信，而是通过服务端中转消息。
		msgType, data, err := conn.ReadMessage()
		if err != nil {
			break
		}
		//检查客户端发送的消息是不是二进制消息
		if msgType != websocket.BinaryMessage {
			Log.Printf("expected binary protobuf message")
			return
		}

		// 【未学】将二进制 Protobuf 数据解码为 Go 结构体。
		var msg codec.ImWebsocketMsg
		if err := proto.Unmarshal(data, &msg); err != nil {
			Log.Printf("decode websocket message: %v", err)
			return
		}
		//这里检查客户端发送的命令是不是 Connect。
		//按照这段代码的设计，第一次收到的命令必须是 Connect。
		//为什么？
		//因为服务端需要先知道：
		//- 客户端是谁？
		//- 属于哪个 App？
		//- Token 是否有效？
		//- 设备是什么？
		//完成身份验证和连接初始化后，才能继续处理其他消息
		if msg.Cmd != 0 {
			Log.Printf("first command must be Connect")
			return
		}

		body := msg.GetConnectMsgBody()
		if body == nil {
			Log.Printf("Connect body missing; payload support pending")
			return
		}

		// 不打印 Token。
		Log.Printf("Connect received: appkey=%q, protoId=%q",
			body.Appkey, body.ProtoId)
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
