package elog

import (
	"context"
	"errors"
	"hyjs-server/config"
	"hyjs-server/constUtil"
	"sync/atomic"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

var eLogger = zap.NewNop()
var logSink *fileSink
var suppressed atomic.Int64
var maintenanceCore zapcore.Core = zapcore.NewNopCore()

func GetLogger() *zap.Logger {
	return eLogger
}

func LogInit() error {
	if config.AppConfigIns == nil {
		return errors.New("logging configuration unavailable")
	}
	cfg := config.AppConfigIns.LogConfig
	level, err := zapcore.ParseLevel(cfg.LogLevel)
	if err != nil {
		return err
	}
	sink, err := newFileSink(cfg, time.Now())
	if err != nil {
		return err
	}
	encoderConfig := zap.NewProductionEncoderConfig()
	encoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder
	core := zapcore.NewCore(
		zapcore.NewJSONEncoder(encoderConfig),
		zapcore.AddSync(sink),
		level,
	)
	suppressed.Store(0)
	logSink = sink
	maintenanceCore = core
	eLogger = zap.New(sampledCore(core, &suppressed))
	return nil
}

func Sync() {
	flushSuppressed()
	_ = eLogger.Sync()
	if logSink != nil {
		_ = logSink.Close()
	}
}

// sampledCore limits repeated informational/warning messages by fixed message
// and level. Fields never form an unbounded player/error-key cache; errors bypass it.
func sampledCore(core zapcore.Core, dropped *atomic.Int64) zapcore.Core {
	lower := &levelCore{Core: core, accept: func(level zapcore.Level) bool { return level < zapcore.ErrorLevel }}
	errors := &levelCore{Core: core, accept: func(level zapcore.Level) bool { return level >= zapcore.ErrorLevel }}
	sampled := zapcore.NewSamplerWithOptions(lower, constUtil.LogSampleInterval,
		constUtil.LogSampleInitial, constUtil.LogSampleThereafter,
		zapcore.SamplerHook(func(_ zapcore.Entry, decision zapcore.SamplingDecision) {
			if decision&zapcore.LogDropped != 0 {
				dropped.Add(1)
			}
		}))
	return zapcore.NewTee(sampled, errors)
}

type levelCore struct {
	zapcore.Core
	accept func(zapcore.Level) bool
}

func (c *levelCore) Enabled(level zapcore.Level) bool {
	return c.accept(level) && c.Core.Enabled(level)
}
func (c *levelCore) With(fields []zapcore.Field) zapcore.Core {
	return &levelCore{Core: c.Core.With(fields), accept: c.accept}
}
func (c *levelCore) Check(entry zapcore.Entry, checked *zapcore.CheckedEntry) *zapcore.CheckedEntry {
	if c.Enabled(entry.Level) {
		return checked.AddCore(entry, c)
	}
	return checked
}

func flushSuppressed() {
	if count := suppressed.Swap(0); count > 0 {
		// This fixed summary bypasses the sampler and contains no request payloads.
		entry := zapcore.Entry{Level: zapcore.WarnLevel, Time: time.Now(), Message: "repeated logs suppressed"}
		if checked := maintenanceCore.Check(entry, nil); checked != nil {
			checked.Write(zap.Int64("count", count))
		}
	}
}

// ServeMaintenance shares the process context and shutdown wait. Rotation runs
// while idle too, so an old active file cannot evade retention by staying small.
func ServeMaintenance(ctx context.Context) error {
	ticker := time.NewTicker(constUtil.LogMaintenanceInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case now := <-ticker.C:
			flushSuppressed()
			if logSink != nil {
				if err := logSink.maintain(now); err != nil {
					// Remain available during disk pressure; report once per interval.
					Error("log retention failed", zap.Error(err))
				}
			}
		}
	}
}

func Debug(msg string, fields ...zap.Field) {
	eLogger.Debug(msg, fields...)
}

func Info(msg string, fields ...zap.Field) {
	eLogger.Info(msg, fields...)
}

func Warn(msg string, fields ...zap.Field) {
	eLogger.Warn(msg, fields...)
}

func Error(msg string, fields ...zap.Field) {
	eLogger.Error(msg, fields...)
}
