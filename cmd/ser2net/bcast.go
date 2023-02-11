package main

import (
	"context"

	"github.com/jclark/gps2phc/logctx"
	"golang.org/x/exp/constraints"
)

type subscriber struct {
	c             chan<- []byte
	nextSendIndex int
}

type bcast struct {
	subscribe   chan chan<- []byte
	msg         chan []byte
	q           [][]byte
	nextQIndex  int
	subscribers []*subscriber
}

func newBcast() *bcast {
	return &bcast{
		subscribe: make(chan chan<- []byte),
		msg:       make(chan []byte, 1),
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
		select {
		case s := <-b.subscribe:
			b.subscribers = append(b.subscribers, &subscriber{s, b.nextQIndex})
		case m := <-b.msg:
			if len(b.subscribers) != 0 {
				b.q = append(b.q, m)
				b.nextQIndex++
			}
		case <-ctx.Done():
			return
		default:
		}
		for {
			progress := false
			needIndex := b.nextQIndex
			for _, s := range b.subscribers {
				if s.nextSendIndex < b.nextQIndex {
					lg.Debug("bcast", "nextSendIndex", s.nextSendIndex, "nextQIndex", b.nextQIndex, "len(b.q)", len(b.q))
					qStartIndex := b.nextQIndex - len(b.q)
					select {
					case s.c <- b.q[s.nextSendIndex-qStartIndex]:
						s.nextSendIndex++
						progress = true
					case <-ctx.Done():
						return
					default:
					}
					needIndex = min(needIndex, s.nextSendIndex)
				}
			}
			nNeed := b.nextQIndex - needIndex
			if nNeed == 0 {
				b.q = b.q[:0]
			} else if nDiscard := len(b.q) - nNeed; nDiscard > 0 {
				oldQ := b.q[nDiscard:]
				b.q = b.q[0:nNeed]
				copy(b.q, oldQ)
			}
			if !progress {
				break
			}
		}
	}
}

func min[T constraints.Ordered](a, b T) T {
	if a < b {
		return a
	}
	return b
}
