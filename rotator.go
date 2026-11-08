package axio

import (
	"compress/gzip"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

const (
	// backupTimeFormat is the time in a backup's name: lumberjack's, so the
	// backups it left are counted by MaxBackups and MaxAge.
	backupTimeFormat = "2006-01-02T15-04-05.000"
	// compressedSuffix ends the name of a backup compressed with gzip.
	compressedSuffix = ".gz"
	// lockSuffix ends the name of the file beside a rotating log that holds its
	// lock between processes.
	lockSuffix = ".lock"
	// rotatingFileMode is the mode of a rotating log created where none was,
	// and of its lock.
	rotatingFileMode os.FileMode = 0o600
)

// rotator is the one writer of a rotating log file in this process: every
// [RotatingFile] on the same file writes through it, so no two rotations of the
// file run against each other. It holds the lock of the file against other
// processes while any output uses it, or while a cleanup of its backups runs.
//
// A write rotates the log first when it would take the log past its size or
// when its interval has passed, and writes to the log as it is when the
// rotation fails. Once no output uses the rotator, a write returns
// [ErrOutputClosed].
type rotator struct {
	path     string // resolved path of the log
	rotation RotationConfig
	maxBytes int64 // size that rotates the log; zero for none
	// now, openFile and compressFile are time.Now, openLogFile and
	// compressBackup, or what a test drives instead.
	now           func() time.Time
	openFile      func(path string, flag int, mode os.FileMode) (*os.File, error)
	compressFile  func(path string) error
	lock          *os.File   // touched only under the mutex of filesByPath
	mutex         sync.Mutex // guards everything below
	file          *os.File
	size          int64
	intervalStart time.Time
	released      bool  // no output uses the rotator any more
	retrySize     int64 // size a failed rotation waits for before trying again
	cleaned       chan struct{}
	pending       bool // a rotation came during the cleanup, which runs again
	cleanupTime   time.Time
	lastError     atomic.Pointer[error]
}

// openRotator returns this process's rotator for the file at path, creating
// its directory and the file when missing: the same rotator for every path
// that reaches the same file, a relative one or one through a symlink. The
// first one takes the lock of the file. A file that no output uses any more,
// held only by the cleanup of its backups, is opened with another rotation
// once that cleanup ends.
//
// Returns [ErrOpenFile] when the file cannot be opened, [ErrRotationConflict]
// when this process has it open with another rotation or without one, and
// [ErrRotatingFileLocked] when another process holds it.
func openRotator(path string, rotation RotationConfig) (*rotator, error) {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return nil, fmt.Errorf("%w: %s: %w", ErrOpenFile, path, err)
	}
	for {
		writer, cleaned, err := acquireRotator(absolute, rotation)
		if cleaned == nil {
			return writer, err
		}
		<-cleaned
	}
}

// acquireRotator returns the rotator of the file at absolute, or the channel to
// wait on when the only holder of the file is a cleanup that the request would
// conflict with. A path already registered as it is opens nothing; any other
// is opened and resolved, all under the mutex of filesByPath, so no other
// rotator of the file can appear meanwhile.
func acquireRotator(absolute string, rotation RotationConfig) (*rotator, <-chan struct{}, error) {
	filesByPath.mutex.Lock()
	defer filesByPath.mutex.Unlock()
	if entry, ok := filesByPath.entries[absolute]; ok {
		return entry.acquire(absolute, rotation)
	}
	file, err := openCreatingDirectory(absolute)
	if err != nil {
		return nil, nil, fmt.Errorf("%w: %s: %w", ErrOpenFile, absolute, err)
	}
	key := canonicalPath(file, absolute)
	if key != absolute {
		// The file is reached at key, where a rotator may have it open already;
		// the one opened here may be a log that rotator has since moved aside.
		_ = file.Close()
		if entry, ok := filesByPath.entries[key]; ok {
			return entry.acquire(key, rotation)
		}
		if file, err = openLogFile(key, os.O_APPEND|os.O_CREATE|os.O_WRONLY, rotatingFileMode); err != nil {
			return nil, nil, fmt.Errorf("%w: %s: %w", ErrOpenFile, absolute, err)
		}
	}
	writer, err := newRotator(key, rotation, file)
	if err != nil {
		return nil, nil, err
	}
	filesByPath.entries[key] = fileEntry{rotation: rotation, references: 1, rotator: writer}
	return writer, nil, nil
}

