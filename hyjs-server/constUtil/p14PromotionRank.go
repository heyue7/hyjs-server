package constUtil

const (
	P14PromotionRankSource            = "local-signed-accepted-promotion-score-v1; originaltype2; own-active-spends-only; qualified-roster-power; prepublished-score-desc-power-desc-server-asc-player-asc"
	P14PromotionRankRules             = `{"order":"scoreDesc,powerDesc,serverAsc,playerAsc","powerSource":"qualifiedRoster","initialScore":1,"winDelta":15,"lossDelta":0}`
	P14PromotionRankProtocol    int64 = 1
	P14PromotionRankPath              = "/internal/p14/promotion-rank"
	P14PromotionRankClosed            = "跨服晋级积分榜尚未发布或来源未同步"
	P14PromotionRankCorrupt           = "跨服晋级积分榜与真实资格或计分版本不符"
	P14PromotionRankPeerLimit         = 64
	P14PromotionRankBatch             = 32
	P14PromotionRankMaxAccepted int64 = 280
	P14PromotionRankWinDelta    int64 = 15
)
