package constUtil

const (
	CookingMakeOpcode          int32 = 24713
	CookingUseOpcode           int32 = 24715
	CookingMaxBatch            int32 = 1000
	CookingLegacyGemHalfSource       = "User-approved gem-half-v1 permanent cooking: each affected attribute gains 50% of the corresponding ordinary gem at dish level 1/2/4/6; permanent3 golden meal uses gem level 10. Provisional accumulated attribute caps use approximately 20/40/60 servings; golden final cap remains unconfigured. Original recipes, gates and timed effects remain unchanged."
	CookingLocalRuleSource           = "User-approved gem-half-golden80-v2 permanent cooking: each affected attribute gains 50% of the corresponding ordinary gem at dish level 1/2/4/6; permanent3 golden meal uses gem level 10. Provisional accumulated attribute caps use approximately 20/40/60 servings; golden final accumulated cap uses approximately 80 level10 half-gem servings across the same existing attribute totals. Original recipes, gates and timed effects remain unchanged."
	CookingLegacyGemHalfGains        = `{"gemScale":0.5,"requireLimits":1,"limit2.att":2000,"limit2.hp":15200,"limit2.pdef":600,"limit2.mdef":1200,"limit2.cri":6,"limit2.antiCri":12,"limit4.att":20000,"limit4.hp":76000,"limit4.pdef":6000,"limit4.mdef":6000,"limit4.cri":48,"limit4.antiCri":48,"limit6.att":45000,"limit6.hp":171000,"limit6.pdef":13500,"limit6.mdef":13500,"limit6.cri":108,"limit6.antiCri":108}`
	CookingDefaultGains              = `{"gemScale":0.5,"requireLimits":1,"limit2.att":2000,"limit2.hp":15200,"limit2.pdef":600,"limit2.mdef":1200,"limit2.cri":6,"limit2.antiCri":12,"limit4.att":20000,"limit4.hp":76000,"limit4.pdef":6000,"limit4.mdef":6000,"limit4.cri":48,"limit4.antiCri":48,"limit6.att":45000,"limit6.hp":171000,"limit6.pdef":13500,"limit6.mdef":13500,"limit6.cri":108,"limit6.antiCri":108,"limit10.att":160000,"limit10.hp":608000,"limit10.pdef":48000,"limit10.mdef":48000,"limit10.cri":240,"limit10.antiCri":240}`
)
