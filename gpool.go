// Package gpool provides a generic alternative to sync.Pool.
//
// It is implemented using channels and does not use any other form of locking.
// The main difference with sync.Pool, is that instances in the Pool
// don't get garbage collected every other run and the size of the Pool is fixed.
// Also by using type parameters, this package is generic and can be used without
// runtime assertion.
// This makes it suitable for different applications, such as connection Pooling.
package gpool

import (
	"sync"
)

// Pool allows reuse of memory between Go routines.
type Pool[T any] interface {
	// Get an instance from the Pool,
	// or NewFunc if it's not nil.
	Get() T

	// Put an instance in the pool.
	// If the Pool is full the instance is discarded,
	// calling CloseFunc in a seperate Go routine
	// if it is not nil.
	Put(instance T)

	// Close discards all instances in the pool.
	// If the Pool was created with a CloseFunc,
	// it is called for each instance in a seperate Go routine.
	// Callers can Wait() on all routines to finish.
	Close() *sync.WaitGroup
}

type pool[T any] struct {
	c     chan T
	new   func() T
	close func(T)
	wg    sync.WaitGroup
}

func (p *pool[T]) maybeNew() (v T) {
	if p.new != nil {
		return p.new()
	}
	return
}

func (p *pool[T]) maybeClose(v T) {
	if p.close != nil {
		p.wg.Add(1)

		go func() {
			defer p.wg.Done()
			p.close(v)
		}()
	}
}

func (p *pool[T]) Get() T {
	select {
	case v := <-p.c:
		return v
	default:
		return p.maybeNew()
	}
}

func (p *pool[T]) Put(v T) {
	select {
	case p.c <- v:
	default:
		p.maybeClose(v)
	}
}

func (p *pool[T]) Close() *sync.WaitGroup {
	close(p.c)

	for v := range p.c {
		p.maybeClose(v)
	}

	return &p.wg
}

// Options controll the behaviour of a Pool.
type Options[T any] struct {
	// If not nil, NewFunc is called each time Get() is called on an empty Pool.
	NewFunc func() T

	// If not nil, CloseFunc is called for each instance in the Pool that is being discarded.
	// This can be when the Pool is full or when Pool.Close() is called.
	// CloseFunc is called from seperate Go routines, so it must be concurrency safe.
	CloseFunc func(intance T)
}

// NewPool that can hold "size" amount of instances of T.
func NewPool[T any](size int, opt Options[T]) Pool[T] {
	p := &pool[T]{
		c:     make(chan T, size),
		new:   opt.NewFunc,
		close: opt.CloseFunc,
	}

	return p
}

// Resetter is a type that holds a Reset() method,
// such as bytes.Buffer of strings.Builder.
type Resetter interface {
	Reset()
}

type resetPool[T Resetter] struct {
	Pool[T]
}

func (p *resetPool[T]) Put(v T) {
	v.Reset()
	p.Pool.Put(v)
}

// NewResetterPool returns a Pool for types that impelement the Ressetter interface.
// Each intance passed to Pool.Put() has its Reset() method called.
func NewResetterPool[T Resetter](size int, opt Options[T]) Pool[T] {
	p := NewPool(size, opt)
	return &resetPool[T]{p}
}
