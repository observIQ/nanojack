package nanojack

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
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

// newFakeTime adds specified wait time to the fake "current time".
func newFakeTime(wait time.Duration) {
	fakeCurrentTime = fakeCurrentTime.Add(wait)
}

func TestNewFile(t *testing.T) {
	currentTime = fakeTime

	dir := makeTempDir(t)
	defer os.RemoveAll(dir)
	l := &Logger{
		Filename: logFile(dir),
	}
	defer l.Close()
	b := []byte("boo!\n")
	n, err := l.Write(b)
	require.NoError(t, err)
	require.Equal(t, len(b), n)
	existsWithLines(logFile(dir), 1, t)
	fileCount(dir, 1, t)
}

func TestAppendExisting(t *testing.T) {
	currentTime = fakeTime
	dir := makeTempDir(t)
	defer os.RemoveAll(dir)

	filename := logFile(dir)
	data := []byte("foo!\n")
	require.NoError(t, ioutil.WriteFile(filename, data, 0644))
	existsWithLines(filename, 1, t)

	l := &Logger{
		Filename: filename,
	}
	defer l.Close()
	b := []byte("boo!\n")
	n, err := l.Write(b)
	require.NoError(t, err)
	require.Equal(t, len(b), n)

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
	require.NoError(t, err)
	require.Equal(t, len(b), n)
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

	require.NoError(t, err)
	require.Equal(t, len(b), n)
	existsWithLines(filename, 1, t)
}

func TestAutoRotate(t *testing.T) {
	currentTime = fakeTime

	dir := makeTempDir(t)
	defer os.RemoveAll(dir)

	filename := logFile(dir)
	l := &Logger{
		Filename: filename,
		MaxLines: 1,
	}
	defer l.Close()
	b := []byte("boo!\n")
	n, err := l.Write(b)
	require.NoError(t, err)
	require.Equal(t, len(b), n)

	existsWithLines(filename, 1, t)
	fileCount(dir, 1, t)

	newFakeTime(time.Second)

	b2 := []byte("foooooo!\n")
	n, err = l.Write(b2)
	require.NoError(t, err)
	require.Equal(t, len(b2), n)

	// the old logfile should be moved aside and the main logfile should have
	// only the last write in it.
	existsWithLines(filename, 1, t)

	// the backup file will use the current fake time and have the old contents.
	existsWithLines(backupFile(dir), 1, t)

	fileCount(dir, 2, t)
}

func TestSequentialRotate(t *testing.T) {
	t.Run("MoveCreate", testSequentialRotate(t, false))
	t.Run("CopyTruncate", testSequentialRotate(t, true))
}

func testSequentialRotate(t *testing.T, copyTruncate bool) func(t *testing.T) {

	return func(t *testing.T) {
		dir := makeTempDir(t)
		defer os.RemoveAll(dir)

		filename := logFile(dir)
		bkpName1 := fmt.Sprintf("%s.1", filename)
		bkpName2 := fmt.Sprintf("%s.2", filename)

		l := &Logger{
			Filename:   filename,
			MaxLines:   1,
			MaxBackups: 2,
			Sequential: true,
		}
		defer l.Close()

		fileCount(dir, 0, t)

		one := []byte("1!\n")
		n, err := l.Write(one)
		require.NoError(t, err)
		require.Equal(t, len(one), n)

		fileCount(dir, 1, t)
		existsWithLines(filename, 1, t)

		two := []byte("two!\n")
		n, err = l.Write(two)
		require.NoError(t, err)
		require.Equal(t, len(two), n)

		fileCount(dir, 2, t)
		existsWithLines(filename, 1, t)
		existsWithLines(bkpName1, 1, t)

		three := []byte("three!\n")
		n, err = l.Write(three)
		require.NoError(t, err)
		require.Equal(t, len(three), n)

		fileCount(dir, 3, t)
		existsWithLines(filename, 1, t)
		existsWithLines(bkpName1, 1, t)
		existsWithLines(bkpName2, 1, t)

		four := []byte("four!\n")
		n, err = l.Write(four)
		require.NoError(t, err)
		require.Equal(t, len(four), n)

		fileCount(dir, 3, t) // still 3
		existsWithLines(filename, 1, t)
		existsWithLines(bkpName1, 1, t)
		existsWithLines(bkpName2, 1, t)
	}
}

