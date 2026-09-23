package axio

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
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
			want:     "CNPJ: **.***.***/****-**",
		},
		{
			name:     "cnpj_without_punctuation",
			patterns: []PIIPattern{PatternCNPJ},
			input:    "CNPJ: 12345678000190",
			want:     "CNPJ: **.***.***/****-**",
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

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			masker, err := NewPIIMasker(PIIConfig{Patterns: test.patterns})
			assertNoError(t, err)

			got := masker.MaskString(test.input)
			assertEqual(t, got, test.want)
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
			Annotate("document", "123.456.789-01"),
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
			Annotate("user_password", "secret123"),
			Annotate("api_token", "abc123"),
			Annotate("username", "john"),
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

	t.Run("name_match_ignores_case", func(t *testing.T) {
		masker, _ := NewPIIMasker(PIIConfig{Fields: []string{"password"}})
		annotations := Annotations{Annotate("USER_PASSWORD", "x")}
		masker.MaskFields(annotations)
		assertEqual(t, annotations[0].Data().(string), "[REDACTED]")
	})
}

func TestPIIMasker_MaskFields_MapRecursion(t *testing.T) {
	masker, _ := NewPIIMasker(PIIConfig{
		Patterns: []PIIPattern{PatternCPF},
		Fields:   []string{"password"},
	})

	annotations := Annotations{
		Annotate("context", map[string]any{
			"user":     "alice",
			"password": "hunter2",
			"document": "123.456.789-01",
		}),
	}

	masker.MaskFields(annotations)

	masked, ok := annotations[0].Data().(map[string]any)
	if !ok {
		t.Fatalf("expected map[string]any after masking, got %T", annotations[0].Data())
	}

	assertEqual(t, masked["user"].(string), "alice")
	assertEqual(t, masked["password"].(string), "[REDACTED]")
	assertEqual(t, masked["document"].(string), "***.***.***-**")
}

func TestPIIMasker_MaskFields_MapRecursion_DepthCap(t *testing.T) {
	build := func() Annotations {
		// payload nests "password" at depth 3 from the annotation root:
		// annotation -> level1 -> level2 -> level3 -> password
		level3 := map[string]any{"password": "p3"}
		level2 := map[string]any{"deep": level3}
		level1 := map[string]any{"nested": level2}
		return Annotations{Annotate("root", level1)}
	}

	t.Run("beyond_max_depth_is_redacted", func(t *testing.T) {
		masker, _ := NewPIIMasker(PIIConfig{
			Fields:   []string{"password"},
			MaxDepth: 2,
		})
		annotations := build()
		masker.MaskFields(annotations)

		root := annotations[0].Data().(map[string]any)
		nested := root["nested"].(map[string]any)
		if nested["deep"] != "[REDACTED]" {
			t.Errorf("expected the map at depth 3 redacted with MaxDepth=2, got %v", nested["deep"])
		}
	})

	t.Run("default_max_depth_reaches_depth_3", func(t *testing.T) {
		masker, _ := NewPIIMasker(PIIConfig{
			Fields: []string{"password"},
		})
		annotations := build()
		masker.MaskFields(annotations)

		root := annotations[0].Data().(map[string]any)
		nested := root["nested"].(map[string]any)
		deep := nested["deep"].(map[string]any)
		if deep["password"] != "[REDACTED]" {
			t.Errorf("expected masked at depth 3 with the default MaxDepth, got %v", deep["password"])
		}
	})

	t.Run("max_depth_3_masks_depth_3", func(t *testing.T) {
		masker, _ := NewPIIMasker(PIIConfig{
			Fields:   []string{"password"},
			MaxDepth: 3,
		})
		annotations := build()
		masker.MaskFields(annotations)

		root := annotations[0].Data().(map[string]any)
		nested := root["nested"].(map[string]any)
		deep := nested["deep"].(map[string]any)
		if deep["password"] != "[REDACTED]" {
			t.Errorf("expected masked at depth 3 with MaxDepth=3, got %v", deep["password"])
		}
	})
}

