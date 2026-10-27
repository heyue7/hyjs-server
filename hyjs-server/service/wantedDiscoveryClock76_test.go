package service

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

// A real sweep after a GM date shift must create a visible Wanted
// instance on the same game clock used by the original list/start consumers.
func TestWantedDiscovery76SweepUsesShiftedGameClockAndReplay(t *testing.T) {
	f, admin := wantedClock75Setup(t)
	for _, initialize := range []func(context.Context) error{f.s.InitializeCopyDrops, f.s.InitializeP4CopyBenefits, f.s.InitializeP9CopyIntimacy} {
		if err := initialize(f.ctx); err != nil {
			t.Fatal(err)
		}
	}
	var policy db.P12WantedPolicy
	if err := db.GetIns().First(&policy, 1).Error; err != nil {
		t.Fatal(err)
	}
	var def p12WantedDefinition
	if err := json.Unmarshal([]byte(policy.DefinitionJSON), &def); err != nil {
		t.Fatal(err)
	}
	for _, row := range def.Tables["WantedSpecialTime"] {
		row["Odd"] = "100"
	}
	body, err := json.Marshal(def)
	if err != nil {
		t.Fatal(err)
	}
	if err = db.GetIns().Model(&policy).Updates(map[string]any{"definition_json": string(body), "definition_hash": p6Digest(body)}).Error; err != nil {
		t.Fatal(err)
	}
	if err = updateProfile(f.ctx, f.id, func(p *pb.Player, g *model.GameState) error {
		p.Level = proto.Int32(100)
		p.Stamina = proto.Int32(100)
		f.s.refreshCopyProgress(g)
		for _, v := range g.Copy.PassedCopyInfo {
			if v.GetChapterId() == "1" && v.GetMissionId() == "1" {
				v.Star = proto.Int32(5)
			}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	ctx := wantedClock75Move(t, f, admin, 1)
	req := &pb.CSCopySweep{ChapterId: proto.String("1"), MissionId: proto.String("1")}
	ctx = context.WithValue(ctx, constUtil.GameRequestContextKey, req)
	real := time.Now()
	out, err := f.s.copySweepAt(ctx, f.id, req, 760001, real)
	if err != nil {
		t.Fatal("original sweep", err)
	}
	if out[0].(*pb.SCCopySweep).GetWantedId() == "" {
		t.Fatal("isolated guaranteed fixture did not discover")
	}
	var instance db.P12WantedInstance
	if err = db.GetIns().Where("finder_id=?", f.id).First(&instance).Error; err != nil {
		t.Fatal(err)
	}
	gameNow := LocalPlayAt(ctx, real)
	if instance.CreatedAt != gameNow.UnixMilli() || instance.ExpiresAt <= gameNow.UnixMilli() {
		t.Fatalf("discovered instance not in game clock: created=%d expires=%d game=%d", instance.CreatedAt, instance.ExpiresAt, gameNow.UnixMilli())
	}
	list, err := f.s.p12WantedList(ctx, f.id, gameNow)
	if err != nil {
		t.Fatal(err)
	}
	if len(list[0].(*pb.SCGetRebels).GetRebels()) != 1 {
		t.Fatal("newly discovered Wanted missing from list")
	}
	before, _ := f.profile(t)
	replay, err := f.s.copySweepAt(ctx, f.id, req, 760001, real)
	after, _ := f.profile(t)
	if err != nil || !proto.Equal(out[0], replay[0]) || !proto.Equal(before, after) {
		t.Fatal("sweep replay changed existing reward or discovery", err)
	}
	ctx = wantedClock75Move(t, f, admin, -1)
	ctx = context.WithValue(ctx, constUtil.GameRequestContextKey, req)
	list, err = f.s.p12WantedList(ctx, f.id, LocalPlayNow(ctx))
	if err != nil || len(list[0].(*pb.SCGetRebels).GetRebels()) != 0 {
		t.Fatal("future discovered instance was visible after rewind", err)
	}
	next, err := f.s.copySweepAt(ctx, f.id, req, 760002, time.Now())
	if err != nil || next[0].(*pb.SCCopySweep).GetWantedId() == "" {
		t.Fatal("future instance blocked current-day natural discovery", err)
	}
	var unchanged db.P12WantedInstance
	if err = db.GetIns().First(&unchanged, "id=?", instance.ID).Error; err != nil || unchanged.CreatedAt != instance.CreatedAt || unchanged.ExpiresAt != instance.ExpiresAt || unchanged.CurrentHP != instance.CurrentHP {
		t.Fatal("rewind rewrote historical wanted instance", err)
	}
}
