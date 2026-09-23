package logline_test

import (
	"bytes"
	"sync"
	"testing"

	"github.com/pragmabits/axio"
)

// assertEqual checks that got equals want.
func assertEqual[T comparable](t *testing.T, got, want T) {
	t.Helper()
	if got != want {
		t.Errorf("got %v, want %v", got, want)
	}
}

// assertNoError checks that err is nil.
func assertNoError(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

// memoryOutput is an axio.Output that keeps everything written to it in memory.
type memoryOutput struct {
	format axio.Format
	mutex  sync.Mutex
	buffer bytes.Buffer
}

// newMemoryOutput returns an empty memoryOutput encoding in format.
func newMemoryOutput(format axio.Format) *memoryOutput {
	return &memoryOutput{format: format}
}

func (m *memoryOutput) Format() axio.Format   { return m.format }
func (m *memoryOutput) Type() axio.OutputType { return axio.OutputStdout }
func (m *memoryOutput) Sync() error           { return nil }
func (m *memoryOutput) Close() error          { return nil }

func (m *memoryOutput) Write(data []byte) (int, error) {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	return m.buffer.Write(data)
}

// String returns everything written so far.
func (m *memoryOutput) String() string {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	return m.buffer.String()
}
