package service

import (
	"bytes"
	"encoding/json"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

func p4PopularityPublish(t *testing.T, f *p1Fixture, at time.Time) string {
	t.Helper()
	key := constUtil.P4PopularityRankRulePrefix + "local-v1"
	var cfg db.P4CommerceRule
	if err := db.GetIns().First(&cfg, "id=?", key).Error; err != nil {
		t.Fatal(err)
	}
	var rules p4CommerceRules
	if err := json.Unmarshal(cfg.Rules, &rules); err != nil {
		t.Fatal(err)
	}
	rules.RankPublishedAt = at.Add(-2 * time.Second).UnixMilli()
	raw, _ := json.Marshal(rules)
	if err := db.GetIns().Model(&cfg).Updates(map[string]any{"enabled": true, "start_at": at.Add(-time.Second).UnixMilli(), "end_at": at.Add(time.Hour).UnixMilli(), "claim_end_at": at.Add(time.Hour).UnixMilli(), "rules": raw}).Error; err != nil {
		t.Fatal(err)
	}
	return key
}
func p4PopularityFind(t *testing.T, items []*pb.ShopItem, key string) *pb.ShopItem {
	t.Helper()
	for _, item := range items {
		if item.GetId() == key {
			return item
		}
	}
	t.Fatal("missing original shop4 item", key)
	return nil
}

func TestP4PopularityOriginalTabsEnumsAndBatchQuotaUnchanged(t *testing.T) {
	f := p4Setup(t)
	p4Fund(t, f)
	// Explicit finite quota fixture; production's unconfigured catalog stays off.
	if err := db.GetIns().Model(&db.P4CommerceRule{}).Where("kind=?", "popularity").Update("enabled", true).Error; err != nil {
		t.Fatal(err)
	}
	at := time.Now()
	query := &pb.CSShopProductList{ShopType: proto.Int32(4)}
	out := p4Trade(t, f, query, 0, at).(*pb.SCShopProductList)
	if len(out.ShopItemList) != 2 {
		t.Fatal("defaultoff rank source changed ordinary directory")
	}
	for _, item := range out.ShopItemList {
		if item.Condition != nil || item.GetCanBuy() != 1 || item.GetDayLeftTimes() != 20 {
			t.Fatal("original normal tab/canBuy enum", item)
		}
	}
	buy := &pb.CSShopBuy{ShopType: proto.Int32(4), ShopItemId: proto.String("p4Popularity_1"), BuyCount: proto.Int32(20)}
	got := p4Trade(t, f, buy, 901, at).(*pb.SCShopBuy)
	if got.GetCost()[0].GetNum() != 400 || p4PopularityFind(t, got.ShopItemList, "p4Popularity_1").GetCanBuy() != 0 {
		t.Fatal("original batch cost or day quota changed", got)
	}
	if _, err := f.s.p4Commerce(f.ctx, f.id, buy, 902, at); err == nil {
		t.Fatal("ordinary daily quota bypass")
	}
	key := p4PopularityPublish(t, f, at)
	before, _ := f.profile(t)
	listed := p4Trade(t, f, query, 0, at).(*pb.SCShopProductList)
	rankSKU := p4PopularityFind(t, listed.ShopItemList, "p4PopularityRank_local_v1_1")
	if rankSKU.GetCondition().GetType() != "rank" || rankSKU.GetCanBuy() != 0 || rankSKU.GetCondition().GetValue() != 1000 {
		t.Fatal("initial rank granted actual reward", rankSKU)
	}
	request := &pb.CSShopBuy{ShopType: proto.Int32(4), ShopItemId: rankSKU.Id, BuyCount: proto.Int32(1)}
	if _, err := f.s.p4Commerce(f.ctx, f.id, request, 903, at); err == nil {
		t.Fatal("unwitnessed rank purchased")
	}
	after, _ := f.profile(t)
	if !proto.Equal(before, after) || p1Count(t, &db.P4PopularityRankClaim{}, "rule_id=?", key) != 0 {
		t.Fatal("rejected rank cost/claim changed")
	}
}

func TestP4PopularityActualArenaDailyCurrencyRankExchangeColdAndGuide(t *testing.T) {
	if constUtil.P6ArenaNativeProof == "" {
		t.Skip("independent original Arena2 certificate pending")
	}
	f, owner, clock, end := p11ArenaDailySetup(t)
	if err := f.s.InitializeGameplayPools(f.ctx); err != nil {
		t.Fatal(err)
	}
	if err := f.s.InitializeP4(f.ctx); err != nil {
		t.Fatal(err)
	}
	at := *clock
	key := p4PopularityPublish(t, f, at)
	if err := updateProfile(f.ctx, owner, func(p *pb.Player, g *model.GameState) error { p.NoviceGuideSchedule = proto.Int32(13); return nil }); err != nil {
		t.Fatal(err)
	}
	if _, err := f.s.p11ArenaQuery(f.ctx, owner, &pb.CSPvpFindMatch{}, 9101, at); err != nil {
		t.Fatal(err)
	}
	fight, err := f.s.p11ArenaScopedFightAt(f.ctx, owner, &pb.CSPvpFightResult{PlayerId: proto.Int64(f.id), ServerId: proto.Int32(1)}, 9102, at)
	if err != nil || fight[0].(*pb.SCPvpFightResult).GetWinPlayerId() != owner {
		t.Fatal("actual accepted winner prerequisite", err)
	}
	*clock = end.Add(time.Second)
	if err = f.s.TickP11ArenaDaily(f.ctx, *clock); err != nil {
		t.Fatal(err)
	}
	var award db.P11ArenaDailyAward
	if err = db.GetIns().First(&award, "player_id=?", owner).Error; err != nil {
		t.Fatal(err)
	}
	if _, err = f.s.RewardCenter(f.ctx, owner, &pb.CSAddRewardsToPlayer{RewardIds: []int64{award.DeliveryID}}); err != nil {
		t.Fatal(err)
	}
	before, _ := p10LuckyProfile(t, owner)
	if before.GetPopularity() != 5050 {
		t.Fatal("real original P1 daily source missing", before.GetPopularity())
	}
	query := &pb.CSShopProductList{ShopType: proto.Int32(4)}
	list, err := f.s.p4Commerce(f.ctx, owner, query, 0, at)
	if err != nil {
		t.Fatal(err)
	}
	sku := p4PopularityFind(t, list[0].(*pb.SCShopProductList).ShopItemList, "p4PopularityRank_local_v1_1")
	if sku.GetCanBuy() != 1 {
		t.Fatal("actual accepted rank not reachable", sku)
	}
	// Altering mutable policy terms cannot rewrite the first original list's price,
	// rank threshold, one-time quota or publication source.
	var cfg db.P4CommerceRule
	if err = db.GetIns().First(&cfg, "id=?", key).Error; err != nil {
		t.Fatal(err)
	}
	var rules p4CommerceRules
	_ = json.Unmarshal(cfg.Rules, &rules)
	rules.Products[0].Cost = "popularity,4999"
	rules.Products[0].MaxRank = 1
	raw, _ := json.Marshal(rules)
	if err = db.GetIns().Model(&cfg).Update("rules", raw).Error; err != nil {
		t.Fatal(err)
	}
	request := &pb.CSShopBuy{ShopType: proto.Int32(4), ShopItemId: sku.Id, BuyCount: proto.Int32(1)}
	replies, err := f.s.p4Commerce(f.ctx, owner, request, 9201, at)
	if err != nil {
		t.Fatal(err)
	}
	bought := replies[0].(*pb.SCShopBuy)
	original, _ := proto.Marshal(bought)
	current, game := p10LuckyProfile(t, owner)
	if current.GetPopularity() != 5030 || current.GetNoviceGuideSchedule() != 14 || simpleOwned(current, game, "item_001") != 1 || bought.GetCost()[0].GetNum() != 20 || p4PopularityFind(t, bought.ShopItemList, sku.GetId()).GetCanBuy() != 2 {
		t.Fatal("rank source/actual cost/quota/guide", current, bought)
	}
	var claim db.P4PopularityRankClaim
	if err = db.GetIns().First(&claim, "player_id=?", owner).Error; err != nil || claim.Rank != 1 || len(claim.RankProof) != 64 {
		t.Fatal("accepted witness not retained", claim, err)
	}
	if _, err = f.s.p4Commerce(f.ctx, owner, request, 9202, at); err == nil {
		t.Fatal("second stamp duplicated one-time reward")
	}
	// A real earlier purchase resumes Guide14 after cursor13 is reached later.
	if err = updateProfile(f.ctx, owner, func(p *pb.Player, g *model.GameState) error { p.NoviceGuideSchedule = proto.Int32(13); return nil }); err != nil {
		t.Fatal(err)
	}
	if _, err = f.s.p4Commerce(f.ctx, owner, query, 0, at); err != nil {
		t.Fatal(err)
	}
	current, _ = p10LuckyProfile(t, owner)
	if current.GetNoviceGuideSchedule() != 14 {
		t.Fatal("actual earlier claim could not resume current guide")
	}
	connection, _ := db.GetIns().DB()
	if err = connection.Close(); err != nil {
		t.Fatal(err)
	}
	f.open(t)
	if err = db.GetIns().Model(&db.P4CommerceRule{}).Where("id=?", key).Update("enabled", false).Error; err != nil {
		t.Fatal(err)
	}
	again, err := f.s.p4Commerce(f.ctx, owner, request, 9201, at)
	if err != nil {
		t.Fatal(err)
	}
	replay, _ := proto.Marshal(again[0])
	current, game = p10LuckyProfile(t, owner)
	if !bytes.Equal(original, replay) || current.GetPopularity() != 5030 || simpleOwned(current, game, "item_001") != 1 || p1Count(t, &db.P4PopularityRankClaim{}, "player_id=?", owner) != 1 {
		t.Fatal("cold replay changed actual wallet/item/claim")
	}
}
