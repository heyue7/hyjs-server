package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

func recruitRuntimeInfo(t *testing.T, s *GameService, ctx context.Context, id int64, kind int32) *pb.WarriorRecruit {
	t.Helper()
	messages, err := s.recruitInfo(ctx, id, &pb.CSWarriorRecruitGet{CostType: []int32{kind}})
	if err != nil || len(messages) != 1 {
		t.Fatal("recruit info", kind, err)
	}
	entries := messages[0].(*pb.SCWarriorRecruitGet).GetRecruit()
	if len(entries) != 1 || entries[0].GetCostType() != kind {
		t.Fatal("recruit info entry", kind, entries)
	}
	return entries[0]
}

func setRecruitRuntimeRule(t *testing.T, kind, daily, guarantee int32) {
	t.Helper()
	if err := db.GetIns().Model(&db.RecruitRuntimeRule{}).Where("kind=?", kind).Updates(map[string]any{"daily_limit": daily, "guarantee_every": guarantee}).Error; err != nil {
		t.Fatal(err)
	}
}

// 王者界面num=每日余量、leftTimes=保底倒计时；免费与十连共用每日次数，降上限不回退已用次数。
func TestRecruitRuntimeKingFieldsAndServerLimit(t *testing.T) {
	s, ctx := newP2Test(t)
	id := dailyCopyTestPlayer(t, ctx, "runtime-king", 20)
	p2Funding(t, s, ctx, id, "diamond,30000")
	if info := recruitRuntimeInfo(t, s, ctx, id, 3); info.GetNum() != 50 || info.GetLeftTimes() != 9 {
		t.Fatal("initial king daily/pity fields", info)
	}
	if messages, err := s.warriorRecruit(ctx, id, p2Recruit(1, 1), 91001); err != nil || len(messages[0].(*pb.SCWarriorRecruit).GetCost()) != 0 {
		t.Fatal("actual king free draw", err)
	}
	if info := recruitRuntimeInfo(t, s, ctx, id, 3); info.GetNum() != 49 || info.GetLeftTimes() < 0 || info.GetLeftTimes() > 9 {
		t.Fatal("king free draw fields", info)
	}
	if messages, err := s.warriorRecruit(ctx, id, p2Recruit(2, 0), 91002); err != nil || len(messages[0].(*pb.SCWarriorRecruit).GetGet().GetWars()) != 10 {
		t.Fatal("actual king ten draw", err)
	}
	if info := recruitRuntimeInfo(t, s, ctx, id, 3); info.GetNum() != 39 || info.GetLeftTimes() < 0 || info.GetLeftTimes() > 9 {
		t.Fatal("king ten draw fields", info)
	}
	// 上限调低到已用次数以下时拒绝且不扣费，历史Draws保留。
	setRecruitRuntimeRule(t, constUtil.RecruitRuntimeKindKing, 5, 10)
	before, beforeGame := p2Profile(t, id)
	var beforeKing db.KingRecruitState
	if err := db.GetIns().First(&beforeKing, id).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := s.warriorRecruit(ctx, id, p2Recruit(1, 0), 91003); err == nil {
		t.Fatal("king limit accepted paid draw")
	}
	after, afterGame := p2Profile(t, id)
	var afterKing db.KingRecruitState
	if err := db.GetIns().First(&afterKing, id).Error; err != nil {
		t.Fatal(err)
	}
	if !proto.Equal(before, after) || len(beforeGame.Warriors) != len(afterGame.Warriors) || beforeKing.Draws != 11 || afterKing.Draws != 11 {
		t.Fatal("king limit charged or reset used draws", beforeKing, afterKing)
	}
	if info := recruitRuntimeInfo(t, s, ctx, id, 3); info.GetNum() != 0 || info.GetLeftTimes() < 0 || info.GetLeftTimes() > 9 {
		t.Fatal("king limit fields", info)
	}
}

