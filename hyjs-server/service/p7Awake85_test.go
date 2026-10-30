package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

type awake85TestChapter struct {
	copyID, element, monster, battleMap, task string
	firstBox, dayOffset                       int
	party                                     []awake85TestEnemy
}

type awake85TestEnemy struct {
	id, guid string
	pose     int32
}

// CopyEnemy row IDs, original formation positions, and chapter days are
// independently recorded from the original tables for this finite fixture.
var awake85TestChapters = []awake85TestChapter{
	{"102", "item_322", "mon_024", "map_nightforest_001", "21", 17, 0, []awake85TestEnemy{{"monster_019", "monster-48", 7}, {"monster_019", "monster-49", 9}}},
	{"202", "item_324", "mon_010", "map_ice_001", "9", 13, 1, []awake85TestEnemy{{"monster_033", "monster-18", 7}, {"monster_033", "monster-19", 9}}},
	{"302", "item_323", "mon_030", "map_volcano_001", "9", 9, 2, []awake85TestEnemy{{"monster_033", "monster-18", 7}, {"monster_033", "monster-19", 9}}},
	{"402", "item_321", "mon_016", "map_desert_001", "11", 5, 3, []awake85TestEnemy{{"monster_033", "monster-22", 3}, {"monster_051", "monster-23", 7}, {"monster_051", "monster-24", 9}}},
	{"502", "item_320", "mon_006", "map_forest_003", "3", 1, 4, []awake85TestEnemy{{"monster_006", "monster-4", 3}, {"monster_006", "monster-5", 13}}},
}

func awake85Monday() time.Time {
	return time.Date(2026, 10, 12, 12, 0, 0, 0, time.FixedZone("UTC+8", 8*3600))
}

// Publish the already specified finite configuration inside the isolated test
// schema. The surrounding fixture provides a captured level-75 roster; this
// does not represent a natural player progression or a Unity run.
func awake85BusinessFixture(t *testing.T) *p1Fixture {
	t.Helper()
	f := p7AwakeBusinessFixture(t)
	var policies []db.P7AwakePolicy
	var rules []db.P7AwakeStageRule
	if err := db.GetIns().Where("id IN ?", []int64{constUtil.P7AwakeExplorePolicyID, constUtil.P7AwakeBattlePolicyID}).Order("id").Find(&policies).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.GetIns().Order("copy_id").Find(&rules).Error; err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		for _, policy := range policies {
			if err := db.GetIns().Save(&policy).Error; err != nil {
				t.Error(err)
			}
		}
		for _, rule := range rules {
			if err := db.GetIns().Save(&rule).Error; err != nil {
				t.Error(err)
			}
		}
	})
	// Prior diagnostic rules may remain in a shared isolated test schema.
	// Hide them only for this fixture and restore their exact rows on cleanup.
	if err := db.GetIns().Model(&db.P7AwakeStageRule{}).Where("copy_id NOT IN ?", []string{"102", "202", "302", "402", "501", "502"}).Update("content_restored", false).Error; err != nil {
		t.Fatal(err)
	}
	for _, p := range []db.P7AwakePolicy{
		{ID: constUtil.P7AwakeExplorePolicyID, MinLevel: 70, Enabled: true, Source: constUtil.Awake85ExploreSource},
		{ID: constUtil.P7AwakeBattlePolicyID, MinLevel: 70, Enabled: true, Source: constUtil.Awake85BattleSource},
	} {
		if err := db.GetIns().Save(&p).Error; err != nil {
			t.Fatal(err)
		}
	}
	legacy, err := p7AwakeLocalCatalogue(false)
	if err != nil {
		t.Fatal(err)
	}
	if err = db.GetIns().Save(&legacy).Error; err != nil {
		t.Fatal(err)
	}
	for _, chapter := range awake85Chapters() {
		rule, err := awake85Catalogue(chapter, true)
		if err != nil {
			t.Fatal(err)
		}
		if err = db.GetIns().Save(&rule).Error; err != nil {
			t.Fatal(err)
		}
	}
	return f
}

func awake85ResourceTotals(resources ...[]*pb.BaseResource) map[string]int64 {
	result := map[string]int64{}
	for _, list := range resources {
		for _, resource := range list {
			result[resource.GetId()] += int64(resource.GetNum())
		}
	}
	return result
}

func awake85WarriorCount(g *model.GameState, id string) int64 {
	var count int64
	for _, warrior := range g.Warriors {
		if warrior.GetId() == id {
			count++
		}
	}
	return count
}

func awake85Owned(p *pb.Player, g *model.GameState, id string) int64 {
	if strings.HasPrefix(id, "hero_") {
		return awake85WarriorCount(g, id)
	}
	return simpleOwned(p, g, id)
}

func awake85ReadCatalogue(t *testing.T, f *p1Fixture, now time.Time) *pb.SCAwakeCopyData {
	t.Helper()
	result, err := f.s.p7AwakeCatalogue(f.ctx, f.id, now)
	if err != nil || len(result) != 1 {
		t.Fatal("catalogue", err)
	}
	return result[0].(*pb.SCAwakeCopyData)
}