func TestPIIMasker_MaskFields_MapRecursion_NoAliasing(t *testing.T) {
	masker, _ := NewPIIMasker(PIIConfig{
		Fields: []string{"password"},
	})

	original := map[string]any{
		"user":     "alice",
		"password": "hunter2",
	}
	annotations := Annotations{Annotate("context", original)}

	masker.MaskFields(annotations)

	// The original caller-side map must be untouched by masking.
	if original["password"] != "hunter2" {
		t.Errorf("expected caller map unchanged, got password=%v", original["password"])
	}

	masked := annotations[0].Data().(map[string]any)
	if masked["password"] != "[REDACTED]" {
		t.Errorf("expected masked output, got password=%v", masked["password"])
	}
}

func TestPIIMasker_MaskFields_MapRecursion_MixedValues(t *testing.T) {
	masker, _ := NewPIIMasker(PIIConfig{
		Patterns: []PIIPattern{PatternCPF},
		Fields:   []string{"password"},
	})

	annotations := Annotations{
		Annotate("payload", map[string]any{
			"name":     "alice",
			"age":      30,
			"active":   true,
			"document": "123.456.789-01",
			"password": "hunter2",
			"profile": map[string]any{
				"phone":    "11999998888",
				"password": "nested-secret",
			},
		}),
	}

	masker.MaskFields(annotations)

	payload := annotations[0].Data().(map[string]any)
	assertEqual(t, payload["name"].(string), "alice")
	assertEqual(t, payload["age"].(int), 30)
	assertEqual(t, payload["active"].(bool), true)
	assertEqual(t, payload["document"].(string), "***.***.***-**")
	assertEqual(t, payload["password"].(string), "[REDACTED]")

	profile := payload["profile"].(map[string]any)
	assertEqual(t, profile["password"].(string), "[REDACTED]")
}

// piiCustomer is a struct annotation carrying a CPF, a sensitive field and a number.
type piiCustomer struct {
	Name     string `json:"name"`
	Document string `json:"document"`
	Password string `json:"password"`
	Age      int    `json:"age"`
}

// piiDocument is a fmt.Stringer whose text carries a CPF.
type piiDocument struct {
	number string
}

func (p piiDocument) String() string { return "doc " + p.number }

// piiRequest is a struct annotation whose byte field encoding/json writes as base64.
type piiRequest struct {
	Body []byte `json:"body"`
}

// piiToken returns a JWT with a fixed header and signature and claims as its payload.
func piiToken(claims string) string {
	encode := base64.RawURLEncoding.EncodeToString
	return encode([]byte(`{"alg":"HS256","typ":"JWT"}`)) + "." + encode([]byte(claims)) + "." + encode([]byte("signature"))
}

// piiOrder is a struct annotation with nothing to mask.
type piiOrder struct {
	ID string `json:"id"`
}

