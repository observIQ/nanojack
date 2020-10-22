// +build !windows

package nanojack

import (
	"os"
	"syscall"
	"testing"
	"time"
)

func TestMaintainMode(t *testing.T) {
	t.Run("MoveCreate", testMaintainMode(t, false))
	t.Run("CopyTruncate", testMaintainMode(t, true))
}

func testMaintainMode(t *testing.T, copyTruncate bool) func(t *testing.T) {
	return func(t *testing.T) {

		currentTime = fakeTime
		dir := makeTempDir(t)
		defer os.RemoveAll(dir)

		filename := logFile(dir)

		mode := os.FileMode(0600)
		f, err := os.OpenFile(filename, os.O_CREATE|os.O_RDWR, mode)
		isNil(err, t)
		f.Close()

		l := &Logger{
			Filename:     filename,
			MaxBackups:   1,
			MaxLines:     10,
			CopyTruncate: copyTruncate,
		}
		defer l.Close()
		b := []byte("boo!")
		n, err := l.Write(b)
		isNil(err, t)
		equals(len(b), n, t)

		newFakeTime(time.Second)

		err = l.Rotate()
		isNil(err, t)

		filename2 := backupFile(dir)
		info, err := os.Stat(filename)
		isNil(err, t)
		info2, err := os.Stat(filename2)
		isNil(err, t)
		equals(mode, info.Mode(), t)
		equals(mode, info2.Mode(), t)
	}
}

func TestMaintainOwner(t *testing.T) {
	t.Run("MoveCreate", testMaintainMode(t, false))
	t.Run("CopyTruncate", testMaintainMode(t, true))
}

func testMaintainOwner(t *testing.T, copyTruncate bool) func(t *testing.T) {
	return func(t *testing.T) {
		fakeC := fakeChown{}
		os_Chown = fakeC.Set
		os_Stat = fakeStat
		defer func() {
			os_Chown = os.Chown
			os_Stat = os.Stat
		}()
		currentTime = fakeTime
		dir := makeTempDir(t)
		defer os.RemoveAll(dir)

		filename := logFile(dir)

		l := &Logger{
			Filename:     filename,
			MaxBackups:   1,
			MaxLines:     10,
			CopyTruncate: false,
		}
		defer l.Close()
		b := []byte("boo!")
		n, err := l.Write(b)
		isNil(err, t)
		equals(len(b), n, t)

		newFakeTime(time.Second)

		err = l.Rotate()
		isNil(err, t)

		equals(555, fakeC.uid, t)
		equals(666, fakeC.gid, t)
	}
}

type fakeChown struct {
	name string
	uid  int
	gid  int
}

func (f *fakeChown) Set(name string, uid, gid int) error {
	f.name = name
	f.uid = uid
	f.gid = gid
	return nil
}

func fakeStat(name string) (os.FileInfo, error) {
	info, err := os.Stat(name)
	if err != nil {
		return info, err
	}
	stat := info.Sys().(*syscall.Stat_t)
	stat.Uid = 555
	stat.Gid = 666
	return info, nil
}
