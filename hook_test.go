package axio

import (
	"context"
	"errors"
	"sync"
	"testing"
)

func TestNewHookChain(t *testing.T) {
	t.Run("empty_chain", func(t *testing.T) {
		chain := NewHookChain(NoopMetrics{})
		assertEqual(t, chain.Len(), 0)
	})

	t.Run("with_hooks", func(t *testing.T) {
		chain := NewHookChain(NoopMetrics{}, NoopHook(), NoopHook())
		assertEqual(t, chain.Len(), 2)
	})

	t.Run("nil_metrics_uses_noop", func(t *testing.T) {
		chain := NewHookChain(nil, NoopHook())
		assertEqual(t, chain.Len(), 1)
		// Should not panic when processing
		err := chain.Process(context.Background(), &Entry{})
		assertNoError(t, err)
	})
}

func TestHookChain_Process(t *testing.T) {
	var mu sync.Mutex
	var order []string

	hookA := &testHook{name: "a", fn: func(ctx context.Context, entry *Entry) error {
		mu.Lock()
		order = append(order, "a")
		mu.Unlock()
		return nil
	}}
	hookB := &testHook{name: "b", fn: func(ctx context.Context, entry *Entry) error {
		mu.Lock()
		order = append(order, "b")
		mu.Unlock()
		return nil
	}}

	chain := NewHookChain(NoopMetrics{}, hookA, hookB)
	err := chain.Process(context.Background(), &Entry{})
	assertNoError(t, err)

	assertEqual(t, len(order), 2)
	assertEqual(t, order[0], "a")
	assertEqual(t, order[1], "b")
}

func TestHookChain_Process_error_stops_chain(t *testing.T) {
	called := false
	hookA := &testHook{name: "failing", fn: func(ctx context.Context, entry *Entry) error {
		return errors.New("hook error")
	}}
	hookB := &testHook{name: "after", fn: func(ctx context.Context, entry *Entry) error {
		called = true
		return nil
	}}

	chain := NewHookChain(NoopMetrics{}, hookA, hookB)
	err := chain.Process(context.Background(), &Entry{})

	assertError(t, err)
	if called {
		t.Error("second hook should not have been called after first returned error")
	}
}

func TestHookChain_Add(t *testing.T) {
	chain := NewHookChain(NoopMetrics{})
	assertEqual(t, chain.Len(), 0)

	called := false
	chain.Add(&testHook{name: "added", fn: func(ctx context.Context, entry *Entry) error {
		called = true
		return nil
	}})
	assertEqual(t, chain.Len(), 1)

	err := chain.Process(context.Background(), &Entry{})
	assertNoError(t, err)
	if !called {
		t.Error("added hook should have been called")
	}
}

func TestHookChain_MetricsAware(t *testing.T) {
	metrics := NoopMetrics{}
	hook := &metricsAwareHook{}

	chain := NewHookChain(metrics, hook)
	_ = chain

	if !hook.received {
		t.Error("MetricsAware hook should have received metrics")
	}
}

func TestNoopHook(t *testing.T) {
	hook := NoopHook()
	assertEqual(t, hook.Name(), "noop")

	err := hook.Process(context.Background(), &Entry{})
	assertNoError(t, err)
}

// test helpers

type testHook struct {
	name string
	fn   func(context.Context, *Entry) error
}

func (h *testHook) Name() string { return h.name }
func (h *testHook) Process(ctx context.Context, entry *Entry) error {
	return h.fn(ctx, entry)
}

type metricsAwareHook struct {
	received bool
}

func (h *metricsAwareHook) Name() string                                    { return "metrics-aware" }
func (h *metricsAwareHook) Process(ctx context.Context, entry *Entry) error { return nil }
func (h *metricsAwareHook) SetMetrics(metrics Metrics)                      { h.received = true }
