package axio

import (
	"context"
	"testing"
)

func TestNoopTracer(t *testing.T) {
	tracer := NoopTracer{}
	traceID, spanID, ok := tracer.Extract(context.Background())

	assertEqual(t, traceID, "")
	assertEqual(t, spanID, "")
	if ok {
		t.Error("NoopTracer should return false")
	}
}

func TestNoopTracing(t *testing.T) {
	tracer := NoopTracing()
	if tracer == nil {
		t.Error("NoopTracing should return a non-nil tracer")
	}

	_, ok := tracer.(NoopTracer)
	if !ok {
		t.Error("NoopTracing should return a NoopTracer")
	}
}

func TestOtel_no_span(t *testing.T) {
	tracer := Otel()
	traceID, spanID, ok := tracer.Extract(context.Background())

	assertEqual(t, traceID, "")
	assertEqual(t, spanID, "")
	if ok {
		t.Error("should return false when no span in context")
	}
}

func TestBuildTracer(t *testing.T) {
	t.Run("custom_tracer_takes_precedence", func(t *testing.T) {
		custom := NoopTracer{}
		config := Config{tracer: custom}
		tracer := BuildTracer(config)

		_, ok := tracer.(NoopTracer)
		if !ok {
			t.Error("should return custom tracer")
		}
	})

	t.Run("otel_type", func(t *testing.T) {
		config := Config{TracerType: "otel"}
		tracer := BuildTracer(config)

		_, ok := tracer.(*otelTraceExtractor)
		if !ok {
			t.Error("should return otelTraceExtractor")
		}
	})

	t.Run("noop_type", func(t *testing.T) {
		config := Config{TracerType: "noop"}
		tracer := BuildTracer(config)

		_, ok := tracer.(NoopTracer)
		if !ok {
			t.Error("should return NoopTracer")
		}
	})

	t.Run("empty_type", func(t *testing.T) {
		config := Config{TracerType: ""}
		tracer := BuildTracer(config)

		_, ok := tracer.(NoopTracer)
		if !ok {
			t.Error("should return NoopTracer for empty type")
		}
	})

	t.Run("unknown_type_falls_back_to_noop", func(t *testing.T) {
		config := Config{TracerType: "unknown"}
		tracer := BuildTracer(config)

		_, ok := tracer.(NoopTracer)
		if !ok {
			t.Error("should return NoopTracer for unknown type")
		}
	})
}
