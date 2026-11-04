package codec

import (
	"testing"

	"google.golang.org/protobuf/proto"
)

// connectPrefix 在测试里独立写死一份，不复用实现里的 knownConnectPrefix：
// 实现如果算错前缀，这个测试就会红。
var connectPrefix = []byte{0x0A, 0x08, 'j', 'u', 'g', '9', 'l', 'e', '1', 'm'}

func mustMarshal(t *testing.T, m proto.Message) []byte {
	t.Helper()
	raw, err := proto.Marshal(m)
	if err != nil {
		t.Fatalf("marshal %T: %v", m, err)
	}
	return raw
}

// maskForConnect 按客户端规则，从 Connect 明文的已知前缀反推 8 字节混淆码。
func maskForConnect(t *testing.T, plain []byte) [8]byte {
	t.Helper()
	if len(plain) < len(connectPrefix) {
		t.Fatalf("connect plain too short: %d", len(plain))
	}
	var mask [8]byte
	for i := range mask {
		mask[i] = plain[i] ^ connectPrefix[i]
	}
	return mask
}

// applyMask 用同一把码混淆任意明文；Connect 和后续的 Disconnect 都用它。
func applyMask(mask [8]byte, plain []byte) []byte {
	out := append([]byte(nil), plain...)
	for i := range out {
		out[i] ^= mask[i%8]
	}
	return out
}

// maskedConnectFrame 造一帧"混淆过的 Connect"，返回整帧和它的混淆码。
func maskedConnectFrame(t *testing.T) (raw []byte, mask [8]byte) {
	t.Helper()
	plain := mustMarshal(t, &ConnectMsgBody{ProtoId: ProtoID, Appkey: "app-a", Token: "token-a"})
	mask = maskForConnect(t, plain)
	raw = mustMarshal(t, &ImWebsocketMsg{
		Version: Version,
		Cmd:     Connect,
		Qos:     1,
		Payload: applyMask(mask, plain),
	})
	return raw, mask
}

func TestDecodeConnectPlaintext(t *testing.T) {
	raw := mustMarshal(t, &ImWebsocketMsg{
		Version: Version,
		Cmd:     Connect,
		Qos:     1,
		Testof: &ImWebsocketMsg_ConnectMsgBody{ConnectMsgBody: &ConnectMsgBody{
			ProtoId: ProtoID,
			Appkey:  "app-a",
			Token:   "token-a",
		}},
	})

	w := &Wire{}
	msg, err := w.Decode(raw)
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if got := msg.GetConnectMsgBody().GetAppkey(); got != "app-a" {
		t.Fatalf("appkey = %q, want %q", got, "app-a")
	}
	if w.masked {
		t.Fatal("明文 Connect 不应该置 masked")
	}
}

func TestDecodeConnectMasked(t *testing.T) {
	raw, _ := maskedConnectFrame(t)

	w := &Wire{}
	msg, err := w.Decode(raw)
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	body := msg.GetConnectMsgBody()
	if body == nil {
		t.Fatal("ConnectMsgBody 没有写进 oneof")
	}
	if body.GetProtoId() != ProtoID || body.GetAppkey() != "app-a" || body.GetToken() != "token-a" {
		t.Fatalf("body = %v", body)
	}
	if msg.GetPayload() != nil {
		t.Fatal("解混淆后应清空 Payload")
	}
	if !w.masked {
		t.Fatal("混淆 Connect 必须置 masked")
	}
}

func TestDecodeDisconnectMasked(t *testing.T) {
	raw, mask := maskedConnectFrame(t)

	w := &Wire{}
	if _, err := w.Decode(raw); err != nil {
		t.Fatalf("connect: %v", err)
	}

	plain := mustMarshal(t, &DisconnectMsgBody{Code: 1, Timestamp: 123})
	dis := mustMarshal(t, &ImWebsocketMsg{
		Version: Version,
		Cmd:     Disconnect,
		Qos:     0,
		Payload: applyMask(mask, plain),
	})

	msg, err := w.Decode(dis)
	if err != nil {
		t.Fatalf("disconnect: %v", err)
	}
	body := msg.GetDisconnectMsgBody()
	if body == nil || body.GetCode() != 1 || body.GetTimestamp() != 123 {
		t.Fatalf("disconnect body = %v", body)
	}
}

func TestDecodeDisconnectMaskedBeforeConnect(t *testing.T) {
	// 没收到 Connect 就用一把码发 Disconnect，必须拒绝
	var mask [8]byte
	copy(mask[:], []byte{1, 2, 3, 4, 5, 6, 7, 8})
	plain := mustMarshal(t, &DisconnectMsgBody{Code: 1})
	raw := mustMarshal(t, &ImWebsocketMsg{
		Version: Version,
		Cmd:     Disconnect,
		Payload: applyMask(mask, plain),
	})

	w := &Wire{}
	if _, err := w.Decode(raw); err == nil {
		t.Fatal("期望报错，实际 nil")
	}
}

