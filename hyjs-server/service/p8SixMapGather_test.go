package service

import (
	"context"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"hyjs-server/db"
)

// TestP8SixMapOriginalNodes preserves every original candidate and each map's
// independent budget/rebirth. It neither selects a desired reward nor fabricates
// scene assets, and leaves real transfer/acquisition to the source-chain test.
func TestP8SixMapOriginalNodes(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "six-map-resources.db")
	if err := db.InitGame(ctx, path); err != nil {
		t.Fatal(err)
	}
	defer func() { pool, _ := db.GetIns().DB(); pool.Close() }()
	s := &GameService{}
	for _, load := range []func(string) error{s.LoadTables, s.LoadP8Tables} {
		if err := load("../../analysis/tables"); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.InitializeP8Gather(ctx); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UnixMilli()
	count := 0
	for index := 1; index <= 6; index++ {
		scene := "field_00" + strconv.Itoa(index)
		row := s.p8GatherMap(scene)
		if row == nil {
			t.Fatal("original scene catalogue missing", scene)
		}
		expected := map[string]bool{}
		for _, key := range []string{"MinePoints", "FishPoints"} {
			for _, pos := range strings.Split(row[key], ";") {
				expected[pos] = true
				node, err := s.p8SpawnNode(1, scene, pos, 1)
				if err != nil || node.Scene != scene || node.PosID != pos {
					t.Fatal("original full candidate failed", scene, pos, err)
				}
				count++
			}
		}
		nodes, changed, err := s.p8Nodes(db.GetIns(), 1, scene, now)
		maximum := int(tableNumber(row, "MineMax") + tableNumber(row, "FishGroupMax"))
		if err != nil || !changed || len(nodes) != maximum {
			t.Fatal("original scene budget", scene, len(nodes), maximum, err)
		}
		seen := map[string]bool{}
		for _, node := range nodes {
			if node.Scene != scene || !expected[node.PosID] || seen[node.PosID] {
				t.Fatal("cross-map or duplicate point", node)
			}
			seen[node.PosID] = true
			cfg, err := p8DecodeConfig(node.Snapshot)
			if err != nil || cfg.Level < 1 || cfg.Level > 6 || cfg.GatherMS != 5000 && cfg.GatherMS != 3200 || node.Remaining != 5 {
				t.Fatal("original generation parameters", err)
			}
		}
		again, changed, err := s.p8Nodes(db.GetIns(), 1, scene, now)
		if err != nil || changed || len(again) != maximum {
			t.Fatal("repeat query respawned original live nodes", err)
		}
		target := nodes[0]
		if err := db.GetIns().Model(&db.P8ResourceNode{}).Where("player_id=? AND scene=? AND pos_id=?", 1, scene, target.PosID).Updates(map[string]any{"remaining": 0, "refresh_at": now - 1}).Error; err != nil {
			t.Fatal(err)
		}
		restored, changed, err := s.p8Nodes(db.GetIns(), 1, scene, now)
		if err != nil || !changed || len(restored) != maximum {
			t.Fatal("original same-map rebirth", err)
		}
		for _, node := range restored {
			if node.PosID == target.PosID && (node.Generation != 2 || node.Scene != scene || node.Remaining != 5) {
				t.Fatal("rebirth moved or changed scene", node)
			}
		}
	}
	if count != 190 {
		t.Fatal("full original candidate count changed", count)
	}
	if _, err := s.p8SpawnNode(1, "field_002", "fish_pos_1", 1); err == nil {
		t.Fatal("foreign original point accepted for another scene")
	}
	if _, _, err := s.p8Nodes(db.GetIns(), 1, "field_missing", now); err == nil {
		t.Fatal("missing scene fabricated nodes")
	}
	t.Log("all 130 mine and 60 fish original candidates; six budgets, stable queries and per-map rebirth verified")
}
