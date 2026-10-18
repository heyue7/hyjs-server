package service

import (
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	"hyjs-server/db"
	pb "hyjs-server/model/protocol"
)

// TestDailyCopyOriginalCampUID preserves real original automatic-AI identities
// while rejecting opposite-camp actors and duplicate aliases in one formation.
func TestDailyCopyOriginalCampUID(t *testing.T) {
	s, ctx, path := newDailyCopyTest(t)
	id := dailyCopyTestPlayer(t, ctx, "daily-original-camp-uid", 100)
	now := time.Date(2026, 10, 14, 12, 0, 0, 0, time.FixedZone("UTC+8", 8*3600))
	starts, err := s.dailyCopyAt(ctx, id, &pb.CSDailyCopyFightStart{ChapterId: proto.String("49")}, 100, now)
	if err != nil {
		t.Fatal(err)
	}
	start := starts[0].(*pb.SCDailyCopyFightStart)
	actor := start.GetBattlePlayer().GetFormation()[0]
	enemy := start.GetMonster().GetFormation()[0]
	finish := dailyCopyWin(start, "49")
	finish.OptRecord.SkillOpts = []*pb.BattleSkillOperate{
		{RoundNum: proto.Int32(1), RoundTime: proto.Float64(0), Camp: proto.Int32(0), ActorUid: proto.String("0_" + actor.GetGuid()), SkillIndex: proto.Int32(0)},
		{RoundNum: proto.Int32(1), RoundTime: proto.Float64(0), Camp: proto.Int32(1), ActorUid: proto.String("1_" + enemy.GetGuid()), SkillIndex: proto.Int32(0)},
	}
	var before db.Profile
	if err := db.GetIns().First(&before, id).Error; err != nil {
		t.Fatal(err)
	}
	for _, invalid := range []string{"1_" + actor.GetGuid(), "0_" + enemy.GetGuid(), "0_foreign-instance"} {
		forged := proto.Clone(finish).(*pb.CSDailyCopyFightEnd)
		forged.OptRecord.SkillOpts[0].ActorUid = proto.String(invalid)
		if _, err := s.dailyCopyAt(ctx, id, forged, 101, now); err == nil {
			t.Fatal("forged original camp/instance accepted", invalid)
		}
	}
	aliases := proto.Clone(finish).(*pb.CSDailyCopyFightEnd)
	aliases.OptRecord.AttackerTransOpt = &pb.BattleTransOperate{RoundNum: proto.Int32(1), ActorUids: []string{actor.GetGuid(), "0_" + actor.GetGuid()}, Fpids: []int32{1, 2}}
	if validateDailyCopyEnd(start, aliases) == nil {
		t.Fatal("one actual actor duplicated through raw/native aliases")
	}
	var after db.Profile
	if err := db.GetIns().First(&after, id).Error; err != nil || *before.Player != *after.Player || *before.Game != *after.Game {
		t.Fatal("rejected original identity changed paid profile", err)
	}
	historical := proto.Clone(finish).(*pb.CSDailyCopyFightEnd)
	historical.OptRecord.SkillOpts[0].ActorUid = actor.Guid
	historical.OptRecord.SkillOpts[1].ActorUid = enemy.Guid
	if err := validateDailyCopyEnd(start, historical); err != nil {
		t.Fatal("historical raw GUID identity rejected", err)
	}
	settled, err := s.dailyCopyAt(ctx, id, finish, 102, now)
	if err != nil {
		t.Fatal("original camp_GUID operations rejected", err)
	}
	connection, _ := db.GetIns().DB()
	connection.Close()
	if err := db.InitGame(ctx, path); err != nil {
		t.Fatal(err)
	}
	defer func() { pool, _ := db.GetIns().DB(); pool.Close() }()
	replay, err := s.dailyCopyAt(ctx, id, finish, 103, now)
	if err != nil || !proto.Equal(settled[0], replay[0]) {
		t.Fatal("original operations cold receipt changed", err)
	}
	if _, err := s.dailyCopyAt(ctx, id, historical, 104, now); err == nil {
		t.Fatal("stored original operation bytes silently rewritten as a different receipt")
	}
	var quota db.DailyCopyState
	if err := db.GetIns().First(&quota, "player_id=? AND day=? AND type=?", id, dailyCopyDay(now), "PetEquip").Error; err != nil || quota.Used != 1 || quota.Limit != 3 {
		t.Fatal("same original end/cold consumed another actual daily quota", err)
	}
}
