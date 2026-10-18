package service

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

func p11PlunderFightSetup(t *testing.T) (*p1Fixture, int64, int64, time.Time) {
	t.Helper()
	f, b, c := p11ArenaSetup(t)
	if err := f.s.InitializeP11PlunderFight(f.ctx); err != nil {
		t.Fatal(err)
	}
	for _, table := range []any{&db.P11PlunderFightSettlement{}, &db.P11PlunderFightMatch{}, &db.P11PlunderFightOffer{}, &db.P11PlunderFightPolicy{}} {
		if err := db.GetIns().Where("1=1").Delete(table).Error; err != nil {
			t.Fatal(err)
		}
	}
	if err := f.s.InitializeP11PlunderFight(f.ctx); err != nil {
		t.Fatal(err)
	}
	return f, b, c, time.Now().Truncate(time.Millisecond)
}

func p11PlunderPublish(t *testing.T, now time.Time) {
	t.Helper()
	if err := db.GetIns().Model(&db.P11PlunderFightPolicy{}).Where("id=1").Updates(map[string]any{"enabled": true, "published_at": now.Add(-3 * time.Hour).UnixMilli(), "start_at": now.Add(-2 * time.Hour).UnixMilli(), "end_at": now.Add(3 * time.Hour).UnixMilli()}).Error; err != nil {
		t.Fatal(err)
	}
}

// p11PlunderFiniteItems provides explicit finite qualification inventory through
// the existing grant primitive. This fixture does not claim a natural drop or
// fabricate any battle result; the actual public source chain is a native test.
func p11PlunderFiniteItems(t *testing.T, f *p1Fixture, id int64, contents string) {
	t.Helper()
	if err := updateProfile(f.ctx, id, func(player *pb.Player, game *model.GameState) error {
		_, err := f.s.GrantFixedReward(player, game, contents)
		return err
	}); err != nil {
		t.Fatal(err)
	}
}

