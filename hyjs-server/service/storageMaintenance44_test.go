package service

import (
	"context"
	"reflect"
	"strings"
	"testing"
	"time"

	"hyjs-server/db"
)

func TestStorage44ExpiredPublicationCleanup(t *testing.T) {
	f, _ := adminModesFixture(t)
	s := f.s
	tx := db.GetIns()
	ctx := context.Background()
	if err := tx.AutoMigrate(&db.P11ArenaMatch{}, &db.P11ArenaFightSettlement{}, &db.P11PlunderFightMatch{}, &db.P11PlunderFightSettlement{}, &db.P11LimitFightMatch{}, &db.P11LimitFightSettlement{}); err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	for _, table := range unusedMatchTables {
		// Empty tables are sufficient for all other columns, which have database defaults.
		for _, state := range []string{"expired-unused", "expired-accepted", "live-unused"} {
			expires := now.Add(-time.Hour).UnixMilli()
			if state == "live-unused" {
				expires = now.Add(time.Hour).UnixMilli()
			}
			row := storage44Fixture(table.match, state, expires)
			// Raw minimal rows are schema-independent test fixtures, never production records.
			if err := tx.Table(table.match).Create(row).Error; err != nil {
				t.Fatal(err)
			}
		}
		if err := tx.Table(table.settlement).Create(storage44Fixture(table.settlement, "expired-accepted", 0)).Error; err != nil {
			t.Fatal(err)
		}
	}
	before, err := s.CleanupUnusedExpiredMatches(ctx, now, false)
	if err != nil {
		t.Fatal(err)
	}
	for _, count := range before {
		if count.Eligible != 1 || count.Removed != 0 {
			t.Fatalf("dry run changed data: %+v", count)
		}
	}
	after, err := s.CleanupUnusedExpiredMatches(ctx, now, true)
	if err != nil {
		t.Fatal(err)
	}
	for _, count := range after {
		if count.Removed != 1 {
			t.Fatalf("wrong removed count: %+v", count)
		}
		var ids []string
		if err := tx.Table(count.Table).Order("id ASC").Pluck("id", &ids).Error; err != nil {
			t.Fatal(err)
		}
		if strings.Join(ids, ",") != "expired-accepted,live-unused" {
			t.Fatalf("retained rows differ: %v", ids)
		}
	}
	again, err := s.CleanupUnusedExpiredMatches(ctx, now, true)
	if err != nil {
		t.Fatal(err)
	}
	for _, count := range again {
		if count.Eligible != 0 || count.Removed != 0 {
			t.Fatalf("cleanup not idempotent: %+v", count)
		}
	}
}

func storage44Fixture(table, id string, expires int64) any {
	var row any
	switch table {
	case "p11_arena_matches":
		row = &db.P11ArenaMatch{}
	case "p11_plunder_fight_matches":
		row = &db.P11PlunderFightMatch{}
	case "p11_limit_fight_matches":
		row = &db.P11LimitFightMatch{}
	case "p11_arena_fight_settlements":
		row = &db.P11ArenaFightSettlement{}
	case "p11_plunder_fight_settlements":
		row = &db.P11PlunderFightSettlement{}
	case "p11_limit_fight_settlements":
		row = &db.P11LimitFightSettlement{}
	}
	value := reflect.ValueOf(row).Elem()
	for i := 0; i < value.NumField(); i++ {
		if value.Field(i).Kind() == reflect.Slice {
			value.Field(i).Set(reflect.MakeSlice(value.Field(i).Type(), 0, 0))
		}
	}
	if strings.HasSuffix(table, "matches") {
		value.FieldByName("ID").SetString(id)
		value.FieldByName("ExpiresAt").SetInt(expires)
	} else {
		value.FieldByName("MatchID").SetString(id)
		value.FieldByName("PlayerID").SetInt(1)
		value.FieldByName("Stamp").SetInt(1)
	}
	return row
}
