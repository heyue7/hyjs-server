package constUtil

const (
	P8FirstFieldScene               = "field_001"
	P8FieldSceneUnavailable         = "原野外场景配置或资源尚未开放"
	P8FieldSceneLevel               = "未达到该原野外场景开放等级"
	P8FieldSceneStateConflict       = "野外场景与未完成战斗不一致"
	P8SnowBattleMinimumBuild  int64 = 2026102202
	P8SnowBattleClientUpdate        = "请更新客户端后再挑战雪原怪物"
)

// P8OriginalFieldScene enumerates the six original APK field identities.
func P8OriginalFieldScene(scene string) bool {
	switch scene {
	case "field_001", "field_002", "field_003", "field_004", "field_005", "field_006":
		return true
	default:
		return false
	}
}

// P8PackagedFieldScene is backed by the audited original APK scene bundles.
// The remaining original identities keep their rules until their real scene
// bundles are recovered and verified for the client.
func P8PackagedFieldScene(scene string) bool {
	return scene == "field_001" || scene == "field_002"
}
