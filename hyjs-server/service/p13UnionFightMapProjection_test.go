package service

import (
	"encoding/json"
	"math"
	"strings"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	pb "hyjs-server/model/protocol"
)

func TestP13UFMapProjectionLegacyDistanceFlatFactoryAndTrustedTarget(t *testing.T) {
	actor := &db.P13UnionFightFieldPlayer{X: 15, Y: 0, Z: 26}
	// Original UnionFightNpc1 is (15,-4.02,26); legacy scenes keep height.
	if got := p13UFResourceDistance(actor, 15, -4.02, 26); math.Abs(got-4.02) > .00001 {
		t.Fatal("legacy 3D distance changed", got)
	}
	actor.MapProjection = constUtil.P13UnionFightMapProjectionReFlat
	if got := p13UFResourceDistance(actor, 15, -4.02, 26); got != 0 {
		t.Fatal("flat factory NPC inaccessible", got)
	}
	if got := p13UFResourceDistance(actor, 18, -4.02, 30); got != 5 {
		t.Fatal("flat XZ distance bypassed", got)
	}
	trusted := &pb.PointNew{X: proto.Float32(15), Y: proto.Float32(-4.02), Z: proto.Float32(26)}
	flat, err := p13UFTransferProjectedTarget(actor, trusted)
	if err != nil || flat.GetY() != 0 || trusted.GetY() == 0 || flat.GetX() != trusted.GetX() {
		t.Fatal("trusted endpoint projection mutated original", err)
	}
	actor.MapProjection = ""
	legacy, err := p13UFTransferProjectedTarget(actor, trusted)
	if err != nil || !proto.Equal(legacy, trusted) {
		t.Fatal("legacy trusted transfer changed", err)
	}
	actor.MapProjection = "unrecognized"
	if _, err := p13UFTransferProjectedTarget(actor, trusted); err == nil || !math.IsInf(p13UFResourceDistance(actor, 15, 0, 26), 1) {
		t.Fatal("unknown actor projection accepted")
	}
	old, _ := json.Marshal(&p13UFFieldSnapshot{})
	if strings.Contains(string(old), "MapProjection") {
		t.Fatal("empty projection changed legacy snapshot JSON")
	}
}

