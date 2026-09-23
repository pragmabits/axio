package logline_test

import (
	"strings"
	"testing"

	"github.com/pragmabits/axio/internal/logline"
)

const (
	firstHash  = "780ae54885c0990d1a0ff8e0a9a27cda128c6f22665331d05789982ce28f439c"
	secondHash = "876e10d7999c558e44c69ce8b241142212de3f7ec80e33cb5798d1405b597e37"
)

func TestShortHash(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{firstHash, "780ae5"},
		{"abc", "abc"},
		{"", ""},
	}

	for _, test := range tests {
		assertEqual(t, logline.ShortHash(test.input), test.want)
	}
}

func TestFieldKey(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"order_id", "order_id"},
		{"message", "_message"},
		{"hash", "_hash"},
		{"errorVerbose", "_errorVerbose"},
		{"_message", "_message"},
		{"Message", "Message"},
	}

	for _, test := range tests {
		assertEqual(t, logline.FieldKey(test.input), test.want)
	}
}

func TestAppendTrailer(t *testing.T) {
	t.Run("closes_body_with_both_hashes_and_newline", func(t *testing.T) {
		line := logline.AppendTrailer([]byte(`{"level":"info"`), firstHash, secondHash)
		assertEqual(t, string(line), `{"level":"info","previous_hash":"`+firstHash+`","hash":"`+secondHash+`"}`+"\n")
	})

	t.Run("first_entry_carries_empty_previous_hash", func(t *testing.T) {
		line := logline.AppendTrailer([]byte(`{"level":"info"`), "", firstHash)
		assertEqual(t, string(line), `{"level":"info","previous_hash":"","hash":"`+firstHash+`"}`+"\n")
	})

	t.Run("does_not_write_into_body", func(t *testing.T) {
		backing := make([]byte, 0, 256)
		body := append(backing, `{"level":"info"`...)
		logline.AppendTrailer(body, "", firstHash)
		if strings.Contains(string(backing[:cap(backing)][len(body):]), "hash") {
			t.Error("AppendTrailer wrote into the spare capacity of body")
		}
	})
}

func TestSplitTrailer(t *testing.T) {
	t.Run("round_trips_append_trailer", func(t *testing.T) {
		line := logline.AppendTrailer([]byte(`{"level":"info","message":"order created"`), firstHash, secondHash)
		body, previousHash, hash, ok := logline.SplitTrailer(line)
		assertEqual(t, ok, true)
		assertEqual(t, string(body), `{"level":"info","message":"order created"`)
		assertEqual(t, previousHash, firstHash)
		assertEqual(t, hash, secondHash)
	})

	t.Run("accepts_line_without_newline", func(t *testing.T) {
		line := strings.TrimSuffix(string(logline.AppendTrailer([]byte(`{"a":1`), "", firstHash)), "\n")
		_, _, _, ok := logline.SplitTrailer([]byte(line))
		assertEqual(t, ok, true)
	})

	t.Run("ignores_trailer_text_inside_a_string_value", func(t *testing.T) {
		body := `{"note":"x\",\"previous_hash\":\"\",\"hash\":\"` + firstHash + `\"}"`
		line := logline.AppendTrailer([]byte(body), "", secondHash)
		gotBody, _, hash, ok := logline.SplitTrailer(line)
		assertEqual(t, ok, true)
		assertEqual(t, string(gotBody), body)
		assertEqual(t, hash, secondHash)
	})

	rejected := []struct {
		name string
		line string
	}{
		{name: "no_trailer", line: `{"level":"info"}`},
		{name: "hash_too_short", line: `{"a":1,"previous_hash":"","hash":"abc"}`},
		{name: "trailer_not_last", line: `{"previous_hash":"","hash":"` + firstHash + `","a":1}`},
		{name: "not_json", line: `plain text`},
	}
	for _, test := range rejected {
		t.Run(test.name, func(t *testing.T) {
			_, _, _, ok := logline.SplitTrailer([]byte(test.line))
			assertEqual(t, ok, false)
		})
	}
}

func TestBody(t *testing.T) {
	body, ok := logline.Body([]byte(`{"level":"info","message":"x"}` + "\n"))
	assertEqual(t, ok, true)
	assertEqual(t, string(body), `{"level":"info","message":"x"`)

	_, ok = logline.Body([]byte("not an object\n"))
	assertEqual(t, ok, false)
}
