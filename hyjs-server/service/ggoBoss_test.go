package service

import (
	"encoding/json"
	"errors"
	"fmt"
	"testing"
	"time"

	"gorm.io/gorm"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

func TestGGOBossCalendarAndMergedChips(t *testing.T) {
	cfg := defaultGGORuntime().Boss
	zone := time.FixedZone("UTC+8", 8*3600)
	base := time.Date(2026, 10, 28, 0, 0, 0, 0, zone)
	starts := map[int64]bool{}
	for minute := 0; minute < 1440; minute++ {
		now := base.Add(time.Duration(minute) * time.Minute).UnixMilli()
		start, end, err := ggoBossWindow(cfg, now)
		if err != nil {
			t.Fatal(err)
		}
		if now >= start && now < end {
			starts[start] = true
		}
	}
	if len(starts) != 30 {
		t.Fatal("calendar events", len(starts))
	}
	start, _, err := ggoBossWindow(cfg, base.Add(17*time.Hour+20*time.Minute).UnixMilli())
	if err != nil || start != base.Add(18*time.Hour).UnixMilli() {
		t.Fatal("day-evening boundary", start, err)
	}
	event := &ggoBossEvent{Boss: cfg}
	for _, quality := range []int{3, 4, 5} {
		event.Boss.TailRewards = []model.AdminFieldGGOWeight{{ItemID: "item_970" + string(rune('1'+quality)), Weight: 10000}}
		chips, err := ggoBossChipReward(event, float64(cfg.HP)*.03, 1, true)
		if err != nil {
			t.Fatal(err)
		}
		expected := [6]int64{}
		expected[2] = 2
		expected[3] = 1
		expected[quality]++
		if chips != expected {
			t.Fatal("merged participation rank and tail", quality, chips, expected)
		}
	}
	low, err := ggoBossChipReward(event, 1, 11, false)
	if err != nil || low != ([6]int64{0, 0, 1, 0, 0, 0}) {
		t.Fatal("guaranteed participation", low, err)
	}
}

func TestGGOBossCopiesOnlyParticipationMaterial(t *testing.T) {
	cfg := defaultGGORuntime().Boss
	cfg.ResourceDrops = []model.AdminFieldDrop{growth74Drop("diamond", 100), growth74Drop("MineItem_009", 10), growth74Drop("FishItem_025", 1)}
	cfg.RankResourceDrops = []model.AdminFieldBossRank73{{From: 1, To: 1, Drops: []model.AdminFieldDrop{growth74Drop("diamond", 5), growth74Drop("MineItem_009", 2)}}}
	cfg.TailResourceDrops = []model.AdminFieldDrop{growth74Drop("diamond", 3)}
	cfg.SproutMax = 1
	cfg.SproutBonusBps = 10000
	cfg.OverclockRewards = []model.AdminFieldGGOOverclockReward{{Sprouts: 1, Quantities: []model.AdminFieldQuantity{{Amount: 6, Weight: 1}}}}
	text, err := ggoBossResourceReward(&ggoBossEvent{Boss: cfg}, 1, 1, true)
	if err != nil {
		t.Fatal(err)
	}
	values := map[string]int64{}
	if err = ggoBossAddContent(values, text); err != nil {
		t.Fatal(err)
	}
	if values["diamond"] != 108 || values["MineItem_009"] != 22 || values["FishItem_025"] != 1 || values["item_9711"] != 6 {
		t.Fatal(values)
	}
}

func TestGGOBossLateAwardsAtomicAllBatchesAndCold(t *testing.T) {
	f := p1Setup(t)
	if err := db.GetIns().AutoMigrate(&db.FieldBossContribution73{}, &db.GGOFieldReceipt{}); err != nil {
		t.Fatal(err)
	}
	award := ggoBossAward{EconomyVersion: "old-desert", ExchangeValues: [6]int64{3, 9, 30, 90, 300, 900}, Chips: [6]int64{0, 0, 1, 0, 0, 0}}
	raw, err := json.Marshal(award)
	if err != nil {
		t.Fatal(err)
	}
	rows := make([]db.FieldBossContribution73, 205)
	for i := range rows {
		rows[i] = db.FieldBossContribution73{ServerID: f.s.OriginalServerID(), SpawnUUID: fmt.Sprintf("late-event-%d", i), PlayerID: f.id, Damage: 1, Rules: []byte("{}"), DailyLimit: 3, RewardDay: "2026-10-01", Rank: 1, Frozen: true, Claimed: true, GGOChips: raw}
	}
	if err = db.GetIns().CreateInBatches(rows, 100).Error; err != nil {
		t.Fatal(err)
	}
	forced := errors.New("rollback late settlement")
	apply := func(abort bool, want int64) error {
		return updateProfileWithTx(f.ctx, f.id, func(tx *gorm.DB, _ *pb.Player, g *model.GameState) error {
			n, err := f.s.ggoSettleLateBossAwardsLocked(tx, f.id, g)
			if err != nil {
				return err
			}
			if n != want {
				return fmt.Errorf("late sum %d want %d", n, want)
			}
			if abort {
				return forced
			}
			return nil
		})
	}
	if err = apply(true, 6150); !errors.Is(err, forced) {
		t.Fatal(err)
	}
	var claimed int64
	if err = db.GetIns().Model(&db.FieldBossContribution73{}).Where("player_id=? AND ggo_chips_claimed=?", f.id, true).Count(&claimed).Error; err != nil || claimed != 0 {
		t.Fatal("rollback claimed", claimed, err)
	}
	if err = apply(false, 6150); err != nil {
		t.Fatal(err)
	}
	if err = apply(false, 0); err != nil {
		t.Fatal("replay", err)
	}
	f.restart(t)
	_, g := f.profile(t)
	if ggoCreditBalance(g) != 6150 {
		t.Fatal("cold balance", ggoCreditBalance(g))
	}
	if err = apply(false, 0); err != nil {
		t.Fatal("cold replay", err)
	}
	var receipts int64
	if err = db.GetIns().Model(&db.GGOFieldReceipt{}).Where("player_id=?", f.id).Count(&receipts).Error; err != nil || receipts != 205 {
		t.Fatal("receipts", receipts, err)
	}
}