func TestFirstWriteRotate(t *testing.T) {
	currentTime = fakeTime
	dir := makeTempDir(t)
	defer os.RemoveAll(dir)

	filename := logFile(dir)
	l := &Logger{
		Filename: filename,
		MaxLines: 1,
	}
	defer l.Close()

	start := []byte("boooooo!\n")
	require.NoError(t, ioutil.WriteFile(filename, start, 0600))

	newFakeTime(time.Second)

	// this would make us rotate
	b := []byte("fooo!\n")
	n, err := l.Write(b)
	require.NoError(t, err)
	require.Equal(t, len(b), n)

	existsWithLines(filename, 1, t)
	existsWithLines(backupFile(dir), 1, t)

	fileCount(dir, 2, t)
}

func TestMaxBackups(t *testing.T) {
	currentTime = fakeTime
	dir := makeTempDir(t)
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
	require.NoError(t, err)
	require.Equal(t, len(b), n)

	existsWithLines(filename, 1, t)
	fileCount(dir, 1, t)

	newFakeTime(time.Second)

	// this will put us over the max
	b2 := []byte("foooooo!\n")
	n, err = l.Write(b2)
	require.NoError(t, err)
	require.Equal(t, len(b2), n)

	// this will use the new fake time
	secondFilename := backupFile(dir)
	existsWithLines(secondFilename, 1, t)

	// make sure the old file still exists with the same size.
	existsWithLines(filename, 1, t)

	fileCount(dir, 2, t)

	newFakeTime(time.Second)

	// this will make us rotate again
	n, err = l.Write(b2)
	require.NoError(t, err)
	require.Equal(t, len(b2), n)

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
	require.NoError(t, ioutil.WriteFile(notlogfile, []byte("data\n"), 0644))

	// Make a directory that exactly matches our log file filters... it still
	// shouldn't get caught by the deletion filter since it's a directory.
	notlogfiledir := backupFile(dir)
	require.NoError(t, os.Mkdir(notlogfiledir, 0700))

	newFakeTime(time.Second)

	// this will make us rotate again
	n, err = l.Write(b2)
	require.NoError(t, err)
	require.Equal(t, len(b2), n)

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

	dir := makeTempDir(t)
	defer os.RemoveAll(dir)

	// make 3 backup files

	data := []byte("data\n")
	backup := backupFile(dir)
	require.NoError(t, ioutil.WriteFile(backup, data, 0644))

	newFakeTime(time.Second)

	backup = backupFile(dir)
	require.NoError(t, ioutil.WriteFile(backup, data, 0644))

	newFakeTime(time.Second)

	backup = backupFile(dir)
	require.NoError(t, ioutil.WriteFile(backup, data, 0644))

	// now create a primary log file with some data
	filename := logFile(dir)
	require.NoError(t, ioutil.WriteFile(filename, data, 0644))

	l := &Logger{
		Filename:   filename,
		MaxLines:   1,
		MaxBackups: 1,
	}
	defer l.Close()

	newFakeTime(time.Second)

	b2 := []byte("foooooo!\n")
	n, err := l.Write(b2)
	require.NoError(t, err)
	require.Equal(t, len(b2), n)

	// we need to wait a little bit since the files get deleted on a different
	// goroutine.
	<-time.After(time.Millisecond * 10)

	// now we should only have 2 files left - the primary and one backup
	fileCount(dir, 2, t)
}

func TestOldLogFiles(t *testing.T) {
	currentTime = fakeTime

	dir := makeTempDir(t)
	defer os.RemoveAll(dir)

	filename := logFile(dir)
	data := []byte("data\n")
	require.NoError(t, ioutil.WriteFile(filename, data, 07))

	// This gives us a time with the same precision as the time we get from the
	// timestamp in the name.
	t1, err := time.Parse(backupTimeFormat, fakeTime().UTC().Format(backupTimeFormat))
	require.NoError(t, err)

	backup := backupFile(dir)
	require.NoError(t, ioutil.WriteFile(backup, data, 07))

	newFakeTime(time.Second)

	t2, err := time.Parse(backupTimeFormat, fakeTime().UTC().Format(backupTimeFormat))
	require.NoError(t, err)

	backup2 := backupFile(dir)
	require.NoError(t, ioutil.WriteFile(backup2, data, 07))

	l := &Logger{Filename: filename}
	files, err := l.oldLogFiles()
	require.NoError(t, err)
	require.Equal(t, 2, len(files))

	// should be sorted by newest file first, which would be t2
	require.Equal(t, t2, files[0].timestamp)
	require.Equal(t, t1, files[1].timestamp)
}