// newRotator returns the rotator of the log at path, open in file, once it has
// taken the lock of the log. On failure it closes file.
func newRotator(path string, rotation RotationConfig, file *os.File) (*rotator, error) {
	lock, err := claimRotation(path)
	if err == nil {
		writer := &rotator{
			path:         path,
			rotation:     rotation,
			maxBytes:     megabytes(rotation.MaxSize),
			now:          time.Now,
			openFile:     openLogFile,
			compressFile: compressBackup,
			lock:         lock,
			file:         file,
		}
		if err = writer.prepare(); err == nil {
			return writer, nil
		}
		// Unlocking is all that is left to undo; the error that brought us here is the one to report.
		_ = lock.Close()
	}
	// Nothing was written through the log, so its close error says nothing.
	_ = file.Close()
	return nil, err
}

// release gives back one reference to the rotator. The last one closes the
// log, unlocks it and forgets the rotator, so the next output on the file
// opens it anew; while a cleanup of the backups runs, the lock is kept and the
// cleanup does the rest when it ends.
func (r *rotator) release() error {
	filesByPath.mutex.Lock()
	defer filesByPath.mutex.Unlock()
	entry, ok := filesByPath.entries[r.path]
	if !ok || entry.rotator != r {
		return nil
	}
	entry.references--
	filesByPath.entries[r.path] = entry
	if entry.references > 0 {
		return nil
	}
	cleaning, closeErr := r.close()
	if cleaning {
		return closeErr
	}
	delete(filesByPath.entries, r.path)
	return errors.Join(closeErr, r.unlock())
}

// lastRotationError returns the error of the most recent rotation attempt,
// nil when it succeeded or none was made.
func (r *rotator) lastRotationError() error {
	if stored := r.lastError.Load(); stored != nil {
		return *stored
	}
	return nil
}

// waitCleanup waits for the cleanup of backups in progress to end.
func (r *rotator) waitCleanup() {
	for cleaned := r.cleanupDone(); cleaned != nil; cleaned = r.cleanupDone() {
		<-cleaned
	}
}

// cleanupDone returns the channel closed when the cleanup of the backups in
// progress ends, nil when none runs.
func (r *rotator) cleanupDone() <-chan struct{} {
	r.mutex.Lock()
	defer r.mutex.Unlock()
	return r.cleaned
}

// reopen takes the rotator back for an output, after the last one released it.
func (r *rotator) reopen() {
	r.mutex.Lock()
	defer r.mutex.Unlock()
	r.released = false
}

// open opens the log for appending when it is not open, creating it.
func (r *rotator) open() error {
	if r.file != nil {
		return nil
	}
	file, err := r.openFile(r.path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, rotatingFileMode)
	if err != nil {
		return fmt.Errorf("%w: %s: %w", ErrOpenFile, r.path, err)
	}
	r.file = file
	if err := r.measure(); err != nil {
		// Nothing was written through the file, so its close error says nothing.
		_ = file.Close()
		r.file = nil
		return err
	}
	return nil
}

// prepare takes the size of the open log and, when it rotates by time, the
// start of its interval: the modification time of its lock, the time of its
// last rotation.
func (r *rotator) prepare() error {
	if err := r.measure(); err != nil {
		return err
	}
	if r.rotation.Interval <= 0 {
		return nil
	}
	info, err := r.lock.Stat()
	if err != nil {
		return fmt.Errorf("%w: %s%s: %w", ErrOpenFile, r.path, lockSuffix, err)
	}
	r.intervalStart = info.ModTime()
	return nil
}

// measure takes the size of the open log, the offset of its end, which an
// append-only file writes at.
func (r *rotator) measure() error {
	size, err := r.file.Seek(0, io.SeekEnd)
	if err != nil {
		return fmt.Errorf("%w: %s: %w", ErrOpenFile, r.path, err)
	}
	r.size = size
	return nil
}

// close closes the log and refuses every later write, and reports whether a
// cleanup of the backups is still running.
func (r *rotator) close() (bool, error) {
	r.mutex.Lock()
	defer r.mutex.Unlock()
	r.released = true
	cleaning := r.cleaned != nil
	if r.file == nil {
		return cleaning, nil
	}
	err := r.file.Close()
	r.file = nil
	if err != nil {
		return cleaning, fmt.Errorf("close rotating file %s: %w", r.path, err)
	}
	return cleaning, nil
}

// unlock releases the lock of the log to other processes.
func (r *rotator) unlock() error {
	if err := r.lock.Close(); err != nil {
		return fmt.Errorf("unlock rotating file %s: %w", r.path, err)
	}
	return nil
}

