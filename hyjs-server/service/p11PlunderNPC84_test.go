package service

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

// The wire power is independently recalculated from the actual battle actors,
// then each roster enters the existing type3 mechanism/terminal validator.
func TestP11Plunder84TenActualRostersAndCertifiedComputation(t *testing.T) {
	s := &GameService{}
	if err := s.LoadTables("../../analysis/tables"); err != nil {
		t.Fatal(err)
	}
	if err := s.LoadP5Tables("../../analysis/tables"); err != nil {
		t.Fatal(err)
	}
	if err := s.LoadP11Tables("../../analysis/tables"); err != nil {
		t.Fatal(err)
	}
	if err := s.loadP7Table("../../analysis/tables", "BuffInfExtend"); err != nil {
		t.Fatal(err)
	}
	attacker, err := s.p11PlunderNPC84Projection(0)
	if err != nil {
		t.Fatal(err)
	}
	for index, template := range constUtil.P11PlunderNPC84Templates() {
		t.Run(fmt.Sprintf("roster-%02d", index+1), func(t *testing.T) {
			candidate, err := s.p11PlunderNPC84Projection(index)
			if err != nil {
				t.Fatal(err)
			}
			if len(candidate.Battle.Formation) != len(template.Heroes) {
				t.Fatal("original heroes missing")
			}
			var total float64
			for i, actor := range candidate.Battle.Formation {
				if actor.GetId() != template.Heroes[i] || actor.GetInst().GetCurHp() != actor.GetProp().GetHp() {
					t.Fatal("roster or active HP changed")
				}
				original := s.table("Hero", "id", actor.GetId())
				if original == nil {
					t.Fatal("original hero absent")
				}
				var baseline float64
				for _, key := range []string{"hit", "dodge", "cri", "antiCri", "criFactor", "damageAdd", "damageReduce"} {
					baseline += tableNumber(original, key)
				}
				prop := actor.GetProp()
				var weighted float64
				properties := map[string]float64{"att": prop.GetAtt(), "hp": prop.GetHp(), "pdef": prop.GetPdef(), "mdef": prop.GetMdef()}
				for _, key := range []string{"att", "hp", "pdef", "mdef"} {
					factor, e := strconv.ParseFloat(s.table("GameConfigure", "StaticName", "fightpower_"+key+"_factor")["NameValue"], 64)
					if e != nil {
						t.Fatal(e)
					}
					weighted += properties[key] * factor
				}
				total += weighted * 0.1 * (100 + prop.GetHit() + prop.GetDodge() + prop.GetCri() + prop.GetAntiCri() + prop.GetCriFactor() + prop.GetDamageAdd() + prop.GetDamageReduce() - baseline) * 0.01
			}
			if int32(math.RoundToEven(total)) != candidate.Info.GetFightPower() {
				t.Fatalf("wire=%d actual=%f", candidate.Info.GetFightPower(), total)
			}
			left := proto.Clone(attacker.Battle).(*pb.BattlePlayer)
			left.PlayerId = proto.Int64(42)
			right := proto.Clone(candidate.Battle).(*pb.BattlePlayer)
			if err = SetP6FirstOrder(left, right, false); err != nil {
				t.Fatal(err)
			}
			input, err := s.FreezeBattleInput(model.FrozenBattleInput{Attacker: left, Defender: right, BattleType: 3, MaxRound: 20, Seed: 84})
			if err != nil {
				t.Fatal(err)
			}
			qualification, err := s.ValidateP6GenericMechanisms(t.Context(), input)
			if err != nil {
				t.Fatal("original source admission", err)
			}
			terminal, err := s.ComputeP6PlunderSettlement(t.Context(), input)
			if err != nil || terminal == nil || terminal.Outcome == nil || !terminal.Outcome.Authoritative || !p6KnownPlunderSettlementProof(input, terminal.Outcome.CapabilityHash) {
				t.Fatal("actual type3 computation", err)
			}
			p6GenericExportRepresentative(t, fmt.Sprintf("npc84-%02d", index+1), input, terminal, qualification)
			p11Plunder84ExportMetadata(t, fmt.Sprintf("npc84-%02d", index+1), "frag_trea_001_1", candidate.Info, candidate.Battle, nil)
			t.Logf("heroes=%v target=%d actual=%d delta=%d mechanism=%s proof=%s", template.Heroes, template.TargetPower, candidate.Info.GetFightPower(), candidate.Info.GetFightPower()-template.TargetPower, qualification.MechanismHash, terminal.Outcome.CapabilityHash)
		})
	}
}