func TestP7Awake85OriginalCalendarCatalogueSharedBudgetAndDayReset(t *testing.T) {
	f := awake85BusinessFixture(t)
	monday := awake85Monday()
	for day := 0; day < 7; day++ {
		catalogue := awake85ReadCatalogue(t, f, monday.AddDate(0, 0, day))
		if catalogue.GetLastNum() != 2 || len(catalogue.GetCopys()) != 5 {
			t.Fatal("five original chapters and common daily budget", day, catalogue)
		}
		for index, chapter := range catalogue.GetCopys() {
			want := awake85TestChapters[index]
			open := day >= 5 || day == index
			if chapter.GetChapterId() != fmt.Sprint(index+1) || (chapter.GetIsOpen() == 1) != open || len(chapter.GetSmallCopys()) != 1 || chapter.SmallCopys[0].GetCopyId() != want.copyID {
				t.Fatal("weekday, chapter or difficulty changed", day, chapter)
			}
			preview := chapter.SmallCopys[0].GetDropReward()
			if !strings.Contains(preview, want.element) || !strings.Contains(preview, "hero_exp_002") {
				t.Fatal("chapter preview omitted element or score material", day, preview)
			}
		}
	}
	if err := db.GetIns().Model(&db.P7AwakeStageRule{}).Where("copy_id=?", "102").Update("enabled", false).Error; err != nil {
		t.Fatal(err)
	}
	closed := awake85ReadCatalogue(t, f, monday).GetCopys()[0]
	if len(closed.GetSmallCopys()) != 0 {
		t.Fatal("disabled chapter still advertised", closed)
	}
	if _, err := f.s.p7AwakeExplore(f.ctx, f.id, &pb.CSEnterAwakeCopy{CopyId: proto.String("102")}, monday); err == nil {
		t.Fatal("disabled difficulty accepted")
	}
	if err := db.GetIns().Model(&db.P7AwakeStageRule{}).Where("copy_id=?", "102").Update("enabled", true).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := f.s.p7AwakeExplore(f.ctx, f.id, &pb.CSEnterAwakeCopy{CopyId: proto.String("202")}, monday); err == nil {
		t.Fatal("Tuesday chapter accepted on Monday")
	}
	if err := db.GetIns().Model(&db.P7AwakePolicy{}).Where("id=?", constUtil.P7AwakeExplorePolicyID).Update("enabled", false).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := f.s.p7AwakeExplore(f.ctx, f.id, &pb.CSEnterAwakeCopy{CopyId: proto.String("102")}, monday); err == nil {
		t.Fatal("disabled exploration policy accepted a new entry")
	}
	if err := db.GetIns().Model(&db.P7AwakePolicy{}).Where("id=?", constUtil.P7AwakeExplorePolicyID).Update("enabled", true).Error; err != nil {
		t.Fatal(err)
	}
	saturday := monday.AddDate(0, 0, 5).Add(11 * time.Hour)
	before, _ := f.profile(t)
	for i, copyID := range []string{"102", "202"} {
		p7AwakeExploreCall(t, f, &pb.CSEnterAwakeCopy{CopyId: proto.String(copyID)}, saturday)
		p7AwakeExploreCall(t, f, &pb.CSAwakeCopySettle{}, saturday)
		if left := awake85ReadCatalogue(t, f, saturday).GetLastNum(); left != int32(1-i) {
			t.Fatal("chapter-specific entry did not consume common budget", copyID, left)
		}
	}
	if _, err := f.s.p7AwakeExplore(f.ctx, f.id, &pb.CSEnterAwakeCopy{CopyId: proto.String("302")}, saturday); err == nil {
		t.Fatal("third chapter bypassed the shared two-entry budget")
	}
	after, _ := f.profile(t)
	if before.GetStamina() != after.GetStamina() || before.GetEnergy() != after.GetEnergy() || before.GetDiamond() != after.GetDiamond() {
		t.Fatal("zero-stamina entries spent player resources")
	}
	midnight := time.Date(2026, 10, 18, 0, 0, 0, 0, saturday.Location())
	if got := awake85ReadCatalogue(t, f, midnight).GetLastNum(); got != 2 {
		t.Fatal("midnight did not reset common budget", got)
	}
	if _, err := f.s.p7AwakeExplore(f.ctx, f.id, &pb.CSEnterAwakeCopy{CopyId: proto.String("302")}, midnight); err != nil {
		t.Fatal("Sunday entry after reset", err)
	}
}

func TestP7Awake85OriginalBracketWeightBoundariesSelectOne(t *testing.T) {
	f := p7AwakeSetup(t)
	for _, tc := range []struct {
		box         string
		branchIndex int
	}{{"box5", 1}, {"box17", 1}, {"box8", 5}, {"box20", 5}} {
		for _, draw := range []int64{0, 332, 333, 665, 666, 999} {
			calls := 0
			loot, err := f.s.p7AwakeRollBox(tc.box, func(bound int64) (int64, error) {
				if bound != 1000 {
					t.Fatal("original weight denominator", tc.box, bound)
				}
				index := calls
				calls++
				if index == tc.branchIndex {
					return draw, nil
				}
				return 999, nil
			})
			if err != nil || calls != tc.branchIndex+2 {
				t.Fatal("original bracketed group and independent inner draw", tc.box, draw, calls, err)
			}
			selected := "item_317"
			if draw >= 666 {
				selected = "item_319"
			} else if draw >= 333 {
				selected = "item_318"
			}
			for _, candidate := range []string{"item_317", "item_318", "item_319"} {
				if strings.Contains(loot, candidate+",1") != (candidate == selected) {
					t.Fatal("outer 333/333/334 selection must grant exactly one class branch", tc.box, draw, loot)
				}
			}
		}
	}
}

