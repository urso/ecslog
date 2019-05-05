package appender

import (
	"os"
	"sync"
	"time"

	"github.com/urso/ecslog/backend"
	"github.com/urso/ecslog/backend/layout"
)

type rolling struct {
	lvl backend.Level

	manager  *RollingFile
	trigger  Trigger
	strategy RolloverStrategy
	layout   layout.Layout
}

type RollingFile struct {
	mu sync.Mutex

	f    *os.File
	path string
	sz   uint64
	ts   time.Time
}

type Trigger interface {
	CheckTrigger(evt backend.Message) bool
}

type RolloverStrategy interface {
	Rollover(manager *RollingFile) RolloverDescription
}

type RolloverDescription struct {
	Sync  func(manager *RollingFile) error
	Async func(manager *RollingFile) error
}

func NewRollingFile(
	lvl backend.Level,
	layout layout.Factory,
	trigger func(*RollingFile) Trigger,
	strategy func(*RollingFile) RolloverStrategy,
) (backend.Backend, error) {
	m := &RollingFile{}

	l, err := layout(m)
	if err != nil {
		return nil, err
	}

	t := trigger(m)
	strat := strategy(m)

	return &rolling{
		lvl:      lvl,
		manager:  m,
		trigger:  t,
		strategy: strat,
		layout:   l,
	}, nil
}

func (f *rolling) For(name string) backend.Backend {
	return f
}

func (f *rolling) IsEnabled(lvl backend.Level) bool {
	return lvl >= f.lvl
}

func (f *rolling) UseContext() bool {
	return f.layout.UseContext()
}

func (f *rolling) Log(msg backend.Message) {
	f.layout.Log(msg)
	if f.trigger.CheckTrigger(msg) {
		f.rollover()
	}
}

func (f *rolling) rollover() {
}

func (m *RollingFile) FileName() string {
	return m.path
}

func (m *RollingFile) FileSize() uint64 {
	return m.sz
}

func (m *RollingFile) FileCreated() time.Time {
	return m.ts
}

func (m *RollingFile) Write(b []byte) (int, error) {
	return m.f.Write(b)
}
