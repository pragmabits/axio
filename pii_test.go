package axio

import (
	"context"
	"testing"
)

func TestPIIMasker_MaskString(t *testing.T) {
	tests := []struct {
		name     string
		patterns []PIIPattern
		input    string
		want     string
	}{
		{
			name:     "cpf_with_punctuation",
			patterns: []PIIPattern{PatternCPF},
			input:    "CPF: 123.456.789-01",
			want:     "CPF: ***.***.***-**",
		},
		{
			name:     "cpf_without_punctuation",
			patterns: []PIIPattern{PatternCPF},
			input:    "CPF: 12345678901",
			want:     "CPF: ***.***.***-**",
		},
		{
			name:     "cnpj_with_punctuation",
			patterns: []PIIPattern{PatternCNPJ},
			input:    "CNPJ: 12.345.678/0001-90",
			want:     "CNPJ: **.***.***/**01-**",
		},
		{
			name:     "cnpj_without_punctuation",
			patterns: []PIIPattern{PatternCNPJ},
			input:    "CNPJ: 12345678000190",
			want:     "CNPJ: **.***.***/**01-**",
		},
		{
			name:     "credit_card_with_dashes",
			patterns: []PIIPattern{PatternCreditCard},
			input:    "Card: 1234-5678-9012-3456",
			want:     "Card: ****-****-****-****",
		},
		{
			name:     "credit_card_with_spaces",
			patterns: []PIIPattern{PatternCreditCard},
			input:    "Card: 1234 5678 9012 3456",
			want:     "Card: ****-****-****-****",
		},
		{
			name:     "credit_card_no_separators",
			patterns: []PIIPattern{PatternCreditCard},
			input:    "Card: 1234567890123456",
			want:     "Card: ****-****-****-****",
		},
		{
			name:     "email",
			patterns: []PIIPattern{PatternEmail},
			input:    "Email: user@example.com.br",
			want:     "Email: ***@***.***",
		},
		{
			name:     "phone_with_ddd",
			patterns: []PIIPattern{PatternPhone},
			input:    "Tel: (11) 99999-8888",
			want:     "Tel: (**) *****-****",
		},
		{
			name:     "phone_no_ddd",
			patterns: []PIIPattern{PatternPhoneNoDDD},
			input:    "Tel: 99999-8888",
			want:     "Tel: *****-****",
		},
		{
			name:     "no_pii_unchanged",
			patterns: []PIIPattern{PatternCPF, PatternCNPJ, PatternEmail},
			input:    "Message without sensitive data",
			want:     "Message without sensitive data",
		},
		{
			name:     "multiple_pii_same_string",
			patterns: []PIIPattern{PatternCPF, PatternEmail},
			input:    "Customer CPF 123.456.789-01 email user@test.com",
			want:     "Customer CPF ***.***.***-** email ***@***.***",
		},
		{
			name:     "empty_string",
			patterns: []PIIPattern{PatternCPF},
			input:    "",
			want:     "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			masker, err := NewPIIMasker(PIIConfig{Patterns: tt.patterns})
			assertNoError(t, err)

			got := masker.MaskString(tt.input)
			assertEqual(t, got, tt.want)
		})
	}
}

func TestPIIMasker_CustomPattern(t *testing.T) {
	config := PIIConfig{
		CustomPatterns: []CustomPII{
			{
				Name:    "registration",
				Pattern: `MAT-\d{6}`,
				Mask:    "MAT-******",
			},
		},
	}

	masker, err := NewPIIMasker(config)
	assertNoError(t, err)

	got := masker.MaskString("Registration: MAT-123456")
	want := "Registration: MAT-******"
	assertEqual(t, got, want)
}

func TestPIIMasker_InvalidCustomPattern(t *testing.T) {
	config := PIIConfig{
		CustomPatterns: []CustomPII{
			{
				Name:    "invalid",
				Pattern: `[invalid`, // invalid regex
				Mask:    "***",
			},
		},
	}

	_, err := NewPIIMasker(config)
	assertError(t, err)
}