func TestP11Plunder84RealHolderPriorityRealmAndNoFakeProfiles(t *testing.T) {
	f, first, second, now := p11PlunderFightSetup(t)
	p11PlunderPublish(t, now)
	fragment := "frag_trea_001_1"
	var before int64
	if err := db.GetIns().Model(&db.Profile{}).Count(&before).Error; err != nil {
		t.Fatal(err)
	}
	list := func(fragment string) []*pb.TreasureBattleInfo {
		t.Helper()
		out, err := f.s.p11PlunderListAt(f.ctx, f.id, fragment, now)
		if err != nil {
			t.Fatal(err)
		}
		return out[0].(*pb.SCTreasureBattleList).InfoList
	}
	items := list(fragment)
	if len(items) != 3 {
		t.Fatal("empty realm should expose three virtual candidates", len(items))
	}
	for _, item := range items {
		if item.GetIsRobot() != 1 || !p11PlunderNPC84ID(item.GetSimplePlayerInfo().GetPlayerId()) || item.GetSimplePlayerInfo().GetServerId() != int32(f.s.OriginalServerID()) {
			t.Fatal("virtual identity or realm")
		}
	}
	if got := list("frag_trea_015_1"); len(got) != 0 {
		t.Fatal("red treasure gained virtual candidates")
	}
	p11PlunderFiniteItems(t, f, first, fragment+",1")
	if got := list(fragment); len(got) != 1 || got[0].GetIsRobot() != 0 || got[0].GetSimplePlayerInfo().GetPlayerId() != first {
		t.Fatal("one real holder was padded with NPCs", got)
	}
	p11PlunderFiniteItems(t, f, second, fragment+",1")
	if got := list(fragment); len(got) != 2 {
		t.Fatal("two real holders were padded with NPCs", got)
	}
	if err := db.GetIns().Model(&db.Profile{}).Where("id=?", second).Update("server_id", f.s.OriginalServerID()+1).Error; err != nil {
		t.Fatal(err)
	}
	if got := list(fragment); len(got) != 1 || got[0].GetSimplePlayerInfo().GetPlayerId() != first {
		t.Fatal("other realm holder was selected", got)
	}
	var after int64
	if err := db.GetIns().Model(&db.Profile{}).Count(&after).Error; err != nil || after != before {
		t.Fatal("virtual candidates created profiles", before, after, err)
	}
}

func TestP11Plunder84ScanLimitNeverDeclaresUnknownEmpty(t *testing.T) {
	f, _, _, now := p11PlunderFightSetup(t)
	for i := 0; i < 2; i++ {
		account, err := LoginAccount(f.ctx, fmt.Sprintf("p11-plunder-limit-%d-%d", i, time.Now().UnixNano()), "local-isolated-password")
		if err != nil {
			t.Fatal(err)
		}
		if _, err = CreatePlayer(f.ctx, account.Id, fmt.Sprintf("界限玩家%d", i), "hero_047"); err != nil {
			t.Fatal(err)
		}
	}
	p11PlunderPublish(t, now)
	if err := db.GetIns().Model(&db.P11PlunderFightPolicy{}).Where("id=1").Update("scan_limit", 3).Error; err != nil {
		t.Fatal(err)
	}
	out, err := f.s.p11PlunderListAt(f.ctx, f.id, "frag_trea_001_1", now)
	if err != nil || len(out[0].(*pb.SCTreasureBattleList).InfoList) != 0 {
		t.Fatal("incomplete bounded scan invented empty-realm NPCs", err)
	}
}

func TestP11Plunder84PowerDistanceTieUsesLowerPower(t *testing.T) {
	f, _, _, _ := p11PlunderFightSetup(t)
	low, err := f.s.p11PlunderNPC84Projection(0)
	if err != nil {
		t.Fatal(err)
	}
	high, err := f.s.p11PlunderNPC84Projection(1)
	if err != nil {
		t.Fatal(err)
	}
	midpoint := int32((int64(low.Info.GetFightPower()) + int64(high.Info.GetFightPower())) / 2)
	candidates, err := f.s.p11PlunderNPC84Candidates(db.GetIns(), midpoint)
	if err != nil || len(candidates) != 3 || candidates[0].Index != 0 || candidates[1].Index != 1 {
		t.Fatal("nearest candidates or lower-power tie ordering", err)
	}
}

