set -euo pipefail

# 调用前由 AI 判定并注入（示例默认值）
: "${PROJECT_NAME:=gin-project}"
: "${TARGET_PATH:=.}"
ROOT_DIR="${TARGET_PATH%/}/${PROJECT_NAME}"
MODULE_NAME="${MODULE_NAME:-$PROJECT_NAME}"
MODULE_NAME_SED="$(printf '%s' "$MODULE_NAME" | sed 's/[&|]/\\&/g')"

GO_VERSION="$(go env GOVERSION 2>/dev/null | sed 's/^go//')"
if [[ -z "$GO_VERSION" ]]; then
  GO_VERSION="$(go version | awk '{print $3}' | sed 's/^go//')"
fi
if [[ -z "$GO_VERSION" ]]; then
  echo "Failed to detect local Go version" >&2
  exit 1
fi

# 1) 创建目录结构
mkdir -p \
  "$ROOT_DIR/cache" \
  "$ROOT_DIR/config" \
  "$ROOT_DIR/constUtil" \
  "$ROOT_DIR/db" \
  "$ROOT_DIR/elog" \
  "$ROOT_DIR/fileStorage" \
  "$ROOT_DIR/handler" \
  "$ROOT_DIR/httpClient" \
  "$ROOT_DIR/locales" \
  "$ROOT_DIR/middleware" \
  "$ROOT_DIR/model" \
  "$ROOT_DIR/routes" \
  "$ROOT_DIR/service" \
  "$ROOT_DIR/utils" \
  "$ROOT_DIR/wrapper"

# 2) 写入文件内容
cat > "$ROOT_DIR/.gitignore" <<'__YFLOW_FILE_1__'
# Binaries
*.exe
*.exe~
*.dll
*.so
*.dylib
yflow-server

# Test binary
*.test

# Output
*.out

# Dependency directories
vendor/

# Go workspace file
go.work

# Environment
.env.local
.env.*.local

