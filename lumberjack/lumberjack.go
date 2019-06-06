// Package lumberjack provides a rolling logger.
//
// The package is based on lumberjack package at
// https://github.com/natefinch/lumberjack under the v2.0 branch.
//
// Lumberjack is intended to be one part of a logging infrastructure.
// It is not an all-in-one solution, but instead is a pluggable
// component at the bottom of the logging stack that simply controls the files
// to which logs are written.
//
// Lumberjack plays well with any logging package that can write to an
// io.Writer, including the standard library's log package.
//
// Lumberjack assumes that only one process is writing to the output files.
// Using the same lumberjack configuration from multiple processes on the same
// machine will result in improper behavior.
package lumberjack

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"time"
)

const (
	timeFormat          = "20060102_150405"
	defaultMaxSize      = 100
	defaultRotationTime = 5
)

// ensure we always implement io.WriteCloser
var _ io.WriteCloser = (*Logger)(nil)

// Logger is an io.WriteCloser that writes to the specified filename.
//
// Logger opens or creates the logfile with timestamp based on rotation time and
// base time 00:00:00.000 on first Write. If the file exists and is less than
// MaxSize megabytes, lumberjack will open and append that file.
// If the file exists and its size is >= MaxSize megabytes, the file is renamed
// by putting index in the name immediately before the file's extension (or the
// end of the filename if there's no extension). A symbolic link is created
// using original filename.
//
// Whenever a write would cause the current log file exceed MaxSize megabytes,
// the current file is closed, renamed, and a new log file created.
//
// Lumberjack uses the log file name given to Logger, in the form
// `name-timestamp.ext` where name is the filename without the extension,
// timestamp is the time at which the log was rotated formatted with the
// rotation time + the time.Time format of `20060102_150405` and the extension
// is the original extension. For example, if your Logger.Filename is
// `/var/log/foo/server.log`, a backup created at 6:30pm on Nov 11 2016 with
// rotation time 60 minutes would use the filename
// `/var/log/foo/server-20161104_183000.log`
//
type Logger struct {
	// Filename is the file to write logs to. Backup log files will be retained
	// in the same directory. It uses <processname>-lumberjack.log in
	// os.Tempget_dir() if empty.
	Filename string `json:"filename" yaml:"filename"`

	// RotationTime is number of minutes. Log file is rotated every
	// "RotationTime" minutes since base time 00:00:00.000.
	// Default value is 5 minutes.
	RotationTime int `json:"rotationtime" yaml:"rotationtime"`

	// MaxSize is the maximum size in megabytes of the log file before it gets
	// rotated. It defaults to 100 megabytes.
	MaxSize int `json:"maxsize" yaml:"maxsize"`

	// LocalTime determines if the time used for formatting the timestamps in
	// backup files is the computer's local time. The default is to use UTC
	// time.
	LocalTime bool `json:"localtime" yaml:"localtime"`

	size int64
	file *os.File
	mu   sync.Mutex
}

var (
	// currentTime exists so it can be mocked out by tests.
	currentTime = time.Now

	// os_Stat exists so it can be mocked out by tests.
	os_Stat = os.Stat

	// megabyte is the conversion factor between MaxSize and bytes. It is a
	// variable so tests can mock it out and not need to write megabytes of data
	// to disk.
	megabyte = 1024 * 1024
)

