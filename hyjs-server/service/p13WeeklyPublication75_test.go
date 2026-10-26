package service

import (
	"encoding/json"
	"testing"

	"hyjs-server/db"
)

func TestP13Weekly75PolicyValidationDoesNotInventMatchAnchor(t *testing.T) {
	policy := db.P13UnionFightWeeklyPolicy{WeekStart: "2026-11-02", Group: "A", Enabled: true, Version: "isolated-future-policy", Source: "finite unpublished match policy fixture", PublishedAt: 1, Rewards: `[{"rank":1,"rewards":"coin,1"}]`}
	raw, err := json.Marshal(policy)
	if err != nil {
		t.Fatal(err)
	}
	offer := db.P13UnionFightWeeklyOffer{WeekStart: policy.WeekStart, Group: policy.Group, Rules: raw, RulesHash: p6Digest(raw), FrozenAt: 1}
	if _, _, err := p13UFWeeklyPolicyPayload(&offer); err != nil {
		t.Fatal("valid future policy incorrectly requires an invented match", err)
	}
	if _, _, err := p13UFWeeklyRules(&offer); err == nil {
		t.Fatal("actual weekly offer accepted missing match/bye anchor")
	}
	offer.FirstMatchID, offer.FirstByeID = "real-match-identity", "real-bye-identity"
	if _, _, err := p13UFWeeklyRules(&offer); err == nil {
		t.Fatal("actual weekly offer accepted two competing anchors")
	}
	offer.FirstByeID = ""
	offer.RulesHash = "corrupt"
	if _, _, err := p13UFWeeklyRules(&offer); err == nil {
		t.Fatal("actual weekly offer skipped policy integrity after anchor validation")
	}
}
