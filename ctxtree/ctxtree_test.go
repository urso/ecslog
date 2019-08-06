package ctxtree

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/urso/ecslog/fld"
)

type testVisitor struct {
	M     map[string]interface{}
	keys  []string
	stack []map[string]interface{}
}

func TestCtxBuild(t *testing.T) {
	t.Run("new empty context", func(t *testing.T) {
		ctx := New(nil, nil)
		assertCtx(t, nil, ctx)
	})

	t.Run("new empty with non-empty before", func(t *testing.T) {
		before := New(nil, nil)
		before.AddAll("hello", "world")
		ctx := New(before, nil)
		assertCtx(t, map[string]interface{}{
			"hello": "world",
		}, ctx)
	})

	t.Run("new empty with non-empty after", func(t *testing.T) {
		after := New(nil, nil)
		after.AddAll("hello", "world")
		ctx := New(nil, after)
		assertCtx(t, map[string]interface{}{
			"hello": "world",
		}, ctx)
	})

	t.Run("new empty with non-empty before and after", func(t *testing.T) {
		before := New(nil, nil)
		before.AddAll("before", "hello", "overwrite", 1)

		after := New(nil, nil)
		after.AddAll("after", "world", "overwrite", 2)

		ctx := New(before, after)
		assertCtx(t, map[string]interface{}{
			"before":    "hello",
			"after":     "world",
			"overwrite": 2,
		}, ctx)
	})

	t.Run("new context overwrites before elements", func(t *testing.T) {
		before := New(nil, nil)
		before.AddAll("before", "hello", "overwrite", 1)

		ctx := New(before, nil)
		ctx.AddAll("overwrite", 2)
		assertCtx(t, map[string]interface{}{
			"before":    "hello",
			"overwrite": 2,
		}, ctx)
	})

	t.Run("new context does not overwrite before elements", func(t *testing.T) {
		after := New(nil, nil)
		after.AddAll("hello", "world", "overwrite", 1)

		ctx := New(nil, after)
		ctx.AddAll("overwrite", 2)
		assertCtx(t, map[string]interface{}{
			"hello":     "world",
			"overwrite": 1,
		}, ctx)
	})
}

func TestCtxAdd(t *testing.T) {
	ctx := New(nil, nil)
	ctx.Add("hello", fld.ValString("world"))
	assertCtx(t, map[string]interface{}{
		"hello": "world",
	}, ctx)
}

func TestCtxAddAll(t *testing.T) {
	cases := map[string]struct {
		in   []interface{}
		want map[string]interface{}
	}{
		"unique keys": {
			in:   []interface{}{"key1", 1, "key2", 2},
			want: map[string]interface{}{"key1": 1, "key2": 2},
		},
		"duplicate keys": {
			in:   []interface{}{"key", 1, "key", 2},
			want: map[string]interface{}{"key": 2},
		},
		"accepts fld.Value": {
			in:   []interface{}{"key", fld.ValInt(10)},
			want: map[string]interface{}{"key": 10},
		},
		"accepts fld.Field": {
			in:   []interface{}{fld.Field{Key: "key", Value: fld.ValInt(10)}},
			want: map[string]interface{}{"key": 10},
		},
		"mix fields with key values": {
			in: []interface{}{
				"before", "hello",
				fld.Field{Key: "key", Value: fld.ValInt(2)},
				"after", "world",
			},
			want: map[string]interface{}{
				"before": "hello",
				"key":    2,
				"after":  "world",
			},
		},
	}

	for name, test := range cases {
		t.Run(name, func(t *testing.T) {
			ctx := New(nil, nil)
			ctx.AddAll(test.in...)
			assertCtx(t, test.want, ctx)
		})
	}
}

func TestCtxAddField(t *testing.T) {
	t.Run("user field", func(t *testing.T) {
		ctx := New(nil, nil)
		ctx.AddField(fld.String("hello", "world"))
		assertCtx(t, map[string]interface{}{
			"hello": "world",
		}, ctx)
		assert.Equal(t, 1, ctx.totUser)
		assert.Equal(t, 0, ctx.totStd)
	})

	t.Run("standardized field", func(t *testing.T) {
		f := fld.String("hello", "world")
		f.Standardized = true
		ctx := New(nil, nil)
		ctx.AddField(f)
		assertCtx(t, map[string]interface{}{
			"hello": "world",
		}, ctx)
		assert.Equal(t, 0, ctx.totUser)
		assert.Equal(t, 1, ctx.totStd)
	})
}

func TestCtxAddFields(t *testing.T) {
	cases := map[string]struct {
		in        []fld.Field
		want      map[string]interface{}
		user, std int
	}{
		"unique keys": {
			in: []fld.Field{
				fld.Int("key1", 1),
				fld.Int("key2", 2),
			},
			want: map[string]interface{}{"key1": 1, "key2": 2},
			user: 2,
		},
		"duplicate keys": {
			in: []fld.Field{
				fld.Int("key", 1),
				fld.Int("key", 2),
			},
			want: map[string]interface{}{"key": 2},
			user: 2, // both keys are stored
		},
		"standardized and user fields": {
			in: []fld.Field{
				fld.Int("key", 1),
				fld.Field{Key: "test", Value: fld.ValInt(2), Standardized: true},
			},
			want: map[string]interface{}{"key": 1, "test": 2},
			user: 1,
			std:  1,
		},
	}

	for name, test := range cases {
		t.Run(name, func(t *testing.T) {
			ctx := New(nil, nil)
			ctx.AddFields(test.in...)
			assertCtx(t, test.want, ctx)
			assert.Equal(t, test.user, ctx.totUser)
			assert.Equal(t, test.std, ctx.totStd)
		})
	}
}

