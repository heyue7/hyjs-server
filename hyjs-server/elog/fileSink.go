package elog

import (
	"errors"
	"fmt"
	"hyjs-server/config"
	"hyjs-server/constUtil"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"gopkg.in/natefinch/lumberjack.v2"
)

// fileSink serializes timer rotation and writes through the same owning writer.
// External truncation/renames would race its file descriptor and byte counter.
type fileSink struct {
	mu           sync.Mutex
	writer       *lumberjack.Logger
	nextRotation time.Time
}

func newFileSink(cfg config.LogConfig, now time.Time) (*fileSink, error) {
	if strings.TrimSpace(cfg.LogFile) == "" {
		return nil, errors.New("application log filename required")
	}
	if cfg.MaxSize <= 0 {
		cfg.MaxSize = constUtil.LogDefaultMaxSizeMB
	}
	if cfg.MaxBackups <= 0 {
		cfg.MaxBackups = constUtil.LogDefaultMaxBackups
	}
	// Lumberjack zero values mean unlimited retention; private legacy settings
	// cannot extend the project's one-day maximum.
	cfg.MaxAge = constUtil.LogMaxAgeDays
	if err := os.MkdirAll(filepath.Dir(cfg.LogFile), 0o750); err != nil {
		return nil, err
	}
	sink := &fileSink{writer: &lumberjack.Logger{
		Filename: cfg.LogFile, MaxSize: cfg.MaxSize, MaxBackups: cfg.MaxBackups,
		MaxAge: cfg.MaxAge, Compress: false,
	}, nextRotation: now.Add(constUtil.LogRotationInterval)}
	// Establish an age boundary after restart, including an old, small active file.
	if info, err := os.Lstat(cfg.LogFile); err == nil {
		if !info.Mode().IsRegular() {
			return nil, errors.New("application log must be a regular file")
		}
		if info.Size() > 0 {
			if err = sink.writer.Rotate(); err != nil {
				return nil, err
			}
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	if err := sink.removeExpired(now); err != nil {
		return nil, err
	}
	return sink, nil
}

func (s *fileSink) Write(value []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.writer.Write(value)
}

func (s *fileSink) maintain(now time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	rotate := !now.Before(s.nextRotation)
	if info, err := os.Lstat(s.writer.Filename); err == nil {
		if !info.Mode().IsRegular() {
			return errors.New("application log must be a regular file")
		}
		created, err := logFileCreatedAt(s.writer.Filename)
		if err != nil {
			return err
		}
		rotate = rotate || !created.After(now.Add(-24*time.Hour))
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if rotate {
		if err := s.writer.Rotate(); err != nil {
			return err
		}
		s.nextRotation = now.Add(constUtil.LogRotationInterval)
	}
	return s.removeExpired(now)
}

func (s *fileSink) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.writer.Close()
}

// removeExpired deletes only regular files matching this writer's lumberjack
// archive names. A rename preserves the inode's creation timestamp; compressed
// legacy files use their own creation time as a conservative boundary.
func (s *fileSink) removeExpired(now time.Time) error {
	dir := filepath.Dir(s.writer.Filename)
	base := filepath.Base(s.writer.Filename)
	ext := filepath.Ext(base)
	prefix := strings.TrimSuffix(base, ext) + "-"
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		name := entry.Name()
		if !strings.HasPrefix(name, prefix) {
			continue
		}
		stem := strings.TrimPrefix(name, prefix)
		stem = strings.TrimSuffix(stem, ".gz")
		if !strings.HasSuffix(stem, ext) {
			continue
		}
		stamp := strings.TrimSuffix(stem, ext)
		if _, err := time.Parse("2006-01-02T15-04-05.000", stamp); err != nil {
			continue
		}
		path := filepath.Join(dir, name)
		info, err := os.Lstat(path)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			continue
		}
		created, err := logFileCreatedAt(path)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return fmt.Errorf("archive creation time: %w", err)
		}
		if !created.After(now.Add(-24 * time.Hour)) {
			if err = os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
				return err
			}
		}
	}
	return nil
}
