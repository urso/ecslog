package txtlog

import (
	"bytes"
	"fmt"
	"io"
	"path/filepath"
	"sync"
	"time"

	"github.com/urso/ecslog/backend"
	"github.com/urso/ecslog/ctxtree"
	"github.com/urso/ecslog/errx"
	"github.com/urso/ecslog/fld"
)

type Output interface {
	Enabled(lvl backend.Level) bool
	WithContext() bool
	Write(msg []byte)
}

type writerOutput struct {
	lvl     backend.Level
	withCtx bool
	out     io.Writer
}

type Logger struct {
	mux sync.Mutex
	out Output
	buf bytes.Buffer
}

type ctxPrinter struct {
	log *Logger
	n   int
}

func Writer(out io.Writer, lvl backend.Level, withCtx bool) Output {
	return &writerOutput{
		lvl:     lvl,
		withCtx: withCtx,
		out:     out,
	}
}

func NewTextBackend(out Output) *Logger {
	return &Logger{out: out}
}

func (l *Logger) IsEnabled(lvl backend.Level) bool { return l.out.Enabled(lvl) }
func (l *Logger) UseContext() bool                 { return l.out.WithContext() }

func (l *Logger) Log(
	lvl backend.Level,
	caller backend.Caller,
	msg string, ctx ctxtree.Ctx,
	errors []error,
) {
	l.mux.Lock()
	defer l.mux.Unlock()

	defer l.buf.Reset()

	ts := time.Now()

	_, err := fmt.Fprintf(&l.buf, "%v %v\t%v:%v\t%v",
		ts.Format(time.RFC3339), level(lvl), filepath.Base(caller.File()), caller.Line(), msg)
	if err != nil {
		return
	}

	ctx.VisitKeyValues(&ctxPrinter{log: l})
	l.buf.WriteRune('\n')

	// write errors
	switch len(errors) {
	case 0:
		// do nothing

	case 1:
		if ioErr := l.OnErrorValue(errors[0], "\t"); ioErr != nil {
			return
		}

	case 2:
		written := 0
		l.buf.WriteString("\tcaused by:\n")
		for _, err := range errors {
			if err == nil {
				continue
			}

			if written != 0 {
				l.buf.WriteString("\tand\n")
			}

			written++
			if ioErr := l.OnErrorValue(err, "\t    "); ioErr != nil {
				return
			}
		}
	}

	l.flush()
}

func (l *Logger) flush() {
	l.out.Write(l.buf.Bytes())
}

func (l *Logger) OnErrorValue(err error, indent string) error {
	l.buf.WriteString(indent)

	if file, line := errx.At(err); file != "" {
		fmt.Fprintf(&l.buf, "%v:%v\t", filepath.Base(file), line)
	}

	l.buf.WriteString(err.Error())

	if l.out.WithContext() {
		if ctx := errx.ErrContext(err); ctx.Len() > 0 {
			ctx.VisitKeyValues(&ctxPrinter{log: l})
		}
	}

	if _, ioErr := l.buf.WriteRune('\n'); ioErr != nil {
		return ioErr
	}

	n := errx.NumCauses(err)
	switch n {
	case 0:
		// do nothing
	case 1:
		cause := errx.Cause(err, 0)
		if cause != nil {
			return l.OnErrorValue(cause, indent)
		}
	default:
		causeIndent := indent + "    "
		written := 0
		fmt.Fprintf(&l.buf, "%vmulti-error caused by:\n", indent)
		for i := 0; i < n; i++ {
			cause := errx.Cause(err, i)
			if cause != nil {
				if written != 0 {
					fmt.Fprintf(&l.buf, "%vand\n", indent)
				}

				written++
				if err := l.OnErrorValue(cause, causeIndent); err != nil {
					return err
				}
			}
		}
	}

	return nil
}

func level(l backend.Level) string {
	switch l {
	case backend.Trace:
		return "TRACE"
	case backend.Debug:
		return "DEBUG"
	case backend.Info:
		return "INFO"
	case backend.Error:
		return "ERROR"
	default:
		return fmt.Sprintf("<%v>", l)
	}
}

func (p *ctxPrinter) OnObjStart(key string) error {
	if err := p.onKey(key); err != nil {
		return err
	}
	_, err := p.log.buf.WriteRune('{')
	return err
}

func (p *ctxPrinter) OnObjEnd() error {
	_, err := p.log.buf.WriteRune('}')
	return err
}

func (p *ctxPrinter) OnValue(key string, v fld.Value) (err error) {
	p.onKey(key)
	v.Reporter.Ifc(&v, func(value interface{}) {
		switch v := value.(type) {
		case *ctxtree.Ctx:
			p.log.buf.WriteRune('{')
			err = v.VisitKeyValues(p)
			p.log.buf.WriteRune('}')
		case string, []byte:
			fmt.Fprintf(&p.log.buf, "%q", v)
		default:
			fmt.Fprintf(&p.log.buf, "%v", v)
		}
	})

	return err
}

func (p *ctxPrinter) onKey(key string) error {
	if p.n > 0 {
		p.log.buf.WriteRune(' ')
	} else {
		p.log.buf.WriteString("\t| ")
	}
	p.log.buf.WriteString(key)
	p.log.buf.WriteRune('=')
	p.n++
	return nil
}

func (wo *writerOutput) Enabled(lvl backend.Level) bool { return lvl >= wo.lvl }

func (wo *writerOutput) WithContext() bool { return wo.withCtx }

func (wo *writerOutput) Write(msg []byte) {
	wo.out.Write(msg)

	// flush if output is buffered
	switch f := wo.out.(type) {
	case interface{ Flush() error }:
		f.Flush()

	case interface{ Flush() bool }:
		f.Flush()

	case interface{ Flush() }:
		f.Flush()
	}
}
