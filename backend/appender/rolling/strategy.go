// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//        http://www.apache.org/licenses/LICENSE-2.0

package rolling

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/urso/sderr"
)

type strategyFactory func(*Background, FileStater) Strategy

type Strategy interface {
	Rotate(FileInfo) (syncAction, asyncAction)
}

type syncAction func(FileInfo) (*os.File, error)

type asyncAction func(FileStater, FileInfo) error

// RotateStrategy implements the default rollover strategy.
// On rollover the timestamp is added to the log file name. For example
// the log file /path/to/file.log will be renamed to /path/to/file_2019_05_01T20_00_00.000.log.
//
// Adding timestamps simplifies the asynchronous processing of old logs. RotateStrategy always
// rotates the current log file first. If compression is enabled, then the file will be asynchronously compressed
// in a second step.
// This process also reduces the chance of conflicts with concurrently running
// file collectors keeping log files open, basically blocking the log producing
// application on some OSes (e.g. Windows).
type RotateStrategy struct {
	// FileName is the file name to actively write logs to.
	FileName    string
	logFileName string // file name without extension
	extension   string // log file extension

	// Permission sets the default file permissions.
	Permission os.FileMode

	// MaxBackups is the maximum number of old log files to retain.
	// If -1, all log files younger than MaxAge are retained.
	MaxBackups int

	// Maximum duration to retain old log files.
	MaxAge time.Duration

	// Number of compressed backup files. Compressed must be <= MaxBackups.
	// If Compressed == MaxBackups, then all backup files are compressed.
	Compressed int

	Compression Compression

	stater     FileStater
	background *Background
}

type backupFileInfo struct {
	path       string
	timestamp  time.Time
	compressed bool
}

const timestampFormat = "2006_01_02T15_04_05.000000"

// Build creates the rollver Strategy to be used with the rolling log file
// appender.
func (s RotateStrategy) Build(b *Background, st FileStater) Strategy {
	s.stater = st
	s.background = b

	if s.Compression == nil {
		s.Compressed = 0
	}

	s.logFileName = s.FileName
	s.extension = filepath.Ext(s.FileName)
	if s.extension != "" {
		s.logFileName = s.logFileName[:len(s.logFileName)-len(s.extension)]
		s.extension = extNorm(s.extension)
	}

	if s.Permission == 0 {
		s.Permission = 0600
	}

	return &s
}

// Rotate creates the concrete rotation strategy to be executed by the file
// manager.
func (s *RotateStrategy) Rotate(stat FileInfo) (syncAction, asyncAction) {
	if s.MaxBackups < 0 && s.MaxAge == 0 && s.Compressed == 0 {
		// Note: A config with MaxAge == 0 never deletes old files.
		//       This strategy does not keep track of old files during rotation,
		//       meaning that it is safe to use external tools to delete old
		//       log files.
		return s.syncStep, nil
	}
	return s.syncStep, s.asyncStep
}

func (s *RotateStrategy) syncStep(stat FileInfo) (*os.File, error) {
	newPath := s.rolloverName()

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

func (s *RotateStrategy) asyncStep(_ FileStater, _ FileInfo) error {
	backups, err := s.oldLogs()
	if err != nil {
		return sderr.Wrap(err, "failed to query old files")
	}

	backups, err = s.removeOld(backups)
	if err != nil {
		return sderr.Wrap(err, "failed to remove old files")
	}

	uncompressed := s.MaxBackups - s.Compressed
	if uncompressed >= len(backups) { // keep all files
		return nil
	}

	var wg sync.WaitGroup
	defer wg.Wait()

	mustCompressed := backups[:len(backups)-uncompressed]
	ext := s.compressedExtension()
	for _, info := range mustCompressed {
		if info.compressed {
			continue
		}

		path := info.path
		compressedPath := info.path + ext

		// start concurrent compression writer. Normally only one should be active,
		// but if errors occured in the past, or if the application has been
		// restarted, then we have to compress some more files.  If a compressed
		// file exists already, then we assume that the rotation was incomplete,
		// and truncate it on open.
		wg.Add(1)
		go func() {
			defer wg.Done()

			// TODO: we should report this error somehow
			s.compressLog(path, compressedPath)
		}()
	}

	return nil
}

// compressLog compresses the log file, and removes the original log file upon
// success.  If a close signal is passed in the background, then the files will
// be closed immediately, and the compressed file wil be left in an incomplete
// state. Upon next rotation we will clean this up.
func (s *RotateStrategy) compressLog(path, compressedPath string) error {
	fin, err := os.Open(path)
	if err != nil {
		return err
	}
	finCloser := newFileCloser(s.background, fin)
	defer finCloser.Done()

	fout, err := os.OpenFile(compressedPath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, s.Permission)
	if err != nil {
		return err
	}
	foutClose := newFileCloser(s.background, fout)
	defer foutClose.Done()

	if err := s.Compression.Compress(fin, fout); err != nil {
		return err
	}
	return os.Remove(path)
}

func (s *RotateStrategy) removeOld(backups []backupFileInfo) ([]backupFileInfo, error) {
	del := 0 // number of files to be removed from the backup list
	if s.MaxBackups >= 0 && s.MaxBackups < len(backups) {
		del = len(backups) - s.MaxBackups
	}
	if s.MaxAge > 0 {
		for ; del < len(backups); del++ {
			if time.Since(backups[del].timestamp) < s.MaxAge {
				break
			}
		}
	}

	for _, info := range backups[:del] {
		if err := os.Remove(info.path); err != nil {
			return nil, err
		}
	}
	return backups[del:], nil
}

func (s *RotateStrategy) oldLogs() ([]backupFileInfo, error) {
	ext := s.fileExtension()
	extCompressed := s.compressedExtension()

	files, err := filepath.Glob(fmt.Sprintf("%v_*", s.logFileName))
	if err != nil {
		return nil, err
	}

	backups := make([]backupFileInfo, 0, len(files))
	for _, path := range files {
		fullPath := path

		if !strings.HasPrefix(path, s.logFileName) {
			continue
		}
		path = path[len(s.logFileName)+1:] // remove <filename>_ from path

		compressed := extCompressed != "" && strings.HasSuffix(path, extCompressed)
		if compressed {
			path = path[:len(path)-len(extCompressed)] // remove filename extension for compressed files
		}

		if ext != "" && !strings.HasSuffix(path, ext) {
			continue
		}
		path = path[:len(path)-len(ext)]
		ts, err := time.Parse(timestampFormat, path)
		if err != nil {
			continue
		}

		backups = append(backups, backupFileInfo{
			path:       fullPath,
			timestamp:  ts,
			compressed: compressed,
		})
	}

	sort.SliceStable(backups, func(i, j int) bool {
		return backups[i].timestamp.Before(backups[j].timestamp)
	})

	return backups, nil
}

// rolloverName creates the new log file name to be used upon rollover.
func (s *RotateStrategy) rolloverName() string {
	ts := time.Now().Format(timestampFormat)
	path := fmt.Sprintf("%v_%v", s.logFileName, ts)
	if s.extension != "" {
		path += s.extension
	}
	return path
}

func (s *RotateStrategy) fileExtension() string {
	return s.extension
}

func (s *RotateStrategy) compressedExtension() string {
	if s.Compression == nil {
		return ""
	}
	return extNorm(s.Compression.Extension())
}

func extNorm(ext string) string {
	if ext != "" && ext[0] != '.' {
		return "." + ext
	}
	return ext
}

func fileExists(path string) bool {
	s, err := os.Stat(path)
	return err == nil && s.Mode().IsRegular()
}
