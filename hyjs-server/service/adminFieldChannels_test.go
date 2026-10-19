package service

import (
	"google.golang.org/protobuf/proto"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
	"strings"
	"testing"
)

// TestAdminFieldChannelsOriginalProtocol keeps realm/map ownership, active-action protection and reconnect persistence on original messages.
func TestAdminFieldChannelsOriginalProtocol(t *testing.T) {
	f, other, _ := p10SceneSetup(t)
	for _, init := range []func() error{
		func() error { return f.s.LoadP8Tables("../../analysis/tables") },
		func() error { return f.s.LoadP12WorldBossTables("../../analysis/tables") },
		func() error { return f.s.InitializeAdminGameServers(f.ctx) },
		func() error { return f.s.InitializeAdminFields(f.ctx) },
		func() error { return f.s.InitializeP8Gather(f.ctx) },
	} {
		if err := init(); err != nil {
			t.Fatal(err)
		}
	}
	if err := db.GetIns().AutoMigrate(&db.AdminUser{}, &db.AdminAudit{}); err != nil {
		t.Fatal(err)
	}
	operator := db.AdminUser{Username: "isolated-field-channel-operator", PasswordHash: "isolated-no-login", Role: constUtil.AdminRoleOperator, Enabled: true}
	if err := db.GetIns().Create(&operator).Error; err != nil {
		t.Fatal(err)
	}
	actor := &model.AdminPrincipal{UserID: operator.ID, Username: operator.Username, Role: operator.Role}
	req := model.AdminFieldChannelSaveRequest{ServerID: f.s.OriginalServerID(), Scene: "field_001", LineCount: 3, MaxPlayers: 7}
	result, err := f.s.AdminSaveFieldChannels(f.ctx, actor, req)
	if err != nil {
		t.Fatal(err)
	}
	for _, row := range result.Items {
		if row.Scene == req.Scene {
			req.Version = row.Version
		}
	}
	if _, err = f.s.AdminSaveFieldChannels(f.ctx, actor, model.AdminFieldChannelSaveRequest{ServerID: req.ServerID, Scene: req.Scene, LineCount: 2, MaxPlayers: 8}); err == nil {
		t.Fatal("stale capacity save accepted")
	}
	// Only legitimate map access and actual socket sessions are prepared, with no combat outcomes or resources injected.
	for _, id := range []int64{f.id, other} {
		if err = updateProfile(f.ctx, id, func(p *pb.Player, g *model.GameState) error { p.Level = proto.Int32(25); return nil }); err != nil {
			t.Fatal(err)
		}
		if _, err = f.s.fieldTransfer(f.ctx, id, &pb.CSTransferScene{Target: proto.String(req.Scene)}); err != nil {
			t.Fatal(err)
		}
		if _, err = f.s.fieldScene(f.ctx, id, false); err != nil {
			t.Fatal(err)
		}
		p10SceneConnect(t, f, id, &p10SceneRecorder{})
	}
	frames, _, err := f.s.DispatchP10Scene(f.ctx, f.id, &pb.CSGetLineInfo{}, 0)
	if err != nil || len(frames[0].(*pb.SCGetLineInfo).GetLineInfos()) != 3 {
		t.Fatal("configured original line list", err)
	}
	var attempt = db.P8GatherAttempt{PlayerID: f.id, Active: true}
	if err = db.GetIns().Create(&attempt).Error; err != nil {
		t.Fatal(err)
	}
	if _, _, err = f.s.DispatchP10Scene(f.ctx, f.id, &pb.CSTransferLine{Line: proto.Int32(3)}, 0); err == nil || !strings.Contains(err.Error(), "采集") {
		t.Fatal("accepted gather torn down by line transfer", err)
	}
	if err = db.GetIns().Model(&db.P8GatherAttempt{}).Where("player_id=?", f.id).Update("active", false).Error; err != nil {
		t.Fatal(err)
	}
	if _, _, err = f.s.DispatchP10Scene(f.ctx, f.id, &pb.CSTransferLine{Line: proto.Int32(3)}, 0); err != nil {
		t.Fatal(err)
	}
	frames, err = f.s.fieldScene(f.ctx, f.id, false)
	if err != nil || frames[0].(*pb.SCCurrentPosition).GetLimit() != 7 {
		t.Fatal("original post-line scene recovery capacity", err)
	}
	req.LineCount = 2
	if _, err = f.s.AdminSaveFieldChannels(f.ctx, actor, req); err == nil {
		t.Fatal("occupied line removed")
	}
	frames, _, err = f.s.DispatchP10Scene(f.ctx, other, &pb.CSMovePlayerInfo{PlayerIds: []int64{f.id}}, 0)
	if err != nil || len(frames[0].(*pb.SCMovePlayerInfo).GetPlayerInfos()) != 0 {
		t.Fatal("cross-line actor leaked", err)
	}
	// A different map continues to use the original two-line default.
	var cfg db.P10ScenePolicy
	if err = db.GetIns().First(&cfg, constUtil.P10ScenePolicyID).Error; err != nil {
		t.Fatal(err)
	}
	snow, err := f.s.adminFieldSceneConfig(db.GetIns(), "field_002", &cfg)
	if err != nil || snow.LineCount != 2 || snow.MaxPlayers != 100 {
		t.Fatal("map capacity leaked", err)
	}
	if err = db.GetIns().Model(&db.P10ScenePolicy{}).Where("id=?", constUtil.P10ScenePolicyID).Update("enabled", false).Error; err != nil {
		t.Fatal(err)
	}
	if err = f.s.InitializeAdminFields(f.ctx); err != nil {
		t.Fatal(err)
	}
	var persisted db.P10ScenePolicy
	if err = db.GetIns().First(&persisted, constUtil.P10ScenePolicyID).Error; err != nil || persisted.Enabled {
		t.Fatal("restart undid operator line shutdown", err)
	}
	forest, err := f.s.adminFieldSceneConfig(db.GetIns(), "field_001", &cfg)
	if err != nil || forest.LineCount != 3 || forest.MaxPlayers != 7 {
		t.Fatal("restart replaced published capacity", err)
	}
}
