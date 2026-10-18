package service

import "testing"

func TestP6NativeOriginalClockV81TickGolden(t *testing.T) {
	p6NativeAutomaticPVPOriginalTickGolden(t, "native-next-real-golden-v66a.json", "755f6ce57473743d484bf90c8de2873d2b0164f704ec1da0ed9fec4d9a9c63e7", 14)
}
func TestP6NativeOriginalClockV81DynamicGoldens(t *testing.T) {
	const (
		file = "native-next-real-golden-v66a.json"
		sha  = "755f6ce57473743d484bf90c8de2873d2b0164f704ec1da0ed9fec4d9a9c63e7"
	)
	t.Run("actor44", func(t *testing.T) { p6NativeActorPropertiesFromFixture(t, "", true, file, sha) })
	t.Run("lifetimes", func(t *testing.T) { p6NativeDetailsFromFixture(t, "", true, file, sha) })
	t.Run("Buffmetadata", func(t *testing.T) { p6NativeBuffSourceAndValuesFromFixture(t, "", true, file, sha) })
}

func TestP6NativeOriginalManagerV81Golden(t *testing.T) {
	p6NativeOriginalManagerFixture(t, "native-manager-flags-golden-v66abatch.json", "bf9ebc6e027475f17e51710c5295fd9ec78d54646eaf691244a7bad4d42d37f3", 1)
}
