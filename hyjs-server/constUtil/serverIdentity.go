package constUtil

import "time"

// GameDefaultServerID 保留既有本地部署与未注入区服的隔离服务默认编号。
const GameDefaultServerID int64 = 1

// GameCreatePlayerMaxAttempts 对已回滚的InnoDB死锁重做完整幂等创建事务。
const GameCreatePlayerMaxAttempts = 4

// GameCreatePlayerRetryDelay 错开不同角色的首次公共进度写入竞争。
const GameCreatePlayerRetryDelay = 20 * time.Millisecond

// GameLocalMutationMaxAttempts 仅重试已回滚、无外部副作用的已审计本地交易请求。
const GameLocalMutationMaxAttempts = 4

// GameLocalMutationRetryDelay 错开共用进度行竞争，等待沿请求ctx取消。
const GameLocalMutationRetryDelay = 20 * time.Millisecond
