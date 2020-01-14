package rolling

import (
	"os"
	"sync"
	"time"

	"github.com/urso/ecslog/backend"
	"github.com/urso/ecslog/backend/layout"
)

// Appender implements the rolling file appender.
//
// TODO: buffered output + buffer flush timeout.
type Appender struct {
	lvl backend.Level

	trigger  Trigger
	strategy Strategy
	layout   layout.Layout

	mu         sync.Mutex
	wg         sync.WaitGroup
	closed     bool
	background *Background

	file *os.File
	stat FileInfo
}

// Rotator is used by the trigger to start the rotate process.
type Rotator interface {
	Rotate() error
}

var _ Rotator = (*Appender)(nil)
var _ FileStater = (*Appender)(nil)

var newline = []byte("\n")

// FileStater is used by the trigger and strategy to query the state of the
// current active log file.
type FileStater interface {
	FileStat() FileInfo
}

type FileInfo struct {
	Name    string
	Size    uint64
	Created time.Time
}

// appenderWriter provides the Write operation required by the Layout instance.
// It is used so to not Export an unsafe Write operation in the public API of
// Appender.
type appenderWriter Appender

// NewAppender creates a new rolling file appender based on the configured
// layout, triggers, and rolling strategy.
func NewAppender(
	lvl backend.Level,
	layout layout.Factory,
	triggerFactory triggerFactory,
	strategyFactory strategyFactory,
) (*Appender, error) {
	a := &Appender{
		lvl: lvl,
		background: &Background{
			done: make(chan struct{}),
		},
	}

	l, err := layout((*appenderWriter)(a))
	if err != nil {
		return nil, err
	}

	a.layout = l
	a.strategy = strategyFactory(a.background, a)

	// trigger factory should be initialized last. All state must be initialized here,
	// as triggers are allowed to trigger a rotate right on startup.
	// This will lead to two rotate calls. The first one will try to open the file,
	// the second one (executed within this constructor) will do the rotation.
	// If the log file is empty, no rotation will occur on the second rotation signal.
	a.trigger = triggerFactory(a.background, a, a)

	err = a.Rotate()
	return a, err
}

func (a *Appender) Close() error {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.closed {
		return nil
	}

	a.background.shutdown()

	a.wg.Wait() // wait for async rotation jobs to finish
	a.closed = true

	a.background.wait()

	return nil
}

func (a *Appender) For(name string) backend.Backend {
	return a
}

func (a *Appender) IsEnabled(lvl backend.Level) bool {
	return lvl >= a.lvl
}

func (a *Appender) UseContext() bool {
	return a.layout.UseContext()
}

func (a *Appender) Log(msg backend.Message) {
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.closed {
		return
	}

	a.layout.Log(msg)
	if a.trigger != nil && a.trigger.CheckTrigger(msg, a.stat) {
		a.execRotate()
	}
}

func (a *Appender) Rotate() error {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.execRotate()
}

func (a *Appender) execRotate() error {
	if a.closed {
		return ErrClosed
	}

	if a.file != nil {
		if a.stat.Size == 0 {
			return nil // file was just rotated -> no action
		}

		a.file.Close() // TODO: report error in multi-error
		a.file = nil
	}

	stat := a.stat
	a.stat = FileInfo{}

	sync, async := a.strategy.Rotate(stat)
	file, err := sync(stat)
	if err != nil {
		return err
	}

	fi, err := file.Stat()
	if err != nil {
		file.Close()
		return err
	}

	sz := uint64(fi.Size())
	timestamp := fi.ModTime()
	if fi.Size() <= 0 {
		sz = 0
		timestamp = time.Now()
	}

	a.file = file
	a.stat = FileInfo{
		Name:    a.file.Name(),
		Created: timestamp,
		Size:    sz,
	}
	if async == nil {
		return nil
	}

	a.wg.Wait() // wait for rotation from last run to finish
	a.wg.Add(1)
	go func() {
		defer a.wg.Done()
		async(a, stat) // TODO: collect and report error
	}()

	return nil
}

func (a *Appender) FileStat() FileInfo {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.stat
}

// Write is only used by the configured layout to write the serialized log
// event to the appender. It is indirectly called via the (*Appender).Log
// method, which will also acquire the required mutex.
func (a *appenderWriter) Write(b []byte) (int, error) {
	if a.file == nil {
		err := a.appender().execRotate() // retry rotation, hoping we can open a file now
		if err != nil {
			return 0, err
		}
	}

	n, err := a.file.Write(b)
	a.stat.Size += uint64(n)
	if err == nil {
		a.file.Write(newline)
		a.stat.Size++
	}
	return n, err
}

func (a *appenderWriter) appender() *Appender {
	return (*Appender)(a)
}
