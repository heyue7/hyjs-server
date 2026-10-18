package service

import "math"

// Original class1 Effect/UnEffect freezes these values; Update0 does nothing.
// Native hit/cri are fractions; this engine keeps their panel percentage units.
func p6PropertyTransferKey(factor string) string {
	switch factor {
	case "sourceNhitToperdef":
		return "perdef"
	case "angerTocri", "comboNumTocri":
		return "cri"
	case "angerToperatt":
		return "peratt"
	case "criTocriFactor":
		return "criFactor"
	case "sourcepermdefTomdef":
		return "mdef"
	}
	return ""
}
func p6PropertyTransferDivides(factor string) bool {
	return factor == "angerTocri" || factor == "comboNumTocri" || factor == "angerToperatt" || factor == "criTocriFactor"
}
func (e *combatRoundEngine) propertyTransferValue(source, target *combatRoundActor, row map[string]string) float64 {
	x := tableNumber(row, "Args1")
	switch row["Factor"] {
	case "sourceNhitToperdef":
		return source.attribute("hit") * x
	case "sourcepermdefTomdef":
		return source.attribute("mdef") * x * .01
	}
	input := e.players[target.camp].anger
	if row["Factor"] == "comboNumTocri" {
		input = float64(e.players[target.camp].combo)
	}
	if row["Factor"] == "criTocriFactor" {
		input = target.attribute("cri")
	}
	value := math.Floor(input/x) * tableNumber(row, "Args2")
	if bound := tableNumber(row, "Args3"); bound != 0 {
		value = math.Min(value, bound)
	}
	return value
}
