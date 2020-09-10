package nanojack

import (
	"encoding/json"
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"
	"time"

	"gopkg.in/yaml.v2"
)

// !!!NOTE!!!
//
// Running these tests in parallel will almost certainly cause sporadic (or even
// regular) failures, because they're all messing with the same global variable
// that controls the logic's mocked time.Now.  So... don't do that.

// Since all the tests uses the time to determine filenames etc, we need to
// control the wall clock as much as possible, which means having a wall clock
// that doesn't change unless we want it to.
var fakeCurrentTime = time.Now()

func fakeTime() time.Time {
	return fakeCurrentTime
}

func TestNewFile(t *testing.T) {
	currentTime = fakeTime

	dir := makeTempDir("TestNewFile", t)
	defer os.RemoveAll(dir)
	l := &Logger{
		Filename: logFile(dir),
	}
	defer l.Close()
	b := []byte("boo!\n")
	n, err := l.Write(b)
	isNil(err, t)
	equals(len(b), n, t)
	existsWithLines(logFile(dir), 1, t)
	fileCount(dir, 1, t)
}

func TestAppendExisting(t *testing.T) {
	currentTime = fakeTime
	dir := makeTempDir("TestAppendExisting", t)
	defer os.RemoveAll(dir)

	filename := logFile(dir)
	data := []byte("foo!\n")
	err := ioutil.WriteFile(filename, data, 0644)
	isNil(err, t)
	existsWithLines(filename, 1, t)

	l := &Logger{
		Filename: filename,
	}
	defer l.Close()
	b := []byte("boo!\n")
	n, err := l.Write(b)
	isNil(err, t)
	equals(len(b), n, t)

	// make sure no other files were created
	fileCount(dir, 1, t)

	// make sure the file got appended
	existsWithLines(filename, 2, t)
}

func TestMakeLogDir(t *testing.T) {
	currentTime = fakeTime
	dir := time.Now().Format("TestMakeLogDir" + backupTimeFormat)
	dir = filepath.Join(os.TempDir(), dir)
	defer os.RemoveAll(dir)
	filename := logFile(dir)
	l := &Logger{
		Filename: filename,
	}
	defer l.Close()
	b := []byte("boo!\n")
	n, err := l.Write(b)
	isNil(err, t)
	equals(len(b), n, t)
	existsWithLines(logFile(dir), 1, t)
	fileCount(dir, 1, t)
}

func TestDefaultFilename(t *testing.T) {
	currentTime = fakeTime
	dir := os.TempDir()
	filename := filepath.Join(dir, filepath.Base(os.Args[0])+"-nanojack.log")
	defer os.Remove(filename)
	l := &Logger{}
	defer l.Close()
	b := []byte("boo!\n")
	n, err := l.Write(b)

	isNil(err, t)
	equals(len(b), n, t)
	existsWithLines(filename, 1, t)
}

func TestAutoRotate(t *testing.T) {
	currentTime = fakeTime

	dir := makeTempDir("TestAutoRotate", t)
	defer os.RemoveAll(dir)

	filename := logFile(dir)
	l := &Logger{
		Filename: filename,
		MaxLines: 1,
	}
	defer l.Close()
	b := []byte("boo!\n")
	n, err := l.Write(b)
	isNil(err, t)
	equals(len(b), n, t)

	existsWithLines(filename, 1, t)
	fileCount(dir, 1, t)

	newFakeTime(time.Second)

	b2 := []byte("foooooo!\n")
	n, err = l.Write(b2)
	isNil(err, t)
	equals(len(b2), n, t)

	// the old logfile should be moved aside and the main logfile should have
	// only the last write in it.
	existsWithLines(filename, 1, t)

	// the backup file will use the current fake time and have the old contents.
	existsWithLines(backupFile(dir), 1, t)

	fileCount(dir, 2, t)
}

func TestFirstWriteRotate(t *testing.T) {
	currentTime = fakeTime
	dir := makeTempDir("TestFirstWriteRotate", t)
	defer os.RemoveAll(dir)

	filename := logFile(dir)
	l := &Logger{
		Filename: filename,
		MaxLines: 1,
	}
	defer l.Close()

	start := []byte("boooooo!\n")
	err := ioutil.WriteFile(filename, start, 0600)
	isNil(err, t)

	newFakeTime(time.Second)

	// this would make us rotate
	b := []byte("fooo!\n")
	n, err := l.Write(b)
	isNil(err, t)
	equals(len(b), n, t)

	existsWithLines(filename, 1, t)
	existsWithLines(backupFile(dir), 1, t)

	fileCount(dir, 2, t)
}