func TestP7Awake85FiveChaptersActualBattleBagAndSettlement(t *testing.T) {
	for _, tc := range awake85TestChapters {
		t.Run(tc.copyID, func(t *testing.T) {
			f := awake85BusinessFixture(t)
			now := awake85Monday().AddDate(0, 0, tc.dayOffset)
			before, beforeGame := f.profile(t)
			entered := p7AwakeExploreCall(t, f, &pb.CSEnterAwakeCopy{CopyId: proto.String(tc.copyID)}, now).(*pb.SCEnterAwakeCopy)
			if len(entered.GetBoxes()) != 4 || len(entered.GetMonsters()) != 1 || entered.Monsters[0].GetMonId() != tc.monster || entered.GetRefreshData().GetEndTime()-now.UnixMilli() != 30*60000 || entered.GetRefreshData().GetScore() != 0 {
				t.Fatal("original node identities, duration and score", entered)
			}
			if !strings.HasPrefix(entered.Monsters[0].GetGuid(), constUtil.Awake85RunPrefix) {
				t.Fatal("new run has no scoped node identity")
			}
			var boxRewards [][]*pb.BaseResource
			for index, box := range entered.Boxes {
				if box.GetBoxId() != fmt.Sprintf("box%d", tc.firstBox+index) {
					t.Fatal("wrong element box", tc.copyID, box)
				}
				opened := p7AwakeExploreCall(t, f, &pb.CSAwakeCopyOpenBox{BoxGuid: box.Guid}, now).(*pb.SCAwakeCopyOpenBox)
				boxRewards = append(boxRewards, opened.GetRewards())
				again := p7AwakeExploreCall(t, f, &pb.CSAwakeCopyOpenBox{BoxGuid: box.Guid}, now).(*pb.SCAwakeCopyOpenBox)
				if len(again.GetRewards()) != 0 || opened.GetRefreshData().GetScore() != int32(5*(index+1)) || again.GetRefreshData().GetScore() != opened.GetRefreshData().GetScore() {
					t.Fatal("duplicate box added loot or score", tc.copyID, box.GetGuid())
				}
			}
			start, end := p7AwakeBusinessStartEnd(t, f, entered.Monsters[0].GetGuid(), now)
			if start.GetRoomId() != tc.battleMap || start.GetMaxRoundNum() != 40 || start.GetBattleMonser().GetTaskId() != tc.task || start.GetBattleMonser().GetGroupIndex() != 0 || len(start.GetBattleMonser().GetFormation()) != len(tc.party) {
				t.Fatal("wrong original CopyEnemy encounter or scene", tc.copyID, start.GetBattleMonser())
			}
			for index, enemy := range start.GetBattleMonser().GetFormation() {
				want := tc.party[index]
				if enemy.GetId() != want.id || enemy.GetGuid() != want.guid || enemy.GetPose() != want.pose {
					t.Fatal("incomplete or substituted original formation", tc.copyID, index, enemy)
				}
			}
			if end.GetIsWinClient() != 1 || end.GetOptRecord() == nil || len(end.GetOptRecord().GetSkillOpts()) == 0 {
				t.Fatal("finite real roster produced no independent winning operation record", tc.copyID)
			}
			accepted, err := f.s.p7AwakeFightEnd(f.ctx, f.id, end, now)
			if err != nil || len(accepted) != 1 {
				t.Fatal("original calculated End", tc.copyID, err)
			}
			battle := accepted[0].(*pb.SCAwakeCopyFightEnd)
			if battle.GetIsWinServer() != 1 || battle.GetRefreshData().GetScore() != 100 || battle.GetFightMonster().GetHpPercent() != 0 {
				t.Fatal("calculated terminal did not award 80 fight points", tc.copyID, battle)
			}
			bag := awake85ResourceTotals(boxRewards...)
			for id, amount := range awake85ResourceTotals(battle.GetRewards()) {
				bag[id] += amount
			}
			if bag[tc.element] < 19 || bag[tc.element] > 20 {
				t.Fatal("four guaranteed element boxes or original monster pool", tc.copyID, bag)
			}
			resumed := p7AwakeExploreCall(t, f, &pb.CSEnterAwakeCopy{CopyId: proto.String(tc.copyID)}, now).(*pb.SCEnterAwakeCopy)
			if resumed.GetRefreshData().GetScore() != 100 || len(resumed.GetMonsters()) != 0 || !reflect.DeepEqual(awake85ResourceTotals(resumed.GetRewards()), bag) {
				t.Fatal("actual rewards were not held in the exploration bag", tc.copyID, resumed)
			}
			pending, pendingGame := f.profile(t)
			for id := range bag {
				if awake85Owned(before, beforeGame, id) != awake85Owned(pending, pendingGame, id) {
					t.Fatal("bag reward entered inventory before exit", tc.copyID, id)
				}
			}
			if before.GetStamina() != pending.GetStamina() || before.GetEnergy() != pending.GetEnergy() || before.GetDiamond() != pending.GetDiamond() {
				t.Fatal("entry consumed resources", tc.copyID)
			}
			stamp := int64(855000 + tc.dayOffset*10)
			settleMessages, err := f.s.p7AwakeExplore(f.ctx, f.id, &pb.CSAwakeCopySettle{}, now, stamp)
			if err != nil {
				t.Fatal("actual exit settlement", tc.copyID, err)
			}
			settled := settleMessages[0].(*pb.SCAwakeCopySettle)
			scoreReward := awake85ResourceTotals(settled.GetScoreRewards())
			if settled.GetScore() != 100 || !reflect.DeepEqual(awake85ResourceTotals(settled.GetBoxRewards()), bag) || !reflect.DeepEqual(scoreReward, map[string]int64{"coin": 5000, "hero_exp_002": 1}) {
				t.Fatal("100-point exit, bag or daily reward", tc.copyID, settled)
			}
			for id, amount := range scoreReward {
				bag[id] += amount
			}
			after, afterGame := f.profile(t)
			for id, amount := range bag {
				if got := awake85Owned(after, afterGame, id) - awake85Owned(before, beforeGame, id); got != amount {
					t.Fatal("actual exit inventory delta", tc.copyID, id, got, amount)
				}
			}
			if p1Count(t, &db.P6BattleRecord{}, "player_id=? AND source=? AND authoritative=?", f.id, constUtil.P7AwakeBattleReplaySource, true) != 1 || p1Count(t, &db.EventFact{}, "player_id=? AND kind=?", f.id, constUtil.P7AwakeCompleteDailyEvent) != 1 {
				t.Fatal("actual calculation and daily award must be committed once", tc.copyID)
			}
			if tc.copyID == "102" {
				f.restart(t)
			}
			repeatedEnd, err := f.s.p7AwakeFightEnd(f.ctx, f.id, end, now)
			if err != nil || !proto.Equal(repeatedEnd[0], accepted[0]) {
				t.Fatal("actual End receipt after retry/restart", tc.copyID, err)
			}
			repeatedSettle, err := f.s.p7AwakeExplore(f.ctx, f.id, &pb.CSAwakeCopySettle{}, now, stamp)
			if err != nil || !proto.Equal(repeatedSettle[0], settled) {
				t.Fatal("exit receipt after retry/restart", tc.copyID, err)
			}
			cold, coldGame := f.profile(t)
			if !proto.Equal(cold, after) || !p6CopySeriesJSONEqual(t, coldGame, afterGame) {
				t.Fatal("retry minted assets", tc.copyID)
			}
		})
	}
}

