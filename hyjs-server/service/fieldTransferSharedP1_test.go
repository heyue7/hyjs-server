package service

import (
	"context"
	"os"
	"sync"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

// A real active mine is cancelled by each ordinary field-to-town transfer.
// A second shared reader stays open until the transfer and its checks finish.
func TestFieldReturnSharedP1RealMySQL(t *testing.T) {
	if os.Getenv("HYJS_P4_MYSQL_DATABASE") == "" {
		t.Skip("requires an isolated hyjs_test_p4_ MySQL database")
	}
	f := p4Setup(t)
	if err := f.s.LoadP8Tables("../../analysis/tables"); err != nil {
		t.Fatal(err)
	}
	for _, initialize := range []func(context.Context) error{f.s.InitializeP8Gather, f.s.InitGems} {
		if err := initialize(f.ctx); err != nil {
			t.Fatal(err)
		}
	}
	f.s.P8GatherReady = true
	if err := db.GetIns().Save(&db.GemCraftState{PlayerID: f.id, Level: 6}).Error; err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.WithValue(f.ctx, constUtil.GameServiceContextKey, f.s), 45*time.Second)
	defer cancel()
	if err := updateProfileWithTx(ctx, f.id, func(_ *gorm.DB, player *pb.Player, _ *model.GameState) error {
		player.Level = proto.Int32(60)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	database := db.GetIns()
	previous := database.Config.Logger
	trace := &loginPeriodLockTrace{Interface: previous}
	database.Config.Logger = trace
	defer func() { database.Config.Logger = previous }()
	for _, scene := range []string{"field_001", "field_002"} {
		t.Run(scene, func(t *testing.T) {
			if _, err := f.s.fieldTransfer(ctx, f.id, &pb.CSTransferScene{Target: proto.String(scene)}); err != nil {
				t.Fatal(err)
			}
			if err := updateProfileWithTx(ctx, f.id, func(tx *gorm.DB, player *pb.Player, game *model.GameState) error {
				game.PositionLoading = false
				_, err := f.s.RefreshP8Resources(tx, player, game, time.Now(), true)
				return err
			}); err != nil {
				t.Fatal(err)
			}
			nodes, err := P8GatherNodes(database, f.id)
			if err != nil {
				t.Fatal(err)
			}
			var selected *db.P8ResourceNode
			var cfg *p8GatherConfig
			for i := range nodes {
				candidate, e := p8DecodeConfig(nodes[i].Snapshot)
				if e != nil {
					t.Fatal(e)
				}
				if nodes[i].Scene == scene && candidate.Kind == "mine" && nodes[i].Remaining > 0 {
					selected, cfg = &nodes[i], candidate
					break
				}
			}
			if selected == nil {
				t.Fatal("fixture has no available mine")
			}
			if err := updateProfileWithTx(ctx, f.id, func(_ *gorm.DB, _ *pb.Player, game *model.GameState) error {
				game.Position.Current = proto.Clone(cfg.Point).(*pb.PointNew)
				game.Field.Side.Tiredness = proto.Int32(200)
				return nil
			}); err != nil {
				t.Fatal(err)
			}
			start, err := f.s.p8StartGather(ctx, f.id, &pb.CSFieldResourceStartGather{PosId: proto.String(selected.PosID)}, 0)
			if err != nil || start[0].(*pb.SCFieldResourceStartGather).GetResult() != constUtil.P8ResourceAvailable {
				t.Fatalf("start mine: %v", err)
			}
			var attempt db.P8GatherAttempt
			if err := database.First(&attempt, "player_id=? AND active=?", f.id, true).Error; err != nil {
				t.Fatal(err)
			}

			acquired, release := make(chan struct{}), make(chan struct{})
			holder := make(chan error, 1)
			var releaseOnce sync.Once
			defer releaseOnce.Do(func() { close(release) })
			go func() {
				shared := context.WithValue(ctx, constUtil.P1SharedReadContextKey{}, true)
				holder <- database.WithContext(shared).Transaction(func(tx *gorm.DB) error {
					if _, err := p1LockPeriods(tx); err != nil {
						return err
					}
					close(acquired)
					select {
					case <-release:
						return nil
					case <-ctx.Done():
						return ctx.Err()
					}
				})
			}()
			select {
			case <-acquired:
			case err := <-holder:
				t.Fatalf("shared holder failed: %v", err)
			case <-ctx.Done():
				t.Fatal("shared holder timed out")
			}
			trace.shared.Store(0)
			trace.exclusive.Store(0)
			done := make(chan error, 1)
			go func() {
				_, err := f.s.fieldTransfer(ctx, f.id, &pb.CSTransferScene{Target: proto.String("town")})
				done <- err
			}()
			select {
			case err := <-done:
				if err != nil {
					t.Fatal(err)
				}
			case <-time.After(4 * time.Second):
				t.Fatal("field return blocked behind an unrelated shared reader")
			}
			if trace.shared.Load() == 0 || trace.exclusive.Load() != 0 {
				t.Fatalf("unexpected window locks: shared=%d exclusive=%d", trace.shared.Load(), trace.exclusive.Load())
			}
			releaseOnce.Do(func() { close(release) })
			if err := <-holder; err != nil {
				t.Fatal(err)
			}
			var persisted db.Profile
			if err := database.First(&persisted, f.id).Error; err != nil {
				t.Fatal(err)
			}
			_, game, err := decodeProfile(&persisted)
			gate, _ := fieldPoint("41.11,-2.25,-42.33")
			if err != nil || game.Position.GetScene() != "town" || !proto.Equal(game.Position.GetCurrent(), gate) || !game.PositionLoading {
				t.Fatalf("return did not preserve the original gate and loading state: %v", err)
			}
			var cancelled db.P8GatherAttempt
			if err := database.First(&cancelled, "player_id=?", f.id).Error; err != nil || cancelled.Active || cancelled.Token != attempt.Token {
				t.Fatalf("active mine was not cancelled exactly once: %+v, %v", cancelled, err)
			}
		})
	}
}
