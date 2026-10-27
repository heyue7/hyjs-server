package service

import (
	"context"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

// Only finite level, one original book/food, position and damaged HP are fixture
// prerequisites. The original consumers produce all events/costs and receipts.
func TestP8Calendar76OriginalGatherBookFoodNegativeDayAndReplay(t *testing.T) {
	f := p1Setup(t)
	for _, load := range []func(string) error{f.s.LoadP5Tables, f.s.LoadP8Tables} {
		if err := load("../../analysis/tables"); err != nil {
			t.Fatal(err)
		}
	}
	for _, fn := range []func(context.Context) error{f.s.InitializeP5, f.s.InitializeP8Gather, f.s.InitializeP8Life, f.s.InitGems, f.s.InitCooking, f.s.InitializeLocalPlayActivities, f.s.InitializeServerGameClock} {
		if err := fn(f.ctx); err != nil {
			t.Fatal(err)
		}
	}
	if err := updateProfile(f.ctx, f.id, func(p *pb.Player, g *model.GameState) error {
		p.Level = proto.Int32(80)
		if _, err := f.s.GrantFixedReward(p, g, "item_063,1;item_068,1"); err != nil {
			return err
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	actor := adminWalletTestActor(t, f, constUtil.AdminRoleAdmin)
	ctx := guildClock75Move(t, f, actor, f.id, -1)
	if _, err := f.s.fieldTransfer(ctx, f.id, &pb.CSTransferScene{Target: proto.String("field_001")}); err != nil {
		t.Fatal(err)
	}
	nodes, err := P8GatherNodes(db.GetIns(), f.id)
	if err != nil {
		t.Fatal(err)
	}
	// Transfer materializes original resource nodes only when this real query runs.
	if len(nodes) == 0 {
		if _, err := f.s.p8ResourceQuery(ctx, f.id, &pb.CSSideResourceCheck{PosId: proto.String("mine_pos_1")}); err != nil {
			t.Fatal(err)
		}
		nodes, err = P8GatherNodes(db.GetIns(), f.id)
		if err != nil {
			t.Fatal(err)
		}
	}
	var chosen *db.P8ResourceNode
	var cfg *p8GatherConfig
	for i := range nodes {
		v, e := p8DecodeConfig(nodes[i].Snapshot)
		if e != nil {
			t.Fatal(e)
		}
		if v.Kind == "mine" && v.Level == 1 && nodes[i].Remaining > 0 {
			chosen = &nodes[i]
			cfg = v
			break
		}
	}
	if chosen == nil {
		t.Fatal("original first-map level1 node absent")
	}
	if err = updateProfile(ctx, f.id, func(p *pb.Player, g *model.GameState) error {
		g.Position.Current = proto.Clone(cfg.Point).(*pb.PointNew)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	stamp := time.Now().UnixMilli()
	if _, err = f.s.p8StartGather(ctx, f.id, &pb.CSFieldResourceStartGather{PosId: proto.String(chosen.PosID)}, stamp); err != nil {
		t.Fatal(err)
	}
	var attempt db.P8GatherAttempt
	if err = db.GetIns().First(&attempt, "player_id=?", f.id).Error; err != nil {
		t.Fatal(err)
	}
	if attempt.ReadyAt-attempt.StartedAt != cfg.GatherMS || attempt.StartedAt < time.Now().Add(-time.Minute).UnixMilli() {
		t.Fatal("gather clock is not original wall time")
	}
	if _, err = f.s.p8ClaimGather(ctx, f.id, &pb.CSFieldGetResource{}, stamp+1); err == nil {
		t.Fatal("negative date skipped actual wait")
	}
	time.Sleep(time.Until(time.UnixMilli(attempt.ReadyAt).Add(10 * time.Millisecond)))
	out, err := f.s.p8ClaimGather(ctx, f.id, &pb.CSFieldGetResource{}, stamp+2)
	if err != nil {
		t.Fatal("negative day original waited claim rejected", err)
	}
	beforeP, beforeG := p10LuckyProfile(t, f.id)
	again, err := f.s.p8ClaimGather(ctx, f.id, &pb.CSFieldGetResource{}, stamp+2)
	if err != nil || !proto.Equal(out[0], again[0]) {
		t.Fatal("gather receipt changed", err)
	}
	afterP, afterG := p10LuckyProfile(t, f.id)
	if !proto.Equal(beforeP, afterP) || beforeG.Field.Side.GetTiredness() != afterG.Field.Side.GetTiredness() {
		t.Fatal("gather replay charged twice")
	}
	book := &pb.CSItemUse{CostItem: &pb.BaseResource{Id: proto.String("item_063"), Num: proto.Int32(1)}}
	if _, err = f.s.p8SkillBook(ctx, f.id, book, stamp+3); err != nil {
		t.Fatal("negative day original book rejected", err)
	}
	if _, err = f.s.p8SkillBook(ctx, f.id, book, stamp+3); err != nil {
		t.Fatal("book repeat rejected", err)
	}
	if err = updateProfile(ctx, f.id, func(p *pb.Player, g *model.GameState) error {
		g.Position.Current = &pb.PointNew{X: proto.Float32(1000), Y: proto.Float32(0), Z: proto.Float32(1000)}
		g.Field.Side.CurHp = proto.Float64(.5)
		g.Field.Side.BattlePosHps = nil
		g.Field.Side.State = proto.Int32(0)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	food := &pb.CSHpRecover{CostItem: &pb.BaseResource{Id: proto.String("item_068"), Num: proto.Int32(1)}}
	if _, err = f.s.p8HpRecover(ctx, f.id, food, stamp+4); err != nil {
		t.Fatal("negative day original food rejected", err)
	}
	if _, err = f.s.p8HpRecover(ctx, f.id, food, stamp+4); err != nil {
		t.Fatal("food repeat rejected", err)
	}
	var facts []db.EventFact
	if err = db.GetIns().Where("player_id=? AND kind IN ?", f.id, []string{constUtil.P8EventMineGather, constUtil.P8EventLifeBook, constUtil.P8EventHpRecover}).Find(&facts).Error; err != nil || len(facts) != 3 {
		t.Fatal("original consumer facts not once", len(facts), err)
	}
	day := p1Day(LocalPlayNow(ctx))
	for _, fact := range facts {
		if fact.Day != day || fact.OccurredAt > LocalPlayNow(ctx).Add(time.Minute).UnixMilli() || fact.OccurredAt < LocalPlayNow(ctx).Add(-time.Minute).UnixMilli() {
			t.Fatal("event used wall/double-offset date", fact.Day, day)
		}
	}
	var life db.P8LifeState
	if err = db.GetIns().First(&life, "player_id=?", f.id).Error; err != nil {
		t.Fatal(err)
	}
	if life.FoodReadyAt < time.Now().UnixMilli() || life.FoodReadyAt > time.Now().Add(time.Hour).UnixMilli() {
		t.Fatal("food cooldown stopped using real clock", life.FoodReadyAt)
	}
}