func TestMaxBackups(t *testing.T) {
	currentTime = fakeTime
	dir := makeTempDir("TestMaxBackups", t)
	defer os.RemoveAll(dir)

	filename := logFile(dir)
	l := &Logger{
		Filename:   filename,
		MaxLines:   1,
		MaxBackups: 1,
	}
	defer l.Close()
	b := []byte("boo!\n")
	n, err := l.Write(b)
	isNil(err, t)
	equals(len(b), n, t)

	existsWithLines(filename, 1, t)
	fileCount(dir, 1, t)

	newFakeTime(time.Second)

	// this will put us over the max
	b2 := []byte("foooooo!\n")
	n, err = l.Write(b2)
	isNil(err, t)
	equals(len(b2), n, t)

	// this will use the new fake time
	secondFilename := backupFile(dir)
	existsWithLines(secondFilename, 1, t)

	// make sure the old file still exists with the same size.
	existsWithLines(filename, 1, t)

	fileCount(dir, 2, t)

	newFakeTime(time.Second)

	// this will make us rotate again
	n, err = l.Write(b2)
	isNil(err, t)
	equals(len(b2), n, t)

	// this will use the new fake time
	thirdFilename := backupFile(dir)
	existsWithLines(thirdFilename, 1, t)

	existsWithLines(filename, 1, t)

	// we need to wait a little bit since the files get deleted on a different
	// goroutine.
	<-time.After(time.Millisecond * 10)

	// should only have two files in the dir still
	fileCount(dir, 2, t)

	// second file name should still exist
	existsWithLines(thirdFilename, 1, t)

	// should have deleted the first backup
	notExist(secondFilename, t)

	// now test that we don't delete directories or non-logfile files

	newFakeTime(time.Second)

	// create a file that is close to but different from the logfile name.
	// It shouldn't get caught by our deletion filters.
	notlogfile := logFile(dir) + ".foo"
	err = ioutil.WriteFile(notlogfile, []byte("data\n"), 0644)
	isNil(err, t)

	// Make a directory that exactly matches our log file filters... it still
	// shouldn't get caught by the deletion filter since it's a directory.
	notlogfiledir := backupFile(dir)
	err = os.Mkdir(notlogfiledir, 0700)
	isNil(err, t)

	newFakeTime(time.Second)

	// this will make us rotate again
	n, err = l.Write(b2)
	isNil(err, t)
	equals(len(b2), n, t)

	// this will use the new fake time
	fourthFilename := backupFile(dir)
	existsWithLines(fourthFilename, 1, t)

	// we need to wait a little bit since the files get deleted on a different
	// goroutine.
	<-time.After(time.Millisecond * 10)

	// We should have four things in the directory now - the 2 log files, the
	// not log file, and the directory
	fileCount(dir, 4, t)

	// third file name should still exist
	existsWithLines(filename, 1, t)

	existsWithLines(fourthFilename, 1, t)

	// should have deleted the first filename
	notExist(thirdFilename, t)

	// the not-a-logfile should still exist
	exists(notlogfile, t)

	// the directory
	exists(notlogfiledir, t)
}

func TestCleanupExistingBackups(t *testing.T) {
	// test that if we start with more backup files than we're supposed to have
	// in total, that extra ones get cleaned up when we rotate.

	currentTime = fakeTime

	dir := makeTempDir("TestCleanupExistingBackups", t)
	defer os.RemoveAll(dir)

	// make 3 backup files

	data := []byte("data\n")
	backup := backupFile(dir)
	err := ioutil.WriteFile(backup, data, 0644)
	isNil(err, t)

	newFakeTime(time.Second)

	backup = backupFile(dir)
	err = ioutil.WriteFile(backup, data, 0644)
	isNil(err, t)

	newFakeTime(time.Second)

	backup = backupFile(dir)
	err = ioutil.WriteFile(backup, data, 0644)
	isNil(err, t)

	// now create a primary log file with some data
	filename := logFile(dir)
	err = ioutil.WriteFile(filename, data, 0644)
	isNil(err, t)

	l := &Logger{
		Filename:   filename,
		MaxLines:   1,
		MaxBackups: 1,
	}
	defer l.Close()

	newFakeTime(time.Second)

	b2 := []byte("foooooo!\n")
	n, err := l.Write(b2)
	isNil(err, t)
	equals(len(b2), n, t)

	// we need to wait a little bit since the files get deleted on a different
	// goroutine.
	<-time.After(time.Millisecond * 10)

	// now we should only have 2 files left - the primary and one backup
	fileCount(dir, 2, t)
}