func TestPIIMasker_MaskFields(t *testing.T) {
	t.Run("masks_string_values", func(t *testing.T) {
		masker, _ := NewPIIMasker(PIIConfig{
			Patterns: []PIIPattern{PatternCPF},
		})

		annotations := Annotations{
			&Note{Key: "document", Value: "123.456.789-01"},
		}

		masker.MaskFields(annotations)

		got := annotations[0].Data().(string)
		want := "***.***.***-**"
		assertEqual(t, got, want)
	})

	t.Run("redacts_sensitive_fields", func(t *testing.T) {
		masker, _ := NewPIIMasker(PIIConfig{
			Fields: []string{"password", "token"},
		})

		annotations := Annotations{
			&Note{Key: "user_password", Value: "secret123"},
			&Note{Key: "api_token", Value: "abc123"},
			&Note{Key: "username", Value: "john"},
		}

		masker.MaskFields(annotations)

		assertEqual(t, annotations[0].Data().(string), "[REDACTED]")
		assertEqual(t, annotations[1].Data().(string), "[REDACTED]")
		assertEqual(t, annotations[2].Data().(string), "john")
	})

	t.Run("handles_nil_annotations", func(t *testing.T) {
		masker, _ := NewPIIMasker(PIIConfig{})
		masker.MaskFields(nil) // should not panic
	})
}

func TestPIIMasker_MaskStringWithCounts(t *testing.T) {
	masker, _ := NewPIIMasker(PIIConfig{
		Patterns: []PIIPattern{PatternCPF, PatternEmail},
	})

	input := "CPF 123.456.789-01 and 987.654.321-00, email user@test.com"
	result := masker.MaskStringWithCounts(input)

	if result.Matches[PatternCPF] != 2 {
		t.Errorf("expected 2 CPFs, got %d", result.Matches[PatternCPF])
	}

	if result.Matches[PatternEmail] != 1 {
		t.Errorf("expected 1 email, got %d", result.Matches[PatternEmail])
	}

	if result.Masked == input {
		t.Error("string should have been masked")
	}
}

func TestDefaultPIIConfig(t *testing.T) {
	config := DefaultPIIConfig()

	if len(config.Patterns) != 3 {
		t.Errorf("expected 3 patterns, got %d", len(config.Patterns))
	}

	hasPattern := func(p PIIPattern) bool {
		for _, pattern := range config.Patterns {
			if pattern == p {
				return true
			}
		}
		return false
	}

	if !hasPattern(PatternCPF) {
		t.Error("expected PatternCPF in DefaultPIIConfig")
	}
	if !hasPattern(PatternCNPJ) {
		t.Error("expected PatternCNPJ in DefaultPIIConfig")
	}
	if !hasPattern(PatternCreditCard) {
		t.Error("expected PatternCreditCard in DefaultPIIConfig")
	}

	if len(config.Fields) == 0 {
		t.Error("expected sensitive fields in DefaultPIIConfig")
	}
}

func TestMustPIIMasker(t *testing.T) {
	t.Run("valid_config_returns_masker", func(t *testing.T) {
		masker := MustPIIMasker(DefaultPIIConfig())
		if masker == nil {
			t.Error("expected non-nil masker")
		}
	})

	t.Run("invalid_config_panics", func(t *testing.T) {
		defer func() {
			if r := recover(); r == nil {
				t.Error("expected panic with invalid regex")
			}
		}()

		MustPIIMasker(PIIConfig{
			CustomPatterns: []CustomPII{{Pattern: "[invalid"}},
		})
	})
}

func TestPIIHook(t *testing.T) {
	t.Run("masks_message", func(t *testing.T) {
		hook, err := NewPIIHook(PIIConfig{
			Patterns: []PIIPattern{PatternCPF},
		})
		assertNoError(t, err)

		entry := &Entry{
			Message: "Customer CPF 123.456.789-01",
		}

		err = hook.Process(context.Background(), entry)
		assertNoError(t, err)
		assertEqual(t, entry.Message, "Customer CPF ***.***.***-**")
	})

	t.Run("masks_annotations", func(t *testing.T) {
		hook, err := NewPIIHook(PIIConfig{
			Patterns: []PIIPattern{PatternEmail},
		})
		assertNoError(t, err)

		entry := &Entry{
			Message: "log",
			Annotations: Annotations{
				&Note{Key: "email", Value: "user@test.com"},
			},
		}

		err = hook.Process(context.Background(), entry)
		assertNoError(t, err)
		assertEqual(t, entry.Annotations[0].Data().(string), "***@***.***")
	})

	t.Run("hook_name", func(t *testing.T) {
		hook, _ := NewPIIHook(DefaultPIIConfig())
		assertEqual(t, hook.Name(), "pii")
	})
}

func TestMustPIIHook(t *testing.T) {
	t.Run("valid_config_returns_hook", func(t *testing.T) {
		hook := MustPIIHook(DefaultPIIConfig())
		if hook == nil {
			t.Error("expected non-nil hook")
		}
	})

	t.Run("invalid_config_panics", func(t *testing.T) {
		defer func() {
			if r := recover(); r == nil {
				t.Error("expected panic with invalid regex")
			}
		}()

		MustPIIHook(PIIConfig{
			CustomPatterns: []CustomPII{{Pattern: "[invalid"}},
		})
	})
}