func TestP11PlunderRealHolderFreezeProtectionConsumeAndIndependentPending(t *testing.T) {
	f, b, _, now := p11PlunderFightSetup(t)
	query := &pb.CSTreasureBattleList{FragTreasId: proto.String("frag_trea_001_1")}
	if _, err := f.s.p11PlunderListAt(f.ctx, f.id, query.GetFragTreasId(), now); err == nil {
		t.Fatal("default-off policy generated candidates")
	}
	var count int64
	db.GetIns().Model(&db.P11PlunderFightOffer{}).Count(&count)
	if count != 0 {
		t.Fatal("closed request froze a historical offer")
	}
	p11PlunderFiniteItems(t, f, b, "frag_trea_001_1,1;item_015,1")
	p11PlunderPublish(t, now)
	list, err := f.s.p11PlunderListAt(f.ctx, f.id, query.GetFragTreasId(), now)
	if err != nil {
		t.Fatal(err)
	}
	out := list[0].(*pb.SCTreasureBattleList)
	if len(out.InfoList) != 1 || out.InfoList[0].GetSimplePlayerInfo().GetPlayerId() != b || out.InfoList[0].GetIsRobot() != 0 || out.InfoList[0].GetGetChance() != 3 {
		t.Fatal("original visible candidate requires actual holder and1..6 chance tier", out)
	}
	var match db.P11PlunderFightMatch
	if err = db.GetIns().Where("player_id=?", f.id).First(&match).Error; err != nil {
		t.Fatal(err)
	}
	attacker, defender := &pb.BattlePlayer{}, &pb.BattlePlayer{}
	if proto.Unmarshal(match.Attacker, attacker) != nil || proto.Unmarshal(match.Defender, defender) != nil || attacker.GetPlayerId() != f.id || defender.GetPlayerId() != b || attacker.GetInst() == nil || defender.GetInst() == nil || match.SourceHash != p11PlunderMatchHash(&match) || len(attacker.Formation) != 1 || len(defender.Formation) != 1 || attacker.Formation[0].GetGuid() == "" || attacker.Formation[0].GetSkillOrder() != "047_1,047_2,047_3" {
		t.Fatal("actual complete original teams/Inst/source did not freeze")
	}
	beforeP, beforeG := p10LuckyProfile(t, f.id)
	beforeGame, _ := json.Marshal(beforeG)
	request := &pb.CSFightTreasureStartResult{FightPlayerId: proto.Int64(b), TargetFlagId: query.FragTreasId}
	if constUtil.P6PlunderNativeProof == "" {
		var beforeRecords int64
		db.GetIns().Model(&db.P6BattleRecord{}).Count(&beforeRecords)
		_, err = f.s.p11PlunderFightAt(f.ctx, f.id, request, 101, now)
		if err == nil || !strings.Contains(err.Error(), constUtil.CombatUnsupported) {
			t.Fatal("independent type3 certificate must reject its complete original input", err)
		}
		db.GetIns().Model(&db.P6BattleRecord{}).Count(&count)
		if count != beforeRecords {
			t.Fatal("rejected proof persisted a P6 record")
		}
		db.GetIns().Model(&db.P11PlunderFightSettlement{}).Count(&count)
		if count != 0 {
			t.Fatal("rejected proof committed resources/result")
		}
	}
	afterP, afterG := p10LuckyProfile(t, f.id)
	afterGame, _ := json.Marshal(afterG)
	if !proto.Equal(beforeP, afterP) || string(beforeGame) != string(afterGame) {
		t.Fatal("unpaid matching/failed fight changed resources or role")
	}
	if err = db.GetIns().Model(&db.P11PlunderPolicy{}).Where("id=1").Update("avoid_enabled", true).Error; err != nil {
		t.Fatal(err)
	}
	if _, err = f.s.p11PlunderAt(f.ctx, b, &pb.CSUseAvoidPlunder{UsedItem: proto.String("item_015")}, 102, now); err != nil {
		t.Fatal(err)
	}
	list, err = f.s.p11PlunderListAt(f.ctx, f.id, query.GetFragTreasId(), now)
	if err != nil || len(list[0].(*pb.SCTreasureBattleList).InfoList) != 0 {
		t.Fatal("actually protected holder remained selectable", err)
	}
	if _, err = f.s.p11PlunderFightAt(f.ctx, f.id, request, 103, now); err == nil || !strings.Contains(err.Error(), constUtil.P11PlunderFightRefresh) {
		t.Fatal("cached candidate bypassed current real protection", err)
	}
	// Advance only the domain test's business clock beyond the actual original
	// one-hour stone expiry. No session/avoid deadline or winner is rewritten.
	later := now.Add(time.Hour + time.Second)
	if err = db.GetIns().Model(&db.P11PlunderFightPolicy{}).Where("id=1").Updates(map[string]any{"chance_tier": 6, "drop_ppm": 1000000}).Error; err != nil {
		t.Fatal(err)
	}
	list, err = f.s.p11PlunderListAt(f.ctx, f.id, query.GetFragTreasId(), later)
	if err != nil || len(list[0].(*pb.SCTreasureBattleList).InfoList) != 1 || list[0].(*pb.SCTreasureBattleList).InfoList[0].GetGetChance() != 3 {
		t.Fatal("real expiry or frozen displayed chance drifted", err)
	}
	connection, err := db.GetIns().DB()
	if err != nil {
		t.Fatal(err)
	}
	if err = connection.Close(); err != nil {
		t.Fatal(err)
	}
	f.open(t)
	list, err = f.s.p11PlunderListAt(f.ctx, f.id, query.GetFragTreasId(), later)
	if err != nil || len(list[0].(*pb.SCTreasureBattleList).InfoList) != 1 || list[0].(*pb.SCTreasureBattleList).InfoList[0].GetGetChance() != 3 {
		t.Fatal("cold offer/actual target projection", err)
	}
	p11PlunderFiniteItems(t, f, b, "frag_trea_001_2,1;frag_trea_001_3,1")
	combined, err := f.s.treasureCombine(f.ctx, b, &pb.CSCombine{TargetId: proto.String("trea_001")}, 104)
	if err != nil || len(combined[0].(*pb.SCCombine).GetGet().GetTreas()) != 1 {
		t.Fatal("original full fragment consumption/real treasure entity", err)
	}
	list, err = f.s.p11PlunderListAt(f.ctx, f.id, query.GetFragTreasId(), later)
	if err != nil || len(list[0].(*pb.SCTreasureBattleList).InfoList) != 0 {
		t.Fatal("spent real fragment remained a candidate", err)
	}
	if _, err = f.s.p11PlunderFightAt(f.ctx, f.id, request, 105, later); err == nil || !strings.Contains(err.Error(), constUtil.P11PlunderFightRefresh) {
		t.Fatal("old candidate transferred consumed inventory", err)
	}
}

