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

func TestP6Certificate86FourGenerationsIdentityBoundaries(t *testing.T) {
	families := []struct {
		name   string
		proofs []string
		mode   int32
		engine string
		known  func(*model.FrozenBattleInput, string) bool
	}{
		{"arena", []string{constUtil.P6ArenaNativeProof, constUtil.P6ArenaRevalidated20261029Proof, constUtil.P6ArenaRevalidated20261027Proof, constUtil.P6ArenaParentProof}, 2, constUtil.P6ArenaSettlementEngine, p6KnownArenaSettlementProof},
		{"starter", []string{constUtil.P6StarterNativeProof, constUtil.P6StarterRevalidated20261029Proof, constUtil.P6StarterRevalidated20261027Proof, constUtil.P6StarterParentProof}, 11, constUtil.P6LinearPhysicalEngine, p6KnownStarterSettlementProof},
		{"owned", []string{constUtil.P6OwnedPartnerNativeProof, constUtil.P6OwnedPartnerRevalidated20261029Proof, constUtil.P6OwnedPartnerRevalidated20261027Proof, constUtil.P6OwnedPartnerParentProof}, 11, constUtil.P6OwnedPartnerSettlementEngine, p6KnownOwnedPartnerSettlementProof},
		{"linear", []string{constUtil.P6LinearPhysicalNativeProof, constUtil.P6LinearPhysicalRevalidated20261029Proof, constUtil.P6LinearPhysicalRevalidated20261027Proof, constUtil.P6LinearPhysicalParentProof}, 11, constUtil.P6LinearPhysicalEngine, p6KnownScopedSettlementProof},
	}
	for _, family := range families {
		t.Run(family.name, func(t *testing.T) {
			valid := model.FrozenBattleInput{BattleType: family.mode, EngineVersion: family.engine, ClientVersion: constUtil.P6OriginalClientHash, ConfigurationHash: constUtil.P6LinearPhysicalTables}
			seen := map[string]bool{}
			for _, proof := range family.proofs {
				if proof == "" || seen[proof] || !family.known(&valid, proof) {
					t.Fatal("known certificate generation rejected or repeated")
				}
				seen[proof] = true
				for _, mutate := range []func(*model.FrozenBattleInput){
					func(x *model.FrozenBattleInput) { x.BattleType = 8 },
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
			if family.known(nil, family.proofs[1]) || family.known(&valid, "") || family.known(&valid, "unknown") {
				t.Fatal("unrecognized proof accepted")
			}
			for _, other := range families {
				if other.name != family.name && family.known(&valid, other.proofs[1]) {
					t.Fatal("other family historical proof accepted")
				}
			}
			if family.name == "linear" {
				valid.BattleType = 2
				for _, proof := range family.proofs {
					if !family.known(&valid, proof) {
						t.Fatal("linear mode 2 lost an existing generation")
					}
				}
				valid.EngineVersion = constUtil.P6LinearPhysicalOriginalEngine
				if !family.known(&valid, constUtil.P6LinearPhysicalPreviousProof) || !family.known(&valid, constUtil.P6LinearPhysicalOriginalProof) {
					t.Fatal("linear v109 mode 2 proof lost")
				}
				for _, proof := range family.proofs {
					if family.known(&valid, proof) {
						t.Fatal("newer proof admitted under v109")
					}
				}
				valid.BattleType = 11
				if !family.known(&valid, constUtil.P6LinearPhysicalPreviousProof) || family.known(&valid, constUtil.P6LinearPhysicalOriginalProof) {
					t.Fatal("linear v109 mode 11 boundary changed")
				}
			}
		})
	}
}

// The historical proof is reconstructed only in this isolated fixture. Real
// stored receipts and their original terminal records are never rewritten.
func TestP6Certificate86Arena20261029ReceiptCold(t *testing.T) {
	f, challenger := p11ArenaScopedSetup(t)
	defender := f.id
	now := time.Now()
	if err := db.GetIns().Model(&db.P11ArenaPolicy{}).Where("id=1").Update("enabled", true).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.GetIns().Model(&db.P11ArenaFightPolicy{}).Where("id=1").Update("enabled", true).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := f.s.p11ArenaQuery(f.ctx, challenger, &pb.CSPvpFindMatch{}, 86001, now); err != nil {
		t.Fatal(err)
	}
	request := &pb.CSPvpFightResult{PlayerId: proto.Int64(defender), ServerId: proto.Int32(1)}
	first, err := f.s.p11ArenaScopedFightAt(f.ctx, challenger, request, 86002, now)
	if err != nil {
		t.Fatal(err)
	}
	expected, err := proto.MarshalOptions{Deterministic: true}.Marshal(first[0])
	if err != nil {
		t.Fatal(err)
	}
	var row db.P11ArenaFightSettlement
	if err = db.GetIns().First(&row, "player_id=? AND stamp=?", challenger, 86002).Error; err != nil || row.ScopeProof != constUtil.P6ArenaNativeProof {
		t.Fatal("current arena receipt is missing", err)
	}
	var award db.P11ArenaPrizeAward
	if err = db.GetIns().First(&award, "fight_id=?", row.FightID).Error; err != nil {
		t.Fatal("arena prize receipt is missing", err)
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
	proof := constUtil.P6ArenaRevalidated20261029Proof
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
		retry, err := f.s.p11ArenaScopedFightAt(f.ctx, challenger, request, 86002, now.Add(time.Hour))
		if err != nil {
			t.Fatal("historical arena retry", err)
		}
		raw, err := proto.MarshalOptions{Deterministic: true}.Marshal(retry[0])
		if err != nil || !bytes.Equal(raw, expected) {
			t.Fatal("historical arena response changed", err)
		}
	}
	if !bytes.Equal(before, p6RenewalProfiles(t, challenger, defender)) {
		t.Fatal("historical arena retry changed profiles")
	}
	var ranksAfter []db.P11ArenaMember
	if err = db.GetIns().Order("player_id").Find(&ranksAfter).Error; err != nil {
		t.Fatal(err)
	}
	rankBytes, _ := json.Marshal(ranksAfter)
	if !bytes.Equal(ranksBefore, rankBytes) {
		t.Fatal("historical arena retry changed rankings")
	}
	p6RenewalCheckCounts(t, challenger, &db.P11ArenaFightSettlement{})
	var awardAfter db.P11ArenaPrizeAward
	if err = db.GetIns().First(&awardAfter, "fight_id=?", row.FightID).Error; err != nil {
		t.Fatal(err)
	}
	awardBytes, _ := json.Marshal(awardAfter)
	if !bytes.Equal(awardBefore, awardBytes) {
		t.Fatal("historical arena retry changed prize receipt")
	}
}

func TestP6Certificate86Friend20261029ReceiptsCold(t *testing.T) {
	legacy := map[string]string{
		"starter": constUtil.P6StarterRevalidated20261029Proof,
		"owned":   constUtil.P6OwnedPartnerRevalidated20261029Proof,
		"linear":  constUtil.P6LinearPhysicalRevalidated20261029Proof,
	}
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
					if err := updateProfile(f.ctx, id, func(p *pb.Player, _ *model.GameState) error { p.Level = proto.Int32(100); return nil }); err != nil {
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
			const stamp int64 = 86003
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
				t.Fatal("current friend receipt is missing", err)
			}
			if err = db.GetIns().Model(&db.P11ArenaPolicy{}).Where("id=1").Updates(map[string]any{"enabled": false, "duel_enabled": false}).Error; err != nil {
				t.Fatal(err)
			}
			before := p6RenewalProfiles(t, f.id, peer)
			proof := legacy[family.name]
			if proof == "" {
				t.Fatal("historical family proof is missing")
			}
			terminal := p6RenewalHistoricalTerminal(t, row.P6RecordID, row.Terminal, proof)
			if err = db.GetIns().Model(&row).Updates(map[string]any{"scope_proof": proof, "terminal": terminal, "terminal_hash": p6Digest(terminal)}).Error; err != nil {
				t.Fatal(err)
			}
			f.restart(t)
			for i := 0; i < 2; i++ {
				retry, err := f.s.p11FriendScopedFightAt(f.ctx, f.id, request, stamp, now.Add(time.Hour))
				if err != nil {
					t.Fatal("historical friend retry", err)
				}
				raw, err := proto.MarshalOptions{Deterministic: true}.Marshal(retry[0])
				if err != nil || !bytes.Equal(raw, expected) {
					t.Fatal("historical friend response changed", err)
				}
			}
			if !bytes.Equal(before, p6RenewalProfiles(t, f.id, peer)) {
				t.Fatal("historical friend retry changed profiles")
			}
			p6RenewalCheckCounts(t, f.id, &db.P11FriendFightSettlement{})
		})
	}
}
