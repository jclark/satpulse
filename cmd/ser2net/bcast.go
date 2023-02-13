package main

import (
	"context"
	"reflect"

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

func (b *bcast) run(ctx context.Context) {
	defer func() {
		for _, s := range b.subscribers {
			close(s.c)
		}
	}()
	lg := logctx.FromContext(ctx)
	for {
		cases := []reflect.SelectCase{
			{Dir: reflect.SelectRecv, Chan: reflect.ValueOf(b.subscribe)},
			{Dir: reflect.SelectRecv, Chan: reflect.ValueOf(b.unsubscribe)},
			{Dir: reflect.SelectRecv, Chan: reflect.ValueOf(b.msg)},
			{Dir: reflect.SelectRecv, Chan: reflect.ValueOf(ctx.Done())},
		}
		subscribersToDo := []*subscriber{}
		qStartIndex := b.nextQIndex - len(b.q)
		for i := 0; i < len(b.subscribers); i++ {
			s := &b.subscribers[i]
			if s.nextSendIndex < b.nextQIndex {
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
			s := recv.Interface().(chan<- []byte)
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
				return
			}
			m := recv.Interface().([]byte)
			if len(b.subscribers) != 0 {
				b.q = append(b.q, m)
				b.nextQIndex++
			}
		case 3: // Done
			return
		default: // send to a subscriber
			subscribersToDo[chosen-4].nextSendIndex++
			b.trimQ()
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
