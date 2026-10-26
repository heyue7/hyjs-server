package service

import (
	"encoding/json"
	"fmt"
	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
	"strings"
	"testing"
)

func TestFieldBossDeath73ContributionFreezeAndStableRank(t *testing.T) {
	f := p1Setup(t)
	if err := f.s.InitializeFieldBossDeath73(f.ctx); err != nil {
		t.Fatal(err)
	}
	rules := &model.AdminFieldBossGroup{DeathSettlement: true, Drops: []model.AdminFieldDrop{{ResourceID: "coin", Quantities: []model.AdminFieldQuantity{{Amount: 10, Weight: 1}}}}, KillDrops: []model.AdminFieldDrop{{ResourceID: "coin", Quantities: []model.AdminFieldQuantity{{Amount: 7, Weight: 1}}}}, RankDrops: []model.AdminFieldBossRank73{{From: 1, To: 1, Drops: []model.AdminFieldDrop{{ResourceID: "coin", Quantities: []model.AdminFieldQuantity{{Amount: 20, Weight: 1}}}}}}}
	field := &model.FieldState{Scene: "field_001", Fight: &model.FieldFight{BossUUID: "death73", BossDeathRules73: rules}}
	apply := func(id int64, damage float64, killed bool) {
		t.Helper()
		if err := db.GetIns().Transaction(func(tx *gorm.DB) error { return f.s.fieldBossContribute73(tx, id, field, damage, killed) }); err != nil {
			t.Fatal(err)
		}
	}
	apply(f.id, 20, false)
	apply(f.id+1, 40, false)
	apply(f.id, 20, true)
	var rows []db.FieldBossContribution73
	if err := db.GetIns().Where("server_id=? AND spawn_uuid=? AND player_id IN ?", f.s.OriginalServerID(), "death73", []int64{f.id, f.id + 1}).Order("reward_rank").Find(&rows).Error; err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 || rows[0].PlayerID != f.id || rows[0].Rank != 1 || rows[1].Rank != 2 || rows[0].Damage != 40 || rows[1].Damage != 40 {
		t.Fatalf("unstable contribution rank: %+v", rows)
	}
	if !strings.Contains(rows[0].Reward, "coin,20") || !strings.Contains(rows[0].Reward, "coin,7") || rows[1].Reward != "coin,10" {
		t.Fatalf("rewards %+v", rows)
	}
	apply(f.id, 999, true)
	var after db.FieldBossContribution73
	db.GetIns().First(&after, rows[0].ID)
	if after.Damage != 40 || after.Reward != rows[0].Reward {
		t.Fatal("frozen settlement mutated")
	}
	apply(f.id+2, 0, true)
	var count int64
	db.GetIns().Model(&db.FieldBossContribution73{}).Where("server_id=? AND spawn_uuid=? AND player_id IN ?", f.s.OriginalServerID(), "death73", []int64{f.id, f.id + 1, f.id + 2}).Count(&count)
	if count != 2 {
		t.Fatal("zero damage participated")
	}
}

