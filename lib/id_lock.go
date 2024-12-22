package lib

import (
	"sync"
)

// IDLock provides a locking mechanism that allows concurrent operations with the same ID
// while limiting the total number of parallel operations.
type IDLock[T comparable] struct {
	locks        map[T]*sync.Mutex
	maxParallel  uint
	globalLock   *sync.Mutex
	globalTokens chan struct{}
	lockMutex    sync.Mutex
}

// NewIDLock creates a new IDLock with the specified maximum parallel operations.
func NewIDLock[T comparable](maxParallel uint) *IDLock[T] {
	return &IDLock[T]{
		locks:        make(map[T]*sync.Mutex),
		maxParallel:  maxParallel,
		globalLock:   &sync.Mutex{},
		globalTokens: make(chan struct{}, maxParallel),
		lockMutex:    sync.Mutex{},
	}
}

// Acquire acquires the lock for the given ID.
// If maxParallel is set, it also acquires a token to limit concurrency.
func (l *IDLock[T]) Acquire(id T) {
	// Acquire a global token if maxParallel is set
	if l.maxParallel > 0 {
		l.globalTokens <- struct{}{}
	}

	l.lockMutex.Lock()
	if _, ok := l.locks[id]; !ok {
		l.locks[id] = &sync.Mutex{}
	}
	lock := l.locks[id]
	l.lockMutex.Unlock()
	lock.Lock()
}

// Release releases the lock for the given ID.
// If maxParallel is set, it also releases the acquired token.
func (l *IDLock[T]) Release(id T) {
	l.lockMutex.Lock()
	if lock, ok := l.locks[id]; ok {
		lock.Unlock()
		if !lock.TryLock() { // Check if the lock is actually unlocked
			delete(l.locks, id)
		}
	}
	l.lockMutex.Unlock()

	// Release a global token if maxParallel is set
	if l.maxParallel > 0 {
		<-l.globalTokens
	}
}
