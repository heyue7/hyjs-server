package service

import (
	"encoding/json"
	"google.golang.org/protobuf/encoding/protowire"
	"google.golang.org/protobuf/proto"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
	"testing"
	"time"
)

func catQuote63(t *testing.T, info *pb.ActivityFortuneCatData) uint64 {
	t.Helper()
	raw := info.ProtoReflect().GetUnknown()
	num, kind, n := protowire.ConsumeTag(raw)
	if num != 100 || kind != protowire.VarintType || n < 0 {
		t.Fatal("missing scalar quote", raw)
	}
	value, m := protowire.ConsumeVarint(raw[n:])
	if m < 0 || n+m != len(raw) {
		t.Fatal("invalid quote wire")
	}
	return value
}
func TestTreasure63InfoQuoteFreezeAndReplay(t *testing.T) {
	f, actor := adminModesFixture(t)
	at := time.Now()
	_, key := reWelfarePublish(t, f, actor, "fortune-cat", at)
	if err := updateProfile(f.ctx, f.id, func(p *pb.Player, _ *model.GameState) error {
		p.Diamond = proto.Int64(10000)
		p.Coin = proto.Int64(0)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	info, err := f.s.p4Welfare(f.ctx, f.id, &pb.CSFortuneCatInfo{}, 0, at)
	if err != nil {
		t.Fatal(err)
	}
	if catQuote63(t, info[0].(*pb.SCFortuneCatInfo).GetInfo()) != 105 {
		t.Fatal("initial quote")
	}
	var cfg db.P4WelfareRule
	if err := db.GetIns().First(&cfg, "id=?", key).Error; err != nil {
		t.Fatal(err)
	}
	var rules reOperationsWelfareRules
	if err := json.Unmarshal(cfg.Rules, &rules); err != nil {
		t.Fatal(err)
	}
	rules.Tiers[0].Reward = "coin,777"
	rules.Tiers[1].Reward = "coin,888"
	rules.Tiers[1].Cost = 301
	if err := db.GetIns().Model(&cfg).Update("rules", adminJSON(rules)).Error; err != nil {
		t.Fatal(err)
	}
	req := &pb.CSFortuneCatReward{Type: proto.Int32(0)}
	first, err := f.s.p4Welfare(f.ctx, f.id, req, 63001, at)
	if err != nil {
		t.Fatal(err)
	}
	if first[0].(*pb.SCFortuneCatReward).GetGet().Simps[0].GetNum() != 105 {
		t.Fatal("background edit silently replaced visible quote")
	}
	info, err = f.s.p4Welfare(f.ctx, f.id, &pb.CSFortuneCatInfo{}, 0, at)
	if err != nil {
		t.Fatal(err)
	}
	current := info[0].(*pb.SCFortuneCatInfo).GetInfo()
	if current.GetTodayLeft() != 4 || current.GetCoinPool() != 301 || catQuote63(t, current) != 888 {
		t.Fatal("new Info failed to refresh quote while preserving progress")
	}
	second, err := f.s.p4Welfare(f.ctx, f.id, req, 63002, at)
	if err != nil {
		t.Fatal(err)
	}
	if second[0].(*pb.SCFortuneCatReward).GetGet().Simps[0].GetNum() != 888 {
		t.Fatal("claim diverged from quoted payout")
	}
	replay, err := f.s.p4Welfare(f.ctx, f.id, req, 63001, at)
	if err != nil || !proto.Equal(first[0], replay[0]) {
		t.Fatal("old immutable receipt changed", err)
	}
	p, _ := f.profile(t)
	if p.GetDiamond() != 9599 || p.GetCoin() != 993 {
		t.Fatal("resource accounting", p.GetDiamond(), p.GetCoin())
	}
}
