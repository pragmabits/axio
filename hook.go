package axio

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// Entry represents a log entry passed to hooks for processing.
//
// Hooks can modify fields in-place before the entry is written to outputs.
// All fields are populated by the logger before calling hooks, except
// Hash and PreviousHash which are populated by [AuditHook].
type Entry struct {
	// Timestamp is the moment when the log was created.
	Timestamp time.Time
	// Level is the log severity.
	Level Level
	// Message is the formatted log message.
	Message string
	// Error is the error associated with the log, if any.
	Error error
	// Logger is the logger name (defined via Named).
	Logger string
	// Caller is the source code location (file:line).
	Caller string
	// TraceID is the distributed trace identifier (if available).
	TraceID string
	// SpanID is the span identifier (if available).
	SpanID string
	// Annotations contains the structured log fields.
	Annotations Annotations

	// Hash is the SHA256 hash of this entry (populated by AuditHook).
	Hash string
	// PreviousHash is the hash of the previous entry (populated by AuditHook).
	PreviousHash string
}

// Hook processes log entries before they are written to outputs.
//
// Hooks are executed in the order they were registered and can
// modify the log entry in-place. If a hook returns an error,
// processing is stopped and the entry is not written.
//
// Hooks included in the package:
//   - [PIIHook]: masks sensitive personal data
//   - [AuditHook]: adds hash chain for auditing
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
//	        axio.Annotate("tenant_id", h.tenantID))
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
//  2. AuditHook (hash chain for integrity)
//  3. Custom hooks (in the order passed to WithHooks)
//
// This order is intentional and not configurable:
//   - PII must mask BEFORE audit calculates hash, ensuring that
//     sensitive data never appears in the audit chain
//   - Custom hooks execute last to have access to the already
//     processed entry (with masked PII and calculated hash)
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

// noopHook is a hook that does nothing.
type noopHook struct{}

func (noopHook) Name() string                                    { return "noop" }
func (noopHook) Process(ctx context.Context, entry *Entry) error { return nil }

// NoopHook returns a hook that does nothing.
//
// Useful for tests or as a placeholder.
func NoopHook() Hook {
	return noopHook{}
}

// buildHooks creates hooks from configuration.
//
// Creation order follows the fixed execution order:
//  1. PIIHook (if PIIEnabled)
//  2. AuditHook (if Audit.Enabled)
//  3. Custom hooks (from WithHooks)
//
// PII must mask before audit calculates the hash; custom hooks run
// last and observe the already-masked and already-hashed entry.
func buildHooks(config Config) ([]Hook, error) {
	var hooks []Hook

	if config.PIIEnabled {
		piiConfig := PIIConfig{
			Patterns:       config.PIIPatterns,
			CustomPatterns: config.PIICustomPatterns,
			Fields:         config.PIIFields,
		}
		piiHook, err := NewPIIHook(piiConfig)
		if err != nil {
			return nil, fmt.Errorf("create PII hook: %w", err)
		}
		hooks = append(hooks, piiHook)
	}

	if config.Audit.Enabled {
		store := NewFileStore(config.Audit.StorePath)
		auditHook, err := NewAuditHook(store)
		if err != nil {
			return nil, fmt.Errorf("create audit hook: %w", err)
		}
		hooks = append(hooks, auditHook)
	}

	if len(config.hooks) > 0 {
		hooks = append(hooks, config.hooks...)
	}

	return hooks, nil
}
