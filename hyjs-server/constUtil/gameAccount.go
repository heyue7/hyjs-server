package constUtil

import "errors"

const (
	GameAccountUsernameMinBytes              = 3
	GameAccountUsernameMaxBytes              = 32
	GameAccountPasswordMinCharacters         = 6
	GameAccountPasswordMaxBytes              = 72
	GameAccountRegisterBodyBytes             = 4096
	GameAccountRegisterWindowSeconds   int64 = 60
	GameAccountRegisterPerWindow             = 5
	GameAccountRegisterMaxClients            = 10000
	GameAccountRegisterHashConcurrency       = 2
)

var (
	ErrGameAccountExists          = errors.New("账号已存在，请使用其他账号名或直接登录")
	ErrGameAccountRegisterLimited = errors.New("注册操作过于频繁，请稍后重试")
)