// rotateIfDue rotates the log when a write of length would take it past its
// size or its interval has passed. The interval starts again at every rotation
// that moves the log aside, which also cleans the backups; at an empty log,
// which is not rotated; and at a start later than now, which only a clock set
// back leaves. A failed rotation is reported on stderr and recorded, and one
// that left the log in place waits for another interval or for the log to grow
// by its size once more.
func (r *rotator) rotateIfDue(length int64) {
	sizeDue := r.sizeDue(length)
	if !sizeDue && r.rotation.Interval <= 0 {
		return
	}
	now := r.now()
	if r.intervalStart.After(now) {
		r.restartClock(now)
	}
	if !sizeDue && !r.intervalDue(now) {
		return
	}
	if r.size == 0 {
		r.restartClock(now)
		return
	}
	moved, err := r.rotate(now)
	r.retrySize = 0
	if moved {
		r.restartClock(now)
		r.scheduleCleanup(now)
	} else {
		r.retrySize = r.size + r.maxBytes
		r.intervalStart = now
	}
	r.recordRotation(err)
}

// recordRotation keeps the outcome of a rotation for [rotator.lastRotationError],
// and reports a failure on stderr.
func (r *rotator) recordRotation(err error) {
	if err == nil {
		r.lastError.Store(nil)
		return
	}
	r.lastError.Store(&err)
	fmt.Fprintf(os.Stderr, "axio: rotation error: %v\n", err)
}

// intervalDue reports whether the interval of the log has passed at now.
func (r *rotator) intervalDue(now time.Time) bool {
	return r.rotation.Interval > 0 && now.Sub(r.intervalStart) >= time.Duration(r.rotation.Interval)
}

// restartClock starts the interval of the log at now, in the rotator and in
// the modification time of its lock, where the next rotator of the file reads
// it — after the last output closed it, or in the next process.
func (r *rotator) restartClock(now time.Time) {
	r.intervalStart = now
	if err := os.Chtimes(r.path+lockSuffix, now, now); err != nil {
		fmt.Fprintf(os.Stderr, "axio: rotation error: restart the interval of %s: %v\n", r.path, err)
	}
}

// sizeDue reports whether a write of length takes the log past its size. An
// empty log takes the write whole, however long.
func (r *rotator) sizeDue(length int64) bool {
	return r.maxBytes > 0 && r.size > 0 && r.size+length > r.maxBytes && r.size >= r.retrySize
}

// rotate moves the log to its backup name and opens a new one in its place,
// with its mode and owner, and reports whether the log was moved. A failure
// leaves the log closed, for the write to open it again: the old one when it
// was not moved, a new one when it was.
func (r *rotator) rotate(now time.Time) (bool, error) {
	previous, err := r.file.Stat()
	if err != nil {
		return false, fmt.Errorf("stat %s: %w", r.path, err)
	}
	closeErr := r.file.Close()
	r.file = nil
	if closeErr != nil {
		return false, fmt.Errorf("close %s: %w", r.path, closeErr)
	}
	if err := os.Rename(r.path, r.backupName(now)); err != nil {
		return false, fmt.Errorf("move %s aside: %w", r.path, err)
	}
	r.size = 0
	file, err := r.openFile(r.path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, previous.Mode().Perm())
	if err != nil {
		return true, fmt.Errorf("open new %s: %w", r.path, err)
	}
	r.file = file
	r.keepAttributes(previous)
	return true, nil
}

// keepAttributes gives the new log the mode of the one it replaced, which the
// umask may have narrowed, and its owner. A failure is reported on stderr and
// the rotation stands.
func (r *rotator) keepAttributes(previous os.FileInfo) {
	if err := r.file.Chmod(previous.Mode().Perm()); err != nil {
		fmt.Fprintf(os.Stderr, "axio: rotation error: keep the mode of %s: %v\n", r.path, err)
	}
	if err := copyOwner(r.path, previous); err != nil {
		fmt.Fprintf(os.Stderr, "axio: rotation error: keep the owner of %s: %v\n", r.path, err)
	}
}

// backupName returns the name the log moves to when rotated at now, in
// lumberjack's form, its time in UTC unless LocalTime is set. A name a backup
// already holds, compressed or not, moves the time a millisecond on, so no
// backup is overwritten.
func (r *rotator) backupName(now time.Time) string {
	if !r.rotation.LocalTime {
		now = now.UTC()
	}
	directory, prefix, extension := backupParts(r.path)
	for {
		name := filepath.Join(directory, prefix+now.Format(backupTimeFormat)+extension)
		if !exists(name) && !exists(name+compressedSuffix) {
			return name
		}
		now = now.Add(time.Millisecond)
	}
}

