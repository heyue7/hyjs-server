package service

import (
	"bytes"
	"strings"
	"testing"

	"google.golang.org/protobuf/proto"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

// TestAdminFieldPKPublicationActualDualSession exercises the original PK message,
// real TCP presence and authoritative native terminal while the global PK switch
// remains false. Only the explicitly published map grants operational permission.
func TestAdminFieldPKPublicationActualDualSession(t *testing.T) {
	f, opponent, _, sessions := p12FieldPVPSetup(t)
	if err := f.s.LoadP12WorldBossTables("../../analysis/tables"); err != nil {
		t.Fatal(err)
	}
	if err := f.s.InitializeAdminFields(f.ctx); err != nil {
		t.Fatal(err)
	}
	if err := db.GetIns().AutoMigrate(&db.AdminUser{}, &db.AdminAudit{}); err != nil {
		t.Fatal(err)
	}
	operator := db.AdminUser{Username: "isolated-map-pk-operator", PasswordHash: "isolated-no-login", Role: constUtil.AdminRoleOperator, Enabled: true}
	if err := db.GetIns().Create(&operator).Error; err != nil {
		t.Fatal(err)
	}
	actor := &model.AdminPrincipal{UserID: operator.ID, Username: operator.Username, Role: operator.Role}
	var policy db.P12FieldPVPPolicy
	if err := db.GetIns().First(&policy, 1).Error; err != nil || policy.Enabled {
		t.Fatal("fixture must keep global PK off", err)
	}
	request := &pb.CSSidePvpFightResult{PlayerId: proto.Int64(opponent), FightType: proto.Int32(0)}
	dispatch := func(stamp int64) ([]proto.Message, error) {
		runtime, err := f.s.fieldRuntimeForRequest(f.ctx, &model.GamePacket{Body: request})
		if err != nil {
			return nil, err
		}
		ctx := bindFieldRuntime(f.ctx, runtime)
		out, handled, err := runtime.DispatchP12FieldPVP(ctx, f.id, sessions[f.id], request, stamp)
		if !handled {
			t.Fatal("original PK message not routed")
		}
		return out, err
	}
	before := p12FieldPVPProfiles(t, f.id, opponent)
	if _, err := dispatch(999601); err == nil || !strings.Contains(err.Error(), constUtil.P12FieldPVPClosed) {
		t.Fatal("undeployed map bypassed global off", err)
	}
	if !bytes.Equal(before, p12FieldPVPProfiles(t, f.id, opponent)) {
		t.Fatal("closed request mutated players")
	}
	cfg, err := f.s.AdminFieldOriginalReference("field_002")
	if err != nil {
		t.Fatal(err)
	}
	cfg.AllowPK = false
	scheme, err := f.s.AdminSaveField(f.ctx, actor, model.AdminFieldSaveRequest{Name: "隔离区服PK开关", Configuration: cfg})
	if err != nil {
		t.Fatal(err)
	}
	var deployment model.AdminFieldDeploymentDTO
	publish := func(allow bool) {
		t.Helper()
		cfg.AllowPK = allow
		if scheme.Configuration.AllowPK != allow {
			saved, e := f.s.AdminSaveField(f.ctx, actor, model.AdminFieldSaveRequest{ID: scheme.ID, Name: scheme.Name, Version: scheme.Version, Configuration: cfg})
			if e != nil {
				t.Fatal(e)
			}
			scheme = saved
		}
		published, e := f.s.AdminDeployField(f.ctx, actor, model.AdminFieldDeployRequest{ServerID: f.s.OriginalServerID(), Scene: cfg.Scene, TemplateID: scheme.ID, TemplateVersion: scheme.Version, Version: deployment.Version, Enabled: true})
		if e != nil {
			t.Fatal(e)
		}
		for _, row := range published.Items {
			if row.Scene == cfg.Scene {
				deployment = row
			}
		}
	}
	publish(false)
	if _, err = dispatch(999602); err == nil {
		t.Fatal("published map with PK forbidden accepted original request")
	}
	publish(true)
	// Preserve the existing native fixture's finite injury. No winner or terminal
	// is supplied by this test; the authoritative engine computes it from actors.
	if err = updateProfile(f.ctx, opponent, func(_ *pb.Player, g *model.GameState) error { g.Field.Side.CurHp = proto.Float64(.01); return nil }); err != nil {
		t.Fatal(err)
	}
	out, err := dispatch(999603)
	if err != nil || len(out) == 0 {
		t.Fatal("published allow did not reach real PK settlement", err)
	}
	response := out[0].(*pb.SCSidePvpFightResult)
	if response.GetCode() != 1 {
		t.Fatal("actual PK not accepted", response.GetCode())
	}
	var receipts []db.P12FieldPVPReceipt
	if err = db.GetIns().Where("player_id=?", f.id).Find(&receipts).Error; err != nil || len(receipts) != 1 || len(receipts[0].Terminal) == 0 {
		t.Fatal("native PK receipt missing", len(receipts), err)
	}
	if _, _, err = f.s.p12FieldPVPReadReceipt(db.GetIns(), &receipts[0]); err != nil {
		t.Fatal("actual certified receipt", err)
	}
	if err = db.GetIns().First(&policy, 1).Error; err != nil || policy.Enabled {
		t.Fatal("map publication changed global PK switch", err)
	}
	if _, err = f.s.p12FieldPVPMapPolicy(db.GetIns(), "field_001"); err == nil {
		t.Fatal("one published map opened undeployed map")
	}
	publish(false)
	if _, err = dispatch(999604); err == nil {
		t.Fatal("map PK disabled but fresh original request accepted")
	}
	// Existing settlement remains readable after map disablement, with no replay debit.
	beforeReplay := p12FieldPVPProfiles(t, f.id, opponent)
	replay, err := dispatch(999603)
	if err != nil {
		t.Fatal("existing accepted receipt blocked by new map policy", err)
	}
	encoded, _ := proto.Marshal(response)
	again, _ := proto.Marshal(replay[0])
	if !bytes.Equal(encoded, again) || !bytes.Equal(beforeReplay, p12FieldPVPProfiles(t, f.id, opponent)) {
		t.Fatal("old PK receipt changed/regranted")
	}
	// A map publication never bypasses the original mechanism identity.
	if err = db.GetIns().Model(&policy).Update("source", "invalid-policy-source").Error; err != nil {
		t.Fatal(err)
	}
	if _, err = f.s.p12FieldPVPMapPolicy(db.GetIns(), "field_002"); err == nil || !strings.Contains(err.Error(), constUtil.P12FieldPVPConflict) {
		t.Fatal("map policy bypassed original mechanism source", err)
	}
}
