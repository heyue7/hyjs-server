package service

import (
	"encoding/json"
	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
	"sync"
	"testing"
)

func TestP5GoldEggOriginalAll54AndOneUse(t *testing.T) {
	f, id := p3DemoProviderFixture(t)
	f.id = id
	if err := f.s.InitializeP4(f.ctx); err != nil {
		t.Fatal(err)
	}
	if err := f.s.InitializeP5(f.ctx); err != nil {
		t.Fatal(err)
	}
	if err := updateProfile(f.ctx, f.id, func(p *pb.Player, _ *model.GameState) error { p.Level = proto.Int32(100); return nil }); err != nil {
		t.Fatal(err)
	}
	fundedRechargeRewardFixture75(t, f, "original-gold-egg", "item_360,1")
	use := &pb.CSItemUse{CostItem: &pb.BaseResource{Id: proto.String("item_360"), Num: proto.Int32(1)}}
	bad := proto.Clone(use).(*pb.CSItemUse)
	bad.CostItem.Num = proto.Int32(2)
	if _, e := f.s.p5GoldEggUse(f.ctx, f.id, bad, 2); e == nil {
		t.Fatal("original gold egg one-use violated")
	}
	var wg sync.WaitGroup
	errs := make(chan error, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); _, e := f.s.p5GoldEggUse(f.ctx, f.id, use, 3); errs <- e }()
	}
	wg.Wait()
	close(errs)
	for e := range errs {
		if e != nil {
			t.Fatal(e)
		}
	}
	p, g := f.profile(t)
	if simpleOwned(p, g, "item_360") != 0 {
		t.Fatal("actual consumption")
	}
	var cfg db.P5GoldEggRule
	if e := db.GetIns().First(&cfg, "id=?", constUtil.P5GoldEggRuleID).Error; e != nil {
		t.Fatal(e)
	}
	var options []p5GoldEggOption
	if json.Unmarshal(cfg.Frozen, &options) != nil || len(options) != 54 {
		t.Fatal("54 frozen original options")
	}
	var total int64
	for _, o := range options {
		total += o.Weight
		p := &pb.Player{EquipBagSize: proto.Int32(60), WarriorBagSize: proto.Int32(60), Level: proto.Int32(100)}
		g := &model.GameState{}
		if _, e := f.s.GrantFixedReward(p, g, o.Reward); e != nil {
			t.Fatalf("original %s %s: %v", o.ID, o.Reward, e)
		}
		if o.ID == "881" && o.Reward != "MineItem_028,1" {
			t.Fatal("narrow original punctuation repair")
		}
	}
	if total != 110150 {
		t.Fatal("original relative weight total")
	}
	if e := db.GetIns().Model(&cfg).Update("rules", []byte(`[]`)).Error; e != nil {
		t.Fatal(e)
	}
	// Current frozen options remain independently valid; malformed source bytes cannot change them.
	if e := db.GetIns().Transaction(func(tx *gorm.DB) error { _, e := f.s.p5GoldEggOptions(tx); return e }); e != nil {
		t.Fatal(e)
	}
	if e := db.GetIns().Model(&cfg).Update("enabled", false).Error; e != nil {
		t.Fatal(e)
	}
	prior, e := f.s.p5GoldEggUse(f.ctx, f.id, use, 3)
	if e != nil || prior[0].(*pb.SCItemUse).GetCost().GetNum() != 1 {
		t.Fatal("closed old exact receipt", e)
	}
	if _, e = f.s.p5GoldEggUse(f.ctx, f.id, use, 4); e == nil {
		t.Fatal("operator-off use")
	}
	var enabledDirect int64
	if err := db.GetIns().Model(&db.P4CommerceRule{}).Where("id LIKE ? AND enabled=1", constUtil.P5GoldEggSupplyPrefix+"%").Count(&enabledDirect).Error; err != nil || enabledDirect != 0 {
		t.Fatal("forbidden direct gold source enabled", enabledDirect, err)
	}
	pool, _ := db.GetIns().DB()
	pool.Close()
	f.open(t)
	if e = f.s.InitializeP5(f.ctx); e != nil {
		t.Fatal(e)
	}
	again, e := f.s.p5GoldEggUse(f.ctx, f.id, use, 3)
	if e != nil || !proto.Equal(prior[0], again[0]) {
		t.Fatal("cold original item receipt", e)
	}
	t.Log("54 original awards/110150 frozen relative weights/single action/8once/cold/off/earned actual recharge source; original private param/rate/pity unknown")
}
