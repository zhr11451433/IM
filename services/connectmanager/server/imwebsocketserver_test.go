package server

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"IM/commons/token"
	"IM/services/connectmanager/server/codec"

	"github.com/gorilla/websocket"
	"google.golang.org/protobuf/proto"
)

func send(t *testing.T, c *websocket.Conn, m *codec.ImWebsocketMsg) {
	t.Helper()
	b, err := proto.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	if err := c.WriteMessage(websocket.BinaryMessage, b); err != nil {
		t.Fatal(err)
	}
	t.Logf("send: cmd=%d bytes=%d", m.Cmd, len(b))
}

func receive(t *testing.T, c *websocket.Conn) *codec.ImWebsocketMsg {
	t.Helper()
	if err := c.SetReadDeadline(time.Now().Add(3 * time.Second)); err != nil {
		t.Fatal(err)
	}
	kind, b, err := c.ReadMessage()
	if err != nil {
		t.Fatal(err)
	}
	if kind != websocket.BinaryMessage {
		t.Fatal("expected binary message")
	}
	var m codec.ImWebsocketMsg
	if err := proto.Unmarshal(b, &m); err != nil {
		t.Fatal(err)
	}
	return &m
}

func waitEmpty(t *testing.T, s *ImWebsocketServer) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		s.mu.Lock()
		n := len(s.conns) + len(s.sessions)
		s.mu.Unlock()
		if n == 0 {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("connections or sessions were not cleaned")
}

func expectClosed(t *testing.T, c *websocket.Conn) {
	t.Helper()
	_ = c.SetReadDeadline(time.Now().Add(3 * time.Second))
	_, _, err := c.ReadMessage()
	if err == nil {
		t.Fatal("expected connection closure")
	}
	var netErr interface{ Timeout() bool }
	if errors.As(err, &netErr) && netErr.Timeout() {
		t.Fatalf("client timed out instead of observing closure: %v", err)
	}
}

func connectMessage(tok, app string) *codec.ImWebsocketMsg {
	return &codec.ImWebsocketMsg{
		Version: 1, Cmd: 0, Qos: 1,
		Testof: &codec.ImWebsocketMsg_ConnectMsgBody{
			ConnectMsgBody: &codec.ConnectMsgBody{
				ProtoId: "jug9le1m", Appkey: app, Token: tok,
			},
		},
	}
}

