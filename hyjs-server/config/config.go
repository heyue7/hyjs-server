package config

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/viper"
)

// GameConfig 定义本地兼容服监听地址、存档和 APK 配置表来源。
type GameConfig struct {
	// AccountsDatabase optionally selects a pre-created shared identity schema on the same MySQL instance; empty uses the realm database.
	AccountsDatabase string `mapstructure:"accountsDatabase"`
	// ServerID 来自部署配置，标识该独立存档区服；客户端不能修改。
	ServerID      int64  `mapstructure:"serverId"`
	Bind          string `mapstructure:"bind"`
	AdvertiseHost string `mapstructure:"advertiseHost"`
	HTTPPort      int    `mapstructure:"httpPort"`
	TCPPort       int    `mapstructure:"tcpPort"`
	Driver        string `mapstructure:"driver"`
	Database      string `mapstructure:"database"`
	Tables        string `mapstructure:"tables"`
	// EnableTestPayments 仅启用loopback隔离测试确认，真实收费始终独立关闭。
	EnableTestPayments bool `mapstructure:"enableTestPayments"`
	// EnableLocalWallet opens demo wallet debit and original order confirmation in dev/test.
	EnableLocalWallet bool `mapstructure:"enableLocalWallet"`
}

type AppConfig struct {
	Security      SecurityConfig `mapstructure:"security"`
	Game          GameConfig     `mapstructure:"game"`
	Port          string         `mapstructure:"port"`
	LogConfig     LogConfig      `mapstructure:"logConfig"`
	JwtKey        string         `mapstructure:"jwtKey"`
	MysqlConfig   MysqlConfig    `mapstructure:"mysql"`
	Redis         RedisConfig    `mapstructure:"redis"`
	StorageConfig StorageConfig  `mapstructure:"storageConfig"`
}

// SecurityConfig binds deployment trust explicitly; it does not enable MFA or change session lifetimes.
type SecurityConfig struct {
	// RushHistoryCertificateKey authenticates derived history certificates; empty disables reuse.
	RushHistoryCertificateKey string `mapstructure:"rushHistoryCertificateKey"`
	TrustedProxies            string `mapstructure:"trustedProxies"`
	AdminSecureCookies        bool   `mapstructure:"adminSecureCookies"`
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
	StorageName     string `mapstructure:"storageName"`
	BucketName      string `mapstructure:"BucketName"`
	CosRegion       string `mapstructure:"cosRegion"`
	CosAppId        string `mapstructure:"cosAppId"`
	CosSecretId     string `mapstructure:"cosSecretId"`
	CosSecretKey    string `mapstructure:"cosSecretKey"`
	OssEndPoint     string `mapstructure:"ossEndPoint"`
	OssAccessKey    string `mapstructure:"ossAccessKey"`
	OssAccessSecret string `mapstructure:"ossAccessSecret"`
}

var AppConfigIns *AppConfig
var Env string

// Init 先读取私有环境配置，再应用环境变量；入口的显式 CLI 参数继续具有最高优先级。
func Init(env string) error {
	env = strings.ToLower(strings.TrimSpace(env))
	if env == "" {
		env = "dev"
	}
	reader := viper.New()
	// 独立 reader 避免重复初始化时继承其他环境的密码或配置键。
	if file := strings.TrimSpace(os.Getenv("HYJS_CONFIG_FILE")); file != "" {
		reader.SetConfigFile(file)
		if err := reader.ReadInConfig(); err != nil {
			return err
		}
	} else {
		reader.SetConfigFile("config." + env + ".yaml")
		if err := reader.ReadInConfig(); err != nil {
			reader.SetConfigFile("config.yaml")
			if err2 := reader.ReadInConfig(); err2 != nil {
				return err
			}
		}
	}
	// 显式绑定使仅由环境注入、未写入 YAML 的字段也能参与反序列化。
	reader.AllowEmptyEnv(true)
	for key, variable := range environmentBindings {
		if err := reader.BindEnv(key, variable); err != nil {
			return err
		}
	}
	cfg := new(AppConfig)
	if err := reader.Unmarshal(cfg); err != nil {
		return fmt.Errorf("invalid application configuration: %w", err)
	}
	if cfg.Game.Driver == "" {
		cfg.Game.Driver = "mysql"
	}
	Env = env
	AppConfigIns = cfg
	return nil
}

