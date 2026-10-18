package constUtil

const (
	P14FinalGameSource           = "local-final-first-game-sealed-origins-v1; original-first-round8-only; sealed-last-purpose3-or-frozen-promotion-roster; no-series-result"
	P14FinalGameRules            = "all-actual-origin-seals-after-lock; first-real-adjacent-pair; seed=sha256(warId)-positive63bit; max40; original-room-fallback; no-result"
	P14FinalGameCorrupt          = "决赛首局来源封存与实际锁定阵容不符"
	P14FinalGamePending          = "决赛首局尚未收到全部来源服锁定阵容"
	P14FinalGameProtocol   int32 = 1
	P14FinalGameImportPath       = "/internal/p14/final-seal"
	P14FinalGameMaxRound   int32 = 40
	P14FinalSealFinal      int32 = 3
	P14FinalSealPromotion  int32 = 2
	P14FinalSealRoster     int32 = 1
)
