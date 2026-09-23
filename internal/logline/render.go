package logline

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// RenderOptions configures a [Renderer].
type RenderOptions struct {
	// Color colors the level column the way the Console does.
	Color bool
	// Location is the time zone timestamps are shown in. Nil means the local
	// time zone, as the Console uses.
	Location *time.Location
}

// Renderer turns axio JSON lines back into the Console's text.
//
// It does not reimplement the text format: it decodes a JSON line into the
// entry and fields it was written from and hands them to the encoder the
// Console uses, so the text matches the Console byte for byte and keeps
// matching when the format changes. Keys that only the JSON carries — the
// service metadata and the previous hash — are dropped, and the hash is
// shortened, as the Console does. Lines that are not axio entries or events
// pass through unchanged.
type Renderer struct {
	entryEncoder zapcore.Encoder
	eventEncoder zapcore.Encoder
	location     *time.Location
}

// NewRenderer returns a Renderer configured by options.
func NewRenderer(options RenderOptions) *Renderer {
	location := options.Location
	if location == nil {
		location = time.Local
	}

	entryConfig := ConsoleEncoderConfig()
	if !options.Color {
		entryConfig.EncodeLevel = zapcore.CapitalLevelEncoder
	}
	eventConfig := ConsoleEncoderConfig()
	eventConfig.EncodeLevel = eventLevelEncoder(options.Color)

	return &Renderer{
		entryEncoder: zapcore.NewConsoleEncoder(entryConfig),
		eventEncoder: zapcore.NewConsoleEncoder(eventConfig),
		location:     location,
	}
}

// Render copies reader to writer one line at a time, rendering every axio
// line and passing every other line through unchanged. Each line is written as
// soon as it is read, so a stream that never ends — kubectl logs -f — renders
// as it arrives. Lines have no length limit.
func (r *Renderer) Render(reader io.Reader, writer io.Writer) error {
	lines := bufio.NewReader(reader)
	for {
		line, readErr := lines.ReadBytes('\n')
		if len(line) > 0 {
			if _, err := writer.Write(r.Line(line)); err != nil {
				return fmt.Errorf("write rendered line: %w", err)
			}
		}
		if errors.Is(readErr, io.EOF) {
			return nil
		}
		if readErr != nil {
			return fmt.Errorf("read line: %w", readErr)
		}
	}
}

// Line renders one line, line ending included. Anything before the JSON
// object — the pod name kubectl logs --prefix adds, the service name docker
// compose adds — is kept in front of the text. A line that is not an axio
// entry or event comes back unchanged.
func (r *Renderer) Line(line []byte) []byte {
	content := bytes.TrimRight(line, "\r\n")
	start := bytes.IndexByte(content, '{')
	if start < 0 {
		return line
	}
	decoded, err := decode(content[start:], r.location)
	if err != nil {
		return line
	}

	encoder := r.entryEncoder
	if decoded.isEvent {
		encoder = r.eventEncoder
	}
	encoded, err := encoder.EncodeEntry(decoded.entry, decoded.fields)
	if err != nil {
		return line
	}
	defer encoded.Free()

	rendered := make([]byte, 0, start+encoded.Len())
	rendered = append(rendered, content[:start]...)
	return append(rendered, encoded.Bytes()...)
}

// record is a JSON line decoded back into what the logger wrote it from.
type record struct {
	entry    zapcore.Entry
	fields   []zapcore.Field
	hasTime  bool
	hasLevel bool
	isEvent  bool
}

// set stores one key of the line in the record.
func (r *record) set(key string, raw json.RawMessage, location *time.Location) error {
	if setter, ok := reservedKeys[key]; ok {
		return setter(r, raw, location)
	}
	r.fields = append(r.fields, zap.Any(key, rawValue{raw}))
	return nil
}

// complete reports whether the line had what every axio line has: a
// timestamp, and a level or an event name.
func (r *record) complete() bool {
	return r.hasTime && (r.hasLevel || r.isEvent)
}

// rawValue hands zap a field value exactly as it appeared in the JSON line,
// so the text shows it byte for byte: the field order of a struct and the
// escapes of a string included.
type rawValue struct {
	data json.RawMessage
}

// MarshalJSON returns the value as it appeared in the line.
func (r rawValue) MarshalJSON() ([]byte, error) {
	return r.data, nil
}

// errNotAnEntry reports a line that is valid text but not an axio entry or event.
var errNotAnEntry = errors.New("not an axio entry")