// Tampering is confined to this fixture's own rows. Recomputing FreezeHash in
// the semantic cases checks that validation binds the values to their sources.
func TestP7Awake85RejectsForeignNodesChangedFreezeAndReplay(t *testing.T) {
	f := awake85BusinessFixture(t)
	now := awake85Monday()
	entered := p7AwakeExploreCall(t, f, &pb.CSEnterAwakeCopy{CopyId: proto.String("102")}, now).(*pb.SCEnterAwakeCopy)
	if _, err := f.s.p7AwakeExplore(f.ctx, f.id, &pb.CSAwakeCopyOpenBox{BoxGuid: proto.String("foreign-box")}, now); err == nil {
		t.Fatal("foreign box accepted")
	}
	if _, err := f.s.p7AwakeFightBegin(f.ctx, f.id, &pb.CSAwakeCopyFightBegin{MonGuid: proto.String("foreign-monster")}, now); err == nil {
		t.Fatal("foreign monster accepted")
	}
	if _, err := f.s.p7AwakeExplore(f.ctx, f.id+123, &pb.CSAwakeCopyOpenBox{BoxGuid: entered.Boxes[0].Guid}, now); err == nil {
		t.Fatal("foreign owner accepted a node")
	}
	var run db.P7AwakeRun
	if err := db.GetIns().Where("player_id=? AND copy_id=?", f.id, "102").First(&run).Error; err != nil {
		t.Fatal(err)
	}
	original := append([]byte(nil), run.Freeze...)
	for _, mode := range []string{"digest", "battle-map", "node-position", "monster-source", "box-loot-key", "monster-loot-key"} {
		var frozen p7AwakeFrozen
		if err := json.Unmarshal(original, &frozen); err != nil {
			t.Fatal(err)
		}
		switch mode {
		case "battle-map":
			frozen.BattleMap = "map_forest_003"
		case "node-position":
			frozen.Content.Boxes[0].Pos = proto.String("999,0,999")
		case "monster-source":
			enemy, err := f.s.p7Awake85Enemy("mon_006")
			if err != nil {
				t.Fatal(err)
			}
			frozen.MonsterSources85[entered.Monsters[0].GetGuid()], err = proto.Marshal(enemy)
			if err != nil {
				t.Fatal(err)
			}
		case "box-loot-key":
			guid := entered.Boxes[0].GetGuid()
			frozen.BoxLoot["foreign-box"] = frozen.BoxLoot[guid]
			delete(frozen.BoxLoot, guid)
		case "monster-loot-key":
			guid := entered.Monsters[0].GetGuid()
			frozen.MonsterLoot["foreign-monster"] = frozen.MonsterLoot[guid]
			delete(frozen.MonsterLoot, guid)
		}
		raw, err := json.Marshal(frozen)
		if err != nil {
			t.Fatal(err)
		}
		hash := p6Digest(raw)
		if mode == "digest" {
			raw = append(raw, ' ')
		}
		if err = db.GetIns().Model(&run).Updates(map[string]any{"freeze": raw, "freeze_hash": hash}).Error; err != nil {
			t.Fatal(err)
		}
		if _, err = f.s.p7AwakeFightBegin(f.ctx, f.id, &pb.CSAwakeCopyFightBegin{MonGuid: entered.Monsters[0].Guid}, now); err == nil {
			t.Fatal("tampered frozen source accepted", mode)
		}
		if err = db.GetIns().Model(&run).Updates(map[string]any{"freeze": original, "freeze_hash": p6Digest(original)}).Error; err != nil {
			t.Fatal(err)
		}
	}
	if err := db.GetIns().Model(&db.P7AwakeRun{}).Where("id=?", run.ID).Update("copy_id", "202").Error; err != nil {
		t.Fatal(err)
	}
	if _, err := f.s.p7AwakeFightBegin(f.ctx, f.id, &pb.CSAwakeCopyFightBegin{MonGuid: entered.Monsters[0].Guid}, now); err == nil {
		t.Fatal("run CopyID differed from its frozen chapter")
	}
	if err := db.GetIns().Model(&db.P7AwakeRun{}).Where("id=?", run.ID).Update("copy_id", run.CopyID).Error; err != nil {
		t.Fatal(err)
	}
	for _, box := range entered.Boxes {
		p7AwakeExploreCall(t, f, &pb.CSAwakeCopyOpenBox{BoxGuid: box.Guid}, now)
	}
	_, end := p7AwakeBusinessStartEnd(t, f, entered.Monsters[0].GetGuid(), now)
	if end.GetIsWinClient() != 1 {
		t.Fatal("finite roster did not win its actual source")
	}
	accepted, err := f.s.p7AwakeFightEnd(f.ctx, f.id, end, now)
	if err != nil {
		t.Fatal(err)
	}
	forged := proto.Clone(end).(*pb.CSAwakeCopyFightEnd)
	forged.OptRecord.AttackerTotalLeftHp = proto.Float64(forged.OptRecord.GetAttackerTotalLeftHp() + 1)
	if _, err = f.s.p7AwakeFightEnd(f.ctx, f.id, forged, now); err == nil {
		t.Fatal("changed End replay accepted")
	}
	retry, err := f.s.p7AwakeFightEnd(f.ctx, f.id, end, now)
	if err != nil || !proto.Equal(retry[0], accepted[0]) {
		t.Fatal("unchanged End receipt", err)
	}
	if p1Count(t, &db.P6BattleRecord{}, "player_id=? AND source=? AND authoritative=?", f.id, constUtil.P7AwakeBattleReplaySource, true) != 1 {
		t.Fatal("replay changed certified battle count")
	}
}