# IDE
.idea/
.vscode/
.cusor/*
.sisyphus/*
.claude/*
.codex/*
AGENTS.md
*.swp
*.swo

# OS
.DS_Store
Thumbs.db

# Logs
logs/
__YFLOW_FILE_1__

cat > "$ROOT_DIR/AGENTS.md" <<'__YFLOW_FILE_AGENTS__'
# AGENTS.md

## 项目约定

- 这是一个 Gin + Gorm 的 Go 服务，入口在 `main.go`。
- 代码变更后先执行 `gofmt`，再执行 `go test ./...` 做编译级校验。
- 依赖整理使用 `go mod tidy`，不要手工编辑 `go.sum`。
- Shell 命令遵循本机 Codex 约定，优先使用 `rtk` 前缀。
- 同时定义大于 1 个变量时，更偏向使用 `var (...)`，不要用多变量 `:=`；短小且紧贴使用点的临时变量可以酌情例外。
- 代码和实施计划都要有适量的步骤说明式注释，使用陈述句说明当前步骤的动作和目的，尤其是状态机推进、外部副作用、并发/ctx、配置来源和跨系统调用；不要写提问式注释，也不要写逐行复述代码的空注释。

## 分层习惯

- `handler` 只做参数绑定、参数清洗、全部 HTTP 入参校验、调用 `service`、返回统一响应；枚举值、URL 合法性、跨字段必填/互斥等请求参数校验也放在 `handler`，不要下沉到 `service`。参数校验代码量不大时直接平铺在 Handler 函数内，不额外抽 helper。
- `service` 放业务逻辑、数据库访问、内部/外部服务编排；默认接收已经由 `handler` 清洗和校验过的请求结构，不再重复做 HTTP 入参校验。
- `service` 包里不要定义包级常量；业务状态、类型、锁 key、Redis key、超时时间、重试间隔、错误消息模板等常量统一放到 `constUtil/`，可以按业务领域新建文件。只允许在函数内部使用非常短小、一次性的局部 `const`。
- 同一个业务领域优先写在同一个 `service` 文件里，入口函数、回调处理、调度/重试、失败处理、payload 构造等强相关逻辑不要拆到多个 `*Service.go` 里来回跳转；只有跨业务复用的基础设施、独立外部系统适配器或明显独立的子域，才拆成单独 service 文件。
- 函数调用链中已有 `context.Context` 时必须继续向下传递，不要换成 `context.Background()`；启动 goroutine 也要显式传入合适的 ctx，需要脱离请求取消但保留 ctx values 时使用 `context.WithoutCancel(ctx)`。
- `model` 放请求/响应结构和统一业务响应结构。
- `db` 放 Gorm 数据模型、分页结构、数据库初始化，不把 HTTP 入参结构放进 `db`。
- 通用常量和业务错误码统一放在 `constUtil/` 目录里，可以按领域或用途新建文件，不要散落在 `handler`/`service`/`model` 里；`db` model 相关的 enum 常量可以就近放在 `db` 包里，方便看表结构时一起看取值。
- `constUtil/errorCode.go` 维护错误码：`7001-7100` 为系统固定错误码，业务错误码从 `7101` 开始分配。
- 路由通过 `handler` 的 `init()` 注册到 `routes`，例如 `routes.RegisterPostRouter(url, wrapper.Wrapper(fn))`。

## 接口 DTO 与 Swagger 习惯

- HTTP body 入参和出参结构放在 `model` 包；API DTO 的 `json` tag 统一使用小驼峰，例如 `userId`、`callbackUrl`、`itemList`。
- `db` 包里的 Gorm model 字段按数据库列使用下划线 column/tag 没问题，不用为了 HTTP JSON 风格改数据库模型。
- Go 结构体里的 ID、外键 ID、状态值、类型值等整数统一优先使用 `int64`，不要使用 `uint64`，即使数据库字段是 `BIGINT UNSIGNED`。
- 接口必填 body 字段要加 `binding:"required"`，并在 handler 中保留必要的非零校验；例如 `userId`、`requestType`。
- 每个接口都要定义独立的 Request/Response 类型，即使当前字段完全一样，也不要复用一个通用 Request 来耦合不同接口。
- Request/Response 命名按 Handler 名去掉 `Handler` 后缀再追加 `Request` / `Response`，例如 `UserProfileHandler` 对应 `UserProfileRequest` 和 `UserProfileResponse`。
- 接口返回字段要按前端展示需要显式收敛，不要直接返回 db model；内部字段如 slug、uri、时间字段、排序字段等，未明确要求就不要出现在响应里。
- 接口相关的入参、出参字段只要有枚举含义，注释必须写清楚枚举值；出参状态可以写 `0=未知，1=xxx，2=yyy`，入参枚举不要写 `0=未知`，除非 0 是明确可传的业务值；布尔枚举写成 `false=xxx，true=yyy`。
- 每个 HTTP Handler 都要写函数注释和 Swagger 可读取注释，至少包含 `@Summary`、`@Description`、`@Tags`、`@Accept`、`@Produce`、`@Param`、`@Success`、`@Router`。
- Request、Response 以及对外返回 item 的导出类型和导出字段都要写注释，保证 Swagger 文档能读到清晰说明。

## 错误和响应

- Handler 返回 `error`，由 `wrapper.Wrapper` 统一处理错误响应；成功响应在 handler 中直接写 `c.JSON(http.StatusOK, model.NewBizCodeData(data))`。
- 参数错误使用 `model.NewErrParamsError()`。
- 系统错误使用 `model.NewErrSystemError()` 或让底层错误返回给 wrapper 统一处理。
- 业务错误使用 `model.NewErrBizCodeMsg(code, msg)`，错误码定义在 `constUtil/errorCode.go`。
- 成功响应使用 `c.JSON(http.StatusOK, model.NewBizCodeData(data))`。

## HTTP 调用

- 通过 HTTP 访问内部服务时，比如调用 Python 服务，必须使用 `httpClient/http.go` 暴露的方法：
  - `httpClient.HttpInner(serverName)`
  - `httpClient.HttpUserInner(c, serverName)`
- 调外部服务使用 `httpClient.HttpOutput()`。
- 不要在业务代码里直接使用 `utils.NewHttp()` 或绕过 `httpClient` 包；`utils/http.go` 是底层 HTTP client 实现，业务层不直接依赖它。
- 内部服务地址池通过 `httpClient.Init(serverPools)` 初始化，`serverName` 对应 `map[string][]string` 里的 key。

## 日志和配置

- 日志统一使用 `elog`，结构化字段使用 `zap.Field`。
- 配置从 `config.<env>.yaml` 读取，读取失败才回退到 `config.yaml`。
- 新增配置时同步更新 `config/config.go` 的结构体和各环境 yaml。

## 数据库习惯

- Gorm model 放在 `db` 包，一个表一个 Go 文件，结构体字段带 `gorm` 和 `json` tag。
- 表名通过 `TableName()` 显式声明。
- 查询时优先使用 `db.GetIns().WithContext(c)`，保留请求上下文。
- SQL 源文件统一放在 `db/sql/` 目录，一个表一个 `.sql` 文件；新增或调整表结构时，同步维护对应 SQL 文件。
- SQL seed 文件用对应表结构文件的 basename 加 `.seed.sql` 后缀命名，并通过 `.seed.sql` 判断 seed 文件；例如 `<nn>_<table>.sql` 对应 `<nn>_<table>.seed.sql`，不要使用 `<nn>_1_<table>.sql` 或 `<table>_seed.sql`。

## 示例代码风格

- `handler/exampleHandler.go` 展示 handler 只做参数校验和分配 service。
- `service/exampleService.go` 展示 service 访问 `db` model 并返回 `model` response。
- `model/exampleModel.go` 展示请求/响应 DTO。
- `db/exampleModel.go` 展示 Gorm 表模型。
__YFLOW_FILE_AGENTS__

cat > "$ROOT_DIR/config.dev.yaml" <<'__YFLOW_FILE_2__'
port: 8080

# log
logConfig:
  logFile: logs/app.log
  gormLogFile: logs/sql.log
  maxSize: 10
  maxBackups: 3
  maxAge: 7
  compress: false
  logLevel: debug
  gormLogLevel: info
  gormSlowThresholdMs: 200

# JWT
jwtKey: dev-jwt-secret-key

# mysql
mysql:
  user: root
  pass: dev-password
  host: localhost:3306
  dbName: db_dev

# redis
redis:
  host: localhost:6379
  password: ""
  db: 0
__YFLOW_FILE_2__

cat > "$ROOT_DIR/config.prd.yaml" <<'__YFLOW_FILE_3__'
port: 8080

# log
logConfig:
  logFile: logs/app.log
  gormLogFile: logs/sql.log
  maxSize: 50
  maxBackups: 30
  maxAge: 90
  compress: true
  logLevel: warn
  gormLogLevel: warn
  gormSlowThresholdMs: 500

# JWT (生产环境请使用强密钥)
jwtKey: prd-jwt-secret-key-change-this

# mysql
mysql:
  user: root
  pass: prd-password
  host: localhost:3306
  dbName: db_prod

# redis
redis:
  host: localhost:6379
  password: ""
  db: 0
__YFLOW_FILE_3__

cat > "$ROOT_DIR/config.test.yaml" <<'__YFLOW_FILE_4__'
port: 8080

# log
logConfig:
  logFile: logs/app.log
  gormLogFile: logs/sql.log
  maxSize: 10
  maxBackups: 3
  maxAge: 7
  compress: false
  logLevel: info
  gormLogLevel: warn
  gormSlowThresholdMs: 300

# JWT
jwtKey: test-jwt-secret-key

# mysql
mysql:
  user: root
  pass: test-password
  host: localhost:3306
  dbName: db_test

# redis
redis:
  host: localhost:6379
  password: ""
  db: 0
__YFLOW_FILE_4__

cat > "$ROOT_DIR/config.yaml" <<'__YFLOW_FILE_5__'
port: 8080

# log
logConfig:
  logFile: logs/app.log
  gormLogFile: logs/sql.log
  maxSize: 10
  maxBackups: 3
  maxAge: 7
  compress: false
  logLevel: info
  gormLogLevel: warn
  gormSlowThresholdMs: 300

# JWT
jwtKey: your-jwt-secret-key

# mysql
mysql:
  user: root
  pass: password
  host: localhost:3306
  dbName: db_name

# redis
redis:
  host: localhost:6379
  password: ""
  db: 0
__YFLOW_FILE_5__

cat > "$ROOT_DIR/config/config.go" <<'__YFLOW_FILE_6__'
package config

import (
	"strings"

	"github.com/spf13/viper"
)

type AppConfig struct {
	Port         string         `mapstructure:"port"`
	LogConfig    LogConfig      `mapstructure:"logConfig"`
	JwtKey       string         `mapstructure:"jwtKey"`
	MysqlConfig  MysqlConfig    `mapstructure:"mysql"`
	Redis        RedisConfig    `mapstructure:"redis"`
	StorageConfig StorageConfig `mapstructure:"storageConfig"`
}

type LogConfig struct {
	LogFile             string `mapstructure:"logFile"`
	GormLogFile         string `mapstructure:"gormLogFile"`
	MaxSize             int    `mapstructure:"maxSize"`
	MaxBackups          int    `mapstructure:"maxBackups"`
	MaxAge              int    `mapstructure:"maxAge"`
	Compress            bool   `mapstructure:"compress"`
	LogLevel            string `mapstructure:"logLevel"`
	GormLogLevel        string `mapstructure:"gormLogLevel"`
	GormSlowThresholdMs int    `mapstructure:"gormSlowThresholdMs"`
}

type MysqlConfig struct {
	User   string `mapstructure:"user"`
	Pass   string `mapstructure:"pass"`
	Host   string `mapstructure:"host"`
	DbName string `mapstructure:"dbName"`
}

type RedisConfig struct {
	Host     string `mapstructure:"host"`
	Password string `mapstructure:"password"`
	Db       int    `mapstructure:"db"`
}

type StorageConfig struct {
	StorageName   string `mapstructure:"storageName"`
	BucketName    string `mapstructure:"BucketName"`
	CosRegion     string `mapstructure:"cosRegion"`
	CosAppId      string `mapstructure:"cosAppId"`
	CosSecretId   string `mapstructure:"cosSecretId"`
	CosSecretKey  string `mapstructure:"cosSecretKey"`
	OssEndPoint   string `mapstructure:"ossEndPoint"`
	OssAccessKey  string `mapstructure:"ossAccessKey"`
	OssAccessSecret string `mapstructure:"ossAccessSecret"`
}

var AppConfigIns *AppConfig
var Env string

func Init(env string) error {
	if env == "" {
		env = "dev"
	}
	env = strings.ToLower(strings.TrimSpace(env))
	Env = env
	viper.RegisterAlias("logConfig.gormLogLevel", "logConfig.gormloglevel")
	viper.RegisterAlias("logConfig.gormLogFile", "logConfig.gormlogfile")
	viper.RegisterAlias("logConfig.gormSlowThresholdMs", "logConfig.gormslowthresholdms")

	viper.SetConfigFile("config." + env + ".yaml")
	if err := viper.ReadInConfig(); err != nil {
		viper.SetConfigFile("config.yaml")
		if err2 := viper.ReadInConfig(); err2 != nil {
			return err
		}
	}

	config := new(AppConfig)
	if err := viper.Unmarshal(config); err != nil {
		return err
	}
	AppConfigIns = config
	return nil
}
__YFLOW_FILE_6__

cat > "$ROOT_DIR/cache/redis.go" <<'__YFLOW_FILE_CACHE__'
package cache

import (
	"context"
	"yflow-server/config"
	"yflow-server/elog"

	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

var redisClient *redis.Client

func Init() error {
	client := redis.NewClient(
		&redis.Options{
			Addr:     config.AppConfigIns.Redis.Host,
			Password: config.AppConfigIns.Redis.Password,
			DB:       config.AppConfigIns.Redis.Db,
		})

	cmd := client.Ping(context.TODO())
	if cmd.Err() != nil {
		elog.Error("redis init error", zap.Error(cmd.Err()))
		return cmd.Err()
	}

	redisClient = client
	return nil
}

func GetClient() *redis.Client {
	return redisClient
}
__YFLOW_FILE_CACHE__

cat > "$ROOT_DIR/constUtil/const.go" <<'__YFLOW_FILE_7__'
// Package constUtil contains application constants
// Add your constants here based on project requirements
//
// Common constants include:
// - Error codes
// - Redis keys
// - API endpoints
// - Configuration keys
package constUtil

const CtxUid = "user_id"
__YFLOW_FILE_7__

cat > "$ROOT_DIR/constUtil/errorCode.go" <<'__YFLOW_FILE_ERROR_CODE__'
package constUtil

// 7001 - 7100 为系统固定错误码
const (
	ErrParamsError int64 = 7001
	ErrSystemError int64 = 7002
)

// 从 7101 开始是业务错误码，业务用的多分配2位数(如7101-7199)，业务用得少就分配1位数(如7101-7109)
const (
	ExampleError int64 = 7101
)
__YFLOW_FILE_ERROR_CODE__

cat > "$ROOT_DIR/db/db.go" <<'__YFLOW_FILE_8__'
package db

import (
	"fmt"
	"log"
	"yflow-server/config"
	"yflow-server/elog"
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
__YFLOW_FILE_8__

cat > "$ROOT_DIR/db/page.go" <<'__YFLOW_FILE_9__'
package db

type PageInfo struct {
	Curr  int64 `json:"curr"`
	Size  int64 `json:"size"`
	Total int64 `json:"total"`
}

func Page(maxLimit int, page PageInfo) (limit, offset int, curr int64) {
	limit = 20
	curr = 1

	if page.Size > 0 {
		limit = int(page.Size)
		if limit > maxLimit {
			limit = maxLimit
		}
	}
	if page.Curr > 0 {
		curr = page.Curr
		offset = int(page.Curr-1) * limit
	}

	return
}
__YFLOW_FILE_9__

cat > "$ROOT_DIR/db/exampleModel.go" <<'__YFLOW_FILE_DB_EXAMPLE_MODEL__'
package db

import "time"

type ExampleModel struct {
	Id         int64     `gorm:"primaryKey;autoIncrement" json:"id"` // ID
	FirstName  string    `gorm:"size:50;not null" json:"first_name"`
	LastName   string    `gorm:"size:50;not null" json:"last_name"`
	CreateTime time.Time `gorm:"column:create_time;autoCreateTime" json:"create_time"` // 创建时间
	UpdateTime time.Time `gorm:"column:update_time;autoUpdateTime" json:"update_time"` // 更新时间
}

func (ExampleModel) TableName() string {
	return "t_example"
}
__YFLOW_FILE_DB_EXAMPLE_MODEL__

cat > "$ROOT_DIR/elog/elog.go" <<'__YFLOW_FILE_10__'
package elog

import (
	"yflow-server/config"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"gopkg.in/natefinch/lumberjack.v2"
)

var eLogger *zap.Logger

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
__YFLOW_FILE_10__

cat > "$ROOT_DIR/fileStorage/cosStorage.go" <<'__YFLOW_FILE_11__'
package fileStorage

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"time"
	"yflow-server/config"

	"github.com/tencentyun/cos-go-sdk-v5"
)

type CosStorage struct {
	region       string
	appId        string
	secretId     string
	secretKey    string
	bucketClient map[string]*cos.Client
}

func initCosStorage() *CosStorage {
	return &CosStorage{
		region:       config.AppConfigIns.StorageConfig.CosRegion,
		appId:        config.AppConfigIns.StorageConfig.CosAppId,
		secretId:     config.AppConfigIns.StorageConfig.CosSecretId,
		secretKey:    config.AppConfigIns.StorageConfig.CosSecretKey,
		bucketClient: make(map[string]*cos.Client),
	}
}

func (c *CosStorage) getClient(bucketName string) *cos.Client {
	client, ok := c.bucketClient[bucketName]
	if ok {
		return client
	}

	u, _ := url.Parse(fmt.Sprintf("https://%v-%v.cos.%v.myqcloud.com", bucketName, c.appId, c.region))
	b := &cos.BaseURL{BucketURL: u}
	client = cos.NewClient(b, &http.Client{
		Transport: &cos.AuthorizationTransport{
			SecretID:  c.secretId,
			SecretKey: c.secretKey,
		},
	})
	c.bucketClient[bucketName] = client

	return client
}

func (c *CosStorage) GpUploadUrl(ctx context.Context, bucketName, fileName string, expired time.Duration) (string, error) {
	presignedURL, err := c.getClient(bucketName).Object.GetPresignedURL(ctx, http.MethodPut, fileName, c.secretId, c.secretKey, expired, nil)
	if err != nil {
		return "", err
	}
	return presignedURL.String(), nil
}

func (c *CosStorage) GpDownloadUrl(ctx context.Context, bucketName, fileName string, expired time.Duration) (string, error) {
	presignedURL, err := c.getClient(bucketName).Object.GetPresignedURL(ctx, http.MethodGet, fileName, c.secretId, c.secretKey, expired, nil)
	if err != nil {
		return "", err
	}
	return presignedURL.String(), nil
}

func (c *CosStorage) UploadFile(ctx context.Context, bucketName string, file *os.File) (string, error) {
	opt := &cos.ObjectPutOptions{
		ObjectPutHeaderOptions: &cos.ObjectPutHeaderOptions{},
		ACLHeaderOptions:      &cos.ACLHeaderOptions{},
	}
	_, err := c.getClient(bucketName).Object.Put(ctx, path.Base(file.Name()), file, opt)
	if err != nil {
		return "", err
	}

	return fmt.Sprintf("https://%v-%v.cos.%v.myqcloud.com/%s",
		bucketName, c.appId, c.region, url.PathEscape(path.Base(file.Name()))), nil
}

func (c *CosStorage) DownloadFile(ctx context.Context, bucketName, fileName string) ([]byte, error) {
	resp, err := c.getClient(bucketName).Object.Get(ctx, fileName, nil)
	if err != nil {
		return nil, err
	}
	b, err := io.ReadAll(resp.Body)
	resp.Body.Close()
	return b, err
}

func (c *CosStorage) DownloadFileLocal(ctx context.Context, bucketName, fileName, savePath string) error {
	_, err := c.getClient(bucketName).Object.GetToFile(ctx, fileName, savePath, nil)
	if err != nil {
		return err
	}
	return nil
}
__YFLOW_FILE_11__

cat > "$ROOT_DIR/fileStorage/ossStorage.go" <<'__YFLOW_FILE_12__'
package fileStorage

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/url"
	"os"
	"path"
	"strings"
	"time"
	"yflow-server/config"

	"github.com/aliyun/aliyun-oss-go-sdk/oss"
)

type OssStorage struct {
	endPoint     string
	accessKey    string
	accessSecret string
	client       *oss.Client
}

func initOssStorage() *OssStorage {
	return &OssStorage{
		endPoint:     config.AppConfigIns.StorageConfig.OssEndPoint,
		accessKey:    config.AppConfigIns.StorageConfig.OssAccessKey,
		accessSecret: config.AppConfigIns.StorageConfig.OssAccessSecret,
	}
}

func (c *OssStorage) getClient() (*oss.Client, error) {
	if c.client != nil {
		return c.client, nil
	}
	client, err := oss.New(c.endPoint, c.accessKey, c.accessSecret)
	if err != nil {
		return nil, err
	}

	c.client = client
	return client, nil
}

func (c *OssStorage) GpUploadUrl(ctx context.Context, bucketName, fileName string, expired time.Duration) (string, error) {
	client, err := c.getClient()
	if err != nil {
		return "", err
	}
	bucket, err := client.Bucket(bucketName)
	if err != nil {
		return "", err
	}

	signURL, err := bucket.SignURL(fileName, oss.HTTPPut, int64(expired.Seconds()), oss.ContentType("text/plain;charset=utf8"))
	if err != nil {
		return "", err
	}
	return signURL, nil
}

func (c *OssStorage) GpDownloadUrl(ctx context.Context, bucketName, fileName string, expired time.Duration) (string, error) {
	client, err := c.getClient()
	if err != nil {
		return "", err
	}
	bucket, err := client.Bucket(bucketName)
	if err != nil {
		return "", err
	}

	signURL, err := bucket.SignURL(fileName, oss.HTTPGet, int64(expired.Seconds()), oss.ContentType("text/plain;charset=utf8"))
	if err != nil {
		return "", err
	}
	return signURL, nil
}

func (c *OssStorage) UploadFile(ctx context.Context, bucketName string, file *os.File) (string, error) {
	client, err := c.getClient()
	if err != nil {
		return "", err
	}
	bucket, err := client.Bucket(bucketName)
	if err != nil {
		return "", err
	}

	objectKey := path.Base(file.Name())
	err = bucket.PutObject(objectKey, file)
	if err != nil {
		return "", err
	}

	endpoint := strings.TrimPrefix(strings.TrimPrefix(c.endPoint, "https://"), "http://")
	return fmt.Sprintf("https://%s.%s/%s", bucketName, endpoint, url.PathEscape(objectKey)), nil
}

func (c *OssStorage) DownloadFile(ctx context.Context, bucketName, fileName string) ([]byte, error) {
	client, err := c.getClient()
	if err != nil {
		return nil, err
	}
	bucket, err := client.Bucket(bucketName)
	if err != nil {
		return nil, err
	}

	body, err := bucket.GetObject(fileName)
	if err != nil {
		return nil, err
	}
	defer body.Close()

	buf := new(bytes.Buffer)
	_, err = io.Copy(buf, body)
	if err != nil {
		return nil, err
	}

	return buf.Bytes(), nil
}

func (c *OssStorage) DownloadFileLocal(ctx context.Context, bucketName, fileName, savePath string) error {
	client, err := c.getClient()
	if err != nil {
		return err
	}
	bucket, err := client.Bucket(bucketName)
	if err != nil {
		return err
	}

	err = bucket.GetObjectToFile(fileName, savePath)
	if err != nil {
		return err
	}

	return nil
}
__YFLOW_FILE_12__

cat > "$ROOT_DIR/fileStorage/storage.go" <<'__YFLOW_FILE_13__'
package fileStorage

import (
	"context"
	"errors"
	"os"
	"time"
)

type Storage interface {
	GpUploadUrl(ctx context.Context, bucketName, fileName string, expired time.Duration) (string, error)
	GpDownloadUrl(ctx context.Context, bucketName, fileName string, expired time.Duration) (string, error)
	UploadFile(ctx context.Context, bucketName string, file *os.File) (string, error)
	DownloadFile(ctx context.Context, bucketName, fileName string) ([]byte, error)
	DownloadFileLocal(ctx context.Context, bucketName, fileName, savePath string) error
}

var fileStorage Storage

func GetStorage() Storage {
	return fileStorage
}

func Init(name string) error {
	if name == "cos" {
		fileStorage = initCosStorage()
		return nil
	}
	if name == "oss" {
		fileStorage = initOssStorage()
		return nil
	}
	return errors.New("not support storage")
}
__YFLOW_FILE_13__

cat > "$ROOT_DIR/go.mod" <<'__YFLOW_FILE_14__'
module yflow-server

go 1.26.3

require (
	github.com/aliyun/aliyun-oss-go-sdk v3.0.2+incompatible
	github.com/gin-gonic/gin v1.12.0
	github.com/go-sql-driver/mysql v1.9.3
	github.com/golang-jwt/jwt/v5 v5.2.3
	github.com/google/uuid v1.1.1
	github.com/nicksnyder/go-i18n/v2 v2.6.1
	github.com/redis/go-redis/v9 v9.17.2
	github.com/speps/go-hashids v2.0.0+incompatible
	github.com/spf13/viper v1.21.0
	github.com/swaggo/files v1.0.1
	github.com/swaggo/gin-swagger v1.6.1
	github.com/tencentyun/cos-go-sdk-v5 v0.7.72
	go.uber.org/zap v1.27.1
	golang.org/x/crypto v0.48.0
	golang.org/x/text v0.34.0
	gopkg.in/natefinch/lumberjack.v2 v2.2.1
	gopkg.in/yaml.v3 v3.0.1
	gorm.io/driver/mysql v1.5.7
	gorm.io/gorm v1.25.12
	resty.dev/v3 v3.0.0-rc.2
)

require (
	filippo.io/edwards25519 v1.1.0 // indirect
	github.com/KyleBanks/depth v1.2.1 // indirect
	github.com/PuerkitoBio/purell v1.1.1 // indirect
	github.com/PuerkitoBio/urlesc v0.0.0-20170810143723-de5bf2ad4578 // indirect
	github.com/bytedance/gopkg v0.1.3 // indirect
	github.com/bytedance/sonic v1.15.0 // indirect
	github.com/bytedance/sonic/loader v0.5.0 // indirect
	github.com/cespare/xxhash/v2 v2.3.0 // indirect
	github.com/clbanning/mxj v1.8.4 // indirect
	github.com/cloudwego/base64x v0.1.6 // indirect
	github.com/dgryski/go-rendezvous v0.0.0-20200823014737-9f7001d12a5f // indirect
	github.com/fsnotify/fsnotify v1.9.0 // indirect
	github.com/gabriel-vasile/mimetype v1.4.12 // indirect
	github.com/gin-contrib/sse v1.1.0 // indirect
	github.com/go-openapi/jsonpointer v0.19.5 // indirect
	github.com/go-openapi/jsonreference v0.19.6 // indirect
	github.com/go-openapi/spec v0.20.4 // indirect
	github.com/go-openapi/swag v0.19.15 // indirect
	github.com/go-playground/locales v0.14.1 // indirect
	github.com/go-playground/universal-translator v0.18.1 // indirect
	github.com/go-playground/validator/v10 v10.30.1 // indirect
	github.com/go-viper/mapstructure/v2 v2.4.0 // indirect
	github.com/goccy/go-json v0.10.5 // indirect
	github.com/goccy/go-yaml v1.19.2 // indirect
	github.com/google/go-querystring v1.0.0 // indirect
	github.com/josharian/intern v1.0.0 // indirect
	github.com/json-iterator/go v1.1.12 // indirect
	github.com/jinzhu/inflection v1.0.0 // indirect
	github.com/jinzhu/now v1.1.5 // indirect
	github.com/klauspost/cpuid/v2 v2.3.0 // indirect
	github.com/leodido/go-urn v1.4.0 // indirect
	github.com/mailru/easyjson v0.7.6 // indirect
	github.com/mattn/go-isatty v0.0.20 // indirect
	github.com/mitchellh/mapstructure v1.4.3 // indirect
	github.com/modern-go/concurrent v0.0.0-20180306012644-bacd9c7ef1dd // indirect
	github.com/modern-go/reflect2 v1.0.2 // indirect
	github.com/mozillazg/go-httpheader v0.2.1 // indirect
	github.com/pelletier/go-toml/v2 v2.2.4 // indirect
	github.com/quic-go/qpack v0.6.0 // indirect
	github.com/quic-go/quic-go v0.59.0 // indirect
	github.com/sagikazarmark/locafero v0.11.0 // indirect
	github.com/sourcegraph/conc v0.3.1-0.20240121214520-5f936abd7ae8 // indirect
	github.com/spf13/afero v1.15.0 // indirect
	github.com/spf13/cast v1.10.0 // indirect
	github.com/spf13/pflag v1.0.10 // indirect
	github.com/subosito/gotenv v1.6.0 // indirect
	github.com/swaggo/swag v1.8.12 // indirect
	github.com/twitchyliquid64/golang-asm v0.15.1 // indirect
	github.com/ugorji/go/codec v1.3.1 // indirect
	go.mongodb.org/mongo-driver/v2 v2.5.0 // indirect
	go.uber.org/multierr v1.10.0 // indirect
	go.yaml.in/yaml/v3 v3.0.4 // indirect
	golang.org/x/arch v0.22.0 // indirect
	golang.org/x/net v0.51.0 // indirect
	golang.org/x/sys v0.41.0 // indirect
	golang.org/x/time v0.14.0 // indirect
	golang.org/x/tools v0.41.0 // indirect
	google.golang.org/protobuf v1.36.10 // indirect
	gopkg.in/yaml.v2 v2.4.0 // indirect
)
__YFLOW_FILE_14__

cat > "$ROOT_DIR/go.sum" <<'__YFLOW_FILE_15__'
filippo.io/edwards25519 v1.1.0 h1:FNf4tywRC1HmFuKW5xopWpigGjJKiJSV0Cqo0cJWDaA=
filippo.io/edwards25519 v1.1.0/go.mod h1:BxyFTGdWcka3PhytdK4V28tE5sGfRvvvRV7EaN4VDT4=
github.com/BurntSushi/toml v1.6.0 h1:dRaEfpa2VI55EwlIW72hMRHdWouJeRF7TPYhI+AUQjk=
github.com/BurntSushi/toml v1.6.0/go.mod h1:ukJfTF/6rtPPRCnwkur4qwRxa8vTRFBF0uk2lLoLwho=
github.com/KyleBanks/depth v1.2.1 h1:5h8fQADFrWtarTdtDudMmGsC7GPbOAu6RVB3ffsVFHc=
github.com/KyleBanks/depth v1.2.1/go.mod h1:jzSb9d0L43HxTQfT+oSA1EEp2q+ne2uh6XgeJcm8brE=
github.com/PuerkitoBio/purell v1.1.1 h1:WEQqlqaGbrPkxLJWfBwQmfEAE1Z7ONdDLqrN38tNFfI=
github.com/PuerkitoBio/purell v1.1.1/go.mod h1:c11w/QuzBsJSee3cPx9rAFu61PvFxuPbtSwDGJws/X0=
github.com/PuerkitoBio/urlesc v0.0.0-20170810143723-de5bf2ad4578 h1:d+Bc7a5rLufV/sSk/8dngufqelfh6jnri85riMAaF/M=
github.com/PuerkitoBio/urlesc v0.0.0-20170810143723-de5bf2ad4578/go.mod h1:uGdkoq3SwY9Y+13GIhn11/XLaGBb4BfwItxLd5jeuXE=
github.com/aliyun/aliyun-oss-go-sdk v3.0.2+incompatible h1:8psS8a+wKfiLt1iVDX79F7Y6wUM49Lcha2FMXt4UM8g=
github.com/aliyun/aliyun-oss-go-sdk v3.0.2+incompatible/go.mod h1:T/Aws4fEfogEE9v+HPhhw+CntffsBHJ8nXQCwKr0/g8=
github.com/bytedance/gopkg v0.1.3 h1:TPBSwH8RsouGCBcMBktLt1AymVo2TVsBVCY4b6TnZ/M=
github.com/bytedance/gopkg v0.1.3/go.mod h1:576VvJ+eJgyCzdjS+c4+77QF3p7ubbtiKARP3TxducM=
github.com/bytedance/sonic v1.15.0 h1:/PXeWFaR5ElNcVE84U0dOHjiMHQOwNIx3K4ymzh/uSE=
github.com/bytedance/sonic v1.15.0/go.mod h1:tFkWrPz0/CUCLEF4ri4UkHekCIcdnkqXw9VduqpJh0k=
github.com/bytedance/sonic/loader v0.5.0 h1:gXH3KVnatgY7loH5/TkeVyXPfESoqSBSBEiDd5VjlgE=
github.com/bytedance/sonic/loader v0.5.0/go.mod h1:AR4NYCk5DdzZizZ5djGqQ92eEhCCcdf5x77udYiSJRo=
github.com/cespare/xxhash/v2 v2.3.0 h1:UL815xU9SqsFlibzuggzjXhog7bL6oX9BbNZnL2UFvs=
github.com/cespare/xxhash/v2 v2.3.0/go.mod h1:VGX0DQ3Q6kWi7AoAeZDth3/j3BFtOZR5XLFGgcrjCOs=
github.com/clbanning/mxj v1.8.4 h1:HuhwZtbyvyOw+3Z1AowPkU87JkJUSv751ELWaiTpj8I=
github.com/clbanning/mxj v1.8.4/go.mod h1:BVjHeAH+rl9rs6f+QIpeRl0tfu10SXn1pUSa5PVGJng=
github.com/cloudwego/base64x v0.1.6 h1:t11wG9AECkCDk5fMSoxmufanudBtJ+/HemLstXDLI2M=
github.com/cloudwego/base64x v0.1.6/go.mod h1:OFcloc187FXDaYHvrNIjxSe8ncn0OOM8gEHfghB2IPU=
github.com/creack/pty v1.1.9/go.mod h1:oKZEueFk5CKHvIhNR5MUki03XCEU+Q6VDXinZuGJ33E=
github.com/davecgh/go-spew v1.1.0/go.mod h1:J7Y8YcW2NihsgmVo/mv3lAwl/skON4iLHjSsI+c5H38=
github.com/davecgh/go-spew v1.1.1 h1:vj9j/u1bqnvCEfJOwUhtlOARqs3+rkHYY13jYWTU97c=
github.com/davecgh/go-spew v1.1.1/go.mod h1:J7Y8YcW2NihsgmVo/mv3lAwl/skON4iLHjSsI+c5H38=
github.com/dgryski/go-rendezvous v0.0.0-20200823014737-9f7001d12a5f h1:lO4WD4F/rVNCu3HqELle0jiPLLBs70cWOduZpkS1E78=
github.com/dgryski/go-rendezvous v0.0.0-20200823014737-9f7001d12a5f/go.mod h1:cuUVRXasLTGF7a8hSLbxyZXjz+1KgoB3wDUb6vlszIc=
github.com/frankban/quicktest v1.14.6 h1:7Xjx+VpznH+oBnejlPUj8oUpdxnVs4f8XU8WnHkI4W8=
github.com/frankban/quicktest v1.14.6/go.mod h1:4ptaffx2x8+WTWXmUCuVU6aPUX1/Mz7zb5vbUoiM6w0=
github.com/fsnotify/fsnotify v1.9.0 h1:2Ml+OJNzbYCTzsxtv8vKSFD9PbJjmhYF14k/jKC7S9k=
github.com/fsnotify/fsnotify v1.9.0/go.mod h1:8jBTzvmWwFyi3Pb8djgCCO5IBqzKJ/Jwo8TRcHyHii0=
github.com/gabriel-vasile/mimetype v1.4.12 h1:e9hWvmLYvtp846tLHam2o++qitpguFiYCKbn0w9jyqw=
github.com/gabriel-vasile/mimetype v1.4.12/go.mod h1:d+9Oxyo1wTzWdyVUPMmXFvp4F9tea18J8ufA774AB3s=
github.com/gin-contrib/gzip v0.0.6 h1:NjcunTcGAj5CO1gn4N8jHOSIeRFHIbn51z6K+xaN4d4=
github.com/gin-contrib/gzip v0.0.6/go.mod h1:QOJlmV2xmayAjkNS2Y8NQsMneuRShOU/kjovCXNuzzk=
github.com/gin-contrib/sse v1.1.0 h1:n0w2GMuUpWDVp7qSpvze6fAu9iRxJY4Hmj6AmBOU05w=
github.com/gin-contrib/sse v1.1.0/go.mod h1:hxRZ5gVpWMT7Z0B0gSNYqqsSCNIJMjzvm6fqCz9vjwM=
github.com/gin-gonic/gin v1.12.0 h1:b3YAbrZtnf8N//yjKeU2+MQsh2mY5htkZidOM7O0wG8=
github.com/gin-gonic/gin v1.12.0/go.mod h1:VxccKfsSllpKshkBWgVgRniFFAzFb9csfngsqANjnLc=
github.com/go-openapi/jsonpointer v0.19.3/go.mod h1:Pl9vOtqEWErmShwVjC8pYs9cog34VGT37dQOVbmoatg=
github.com/go-openapi/jsonpointer v0.19.5 h1:gZr+CIYByUqjcgeLXnQu2gHYQC9o73G2XUeOFYEICuY=
github.com/go-openapi/jsonpointer v0.19.5/go.mod h1:Pl9vOtqEWErmShwVjC8pYs9cog34VGT37dQOVbmoatg=
github.com/go-openapi/jsonreference v0.19.6 h1:UBIxjkht+AWIgYzCDSv2GN+E/togfwXUJFRTWhl2Jjs=
github.com/go-openapi/jsonreference v0.19.6/go.mod h1:diGHMEHg2IqXZGKxqyvWdfWU/aim5Dprw5bqpKkTvns=
github.com/go-openapi/spec v0.20.4 h1:O8hJrt0UMnhHcluhIdUgCLRWyM2x7QkBXRvOs7m+O1M=
github.com/go-openapi/spec v0.20.4/go.mod h1:faYFR1CvsJZ0mNsmsphTMSoRrNV3TEDoAM7FOEWeq8I=
github.com/go-openapi/swag v0.19.5/go.mod h1:POnQmlKehdgb5mhVOsnJFsivZCEZ/vjK9gh66Z9tfKk=
github.com/go-openapi/swag v0.19.15 h1:D2NRCBzS9/pEY3gP9Nl8aDqGUcPFrwG2p+CNFrLyrCM=
github.com/go-openapi/swag v0.19.15/go.mod h1:QYRuS/SOXUCsnplDa677K7+DxSOj6IPNl/eQntq43wQ=
github.com/go-playground/assert/v2 v2.2.0 h1:JvknZsQTYeFEAhQwI4qEt9cyV5ONwRHC+lYKSsYSR8s=
github.com/go-playground/assert/v2 v2.2.0/go.mod h1:VDjEfimB/XKnb+ZQfWdccd7VUvScMdVu0Titje2rxJ4=
github.com/go-playground/locales v0.14.1 h1:EWaQ/wswjilfKLTECiXz7Rh+3BjFhfDFKv/oXslEjJA=
github.com/go-playground/locales v0.14.1/go.mod h1:hxrqLVvrK65+Rwrd5Fc6F2O76J/NuW9t0sjnWqG1slY=
github.com/go-playground/universal-translator v0.18.1 h1:Bcnm0ZwsGyWbCzImXv+pAJnYK9S473LQFuzCbDbfSFY=
github.com/go-playground/universal-translator v0.18.1/go.mod h1:xekY+UJKNuX9WP91TpwSH2VMlDf28Uj24BCp08ZFTUY=
github.com/go-playground/validator/v10 v10.30.1 h1:f3zDSN/zOma+w6+1Wswgd9fLkdwy06ntQJp0BBvFG0w=
github.com/go-playground/validator/v10 v10.30.1/go.mod h1:oSuBIQzuJxL//3MelwSLD5hc2Tu889bF0Idm9Dg26cM=
github.com/go-sql-driver/mysql v1.9.3 h1:U/N249h2WzJ3Ukj8SowVFjdtZKfu9vlLZxjPXV1aweo=
github.com/go-sql-driver/mysql v1.9.3/go.mod h1:qn46aNg1333BRMNU69Lq93t8du/dwxI64Gl8i5p1WMU=
github.com/go-viper/mapstructure/v2 v2.4.0 h1:EBsztssimR/CONLSZZ04E8qAkxNYq4Qp9LvH92wZUgs=
github.com/go-viper/mapstructure/v2 v2.4.0/go.mod h1:oJDH3BJKyqBA2TXFhDsKDGDTlndYOZ6rGS0BRZIxGhM=
github.com/goccy/go-json v0.10.5 h1:Fq85nIqj+gXn/S5ahsiTlK3TmC85qgirsdTP/+DeaC4=
github.com/goccy/go-json v0.10.5/go.mod h1:oq7eo15ShAhp70Anwd5lgX2pLfOS3QCiwU/PULtXL6M=
github.com/goccy/go-yaml v1.19.2 h1:PmFC1S6h8ljIz6gMRBopkjP1TVT7xuwrButHID66PoM=
github.com/goccy/go-yaml v1.19.2/go.mod h1:XBurs7gK8ATbW4ZPGKgcbrY1Br56PdM69F7LkFRi1kA=
github.com/golang-jwt/jwt/v5 v5.2.3 h1:kkGXqQOBSDDWRhWNXTFpqGSCMyh/PLnqUvMGJPDJDs0=
github.com/golang-jwt/jwt/v5 v5.2.3/go.mod h1:pqrtFR0X4osieyHYxtmOUWsAWrfe1Q5UVIyoH402zdk=
github.com/google/go-cmp v0.7.0 h1:wk8382ETsv4JYUZwIsn6YpYiWiBsYLSJiTsyBybVuN8=
github.com/google/go-cmp v0.7.0/go.mod h1:pXiqmnSA92OHEEa9HXL2W4E7lf9JzCmGVUdgjX3N/iU=
github.com/google/go-querystring v1.0.0 h1:Xkwi/a1rcvNg1PPYe5vI8GbeBY/jrVuDX5ASuANWTrk=
github.com/google/go-querystring v1.0.0/go.mod h1:odCYkC5MyYFN7vkCjXpyrEuKhc/BUO6wN/zVPAxq5ck=
github.com/google/gofuzz v1.0.0/go.mod h1:dBl0BpW6vV/+mYPU4Po3pmUjxk6FQPldtuIdl/M65Eg=
github.com/google/uuid v1.1.1 h1:Gkbcsh/GbpXz7lPftLA3P6TYMwjCLYm83jiFQZF/3gY=
github.com/google/uuid v1.1.1/go.mod h1:TIyPZe4MgqvfeYDBFedMoGGpEw/LqOeaOT+nhxU+yHo=
github.com/jinzhu/inflection v1.0.0 h1:K317FqzuhWc8YvSVlFMCCUb36O/S9MCKRDI7QkRKD/E=
github.com/jinzhu/inflection v1.0.0/go.mod h1:h+uFLlag+Qp1Va5pdKtLDYj+kHp5pxUVkryuEj+Srlc=
github.com/jinzhu/now v1.1.5 h1:/o9tlHleP7gOFmsnYNz3RGnqzefHA47wQpKrrdTIwXQ=
github.com/jinzhu/now v1.1.5/go.mod h1:d3SSVoowX0Lcu0IBviAWJpolVfI5UJVZZ7cO71lE/z8=
github.com/josharian/intern v1.0.0 h1:vlS4z54oSdjm0bgjRigI+G1HpF+tI+9rE5LLzOg8HmY=
github.com/josharian/intern v1.0.0/go.mod h1:5DoeVV0s6jJacbCEi61lwdGj/aVlrQvzHFFd8Hwg//Y=
github.com/json-iterator/go v1.1.12 h1:PV8peI4a0ysnczrg+LtxykD8LfKY9ML6u2jnxaEnrnM=
github.com/json-iterator/go v1.1.12/go.mod h1:e30LSqwooZae/UwlEbR2852Gd8hjQvJoHmT4TnhNGBo=
github.com/klauspost/cpuid/v2 v2.3.0 h1:S4CRMLnYUhGeDFDqkGriYKdfoFlDnMtqTiI/sFzhA9Y=
github.com/klauspost/cpuid/v2 v2.3.0/go.mod h1:hqwkgyIinND0mEev00jJYCxPNVRVXFQeu1XKlok6oO0=
github.com/kr/pretty v0.1.0/go.mod h1:dAy3ld7l9f0ibDNOQOHHMYYIIbhfbHSm3C4ZsoJORNo=
github.com/kr/pretty v0.3.1 h1:flRD4NNwYAUpkphVc1HcthR4KEIFJ65n8Mw5qdRn3LE=
github.com/kr/pretty v0.3.1/go.mod h1:hoEshYVHaxMs3cyo3Yncou5ZscifuDolrwPKZanG3xk=
github.com/kr/pty v1.1.1/go.mod h1:pFQYn66WHrOpPYNljwOMqo10TkYh1fy3cYio2l3bCsQ=
github.com/kr/text v0.1.0/go.mod h1:4Jbv+DJW3UT/LiOwJeYQe1efqtUx/iVham/4vfdArNI=
github.com/kr/text v0.2.0 h1:5Nx0Ya0ZqY2ygV366QzturHI13Jq95ApcVaJBhpS+AY=
github.com/kr/text v0.2.0/go.mod h1:eLer722TekiGuMkidMxC/pM04lWEeraHUUmBw8l2grE=
github.com/leodido/go-urn v1.4.0 h1:WT9HwE9SGECu3lg4d/dIA+jxlljEa1/ffXKmRjqdmIQ=
github.com/leodido/go-urn v1.4.0/go.mod h1:bvxc+MVxLKB4z00jd1z+Dvzr47oO32F/QSNjSBOlFxI=
github.com/mailru/easyjson v0.0.0-20190614124828-94de47d64c63/go.mod h1:C1wdFJiN94OJF2b5HbByQZoLdCWB1Yqtg26g4irojpc=
github.com/mailru/easyjson v0.0.0-20190626092158-b2ccc519800e/go.mod h1:C1wdFJiN94OJF2b5HbByQZoLdCWB1Yqtg26g4irojpc=
github.com/mailru/easyjson v0.7.6 h1:8yTIVnZgCoiM1TgqoeTl+LfU5Jg6/xL3QhGQnimLYnA=
github.com/mailru/easyjson v0.7.6/go.mod h1:xzfreul335JAWq5oZzymOObrkdz5UnU4kGfJJLY9Nlc=
github.com/mattn/go-isatty v0.0.20 h1:xfD0iDuEKnDkl03q4limB+vH+GxLEtL/jb4xVJSWWEY=
github.com/mattn/go-isatty v0.0.20/go.mod h1:W+V8PltTTMOvKvAeJH7IuucS94S2C6jfK/D7dTCTo3Y=
github.com/mitchellh/mapstructure v1.4.3 h1:OVowDSCllw/YjdLkam3/sm7wEtOy59d8ndGgCcyj8cs=
github.com/mitchellh/mapstructure v1.4.3/go.mod h1:bFUtVrKA4DC2yAKiSyO/QUcy7e+RRV2QTWOzhPopBRo=
github.com/modern-go/concurrent v0.0.0-20180228061459-e0a39a4cb421/go.mod h1:6dJC0mAP4ikYIbvyc7fijjWJddQyLn8Ig3JB5CqoB9Q=
github.com/modern-go/concurrent v0.0.0-20180306012644-bacd9c7ef1dd h1:TRLaZ9cD/w8PVh93nsPXa1VrQ6jlwL5oN8l14QlcNfg=
github.com/modern-go/concurrent v0.0.0-20180306012644-bacd9c7ef1dd/go.mod h1:6dJC0mAP4ikYIbvyc7fijjWJddQyLn8Ig3JB5CqoB9Q=
github.com/modern-go/reflect2 v1.0.2 h1:xBagoLtFs94CBntxluKeaWgTMpvLxC4ur3nMaC9Gz0M=
github.com/modern-go/reflect2 v1.0.2/go.mod h1:yWuevngMOJpCy52FWWMvUC8ws7m/LJsjYzDa0/r8luk=
github.com/mozillazg/go-httpheader v0.2.1 h1:geV7TrjbL8KXSyvghnFm+NyTux/hxwueTSrwhe88TQQ=
github.com/mozillazg/go-httpheader v0.2.1/go.mod h1:jJ8xECTlalr6ValeXYdOF8fFUISeBAdw6E61aqQma60=
github.com/nicksnyder/go-i18n/v2 v2.6.1 h1:JDEJraFsQE17Dut9HFDHzCoAWGEQJom5s0TRd17NIEQ=
github.com/nicksnyder/go-i18n/v2 v2.6.1/go.mod h1:Vee0/9RD3Quc/NmwEjzzD7VTZ+Ir7QbXocrkhOzmUKA=
github.com/niemeyer/pretty v0.0.0-20200227124842-a10e7caefd8e/go.mod h1:zD1mROLANZcx1PVRCS0qkT7pwLkGfwJo4zjcN/Tysno=
github.com/pelletier/go-toml/v2 v2.2.4 h1:mye9XuhQ6gvn5h28+VilKrrPoQVanw5PMw/TB0t5Ec4=
github.com/pelletier/go-toml/v2 v2.2.4/go.mod h1:2gIqNv+qfxSVS7cM2xJQKtLSTLUE9V8t9Stt+h56mCY=
github.com/pmezard/go-difflib v1.0.0 h1:4DBwDE0NGyQoBHbLQYPwSUPoCMWR5BEzIk/f1lZbAQM=
github.com/pmezard/go-difflib v1.0.0/go.mod h1:iKH77koFhYxTK1pcRnkKkqfTogsbg7gZNVY4sRDYZ/4=
github.com/quic-go/qpack v0.6.0 h1:g7W+BMYynC1LbYLSqRt8PBg5Tgwxn214ZZR34VIOjz8=
github.com/quic-go/qpack v0.6.0/go.mod h1:lUpLKChi8njB4ty2bFLX2x4gzDqXwUpaO1DP9qMDZII=
github.com/quic-go/quic-go v0.59.0 h1:OLJkp1Mlm/aS7dpKgTc6cnpynnD2Xg7C1pwL6vy/SAw=
github.com/quic-go/quic-go v0.59.0/go.mod h1:upnsH4Ju1YkqpLXC305eW3yDZ4NfnNbmQRCMWS58IKU=
github.com/redis/go-redis/v9 v9.17.2 h1:P2EGsA4qVIM3Pp+aPocCJ7DguDHhqrXNhVcEp4ViluI=
github.com/redis/go-redis/v9 v9.17.2/go.mod h1:u410H11HMLoB+TP67dz8rL9s6QW2j76l0//kSOd3370=
github.com/rogpeppe/go-internal v1.10.0 h1:TMyTOH3F/DB16zRVcYyreMH6GnZZrwQVAoYjRBZyWFQ=
github.com/rogpeppe/go-internal v1.10.0/go.mod h1:UQnix2H7Ngw/k4C5ijL5+65zddjncjaFoBhdsK/akog=
github.com/rs/dnscache v0.0.0-20230804202142-fc85eb664529/go.mod h1:qe5TWALJ8/a1Lqznoc5BDHpYX/8HU60Hm2AwRmqzxqA=
github.com/sagikazarmark/locafero v0.11.0 h1:1iurJgmM9G3PA/I+wWYIOw/5SyBtxapeHDcg+AAIFXc=
github.com/sagikazarmark/locafero v0.11.0/go.mod h1:nVIGvgyzw595SUSUE6tvCp3YYTeHs15MvlmU87WwIik=
github.com/sourcegraph/conc v0.3.1-0.20240121214520-5f936abd7ae8 h1:+jumHNA0Wrelhe64i8F6HNlS8pkoyMv5sreGx2Ry5Rw=
github.com/sourcegraph/conc v0.3.1-0.20240121214520-5f936abd7ae8/go.mod h1:3n1Cwaq1E1/1lhQhtRK2ts/ZwZEhjcQeJQ1RuC6Q/8U=
github.com/speps/go-hashids v2.0.0+incompatible h1:kSfxGfESueJKTx0mpER9Y/1XHl+FVQjtCqRyYcviFbw=
github.com/speps/go-hashids v2.0.0+incompatible/go.mod h1:P7hqPzMdnZOfyIk+xrlG1QaSMw+gCBdHKsBDnhpaZvc=
github.com/spf13/afero v1.15.0 h1:b/YBCLWAJdFWJTN9cLhiXXcD7mzKn9Dm86dNnfyQw1I=
github.com/spf13/afero v1.15.0/go.mod h1:NC2ByUVxtQs4b3sIUphxK0NioZnmxgyCrfzeuq8lxMg=
github.com/spf13/cast v1.10.0 h1:h2x0u2shc1QuLHfxi+cTJvs30+ZAHOGRic8uyGTDWxY=
github.com/spf13/cast v1.10.0/go.mod h1:jNfB8QC9IA6ZuY2ZjDp0KtFO2LZZlg4S/7bzP6qqeHo=
github.com/spf13/pflag v1.0.10 h1:4EBh2KAYBwaONj6b2Ye1GiHfwjqyROoF4RwYO+vPwFk=
github.com/spf13/pflag v1.0.10/go.mod h1:McXfInJRrz4CZXVZOBLb0bTZqETkiAhM9Iw0y3An2Bg=
github.com/spf13/viper v1.21.0 h1:x5S+0EU27Lbphp4UKm1C+1oQO+rKx36vfCoaVebLFSU=
github.com/spf13/viper v1.21.0/go.mod h1:P0lhsswPGWD/1lZJ9ny3fYnVqxiegrlNrEmgLjbTCAY=
github.com/stretchr/objx v0.1.0/go.mod h1:HFkY916IF+rwdDfMAkV7OtwuqBVzrE8GR6GFx+wExME=
github.com/stretchr/objx v0.4.0/go.mod h1:YvHI0jy2hoMjB+UWwv71VJQ9isScKT/TqJzVSSt89Yw=
github.com/stretchr/objx v0.5.0/go.mod h1:Yh+to48EsGEfYuaHDzXPcE3xhTkx73EhmCGUpEOglKo=
github.com/stretchr/objx v0.5.2/go.mod h1:FRsXN1f5AsAjCGJKqEizvkpNtU+EGNCLh3NxZ/8L+MA=
github.com/stretchr/testify v1.3.0/go.mod h1:M5WIy9Dh21IEIfnGCwXGc5bZfKNJtfHm1UVUgZn+9EI=
github.com/stretchr/testify v1.6.1/go.mod h1:6Fq8oRcR53rry900zMqJjRRixrwX3KX962/h/Wwjteg=
github.com/stretchr/testify v1.7.1/go.mod h1:6Fq8oRcR53rry900zMqJjRRixrwX3KX962/h/Wwjteg=
github.com/stretchr/testify v1.8.0/go.mod h1:yNjHg4UonilssWZ8iaSj1OCr/vHnekPRkoO+kdMU+MU=
github.com/stretchr/testify v1.8.4/go.mod h1:sz/lmYIOXD/1dqDmKjjqLyZ2RngseejIcXlSw2iwfAo=
github.com/stretchr/testify v1.10.0/go.mod h1:r2ic/lqez/lEtzL7wO/rwa5dbSLXVDPFyf8C91i36aY=
github.com/stretchr/testify v1.11.1 h1:7s2iGBzp5EwR7/aIZr8ao5+dra3wiQyKjjFuvgVKu7U=
github.com/stretchr/testify v1.11.1/go.mod h1:wZwfW3scLgRK+23gO65QZefKpKQRnfz6sD981Nm4B6U=
github.com/subosito/gotenv v1.6.0 h1:9NlTDc1FTs4qu0DDq7AEtTPNw6SVm7uBMsUCUjABIf8=
github.com/subosito/gotenv v1.6.0/go.mod h1:Dk4QP5c2W3ibzajGcXpNraDfq2IrhjMIvMSWPKKo0FU=
github.com/swaggo/files v1.0.1 h1:J1bVJ4XHZNq0I46UU90611i9/YzdrF7x92oX1ig5IdE=
github.com/swaggo/files v1.0.1/go.mod h1:0qXmMNH6sXNf+73t65aKeB+ApmgxdnkQzVTAj2uaMUg=
github.com/swaggo/gin-swagger v1.6.1 h1:Ri06G4gc9N4t4k8hekMigJ9zKTFSlqj/9paAQCQs7cY=
github.com/swaggo/gin-swagger v1.6.1/go.mod h1:LQ+hJStHakCWRiK/YNYtJOu4mR2FP+pxLnILT/qNiTw=
github.com/swaggo/swag v1.8.12 h1:pctzkNPu0AlQP2royqX3apjKCQonAnf7KGoxeO4y64w=
github.com/swaggo/swag v1.8.12/go.mod h1:lNfm6Gg+oAq3zRJQNEMBE66LIJKM44mxFqhEEgy2its=
github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/common v1.0.563/go.mod h1:7sCQWVkxcsR38nffDW057DRGk8mUjK1Ing/EFOK8s8Y=
github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/kms v1.0.563/go.mod h1:uom4Nvi9W+Qkom0exYiJ9VWJjXwyxtPYTkKkaLMlfE0=
github.com/tencentyun/cos-go-sdk-v5 v0.7.72 h1:k9aD8ri7Sqy2hYGYo6I2+OslDgY6IT5R0jUOHHSjW5Y=
github.com/tencentyun/cos-go-sdk-v5 v0.7.72/go.mod h1:STbTNaNKq03u+gscPEGOahKzLcGSYOj6Dzc5zNay7Pg=
github.com/tencentyun/qcloud-cos-sts-sdk v0.0.0-20250515025012-e0eec8a5d123/go.mod h1:b18KQa4IxHbxeseW1GcZox53d7J0z39VNONTxvvlkXw=
github.com/twitchyliquid64/golang-asm v0.15.1 h1:SU5vSMR7hnwNxj24w34ZyCi/FmDZTkS4MhqMhdFk5YI=
github.com/twitchyliquid64/golang-asm v0.15.1/go.mod h1:a1lVb/DtPvCB8fslRZhAngC2+aY1QWCk3Cedj/Gdt08=
github.com/ugorji/go/codec v1.3.1 h1:waO7eEiFDwidsBN6agj1vJQ4AG7lh2yqXyOXqhgQuyY=
github.com/ugorji/go/codec v1.3.1/go.mod h1:pRBVtBSKl77K30Bv8R2P+cLSGaTtex6fsA2Wjqmfxj4=
github.com/yuin/goldmark v1.4.13/go.mod h1:6yULJ656Px+3vBD8DxQVa3kxgyrAnzto9xy5taEt/CY=
go.mongodb.org/mongo-driver/v2 v2.5.0 h1:yXUhImUjjAInNcpTcAlPHiT7bIXhshCTL3jVBkF3xaE=
go.mongodb.org/mongo-driver/v2 v2.5.0/go.mod h1:yOI9kBsufol30iFsl1slpdq1I0eHPzybRWdyYUs8K/0=
go.uber.org/goleak v1.3.0 h1:2K3zAYmnTNqV73imy9J1T3WC+gmCePx2hEGkimedGto=
go.uber.org/goleak v1.3.0/go.mod h1:CoHD4mav9JJNrW/WLlf7HGZPjdw8EucARQHekz1X6bE=
go.uber.org/mock v0.6.0 h1:hyF9dfmbgIX5EfOdasqLsWD6xqpNZlXblLB/Dbnwv3Y=
go.uber.org/mock v0.6.0/go.mod h1:KiVJ4BqZJaMj4svdfmHM0AUx4NJYO8ZNpPnZn1Z+BBU=
go.uber.org/multierr v1.10.0 h1:S0h4aNzvfcFsC3dRF1jLoaov7oRaKqRGC/pUEJ2yvPQ=
go.uber.org/multierr v1.10.0/go.mod h1:20+QtiLqy0Nd6FdQB9TLXag12DsQkrbs3htMFfDN80Y=
go.uber.org/zap v1.27.1 h1:08RqriUEv8+ArZRYSTXy1LeBScaMpVSTBhCeaZYfMYc=
go.uber.org/zap v1.27.1/go.mod h1:GB2qFLM7cTU87MWRP2mPIjqfIDnGu+VIO4V/SdhGo2E=
go.yaml.in/yaml/v3 v3.0.4 h1:tfq32ie2Jv2UxXFdLJdh3jXuOzWiL1fo0bu/FbuKpbc=
go.yaml.in/yaml/v3 v3.0.4/go.mod h1:DhzuOOF2ATzADvBadXxruRBLzYTpT36CKvDb3+aBEFg=
golang.org/x/arch v0.22.0 h1:c/Zle32i5ttqRXjdLyyHZESLD/bB90DCU1g9l/0YBDI=
golang.org/x/arch v0.22.0/go.mod h1:dNHoOeKiyja7GTvF9NJS1l3Z2yntpQNzgrjh1cU103A=
gorm.io/driver/mysql v1.5.7 h1:MndhOPYOfEp2rHKgkZIhJ16eVUIRf2HmzgoPmh7FCWo=
gorm.io/driver/mysql v1.5.7/go.mod h1:sEtPWMiqiN1N1cMXoXmBbd8C6/l+TESwriotuRRpkDM=
gorm.io/gorm v1.25.12 h1:I0u8i2hWQItBq1WfE0o2+WuL9+8L21K9e2HHSTE/0f8=
gorm.io/gorm v1.25.12/go.mod h1:xh7N7RHfYlNc5EmcI/El95gXusucDrQnHXe0+CgWcLQ=
golang.org/x/crypto v0.0.0-20190308221718-c2843e01d9a2/go.mod h1:djNgcEr1/C05ACkg1iLfiJU5Ep61QUkGW8qpdssI0+w=
golang.org/x/crypto v0.0.0-20210921155107-089bfa567519/go.mod h1:GvvjBRRGRdwPK5ydBHafDWAxML/pGHZbMvKqRZ5+Abc=
golang.org/x/crypto v0.48.0 h1:/VRzVqiRSggnhY7gNRxPauEQ5Drw9haKdM0jqfcCFts=
golang.org/x/crypto v0.48.0/go.mod h1:r0kV5h3qnFPlQnBSrULhlsRfryS2pmewsg+XfMgkVos=
golang.org/x/mod v0.6.0-dev.0.20220419223038-86c51ed26bb4/go.mod h1:jJ57K6gSWd91VN4djpZkiMVwK6gcyfeH4XE8wZrZaV4=
golang.org/x/mod v0.32.0 h1:9F4d3PHLljb6x//jOyokMv3eX+YDeepZSEo3mFJy93c=
golang.org/x/mod v0.32.0/go.mod h1:SgipZ/3h2Ci89DlEtEXWUk/HteuRin+HHhN+WbNhguU=
golang.org/x/net v0.0.0-20190620200207-3b0461eec859/go.mod h1:z5CRVTTTmAJ677TzLLGU+0bjPO0LkuOLi4/5GtJWs/s=
golang.org/x/net v0.0.0-20210226172049-e18ecbb05110/go.mod h1:m0MpNAwzfU5UDzcl9v0D8zg8gWTRqZa9RBIspLL5mdg=
golang.org/x/net v0.0.0-20210421230115-4e50805a0758/go.mod h1:72T/g9IO56b78aLF+1Kcs5dz7/ng1VjMUvfKvpfy+jM=
golang.org/x/net v0.0.0-20220722155237-a158d28d115b/go.mod h1:XRhObCWvk6IyKnWLug+ECip1KBveYUHfp+8e9klMJ9c=
golang.org/x/net v0.7.0/go.mod h1:2Tu9+aMcznHK/AK1HMvgo6xiTLG5rD5rZLDS+rp2Bjs=
golang.org/x/net v0.51.0 h1:94R/GTO7mt3/4wIKpcR5gkGmRLOuE/2hNGeWq/GBIFo=
golang.org/x/net v0.51.0/go.mod h1:aamm+2QF5ogm02fjy5Bb7CQ0WMt1/WVM7FtyaTLlA9Y=
golang.org/x/sync v0.0.0-20190423024810-112230192c58/go.mod h1:RxMgew5VJxzue5/jJTE5uejpjVlOe/izrB70Jof72aM=
golang.org/x/sync v0.0.0-20220722155255-886fb9371eb4/go.mod h1:RxMgew5VJxzue5/jJTE5uejpjVlOe/izrB70Jof72aM=
golang.org/x/sync v0.19.0 h1:vV+1eWNmZ5geRlYjzm2adRgW2/mcpevXNg50YZtPCE4=
golang.org/x/sync v0.19.0/go.mod h1:9KTHXmSnoGruLpwFjVSX0lNNA75CykiMECbovNTZqGI=
golang.org/x/sys v0.0.0-20190215142949-d0b11bdaac8a/go.mod h1:STP8DvDyc/dI5b8T5hshtkjS+E42TnysNCUPdjciGhY=
golang.org/x/sys v0.0.0-20201119102817-f84b799fce68/go.mod h1:h1NjWce9XRLGQEsW7wpKNCjG9DtNlClVuFLEZdDNbEs=
golang.org/x/sys v0.0.0-20210420072515-93ed5bcd2bfe/go.mod h1:h1NjWce9XRLGQEsW7wpKNCjG9DtNlClVuFLEZdDNbEs=
golang.org/x/sys v0.0.0-20210615035016-665e8c7367d1/go.mod h1:oPkhp1MJrh7nUepCBck5+mAzfO9JrbApNNgaTdGDITg=
golang.org/x/sys v0.0.0-20220520151302-bc2c85ada10a/go.mod h1:oPkhp1MJrh7nUepCBck5+mAzfO9JrbApNNgaTdGDITg=
golang.org/x/sys v0.0.0-20220722155257-8c9f86f7a55f/go.mod h1:oPkhp1MJrh7nUepCBck5+mAzfO9JrbApNNgaTdGDITg=
golang.org/x/sys v0.5.0/go.mod h1:oPkhp1MJrh7nUepCBck5+mAzfO9JrbApNNgaTdGDITg=
golang.org/x/sys v0.6.0/go.mod h1:oPkhp1MJrh7nUepCBck5+mAzfO9JrbApNNgaTdGDITg=
golang.org/x/sys v0.41.0 h1:Ivj+2Cp/ylzLiEU89QhWblYnOE9zerudt9Ftecq2C6k=
golang.org/x/sys v0.41.0/go.mod h1:OgkHotnGiDImocRcuBABYBEXf8A9a87e/uXjp9XT3ks=
golang.org/x/term v0.0.0-20201126162022-7de9c90e9dd1/go.mod h1:bj7SfCRtBDWHUb9snDiAeCFNEtKQo2Wmx5Cou7ajbmo=
golang.org/x/term v0.0.0-20210927222741-03fcf44c2211/go.mod h1:jbD1KX2456YbFQfuXm/mYQcufACuNUgVhRMnK/tPxf8=
golang.org/x/term v0.5.0/go.mod h1:jMB1sMXY+tzblOD4FWmEbocvup2/aLOaQEp7JmGp78k=
golang.org/x/text v0.3.0/go.mod h1:NqM8EUOU14njkJ3fqMW+pc6Ldnwhi/IjpwHt7yyuwOQ=
golang.org/x/text v0.3.3/go.mod h1:5Zoc/QRtKVWzQhOtBMvqHzDpF6irO9z98xDceosuGiQ=
golang.org/x/text v0.3.6/go.mod h1:5Zoc/QRtKVWzQhOtBMvqHzDpF6irO9z98xDceosuGiQ=
golang.org/x/text v0.3.7/go.mod h1:u+2+/6zg+i71rQMx5EYifcz6MCKuco9NR6JIITiCfzQ=
golang.org/x/text v0.7.0/go.mod h1:mrYo+phRRbMaCq/xk9113O4dZlRixOauAjOtrjsXDZ8=
golang.org/x/text v0.34.0 h1:oL/Qq0Kdaqxa1KbNeMKwQq0reLCCaFtqu2eNuSeNHbk=
golang.org/x/text v0.34.0/go.mod h1:homfLqTYRFyVYemLBFl5GgL/DWEiH5wcsQ5gSh1yziA=
golang.org/x/time v0.14.0 h1:MRx4UaLrDotUKUdCIqzPC48t1Y9hANFKIRpNx+Te8PI=
golang.org/x/time v0.14.0/go.mod h1:eL/Oa2bBBK0TkX57Fyni+NgnyQQN4LitPmob2Hjnqw4=
golang.org/x/tools v0.0.0-20180917221912-90fa682c2a6e/go.mod h1:n7NCudcB/nEzxVGmLbDWY5pfWTLqBcC2KZ6jyYvM4mQ=
golang.org/x/tools v0.0.0-20191119224855-298f0cb1881e/go.mod h1:b+2E5dAYhXwXZwtnZ6UAqBI28+e2cm9otk0dWdXHAEo=
golang.org/x/tools v0.1.12/go.mod h1:hNGJHUnrk76NpqgfD5Aqm5Crs+Hm0VOH/i9J2+nxYbc=
golang.org/x/tools v0.41.0 h1:a9b8iMweWG+S0OBnlU36rzLp20z1Rp10w+IY2czHTQc=
golang.org/x/tools v0.41.0/go.mod h1:XSY6eDqxVNiYgezAVqqCeihT4j1U2CCsqvH3WhQpnlg=
golang.org/x/xerrors v0.0.0-20190717185122-a985d3407aa7/go.mod h1:I/5z698sn9Ka8TeJc9MKroUUfqBBauWjQqLJ2OPfmY0=
google.golang.org/protobuf v1.36.10 h1:AYd7cD/uASjIL6Q9LiTjz8JLcrh/88q5UObnmY3aOOE=
google.golang.org/protobuf v1.36.10/go.mod h1:HTf+CrKn2C3g5S8VImy6tdcUvCska2kB7j23XfzDpco=
gopkg.in/check.v1 v0.0.0-20161208181325-20d25e280405/go.mod h1:Co6ibVJAznAaIkqp8huTwlJQCZ016jof/cbN4VW5Yz0=
gopkg.in/check.v1 v1.0.0-20180628173108-788fd7840127/go.mod h1:Co6ibVJAznAaIkqp8huTwlJQCZ016jof/cbN4VW5Yz0=
gopkg.in/check.v1 v1.0.0-20200227125254-8fa46927fb4f/go.mod h1:Co6ibVJAznAaIkqp8huTwlJQCZ016jof/cbN4VW5Yz0=
gopkg.in/check.v1 v1.0.0-20201130134442-10cb98267c6c h1:Hei/4ADfdWqJk1ZMxUNpqntNwaWcugrBjAiHlqqRiVk=
gopkg.in/check.v1 v1.0.0-20201130134442-10cb98267c6c/go.mod h1:JHkPIbrfpd72SG/EVd6muEfDQjcINNoR0C8j2r3qZ4Q=
gopkg.in/natefinch/lumberjack.v2 v2.2.1 h1:bBRl1b0OH9s/DuPhuXpNl+VtCaJXFZ5/uEFST95x9zc=
gopkg.in/natefinch/lumberjack.v2 v2.2.1/go.mod h1:YD8tP3GAjkrDg1eZH7EGmyESg/lsYskCTPBJVb9jqSc=
gopkg.in/yaml.v2 v2.2.2/go.mod h1:hI93XBmqTisBFMUTm0b8Fm+jr3Dg1NNxqwp+5A1VGuI=
gopkg.in/yaml.v2 v2.4.0 h1:D8xgwECY7CYvx+Y2n4sBz93Jn9JRvxdiyyo8CTfuKaY=
gopkg.in/yaml.v2 v2.4.0/go.mod h1:RDklbk79AGWmwhnvt/jBztapEOGDOx6ZbXqjP6csGnQ=
gopkg.in/yaml.v3 v3.0.0-20200313102051-9f266ea9e77c/go.mod h1:K4uyk7z7BCEPqu6E+C64Yfv1cQ7kz7rIZviUmN+EgEM=
gopkg.in/yaml.v3 v3.0.0-20200615113413-eeeca48fe776/go.mod h1:K4uyk7z7BCEPqu6E+C64Yfv1cQ7kz7rIZviUmN+EgEM=
gopkg.in/yaml.v3 v3.0.1 h1:fxVm/GzAzEWqLHuvctI91KS9hhNmmWOoWu0XTYJS7CA=
gopkg.in/yaml.v3 v3.0.1/go.mod h1:K4uyk7z7BCEPqu6E+C64Yfv1cQ7kz7rIZviUmN+EgEM=
resty.dev/v3 v3.0.0-rc.2 h1:NE6RqPkUd27eQ5dFSb5tfyT3s6XX9ZySp9qZ7Nw1bn0=
resty.dev/v3 v3.0.0-rc.2/go.mod h1:NTOerrC/4T7/FE6tXIZGIysXXBdgNqwMZuKtxpea9NM=
__YFLOW_FILE_15__

cat > "$ROOT_DIR/handler/exampleHandler.go" <<'__YFLOW_FILE_16__'
package handler

import (
	"net/http"
	"strings"

	"yflow-server/constUtil"
	"yflow-server/model"
	"yflow-server/routes"
	"yflow-server/service"
	"yflow-server/wrapper"

	"github.com/gin-gonic/gin"
)

func init() {
	urlPrefix := "/api"
	routes.RegisterPostRouter(urlPrefix+"/hello", wrapper.Wrapper(HelloHandler))
}

// HelloHandler 处理函数
// Handler 只做参数校验 和 分配Service
func HelloHandler(c *gin.Context) error {
	// 业务逻辑
	var req model.ExampleListRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		return model.NewErrParamsError()
	}

	req.FirstName = strings.TrimSpace(req.FirstName)

	if req.FirstName == "Claude" {
		// 自定义错误消息
		return model.NewErrBizCodeMsg(constUtil.ExampleError, "Claude 已被禁用")
	}

	resp, err := service.ExampleService(c, req)
	if err != nil {
		return err
	}

	c.JSON(http.StatusOK, model.NewBizCodeData(resp))

	return nil
}
__YFLOW_FILE_16__

cat > "$ROOT_DIR/httpClient/client.go" <<'__YFLOW_FILE_17__'
package httpClient

import "yflow-server/utils"

// HttpClient provides two clients:
// - OutputCli: 外部服务调用
// - InnerCli: 内部服务调用（默认 ServerName 对应的 LB）
// ServerPools 用于维护服务地址池：ServerName -> []address
type HttpClient struct {
	OutputCli         *utils.HttpClient
	InnerCli          *utils.HttpClient
	ServerPools       map[string][]string
	ServiceLBs        map[string]*utils.LB
	LBOptions         []utils.LBOption
}

// NewHttpClient - 创建 HttpClient
// 为每个 ServerName 创建独立 LB（使用同一策略），InnerCli 不绑定默认 LB
func NewHttpClient(serverPools map[string][]string, lbOptions ...utils.LBOption) *HttpClient {
	outputCli := utils.NewHttp()
	innerCli := utils.NewHttp()

	serviceLBs := make(map[string]*utils.LB, len(serverPools))
	for name, servers := range serverPools {
		if len(servers) == 0 {
			continue
		}
		serviceLBs[name] = utils.NewLB(servers, lbOptions...)
	}

	return &HttpClient{
		OutputCli:         outputCli,
		InnerCli:          innerCli,
		ServerPools:       copyServerPools(serverPools),
		ServiceLBs:        serviceLBs,
		LBOptions:         lbOptions,
	}
}

// InnerFor - 获取指定 ServerName 的内部客户端（对应服务地址池的 LB）
func (hc *HttpClient) InnerFor(serverName string) *utils.HttpClient {
	if hc == nil {
		return utils.NewHttp()
	}
	if lb, ok := hc.ServiceLBs[serverName]; ok {
		return utils.NewHttp().WithLB(lb)
	}
	return utils.NewHttp()
}

func copyServerPools(pools map[string][]string) map[string][]string {
	if pools == nil {
		return nil
	}
	out := make(map[string][]string, len(pools))
	for k, v := range pools {
		cp := make([]string, len(v))
		copy(cp, v)
		out[k] = cp
	}
	return out
}
__YFLOW_FILE_17__

cat > "$ROOT_DIR/httpClient/http.go" <<'__YFLOW_FILE_18__'
package httpClient

import (
	"yflow-server/utils"

	"github.com/gin-gonic/gin"
)

var httpCli *HttpClient

func Init(serverPools map[string][]string) {
	httpCli = NewHttpClient(serverPools, utils.LBRandom())
}

func HttpOutput() *utils.HttpClient {
	return httpCli.OutputCli
}

func HttpInner(serverName string) *utils.HttpClient {
	return httpCli.InnerFor(serverName)
}

func HttpUserInner(c *gin.Context, serverName string) *utils.HttpClient {
	return httpCli.InnerFor(serverName).WithLBKeyUserId(c)
}
__YFLOW_FILE_18__

cat > "$ROOT_DIR/i18n.sh" <<'__YFLOW_FILE_19__'
#!/bin/sh
#
# i18n.sh - go-i18n 辅助脚本
#
# 用法：
#   1.填写active.en.yaml
#   2.执行 sh i18n.sh merge
#     - 执行 goi18n merge
#     - 生成/更新 locales/translate.zh.yaml
#   3.修改translate.zh.yaml，将英文改为中文
#   4.执行 sh i18n.sh active
#     - 将 translate.zh.yaml 添加到 active.zh.yaml
#
# 前置条件：
#   1. go install -v github.com/nicksnyder/go-i18n/v2/goi18n@latest
#   2. brew install yq

set -e

CMD="$1"
LOCALES_DIR="locales"
SRC_LANG_FILE="$LOCALES_DIR/active.en.yaml"
TRANSLATE_ZH_FILE="$LOCALES_DIR/translate.zh.yaml"
ACTIVE_ZH_FILE="$LOCALES_DIR/active.zh.yaml"

case "$CMD" in
  merge)
    echo "▶ goi18n merge (yaml)..."
    if [ ! -f "$SRC_LANG_FILE" ]; then
      echo "❌ $SRC_LANG_FILE 不存在"
      exit 1
    fi
    if [ ! -f "$TRANSLATE_ZH_FILE" ]; then
      touch "$TRANSLATE_ZH_FILE"
    fi
    goi18n merge -format yaml -outdir "$LOCALES_DIR" "$SRC_LANG_FILE" "$TRANSLATE_ZH_FILE"
    echo "✅ 已生成/更新 $TRANSLATE_ZH_FILE"
    ;;
  active)
    echo "▶ 将 translate.zh.yaml 添加到 active.zh.yaml"
    if [ ! -f "$TRANSLATE_ZH_FILE" ]; then
      echo "❌ $TRANSLATE_ZH_FILE 不存在"
      exit 1
    fi
    if [ ! -f "$ACTIVE_ZH_FILE" ]; then
      cp "$TRANSLATE_ZH_FILE" "$ACTIVE_ZH_FILE"
    else
      yq eval-all --indent=4 'select(fileIndex == 0) * select(fileIndex == 1)' \
        "$ACTIVE_ZH_FILE" "$TRANSLATE_ZH_FILE" > "$ACTIVE_ZH_FILE.tmp"
      mv "$ACTIVE_ZH_FILE.tmp" "$ACTIVE_ZH_FILE"
    fi
    cp "$ACTIVE_ZH_FILE" "$TRANSLATE_ZH_FILE"
    echo "✅ 已更新 $ACTIVE_ZH_FILE"
    ;;
  *)
    echo "❌ 未知命令: $CMD"
    echo "用法: ./i18n.sh {merge|active}"
    exit 1
    ;;
esac
__YFLOW_FILE_19__

cat > "$ROOT_DIR/locales/README.md" <<'__YFLOW_FILE_20__'
# 国际化语言文件

# active.en.yaml - 英文翻译源文件
# active.zh.yaml - 中文翻译文件

# 在此目录下创建语言文件，使用以下命名格式：
# - active.en.yaml (英文)
# - active.zh.yaml (中文)
# - translate.zh.yaml (翻译模板，由 i18n.sh 生成)
__YFLOW_FILE_20__

cat > "$ROOT_DIR/main.go" <<'__YFLOW_FILE_21__'
package main

import (
	"flag"
	"yflow-server/cache"
	"yflow-server/config"
	"yflow-server/db"
	"yflow-server/elog"
	_ "yflow-server/handler"
	"yflow-server/httpClient"
	"yflow-server/middleware"
	"yflow-server/routes"

	"github.com/gin-gonic/gin"
	swaggerFiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"

	"go.uber.org/zap"
)

// @title API Title
// @version 1.0
// @description API Description
// @host localhost:8080
// @BasePath /
func main() {
	env := flag.String("env", "dev", "config env: dev|test|prd")
	flag.Parse()
	if err := config.Init(*env); err != nil {
		panic(err)
	}

	elog.LogInit()

	if err := db.Init(); err != nil {
		elog.Error("db init error", zap.Error(err))
		return
	}

	if err := cache.Init(); err != nil {
		elog.Error("redis init error", zap.Error(err))
		return
	}

	httpClient.Init(make(map[string][]string))

	r := gin.New()
	r.Use(middleware.Recovery(), middleware.Logger(), middleware.CORS())
	// 信任的IP（适用于：Docker / K8s / 内网部署）
	// 用于获取 REAL_IP 时，跳过本地 和 代理IP
	r.SetTrustedProxies([]string{
		"127.0.0.1",
		"10.0.0.0/8",
		"172.16.0.0/12",
		"192.168.0.0/16",
	})

	// Swagger 文档路由
	r.GET("/swagger/*any", ginSwagger.WrapHandler(swaggerFiles.Handler))

	// 注册路由
	routes.RouteRegister(r)

	elog.Info("app init success")

	r.Run(":" + config.AppConfigIns.Port)
}
__YFLOW_FILE_21__

cat > "$ROOT_DIR/middleware/logger.go" <<'__YFLOW_FILE_22__'
package middleware

import (
	"net/http"
	"runtime/debug"
	"time"

	"yflow-server/elog"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

func Logger() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		path := c.Request.URL.Path
		raw := c.Request.URL.RawQuery
		if raw != "" {
			path = path + "?" + raw
		}
		clientIP := c.ClientIP()
		method := c.Request.Method

		c.Next()

		latency := time.Since(start)
		status := c.Writer.Status()

		elog.Debug("request",
			zap.String("method", method),
			zap.String("path", path),
			zap.Int("status", status),
			zap.Duration("latency", latency),
			zap.String("clientIP", clientIP),
		)
	}
}

func Recovery() gin.HandlerFunc {
	return func(c *gin.Context) {
		defer func() {
			if err := recover(); err != nil {
				elog.Error("panic recovered",
					zap.Any("err", err),
					zap.String("stack", string(debug.Stack())),
					zap.String("path", c.Request.URL.Path),
					zap.String("method", c.Request.Method),
					zap.String("clientIP", c.ClientIP()),
				)
				c.JSON(http.StatusOK, gin.H{"code": 500, "message": "system error"})
				c.Abort()
			}
		}()
		c.Next()
	}
}

func CORS() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Writer.Header().Set("Access-Control-Allow-Origin", "*")
		c.Writer.Header().Set("Access-Control-Allow-Credentials", "true")
		c.Writer.Header().Set("Access-Control-Allow-Headers", "*")
		c.Writer.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS, GET, PUT, DELETE")

		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(204)
			return
		}

		c.Next()
	}
}
__YFLOW_FILE_22__

cat > "$ROOT_DIR/model/model.go" <<'__YFLOW_FILE_23__'
package model

import (
	"fmt"
	"yflow-server/constUtil"
	"yflow-server/db"
)

type BizCode struct {
	Code int64       `json:"code"`
	Msg  string      `json:"msg"`
	Data interface{} `json:"data"`
}

func NewBizCode() *BizCode {
	return &BizCode{
		Code: 0,
		Msg:  "success",
	}
}

func NewErrBizCode(msg string) *BizCode {
	return &BizCode{
		Code: 1,
		Msg:  msg,
	}
}

func NewErrParamsError() *BizCode {
	return &BizCode{
		Code: constUtil.ErrParamsError,
		Msg:  "参数错误",
	}
}

func NewErrSystemError() *BizCode {
	return &BizCode{
		Code: constUtil.ErrSystemError,
		Msg:  "系统错误",
	}
}

func NewErrBizCodeMsg(code int64, msg string) *BizCode {
	return &BizCode{
		Code: code,
		Msg:  msg,
	}
}

func NewBizCodeData(data interface{}) *BizCode {
	return &BizCode{
		Code: 0,
		Msg:  "success",
		Data: data,
	}
}

func NewLangSystemErrBizCode() *BizCode {
	return &BizCode{
		Code: constUtil.ErrSystemError,
		Msg:  "System error",
	}
}

func (c BizCode) Error() string {
	if c.Msg != "" {
		return c.Msg
	}
	return fmt.Sprintf("%d", c.Code)
}

type Page = db.PageInfo
__YFLOW_FILE_23__

cat > "$ROOT_DIR/model/exampleModel.go" <<'__YFLOW_FILE_MODEL_EXAMPLE_MODEL__'
package model

type ExampleListRequest struct {
	FirstName string `json:"firstName"`
}

type ExampleListResponse struct {
	LastName string `json:"lastName"`
}
__YFLOW_FILE_MODEL_EXAMPLE_MODEL__

cat > "$ROOT_DIR/README.md" <<'__YFLOW_FILE_24__'
# yflow-server

Gin-based REST API server.

## Quick Start

```bash
go mod tidy
go run main.go -env=dev
```

## API Endpoints

- `GET /health` - Health check
- `GET /swagger/index.html` - API documentation
__YFLOW_FILE_24__

cat > "$ROOT_DIR/routes/registRoutes.go" <<'__YFLOW_FILE_25__'
package routes

import (
	"github.com/gin-gonic/gin"
)

// POST 路由映射
var routerPostMap = make(map[string]func(c *gin.Context))

// 注册需要认证的 POST 路由
var routerPostAuthMap = make(map[string]func(c *gin.Context))

// 注册 POST 路由
func RegisterPostRouter(url string, fn func(c *gin.Context)) {
	routerPostMap[url] = fn
}

func RegisterPostAuthRouter(url string, fn func(c *gin.Context)) {
	routerPostAuthMap[url] = fn
}

// 注册所有路由到 gin.Engine
func RouteRegister(app *gin.Engine) {
	for url, handler := range routerPostMap {
		app.POST(url, handler)
	}
	for url, handler := range routerPostAuthMap {
		app.POST(url, handler)
	}
}
__YFLOW_FILE_25__

cat > "$ROOT_DIR/service/exampleService.go" <<'__YFLOW_FILE_SERVICE_EXAMPLE_SERVICE__'
package service

import (
	"yflow-server/db"
	"yflow-server/model"

	"github.com/gin-gonic/gin"
)

func ExampleService(c *gin.Context, req model.ExampleListRequest) (resp model.ExampleListResponse, err error) {
	var lastName string
	if lastName, err = exampleFunc(c, req.FirstName); err != nil {
		return
	}

	var exampleModel db.ExampleModel
	if err = db.GetIns().WithContext(c).Where("first_name = ?", req.FirstName).First(&exampleModel).Error; err != nil {
		return
	}

	resp.LastName = lastName
	resp = model.ExampleListResponse{
		LastName: exampleModel.LastName,
	}

	return
}

func exampleFunc(c *gin.Context, firstName string) (lastName string, err error) {
	return
}

func example2Func(c *gin.Context, firstName string) (string, error) {
	return "lastName", nil
}
__YFLOW_FILE_SERVICE_EXAMPLE_SERVICE__

cat > "$ROOT_DIR/utils/crypto.go" <<'__YFLOW_FILE_26__'
package utils

import (
	"bytes"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"errors"
	"fmt"
	mrand "math/rand"
	"regexp"
	"strings"
	"time"
	"unicode"

	"golang.org/x/crypto/argon2"
)

func Base64Encode(buf []byte) string {
	return base64.StdEncoding.EncodeToString(buf)
}

func Base64Decode(str string) []byte {
	var count = (4 - len(str)%4) % 4
	str += strings.Repeat("=", count)
	buf, _ := base64.StdEncoding.DecodeString(str)
	return buf
}

func SHAWithRSA(key, data []byte) (string, error) {
	pkey, err := x509.ParsePKCS8PrivateKey(key)
	if err != nil {
		return "", err
	}

	h := crypto.Hash.New(crypto.SHA1)
	h.Write(data)
	hashed := h.Sum(nil)

	buf, err := rsa.SignPKCS1v15(rand.Reader, pkey.(*rsa.PrivateKey), crypto.SHA1, hashed)
	if err != nil {
		return "", err
	}
	return Base64Encode(buf), nil
}

func NewSalt() string {
	salt := make([]byte, 16)
	if _, err := rand.Read(salt); err != nil {
		return ""
	}

	return base64.RawStdEncoding.EncodeToString(salt)
}

type Argon2Options struct {
	Time    uint32
	Memory  uint32
	Threads uint8
	KeyLen  uint32
}

func Argon2(password, saltBase64 string) (string, error) {
	p := &Argon2Options{
		Time:    1,
		Memory:  64 * 1024,
		Threads: 4,
		KeyLen:  32,
	}

	salt, err := base64.RawStdEncoding.DecodeString(saltBase64)
	if err != nil {
		return "", err
	}

	hash := argon2.IDKey([]byte(password), salt, p.Time, p.Memory, p.Threads, p.KeyLen)

	encoded := fmt.Sprintf("$argon2id$v=19$t=%d$m=%d$p=%d$%s",
		p.Time, p.Memory, p.Threads,
		base64.RawStdEncoding.EncodeToString(hash),
	)

	return encoded, nil
}

func VerifyArgon2(password, encodedHash, saltBase64 string) (bool, error) {
	var (
		version      int
		time, memory uint32
		threads      uint8
		hashBase64   string
	)

	parts := strings.Split(encodedHash, "$")
	if len(parts) < 6 {
		return false, errors.New("invalid argon2 hash format")
	}

	if parts[1] != "argon2id" {
		return false, errors.New("invalid argon2 algorithm")
	}

	_, err := fmt.Sscanf(parts[2], "v=%d", &version)
	if err != nil {
		return false, errors.New("invalid version format")
	}
	if version != 19 {
		return false, fmt.Errorf("unsupported argon2 version: %d", version)
	}

	_, err = fmt.Sscanf(parts[3], "t=%d", &time)
	if err != nil {
		return false, errors.New("invalid time parameter")
	}
	_, err = fmt.Sscanf(parts[4], "m=%d", &memory)
	if err != nil {
		return false, errors.New("invalid memory parameter")
	}
	_, err = fmt.Sscanf(parts[5], "p=%d", &threads)
	if err != nil {
		return false, errors.New("invalid threads parameter")
	}

	if len(parts) < 7 || parts[6] == "" {
		return false, errors.New("missing hash in encoded format")
	}
	hashBase64 = parts[6]

	if time == 0 || time > 100 {
		return false, errors.New("time parameter out of valid range")
	}
	if memory == 0 || memory > 1024*1024 {
		return false, errors.New("memory parameter out of valid range")
	}
	if threads == 0 || threads > 16 {
		return false, errors.New("threads parameter out of valid range")
	}

	salt, err := base64.RawStdEncoding.DecodeString(saltBase64)
	if err != nil {
		return false, err
	}
	originalHash, err := base64.RawStdEncoding.DecodeString(hashBase64)
	if err != nil {
		return false, fmt.Errorf("invalid hash format: %w", err)
	}
	if len(originalHash) == 0 {
		return false, errors.New("hash cannot be empty after decoding")
	}

	const expectedKeyLen = 32
	if len(originalHash) != expectedKeyLen {
		return false, errors.New("hash length mismatch")
	}

	newHash := argon2.IDKey([]byte(password), salt, time, memory, threads, expectedKeyLen)

	if !bytes.Equal(newHash, originalHash) {
		return false, nil
	}
	return true, nil
}

func MaskRandomChars(s string) string {
	runes := []rune(s)
	n := len(runes)

	if n <= 1 {
		return s
	}

	r := mrand.New(mrand.NewSource(time.Now().UnixNano()))
	count := r.Intn(n-1) + 1

	replaceIndex := make(map[int]struct{})

	for len(replaceIndex) < count {
		idx := r.Intn(n)
		if _, exists := replaceIndex[idx]; !exists {
			runes[idx] = '*'
			replaceIndex[idx] = struct{}{}
		}
	}

	return string(runes)
}

func ValidatePassword(pwd string) (status int) {
	if len(pwd) < 6 {
		return 1
	}

	if strings.ContainsFunc(pwd, unicode.IsSpace) {
		return 2
	}

	hasUpper := strings.ContainsFunc(pwd, unicode.IsUpper)
	hasLower := strings.ContainsFunc(pwd, unicode.IsLower)
	hasDigit := strings.ContainsFunc(pwd, unicode.IsDigit)
	hasSpecial := strings.ContainsFunc(pwd, func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	})

	count := 0
	for _, has := range []bool{hasUpper, hasLower, hasDigit, hasSpecial} {
		if has {
			count++
		}
	}

	if count < 2 {
		return 3
	}

	return 0
}

func IsEmailValid(email string) bool {
	re := regexp.MustCompile(`^[a-zA-Z0-9._%+\-]+@[a-zA-Z0-9.\-]+\.[a-zA-Z]{2,}$`)
	return re.MatchString(email)
}

func IsPhoneValid(phone string) bool {
	re := regexp.MustCompile(`^1[3-9]\d{9}$`)
	return re.MatchString(phone)
}

func GetTrimmedCell(row []string, cellIndex int) string {
	if cellIndex < 0 || cellIndex >= len(row) {
		return ""
	}
	return strings.TrimSpace(row[cellIndex])
}

var markdownImageWithOptionalPrevNewlineRe = regexp.MustCompile(`(?:\r?\n)?!\[[^\]]*\]\([^)]*\)`)

func RemoveMarkdownImageLinks(s string) string {
	if s == "" {
		return s
	}
	return markdownImageWithOptionalPrevNewlineRe.ReplaceAllString(s, "")
}
__YFLOW_FILE_26__

cat > "$ROOT_DIR/utils/http.go" <<'__YFLOW_FILE_27__'
package utils

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
	"yflow-server/constUtil"

	"github.com/gin-gonic/gin"
	resty "resty.dev/v3"
)

type HttpClient struct {
	client  *resty.Client
	headers map[string][]string
	fields  map[string][]string
	lb      *LB
	lbKey   string
}

func NewHttp() *HttpClient {
	client := resty.New()
	client.SetTimeout(30 * time.Second)

	return &HttpClient{
		client:  client,
		headers: make(map[string][]string),
		fields:  make(map[string][]string),
	}
}

func (hc *HttpClient) WithLB(lb *LB) *HttpClient {
	hc.lb = lb
	if hc.lb != nil {
		hc.client.SetLoadBalancer(hc.lb.LoadBalancer)
	}
	return hc
}

func (hc *HttpClient) WithLBKey(key string) *HttpClient {
	hc.lbKey = key
	return hc
}

func (hc *HttpClient) WithLBKeyUserId(c *gin.Context) *HttpClient {
	userID := c.GetInt64(constUtil.CtxUid)
	if userID != 0 {
		hc.lbKey = fmt.Sprintf("%d", userID)
	}
	return hc
}

func (hc *HttpClient) SetTimeout(timeout time.Duration) *HttpClient {
	hc.client.SetTimeout(timeout)
	return hc
}

func (hc *HttpClient) SetHeader(headers map[string][]string) *HttpClient {
	hc.headers = headers
	return hc
}

func (hc *HttpClient) AddHeader(key, value string) *HttpClient {
	hc.headers[key] = append(hc.headers[key], value)
	return hc
}

func (hc *HttpClient) SetField(fields map[string][]string) *HttpClient {
	hc.fields = fields
	return hc
}

func (hc *HttpClient) AddField(key, value string) *HttpClient {
	hc.fields[key] = append(hc.fields[key], value)
	return hc
}

func (hc *HttpClient) doRequest(method, rawURL string, body interface{}) (int, []byte, error) {
	reqClient := hc.clientForRequest()
	req := reqClient.R()
	hc.applyHeaders(req)

	if body != nil {
		req.SetBody(body)
		if !hc.hasHeader("Content-Type") {
			req.SetHeader("Content-Type", "application/json")
		}
	}

	resp, err := req.Execute(method, rawURL)
	if err != nil {
		if resp != nil {
			return resp.StatusCode(), resp.Bytes(), err
		}
		return 0, []byte{}, err
	}

	return resp.StatusCode(), resp.Bytes(), nil
}

func (hc *HttpClient) Get(rawURL string, query map[string]string) (int, []byte, error) {
	if len(query) == 0 {
		return hc.doRequest(http.MethodGet, rawURL, nil)
	}

	reqClient := hc.clientForRequest()
	req := reqClient.R()
	hc.applyHeaders(req)
	req.SetQueryParams(query)

	resp, err := req.Get(rawURL)
	if err != nil {
		if resp != nil {
			return resp.StatusCode(), resp.Bytes(), err
		}
		return 0, []byte{}, err
	}

	return resp.StatusCode(), resp.Bytes(), nil
}

func (hc *HttpClient) Post(rawURL string, body interface{}) (int, []byte, error) {
	return hc.doRequest(http.MethodPost, rawURL, body)
}

func (hc *HttpClient) Put(rawURL string, body interface{}) (int, []byte, error) {
	return hc.doRequest(http.MethodPut, rawURL, body)
}

func (hc *HttpClient) Delete(rawURL string, body interface{}) (int, []byte, error) {
	return hc.doRequest(http.MethodDelete, rawURL, body)
}

func (hc *HttpClient) PostForm(rawURL string, data url.Values) (int, []byte, error) {
	reqClient := hc.clientForRequest()
	req := reqClient.R()
	hc.applyHeaders(req)

	if data != nil {
		req.SetFormDataFromValues(data)
	}

	if !hc.hasHeader("Content-Type") {
		req.SetHeader("Content-Type", "application/x-www-form-urlencoded")
	}

	resp, err := req.Post(rawURL)
	if err != nil {
		if resp != nil {
			return resp.StatusCode(), resp.Bytes(), err
		}
		return 0, []byte{}, err
	}

	return resp.StatusCode(), resp.Bytes(), nil
}

func (hc *HttpClient) UploadFile(rawURL string, fieldName, filePath string) (int, []byte, error) {
	reqClient := hc.clientForRequest()
	req := reqClient.R()
	hc.applyHeaders(req)

	req.SetFile(fieldName, filePath)

	resp, err := req.Post(rawURL)
	if err != nil {
		if resp != nil {
			return resp.StatusCode(), resp.Bytes(), err
		}
		return 0, []byte{}, err
	}

	return resp.StatusCode(), resp.Bytes(), nil
}

func (hc *HttpClient) DownloadFile(rawURL string) (string, error) {
	reqClient := hc.clientForRequest()
	resp, err := reqClient.R().Get(rawURL)
	if err != nil {
		return "", err
	}
	return resp.String(), nil
}

func (hc *HttpClient) applyHeaders(req *resty.Request) {
	if len(hc.headers) == 0 {
		return
	}
	req.SetHeaderMultiValues(hc.headers)
}

func (hc *HttpClient) hasHeader(key string) bool {
	for k := range hc.headers {
		if strings.EqualFold(k, key) {
			return true
		}
	}
	return false
}

func (hc *HttpClient) clientForRequest() *resty.Client {
	key := hc.consumeLBKey()
	if hc.lb == nil || key == "" {
		return hc.client
	}

	reqClient := hc.client.Clone(context.TODO())
	lb := hc.lb.BuildLoadBalancerForKey(key)
	if lb != nil {
		reqClient.SetLoadBalancer(lb)
	}
	return reqClient
}

func (hc *HttpClient) consumeLBKey() string {
	if hc.lbKey == "" {
		return ""
	}
	key := hc.lbKey
	hc.lbKey = ""
	return key
}

func (hc *HttpClient) PostStreamWithContext(ctx context.Context, rawURL string, body interface{}, dataChan chan<- map[string]interface{}) error {
	reqClient := hc.clientForRequest()
	req := reqClient.R().SetContext(ctx).SetResponseDoNotParse(true)
	hc.applyHeaders(req)

	if body != nil {
		req.SetBody(body)
		if !hc.hasHeader("Content-Type") {
			req.SetHeader("Content-Type", "application/json")
		}
	}

	resp, err := req.Post(rawURL)
	if err != nil {
		return fmt.Errorf("send request failed: %w", err)
	}
	if resp.Body != nil {
		defer resp.Body.Close()
	}

	if resp.StatusCode() != http.StatusOK {
		respBytes, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("HTTP %d: %s", resp.StatusCode(), string(respBytes))
	}

	contentType := resp.Header().Get("Content-Type")
	if !strings.Contains(contentType, "text/event-stream") {
		respBytes, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("unexpected content-type: %s, response: %s", contentType, string(respBytes))
	}

	done := make(chan error, 1)
	go func() {
		defer close(done)
		done <- hc.readSSEStream(ctx, resp.Body, dataChan)
	}()

	select {
	case err := <-done:
		return err
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (hc *HttpClient) readSSEStream(ctx context.Context, body io.Reader, dataChan chan<- map[string]interface{}) error {
	reader := bufio.NewReaderSize(body, 64*1024)
	var currentData strings.Builder
	var inDataBlock bool

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		line, err := reader.ReadString('\n')
		if err != nil {
			if err == io.EOF {
				if inDataBlock && currentData.Len() > 0 {
					hc.processSSEData(currentData.String(), dataChan)
				}
				return nil
			}
			return fmt.Errorf("read line failed: %w", err)
		}

		line = strings.TrimRight(line, "\r\n")

		if line == "" {
			if inDataBlock && currentData.Len() > 0 {
				hc.processSSEData(currentData.String(), dataChan)
				currentData.Reset()
				inDataBlock = false
			}
			continue
		}

		if strings.HasPrefix(line, "data: ") {
			dataStr := strings.TrimPrefix(line, "data: ")
			if currentData.Len() > 0 {
				currentData.WriteString("\n")
			}
			currentData.WriteString(dataStr)
			inDataBlock = true
		}
	}
}

func (hc *HttpClient) processSSEData(dataStr string, dataChan chan<- map[string]interface{}) error {
	if dataStr == "" {
		return nil
	}

	var data map[string]interface{}
	if err := json.Unmarshal([]byte(dataStr), &data); err != nil {
		return fmt.Errorf("unmarshal JSON failed: %w", err)
	}

	select {
	case dataChan <- data:
		return nil
	case <-time.After(10 * time.Second):
		return fmt.Errorf("send data to channel timeout")
	}
}
__YFLOW_FILE_27__

cat > "$ROOT_DIR/utils/i18n.go" <<'__YFLOW_FILE_28__'
package utils

import (
	"path/filepath"

	"github.com/nicksnyder/go-i18n/v2/i18n"
	"golang.org/x/text/language"
	"gopkg.in/yaml.v3"
)

type I18nClient struct {
	bundle *i18n.Bundle
	lang   string
	accept string
}

var i18nCli *I18nClient

func InitI18n() error {
	i18nCli = new(I18nClient)

	i18nCli.bundle = i18n.NewBundle(language.English)
	i18nCli.bundle.RegisterUnmarshalFunc("yaml", yaml.Unmarshal)

	matches, err := filepath.Glob("locales/active.*.yaml")
	if err != nil {
		return err
	}

	for _, match := range matches {
		if _, err = i18nCli.bundle.LoadMessageFile(match); err != nil {
			return err
		}
	}

	return nil
}

func GetI18nClient() *I18nClient {
	return i18nCli
}

func (cli *I18nClient) SetLang(lang string) {
	cli.lang = lang
}

func (cli *I18nClient) SetAccept(accept string) {
	cli.accept = accept
}

func (cli *I18nClient) Greet(messageID string, templateData map[string]interface{}) string {
	if cli.lang == "" && cli.accept == "" {
		cli.lang = "en"
	}
	localizer := i18n.NewLocalizer(cli.bundle, cli.lang, cli.accept)
	msg, _ := localizer.Localize(&i18n.LocalizeConfig{
		MessageID:    messageID,
		TemplateData: templateData,
	})
	return msg
}
__YFLOW_FILE_28__

cat > "$ROOT_DIR/utils/jwtUtils.go" <<'__YFLOW_FILE_29__'
package utils

import (
	"errors"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

type BClaims struct {
	AdminId   int64
	AdminRole string
	jwt.RegisteredClaims
}

type CClaims struct {
	UserId  int64
	LoginAt int64
	jwt.RegisteredClaims
}

type ResetClaims struct {
	Purpose int64
	UserId  int64
	jwt.RegisteredClaims
}

func GenBToken(jwtKey string, claims BClaims) (string, error) {
	mySigningKey := []byte(jwtKey)
	claims.RegisteredClaims = jwt.RegisteredClaims{
		Issuer:  "admin-system",
		IssuedAt: jwt.NewNumericDate(time.Now()),
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(mySigningKey)
}

func GenCToken(jwtKey string, expireHour int64, claims CClaims) (string, error) {
	nw := time.Now()
	mySigningKey := []byte(jwtKey)
	claims.RegisteredClaims = jwt.RegisteredClaims{
		Issuer:    "user-system",
		ExpiresAt: jwt.NewNumericDate(nw.Add(time.Duration(expireHour) * time.Hour)),
		IssuedAt:  jwt.NewNumericDate(nw),
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(mySigningKey)
}

func ParseBToken(jwtKey, tokenString string) (*BClaims, error) {
	mySigningKey := []byte(jwtKey)
	token, err := jwt.ParseWithClaims(tokenString, &BClaims{}, func(token *jwt.Token) (interface{}, error) {
		return mySigningKey, nil
	})
	if err != nil {
		return nil, err
	}
	if claims, ok := token.Claims.(*BClaims); ok && token.Valid {
		return claims, nil
	}
	return nil, errors.New("invalid token")
}

func ParseCToken(jwtKey, tokenString string) (*CClaims, error) {
	mySigningKey := []byte(jwtKey)
	token, err := jwt.ParseWithClaims(tokenString, &CClaims{}, func(token *jwt.Token) (interface{}, error) {
		return mySigningKey, nil
	})
	if err != nil {
		return nil, err
	}
	if claims, ok := token.Claims.(*CClaims); ok && token.Valid {
		return claims, nil
	}
	return nil, errors.New("invalid token")
}

func GenResetToken(jwtKey string, expireMinute int64, claims ResetClaims) (string, error) {
	nw := time.Now()
	mySigningKey := []byte(jwtKey)
	claims.RegisteredClaims = jwt.RegisteredClaims{
		Issuer:    "user-system",
		ExpiresAt: jwt.NewNumericDate(nw.Add(time.Duration(expireMinute) * time.Minute)),
		IssuedAt:  jwt.NewNumericDate(nw),
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(mySigningKey)
}

func ParseResetToken(jwtKey, tokenString string) (*ResetClaims, error) {
	mySigningKey := []byte(jwtKey)
	token, err := jwt.ParseWithClaims(tokenString, &ResetClaims{}, func(token *jwt.Token) (interface{}, error) {
		return mySigningKey, nil
	})
	if err != nil {
		return nil, err
	}
	if claims, ok := token.Claims.(*ResetClaims); ok && token.Valid {
		return claims, nil
	}
	return nil, errors.New("invalid token")
}
__YFLOW_FILE_29__

cat > "$ROOT_DIR/utils/lb.go" <<'__YFLOW_FILE_30__'
package utils

import (
	"context"
	"fmt"
	"hash/fnv"
	"math/rand"
	"net/url"
	"os/exec"
	"strings"
	"sync"
	"time"

	"resty.dev/v3"
)

type LB struct {
	LBServers      []string
	InitialServers []string
	LastSelected   string
	LoadBalancer   resty.LoadBalancer
	serversVersion uint64
	mu             *sync.Mutex
	healthStop     chan struct{}
	healthRunning  bool
	retryBaseURL   string
	retryLeft      int
	retryUntil     time.Time
}

type LBOption func(*LB)

func (lb *LB) BuildLoadBalancerForKey(key string) resty.LoadBalancer {
	if lb == nil || lb.LoadBalancer == nil {
		return nil
	}

	switch lb.LoadBalancer.(type) {
	case *hashLB:
		return newHashLB(lb, key)
	default:
		return lb.LoadBalancer
	}
}

func NewLB(servers []string, opts ...LBOption) *LB {
	normalized, err := normalizeBaseURLs(servers)
	if err != nil || len(normalized) == 0 {
		normalized = copyStrings(servers)
	}
	lb := &LB{
		LBServers:      normalized,
		InitialServers: copyStrings(normalized),
		serversVersion: 1,
		mu:             &sync.Mutex{},
		healthStop:     make(chan struct{}),
	}
	for _, opt := range opts {
		if opt != nil {
			opt(lb)
		}
	}
	if lb.LoadBalancer == nil {
		LBRandom()(lb)
	}
	lb.startHealthCheck()
	return lb
}

func LBRandom() LBOption {
	return func(lb *LB) {
		if lb == nil {
			return
		}
		lb.LoadBalancer = newRandomLB(lb)
	}
}

func LBRoundRobin() LBOption {
	return func(lb *LB) {
		if lb == nil {
			return
		}
		lb.LoadBalancer = newRoundRobinLB(lb)
	}
}

func LBHash(key string) LBOption {
	return func(lb *LB) {
		if lb == nil {
			return
		}
		lb.LoadBalancer = newHashLB(lb, key)
	}
}

func (lb *LB) startHealthCheck() {
	lb.mu.Lock()
	if lb.healthRunning {
		lb.mu.Unlock()
		return
	}
	lb.healthRunning = true
	stopCh := lb.healthStop
	lb.mu.Unlock()

	go func() {
		ticker := time.NewTicker(1 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				lb.refreshServers()
			case <-stopCh:
				return
			}
		}
	}()
}

func (lb *LB) StopHealthCheck() {
	lb.mu.Lock()
	if !lb.healthRunning {
		lb.mu.Unlock()
		return
	}
	lb.healthRunning = false
	close(lb.healthStop)
	lb.healthStop = make(chan struct{})
	lb.mu.Unlock()
}

func (lb *LB) refreshServers() {
	current := lb.snapshotServers()
	initial := lb.snapshotInitialServers()

	results := lb.probeServers(append(current, initial...))

	aliveSet := make(map[string]struct{}, len(results))
	for srv, ok := range results {
		if ok {
			aliveSet[srv] = struct{}{}
		}
	}

	newList := make([]string, 0, len(aliveSet))
	initialSet := make(map[string]struct{}, len(initial))
	for _, srv := range initial {
		initialSet[srv] = struct{}{}
		if _, ok := aliveSet[srv]; ok {
			newList = append(newList, srv)
		}
	}

	for _, srv := range current {
		if _, ok := aliveSet[srv]; !ok {
			continue
		}
		if _, ok := initialSet[srv]; ok {
			continue
		}
		newList = append(newList, srv)
	}

	lb.updateServers(newList)
}

func (lb *LB) snapshotServers() []string {
	lb.mu.Lock()
	defer lb.mu.Unlock()
	return copyStrings(lb.LBServers)
}

func (lb *LB) snapshotInitialServers() []string {
	lb.mu.Lock()
	defer lb.mu.Unlock()
	return copyStrings(lb.InitialServers)
}

func (lb *LB) probeServers(servers []string) map[string]bool {
	result := make(map[string]bool, len(servers))
	seen := make(map[string]struct{}, len(servers))
	for _, srv := range servers {
		if _, ok := seen[srv]; ok {
			continue
		}
		seen[srv] = struct{}{}
		result[srv] = probeServer(srv)
	}
	return result
}

func probeServer(server string) bool {
	urlStr := normalizeProbeURL(server)
	ctx, cancel := context.WithTimeout(context.TODO(), 800*time.Millisecond)
	defer cancel()

	cmd := exec.CommandContext(ctx, "curl", "-s", "-o", "/dev/null", "-w", "%{http_code}", urlStr)
	output, err := cmd.Output()
	if err != nil {
		return false
	}
	return strings.TrimSpace(string(output)) == "200"
}

func normalizeProbeURL(server string) string {
	if !strings.HasPrefix(server, "http://") && !strings.HasPrefix(server, "https://") {
		server = "http://" + server
	}
	u, err := url.Parse(server)
	if err != nil {
		return server
	}
	if u.Path == "" || u.Path == "/" {
		u.Path = "/health"
	}
	return u.String()
}

func copyStrings(src []string) []string {
	if len(src) == 0 {
		return nil
	}
	dst := make([]string, len(src))
	copy(dst, src)
	return dst
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func (lb *LB) updateServers(servers []string) {
	lb.mu.Lock()
	defer lb.mu.Unlock()
	if equalStrings(lb.LBServers, servers) {
		return
	}
	lb.LBServers = servers
	lb.serversVersion++
}

func (lb *LB) currentServersVersion() uint64 {
	lb.mu.Lock()
	defer lb.mu.Unlock()
	return lb.serversVersion
}

func normalizeBaseURLs(baseURLs []string) ([]string, error) {
	normalized := make([]string, 0, len(baseURLs))
	for _, raw := range baseURLs {
		raw = strings.TrimSpace(raw)
		if raw == "" {
			continue
		}
		if !strings.Contains(raw, "://") {
			raw = "http://" + raw
		}
		u, err := url.Parse(raw)
		if err != nil {
			return nil, err
		}
		u.Path = ""
		u.RawQuery = ""
		normalized = append(normalized, strings.TrimRight(u.String(), "/"))
	}
	return normalized, nil
}

type roundRobinLB struct {
	parent *LB
	mu     sync.Mutex
	idx    int
}

func newRoundRobinLB(parent *LB) *roundRobinLB {
	return &roundRobinLB{parent: parent}
}

func (lb *roundRobinLB) NextWithContext(ctx context.Context) (string, error) {
	select {
	case <-ctx.Done():
		return "", ctx.Err()
	default:
	}
	return lb.parent.nextWithRetry(func(servers []string) (string, error) {
		lb.mu.Lock()
		defer lb.mu.Unlock()
		baseURL := servers[lb.idx%len(servers)]
		lb.idx = (lb.idx + 1) % len(servers)
		return baseURL, nil
	})
}

func (lb *roundRobinLB) Feedback(f *resty.RequestFeedback) { lb.parent.handleFeedback(f) }
func (lb *roundRobinLB) Close() error                      { return nil }

type randomLB struct {
	parent *LB
	mu     sync.Mutex
	rand   *rand.Rand
}

func newRandomLB(parent *LB) *randomLB {
	return &randomLB{
		parent: parent,
		rand:   rand.New(rand.NewSource(time.Now().UnixNano())),
	}
}

func (lb *randomLB) NextWithContext(ctx context.Context) (string, error) {
	select {
	case <-ctx.Done():
		return "", ctx.Err()
	default:
	}
	return lb.parent.nextWithRetry(func(servers []string) (string, error) {
		lb.mu.Lock()
		defer lb.mu.Unlock()
		return servers[lb.rand.Intn(len(servers))], nil
	})
}

func (lb *randomLB) Feedback(f *resty.RequestFeedback) { lb.parent.handleFeedback(f) }
func (lb *randomLB) Close() error                      { return nil }

type hashLB struct {
	parent *LB
	key    string
}

func newHashLB(parent *LB, key string) *hashLB {
	return &hashLB{parent: parent, key: key}
}

func (lb *hashLB) NextWithContext(ctx context.Context) (string, error) {
	select {
	case <-ctx.Done():
		return "", ctx.Err()
	default:
	}
	return lb.parent.nextWithRetry(func(servers []string) (string, error) {
		if lb.key == "" {
			return servers[0], nil
		}
		idx := int(hashString(lb.key) % uint32(len(servers)))
		return servers[idx], nil
	})
}

func (lb *hashLB) Feedback(f *resty.RequestFeedback) { lb.parent.handleFeedback(f) }
func (lb *hashLB) Close() error                      { return nil }

func hashString(s string) uint32 {
	h := fnv.New32a()
	_, _ = h.Write([]byte(s))
	return h.Sum32()
}

func (lb *LB) setLastSelected(server string) {
	lb.mu.Lock()
	lb.LastSelected = server
	lb.mu.Unlock()
}

func (lb *LB) nextWithRetry(pick func([]string) (string, error)) (string, error) {
	if baseURL, ok := lb.consumeRetry(); ok {
		lb.setLastSelected(baseURL)
		return baseURL, nil
	}

	servers := lb.snapshotServers()
	if len(servers) == 0 {
		return "", fmt.Errorf("load balancer: no base urls")
	}

	baseURL, err := pick(servers)
	if err != nil {
		return "", err
	}
	lb.setLastSelected(baseURL)
	return baseURL, nil
}

func (lb *LB) consumeRetry() (string, bool) {
	lb.mu.Lock()
	defer lb.mu.Unlock()
	if lb.retryLeft <= 0 || lb.retryBaseURL == "" {
		return "", false
	}
	if time.Now().After(lb.retryUntil) {
		lb.retryLeft = 0
		lb.retryBaseURL = ""
		return "", false
	}
	if !containsString(lb.LBServers, lb.retryBaseURL) {
		lb.retryLeft = 0
		lb.retryBaseURL = ""
		return "", false
	}
	lb.retryLeft--
	return lb.retryBaseURL, true
}

func (lb *LB) handleFeedback(f *resty.RequestFeedback) {
	if f == nil {
		return
	}
	if f.Success {
		lb.clearRetry()
		return
	}
	if f.BaseURL == "" {
		return
	}

	lb.mu.Lock()
	lb.retryBaseURL = f.BaseURL
	lb.retryLeft = 3
	lb.retryUntil = time.Now().Add(2 * time.Second)
	lb.mu.Unlock()
}

func (lb *LB) clearRetry() {
	lb.mu.Lock()
	lb.retryLeft = 0
	lb.retryBaseURL = ""
	lb.mu.Unlock()
}

func containsString(list []string, value string) bool {
	for _, item := range list {
		if item == value {
			return true
		}
	}
	return false
}
__YFLOW_FILE_30__

cat > "$ROOT_DIR/utils/uuid.go" <<'__YFLOW_FILE_31__'
package utils

import (
	"crypto/rand"
	"fmt"
	"math/big"
	"strings"

	"github.com/google/uuid"
	"github.com/speps/go-hashids"
)

func UnsignedUUID() string {
	id := uuid.New()
	return strings.ReplaceAll(id.String(), "-", "")
}

func InviteCode(uid int64) string {
	hd := hashids.NewData()
	hd.Salt = "inviteCode"
	hd.MinLength = 6
	h, _ := hashids.NewWithData(hd)
	code, _ := h.EncodeInt64([]int64{uid})
	return code
}

func VerificationCode() string {
	maxValue := big.NewInt(1000000)
	n, err := rand.Int(rand.Reader, maxValue)
	if err != nil {
		return ""
	}
	return fmt.Sprintf("%06d", n.Int64())
}
__YFLOW_FILE_31__

cat > "$ROOT_DIR/wrapper/wrapper.go" <<'__YFLOW_FILE_32__'
package wrapper

import (
	"errors"
	"fmt"
	"net/http"
	"reflect"
	"runtime"
	"yflow-server/elog"
	"yflow-server/model"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// Wrapper - fn returns model.BizCode or other error
func Wrapper(fn func(c *gin.Context) error) gin.HandlerFunc {
	return func(c *gin.Context) {
		if err := fn(c); err != nil {
			var bizCode = new(model.BizCode)
			if errors.As(err, &bizCode) {
				c.JSON(http.StatusOK, bizCode)
				return
			}
			elog.Error(fmt.Sprintf("%s error.", runtime.FuncForPC(reflect.ValueOf(fn).Pointer()).Name()), zap.Error(err))
			c.JSON(http.StatusOK, model.NewErrBizCode("系统错误"))
			return
		}
	}
}

// I18nWrapper - fn returns i18n model.BizCode or other error
func I18nWrapper(fn func(c *gin.Context) error) gin.HandlerFunc {
	return func(c *gin.Context) {
		if err := fn(c); err != nil {
			var bizCode = new(model.BizCode)
			if errors.As(err, &bizCode) {
				c.JSON(http.StatusOK, bizCode)
				return
			}
			elog.Error(fmt.Sprintf("%s error.", runtime.FuncForPC(reflect.ValueOf(fn).Pointer()).Name()), zap.Error(err))
			c.JSON(http.StatusOK, model.NewLangSystemErrBizCode())
			return
		}
	}
}
__YFLOW_FILE_32__

# 3) 根据当前项目名和本机 Go 版本改写模板占位内容
find "$ROOT_DIR" -type f \( -name '*.go' -o -name 'go.mod' -o -name 'README.md' -o -name '.gitignore' \) -exec sed -i.bak "s|yflow-server|${MODULE_NAME_SED}|g" {} +
find "$ROOT_DIR" -type f -name '*.bak' -delete
sed -i.bak "s/^go [0-9][0-9.]*$/go ${GO_VERSION}/" "$ROOT_DIR/go.mod"
rm -f "$ROOT_DIR/go.mod.bak"

# 4) 补充可执行权限（与原项目一致）
chmod +x "$ROOT_DIR/i18n.sh"

# 5) 格式化、编译级校验、依赖更新与最终校验
(
  cd "$ROOT_DIR"
  find . -name '*.go' -type f -exec gofmt -w {} +
  go test ./...
  go get -u ./...
  go mod tidy
  go test ./...
)

echo "${ROOT_DIR} created."