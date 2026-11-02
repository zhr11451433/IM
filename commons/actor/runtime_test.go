package actor

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

func TestSyncEcho(t *testing.T) {
	rt := New()
	if err := rt.Register("echo", func(ctx context.Context, msg *Message) (any, error) {
		return AppKeyFrom(ctx) + ":" + msg.Payload.(string), nil
	}); err != nil {
		t.Fatal(err)
	}
	ctx := WithAppKey(context.Background(), "app-1")
	got, err := rt.Sync(ctx, "echo", "u1", "ping", time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if got != "app-1:ping" {
		t.Fatalf("got %v", got)
	}
}

func TestMethodNotFound(t *testing.T) {
	rt := New()
	_, err := rt.Sync(context.Background(), "missing", "u1", nil, time.Second)
	if !errors.Is(err, ErrMethodNotFound) {
		t.Fatalf("got %v", err)
	}
}

func TestSyncTimeout(t *testing.T) {
	rt := New()
	if err := rt.Register("slow", func(ctx context.Context, msg *Message) (any, error) {
		time.Sleep(200 * time.Millisecond)
		return "late", nil
	}); err != nil {
		t.Fatal(err)
	}
	_, err := rt.Sync(context.Background(), "slow", "u1", nil, 20*time.Millisecond)
	if !errors.Is(err, ErrTimeout) {
		t.Fatalf("got %v", err)
	}
}

// 邮箱容量与 Register 中的缓冲一致。handler 占住处理循环后，再塞满这条缓冲。
const mailboxBuffer = 16

func fillBlockedMailbox(t *testing.T, rt *Runtime, method string) (release func()) {
	t.Helper()
	started := make(chan struct{})
	var once sync.Once
	unblock := make(chan struct{})
	if err := rt.Register(method, func(ctx context.Context, msg *Message) (any, error) {
		once.Do(func() { close(started) })
		<-unblock
		return "ok", nil
	}); err != nil {
		t.Fatal(err)
	}
	bg := make(chan struct{})
	go func() {
		defer close(bg)
		_, _ = rt.Sync(context.Background(), method, "u1", "first", 5*time.Second)
	}()
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("handler did not start")
	}
	for i := 0; i < mailboxBuffer; i++ {
		if err := rt.Async(context.Background(), method, "u1", i); err != nil {
			t.Fatal(err)
		}
	}
	return func() {
		close(unblock)
		select {
		case <-bg:
		case <-time.After(5 * time.Second):
			t.Error("background sync did not finish")
		}
	}
}

func TestSyncTimeoutWhileMailboxFull(t *testing.T) {
	rt := New()
	release := fillBlockedMailbox(t, rt, "block")
	defer release()

	start := time.Now()
	_, err := rt.Sync(context.Background(), "block", "u1", "overflow", 50*time.Millisecond)
	elapsed := time.Since(start)
	if !errors.Is(err, ErrTimeout) {
		t.Fatalf("got %v", err)
	}
	if elapsed > 500*time.Millisecond {
		t.Fatalf("enqueue blocked for %s", elapsed)
	}
	n := 0
	rt.waiters.Range(func(_, _ any) bool {
		n++
		return true
	})
	// 只有占住 handler 的那次调用还在等；本次入队失败的 waiter 必须已删除。
	if n != 1 {
		t.Fatalf("waiters = %d, want 1", n)
	}
}

func TestSyncCancelWhileMailboxFull(t *testing.T) {
	rt := New()
	release := fillBlockedMailbox(t, rt, "block")
	defer release()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	start := time.Now()
	_, err := rt.Sync(ctx, "block", "u1", "overflow", time.Second)
	elapsed := time.Since(start)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("got %v", err)
	}
	if elapsed > 500*time.Millisecond {
		t.Fatalf("enqueue blocked for %s", elapsed)
	}
	n := 0
	rt.waiters.Range(func(_, _ any) bool {
		n++
		return true
	})
	if n != 1 {
		t.Fatalf("waiters = %d, want 1", n)
	}
}

func TestAsyncDoesNotReturnResult(t *testing.T) {
	rt := New()
	done := make(chan struct{})
	if err := rt.Register("work", func(ctx context.Context, msg *Message) (any, error) {
		close(done)
		return "hidden", nil
	}); err != nil {
		t.Fatal(err)
	}
	if err := rt.Async(WithAppKey(context.Background(), "app-1"), "work", "u1", "x"); err != nil {
		t.Fatal(err)
	}
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("async handler did not run")
	}
}
