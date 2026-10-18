package service

import "testing"

// Original desc09 consumes real400 mine/camp ore, completes the guild task once,
// credits actual prestige and survives source closure, cold rebuild and car production.
func TestP13GuildMissionActualFactory400PrestigeColdAndOnce(t *testing.T) {
	p13GuildFactoryActual400(t, true)
}
