package ecslog

import (
	"fmt"

	"github.com/urso/ecslog/backend"
	"github.com/urso/ecslog/ctxtree"
	"github.com/urso/ecslog/fld"
)

type Logger struct {
	ctx     ctxtree.Ctx
	backend backend.Backend
}

type Level = backend.Level

const (
	Trace Level = backend.Trace
	Debug Level = backend.Debug
	Info  Level = backend.Info
	Error Level = backend.Error
)

func New(backend backend.Backend) *Logger {
	return &Logger{
		ctx:     ctxtree.Make(nil, nil),
		backend: backend,
	}
}

func (l *Logger) IsEnabled(lvl Level) bool {
	return l.backend.IsEnabled(lvl)
}

func (l *Logger) With(args ...interface{}) *Logger {
	nl := &Logger{
		ctx:     ctxtree.Make(&l.ctx, nil),
		backend: l.backend,
	}
	nl.ctx.AddAll(args...)
	return nl
}

func (l *Logger) WithFields(fields ...fld.Field) *Logger {
	nl := &Logger{
		ctx:     ctxtree.Make(&l.ctx, nil),
		backend: l.backend,
	}
	nl.ctx.AddFields(fields)
	return nl
}

func (l *Logger) Trace(msg string, args ...interface{}) {
	l.log(Trace, 1, msg, args)
}

func (l *Logger) Debug(msg string, args ...interface{}) {
	l.log(Debug, 1, msg, args)
}

func (l *Logger) Info(msg string, args ...interface{}) {
	l.log(Info, 1, msg, args)
}

func (l *Logger) Error(msg string, args ...interface{}) {
	l.log(Error, 1, msg, args)
}

func (l *Logger) log(lvl Level, skip int, msg string, args []interface{}) {
	if !l.IsEnabled(lvl) {
		return
	}

	if l.backend.UseContext() {
		l.logMsgCtx(lvl, skip+1, msg, args)
	} else {
		l.logMsg(lvl, skip+1, msg, args)
	}
}

func (l *Logger) logMsgCtx(lvl Level, skip int, msg string, args []interface{}) {
	ctx := ctxtree.Make(&l.ctx, nil)
	var causes0 [1]error
	causes := causes0[:0]

	msg, rest := fld.Format(func(key string, idx int, val interface{}) {
		if field, ok := (val).(fld.Field); ok {
			if key != "" {
				ctx.Add(fmt.Sprintf("%v.%v", key, field.Key), field.Value)
			} else {
				ctx.AddField(field)
			}
			return
		}

		switch v := val.(type) {
		case fld.Value:
			ctx.Add(ensureKey(key, idx), v)
		case error:
			causes = append(causes, v)
			if key != "" {
				ctx.AddField(fld.String(key, v.Error()))
			}
		default:
			ctx.AddField(fld.Any(ensureKey(key, idx), val))
		}
	}, msg, args...)

	if len(rest) > 0 {
		msg = fmt.Sprintf("%s {EXTRA_FIELDS: %v}", msg, rest)
	}

	l.backend.Log(lvl, backend.GetCaller(skip+1), msg, ctx, causes)
}

func ensureKey(key string, idx int) string {
	if key == "" {
		return fmt.Sprintf("%v", idx)
	}
	return key
}

func (l *Logger) logMsg(lvl Level, skip int, msg string, args []interface{}) {
	var causes0 [1]error
	causes := causes0[:0]

	msg, rest := fld.Format(func(key string, idx int, val interface{}) {
		if err, ok := val.(error); ok {
			causes = append(causes, err)
		}
	}, msg, args...)

	if len(rest) > 0 {
		msg = fmt.Sprintf("%s {EXTRA_FIELDS: %v}", msg, rest)
	}

	l.backend.Log(lvl, backend.GetCaller(skip+1), msg, ctxtree.Make(nil, nil), causes)
}
