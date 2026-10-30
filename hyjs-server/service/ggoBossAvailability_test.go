package service

import (
	"encoding/json"
	"testing"
	"time"

	"gorm.io/gorm"
	"hyjs-server/constUtil"
	"hyjs-server/db"
)

func TestGGOBossStaysAvailableUntilKilledAndRespawnsAfterOneHour(t *testing.T) {
	f, _, _, original := ggoBossIntegrationFixture(t)
	now := time.Now().UnixMilli()
	var other *ggoBossEvent
	if err := db.GetIns().Transaction(func(tx *gorm.DB) error {
		row, event, boss, hp, err := f.s.ggoBossCurrent(tx, f.s.GGOFieldConfig, now, true, 1)
		if err != nil {
			return err
		}
		// A previous scheduled event has expired but retains damage and identity.
		event.StartsAtMS, event.EndsAtMS = now-7200000, now-3600000
		row.GGOEvent, err = json.Marshal(event)
		if err != nil {
			return err
		}
		hp[ggoBossGUID(event.ID)] = float64(event.Boss.HP) / 2
		if err = saveSharedFieldBoss55(tx, row, boss, hp); err != nil {
			return err
		}
		_, other, _, _, err = f.s.ggoBossCurrent(tx, f.s.GGOFieldConfig, now, true, 2)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if err := db.GetIns().Transaction(func(tx *gorm.DB) error {
		v, err := f.s.ggoBossSnapshotLocked(tx, f.s.GGOFieldConfig, now, 1)
		if err != nil {
			return err
		}
		if v.State != "active" || v.EventID != original.ID || v.HP != v.MaxHP/2 {
			t.Fatalf("legacy boss changed: %+v", v)
		}
		return sharedFieldBoss55Query(tx.Model(&db.SharedFieldBoss55{}), f.s.OriginalServerID(), constUtil.GGOFieldScene, constUtil.GGOFieldBossPosition, 1).
			Updates(map[string]any{"killer_id": f.id, "respawn_at": now + constUtil.GGOFieldBossRespawnMS}).Error
	}); err != nil {
		t.Fatal(err)
	}
	if err := db.GetIns().Transaction(func(tx *gorm.DB) error {
		v, err := f.s.ggoBossSnapshotLocked(tx, f.s.GGOFieldConfig, now+1000, 1)
		if err != nil {
			return err
		}
		if v.State != "waiting" || v.StartsAtMS != now+constUtil.GGOFieldBossRespawnMS || v.EventID != original.ID {
			t.Fatalf("wrong cooldown: %+v", v)
		}
		v, err = f.s.ggoBossSnapshotLocked(tx, f.s.GGOFieldConfig, now+constUtil.GGOFieldBossRespawnMS, 1)
		if err != nil {
			return err
		}
		if v.State != "active" || v.EventID == original.ID || v.HP != v.MaxHP {
			t.Fatalf("boss did not respawn: %+v", v)
		}
		v, err = f.s.ggoBossSnapshotLocked(tx, f.s.GGOFieldConfig, now+constUtil.GGOFieldBossRespawnMS, 2)
		if err != nil {
			return err
		}
		if v.EventID != other.ID || v.State != "active" {
			t.Fatalf("other line changed: %+v", v)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}