// scheduleCleanup starts the cleanup of the backups after a rotation at now,
// when the rotation asks for one: one cleanup at a time, run once more when a
// rotation comes while it runs.
func (r *rotator) scheduleCleanup(now time.Time) {
	if r.rotation.MaxBackups <= 0 && r.rotation.MaxAge <= 0 && !r.rotation.Compress {
		return
	}
	r.cleanupTime = now
	if r.cleaned != nil {
		r.pending = true
		return
	}
	r.cleaned = make(chan struct{})
	go r.runCleanup()
}

// runCleanup cleans the backups until no rotation asks for another cleanup.
func (r *rotator) runCleanup() {
	for {
		r.mutex.Lock()
		now := r.cleanupTime
		r.mutex.Unlock()
		r.cleanBackups(now)
		if r.finishCleanup() {
			return
		}
	}
}

// finishCleanup ends the cleanup unless a rotation asked for another, and
// reports whether it ended. A cleanup that ends with no output on the rotator
// unlocks the log and forgets the rotator, as the last release would have.
func (r *rotator) finishCleanup() bool {
	filesByPath.mutex.Lock()
	defer filesByPath.mutex.Unlock()
	r.mutex.Lock()
	defer r.mutex.Unlock()
	if r.pending {
		r.pending = false
		return false
	}
	close(r.cleaned)
	r.cleaned = nil
	entry, ok := filesByPath.entries[r.path]
	if !ok || entry.rotator != r || entry.references > 0 {
		return true
	}
	delete(filesByPath.entries, r.path)
	if err := r.unlock(); err != nil {
		fmt.Fprintf(os.Stderr, "axio: rotation error: %v\n", err)
	}
	return true
}

// cleanBackups removes the backups that MaxBackups and MaxAge leave out at
// now, and compresses the rest when Compress is set. A failure is reported on
// stderr and the cleanup goes on with the next backup.
func (r *rotator) cleanBackups(now time.Time) {
	backups, err := listBackups(r.path, r.rotation.LocalTime)
	if err != nil {
		fmt.Fprintf(os.Stderr, "axio: rotation cleanup error: %v\n", err)
		return
	}
	remove, compress := selectBackups(backups, r.rotation, now)
	for _, backup := range remove {
		if err := os.Remove(backup.path); err != nil {
			fmt.Fprintf(os.Stderr, "axio: rotation cleanup error: %v\n", err)
		}
	}
	for _, backup := range compress {
		if err := r.compressFile(backup.path); err != nil {
			fmt.Fprintf(os.Stderr, "axio: rotation cleanup error: %v\n", err)
		}
	}
}

func (r *rotator) Write(data []byte) (int, error) {
	r.mutex.Lock()
	defer r.mutex.Unlock()
	if r.released {
		return 0, fmt.Errorf("%w: %s: %w", ErrOutputClosed, r.path, os.ErrClosed)
	}
	if err := r.open(); err != nil {
		return 0, err
	}
	r.rotateIfDue(int64(len(data)))
	if err := r.open(); err != nil {
		return 0, err
	}
	written, err := r.file.Write(data)
	r.size += int64(written)
	return written, err
}

func (r *rotator) Sync() error {
	r.mutex.Lock()
	defer r.mutex.Unlock()
	if r.file == nil {
		return nil
	}
	return r.file.Sync()
}

// fileEntry is a log file this process has open, a plain [File] or a rotating
// one, with the number of outputs that use it.
type fileEntry struct {
	rotation   RotationConfig
	references int
	rotator    *rotator // nil for a plain file
}

// acquire returns the rotator of the entry, registered at key, for one more
// output with rotation. When the file is open with another rotation or without
// one, it returns the channel to wait on if only a cleanup holds the file, and
// [ErrRotationConflict] otherwise.
func (e fileEntry) acquire(key string, rotation RotationConfig) (*rotator, <-chan struct{}, error) {
	if e.rotator == nil || e.rotation != rotation {
		if cleaned := e.cleanupDone(); cleaned != nil {
			return nil, cleaned, nil
		}
		return nil, nil, fmt.Errorf("%w: %s", ErrRotationConflict, key)
	}
	e.references++
	filesByPath.entries[key] = e
	e.rotator.reopen()
	return e.rotator, nil, nil
}