// Write implements io.Writer. If a write would cause the log file to be larger
// than MaxSize, the file is closed, renamed to include a timestamp of the
// current time, and a new log file is created using the original log file name.
// If the length of the write is greater than MaxSize, an error is returned.
func (l *Logger) Write(p []byte) (n int, err error) {
	l.mu.Lock()
	defer l.mu.Unlock()

	writeLen := int64(len(p))
	if writeLen > l.get_max_size() {
		return 0, fmt.Errorf(
			"write length %d exceeds maximum file size %d", writeLen, l.get_max_size(),
		)
	}

	if err = l.openExistingOrNew(len(p)); err != nil {
		return 0, err
	}

	if l.size+writeLen > l.get_max_size() {
		if err := l.rotate(); err != nil {
			return 0, err
		}
	}

	w := bufio.NewWriter(l.file)
	n, err = w.Write(p)
	l.size += int64(n)

	w.Flush()

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
// new one. This is a helper function for applications that want to initiate
// rotations outside of the normal rotation rules, such as in response to
// SIGHUP. After rotating, this initiates compression and removal of old log
// files according to the configuration.
func (l *Logger) Rotate() error {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.rotate()
}

// rotate closes the current file, moves it aside with a timestamp in the name,
// (if it exists), opens a new file with the original filename, and then runs
// post-rotation processing and removal.
func (l *Logger) rotate() error {
	if err := l.close(); err != nil {
		return err
	}
	if err := l.openNew(); err != nil {
		return err
	}
	return nil
}

// openNew opens a new log file for writing, moving any old log file out of the
// way. This methods assumes the file has already been closed.
func (l *Logger) openNew() error {
	err := os.MkdirAll(l.get_dir(), 0755)
	if err != nil {
		return fmt.Errorf("can't make directories for new logfile: %s", err)
	}

	name := l.processName(0)
	mode := os.FileMode(0600)
	_, err = os_Stat(name)
	var f *os.File
	if err == nil {
		f, err = os.OpenFile(name, os.O_APPEND|os.O_WRONLY, mode)
		if err != nil {
			return fmt.Errorf("can't open existing logfile: %s", err)
		}
	} else {
		// we use truncate here because this should only get called when we've moved
		// the file ourselves. if someone else creates the file in the meantime,
		// just wipe out the contents.
		f, err = os.OpenFile(name, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, mode)
		if err != nil {
			return fmt.Errorf("can't open new logfile: %s", err)
		}
	}

	// Remove symbolic link if it existed
	if _, err := os.Lstat(l.get_filename()); err == nil {
		if err := os.Remove(l.get_filename()); err != nil {
			return fmt.Errorf("can't unlink: %s", err)
		}
	}
	// Create symbolic link
	if err := os.Symlink(name, l.get_filename()); err != nil {
		return fmt.Errorf("can't create symbolic link to new logfile: %s", err)
	}

	l.file = f
	info, _ := f.Stat()
	l.size = info.Size()

	return nil
}

// processName creates a new filename from the given name, inserting a timestamp
// between the filename and the extension, using the local time if requested
// (otherwise UTC). If existing file will exceed size limit after writing, we'll
// choose new filename with an index.
func (l Logger) processName(write_length int) string {
	dir := filepath.Dir(l.get_filename())
	filename := filepath.Base(l.get_filename())
	ext := filepath.Ext(filename)
	prefix := filename[:len(filename)-len(ext)]
	t := currentTime()
	if !l.LocalTime {
		t = t.UTC()
	}

	timestamp := ""
	if l.get_rotation_time() > 0 {
		// Get current date
		current_year := t.Year()
		current_month := t.Month()
		current_day := t.Day()
		// Create base date time YYYY:MM:DD 00:00:00.000
		var base_datetime time.Time
		if !l.LocalTime {
			base_datetime = time.Date(current_year, current_month, current_day, 0, 0, 0, 0, time.UTC)
		} else {
			base_datetime = time.Date(current_year, current_month, current_day, 0, 0, 0, 0, time.Local)
		}
		// Find closet time based on rotation time
		rotation_datetime := base_datetime
		for rotation_datetime.Add(time.Minute * time.Duration(l.get_rotation_time())).Before(t) {
			rotation_datetime = rotation_datetime.Add(time.Minute * time.Duration(l.get_rotation_time()))
		}
		timestamp = rotation_datetime.Format(timeFormat)
	} else {
		timestamp = t.Format(timeFormat)
	}
	name := filepath.Join(dir, fmt.Sprintf("%s-%s%s", prefix, timestamp, ext))

	// If file with this name already existed and its size will exceed limit
	// after writing, we will find other suitable name
	if info, err := os_Stat(name); !os.IsNotExist(err) {
		if info.Size()+int64(write_length) > l.get_max_size() {
			counter := 2
			for true {
				name = filepath.Join(dir, fmt.Sprintf("%s-%s_%d%s", prefix, timestamp, counter, ext))
				if info, err := os_Stat(name); os.IsNotExist(err) {
					return name
				} else {
					if info.Size()+int64(write_length) > l.get_max_size() {
						counter = counter + 1
					} else {
						return name
					}
				}
			}
		}
	}

	return name
}

// openExistingOrNew opens the logfile if it exists and if the current write
// would not put it over MaxSize. If there is no such file or the write would
// put it over the MaxSize, a new file is created.
func (l *Logger) openExistingOrNew(writeLen int) error {
	name := l.processName(writeLen)
	info, err := os_Stat(name)
	if os.IsNotExist(err) {
		return l.openNew()
	}
	if err != nil {
		return fmt.Errorf("error getting log file info: %s", err)
	}

	if info.Size()+int64(writeLen) >= l.get_max_size() {
		return l.rotate()
	}

	file, err := os.OpenFile(name, os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		// if we fail to open the old log file for some reason, just ignore
		// it and open a new log file.
		return l.openNew()
	}
	l.file = file
	l.size = info.Size()
	return nil
}

// get_filename generates the name of the logfile from the current time.
func (l *Logger) get_filename() string {
	if l.Filename != "" {
		return l.Filename
	}
	name := filepath.Base(os.Args[0]) + "-lumberjack.log"
	return filepath.Join(os.TempDir(), name)
}

// get_max_size returns the maximum size in bytes of log files before rolling.
func (l *Logger) get_max_size() int64 {
	if l.MaxSize <= 0 {
		return int64(defaultMaxSize * megabyte)
	}
	return int64(l.MaxSize) * int64(megabyte)
}

// get_rotation_time returns the rotation time to roll.
func (l *Logger) get_rotation_time() int {
	if l.RotationTime <= 0 {
		return defaultRotationTime
	}
	return l.RotationTime
}

// get_dir returns the directory for the current filename.
func (l *Logger) get_dir() string {
	return filepath.Dir(l.get_filename())
}