func TestP11PlunderPrepublicationExactOriginalFragmentAndZeroMutation(t *testing.T) {
	f, b, _, now := p11PlunderFightSetup(t)
	p11PlunderFiniteItems(t, f, b, "frag_trea_001_1,1")
	p11PlunderPublish(t, now)
	if err := db.GetIns().Model(&db.P11PlunderFightPolicy{}).Where("id=1").Update("published_at", now.UnixMilli()).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := f.s.p11PlunderListAt(f.ctx, f.id, "frag_trea_001_1", now); err == nil {
		t.Fatal("late publication manufactured historical offer")
	}
	for _, invalid := range []string{"frag_hero_001", "frag_trea_unknown", "trea_001", "item_001"} {
		if _, err := f.s.p11PlunderListAt(f.ctx, f.id, invalid, now); err == nil {
			t.Fatal("nonoriginal treasure fragment", invalid)
		}
	}
	for _, stamp := range []int64{0, -1} {
		if _, err := f.s.p11PlunderFightAt(f.ctx, f.id, &pb.CSFightTreasureStartResult{FightPlayerId: proto.Int64(b), TargetFlagId: proto.String("frag_trea_001_1")}, stamp, now); err == nil {
			t.Fatal("zero/negative mutation stamp", stamp)
		}
	}
	var count int64
	db.GetIns().Model(&db.P11PlunderFightOffer{}).Count(&count)
	if count != 0 {
		t.Fatal("failed qualifications persisted an offer")
	}
	db.GetIns().Model(&db.P11PlunderFightMatch{}).Count(&count)
	if count != 0 {
		t.Fatal("failed qualifications manufactured a target")
	}
}

