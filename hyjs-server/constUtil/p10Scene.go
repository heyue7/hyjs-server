package constUtil

import "time"

const (
	P10ScenePolicyID       int64 = 1
	P10SceneCleanupTimeout       = 5 * time.Second
	P10SceneClosed               = "有限多人分线尚未开放"
	P10SceneSource               = "原MovePlayer/SCPlayerAppear/Disappear/SCMovePush及GetLine/TransferLine，LineInfo.state=2繁忙、其余空闲；本地有限town_main与原allSafe field_001，2条分线/100人数为明确兼容运营值。原实例内玩家不加入公共同屏；在线资格只来自当前真实TCP会话，不从持久化profile伪造在线玩家，默认关闭。"
)
