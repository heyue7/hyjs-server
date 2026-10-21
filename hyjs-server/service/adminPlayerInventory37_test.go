package service

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/google/uuid"
	"google.golang.org/protobuf/proto"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
	"strconv"
	"testing"
)

func TestAdminInventory37ActualCRUDReplayColdGuards(t *testing.T) {
	f := p1Setup(t)
	for _, load := range []func(string) error{f.s.LoadP5Tables, f.s.LoadP8Tables, f.s.LoadP9Tables} {
		if err := load("../../analysis/tables"); err != nil {
			t.Fatal(err)
		}
	}
	for _, init := range []func(context.Context) error{func(ctx context.Context) error { return AdminInitialize(ctx, "") }, f.s.InitializeGlobalGameAccounts, f.s.InitializeP9Pets, f.s.InitializeP9PetNext, f.s.InitializeP9PetIntimacy} {
		if err := init(f.ctx); err != nil {
			t.Fatal(err)
		}
	}
	if err := db.GetIns().AutoMigrate(&db.P9PetRetired{}); err != nil {
		t.Fatal(err)
	}
	f.s.P9PetsReady = true
	user := db.AdminUser{Username: "inventory37", PasswordHash: "fixture", Role: constUtil.AdminRoleOperator, Enabled: true}
	if err := db.GetIns().Create(&user).Error; err != nil {
		t.Fatal(err)
	}
	actor := &model.AdminPrincipal{UserID: user.ID, Username: user.Username, Role: user.Role}
	ctx := context.WithValue(f.ctx, constUtil.AdminServerScopeContextKey, int64(1))
	if err := updateProfile(f.ctx, f.id, func(p *pb.Player, g *model.GameState) error {
		p.Level = proto.Int32(75)
		p.PetBagSize = proto.Int32(100)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	read := func() *model.AdminPlayerInventoryResponse {
		t.Helper()
		v, e := f.s.AdminPlayerInventory(ctx, f.id, "", "", nil, 1, 100)
		if e != nil {
			t.Fatal(e)
		}
		return v
	}
	mutate := func(req model.AdminPlayerInventoryRequest) *model.AdminPlayerInventoryMutationResponse {
		t.Helper()
		if req.Version == "" {
			req.Version = read().Version
		}
		if req.RequestID == "" {
			req.RequestID = uuid.NewString()
		}
		req.Reason = "isolated backend inventory verification"
		out, e := f.s.AdminUpdatePlayerInventory(ctx, actor, f.id, req)
		if e != nil {
			t.Fatal(req.Operation, req.Category, e)
		}
		if out.Version != read().Version {
			t.Fatal("returned version differs")
		}
		return out
	}
	amount := int64(2)
	initial := read()
	req := model.AdminPlayerInventoryRequest{Version: initial.Version, RequestID: uuid.NewString(), Operation: "add", Category: "item", ResourceID: "item_005", Amount: &amount, Reason: "isolated backend inventory verification"}
	first := mutate(req)
	if first.Version == initial.Version {
		t.Fatal("version unchanged")
	}
	mutate(req)
	if p1Count(t, &db.AdminAudit{}, "action=?", "player.inventory.add") != 1 {
		t.Fatal("replay added audit")
	}
	changed := req
	three := int64(3)
	changed.Amount = &three
	if _, err := f.s.AdminUpdatePlayerInventory(ctx, actor, f.id, changed); err == nil {
		t.Fatal("UUID body changed")
	}
	if _, err := f.s.AdminUpdatePlayerInventory(context.WithValue(ctx, constUtil.AdminServerScopeContextKey, int64(2)), actor, f.id, req); !errors.Is(err, constUtil.ErrAdminPermission) {
		t.Fatal("scope replay bypass", err)
	}
	for _, entry := range []struct{ category, id string }{{"warrior", "hero_001"}, {"warrior", "hero_exp_001"}, {"equipment", "equip_037"}, {"accessory", "trea_001"}, {"gem", "MineItem_019"}, {"cooking", "FishItem_007"}, {"pet", "pet_002"}} {
		mutate(model.AdminPlayerInventoryRequest{Operation: "add", Category: entry.category, ResourceID: entry.id, Amount: &amount})
	}
	current := read()
	if len(current.Categories) != 7 {
		t.Fatal("missing categories")
	}
	var warrior, pet, equip, main model.AdminInventoryRow
	p, _ := f.profile(t)
	for _, r := range current.Items {
		if r.GUID == p.GetHeroWarId() {
			main = r
		}
		if r.ResourceID == "hero_001" {
			warrior = r
		}
		if r.Category == "pet" {
			pet = r
		}
		if r.Category == "equipment" {
			equip = r
		}
	}
	stale := req
	stale.RequestID = uuid.NewString()
	if _, err := f.s.AdminUpdatePlayerInventory(ctx, actor, f.id, stale); !errors.Is(err, constUtil.ErrAdminConfigConflict) {
		t.Fatal("stale version accepted", err)
	}
	mutate(model.AdminPlayerInventoryRequest{Operation: "update", Category: "warrior", Key: warrior.Key, Attributes: map[string]int64{"level": 2}})
	mutate(model.AdminPlayerInventoryRequest{Operation: "update", Category: "equipment", Key: equip.Key, Attributes: map[string]int64{"level": 2}})
	if err := updateProfile(f.ctx, f.id, func(p *pb.Player, g *model.GameState) error {
		p.Coin = proto.Int64(1000000)
		_, err := f.s.GrantFixedReward(p, g, "item_126,5")
		return err
	}); err != nil {
		t.Fatal(err)
	}
	petGrow := func(stamp int64) {
		t.Helper()
		request := &pb.CSPetLevelup{Guid: proto.String(pet.Key), Cost: []*pb.BaseResource{{Id: proto.String("item_126"), Num: proto.Int32(1)}}}
		requestCtx := context.WithValue(context.WithValue(f.ctx, constUtil.GameServiceContextKey, f.s), constUtil.GameRequestContextKey, request)
		if _, err := f.s.p9PetLevelup(requestCtx, f.id, request, stamp); err != nil {
			t.Fatal(err)
		}
	}
	petGrow(200)
	var petPaid db.P9PetInvestment
	if err := db.GetIns().First(&petPaid, "guid=?", pet.Key).Error; err != nil {
		t.Fatal(err)
	}
	mutate(model.AdminPlayerInventoryRequest{Operation: "update", Category: "pet", Key: pet.Key, Attributes: map[string]int64{"level": 2, "exp": 0}})
	petGrow(201)
	var petContinued db.P9PetInvestment
	if err := db.GetIns().First(&petContinued, "guid=?", pet.Key).Error; err != nil {
		t.Fatal(err)
	}
	var paidCosts, continuedCosts map[string]int64
	if json.Unmarshal(petPaid.Costs, &paidCosts) != nil || json.Unmarshal(petContinued.Costs, &continuedCosts) != nil || continuedCosts["item_126"] != paidCosts["item_126"]+1 {
		t.Fatal("pet paid growth history did not continue")
	}
	five := int64(5)
	mutate(model.AdminPlayerInventoryRequest{Operation: "update", Category: "item", Key: "item_005", Amount: &five})
	if err := updateProfile(f.ctx, f.id, func(p *pb.Player, g *model.GameState) error { p.EquipBagSize = proto.Int32(10); return nil }); err != nil {
		t.Fatal(err)
	}
	beforeCapacity := read()
	tooMany := int64(100)
	capacity := model.AdminPlayerInventoryRequest{Version: beforeCapacity.Version, RequestID: uuid.NewString(), Operation: "add", Category: "equipment", ResourceID: "equip_037", Amount: &tooMany, Reason: "capacity rollback"}
	if _, err := f.s.AdminUpdatePlayerInventory(ctx, actor, f.id, capacity); err == nil {
		t.Fatal("capacity overflow accepted")
	}
	if read().Version != beforeCapacity.Version {
		t.Fatal("capacity failure changed state")
	}
	before := read()
	bad := model.AdminPlayerInventoryRequest{Version: before.Version, RequestID: uuid.NewString(), Reason: "rollback", Operation: "update", Category: "warrior", Key: warrior.Key, Attributes: map[string]int64{"level": 3, "guid": 9}}
	if _, err := f.s.AdminUpdatePlayerInventory(ctx, actor, f.id, bad); err == nil {
		t.Fatal("unknown attribute accepted")
	}
	if read().Version != before.Version {
		t.Fatal("failed mutation persisted")
	}
	bad.Operation = "remove"
	bad.Key = main.Key
	bad.Attributes = nil
	if _, err := f.s.AdminUpdatePlayerInventory(ctx, actor, f.id, bad); err == nil {
		t.Fatal("main removed")
	}
	if err := updateProfile(f.ctx, f.id, func(p *pb.Player, g *model.GameState) error {
		g.Formation = append(g.Formation, &pb.FormationItem{FormatPos: proto.Int32(6), Things: []*pb.FormationThing{{Pos: proto.Int32(1), Guid: proto.String(equip.Key)}}})
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	protected := read()
	bad.Version = protected.Version
	bad.RequestID = uuid.NewString()
	bad.Category = "equipment"
	bad.Key = equip.Key
	if _, err := f.s.AdminUpdatePlayerInventory(ctx, actor, f.id, bad); err == nil {
		t.Fatal("mounted equipment removed")
	}
	if err := updateProfile(f.ctx, f.id, func(p *pb.Player, g *model.GameState) error {
		g.Formation = g.Formation[:len(g.Formation)-1]
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	// Independent protobuf data participates in the version even when the profile row is untouched.
	beforePetVersion := read()
	native, err := p9OwnedPet(db.GetIns(), f.id, pet.Key)
	if err != nil {
		t.Fatal(err)
	}
	native.Exp = proto.Int64(1)
	data, err := proto.Marshal(native)
	if err != nil {
		t.Fatal(err)
	}
	if err = db.GetIns().Model(&db.P9PetInstance{}).Where("guid=?", pet.Key).Update("data", data).Error; err != nil {
		t.Fatal(err)
	}
	if read().Version == beforePetVersion.Version {
		t.Fatal("pet-only update missed version")
	}
	for _, r := range []model.AdminInventoryRow{warrior, pet, equip} {
		mutate(model.AdminPlayerInventoryRequest{Operation: "remove", Category: r.Category, Key: r.Key})
	}
	if p1Count(t, &db.P9PetRetired{}, "guid=?", pet.Key) != 1 || p1Count(t, &db.P9PetInstance{}, "guid=?", pet.Key) != 0 {
		t.Fatal("pet retirement lost")
	}
	saved := read()
	f.restart(t)
	if read().Version != saved.Version {
		t.Fatal("cold reload differs")
	}
	if err := db.GetIns().Model(&user).Update("role", constUtil.AdminRoleViewer).Error; err != nil {
		t.Fatal(err)
	}
	req.Version = saved.Version
	req.RequestID = uuid.NewString()
	if _, err := f.s.AdminUpdatePlayerInventory(ctx, actor, f.id, req); !errors.Is(err, constUtil.ErrAdminPermission) {
		t.Fatal("revoked role edited", err)
	}
}

// TestAdminInventory37PaidGrowthContinues verifies real costs and refund history survive administrative edits.
func TestAdminInventory37PaidGrowthContinues(t *testing.T) {
	f := p1Setup(t)
	if err := f.s.LoadP5Tables("../../analysis/tables"); err != nil {
		t.Fatal(err)
	}
	for _, initialize := range []func(context.Context) error{func(ctx context.Context) error { return AdminInitialize(ctx, "") }, f.s.InitializeGlobalGameAccounts, f.s.InitializeP5, f.s.InitializeWarriorGrowth, f.s.InitializeGameplayPools, f.s.InitializeP2, f.s.InitializeOpenServerActivity, f.s.InitializeDailyTasks, f.s.InitEquipmentMutations} {
		if err := initialize(f.ctx); err != nil {
			t.Fatal(err)
		}
	}
	f.s.P5Ready = true
	user := db.AdminUser{Username: "growth37", PasswordHash: "fixture", Role: constUtil.AdminRoleOperator, Enabled: true}
	if err := db.GetIns().Create(&user).Error; err != nil {
		t.Fatal(err)
	}
	actor := &model.AdminPrincipal{UserID: user.ID, Username: user.Username, Role: user.Role}
	var guid string
	if err := updateProfile(f.ctx, f.id, func(p *pb.Player, g *model.GameState) error {
		p.Level = proto.Int32(75)
		p.Coin = proto.Int64(10000000)
		gain, e := f.s.GrantFixedReward(p, g, "equip_037,1")
		if e == nil {
			guid = gain.GetEquips()[0].GetGuid()
		}
		return e
	}); err != nil {
		t.Fatal(err)
	}
	grow := func(stamp int64) {
		t.Helper()
		_, g := f.profile(t)
		equip, e := f.s.equipmentRecord(g, guid)
		if e != nil {
			t.Fatal(e)
		}
		quality := f.s.table("Equip", "id", equip.GetEquipId())["quality"]
		var coin int64
		for _, r := range f.s.Tables["EquipExp"] {
			if r["quality"] == quality && r["level"] == strconv.Itoa(int(equip.GetLevel())) {
				coin = int64(tableNumber(r, "coin"))
			}
		}
		request := &pb.CSEquipStrengthen{Guid: proto.String(guid), Times: proto.Int32(1), Cost: []*pb.BaseResource{{Id: proto.String("coin"), Num: proto.Int32(int32(coin))}}}
		ctx := context.WithValue(context.WithValue(f.ctx, constUtil.GameServiceContextKey, f.s), constUtil.GameRequestContextKey, request)
		if _, err := f.s.equipmentStrengthenStamped(ctx, f.id, request, stamp); err != nil {
			t.Fatal(err)
		}
	}
	grow(100)
	_, g := f.profile(t)
	var old p5Investment
	if err := json.Unmarshal([]byte(p5KV(g, "p5.invest:"+guid)), &old); err != nil || len(old.Costs) == 0 {
		t.Fatal("paid investment missing", err)
	}
	current, err := f.s.AdminPlayerInventory(f.ctx, f.id, "", "", nil, 1, 100)
	if err != nil {
		t.Fatal(err)
	}
	req := model.AdminPlayerInventoryRequest{Version: current.Version, RequestID: uuid.NewString(), Operation: "update", Category: "equipment", Key: guid, Attributes: map[string]int64{"level": 5}, Reason: "verify paid growth continuation"}
	if _, err = f.s.AdminUpdatePlayerInventory(f.ctx, actor, f.id, req); err != nil {
		t.Fatal(err)
	}
	_, g = f.profile(t)
	equip, _ := f.s.equipmentRecord(g, guid)
	state, err := p5InvestmentState(g, guid, equip)
	if err != nil || string(state.Baseline) != string(old.Baseline) || !proto.Equal(state.Costs[0], old.Costs[0]) {
		t.Fatal("administrative edit lost real history", err)
	}
	grow(101)
	_, g = f.profile(t)
	equip, _ = f.s.equipmentRecord(g, guid)
	state, err = p5InvestmentState(g, guid, equip)
	if err != nil || equip.GetLevel() != 6 || state.Costs[0].GetNum() <= old.Costs[0].GetNum() {
		t.Fatal("normal paid path did not continue", err)
	}
}

// TestAdminInventory37FullPagination verifies legacy inventory is not truncated at 2000 rows.
func TestAdminInventory37FullPagination(t *testing.T) {
	f := p1Setup(t)
	if err := updateProfile(f.ctx, f.id, func(_ *pb.Player, g *model.GameState) error {
		g.Items = []*pb.SimpleItem{}
		for i := 0; i < 2105; i++ {
			g.Items = append(g.Items, &pb.SimpleItem{Id: proto.String("legacy_" + strconv.Itoa(i)), Num: proto.Int32(1)})
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	result, err := f.s.AdminPlayerInventory(f.ctx, f.id, "item", "", nil, 22, 100)
	if err != nil || result.Total != 2105 || len(result.Items) != 5 {
		t.Fatal("truncated page", result, err)
	}
	result, err = f.s.AdminPlayerInventory(f.ctx, f.id, "item", "legacy_2104", nil, 1, 100)
	if err != nil || result.Total != 1 || len(result.Items) != 1 {
		t.Fatal("search after limit", result, err)
	}
	catalogue := f.s.AdminPlayerInventoryCatalogue("warrior", "hero_exp_001", nil, 1, 100)
	if catalogue.Total != 1 || len(catalogue.Items) != 1 || catalogue.Items[0].Category != "warrior" {
		t.Fatal("native experience card catalogue", catalogue)
	}
	if list := f.s.AdminPlayerInventoryCatalogue("warrior", "hero_043", nil, 1, 100); list.Total != 0 {
		t.Fatal("main class template addable")
	}
}