func TestP7Awake85PendingFrozenBattleSurvivesSourceTableChange(t *testing.T) {
	f := awake85BusinessFixture(t)
	now := awake85Monday()
	entered := p7AwakeExploreCall(t, f, &pb.CSEnterAwakeCopy{CopyId: proto.String("102")}, now).(*pb.SCEnterAwakeCopy)
	for _, box := range entered.Boxes {
		p7AwakeExploreCall(t, f, &pb.CSAwakeCopyOpenBox{BoxGuid: box.Guid}, now)
	}
	start, end := p7AwakeBusinessStartEnd(t, f, entered.Monsters[0].GetGuid(), now)
	if end.GetIsWinClient() != 1 {
		t.Fatal("finite roster did not produce a winning original record")
	}
	var fight db.P7AwakeFight
	if err := db.GetIns().First(&fight, "id=?", start.GetFightId()).Error; err != nil || fight.ScopeProof != constUtil.P7Awake16GenericCompatibilityProof {
		t.Fatal("test requires the captured generic type16 scope", err)
	}
	// Both tables are replaced only in this GameService fixture after actual
	// Begin captured P6 input; no production table or source file is edited.
	copyEnemy := f.s.Tables["CopyEnemy"]
	outside := f.s.Tables["AwakeCopyMonsterOutside"]
	t.Cleanup(func() {
		f.s.Tables["CopyEnemy"] = copyEnemy
		f.s.Tables["AwakeCopyMonsterOutside"] = outside
	})
	f.s.Tables["CopyEnemy"] = nil
	f.s.Tables["AwakeCopyMonsterOutside"] = nil
	f.restart(t)
	resumed, err := f.s.p7AwakeExplore(f.ctx, f.id, &pb.CSEnterAwakeCopy{CopyId: proto.String("102")}, now)
	if err != nil || resumed[0].(*pb.SCEnterAwakeCopy).GetRefreshData().GetScore() != 20 {
		t.Fatal("cold exploration did not retain frozen source", err)
	}
	restarted, err := f.s.p7AwakeFightBegin(f.ctx, f.id, &pb.CSAwakeCopyFightBegin{MonGuid: entered.Monsters[0].Guid}, now)
	if err != nil || !proto.Equal(restarted[0], start) {
		t.Fatal("cold pending Start did not retain original encounter", err)
	}
	accepted, err := f.s.p7AwakeFightEnd(f.ctx, f.id, end, now)
	if err != nil || accepted[0].(*pb.SCAwakeCopyFightEnd).GetRefreshData().GetScore() != 100 {
		t.Fatal("actual End did not use captured battle input", err)
	}
	settlement, err := f.s.p7AwakeExplore(f.ctx, f.id, &pb.CSAwakeCopySettle{}, now, 855200)
	if err != nil || settlement[0].(*pb.SCAwakeCopySettle).GetScore() != 100 {
		t.Fatal("actual frozen reward did not settle after source change", err)
	}
	f.restart(t)
	replayed, err := f.s.p7AwakeFightEnd(f.ctx, f.id, end, now)
	if err != nil || !proto.Equal(replayed[0], accepted[0]) {
		t.Fatal("cold accepted End changed after source-table change", err)
	}
}

type awake85ConfigSnapshot struct {
	Training []db.WarriorGrowthRule
	Policies []db.P7AwakePolicy
	Rules    []db.P7AwakeStageRule
	Audit    []db.AdminAudit
}

func awake85ReadConfig(t *testing.T) awake85ConfigSnapshot {
	t.Helper()
	var value awake85ConfigSnapshot
	for _, query := range []struct {
		out   any
		where string
		arg   any
		order string
	}{
		{&value.Training, "id IN ?", []string{"training_1", "training_2", "training_3", "breach"}, "id"},
		{&value.Policies, "id IN ?", []int64{constUtil.P7AwakeExplorePolicyID, constUtil.P7AwakeBattlePolicyID}, "id"},
		{&value.Rules, "copy_id IN ?", []string{"102", "202", "302", "402", "501", "502"}, "copy_id"},
		{&value.Audit, "target=?", constUtil.Awake85Target, "id"},
	} {
		if err := db.GetIns().Where(query.where, query.arg).Order(query.order).Find(query.out).Error; err != nil {
			t.Fatal(err)
		}
	}
	return value
}

