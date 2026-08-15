package exec

import (
	"sync"
)

// LifecycleRunner tracks spawned goroutines and blocks command exit until they complete.
type LifecycleRunner struct {
	wg sync.WaitGroup
}

// NewLifecycleRunner initializes a new LifecycleRunner.
func NewLifecycleRunner() *LifecycleRunner {
	return &LifecycleRunner{}
}

// Go executes the given function in a new tracked goroutine.
func (l *LifecycleRunner) Go(f func()) {
	l.wg.Add(1)
	go func() {
		defer l.wg.Done()
		f()
	}()
}

// Wait blocks until all spawned goroutines have finished.
func (l *LifecycleRunner) Wait() {
	l.wg.Wait()
}
