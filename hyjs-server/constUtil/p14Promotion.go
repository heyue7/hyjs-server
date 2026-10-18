package constUtil

const (
	// Original UI.ServerBattleState.Promotion=6; 7 is Final16_Before.
	P14PromotionStage int32 = 6
	// Legacy stage7 is accepted only by frozen history readers, never new producers.
	P14LegacyPromotionStage      int32 = 7
	P14Final16BeforeStage        int32 = 7
	P14Final16Stage              int32 = 8
	P14PromotionSource                 = "local-original-qualified-promotion-points-v1"
	P14PromotionFirstGrantSource       = "local-first-noon-not-before-stage-and-qualification-v1"
	P14PromotionMaxStageMillis   int64 = 14 * 24 * 60 * 60 * 1000
)
