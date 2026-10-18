package service

import "testing"

func TestP6NativeOriginalClockV73TickGolden(t *testing.T) {
	p6NativeAutomaticPVPOriginalTickGolden(t, "native-next-real-golden-v64.json", "a767c3a3b3f9afe293131c079a6d0d7567d1ea9607504923abc154cefaae8996", 14)
}
func TestP6NativeOriginalClockV73DynamicGoldens(t *testing.T) {
	const (
		file = "native-next-real-golden-v64.json"
		sha  = "a767c3a3b3f9afe293131c079a6d0d7567d1ea9607504923abc154cefaae8996"
	)
	t.Run("actor44", func(t *testing.T) { p6NativeActorPropertiesFromFixture(t, "", true, file, sha) })
	t.Run("lifetimes", func(t *testing.T) { p6NativeDetailsFromFixture(t, "", true, file, sha) })
	t.Run("Buffmetadata", func(t *testing.T) { p6NativeBuffSourceAndValuesFromFixture(t, "", true, file, sha) })
}

func TestP6NativeOriginalManagerV73Golden(t *testing.T) {
	p6NativeOriginalManagerFixture(t, "native-manager-flags-golden-v64batch.json", "8e4eff87593dd29e451f59ba4cd7780048ded9e157ecd8f699f14a0acafb8b2b", 1)
}
