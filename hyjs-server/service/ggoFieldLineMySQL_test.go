package service

import (
	"context"
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	"gorm.io/gorm"
	"hyjs-server/db"
	"hyjs-server/model"
)

// Synthetic coordinates are confined to this transaction test. They are not
// placement evidence or a publishable map.
func ggoLineTestConfiguration(t *testing.T) *model.AdminFieldGGOConfiguration {
	t.Helper()
	cfg := &model.AdminFieldGGOConfiguration{SchemaVersion: 3, Chips: defaultGGOChips(), Rules: defaultGGORules(), Runtime: defaultGGORuntime()}
	cfg.Runtime.Enabled = true
	cfg.Runtime.MinClientBuild = 1 // Isolated synthetic fixture, not a release build.
	position := func(x float64) *model.AdminFieldGGOPosition {
		return &model.AdminFieldGGOPosition{X: ggoFloat64(x), Y: ggoFloat64(0), Z: ggoFloat64(8)}
	}
	cfg.Runtime.EntryPoints = []*model.AdminFieldGGOPosition{position(20)}
	safe := &model.AdminFieldGGOPosition{X: ggoFloat64(-7), Y: ggoFloat64(0), Z: ggoFloat64(-6)}
	cfg.NPCs = append(cfg.NPCs, model.AdminFieldGGONPC{ID: "guide", HeroID: "hero_005", Role: "teleport", Position: safe}, model.AdminFieldGGONPC{ID: "clerk", HeroID: "clerk01_npc", Role: "settlement", Position: safe})
	for i := 0; i < 3; i++ {
		cfg.NPCs = append(cfg.NPCs, model.AdminFieldGGONPC{ID: fmt.Sprintf("exitnpc%d", i), HeroID: "hero_029", Role: "extraction", Position: position(float64(i + 10))})
	}
	add := func(kind string, level int64, count int) {
		for i := 0; i < count; i++ {
			p := position(float64(i + 10))
			point := model.AdminFieldGGOPoint{ID: fmt.Sprintf("%s-%d-%d", kind, level, i), Kind: kind, Level: level, Position: p, StandPosition: p, Radius: 3}
			if kind == "monster" && level == 35 && i < 4 {
				point.Patrol = &model.AdminFieldGGOPatrol{Waypoints: []*model.AdminFieldGGOPosition{p, position(float64(i + 11))}, Speed: 3.2}
			}
			cfg.Runtime.Points = append(cfg.Runtime.Points, point)
		}
	}
	add("mine", 0, 18)
	add("fish", 0, 12)
	add("monster", 35, 10)
	add("monster", 45, 8)
	add("monster", 60, 6)
	add("boss", 0, 1)
	add("exit", 0, 3)
	if !model.ValidAdminFieldGGORuntime(cfg) {
		t.Fatal("isolated test configuration invalid")
	}
	return cfg
}

