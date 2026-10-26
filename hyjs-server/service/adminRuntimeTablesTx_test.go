package service

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"gorm.io/gorm"
	"hyjs-server/db"
)

func TestAdminRuntimeTablesTxReadsOwnSettingsWithoutMutatingBase(t *testing.T) {
	f, _ := p13GuildSetup(t)
	if err := f.s.InitializeAdminGuildSettings(f.ctx); err != nil {
		t.Fatal(err)
	}
	// A small independent settings overlay, with no game/source authority claim.
	base := map[string][]map[string]string{"GameConfigure": {{"StaticName": "UnionNoticeLengthLimit", "NameValue": "1"}}}
	before, err := AdminGuildRuntimeTables(f.ctx, base)
	if err != nil {
		t.Fatal(err)
	}
	beforeValue := before["GameConfigure"][0]["NameValue"]
	ctx, cancel := context.WithTimeout(f.ctx, 3*time.Second)
	defer cancel()
	rollback := errors.New("rollback settings visibility probe")
	err = db.GetIns().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		raw, err := json.Marshal(adminGuildStored{Fields: map[string]string{"UnionNoticeLengthLimit": "2"}})
		if err != nil {
			return err
		}
		if err = tx.Model(&db.AdminGuildSetting{}).Where("id=1").Update("settings", string(raw)).Error; err != nil {
			return err
		}
		got, err := adminGuildRuntimeTablesTx(tx, base)
		if err != nil {
			return err
		}
		if got["GameConfigure"][0]["NameValue"] != "2" {
			t.Fatal("helper did not observe same-transaction settings")
		}
		if base["GameConfigure"][0]["NameValue"] != "1" {
			t.Fatal("runtime overlay mutated shared base table")
		}
		return rollback
	})
	if !errors.Is(err, rollback) {
		t.Fatal("single-connection tx lookup failed or escaped transaction", err)
	}
	after, err := AdminGuildRuntimeTables(f.ctx, base)
	if err != nil || after["GameConfigure"][0]["NameValue"] != beforeValue {
		t.Fatal("rolled-back setting escaped into public runtime", err)
	}
	if got, err := adminGuildRuntimeTablesTx(db.GetIns(), nil); err != nil || got != nil {
		t.Fatal("nil-table fallback changed", err)
	}
}
