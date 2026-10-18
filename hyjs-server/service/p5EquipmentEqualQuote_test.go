package service

import (
	"context"
	"path/filepath"
	"testing"

	"google.golang.org/protobuf/proto"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

// The two actual red rows179/180 both cost502446. Price equality cannot identify
// a replay, so the original timestamp must freeze exactly one paid mutation.
func TestP5OriginalEquipmentEqualQuoteReceipt(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "equal-quote.db")
	if e := db.InitGame(ctx, path); e != nil {
		t.Fatal(e)
	}
	defer func() { pool, _ := db.GetIns().DB(); pool.Close() }()
	s := &GameService{}
	for _, load := range []func(string) error{s.LoadTables, s.LoadP5Tables} {
		if e := load("../../analysis/tables"); e != nil {
			t.Fatal(e)
		}
	}
	for _, init := range []func(context.Context) error{s.InitializeP1, s.InitializeP5, s.InitializeWarriorGrowth, s.InitializeGameplayPools, s.InitializeP2, s.InitializeOpenServerActivity, s.InitializeDailyTasks, s.InitEquipmentMutations} {
		if e := init(ctx); e != nil {
			t.Fatal(e)
		}
	}
	s.P5Ready = true
	account, e := LoginAccount(ctx, "equipment-equal-original-quote", "isolated-test-only")
	if e != nil {
		t.Fatal(e)
	}
	if _, e = CreatePlayer(ctx, account.Id, "原同价强化回执", "hero_043"); e != nil {
		t.Fatal(e)
	}
	var guid string
	if e = updateProfile(ctx, account.Id, func(p *pb.Player, g *model.GameState) error {
		p.Level = proto.Int32(81)
		p.Coin = proto.Int64(1000000000)
		p.Diamond = proto.Int64(10000)
		p.EquipBagSize = proto.Int32(1000)
		gain, e := s.GrantFixedReward(p, g, "equip_037,1")
		if e == nil {
			guid = gain.GetEquips()[0].GetGuid()
		}
		return e
	}); e != nil {
		t.Fatal(e)
	}
	requestCtx := func(b proto.Message) context.Context {
		return context.WithValue(context.WithValue(ctx, constUtil.GameServiceContextKey, s), constUtil.GameRequestContextKey, b)
	}
	key := &pb.CSEquipStrengthenOneKey{Guid: proto.String(guid)}
	if _, e = s.p5StrengthenOne(requestCtx(key), account.Id, key, 100); e != nil {
		t.Fatal(e)
	}
	var before db.Profile
	if e = db.GetIns().First(&before, account.Id).Error; e != nil {
		t.Fatal(e)
	}
	p, g, e := decodeProfile(&before)
	if e != nil {
		t.Fatal(e)
	}
	gear, e := s.equipmentRecord(g, guid)
	if e != nil || gear.GetLevel() != 179 {
		t.Fatal("actual zero→179 original player81 cap", e, gear.GetLevel())
	}
	if e = updateProfile(ctx, account.Id, func(p *pb.Player, g *model.GameState) error { p.Level = proto.Int32(100); return nil }); e != nil {
		t.Fatal(e)
	}
	request := &pb.CSEquipStrengthen{Guid: proto.String(guid), Times: proto.Int32(1), Cost: []*pb.BaseResource{{Id: proto.String("coin"), Num: proto.Int32(502446)}}}
	var replies [8]proto.Message
	var failures [8]error
	for i := range replies {
		out, e := s.equipmentStrengthenStamped(requestCtx(request), account.Id, request, 101)
		failures[i] = e
		if e == nil {
			replies[i] = out[0]
		}
	}

	for i, r := range replies {
		if failures[i] != nil || !proto.Equal(r, replies[0]) || r.(*pb.SCEquipStrengthen).GetEquip().GetLevel() != 180 {
			t.Fatal("eight repeated original same stamp paid more than once", i, failures[i])
		}
	}
	var after db.Profile
	if e = db.GetIns().First(&after, account.Id).Error; e != nil {
		t.Fatal(e)
	}
	paid, _, e := decodeProfile(&after)
	if e != nil || p.GetCoin()-paid.GetCoin() != 502446 {
		t.Fatal("real same quote total fees", e)
	}
	out, e := s.equipmentStrengthenStamped(requestCtx(request), account.Id, request, 102)
	if e != nil || out[0].(*pb.SCEquipStrengthen).GetEquip().GetLevel() != 181 {
		t.Fatal("legitimate next same-price stage with new frame stamp", e)
	}
	if _, e = s.equipmentStrengthenStamped(requestCtx(request), account.Id, request, 101); e == nil {
		t.Fatal("old180 full client projection accepted after181")
	}
	pool, _ := db.GetIns().DB()
	pool.Close()
	if e = db.InitGame(ctx, path); e != nil {
		t.Fatal(e)
	}
	replay, e := s.equipmentStrengthenStamped(requestCtx(request), account.Id, request, 102)
	if e != nil || !proto.Equal(replay[0], out[0]) {
		t.Fatal("cold current181 receipt", e)
	}
	reset := &pb.CSEquipReBuild{Guid: proto.String(guid)}
	if _, e = s.p5EquipmentReset(requestCtx(reset), account.Id, reset, 200); e != nil {
		t.Fatal(e)
	}
	if _, e = s.equipmentStrengthenStamped(requestCtx(request), account.Id, request, 102); e == nil {
		t.Fatal("old ordinary181 receipt accepted after reset")
	}
}
