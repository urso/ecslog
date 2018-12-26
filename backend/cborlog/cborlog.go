package jsonlog

import (
	"io"

	"github.com/urso/ecslog/backend/enclog"
	"github.com/urso/ecslog/backend/structlog"
	"github.com/urso/ecslog/fld"

	"github.com/elastic/go-structform"
	"github.com/elastic/go-structform/cborl"
	"github.com/elastic/go-structform/gotype"
)

func New(out enclog.Output, fields []fld.Field, opts ...gotype.Option) (*structlog.Logger, error) {
	return enclog.New(out, mkEncoder, fields, opts...)
}

func mkEncoder(out io.Writer) structform.Visitor {
	return cborl.NewVisitor(out)
}
