package service

import "testing"

// Real signup, timed mining/deposit, original result, offline P1 and cold claims
// share the terminal test's lifecycle with a separately prepublished MVP reward.
func TestP13TransportMVPActualTimedOreAndLocalInbox(t *testing.T) {
	p13ZeroFightActualTimedRoundAndLocalInbox(t, true, false)
}

// Two real equal deposits retain both statistics and grant no unique-leader award.
func TestP13TransportMVPTiedActualOreHasNoLocalUniqueAward(t *testing.T) {
	p13ZeroFightActualTimedRoundAndLocalInbox(t, true, true)
}