func TestPIIMasker_MaskFields_EveryValue(t *testing.T) {
	customer := piiCustomer{Name: "alice", Document: "123.456.789-01", Password: "hunter2", Age: 30}
	const maskedCustomer = `{"age":30,"document":"***.***.***-**","name":"alice","password":"[REDACTED]"}`
	payload := []byte(`{"cpf":"123.456.789-01"}`)
	maskedPayload := `"` + base64.StdEncoding.EncodeToString([]byte(`{"cpf":"***.***.***-**"}`)) + `"`
	binary := []byte{0xff, 0xfe, 0x00, 0x01}
	token := piiToken(`{"sub":"42","cpf":"123.456.789-01","password":"x"}`)
	maskedToken := piiToken(`{"cpf":"***.***.***-**","password":"[REDACTED]","sub":"42"}`)
	unpadded := []byte(`{"cpf":"123.456.789-01","ok":1}`)
	urlSafe := []byte(`{"cpf":"123.456.789-01","note":"~~~"}`)

	tests := []struct {
		name  string
		value any
		want  string
	}{
		{
			name:  "struct",
			value: customer,
			want:  maskedCustomer,
		},
		{
			name:  "pointer_to_struct",
			value: &customer,
			want:  maskedCustomer,
		},
		{
			name:  "struct_inside_map",
			value: map[string]any{"customer": customer},
			want:  `{"customer":` + maskedCustomer + `}`,
		},
		{
			name:  "string_slice",
			value: []string{"123.456.789-01", "ok"},
			want:  `["***.***.***-**","ok"]`,
		},
		{
			name:  "string_map",
			value: map[string]string{"password": "hunter2", "document": "123.456.789-01"},
			want:  `{"document":"***.***.***-**","password":"[REDACTED]"}`,
		},
		{
			name:  "http_header",
			value: http.Header{"Authorization": {"Bearer abc"}, "Accept": {"text/plain"}},
			want:  `{"Accept":["text/plain"],"Authorization":"[REDACTED]"}`,
		},
		{
			name:  "stringer",
			value: piiDocument{number: "123.456.789-01"},
			want:  `"doc ***.***.***-**"`,
		},
		{
			name:  "text_bytes",
			value: payload,
			want:  maskedPayload,
		},
		{
			name:  "text_bytes_without_pii",
			value: []byte("hello"),
			want:  `"aGVsbG8="`,
		},
		{
			name:  "binary_bytes",
			value: binary,
			want:  `"[REDACTED]"`,
		},
		{
			name:  "bytes_inside_map",
			value: map[string]any{"body": payload, "blob": binary},
			want:  `{"blob":"[REDACTED]","body":` + maskedPayload + `}`,
		},
		{
			name:  "bytes_inside_struct",
			value: piiRequest{Body: payload},
			want:  `{"body":` + maskedPayload + `}`,
		},
		{
			name:  "base64_string",
			value: base64.StdEncoding.EncodeToString(payload),
			want:  maskedPayload,
		},
		{
			name:  "base64_string_inside_map",
			value: map[string]any{"body": base64.StdEncoding.EncodeToString(payload)},
			want:  `{"body":` + maskedPayload + `}`,
		},
		{
			name:  "unpadded_base64_string",
			value: base64.RawStdEncoding.EncodeToString(unpadded),
			want:  `"` + base64.RawStdEncoding.EncodeToString([]byte(`{"cpf":"***.***.***-**","ok":1}`)) + `"`,
		},
		{
			name:  "url_base64_string",
			value: base64.URLEncoding.EncodeToString(urlSafe),
			want:  `"` + base64.URLEncoding.EncodeToString([]byte(`{"cpf":"***.***.***-**","note":"~~~"}`)) + `"`,
		},
		{
			name:  "jwt",
			value: token,
			want:  `"` + maskedToken + `"`,
		},
		{
			name:  "jwt_inside_text",
			value: "Bearer " + token + " expired",
			want:  `"Bearer ` + maskedToken + ` expired"`,
		},
	}

	masker := MustPIIMasker(PIIConfig{
		Patterns: []PIIPattern{PatternCPF},
		Fields:   []string{"password", "authorization"},
	})
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			annotations := Annotations{Annotate("value", test.value)}
			masker.MaskFields(annotations)

			encoded, err := json.Marshal(annotations[0].Data())
			assertNoError(t, err)
			assertEqual(t, string(encoded), test.want)
		})
	}

	t.Run("error", func(t *testing.T) {
		original := fmt.Errorf("lookup 123.456.789-01 failed")
		annotations := Annotations{Annotate("cause", original)}
		masker.MaskFields(annotations)

		masked, ok := annotations[0].Data().(error)
		if !ok {
			t.Fatalf("expected an error after masking, got %T", annotations[0].Data())
		}
		assertEqual(t, masked.Error(), "lookup ***.***.***-** failed")
		if !errors.Is(masked, original) {
			t.Error("the masked error should still match the original with errors.Is")
		}
	})

	t.Run("value_without_pii_keeps_its_type", func(t *testing.T) {
		annotations := Annotations{Annotate("order", piiOrder{ID: "ord_8812"})}
		masker.MaskFields(annotations)

		if _, ok := annotations[0].Data().(piiOrder); !ok {
			t.Errorf("expected the original struct, got %T", annotations[0].Data())
		}
	})

	t.Run("struct_beyond_max_depth_is_redacted", func(t *testing.T) {
		shallow := MustPIIMasker(PIIConfig{Fields: []string{"password"}, MaxDepth: 1})
		annotations := Annotations{Annotate("value", map[string]any{"customer": customer})}
		shallow.MaskFields(annotations)

		encoded, err := json.Marshal(annotations[0].Data())
		assertNoError(t, err)
		assertEqual(t, string(encoded), `{"customer":"[REDACTED]"}`)
	})
}

