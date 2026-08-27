package mewsync

import (
	"time"

	"github.com/arshnah/detsim/rt"
)

type rtRuntime struct {
	sched *rt.Sched
}

func newRtRuntime(sched *rt.Sched) engineRuntime { return rtRuntime{sched: sched} }

func (r rtRuntime) GoNamed(name string, fn func()) { r.sched.GoNamed(name, fn) }
func (r rtRuntime) Sleep(d time.Duration)          { r.sched.Sleep(rt.VirtualTime(d)) }
func (r rtRuntime) Now() time.Time                 { return time.Unix(0, int64(r.sched.Now())) }
func (r rtRuntime) NewMutex() locker               { return rt.NewMutex(r.sched) }
func (r rtRuntime) NewRWMutex() rwLocker           { return rt.NewRWMutex(r.sched) }
func (r rtRuntime) NewChan(cap int) statusChan {
	return rtStatusChan{rt.NewChan[statusMsg](r.sched, cap)}
}

type rtStatusChan struct{ ch *rt.Chan[statusMsg] }

func (c rtStatusChan) Send(msg statusMsg)        { c.ch.Send(msg) }
func (c rtStatusChan) RecvOK() (statusMsg, bool) { return c.ch.RecvOK() }
func (c rtStatusChan) Close()                    { c.ch.Close() }