// TestP11PlunderCertifiedWinAndDefeatConsumeActualResources exercises the new
// independent type3 certificate. Its finite fixture changes only qualification
// levels and held fragments; both winner and original records are computed.
func TestP11PlunderCertifiedWinAndDefeatConsumeActualResources(t *testing.T) {
	if constUtil.P6PlunderNativeProof == "" {
		t.Skip("independent original type3 certificate is not present in this fixed source")
	}
	for _, strongAttacker := range []bool{true, false} {
		t.Run(fmt.Sprintf("strongAttacker-%t", strongAttacker), func(t *testing.T) {
			f, opponent, _, now := p11PlunderFightSetup(t)
			for _, id := range []int64{f.id, opponent} {
				if err := updateProfile(f.ctx, id, func(player *pb.Player, game *model.GameState) error {
					player.Level = proto.Int32(20)
					if (id == f.id) == strongAttacker {
						player.Level = proto.Int32(100)
					}
					for _, warrior := range game.Warriors {
						if warrior.GetGuid() == player.GetHeroWarId() {
							level := int32(20)
							if (id == f.id) == strongAttacker {
								level = 100
							}
							warrior.Level = proto.Int32(level)
							warrior.Exp = proto.Int64(0)
						}
					}
					return nil
				}); err != nil {
					t.Fatal(err)
				}
			}
			p11PlunderFiniteItems(t, f, opponent, "frag_trea_001_1,1")
			p11PlunderPublish(t, now)
			if err := db.GetIns().Model(&db.P11PlunderFightPolicy{}).Where("id=1").Updates(map[string]any{"drop_ppm": 1000000, "level_difference": 100}).Error; err != nil {
				t.Fatal(err)
			}
			listed, err := f.s.p11PlunderListAt(f.ctx, f.id, "frag_trea_001_1", now)
			if err != nil || len(listed[0].(*pb.SCTreasureBattleList).InfoList) != 1 {
				t.Fatal("real target", err)
			}
			beforeP, _ := p10LuckyProfile(t, f.id)
			targetBefore, _ := p10LuckyProfile(t, opponent)
			request := &pb.CSFightTreasureStartResult{FightPlayerId: proto.Int64(opponent), TargetFlagId: proto.String("frag_trea_001_1")}
			replies, err := f.s.p11PlunderFightAt(f.ctx, f.id, request, 3101, now)
			if err != nil {
				t.Fatal(err)
			}
			actual := replies[0].(*pb.SCFightTreasureStartResult)
			if (actual.GetIsWin() == 1) != strongAttacker || actual.GetMaxRoundNum() != 20 || len(actual.Cost) != 1 || actual.Cost[0].GetId() != "energy" || actual.Cost[0].GetNum() != 2 {
				t.Fatal("computed type3 result and original fee", actual)
			}
			afterP, afterG := p10LuckyProfile(t, f.id)
			targetAfter, targetGame := p10LuckyProfile(t, opponent)
			owned, err := p11PlunderFragmentCount(afterG, "frag_trea_001_1")
			if err != nil {
				t.Fatal(err)
			}
			held, err := p11PlunderFragmentCount(targetGame, "frag_trea_001_1")
			if err != nil {
				t.Fatal(err)
			}
			if afterP.GetEnergy() != beforeP.GetEnergy()-2 || !proto.Equal(targetBefore, targetAfter) {
				t.Fatal("actual fee or unrelated defender state")
			}
			if strongAttacker {
				if owned != 1 || held != 0 || actual.GetIsGetFrag() != 1 || actual.GetReward() != "frag_trea_001_1,1" || afterP.GetCoin() != beforeP.GetCoin()+5500 || len(actual.GetGet().GetSimps()) != 3 {
					t.Fatal("actual winner fragment transfer/base award", actual)
				}
			} else if owned != 0 || held != 1 || actual.GetIsGetFrag() != 0 || len(actual.GetGet().GetSimps()) != 0 || actual.GetReward() != "coin,0" || afterP.GetCoin() != beforeP.GetCoin() {
				t.Fatal("defeat preserves held fragment and grants no local winner resources", actual)
			}
			var settlements, records int64
			db.GetIns().Model(&db.P11PlunderFightSettlement{}).Count(&settlements)
			db.GetIns().Model(&db.P6BattleRecord{}).Where("source=? AND authoritative=?", constUtil.P11PlunderFightSource, true).Count(&records)
			if settlements != 1 || records != 1 {
				t.Fatal("single actual independent accepted result", settlements, records)
			}
			savedP, _ := json.Marshal(afterP)
			savedG, _ := json.Marshal(afterG)
			targetP, _ := json.Marshal(targetAfter)
			targetG, _ := json.Marshal(targetGame)
			connection, err := db.GetIns().DB()
			if err != nil {
				t.Fatal(err)
			}
			if err = connection.Close(); err != nil {
				t.Fatal(err)
			}
			f.open(t)
			// Operator closure affects new matches while the already accepted original
			// result remains readable after restart without another fee or transfer.
			if err = db.GetIns().Model(&db.P11PlunderFightPolicy{}).Where("id=1").Update("enabled", false).Error; err != nil {
				t.Fatal(err)
			}
			repeated, err := f.s.p11PlunderFightAt(f.ctx, f.id, request, 3101, now.Add(time.Second))
			if err != nil || !proto.Equal(repeated[0], actual) {
				t.Fatal("accepted original result after cold", err)
			}
			currentP, currentG := p10LuckyProfile(t, f.id)
			otherP, otherG := p10LuckyProfile(t, opponent)
			rawP, _ := json.Marshal(currentP)
			rawG, _ := json.Marshal(currentG)
			rawOtherP, _ := json.Marshal(otherP)
			rawOtherG, _ := json.Marshal(otherG)
			if string(rawP) != string(savedP) || string(rawG) != string(savedG) || string(rawOtherP) != string(targetP) || string(rawOtherG) != string(targetG) {
				t.Fatal("cold replay consumed another fee or fragment")
			}
		})
	}
}
