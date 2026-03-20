package stream

import (
	"testing"
	"time"
)

func TestBackoffDelay(t *testing.T) {
	b := newBackoff()
	for range 100 {
		d := b.delay()
		if d < 0 || d >= time.Duration(b.cur*float64(time.Second)) {
			t.Fatalf("delay %v out of [0, %v)", d, time.Duration(b.cur*float64(time.Second)))
		}
	}
}

func TestBackoffIncrease(t *testing.T) {
	b := newBackoff()
	b.increase()
	if b.cur != backoffInitial*backoffMultiplier {
		t.Fatalf("expected %v, got %v", backoffInitial*backoffMultiplier, b.cur)
	}
}

func TestBackoffCap(t *testing.T) {
	b := newBackoff()
	for range 200 {
		b.increase()
	}
	if b.cur != backoffCap {
		t.Fatalf("expected cap %v, got %v", backoffCap, b.cur)
	}
}

func TestBackoffDecrease(t *testing.T) {
	b := newBackoff()
	b.cur = 10.0
	b.decrease()
	want := 10.0 / backoffMultiplier
	if b.cur != want {
		t.Fatalf("expected %v, got %v", want, b.cur)
	}
}

func TestBackoffFloor(t *testing.T) {
	b := newBackoff()
	for range 10 {
		b.decrease()
	}
	if b.cur != backoffFloor {
		t.Fatalf("expected floor %v, got %v", backoffFloor, b.cur)
	}
}
