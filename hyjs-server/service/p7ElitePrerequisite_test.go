package service

import (
	"testing"
	"time"

	"hyjs-server/constUtil"
	"hyjs-server/db"
	pb "hyjs-server/model/protocol"
)

// 只打前三层；其余断点通过破坏已有见证检查，避免为前置桥跑全目录。
func TestP7ElitePrerequisiteUsesAcceptedContinuousTrialFloors(t *testing.T) {
	f := p7SectionFixture(t)
	p7SectionPublish(t)
	var elite db.P7RushStageRule
	if err := db.GetIns().First(&elite, "id=?", "elite:01").Error; err != nil {
		t.Fatal(err)
	}
	assertEligible := func(want bool) {
		t.Helper()
		got, err := f.s.p7StagePrerequisite(db.GetIns(), f.id, elite)
		if err != nil || got != want {
			t.Fatal("精英原前置03与真实连续楼层不匹配", got, want, err)
		}
	}
	assertEligible(false)
	p7SectionPlayFloor(t, f, 1, 1, 31201)
	p7SectionPlayFloor(t, f, 2, 1, 31202)
	assertEligible(false)
	p7SectionPlayFloor(t, f, 3, 1, 31203)
	assertEligible(true)
	if count := p1Count(t, &db.P7RushProgress{}, "player_id=? AND task_id=?", f.id, "03"); count != 0 {
		t.Fatal("兼容桥写入了虚构原任务通关", count)
	}
	var first db.P7RushFight
	if err := db.GetIns().Where("player_id=? AND task_id=?", f.id, "local-section-1").First(&first).Error; err != nil {
		t.Fatal(err)
	}
	originalRun := first.RunID
	if err := db.GetIns().Model(&first).Update("diagnostic", true).Error; err != nil {
		t.Fatal(err)
	}
	assertEligible(false)
	if err := db.GetIns().Model(&first).Updates(map[string]any{"diagnostic": false, "run_id": first.RunID + 1}).Error; err != nil {
		t.Fatal(err)
	}
	assertEligible(false)
	if err := db.GetIns().Model(&first).Update("run_id", originalRun).Error; err != nil {
		t.Fatal(err)
	}
	assertEligible(true)
	if err := db.GetIns().Model(&db.P6BattleRecord{}).Where("id=?", first.ReplayID).Update("authoritative", false).Error; err != nil {
		t.Fatal(err)
	}
	if got, err := f.s.p7StagePrerequisite(db.GetIns(), f.id, elite); got || err == nil {
		t.Fatal("损坏的权威通关见证获得精英资格", got, err)
	}
	if err := db.GetIns().Model(&db.P6BattleRecord{}).Where("id=?", first.ReplayID).Update("authoritative", true).Error; err != nil {
		t.Fatal(err)
	}
	f.restart(t)
	assertEligible(true)
	// 重置当前轮不撤销已经自然达成的历史前置。
	if _, err := f.s.P7RushMutation(f.ctx, f.id, &pb.CSRushResetChallengeCount{}, 31204); err != nil {
		t.Fatal(err)
	}
	assertEligible(true)
	if _, err := f.s.P7RushQuery(f.ctx, f.id, &pb.CSGetRushEliteBossInfo{}, time.Now()); err != nil {
		t.Fatal("精英目录投影无法使用同一前置桥", err)
	}
}

func TestP7ElitePrerequisitePreservesExactTaskAndRejectsHighFloorAlone(t *testing.T) {
	f := p7SectionFixture(t)
	var elite db.P7RushStageRule
	if err := db.GetIns().First(&elite, "id=?", "elite:01").Error; err != nil {
		t.Fatal(err)
	}
	if _, err := f.s.P7RushQuery(f.ctx, f.id, &pb.CSGetRushInfo{}, time.Now()); err != nil {
		t.Fatal(err)
	}
	if err := db.GetIns().Model(&db.P7RushState{}).Where("player_id=?", f.id).Update("high_floor", 100).Error; err != nil {
		t.Fatal(err)
	}
	if got, err := f.s.p7StagePrerequisite(db.GetIns(), f.id, elite); got || err != nil {
		t.Fatal("单独历史最高层替代了实际战斗链", got, err)
	}
	progress := db.P7RushProgress{PlayerID: f.id, Kind: constUtil.P7RushTrial, TaskID: "03", Star: 1, FirstFight: "legacy-exact-task-contract"}
	if err := db.GetIns().Create(&progress).Error; err != nil {
		t.Fatal(err)
	}
	if got, err := f.s.p7StagePrerequisite(db.GetIns(), f.id, elite); !got || err != nil {
		t.Fatal("原精确task_id合同被桥接覆盖", got, err)
	}
	if p1Count(t, &db.P7RushFight{}, "player_id=?", f.id) != 0 {
		t.Fatal("前置查询制造了战斗")
	}
}
