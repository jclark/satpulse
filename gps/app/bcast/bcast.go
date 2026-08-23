package bcast

import (
	"context"
	"log/slog"
	"reflect"
	"sync"
)

type subscriber[T any] struct {
	c             chan T
	nextSendIndex int
}

type Bcast[T any] struct {
	mu          sync.Mutex
	closed      bool
	subscribe   chan chan T
	unsubscribe chan (<-chan T)
	msg         <-chan T
	q           []T
	nextQIndex  int
	subscribers []subscriber[T]
}

func New[T any](msg <-chan T) *Bcast[T] {
	return &Bcast[T]{
		// use buffer size of 1 here, because we hold the mutex while sending on the channel
		subscribe:   make(chan chan T, 1),
		unsubscribe: make(chan (<-chan T), 1),
		msg:         msg,
	}
}

// Close stops further broadcasting.
// Existing subscribers will be closed.
// Multiple Close calls are safe.
func (b *Bcast[T]) Close() {
	b.mu.Lock()
	defer b.mu.Unlock()
	if !b.closed {
		b.closed = true
		close(b.subscribe)
	}
}

func (b *Bcast[T]) Subscribe() <-chan T {
	ch := make(chan T)
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed {
		// return a closed channel
		close(ch)
	} else {
		b.subscribe <- ch
	}
	return ch
}

func (b *Bcast[T]) Unsubscribe(ch <-chan T) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if !b.closed {
		b.unsubscribe <- ch
	}
}

// This should be called in a goroutine.
// It will exit when both
// - Close has being called on Bcast
// - the msg channel is closed
// This should be run in a separate goroutine
func (b *Bcast[T]) Run(ctx context.Context, lg *slog.Logger) {
	defer lg.Debug("about to exit broadcast goroutine")
	msg := b.msg
	subscribe := b.subscribe
	done := ctx.Done()
	// closing is true after we close subscriber channels
	closing := false
	for subscribe != nil || msg != nil {
		cases := []reflect.SelectCase{
			{Dir: reflect.SelectRecv, Chan: reflect.ValueOf(subscribe)},
			{Dir: reflect.SelectRecv, Chan: reflect.ValueOf(b.unsubscribe)},
			{Dir: reflect.SelectRecv, Chan: reflect.ValueOf(msg)},
			{Dir: reflect.SelectRecv, Chan: reflect.ValueOf(done)},
		}
		subscribersToDo := []*subscriber[T]{}
		qStartIndex := b.nextQIndex - len(b.q)
		for i := range b.subscribers {
			s := &b.subscribers[i]
			if !closing && s.nextSendIndex < b.nextQIndex {
				subscribersToDo = append(subscribersToDo, s)
				sc := reflect.SelectCase{
					Dir:  reflect.SelectSend,
					Chan: reflect.ValueOf(s.c),
					Send: reflect.ValueOf(b.q[s.nextSendIndex-qStartIndex]),
				}
				cases = append(cases, sc)
			}
		}
		chosen, recv, ok := reflect.Select(cases)
		switch chosen {
		case 0: // subscribe
			if !ok {
				subscribe = nil
				break
			}
			s := recv.Interface().(chan T)
			if closing {
				close(s)
				break
			}
			b.subscribers = append(b.subscribers, subscriber[T]{s, b.nextQIndex})
			lg.Debug("received request to subscribe to broadcast data", "chan", s)
		case 1: // unsubscribe
			s := recv.Interface().(<-chan T)
			i := b.subscriberIndex(s)
			if i < 0 {
				break
			}
			b.subscribers = append(b.subscribers[:i], b.subscribers[i+1:]...)
			lg.Debug("received request to unsubscribe to broadcast data", "chan", s)
		case 2: // msg
			if !ok {
				msg = nil
				lg.Debug("broadcast input message channel closed")
				break
			}
			m := recv.Interface().(T)
			if len(b.subscribers) != 0 {
				b.q = append(b.q, m)
				b.nextQIndex++
			}
		case 3: // Done
			done = nil
			lg.Debug("broadcast goroutine received cancellation")
		default: // send to a subscriber
			subscribersToDo[chosen-4].nextSendIndex++
			b.trimQ()
		}
		if !closing && (msg == nil || subscribe == nil || done == nil) {
			closing = true
			lg.Debug("broadcast goroutine closing down")
			for _, s := range b.subscribers {
				close(s.c)
			}
		}
	}
}

func (b *Bcast[T]) trimQ() {
	needIndex := b.nextQIndex
	for _, s := range b.subscribers {
		needIndex = min(needIndex, s.nextSendIndex)
	}
	nNeed := b.nextQIndex - needIndex
	if nNeed == 0 {
		b.q = b.q[:0]
	} else if nDiscard := len(b.q) - nNeed; nDiscard > 0 {
		oldQ := b.q[nDiscard:]
		b.q = b.q[0:nNeed]
		copy(b.q, oldQ)
	}
}

func (b *Bcast[T]) subscriberIndex(c <-chan T) int {
	for i, s := range b.subscribers {
		if s.c == c {
			return i
		}
	}
	return -1
}