func TestFieldBossDeath73ActualDamageAndOldGeneration(t *testing.T) {
	f := p1Setup(t)
	if err := f.s.InitializeFieldBossDeath73(f.ctx); err != nil {
		t.Fatal(err)
	}
	if err := db.GetIns().AutoMigrate(&db.SharedFieldBoss55{}); err != nil {
		t.Fatal(err)
	}
	f.s.SharedFieldBoss55Ready = true
	spawn := fmt.Sprint("real73-", f.id)
	position := fmt.Sprint("pos73-", f.id)
	boss := &pb.SideBoss{Uuid: proto.String(spawn), BossPosId: proto.String(position), State: proto.Int32(1)}
	raw, _ := json.Marshal(boss)
	// Shared payload is JSON by contract.
	game := &model.GameState{Field: &model.FieldState{Scene: "field_001", BossHP: map[string]map[string]float64{}, RespawnAt: map[string]int64{}, Fight: &model.FieldFight{BossUUID: spawn, BossLeaderGUID: "actor", BossDeathRules73: &model.AdminFieldBossGroup{DeathSettlement: true}}}}
	input := &model.FrozenBattleInput{Monster: &pb.BattleMonster{Formation: []*pb.BattleHero{{Guid: proto.String("actor"), Prop: &pb.BaseProp{Hp: proto.Float64(100)}, Inst: &pb.BattleHeroInst{CurHp: proto.Float64(100)}}}}}
	if err := db.GetIns().Create(&db.SharedFieldBoss55{ServerID: f.s.OriginalServerID(), Scene: "field_001", PosID: position, UUID: spawn, Payload: raw, HP: []byte(`{"actor":10}`)}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.GetIns().Transaction(func(tx *gorm.DB) error {
		_, k, e := f.s.applySharedFieldBoss55(tx, f.id, game.Field, boss, input, map[string]float64{"actor": 0}, 3600000)
		if !k {
			t.Fatal("not killed")
		}
		return e
	}); err != nil {
		t.Fatal(err)
	}
	var contribution db.FieldBossContribution73
	if err := db.GetIns().Where("server_id=? AND spawn_uuid=? AND player_id=?", f.s.OriginalServerID(), spawn, f.id).First(&contribution).Error; err != nil {
		t.Fatal(err)
	}
	if contribution.Damage != 10 {
		t.Fatal("overkill entered rank", contribution.Damage)
	}
	if err := db.GetIns().Model(&db.SharedFieldBoss55{}).Where("server_id=? AND scene=? AND pos_id=?", f.s.OriginalServerID(), "field_001", position).Update("uuid", spawn+"-next").Error; err != nil {
		t.Fatal(err)
	}
	if _, _, err := f.s.applySharedFieldBoss55(db.GetIns(), f.id, game.Field, boss, input, map[string]float64{"actor": 0}, 3600000); err == nil {
		t.Fatal("old generation accepted")
	}

}

func TestFieldBossDeath73DropProbabilityBoundaries(t *testing.T) {
	fixed := []model.AdminFieldDrop{{ResourceID: "diamond", Quantities: []model.AdminFieldQuantity{{Amount: 5, Weight: 1}}}}
	for _, n := range []int64{0, 1, 2, 9} {
		got, err := bossDropContent73(fixed, n, 10000)
		if err != nil || got != "diamond,5" {
			t.Fatal("fixed amount changed", got, err)
		}
	}
	drops := []model.AdminFieldDrop{{ResourceID: "coin", Quantities: []model.AdminFieldQuantity{{Amount: 0, Weight: 1}, {Amount: 5, Weight: 1}}}}
	for i := 0; i < 20; i++ {
		got, err := bossDropContent73(drops, 2, 10000)
		if err != nil || got != "coin,5" {
			t.Fatal("capped appearance failed", got, err)
		}
	}
}

func TestFieldBossDeath73PendingDailyCapAndColdRetry(t *testing.T) {
	f := p1Setup(t)
	if err := f.s.InitializeFieldBossDeath73(f.ctx); err != nil {
		t.Fatal(err)
	}
	if err := updateProfile(f.ctx, f.id, func(p *pb.Player, g *model.GameState) error {
		p.Coin = proto.Int64(0)
		g.Field = &model.FieldState{Scene: "field_001"}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	day := LocalPlayNow(f.ctx).Format("2006-01-02")
	for i := 0; i < 4; i++ {
		r := db.FieldBossContribution73{ServerID: f.s.OriginalServerID(), SpawnUUID: fmt.Sprint("pending", i), PlayerID: f.id, Damage: 10, Rules: []byte(`{}`), DailyLimit: 3, Reward: "coin,5", RewardDay: day, Rank: 1, Frozen: true}
		if err := db.GetIns().Create(&r).Error; err != nil {
			t.Fatal(err)
		}
	}
	if err := f.s.CollectFieldBossPending(f.ctx, f.id); err != nil {
		t.Fatal(err)
	}
	p, g := f.profile(t)
	if p.GetCoin() != 15 || g.Field.BossRewardClaims != 3 {
		t.Fatal("daily quota", p.GetCoin(), g.Field.BossRewardClaims)
	}
	f.restart(t)
	if err := f.s.CollectFieldBossPending(f.ctx, f.id); err != nil {
		t.Fatal(err)
	}
	p, g = f.profile(t)
	if p.GetCoin() != 15 || g.Field.BossRewardClaims != 3 {
		t.Fatal("cold receipt repeated")
	}
	// A prior-day receipt keeps its death date and cannot move today's shared counter backwards.
	r := db.FieldBossContribution73{ServerID: f.s.OriginalServerID(), SpawnUUID: "yesterday", PlayerID: f.id, Damage: 1, Rules: []byte(`{}`), DailyLimit: 3, Reward: "coin,2", RewardDay: "2000-01-01", Rank: 1, Frozen: true}
	if err := db.GetIns().Create(&r).Error; err != nil {
		t.Fatal(err)
	}
	if err := f.s.CollectFieldBossPending(f.ctx, f.id); err != nil {
		t.Fatal(err)
	}
	p, g = f.profile(t)
	if p.GetCoin() != 17 || g.Field.BossRewardDay != day || g.Field.BossRewardClaims != 3 {
		t.Fatal("cross-day receipt broke quota")
	}
}

func TestFieldBossDeath73VisibleGainAndSavedReceipt(t *testing.T) {
	f := p1Setup(t)
	if err := f.s.InitializeFieldBossDeath73(f.ctx); err != nil {
		t.Fatal(err)
	}
	if err := updateProfile(f.ctx, f.id, func(p *pb.Player, g *model.GameState) error {
		g.Field = &model.FieldState{Scene: "field_001", Fight: &model.FieldFight{BossUUID: "visible73", Settled: true, BossResult: &pb.SCSidePveBossFightEnd{Get: &pb.ResultObject{}}}}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	r := db.FieldBossContribution73{ServerID: f.s.OriginalServerID(), PlayerID: f.id, SpawnUUID: "visible73", Damage: 1, Rules: []byte(`{}`), DailyLimit: 3, Reward: "coin,4", RewardDay: LocalPlayNow(f.ctx).Format("2006-01-02"), Rank: 1, Frozen: true}
	if err := db.GetIns().Create(&r).Error; err != nil {
		t.Fatal(err)
	}
	gain, err := f.s.CollectFieldBossPendingRewards(f.ctx, f.id)
	if err != nil || gain == nil || len(gain.Simps) != 1 {
		t.Fatal("missing visible gain", gain, err)
	}
	_, g := f.profile(t)
	if !proto.Equal(gain, g.Field.Fight.BossResult.Get) {
		t.Fatal("saved receipt does not match new reward")
	}
	again, err := f.s.CollectFieldBossPendingRewards(f.ctx, f.id)
	if err != nil || again != nil {
		t.Fatal("duplicate produced new gain", again, err)
	}
	_, g = f.profile(t)
	if !proto.Equal(gain, g.Field.Fight.BossResult.Get) {
		t.Fatal("retry changed saved receipt")
	}
}
