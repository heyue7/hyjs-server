package service

import (
	"bytes"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

type p6RenewalFamily struct {
	name                      string
	current, retained, parent string
	mode                      int32
	engine                    string
	known                     func(*model.FrozenBattleInput, string) bool
}

func p6RenewalFamilies() []p6RenewalFamily {
	return []p6RenewalFamily{
		{"arena", constUtil.P6ArenaNativeProof, constUtil.P6ArenaRevalidated20261027Proof, constUtil.P6ArenaParentProof, 2, constUtil.P6ArenaSettlementEngine, p6KnownArenaSettlementProof},
		{"starter", constUtil.P6StarterNativeProof, constUtil.P6StarterRevalidated20261027Proof, constUtil.P6StarterParentProof, 11, constUtil.P6LinearPhysicalEngine, p6KnownStarterSettlementProof},
		{"owned", constUtil.P6OwnedPartnerNativeProof, constUtil.P6OwnedPartnerRevalidated20261027Proof, constUtil.P6OwnedPartnerParentProof, 11, constUtil.P6OwnedPartnerSettlementEngine, p6KnownOwnedPartnerSettlementProof},
		{"linear", constUtil.P6LinearPhysicalNativeProof, constUtil.P6LinearPhysicalRevalidated20261027Proof, constUtil.P6LinearPhysicalParentProof, 11, constUtil.P6LinearPhysicalEngine, p6KnownScopedSettlementProof},
	}
}

func TestP6Certificate20261029ThreeGenerationsIdentityBoundaries(t *testing.T) {
	families := p6RenewalFamilies()
	for _, family := range families {
		t.Run(family.name, func(t *testing.T) {
			valid := model.FrozenBattleInput{BattleType: family.mode, EngineVersion: family.engine, ClientVersion: constUtil.P6OriginalClientHash, ConfigurationHash: constUtil.P6LinearPhysicalTables}
			proofs := []string{family.current, family.retained, family.parent}
			if family.current == "" || family.current == family.retained || family.retained == family.parent || family.current == family.parent {
				t.Fatal("certificate generations are not distinct")
			}
			for _, proof := range proofs {
				if !family.known(&valid, proof) {
					t.Fatal("known generation rejected")
				}
				for _, mutate := range []func(*model.FrozenBattleInput){
					func(x *model.FrozenBattleInput) { x.BattleType = 9 },
					func(x *model.FrozenBattleInput) { x.EngineVersion = "foreign" },
					func(x *model.FrozenBattleInput) { x.ClientVersion = "foreign" },
					func(x *model.FrozenBattleInput) { x.ConfigurationHash = "foreign" },
				} {
					changed := valid
					mutate(&changed)
					if family.known(&changed, proof) {
						t.Fatal("foreign identity accepted")
					}
				}
			}
			if family.known(nil, family.retained) || family.known(&valid, "") || family.known(&valid, "unknown") {
				t.Fatal("invalid proof accepted")
			}
			for _, other := range families {
				if other.name != family.name && family.known(&valid, other.retained) {
					t.Fatal("other family proof accepted")
				}
			}
			if family.name == "linear" {
				valid.BattleType = 2
				for _, proof := range proofs {
					if !family.known(&valid, proof) {
						t.Fatal("linear mode 2 lost")
					}
				}
				valid.EngineVersion = constUtil.P6LinearPhysicalOriginalEngine
				if !family.known(&valid, constUtil.P6LinearPhysicalPreviousProof) || !family.known(&valid, constUtil.P6LinearPhysicalOriginalProof) {
					t.Fatal("old linear mode 2 lost")
				}
				valid.BattleType = 11
				if !family.known(&valid, constUtil.P6LinearPhysicalPreviousProof) || family.known(&valid, constUtil.P6LinearPhysicalOriginalProof) {
					t.Fatal("old linear mode 11 boundary changed")
				}
				if family.known(&valid, family.retained) || family.known(&valid, family.parent) {
					t.Fatal("newer proof admitted under v109 engine")
				}
			}
		})
	}
}

// Rebuild only isolated test rows to model an already accepted historical proof.
// Original responses, input, operation recordings and all real stored rows remain untouched.
func p6RenewalHistoricalTerminal(t *testing.T, recordID string, terminalRaw []byte, proof string) []byte {
	t.Helper()
	var terminal model.CombatFinalHPOutcome
	if err := json.Unmarshal(terminalRaw, &terminal); err != nil || terminal.Outcome == nil {
		t.Fatal("invalid fixture terminal", err)
	}
	terminal.Outcome.CapabilityHash = proof
	outcome, err := json.Marshal(terminal.Outcome)
	if err != nil {
		t.Fatal(err)
	}
	final, err := json.Marshal(&terminal)
	if err != nil {
		t.Fatal(err)
	}
	if err = db.GetIns().Model(&db.P6BattleRecord{}).Where("id=?", recordID).Updates(map[string]any{"outcome": outcome, "outcome_hash": p6Digest(outcome)}).Error; err != nil {
		t.Fatal(err)
	}
	return final
}

func p6RenewalProfiles(t *testing.T, ids ...int64) []byte {
	t.Helper()
	var snapshots []string
	for _, id := range ids {
		var row db.Profile
		if err := db.GetIns().First(&row, id).Error; err != nil || row.Player == nil || row.Game == nil {
			t.Fatal("profile snapshot", err)
		}
		snapshots = append(snapshots, *row.Player, *row.Game)
	}
	raw, err := json.Marshal(snapshots)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func p6RenewalCheckCounts(t *testing.T, owner int64, receipt any) {
	t.Helper()
	for _, table := range []any{receipt, &db.P6BattleRecord{}} {
		var count int64
		if err := db.GetIns().Model(table).Where("player_id=?", owner).Count(&count).Error; err != nil || count != 1 {
			t.Fatal("historical retry duplicated evidence", count, err)
		}
	}
}

func TestP6Certificate20261029ArenaHistoricalReceiptCold(t *testing.T) {
	f, challenger := p11ArenaScopedSetup(t)
	defender := f.id
	now := time.Now()
	if err := db.GetIns().Model(&db.P11ArenaPolicy{}).Where("id=1").Update("enabled", true).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.GetIns().Model(&db.P11ArenaFightPolicy{}).Where("id=1").Update("enabled", true).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := f.s.p11ArenaQuery(f.ctx, challenger, &pb.CSPvpFindMatch{}, 29101, now); err != nil {
		t.Fatal(err)
	}
	request := &pb.CSPvpFightResult{PlayerId: proto.Int64(defender), ServerId: proto.Int32(1)}
	first, err := f.s.p11ArenaScopedFightAt(f.ctx, challenger, request, 29102, now)
	if err != nil {
		t.Fatal(err)
	}
	expected, err := proto.MarshalOptions{Deterministic: true}.Marshal(first[0])
	if err != nil {
		t.Fatal(err)
	}
	var row db.P11ArenaFightSettlement
	if err = db.GetIns().First(&row, "player_id=? AND stamp=?", challenger, 29102).Error; err != nil || row.ScopeProof != constUtil.P6ArenaNativeProof {
		t.Fatal("actual arena fixture used another proof", err)
	}
	// The accepted Arena path also records its prize proof. The isolated
	// historical fixture must keep that linked integrity chain consistent.
	var award db.P11ArenaPrizeAward
	if err = db.GetIns().First(&award, "fight_id=?", row.FightID).Error; err != nil {
		t.Fatal("arena prize fixture missing", err)
	}
	if err = db.GetIns().Model(&db.P11ArenaPolicy{}).Where("id=1").Update("enabled", false).Error; err != nil {
		t.Fatal(err)
	}
	if err = db.GetIns().Model(&db.P11ArenaFightPolicy{}).Where("id=1").Update("enabled", false).Error; err != nil {
		t.Fatal(err)
	}
	before := p6RenewalProfiles(t, challenger, defender)
	var ranks []db.P11ArenaMember
	if err = db.GetIns().Order("player_id").Find(&ranks).Error; err != nil {
		t.Fatal(err)
	}
	ranksBefore, _ := json.Marshal(ranks)
	for _, proof := range []string{constUtil.P6ArenaRevalidated20261027Proof, constUtil.P6ArenaParentProof} {
		terminal := p6RenewalHistoricalTerminal(t, row.P6RecordID, row.Terminal, proof)
		if err = db.GetIns().Model(&row).Updates(map[string]any{"scope_proof": proof, "terminal": terminal, "terminal_hash": p6Digest(terminal)}).Error; err != nil {
			t.Fatal(err)
		}
		award.ScopeProof, award.TerminalHash = proof, p6Digest(terminal)
		award.SourceHash = p11ArenaPrizeAwardHash(award)
		if err = db.GetIns().Model(&award).Updates(map[string]any{"scope_proof": award.ScopeProof, "terminal_hash": award.TerminalHash, "source_hash": award.SourceHash}).Error; err != nil {
			t.Fatal(err)
		}
		awardBefore, _ := json.Marshal(award)
		f.restart(t)
		for i := 0; i < 2; i++ {
			retry, e := f.s.p11ArenaScopedFightAt(f.ctx, challenger, request, 29102, now.Add(time.Hour))
			if e != nil {
				t.Fatal("historical arena retry", e)
			}
			raw, e := proto.MarshalOptions{Deterministic: true}.Marshal(retry[0])
			if e != nil || !bytes.Equal(raw, expected) {
				t.Fatal("historical arena response changed", e)
			}
		}
		if !bytes.Equal(before, p6RenewalProfiles(t, challenger, defender)) {
			t.Fatal("historical arena retry changed profiles")
		}
		var after []db.P11ArenaMember
		if err = db.GetIns().Order("player_id").Find(&after).Error; err != nil {
			t.Fatal(err)
		}
		ranksAfter, _ := json.Marshal(after)
		if !bytes.Equal(ranksBefore, ranksAfter) {
			t.Fatal("historical arena retry changed ranks")
		}
		p6RenewalCheckCounts(t, challenger, &db.P11ArenaFightSettlement{})
		var afterAward db.P11ArenaPrizeAward
		if err = db.GetIns().First(&afterAward, "fight_id=?", row.FightID).Error; err != nil {
			t.Fatal(err)
		}
		awardAfter, _ := json.Marshal(afterAward)
		if !bytes.Equal(awardBefore, awardAfter) {
			t.Fatal("historical retry changed prize receipt")
		}
	}
}

// The linear-only original family uses pose 1. Build a historical Friend receipt
// from a retained native input and the real capture/compute/store functions,
// because the modern live roster projection normally uses another formation pose.
func p6RenewalLinearFriendFixture(t *testing.T, f *p1Fixture, peer int64, request *pb.CSFriendFight, stamp int64, now time.Time) []proto.Message {
	t.Helper()
	input := p6LinearPhysicalScopeInput(t)
	input.BattleType = 11
	input.Attacker.PlayerId = proto.Int64(f.id)
	input.Defender.PlayerId = proto.Int64(peer)
	input.Attacker.ServerId = proto.Int32(1)
	input.Defender.ServerId = proto.Int32(1)
	input.DefenderFirst = input.Attacker.GetInst().GetFirstOrder() == 0
	calc := p6OriginalTables(t)
	const fightID = "p6-linear-historical-fixture-20261029"
	var response *pb.SCFriendFight
	err := db.GetIns().WithContext(f.ctx).Transaction(func(tx *gorm.DB) error {
		id, err := calc.CaptureBattleReplay(f.ctx, tx, f.id, constUtil.P11FriendScopedSource, fightID, *input)
		if err != nil {
			return err
		}
		var record db.P6BattleRecord
		if err = tx.First(&record, "id=?", id).Error; err != nil {
			return err
		}
		frozen, err := decodeP6Record(&record)
		if err != nil {
			return err
		}
		terminal, err := calc.ComputeP6ScopedSettlement(f.ctx, frozen)
		if err != nil {
			return err
		}
		if terminal.Outcome.CapabilityHash != constUtil.P6LinearPhysicalNativeProof {
			return fmt.Errorf("linear proof missing")
		}
		winner := peer
		if terminal.Outcome.Winner == 0 {
			winner = f.id
		}
		response = &pb.SCFriendFight{FightId: proto.String(fightID), RoomId: proto.String(constUtil.P11FriendScopedRoom), RandSeed: proto.Int64(frozen.Seed), PlayerList: []*pb.BattlePlayer{proto.Clone(frozen.Attacker).(*pb.BattlePlayer), proto.Clone(frozen.Defender).(*pb.BattlePlayer)}, WinPlayerId: proto.Int64(winner), MaxRoundNum: proto.Int32(frozen.MaxRound)}
		body, err := proto.MarshalOptions{Deterministic: true}.Marshal(response)
		if err != nil {
			return err
		}
		final, err := json.Marshal(terminal)
		if err != nil {
			return err
		}
		requestBody, err := proto.MarshalOptions{Deterministic: true}.Marshal(request)
		if err != nil {
			return err
		}
		if err = p11StoreScopedFriendP6(tx, &record, terminal.Outcome, now); err != nil {
			return err
		}
		return tx.Create(&db.P11FriendFightSettlement{PlayerID: f.id, Stamp: stamp, OpponentID: peer, FightID: fightID, P6RecordID: id, RequestHash: p6Digest(requestBody), ScopeProof: terminal.Outcome.CapabilityHash, Response: body, ResponseHash: p6Digest(body), Terminal: final, TerminalHash: p6Digest(final), CreatedAt: now.UnixMilli()}).Error
	})
	if err != nil {
		t.Fatal("linear historical capture", err)
	}
	return []proto.Message{response}
}

func TestP6Certificate20261029FriendHistoricalReceiptsCold(t *testing.T) {
	for _, family := range p6RenewalFamilies()[1:] {
		t.Run(family.name, func(t *testing.T) {
			f, peer, _ := p11ArenaSetup(t)
			now := time.Now()
			if family.name == "owned" {
				if err := f.s.LoadP5Tables("../../analysis/tables"); err != nil {
					t.Fatal(err)
				}
				if err := f.s.InitializeP5(f.ctx); err != nil {
					t.Fatal(err)
				}
				f.s.P5Ready = true
				for _, id := range []int64{f.id, peer} {
					if err := updateProfile(f.ctx, id, func(p *pb.Player, g *model.GameState) error { p.Level = proto.Int32(100); return nil }); err != nil {
						t.Fatal(err)
					}
				}
				p11ClaimAndFormPartner(t, f, f.id, "hero_069", 2)
				p11ClaimAndFormPartner(t, f, peer, "hero_070", 2)
			}
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
			const stamp int64 = 29103
			var first []proto.Message
			var err error
			if family.name == "linear" {
				first = p6RenewalLinearFriendFixture(t, f, peer, request, stamp, now)
			} else {
				first, err = f.s.p11FriendScopedFightAt(f.ctx, f.id, request, stamp, now)
				if err != nil {
					t.Fatal(err)
				}
			}
			expected, err := proto.MarshalOptions{Deterministic: true}.Marshal(first[0])
			if err != nil {
				t.Fatal(err)
			}
			var row db.P11FriendFightSettlement
			if err = db.GetIns().First(&row, "player_id=? AND stamp=?", f.id, stamp).Error; err != nil || row.ScopeProof != family.current {
				t.Fatal("fixture used another proof", family.name, err)
			}
			if err = db.GetIns().Model(&db.P11ArenaPolicy{}).Where("id=1").Updates(map[string]any{"enabled": false, "duel_enabled": false}).Error; err != nil {
				t.Fatal(err)
			}
			before := p6RenewalProfiles(t, f.id, peer)
			for _, proof := range []string{family.retained, family.parent} {
				terminal := p6RenewalHistoricalTerminal(t, row.P6RecordID, row.Terminal, proof)
				if err = db.GetIns().Model(&row).Updates(map[string]any{"scope_proof": proof, "terminal": terminal, "terminal_hash": p6Digest(terminal)}).Error; err != nil {
					t.Fatal(err)
				}
				f.restart(t)
				for i := 0; i < 2; i++ {
					retry, e := f.s.p11FriendScopedFightAt(f.ctx, f.id, request, stamp, now.Add(time.Hour))
					if e != nil {
						t.Fatal("historical friend retry", e)
					}
					raw, e := proto.MarshalOptions{Deterministic: true}.Marshal(retry[0])
					if e != nil || !bytes.Equal(raw, expected) {
						t.Fatal("historical friend response changed", e)
					}
				}
				if !bytes.Equal(before, p6RenewalProfiles(t, f.id, peer)) {
					t.Fatal("historical friend retry changed profiles")
				}
				p6RenewalCheckCounts(t, f.id, &db.P11FriendFightSettlement{})
			}
		})
	}
}
