package axio

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// Entry represents a log entry passed to hooks for processing.
//
// A hook changes what is written through Message, Error, TraceID, SpanID and
// Annotations, in place, before the entry is written to outputs. Timestamp,
// Level, Logger and Caller describe the entry and are read only: changing them
// changes nothing written. With auditing on, the hash is computed when the
// entry is written, after every hook, so it covers whatever the hooks changed.
type Entry struct {
	// Timestamp is the moment when the log was created.
	Timestamp time.Time
	// Level is the log severity.
	Level Level
	// Message is the log message, or the name of an [Event].
	Message string
	// Error is the error associated with the log, if any.
	Error error
	// Logger is the logger name (defined via Named).
	Logger string
	// Caller is the source code location as the line writes it: the file's
	// directory, the file and the line, as in "checkout/handler.go:42". It is
	// empty with [WithOmitCaller] and for an [Event].
	Caller string
	// TraceID is the distributed trace identifier (if available).
	TraceID string
	// SpanID is the span identifier (if available).
	SpanID string
	// Annotations contains the structured log fields.
	Annotations Annotations
}

// Hook processes log entries before they are written to outputs.
//
// Hooks are executed in the order they were registered and can
// modify the log entry in-place. If a hook returns an error,
// processing is stopped and the entry is not written.
//
// Hooks included in the package:
//   - [PIIHook]: masks sensitive personal data
//
// Example of custom hook:
//
//	type TenantHook struct {
//	    tenantID string
//	}
//
//	func (h TenantHook) Name() string { return "tenant" }
//
//	func (h TenantHook) Process(ctx context.Context, entry *axio.Entry) error {
//	    entry.Annotations = append(entry.Annotations,
//	        axio.Field("tenant_id", h.tenantID))
//	    return nil
//	}
type Hook interface {
	// Name returns the hook identifier, used for metrics and debugging.
	Name() string
	// Process modifies the entry in-place. Return a non-nil error to abort
	// the write.
	//
	// The *Entry passed in is borrowed from an internal pool and is only
	// valid for the duration of the call. Implementations must not retain
	// the pointer, store it for async use, or hand it to another goroutine;
	// copy any needed values before returning.
	Process(ctx context.Context, entry *Entry) error
}

// MetricsAware indicates that a hook can emit metrics.
//
// Hooks that implement this interface will receive the Metrics object
// automatically during logger construction.
type MetricsAware interface {
	// SetMetrics configures the metrics object for the hook.
	SetMetrics(metrics Metrics)
}

// hookChain manages a sequence of hooks executed in order.
//
// The chain is built by the logger from hooks passed via [WithHooks].
//
// # Execution Order
//
// Hooks are executed in the following fixed order:
//
//  1. PIIHook (sensitive data masking)
//  2. Custom hooks (in the order passed to WithHooks)
//
// This order is intentional and not configurable: custom hooks see the entry
// with PII already masked. Auditing is not a hook: the hash is computed when
// the entry is written, after the whole chain, so it covers what every hook
// changed and sensitive data never reaches the audit chain.
type hookChain struct {
	hooks   []Hook
	metrics Metrics
	mutex   sync.RWMutex
}

// newHookChain creates a new hook chain with metrics support.
//
// If metrics is nil, NoopMetrics will be used.
// Hooks that implement [MetricsAware] will receive the metrics object automatically.
func newHookChain(metrics Metrics, hooks ...Hook) *hookChain {
	if metrics == nil {
		metrics = NoopMetrics{}
	}

	chain := &hookChain{
		hooks:   hooks,
		metrics: metrics,
	}

	for _, hook := range hooks {
		if aware, ok := hook.(MetricsAware); ok {
			aware.SetMetrics(metrics)
		}
	}

	return chain
}

// add appends a hook to the end of the chain.
//
// If the hook implements [MetricsAware], it will receive the metrics object automatically.
func (c *hookChain) add(hook Hook) {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	c.hooks = append(c.hooks, hook)

	if aware, ok := hook.(MetricsAware); ok {
		aware.SetMetrics(c.metrics)
	}
}

// process executes all hooks in sequence on the entry.
//
// For each hook, records execution duration via metrics.
// If any hook returns an error, processing stops and the error is returned.
func (c *hookChain) process(ctx context.Context, entry *Entry) error {
	c.mutex.RLock()
	hooks := c.hooks
	c.mutex.RUnlock()

	for _, hook := range hooks {
		start := time.Now()
		err := hook.Process(ctx, entry)
		duration := time.Since(start)

		c.metrics.HookDuration(ctx, hook.Name(), duration, err != nil)
		if err != nil {
			return fmt.Errorf("hook '%s': %w", hook.Name(), err)
		}
	}
	return nil
}

// length returns the number of hooks in the chain.
func (c *hookChain) length() int {
	c.mutex.RLock()
	defer c.mutex.RUnlock()
	return len(c.hooks)
}

// readsCaller reports whether a hook in the chain may read [Entry.Caller]:
// any hook but a [PIIHook], which never does.
func (c *hookChain) readsCaller() bool {
	c.mutex.RLock()
	defer c.mutex.RUnlock()
	for _, hook := range c.hooks {
		if _, masks := hook.(*PIIHook); !masks {
			return true
		}
	}
	return false
}

// noopHook is a hook that does nothing.
type noopHook struct{}

// NoopHook returns a hook that does nothing.
//
// Useful for tests or as a placeholder.
//
// Example:
//
//	logger, err := axio.New(config, axio.WithHooks(axio.NoopHook()))
func NoopHook() Hook {
	return noopHook{}
}

func (noopHook) Name() string { return "noop" }

func (noopHook) Process(ctx context.Context, entry *Entry) error { return nil }

// buildHooks creates hooks from configuration.
//
// Creation order follows the fixed execution order:
//  1. PIIHook (unless PIIDisabled)
//  2. Custom hooks (from WithHooks)
//
// Custom hooks run after PII masking and observe the masked entry.
func buildHooks(config Config) ([]Hook, error) {
	var hooks []Hook

	if !config.PIIDisabled {
		piiConfig := PIIConfig{
			Patterns:         config.PIIPatterns,
			CustomPatterns:   config.PIICustomPatterns,
			Fields:           config.PIIFields,
			MaxDepth:         config.PIIMaxDepth,
			OmitErrorVerbose: config.PIIOmitErrorVerbose,
		}
		piiHook, err := NewPIIHook(piiConfig)
		if err != nil {
			return nil, fmt.Errorf("create PII hook: %w", err)
		}
		hooks = append(hooks, piiHook)
	}

	if len(config.hooks) > 0 {
		hooks = append(hooks, config.hooks...)
	}

	return hooks, nil
}
