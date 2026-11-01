package actor

import (
	"context"
	"errors"
	"sync"
	"time"
)

var (
	ErrMethodNotFound = errors.New("actor method not found")
	ErrTimeout        = errors.New("actor call timeout")
	ErrRegistered     = errors.New("actor method already registered")
)

type ctxKey struct {
}

func WithAppKey(ctx context.Context, appKey string) context.Context {
	return context.WithValue(ctx, ctxKey{}, appKey)
}

func AppKeyFrom(ctx context.Context) string {
	v, _ := ctx.Value(ctxKey{}).(string)
	return v
}

type Message struct {
	AppKey   string
	Method   string
	TargetID string
	Payload  any
}

type Handler func(ctx context.Context, msg *Message) (any, error)

type Runtime struct {
	mu       sync.Mutex
	handlers map[string]Handler
	boxes    map[string]chan *envelope
	waiters  sync.Map
	nextID   uint64
}
type envelope struct {
	msg      *Message
	replyID  uint64
	hasReply bool
}
type reply struct {
	value any
	err   error
}

func New() *Runtime {
	return &Runtime{
		handlers: make(map[string]Handler),
		boxes:    make(map[string]chan *envelope),
	}
}

func (r *Runtime) Register(method string, handler Handler) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.handlers[method]; ok {
		return ErrRegistered
	}
	box := make(chan *envelope, 16)
	r.handlers[method] = handler
	r.boxes[method] = box
	//让 loop 持续监听这个消息队列
	go r.loop(handler, box)
	return nil
}

// 持续监听并处理消息
func (r *Runtime) loop(handler Handler, box <-chan *envelope) {
	for env := range box {
		ctx := WithAppKey(context.Background(), env.msg.AppKey)
		//handler 就是之前通过 Register 注册进来的函数
		value, err := handler(ctx, env.msg)
		if !env.hasReply {
			continue
		}
		//找到之前发起请求、正在等待结果的 Goroutine，
		//然后把当前 Handler 的执行结果发送给它
		if waiter, ok := r.waiters.LoadAndDelete(env.replyID); ok {
			//我确定 waiter 里面存储的是一个 chan reply，请把它转换成这个类型
			waiter.(chan reply) <- reply{
				value: value,
				err:   err,
			}
		}
	}
}

func (r *Runtime) newID() uint64 {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.nextID++
	return r.nextID
}

func (r *Runtime) mailbox(method string) (chan *envelope, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	box, ok := r.boxes[method]
	return box, ok
}

func (r *Runtime) newMessage(ctx context.Context, method, targetID string, payload any) *Message {
	return &Message{
		AppKey:   AppKeyFrom(ctx),
		Method:   method,
		TargetID: targetID,
		Payload:  payload,
	}
}

//同步请求，发送消息后，等待对方处理完成并返回结果

func (r *Runtime) Sync(ctx context.Context, method, targetID string, payload any, timeout time.Duration) (any, error) {
	box, ok := r.mailbox(method)
	if !ok {
		return nil, ErrMethodNotFound
	}
	id := r.newID()
	ch := make(chan reply, 1)
	//请求 ID → 对应请求的响应 Channel
	r.waiters.Store(id, ch)
	box <- &envelope{
		msg:      r.newMessage(ctx, method, targetID, payload),
		replyID:  id,
		hasReply: true,
	}
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case resp := <-ch:
		return resp.value, resp.err
	case <-timer.C:
		r.waiters.Delete(id)
		return nil, ErrTimeout
	case <-ctx.Done():
		r.waiters.Delete(id)
		return nil, ctx.Err()
	}
}

//异步发送，只负责把消息放进队列，不等待对方处理完成

func (r *Runtime) Async(ctx context.Context, method, targetID string, payload any) error {
	box, ok := r.mailbox(method)
	if !ok {
		return ErrMethodNotFound
	}
	box <- &envelope{msg: r.newMessage(ctx, method, targetID, payload)}
	return nil
}