// environmentBindings 公开变量名合同；实际部署值只保存在私有 YAML 或进程环境中。
var environmentBindings = map[string]string{
	"security.rushHistoryCertificateKey": "HYJS_RUSH_HISTORY_CERTIFICATE_KEY",
	"security.trustedProxies":            "HYJS_TRUSTED_PROXIES",
	"security.adminSecureCookies":        "HYJS_ADMIN_SECURE_COOKIES",
	"game.accountsDatabase":              "HYJS_GAME_ACCOUNTS_DATABASE",
	"game.serverId":                      "HYJS_GAME_SERVER_ID",
	"game.bind":                          "HYJS_GAME_BIND",
	"game.advertiseHost":                 "HYJS_GAME_ADVERTISE_HOST",
	"game.httpPort":                      "HYJS_GAME_HTTP_PORT",
	"game.tcpPort":                       "HYJS_GAME_TCP_PORT",
	"game.driver":                        "HYJS_GAME_DRIVER",
	"game.database":                      "HYJS_GAME_DATABASE",
	"game.tables":                        "HYJS_GAME_TABLES",
	"game.enableTestPayments":            "HYJS_GAME_ENABLE_TEST_PAYMENTS",
	"game.enableLocalWallet":             "HYJS_GAME_ENABLE_LOCAL_WALLET",
	"mysql.host":                         "HYJS_MYSQL_HOST",
	"mysql.user":                         "HYJS_MYSQL_USER",
	"mysql.pass":                         "HYJS_MYSQL_PASSWORD",
	"mysql.dbName":                       "HYJS_MYSQL_DATABASE",
	"port":                               "HYJS_PORT",
	"jwtKey":                             "HYJS_JWT_KEY",
	"redis.host":                         "HYJS_REDIS_HOST",
	"redis.password":                     "HYJS_REDIS_PASSWORD",
	"redis.db":                           "HYJS_REDIS_DB",
	"logConfig.logFile":                  "HYJS_LOG_FILE",
	"logConfig.gormLogFile":              "HYJS_GORM_LOG_FILE",
	"logConfig.logLevel":                 "HYJS_LOG_LEVEL",
	"logConfig.gormLogLevel":             "HYJS_GORM_LOG_LEVEL",
	"logConfig.gormSlowThresholdMs":      "HYJS_GORM_SLOW_THRESHOLD_MS",
	"logConfig.maxSize":                  "HYJS_LOG_MAX_SIZE",
	"logConfig.maxBackups":               "HYJS_LOG_MAX_BACKUPS",
	"logConfig.maxAge":                   "HYJS_LOG_MAX_AGE",
	"logConfig.compress":                 "HYJS_LOG_COMPRESS",
	"storageConfig.storageName":          "HYJS_STORAGE_NAME",
	"storageConfig.bucketName":           "HYJS_STORAGE_BUCKET_NAME",
	"storageConfig.cosRegion":            "HYJS_COS_REGION",
	"storageConfig.cosAppId":             "HYJS_COS_APP_ID",
	"storageConfig.cosSecretId":          "HYJS_COS_SECRET_ID",
	"storageConfig.cosSecretKey":         "HYJS_COS_SECRET_KEY",
	"storageConfig.ossEndPoint":          "HYJS_OSS_ENDPOINT",
	"storageConfig.ossAccessKey":         "HYJS_OSS_ACCESS_KEY",
	"storageConfig.ossAccessSecret":      "HYJS_OSS_ACCESS_SECRET",
}