// reservedKeys decode the keys that the Console writes outside the fields
// object, and drop the keys that only the JSON carries.
var reservedKeys = map[string]func(*record, json.RawMessage, *time.Location) error{
	TimeKey:         setTime,
	LevelKey:        setLevel,
	LoggerKey:       setLogger,
	CallerKey:       setCaller,
	MessageKey:      setMessage,
	EventKey:        setEvent,
	StacktraceKey:   setStacktrace,
	HashKey:         setHash,
	ServiceKey:      skipKey,
	DeploymentKey:   skipKey,
	PreviousHashKey: skipKey,
}

// decode reads a JSON object back into the entry and fields it was written from.
func decode(object []byte, location *time.Location) (record, error) {
	decoder := json.NewDecoder(bytes.NewReader(object))
	if token, err := decoder.Token(); err != nil || token != json.Delim('{') {
		return record{}, errNotAnEntry
	}

	var decoded record
	for decoder.More() {
		key, raw, err := nextMember(decoder)
		if err != nil {
			return record{}, err
		}
		if err := decoded.set(key, raw, location); err != nil {
			return record{}, err
		}
	}
	if _, err := decoder.Token(); err != nil || decoder.More() || !decoded.complete() {
		return record{}, errNotAnEntry
	}
	return decoded, nil
}

// nextMember reads one key and its raw value from an object.
func nextMember(decoder *json.Decoder) (string, json.RawMessage, error) {
	token, err := decoder.Token()
	if err != nil {
		return "", nil, err
	}
	key, ok := token.(string)
	if !ok {
		return "", nil, errNotAnEntry
	}
	var raw json.RawMessage
	if err := decoder.Decode(&raw); err != nil {
		return "", nil, err
	}
	return key, raw, nil
}

func setTime(decoded *record, raw json.RawMessage, location *time.Location) error {
	var text string
	if err := decodeString(raw, &text); err != nil {
		return err
	}
	parsed, err := time.Parse(time.RFC3339Nano, text)
	if err != nil {
		return err
	}
	decoded.entry.Time = parsed.In(location)
	decoded.hasTime = true
	return nil
}

func setLevel(decoded *record, raw json.RawMessage, _ *time.Location) error {
	var text string
	if err := decodeString(raw, &text); err != nil {
		return err
	}
	level, err := zapcore.ParseLevel(text)
	if err != nil {
		return err
	}
	decoded.entry.Level = level
	decoded.hasLevel = true
	return nil
}

func setLogger(decoded *record, raw json.RawMessage, _ *time.Location) error {
	return decodeString(raw, &decoded.entry.LoggerName)
}

func setCaller(decoded *record, raw json.RawMessage, _ *time.Location) error {
	var text string
	if err := decodeString(raw, &text); err != nil {
		return err
	}
	separator := strings.LastIndexByte(text, ':')
	if separator < 0 {
		return errNotAnEntry
	}
	line, err := strconv.Atoi(text[separator+1:])
	if err != nil {
		return err
	}
	decoded.entry.Caller = zapcore.EntryCaller{Defined: true, File: text[:separator], Line: line}
	return nil
}

func setMessage(decoded *record, raw json.RawMessage, _ *time.Location) error {
	return decodeString(raw, &decoded.entry.Message)
}

func setEvent(decoded *record, raw json.RawMessage, _ *time.Location) error {
	decoded.isEvent = true
	return decodeString(raw, &decoded.entry.Message)
}

func setStacktrace(decoded *record, raw json.RawMessage, _ *time.Location) error {
	return decodeString(raw, &decoded.entry.Stack)
}

func setHash(decoded *record, raw json.RawMessage, _ *time.Location) error {
	var hash string
	if err := decodeString(raw, &hash); err != nil {
		return err
	}
	decoded.fields = append(decoded.fields, zap.String(HashKey, ShortHash(hash)))
	return nil
}

func skipKey(*record, json.RawMessage, *time.Location) error {
	return nil
}

// decodeString decodes a JSON string value into target.
func decodeString(raw json.RawMessage, target *string) error {
	return json.Unmarshal(raw, target)
}

// eventLevelEncoder writes EVENT in the level column of an event, cyan when
// colored: the Console already uses magenta for DEBUG.
func eventLevelEncoder(color bool) zapcore.LevelEncoder {
	label := "EVENT"
	if color {
		label = "\x1b[36mEVENT\x1b[0m"
	}
	return func(_ zapcore.Level, encoder zapcore.PrimitiveArrayEncoder) {
		encoder.AppendString(label)
	}
}