func TestP13UFMapProjectionFlatActualMovementTransferColdAndFrozenPolicy(t *testing.T) {
	f, ids, first, clock, match := p13UFFieldSetup(t)
	if err := db.GetIns().Model(&db.P13UnionFightFieldPolicy{}).Where("id=1").Updates(map[string]any{"enabled": true, "preparation_millis": 300000, "left_revive_point_id": 1, "right_revive_point_id": 3, "initial_guard_hp": 1000, "initial_gate_hp": 10000, "move_tolerance": 3, "map_projection": constUtil.P13UnionFightMapProjectionReFlat, "source": "isolated explicitly published Re flat projection"}).Error; err != nil {
		t.Fatal(err)
	}
	now := func() time.Time { return time.UnixMilli(clock.Load()) }
	id := ids[0]
	if _, err := f.s.p13UFFieldEnter(f.ctx, id, now()); err != nil {
		t.Fatal(err)
	}
	if _, err := f.s.p13UFFieldFrames(f.ctx, id, &pb.CSCurrentPosition{}, now()); err != nil {
		t.Fatal(err)
	}
	actor, _, snapshot, _, err := f.s.p13UFFieldCurrent(db.GetIns(), id, now())
	if err != nil || actor.MapProjection != constUtil.P13UnionFightMapProjectionReFlat || actor.Y != 0 || snapshot.LeftSpawn.GetY() != 0 || snapshot.RightSpawn.GetY() != 0 {
		t.Fatal("new field did not freeze flat spawn/actor", err)
	}
	original, err := f.s.p13UFFieldOriginal(match, db.GetIns())
	if err != nil {
		t.Fatal(err)
	}
	npc, err := fieldPoint(original.table("UnionFightNpc", "UFNId", "1")["Position"])
	if err != nil || math.Abs(float64(npc.GetY())+4.02) > .00001 {
		t.Fatal("original factory NPC identity changed", err)
	}
	clock.Store(first + 20000)
	point := proto.Clone(npc).(*pb.PointNew)
	point.Y = proto.Float32(.01)
	replies, err := f.s.p13UFFieldMove(f.ctx, id, &pb.CSMove{PointNew: point}, now())
	if err != nil || len(replies) != 1 || replies[0].(*pb.SCUFElementMove).Pos[0].GetY() != 0 || point.GetY() == 0 {
		t.Fatal("flat move did not canonicalize actual response", err)
	}
	actor, _, _, _, err = f.s.p13UFFieldCurrent(db.GetIns(), id, now())
	if err != nil || actor.Y != 0 || p13UFResourceDistance(actor, npc.GetX(), npc.GetY(), npc.GetZ()) != 0 {
		t.Fatal("actual flat player cannot reach original factory NPC", err)
	}
	var profile db.Profile
	if err = db.GetIns().First(&profile, id).Error; err != nil {
		t.Fatal(err)
	}
	_, game, err := decodeProfile(&profile)
	if err != nil || game.Position.GetCurrent().GetY() != 0 {
		t.Fatal("profile retained nonzero plane height", err)
	}
	badY := &pb.PointNew{X: proto.Float32(actor.X), Y: proto.Float32(.051), Z: proto.Float32(actor.Z)}
	if _, err = f.s.p13UFFieldMove(f.ctx, id, &pb.CSMove{PointNew: badY}, now()); err == nil {
		t.Fatal("client height injection accepted")
	}
	jump := &pb.PointNew{X: proto.Float32(actor.X + 30), Y: proto.Float32(0), Z: proto.Float32(actor.Z)}
	if _, err = f.s.p13UFFieldMove(f.ctx, id, &pb.CSMove{PointNew: jump}, now()); err == nil {
		t.Fatal("flat projection bypassed horizontal speed/debt")
	}
	var selected map[string]string
	for _, row := range original.Tables["UnionFightTransfer"] {
		if row["Camp"] == "0" && actor.Camp == 0 || row["Camp"] == "1" && actor.Camp == 1 {
			if row["TransferType"] == "1" {
				selected = row
				break
			}
		}
	}
	if selected == nil {
		t.Fatal("original camp return transfer missing")
	}
	target, err := fieldPoint(selected["TransferEnd"])
	if err != nil {
		t.Fatal(err)
	}
	target.Y = proto.Float32(0)
	request := &pb.CSUFBeginTransfer{TransferId: proto.String(selected["UFTransferId"]), Target: target}
	begin, err := f.s.p13UFTransferBegin(f.ctx, id, request, now())
	if err != nil || begin[0].(*pb.SCUFBeginTransfer).GetTarget().GetY() != 0 {
		t.Fatal("flat transfer begin", err)
	}
	var saved db.P13UnionFightField
	if err = db.GetIns().First(&saved, "match_id=?", match.ID).Error; err != nil {
		t.Fatal(err)
	}
	// Subsequent live-policy edits cannot convert an accepted field/task.
	if err = db.GetIns().Model(&db.P13UnionFightFieldPolicy{}).Where("id=1").Update("map_projection", "").Error; err != nil {
		t.Fatal(err)
	}
	sqlDB, _ := db.GetIns().DB()
	sqlDB.Close()
	f.open(t)
	if _, err = f.s.p13UFFieldEnter(f.ctx, id, now()); err != nil {
		t.Fatal("cold reentry changed frozen projection", err)
	}
	clock.Add(5000)
	end, err := f.s.p13UFTransferEnd(f.ctx, id, &pb.CSUFEndTransfer{Target: target}, now())
	if err != nil || end[0].(*pb.SCUFEndTransfer).GetTarget().GetY() != 0 {
		t.Fatal("cold flat transfer completion", err)
	}
	var current db.P13UnionFightField
	if err = db.GetIns().First(&current, "match_id=?", match.ID).Error; err != nil || current.SnapshotHash != saved.SnapshotHash {
		t.Fatal("live policy rewrote frozen map", err)
	}
	actor, _, _, _, err = f.s.p13UFFieldCurrent(db.GetIns(), id, now())
	if err != nil || actor.Y != 0 || actor.MapProjection != constUtil.P13UnionFightMapProjectionReFlat {
		t.Fatal("cold actor lost flat contract", err)
	}
	if err = db.GetIns().Model(actor).Update("map_projection", "").Error; err != nil {
		t.Fatal(err)
	}
	if _, _, _, _, err = f.s.p13UFFieldCurrent(db.GetIns(), id, now()); err == nil {
		t.Fatal("actor/snapshot projection mismatch accepted")
	}
	if _, err = f.s.p13UFFieldEnter(f.ctx, id, now()); err == nil {
		t.Fatal("reentry silently rewrote mismatched actor")
	}
}

func TestP13UFMapProjectionUnknownNewPolicyAndFrozenValueReject(t *testing.T) {
	f, ids, _, clock, _ := p13UFFieldSetup(t)
	if err := db.GetIns().Model(&db.P13UnionFightFieldPolicy{}).Where("id=1").Updates(map[string]any{"enabled": true, "preparation_millis": 300000, "left_revive_point_id": 1, "right_revive_point_id": 3, "initial_guard_hp": 1000, "initial_gate_hp": 10000, "move_tolerance": 3, "map_projection": "unknown", "source": "isolated unknown projection"}).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := f.s.p13UFFieldEnter(f.ctx, ids[0], time.UnixMilli(clock.Load())); err == nil {
		t.Fatal("unknown newly published projection accepted")
	}
	if err := p13UFFieldProjection(&p13UFFieldSnapshot{MapProjection: "unknown"}); err == nil {
		t.Fatal("unknown frozen projection accepted")
	}
}
