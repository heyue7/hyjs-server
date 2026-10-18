package service

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"google.golang.org/protobuf/proto"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

// TestOriginalCopyFullContentLifecycle verifies all original progression and paid
// reward persistence. Client-reported wins here are isolated contract fixtures;
// original native automatic battle results are an independent P6 gate.
func TestOriginalCopyFullContentLifecycle(t *testing.T) {
	if os.Getenv("HYJS_COPY_FULL_SCOPE") != "1" {
		t.Skip("explicit original27 chapters/161 missions/173 quests matrix")
	}
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "copy-all.db")
	if err := db.InitGame(ctx, path); err != nil {
		t.Fatal(err)
	}
	defer func() { pool, _ := db.GetIns().DB(); pool.Close() }()
	s := &GameService{}
	for _, load := range []func(string) error{s.LoadTables, s.LoadP5Tables} {
		if err := load("../../analysis/tables"); err != nil {
			t.Fatal(err)
		}
	}
	for _, init := range []func(context.Context) error{s.InitializeP1, s.InitializeP5, s.InitializeWarriorGrowth, s.InitializeOpenServerActivity, s.InitSkillSpecialty, s.InitializeCopyDrops, s.InitializeDailyCopy, s.InitCooking} {
		if err := init(ctx); err != nil {
			t.Fatal(err)
		}
	}
	a, err := LoginAccount(ctx, "original-copy-all", "isolated-test-only")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = CreatePlayer(ctx, a.Id, "原全章测试", "hero_043"); err != nil {
		t.Fatal(err)
	}
	if err = updateProfile(ctx, a.Id, func(p *pb.Player, g *model.GameState) error {
		p.Level = proto.Int32(100)
		p.Coin = proto.Int64(1000000000)
		p.Diamond = proto.Int64(100000)
		p.Stamina = proto.Int32(100000)
		p.WarriorBagSize = proto.Int32(10000)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	read := func() (*pb.Player, *model.GameState) {
		t.Helper()
		var row db.Profile
		if err := db.GetIns().First(&row, a.Id).Error; err != nil {
			t.Fatal(err)
		}
		p, g, err := decodeProfile(&row)
		if err != nil {
			t.Fatal(err)
		}
		return p, g
	}
	assertUnchanged := func(beforeP *pb.Player, beforeG *model.GameState) {
		t.Helper()
		p, g := read()
		x, _ := json.Marshal(beforeG)
		y, _ := json.Marshal(g)
		if !proto.Equal(beforeP, p) || string(x) != string(y) {
			t.Fatal("rejected original path changed complete save")
		}
	}
	var lastEnd *pb.CSCopyFightEnd
	var lastReply proto.Message
	fights, quests, boxes := 0, 0, 0
	for i, mission := range s.Tables["CopyMission"] {
		chapter, id := mission["ChapterId"], mission["MissionId"]
		p, g := read()
		if i+1 < len(s.Tables["CopyMission"]) {
			next := s.Tables["CopyMission"][i+1]
			if _, err := s.copyEnter(ctx, a.Id, &pb.CSCopyEnter{ChapterId: proto.String(next["ChapterId"]), MissionId: proto.String(next["MissionId"])}); err == nil {
				t.Fatal("skipped predecessor", id)
			}
			assertUnchanged(p, g)
		}
		if _, err := s.copyEnter(ctx, a.Id, &pb.CSCopyEnter{ChapterId: proto.String(chapter), MissionId: proto.String(id)}); err != nil {
			t.Fatal("original enter", chapter, id, err)
		}
		for _, task := range s.tasks(id) {
			req := &pb.CSCopyFightStart{ChapterId: proto.String(chapter), MissionId: proto.String(id), TaskId: proto.String(task["TaskId"])}
			msgs, err := s.copyFightStart(ctx, a.Id, req)
			if err != nil {
				t.Fatal("original fight", id, task["TaskId"], err)
			}
			start := msgs[0].(*pb.SCCopyFightStart)
			replay, err := s.copyFightStart(ctx, a.Id, req)
			if err != nil || !proto.Equal(start, replay[0]) {
				t.Fatal("original start replay", err)
			}
			lastEnd = &pb.CSCopyFightEnd{FightId: start.FightId, Result: proto.Int32(1), BattlePlayer: proto.Clone(start.GetBattlePlayer()).(*pb.BattlePlayer)}
			out, err := s.copyFightEnd(ctx, a.Id, lastEnd)
			if err != nil {
				t.Fatal("original isolated client-result settlement", id, task["TaskId"], err)
			}
			lastReply = out[0]
			p, g = read()
			again, err := s.copyFightEnd(ctx, a.Id, lastEnd)
			if err != nil || !proto.Equal(lastReply, again[0]) {
				t.Fatal("settlement replay", err)
			}
			assertUnchanged(p, g)
			fights++
		}
		// 原对白和战斗任务只在对应关已通过后按order领取，下一关OpenId随真实领取开放。
		for _, quest := range s.Tables["StoryQuest"] {
			if quest["missionId"] != id {
				continue
			}
			if _, err := s.storyQuestReward(ctx, a.Id, quest["questId"]); err != nil {
				t.Fatal("original quest", quest["questId"], err)
			}
			p, g = read()
			if _, err := s.storyQuestReward(ctx, a.Id, quest["questId"]); err == nil {
				t.Fatal("duplicate quest")
			}
			assertUnchanged(p, g)
			quests++
		}
		lastInChapter := i+1 == len(s.Tables["CopyMission"]) || s.Tables["CopyMission"][i+1]["ChapterId"] != chapter
		if lastInChapter {
			for box := int32(0); box < 3; box++ {
				r := &pb.CSCopyGetStarAward{ChapterId: proto.String(chapter), StarIndex: proto.Int32(box)}
				if _, err := s.chapterStarReward(ctx, a.Id, r); err != nil {
					t.Fatal("all original chapter starbox", chapter, box, err)
				}
				p, g = read()
				if _, err := s.chapterStarReward(ctx, a.Id, r); err == nil {
					t.Fatal("double box")
				}
				assertUnchanged(p, g)
				boxes++
			}
		}
	}
	if len(s.Tables["CopyMission"]) != 161 || len(s.Tables["CopyChapter"]) != 27 || fights != 638 || quests != 173 || boxes != 81 {
		t.Fatal("full original path totals", fights, quests, boxes)
	}
	p, g := read()
	if len(g.Quests) != 173 || len(g.Copy.GetPassedCopyInfo()) != 161 || len(g.Copy.GetPassedChapterInfo()) != 27 {
		t.Fatal("full original persisted progression")
	}
	for _, info := range g.Copy.GetPassedCopyInfo() {
		if info.GetStar() != 5 || info.GetChallengeTimes() != 1 {
			t.Fatal("actual completion counts", info)
		}
	}
	if simpleOwned(p, g, "item_439") < 20 || simpleOwned(p, g, "item_440") < 15 {
		t.Fatal("original30boss natural specialty book source did not arrive")
	}
	// 从原实际掉落消费至另一原零专精红伙伴，不写入书本材料。
	var guid string
	if err := updateProfile(ctx, a.Id, func(p *pb.Player, g *model.GameState) error {
		gain, err := s.GrantFixedReward(p, g, "hero_001,1")
		if err != nil {
			return err
		}
		guid = gain.GetWars()[0].GetGuid()
		w, err := p5Warrior(g, guid)
		if err != nil {
			return err
		}
		w.Level = proto.Int32(75)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	p, g = read()
	beforeBooks := []int64{simpleOwned(p, g, "item_439"), simpleOwned(p, g, "item_440")}
	w, _ := p5Warrior(g, guid)
	baseline, err := s.battleWarrior(p, w, g, &pb.FormationItem{BattlePos: proto.Int32(2)})
	if err != nil {
		t.Fatal(err)
	}
	req := &pb.CSUpRoleSkillRefineLevel{RoleGuid: proto.String(guid), Type: proto.Int32(2)}
	reqCtx := context.WithValue(context.WithValue(ctx, constUtil.GameServiceContextKey, s), constUtil.GameRequestContextKey, req)
	out, err := s.skillSpecialtyUpgrade(reqCtx, a.Id, req, 10000)
	if err != nil {
		t.Fatal("natural drop books actual specialty", err)
	}
	p, g = read()
	w, _ = p5Warrior(g, guid)
	after, err := s.battleWarrior(p, w, g, &pb.FormationItem{BattlePos: proto.Int32(2)})
	if err != nil || after.GetProp().GetMdef()-baseline.GetProp().GetMdef() != 18 {
		t.Fatal("original material actual property18", err)
	}
	if beforeBooks[0]-simpleOwned(p, g, "item_439") != 20 || beforeBooks[1]-simpleOwned(p, g, "item_440") != 15 {
		t.Fatal("original natural books exact consumption")
	}
	replay, err := s.skillSpecialtyUpgrade(reqCtx, a.Id, req, 10000)
	if err != nil || !proto.Equal(out[0], replay[0]) {
		t.Fatal("book consumption receipt", err)
	}
	beforeP, beforeG := read()
	pool, _ := db.GetIns().DB()
	pool.Close()
	if err := db.InitGame(ctx, path); err != nil {
		t.Fatal(err)
	}
	cold, err := s.copyFightEnd(ctx, a.Id, lastEnd)
	if err != nil || !proto.Equal(lastReply, cold[0]) {
		t.Fatal("cold final fight", err)
	}
	assertUnchanged(beforeP, beforeG)
	t.Logf("real progression27chapters161missions638encounters173quests81starboxes; original30boss books439/440 consumed20/15 and mdef+18; isolated client-result fixture, native combat certification remains separate")
}

// TestCopyDropsFreezeAndFailure verifies configuration freeze, failed ownership,
// capacity rollback and single concurrent settlement without changing APK odds.
func TestCopyDropsFreezeAndFailure(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "copy-drops.db")
	if err := db.InitGame(ctx, path); err != nil {
		t.Fatal(err)
	}
	defer func() { pool, _ := db.GetIns().DB(); pool.Close() }()
	s := &GameService{}
	if err := s.LoadTables("../../analysis/tables"); err != nil {
		t.Fatal(err)
	}
	if err := s.InitializeCopyDrops(ctx); err != nil {
		t.Fatal(err)
	}
	if err := s.InitializeDailyCopy(ctx); err != nil {
		t.Fatal(err)
	}
	a, err := LoginAccount(ctx, "copy-freeze", "isolated-test-only")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = CreatePlayer(ctx, a.Id, "真实冻结", "hero_043"); err != nil {
		t.Fatal(err)
	}
	if _, err = s.copyEnter(ctx, a.Id, &pb.CSCopyEnter{ChapterId: proto.String("1"), MissionId: proto.String("1")}); err != nil {
		t.Fatal(err)
	}
	req := &pb.CSCopyFightStart{ChapterId: proto.String("1"), MissionId: proto.String("1"), TaskId: proto.String("1")}
	msgs, err := s.copyFightStart(ctx, a.Id, req)
	if err != nil {
		t.Fatal(err)
	}
	start := msgs[0].(*pb.SCCopyFightStart)
	if err := db.GetIns().Model(&db.CopyDropRule{}).Where("id=?", constUtil.CopyDropRuleID).Updates(map[string]any{"enabled": false, "mode": "invalid"}).Error; err != nil {
		t.Fatal(err)
	}
	originalOdds := s.table("CopyTask", "TaskId", "1")["DropOdds"]
	s.table("CopyTask", "TaskId", "1")["DropOdds"] = "item_does_not_exist:1,100"
	finish := &pb.CSCopyFightEnd{FightId: start.FightId, Result: proto.Int32(1), BattlePlayer: start.BattlePlayer}
	var wg sync.WaitGroup
	results := make(chan proto.Message, 8)
	errs := make(chan error, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			r, e := s.copyFightEnd(ctx, a.Id, finish)
			errs <- e
			if e == nil {
				results <- r[0]
			}
		}()
	}
	wg.Wait()
	close(errs)
	close(results)
	for e := range errs {
		if e != nil {
			t.Fatal(e)
		}
	}
	var first proto.Message
	for r := range results {
		if first == nil {
			first = r
		}
		if !proto.Equal(first, r) {
			t.Fatal("concurrent different random award")
		}
	}
	result := first.(*pb.SCCopyFightEnd)
	if len(result.GetGain().GetWars()) == 0 {
		t.Fatal("real original guaranteed positive ordinary hero material omitted")
	}
	var saved db.CopyDropSnapshot
	if err := db.GetIns().First(&saved, "player_id=? AND fight_id=?", a.Id, start.GetFightId()).Error; err != nil {
		t.Fatal(err)
	}
	if saved.InputHash != p6Digest(saved.Input) {
		t.Fatal("frozen drop integrity")
	}
	pool, _ := db.GetIns().DB()
	pool.Close()
	if err := db.InitGame(ctx, path); err != nil {
		t.Fatal(err)
	}
	cold, err := s.copyFightEnd(ctx, a.Id, finish)
	if err != nil || !proto.Equal(first, cold[0]) {
		t.Fatal("cold exact random award", err)
	}
	s.table("CopyTask", "TaskId", "1")["DropOdds"] = originalOdds
	if err := db.GetIns().Model(&db.CopyDropRule{}).Where("id=?", constUtil.CopyDropRuleID).Updates(map[string]any{"enabled": true, "mode": constUtil.CopyDropRelativeWeights}).Error; err != nil {
		t.Fatal(err)
	}
	// 全原普通掉落解析逐条使用完整原ID与数量，未知策略/坏表不会偶发成功。
	for _, task := range s.Tables["CopyTask"] {
		if _, err := s.copyDropInput(db.GetIns(), task["DropOdds"]); err != nil {
			t.Fatal("original drop row", task["TaskId"], err)
		}
	}
	for _, bad := range []string{"item_439:0,0;5,90", "item_439:-1,10;5,90", "item_x:0,100;5,1", "item_439:0,10;5"} {
		if _, err := s.copyDropInput(db.GetIns(), bad); err == nil {
			t.Fatal("invalid drop accepted", bad)
		}
	}
	// 原第2章首关必须先通过原第1章末关；OpenId仍独立检查。
	if s.copyMissionOpen(&model.GameState{}, s.table("CopyMission", "MissionId", "6")) {
		t.Fatal("cross chapter skipped")
	}
	g := &model.GameState{Copy: &pb.SCCopyPassedInfo{PassedCopyInfo: []*pb.CopySimpleInfo{{ChapterId: proto.String("1"), MissionId: proto.String("5"), Star: proto.Int32(5)}}}}
	if !s.copyMissionOpen(g, s.table("CopyMission", "MissionId", "6")) {
		t.Fatal("cross chapter predecessor not opened")
	}
	if s.copyMissionOpen(g, s.table("CopyMission", "MissionId", "2")) {
		t.Fatal("OpenId skipped")
	}
}