// cleanupDone returns the channel closed when the cleanup that alone holds the
// entry ends, nil when an output uses the entry or no cleanup runs.
func (e fileEntry) cleanupDone() <-chan struct{} {
	if e.references > 0 || e.rotator == nil {
		return nil
	}
	return e.rotator.cleanupDone()
}

// filesByPath holds the log files this process has open, by resolved path, so
// every output on one file shares its rotation and no rotation moves the file
// under another. Besides chainsByPath, it is the package's other deliberate
// piece of state, for the same reason: two writers of one file in one process.
var filesByPath = struct {
	mutex   sync.Mutex
	entries map[string]fileEntry
}{entries: make(map[string]fileEntry)}

// registerPlainFile counts one more [File] on the file at path, open in file,
// and returns the key that releases it. A file held only by the cleanup of a
// rotating file is registered once that cleanup ends. Returns
// [ErrRotationConflict] when this process has the file open as a rotating one.
func registerPlainFile(path string, file *os.File) (string, error) {
	absolute, err := filepath.Abs(path)
	if err != nil {
		absolute = path
	}
	key := canonicalPath(file, absolute)
	for {
		cleaned, err := registerPlainKey(key, path)
		if cleaned == nil {
			return key, err
		}
		<-cleaned
	}
}

// registerPlainKey counts one more [File] on the file registered at key, or
// returns the channel to wait on when only a cleanup of a rotating file holds
// it.
func registerPlainKey(key, path string) (<-chan struct{}, error) {
	filesByPath.mutex.Lock()
	defer filesByPath.mutex.Unlock()
	entry, ok := filesByPath.entries[key]
	if !ok {
		filesByPath.entries[key] = fileEntry{references: 1}
		return nil, nil
	}
	if entry.rotator != nil {
		if cleaned := entry.cleanupDone(); cleaned != nil {
			return cleaned, nil
		}
		return nil, fmt.Errorf("%w: %s", ErrRotationConflict, path)
	}
	entry.references++
	filesByPath.entries[key] = entry
	return nil, nil
}

// releasePlainFile counts one [File] less on the file registered under key.
func releasePlainFile(key string) {
	filesByPath.mutex.Lock()
	defer filesByPath.mutex.Unlock()
	entry, ok := filesByPath.entries[key]
	if !ok || entry.rotator != nil {
		return
	}
	entry.references--
	if entry.references == 0 {
		delete(filesByPath.entries, key)
		return
	}
	filesByPath.entries[key] = entry
}

// claimRotation opens the lock file of the log at path and takes it. Returns
// [ErrRotatingFileLocked] when another process holds it, and [ErrOpenFile]
// when the lock cannot be opened or taken.
func claimRotation(path string) (*os.File, error) {
	lock, err := openLogFile(path+lockSuffix, os.O_RDWR|os.O_CREATE, rotatingFileMode)
	if err != nil {
		return nil, fmt.Errorf("%w: %s%s: %w", ErrOpenFile, path, lockSuffix, err)
	}
	locked, err := lockExclusive(lock)
	if err != nil || !locked {
		// The lock is not kept, so the file holds nothing worth reporting a close error for.
		_ = lock.Close()
	}
	if err != nil {
		return nil, fmt.Errorf("%w: lock %s: %w", ErrOpenFile, path, err)
	}
	if !locked {
		return nil, fmt.Errorf("%w: %s", ErrRotatingFileLocked, path)
	}
	return lock, nil
}

// openCreatingDirectory opens the log at path for appending, creating it, and
// its directory only when the open finds the directory missing.
func openCreatingDirectory(path string) (*os.File, error) {
	file, err := openLogFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, rotatingFileMode)
	if err == nil || !errors.Is(err, os.ErrNotExist) {
		return file, err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	return openLogFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, rotatingFileMode)
}

// backup is a rotated copy of a log, compressed or not.
type backup struct {
	path       string
	name       string // the name without the compressed suffix
	time       time.Time
	compressed bool
}

