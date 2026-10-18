package db

import (
	"fmt"
	"hyjs-server/config"
	"hyjs-server/elog"
	"log"
	"strings"
	"time"

	"go.uber.org/zap"
	"gopkg.in/natefinch/lumberjack.v2"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
)

var dbIns *gorm.DB

func GetIns() *gorm.DB {
	return dbIns
}

func Init() error {
	dsn := fmt.Sprintf("%v:%v@tcp(%v)/%v?charset=utf8mb4&parseTime=True&loc=Local",
		config.AppConfigIns.MysqlConfig.User,
		config.AppConfigIns.MysqlConfig.Pass,
		config.AppConfigIns.MysqlConfig.Host,
		config.AppConfigIns.MysqlConfig.DbName)

	gormConfig := &gorm.Config{}
	if gormLog := newGormLogger(); gormLog != nil {
		gormConfig.Logger = gormLog
	}

	db, err := gorm.Open(mysql.Open(dsn), gormConfig)
	if err != nil {
		elog.Error("db init err", zap.Error(err))
		return err
	}
	dbIns = db
	return nil
}

func newGormLogger() gormlogger.Interface {
	logLevel := strings.TrimSpace(config.AppConfigIns.LogConfig.GormLogLevel)
	if logLevel == "" {
		return nil
	}

	var level gormlogger.LogLevel
	switch strings.ToLower(logLevel) {
	case "silent":
		level = gormlogger.Silent
	case "error":
		level = gormlogger.Error
	case "warn", "warning":
		level = gormlogger.Warn
	case "info":
		level = gormlogger.Info
	default:
		return nil
	}

	slowThreshold := 200 * time.Millisecond
	if ms := config.AppConfigIns.LogConfig.GormSlowThresholdMs; ms > 0 {
		slowThreshold = time.Duration(ms) * time.Millisecond
	}

	return gormlogger.New(
		newGormLogWriter(),
		gormlogger.Config{
			SlowThreshold:             slowThreshold,
			LogLevel:                  level,
			IgnoreRecordNotFoundError: true,
			Colorful:                  false,
		},
	)
}

func newGormLogWriter() gormlogger.Writer {
	logFile := strings.TrimSpace(config.AppConfigIns.LogConfig.GormLogFile)
	if logFile == "" {
		logFile = "logs/sql.log"
	}

	gormFileLogger := &lumberjack.Logger{
		Filename:   logFile,
		MaxSize:    config.AppConfigIns.LogConfig.MaxSize,
		MaxBackups: config.AppConfigIns.LogConfig.MaxBackups,
		MaxAge:     config.AppConfigIns.LogConfig.MaxAge,
		Compress:   config.AppConfigIns.LogConfig.Compress,
	}

	return gormStdWriter{logger: log.New(gormFileLogger, "", log.LstdFlags)}
}

type gormStdWriter struct {
	logger *log.Logger
}

func (w gormStdWriter) Printf(format string, args ...interface{}) {
	if w.logger == nil {
		return
	}
	w.logger.Printf(format, args...)
}
