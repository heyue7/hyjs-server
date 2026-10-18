package constUtil

const (
	P14FinalDrawSource        = "local-complete-final32-common-hash-draw-v1; original81046-groups-A-D; real-eight-slot-slices-adjacent-pairs; no-filler-no-bye-result"
	P14FinalDrawRules         = "sha256(commonQualificationHash/serverID/playerID)-asc; tie=serverID-playerID-asc; contiguous8=A-D; adjacent-real-pairs; no-filled-player; no-bye-win; first-round-only"
	P14FinalDrawCorrupt       = "决赛真实分组与完整资格及提前发布抽签规则不符"
	P14FinalDrawGroupCapacity = 8
)