// listBackups returns the backups of the log at path, newest first: the files
// beside it named as lumberjack names them, compressed or not, their times read
// in local time when local is set and in UTC otherwise.
func listBackups(path string, local bool) ([]backup, error) {
	directory, prefix, extension := backupParts(path)
	entries, err := os.ReadDir(directory)
	if err != nil {
		return nil, fmt.Errorf("list backups of %s: %w", path, err)
	}
	var backups []backup
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		compressed := strings.HasSuffix(name, compressedSuffix)
		plain := strings.TrimSuffix(name, compressedSuffix)
		if moment, ok := backupTime(plain, prefix, extension, local); ok {
			backups = append(backups, backup{path: filepath.Join(directory, name), name: plain, time: moment, compressed: compressed})
		}
	}
	slices.SortFunc(backups, func(first, second backup) int { return second.time.Compare(first.time) })
	return backups, nil
}

// backupTime returns the time in the name of a backup with prefix and
// extension, and false when name is not one.
func backupTime(name, prefix, extension string, local bool) (time.Time, bool) {
	if len(name) < len(prefix)+len(extension) || !strings.HasPrefix(name, prefix) || !strings.HasSuffix(name, extension) {
		return time.Time{}, false
	}
	location := time.UTC
	if local {
		location = time.Local
	}
	moment, err := time.ParseInLocation(backupTimeFormat, name[len(prefix):len(name)-len(extension)], location)
	return moment, err == nil
}

// selectBackups splits backups, newest first, into those rotation removes at
// now — past MaxBackups, a backup and its compressed copy counted as one, or
// older than MaxAge days — and those it compresses.
func selectBackups(backups []backup, rotation RotationConfig, now time.Time) (remove, compress []backup) {
	cutoff := now.Add(-time.Duration(rotation.MaxAge) * 24 * time.Hour)
	counted := make(map[string]bool)
	for _, candidate := range backups {
		counted[candidate.name] = true
		tooMany := rotation.MaxBackups > 0 && len(counted) > rotation.MaxBackups
		tooOld := rotation.MaxAge > 0 && candidate.time.Before(cutoff)
		switch {
		case tooMany || tooOld:
			remove = append(remove, candidate)
		case rotation.Compress && !candidate.compressed:
			compress = append(compress, candidate)
		}
	}
	return remove, compress
}

// resolvePath returns the absolute path with every symlink in it resolved, or
// the path as given where it cannot be resolved.
func resolvePath(absolute string) string {
	resolved, err := filepath.EvalSymlinks(absolute)
	if err != nil {
		return absolute
	}
	return resolved
}

// backupParts returns the directory of the log at path and what a backup's
// name carries before and after its time.
func backupParts(path string) (directory, prefix, extension string) {
	base := filepath.Base(path)
	extension = filepath.Ext(base)
	return filepath.Dir(path), base[:len(base)-len(extension)] + "-", extension
}

// exists reports whether something is at path. An error other than its absence
// reads as absence, so the rename that follows reports it.
func exists(path string) bool {
	_, err := os.Lstat(path)
	return err == nil
}

// megabytes returns size megabytes in bytes, zero for no size: a size of zero
// or less, or one whose bytes do not fit an int64.
func megabytes(size int) int64 {
	if size <= 0 || int64(size) > math.MaxInt64>>20 {
		return 0
	}
	return int64(size) << 20
}

// compressBackup compresses the backup at path into path.gz, with its mode and
// owner, and removes it once closed. A compressed copy left by an attempt that
// failed is replaced; one this attempt leaves half written is removed.
func compressBackup(path string) error {
	source, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("compress %s: %w", path, err)
	}
	err = compressInto(path+compressedSuffix, source)
	// The backup was only read, so its close error says nothing about the copy.
	_ = source.Close()
	if err != nil {
		// The half-written copy is the thing to drop; its removal error adds nothing to err.
		_ = os.Remove(path + compressedSuffix)
		return fmt.Errorf("compress %s: %w", path, err)
	}
	return os.Remove(path)
}

// compressInto writes source, compressed with gzip, into a new file at target
// with the mode and owner of source.
func compressInto(target string, source *os.File) error {
	info, err := source.Stat()
	if err != nil {
		return err
	}
	file, err := os.OpenFile(target, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, info.Mode().Perm())
	if err != nil {
		return err
	}
	return writeCompressed(file, source, info)
}

// writeCompressed writes source into target with gzip, gives target the mode
// and owner in info, and closes it.
func writeCompressed(target *os.File, source io.Reader, info os.FileInfo) error {
	writer := gzip.NewWriter(target)
	_, copyErr := io.Copy(writer, source)
	err := errors.Join(copyErr, writer.Close())
	if err == nil {
		err = errors.Join(target.Chmod(info.Mode().Perm()), copyOwner(target.Name(), info))
	}
	return errors.Join(err, target.Close())
}
