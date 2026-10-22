package service

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"google.golang.org/protobuf/proto"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

func TestAdminResourceBatch48AtomicReplayAndCold(t *testing.T) {
	f := p1Setup(t)
	if err := AdminInitialize(f.ctx, ""); err != nil {
		t.Fatal(err)
	}
	if err := InitializeAdminOperations(f.ctx); err != nil {
		t.Fatal(err)
	}
	user := db.AdminUser{Username: "batch48", PasswordHash: "fixture", Role: constUtil.AdminRoleOperator, Enabled: true}
	if err := db.GetIns().Create(&user).Error; err != nil {
		t.Fatal(err)
	}
	actor := &model.AdminPrincipal{UserID: user.ID, Username: user.Username, Role: user.Role}
	ctx := context.WithValue(f.ctx, constUtil.AdminServerScopeContextKey, int64(1))
	if err := updateProfile(ctx, f.id, func(p *pb.Player, g *model.GameState) error { p.WarriorBagSize = proto.Int32(2000); return nil }); err != nil {
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
	before := read()
	req := model.AdminInventoryBatchRequest{Version: before.Version, RequestID: uuid.NewString(), Reason: "isolated batch verification", Resources: []model.AdminInventoryBatchResource{{Category: "warrior", ResourceID: "hero_exp_004", Amount: 100}, {Category: "item", ResourceID: "item_005", Amount: 3}}}
	if _, err := f.s.AdminInventoryBatch(ctx, actor, f.id, req); err != nil {
		t.Fatal("100 crystals and material", err)
	}
	p, g := f.profile(t)
	crystals := 0
	for _, w := range g.Warriors {
		if w.GetId() == "hero_exp_004" {
			crystals++
		}
	}
	if crystals != 100 || simpleOwned(p, g, "item_005") != 3 {
		t.Fatal("batch amounts", crystals, simpleOwned(p, g, "item_005"))
	}
	current := read()
	if _, err := f.s.AdminInventoryBatch(ctx, actor, f.id, req); err != nil {
		t.Fatal("retry", err)
	}
	if read().Version != current.Version {
		t.Fatal("retry changed inventory")
	}
	changed := req
	changed.Resources = append([]model.AdminInventoryBatchResource(nil), req.Resources...)
	changed.Resources[1].Amount++
	if _, err := f.s.AdminInventoryBatch(ctx, actor, f.id, changed); !errors.Is(err, constUtil.ErrAdminConfigConflict) {
		t.Fatal("batch body reuse", err)
	}
	invalid := req
	invalid.Version = current.Version
	invalid.RequestID = uuid.NewString()
	invalid.Resources = []model.AdminInventoryBatchResource{{Category: "item", ResourceID: "item_005", Amount: 5}, {Category: "item", ResourceID: "missing", Amount: 1}}
	if _, err := f.s.AdminInventoryBatch(ctx, actor, f.id, invalid); err == nil {
		t.Fatal("invalid final resource accepted")
	}
	if read().Version != current.Version {
		t.Fatal("partial batch survived rollback")
	}
	stale := req
	stale.RequestID = uuid.NewString()
	if _, err := f.s.AdminInventoryBatch(ctx, actor, f.id, stale); !errors.Is(err, constUtil.ErrAdminConfigConflict) {
		t.Fatal("stale batch", err)
	}
	wrongScope := context.WithValue(ctx, constUtil.AdminServerScopeContextKey, int64(2))
	if _, err := f.s.AdminInventoryBatch(wrongScope, actor, f.id, req); !errors.Is(err, constUtil.ErrAdminPermission) {
		t.Fatal("scope replay", err)
	}
	grant := model.AdminBatchGrantRequest{PlayerID: f.id, RequestID: uuid.NewString(), Reason: "isolated mixed attachment", Resources: []model.AdminResourceAmount{{ResourceID: "hero_exp_004", Amount: 100}, {ResourceID: "item_005", Amount: 4}}}
	delivery, err := f.s.AdminBatchGrant(ctx, actor, grant)
	if err != nil {
		t.Fatal("grant", err)
	}
	again, err := f.s.AdminBatchGrant(ctx, actor, grant)
	if err != nil || again.DeliveryID != delivery.DeliveryID {
		t.Fatal("grant retry", err)
	}
	var saved db.RewardDelivery
	if err := db.GetIns().First(&saved, delivery.DeliveryID).Error; err != nil || saved.Rewards != "hero_exp_004,100;item_005,4" {
		t.Fatal("frozen resources", err)
	}
	f.restart(t)
	if _, err := f.s.AdminInventoryBatch(ctx, actor, f.id, req); err != nil {
		t.Fatal("cold retry", err)
	}
	if read().Version != current.Version {
		t.Fatal("cold retry changed inventory")
	}
	if _, err := f.s.RewardCenter(ctx, f.id, &pb.CSAddRewardsToPlayer{RewardIds: []int64{delivery.DeliveryID}}); err != nil {
		t.Fatal("claim mixed", err)
	}
	p, g = f.profile(t)
	crystals = 0
	for _, w := range g.Warriors {
		if w.GetId() == "hero_exp_004" {
			crystals++
		}
	}
	if crystals != 200 || simpleOwned(p, g, "item_005") != 7 {
		t.Fatal("actual claim", crystals, simpleOwned(p, g, "item_005"))
	}
	if _, err := f.s.RewardCenter(ctx, f.id, &pb.CSAddRewardsToPlayer{RewardIds: []int64{delivery.DeliveryID}}); err != nil {
		t.Fatal("claim retry", err)
	}
	p, g = f.profile(t)
	if simpleOwned(p, g, "item_005") != 7 {
		t.Fatal("duplicate claim")
	}

	// Batch children use an internal namespace that the single HTTP endpoint cannot supply.
	collision := model.AdminInventoryBatchRequest{RequestID: uuid.NewString(), Reason: "isolated namespace check", Resources: []model.AdminInventoryBatchResource{{Category: "item", ResourceID: "item_005", Amount: 1}}}
	one := int64(1)
	legacy := model.AdminPlayerInventoryRequest{Version: read().Version, RequestID: uuid.NewSHA1(uuid.MustParse(collision.RequestID), []byte("0")).String(), Operation: "add", Category: "item", ResourceID: "item_005", Amount: &one, Reason: collision.Reason}
	if _, err := f.s.AdminUpdatePlayerInventory(ctx, actor, f.id, legacy); err != nil {
		t.Fatal("single namespace", err)
	}
	collision.Version = read().Version
	if _, err := f.s.AdminInventoryBatch(ctx, actor, f.id, collision); err != nil {
		t.Fatal("batch namespace", err)
	}
	p, g = f.profile(t)
	if simpleOwned(p, g, "item_005") != 9 {
		t.Fatal("batch child collided with single receipt")
	}

}

func TestPointShop48RealProjectionKeepsOffer(t *testing.T) {
	f := p4Setup(t)
	product := p4BaseProduct("red-sword", "equip_037,1", "item_021,10", 2, 22)
	product.Priority = 2001
	rules := p4CommerceRules{Products: []p4Product{product}}
	cfg := db.P4CommerceRule{ID: "test48", Kind: "point", ShopType: 5}
	p, _ := f.profile(t)
	p.Level = proto.Int32(100)
	state := &p4CommerceProgress{Bought: map[string]int32{}, Total: map[string]int32{}}
	items, _, err := f.s.p4Projection(db.GetIns(), p, cfg, rules, state)
	if err != nil || len(items) != 1 {
		t.Fatal("projection", err)
	}
	if items[0].GetPriority() != 4001 || items[0].GetPrice()[0].GetId() != "item_021" || items[0].GetPrice()[0].GetNum() != 10 || rules.Products[0].Priority != 2001 {
		t.Fatal("red tab or offer changed", items)
	}
	for _, tc := range []struct {
		id       string
		priority int32
		want     int32
	}{{"item_021", 2001, 1001}, {"equip_037", 1001, 4001}} {
		if got := f.s.pointShop48Priority(tc.priority, []*pb.BaseResource{{Id: proto.String(tc.id)}}); got != tc.want {
			t.Fatal(tc.id, got)
		}
	}
}
