package server

import (
	Log "log"
	"net/http"

	"github.com/gorilla/websocket"
)

type ImWebsocketServer struct{}

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
		msgType, data, err := coon.ReadMessage()
		if err != nil {
			break
		}
		Log.Printf("msgType=%d, msgSize=%d", msgType, len(data))
	}
}
