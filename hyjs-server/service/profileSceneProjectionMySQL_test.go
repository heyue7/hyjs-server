package service

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"

	"gorm.io/gorm"
	"hyjs-server/db"
)

func TestPlayerConcurrencyMySQLSceneProjectionMigration(t *testing.T) {
	name := os.Getenv("HYJS_P1_MYSQL_DATABASE")
	if name == "" {
		t.Skip("requires a coordinated isolated hyjs_test_p1_ MySQL database")
	}
	if !strings.HasPrefix(name, "hyjs_test_p1_") || os.Getenv("HYJS_GAME_ACCOUNTS_DATABASE") != "" {
		t.Fatal("requires an isolated game and accounts fixture")
	}

	f := p1Setup(t)
	database := db.GetIns()
	writeGame := func(tx *gorm.DB, game string) error {
		return tx.Exec("UPDATE profiles SET game=? WHERE id=?", game, f.id).Error
	}
	if err := writeGame(database, `{"position":{"scene":"town_main"}}`); err != nil {
		t.Fatal(err)
	}
	if err := database.Exec("ALTER TABLE profiles DROP COLUMN scene_scope").Error; err != nil {
		t.Fatal(err)
	}
	pool, err := database.DB()
	if err != nil {
		t.Fatal(err)
	}
	if err := pool.Close(); err != nil {
		t.Fatal(err)
	}
	f.open(t)
	database = db.GetIns()

	readScope := func(tx *gorm.DB) (string, error) {
		var profile db.Profile
		if err := tx.Select("scene_scope").First(&profile, "id=?", f.id).Error; err != nil {
			return "", err
		}
		if profile.SceneScope == nil {
			return "", errors.New("scene_scope is NULL")
		}
		return *profile.SceneScope, nil
	}
	checkScope := func(t *testing.T, tx *gorm.DB, want string) {
		t.Helper()
		got, err := readScope(tx)
		if err != nil {
			t.Fatal(err)
		}
		if got != want {
			t.Fatalf("scene_scope = %q, want %q", got, want)
		}
	}
	checkScope(t, database, "town_main")

	for _, tc := range []struct {
		name string
		game string
		want string
	}{
		{"town-loading", `{"position":{"scene":"town"},"position_loading":true}`, "town_main"},
		{"field", `{"position":{"scene":"field_001"}}`, "field_001"},
		{"field-loading", `{"position":{"scene":"field_001"},"position_loading":true}`, ""},
		{"ggo-missing-state", `{"position":{"scene":"field_003"}}`, ""},
		{"ggo", `{"position":{"scene":"field_003"},"ggo_field":{}}`, "field_003"},
		{"town-incomplete-run", `{"position":{"scene":"town"},"run":{"complete":false}}`, ""},
		{"town-complete-run", `{"position":{"scene":"town"},"run":{"complete":true}}`, "town_main"},
		{"field-unsettled-fight", `{"position":{"scene":"field_001"},"field":{"fight":{"settled":false}}}`, ""},
		{"field-settled-fight", `{"position":{"scene":"field_001"},"field":{"fight":{"settled":true}}}`, "field_001"},
		{"town-union", `{"position":{"scene":"town_union"}}`, "town_union"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if err := writeGame(database, tc.game); err != nil {
				t.Fatal(err)
			}
			checkScope(t, database, tc.want)
		})
	}

	rollback := errors.New("force rollback")
	err = database.Transaction(func(tx *gorm.DB) error {
		if err := writeGame(tx, `{"position":{"scene":"town_main"}}`); err != nil {
			return err
		}
		got, err := readScope(tx)
		if err != nil {
			return err
		}
		if got != "town_main" {
			return fmt.Errorf("transaction scene_scope = %q, want town_main", got)
		}
		return rollback
	})
	if !errors.Is(err, rollback) {
		t.Fatalf("transaction rollback: %v", err)
	}
	checkScope(t, database, "town_union")
}