// Construct the audited legacy state on the fixture's isolated database.
// Configuration and scoped audit rows are restored after the test.
func awake85MigrationFixture(t *testing.T) (*p1Fixture, *model.AdminPrincipal) {
	t.Helper()
	f := awake85BusinessFixture(t)
	actor := adminWalletTestActor(t, f, constUtil.AdminRoleAdmin)
	if err := InitializeAdminOperations(f.ctx); err != nil {
		t.Fatal(err)
	}
	for _, initialize := range []func(context.Context) error{f.s.InitializeServerGameClock, f.s.InitializeClientUpdates, f.s.InitializeWarriorGrowth} {
		if err := initialize(f.ctx); err != nil {
			t.Fatal(err)
		}
	}
	var maintenance db.ClientUpdatePolicy
	if err := db.GetIns().First(&maintenance, 1).Error; err != nil {
		t.Fatal(err)
	}
	var clock db.ServerGameClock
	if err := db.GetIns().First(&clock, "server_id=1").Error; err != nil {
		t.Fatal(err)
	}
	prior := awake85ReadConfig(t)
	t.Cleanup(func() {
		if err := db.GetIns().Where("target=?", constUtil.Awake85Target).Delete(&db.AdminAudit{}).Error; err != nil {
			t.Error(err)
		}
		for _, audit := range prior.Audit {
			if err := db.GetIns().Create(&audit).Error; err != nil {
				t.Error(err)
			}
		}
		for _, rule := range prior.Training {
			if err := db.GetIns().Save(&rule).Error; err != nil {
				t.Error(err)
			}
		}
		if err := db.GetIns().Save(&maintenance).Error; err != nil {
			t.Error(err)
		}
		if err := db.GetIns().Save(&clock).Error; err != nil {
			t.Error(err)
		}
		if err := db.GetIns().Delete(&db.AdminUser{}, actor.UserID).Error; err != nil {
			t.Error(err)
		}
	})
	if err := db.GetIns().Where("target=?", constUtil.Awake85Target).Delete(&db.AdminAudit{}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.GetIns().Model(&db.ClientUpdatePolicy{}).Where("id=1").Update("maintenance", true).Error; err != nil {
		t.Fatal(err)
	}
	training85RestoreLegacy(t)
	for _, policy := range []db.P7AwakePolicy{
		{ID: constUtil.P7AwakeExplorePolicyID, Enabled: true, MinLevel: 70, Source: constUtil.P7AwakeExploreSource},
		{ID: constUtil.P7AwakeBattlePolicyID, Enabled: true, MinLevel: 70, Source: constUtil.P7AwakeBattleSource},
	} {
		if err := db.GetIns().Save(&policy).Error; err != nil {
			t.Fatal(err)
		}
	}
	legacy, err := p7AwakeLocalCatalogue(true)
	if err != nil {
		t.Fatal(err)
	}
	if err = db.GetIns().Save(&legacy).Error; err != nil {
		t.Fatal(err)
	}
	for _, chapter := range awake85Chapters() {
		rule, err := awake85Catalogue(chapter, false)
		if err != nil {
			t.Fatal(err)
		}
		if err = db.GetIns().Save(&rule).Error; err != nil {
			t.Fatal(err)
		}
	}
	return f, actor
}

func awake85AssertNoConfigChange(t *testing.T, before awake85ConfigSnapshot) {
	t.Helper()
	if after := awake85ReadConfig(t); !reflect.DeepEqual(after, before) {
		t.Fatal("rejected publication partially changed configuration or audit", before, after)
	}
}

func TestFeedback85PublicationRequiresMaintenanceAdminAndValidClock(t *testing.T) {
	f, actor := awake85MigrationFixture(t)
	operator := adminWalletTestActor(t, f, constUtil.AdminRoleOperator)
	viewer := adminWalletTestActor(t, f, constUtil.AdminRoleViewer)
	t.Cleanup(func() {
		for _, user := range []*model.AdminPrincipal{operator, viewer} {
			if err := db.GetIns().Delete(&db.AdminUser{}, user.UserID).Error; err != nil {
				t.Error(err)
			}
		}
	})
	for _, principal := range []*model.AdminPrincipal{nil, operator, viewer} {
		before := awake85ReadConfig(t)
		if err := f.s.ApplyFeedback85Config(f.ctx, principal); err == nil {
			t.Fatal("non-admin published feedback85", principal)
		}
		awake85AssertNoConfigChange(t, before)
	}
	if err := db.GetIns().Model(&db.ClientUpdatePolicy{}).Where("id=1").Update("maintenance", false).Error; err != nil {
		t.Fatal(err)
	}
	before := awake85ReadConfig(t)
	if err := f.s.ApplyFeedback85Config(f.ctx, actor); err == nil {
		t.Fatal("publication outside maintenance")
	}
	awake85AssertNoConfigChange(t, before)
	if err := db.GetIns().Model(&db.ClientUpdatePolicy{}).Where("id=1").Update("maintenance", true).Error; err != nil {
		t.Fatal(err)
	}
	bound, _, err := f.s.bindServerGameClockWorker75(f.ctx)
	if err != nil {
		t.Fatal(err)
	}
	var clock db.ServerGameClock
	if err = db.GetIns().First(&clock, "server_id=1").Error; err != nil {
		t.Fatal(err)
	}
	originalClock := clock
	for _, revision := range []int64{-1, clock.Revision + 1} {
		if err = db.GetIns().Model(&clock).Update("revision", revision).Error; err != nil {
			t.Fatal(err)
		}
		before = awake85ReadConfig(t)
		if err = f.s.ApplyFeedback85Config(bound, actor); err == nil {
			t.Fatal("invalid or stale server clock published", revision)
		}
		awake85AssertNoConfigChange(t, before)
	}
	if err = db.GetIns().Save(&originalClock).Error; err != nil {
		t.Fatal(err)
	}
	f.s.ServerClockReady = false
	before = awake85ReadConfig(t)
	if err = f.s.ApplyFeedback85Config(f.ctx, actor); err == nil {
		t.Fatal("unready clock published")
	}
	awake85AssertNoConfigChange(t, before)
	f.s.ServerClockReady = true
}

func TestFeedback85PublicationAtomicIdempotentAndRejectsDrift(t *testing.T) {
	f, actor := awake85MigrationFixture(t)
	for _, mutation := range []struct {
		name   string
		apply  func() error
		revert func() error
	}{
		{"training", func() error {
			return db.GetIns().Model(&db.WarriorGrowthRule{}).Where("id=?", "training_2").Update("distribution", "[[1,100]]").Error
		}, func() error { training85RestoreLegacy(t); return nil }},
		{"policy", func() error {
			return db.GetIns().Model(&db.P7AwakePolicy{}).Where("id=?", constUtil.P7AwakeBattlePolicyID).Update("source", "custom policy").Error
		}, func() error {
			return db.GetIns().Model(&db.P7AwakePolicy{}).Where("id=?", constUtil.P7AwakeBattlePolicyID).Update("source", constUtil.P7AwakeBattleSource).Error
		}},
		{"partial-policy", func() error {
			return db.GetIns().Model(&db.P7AwakePolicy{}).Where("id=?", constUtil.P7AwakeBattlePolicyID).Update("source", constUtil.Awake85BattleSource).Error
		}, func() error {
			return db.GetIns().Model(&db.P7AwakePolicy{}).Where("id=?", constUtil.P7AwakeBattlePolicyID).Update("source", constUtil.P7AwakeBattleSource).Error
		}},
		{"legacy", func() error {
			return db.GetIns().Model(&db.P7AwakeStageRule{}).Where("copy_id=?", "501").Update("content", []byte("custom")).Error
		}, func() error {
			rule, err := p7AwakeLocalCatalogue(true)
			if err != nil {
				return err
			}
			return db.GetIns().Save(&rule).Error
		}},
		{"new-rule", func() error {
			return db.GetIns().Model(&db.P7AwakeStageRule{}).Where("copy_id=?", "202").Update("fight_power", 101).Error
		}, func() error {
			chapter, _ := awake85Definition("202")
			rule, err := awake85Catalogue(chapter, false)
			if err != nil {
				return err
			}
			return db.GetIns().Save(&rule).Error
		}},
		{"partial-new-rule", func() error {
			return db.GetIns().Model(&db.P7AwakeStageRule{}).Where("copy_id=?", "202").Update("enabled", true).Error
		}, func() error {
			return db.GetIns().Model(&db.P7AwakeStageRule{}).Where("copy_id=?", "202").Update("enabled", false).Error
		}},
	} {
		t.Run(mutation.name, func(t *testing.T) {
			if err := mutation.apply(); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() {
				if err := mutation.revert(); err != nil {
					t.Error(err)
				}
			})
			before := awake85ReadConfig(t)
			if err := f.s.ApplyFeedback85Config(f.ctx, actor); !errors.Is(err, constUtil.ErrAdminConfigConflict) {
				t.Fatal("drifted legacy configuration accepted", err)
			}
			awake85AssertNoConfigChange(t, before)
		})
	}
	if err := f.s.ApplyFeedback85Config(f.ctx, actor); err != nil {
		t.Fatal("complete legacy upgrade", err)
	}
	after := awake85ReadConfig(t)
	if len(after.Training) != 4 || !reflect.DeepEqual(after.Training[1:], training85Defaults()) || len(after.Policies) != 2 || len(after.Rules) != 6 || len(after.Audit) != 1 || after.Audit[0].Action != constUtil.Awake85Migration {
		t.Fatal("atomic migration did not publish all targets once", after)
	}
	for _, policy := range after.Policies {
		want := constUtil.Awake85ExploreSource
		if policy.ID == constUtil.P7AwakeBattlePolicyID {
			want = constUtil.Awake85BattleSource
		}
		if !policy.Enabled || policy.MinLevel != 70 || policy.Source != want {
			t.Fatal("published policy differs", policy)
		}
	}
	for _, rule := range after.Rules {
		if rule.Enabled != (rule.CopyID != "501") {
			t.Fatal("legacy/new difficulty publication", rule.CopyID, rule.Enabled)
		}
	}
	if err := f.s.ApplyFeedback85Config(f.ctx, actor); err != nil || !reflect.DeepEqual(awake85ReadConfig(t), after) {
		t.Fatal("repeat publication changed configuration or appended audit", err)
	}
	f.restart(t)
	if err := f.s.ApplyFeedback85Config(f.ctx, actor); err != nil || !reflect.DeepEqual(awake85ReadConfig(t), after) {
		t.Fatal("cold publication retry", err)
	}
	if err := db.GetIns().Model(&db.P7AwakeStageRule{}).Where("copy_id=?", "202").Update("enabled", false).Error; err != nil {
		t.Fatal(err)
	}
	drift := awake85ReadConfig(t)
	if err := f.s.ApplyFeedback85Config(f.ctx, actor); !errors.Is(err, constUtil.ErrAdminConfigConflict) {
		t.Fatal("later operator closure reopened by replay", err)
	}
	awake85AssertNoConfigChange(t, drift)
}

