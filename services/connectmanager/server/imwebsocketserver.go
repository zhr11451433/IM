package server

import (
	"IM/services/connectmanager/server/codec"
	"context"
	Log "log"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"google.golang.org/protobuf/proto"
)

type ImWebsocketServer struct {
	lookup AppLookup
	opts   Options

	mu       sync.Mutex
	closed   bool
	conns    map[*websocket.Conn]struct{}
	sessions map[string]Session
	wg       sync.WaitGroup
}

var upgrader = websocket.Upgrader{
	ReadBufferSize:  4196,
	WriteBufferSize: 1124,
}

//为什么 WebSocket 函数还接收 HTTP 参数？
//因为 WebSocket 最开始需要通过 HTTP 握手建立连接，然后升级为 WebSocket

func (s *ImWebsocketServer) ImWsServer(w http.ResponseWriter, r *http.Request) {
	//它尝试将当前 HTTP 连接升级为 WebSocket 连接
	coon, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		Log.Printf("error upgrading websocket connection: %v", err)
		return
	}
	defer func() {
		_ = coon.Close()
	}()
	for {
		//ReadMessage()：读取客户端发给服务端的消息。
		//WriteMessage()：服务端向客户端发送消息
		//客户端之间不直接通信，而是通过服务端中转消息。
		msgType, data, err := coon.ReadMessage()
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