func TestDecodeConnectRejectsOneofAndPayload(t *testing.T) {
	raw, _ := maskedConnectFrame(t)

	// 同一帧里既塞 payload 又带 oneof，属于歧义帧
	var outer ImWebsocketMsg
	if err := proto.Unmarshal(raw, &outer); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	outer.Testof = &ImWebsocketMsg_ConnectMsgBody{ConnectMsgBody: &ConnectMsgBody{ProtoId: ProtoID}}
	raw = mustMarshal(t, &outer)

	w := &Wire{}
	if _, err := w.Decode(raw); err == nil {
		t.Fatal("期望报错，实际 nil")
	}
}

func TestDecodeConnectRecoversConsistentMask(t *testing.T) {
	// 混淆码是从已知 protoId 前缀反推的。客户端只要按 8 字节循环 XOR，
	// 用哪把码都能还原；这把码本身不会让前缀校验失败。
	var mask [8]byte
	copy(mask[:], []byte{9, 9, 9, 9, 9, 9, 9, 9})
	plain := mustMarshal(t, &ConnectMsgBody{ProtoId: ProtoID, Appkey: "app-a", Token: "token-a"})
	raw := mustMarshal(t, &ImWebsocketMsg{
		Version: Version,
		Cmd:     Connect,
		Payload: applyMask(mask, plain),
	})

	w := &Wire{}
	msg, err := w.Decode(raw)
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if msg.GetConnectMsgBody().GetToken() != "token-a" {
		t.Fatalf("token = %q", msg.GetConnectMsgBody().GetToken())
	}
	if w.mask != mask || !w.masked {
		t.Fatalf("mask = %v, masked = %v", w.mask, w.masked)
	}
}

func TestDecodeConnectRejectsBrokenPrefix(t *testing.T) {
	// 前 8 字节用来反推码，第 9 字节起才验证循环是否成立。改掉它必须失败，
	// 并且不能把这把未经验证的码记到连接上。
	plain := mustMarshal(t, &ConnectMsgBody{ProtoId: ProtoID, Appkey: "app-a", Token: "token-a"})
	payload := applyMask(maskForConnect(t, plain), plain)
	payload[len(connectPrefix)-1] ^= 0xFF
	raw := mustMarshal(t, &ImWebsocketMsg{
		Version: Version,
		Cmd:     Connect,
		Payload: payload,
	})

	w := &Wire{}
	if _, err := w.Decode(raw); err == nil {
		t.Fatal("期望报错，实际 nil")
	}
	if w.masked {
		t.Fatal("前缀校验失败不应该置 masked")
	}
}

func TestDecodeConnectRejectsShortPayload(t *testing.T) {
	raw := mustMarshal(t, &ImWebsocketMsg{
		Version: Version,
		Cmd:     Connect,
		Payload: []byte{1, 2, 3},
	})

	w := &Wire{}
	if _, err := w.Decode(raw); err == nil {
		t.Fatal("期望报错，实际 nil")
	}
}

func TestDecodeVersionMismatch(t *testing.T) {
	raw := mustMarshal(t, &ImWebsocketMsg{
		Version: 2,
		Cmd:     Ping,
	})

	w := &Wire{}
	if _, err := w.Decode(raw); err == nil {
		t.Fatal("期望报错，实际 nil")
	}
}

func TestDecodePingWithoutBody(t *testing.T) {
	raw := mustMarshal(t, &ImWebsocketMsg{Version: Version, Cmd: Ping, Qos: 1})

	w := &Wire{}
	msg, err := w.Decode(raw)
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if msg.GetCmd() != Ping {
		t.Fatalf("cmd = %d, want %d", msg.GetCmd(), Ping)
	}
}

func TestDecodeUnknownCmdWithPayload(t *testing.T) {
	raw := mustMarshal(t, &ImWebsocketMsg{
		Version: Version,
		Cmd:     3, // Publish，T05 不解
		Payload: []byte{1, 2, 3},
	})

	w := &Wire{}
	if _, err := w.Decode(raw); err == nil {
		t.Fatal("期望报错，实际 nil")
	}
}

func TestDecodeRejectsGarbage(t *testing.T) {
	w := &Wire{}
	if _, err := w.Decode([]byte{0xFF, 0xFF}); err == nil {
		t.Fatal("期望报错，实际 nil")
	}
}

func TestDecodePingRejectsBody(t *testing.T) {
	frames := []*ImWebsocketMsg{
		{Version: Version, Cmd: Ping, Payload: []byte{1}},
		{Version: Version, Cmd: Ping, Testof: &ImWebsocketMsg_ConnectMsgBody{
			ConnectMsgBody: &ConnectMsgBody{ProtoId: ProtoID},
		}},
	}
	for _, frame := range frames {
		w := &Wire{}
		if _, err := w.Decode(mustMarshal(t, frame)); err == nil {
			t.Fatal("Ping 带消息体时期望报错，实际 nil")
		}
	}
}

