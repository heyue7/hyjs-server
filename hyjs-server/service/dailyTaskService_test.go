package service

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

// TestDailyTaskCalendar 验证真实提交事件、冻结奖励、并发领取、重启和 UTC+8 日界。
func TestDailyTaskCalendar(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "daily.db")
	if err := db.InitGame(ctx, path); err != nil {
		t.Fatal(err)
	}
	sqlDB, _ := db.GetIns().DB()
	defer func() { sqlDB.Close() }()
	s := &GameService{}
	if err := s.LoadTables("../../analysis/tables"); err != nil {
		t.Fatal(err)
	}
	// 直接使用共享装载路径，表接线遗漏必须在测试中暴露。
	if err := s.InitializeDailyTasks(ctx); err != nil {
		t.Fatal(err)
	}
	account, err := LoginAccount(ctx, "isolated-daily", "password")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := CreatePlayer(ctx, account.Id, "日常验收", "hero_043"); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 10, 12, 15, 59, 0, 0, time.UTC)
	info := func(at time.Time) *pb.SCTaskList {
		t.Helper()
		messages, err := s.dailyTasksAt(ctx, account.Id, &pb.CSTaskList{}, at)
		if err != nil {
			t.Fatal(err)
		}
		return messages[0].(*pb.SCTaskList)
	}
	claim := func(request proto.Message, at time.Time) error {
		_, err := s.dailyTasksAt(ctx, account.Id, request, at)
		return err
	}
	initial := info(now)
	if len(initial.TaskList) != 7 || len(initial.BoxList) != 4 || initial.GetScore() != 0 {
		t.Fatal(initial)
	}
	for _, task := range initial.TaskList {
		if task.GetCount() != 0 || task.GetState() != constUtil.DailyTaskDoing {
			t.Fatal("fabricated progress", task)
		}
	}
	if err := claim(&pb.CSGetTaskReward{TaskId: proto.String("local_copy")}, now); err == nil {
		t.Fatal("unfinished granted")
	}
	if err := claim(&pb.CSGetTaskBoxReward{BoxId: proto.String("box_001")}, now); err == nil {
		t.Fatal("zero score granted")
	}
	// 事务中模拟已验证业务增量；失败事务必须连同任务计数回滚。
	event := func(request proto.Message, change func(*model.GameState), rollback bool) error {
		eventCtx := context.WithValue(ctx, constUtil.GameRequestContextKey, request)
		return db.GetIns().WithContext(eventCtx).Transaction(func(tx *gorm.DB) error {
			var row db.Profile
			if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&row, account.Id).Error; err != nil {
				return err
			}
			p, g, err := decodeProfile(&row)
			if err != nil {
				return err
			}
			change(g)
			if err := s.recordDailyTaskSaveAt(tx, &row, p, g, now); err != nil {
				return err
			}
			if rollback {
				return errors.New("isolated business rollback")
			}
			// 无请求上下文的保存避免测试时钟被真实时钟替换。
			return saveProfile(tx.WithContext(ctx), &row, p, g)
		})
	}
	if err := event(&pb.CSWarriorRecruit{}, func(g *model.GameState) { g.Warriors = append(g.Warriors, &pb.Warrior{Guid: proto.String("rollback")}) }, true); err == nil {
		t.Fatal("expected rollback")
	}
	if got := info(now); got.TaskList[1].GetCount() != 0 {
		t.Fatal("rolled back count", got)
	}
	if err := event(&pb.CSWarriorRecruit{}, func(g *model.GameState) {
		for i := 0; i < 3; i++ {
			g.Warriors = append(g.Warriors, &pb.Warrior{Guid: proto.String("recruited")})
		}
	}, false); err != nil {
		t.Fatal(err)
	}
	// 相同存档的网络重试不计数；查询本身也不补出任务。
	if err := event(&pb.CSWarriorRecruit{}, func(g *model.GameState) {}, false); err != nil {
		t.Fatal(err)
	}
	if err := db.GetIns().Model(&db.DailyTaskRule{}).Where("id = ?", "local_recruit").Updates(map[string]any{"reward": "coin,999999", "score": 999}).Error; err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	wins := make(chan bool, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			wins <- claim(&pb.CSGetTaskReward{TaskId: proto.String("local_recruit")}, now) == nil
		}()
	}
	wg.Wait()
	close(wins)
	success := 0
	for win := range wins {
		if win {
			success++
		}
	}
	if success != 1 {
		t.Fatal("duplicate rewards", success)
	}
	if got := info(now); got.GetScore() != 20 {
		t.Fatal("frozen score", got)
	}
	// 冻结日档保持原奖励，新 SQL 次日才生效。
	var profile db.Profile
	if err := db.GetIns().First(&profile, account.Id).Error; err != nil {
		t.Fatal(err)
	}
	p, _, err := decodeProfile(&profile)
	if err != nil || p.GetCoin() != 1000 {
		t.Fatal("reward snapshot", p, err)
	}
	// 覆盖其余所有受支持的事件和经验未升级的精炼、饰品强化。
	seed := func(g *model.GameState) {
		g.Equipment = []*pb.Equipment{{Guid: proto.String("equipment"), Level: proto.Int32(1)}}
		g.Treasures = []*pb.Treasure{{Guid: proto.String("treasure"), StrengthLevel: proto.Int32(1)}}
		g.Run = &model.CopyRun{}
	}
	if err := event(&pb.CSMove{}, seed, false); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 3; i++ {
		if err := event(&pb.CSMove{}, func(g *model.GameState) { g.Run = &model.CopyRun{} }, false); err != nil {
			t.Fatal(err)
		}
		if err := event(&pb.CSCopyFightEnd{}, func(g *model.GameState) { g.Run.Complete = true }, false); err != nil {
			t.Fatal(err)
		}
		if err := event(&pb.CSCopyFightEnd{}, func(g *model.GameState) {}, false); err != nil {
			t.Fatal(err)
		}
	}
	if err := event(&pb.CSEquipStrengthen{Guid: proto.String("equipment")}, func(g *model.GameState) { g.Equipment[0].Level = proto.Int32(4) }, false); err != nil {
		t.Fatal(err)
	}
	if err := event(&pb.CSEquipRefine{Guid: proto.String("equipment")}, func(g *model.GameState) { g.Equipment[0].RefineExp = proto.Int32(1) }, false); err != nil {
		t.Fatal(err)
	}
	if err := event(&pb.CSTreasureStrengthen{TargetId: proto.String("treasure")}, func(g *model.GameState) { g.Treasures[0].CurrentExp = proto.Int32(1) }, false); err != nil {
		t.Fatal(err)
	}
	if err := event(&pb.CSCombine{}, func(g *model.GameState) {
		g.Treasures = append(g.Treasures, &pb.Treasure{Guid: proto.String("composed")})
	}, false); err != nil {
		t.Fatal(err)
	}
	if err := event(&pb.CSWarriorTraining{NumType: proto.Int32(1)}, func(g *model.GameState) {}, false); err != nil {
		t.Fatal(err)
	}
	for _, task := range info(now).TaskList {
		if task.GetState() == constUtil.DailyTaskClaimable {
			if err := claim(&pb.CSGetTaskReward{TaskId: task.TaskId}, now); err != nil {
				t.Fatal(err)
			}
		}
	}
	complete := info(now)
	if complete.GetScore() != 140 {
		t.Fatal("real event total", complete)
	}
	for _, box := range complete.BoxList {
		if err := claim(&pb.CSGetTaskBoxReward{BoxId: box.BoxId}, now); err != nil {
			t.Fatal(err)
		}
		if err := claim(&pb.CSGetTaskBoxReward{BoxId: box.BoxId}, now); err == nil {
			t.Fatal("box replay granted")
		}
	}
	if err := claim(&pb.CSGetTaskBoxReward{BoxId: proto.String("box_005")}, now); err == nil {
		t.Fatal("other level box granted")
	}
	sqlDB.Close()
	if err := db.InitGame(ctx, path); err != nil {
		t.Fatal(err)
	}
	sqlDB, _ = db.GetIns().DB()
	if err := s.InitializeDailyTasks(ctx); err != nil {
		t.Fatal(err)
	}
	if got := info(now); got.GetScore() != 140 || got.BoxList[0].GetState() != 2 {
		t.Fatal("restart receipt lost", got)
	}
	next := now.Add(time.Minute)
	if got := info(next); got.GetScore() != 0 || got.TaskList[1].GetReward() != "coin,999999" || got.TaskList[1].GetScore() != 999 {
		t.Fatal("UTC+8 midnight", got)
	}
	// 跨日重放已完成主线不能补发进度；扫荡只有新回执增加一次计数。
	now = next
	if err := event(&pb.CSCopyFightEnd{}, func(g *model.GameState) {}, false); err != nil {
		t.Fatal(err)
	}
	if got := info(next); got.TaskList[0].GetCount() != 0 {
		t.Fatal("old completion counted on new day", got)
	}
	if err := event(&pb.CSCopySweep{}, func(g *model.GameState) {
		g.CopySweepReceipt = &model.CopySweepReceipt{Timestamp: 100, Response: &pb.SCCopySweep{}}
	}, false); err != nil {
		t.Fatal(err)
	}
	if err := event(&pb.CSCopySweep{}, func(g *model.GameState) {}, false); err != nil {
		t.Fatal(err)
	}
	if err := event(&pb.CSCopySweep{}, func(g *model.GameState) { g.CopySweepReceipt.Timestamp = 101 }, false); err != nil {
		t.Fatal(err)
	}
	if got := info(next); got.TaskList[0].GetCount() != 2 || got.TaskList[0].GetState() != constUtil.DailyTaskDoing {
		t.Fatal("sweep receipt counted more than once", got)
	}
	// 日档损坏必须拒绝并保持原资源。
	var row db.DailyTaskState
	if err := db.GetIns().Where("player_id = ? AND day = ?", account.Id, "2026-10-13").First(&row).Error; err != nil {
		t.Fatal(err)
	}
	var snapshot dailyTaskSnapshot
	if err := json.Unmarshal([]byte(row.Snapshot), &snapshot); err != nil {
		t.Fatal(err)
	}
	snapshot.Tasks[0].Count = proto.Int32(3)
	snapshot.Tasks[0].State = proto.Int32(0)
	snapshot.Tasks[0].Reward = proto.String("unknown,1")
	if err := saveDailyTaskState(db.GetIns(), &row, &snapshot); err != nil {
		t.Fatal(err)
	}
	if err := claim(&pb.CSGetTaskReward{TaskId: proto.String("local_copy")}, next); err == nil {
		t.Fatal("invalid reward granted")
	}
}
