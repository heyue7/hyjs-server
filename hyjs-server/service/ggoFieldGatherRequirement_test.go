package service

import (
	"errors"
	"fmt"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

func TestGGOFieldGatherRequirementMessagesAndRollback(t *testing.T) {
	t.Setenv("HYJS_P1_MYSQL_DATABASE", "")
	f := ggoTradeSetup(t)
	if err := db.GetIns().AutoMigrate(&db.GGOFieldLine{}, &db.GemCraftState{}, &db.CookingCraftState{}); err != nil {
		t.Fatal(err)
	}
	f.s.GGOIntegrationReady = true
	type point struct {
		node     ggoResourceNode
		position *model.AdminFieldGGOPosition
	}
	points := map[string]point{}
	now := time.Now().UnixMilli()
	err := db.GetIns().Transaction(func(tx *gorm.DB) error {
		row, state, _, err := ggoLoadLine(tx, f.s.OriginalServerID(), 1, f.config, "isolated-trade-config", now)
		if err != nil {
			return err
		}
		for i := range state.Resources {
			node := &state.Resources[i]
			if _, exists := points[node.Kind]; exists {
				continue
			}
			for _, configured := range f.config.Runtime.Points {
				if configured.ID == node.PointID {
					node.Level = 3
					points[node.Kind] = point{node: *node, position: configured.StandPosition}
					break
				}
			}
		}
		return ggoLineSave(tx, row, state, now)
	})
	if err != nil || len(points) != 2 {
		t.Fatal("fixture points", points, err)
	}
	for index, kind := range []string{"mine", "fish"} {
		id, selected := f.ids[index], points[kind]
		if err := updateProfile(f.ctx, id, func(_ *pb.Player, g *model.GameState) error {
			g.Position.Current = &pb.PointNew{X: proto.Float32(float32(*selected.position.X)), Y: proto.Float32(float32(*selected.position.Y)), Z: proto.Float32(float32(*selected.position.Z))}
			g.Field.Side.Tiredness = proto.Int32(1)
			return nil
		}); err != nil {
			t.Fatal(err)
		}
		request := model.GGOFieldGatherStartRequest{GGOFieldMutation: model.GGOFieldMutation{GGOFieldAuth: f.auth(index), RequestID: fmt.Sprintf("00000000-0000-4000-8000-%012d", index+1)}, PointID: selected.node.PointID, Generation: selected.node.Generation}
		_, err := f.s.GGOFieldGatherStart(f.ctx, request, f.sessions[index])
		name := "冶炼"
		if kind == "fish" {
			name = "料理"
		}
		var biz *model.BizCode
		if !errors.As(err, &biz) || biz.Code != constUtil.GGOFieldInsufficientLife || biz.Msg != fmt.Sprintf("需要%s等级3，当前等级1", name) {
			t.Fatalf("%s level error: %v", kind, err)
		}
		if kind == "mine" {
			err = db.GetIns().Create(&db.GemCraftState{PlayerID: id, Level: 3}).Error
		} else {
			err = db.GetIns().Create(&db.CookingCraftState{PlayerID: id, Level: 3}).Error
		}
		if err != nil {
			t.Fatal(err)
		}
		_, err = f.s.GGOFieldGatherStart(f.ctx, request, f.sessions[index])
		if !errors.As(err, &biz) || biz.Code != constUtil.GGOFieldInsufficientLife || biz.Msg != fmt.Sprintf("疲劳不足，当前1，需要%d", f.config.Runtime.Resource.Fatigue) {
			t.Fatalf("%s fatigue error: %v", kind, err)
		}
		if p1Count(t, &db.P8GatherAttempt{}, "player_id=?", id) != 0 || p1Count(t, &db.GGOFieldReceipt{}, "player_id=?", id) != 0 {
			t.Fatal("rejected request created an attempt or receipt")
		}
		if err := updateProfile(f.ctx, id, func(_ *pb.Player, g *model.GameState) error {
			g.Field.Side.Tiredness = proto.Int32(int32(f.config.Runtime.Resource.Fatigue))
			return nil
		}); err != nil {
			t.Fatal(err)
		}
		accepted, err := f.s.GGOFieldGatherStart(f.ctx, request, f.sessions[index])
		if err != nil || accepted.AttemptToken == "" {
			t.Fatalf("%s corrected retry: %+v %v", kind, accepted, err)
		}
		replayed, err := f.s.GGOFieldGatherStart(f.ctx, request, f.sessions[index])
		if err != nil || replayed.AttemptToken != accepted.AttemptToken {
			t.Fatalf("%s accepted request did not replay: %+v %v", kind, replayed, err)
		}
	}
}
