package axio

import (
	"compress/gzip"
	"errors"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestRotator_Write(t *testing.T) {
	moment := time.Date(2026, 11, 7, 4, 18, 18, 123_000_000, time.UTC)

	t.Run("appends_to_existing_file", func(t *testing.T) {
		path := tempFile(t, "app.log")
		writeFile(t, path, "old\n")
		writer := openTestRotator(t, path, RotationConfig{MaxSize: 10})
		writeLine(t, writer, "new\n")
		releaseTestRotator(t, writer)

		assertEqual(t, readFile(t, path), "old\nnew\n")
	})

	t.Run("rotates_when_size_is_exceeded", func(t *testing.T) {
		path := tempFile(t, "app.log")
		writer := openTestRotator(t, path, RotationConfig{MaxSize: 10})
		writer.maxBytes = 10
		writeLine(t, writer, "first\n")
		writeLine(t, writer, "second\n")
		releaseTestRotator(t, writer)

		assertEqual(t, readFile(t, path), "second\n")
		assertEqual(t, len(logFiles(t, filepath.Dir(path))), 2)
	})

	t.Run("backup_name_keeps_lumberjack_format", func(t *testing.T) {
		path := tempFile(t, "app.log")
		writer := openTestRotator(t, path, RotationConfig{MaxSize: 10})
		writer.maxBytes = 10
		writer.now = func() time.Time { return moment }
		writeLine(t, writer, "first\n")
		writeLine(t, writer, "second\n")
		releaseTestRotator(t, writer)

		assertEqual(t, readFile(t, filepath.Join(filepath.Dir(path), "app-2026-11-07T04-18-18.123.log")), "first\n")
	})

	t.Run("backup_in_the_same_millisecond_is_not_overwritten", func(t *testing.T) {
		path := tempFile(t, "app.log")
		writer := openTestRotator(t, path, RotationConfig{MaxSize: 10})
		writer.maxBytes = 10
		writer.now = func() time.Time { return moment }
		for _, line := range []string{"first\n", "second\n", "third\n", "fourth\n"} {
			writeLine(t, writer, line)
		}
		releaseTestRotator(t, writer)

		directory := filepath.Dir(path)
		assertEqual(t, readFile(t, filepath.Join(directory, "app-2026-11-07T04-18-18.123.log")), "first\n")
		assertEqual(t, readFile(t, filepath.Join(directory, "app-2026-11-07T04-18-18.124.log")), "second\n")
		assertEqual(t, readFile(t, filepath.Join(directory, "app-2026-11-07T04-18-18.125.log")), "third\n")
		assertEqual(t, readFile(t, path), "fourth\n")
	})

	t.Run("oversized_write_is_written_whole", func(t *testing.T) {
		path := tempFile(t, "app.log")
		writer := openTestRotator(t, path, RotationConfig{MaxSize: 10})
		writer.maxBytes = 10
		oversized := strings.Repeat("x", 24) + "\n"
		writeLine(t, writer, oversized)
		releaseTestRotator(t, writer)

		assertEqual(t, readFile(t, path), oversized)
		assertEqual(t, len(logFiles(t, filepath.Dir(path))), 1)
	})

	t.Run("new_file_keeps_previous_mode", func(t *testing.T) {
		if runtime.GOOS == "windows" {
			t.Skip("Windows has no Unix permission bits")
		}
		path := tempFile(t, "app.log")
		writeFile(t, path, "")
		if err := os.Chmod(path, 0o640); err != nil {
			t.Fatalf("chmod %s: %v", path, err)
		}
		writer := openTestRotator(t, path, RotationConfig{MaxSize: 10})
		writer.maxBytes = 10
		writeLine(t, writer, "first\n")
		writeLine(t, writer, "second\n")
		releaseTestRotator(t, writer)

		assertEqual(t, readFile(t, path), "second\n")
		info, err := os.Stat(path)
		assertNoError(t, err)
		assertEqual(t, info.Mode().Perm(), os.FileMode(0o640))
	})

	t.Run("failed_rotation_keeps_the_line", func(t *testing.T) {
		path, writer := readOnlyDirectoryRotator(t)
		stderr := captureStderr(t)
		writeLine(t, writer, "first\n")
		writeLine(t, writer, "second\n")
		reported := stderr()

		assertEqual(t, readFile(t, path), "first\nsecond\n")
		if !strings.Contains(reported, "axio: rotation error") {
			t.Errorf("expected the failed rotation on stderr, got %q", reported)
		}
		if writer.lastRotationError() == nil {
			t.Error("expected the failed rotation in lastRotationError")
		}
	})

	t.Run("log_moved_aside_counts_when_the_new_one_fails_to_open", func(t *testing.T) {
		path := tempFile(t, "app.log")
		writer := openTestRotator(t, path, RotationConfig{MaxSize: 10, MaxBackups: 1})
		writer.maxBytes = 10
		clock := steppedClock(writer)
		failures := 1
		writer.openFile = func(name string, flag int, mode os.FileMode) (*os.File, error) {
			if failures > 0 {
				failures--
				return nil, errors.New("too many open files")
			}
			return openLogFile(name, flag, mode)
		}
		stderr := captureStderr(t)
		for _, line := range []string{"first\n", "second\n", "third\n"} {
			clock.advance(time.Minute)
			writeLine(t, writer, line)
		}
		stderr()
		releaseTestRotator(t, writer)

		assertEqual(t, readFile(t, path), "third\n")
		assertEqual(t, backupContents(t, path), "second\n")
	})

	t.Run("failed_rotation_waits_max_size_before_retrying", func(t *testing.T) {
		path, writer := readOnlyDirectoryRotator(t)
		stderr := captureStderr(t)
		for _, line := range []string{"first\n", "second\n", "third\n"} {
			writeLine(t, writer, line)
		}
		waited := stderr()
		stderr = captureStderr(t)
		writeLine(t, writer, "fourth\n")
		retried := stderr()

		assertEqual(t, readFile(t, path), "first\nsecond\nthird\nfourth\n")
		assertEqual(t, strings.Count(waited, "axio: rotation error"), 1)
		assertEqual(t, strings.Count(retried, "axio: rotation error"), 1)
	})
}

func TestRotator_Interval(t *testing.T) {
	hourly := RotationConfig{Interval: Duration(time.Hour)}

	t.Run("rotates_on_the_first_write_after_interval", func(t *testing.T) {
		path := tempFile(t, "app.log")
		writer := openTestRotator(t, path, hourly)
		clock := steppedClock(writer)
		clock.advance(30 * time.Minute)
		writeLine(t, writer, "first\n")
		clock.advance(time.Hour)
		writeLine(t, writer, "second\n")
		releaseTestRotator(t, writer)

		assertEqual(t, readFile(t, path), "second\n")
		assertEqual(t, len(logFiles(t, filepath.Dir(path))), 2)
		assertEqual(t, lockTime(t, path).Equal(clock.current), true)
	})

	t.Run("empty_file_restarts_the_clock", func(t *testing.T) {
		path := tempFile(t, "app.log")
		writer := openTestRotator(t, path, hourly)
		clock := steppedClock(writer)
		clock.advance(2 * time.Hour)
		writeLine(t, writer, "first\n")
		restarted := clock.current
		clock.advance(30 * time.Minute)
		writeLine(t, writer, "second\n")
		releaseTestRotator(t, writer)

		assertEqual(t, readFile(t, path), "first\nsecond\n")
		assertEqual(t, lockTime(t, path).Equal(restarted), true)
	})

	t.Run("size_rotation_restarts_the_clock", func(t *testing.T) {
		path := tempFile(t, "app.log")
		writer := openTestRotator(t, path, RotationConfig{MaxSize: 10, Interval: Duration(time.Hour)})
		writer.maxBytes = 10
		clock := steppedClock(writer)
		clock.advance(50 * time.Minute)
		writeLine(t, writer, "first\n")
		writeLine(t, writer, "second\n")
		clock.advance(20 * time.Minute)
		writeLine(t, writer, "3\n")
		releaseTestRotator(t, writer)

		assertEqual(t, readFile(t, path), "second\n3\n")
		assertEqual(t, len(logFiles(t, filepath.Dir(path))), 2)
	})

	t.Run("clock_set_back_starts_the_interval_again", func(t *testing.T) {
		path := tempFile(t, "app.log")
		writeFile(t, path, "old\n")
		writeFile(t, path+lockSuffix, "")
		lastRotation := time.Now().Add(7 * 24 * time.Hour)
		if err := os.Chtimes(path+lockSuffix, lastRotation, lastRotation); err != nil {
			t.Fatalf("chtimes %s: %v", path+lockSuffix, err)
		}
		writer := openTestRotator(t, path, hourly)
		clock := &testClock{current: time.Now()}
		writer.now = func() time.Time { return clock.current }
		writeLine(t, writer, "first\n")
		clock.advance(2 * time.Hour)
		writeLine(t, writer, "second\n")
		releaseTestRotator(t, writer)

		assertEqual(t, readFile(t, path), "second\n")
		assertEqual(t, backupContents(t, path), "old\nfirst\n")
	})

	t.Run("clock_is_kept_in_the_lock_file", func(t *testing.T) {
		path := tempFile(t, "app.log")
		writeFile(t, path, "old\n")
		writeFile(t, path+lockSuffix, "")
		lastRotation := time.Now().Add(-2 * time.Hour)
		if err := os.Chtimes(path+lockSuffix, lastRotation, lastRotation); err != nil {
			t.Fatalf("chtimes %s: %v", path+lockSuffix, err)
		}
		writer := openTestRotator(t, path, hourly)
		writeLine(t, writer, "new\n")
		releaseTestRotator(t, writer)

		assertEqual(t, readFile(t, path), "new\n")
		assertEqual(t, len(logFiles(t, filepath.Dir(path))), 2)
	})
}

func TestRotator_Cleanup(t *testing.T) {
	t.Run("max_backups_prunes_oldest", func(t *testing.T) {
		path := tempFile(t, "app.log")
		writer := openTestRotator(t, path, RotationConfig{MaxSize: 10, MaxBackups: 2})
		writer.maxBytes = 10
		clock := steppedClock(writer)
		for _, line := range []string{"first\n", "second\n", "third\n", "fourth\n", "fifth\n"} {
			clock.advance(time.Minute)
			writeLine(t, writer, line)
			writer.waitCleanup()
		}
		releaseTestRotator(t, writer)

		assertEqual(t, backupContents(t, path), "third\n|fourth\n")
		assertEqual(t, readFile(t, path), "fifth\n")
	})

	t.Run("max_age_prunes_old_backups", func(t *testing.T) {
		path := tempFile(t, "app.log")
		now := time.Now()
		writeBackup(t, path, now.Add(-72*time.Hour), time.UTC, "old\n")
		writeBackup(t, path, now.Add(-time.Hour), time.UTC, "recent\n")
		writer := openTestRotator(t, path, RotationConfig{MaxSize: 10, MaxAge: 1})
		writer.maxBytes = 10
		writeLine(t, writer, "first\n")
		writeLine(t, writer, "second\n")
		releaseTestRotator(t, writer)

		assertEqual(t, backupContents(t, path), "recent\n|first\n")
	})

	t.Run("compress_gzips_backups_with_their_mode", func(t *testing.T) {
		path := tempFile(t, "app.log")
		writeFile(t, path, "")
		if err := os.Chmod(path, 0o640); err != nil {
			t.Fatalf("chmod %s: %v", path, err)
		}
		writer := openTestRotator(t, path, RotationConfig{MaxSize: 10, Compress: true})
		writer.maxBytes = 10
		writeLine(t, writer, "first\n")
		writeLine(t, writer, "second\n")
		releaseTestRotator(t, writer)

		backups := backupPaths(t, path)
		if len(backups) != 1 || !strings.HasSuffix(backups[0], compressedSuffix) {
			t.Fatalf("expected one compressed backup, got %v", backups)
		}
		assertEqual(t, readCompressed(t, backups[0]), "first\n")
		if runtime.GOOS == "windows" {
			return
		}
		info, err := os.Stat(backups[0])
		assertNoError(t, err)
		assertEqual(t, info.Mode().Perm(), os.FileMode(0o640))
	})

	t.Run("counts_lumberjack_backups", func(t *testing.T) {
		path := tempFile(t, "app.log")
		now := time.Now()
		writeBackup(t, path, now.Add(-3*time.Hour), time.UTC, "older\n")
		writeBackup(t, path, now.Add(-2*time.Hour), time.UTC, "old\n")
		compressBackupForTest(t, writeBackup(t, path, now.Add(-time.Hour), time.UTC, "recent\n"))
		writer := openTestRotator(t, path, RotationConfig{MaxSize: 10, MaxBackups: 2})
		writer.maxBytes = 10
		writeLine(t, writer, "first\n")
		writeLine(t, writer, "second\n")
		releaseTestRotator(t, writer)

		assertEqual(t, backupContents(t, path), "recent\n|first\n")
	})

	t.Run("local_time_backups_are_read_in_local_time", func(t *testing.T) {
		original := time.Local
		time.Local = time.FixedZone("UTC-3", -3*60*60)
		t.Cleanup(func() { time.Local = original })
		path := tempFile(t, "app.log")
		writeBackup(t, path, time.Now().Add(-22*time.Hour), time.Local, "within a day\n")
		writer := openTestRotator(t, path, RotationConfig{MaxSize: 10, MaxAge: 1, LocalTime: true})
		writer.maxBytes = 10
		writeLine(t, writer, "first\n")
		writeLine(t, writer, "second\n")
		releaseTestRotator(t, writer)

		assertEqual(t, backupContents(t, path), "within a day\n|first\n")
	})

	t.Run("close_does_not_wait_for_cleanup", func(t *testing.T) {
		path := tempFile(t, "app.log")
		writer, unblock := blockedCompressionRotator(t, path)
		writeLine(t, writer, "first\n")
		writeLine(t, writer, "second\n")
		assertNoError(t, writer.release())

		assertEqual(t, len(backupPaths(t, path)), 1)
		assertEqual(t, strings.HasSuffix(backupPaths(t, path)[0], compressedSuffix), false)
		unblock()
		writer.waitCleanup()
		assertEqual(t, strings.HasSuffix(backupPaths(t, path)[0], compressedSuffix), true)
	})

	t.Run("other_rotation_waits_for_the_cleanup_of_a_closed_file", func(t *testing.T) {
		path := tempFile(t, "app.log")
		writer, unblock := blockedCompressionRotator(t, path)
		writeLine(t, writer, "first\n")
		writeLine(t, writer, "second\n")
		assertNoError(t, writer.release())
		opened := make(chan error, 1)
		go func() {
			other, err := RotatingFile(path, FormatJSON, RotationConfig{MaxSize: 20})
			if err == nil {
				err = other.Close()
			}
			opened <- err
		}()
		unblock()

		assertNoError(t, <-opened)
	})

	t.Run("plain_file_waits_for_the_cleanup_of_a_closed_file", func(t *testing.T) {
		path := tempFile(t, "app.log")
		writer, unblock := blockedCompressionRotator(t, path)
		writeLine(t, writer, "first\n")
		writeLine(t, writer, "second\n")
		assertNoError(t, writer.release())
		opened := make(chan error, 1)
		go func() {
			plain, err := File(path, FormatJSON)
			if err == nil {
				err = plain.Close()
			}
			opened <- err
		}()
		unblock()

		assertNoError(t, <-opened)
	})

	t.Run("reopened_during_cleanup_shares_the_rotator", func(t *testing.T) {
		path := tempFile(t, "app.log")
		writer, unblock := blockedCompressionRotator(t, path)
		writeLine(t, writer, "first\n")
		writeLine(t, writer, "second\n")
		assertNoError(t, writer.release())
		reopened := openTestRotator(t, path, RotationConfig{MaxSize: 10, Compress: true})
		writeLine(t, reopened, "3\n")
		unblock()
		releaseTestRotator(t, reopened)

		if reopened != writer {
			t.Error("expected the rotator still cleaning to be reopened")
		}
		assertEqual(t, readFile(t, path), "second\n3\n")
	})
}

// blockedCompressionRotator opens a rotator of 10 bytes that compresses its
// backups only once the returned function is called.
func blockedCompressionRotator(t *testing.T, path string) (*rotator, func()) {
	t.Helper()
	writer := openTestRotator(t, path, RotationConfig{MaxSize: 10, Compress: true})
	writer.maxBytes = 10
	release := make(chan struct{})
	writer.compressFile = func(backup string) error {
		<-release
		return compressBackup(backup)
	}
	var once sync.Once
	unblock := func() { once.Do(func() { close(release) }) }
	t.Cleanup(func() {
		unblock()
		writer.waitCleanup()
	})
	return writer, unblock
}

// writeBackup writes content to a backup of the log at path named for moment
// in location, as lumberjack names them, and returns its path.
func writeBackup(t *testing.T, path string, moment time.Time, location *time.Location, content string) string {
	t.Helper()
	directory, prefix, extension := backupParts(path)
	backup := filepath.Join(directory, prefix+moment.In(location).Format(backupTimeFormat)+extension)
	writeFile(t, backup, content)
	return backup
}

// compressBackupForTest compresses the backup at path as lumberjack does.
func compressBackupForTest(t *testing.T, path string) {
	t.Helper()
	if err := compressBackup(path); err != nil {
		t.Fatalf("compress %s: %v", path, err)
	}
}

// backupPaths returns the backups of the log at path, oldest first.
func backupPaths(t *testing.T, path string) []string {
	t.Helper()
	var backups []string
	for _, file := range logFiles(t, filepath.Dir(path)) {
		if file != path {
			backups = append(backups, file)
		}
	}
	return backups
}

// backupContents returns the contents of the backups of the log at path,
// oldest first, joined by "|".
func backupContents(t *testing.T, path string) string {
	t.Helper()
	var contents []string
	for _, backup := range backupPaths(t, path) {
		if strings.HasSuffix(backup, compressedSuffix) {
			contents = append(contents, readCompressed(t, backup))
			continue
		}
		contents = append(contents, readFile(t, backup))
	}
	return strings.Join(contents, "|")
}

// readCompressed returns the content of the gzip file at path.
func readCompressed(t *testing.T, path string) string {
	t.Helper()
	file, err := os.Open(path)
	if err != nil {
		t.Fatalf("open %s: %v", path, err)
	}
	defer file.Close()
	reader, err := gzip.NewReader(file)
	if err != nil {
		t.Fatalf("read gzip %s: %v", path, err)
	}
	content, err := io.ReadAll(reader)
	if err != nil {
		t.Fatalf("decompress %s: %v", path, err)
	}
	return string(content)
}

// testClock is a clock a test moves by hand, starting at the time a rotator
// read from its lock.
type testClock struct {
	current time.Time
}

// steppedClock replaces the clock of writer with one that starts at the start
// of its interval and moves only when advanced.
func steppedClock(writer *rotator) *testClock {
	clock := &testClock{current: writer.intervalStart}
	writer.now = func() time.Time { return clock.current }
	return clock
}

// advance moves the clock by duration.
func (c *testClock) advance(duration time.Duration) {
	c.current = c.current.Add(duration)
}

// lockTime returns the modification time of the lock beside the log at path.
func lockTime(t *testing.T, path string) time.Time {
	t.Helper()
	info, err := os.Stat(path + lockSuffix)
	if err != nil {
		t.Fatalf("stat lock of %s: %v", path, err)
	}
	return info.ModTime()
}

func TestOpenRotator(t *testing.T) {
	t.Run("same_path_shares_one_rotator", func(t *testing.T) {
		path := tempFile(t, "app.log")
		first := openTestRotator(t, path, RotationConfig{MaxSize: 10})
		second := openTestRotator(t, path, RotationConfig{MaxSize: 10})
		defer releaseTestRotator(t, first)
		defer releaseTestRotator(t, second)

		if first != second {
			t.Error("expected one rotator for one path")
		}
	})

	t.Run("relative_and_absolute_paths_share_one_rotator", func(t *testing.T) {
		directory := tempDir(t)
		t.Chdir(directory)
		relative := openTestRotator(t, "app.log", RotationConfig{MaxSize: 10})
		absolute := openTestRotator(t, filepath.Join(directory, "app.log"), RotationConfig{MaxSize: 10})
		defer releaseTestRotator(t, relative)
		defer releaseTestRotator(t, absolute)

		if relative != absolute {
			t.Error("expected one rotator for a relative and an absolute path to one file")
		}
	})

	t.Run("symlinked_directory_shares_one_rotator", func(t *testing.T) {
		target, link := symlinkedDirectory(t)
		direct := openTestRotator(t, filepath.Join(target, "app.log"), RotationConfig{MaxSize: 10})
		linked := openTestRotator(t, filepath.Join(link, "app.log"), RotationConfig{MaxSize: 10})
		defer releaseTestRotator(t, direct)
		defer releaseTestRotator(t, linked)

		if direct != linked {
			t.Error("expected one rotator for a file reached through a symlinked directory")
		}
	})

	t.Run("symlinked_log_rotates_its_target", func(t *testing.T) {
		directory := tempDir(t)
		target := filepath.Join(directory, "target.log")
		link := filepath.Join(directory, "app.log")
		if err := os.Symlink(target, link); err != nil {
			t.Skipf("symlinks unavailable: %v", err)
		}
		writer := openTestRotator(t, link, RotationConfig{MaxSize: 10})
		writer.maxBytes = 10
		writeLine(t, writer, "first\n")
		writeLine(t, writer, "second\n")
		releaseTestRotator(t, writer)

		info, err := os.Lstat(link)
		assertNoError(t, err)
		if info.Mode()&os.ModeSymlink == 0 {
			t.Errorf("expected %s to stay a symlink, got mode %v", link, info.Mode())
		}
		assertEqual(t, readFile(t, target), "second\n")
		assertEqual(t, len(logFiles(t, directory)), 3)
	})

	t.Run("write_after_last_release_fails", func(t *testing.T) {
		writer := openTestRotator(t, tempFile(t, "app.log"), RotationConfig{MaxSize: 10})
		releaseTestRotator(t, writer)

		if _, err := writer.Write([]byte("late\n")); !errors.Is(err, ErrOutputClosed) {
			t.Errorf("expected ErrOutputClosed, got %v", err)
		}
	})

	t.Run("reopened_after_last_release_appends", func(t *testing.T) {
		path := tempFile(t, "app.log")
		first := openTestRotator(t, path, RotationConfig{MaxSize: 10})
		writeLine(t, first, "first\n")
		releaseTestRotator(t, first)
		second := openTestRotator(t, path, RotationConfig{MaxSize: 10})
		writeLine(t, second, "second\n")
		releaseTestRotator(t, second)

		assertEqual(t, readFile(t, path), "first\nsecond\n")
	})
}

// symlinkedDirectory returns a directory and a symlink to it, skipping the
// test where symlinks cannot be made.
func symlinkedDirectory(t *testing.T) (string, string) {
	t.Helper()
	root := tempDir(t)
	target := filepath.Join(root, "target")
	if err := os.Mkdir(target, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", target, err)
	}
	link := filepath.Join(root, "link")
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	return target, link
}

// openTestRotator opens the rotator of path, failing the test when it cannot.
func openTestRotator(t *testing.T, path string, rotation RotationConfig) *rotator {
	t.Helper()
	writer, err := openRotator(path, rotation)
	if err != nil {
		t.Fatalf("open rotator of %s: %v", path, err)
	}
	return writer
}

// releaseTestRotator releases writer and waits for its cleanup to end.
func releaseTestRotator(t *testing.T, writer *rotator) {
	t.Helper()
	assertNoError(t, writer.release())
	writer.waitCleanup()
}

// writeLine writes line through writer, failing the test when it cannot.
func writeLine(t *testing.T, writer *rotator, line string) {
	t.Helper()
	if _, err := writer.Write([]byte(line)); err != nil {
		t.Fatalf("write %q: %v", line, err)
	}
}

// readOnlyDirectoryRotator opens a rotator of 10 bytes whose directory then
// refuses every rename, and returns the path of its log.
func readOnlyDirectoryRotator(t *testing.T) (string, *rotator) {
	t.Helper()
	if runtime.GOOS == "windows" || os.Geteuid() == 0 {
		t.Skip("needs a directory the process cannot write to")
	}
	directory := tempDir(t)
	path := filepath.Join(directory, "app.log")
	writer := openTestRotator(t, path, RotationConfig{MaxSize: 10})
	writer.maxBytes = 10
	if err := os.Chmod(directory, 0o500); err != nil {
		t.Fatalf("chmod %s: %v", directory, err)
	}
	t.Cleanup(func() {
		_ = os.Chmod(directory, 0o700)
		releaseTestRotator(t, writer)
	})
	return path, writer
}
