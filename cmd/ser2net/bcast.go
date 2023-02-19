package main

import (
	"context"
	"reflect"
	"sync"

	"github.com/jclark/gps2phc/logctx"
	"golang.org/x/exp/constraints"
)

type subscriber struct {
	c             chan<- []byte
	nextSendIndex int
}

type bcast struct {
	subscribe   chan chan<- []byte
	unsubscribe chan chan<- []byte
	msg         chan []byte
	q           [][]byte
	nextQIndex  int
	subscribers []subscriber
}

func newBcast() *bcast {
	return &bcast{
		subscribe:   make(chan chan<- []byte),
		unsubscribe: make(chan chan<- []byte),
		msg:         make(chan []byte, 1),
	}
}

// This should be called in a goroutine.
// It will keep running so long as any of the following apply
// - there are existing subscribers
// - the subscriber channel is still open
// - the msg channel is still open
//
// It will close each subscriber channel when the msg channel is closed or the context is done.
// It will call wg.Done() just before it returns.
func (b *bcast) run(ctx context.Context, wg *sync.WaitGroup) {
	defer func() {
		logctx.FromContext(ctx).Debug("bcastDone")
		wg.Done()
	}()
	lg := logctx.FromContext(ctx)
	msg := b.msg
	subscribe := b.subscribe
	done := ctx.Done()
	// closed is true after we close subscriber channels
	closed := false
	for subscribe != nil || msg != nil || len(b.subscribers) > 0 {
		cases := []reflect.SelectCase{
			{Dir: reflect.SelectRecv, Chan: reflect.ValueOf(subscribe)},
			{Dir: reflect.SelectRecv, Chan: reflect.ValueOf(b.unsubscribe)},
			{Dir: reflect.SelectRecv, Chan: reflect.ValueOf(msg)},
			{Dir: reflect.SelectRecv, Chan: reflect.ValueOf(done)},
		}
		subscribersToDo := []*subscriber{}
		qStartIndex := b.nextQIndex - len(b.q)
		for i := range b.subscribers {
			s := &b.subscribers[i]
			if !closed && s.nextSendIndex < b.nextQIndex {
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
		lg.Debug("bcastSelect", "chosen", chosen)
		switch chosen {
		case 0: // subscribe
			if !ok {
				subscribe = nil
				break
			}
			s := recv.Interface().(chan<- []byte)
			if closed {
				close(s)
				break
			}
			b.subscribers = append(b.subscribers, subscriber{s, b.nextQIndex})
			lg.Debug("subscribe", "chan", s)
		case 1: // unsubscribe
			s := recv.Interface().(chan<- []byte)
			i := b.subscriberIndex(s)
			if i < 0 {
				break
			}
			b.subscribers = append(b.subscribers[:i], b.subscribers[i+1:]...)
			lg.Debug("unsubscribe", "chan", s)
		case 2: // msg
			if !ok {
				msg = nil
				break
			}
			m := recv.Interface().([]byte)
			if len(b.subscribers) != 0 {
				b.q = append(b.q, m)
				b.nextQIndex++
			}
		case 3: // Done
			done = nil
		default: // send to a subscriber
			subscribersToDo[chosen-4].nextSendIndex++
			b.trimQ()
		}
		if !closed && (msg == nil || done == nil) {
			closed = true
			for _, s := range b.subscribers {
				close(s.c)
			}
		}
	}
}

func (b *bcast) trimQ() {
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

func (b *bcast) subscriberIndex(c chan<- []byte) int {
	for i, s := range b.subscribers {
		if s.c == c {
			return i
		}
	}
	return -1
}

func min[T constraints.Ordered](a, b T) T {
	if a < b {
		return a
	}
	return b
}
