package mewsync

import (
	"sync"
	"time"
)

type realRuntime struct{}

// NewRealRuntime is the engineRuntime production code should use: real
// goroutines, real sync.Mutex, real channels, real wall-clock time.
func NewRealRuntime() engineRuntime { return realRuntime{} }

func (realRuntime) GoNamed(name string, fn func()) { go fn() }
func (realRuntime) Sleep(d time.Duration)          { time.Sleep(d) }
func (realRuntime) Now() time.Time                 { return time.Now() }
func (realRuntime) NewMutex() locker               { return &sync.Mutex{} }
func (realRuntime) NewRWMutex() rwLocker           { return &sync.RWMutex{} }
func (realRuntime) NewChan(cap int) statusChan     { return newRealStatusChan(cap) }

type realStatusChan struct {
	ch   chan statusMsg
	once sync.Once
}

func newRealStatusChan(cap int) *realStatusChan {
	return &realStatusChan{ch: make(chan statusMsg, cap)}
}

func (c *realStatusChan) Send(msg statusMsg) { c.ch <- msg }

func (c *realStatusChan) RecvOK() (statusMsg, bool) {
	msg, ok := <-c.ch
	return msg, ok
}

func (c *realStatusChan) Close() {
	c.once.Do(func() { close(c.ch) })
}
