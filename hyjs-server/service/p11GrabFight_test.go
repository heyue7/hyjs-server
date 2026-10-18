package service

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

func p11GrabFightProfileBytes(t *testing.T, id int64) []byte {
	t.Helper()
	var row db.Profile
	if err := db.GetIns().First(&row, id).Error; err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal([]*string{row.Player, row.Game})
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

// The public producer starts from an actual empty occupation. Its first new
// challenge uses a real random seed and either rolls back missing proof or
// applies the actual computed winner; no winner/HP/seed is supplied by the test.
func TestP11GrabFightRealOwnerIndependentGateCostAndColdReceipt(t *testing.T) {
	f, defenderID, _ := p11ArenaSetup(t)
	if err := f.s.InitializeP11GrabFight(f.ctx); err != nil {
		t.Fatal(err)
	}
	p11GrabPrepare(t, f, f.id, defenderID)
	if err := updateProfile(f.ctx, f.id, func(p *pb.Player, g *model.GameState) error {
		p.Level = proto.Int32(40)
		for _, h := range g.Warriors {
			if h.GetId() == "hero_047" {
				h.Level = proto.Int32(40)
			}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	now := time.Now().Truncate(time.Millisecond)
	occupy := p11GrabCall(t, f, defenderID, &pb.CSGrab{Index: proto.Int32(1)}, 81000, now).(*pb.SCGrab)
	if occupy.GetGrabData().GetPlayerId() != defenderID {
		t.Fatal("actual initial owner")
	}
	var previous db.P11GrabSlot
	if err := db.GetIns().First(&previous, 1).Error; err != nil {
		t.Fatal(err)
	}
	beforeOwn, beforePeer := p11GrabFightProfileBytes(t, f.id), p11GrabFightProfileBytes(t, defenderID)
	request := &pb.CSGrab{Index: proto.Int32(1)}
	if _, err := f.s.p11GrabAt(f.ctx, f.id, request, 81001, now); err == nil {
		t.Fatal("defaultoff accepted occupied challenge")
	}
	if !bytes.Equal(beforeOwn, p11GrabFightProfileBytes(t, f.id)) || !bytes.Equal(beforePeer, p11GrabFightProfileBytes(t, defenderID)) {
		t.Fatal("closed attempt changed real profiles")
	}
	if err := db.GetIns().Model(&db.P11GrabFightPolicy{}).Where("id=1").Updates(map[string]any{"enabled": true, "published_at": now.Add(-2 * time.Minute).UnixMilli(), "starts_at": now.Add(-time.Minute).UnixMilli(), "ends_at": now.Add(time.Hour).UnixMilli()}).Error; err != nil {
		t.Fatal(err)
	}
	result, err := f.s.p11GrabAt(f.ctx, f.id, request, 81002, now)
	if constUtil.P6GrabNativeProof == "" {
		if err == nil || result != nil || !strings.Contains(err.Error(), constUtil.CombatUnsupported) {
			t.Fatal("unproven original19 did not reject", err)
		}
		for _, table := range []any{&db.P6BattleRecord{}, &db.P11GrabFightOffer{}, &db.P11GrabFightSettlement{}} {
			var count int64
			if e := db.GetIns().Model(table).Count(&count).Error; e != nil || count != 0 {
				t.Fatal("pending calculation leaked durable fact", count, e)
			}
		}
		if !bytes.Equal(beforeOwn, p11GrabFightProfileBytes(t, f.id)) || !bytes.Equal(beforePeer, p11GrabFightProfileBytes(t, defenderID)) {
			t.Fatal("missing proof charged resources")
		}
		var retained db.P11GrabSlot
		db.GetIns().First(&retained, 1)
		if retained.SessionID != previous.SessionID || retained.StartedAt != previous.StartedAt || retained.ExpiresAt != previous.ExpiresAt {
			t.Fatal("missing proof replaced owner clock")
		}
		return
	}
	if err != nil {
		t.Fatal("certified actual random original19", err)
	}
	response := result[0].(*pb.SCGrab)
	var settlement db.P11GrabFightSettlement
	if err = db.GetIns().First(&settlement, "player_id=? AND stamp=?", f.id, 81002).Error; err != nil {
		t.Fatal(err)
	}
	if settlement.BeforeEnergy-settlement.AfterEnergy != 10 || settlement.BeforeUsed+1 != settlement.AfterUsed || settlement.BeforeRemaining-1 != settlement.AfterRemaining || response.GetCurrEnergy() != settlement.AfterEnergy || response.GetGrabTimes() != settlement.AfterRemaining {
		t.Fatal("original accepted cost", settlement)
	}
	var current db.P11GrabSlot
	db.GetIns().First(&current, 1)
	if settlement.Won {
		if current.OwnerID == nil || *current.OwnerID != f.id || current.SessionID == previous.SessionID || current.StartedAt != now.UnixMilli() || current.ExpiresAt != now.Add(3*time.Hour).UnixMilli() {
			t.Fatal("verified win transfer")
		}
	} else if current.OwnerID == nil || *current.OwnerID != defenderID || current.SessionID != previous.SessionID || current.StartedAt != previous.StartedAt || current.ExpiresAt != previous.ExpiresAt {
		t.Fatal("real loss changed owner or clock")
	}
	if !bytes.Equal(beforePeer, p11GrabFightProfileBytes(t, defenderID)) {
		t.Fatal("challenge mutated defender profile")
	}
	for i := 0; i < 8; i++ {
		same, e := f.s.p11GrabAt(f.ctx, f.id, request, 81002, now)
		if e != nil || !proto.Equal(response, same[0]) {
			t.Fatal("accepted same-stamp result changed", e)
		}
	}
	check, err := f.s.p11ReadGrabFightReceipt(db.GetIns(), &settlement)
	if err != nil || !proto.Equal(response, check) {
		t.Fatal("whole canonical receipt", err)
	}
	for _, kind := range []string{"foreign-proof", "winner", "cost", "source", "terminal-hp", "frame-time"} {
		blob, _ := json.Marshal(settlement)
		var changed db.P11GrabFightSettlement
		json.Unmarshal(blob, &changed)
		switch kind {
		case "foreign-proof":
			changed.ScopeProof = constUtil.P6ActualArenaNativeProof
		case "winner":
			changed.Won = !changed.Won
		case "cost":
			changed.AfterEnergy++
		case "source":
			typed := &pb.SCGrab{}
			proto.Unmarshal(changed.StartPB, typed)
			typed.RandSeed = proto.Int64(typed.GetRandSeed() + 1)
			changed.StartPB, _ = proto.MarshalOptions{Deterministic: true}.Marshal(typed)
			changed.StartHash = p6Digest(changed.StartPB)
		case "terminal-hp":
			var terminal model.CombatFinalHPOutcome
			json.Unmarshal(changed.Terminal, &terminal)
			terminal.FinalActors[0].CurrentHP++
			changed.Terminal, _ = json.Marshal(terminal)
			changed.TerminalHash = p6Digest(changed.Terminal)
		case "frame-time":
			out := &pb.SCGrab{}
			proto.Unmarshal(changed.Response, out)
			out.GrabData.GrabTime = proto.Int64(out.GrabData.GetGrabTime() + 1)
			changed.Response, _ = proto.MarshalOptions{Deterministic: true}.Marshal(out)
			changed.ResponseHash = p6Digest(changed.Response)
		}
		if _, e := f.s.p11ReadGrabFightReceipt(db.GetIns(), &changed); e == nil {
			t.Fatal("tampered historical source accepted", kind)
		}
	}
	// A later real give-up and disabled live policy cannot erase paid history.
	ownerID := defenderID
	if settlement.Won {
		ownerID = f.id
	}
	p11GrabCall(t, f, ownerID, &pb.CSGiveUpGrab{Index: proto.Int32(1)}, 81003, now.Add(time.Second))
	db.GetIns().Model(&db.P11GrabFightPolicy{}).Where("id=1").Update("enabled", false)
	f.restart(t)
	again, e := f.s.p11GrabAt(f.ctx, f.id, request, 81002, now.Add(24*time.Hour))
	if e != nil || !proto.Equal(response, again[0]) {
		t.Fatal("closed policy/later owner state blocked historical receipt", e)
	}
	var count int64
	db.GetIns().Model(&db.P11GrabFightSettlement{}).Where("player_id=?", f.id).Count(&count)
	if count != 1 {
		t.Fatal("accepted once journal", count)
	}
}

func TestP11GrabFightRulesNeverBorrowModeOrUnpublishedPolicy(t *testing.T) {
	now := time.Now().UnixMilli()
	valid := p11GrabFightRules{Policy: db.P11GrabFightPolicy{ID: 1, Enabled: true, Version: "local-test-rules", Source: constUtil.P11GrabFightPolicySource, PublishedAt: now - 2, StartsAt: now - 1, EndsAt: now + 1000, MaxRound: 40, AttackerFirst: true}, Grab: db.P11GrabPolicy{ID: 1, MaxTimes: 3, BuyMax: 3, BuyPrice: 100, Energy: 10, MaxMillis: int64(3 * time.Hour / time.Millisecond), Source: "finite-test-config", RewardsJSON: "[]", RewardsHash: p6Digest([]byte("[]"))}}
	if !p11GrabFightRulesValid(&valid) {
		t.Fatal("valid prepublished policy")
	}
	for _, kind := range []string{"off", "source", "version", "late-publish", "max", "initiative", "cost", "duration"} {
		changed := valid
		switch kind {
		case "off":
			changed.Policy.Enabled = false
		case "source":
			changed.Policy.Source = "mode2"
		case "version":
			changed.Policy.Version = " "
		case "late-publish":
			changed.Policy.PublishedAt = now
		case "max":
			changed.Policy.MaxRound = 20
		case "initiative":
			changed.Policy.AttackerFirst = false
		case "cost":
			changed.Grab.Energy = 2
		case "duration":
			changed.Grab.MaxMillis++
		}
		if p11GrabFightRulesValid(&changed) {
			t.Fatal("unpublished/mismatched policy accepted", kind)
		}
	}
}
