package rolling

import (
	"github.com/urso/ecslog/backend"
)

type triggerFactory func(Rolloverer, FileStater) Trigger

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
	RolloverTriggered()
}

type triggerFunc func(backend.Message, FileInfo) bool

type compositeTrigger struct {
	r        Rolloverer
	triggers []Trigger
}

func (f triggerFunc) CheckTrigger(evt backend.Message, stat FileInfo) bool {
	return f(evt, stat)
}

func ComposeTriggers(factories ...triggerFactory) triggerFactory {
	return func(r Rolloverer, stat FileStater) Trigger {
		ct := &compositeTrigger{r: r}
		for _, f := range factories {
			t := f(ct, stat)
			if t != nil {
				ct.triggers = append(ct.triggers, t)
			}
		}
		return ct
	}
}

func (t *compositeTrigger) Rollover() error {
	for _, trigger := range t.triggers {
		if tl, ok := trigger.(TriggerListener); ok {
			tl.RolloverTriggered()
		}
	}

	return t.r.Rollover()
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

// CronTrigger asynchronously triggers a rollover at a preconfigured interval.
func CronTrigger(config string) triggerFactory {
	panic("TODO")

	// see heartbeat/scheduler/scheduler.go for cron like
	// -> start go-routine to trigger rollover by time.
}

// StartTrigger triggers a log file rollover right on startup.
func StartTrigger() triggerFactory {
	return func(r Rolloverer, _ FileStater) Trigger {
		r.Rollover()
		return nil
	}
}

func makeSyncTrigger(fn func(backend.Message, FileInfo) bool) triggerFactory {
	return func(_ Rolloverer, _ FileStater) Trigger {
		return triggerFunc(func(evt backend.Message, stat FileInfo) bool {
			return fn(evt, stat)
		})
	}
}
