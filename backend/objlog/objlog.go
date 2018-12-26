package objlog

import (
	"github.com/urso/ecslog/backend"
	"github.com/urso/ecslog/backend/structlog"

	structform "github.com/elastic/go-structform"
	"github.com/elastic/go-structform/gotype"
)

type Output interface {
	Enabled(lvl backend.Level) bool
	Log(obj map[string]interface{})
}

type collector struct {
	out      Output
	unfolder *gotype.Unfolder
	active   map[string]interface{}
}

func New(out Output, opts ...gotype.Option) (*structlog.Logger, error) {
	return structlog.New(&collector{out: out}, opts...)
}

func (c *collector) Enabled(lvl backend.Level) bool {
	return c.out.Enabled(lvl)
}

func (c *collector) Reset() {
	c.active = nil
	c.unfolder, _ = gotype.NewUnfolder(nil)
}

func (c *collector) Visitor() structform.Visitor {
	if c.unfolder == nil {
		c.Reset()
	}
	return c.unfolder
}

func (c *collector) Begin() {
	c.unfolder.SetTarget(&c.active)
}

func (c *collector) End() {
	val := c.active
	c.active = nil
	c.out.Log(val)
}
