package constUtil

const (
	P14FinalResultCodec                 = "gzip-json-v1"
	P14FinalResultSource                = "local-signed-canonical-KF-terminal-v1; only-source0-realm; real-joint-winner; independently-certified-terminal; peer-series-only-no-foreign-P6-owner"
	P14FinalResultRules                 = "game1-only; actual165acceptedresult; left-origin-HMAC; full-source-input-and-terminal-digests; exact-series-policy-wrapper-proof; no-peer-P6record; bestof5-one-accepted-win"
	P14FinalResultImportPath            = "/internal/p14/final-result"
	P14FinalResultProtocolVersion int64 = 1
	P14FinalResultMaxDecodedBytes int64 = 32 << 20
	P14FinalResultPending               = "系列赛尚未收到来源服真实认证终局"
	P14FinalResultCorrupt               = "来源服终局与本场联合身份、阵容源或独立证书不符"
)
