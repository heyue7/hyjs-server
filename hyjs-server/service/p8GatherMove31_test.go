package service

import (
	"context"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

// TestP8GatherMove31ProtocolStopFrames uses original tables and protocol consumers.
// Only the isolated timer is advanced; resource rolls and inventory writes remain real.
func TestP8GatherMove31ProtocolStopFrames(t *testing.T) {
	ctx := context.Background()
	if err := db.InitGame(ctx, filepath.Join(t.TempDir(), "gather-move31.sqlite")); err != nil {
		t.Fatal(err)
	}
	pool, err := db.GetIns().DB()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { pool.Close() })
	s := &GameService{}
	if err = s.LoadTables("../../analysis/tables"); err != nil {
		t.Fatal(err)
	}
	if err = s.LoadP8Tables("../../analysis/tables"); err != nil {
		t.Fatal(err)
	}
	for _, init := range []func(context.Context) error{s.InitializeP1, s.InitializeP8Gather, s.InitGems, s.InitCooking} {
		if err = init(ctx); err != nil {
			t.Fatal(err)
		}
	}
	s.P8GatherReady = true
	stamp := int64(310000)
	state := func(id int64) (*model.GameState, db.Profile) {
		t.Helper()
		var row db.Profile
		if err := db.GetIns().First(&row, id).Error; err != nil {
			t.Fatal(err)
		}
		_, g, err := decodeProfile(&row)
		if err != nil {
			t.Fatal(err)
		}
		return g, row
	}
	balances := func(g *model.GameState) map[string]int64 {
		out := map[string]int64{}
		for _, list := range [][]*pb.SimpleItem{g.Items, g.Gems, g.Cooking} {
			for _, item := range list {
				out[item.GetId()] += int64(item.GetNum())
			}
		}
		return out
	}
	for _, kind := range []string{"fish", "mine"} {
		t.Run(kind, func(t *testing.T) {
			login, err := LoginAccount(ctx, "gather-move31-"+kind, "isolated-password")
			if err != nil {
				t.Fatal(err)
			}
			if _, err = CreatePlayer(ctx, login.Id, "采集停止帧代表", "hero_043"); err != nil {
				t.Fatal(err)
			}
			id := login.Id
			if err = updateProfile(ctx, id, func(p *pb.Player, g *model.GameState) error { p.Level = proto.Int32(30); return nil }); err != nil {
				t.Fatal(err)
			}
			if err = db.GetIns().Save(&db.GemCraftState{PlayerID: id, Level: 2}).Error; err != nil {
				t.Fatal(err)
			}
			if err = db.GetIns().Save(&db.CookingCraftState{PlayerID: id, Level: 2}).Error; err != nil {
				t.Fatal(err)
			}
			if _, err = s.fieldTransfer(ctx, id, &pb.CSTransferScene{Target: proto.String("field_001")}); err != nil {
				t.Fatal(err)
			}
			if err = updateProfileWithTx(ctx, id, func(tx *gorm.DB, p *pb.Player, g *model.GameState) error {
				_, e := s.RefreshP8Resources(tx, p, g, time.Now(), true)
				g.PositionLoading = false
				return e
			}); err != nil {
				t.Fatal(err)
			}
			nodes, err := P8GatherNodes(db.GetIns(), id)
			if err != nil {
				t.Fatal(err)
			}
			var node db.P8ResourceNode
			var cfg *p8GatherConfig
			for _, n := range nodes {
				c, e := p8DecodeConfig(n.Snapshot)
				if e != nil {
					t.Fatal(e)
				}
				if c.Kind == kind && n.Remaining > 1 {
					node = n
					cfg = c
					break
				}
			}
			if cfg == nil {
				t.Fatal("missing original resource kind", kind)
			}
			if cfg.Fatigue != 2 {
				t.Fatal("unexpected original fatigue", cfg.Fatigue)
			}
			if err = updateProfile(ctx, id, func(_ *pb.Player, g *model.GameState) error {
				g.Position.Current = proto.Clone(cfg.Point).(*pb.PointNew)
				return nil
			}); err != nil {
				t.Fatal(err)
			}
			call := func(request proto.Message, requestStamp int64) []proto.Message {
				t.Helper()
				raw, e := proto.Marshal(request)
				if e != nil {
					t.Fatal(e)
				}
				decoded := request.ProtoReflect().Type().New().Interface()
				if e = proto.Unmarshal(raw, decoded); e != nil {
					t.Fatal(e)
				}
				_, profile := state(id)
				out, e := s.dispatch(ctx, &model.GamePacket{PlayerId: id, ZoneId: int32(s.OriginalServerID()), Session: profile.Session, Timestamp: requestStamp, Body: decoded}, "")
				if e != nil {
					t.Fatalf("protocol %T: %v", request, e)
				}
				for _, message := range out {
					if failure, ok := message.(*pb.ErrorMessage); ok {
						t.Fatal("protocol error", failure)
					}
				}
				return out
			}
			next := func(request proto.Message) []proto.Message { stamp++; return call(request, stamp) }
			attempt := func() db.P8GatherAttempt {
				t.Helper()
				var a db.P8GatherAttempt
				if e := db.GetIns().First(&a, "player_id=?", id).Error; e != nil {
					t.Fatal(e)
				}
				return a
			}
			next(&pb.CSFieldResourceStartGather{PosId: proto.String(node.PosID)})
			frozen := attempt()
			if !frozen.Active || frozen.ReadyAt-frozen.StartedAt != cfg.GatherMS {
				t.Fatal("original start was not frozen", frozen)
			}
			// Preserve both the identical stop pose and a final point inside the original radius.
			for _, offset := range []float32{0, float32(cfg.Radius / 2)} {
				point := proto.Clone(cfg.Point).(*pb.PointNew)
				point.X = proto.Float32(point.GetX() + offset)
				next(&pb.CSMove{PointNew: point})
				actual := attempt()
				if !actual.Active || actual.Token != frozen.Token || actual.ReadyAt != frozen.ReadyAt || actual.Snapshot != frozen.Snapshot || actual.Rewards != frozen.Rewards {
					t.Fatal("stop frame replaced or cancelled frozen attempt", actual)
				}
			}
			before, _ := state(id)
			beforeBalance := balances(before)
			if result := db.GetIns().Model(&db.P8GatherAttempt{}).Where("player_id=? AND active=?", id, true).Update("ready_at", time.Now().UnixMilli()-1); result.Error != nil || result.RowsAffected != 1 {
				t.Fatal("isolated timer advance", result.Error)
			}
			stamp++
			claimStamp := stamp
			out := call(&pb.CSFieldGetResource{}, claimStamp)
			var reward *pb.SCFieldGetResource
			for _, message := range out {
				if v, ok := message.(*pb.SCFieldGetResource); ok {
					reward = v
				}
			}
			if reward == nil || reward.GetCode() != constUtil.P8ResourceAvailable || len(reward.GetCost()) != 1 || reward.GetCost()[0].GetNum() != 2 {
				t.Fatal("actual resource claim failed", reward)
			}
			after, _ := state(id)
			afterBalance := balances(after)
			if after.Field.Side.GetTiredness() != before.Field.Side.GetTiredness()-2 {
				t.Fatal("claim fatigue did not decrease exactly two")
			}
			var current db.P8ResourceNode
			if err = db.GetIns().First(&current, "player_id=? AND scene=? AND pos_id=?", id, node.Scene, node.PosID).Error; err != nil {
				t.Fatal(err)
			}
			if current.Remaining != node.Remaining-1 || attempt().Active {
				t.Fatal("claim did not consume exactly one node use")
			}
			for _, spec := range strings.Split(frozen.Rewards, ";") {
				if spec == "" {
					continue
				}
				pair := strings.Split(spec, ",")
				amount, e := strconv.ParseInt(pair[1], 10, 64)
				if e != nil {
					t.Fatal(e)
				}
				if afterBalance[pair[0]]-beforeBalance[pair[0]] != amount {
					t.Fatal("frozen rolled reward missing from actual inventory", spec)
				}
			}
			// Original zero-yield mine rolls remain valid and are never replaced by a fabricated reward.
			t.Logf("original %s frozen reward=%q", kind, frozen.Rewards)
			call(&pb.CSFieldGetResource{}, claimStamp)
			next(&pb.CSFieldGetResource{})
			repeated, _ := state(id)
			if repeated.Field.Side.GetTiredness() != after.Field.Side.GetTiredness() {
				t.Fatal("repeat claim charged fatigue")
			}
			for resource, count := range balances(repeated) {
				if count != afterBalance[resource] {
					t.Fatal("repeat claim granted resource again", resource)
				}
			}
			if err = db.GetIns().First(&current, "player_id=? AND scene=? AND pos_id=?", id, node.Scene, node.PosID).Error; err != nil {
				t.Fatal(err)
			}
			if current.Remaining != node.Remaining-1 {
				t.Fatal("repeat claim consumed node again")
			}
			next(&pb.CSFieldResourceStartGather{PosId: proto.String(node.PosID)})
			restarted := attempt()
			if !restarted.Active || restarted.Token == frozen.Token {
				t.Fatal("new start did not create a new attempt")
			}
			outside := proto.Clone(cfg.Point).(*pb.PointNew)
			outside.X = proto.Float32(outside.GetX() + float32(cfg.Radius) + 1)
			next(&pb.CSMove{PointNew: outside})
			if attempt().Active {
				t.Fatal("moving outside original radius did not cancel")
			}
			next(&pb.CSFieldGetResource{})
			cancelled, _ := state(id)
			if cancelled.Field.Side.GetTiredness() != after.Field.Side.GetTiredness() {
				t.Fatal("cancelled claim charged fatigue")
			}
			for resource, count := range balances(cancelled) {
				if count != afterBalance[resource] {
					t.Fatal("cancelled claim granted resources", resource)
				}
			}
			if err = db.GetIns().First(&current, "player_id=? AND scene=? AND pos_id=?", id, node.Scene, node.PosID).Error; err != nil {
				t.Fatal(err)
			}
			if current.Remaining != node.Remaining-1 {
				t.Fatal("cancelled claim consumed node")
			}
		})
	}
}
