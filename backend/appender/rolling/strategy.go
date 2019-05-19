package rolling

import (
	"fmt"
	"os"
	"path/filepath"
	"time"
)

type strategyFactory func(FileStater) Strategy

type Strategy interface {
	Rollover(FileInfo) (syncAction, asyncAction)
}

type syncAction func(FileInfo) (*os.File, error)

type asyncAction func(FileStater, FileInfo) error

// RolloverStrategy implements the default rollover strategy.
// On rollover the timestamp is added to the log file name. For example
// the log file /path/to/file.log will be renamed to /path/to/file-2019-05-01T20:00:00.000.log.
//
// Adding timestamps simplifies the asynchronous processing of old logs. RolloverStrategy always
// rotates the current log file first. If compression is enabled, then the file will be asynchronously compressed
// in a second step.
// This process also reduces the chance of conflicts with concurrently running
// file collectors keeping log files open, basically blocking the log producing
// application on some OSes (e.g. Windows).
type RolloverStrategy struct {
	// FileName is the file name to actively write logs to.
	FileName    string
	logFileName string // file name without extension
	extension   string // log file extension

	// MaxBackups is the maximum number of old log files to retain.
	// If -1, all log files younger than MaxAge are retained.
	MaxBackups int

	// Maximum duration to retain old log files.
	MaxAge time.Duration

	// Number of compressed backup files. Compressed must be <= MaxBackups.
	// If Compressed == MaxBackups, then all backup files are compressed.
	Compressed int

	stater FileStater
}

// Build creates the rollver Strategy to be used with the rolling log file
// appender.
func (s RolloverStrategy) Build(st FileStater) Strategy {
	s.stater = st

	s.logFileName = s.FileName
	s.extension = filepath.Ext(s.FileName)
	if s.extension != "" {
		s.logFileName = s.logFileName[:len(s.logFileName)-len(s.extension)-1]
	}
	return &s
}

// Rollover creates the concrete rollover strategy to be executed by the file
// manager.
func (s *RolloverStrategy) Rollover(stat FileInfo) (syncAction, asyncAction) {
	newPath := s.rolloverName()

	sync := func(_ FileInfo) (*os.File, error) {
		flags := os.O_APPEND | os.O_WRONLY | os.O_CREATE
		if stat.Name == "" {
			return os.OpenFile(s.FileName, flags, 0666)
		}

		if s.MaxBackups == 0 {
			flags |= os.O_TRUNC
		} else {
			if err := os.Rename(s.FileName, newPath); err != nil {
				return nil, err
			}
		}

		return os.OpenFile(s.FileName, flags, 0666)
	}

	async := func(_ FileStater, _ FileInfo) error {
		panic("TODO: implement async processing")
	}

	return sync, async
}

// rolloverName creates the new log file name to be used upon rollover.
func (s *RolloverStrategy) rolloverName() string {
	ts := time.Now().Format("2006-01-02T15:04:05.000")
	path := fmt.Sprintf("%v-%v", s.logFileName, ts)
	if s.extension != "" {
		path = fmt.Sprintf("%v.%v", path, s.extension)
	}
	return path
}
