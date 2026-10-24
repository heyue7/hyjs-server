package service

import (
	"bytes"
	"errors"
	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
	"testing"
	"time"
)

func TestAdminTrial63PublicationConflictPreviewAndFrozenState(t *testing.T) {
	f := p7Setup(t)
	if err := AdminInitialize(f.ctx, ""); err != nil {
		t.Fatal(err)
	}
	user := db.AdminUser{Username: "trial63-" + p6Digest([]byte(t.Name()))[:24], PasswordHash: "isolated-fixture", Role: constUtil.AdminRoleOperator, Enabled: true}
	if err := db.GetIns().Create(&user).Error; err != nil {
		t.Fatal(err)
	}
	actor := &model.AdminPrincipal{UserID: user.ID, Username: user.Username, Role: user.Role}
	initial, err := f.s.AdminTrialConfig(f.ctx, actor)
	if err != nil {
		t.Fatal(err)
	}
	if initial.Config.Floors != 102 || initial.Version != 0 {
		t.Fatal("legacy policy defaults", initial)
	}
	// Keep historical accepted bytes and progress; catalogue save cannot rewrite them.
	fight := db.P7RushFight{ID: "trial63-preserved", PlayerID: f.id, RunID: 1, Rule: []byte("frozen-rule"), Start: []byte("frozen-start")}
	if err = db.GetIns().Create(&fight).Error; err != nil {
		t.Fatal(err)
	}
	state := db.P7RushState{PlayerID: f.id, RunID: 1, Floor: 2, HighFloor: 1, TotalStars: 3, RecordStars: 3, Day: "2026-10-24"}
	if err = db.GetIns().Create(&state).Error; err != nil {
		t.Fatal(err)
	}
	req := model.AdminTrialSaveRequest{ExpectedVersion: initial.Version, Enabled: true, Config: initial.Config, Reason: "隔离试炼生成检查"}
	saved, err := f.s.AdminSaveTrialConfig(f.ctx, actor, req)
	if err != nil {
		t.Fatal(err)
	}
	if saved.Version != 1 || saved.PublishedFloorCount != 102 || !saved.Enabled || saved.PublishedAt <= 0 {
		t.Fatal(saved)
	}
	preview, err := f.s.AdminTrialPreview(f.ctx, actor, model.AdminTrialPreviewRequest{Config: req.Config, Floor: 1})
	if err != nil {
		t.Fatal(err)
	}
	if len(preview.Items) != 3 || len(preview.Items[0].Enemies) != 2 || preview.Items[0].RecommendedPower <= 0 {
		t.Fatal(preview)
	}
	var row db.P7RushStageRule
	if err = db.GetIns().First(&row, "id=?", "re-rush:1:1").Error; err != nil {
		t.Fatal(err)
	}
	if row.RecommendedPower != preview.Items[0].RecommendedPower {
		t.Fatal("preview/published drift")
	}
	policyReads, stageRows := 0, int64(0)
	callback := "trial63-query-evidence"
	if err = db.GetIns().Callback().Query().After("gorm:query").Register(callback, func(tx *gorm.DB) {
		switch tx.Statement.Table {
		case "p7_re_rush_policies":
			policyReads++
		case "p7_rush_stage_rules":
			stageRows += tx.RowsAffected
		}
	}); err != nil {
		t.Fatal(err)
	}
	query, queryErr := f.s.P7RushQuery(f.ctx, f.id, &pb.CSRushCurrentInfo{}, time.Now())
	db.GetIns().Callback().Query().Remove(callback)
	if queryErr != nil {
		t.Fatal(queryErr)
	}
	if len(query[0].(*pb.SCRushCurrentInfo).TaskInfoList) != 3 || policyReads != 1 || stageRows != 3 {
		t.Fatal("query repeated policy or loaded other floors", policyReads, stageRows)
	}

	var coldFight db.P7RushFight
	var coldState db.P7RushState
	if err = db.GetIns().First(&coldFight, "id=?", fight.ID).Error; err != nil {
		t.Fatal(err)
	}
	if err = db.GetIns().First(&coldState, "player_id=?", f.id).Error; err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(coldFight.Rule, fight.Rule) || !bytes.Equal(coldFight.Start, fight.Start) || coldState.Floor != 2 || coldState.TotalStars != 3 {
		t.Fatal("save changed frozen player state")
	}
	if _, err = f.s.AdminSaveTrialConfig(f.ctx, actor, req); !errors.Is(err, constUtil.ErrAdminConfigConflict) {
		t.Fatal("stale save accepted", err)
	}
	req.ExpectedVersion = saved.Version
	req.Enabled = false
	req.Config.InitialCount = 3
	if _, err = f.s.AdminSaveTrialConfig(f.ctx, actor, req); err != nil {
		t.Fatal(err)
	}
	var count int64
	if err = db.GetIns().Model(&db.P7RushStageRule{}).Where("kind=? AND enabled=?", constUtil.P7RushTrial, true).Count(&count).Error; err != nil || count != 0 {
		t.Fatal("disable left active trial", count, err)
	}
	// Explicit publisher reuses saved parameters instead of resetting admin configuration.
	if err = f.s.PublishP7ReRush(f.ctx); err != nil {
		t.Fatal(err)
	}
	after, err := f.s.AdminTrialConfig(f.ctx, actor)
	if err != nil || after.Version != 3 || after.Config.InitialCount != 3 {
		t.Fatal("explicit publication metadata", after, err)
	}
}

