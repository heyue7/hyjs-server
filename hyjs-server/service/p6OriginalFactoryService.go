package service

// Original factory dispatches property and dispel factors by exact switch keys.
// These exact rows have no handler: native logs an error and retains an inert identity.
func p6OriginalFactoryNoEffect(row map[string]string) bool {
	class := int32(tableNumber(row, "BuffClass")) % 10
	// Original BuffOfDispel accepts its five exact factors. These older
	// table names retain their identity and have no Effect/UnEffect branch.
	if class == 3 {
		return row["Factor"] == "clearBuff" || row["Factor"] == "clearDebuff"
	}
	if class != 0 {
		return false
	}
	return row["Id"] == "buff_722" && row["Factor"] == "sourceperattToshield" || row["Id"] == "buff_184" && row["Factor"] == "WeaponPerAtt"
}
