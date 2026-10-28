package elog

import (
	"bytes"
	"context"
	"fmt"
	"hyjs-server/config"
	"hyjs-server/constUtil"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

func TestRepeatedWarningsBoundedAndDistinctErrorsPreserved(t *testing.T) {
	var output bytes.Buffer
	var dropped atomic.Int64
	core := zapcore.NewCore(zapcore.NewJSONEncoder(zap.NewProductionEncoderConfig()), zapcore.AddSync(&output), zapcore.InfoLevel)
	logger := zap.New(sampledCore(core, &dropped))
	for i := 0; i < 10005; i++ {
		logger.Debug("game request", zap.Int("playerId", i))
		logger.Warn("game request failed", zap.String("message", "CSShopBuy"))
	}
	for i := 0; i < 20; i++ {
		logger.Error("storage unavailable", zap.Int("attempt", i))
	}
	result := output.String()
	if strings.Contains(result, "game request\"") {
		t.Fatal("normal debug traffic reached the production log")
	}
	if count := strings.Count(result, `"msg":"game request failed"`); count != 105 {
		t.Fatalf("warning storm produced %d records", count)
	}
	if count := strings.Count(result, `"msg":"storage unavailable"`); count != 20 {
		t.Fatalf("critical errors were sampled or duplicated: %d", count)
	}
	if dropped.Load() != 9900 {
		t.Fatal("suppression count lost", dropped.Load())
	}
	t.Logf("10005 repeated warnings -> 105 records, reduction %.2f%%; all 20 errors preserved", 100*9900.0/10005)
}

func TestApplicationLogSummaryEmittedOnceWithoutDebugTraffic(t *testing.T) {
	previousConfig, previousLogger, previousSink, previousCore := config.AppConfigIns, eLogger, logSink, maintenanceCore
	previousSuppressed := suppressed.Load()
	t.Cleanup(func() {
		config.AppConfigIns, eLogger, logSink, maintenanceCore = previousConfig, previousLogger, previousSink, previousCore
		suppressed.Store(previousSuppressed)
	})
	filename := filepath.Join(t.TempDir(), "app.log")
	config.AppConfigIns = &config.AppConfig{LogConfig: config.LogConfig{LogFile: filename, LogLevel: "info"}}
	if err := LogInit(); err != nil {
		t.Fatal(err)
	}
	defer Sync()
	for i := 0; i < 10; i++ {
		Debug("game request")
		Warn("game request failed")
	}
	Error("critical storage failure")
	flushSuppressed()
	flushSuppressed()
	value, err := os.ReadFile(filename)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(value), `"msg":"game request"`) || bytes.Count(value, []byte("repeated logs suppressed")) != 1 || !bytes.Contains(value, []byte(`"count":5`)) {
		t.Fatal("debug output or duplicated/missing suppression summary", string(value))
	}
	if bytes.Count(value, []byte("critical storage failure")) != 1 {
		t.Fatal("critical error lost or duplicated")
	}
}

func TestFileSinkIdleRotationConcurrentWritesAndFiniteDefaults(t *testing.T) {
	filename := filepath.Join(t.TempDir(), "app.log")
	now := time.Now()
	sink, err := newFileSink(config.LogConfig{LogFile: filename, MaxAge: 30}, now)
	if err != nil {
		t.Fatal(err)
	}
	defer sink.Close()
	if sink.writer.MaxAge != 1 || sink.writer.MaxBackups != constUtil.LogDefaultMaxBackups || sink.writer.MaxSize != constUtil.LogDefaultMaxSizeMB {
		t.Fatal("empty/legacy configuration permits unbounded retention")
	}
	var workers sync.WaitGroup
	if _, err := sink.Write([]byte("before-rotation\n")); err != nil {
		t.Fatal(err)
	}
	for worker := 0; worker < 4; worker++ {
		workers.Add(1)
		go func(worker int) {
			defer workers.Done()
			for i := 0; i < 250; i++ {
				if _, err := fmt.Fprintf(sink, "%d:%d\n", worker, i); err != nil {
					t.Error(err)
				}
			}
		}(worker)
	}
	if err := sink.maintain(now.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	workers.Wait()
	if _, err := sink.Write([]byte("after-idle-rotation\n")); err != nil {
		t.Fatal(err)
	}
	if err := sink.maintain(now.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	files, err := filepath.Glob(filepath.Join(filepath.Dir(filename), "app*.log"))
	if err != nil || len(files) != 2 {
		t.Fatal("idle rotation did not produce one archive", files, err)
	}
	var all []byte
	for _, file := range files {
		value, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		all = append(all, value...)
	}
	if bytes.Count(all, []byte("\n")) != 1002 {
		t.Fatal("rotation lost or duplicated records")
	}
	active, _ := os.ReadFile(filename)
	if !bytes.HasSuffix(active, []byte("after-idle-rotation\n")) || bytes.Contains(active, []byte("before-rotation")) {
		t.Fatal("new writes used the old archive descriptor")
	}
}

func TestIdleMaintenanceUsesRealCreationAge(t *testing.T) {
	directory := t.TempDir()
	now := time.Now()
	archive := filepath.Join(directory, "app-"+now.Add(-time.Hour).Format("2006-01-02T15-04-05.000")+".log")
	other := filepath.Join(directory, "other-"+now.Format("2006-01-02T15-04-05.000")+".log")
	for _, file := range []string{archive, other} {
		if err := os.WriteFile(file, []byte("record\\n"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	sink, err := newFileSink(config.LogConfig{LogFile: filepath.Join(directory, "app.log")}, now)
	if err != nil {
		t.Fatal(err)
	}
	defer sink.Close()
	// The archive's filesystem creation timestamp controls both boundaries.
	if err = sink.maintain(now.Add(23*time.Hour + 59*time.Minute)); err != nil {
		t.Fatal(err)
	}
	if _, err = os.Stat(archive); err != nil {
		t.Fatal("deleted before 24h", err)
	}
	if err = sink.maintain(now.Add(24*time.Hour + time.Minute)); err != nil {
		t.Fatal(err)
	}
	if _, err = os.Stat(archive); !os.IsNotExist(err) {
		t.Fatal("expired archive retained", err)
	}
	if _, err = os.Stat(other); err != nil {
		t.Fatal("unrelated file touched", err)
	}
}

func TestFileSinkStartupBoundaryAndRejectSymlink(t *testing.T) {
	filename := filepath.Join(t.TempDir(), "app.log")
	if err := os.WriteFile(filename, []byte("previous-process\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	sink, err := newFileSink(config.LogConfig{LogFile: filename}, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	defer sink.Close()
	active, err := os.ReadFile(filename)
	if err != nil || len(active) != 0 {
		t.Fatal("previous process log remained active", err)
	}
	link := filepath.Join(filepath.Dir(filename), "linked.log")
	if err := os.Symlink(filename, link); err != nil {
		t.Fatal(err)
	}
	if _, err := newFileSink(config.LogConfig{LogFile: link}, time.Now()); err == nil {
		t.Fatal("symlink accepted as managed log")
	}
}

func TestMaintenanceStopsWithProcessContext(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	finished := make(chan error, 1)
	go func() { finished <- ServeMaintenance(ctx) }()
	cancel()
	select {
	case err := <-finished:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("log maintenance blocked process shutdown")
	}
}
