package serio

import (
	"context"
	"reflect"
	"sync"

	"github.com/jclark/gps2phc/logctx"
	"github.com/jclark/gps2phc/scan"
	"golang.org/x/exp/constraints"
)

type subscriber struct {
	c             chan scan.Frame
	nextSendIndex int
}

type Bcast struct {
	subscribe   chan chan scan.Frame
	unsubscribe chan (<-chan scan.Frame)
	msg         <-chan scan.Frame
	q           []scan.Frame
	nextQIndex  int
	subscribers []subscriber
}

func NewBcast(msg <-chan scan.Frame) *Bcast {
	return &Bcast{
		subscribe:   make(chan chan scan.Frame),
		unsubscribe: make(chan (<-chan scan.Frame)),
		msg:         msg,
	}
}

func (b *Bcast) Close() {
	close(b.subscribe)
}

func (b *Bcast) Subscribe() <-chan scan.Frame {
	ch := make(chan scan.Frame)
	b.subscribe <- ch
	return ch
}

func (b *Bcast) Unsubscribe(ch <-chan scan.Frame) {
	b.unsubscribe <- ch
}

// This should be called in a goroutine.
// It will keep running so long as any of the following apply
// - there are existing subscribers
// - the subscriber channel is still open
// - the msg channel is still open
//
// It will close each subscriber channel when the msg channel is closed or the context is done.
// It will call wg.Done() just before it returns.
func (b *Bcast) Run(ctx context.Context, wg *sync.WaitGroup) {
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
		if chosen != 2 {
			lg.Debug("bcastSelect", "chosen", chosen)
		}
		switch chosen {
		case 0: // subscribe
			if !ok {
				subscribe = nil
				break
			}
			s := recv.Interface().(chan scan.Frame)
			if closed {
				close(s)
				break
			}
			b.subscribers = append(b.subscribers, subscriber{s, b.nextQIndex})
			lg.Debug("subscribe", "chan", s)
		case 1: // unsubscribe
			s := recv.Interface().(<-chan scan.Frame)
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
			m := recv.Interface().(scan.Frame)
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

func (b *Bcast) trimQ() {
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

func (b *Bcast) subscriberIndex(c <-chan scan.Frame) int {
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