func TestAdminTrial63RejectsMissingAssetsAndInvalidCurve(t *testing.T) {
	s, _, _ := worldSeedFixture(t)
	cfg := defaultAdminTrialGeneration()
	if err := s.ValidateAdminTrialGeneration(cfg); err != nil {
		t.Fatal(err)
	}
	cfg.Anchors[1].Floor = 1
	if err := s.ValidateAdminTrialGeneration(cfg); err == nil {
		t.Fatal("unordered anchors accepted")
	}
	cfg = defaultAdminTrialGeneration()
	cfg.Roster[0] = "missing-original"
	if err := s.ValidateAdminTrialGeneration(cfg); err == nil {
		t.Fatal("missing enemy accepted")
	}
	cfg = defaultAdminTrialGeneration()
	cfg.Positions[1] = cfg.Positions[0]
	if err := s.ValidateAdminTrialGeneration(cfg); err == nil {
		t.Fatal("duplicate pose accepted")
	}
}

func TestP7Rush63HistoricalPowerProjectionPreservesRule(t *testing.T) {
	rows, err := p7SectionCatalogue(true)
	if err != nil {
		t.Fatal(err)
	}
	before := append([]byte(nil), rows[0].Enemies...)
	power, err := p7RushProjectedPower(rows[0])
	if err != nil || power <= 0 {
		t.Fatal("historical zero projection", power, err)
	}
	projected, err := p7RushContinuationTaskProjection(rows[0])
	if err != nil || projected.GetFightScore() != power {
		t.Fatal("pending frozen projection", projected, err)
	}
	if rows[0].RecommendedPower != 0 || !bytes.Equal(before, rows[0].Enemies) {
		t.Fatal("projection mutated frozen source")
	}
	actor := &pb.BattleHero{Prop: &pb.BaseProp{Hp: proto.Float64(100), Att: proto.Float64(10), Pdef: proto.Float64(2), Mdef: proto.Float64(3)}}
	raw, _ := proto.Marshal(&pb.BattleMonster{Formation: []*pb.BattleHero{actor, proto.Clone(actor).(*pb.BattleHero)}})
	power, err = p7RushProjectedPower(db.P7RushStageRule{Enemies: raw})
	if err != nil || power != 27 {
		t.Fatal("multi actor score", power, err)
	}
}

func TestAdminTrial63EnemyNamesReuseCatalogueAndOriginalText(t *testing.T) {
	s := &GameService{Tables: map[string][]map[string]string{"Hero": {{"id": "monster_044", "name": "name_monster_044"}, {"id": "monster_missing", "name": "name_missing"}}, "Text": {{"id": "name_monster_044", "text": "原怪物名称夹具"}}}, AdminCatalogueData: &model.AdminCatalogueData{Items: []model.AdminItemDTO{{ID: "hero_005", Name: "已解析目录名称夹具"}}, DisplayNames: map[string]string{"name_monster_033": "原媒体名称夹具"}, SourceTables: map[string][]map[string]string{"Hero": {{"id": "monster_044", "name": "name_monster_044"}, {"id": "monster_033", "name": "name_monster_033"}, {"id": "monster_missing", "name": "name_missing"}}}}}
	names := s.adminTrialEnemyNames()
	if names["monster_044"] != "原怪物名称夹具" || names["monster_033"] != "原媒体名称夹具" || names["hero_005"] != "已解析目录名称夹具" || names["monster_missing"] != "monster_missing" {
		t.Fatal("catalogue resolution drift", names)
	}
}
