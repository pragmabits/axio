package logline_test

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/pragmabits/axio"
	"github.com/pragmabits/axio/internal/logline"
)

// shipment is a struct annotation whose fields are not in alphabetical order.
type shipment struct {
	Zone    string  `json:"zone"`
	Carrier string  `json:"carrier"`
	Weight  float64 `json:"weight"`
}

func TestRenderer_MatchesConsole(t *testing.T) {
	tests := []struct {
		name    string
		config  axio.Config
		audited bool
	}{
		{
			name:   "development",
			config: axio.Config{ServiceName: "checkout", Environment: axio.Development, Level: axio.LevelDebug},
		},
		{
			name:   "production_with_stacktrace",
			config: axio.Config{ServiceName: "checkout", ServiceVersion: "1.4.2", InstanceID: "pod-7f9c", Environment: axio.Production, Level: axio.LevelDebug},
		},
		{
			name:    "production_audited",
			config:  axio.Config{ServiceName: "checkout", ServiceVersion: "1.4.2", InstanceID: "pod-7f9c", Environment: axio.Production, Level: axio.LevelDebug},
			audited: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			text, structured := newMemoryOutput(axio.FormatText), newMemoryOutput(axio.FormatJSON)
			options := []axio.Option{axio.WithOutputs(text, structured)}
			if test.audited {
				chain, err := axio.NewHashChain(nil)
				assertNoError(t, err)
				options = append(options, axio.WithAuditChain(chain))
			}
			logger, err := axio.New(test.config, options...)
			assertNoError(t, err)
			writeSamples(logger)
			assertNoError(t, logger.Close())

			var rendered bytes.Buffer
			assertNoError(t, logline.NewRenderer(logline.RenderOptions{Color: true}).Render(strings.NewReader(structured.String()), &rendered))

			if rendered.String() != text.String() {
				t.Errorf("rendered JSON differs from the Console\n--- rendered\n%s\n--- console\n%s", rendered.String(), text.String())
			}
		})
	}
}

func TestRenderer_Line(t *testing.T) {
	const entry = `{"level":"info","timestamp":"2026-09-23T14:02:55.603542381Z","logger":"orders","caller":"probe/main.go:22","message":"order created","service":{"name":"checkout"},"deployment":{"environment":{"name":"production"}},"order_id":"ord_8812","previous_hash":"","hash":"780ae54885c0990d1a0ff8e0a9a27cda128c6f22665331d05789982ce28f439c"}`
	const renderedEntry = "2026-09-23T14:02:55.603Z\tINFO\torders\tprobe/main.go:22\torder created\t{\"order_id\": \"ord_8812\", \"hash\": \"780ae5\"}\n"
	const event = `{"timestamp":"2026-09-23T14:44:45.897774932Z","event":"http_request","route":"/api/v1/orders","status_code":502,"duration_ms":3}`

	tests := []struct {
		name string
		line string
		want string
	}{
		{name: "entry_drops_json_only_keys_and_shortens_hash", line: entry + "\n", want: renderedEntry},
		{name: "event_takes_event_in_the_level_column", line: event + "\n", want: "2026-09-23T14:44:45.897Z\tEVENT\thttp_request\t{\"route\": \"/api/v1/orders\", \"status_code\": 502, \"duration_ms\": 3}\n"},
		{name: "prefix_before_json_is_kept", line: "[pod/checkout-7f9c/app] " + entry + "\n", want: "[pod/checkout-7f9c/app] " + renderedEntry},
		{name: "crlf_line_is_rendered", line: entry + "\r\n", want: renderedEntry},
		{name: "plain_text_passes_through", line: "starting checkout 1.4.2\n", want: "starting checkout 1.4.2\n"},
		{name: "json_that_is_not_axio_passes_through", line: `{"msg":"from another library"}` + "\n", want: `{"msg":"from another library"}` + "\n"},
		{name: "malformed_json_passes_through", line: `{"level":"info",` + "\n", want: `{"level":"info",` + "\n"},
		{name: "text_after_the_object_passes_through", line: entry + " trailing\n", want: entry + " trailing\n"},
		{name: "unknown_level_passes_through", line: `{"level":"loud","timestamp":"2026-09-23T14:02:55Z","message":"x"}` + "\n", want: `{"level":"loud","timestamp":"2026-09-23T14:02:55Z","message":"x"}` + "\n"},
		{name: "line_without_newline_keeps_none", line: "tail without newline", want: "tail without newline"},
	}

	renderer := logline.NewRenderer(logline.RenderOptions{Location: time.UTC})
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assertEqual(t, string(renderer.Line([]byte(test.line))), test.want)
		})
	}
}

