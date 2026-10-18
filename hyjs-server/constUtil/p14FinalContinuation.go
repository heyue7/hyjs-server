package constUtil

const (
	P14ContinuationResultProtocolVersion int64 = 1
	P14FinalContinuationSource                 = "local-kf-series-game2to5-fresh-upload-v1; source0-left-realm; original-bestof5-first3; each-slot-real-seals; no-HP-anger-carry; no-forfeit-no-prize"
	P14FinalContinuationRules                  = "local-series-continuation-v1; game2..5; exact-current-slot-fresh-P5-upload; own-origin-seals; previous-canonical-sourceResultHash; deterministic-new-input-seed; controlled-joint-wrapper; independent-KF-proof; three-real-wins-stop"
	P14FinalContinuationBattleSource           = "p14-final-kf-continuation"
	P14FinalContinuationImportPath             = "/internal/p14/final-continuation"
	P14FinalContinuationPending                = "系列赛仍等待当前局真实来源或证书"
	P14FinalContinuationCorrupt                = "系列赛当前局与真实来源或终局证书不符"
)