func TestBase64EncodingOf(t *testing.T) {
	encodings := map[string]*base64.Encoding{
		"standard":     base64.StdEncoding,
		"raw_standard": base64.RawStdEncoding,
		"url":          base64.URLEncoding,
		"raw_url":      base64.RawURLEncoding,
	}
	for name, encoding := range encodings {
		t.Run(name+"_decodes_back", func(t *testing.T) {
			for length := 1; length <= 64; length++ {
				data := make([]byte, length)
				for index := range data {
					data[index] = byte(index * 37)
				}
				encoded := encoding.EncodeToString(data)
				recognized := base64EncodingOf(encoded)
				if recognized == nil {
					t.Fatalf("rejected %q, %d bytes", encoded, length)
				}
				decoded, err := recognized.DecodeString(encoded)
				if err != nil || string(decoded) != string(data) {
					t.Errorf("%q decoded to %v, %v", encoded, decoded, err)
				}
			}
		})
	}

	t.Run("other_shapes_are_rejected", func(t *testing.T) {
		for _, text := range []string{"", "a", "abcde", "ab c", "a===", "ab=c", "ab+-", "usr_12345", "123.456.789-01"} {
			if base64EncodingOf(text) != nil {
				t.Errorf("accepted %q", text)
			}
		}
	})
}

func TestPIIMasker_MaskFieldsWithCounts_CountsInsideValues(t *testing.T) {
	masker := MustPIIMasker(PIIConfig{Patterns: []PIIPattern{PatternCPF}})
	annotations := Annotations{
		Annotate("customer", piiCustomer{Document: "123.456.789-01"}),
		Annotate("documents", []string{"987.654.321-00", "111.222.333-44"}),
		Annotate("body", []byte("222.333.444-55")),
		Annotate("request", piiRequest{Body: []byte("333.444.555-66")}),
		Annotate("encoded", base64.StdEncoding.EncodeToString([]byte("444.555.666-77"))),
		Annotate("session", piiToken(`{"cpf":"555.666.777-88"}`)),
	}

	matches := masker.MaskFieldsWithCounts(annotations)

	assertEqual(t, matches[PatternCPF], 7)
}

func TestPIIMasker_NegativeMaxDepth(t *testing.T) {
	if _, err := NewPIIMasker(PIIConfig{MaxDepth: -1}); !errors.Is(err, ErrInvalidPIIMaxDepth) {
		t.Errorf("expected ErrInvalidPIIMaxDepth, got %v", err)
	}
}

func TestPIIConfig_MaxDepth_ZeroDefaults(t *testing.T) {
	masker, err := NewPIIMasker(PIIConfig{})
	if err != nil {
		t.Fatalf("NewPIIMasker failed: %v", err)
	}
	if masker.maxDepth != DefaultPIIMaxDepth {
		t.Errorf("expected maxDepth=%d when MaxDepth is zero, got %d",
			DefaultPIIMaxDepth, masker.maxDepth)
	}
	assertEqual(t, DefaultPIIMaxDepth, 32)
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

	hasPattern := func(wanted PIIPattern) bool {
		for _, pattern := range config.Patterns {
			if pattern == wanted {
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
			if recovered := recover(); recovered == nil {
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
				Annotate("email", "user@test.com"),
			},
		}

		err = hook.Process(context.Background(), entry)
		assertNoError(t, err)
		assertEqual(t, entry.Annotations[0].Data().(string), "***@***.***")
	})

	t.Run("masks_error", func(t *testing.T) {
		hook, err := NewPIIHook(PIIConfig{
			Patterns: []PIIPattern{PatternCPF},
		})
		assertNoError(t, err)

		original := errors.New("customer 123.456.789-01 rejected")
		entry := &Entry{Message: "log", Error: original}

		err = hook.Process(context.Background(), entry)
		assertNoError(t, err)
		assertEqual(t, entry.Error.Error(), "customer ***.***.***-** rejected")
		if !errors.Is(entry.Error, original) {
			t.Error("the masked error should still match the original with errors.Is")
		}
	})

	t.Run("masks_base64_text_in_message_and_error", func(t *testing.T) {
		hook, err := NewPIIHook(PIIConfig{
			Patterns: []PIIPattern{PatternCPF},
		})
		assertNoError(t, err)

		encode := func(text string) string { return base64.StdEncoding.EncodeToString([]byte(text)) }
		entry := &Entry{
			Message: encode("customer 123.456.789-01"),
			Error:   errors.New(encode("customer 987.654.321-00")),
		}

		err = hook.Process(context.Background(), entry)
		assertNoError(t, err)
		assertEqual(t, entry.Message, encode("customer ***.***.***-**"))
		assertEqual(t, entry.Error.Error(), encode("customer ***.***.***-**"))
	})

	t.Run("masks_jwt_in_message", func(t *testing.T) {
		hook, err := NewPIIHook(PIIConfig{
			Patterns: []PIIPattern{PatternCPF},
		})
		assertNoError(t, err)

		entry := &Entry{Message: "login with " + piiToken(`{"cpf":"123.456.789-01"}`)}

		err = hook.Process(context.Background(), entry)
		assertNoError(t, err)
		assertEqual(t, entry.Message, "login with "+piiToken(`{"cpf":"***.***.***-**"}`))
	})

	t.Run("hook_name", func(t *testing.T) {
		hook, _ := NewPIIHook(DefaultPIIConfig())
		assertEqual(t, hook.Name(), "pii")
	})
}

