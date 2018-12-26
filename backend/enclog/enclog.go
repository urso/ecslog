package enclog

import (
	"io"

	"github.com/urso/ecslog/backend"
	"github.com/urso/ecslog/backend/structlog"
	"github.com/urso/ecslog/fld"

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

type writerOutput struct {
	io.Writer
	lvl   backend.Level
	delim string
}

func New(out Output, enc EncodingFactory, fields []fld.Field, opts ...gotype.Option) (*structlog.Logger, error) {
	return structlog.New(&encoder{out: out, factory: enc}, fields, opts...)
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

func Writer(out io.Writer, lvl backend.Level, delim string) Output {
	return &writerOutput{
		Writer: out,
		lvl:    lvl,
		delim:  delim,
	}
}

func (o *writerOutput) Enabled(lvl backend.Level) bool { return lvl >= o.lvl }

func (o *writerOutput) Begin() {}

func (o *writerOutput) End() {
	io.WriteString(o.Writer, o.delim)
}
