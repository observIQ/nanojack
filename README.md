This project was forked from `gopkg.in/natefinch/lumberjack`

### Nanojack is a Go package for writing logs to rolling files.

Package nanojack provides a rolling logger that can be precisely controlled for testing purposes.

Nanojack is intended to be one part of a logging infrastructure.
It is not an all-in-one solution, but instead is a pluggable
component at the bottom of the logging stack that simply controls the files
to which logs are written.

Nanojack plays well with any logging package that can write to an
io.Writer, including the standard library's log package.

Nanojack assumes that only one process is writing to the output files.
Using the same nanojack configuration from multiple processes on the same
machine will result in improper behavior.


**Example**

To use nanojack with the standard library's log package, just pass it into the SetOutput function when your application starts.

Code:

```go
log.SetOutput(&nanojack.Logger{
    Filename:   "/var/log/myapp/foo.log",
    MaxLines:  5,
    MaxBackups: 3,
    CopyTruncate: false,
    Sequential: false,
})
```



## type Logger
``` go
type Logger struct {
    // Filename is the file to write logs to.  Backup log files will be retained
    // in the same directory.  It uses <processname>-nanojack.log in
    // os.TempDir() if empty.
    Filename string `json:"filename" yaml:"filename"`

    // MaxLines is the maximum lines written to the log file before it gets rotated.
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
    
    // Sequential defines whether backups are renamed by
    // timestamp (example-2020-10-20T15-04-05.000000000.log) or
    // by simple integer (example.log.1)
    Sequential bool `json:"sequential" yaml:"sequential"`
}
```
Logger is an io.WriteCloser that writes to the specified filename.

Logger opens or creates the logfile on first Write.  If the file exists, 
nanojack will rotate that file and open a new one.

Whenever a write would cause the current log file exceed MaxLines,
the current file is closed, renamed, and a new log file created with the
original name. Thus, the filename you give Logger is always the "current" log
file.

Backup file names are derived from the log file name given to Logger, depending 
on the configuration of the logger. If sequential backups are used, then backup
file names are of the form `name.ext.N`, where `N` indicates the backup number.
If the logger is not configured to use sequential backups, then backup files are
named using the form `name-timestamp.ext` where name is the filename without the 
extension, timestamp is the time at which the log was rotated formatted with the 
time.Time format of `2006-01-02T15-04-05.000000000` and the extension is the 
original extension.  For example, if your Logger.Filename is 
`/var/log/foo/server.log`, a backup created at 6:30pm on Nov 11 2016 would use 
the filename `/var/log/foo/server-2016-11-04T18-30-00.000000000.log`

### Cleaning Up Old Log Files
Whenever a new logfile gets created, old log files may be deleted.  The most
recent files according to the encoded timestamp will be retained, up to a
number equal to MaxBackups (or all of them if MaxBackups is 0). If MaxBackups 
is 0, no old log files will be deleted.











### func (\*Logger) Close
``` go
func (l *Logger) Close() error
```
Close implements io.Closer, and closes the current logfile.



### func (\*Logger) Rotate
``` go
func (l *Logger) Rotate() error
```
Rotate causes Logger to close the existing log file and immediately create a
new one.  This is a helper function for applications that want to initiate
rotations outside of the normal rotation rules, such as in response to
SIGHUP.  After rotating, this initiates a cleanup of old log files according
to the normal rules.

**Example**

Example of how to rotate in response to SIGHUP.

Code:

```go
l := &nanojack.Logger{}
log.SetOutput(l)
c := make(chan os.Signal, 1)
signal.Notify(c, syscall.SIGHUP)

go func() {
    for {
        <-c
        l.Rotate()
    }
}()
```

### func (\*Logger) Write
``` go
func (l *Logger) Write(p []byte) (n int, err error)
```
Write implements io.Writer.  If a write would cause the log file to be larger
than MaxLines, the file is closed, renamed to include a timestamp of the
current time, and a new log file is created using the original log file name.