// This test uses the local-services lease only. It locks and deletes exclusively
// a unique server id, leaving the admin fixture and its data untouched.
func TestGGOFieldLineMySQLConcurrentReservationAndBoundary(t *testing.T) {
	if os.Getenv("HYJS_GGO_LINE_MYSQL") != "1" {
		t.Skip("requires explicitly leased local MySQL")
	}
	name := os.Getenv("MYSQL_DATABASE")
	host := os.Getenv("MYSQL_HOST")
	port := os.Getenv("MYSQL_PORT")
	if name != "hyjs_test_desert_v4_a18" || host != "127.0.0.1" || port != "3306" {
		t.Fatal("unexpected test lease")
	}
	ctx := context.Background()
	connection, err := db.OpenGameMySQL(ctx, db.MySQLConfig{Host: host + ":" + port, User: os.Getenv("MYSQL_USER"), Password: os.Getenv("MYSQL_PASSWORD"), Database: name}, false)
	if err != nil {
		t.Fatal(err)
	}
	pool, err := connection.DB()
	if err != nil {
		t.Fatal(err)
	}
	pool.SetMaxOpenConns(6)
	pool.SetMaxIdleConns(2)
	if err = connection.AutoMigrate(&db.GGOFieldLine{}); err != nil {
		t.Fatal(err)
	}
	serverID := time.Now().UnixNano()
	t.Cleanup(func() {
		if err := connection.Where("server_id=?", serverID).Delete(&db.GGOFieldLine{}).Error; err != nil {
			t.Error(err)
		}
		if err := pool.Close(); err != nil {
			t.Error(err)
		}
	})
	cfg := ggoLineTestConfiguration(t)
	start := time.Now().UnixMilli()
	var initial ggoLineState
	err = connection.Transaction(func(tx *gorm.DB) error {
		row, state, _, err := ggoLoadLine(tx, serverID, 1, cfg, "test-revision", start)
		if err != nil {
			return err
		}
		state.Resources[0].Remaining = 1
		initial = *state
		return ggoLineSave(tx, row, state, start)
	})
	if err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	outcomes := make(chan error, 12)
	for i := 1; i <= 12; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			outcomes <- connection.Transaction(func(tx *gorm.DB) error {
				row, state, _, err := ggoLoadLine(tx, serverID, 1, cfg, "test-revision", start)
				if err != nil {
					return err
				}
				n := &state.Resources[0]
				if n.ReservedBy != 0 || n.Remaining == 0 {
					return nil
				}
				n.ReservedBy = int64(id)
				n.AttemptToken = fmt.Sprintf("attempt-%d", id)
				n.ReadyAtMS = start + 3200
				return ggoLineSave(tx, row, state, start)
			})
		}(i)
	}
	wg.Wait()
	close(outcomes)
	for e := range outcomes {
		if e != nil {
			t.Fatal(e)
		}
	}
	var held ggoLineState
	err = connection.Transaction(func(tx *gorm.DB) error {
		_, state, _, err := ggoLoadLine(tx, serverID, 1, cfg, "test-revision", start)
		if err != nil {
			return err
		}
		held = *state
		if held.Resources[0].ReservedBy == 0 || held.Resources[0].Remaining != 1 {
			return fmt.Errorf("last use was not reserved once")
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	// At the two-hour boundary every free node changes generation while the
	// in-flight resource remains frozen until its accepted attempt completes.
	boundary := ggoCycleBoundary(start, cfg.Runtime.Resource.CycleSeconds) + cfg.Runtime.Resource.CycleSeconds*1000
	err = connection.Transaction(func(tx *gorm.DB) error {
		row, state, _, err := ggoLoadLine(tx, serverID, 1, cfg, "test-revision", boundary)
		if err != nil {
			return err
		}
		if state.Resources[0].Generation != held.Resources[0].Generation || state.Resources[0].ReservedBy != held.Resources[0].ReservedBy {
			return fmt.Errorf("boundary changed in-flight resource")
		}
		if state.Resources[1].Generation == initial.Resources[1].Generation {
			return fmt.Errorf("boundary did not refresh free resource")
		}
		n := &state.Resources[0]
		n.ReservedBy = 0
		n.AttemptToken = ""
		n.ReadyAtMS = 0
		n.Remaining = 0
		n.RefreshAtMS = boundary + 90000
		if err = ggoLineSave(tx, row, state, boundary); err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	var latest int64
	err = connection.Transaction(func(tx *gorm.DB) error {
		_, state, _, err := ggoLoadLine(tx, serverID, 1, cfg, "test-revision", boundary+1)
		if err != nil {
			return err
		}
		if state.Resources[0].Generation == held.Resources[0].Generation || state.Resources[0].Remaining != cfg.Runtime.Resource.Capacity {
			return fmt.Errorf("completed old attempt did not transition")
		}
		latest = state.Resources[0].Generation
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	err = connection.Transaction(func(tx *gorm.DB) error {
		_, state, changed, err := ggoLoadLine(tx, serverID, 1, cfg, "test-revision", boundary+2)
		if err != nil {
			return err
		}
		if changed || state.Resources[0].Generation != latest {
			return fmt.Errorf("reload rerolled durable state")
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
