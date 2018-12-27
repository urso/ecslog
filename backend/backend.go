package backend

import "github.com/urso/ecslog/ctxtree"

type Backend interface {
	IsEnabled(lvl Level) bool
	UseContext() bool

	Log(lvl Level, caller Caller, msg string, ctx ctxtree.Ctx, causes []error)
}

type Level uint8

const (
	Trace Level = iota
	Debug
	Info
	Error
)

func (l Level) String() string {
	switch l {
	case Trace:
		return "trace"
	case Debug:
		return "debug"
	case Info:
		return "info"
	case Error:
		return "error"
	default:
		return "unknown"
	}
}