func TestT05(t *testing.T) {
	const key = "1234567890abcdef"
	issue := func(at time.Time) string {
		tok, err := token.Issue("app-a", key, "u1", at)
		if err != nil {
			t.Fatal(err)
		}
		return tok
	}
	valid := issue(time.Now())
	old := issue(time.Now().Add(-2 * time.Minute))

	tests := []struct {
		name, app, tok, mode string
		code                 int32
	}{
		{"心跳与主动断开", "app-a", valid, "heartbeat", 0},
		{"断网清理", "app-a", valid, "network", 0},
		{"空闲超时", "app-a", valid, "idle", 0},
		{"服务关闭", "app-a", valid, "shutdown", 0},
		{"混淆连接", "app-a", valid, "masked", 0},
		{"非法Token", "app-a", "invalid", "", 11004},
		{"跨应用", "app-b", valid, "", 11005},
		{"过期", "app-a", old, "", 11006},
		{"应用不存在", "unknown", valid, "", 11003},
		{"缺失参数", "app-a", "", "", 11015},
		{"数据库故障", "db-error", valid, "db-error", 0},
		{"首帧超时", "", "", "no-connect", 0},
		{"未登录Ping", "", "", "early-ping", 0},
		{"非法Protobuf", "", "", "bad-proto", 0},
		{"文本帧", "", "", "text", 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s, err := New(
				func(ctx context.Context, app string) (AuthApp, bool, error) {
					if app == "db-error" {
						return AuthApp{}, false, errors.New("database unavailable")
					}
					return AuthApp{key, 1},
						app == "app-a" || app == "app-b", nil
				},
				Options{
					ConnectTimeout:    300 * time.Millisecond,
					HeartbeatInterval: time.Second,
					HeartbeatTimeout:  2 * time.Second,
				},
			)
			if err != nil {
				t.Fatal(err)
			}
			mux := http.NewServeMux()
			mux.HandleFunc("/im", s.ImWsServer)
			hs := httptest.NewServer(mux)
			t.Cleanup(hs.Close)
			t.Cleanup(s.Close)

			url := "ws" + strings.TrimPrefix(hs.URL, "http") + "/im"
			c, _, err := websocket.DefaultDialer.Dial(url, nil)
			if err != nil {
				t.Fatal(err)
			}
			defer c.Close()

			switch tt.mode {
			case "no-connect":
				expectClosed(t, c)
				waitEmpty(t, s)
				return
			case "early-ping":
				send(t, c, &codec.ImWebsocketMsg{Version: 1, Cmd: 8, Qos: 1})
				expectClosed(t, c)
				waitEmpty(t, s)
				return
			case "bad-proto", "text":
				kind := websocket.BinaryMessage
				if tt.mode == "text" {
					kind = websocket.TextMessage
				}
				if err := c.WriteMessage(kind, []byte{0xff}); err != nil {
					t.Fatal(err)
				}
				expectClosed(t, c)
				waitEmpty(t, s)
				return
			}

			msg := connectMessage(tt.tok, tt.app)
			mask := [8]byte{1, 2, 3, 4, 5, 6, 7, 8}
			if tt.mode == "masked" {
				b, err := proto.Marshal(msg.GetConnectMsgBody())
				if err != nil {
					t.Fatal(err)
				}
				for i := range b {
					b[i] ^= mask[i%8]
				}
				msg.Testof, msg.Payload = nil, b
			}
			send(t, c, msg)

			if tt.mode == "db-error" {
				expectClosed(t, c)
				waitEmpty(t, s)
				return
			}
			reply := receive(t, c)
			if reply.Cmd != 1 || reply.Qos != 0 {
				t.Fatalf("unexpected ACK: %v", reply)
			}
			ack := reply.GetConnectAckMsgBody()
			if tt.mode == "masked" {
				if reply.Testof != nil || len(reply.Payload) == 0 {
					t.Fatal("expected masked ACK payload")
				}
				b := append([]byte(nil), reply.Payload...)
				for i := range b {
					b[i] ^= mask[i%8]
				}
				ack = &codec.ConnectAckMsgBody{}
				if err := proto.Unmarshal(b, ack); err != nil {
					t.Fatal(err)
				}
			}
			if ack == nil || ack.Code != tt.code {
				t.Fatalf("unexpected ACK body: %v", ack)
			}
			if tt.code != 0 {
				expectClosed(t, c)
				waitEmpty(t, s)
				return
			}
			if ack.UserId != "u1" || ack.Session == "" ||
				ack.Timestamp <= 0 || ack.Conf == nil ||
				ack.Conf.HeartbeatInterval != 1 ||
				ack.Conf.HeartbeatTimeout != 2 {
				t.Fatalf("incomplete ACK: %v", ack)
			}
			if s.Count("app-a", "u1") != 1 ||
				s.Count("app-b", "u1") != 0 {
				t.Fatal("session identity or tenant isolation is wrong")
			}

			switch tt.mode {
			case "heartbeat":
				// 总时长超过空闲超时，持续 Ping 应保持连接。
				for range 4 {
					time.Sleep(500 * time.Millisecond)
					send(t, c, &codec.ImWebsocketMsg{Version: 1, Cmd: 8, Qos: 1})
					if receive(t, c).Cmd != 9 {
						t.Fatal("expected IM Pong")
					}
				}
				send(t, c, &codec.ImWebsocketMsg{Version: 1, Cmd: 2})
				expectClosed(t, c)
			case "masked":
				send(t, c, &codec.ImWebsocketMsg{Version: 1, Cmd: 8, Qos: 1})
				if receive(t, c).Cmd != 9 {
					t.Fatal("expected IM Pong")
				}
				_ = c.Close()
			case "network":
				_ = c.Close()
			case "shutdown":
				s.Close()
				expectClosed(t, c)
			case "idle":
				expectClosed(t, c)
			}
			waitEmpty(t, s)
		})
	}
}

// 真实运行服务验收：默认跳过，设置环境变量后执行。
func TestT05RealServer(t *testing.T) {
	url := os.Getenv("IM_WS_URL")
	if url == "" {
		t.Skip("set IM_WS_URL, IM_APPKEY and IM_TOKEN for real-server validation")
	}
	app, tok := os.Getenv("IM_APPKEY"), os.Getenv("IM_TOKEN")
	if app == "" || tok == "" {
		t.Fatal("IM_APPKEY and IM_TOKEN are required")
	}
	c, _, err := websocket.DefaultDialer.Dial(url, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	send(t, c, connectMessage(tok, app))
	ack := receive(t, c).GetConnectAckMsgBody()
	if ack == nil {
		t.Fatal("missing ConnectAck")
	}
	t.Logf("ConnectAck: code=%d user=%q session=%q",
		ack.Code, ack.UserId, ack.Session)
	if ack.Code != 0 {
		t.Fatalf("authentication rejected: %d", ack.Code)
	}
	if ack.UserId == "" || ack.Session == "" || ack.Conf == nil {
		t.Fatal("incomplete successful ACK")
	}
	for range 3 {
		send(t, c, &codec.ImWebsocketMsg{Version: 1, Cmd: 8, Qos: 1})
		if receive(t, c).Cmd != 9 {
			t.Fatal("expected IM Pong")
		}
	}
	send(t, c, &codec.ImWebsocketMsg{Version: 1, Cmd: 2})
	expectClosed(t, c)
}