func TestOldLogFiles(t *testing.T) {
	currentTime = fakeTime

	dir := makeTempDir("TestOldLogFiles", t)
	defer os.RemoveAll(dir)

	filename := logFile(dir)
	data := []byte("data\n")
	err := ioutil.WriteFile(filename, data, 07)
	isNil(err, t)

	// This gives us a time with the same precision as the time we get from the
	// timestamp in the name.
	t1, err := time.Parse(backupTimeFormat, fakeTime().UTC().Format(backupTimeFormat))
	isNil(err, t)

	backup := backupFile(dir)
	err = ioutil.WriteFile(backup, data, 07)
	isNil(err, t)

	newFakeTime(time.Second)

	t2, err := time.Parse(backupTimeFormat, fakeTime().UTC().Format(backupTimeFormat))
	isNil(err, t)

	backup2 := backupFile(dir)
	err = ioutil.WriteFile(backup2, data, 07)
	isNil(err, t)

	l := &Logger{Filename: filename}
	files, err := l.oldLogFiles()
	isNil(err, t)
	equals(2, len(files), t)

	// should be sorted by newest file first, which would be t2
	equals(t2, files[0].timestamp, t)
	equals(t1, files[1].timestamp, t)
}

func TestTimeFromName(t *testing.T) {
	l := &Logger{Filename: "/var/log/myfoo/foo.log"}
	prefix, ext := l.prefixAndExt()
	val := l.timeFromName("foo-2014-05-04T14-44-33.555.log", prefix, ext)
	equals("2014-05-04T14-44-33.555", val, t)

	val = l.timeFromName("foo-2014-05-04T14-44-33.555", prefix, ext)
	equals("", val, t)

	val = l.timeFromName("2014-05-04T14-44-33.555.log", prefix, ext)
	equals("", val, t)

	val = l.timeFromName("foo.log", prefix, ext)
	equals("", val, t)
}

func TestRotateWithMoveCreate(t *testing.T) {
	currentTime = fakeTime
	dir := makeTempDir("TestRotateWithMoveCreate", t)
	defer os.RemoveAll(dir)

	filename := logFile(dir)

	l := &Logger{
		Filename:     filename,
		MaxBackups:   1,
		MaxLines:     10,
		CopyTruncate: false,
	}
	defer l.Close()
	b := []byte("boo!\n")
	n, err := l.Write(b)
	isNil(err, t)
	equals(len(b), n, t)

	existsWithLines(filename, 1, t)
	fileCount(dir, 1, t)

	newFakeTime(time.Second)

	err = l.Rotate()
	isNil(err, t)

	// we need to wait a little bit since the files get deleted on a different
	// goroutine.
	<-time.After(10 * time.Millisecond)

	filename2 := backupFile(dir)
	existsWithLines(filename2, 1, t)
	existsWithLines(filename, 0, t)
	fileCount(dir, 2, t)
	newFakeTime(time.Second)

	err = l.Rotate()
	isNil(err, t)

	// we need to wait a little bit since the files get deleted on a different
	// goroutine.
	<-time.After(10 * time.Millisecond)

	filename3 := backupFile(dir)
	existsWithLines(filename3, 0, t)
	existsWithLines(filename, 0, t)
	fileCount(dir, 2, t)

	b2 := []byte("foooooo!\n")
	n, err = l.Write(b2)
	isNil(err, t)
	equals(len(b2), n, t)

	// this will use the new fake time
	existsWithLines(filename, 1, t)
}