func TestP11Plunder84AutoEnergyUsesOwnedMedicineAndColdReceipt(t *testing.T) {
	for _, tc := range []struct {
		name         string
		energy       int32
		medicine     bool
		count        int32
		wantChildren int32
		wantMedicine int32
		wantError    bool
	}{
		{"energy-sufficient-no-medicine", 242, false, 1, 1, 0, false},
		{"medicine-restores-ten", 0, true, 1, 1, 1, false},
		{"one-bottle-five-fights", 0, true, 5, 5, 1, false},
		{"partial-before-empty", 2, false, 5, 1, 0, false},
		{"no-energy-no-medicine", 0, false, 1, 0, 0, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f, opponent, _, now := p11PlunderFightSetup(t)
			if err := f.s.loadP7Table("../../analysis/tables", "BuffInfExtend"); err != nil {
				t.Fatal(err)
			}
			if err := updateProfile(f.ctx, f.id, func(player *pb.Player, _ *model.GameState) error {
				player.Level = proto.Int32(30)
				player.Energy = proto.Int32(tc.energy)
				player.EnergyLastRefreshTime = proto.Int64(time.Now().UnixMilli())
				return nil
			}); err != nil {
				t.Fatal(err)
			}
			p11PlunderFiniteItems(t, f, opponent, "frag_trea_001_1,1")
			if tc.medicine {
				p11PlunderFiniteItems(t, f, f.id, "item_001,1")
			}
			p11PlunderPublish(t, now)
			if err := db.GetIns().Model(&db.P11PlunderFightPolicy{}).Where("id=1").Updates(map[string]any{"sweep_enabled": true, "drop_ppm": 0}).Error; err != nil {
				t.Fatal(err)
			}
			request := &pb.CSSweepTreasureFight{FragId: proto.String("frag_trea_001_1"), SweepNumber: proto.Int32(tc.count), AutoAddEnergy: proto.Int32(1)}
			before, beforeGame := p10LuckyProfile(t, f.id)
			out, err := f.s.p11PlunderSweepAt(f.ctx, f.id, request, 8401, now)
			if tc.wantError {
				if err == nil {
					t.Fatal("empty energy and medicine accepted")
				}
				after, afterGame := p10LuckyProfile(t, f.id)
				if !proto.Equal(before, after) || simpleOwned(after, afterGame, "item_001") != simpleOwned(before, beforeGame, "item_001") {
					t.Fatal("rejected request changed resources")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			response := out[0].(*pb.SCSweepTreasureFight)
			if len(response.RewardList) != int(tc.wantChildren) {
				t.Fatal("completed child count", len(response.RewardList))
			}
			var energyCost, medicineCost, energyGain int32
			for _, cost := range response.Cost {
				if cost.GetId() == "energy" {
					energyCost += cost.GetNum()
				}
				if cost.GetId() == "item_001" {
					medicineCost += cost.GetNum()
				}
			}
			for _, gain := range response.Get.GetSimps() {
				if gain.GetId() == "energy" {
					energyGain += gain.GetNum()
				}
			}
			after, afterGame := p10LuckyProfile(t, f.id)
			if energyCost != tc.wantChildren*2 || medicineCost != tc.wantMedicine || energyGain != tc.wantMedicine*10 || after.GetEnergy() != before.GetEnergy()+energyGain-energyCost || simpleOwned(after, afterGame, "item_001") != simpleOwned(before, beforeGame, "item_001")-int64(medicineCost) {
				t.Fatal("cost, gain, and balances disagree", response)
			}
			f.restart(t)
			retry, err := f.s.p11PlunderSweepAt(f.ctx, f.id, request, 8401, now.Add(time.Second))
			if err != nil || !proto.Equal(response, retry[0]) {
				t.Fatal("cold receipt changed", err)
			}
		})
	}
}

func TestP11Plunder84NPCFightAndSweepUseFrozenSourceAndColdReceipts(t *testing.T) {
	f, other, _, now := p11PlunderFightSetup(t)
	if err := f.s.LoadP5Tables("../../analysis/tables"); err != nil {
		t.Fatal(err)
	}
	if err := f.s.loadP7Table("../../analysis/tables", "BuffInfExtend"); err != nil {
		t.Fatal(err)
	}
	p11PlunderPublish(t, now)
	if err := db.GetIns().Model(&db.P11PlunderFightPolicy{}).Where("id=1").Updates(map[string]any{"sweep_enabled": true, "drop_ppm": 1000000}).Error; err != nil {
		t.Fatal(err)
	}
	var otherBefore db.Profile
	if err := db.GetIns().First(&otherBefore, other).Error; err != nil {
		t.Fatal(err)
	}
	listed, err := f.s.p11PlunderListAt(f.ctx, f.id, "frag_trea_001_1", now)
	if err != nil || len(listed[0].(*pb.SCTreasureBattleList).InfoList) != 3 {
		t.Fatal("empty local candidates", err)
	}
	opponent := listed[0].(*pb.SCTreasureBattleList).InfoList[0].GetSimplePlayerInfo().GetPlayerId()
	request := &pb.CSFightTreasureStartResult{FightPlayerId: proto.Int64(opponent), TargetFlagId: proto.String("frag_trea_001_1")}
	before, _ := p10LuckyProfile(t, f.id)
	result, err := f.s.p11PlunderFightAt(f.ctx, f.id, request, 8402, now)
	if err != nil {
		t.Fatal("actual NPC computation", err)
	}
	response := result[0].(*pb.SCFightTreasureStartResult)
	var receipt db.P11PlunderFightSettlement
	if err = db.GetIns().First(&receipt, "player_id=? AND stamp=?", f.id, 8402).Error; err != nil {
		t.Fatal(err)
	}
	var record db.P6BattleRecord
	if err = db.GetIns().First(&record, "id=?", receipt.P6RecordID).Error; err != nil {
		t.Fatal(err)
	}
	input, err := decodeP6Record(&record)
	if err != nil || len(input.AppliedProperties["plunderNPC"]) != 4 || input.AppliedProperties["plunderNPC"][0] != constUtil.P11PlunderNPC84Source || !p6KnownPlunderSettlementProof(input, receipt.ScopeProof) {
		t.Fatal("NPC source was not frozen with actual type3 proof", err)
	}
	p11Plunder84ExportReceipt(t, f, &receipt, "npc84-single")
	after, afterGame := p10LuckyProfile(t, f.id)
	owned, err := p11PlunderFragmentCount(afterGame, "frag_trea_001_1")
	if err != nil || after.GetEnergy() != before.GetEnergy()-2 || owned != response.GetIsGetFrag() {
		t.Fatal("NPC battle did not apply actual cost and outcome")
	}
	var otherAfter db.Profile
	if err = db.GetIns().First(&otherAfter, other).Error; err != nil || *otherBefore.Player != *otherAfter.Player || *otherBefore.Game != *otherAfter.Game {
		t.Fatal("virtual target changed another real player", err)
	}
	var npcCount int64
	if err = db.GetIns().Model(&db.Profile{}).Where("id>? AND id<=?", constUtil.P11PlunderNPC84IDBase, constUtil.P11PlunderNPC84IDBase+10).Count(&npcCount).Error; err != nil || npcCount != 0 {
		t.Fatal("virtual player profile created", err)
	}
	sweep := &pb.CSSweepTreasureFight{FragId: proto.String("frag_trea_002_1"), SweepNumber: proto.Int32(1), AutoAddEnergy: proto.Int32(1)}
	swept, err := f.s.p11PlunderSweepAt(f.ctx, f.id, sweep, 8403, now)
	if err != nil || len(swept[0].(*pb.SCSweepTreasureFight).RewardList) != 1 {
		t.Fatal("actual NPC sweep", err)
	}
	var aggregate db.P11PlunderSweepReceipt
	if err = db.GetIns().First(&aggregate, "player_id=? AND stamp=?", f.id, 8403).Error; err != nil {
		t.Fatal(err)
	}
	var plan p11PlunderSweepPlan
	if json.Unmarshal(aggregate.Plan, &plan) != nil || len(plan.Children) != 1 || !p11PlunderNPC84ID(plan.Peers[0]) {
		t.Fatal("NPC sweep was not source-bound")
	}
	var child db.P11PlunderFightSettlement
	if err = db.GetIns().First(&child, "player_id=? AND stamp=?", f.id, plan.Children[0].Stamp).Error; err != nil {
		t.Fatal(err)
	}
	p11Plunder84ExportReceipt(t, f, &child, "npc84-sweep")
	f.restart(t)
	replay, err := f.s.p11PlunderFightAt(f.ctx, f.id, request, 8402, now.Add(time.Second))
	if err != nil || !proto.Equal(response, replay[0]) {
		t.Fatal("NPC cold single receipt changed", err)
	}
	replayedSweep, err := f.s.p11PlunderSweepAt(f.ctx, f.id, sweep, 8403, now.Add(time.Second))
	if err != nil || !proto.Equal(swept[0], replayedSweep[0]) {
		t.Fatal("NPC cold sweep receipt changed", err)
	}
}

// Optional local-original-engine probes capture the actual accepted receipt's
// frozen input and computed terminal; the exporter never changes game state.
func p11Plunder84ExportReceipt(t *testing.T, f *p1Fixture, receipt *db.P11PlunderFightSettlement, name string) {
	t.Helper()
	if os.Getenv("HYJS_P6_GENERIC_EXPORT_DIR") == "" {
		return
	}
	var record db.P6BattleRecord
	if err := db.GetIns().First(&record, "id=?", receipt.P6RecordID).Error; err != nil {
		t.Fatal(err)
	}
	input, err := decodeP6Record(&record)
	if err != nil {
		t.Fatal(err)
	}
	var result model.CombatFinalHPOutcome
	if err = json.Unmarshal(receipt.Terminal, &result); err != nil {
		t.Fatal(err)
	}
	qualification, err := f.s.ValidateP6GenericMechanisms(t.Context(), input)
	if err != nil {
		t.Fatal(err)
	}
	response, err := p11ReadPlunderReceipt(db.GetIns(), receipt)
	if err != nil || record.SourceFight != receipt.FightID || response.GetFightId() != receipt.FightID || response.GetRandSeed() != input.Seed || len(response.PlayerList) != 2 || !proto.Equal(response.PlayerList[0], input.Attacker) || !proto.Equal(response.PlayerList[1], input.Defender) {
		t.Fatal("actual plunder callback and frozen source disagree", err)
	}
	// Keep the accepted fight ID in both diagnostic wire and the actual callback;
	// unlike a standalone roster probe, this input came from a settled child.
	directory, err := filepath.Abs(os.Getenv("HYJS_P6_GENERIC_EXPORT_DIR"))
	if err != nil {
		t.Fatal(err)
	}
	root, err := filepath.Abs("../../analysis")
	if err != nil {
		t.Fatal(err)
	}
	relative, err := filepath.Rel(root, directory)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		t.Fatal("native probes must remain under analysis")
	}
	if err = os.MkdirAll(directory, 0755); err != nil {
		t.Fatal(err)
	}
	wire := &pb.SCFightPlayback{Fightid: proto.String(receipt.FightID), RandSeed: proto.Int64(input.Seed), BattleType: proto.Int32(input.BattleType), Attacker: input.Attacker, Defender1: input.Defender, Defender2: input.Monster, MaxRound: proto.Int32(input.MaxRound), AttackerFactor: proto.Float64(1), DefenderFactor: proto.Float64(1)}
	wireBytes, err := proto.MarshalOptions{Deterministic: true}.Marshal(wire)
	if err != nil {
		t.Fatal(err)
	}
	files := map[string][]byte{name + ".pb": wireBytes, name + ".actual-response.pb": receipt.Response}
	for suffix, value := range map[string]any{".frozen-input.json": input, ".go-final.json": &result, ".qualification.json": qualification} {
		files[name+suffix], err = json.MarshalIndent(value, "", "  ")
		if err != nil {
			t.Fatal(err)
		}
	}
	for filename, contents := range files {
		file, e := os.OpenFile(filepath.Join(directory, filename), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0644)
		if e != nil {
			t.Fatal(e)
		}
		_, writeErr := file.Write(contents)
		closeErr := file.Close()
		if writeErr != nil || closeErr != nil {
			t.Fatal("write accepted plunder source", writeErr, closeErr)
		}
	}
	var match db.P11PlunderFightMatch
	if err = db.GetIns().First(&match, "id=?", receipt.MatchID).Error; err != nil {
		t.Fatal(err)
	}
	info := &pb.TreasureBattleInfo{}
	if err = proto.Unmarshal(match.Info, info); err != nil {
		t.Fatal(err)
	}
	p11Plunder84ExportMetadata(t, name, receipt.FragmentID, info.GetSimplePlayerInfo(), input.Defender, receipt)
}

