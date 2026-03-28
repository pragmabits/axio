package axio

import (
	"encoding/json"
	"testing"
	"time"

	"gopkg.in/yaml.v3"
)

func TestDuration_UnmarshalText(t *testing.T) {
	t.Run("valid_durations", func(t *testing.T) {
		tests := []struct {
			input string
			want  Duration
		}{
			{"24h", Duration(24 * time.Hour)},
			{"1h30m", Duration(90 * time.Minute)},
			{"500ms", Duration(500 * time.Millisecond)},
			{"1s", Duration(time.Second)},
			{" 24h ", Duration(24 * time.Hour)}, // with spaces
		}

		for _, tt := range tests {
			var duration Duration
			err := duration.UnmarshalText([]byte(tt.input))
			assertNoError(t, err)
			assertEqual(t, duration, tt.want)
		}
	})

	t.Run("invalid_duration", func(t *testing.T) {
		var duration Duration
		err := duration.UnmarshalText([]byte("invalid"))
		assertError(t, err)
	})

	t.Run("empty_string", func(t *testing.T) {
		var duration Duration
		err := duration.UnmarshalText([]byte(""))
		assertError(t, err)
	})
}

func TestDuration_MarshalText(t *testing.T) {
	duration := Duration(24 * time.Hour)

	text, err := duration.MarshalText()
	assertNoError(t, err)
	assertEqual(t, string(text), "24h0m0s")
}

func TestDuration_YAMLRoundtrip(t *testing.T) {
	type wrapper struct {
		Interval Duration `yaml:"interval"`
	}

	original := wrapper{Interval: Duration(2 * time.Hour)}

	data, err := yaml.Marshal(original)
	assertNoError(t, err)

	var decoded wrapper
	err = yaml.Unmarshal(data, &decoded)
	assertNoError(t, err)
	assertEqual(t, decoded.Interval, original.Interval)
}

func TestDuration_JSONRoundtrip(t *testing.T) {
	type wrapper struct {
		Interval Duration `json:"interval"`
	}

	original := wrapper{Interval: Duration(30 * time.Minute)}

	data, err := json.Marshal(original)
	assertNoError(t, err)

	var decoded wrapper
	err = json.Unmarshal(data, &decoded)
	assertNoError(t, err)
	assertEqual(t, decoded.Interval, original.Interval)
}
