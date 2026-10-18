package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

// TestAdminFieldPublishedConsumers verifies the real map resource path on a finite
// isolated profile, including frozen attempt, future deployment and idempotent claim.
func TestAdminFieldPublishedConsumers(t *testing.T) {
	f := p1Setup(t)
	s := f.s
	for _, load := range []func(string) error{s.LoadP8Tables, s.LoadP12WorldBossTables} {
		if err := load("../../analysis/tables"); err != nil {
			t.Fatal(err)
		}
	}
	for _, init := range []func(context.Context) error{s.InitializeP8Gather, s.InitializeP8Life, s.InitGems, s.InitCooking, s.InitializeAdminFields} {
		if err := init(f.ctx); err != nil {
			t.Fatal(err)
		}
	}
	if err := db.GetIns().AutoMigrate(&db.AdminUser{}, &db.AdminAudit{}, &db.DailyCopyFight{}); err != nil {
		t.Fatal(err)
	}
	user := db.AdminUser{Username: "isolated-field-operator", Role: constUtil.AdminRoleOperator, Enabled: true, PasswordHash: "isolated-no-login"}
	if err := db.GetIns().Create(&user).Error; err != nil {
		t.Fatal(err)
	}
	actor := &model.AdminPrincipal{UserID: user.ID, Username: user.Username, Role: user.Role}
	catalog, err := s.AdminFields(f.ctx, actor)
	if err != nil || len(catalog.Items) != 6 {
		t.Fatal("original full map schemes", len(catalog.Items), err)
	}
	var first, snow, missing model.AdminFieldTemplateDTO
	for _, v := range catalog.Items {
		switch v.Configuration.Scene {
		case "field_001":
			first = v
		case "field_002":
			snow = v
		case "field_003":
			missing = v
		}
	}
	if !first.Available || !snow.Available || missing.Available {
		t.Fatal("packaged resources confused with server configuration")
	}
	cfg := first.Configuration
	cfg.MineCount = 1
	cfg.FishCount = 0
	cfg.MinLevel = 18
	cfg.AllowPK = false
	// Restrict one original point to one original low-level resource group. This
	// finite fixture changes the next actual gather without injecting inventory.
	cfg.MinePoints = cfg.MinePoints[:1]
	mineID := "group_mine_001"
	cfg.MinePoints[0].Choices = []model.AdminFieldWeight{{ID: mineID, Weight: 1}}
	var material string
	for i := range cfg.GatherGroups {
		g := &cfg.GatherGroups[i]
		if g.ID == mineID {
			material = g.Drops[0].ResourceID
			g.GatherMS = 1
			g.Drops = []model.AdminFieldDrop{{ResourceID: material, Quantities: []model.AdminFieldQuantity{{Amount: 2, Weight: 1}}}}
		}
	}
	published, err := s.AdminSaveField(f.ctx, actor, model.AdminFieldSaveRequest{Name: "隔离森林双份矿", Configuration: cfg})
	if err != nil {
		t.Fatal(err)
	}
	dep, err := s.AdminDeployField(f.ctx, actor, model.AdminFieldDeployRequest{ServerID: 1, Scene: cfg.Scene, TemplateID: published.ID, TemplateVersion: published.Version, Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	var deployed model.AdminFieldDeploymentDTO
	for _, v := range dep.Items {
		if v.Scene == cfg.Scene {
			deployed = v
		}
	}
	if err = s.AdminDeleteField(f.ctx, actor, model.AdminFieldDeleteRequest{ID: published.ID, Version: published.Version}); err == nil {
		t.Fatal("deployed template deleted")
	}
	if _, err = s.AdminDeployField(f.ctx, actor, model.AdminFieldDeployRequest{ServerID: 1, Scene: missing.Configuration.Scene, TemplateID: missing.ID, TemplateVersion: missing.Version, Enabled: true}); err == nil {
		t.Fatal("missing scene was enabled")
	}
	tables, err := s.AdminFieldRuntimeTables(f.ctx, 1)
	if err != nil {
		t.Fatal(err)
	}
	runtime := &GameService{Tables: tables, P8GatherReady: true}
	other, err := s.AdminFieldRuntimeTables(f.ctx, 2)
	if err != nil || other["Field"][0]["MineMax"] != s.Tables["Field"][0]["MineMax"] {
		t.Fatal("realm deployment leaked", err)
	}
	if s.table("Field", "FieldId", cfg.Scene)["MineMax"] == "1" {
		t.Fatal("shared original tables mutated")
	}
	if err = updateProfile(f.ctx, f.id, func(p *pb.Player, g *model.GameState) error { p.Level = proto.Int32(30); return nil }); err != nil {
		t.Fatal(err)
	}
	if _, err = runtime.fieldTransfer(f.ctx, f.id, &pb.CSTransferScene{Target: proto.String(cfg.Scene)}); err != nil {
		t.Fatal(err)
	}
	if _, err = runtime.fieldScene(f.ctx, f.id, false); err != nil {
		t.Fatal("original current-position resource projection", err)
	}
	var nodes []db.P8ResourceNode
	if err = db.GetIns().Where("player_id=? AND scene=?", f.id, cfg.Scene).Find(&nodes).Error; err != nil || len(nodes) != 1 {
		t.Fatal("published mine count", len(nodes), err)
	}
	node := nodes[0]
	if node.ResourceID != mineID {
		t.Fatal("internal scene namespace escaped to original client", node.ResourceID)
	}
	var gathering p8GatherConfig
	if err = json.Unmarshal([]byte(node.Snapshot), &gathering); err != nil {
		t.Fatal(err)
	}
	if err = updateProfile(f.ctx, f.id, func(_ *pb.Player, g *model.GameState) error {
		g.Position.Current = proto.Clone(gathering.Point).(*pb.PointNew)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if _, err = runtime.p8StartGather(f.ctx, f.id, &pb.CSFieldResourceStartGather{PosId: proto.String(node.PosID)}, 991001); err != nil {
		t.Fatal(err)
	}
	// Change both global source and actual realm deployment during a live attempt.
	// Claim must use the accepted two-piece grant, not the new nine-piece grant.
	changed := published.Configuration
	raw, _ := json.Marshal(changed)
	json.Unmarshal(raw, &changed)
	for i := range changed.GatherGroups {
		if changed.GatherGroups[i].ID == mineID {
			changed.GatherGroups[i].Drops[0].Quantities[0].Amount = 9
		}
	}
	changed.MinLevel = 999
	saved, err := s.AdminSaveField(f.ctx, actor, model.AdminFieldSaveRequest{ID: published.ID, Name: published.Name, Version: published.Version, Configuration: changed})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.AdminSaveField(f.ctx, actor, model.AdminFieldSaveRequest{ID: published.ID, Name: published.Name, Version: published.Version, Configuration: changed}); !errors.Is(err, constUtil.ErrAdminConfigConflict) {
		t.Fatal("stale source edit", err)
	}
	if _, err = s.AdminDeployField(f.ctx, actor, model.AdminFieldDeployRequest{ServerID: 1, Scene: cfg.Scene, TemplateID: saved.ID, TemplateVersion: saved.Version, Version: deployed.Version, Enabled: false}); err != nil {
		t.Fatal(err)
	}
	tables, err = s.AdminFieldRuntimeTables(f.ctx, 1)
	if err != nil {
		t.Fatal(err)
	}
	runtime = &GameService{Tables: tables, P8GatherReady: true}
	before, gBefore := f.profile(t)
	beforeFatigue := gBefore.Field.Side.GetTiredness()
	_ = before
	if err = db.GetIns().Model(&db.P8GatherAttempt{}).Where("player_id=?", f.id).Update("ready_at", time.Now().Add(-time.Second).UnixMilli()).Error; err != nil {
		t.Fatal(err)
	}
	claim := &pb.CSFieldGetResource{}
	out, err := runtime.p8ClaimGather(f.ctx, f.id, claim, 991002)
	if err != nil {
		t.Fatal(err)
	}
	gained := out[0].(*pb.SCFieldGetResource)
	if gained.GetCode() != 1 {
		t.Fatal("claim result", gained)
	}
	_, gAfter := f.profile(t)
	var amount int32
	for _, item := range gAfter.Gems {
		if item.GetId() == material {
			amount = item.GetNum()
		}
	}
	if amount != 2 || gAfter.Field.Side.GetTiredness() != beforeFatigue-gathering.Fatigue {
		t.Fatal("actual frozen grant/fatigue", amount, gAfter.Field.Side.GetTiredness(), beforeFatigue, gathering.Fatigue)
	}
	encoded, _ := proto.Marshal(out[0])
	replay, err := runtime.p8ClaimGather(f.ctx, f.id, claim, 991002)
	if err != nil {
		t.Fatal(err)
	}
	again, _ := proto.Marshal(replay[0])
	if !bytes.Equal(encoded, again) {
		t.Fatal("claim receipt changed")
	}
	_, gAgain := f.profile(t)
	for _, item := range gAgain.Gems {
		if item.GetId() == material && item.GetNum() != 2 {
			t.Fatal("claim duplicated grant")
		}
	}
	if _, err = runtime.fieldTransfer(f.ctx, f.id, &pb.CSTransferScene{Target: proto.String("field_003")}); err == nil {
		t.Fatal("missing scene entry")
	}
	if _, err = runtime.p8StartGather(f.ctx, f.id, &pb.CSFieldResourceStartGather{PosId: proto.String(node.PosID)}, 991003); err == nil {
		t.Fatal("disabled realm map accepted new gather")
	}
	reloaded, err := s.AdminFieldDeployments(f.ctx, actor, 1)
	if err != nil {
		t.Fatal(err)
	}
	for _, v := range reloaded.Items {
		if v.Scene == cfg.Scene {
			deployed = v
		}
	}
	changed.MinLevel = 18
	changed.AllowPK = true
	reopened, err := s.AdminSaveField(f.ctx, actor, model.AdminFieldSaveRequest{ID: saved.ID, Name: saved.Name, Version: saved.Version, Configuration: changed})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.AdminDeployField(f.ctx, actor, model.AdminFieldDeployRequest{ServerID: 1, Scene: cfg.Scene, TemplateID: reopened.ID, TemplateVersion: reopened.Version, Version: deployed.Version, Enabled: true}); err != nil {
		t.Fatal(err)
	}
	tables, err = s.AdminFieldRuntimeTables(f.ctx, 1)
	if err != nil {
		t.Fatal(err)
	}
	runtime = &GameService{Tables: tables, P8GatherReady: true}
	// The published monster drop and cooldown are frozen on acceptance, then a
	// subsequent source change cannot rewrite that active battle's settings.
	if _, err = runtime.fieldScene(f.ctx, f.id, false); err != nil {
		t.Fatal("published scene refresh before selecting monster", err)
	}
	_, fieldState := f.profile(t)
	target := fieldState.Field.Monsters[0]
	starts, err := runtime.fieldFightStart(f.ctx, f.id, &pb.CSSidePveFightStart{MonsterId: target.Uuid})
	if err != nil {
		t.Fatal("actual field fight acceptance", err)
	}
	accepted := starts[0].(*pb.SCSidePveFightStart)
	frozen, cd, err := runtime.adminFieldBattleReward(db.GetIns(), f.id, cfg.Scene, accepted.GetFightId(), target.GetMonId())
	if err != nil || frozen == "" || cd < 1 {
		t.Fatal("fight snapshot", err)
	}
	runtime.fieldLeader(target.GetMonId())["rewardGroupId"] = "coin:999999,1"
	check, checkCD, err := runtime.adminFieldBattleReward(db.GetIns(), f.id, cfg.Scene, accepted.GetFightId(), target.GetMonId())
	if err != nil || check != frozen || checkCD != cd {
		t.Fatal("ongoing battle config rewritten", err)
	}
	if _, _, err = runtime.adminFieldBattleReward(db.GetIns(), f.id+1, cfg.Scene, accepted.GetFightId(), target.GetMonId()); err == nil {
		t.Fatal("fight snapshot ownership")
	}
	// This original field mode validates the accepted original client terminal
	// shape. It is a finite protocol fixture and does not claim native replay.
	party := proto.Clone(accepted.GetBattlePlayer()).(*pb.BattlePlayer)
	var totalHP float64
	for _, hero := range party.GetFormation() {
		totalHP += hero.GetInst().GetCurHp()
	}
	finish := &pb.CSSidePveFightEnd{FightId: accepted.FightId, Result: proto.Int32(1), BattlePlayer: party, OptRecord: &pb.BattleOperate{FinalRoundNum: proto.Int32(1), AttackerTotalLeftHp: proto.Float64(totalHP), DefenderTotalLeftHp: proto.Float64(0)}}
	beforeWin, _ := f.profile(t)
	ends, err := runtime.fieldFightEnd(f.ctx, f.id, finish)
	if err != nil {
		t.Fatal("actual field accepted settlement", err)
	}
	won := ends[0].(*pb.SCSidePveFightEnd)
	afterWin, _ := f.profile(t)
	var coinGain int64
	for _, item := range won.GetGetn().GetSimps() {
		if item.GetId() == "coin" {
			coinGain += int64(item.GetNum())
		}
	}
	if coinGain >= 999999 || afterWin.GetCoin()-beforeWin.GetCoin() != coinGain {
		t.Fatal("field config edited accepted reward", coinGain)
	}
	beforeRetry := afterWin.GetCoin()
	if _, err = runtime.fieldFightEnd(f.ctx, f.id, finish); err != nil {
		t.Fatal("field settlement retry", err)
	}
	afterRetry, _ := f.profile(t)
	if afterRetry.GetCoin() != beforeRetry {
		t.Fatal("field retry regranted")
	}
	// Exercise the actual eligibility function on the newly allowed forest,
	// preserving original level25 and safe-zone rules with a finite out-of-safe point.
	pkPlayer, pkState := f.profile(t)
	pkState.PositionLoading = false
	pkState.Position.Current = &pb.PointNew{X: proto.Float32(30), Y: proto.Float32(0), Z: proto.Float32(30)}
	code, err := runtime.p12FieldPVPQualify(db.GetIns(), pkPlayer, pkState, time.Now())
	if err != nil || code != 1 {
		t.Fatal("published forest PK allow flag ineffective", code, err)
	}
	// A zero-quantity mine roll is an accepted empty result, consumes one use and
	// original fatigue, and retries return the same receipt without another debit.
	zeroCfg := reopened.Configuration
	raw, _ = json.Marshal(zeroCfg)
	json.Unmarshal(raw, &zeroCfg)
	for i := range zeroCfg.GatherGroups {
		if zeroCfg.GatherGroups[i].ID == mineID {
			zeroCfg.GatherGroups[i].Drops[0].Quantities = []model.AdminFieldQuantity{{Amount: 0, Weight: 1}}
		}
	}
	zeroSource, err := s.AdminSaveField(f.ctx, actor, model.AdminFieldSaveRequest{ID: reopened.ID, Name: reopened.Name, Version: reopened.Version, Configuration: zeroCfg})
	if err != nil {
		t.Fatal(err)
	}
	currentDeployments, err := s.AdminFieldDeployments(f.ctx, actor, 1)
	if err != nil {
		t.Fatal(err)
	}
	for _, v := range currentDeployments.Items {
		if v.Scene == cfg.Scene {
			deployed = v
		}
	}
	if _, err = s.AdminDeployField(f.ctx, actor, model.AdminFieldDeployRequest{ServerID: 1, Scene: cfg.Scene, TemplateID: zeroSource.ID, TemplateVersion: zeroSource.Version, Version: deployed.Version, Enabled: true}); err != nil {
		t.Fatal(err)
	}
	tables, err = s.AdminFieldRuntimeTables(f.ctx, 1)
	if err != nil {
		t.Fatal(err)
	}
	runtime = &GameService{Tables: tables, P8GatherReady: true}
	if err = updateProfileWithTx(f.ctx, f.id, func(tx *gorm.DB, p *pb.Player, g *model.GameState) error {
		_, err := runtime.RefreshP8Resources(tx, p, g, time.Now(), true)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	var zeroNode db.P8ResourceNode
	if err = db.GetIns().First(&zeroNode, "player_id=? AND scene=?", f.id, cfg.Scene).Error; err != nil {
		t.Fatal(err)
	}
	var zeroConfig p8GatherConfig
	json.Unmarshal([]byte(zeroNode.Snapshot), &zeroConfig)
	if err = updateProfile(f.ctx, f.id, func(_ *pb.Player, g *model.GameState) error {
		g.Position.Current = proto.Clone(zeroConfig.Point).(*pb.PointNew)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if _, err = runtime.p8StartGather(f.ctx, f.id, &pb.CSFieldResourceStartGather{PosId: proto.String(zeroNode.PosID)}, 991011); err != nil {
		t.Fatal(err)
	}
	if err = db.GetIns().Model(&db.P8GatherAttempt{}).Where("player_id=?", f.id).Update("ready_at", time.Now().Add(-time.Second).UnixMilli()).Error; err != nil {
		t.Fatal(err)
	}
	_, zeroBefore := f.profile(t)
	zeroResult, err := runtime.p8ClaimGather(f.ctx, f.id, &pb.CSFieldGetResource{}, 991012)
	if err != nil {
		t.Fatal("legal zero-mine claim", err)
	}
	_, zeroAfter := f.profile(t)
	if zeroAfter.Field.Side.GetTiredness() != zeroBefore.Field.Side.GetTiredness()-zeroConfig.Fatigue {
		t.Fatal("zero mine did not consume original fatigue")
	}
	if _, err = runtime.p8ClaimGather(f.ctx, f.id, &pb.CSFieldGetResource{}, 991012); err != nil {
		t.Fatal(err)
	}
	_, zeroAgain := f.profile(t)
	if zeroAgain.Field.Side.GetTiredness() != zeroAfter.Field.Side.GetTiredness() {
		t.Fatal("zero mine retry consumed twice")
	}
	_ = zeroResult
	// Two distinct original experience-material drops must create distinct GUIDs.
	batchTables := make(map[string][]map[string]string, len(s.Tables))
	for k, v := range s.Tables {
		batchTables[k] = v
	}
	batchTables["FieldMonster"] = []map[string]string{{"MonId": "isolated-batch", "TeamLeader": "1", "rewardGroupId": "hero_exp_001:1,1|hero_exp_002:1,1"}}
	batchService := &GameService{Tables: batchTables}
	batchPlayer := &pb.Player{PlayerId: proto.Int64(f.id), WarriorBagSize: proto.Int32(100)}
	batchState := &model.GameState{Field: &model.FieldState{Fight: &model.FieldFight{Start: &pb.SCSidePveFightStart{FightId: proto.String("isolated-batch-materials")}}}}
	batchGain, err := batchService.fieldReward(f.id, batchPlayer, batchState, &pb.SideMonster{MonId: proto.String("isolated-batch")})
	if err != nil || len(batchGain.GetWars()) != 2 || batchGain.Wars[0].GetGuid() == batchGain.Wars[1].GetGuid() {
		t.Fatal("multi-material GUID conflict", err)
	}
	// Original boss policy remains untouched; new-cycle definition gets configured
	// resources while an existing definition snapshot stays byte-identical.
	def := p12WorldBossDefinition{Tables: map[string][]map[string]string{"WorldBossPos": s.Tables["WorldBossPos"], "WorldBossReward": s.Tables["WorldBossReward"]}}
	policy := db.P12WorldBossPolicy{Enabled: true, PositionID: 1}
	originalBoss, _ := json.Marshal(def)
	snowCfg := snow.Configuration
	snowCfg.BossRewards[0].Drops = []model.AdminFieldDrop{{ResourceID: "coin", Quantities: []model.AdminFieldQuantity{{Amount: 321, Weight: 1}}}}
	snowSaved, err := s.AdminSaveField(f.ctx, actor, model.AdminFieldSaveRequest{Name: "隔离雪原首领", Configuration: snowCfg})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.AdminDeployField(f.ctx, actor, model.AdminFieldDeployRequest{ServerID: 1, Scene: snowCfg.Scene, TemplateID: snowSaved.ID, TemplateVersion: snowSaved.Version, Enabled: true}); err != nil {
		t.Fatal(err)
	}
	tables, err = s.AdminFieldRuntimeTables(f.ctx, 1)
	if err != nil {
		t.Fatal(err)
	}
	runtime = &GameService{Tables: tables}
	if err = runtime.adminFieldBossConfiguration(&policy, &def); err != nil {
		t.Fatal(err)
	}
	if def.Tables["WorldBossReward"][0]["reward"] != "coin,321" || !strings.Contains(policy.DefinitionJSON, "321") {
		t.Fatal("boss configured reward not consumed")
	}
	originalAfter, _ := json.Marshal(p12WorldBossDefinition{Tables: map[string][]map[string]string{"WorldBossPos": s.Tables["WorldBossPos"], "WorldBossReward": s.Tables["WorldBossReward"]}})
	if !bytes.Equal(originalBoss, originalAfter) {
		t.Fatal("original boss source mutated")
	}
	// Save and rebuild runtime after a cold DB open; deployment and ownership stay.
	f.restart(t)
	cold, err := s.AdminFieldRuntimeTables(f.ctx, 1)
	if err != nil || cold["Field"][0]["MineMax"] != "1" {
		t.Fatal("cold deployment", err)
	}
	var audits int64
	db.GetIns().Model(&db.AdminAudit{}).Where("action LIKE ?", "field.%").Count(&audits)
	if audits < 5 {
		t.Fatal("operations missing audits", audits)
	}
	if err = db.GetIns().Model(&user).Update("role", constUtil.AdminRoleViewer).Error; err != nil {
		t.Fatal(err)
	}
	if _, err = s.AdminSaveField(f.ctx, actor, model.AdminFieldSaveRequest{Name: "viewer forbidden", Configuration: cfg}); !errors.Is(err, constUtil.ErrAdminPermission) {
		t.Fatal("viewer wrote map", err)
	}
}
