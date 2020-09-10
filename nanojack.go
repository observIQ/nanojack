// Package nanojack provides a rolling logger that can be precisely
// controlled for the purpose of testing logging software.
// This project was forked from "gopkg.in/natefinch/lumberjack"
//
//   import "github.com/observiq/nanojack"
//
// Nanojack is intended to be one part of a logging infrastructure.
// It is not an all-in-one solution, but instead is a pluggable
// component at the bottom of the logging stack that simply controls the files
// to which logs are written.
//
// Nanojack plays well with any logging package that can write to an
// io.Writer, including the standard library's log package.
//
// Nanojack assumes that only one process is writing to the output files.
// Using the same nanojack configuration from multiple processes on the same
// machine will result in improper behavior.
package nanojack

import (
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

const (
	backupTimeFormat = "2006-01-02T15-04-05.000000000"
	defaultMaxLines  = 10
)

// ensure we always implement io.WriteCloser
var _ io.WriteCloser = (*Logger)(nil)

// Logger is an io.WriteCloser that writes to the specified filename.
//
// Logger opens or creates the logfile on first Write.  If the file exists and
// is less than MaxLines, nanojack will open and append to that file.
// If the file exists and its size is >= MaxLines, the file is renamed
// by putting the current time in a timestamp in the name immediately before the
// file's extension (or the end of the filename if there's no extension). A new
// log file is then created using original filename.
//
// Whenever a write would cause the current log file to exceed MaxLines,
// the current file is closed, renamed, and a new log file created with the
// original name. Thus, the filename you give Logger is always the "current" log
// file.
//
// Backups use the log file name given to Logger, in the form
// `name-timestamp.ext` where name is the filename without the extension,
// timestamp is the time at which the log was rotated formatted with the
// time.Time format of `2006-01-02T15-04-05.000000000` and the extension is the
// original extension.  For example, if your Logger.Filename is
// `/var/log/foo/server.log`, a backup created at 6:30pm on Nov 11 2016 would
// use the filename `/var/log/foo/server-2016-11-04T18-30-00.000000000.log`
//
// Cleaning Up Old Log Files
//
// Whenever a new logfile gets created, old log files may be deleted.  The most
// recent files according to the encoded timestamp will be retained, up to a
// number equal to MaxBackups (or all of them if MaxBackups is 0). If MaxBackups
// is 0, no old log files will be deleted.
type Logger struct {
	// Filename is the file to write logs to.  Backup log files will be retained
	// in the same directory.  It uses <processname>-nanojack.log in
	// os.TempDir() if empty.
	Filename string `json:"filename" yaml:"filename"`

	// MaxLines is the maximum lines to the log file before it gets rotated.
	// It defaults to 10 lines.
	MaxLines int `json:"maxlines" yaml:"maxlines"`

	// MaxBackups is the maximum number of old log files to retain.  The default
	// is to retain all old log files.
	MaxBackups int `json:"maxbackups" yaml:"maxbackups"`

	// CopyTruncate defines the mechanism by which a file is backed up.
	// By default a backup is created by renaming the old file and creating
	// a new file in its place. If CopyTruncate is true, the old file will be
	// copied to a new file and then truncated.
	CopyTruncate bool `json:"copytruncate" yaml:"copytruncate"`

	lines int64
	file  *os.File
	mu    sync.Mutex
}

var (
	// currentTime exists so it can be mocked out by tests.
	currentTime = time.Now

	// os_Stat exists so it can be mocked out by tests.
	os_Stat = os.Stat
)

// Write implements io.Writer.  If a write would cause the log file to be larger
// than MaxLines, the file is closed, renamed to include a timestamp of the
// current time, and a new log file is created using the original log file name.
// If the length of the write is greater than MaxLines, an error is returned.
func (l *Logger) Write(p []byte) (n int, err error) {
	l.mu.Lock()
	defer l.mu.Unlock()

	if l.file == nil {
		if err = l.openExistingOrNew(); err != nil {
			return 0, err
		}
	}

	if l.lines+1 > l.max() {
		if err := l.rotate(); err != nil {
			return 0, err
		}
	}

	n, err = l.file.Write(p)
	l.lines++

	return n, err
}

// Close implements io.Closer, and closes the current logfile.
func (l *Logger) Close() error {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.close()
}

// close closes the file if it is open.
func (l *Logger) close() error {
	if l.file == nil {
		return nil
	}
	err := l.file.Close()
	l.file = nil
	return err
}

// Rotate causes Logger to close the existing log file and immediately create a
// new one.  This is a helper function for applications that want to initiate
// rotations outside of the normal rotation rules, such as in response to
// SIGHUP.  After rotating, this initiates a cleanup of old log files according
// to the normal rules.
func (l *Logger) Rotate() error {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.rotate()
}

// rotate closes the current file, moves it aside with a timestamp in the name,
// (if it exists), opens a new file with the original filename, and then runs
// cleanup.
func (l *Logger) rotate() error {
	if err := l.close(); err != nil {
		return err
	}

	if err := l.openNew(); err != nil {
		return err
	}

	return l.cleanup()
}

// openNew opens a new log file for writing, moving any old log file out of the
// way.  This methods assumes the file has already been closed.
func (l *Logger) openNew() error {
	if err := os.MkdirAll(l.dir(), 0744); err != nil {
		return fmt.Errorf("can't make directories for new logfile: %s", err)
	}
	if _, err := os_Stat(l.filename()); err != nil {
		// No previous file to backup.
		f, err := os.OpenFile(l.filename(), os.O_CREATE|os.O_WRONLY|os.O_TRUNC, os.FileMode(0644))
		if err != nil {
			return fmt.Errorf("can't open new logfile: %s", err)
		}
		l.file = f
		l.lines = 0
		return nil
	}
	return l.backup()
}

// backup and replace the log file according to the configured mechanism.
// This method assumes that the appropriate directory exists.
func (l *Logger) backup() (err error) {
	var f *os.File

	if l.CopyTruncate {
		f, err = l.copyTruncate()
	} else {
		f, err = l.moveCreate()
	}

	if err != nil {
		return
	}

	l.file = f
	l.lines = 0
	return
}

func (l *Logger) copyTruncate() (*os.File, error) {
	name := l.filename()
	bkpName := l.backupName()

	info, err := os_Stat(name)
	if err != nil {
		return nil, err
	}

	f, err := os.OpenFile(name, os.O_RDWR, info.Mode())
	if err != nil {
		return nil, err
	}

	bkp, err := os.OpenFile(bkpName, os.O_CREATE|os.O_RDWR, info.Mode())
	if err != nil {
		return nil, err
	}
	defer bkp.Close()

	// this is a no-op on windows
	if err := chown(bkpName, info); err != nil {
		return nil, err
	}

	if _, err := io.Copy(bkp, f); err != nil {
		return nil, err
	}

	if err := f.Truncate(0); err != nil {
		return nil, err
	} else if _, err = f.Seek(0, 0); err != nil {
		return nil, err
	}

	return f, nil
}

func (l *Logger) moveCreate() (*os.File, error) {
	name := l.filename()
	bkpName := l.backupName()

	info, err := os_Stat(name)
	if err != nil {
		return nil, err
	}

	// move the existing file
	if err := os.Rename(name, bkpName); err != nil {
		return nil, fmt.Errorf("can't rename log file: %s", err)
	}

	// we use truncate here because this should only get called when we've moved
	// the file ourselves. if someone else creates the file in the meantime,
	// just wipe out the contents.
	f, err := os.OpenFile(name, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, info.Mode())
	if err != nil {
		return nil, fmt.Errorf("can't open new logfile: %s", err)
	}

	// this is a no-op on windows
	if err := chown(name, info); err != nil {
		return nil, err
	}

	return f, nil
}

// backupName creates a new filename from the given name, inserting a UTC
// timestamp between the filename and the extension.
func (l *Logger) backupName() string {
	name := l.filename()
	dir := filepath.Dir(name)
	filename := filepath.Base(name)
	ext := filepath.Ext(filename)
	prefix := filename[:len(filename)-len(ext)]
	t := currentTime().UTC()

	timestamp := t.Format(backupTimeFormat)
	return filepath.Join(dir, fmt.Sprintf("%s-%s%s", prefix, timestamp, ext))
}

// openExistingOrNew opens the logfile if it exists.
// If there is no such file or the write would
// put it over the MaxLines, a new file is created.
func (l *Logger) openExistingOrNew() error {
	filename := l.filename()
	info, err := os_Stat(filename)
	if os.IsNotExist(err) {
		return l.openNew()
	}
	if err != nil {
		return fmt.Errorf("error getting log file info: %s", err)
	}

	if info.Size()+1 > l.max() {
		return l.rotate()
	}

	file, err := os.OpenFile(filename, os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		// if we fail to open the old log file for some reason, just ignore
		// it and open a new log file.
		return l.openNew()
	}
	l.file = file
	l.lines, err = linesInFile(l.filename())
	if err != nil {
		// if we fail to count the lines in the old log file for some reason,
		// just ignore it and open a new log file.
		return l.openNew()
	}
	return nil
}

// genFilename generates the name of the logfile from the current time.
func (l *Logger) filename() string {
	if l.Filename != "" {
		return l.Filename
	}
	name := filepath.Base(os.Args[0]) + "-nanojack.log"
	return filepath.Join(os.TempDir(), name)
}

// cleanup deletes old log files, keeping at most l.MaxBackups files.
func (l *Logger) cleanup() error {
	if l.MaxBackups == 0 {
		return nil
	}

	files, err := l.oldLogFiles()
	if err != nil {
		return err
	}

	var deletes []logInfo

	if l.MaxBackups > 0 && l.MaxBackups < len(files) {
		deletes = files[l.MaxBackups:]
		files = files[:l.MaxBackups]
	}

	if len(deletes) == 0 {
		return nil
	}

	go deleteAll(l.dir(), deletes)

	return nil
}

func linesInFile(path string) (int64, error) {
	content, err := ioutil.ReadFile(path)
	if err != nil {
		return 0, err
	}
	lines := strings.FieldsFunc(string(content), func(c rune) bool { return c == '\n' })
	return int64(len(lines)), nil
}

func deleteAll(dir string, files []logInfo) {
	// remove files on a separate goroutine
	for _, f := range files {
		// what am I going to do, log this?
		_ = os.Remove(filepath.Join(dir, f.Name()))
	}
}

// oldLogFiles returns the list of backup log files stored in the same
// directory as the current log file, sorted by ModTime
func (l *Logger) oldLogFiles() ([]logInfo, error) {
	files, err := ioutil.ReadDir(l.dir())
	if err != nil {
		return nil, fmt.Errorf("can't read log file directory: %s", err)
	}
	logFiles := []logInfo{}

	prefix, ext := l.prefixAndExt()

	for _, f := range files {
		if f.IsDir() {
			continue
		}
		name := l.timeFromName(f.Name(), prefix, ext)
		if name == "" {
			continue
		}
		t, err := time.Parse(backupTimeFormat, name)
		if err == nil {
			logFiles = append(logFiles, logInfo{t, f})
		}
		// error parsing means that the suffix at the end was not generated
		// by nanojack, and therefore it's not a backup file.
	}

	sort.Sort(byFormatTime(logFiles))

	return logFiles, nil
}

// timeFromName extracts the formatted time from the filename by stripping off
// the filename's prefix and extension. This prevents someone's filename from
// confusing time.parse.
func (l *Logger) timeFromName(filename, prefix, ext string) string {
	if !strings.HasPrefix(filename, prefix) {
		return ""
	}
	filename = filename[len(prefix):]

	if !strings.HasSuffix(filename, ext) {
		return ""
	}
	filename = filename[:len(filename)-len(ext)]
	return filename
}

// max returns the maximum size in bytes of log files before rolling.
func (l *Logger) max() int64 {
	if l.MaxLines == 0 {
		return int64(defaultMaxLines)
	}
	return int64(l.MaxLines)
}

// dir returns the directory for the current filename.
func (l *Logger) dir() string {
	return filepath.Dir(l.filename())
}

// prefixAndExt returns the filename part and extension part from the Logger's
// filename.
func (l *Logger) prefixAndExt() (prefix, ext string) {
	filename := filepath.Base(l.filename())
	ext = filepath.Ext(filename)
	prefix = filename[:len(filename)-len(ext)] + "-"
	return prefix, ext
}

// logInfo is a convenience struct to return the filename and its embedded
// timestamp.
type logInfo struct {
	timestamp time.Time
	os.FileInfo
}

// byFormatTime sorts by newest time formatted in the name.
type byFormatTime []logInfo

func (b byFormatTime) Less(i, j int) bool {
	return b[i].timestamp.After(b[j].timestamp)
}

func (b byFormatTime) Swap(i, j int) {
	b[i], b[j] = b[j], b[i]
}

func (b byFormatTime) Len() int {
	return len(b)
}
