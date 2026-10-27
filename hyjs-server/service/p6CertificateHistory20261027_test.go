package service

import (
	"bytes"
	"encoding/json"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

// Historical proofs retain the same identity boundary as their current family.
func TestP6Certificate20261027CurrentAndParentProofBoundaries(t *testing.T) {
	families := []struct {
		name            string
		current, parent string
		mode            int32
		engine          string
		known           func(*model.FrozenBattleInput, string) bool
	}{
		{"arena", constUtil.P6ArenaNativeProof, constUtil.P6ArenaParentProof, 2, constUtil.P6ArenaSettlementEngine, p6KnownArenaSettlementProof},
		{"starter", constUtil.P6StarterNativeProof, constUtil.P6StarterParentProof, 11, constUtil.P6LinearPhysicalEngine, p6KnownStarterSettlementProof},
		{"owned", constUtil.P6OwnedPartnerNativeProof, constUtil.P6OwnedPartnerParentProof, 11, constUtil.P6OwnedPartnerSettlementEngine, p6KnownOwnedPartnerSettlementProof},
		{"linear", constUtil.P6LinearPhysicalNativeProof, constUtil.P6LinearPhysicalParentProof, 2, constUtil.P6LinearPhysicalEngine, p6KnownScopedSettlementProof},
	}
	for _, family := range families {
		t.Run(family.name, func(t *testing.T) {
			valid := model.FrozenBattleInput{BattleType: family.mode, EngineVersion: family.engine, ClientVersion: constUtil.P6OriginalClientHash, ConfigurationHash: constUtil.P6LinearPhysicalTables}
			for _, proof := range []string{family.current, family.parent} {
				if !family.known(&valid, proof) {
					t.Fatal("recognized certificate rejected")
				}
				for _, mutate := range []func(*model.FrozenBattleInput){
					func(v *model.FrozenBattleInput) { v.BattleType = 9 },
					func(v *model.FrozenBattleInput) { v.EngineVersion = "foreign" },
					func(v *model.FrozenBattleInput) { v.ClientVersion = "foreign" },
					func(v *model.FrozenBattleInput) { v.ConfigurationHash = "foreign" },
				} {
					changed := valid
					mutate(&changed)
					if family.known(&changed, proof) {
						t.Fatal("foreign identity accepted")
					}
				}
			}
			if family.known(&valid, "random-proof") || family.known(&valid, "") || family.known(nil, family.parent) {
				t.Fatal("unknown proof accepted")
			}
			if family.name == "linear" {
				valid.BattleType = 11
				if !family.known(&valid, family.parent) {
					t.Fatal("linear Friend11 parent lost")
				}
			}
		})
	}
}

// Build an isolated receipt with the exact parent certificate identity. Rehashing
// the fixture models a historically accepted receipt, without rewriting any live
// row or changing the immutable response/operations. Retries must read it cold.
func TestP6Certificate20261027ParentFriendReceiptColdAndNoDuplicate(t *testing.T) {
	f, peer, _ := p11ArenaSetup(t)
	now := time.Now()
	low, high := f.id, peer
	if low > high {
		low, high = high, low
	}
	if err := db.GetIns().Create(&db.P10SocialRelation{LowID: low, HighID: high, CreatedAt: now.UnixMilli()}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.GetIns().Model(&db.P11ArenaPolicy{}).Where("id=1").Updates(map[string]any{"enabled": true, "duel_enabled": true}).Error; err != nil {
		t.Fatal(err)
	}
	request := &pb.CSFriendFight{PlayerId: proto.Int64(peer), ServerId: proto.Int32(1)}
	first, err := f.s.p11FriendScopedFightAt(f.ctx, f.id, request, 20261027, now)
	if err != nil {
		t.Fatal(err)
	}
	expected, err := proto.MarshalOptions{Deterministic: true}.Marshal(first[0])
	if err != nil {
		t.Fatal(err)
	}
	var receipt db.P11FriendFightSettlement
	if err = db.GetIns().First(&receipt, "player_id=? AND stamp=?", f.id, 20261027).Error; err != nil {
		t.Fatal(err)
	}
	var record db.P6BattleRecord
	if err = db.GetIns().First(&record, "id=?", receipt.P6RecordID).Error; err != nil {
		t.Fatal(err)
	}
	var terminal model.CombatFinalHPOutcome
	if err = json.Unmarshal(receipt.Terminal, &terminal); err != nil {
		t.Fatal(err)
	}
	terminal.Outcome.CapabilityHash = constUtil.P6StarterParentProof
	outcome, err := json.Marshal(terminal.Outcome)
	if err != nil {
		t.Fatal(err)
	}
	final, err := json.Marshal(&terminal)
	if err != nil {
		t.Fatal(err)
	}
	if err = db.GetIns().Model(&record).Updates(map[string]any{"outcome": outcome, "outcome_hash": p6Digest(outcome)}).Error; err != nil {
		t.Fatal(err)
	}
	if err = db.GetIns().Model(&receipt).Updates(map[string]any{"scope_proof": constUtil.P6StarterParentProof, "terminal": final, "terminal_hash": p6Digest(final)}).Error; err != nil {
		t.Fatal(err)
	}
	profile := func() []byte {
		t.Helper()
		p, g := p10LuckyProfile(t, f.id)
		a, b := p10LuckyProfile(t, peer)
		raw, e := json.Marshal([]any{p, g, a, b})
		if e != nil {
			t.Fatal(e)
		}
		return raw
	}
	before := profile()
	if err = db.GetIns().Model(&db.P11ArenaPolicy{}).Where("id=1").Updates(map[string]any{"enabled": false, "duel_enabled": false}).Error; err != nil {
		t.Fatal(err)
	}
	f.restart(t)
	for i := 0; i < 2; i++ {
		retry, e := f.s.p11FriendScopedFightAt(f.ctx, f.id, request, 20261027, now.Add(time.Hour))
		if e != nil {
			t.Fatal("parent receipt cold retry", e)
		}
		raw, e := proto.MarshalOptions{Deterministic: true}.Marshal(retry[0])
		if e != nil || !bytes.Equal(raw, expected) {
			t.Fatal("historical response changed", e)
		}
	}
	if !bytes.Equal(before, profile()) {
		t.Fatal("historical retry changed assets")
	}
	for _, table := range []any{&db.P11FriendFightSettlement{}, &db.P6BattleRecord{}} {
		var n int64
		if e := db.GetIns().Model(table).Where("player_id=?", f.id).Count(&n).Error; e != nil || n != 1 {
			t.Fatal("duplicate historical receipt", n, e)
		}
	}
}