func TestRenderer_Options(t *testing.T) {
	const entry = `{"level":"warn","timestamp":"2026-09-23T14:02:55.603542381Z","message":"payment retry"}` + "\n"

	t.Run("location_sets_the_time_zone", func(t *testing.T) {
		saoPaulo := time.FixedZone("-03", -3*3600)
		rendered := string(logline.NewRenderer(logline.RenderOptions{Location: saoPaulo}).Line([]byte(entry)))
		if !strings.HasPrefix(rendered, "2026-09-23T11:02:55.603-0300\t") {
			t.Errorf("expected the time in -0300, got %q", rendered)
		}
	})

	t.Run("color_wraps_the_level_in_escape_codes", func(t *testing.T) {
		colored := string(logline.NewRenderer(logline.RenderOptions{Color: true, Location: time.UTC}).Line([]byte(entry)))
		plain := string(logline.NewRenderer(logline.RenderOptions{Location: time.UTC}).Line([]byte(entry)))
		if !strings.Contains(colored, "\x1b[33mWARN\x1b[0m") {
			t.Errorf("expected a yellow WARN, got %q", colored)
		}
		if strings.Contains(plain, "\x1b[") {
			t.Errorf("expected no escape codes, got %q", plain)
		}
	})
}

func TestRenderer_Render(t *testing.T) {
	t.Run("renders_every_line_and_passes_the_rest", func(t *testing.T) {
		input := "starting\n" + `{"level":"info","timestamp":"2026-09-23T14:02:55.603542381Z","message":"ready"}` + "\nstopping"
		var output bytes.Buffer
		assertNoError(t, logline.NewRenderer(logline.RenderOptions{Location: time.UTC}).Render(strings.NewReader(input), &output))
		assertEqual(t, output.String(), "starting\n2026-09-23T14:02:55.603Z\tINFO\tready\nstopping")
	})

	t.Run("reads_lines_longer_than_64KB", func(t *testing.T) {
		blob := strings.Repeat("x", 300_000)
		input := `{"level":"info","timestamp":"2026-09-23T14:02:55.603542381Z","message":"big","blob":"` + blob + `"}` + "\n"
		var output bytes.Buffer
		assertNoError(t, logline.NewRenderer(logline.RenderOptions{Location: time.UTC}).Render(strings.NewReader(input), &output))
		if !strings.Contains(output.String(), `"blob": "`+blob+`"`) {
			t.Errorf("long field lost, got %d bytes", output.Len())
		}
	})

	t.Run("returns_the_write_error", func(t *testing.T) {
		err := logline.NewRenderer(logline.RenderOptions{}).Render(strings.NewReader("a\nb\n"), failingWriter{})
		if !errors.Is(err, errWriteFailed) {
			t.Errorf("expected the write error, got %v", err)
		}
	})
}

// writeSamples writes entries that exercise every part of the text format.
func writeSamples(logger axio.Logger) {
	ctx := context.Background()
	orders := logger.Named("orders").With(axio.Annotate("order_id", "ord_8812"))
	orders.Info(ctx, "order created")
	orders.With(
		axio.Annotate("note", "<a&b> \"quoted\" ação\ttab"),
		axio.Annotate("ratio", 3.14159),
		axio.Annotate("express", true),
		axio.Annotate("shipment", shipment{Zone: "SP", Carrier: "correios", Weight: 1.25}),
		axio.Annotate("http", axio.HTTP{Method: "POST", URL: "/api/v1/orders", StatusCode: 201, LatencyMS: 45}),
	).Debug(ctx, "tricky values")
	orders.Warn(ctx, errors.New("gateway timeout"), "payment retry %d of %d", 2, 3)
	orders.Error(ctx, errors.New("card declined\nsecond line"), "payment failed")
}

var errWriteFailed = errors.New("write failed")

// failingWriter is an io.Writer whose every write fails.
type failingWriter struct{}

func (failingWriter) Write([]byte) (int, error) { return 0, errWriteFailed }
