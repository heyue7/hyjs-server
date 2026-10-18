package service

import "testing"

func TestP6NativeOriginalClockV76TickGolden(t *testing.T) {
	p6NativeAutomaticPVPOriginalTickGolden(t, "native-next-real-golden-v65a.json", "20efa010a55df78caed84d8dedadbf2ddac85f694195bef9ad18447db8f445c3", 14)
}
func TestP6NativeOriginalClockV76DynamicGoldens(t *testing.T) {
	const (
		file = "native-next-real-golden-v65a.json"
		sha  = "20efa010a55df78caed84d8dedadbf2ddac85f694195bef9ad18447db8f445c3"
	)
	t.Run("actor44", func(t *testing.T) { p6NativeActorPropertiesFromFixture(t, "", true, file, sha) })
	t.Run("lifetimes", func(t *testing.T) { p6NativeDetailsFromFixture(t, "", true, file, sha) })
	t.Run("Buffmetadata", func(t *testing.T) { p6NativeBuffSourceAndValuesFromFixture(t, "", true, file, sha) })
}

func TestP6NativeOriginalManagerV76Golden(t *testing.T) {
	p6NativeOriginalManagerFixture(t, "native-manager-flags-golden-v65abatch.json", "5598530833b0cb839ff48a4ba45e32b4ca4a91bcd65f087f60ba82b076b27881", 1)
}
