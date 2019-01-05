package fld

import (
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"unsafe"
)

// TODO: Improve performance by not relying on fmt.Sprintf.
//  package currently parses message and uses fmt.Sprintf for the actual
//  formatting.

type CB func(key string, idx int, val interface{})

type printer struct {
	cb   CB
	buf  buffer
	buf0 [128]byte
}

type buffer []byte

var printerPool = sync.Pool{
	New: func() interface{} {
		return &printer{}
	},
}

func Format(cb CB, msg string, vs ...interface{}) (string, []interface{}) {
	p := newPrinter(cb)
	defer p.release()

	used := p.printf(msg, vs)
	s := string(p.buf)
	if used >= len(vs) {
		return s, nil
	}

	// collect errors from extra variables
	rest := vs[used:]
	for i := range rest {
		if _, ok := rest[i].(error); ok {
			cb("", used+i, rest[i])
		}
	}

	return s, rest
}

func newPrinter(cb CB) *printer {
	p := printerPool.Get().(*printer)
	p.cb = cb
	p.buf = p.buf0[:0]
	return p
}

func (p *printer) release() {
	p.cb = nil
	p.buf = nil
	printerPool.Put(p)
}

func (p *printer) printf(msg string, vs []interface{}) int {
	args := vs
	end := len(msg)
	argIdx := 0

	for i := 0; i < end; {
		lasti := i

		i = advanceToFmt(msg, i, end)
		if i > lasti {
			p.buf.WriteString(msg[lasti:i])
		}
		if i >= end {
			break
		}

		if msg[i+1] != '{' {
			// normal format identifier, search for space or end
			j := i + strings.IndexRune(msg[i:], ' ')
			next := j
			var format string
			if j < i {
				format = msg[i:]
				next = end
			} else {
				format = msg[i:j]
			}

			// combine prefix and custom format pattern
			var arg interface{}
			if argIdx < len(args) {
				arg = args[argIdx]
			}

			// report errors to formatter callback
			if _, ok := arg.(error); ok {
				p.cb("", argIdx, arg)
			}

			// handle field values being passed:
			if fld, ok := arg.(Field); ok {
				p.cb("", argIdx, arg)
				fmt.Fprintf(p, format, fld.Value.Interface())
			} else {
				fmt.Fprintf(p, format, arg)
			}

			i = next
			argIdx++
			continue
		}

		// found property
		i++ // ignore '%' symbol
		j := i + 1
		for j < end && msg[j] != '}' {
			j++
		}
		if j >= end {
			// invalid format string. just print all contents and exit
			p.buf.WriteString(msg[i:end])
			break
		}

		key, pattern, prefix := parseProperty(msg[i+1 : j])

		// combine prefix and custom format pattern
		var arg interface{}
		if argIdx < len(args) {
			arg = args[argIdx]
		}

		if fld, ok := arg.(Field); ok {
			p.format(prefix, pattern, fld.Value.Interface())
			p.cb("", argIdx, fld)
		} else {
			p.format(prefix, pattern, arg)
			p.cb(key, argIdx, arg)
		}
		argIdx++

		// continue after current property
		i = j + 1
	}

	return argIdx
}

func (p *printer) format(prefix byte, pattern string, arg interface{}) {
	if prefix == '@' {
		enc := json.NewEncoder(p)
		enc.Encode(arg)
		return
	}

	// TODO: optimize me

	if pattern == "" {
		pattern = "v"
	}
	fmtPattern := make([]byte, 0, 8)
	fmtPattern = append(fmtPattern, '%')
	if prefix != 0 {
		fmtPattern = append(fmtPattern, prefix)
	}
	fmtPattern = append(fmtPattern, pattern...)
	fmt.Fprintf(p, *(*string)(unsafe.Pointer(&fmtPattern)), arg)
}

func advanceToFmt(in string, start, end int) (i int) {
	for start < end {
		i := strings.IndexAny(in[start:], `\%`)
		if i < 0 {
			return end
		}

		if in[start] == '\\' {
			// ignore found symbol and continue
			start += i + 1
			continue
		}

		return start + i
	}

	return end
}

func parseProperty(p string) (key, pattern string, prefix byte) {
	i, m := 0, 0

	// search format string marker
	for m < len(p) && m != ':' {
		m++
	}

	pattern = "v"
	if m < len(p) {
		pattern = p[m:]
	}

	if p[i] == '+' || p[i] == '#' || p[i] == '@' {
		prefix = p[i]
		i++
	}

	key = p[i:m]
	return key, pattern, prefix
}

func (b *buffer) Write(p []byte)       { *b = append(*b, p...) }
func (b *buffer) WriteString(s string) { *b = append(*b, s...) }
func (b *buffer) WriteByte(v byte)     { *b = append(*b, v) }

func (p *printer) Write(in []byte) (int, error) {
	p.buf.Write(in)
	return len(in), nil
}
