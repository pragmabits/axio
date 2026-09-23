package logline_test

import (
	"testing"
	"time"

	"github.com/pragmabits/axio/internal/logline"
)

// arrayEncoder is a minimal PrimitiveArrayEncoder for benchmarking.
type arrayEncoder struct {
	last string
}

func (a *arrayEncoder) AppendBool(bool)             {}
func (a *arrayEncoder) AppendByteString([]byte)     {}
func (a *arrayEncoder) AppendComplex128(complex128) {}
func (a *arrayEncoder) AppendComplex64(complex64)   {}
func (a *arrayEncoder) AppendFloat64(float64)       {}
func (a *arrayEncoder) AppendFloat32(float32)       {}
func (a *arrayEncoder) AppendInt(int)               {}
func (a *arrayEncoder) AppendInt64(int64)           {}
func (a *arrayEncoder) AppendInt32(int32)           {}
func (a *arrayEncoder) AppendInt16(int16)           {}
func (a *arrayEncoder) AppendInt8(int8)             {}
func (a *arrayEncoder) AppendString(value string)   { a.last = value }
func (a *arrayEncoder) AppendUint(uint)             {}
func (a *arrayEncoder) AppendUint64(uint64)         {}
func (a *arrayEncoder) AppendUint32(uint32)         {}
func (a *arrayEncoder) AppendUint16(uint16)         {}
func (a *arrayEncoder) AppendUint8(uint8)           {}
func (a *arrayEncoder) AppendUintptr(uintptr)       {}

func BenchmarkEncodeTime(b *testing.B) {
	timestamp := time.Date(2025, 3, 26, 12, 30, 45, 123456789, time.UTC)
	encoder := &arrayEncoder{}

	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		logline.EncodeTime(timestamp, encoder)
	}
}
