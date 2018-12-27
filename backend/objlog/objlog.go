package objlog

import (
	"github.com/urso/ecslog/backend"
	"github.com/urso/ecslog/backend/structlog"
	"github.com/urso/ecslog/fld"

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

type objOutput struct {
	lvl backend.Level
	log func(map[string]interface{})
}

func New(out Output, fields []fld.Field, opts ...gotype.Option) (*structlog.Logger, error) {
	return structlog.New(&collector{out: out}, fields, opts...)
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

func Call(lvl backend.Level, fn func(map[string]interface{})) Output {
	return &objOutput{
		lvl: lvl,
		log: fn,
	}
}

func (o *objOutput) Enabled(lvl backend.Level) bool { return lvl >= o.lvl }
func (o *objOutput) Log(obj map[string]interface{}) { o.log(obj) }