func p11Plunder84ExportMetadata(t *testing.T, name, fragment string, info *pb.SimplePlayerInfo, battle *pb.BattlePlayer, receipt *db.P11PlunderFightSettlement) {
	t.Helper()
	directory := os.Getenv("HYJS_P6_GENERIC_EXPORT_DIR")
	if directory == "" {
		return
	}
	index := int(info.GetPlayerId() - constUtil.P11PlunderNPC84IDBase - 1)
	templates := constUtil.P11PlunderNPC84Templates()
	if index < 0 || index >= len(templates) || battle.GetPlayerId() != info.GetPlayerId() {
		t.Fatal("virtual roster export identity")
	}
	var heroes []string
	for _, actor := range battle.Formation {
		heroes = append(heroes, actor.GetId())
	}
	metadata := map[string]any{"source": constUtil.P11PlunderNPC84Source, "npcID": info.GetPlayerId(), "template": index + 1, "heroes": heroes, "targetFragment": fragment, "targetPower": templates[index].TargetPower, "actualPower": info.GetFightPower(), "powerDifference": info.GetFightPower() - templates[index].TargetPower}
	if receipt != nil {
		metadata["fightID"], metadata["receiptResponseSHA256"], metadata["recordID"], metadata["matchHash"] = receipt.FightID, receipt.ResponseHash, receipt.P6RecordID, receipt.MatchHash
	}
	raw, err := json.MarshalIndent(metadata, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	file, err := os.OpenFile(filepath.Join(directory, name+".template.json"), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0644)
	if err != nil {
		t.Fatal(err)
	}
	_, writeErr := file.Write(raw)
	closeErr := file.Close()
	if writeErr != nil || closeErr != nil {
		t.Fatal("write roster metadata", writeErr, closeErr)
	}
}

func TestP11Plunder84MatchingChecksCurrentTeamAcrossSameMillisecond(t *testing.T) {
	f, opponent, _, now := p11PlunderFightSetup(t)
	if err := f.s.loadP7Table("../../analysis/tables", "BuffInfExtend"); err != nil {
		t.Fatal(err)
	}
	p11PlunderFiniteItems(t, f, opponent, "frag_trea_001_1,1")
	p11PlunderPublish(t, now)
	if _, err := f.s.p11PlunderListAt(f.ctx, f.id, "frag_trea_001_1", now); err != nil {
		t.Fatal(err)
	}
	if err := updateProfile(f.ctx, f.id, func(p *pb.Player, _ *model.GameState) error { p.Level = proto.Int32(21); return nil }); err != nil {
		t.Fatal(err)
	}
	if _, err := f.s.p11PlunderListAt(f.ctx, f.id, "frag_trea_001_1", now); err != nil {
		t.Fatal(err)
	}
	request := &pb.CSFightTreasureStartResult{FightPlayerId: proto.Int64(opponent), TargetFlagId: proto.String("frag_trea_001_1")}
	if _, err := f.s.p11PlunderFightAt(f.ctx, f.id, request, 8404, now); err != nil {
		t.Fatal("fresh team should match one of the same-time frozen offers", err)
	}
	if _, err := f.s.p11PlunderListAt(f.ctx, f.id, "frag_trea_001_1", now.Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	if err := updateProfile(f.ctx, f.id, func(p *pb.Player, _ *model.GameState) error { p.Level = proto.Int32(22); return nil }); err != nil {
		t.Fatal(err)
	}
	if _, err := f.s.p11PlunderFightAt(f.ctx, f.id, request, 8405, now.Add(time.Second)); err == nil {
		t.Fatal("actual changed team reused stale snapshot")
	}
}