func TestTimeFromName(t *testing.T) {
	l := &Logger{Filename: "/var/log/myfoo/foo.log"}
	prefix, ext := l.prefixAndExt()
	require.Equal(t, "2014-05-04T14-44-33.555", l.timeFromName("foo-2014-05-04T14-44-33.555.log", prefix, ext))
	require.Equal(t, "", l.timeFromName("foo-2014-05-04T14-44-33.555", prefix, ext))
	require.Equal(t, "", l.timeFromName("2014-05-04T14-44-33.555.log", prefix, ext))
	require.Equal(t, "", l.timeFromName("foo.log", prefix, ext))
}

func TestRotate(t *testing.T) {
	t.Run("MoveCreate", testRotate(t, false))
	t.Run("CopyTruncate", testRotate(t, true))
}

func testRotate(t *testing.T, copyTruncate bool) func(t *testing.T) {
	return func(t *testing.T) {
		currentTime = fakeTime
		dir := makeTempDir(t)
		defer os.RemoveAll(dir)

		filename := logFile(dir)

		l := &Logger{
			Filename:     filename,
			MaxBackups:   1,
			MaxLines:     10,
			CopyTruncate: copyTruncate,
		}
		defer l.Close()
		b := []byte("boo!\n")
		n, err := l.Write(b)
		require.NoError(t, err)
		require.Equal(t, len(b), n)

		existsWithLines(filename, 1, t)
		fileCount(dir, 1, t)

		newFakeTime(time.Second)

		require.NoError(t, l.Rotate())

		// we need to wait a little bit since the files get deleted on a different
		// goroutine.
		<-time.After(10 * time.Millisecond)

		filename2 := backupFile(dir)
		existsWithLines(filename2, 1, t)
		existsWithLines(filename, 0, t)
		fileCount(dir, 2, t)
		newFakeTime(time.Second)

		require.NoError(t, l.Rotate())

		// we need to wait a little bit since the files get deleted on a different
		// goroutine.
		<-time.After(10 * time.Millisecond)

		filename3 := backupFile(dir)
		existsWithLines(filename3, 0, t)
		existsWithLines(filename, 0, t)
		fileCount(dir, 2, t)

		b2 := []byte("foooooo!\n")
		n, err = l.Write(b2)
		require.NoError(t, err)
		require.Equal(t, len(b2), n)

		// this will use the new fake time
		existsWithLines(filename, 1, t)
	}
}

func TestJson(t *testing.T) {
	data := []byte(`
{
	"filename": "foo",
	"maxlines": 5,
	"maxbackups": 3,
	"copytruncate": true,
	"sequential": true
}`[1:])

	l := Logger{}
	require.NoError(t, json.Unmarshal(data, &l))
	require.Equal(t, "foo", l.Filename)
	require.Equal(t, 5, l.MaxLines)
	require.Equal(t, 3, l.MaxBackups)
	require.True(t, l.CopyTruncate)
	require.True(t, l.Sequential)
}

func TestYaml(t *testing.T) {
	data := []byte(`
filename: foo
maxlines: 5
maxbackups: 3
copytruncate: true
sequential: true`[1:])

	l := Logger{}
	require.NoError(t, yaml.Unmarshal(data, &l))
	require.Equal(t, "foo", l.Filename)
	require.Equal(t, 5, l.MaxLines)
	require.Equal(t, 3, l.MaxBackups)
	require.True(t, l.CopyTruncate)
	require.True(t, l.Sequential)
}

// makeTempDir creates a file with a semi-unique name in the OS temp directory.
// It should be based on the name of the test, to keep parallel tests from
// colliding, and must be cleaned up after the test is finished.
func makeTempDir(t testing.TB) string {
	name := strings.ReplaceAll(t.Name(), "/", "")
	dir := time.Now().Format(name + backupTimeFormat)
	dir = filepath.Join(os.TempDir(), dir)
	require.NoError(t, os.Mkdir(dir, 0700))
	return dir
}

// existsWithLines checks that the given file exists and has the correct length.
func existsWithLines(path string, expected int64, t testing.TB) {
	_, err := os.Stat(path)
	require.NoError(t, err)
	act, err := linesInFile(path)
	require.NoError(t, err)
	require.Equal(t, expected, act)
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
func fileCount(dir string, expected int, t testing.TB) {
	files, err := ioutil.ReadDir(dir)
	require.NoError(t, err)
	// Make sure no other files were created.
	require.Equal(t, expected, len(files))
}

func notExist(path string, t testing.TB) {
	_, err := os.Stat(path)
	require.True(t, os.IsNotExist(err), fmt.Sprintf("expected to get os.IsNotExist, but instead got %v", err))
}

func exists(path string, t testing.TB) {
	_, err := os.Stat(path)
	require.NoError(t, err, fmt.Sprintf("expected file to exist, but got error from os.Stat: %v", err))
}
