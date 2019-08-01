package rolling

import (
	"errors"
	"os"
	"sync"
	"time"

	"github.com/urso/ecslog/backend"
	"github.com/urso/ecslog/backend/layout"
)

// rolling implements the rolling file appender.
//
// TODO: buffered output + buffer flush timeout.
type rolling struct {
	lvl backend.Level

	trigger  Trigger
	strategy Strategy
	layout   layout.Layout

	mu sync.Mutex
	wg sync.WaitGroup

	file *os.File
	stat FileInfo
}

// Rolloverer is used by the trigger to start the rollover process.
type Rotator interface {
	Rotate() error
}

var _ Rotator = (*rolling)(nil)
var _ FileStater = (*rolling)(nil)

var ErrNoFile = errors.New("No log file open")

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

func NewRollingFile(
	lvl backend.Level,
	layout layout.Factory,
	triggerFactory triggerFactory,
	strategyFactory strategyFactory,
) (backend.Backend, error) {
	r := &rolling{
		lvl: lvl,
	}

	l, err := layout(r)
	if err != nil {
		return nil, err
	}

	r.layout = l
	r.strategy = strategyFactory(r)

	// trigger factory should be initialized last. All state must be initialized here,
	// as triggers are allowed to trigger a rollover right on startup.
	// This will lead to two rollover calls. The first one will try to open the file,
	// the second one (executed within this constructor) will do the rollover.
	// If the log file is empty, no rollover will occur on the second rollover signal.
	r.trigger = triggerFactory(r, r)

	err = r.Rotate()
	return r, err
}

func (r *rolling) For(name string) backend.Backend {
	return r
}

func (r *rolling) IsEnabled(lvl backend.Level) bool {
	return lvl >= r.lvl
}

func (r *rolling) UseContext() bool {
	return r.layout.UseContext()
}

func (r *rolling) Log(msg backend.Message) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.layout.Log(msg)
	if r.trigger != nil && r.trigger.CheckTrigger(msg, r.stat) {
		r.execRollover()
	}
}

func (r *rolling) Rotate() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.execRollover()
}

func (r *rolling) execRollover() error {
	if r.file != nil {
		if r.stat.Size == 0 {
			return nil // file was just rotated -> no action
		}

		r.file.Close() // TODO: report error in multi-error
		r.file = nil
	}

	stat := r.stat
	r.stat = FileInfo{}

	sync, async := r.strategy.Rotate(stat)
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

	r.file = file
	r.stat = FileInfo{
		Name:    r.file.Name(),
		Created: timestamp,
		Size:    sz,
	}
	if async == nil {
		return nil
	}

	r.wg.Wait()
	r.wg.Add(1)
	go func() {
		defer r.wg.Done()
		async(r, stat) // TODO: collect and report error
	}()

	return nil
}

func (r *rolling) FileStat() FileInfo {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.stat
}

func (r *rolling) Write(b []byte) (int, error) {
	if r.file == nil {
		err := r.execRollover() // retry rollover, hoping we can open a file
		if err != nil {
			return 0, err
		}
	}

	n, err := r.file.Write(b)
	r.stat.Size += uint64(n)
	return n, err
}
