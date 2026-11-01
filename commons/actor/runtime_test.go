package actor

import (
	"context"
	"errors"
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
