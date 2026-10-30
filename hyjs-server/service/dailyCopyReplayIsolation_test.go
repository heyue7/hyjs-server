package service

import (
	"math"
	"testing"
	"time"

	"google.golang.org/protobuf/encoding/protowire"
	"google.golang.org/protobuf/proto"
	"hyjs-server/db"
	pb "hyjs-server/model/protocol"
)

func TestDailyCopyPlaybackDamageKeepsSettlementGuards(t *testing.T) {
	s, ctx, path := newDailyCopyTest(t)
	id := dailyCopyTestPlayer(t, ctx, "daily-damaged-playback", 100)
	now := time.Date(2026, 10, 13, 12, 0, 0, 0, time.FixedZone("UTC+8", 8*3600))
	started, err := s.dailyCopyAt(ctx, id, &pb.CSDailyCopyFightStart{ChapterId: proto.String("5")}, 301, now)
	if err != nil {
		t.Fatal(err)
	}
	start := started[0].(*pb.SCDailyCopyFightStart)
	end := dailyCopyWin(start, "5")
	for _, mutate := range []func(*pb.CSDailyCopyFightEnd){
		func(x *pb.CSDailyCopyFightEnd) { x.OptRecord = nil },
		func(x *pb.CSDailyCopyFightEnd) { x.OptRecord.DefenderTotalLeftHp = proto.Float64(1) },
		func(x *pb.CSDailyCopyFightEnd) { x.OptRecord.AttackerTotalLeftHp = proto.Float64(math.NaN()) },
	} {
		bad := proto.Clone(end).(*pb.CSDailyCopyFightEnd)
		mutate(bad)
		if _, err = s.dailyCopyAt(ctx, id, bad, 302, now); err == nil {
			t.Fatal("missing or invalid battle result evidence accepted")
		}
	}
	if _, err = s.dailyCopyAt(ctx, id+1, end, 302, now); err == nil {
		t.Fatal("foreign fight owner accepted")
	}
	var fight db.DailyCopyFight
	if err = db.GetIns().First(&fight, "fight_id=?", start.GetFightId()).Error; err != nil {
		t.Fatal(err)
	}
	originalStart := append([]byte(nil), fight.Start...)
	brokenStart := proto.Clone(start).(*pb.SCDailyCopyFightStart)
	brokenStart.Monster.Formation = nil
	brokenStartRaw, _ := proto.Marshal(brokenStart)
	if err = db.GetIns().Model(&db.DailyCopyFight{}).Where("fight_id=?", fight.FightID).Update("start_snapshot", brokenStartRaw).Error; err != nil {
		t.Fatal(err)
	}
	if _, err = s.dailyCopyAt(ctx, id, end, 302, now); err == nil {
		t.Fatal("missing enemy formation accepted")
	}
	if err = db.GetIns().Model(&db.DailyCopyFight{}).Where("fight_id=?", fight.FightID).Update("start_snapshot", originalStart).Error; err != nil {
		t.Fatal(err)
	}
	end.OptRecord.SkillOpts = []*pb.BattleSkillOperate{{RoundNum: proto.Int32(1), RoundTime: proto.Float64(-1), ActorUid: proto.String("foreign")}}
	if validateDailyCopyEnd(start, end) == nil {
		t.Fatal("broken playback incorrectly considered viewable")
	}
	settled, err := s.dailyCopyAt(ctx, id, end, 303, now)
	if err != nil || settled[0].(*pb.SCDailyCopyFightEnd).GetIsWin() != 1 {
		t.Fatal("playback damage blocked valid settlement", err)
	}
	var profile db.Profile
	if err = db.GetIns().First(&profile, id).Error; err != nil {
		t.Fatal(err)
	}
	player, game := *profile.Player, *profile.Game
	if err = db.GetIns().First(&fight, "fight_id=?", start.GetFightId()).Error; err != nil {
		t.Fatal(err)
	}
	var receipt db.DailyCopyStartReceipt
	if err = db.GetIns().First(&receipt, "player_id=? AND stamp=?", id, fight.StartStamp).Error; err != nil {
		t.Fatal(err)
	}
	if _, err = dailyPlaybackProjection(&fight, &receipt); err == nil {
		t.Fatal("damaged playback exposed as playable")
	}
	changed := proto.Clone(end).(*pb.CSDailyCopyFightEnd)
	changed.OptRecord.SkillOpts = nil
	if retry, e := s.dailyCopyAt(ctx, id, changed, 304, now); e != nil || !proto.Equal(settled[0], retry[0]) {
		t.Fatal("same business result with changed playback did not recover", e)
	}
	changed.OptRecord.AttackerTotalLeftHp = proto.Float64(2)
	if _, err = s.dailyCopyAt(ctx, id, changed, 305, now); err == nil {
		t.Fatal("different battle HP accepted as identical receipt")
	}

	// The outer frame and HP remain readable while a nested skill message is broken.
	op := &pb.BattleOperate{FinalRoundNum: end.OptRecord.FinalRoundNum, AttackerTotalLeftHp: end.OptRecord.AttackerTotalLeftHp, DefenderTotalLeftHp: end.OptRecord.DefenderTotalLeftHp}
	opRaw, _ := proto.Marshal(op)
	opRaw = protowire.AppendTag(opRaw, 4, protowire.BytesType)
	opRaw = protowire.AppendBytes(opRaw, []byte{0x80})
	var corrupt []byte
	corrupt = protowire.AppendTag(corrupt, 1, protowire.BytesType)
	corrupt = protowire.AppendString(corrupt, end.GetChapterId())
	corrupt = protowire.AppendTag(corrupt, 2, protowire.VarintType)
	corrupt = protowire.AppendVarint(corrupt, uint64(end.GetIsWin()))
	corrupt = protowire.AppendTag(corrupt, 3, protowire.BytesType)
	corrupt = protowire.AppendBytes(corrupt, opRaw)
	corrupt = protowire.AppendTag(corrupt, 4, protowire.BytesType)
	corrupt = protowire.AppendString(corrupt, end.GetFightId())
	if proto.Unmarshal(corrupt, &pb.CSDailyCopyFightEnd{}) == nil {
		t.Fatal("fixture must damage nested playback")
	}
	if err = db.GetIns().Model(&db.DailyCopyFight{}).Where("fight_id=?", fight.FightID).Update("end_request", corrupt).Error; err != nil {
		t.Fatal(err)
	}
	pool, _ := db.GetIns().DB()
	if err = pool.Close(); err != nil {
		t.Fatal(err)
	}
	if err = db.InitGame(ctx, path); err != nil {
		t.Fatal(err)
	}
	defer func() { reopened, _ := db.GetIns().DB(); _ = reopened.Close() }()
	if retry, e := s.dailyCopyAt(ctx, id, end, 306, now); e != nil || !proto.Equal(settled[0], retry[0]) {
		t.Fatal("damaged stored playback blocked receipt", e)
	}
	if err = db.GetIns().Model(&db.DailyCopyFight{}).Where("fight_id=?", fight.FightID).Update("end_request", []byte{0xff}).Error; err != nil {
		t.Fatal(err)
	}
	if _, err = s.dailyCopyAt(ctx, id, end, 307, now); err == nil {
		t.Fatal("unrecoverable business proof accepted")
	}
	if err = db.GetIns().First(&profile, id).Error; err != nil || *profile.Player != player || *profile.Game != game {
		t.Fatal("retry changed saved assets", err)
	}
}
