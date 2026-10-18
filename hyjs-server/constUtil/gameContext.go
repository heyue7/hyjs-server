package constUtil

// GameContextKey 隔离游戏请求的业务编排对象与协议上下文。
type GameContextKey string

const (
	GameServiceContextKey GameContextKey = "game-service"
	GameRequestContextKey GameContextKey = "game-request"
)