// 强者招募每日上限覆盖免费与付费，十连按10次计数，且与王者次数独立。
func TestRecruitRuntimeStrongDailyIncludesFreeAndTen(t *testing.T) {
	s, ctx := newP2Test(t)
	id := dailyCopyTestPlayer(t, ctx, "runtime-strong", 1)
	p2Funding(t, s, ctx, id, "diamond,30000")
	setRecruitRuntimeRule(t, constUtil.RecruitRuntimeKindStrong, 1, 10)
	if messages, err := s.warriorRecruit(ctx, id, &pb.CSWarriorRecruit{CostType: proto.Int32(2), NumType: proto.Int32(1), Free: proto.Int32(1)}, 92001); err != nil || len(messages[0].(*pb.SCWarriorRecruit).GetCost()) != 0 {
		t.Fatal("actual strong free draw", err)
	}
	_, game := p2Profile(t, id)
	if game.Recruit[2].Draws != 1 {
		t.Fatal("strong free draw not counted", game.Recruit[2])
	}
	before, beforeGame := p2Profile(t, id)
	if _, err := s.warriorRecruit(ctx, id, &pb.CSWarriorRecruit{CostType: proto.Int32(2), NumType: proto.Int32(1), Free: proto.Int32(0)}, 92002); err == nil {
		t.Fatal("strong paid draw passed free-counted limit")
	}
	after, afterGame := p2Profile(t, id)
	if !proto.Equal(before, after) || len(beforeGame.Warriors) != len(afterGame.Warriors) || afterGame.Recruit[2].Draws != 1 {
		t.Fatal("strong limit charged or reset free-counted draw", afterGame.Recruit[2])
	}
	if info := recruitRuntimeInfo(t, s, ctx, id, 3); info.GetNum() != 50 || info.GetLeftTimes() != 9 {
		t.Fatal("strong limit changed king fields", info)
	}
}

func TestRecruitRuntimeStrongTenAndIndependentLimit(t *testing.T) {
	s, ctx := newP2Test(t)
	id := dailyCopyTestPlayer(t, ctx, "runtime-strong-ten", 1)
	p2Funding(t, s, ctx, id, "diamond,30000")
	setRecruitRuntimeRule(t, constUtil.RecruitRuntimeKindStrong, 10, 10)
	if messages, err := s.warriorRecruit(ctx, id, &pb.CSWarriorRecruit{CostType: proto.Int32(2), NumType: proto.Int32(2), Free: proto.Int32(0)}, 93001); err != nil || len(messages[0].(*pb.SCWarriorRecruit).GetGet().GetWars()) != 10 {
		t.Fatal("actual strong ten draw", err)
	}
	_, game := p2Profile(t, id)
	if game.Recruit[2].Draws != 10 {
		t.Fatal("strong ten not counted as ten", game.Recruit[2])
	}
	before, beforeGame := p2Profile(t, id)
	if _, err := s.warriorRecruit(ctx, id, &pb.CSWarriorRecruit{CostType: proto.Int32(2), NumType: proto.Int32(1), Free: proto.Int32(0)}, 93002); err == nil {
		t.Fatal("strong ten limit accepted an eleventh draw")
	}
	after, afterGame := p2Profile(t, id)
	if !proto.Equal(before, after) || len(beforeGame.Warriors) != len(afterGame.Warriors) || afterGame.Recruit[2].Draws != 10 {
		t.Fatal("strong ten limit charged or reset draws", afterGame.Recruit[2])
	}
	setRecruitRuntimeRule(t, constUtil.RecruitRuntimeKindStrong, 12, 10)
	for _, stamp := range []int64{93003, 93004} {
		if _, err := s.warriorRecruit(ctx, id, &pb.CSWarriorRecruit{CostType: proto.Int32(2), NumType: proto.Int32(1), Free: proto.Int32(0)}, stamp); err != nil {
			t.Fatal("raised strong limit rejected draw", stamp, err)
		}
	}
	if _, err := s.warriorRecruit(ctx, id, &pb.CSWarriorRecruit{CostType: proto.Int32(2), NumType: proto.Int32(1), Free: proto.Int32(0)}, 93005); err == nil {
		t.Fatal("raised strong limit accepted thirteenth draw")
	}
	_, game = p2Profile(t, id)
	if game.Recruit[2].Draws != 12 {
		t.Fatal("strong draws after raised limit", game.Recruit[2].Draws)
	}
	var kingCount int64
	if err := db.GetIns().Model(&db.KingRecruitState{}).Where("player_id=?", id).Count(&kingCount).Error; err != nil || kingCount != 0 {
		t.Fatal("strong draws changed king counter", kingCount, err)
	}
}

