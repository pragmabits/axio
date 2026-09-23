package axio

import (
	"testing"
)

func TestEnvironment_Validate(t *testing.T) {
	valid := []Environment{EnvironmentProduction, EnvironmentStaging, EnvironmentDevelopment}
	for _, env := range valid {
		if err := env.Validate(); err != nil {
			t.Errorf("environment %s should be valid", env)
		}
	}

	invalid := Environment("invalid")
	if err := invalid.Validate(); err == nil {
		t.Error("invalid environment should return error")
	}
}

func TestEnvironment_UnmarshalText(t *testing.T) {
	t.Run("valid_environments", func(t *testing.T) {
		tests := []struct {
			input string
			want  Environment
		}{
			{"production", EnvironmentProduction},
			{"staging", EnvironmentStaging},
			{"development", EnvironmentDevelopment},
			{" production ", EnvironmentProduction},
		}

		for _, test := range tests {
			var environment Environment
			err := environment.UnmarshalText([]byte(test.input))
			assertNoError(t, err)
			assertEqual(t, environment, test.want)
		}
	})

	t.Run("invalid_environment", func(t *testing.T) {
		var environment Environment
		err := environment.UnmarshalText([]byte("invalid"))
		assertError(t, err)
	})
}

func TestLevel_Validate(t *testing.T) {
	valid := []Level{LevelDebug, LevelInfo, LevelWarn, LevelError}
	for _, level := range valid {
		if err := level.Validate(); err != nil {
			t.Errorf("level %s should be valid", level)
		}
	}

	invalid := Level("invalid")
	if err := invalid.Validate(); err == nil {
		t.Error("invalid level should return error")
	}
}

func TestLevel_UnmarshalText(t *testing.T) {
	t.Run("valid_levels", func(t *testing.T) {
		tests := []struct {
			input string
			want  Level
		}{
			{"debug", LevelDebug},
			{"info", LevelInfo},
			{"warn", LevelWarn},
			{"error", LevelError},
			{" info ", LevelInfo},
		}

		for _, test := range tests {
			var level Level
			err := level.UnmarshalText([]byte(test.input))
			assertNoError(t, err)
			assertEqual(t, level, test.want)
		}
	})

	t.Run("invalid_level", func(t *testing.T) {
		var level Level
		err := level.UnmarshalText([]byte("invalid"))
		assertError(t, err)
	})
}

func TestFormat_Validate(t *testing.T) {
	valid := []Format{FormatJSON, FormatText}
	for _, format := range valid {
		if err := format.Validate(); err != nil {
			t.Errorf("format %s should be valid", format)
		}
	}

	invalid := Format("invalid")
	if err := invalid.Validate(); err == nil {
		t.Error("invalid format should return error")
	}
}

func TestFormat_UnmarshalText(t *testing.T) {
	t.Run("valid_formats", func(t *testing.T) {
		tests := []struct {
			input string
			want  Format
		}{
			{"json", FormatJSON},
			{"text", FormatText},
			{" json ", FormatJSON},
		}

		for _, test := range tests {
			var format Format
			err := format.UnmarshalText([]byte(test.input))
			assertNoError(t, err)
			assertEqual(t, format, test.want)
		}
	})

	t.Run("invalid_format", func(t *testing.T) {
		var format Format
		err := format.UnmarshalText([]byte("invalid"))
		assertError(t, err)
	})
}