func TestDecodeUnknownCmdWithoutPayload(t *testing.T) {
	raw := mustMarshal(t, &ImWebsocketMsg{Version: Version, Cmd: 3})

	w := &Wire{}
	msg, err := w.Decode(raw)
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if msg.GetCmd() != 3 {
		t.Fatalf("cmd = %d, want 3", msg.GetCmd())
	}
}

func maskedWire(t *testing.T) *Wire {
	t.Helper()
	raw, _ := maskedConnectFrame(t)
	w := &Wire{}
	if _, err := w.Decode(raw); err != nil {
		t.Fatalf("connect: %v", err)
	}
	return w
}

func TestEncodePlaintextLeavesCallerUntouched(t *testing.T) {
	msg := &ImWebsocketMsg{
		Version: Version,
		Cmd:     ConnectAck,
		Qos:     0,
		Testof: &ImWebsocketMsg_ConnectAckMsgBody{ConnectAckMsgBody: &ConnectAckMsgBody{
			Code: 0, UserId: "u1", Session: "s1", Timestamp: 99,
		}},
	}

	w := &Wire{}
	raw, err := w.Encode(msg)
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}
	if msg.GetConnectAckMsgBody().GetUserId() != "u1" || len(msg.GetPayload()) != 0 {
		t.Fatal("明文 Encode 改动了调用者的消息")
	}

	var got ImWebsocketMsg
	if err := proto.Unmarshal(raw, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	body := got.GetConnectAckMsgBody()
	if body.GetUserId() != "u1" || body.GetSession() != "s1" || len(got.GetPayload()) != 0 {
		t.Fatalf("plaintext frame = %v", &got)
	}
}

func TestEncodeConnectAckMasked(t *testing.T) {
	w := maskedWire(t)
	msg := &ImWebsocketMsg{
		Version: Version,
		Cmd:     ConnectAck,
		Qos:     0,
		Testof: &ImWebsocketMsg_ConnectAckMsgBody{ConnectAckMsgBody: &ConnectAckMsgBody{
			Code: 0, UserId: "u1", Session: "s1", Timestamp: 99,
		}},
	}

	raw, err := w.Encode(msg)
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}
	if msg.GetConnectAckMsgBody().GetSession() != "s1" || len(msg.GetPayload()) != 0 {
		t.Fatal("混淆 Encode 改动了调用者的消息")
	}

	var outer ImWebsocketMsg
	if err := proto.Unmarshal(raw, &outer); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if outer.GetCmd() != ConnectAck || outer.GetTestof() != nil || len(outer.GetPayload()) == 0 {
		t.Fatalf("masked frame = cmd %d payload %d oneof %v", outer.GetCmd(), len(outer.GetPayload()), outer.GetTestof())
	}
	plain := append([]byte(nil), outer.GetPayload()...)
	xorBytes(w.mask, plain)
	var body ConnectAckMsgBody
	if err := proto.Unmarshal(plain, &body); err != nil {
		t.Fatalf("unmarshal body: %v", err)
	}
	if body.GetUserId() != "u1" || body.GetSession() != "s1" || body.GetTimestamp() != 99 {
		t.Fatalf("body = %v", &body)
	}
}

func TestEncodeDisconnectMaskedRoundTrip(t *testing.T) {
	w := maskedWire(t)
	raw, err := w.Encode(&ImWebsocketMsg{
		Version: Version,
		Cmd:     Disconnect,
		Testof: &ImWebsocketMsg_DisconnectMsgBody{DisconnectMsgBody: &DisconnectMsgBody{
			Code: 7, Timestamp: 11,
		}},
	})
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}

	msg, err := w.Decode(raw)
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	body := msg.GetDisconnectMsgBody()
	if body.GetCode() != 7 || body.GetTimestamp() != 11 || len(msg.GetPayload()) != 0 {
		t.Fatalf("disconnect = %v", body)
	}
}

func TestEncodePongMaskedHasNoBody(t *testing.T) {
	w := maskedWire(t)
	raw, err := w.Encode(&ImWebsocketMsg{Version: Version, Cmd: Pong, Qos: 0})
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}

	var got ImWebsocketMsg
	if err := proto.Unmarshal(raw, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got.GetCmd() != Pong || len(got.GetPayload()) != 0 || got.GetTestof() != nil {
		t.Fatalf("pong = cmd %d payload %d", got.GetCmd(), len(got.GetPayload()))
	}
}

func TestEncodeMaskedRejectsUnsupported(t *testing.T) {
	w := maskedWire(t)
	frames := []*ImWebsocketMsg{
		nil,
		{Version: Version, Cmd: Ping},
		{Version: Version, Cmd: ConnectAck},
		{Version: Version, Cmd: Pong, Payload: []byte{1}},
	}
	for _, frame := range frames {
		if _, err := w.Encode(frame); err == nil {
			t.Fatalf("cmd %v 期望报错，实际 nil", frame.GetCmd())
		}
	}
}
