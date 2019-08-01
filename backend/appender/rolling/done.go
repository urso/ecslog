package rolling

import "sync"

// Background is used to track background go routine started by triggers
// and strategies. On shutdown go routines are signaled via Done and Err.
// Go routines to be managed must use Add and Finished to signal that they will
// be started or that they have finished. One can use the Go method for convenience.
//
// For example tasks that can trigger asynchronous rotation will need to be
// tracked, such that these go-routines can be cleanly stopped.
type Background struct {
	done chan struct{}
	wg   sync.WaitGroup
}

func (b *Background) shutdown() {
	close(b.done)
}

func (b *Background) wait() {
	b.wg.Wait()
}

// Done returns a channel that will be closed to signal shutdown. Receive
// operations from the closed channel will always succeed.
func (b *Background) Done() <-chan struct{} {
	return b.done
}

// Err returns ErrClosed if the background instance is to be shut down.
func (b *Background) Err() error {
	select {
	case <-b.done:
		return ErrClosed
	default:
		return nil
	}
}

// Add marks the start of a new background go routine to track.
func (b *Background) Add() {
	b.wg.Add(1)
}

// Finished is used by tracked go routines to signal that they will go down.
func (b *Background) Finished() {
	b.wg.Done()
}

// Go spawns a tracked go routine.
func (b *Background) Go(fn func()) {
	b.Add()
	go func() {
		defer b.Finished()
		fn()
	}()
}
