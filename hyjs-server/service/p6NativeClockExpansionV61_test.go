package service

import "testing"

func TestP6NativeOriginalCallbackClockV61TickGolden(t *testing.T) {
	p6NativeAutomaticPVPOriginalTickGolden(t, "native-next-real-golden-v61.json", "dfdf85e1d825401b9b6ce843285a489bab210c1e1a3fa4f429a39d336e96b3dd", 14)
}
func TestP6NativeOriginalCallbackClockV61DynamicGoldens(t *testing.T) {
	const (
		file = "native-next-real-golden-v61.json"
		sha  = "dfdf85e1d825401b9b6ce843285a489bab210c1e1a3fa4f429a39d336e96b3dd"
	)
	t.Run("actor44", func(t *testing.T) { p6NativeActorPropertiesFromFixture(t, "", true, file, sha) })
	t.Run("lifetimes", func(t *testing.T) { p6NativeDetailsFromFixture(t, "", true, file, sha) })
	t.Run("Buffmetadata", func(t *testing.T) { p6NativeBuffSourceAndValuesFromFixture(t, "", true, file, sha) })
}
func TestP6NativeOriginalManagerV61Golden(t *testing.T) {
	p6NativeOriginalManagerFixture(t, "native-manager-flags-golden-v61batch.json", "4104f1f3208e4ab0e93e025cd1cba60979264bbf802a83f852fe646dbab21783", 1)
}
