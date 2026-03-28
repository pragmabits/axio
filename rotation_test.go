package axio

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestRotationConfig_Enabled(t *testing.T) {
	t.Run("empty_config_disabled", func(t *testing.T) {
		rotation := RotationConfig{}
		assertEqual(t, rotation.Enabled(), false)
	})

	t.Run("size_only_enabled", func(t *testing.T) {
		rotation := RotationConfig{MaxSize: 100}
		assertEqual(t, rotation.Enabled(), true)
	})

	t.Run("interval_only_enabled", func(t *testing.T) {
		rotation := RotationConfig{Interval: Duration(24 * time.Hour)}
		assertEqual(t, rotation.Enabled(), true)
	})

	t.Run("both_enabled", func(t *testing.T) {
		rotation := RotationConfig{
			MaxSize:  100,
			Interval: Duration(24 * time.Hour),
		}
		assertEqual(t, rotation.Enabled(), true)
	})

	t.Run("only_maxage_not_enabled", func(t *testing.T) {
		rotation := RotationConfig{MaxAge: 30}
		assertEqual(t, rotation.Enabled(), false)
	})
}

func TestRotatingFile(t *testing.T) {
	t.Run("creates_with_size_rotation", func(t *testing.T) {
		path := tempFile(t, "rotating.log")

		output, err := RotatingFile(path, FormatJSON, RotationConfig{
			MaxSize:    10,
			MaxBackups: 3,
		})
		assertNoError(t, err)
		defer output.Close()

		assertEqual(t, output.Type(), OutputFile)
		assertEqual(t, output.Format(), FormatJSON)
	})

	t.Run("creates_with_time_rotation", func(t *testing.T) {
		path := tempFile(t, "time-rotating.log")

		output, err := RotatingFile(path, FormatJSON, RotationConfig{
			Interval: Duration(time.Hour),
		})
		assertNoError(t, err)
		defer output.Close()

		assertEqual(t, output.Type(), OutputFile)
	})

	t.Run("creates_with_combined_rotation", func(t *testing.T) {
		path := tempFile(t, "combined-rotating.log")

		output, err := RotatingFile(path, FormatJSON, RotationConfig{
			MaxSize:  50,
			Interval: Duration(12 * time.Hour),
			Compress: true,
		})
		assertNoError(t, err)
		defer output.Close()

		assertEqual(t, output.Type(), OutputFile)
	})

	t.Run("writes_to_file", func(t *testing.T) {
		path := tempFile(t, "write-rotating.log")

		output, err := RotatingFile(path, FormatJSON, RotationConfig{
			MaxSize: 10,
		})
		assertNoError(t, err)

		_, err = output.Write([]byte("test log entry\n"))
		assertNoError(t, err)

		output.Close()

		content := readFile(t, path)
		if content != "test log entry\n" {
			t.Errorf("unexpected content: %q", content)
		}
	})

	t.Run("close_stops_ticker", func(t *testing.T) {
		path := tempFile(t, "close-ticker.log")

		output, err := RotatingFile(path, FormatJSON, RotationConfig{
			Interval: Duration(time.Hour),
		})
		assertNoError(t, err)

		// Close should not panic or error
		err = output.Close()
		assertNoError(t, err)
	})
}

func TestMustRotatingFile(t *testing.T) {
	t.Run("valid_path", func(t *testing.T) {
		path := tempFile(t, "must-rotating.log")

		output := MustRotatingFile(path, FormatJSON, RotationConfig{
			MaxSize: 10,
		})
		defer output.Close()

		if output == nil {
			t.Error("output should not be nil")
		}
	})
}

func TestBuildOutputs_WithRotation(t *testing.T) {
	t.Run("builds_rotating_file", func(t *testing.T) {
		path := tempFile(t, "build-rotating.log")
		config := Config{
			Outputs: []OutputConfig{
				{
					Type:   OutputFile,
					Format: FormatJSON,
					Path:   path,
					Rotation: RotationConfig{
						MaxSize:    100,
						MaxBackups: 5,
						Compress:   true,
					},
				},
			},
		}

		outputs, err := BuildOutputs(config)
		assertNoError(t, err)
		defer outputs[0].Close()

		if len(outputs) != 1 {
			t.Fatalf("expected 1 output, got %d", len(outputs))
		}
		assertEqual(t, outputs[0].Type(), OutputFile)
	})

	t.Run("builds_plain_file_without_rotation", func(t *testing.T) {
		path := tempFile(t, "plain.log")
		config := Config{
			Outputs: []OutputConfig{
				{
					Type:   OutputFile,
					Format: FormatJSON,
					Path:   path,
				},
			},
		}

		outputs, err := BuildOutputs(config)
		assertNoError(t, err)
		defer outputs[0].Close()

		if len(outputs) != 1 {
			t.Fatalf("expected 1 output, got %d", len(outputs))
		}
	})
}

func TestRotatingFile_TimeRotation(t *testing.T) {
	t.Run("rotates_on_interval", func(t *testing.T) {
		dir := tempDir(t)
		path := filepath.Join(dir, "timed.log")

		output, err := RotatingFile(path, FormatJSON, RotationConfig{
			Interval: Duration(100 * time.Millisecond),
		})
		assertNoError(t, err)

		// Write initial content
		_, err = output.Write([]byte("before rotation\n"))
		assertNoError(t, err)

		// Wait for at least one rotation tick
		time.Sleep(250 * time.Millisecond)

		// Write after rotation should have happened
		_, err = output.Write([]byte("after rotation\n"))
		assertNoError(t, err)

		output.Close()

		// The current file should contain the post-rotation content
		content := readFile(t, path)
		if content != "after rotation\n" {
			// Rotation may have created a backup; just verify current file is writable
			// and the backup exists
			entries, _ := os.ReadDir(dir)
			if len(entries) < 2 {
				t.Errorf("expected rotated backup file, got %d files", len(entries))
			}
		}
	})
}

func TestRotationConfig_YAMLUnmarshal(t *testing.T) {
	yamlContent := `
serviceName: "rotation-test"
serviceVersion: "1.0.0"
environment: "production"
level: "info"
outputs:
  - type: file
    format: json
    path: /var/log/app.log
    rotation:
      maxSize: 100
      maxAge: 30
      maxBackups: 10
      compress: true
      interval: 24h
`
	config, err := LoadConfigFrom(
		stringReader(yamlContent),
		"yaml",
	)
	assertNoError(t, err)

	if len(config.Outputs) != 1 {
		t.Fatalf("expected 1 output, got %d", len(config.Outputs))
	}

	rotation := config.Outputs[0].Rotation
	assertEqual(t, rotation.MaxSize, 100)
	assertEqual(t, rotation.MaxAge, 30)
	assertEqual(t, rotation.MaxBackups, 10)
	assertEqual(t, rotation.Compress, true)
	assertEqual(t, rotation.Interval, Duration(24*time.Hour))
	assertEqual(t, rotation.Enabled(), true)
}