func TestRotateWithCopyTruncate(t *testing.T) {
	currentTime = fakeTime
	dir := makeTempDir("TestRotateWithCopyTruncate", t)
	defer os.RemoveAll(dir)

	filename := logFile(dir)

	l := &Logger{
		Filename:     filename,
		MaxBackups:   1,
		MaxLines:     10,
		CopyTruncate: true,
	}
	defer l.Close()
	b := []byte("boo!\n")
	n, err := l.Write(b)
	isNil(err, t)
	equals(len(b), n, t)

	existsWithLines(filename, 1, t)
	fileCount(dir, 1, t)

	newFakeTime(time.Second)

	err = l.Rotate()
	isNil(err, t)

	// we need to wait a little bit since the files get deleted on a different
	// goroutine.
	<-time.After(10 * time.Millisecond)

	filename2 := backupFile(dir)
	existsWithLines(filename2, 1, t)
	existsWithLines(filename, 0, t)
	fileCount(dir, 2, t)
	newFakeTime(time.Second)

	err = l.Rotate()
	isNil(err, t)

	// we need to wait a little bit since the files get deleted on a different
	// goroutine.
	<-time.After(10 * time.Millisecond)

	filename3 := backupFile(dir)
	existsWithLines(filename3, 0, t)
	existsWithLines(filename, 0, t)
	fileCount(dir, 2, t)

	b2 := []byte("foooooo!\n")
	n, err = l.Write(b2)
	isNil(err, t)
	equals(len(b2), n, t)

	// this will use the new fake time
	existsWithLines(filename, 1, t)
}

func TestJson(t *testing.T) {
	data := []byte(`
{
	"filename": "foo",
	"maxlines": 5,
	"maxbackups": 3,
	"copytruncate": false
}`[1:])

	l := Logger{}
	err := json.Unmarshal(data, &l)
	isNil(err, t)
	equals("foo", l.Filename, t)
	equals(5, l.MaxLines, t)
	equals(3, l.MaxBackups, t)
}

func TestYaml(t *testing.T) {
	data := []byte(`
filename: foo
maxlines: 5
maxbackups: 3
copytruncate: false`[1:])

	l := Logger{}
	err := yaml.Unmarshal(data, &l)
	isNil(err, t)
	equals("foo", l.Filename, t)
	equals(5, l.MaxLines, t)
	equals(3, l.MaxBackups, t)
}

// makeTempDir creates a file with a semi-unique name in the OS temp directory.
// It should be based on the name of the test, to keep parallel tests from
// colliding, and must be cleaned up after the test is finished.
func makeTempDir(name string, t testing.TB) string {
	dir := time.Now().Format(name + backupTimeFormat)
	dir = filepath.Join(os.TempDir(), dir)
	isNilUp(os.Mkdir(dir, 0700), t, 1)
	return dir
}

// existsWithLines checks that the given file exists and has the correct length.
func existsWithLines(path string, lines int64, t testing.TB) {
	_, err := os.Stat(path)
	isNilUp(err, t, 1)
	act, err := linesInFile(path)
	isNilUp(err, t, 1)
	equalsUp(lines, act, t, 1)
}

// logFile returns the log file name in the given directory for the current fake
// time.
func logFile(dir string) string {
	return filepath.Join(dir, "foobar.log")
}

func backupFile(dir string) string {
	return filepath.Join(dir, "foobar-"+fakeTime().UTC().Format(backupTimeFormat)+".log")
}

// fileCount checks that the number of files in the directory is exp.
func fileCount(dir string, exp int, t testing.TB) {
	files, err := ioutil.ReadDir(dir)
	isNilUp(err, t, 1)
	// Make sure no other files were created.
	equalsUp(exp, len(files), t, 1)
}

// newFakeTime adds specified wait time to the fake "current time".
func newFakeTime(wait time.Duration) {
	fakeCurrentTime = fakeCurrentTime.Add(wait)
}

func notExist(path string, t testing.TB) {
	_, err := os.Stat(path)
	assertUp(os.IsNotExist(err), t, 1, "expected to get os.IsNotExist, but instead got %v", err)
}

func exists(path string, t testing.TB) {
	_, err := os.Stat(path)
	assertUp(err == nil, t, 1, "expected file to exist, but got error from os.Stat: %v", err)
}