// 运行覆盖修改保底后下一次到阈即保底，且不改原静态奖池。
func TestRecruitRuntimeGuaranteeOverrideUsesRealGuaranteedResult(t *testing.T) {
	s, ctx := newP2Test(t)
	id := dailyCopyTestPlayer(t, ctx, "runtime-guarantee", 20)
	p2Funding(t, s, ctx, id, "diamond,20000")
	setRecruitRuntimeRule(t, constUtil.RecruitRuntimeKindKing, 50, 3)
	if err := updateProfile(ctx, id, func(_ *pb.Player, g *model.GameState) error {
		recruitState(g, 3, time.Now()).SinceGuarantee = 2
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if info := recruitRuntimeInfo(t, s, ctx, id, 3); info.GetNum() != 50 || info.GetLeftTimes() != 0 {
		t.Fatal("runtime guarantee countdown", info)
	}
	messages, err := s.warriorRecruit(ctx, id, p2Recruit(1, 0), 94001)
	if err != nil {
		t.Fatal(err)
	}
	wars := messages[0].(*pb.SCWarriorRecruit).GetGet().GetWars()
	if len(wars) != 1 || (s.table("Hero", "id", wars[0].GetId())["quality"] != "20" && s.table("Hero", "id", wars[0].GetId())["quality"] != "23") {
		t.Fatal("runtime guarantee did not award original orange-or-red high quality", messages)
	}
	if info := recruitRuntimeInfo(t, s, ctx, id, 3); info.GetLeftTimes() != 2 {
		t.Fatal("guarantee reset countdown", info)
	}
	cfg, _, err := s.loadGameplayPool(db.GetIns(), "recruit_3")
	if err != nil || cfg.GuaranteeEvery != 3 {
		t.Fatal("runtime guarantee not applied to loaded copy", cfg, err)
	}
	var static db.GameplayPool
	if err := db.GetIns().First(&static, "id=?", "recruit_3").Error; err != nil || static.GuaranteeEvery != 10 {
		t.Fatal("runtime guarantee rewrote static pool", static, err)
	}
	setRecruitRuntimeRule(t, constUtil.RecruitRuntimeKindKing, 50, 1)
	messages, err = s.warriorRecruit(ctx, id, p2Recruit(1, 0), 94002)
	if err != nil {
		t.Fatal(err)
	}
	wars = messages[0].(*pb.SCWarriorRecruit).GetGet().GetWars()
	if len(wars) != 1 || (s.table("Hero", "id", wars[0].GetId())["quality"] != "20" && s.table("Hero", "id", wars[0].GetId())["quality"] != "23") {
		t.Fatal("immediate runtime guarantee did not award original orange-or-red high quality", messages)
	}
}

// 强者每日计数沿用角色日历归档，跨天重置，回到旧日期恢复已用次数。
func TestRecruitRuntimeStrongDayRolloverAndRewind(t *testing.T) {
	f, _ := localPlayActivitiesFixture(t)
	p4Fund(t, f)
	setRecruitRuntimeRule(t, constUtil.RecruitRuntimeKindStrong, 1, 10)
	base := growthRecruitOffset(f.ctx, 0)
	next := growthRecruitOffset(f.ctx, 1)
	request := &pb.CSWarriorRecruit{CostType: proto.Int32(2), NumType: proto.Int32(1), Free: proto.Int32(0)}
	if _, err := f.s.warriorRecruit(base, f.id, request, 95001); err != nil {
		t.Fatal("strong base day draw", err)
	}
	_, game := f.profile(t)
	if game.Recruit[2].Draws != 1 || game.Recruit[2].DrawDay != p1Day(LocalPlayNow(base)) {
		t.Fatal("strong base day count", game.Recruit[2])
	}
	before, beforeGame := f.profile(t)
	if _, err := f.s.warriorRecruit(base, f.id, request, 95002); err == nil {
		t.Fatal("strong base day accepted second draw")
	}
	after, afterGame := f.profile(t)
	if !proto.Equal(before, after) || len(beforeGame.Warriors) != len(afterGame.Warriors) {
		t.Fatal("strong base limit charged")
	}
	if _, err := f.s.warriorRecruit(next, f.id, request, 95003); err != nil {
		t.Fatal("strong next day draw", err)
	}
	_, game = f.profile(t)
	if game.Recruit[2].Draws != 1 || game.Recruit[2].DrawDay != p1Day(LocalPlayNow(next)) {
		t.Fatal("strong next day count", game.Recruit[2])
	}
	for _, stamp := range []int64{95004, 95005} {
		before, beforeGame = f.profile(t)
		if _, err := f.s.warriorRecruit(base, f.id, request, stamp); err == nil {
			t.Fatal("strong rewound old day accepted draw", stamp)
		}
		after, afterGame = f.profile(t)
		if !proto.Equal(before, after) || len(beforeGame.Warriors) != len(afterGame.Warriors) {
			t.Fatal("strong rewound old day charged", stamp)
		}
	}
	if info := recruitRuntimeInfo(t, f.s, next, f.id, 3); info.GetNum() != 50 || info.GetLeftTimes() != 9 {
		t.Fatal("strong draws changed king fields", info)
	}
}

func TestRecruitRuntimeAdminSettingsVersionPermissionAndAudit(t *testing.T) {
	s, ctx := newP2Test(t)
	if err := AdminInitialize(ctx, ""); err != nil {
		t.Fatal(err)
	}
	operator := db.AdminUser{Username: "recruit-runtime-operator", PasswordHash: "unused-fixture", Role: constUtil.AdminRoleOperator, Enabled: true}
	viewer := db.AdminUser{Username: "recruit-runtime-viewer", PasswordHash: "unused-fixture", Role: constUtil.AdminRoleViewer, Enabled: true}
	if err := db.GetIns().Create(&operator).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.GetIns().Create(&viewer).Error; err != nil {
		t.Fatal(err)
	}
	writeActor := &model.AdminPrincipal{UserID: operator.ID, Username: operator.Username, Role: operator.Role}
	readActor := &model.AdminPrincipal{UserID: viewer.ID, Username: viewer.Username, Role: viewer.Role}
	read, err := s.AdminRecruitSettings(ctx, readActor)
	if err != nil || len(read.Items) != 2 || len(read.Version) != 64 {
		t.Fatal("admin recruit settings read", read, err)
	}
	if read.Items[0].Kind != 2 || read.Items[0].Name != constUtil.RecruitRuntimeStrongName || read.Items[0].DailyLimit != 0 || read.Items[0].GuaranteeEvery != 10 || read.Items[1].Kind != 3 || read.Items[1].Name != constUtil.RecruitRuntimeKingName || read.Items[1].DailyLimit != 50 || read.Items[1].GuaranteeEvery != 10 {
		t.Fatal("admin recruit defaults", read.Items)
	}
	valid := []model.AdminRecruitSetting{{Kind: 2, Name: constUtil.RecruitRuntimeStrongName, DailyLimit: 3, GuaranteeEvery: 5}, {Kind: 3, Name: constUtil.RecruitRuntimeKingName, DailyLimit: 20, GuaranteeEvery: 4, LuckExchangeCost: 500}}
	if _, err = s.AdminSaveRecruitSettings(ctx, readActor, model.AdminRecruitSettingsSaveRequest{Version: read.Version, Reason: "只读账号不可写", Items: valid}); !errors.Is(err, constUtil.ErrAdminPermission) {
		t.Fatal("viewer saved recruit runtime", err)
	}
	if _, err = s.AdminSaveRecruitSettings(ctx, writeActor, model.AdminRecruitSettingsSaveRequest{Version: "0000000000000000000000000000000000000000000000000000000000000000", Reason: "旧版本", Items: valid}); !errors.Is(err, constUtil.ErrAdminConfigConflict) {
		t.Fatal("stale recruit runtime version accepted", err)
	}
	badRange := []model.AdminRecruitSetting{{Kind: 2, Name: constUtil.RecruitRuntimeStrongName, DailyLimit: 0, GuaranteeEvery: 10}, {Kind: 3, Name: constUtil.RecruitRuntimeKingName, DailyLimit: 0, GuaranteeEvery: 10}}
	if _, err = s.AdminSaveRecruitSettings(ctx, writeActor, model.AdminRecruitSettingsSaveRequest{Version: read.Version, Reason: "越界", Items: badRange}); !errors.Is(err, constUtil.ErrAdminInput) {
		t.Fatal("out-of-range recruit runtime accepted", err)
	}
	duplicate := []model.AdminRecruitSetting{{Kind: 2, Name: constUtil.RecruitRuntimeStrongName, DailyLimit: 1, GuaranteeEvery: 10}, {Kind: 2, Name: constUtil.RecruitRuntimeStrongName, DailyLimit: 2, GuaranteeEvery: 10}}
	if _, err = s.AdminSaveRecruitSettings(ctx, writeActor, model.AdminRecruitSettingsSaveRequest{Version: read.Version, Reason: "重复", Items: duplicate}); !errors.Is(err, constUtil.ErrAdminInput) {
		t.Fatal("duplicate recruit runtime accepted", err)
	}
	saved, err := s.AdminSaveRecruitSettings(ctx, writeActor, model.AdminRecruitSettingsSaveRequest{Version: read.Version, Reason: "调整每日次数与保底", Items: valid})
	if err != nil || saved.Version == read.Version || len(saved.Items) != 2 || saved.Items[0].DailyLimit != 3 || saved.Items[0].GuaranteeEvery != 5 || saved.Items[1].DailyLimit != 20 || saved.Items[1].GuaranteeEvery != 4 {
		t.Fatal("admin recruit save", saved, err)
	}
	fresh, err := s.AdminRecruitSettings(ctx, writeActor)
	if err != nil || fresh.Version != saved.Version || fresh.Items[0].GuaranteeEvery != 5 || fresh.Items[1].DailyLimit != 20 {
		t.Fatal("admin recruit read after save", fresh, err)
	}
	cfg2, _, err := s.loadGameplayPool(db.GetIns(), "recruit_2")
	if err != nil || cfg2.GuaranteeEvery != 5 {
		t.Fatal("strong runtime guarantee not applied", cfg2, err)
	}
	cfg3, _, err := s.loadGameplayPool(db.GetIns(), "recruit_3")
	if err != nil || cfg3.GuaranteeEvery != 4 {
		t.Fatal("king runtime guarantee not applied", cfg3, err)
	}
	rules, _, _, err := s.kingConfiguration(db.GetIns(), LocalPlayNow(ctx))
	if err != nil || rules.DailyLimit != 20 {
		t.Fatal("king runtime daily not applied", rules, err)
	}
	var audits int64
	if err = db.GetIns().Model(&db.AdminAudit{}).Where("action=? AND target=?", constUtil.RecruitRuntimeAuditAction, constUtil.RecruitRuntimeAuditTarget).Count(&audits).Error; err != nil || audits != 1 {
		t.Fatal("recruit runtime audit", audits, err)
	}
	if _, err = s.AdminSaveRecruitSettings(ctx, writeActor, model.AdminRecruitSettingsSaveRequest{Version: read.Version, Reason: "重复旧版本", Items: valid}); !errors.Is(err, constUtil.ErrAdminConfigConflict) {
		t.Fatal("saved version did not advance", err)
	}
}