func TestFeedback85ConcurrentSameAdminPublicationOnceMySQL(t *testing.T) {
	f, actor := awake85MigrationFixture(t)
	if f.mysql.Database == "" {
		t.Skip("concurrent publication requires the isolated P7 MySQL fixture")
	}
	var group sync.WaitGroup
	start := make(chan struct{})
	errs := make([]error, 2)
	for index := range errs {
		group.Add(1)
		go func(i int) {
			defer group.Done()
			<-start
			errs[i] = f.s.ApplyFeedback85Config(f.ctx, actor)
		}(index)
	}
	close(start)
	group.Wait()
	for index, err := range errs {
		if err != nil {
			t.Fatal("concurrent publication failed", index, err)
		}
	}
	after := awake85ReadConfig(t)
	if len(after.Audit) != 1 || after.Audit[0].Action != constUtil.Awake85Migration || len(after.Training) != 4 || !reflect.DeepEqual(after.Training[1:], training85Defaults()) || len(after.Policies) != 2 || len(after.Rules) != 6 {
		t.Fatal("concurrent publication duplicated audit or partially upgraded", after)
	}
	for _, policy := range after.Policies {
		want := constUtil.Awake85ExploreSource
		if policy.ID == constUtil.P7AwakeBattlePolicyID {
			want = constUtil.Awake85BattleSource
		}
		if !policy.Enabled || policy.Source != want || policy.MinLevel != 70 {
			t.Fatal("concurrent publication left a partial policy", policy)
		}
	}
	for _, rule := range after.Rules {
		if rule.Enabled != (rule.CopyID != "501") {
			t.Fatal("concurrent publication left a partial catalogue", rule.CopyID, rule.Enabled)
		}
	}
}

