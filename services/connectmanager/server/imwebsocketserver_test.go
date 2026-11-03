package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"IM/services/connectmanager/server/codec"

	"github.com/gorilla/websocket"
	"google.golang.org/protobuf/proto"
)

func TestWebSocketConnectDecode(t *testing.T) {
	s := &ImWebsocketServer{}
	mux := http.NewServeMux()
	mux.HandleFunc("/im", s.ImWsServer)

	httpServer := httptest.NewServer(mux)
	defer httpServer.Close()

	url := "ws" + strings.TrimPrefix(httpServer.URL, "http") + "/im"
	conn, _, err := websocket.DefaultDialer.Dial(url, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()

	msg := &codec.ImWebsocketMsg{
		Version: 1,
		Cmd:     0,
		Qos:     1,
		Testof: &codec.ImWebsocketMsg_ConnectMsgBody{
			ConnectMsgBody: &codec.ConnectMsgBody{
				ProtoId: "jug9le1m",
				Appkey:  "app-a",
				Token:   "decode-only-not-a-real-token",
			},
		},
	}

	data, err := proto.Marshal(msg)
	if err != nil {
		t.Fatal(err)
	}
	if err := conn.WriteMessage(websocket.BinaryMessage, data); err != nil {
		t.Fatal(err)
	}

	// 同一连接中，控制 Ping 排在 Connect 数据后。
	// 收到 Pong，表明服务端没有在处理 Connect 时关闭连接，
	// 并且已继续读取后续帧。
	pongReceived := false
	conn.SetPongHandler(func(string) error {
		pongReceived = true
		return conn.Close()
	})
	if err := conn.SetReadDeadline(time.Now().Add(3 * time.Second)); err != nil {
		t.Fatal(err)
	}
	if err := conn.WriteControl(
		websocket.PingMessage, []byte("check"),
		time.Now().Add(time.Second),
	); err != nil {
		t.Fatal(err)
	}

	_, _, _ = conn.ReadMessage()
	if !pongReceived {
		t.Fatal("connection closed or timed out before control Pong")
	}
}
