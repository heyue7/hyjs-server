package service

import (
	"context"
	"encoding/json"
	"math"
	"sync"
	"testing"

	"google.golang.org/protobuf/proto"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

// TestP5OriginalWantedOrder checks the original paid counter and atomic box use, without a bounty winner.
func TestP5OriginalWantedOrder(t *testing.T) {
	f := p1Setup(t)
	if err := f.s.LoadP5Tables("../../analysis/tables"); err != nil {
		t.Fatal(err)
	}
	for _, initialize := range []func(context.Context) error{f.s.InitializeP5, f.s.InitializeShop} {
		if err := initialize(f.ctx); err != nil {
			t.Fatal(err)
		}
	}
	var product db.ShopProduct
	if err := db.GetIns().First(&product, "id=?", "itemShopId_12").Error; err != nil || product.ResourceID != constUtil.P5WantedItemID || product.Cost != "diamond,200" || product.DailyLimit != 10 || product.Source != constUtil.P5WantedShopSource {
		t.Fatal("original fixed slot/price missing", err, product)
	}
	if err := updateProfile(f.ctx, f.id, func(p *pb.Player, g *model.GameState) error {
		// This unit fixture deliberately begins at the natural recovery cap; the paid item adds above it.
		p.WantedAttTimes, p.WantedAttTime = proto.Int32(10), proto.Int64(123)
		g.Items = append(g.Items, &pb.SimpleItem{Id: proto.String(constUtil.P5WantedItemID), Num: proto.Int32(3)})
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	request := &pb.CSItemUse{CostItem: &pb.BaseResource{Id: proto.String(constUtil.P5WantedItemID), Num: proto.Int32(2)}}
	var (
		wg       sync.WaitGroup
		failures = make(chan error, 8)
	)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); _, _, err := f.s.DispatchP5(f.ctx, f.id, request, 100); failures <- err }()
	}
	wg.Wait()
	close(failures)
	for err := range failures {
		if err != nil {
			t.Fatal(err)
		}
	}
	f.restart(t)
	response, handled, err := f.s.DispatchP5(f.ctx, f.id, request, 100)
	if err != nil || !handled {
		t.Fatal("original use cold recovery", err)
	}
	used := response[0].(*pb.SCItemUse)
	var added int32
	for _, gain := range used.GetGet().GetSimps() {
		if gain.GetId() != "wantedAttTimes" {
			t.Fatal("counter returned as incorrect inventory resource")
		}
		added += gain.GetNum()
	}
	p, g := f.profile(t)
	if added != 2 || p.GetWantedAttTimes() != 12 || p.GetWantedAttTime() != 123 || simpleOwned(p, g, constUtil.P5WantedItemID) != 1 || p1Count(t, &db.P5MutationReceipt{}, "player_id=?", f.id) != 1 {
		t.Fatal("original paid attempts clipped/duplicated or recovery clock changed")
	}
	if err = updateProfile(f.ctx, f.id, func(p *pb.Player, _ *model.GameState) error {
		p.WantedAttTimes = proto.Int32(math.MaxInt32)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	p, g = f.profile(t)
	before, _ := json.Marshal([]any{p, g})
	if _, _, err = f.s.DispatchP5(f.ctx, f.id, &pb.CSItemUse{CostItem: &pb.BaseResource{Id: proto.String(constUtil.P5WantedItemID), Num: proto.Int32(1)}}, 101); err == nil {
		t.Fatal("overflow accepted")
	}
	p, g = f.profile(t)
	after, _ := json.Marshal([]any{p, g})
	if string(before) != string(after) {
		t.Fatal("overflow consumed item/counter")
	}
	// Restart seeding preserves an operator's existing catalog edit.
	if err = db.GetIns().Model(&product).Updates(map[string]any{"enabled": false, "source": "operator-choice"}).Error; err != nil {
		t.Fatal(err)
	}
	if err = f.s.InitializeShop(f.ctx); err != nil {
		t.Fatal(err)
	}
	if err = db.GetIns().First(&product, "id=?", "itemShopId_12").Error; err != nil || product.Enabled || product.Source != "operator-choice" {
		t.Fatal("existing catalog overwritten", err)
	}
}
