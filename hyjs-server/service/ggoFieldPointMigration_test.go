package service

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"sort"
	"testing"
	"time"

	"gorm.io/gorm"
	"hyjs-server/db"
)

func TestGGOFieldPointMigrationPreservesState(t *testing.T) {
	t.Setenv("HYJS_P1_MYSQL_DATABASE", "")
	f := p1Setup(t)
	database := db.GetIns()
	if err := database.AutoMigrate(&db.GGOFieldLine{}, &db.GGOFieldPoint{}); err != nil {
		t.Fatal(err)
	}
	cfg := ggoLineTestConfiguration(t)
	const revision = "point-migration-test"
	const line int64 = 1
	serverID := f.s.OriginalServerID()
	now := time.Now().UnixMilli()

	var sourcePayload []byte
	var sourceRevision int64
	var selectedID string
	var reservedResource ggoResourceNode
	var reservedMonster ggoMonsterNode
	if err := database.Transaction(func(tx *gorm.DB) error {
		row, state, _, err := ggoLoadLegacyLine(tx, serverID, line, cfg, revision, now)
		if err != nil {
			return err
		}
		if row.PointStateReady || state.pointRows != nil || len(state.Resources) < 3 || len(state.Monsters) < 2 {
			return fmt.Errorf("invalid legacy line fixture")
		}
		state.Resources[0].ReservedBy = f.id
		state.Resources[0].AttemptToken = "migration-gather"
		state.Resources[0].ReadyAtMS = now + 3000
		reservedResource = state.Resources[0]
		selectedID = state.Resources[1].PointID
		if state.Resources[1].ReservedBy != 0 || state.Resources[1].Remaining < 1 {
			return fmt.Errorf("selected resource is unavailable")
		}
		state.Monsters[0].ReservedBy = f.id
		state.Monsters[0].FightID = "migration-fight"
		state.Monsters[0].ReservedUntilMS = 0
		state.Monsters[0].PatrolPausedAtMS = now
		reservedMonster = state.Monsters[0]
		state.LegacyBlocks = []ggoLegacyBlock{
			{PointID: state.Monsters[1].PointID, Kind: "monster", PlayerID: f.id, Token: "legacy-fight", Generation: state.Monsters[1].Generation},
			{PointID: state.Resources[2].PointID, Kind: "gather", PlayerID: f.id, Token: "legacy-gather", Generation: state.Resources[2].Generation},
		}
		state.NextGeneration += 10
		if err := ggoValidateLineState(state, cfg); err != nil {
			return err
		}
		if err := ggoLineSave(tx, row, state, now); err != nil {
			return err
		}
		sourcePayload = append([]byte(nil), row.Payload...)
		sourceRevision = row.Revision
		return nil
	}); err != nil {
		t.Fatal(err)
	}

	publicState := func(state *ggoLineState) ggoLineState {
		t.Helper()
		raw, err := json.Marshal(state)
		if err != nil {
			t.Fatal(err)
		}
		var result ggoLineState
		if err := json.Unmarshal(raw, &result); err != nil {
			t.Fatal(err)
		}
		sort.Slice(result.Resources, func(i, j int) bool { return result.Resources[i].PointID < result.Resources[j].PointID })
		sort.Slice(result.Monsters, func(i, j int) bool { return result.Monsters[i].PointID < result.Monsters[j].PointID })
		sort.Slice(result.LegacyBlocks, func(i, j int) bool { return result.LegacyBlocks[i].PointID < result.LegacyBlocks[j].PointID })
		return result
	}
	var source ggoLineState
	if err := json.Unmarshal(sourcePayload, &source); err != nil {
		t.Fatal(err)
	}
	expected := publicState(&source)
	foreign := db.GGOFieldLine{ServerID: serverID + 1, Line: line, Revision: 1, Payload: []byte("foreign realm must remain untouched")}
	if err := database.Create(&foreign).Error; err != nil {
		t.Fatal(err)
	}

	readSnapshot := func() (*db.GGOFieldLine, *ggoLineState) {
		t.Helper()
		var row *db.GGOFieldLine
		var state *ggoLineState
		err := database.Transaction(func(tx *gorm.DB) error {
			var err error
			row, state, _, err = ggoReadLineSnapshot(tx, serverID, line, cfg, revision, now)
			return err
		})
		if err != nil {
			t.Fatal(err)
		}
		return row, state
	}
	checkStoredLine := func() {
		t.Helper()
		var row db.GGOFieldLine
		if err := database.First(&row, "server_id=? AND line=?", serverID, line).Error; err != nil {
			t.Fatal(err)
		}
		if !row.PointStateReady || row.Revision != sourceRevision || !bytes.Equal(row.Payload, sourcePayload) {
			t.Fatalf("legacy line changed during point migration or update: ready=%v revision=%d, want %d", row.PointStateReady, row.Revision, sourceRevision)
		}
	}

	for attempt := 0; attempt < 2; attempt++ {
		if err := migrateGGOFieldPoints(database, serverID); err != nil {
			t.Fatal(err)
		}
		row, state := readSnapshot()
		if row.Revision != sourceRevision {
			t.Fatalf("public revision after migration %d = %d, want %d", attempt+1, row.Revision, sourceRevision)
		}
		if state.NextGeneration != source.NextGeneration {
			t.Fatalf("generation high-water mark = %d, want %d", state.NextGeneration, source.NextGeneration)
		}
		if actual := publicState(state); !reflect.DeepEqual(actual, expected) {
			t.Fatalf("public state changed during migration %d", attempt+1)
		}
		checkStoredLine()
	}

	if err := database.Transaction(func(tx *gorm.DB) error {
		row, state, changed, err := ggoLoadLine(tx, serverID, line, cfg, revision, now, selectedID)
		if err != nil {
			return err
		}
		if changed || len(state.Resources) != 1 || state.Resources[0].PointID != selectedID || state.Resources[0].ReservedBy != 0 || state.Resources[0].Remaining < 1 || len(state.Monsters) != 0 {
			return fmt.Errorf("selected resource load is invalid")
		}
		state.Resources[0].Remaining--
		return ggoLineSave(tx, row, state, now)
	}); err != nil {
		t.Fatal(err)
	}
	row, state := readSnapshot()
	if row.Revision != sourceRevision+1 {
		t.Fatalf("public revision after point update = %d, want %d", row.Revision, sourceRevision+1)
	}
	for i := range expected.Resources {
		if expected.Resources[i].PointID == selectedID {
			expected.Resources[i].Remaining--
		}
	}
	actual := publicState(state)
	if !reflect.DeepEqual(actual, expected) {
		t.Fatal("point update changed unrelated public state")
	}
	var foundResource, foundMonster bool
	for _, node := range actual.Resources {
		if node.PointID == reservedResource.PointID {
			foundResource = true
			if !reflect.DeepEqual(node, reservedResource) {
				t.Fatal("reserved resource changed during another point update")
			}
		}
	}
	for _, node := range actual.Monsters {
		if node.PointID == reservedMonster.PointID {
			foundMonster = true
			if !reflect.DeepEqual(node, reservedMonster) {
				t.Fatal("reserved monster changed during another point update")
			}
		}
	}
	if !foundResource || !foundMonster {
		t.Fatal("reserved resource or monster missing after point update")
	}
	checkStoredLine()
	for attempt, wantCount := range []int{1, 0} {
		restored, err := RestoreGGOFieldLinePayloads(context.Background(), serverID)
		if err != nil || restored != wantCount {
			t.Fatalf("restore attempt %d: count=%d want=%d err=%v", attempt, restored, wantCount, err)
		}
	}
	var restored db.GGOFieldLine
	if err := database.First(&restored, "server_id=? AND line=?", serverID, line).Error; err != nil {
		t.Fatal(err)
	}
	var restoredState ggoLineState
	if restored.PointStateReady || restored.Revision != sourceRevision+1 || json.Unmarshal(restored.Payload, &restoredState) != nil || !reflect.DeepEqual(publicState(&restoredState), expected) {
		t.Fatal("legacy restoration lost current points, revision or reservation state")
	}
	if err := migrateGGOFieldPoints(database, serverID); err != nil {
		t.Fatal(err)
	}
	row, state = readSnapshot()
	if row.Revision != sourceRevision+1 || !reflect.DeepEqual(publicState(state), expected) {
		t.Fatal("migration after legacy restore lost current public state")
	}
	var untouched db.GGOFieldLine
	if err := database.First(&untouched, "server_id=? AND line=?", foreign.ServerID, line).Error; err != nil || untouched.PointStateReady || !bytes.Equal(untouched.Payload, foreign.Payload) {
		t.Fatalf("another realm was migrated: %v", err)
	}
}
