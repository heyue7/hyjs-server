package service

import (
	"encoding/json"
	"math"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
	"hyjs-server/db"
)

func TestGGOFormationUpgradePreservesEventAndDamage(t *testing.T) {
	f, _, _, old := ggoBossIntegrationFixture(t)
	now := time.Now().UnixMilli()
	if err := db.GetIns().Transaction(func(tx *gorm.DB) error {
		row, event, boss, hp, err := f.s.ggoBossCurrent(tx, f.s.GGOFieldConfig, now, true, 1)
		if err != nil {
			return err
		}
		event.Boss.HeroID = "hero_017"
		old.Boss.HeroID = "hero_017"
		boss.HeroId = proto.String("hero_017")
		row.GGOEvent, err = json.Marshal(event)
		if err != nil {
			return err
		}
		hp[ggoBossGUID(event.ID)] -= 40550
		return saveSharedFieldBoss55(tx, row, boss, hp)
	}); err != nil {
		t.Fatal(err)
	}
	f.s.GGOFieldConfig.Runtime.Boss.HeroID = "monster_047"
	f.s.GGOFieldConfig.Runtime.Boss.HP = 44000000
	var migrated *ggoBossEvent
	if err := db.GetIns().Transaction(func(tx *gorm.DB) error {
		row, event, boss, hp, err := f.s.ggoBossCurrent(tx, f.s.GGOFieldConfig, now+1, false, 1)
		if err != nil {
			return err
		}
		migrated = event
		if event.ID != old.ID || event.Boss.HeroID != "monster_047" || boss.GetHeroId() != "monster_047" || row.KillerID != 0 || row.RespawnAt != 0 || math.Abs(hp[ggoBossGUID(event.ID)]-float64(44000000-40550)) > .001 {
			t.Fatalf("event migration lost identity or damage: event=%+v hp=%v", event, hp)
		}
		if event.EconomyVersion != old.EconomyVersion || event.Boss.DailyLimit != old.Boss.DailyLimit || event.ExchangeValues != old.ExchangeValues {
			t.Fatal("event reward settings changed")
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if err := db.GetIns().Transaction(func(tx *gorm.DB) error {
		_, event, _, hp, err := f.s.ggoBossCurrent(tx, f.s.GGOFieldConfig, now+2, false, 1)
		if err == nil && (event.ID != migrated.ID || hp[ggoBossGUID(event.ID)] != float64(44000000-40550)) {
			t.Fatal("migration not idempotent")
		}
		return err
	}); err != nil {
		t.Fatal(err)
	}
}
