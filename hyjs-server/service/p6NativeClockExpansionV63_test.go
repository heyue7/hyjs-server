package service

import "testing"

func TestP6NativeOriginalClockV63TickGolden(t *testing.T) {
	p6NativeAutomaticPVPOriginalTickGolden(t, "native-next-real-golden-v63.json", "5cd373890312aa230b5bc3deecbc5dd0ea32764f79a56e43ff8425692bf415ca", 14)
}
func TestP6NativeOriginalClockV63DynamicGoldens(t *testing.T) {
	const (
		file = "native-next-real-golden-v63.json"
		sha  = "5cd373890312aa230b5bc3deecbc5dd0ea32764f79a56e43ff8425692bf415ca"
	)
	t.Run("actor44", func(t *testing.T) { p6NativeActorPropertiesFromFixture(t, "", true, file, sha) })
	t.Run("lifetimes", func(t *testing.T) { p6NativeDetailsFromFixture(t, "", true, file, sha) })
	t.Run("Buffmetadata", func(t *testing.T) { p6NativeBuffSourceAndValuesFromFixture(t, "", true, file, sha) })
}

func TestP6NativeOriginalManagerV63Golden(t *testing.T) {
	p6NativeOriginalManagerFixture(t, "native-manager-flags-golden-v63batch.json", "837bf8eb336a467b570b6340192d1739ee3e4de46ae0a9acb35377359caecbea", 1)
}
