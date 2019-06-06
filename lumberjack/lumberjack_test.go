package lumberjack

import (
	"bytes"
	// "compress/gzip"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"
	"time"

	// "github.com/BurntSushi/toml"
	"github.com/spf13/viper"
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
var fakeCurrentTime = time.Date(2009, 11, 17, 20, 34, 58, 651387237, time.UTC)

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
	b := []byte("boo!")
	n, err := l.Write(b)
	isNil(err, t)
	equals(len(b), n, t)
	existsWithContent(logFile(dir), b, t)
	fileCount(dir, 2, t) // Expect 2 because there is symbolic link
}

func TestOpenExisting(t *testing.T) {
	currentTime = fakeTime
	dir := makeTempDir("TestOpenExisting", t)
	defer os.RemoveAll(dir)

	filename := logFile(dir)
	l := &Logger{
		Filename: filename,
	}
	data := []byte("foo!")
	n, err := l.Write(data)
	isNil(err, t)
	equals(len(data), n, t)
	existsWithContent(filename, data, t)
	l.Close()

	b := []byte("boo!")
	n, err = l.Write(b)
	isNil(err, t)
	equals(len(b), n, t)

	// make sure the file got appended
	existsWithContent(filename, append(data, b...), t)

	// make sure no other files were created
	fileCount(dir, 2, t) // Expect 2 because there is symbolic link
}

func TestWriteTooLong(t *testing.T) {
	currentTime = fakeTime
	megabyte = 1
	dir := makeTempDir("TestWriteTooLong", t)
	defer os.RemoveAll(dir)
	l := &Logger{
		Filename: logFile(dir),
		MaxSize:  5,
	}
	defer l.Close()
	b := []byte("booooooooooooooo!")
	n, err := l.Write(b)
	notNil(err, t)
	equals(0, n, t)
	equals(err.Error(),
		fmt.Sprintf("write length %d exceeds maximum file size %d", len(b), l.MaxSize), t)
	_, err = os.Stat(logFile(dir))
	assert(os.IsNotExist(err), t, "File exists, but should not have been created")
}

func TestMakeLogDir(t *testing.T) {
	currentTime = fakeTime
	dir := time.Now().Format("TestMakeLogDir" + timeFormat)
	dir = filepath.Join(os.TempDir(), dir)
	defer os.RemoveAll(dir)
	filename := logFile(dir)
	l := &Logger{
		Filename: filename,
	}
	defer l.Close()
	b := []byte("boo!")
	n, err := l.Write(b)
	isNil(err, t)
	equals(len(b), n, t)
	existsWithContent(logFile(dir), b, t)
	fileCount(dir, 2, t) // Expect 2 because there is symbolic link
}

func TestAutoRotate(t *testing.T) {
	currentTime = fakeTime
	megabyte = 1

	dir := makeTempDir("TestAutoRotate", t)
	defer os.RemoveAll(dir)

	filename := logFile(dir)
	l := &Logger{
		Filename: filename,
		MaxSize:  10,
	}
	defer l.Close()
	b := []byte("boo!")
	n, err := l.Write(b)
	isNil(err, t)
	equals(len(b), n, t)

	existsWithContent(filename, b, t)
	fileCount(dir, 2, t) // Expect 2 because there is symbolic link

	newFakeTime()

	b2 := []byte("foooooo!")
	n, err = l.Write(b2)
	isNil(err, t)
	equals(len(b2), n, t)

	existsWithContent(filename, b2, t)
	existsWithContent(backupFile(dir), b2, t)

	fileCount(dir, 3, t) // Expect 3 because there is symbolic link

	fakeCurrentTime = time.Date(2009, 11, 17, 20, 34, 58, 651387237, time.UTC)
}

func TestFirstWriteRotate(t *testing.T) {
	currentTime = fakeTime
	megabyte = 1
	dir := makeTempDir("TestFirstWriteRotate", t)
	defer os.RemoveAll(dir)

	filename := logFile(dir)
	l := &Logger{
		Filename: filename,
		MaxSize:  10,
	}
	defer l.Close()

	start := []byte("boooooo!")
	err := ioutil.WriteFile(filepath.Join(dir, "foobar-20091117_203000.log"), start, 0600)
	isNil(err, t)

	newFakeTime()

	// this would make us rotate
	b := []byte("fooo!")
	n, err := l.Write(b)
	isNil(err, t)
	equals(len(b), n, t)

	existsWithContent(filename, b, t)
	existsWithContent(backupFile(dir), b, t)

	fileCount(dir, 3, t) // Expect 3 because there is symbolic link

	fakeCurrentTime = time.Date(2009, 11, 17, 20, 34, 58, 651387237, time.UTC)
}

func TestLocalTime(t *testing.T) {
	currentTime = fakeTime
	megabyte = 1

	dir := makeTempDir("TestLocalTime", t)
	defer os.RemoveAll(dir)

	l := &Logger{
		Filename:  logFile(dir),
		MaxSize:   10,
		LocalTime: true,
	}
	defer l.Close()
	b := []byte("boo!")
	n, err := l.Write(b)
	isNil(err, t)
	equals(len(b), n, t)

	b2 := []byte("fooooooo!")
	n2, err := l.Write(b2)
	isNil(err, t)
	equals(len(b2), n2, t)

	existsWithContent(logFile(dir), append(b, b2...), t)
}

