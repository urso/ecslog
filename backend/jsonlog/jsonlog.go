package jsonlog

import (
	"io"

	"github.com/urso/ecslog/backend/enclog"
	"github.com/urso/ecslog/backend/structlog"

	"github.com/elastic/go-structform"
	"github.com/elastic/go-structform/gotype"
	"github.com/elastic/go-structform/json"
)

func New(out enclog.Output, opts ...gotype.Option) (*structlog.Logger, error) {
	return enclog.New(out, mkEncoder, opts...)
}

func mkEncoder(out io.Writer) structform.Visitor {
	return json.NewVisitor(out)
}
