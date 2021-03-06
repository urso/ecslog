// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//        http://www.apache.org/licenses/LICENSE-2.0

package rolling

import (
	"time"

	"github.com/urso/ecslog/backend"
)

type triggerFactory func(*Background, Rotator, FileStater) Trigger

// Trigger interface is implemented by the different trigger types.
// Triggers are allowed to trigger a file rollover at any time.
// The CheckTrigger method is called after each log message being written.
type Trigger interface {
	// CheckTrigger synchronously checks if the log file needs to be rolled over.
	// The trigger must not execute Rollover, but return 'true', to start the
	// synchronous rollover routine.
	CheckTrigger(evt backend.Message, stat FileInfo) bool
}

// TriggerListener provides an optional trigger extension for stateful triggers
// combined via ComposeTriggers into one common trigger.
// When the composite trigger signals a rollover, any trigger implementing
// TriggerListener will be called, allowing it to reset state.
type TriggerListener interface {
	RotationTriggered()
}

type triggerFunc func(backend.Message, FileInfo) bool

type compositeTrigger struct {
	r        Rotator
	triggers []Trigger
}

func (f triggerFunc) CheckTrigger(evt backend.Message, stat FileInfo) bool {
	return f(evt, stat)
}

func ComposeTriggers(factories ...triggerFactory) triggerFactory {
	return func(b *Background, r Rotator, stat FileStater) Trigger {
		ct := &compositeTrigger{r: r}
		for _, f := range factories {
			t := f(b, ct, stat)
			if t != nil {
				ct.triggers = append(ct.triggers, t)
			}
		}
		return ct
	}
}

func (t *compositeTrigger) Rotate() error {
	for _, trigger := range t.triggers {
		if tl, ok := trigger.(TriggerListener); ok {
			tl.RotationTriggered()
		}
	}

	return t.r.Rotate()
}

func (t *compositeTrigger) CheckTrigger(evt backend.Message, stat FileInfo) bool {
	for _, trigger := range t.triggers {
		if trigger.CheckTrigger(evt, stat) {
			return true
		}
	}
	return false
}

// SizeTrigger triggers rollover once a pre-configured file size is reached.
func SizeTrigger(maxSize uint64) triggerFactory {
	return makeSyncTrigger(func(evt backend.Message, stat FileInfo) bool {
		return stat.Size >= maxSize
	})
}

// TimeTrigger creates a background trigger, that triggers rotation based on the
// timestamps returned by next.
func TimeTrigger(next func() time.Time) triggerFactory {
	return func(b *Background, r Rotator, _ FileStater) Trigger {
		b.Go(func() {
			for {
				dur := time.Now().Sub(next())
				if dur < 0 {
					if b.Err() != nil {
						return
					}
					r.Rotate()
					continue
				}

				select {
				case <-b.Done():
					return
				case <-time.After(dur):
					r.Rotate()
				}
			}
		})
		return nil
	}
}

// Periodical trigger create a background trigger, that triggers rotation
// within a set interval.
func PeriodicTrigger(interval time.Duration) triggerFactory {
	ts := time.Now()
	return TimeTrigger(func() time.Time {
		ts = ts.Add(interval)
		return ts
	})
}

// StartTrigger triggers a log file rollover right on startup.
func StartTrigger() triggerFactory {
	return func(_ *Background, r Rotator, _ FileStater) Trigger {
		r.Rotate()
		return nil
	}
}

func makeSyncTrigger(fn func(backend.Message, FileInfo) bool) triggerFactory {
	return func(_ *Background, _ Rotator, _ FileStater) Trigger {
		return triggerFunc(func(evt backend.Message, stat FileInfo) bool {
			return fn(evt, stat)
		})
	}
}
