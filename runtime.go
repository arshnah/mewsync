package mewsync

import "time"

type locker interface {
	Lock()
	Unlock()
}

type rwLocker interface {
	Lock()
	Unlock()
	RLock()
	RUnlock()
}

type statusChan interface {
	Send(statusMsg)
	RecvOK() (statusMsg, bool)
	Close()
}

// engineRuntime is everything Engine, Shared and SettingsBox need from their
// concurrency primitives. Production gets one backed by real goroutines,
// sync.Mutex and channels (realRuntime). Tests get one backed by detsim's
// rt.Sched, so the same Engine code that ships is what detsim exercises.
type engineRuntime interface {
	GoNamed(name string, fn func())
	Sleep(d time.Duration)
	Now() time.Time
	NewMutex() locker
	NewRWMutex() rwLocker
	NewChan(cap int) statusChan
}
