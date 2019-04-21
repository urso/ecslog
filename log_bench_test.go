package ecslog

import (
	"fmt"
	"math/rand"
	"testing"

	"github.com/urso/ecslog/backend"
)

type benchBackend struct {
	Disabled bool
	Context  bool
}

func BenchmarkPureMessage(b *testing.B) {
	messages := []string{
		makeASCIIMessage(5),
		makeASCIIMessage(20),
		makeASCIIMessage(100),
		makeASCIIMessage(1000),
	}

	for _, msg := range messages {
		msg := msg
		b.Run(fmt.Sprintf("msg=%v", len(msg)), func(b *testing.B) {
			for _, withContext := range []bool{false, true} {
				b.Run(fmt.Sprintf("context=%v", withContext), func(b *testing.B) {
					testBackend := &benchBackend{Context: withContext}
					logger := New(testBackend)

					b.Run("log", func(b *testing.B) {
						for i := 0; i < b.N; i++ {
							logger.Info(msg)
						}
					})

					b.Run("logf", func(b *testing.B) {
						for i := 0; i < b.N; i++ {
							logger.Infof(msg)
						}
					})
				})
			}
		})
	}
}

func makeASCIIMessage(len int) string {
	gen := rngASCIIString(rngIntConst(len))
	return gen(rand.New(rand.NewSource(0)))
}

func rngIntRange(min, max int) func(*rand.Rand) int {
	return func(rng *rand.Rand) int {
		return rand.Intn(max-min) + min
	}
}

func rngIntConst(i int) func(*rand.Rand) int {
	return func(_ *rand.Rand) int { return i }
}

func rngASCIIString(len func(*rand.Rand) int) func(*rand.Rand) string {
	return func(rng *rand.Rand) string {
		L := len(rng)
		buf := make([]byte, L)
		for i := range buf {
			buf[i] = byte(rand.Intn('Z'-'0') + '0')
		}
		return string(buf)
	}
}

func (bb *benchBackend) For(_ string) backend.Backend   { return bb }
func (bb *benchBackend) IsEnabled(_ backend.Level) bool { return !bb.Disabled }
func (bb *benchBackend) UseContext() bool               { return bb.Context }
func (bb *benchBackend) Log(backend.Message)            {}
