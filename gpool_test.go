package gpool

import (
	"bytes"
	"testing"
	"time"
)

func TestPool_maybeNew(t *testing.T) {
	tests := []struct {
		name    string
		newFunc func() int
		want    int
	}{
		{
			"nil",
			nil,
			0,
		},
		{
			"func",
			func() int { return -1 },
			-1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := &pool[int]{
				new: tt.newFunc,
			}

			if got := p.maybeNew(); got != tt.want {
				t.Errorf("pool.MaybeNew = %d, want %d", got, tt.want)
			}
		})
	}
}

// check if a channel is closed within 1 second
func checkClosed(c chan struct{}) bool {
	tc := time.After(time.Second)

	for {
		select {
		case _, ok := <-c:
			if !ok {
				return true
			}
		case <-tc:
			return false
		}

	}
}

func TestPool(t *testing.T) {
	const (
		size = 100
		gets = size * 2
	)

	p := NewPool(size, Options[chan struct{}]{
		NewFunc: func() chan struct{} {
			return make(chan struct{}, 1)
		},
		CloseFunc: func(v chan struct{}) {
			close(v)
		},
	}).(*pool[chan struct{}])

	checks := make([]chan struct{}, gets)

	t.Run("newFunc executions", func(t *testing.T) {
		for i := 0; i < gets; i++ {
			checks[i] = p.Get()
			if checks[i] == nil {
				t.Fatalf("pool.Get() returned %v", checks[i])
			}

			checks[i] <- struct{}{}
		}
	})

	t.Run("overflow pool", func(t *testing.T) {
		for i := 0; i < gets; i++ {
			p.Put(checks[i])

			if i >= size {
				// remaining instances get closed in a seperate go routine,
				if !checkClosed(checks[i]) {
					t.Errorf("pool.Put(): entry #%d closeFunc not called", i)
				}
			}
		}
	})

	t.Run("get from pool", func(t *testing.T) {
		for i := 0; i < size; i++ {
			var ok bool

			v := p.Get()
			defer p.Put(v)

			select {
			case _, ok = <-v:
				if !ok {
					t.Fatalf("pool.Get(): entry #%d unintentionally closed", i)
				}
			default:
			}

			if !ok {
				t.Error("pool.Get() retuned a new empty value")
			}
		}
	})

	t.Run("close pool", func(t *testing.T) {
		p.Close().Wait()

		if len(p.c) > 0 {
			t.Fatal("p.Close(): c not drained")
		}

		for i := 0; i < size; i++ {
			if !checkClosed(checks[i]) {
				t.Errorf("pool.Close(): entry #%d closeFunc not called", i)
			}
		}
	})
}

func Test_resetPool(t *testing.T) {
	p := NewResetterPool(10, Options[*bytes.Buffer]{
		NewFunc: func() *bytes.Buffer { return new(bytes.Buffer) },
	})

	b := p.Get()
	b.WriteString("hello")
	p.Put(b)

	b = p.Get()
	if c := b.Cap(); c == 0 {
		t.Errorf("resetPool.Get(): cap = %d", c)
	}
	if l := b.Len(); l != 0 {
		t.Errorf("resetPool.Get(): len = %d, want %d", l, 0)

	}
}