func TestRotate(t *testing.T) {
	currentTime = fakeTime
	dir := makeTempDir("TestRotate", t)
	defer os.RemoveAll(dir)

	filename := logFile(dir)

	l := &Logger{
		Filename: filename,
		MaxSize:  100, // megabytes
	}
	defer l.Close()
	b := []byte("boo!")
	n, err := l.Write(b)
	isNil(err, t)
	equals(len(b), n, t)

	existsWithContent(filename, b, t)
	fileCount(dir, 2, t) // Expect 2 because there is symbolic link

	newFakeTime()

	err = l.Rotate()
	isNil(err, t)

	// we need to wait a little bit since the files get deleted on a different
	// goroutine.
	<-time.After(10 * time.Millisecond)

	filename2 := backupFile(dir)
	existsWithContent(filepath.Join(dir, "foobar-20091117_203000.log"), b, t)
	existsWithContent(filename2, []byte{}, t)
	fileCount(dir, 3, t)
	newFakeTime()

	err = l.Rotate()
	isNil(err, t)

	// we need to wait a little bit since the files get deleted on a different
	// goroutine.
	<-time.After(10 * time.Millisecond)

	filename3 := backupFile(dir)
	existsWithContent(filename3, []byte{}, t)
	existsWithContent(filename, []byte{}, t)
	fileCount(dir, 4, t)

	b2 := []byte("foooooo!")
	n, err = l.Write(b2)
	isNil(err, t)
	equals(len(b2), n, t)

	// this will use the new fake time
	existsWithContent(filename, b2, t)
}

func TestJson(t *testing.T) {
	data := []byte(`
{
	"filename": "foo",
	"maxsize": 5,
	"localtime": true
}`[1:])

	l := Logger{}
	err := json.Unmarshal(data, &l)
	isNil(err, t)
	equals("foo", l.Filename, t)
	equals(5, l.MaxSize, t)
	equals(true, l.LocalTime, t)
}

func TestYaml(t *testing.T) {
	data := []byte(`
filename: foo
maxsize: 5
localtime: true
`[1:])

	l := Logger{}
	err := yaml.Unmarshal(data, &l)
	isNil(err, t)
	equals("foo", l.Filename, t)
	equals(5, l.MaxSize, t)
	equals(true, l.LocalTime, t)
}

func TestToml(t *testing.T) {
	data := []byte(`
filename = "foo"
maxsize = 5
localtime = true
`[1:])

	l := Logger{}
	viper.SetConfigType("toml")
	viper.ReadConfig(bytes.NewBuffer(data))
	err := viper.Unmarshal(&l)
	isNil(err, t)
	equals("foo", l.Filename, t)
	equals(5, l.MaxSize, t)
	equals(true, l.LocalTime, t)
}

// makeTempDir creates a file with a semi-unique name in the OS temp directory.
// It should be based on the name of the test, to keep parallel tests from
// colliding, and must be cleaned up after the test is finished.
func makeTempDir(name string, t testing.TB) string {
	dir := time.Now().Format(name + timeFormat)
	dir = filepath.Join(os.TempDir(), dir)
	isNilUp(os.Mkdir(dir, 0700), t, 1)
	return dir
}

// existsWithContent checks that the given file exists and has the correct content.
func existsWithContent(path string, content []byte, t testing.TB) {
	info, err := os.Stat(path)
	isNilUp(err, t, 1)
	equalsUp(int64(len(content)), info.Size(), t, 1)

	b, err := ioutil.ReadFile(path)
	isNilUp(err, t, 1)
	equalsUp(content, b, t, 1)
}

// logFile returns the log file name in the given directory for the current fake
// time.
func logFile(dir string) string {
	return filepath.Join(dir, "foobar.log")
}

func backupFile(dir string) string {
	// return filepath.Join(dir, "foobar-"+fakeTime().UTC().Format(timeFormat)+".log")
	return filepath.Join(dir, "foobar-20091119_203000.log")
}

func backupFileLocal(dir string) string {
	return filepath.Join(dir, "foobar-"+fakeTime().Format(timeFormat)+".log")
}

// logFileLocal returns the log file name in the given directory for the current
// fake time using the local timezone.
func logFileLocal(dir string) string {
	return filepath.Join(dir, fakeTime().Format(timeFormat))
}

// fileCount checks that the number of files in the directory is exp.
func fileCount(dir string, exp int, t testing.TB) {
	files, err := ioutil.ReadDir(dir)
	isNilUp(err, t, 1)
	// Make sure no other files were created.
	equalsUp(exp, len(files), t, 1)
}

// newFakeTime sets the fake "current time" to two days later.
func newFakeTime() {
	fakeCurrentTime = fakeTime().Add(time.Hour * 24 * 2)
}

func notExist(path string, t testing.TB) {
	_, err := os.Stat(path)
	assertUp(os.IsNotExist(err), t, 1, "expected to get os.IsNotExist, but instead got %v", err)
}

func exists(path string, t testing.TB) {
	_, err := os.Stat(path)
	assertUp(err == nil, t, 1, "expected file to exist, but got error from os.Stat: %v", err)
}