func TestP7Awake85PublicationPreservesAcceptedLegacyRunAndReceipts(t *testing.T) {
	f, actor := awake85MigrationFixture(t)
	friday := awake85Monday().AddDate(0, 0, 4)
	oldEntry, err := f.s.p7AwakeExplore(f.ctx, f.id, &pb.CSEnterAwakeCopy{CopyId: proto.String("501")}, friday, 855100)
	if err != nil {
		t.Fatal("pre-publication accepted 501", err)
	}
	old := oldEntry[0].(*pb.SCEnterAwakeCopy)
	for _, box := range old.Boxes {
		p7AwakeExploreCall(t, f, &pb.CSAwakeCopyOpenBox{BoxGuid: box.Guid}, friday)
	}
	start, end := p7AwakeBusinessStartEnd(t, f, old.Monsters[0].GetGuid(), friday)
	if end.GetIsWinClient() != 1 {
		t.Fatal("legacy captured roster did not produce an actual win")
	}
	accepted, err := f.s.p7AwakeFightEnd(f.ctx, f.id, end, friday)
	if err != nil || accepted[0].(*pb.SCAwakeCopyFightEnd).GetRefreshData().GetScore() != 100 {
		t.Fatal("actual legacy End", err)
	}
	var runBefore db.P7AwakeRun
	var fightBefore db.P7AwakeFight
	var ledgerBefore db.P7AwakeExploration
	if err = db.GetIns().Where("player_id=? AND copy_id=?", f.id, "501").First(&runBefore).Error; err != nil {
		t.Fatal(err)
	}
	if err = db.GetIns().First(&fightBefore, "id=?", start.GetFightId()).Error; err != nil {
		t.Fatal(err)
	}
	if err = db.GetIns().First(&ledgerBefore, "run_id=?", runBefore.ID).Error; err != nil {
		t.Fatal(err)
	}
	if err = f.s.ApplyFeedback85Config(f.ctx, actor); err != nil {
		t.Fatal("publication with accepted legacy instance", err)
	}
	var runAfter db.P7AwakeRun
	var fightAfter db.P7AwakeFight
	var ledgerAfter db.P7AwakeExploration
	if err = db.GetIns().First(&runAfter, "id=?", runBefore.ID).Error; err != nil {
		t.Fatal(err)
	}
	if err = db.GetIns().First(&fightAfter, "id=?", start.GetFightId()).Error; err != nil {
		t.Fatal(err)
	}
	if err = db.GetIns().First(&ledgerAfter, "run_id=?", runBefore.ID).Error; err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(runBefore, runAfter) || !reflect.DeepEqual(fightBefore, fightAfter) || !reflect.DeepEqual(ledgerBefore, ledgerAfter) {
		t.Fatal("publication changed accepted old run, battle, or progress")
	}
	f.restart(t)
	resume, err := f.s.p7AwakeExplore(f.ctx, f.id, &pb.CSEnterAwakeCopy{CopyId: proto.String("501")}, friday, 855100)
	if err != nil || resume[0].(*pb.SCEnterAwakeCopy).GetRefreshData().GetScore() != 100 {
		t.Fatal("accepted 501 did not resume after publication", err)
	}
	oldSettlement, err := f.s.p7AwakeExplore(f.ctx, f.id, &pb.CSAwakeCopySettle{}, friday, 855101)
	if err != nil || oldSettlement[0].(*pb.SCAwakeCopySettle).GetScore() != 100 || !reflect.DeepEqual(awake85ResourceTotals(oldSettlement[0].(*pb.SCAwakeCopySettle).GetScoreRewards()), map[string]int64{"coin": 5000, "hero_exp_002": 1}) {
		t.Fatal("accepted legacy exit or daily reward", err)
	}
	if _, err = f.s.p7AwakeExplore(f.ctx, f.id, &pb.CSEnterAwakeCopy{CopyId: proto.String("501")}, friday, 855102); err == nil {
		t.Fatal("publication still accepted a new 501")
	}
	newEntry, err := f.s.p7AwakeExplore(f.ctx, f.id, &pb.CSEnterAwakeCopy{CopyId: proto.String("502")}, friday, 855103)
	if err != nil {
		t.Fatal("new Friday chapter after legacy exit", err)
	}
	newRun := newEntry[0].(*pb.SCEnterAwakeCopy)
	if newRun.Boxes[0].GetGuid() == old.Boxes[0].GetGuid() || !strings.HasPrefix(newRun.Boxes[0].GetGuid(), constUtil.Awake85RunPrefix) {
		t.Fatal("new run did not isolate old node identity")
	}
	beforeReplay, beforeGame := f.profile(t)
	oldFightReplay, err := f.s.p7AwakeFightEnd(f.ctx, f.id, end, friday)
	if err != nil || !proto.Equal(oldFightReplay[0], accepted[0]) {
		t.Fatal("accepted legacy End changed while new run is active", err)
	}
	oldExitReplay, err := f.s.p7AwakeExplore(f.ctx, f.id, &pb.CSAwakeCopySettle{}, friday, 855101)
	if err != nil || !proto.Equal(oldExitReplay[0], oldSettlement[0]) {
		t.Fatal("old stamped settlement was not replayed", err)
	}
	if _, err = f.s.p7AwakeExplore(f.ctx, f.id, &pb.CSAwakeCopyOpenBox{BoxGuid: old.Boxes[0].Guid}, friday); err == nil {
		t.Fatal("old node opened in a new run")
	}
	afterReplay, afterGame := f.profile(t)
	if !proto.Equal(beforeReplay, afterReplay) || !p6CopySeriesJSONEqual(t, beforeGame, afterGame) {
		t.Fatal("old receipt replay or stale node changed assets")
	}
	p7AwakeExploreCall(t, f, &pb.CSAwakeCopyOpenBox{BoxGuid: newRun.Boxes[0].Guid}, friday)
	newSettlement, err := f.s.p7AwakeExplore(f.ctx, f.id, &pb.CSAwakeCopySettle{}, friday, 855104)
	if err != nil || newSettlement[0].(*pb.SCAwakeCopySettle).GetScore() != 5 || len(newSettlement[0].(*pb.SCAwakeCopySettle).GetScoreRewards()) != 0 {
		t.Fatal("new run was affected by the old receipt", err)
	}
	if _, err = f.s.p7AwakeExplore(f.ctx, f.id, &pb.CSEnterAwakeCopy{CopyId: proto.String("502")}, friday, 855105); err == nil {
		t.Fatal("legacy and new runs did not share the daily budget")
	}
}