func TestCtxFiltered(t *testing.T) {
	filtLocal := (*Ctx).Local
	filtUser := (*Ctx).User
	filtStd := (*Ctx).Standardized

	cases := map[string]struct {
		in     *Ctx
		filter func(*Ctx) Ctx
		want   map[string]interface{}
		len    int // number of entries in new context
	}{
		"local filter ignores before": {
			in: makeCtx(makeCtx(nil, nil, "before", "hello"), nil,
				"current", "world"),
			filter: filtLocal,
			want: map[string]interface{}{
				"current": "world",
			},
			len: 1,
		},
		"local filter ignores after": {
			in: makeCtx(nil, makeCtx(nil, nil, "after", "world"),
				"key", "value"),
			filter: filtLocal,
			want: map[string]interface{}{
				"key": "value",
			},
			len: 1,
		},

		"user filter transitive": {
			in: makeCtx(
				makeCtx(nil, nil, fld.String("user_before", "test"),
					fld.Field{Key: "std_before", Value: fld.ValInt(1), Standardized: true}),
				makeCtx(nil, nil, fld.String("user_after", "test"),
					fld.Field{Key: "std_after", Value: fld.ValInt(3), Standardized: true}),
				fld.String("user_local", "test"),
				fld.Field{Key: "std_local", Value: fld.ValInt(2), Standardized: true}),
			filter: filtUser,
			want: map[string]interface{}{
				"user_before": "test",
				"user_local":  "test",
				"user_after":  "test",
			},
			len: 3,
		},

		"standardized filter transitive": {
			in: makeCtx(
				makeCtx(nil, nil, fld.String("user_before", "test"),
					fld.Field{Key: "std_before", Value: fld.ValInt(1), Standardized: true}),
				makeCtx(nil, nil, fld.String("user_after", "test"),
					fld.Field{Key: "std_after", Value: fld.ValInt(3), Standardized: true}),
				fld.String("user_local", "test"),
				fld.Field{Key: "std_local", Value: fld.ValInt(2), Standardized: true}),
			filter: filtStd,
			want: map[string]interface{}{
				"std_before": 1,
				"std_local":  2,
				"std_after":  3,
			},
			len: 3,
		},
	}

	for name, test := range cases {
		t.Run(name, func(t *testing.T) {
			ctx := test.filter(test.in)
			assertCtx(t, test.want, &ctx)
			assert.Equal(t, test.len, ctx.Len())
		})
	}
}

func TestCtxVisitKeyValues(t *testing.T) {
	ctx := makeCtx(nil, nil,
		fld.String("a.b.field1", "test"),
		fld.String("a.b.field2", "test"),
		fld.Int("a.c.field1", 1),
		fld.Int("a.c.field2", 2),
		fld.Int("z.c", 5),
		fld.Int("z.d", 6))

	var v testVisitor
	require.NoError(t, ctx.VisitKeyValues(&v))

	assert.Equal(t, map[string]interface{}{
		"a.b.field1": "test",
		"a.b.field2": "test",
		"a.c.field1": 1,
		"a.c.field2": 2,
		"z.c":        5,
		"z.d":        6,
	}, v.Get())
}

func TestCtxVisitStructured(t *testing.T) {
	ctx := makeCtx(nil, nil,
		fld.String("a.b.field1", "test"),
		fld.String("a.b.field2", "test"),
		fld.Int("a.c.field1", 1),
		fld.Int("a.c.field2", 2),
		fld.Int("z.c", 5),
		fld.Int("z.d", 6))

	var v testVisitor
	require.NoError(t, ctx.VisitStructured(&v))

	assert.Equal(t, map[string]interface{}{
		"a": map[string]interface{}{
			"b": map[string]interface{}{
				"field1": "test",
				"field2": "test",
			},
			"c": map[string]interface{}{
				"field1": 1,
				"field2": 2,
			},
		},
		"z": map[string]interface{}{
			"c": 5,
			"d": 6,
		},
	}, v.Get())
}

func makeCtx(before, after *Ctx, vs ...interface{}) *Ctx {
	ctx := New(before, after)
	ctx.AddAll(vs...)
	return ctx
}

func assertCtx(t *testing.T, expected map[string]interface{}, ctx *Ctx) bool {
	var v testVisitor
	ctx.VisitStructured(&v)
	return assert.Equal(t, expected, v.Get())
}

func assertFlatCtx(t *testing.T, expected map[string]interface{}, ctx *Ctx) bool {
	var v testVisitor
	ctx.VisitKeyValues(&v)
	return assert.Equal(t, expected, v.Get())
}

func (v *testVisitor) Get() map[string]interface{} {
	return v.M
}

func (v *testVisitor) OnValue(key string, val fld.Value) error {
	if v.M == nil {
		v.M = map[string]interface{}{}
	}

	v.M[key] = val.Interface()
	return nil
}

func (v *testVisitor) OnObjStart(key string) error {
	v.keys = append(v.keys, key)
	v.stack = append(v.stack, v.M)
	v.M = nil
	return nil
}

func (v *testVisitor) OnObjEnd() error {
	keysEnd := len(v.keys) - 1
	key := v.keys[keysEnd]
	v.keys = v.keys[:keysEnd]

	m := v.M
	mapsEnd := len(v.stack) - 1
	v.M = v.stack[mapsEnd]
	v.stack = v.stack[:mapsEnd]

	if m != nil {
		if v.M == nil {
			v.M = map[string]interface{}{}
		}
		v.M[key] = m
	}

	return nil
}
