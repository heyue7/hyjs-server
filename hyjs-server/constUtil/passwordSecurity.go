package constUtil

import "time"

// 密码凭据参数固定为当前授权的 Argon2id 方案；修改常量会改变新凭据格式，需要同步验证既有数据。
const (
	PasswordSaltBytes            = 16
	PasswordArgon2MemoryKiB      = 19456
	PasswordArgon2Iterations     = 2
	PasswordArgon2Threads        = 1
	PasswordArgon2KeyBytes       = 32
	PasswordCredentialMaxBytes   = 1024
	PasswordCredentialSlots      = 2
	PasswordCredentialMaxWaiters = 128
	PasswordCredentialWait       = 10 * time.Second
)

// 旧游戏密码使用 scrypt N=16384/r=8/p=1，仅用于验证和登录后的自动升级。
const (
	PasswordLegacyScryptN         = 16384
	PasswordLegacyScryptR         = 8
	PasswordLegacyScryptP         = 1
	PasswordLegacyScryptKeyBytes  = 32
	PasswordLegacyScryptSaltBytes = 16
)

// 旧后台密码使用 bcrypt；只接受当前部署产生的合理成本区间，避免异常摘要消耗过量 CPU。
const (
	PasswordLegacyBcryptMinCost = 10
	PasswordLegacyBcryptMaxCost = 14
)
