package elog

import (
	"hyjs-server/config"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"gopkg.in/natefinch/lumberjack.v2"
)

var eLogger = zap.NewNop()

func GetLogger() *zap.Logger {
	return eLogger
}

func LogInit() {
	lumberjackLogger := &lumberjack.Logger{
		Filename:   config.AppConfigIns.LogConfig.LogFile,
		MaxSize:    config.AppConfigIns.LogConfig.MaxSize,
		MaxBackups: config.AppConfigIns.LogConfig.MaxBackups,
		MaxAge:     config.AppConfigIns.LogConfig.MaxAge,
		Compress:   config.AppConfigIns.LogConfig.Compress,
	}

	writeSyncer := zapcore.AddSync(lumberjackLogger)

	encoderConfig := zap.NewProductionEncoderConfig()
	encoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder

	level, _ := zapcore.ParseLevel(config.AppConfigIns.LogConfig.LogLevel)
	core := zapcore.NewCore(
		zapcore.NewJSONEncoder(encoderConfig),
		writeSyncer,
		level,
	)

	logger := zap.New(core)

	eLogger = logger
}

func Sync() {
	if eLogger != nil {
		_ = eLogger.Sync()
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