func TestPIIHook_MasksEveryValueTheLogWrites(t *testing.T) {
	const cpf = "123.456.789-01"
	customer := piiCustomer{Name: "alice", Document: cpf, Password: "hunter2"}
	payload := []byte(`{"cpf":"` + cpf + `"}`)

	logged := newBufferOutput(FormatJSON)
	logger, err := New(minimalConfig(), WithOutputs(logged), WithPII(nil, nil))
	assertNoError(t, err)
	logger.With(
		Annotate("http", HTTP{Method: "GET", URL: "/customers?cpf=" + cpf, StatusCode: 200}),
		Annotate("customer", customer),
		Annotate("cause", fmt.Errorf("lookup %s failed", cpf)),
		Annotate("body", payload),
		Annotate("request", piiRequest{Body: payload}),
	).Error(context.Background(), fmt.Errorf("customer %s rejected", cpf), "registration failed")
	assertNoError(t, logger.Close())

	emitted := newBufferOutput(FormatJSON)
	event, err := NewEvent("registration", minimalConfig(), WithOutputs(emitted), WithPII(nil, nil))
	assertNoError(t, err)
	event.Add("customer", customer)
	event.Add("body", payload)
	event.SetError(fmt.Errorf("customer %s rejected", cpf))
	event.Emit(context.Background())
	assertNoError(t, event.Close())

	for name, line := range map[string]string{"logger": logged.String(), "event": emitted.String()} {
		for _, secret := range []string{cpf, "hunter2", base64.StdEncoding.EncodeToString(payload)} {
			if strings.Contains(line, secret) {
				t.Errorf("%s: %q leaked:\n%s", name, secret, line)
			}
		}
		if !strings.Contains(line, "***.***.***-**") {
			t.Errorf("%s: expected the masked CPF:\n%s", name, line)
		}
	}
}

// TestPIIHook_DoesNotMutateCallerAnnotations verifies that masking works on the
// logger's own copy: the slice the caller passed to With keeps its values, and
// every call through the scoped logger is masked again.
func TestPIIHook_DoesNotMutateCallerAnnotations(t *testing.T) {
	output := newBufferOutput(FormatJSON)
	logger, err := New(minimalConfig(), WithOutputs(output), WithPII([]PIIPattern{PatternCPF}, DefaultSensitiveFields()))
	assertNoError(t, err)
	defer logger.Close()

	annotations := []Annotation{Annotate("doc", "123.456.789-01"), Annotate("password", "hunter2")}
	scoped := logger.With(annotations...)
	scoped.Info(context.Background(), "first")
	scoped.Info(context.Background(), "second")

	assertEqual(t, annotations[0].Data().(string), "123.456.789-01")
	assertEqual(t, annotations[1].Data().(string), "hunter2")
	assertEqual(t, strings.Count(output.String(), `"doc":"***.***.***-**"`), 2)
	assertEqual(t, strings.Count(output.String(), `"password":"[REDACTED]"`), 2)
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
			if recovered := recover(); recovered == nil {
				t.Error("expected panic with invalid regex")
			}
		}()

		MustPIIHook(PIIConfig{
			CustomPatterns: []CustomPII{{Pattern: "[invalid"}},
		})
	})
}
