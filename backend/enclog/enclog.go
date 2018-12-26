package enclog

import (
	"io"

	"github.com/urso/ecslog/backend"
	"github.com/urso/ecslog/backend/structlog"

	"github.com/elastic/go-structform"
	"github.com/elastic/go-structform/gotype"
)

type Output interface {
	io.Writer

	Enabled(lvl backend.Level) bool

	Begin()
	End()
}

type encoder struct {
	out     Output
	factory EncodingFactory
	visitor structform.Visitor
}

type EncodingFactory func(out io.Writer) structform.Visitor

func New(out Output, enc EncodingFactory, opts ...gotype.Option) (*structlog.Logger, error) {
	return structlog.New(&encoder{out: out, factory: enc}, opts...)
}

func (e *encoder) Enabled(lvl backend.Level) bool { return e.out.Enabled(lvl) }

func (e *encoder) Begin() { e.out.Begin() }
func (e *encoder) End()   { e.out.End() }
func (e *encoder) Reset() { e.visitor = e.factory(e.out) }

func (e *encoder) Visitor() structform.Visitor {
	if e.visitor == nil {
		e.Reset()
	}
	return e.visitor
}
