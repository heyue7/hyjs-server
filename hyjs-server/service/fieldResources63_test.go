package service

import (
	"errors"
	"testing"

	"google.golang.org/protobuf/proto"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

func TestFieldResources63CrossMapQuotaLegacyAndDailyReset(t *testing.T) {
	field := &model.FieldState{Scene: "field_001"}
	limit := int64(1)
	fight := &model.FieldFight{BossRewardDailyLimit: &limit}
	content, err := fieldBossResourceContent(t.Context(), field, fight, "coin,1;item_005,1")
	if err != nil || content == "" || field.BossRewardClaims != 1 {
		t.Fatal("combined participation/kill counted incorrectly", err)
	}
	field.Scene = "field_002"
	content, err = fieldBossResourceContent(t.Context(), field, fight, "coin,1")
	if err != nil || content != "" || field.BossRewardClaims != 1 {
		t.Fatal("map change bypassed quota", err)
	}
	field.BossRewardDay = "2000-01-01"
	content, err = fieldBossResourceContent(t.Context(), field, fight, "coin,1")
	if err != nil || content == "" || field.BossRewardClaims != 1 {
		t.Fatal("next day did not reset", err)
	}
	for i := 0; i < 2; i++ {
		content, err = fieldBossResourceContent(t.Context(), field, &model.FieldFight{}, "coin,1")
		if err != nil || content == "" {
			t.Fatal("old accepted fight entitlement changed", err)
		}
	}
	if field.BossRewardClaims != 3 {
		t.Fatal("legacy rewards were not tracked")
	}
	_, err = fieldBossResourceContent(t.Context(), field, fight, "")
	if err != nil || field.BossRewardClaims != 3 {
		t.Fatal("empty reward consumed quota", err)
	}
}

func TestFieldResources63VersionAndAdminPermission(t *testing.T) {
	f := p1Setup(t)
	if err := f.s.InitializeFieldResources(f.ctx); err != nil {
		t.Fatal(err)
	}
	if err := AdminInitialize(f.ctx, ""); err != nil {
		t.Fatal(err)
	}
	user := db.AdminUser{Username: "field63-" + p6Digest([]byte(t.Name()))[:20], PasswordHash: "isolated-fixture", Role: constUtil.AdminRoleOperator, Enabled: true}
	if err := db.GetIns().Create(&user).Error; err != nil {
		t.Fatal(err)
	}
	actor := &model.AdminPrincipal{UserID: user.ID, Username: user.Username, Role: user.Role}
	current, err := f.s.AdminFieldResources(f.ctx, actor)
	if err != nil || current.BossDailyRewards != -1 {
		t.Fatal("legacy unlimited default", err)
	}
	req := model.AdminFieldResourcesSaveRequest{AdminFieldResourcesResponse: *current, Reason: "隔离野外资源限额验证"}
	req.BossDailyRewards = 1
	saved, err := f.s.AdminSaveFieldResources(f.ctx, actor, req)
	if err != nil || saved.BossDailyRewards != 1 || saved.Version == current.Version {
		t.Fatal("save", err)
	}
	if _, err = f.s.AdminSaveFieldResources(f.ctx, actor, req); !errors.Is(err, constUtil.ErrAdminConfigConflict) {
		t.Fatal("stale version accepted", err)
	}
	if _, err = f.s.AdminSaveFieldResources(f.ctx, nil, req); err == nil {
		t.Fatal("unauthorized save")
	}
	var audits int64
	db.GetIns().Model(&db.AdminAudit{}).Where("action=?", constUtil.FieldResourceAuditAction).Count(&audits)
	if audits != 1 {
		t.Fatal("audit count", audits)
	}
}

func TestFieldResources63FrozenStartEmptyRewardAndReceipt(t *testing.T) {
	f := p1Setup(t)
	if err := f.s.LoadP5Tables("../../analysis/tables"); err != nil {
		t.Fatal(err)
	}
	if err := f.s.InitializeP5(f.ctx); err != nil {
		t.Fatal(err)
	}
	if err := f.s.InitializeDailyCopy(f.ctx); err != nil {
		t.Fatal(err)
	}
	if err := f.s.InitializeFieldResources(f.ctx); err != nil {
		t.Fatal(err)
	}
	if err := db.GetIns().Model(&db.FieldResourcePolicy{}).Where("id=?", constUtil.FieldResourcePolicyID).Update("boss_daily_rewards", 0).Error; err != nil {
		t.Fatal(err)
	}
	if err := updateProfile(f.ctx, f.id, func(p *pb.Player, _ *model.GameState) error { p.Level = proto.Int32(100); return nil }); err != nil {
		t.Fatal(err)
	}
	if _, err := f.s.fieldTransfer(f.ctx, f.id, &pb.CSTransferScene{Target: proto.String("field_001")}); err != nil {
		t.Fatal(err)
	}
	_, game := fieldBoss33State(t, f.id)
	uuid := game.Field.Bosses[0].GetUuid()
	start, err := f.s.fieldBossFightStart(f.ctx, f.id, &pb.CSSidePveBossFightStart{Uuid: proto.String(uuid)})
	if err != nil {
		t.Fatal(err)
	}
	_, game = fieldBoss33State(t, f.id)
	if game.Field.Fight.BossRewardDailyLimit == nil || *game.Field.Fight.BossRewardDailyLimit != 0 {
		t.Fatal("start did not freeze limit")
	}
	if err := db.GetIns().Model(&db.FieldResourcePolicy{}).Where("id=?", constUtil.FieldResourcePolicyID).Update("boss_daily_rewards", 10).Error; err != nil {
		t.Fatal(err)
	}
	input, err := fieldBossFrozenInput(game.Field.Fight)
	if err != nil {
		t.Fatal(err)
	}
	terminal, _, err := f.s.ComputeP6GenericMechanismBattle(f.ctx, input, nil)
	if err != nil {
		t.Fatal(err)
	}
	reported := int32(0)
	if terminal.Outcome.Winner == 0 {
		reported = 1
	}
	request := &pb.CSSidePveBossFightEnd{FightId: start[0].(*pb.SCSidePveBossFightStart).FightId, Uuid: proto.String(uuid), Result: proto.Int32(reported), OptRecord: terminal.OriginalOperates}
	end, err := f.s.fieldBossFightEnd(f.ctx, f.id, request)
	if err != nil {
		t.Fatal(err)
	}
	if !proto.Equal(end[0].(*pb.SCSidePveBossFightEnd).GetGet(), &pb.ResultObject{}) {
		t.Fatal("frozen zero cap granted rewards")
	}
	_, game = fieldBoss33State(t, f.id)
	if game.Field.BossRewardClaims != 0 || !game.Field.Fight.Settled {
		t.Fatal("zero quota did not finish combat normally")
	}
	request.OptRecord = nil
	again, err := f.s.fieldBossFightEnd(f.ctx, f.id, request)
	if err != nil || !proto.Equal(again[0], end[0]) {
		t.Fatal("cold persisted receipt changed", err)
	}
}
