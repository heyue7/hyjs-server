package service

import (
	"testing"
)

func TestP6NativeOriginalClockV62TickGolden(t *testing.T) {
	p6NativeAutomaticPVPOriginalTickGolden(t, "native-next-real-golden-v62.json", "408ceb554dcaf42ac57d3afcec18f1ade52abe2c899fa9c9b7b84f69b168be56", 14)
}
func TestP6NativeOriginalClockV62DynamicGoldens(t *testing.T) {
	const (
		file = "native-next-real-golden-v62.json"
		sha  = "408ceb554dcaf42ac57d3afcec18f1ade52abe2c899fa9c9b7b84f69b168be56"
	)
	t.Run("actor44", func(t *testing.T) { p6NativeActorPropertiesFromFixture(t, "", true, file, sha) })
	t.Run("lifetimes", func(t *testing.T) { p6NativeDetailsFromFixture(t, "", true, file, sha) })
	t.Run("Buffmetadata", func(t *testing.T) { p6NativeBuffSourceAndValuesFromFixture(t, "", true, file, sha) })
}

func TestP6NativeOriginalManagerV62Golden(t *testing.T) {
	p6NativeOriginalManagerFixture(t, "native-manager-flags-golden-v62batch.json", "e55b03cd39d54ddab27640f801c80c197fde0aec686f12c0d85a0fbe0ba0d4a9", 1)
}
