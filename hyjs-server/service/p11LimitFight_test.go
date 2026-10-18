package service

import (
	"encoding/json"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	"hyjs-server/db"
	pb "hyjs-server/model/protocol"
)

func TestP11LimitFightOriginalQueryFreezeAndIndependentProofRollback(t *testing.T) {
	f, b, c := p11ArenaSetup(t)
	now := time.Now().Truncate(time.Millisecond)
	if err := f.s.InitializeP11LimitFight(f.ctx); err != nil {
		t.Fatal(err)
	}
	for _, table := range []any{&db.P11LimitFightSettlement{}, &db.P11LimitFightMatch{}, &db.P11LimitFightOffer{}, &db.P11LimitFightPolicy{}} {
		if err := db.GetIns().Where("1=1").Delete(table).Error; err != nil {
			t.Fatal(err)
		}
	}
	if err := f.s.InitializeP11LimitFight(f.ctx); err != nil {
		t.Fatal(err)
	}
	p11OpenLimit(t, now, "finite-mode20-real-members")
	for i, id := range []int64{b, c, f.id} {
		p11LimitCall(t, f, id, &pb.CSLimitPvpFindMatch{}, int64(8000+i), now)
	}
	var count int64
	db.GetIns().Model(&db.P11LimitFightMatch{}).Count(&count)
	if count != 0 {
		t.Fatal("defaultoff query froze a battle", count)
	}
	req := &pb.CSLimitPvpFightResult{PlayerId: proto.Int64(b), ServerId: proto.Int32(1)}
	if _, err := f.s.p11LimitAt(f.ctx, f.id, req, 8100, now); err == nil {
		t.Fatal("defaultoff fight accepted")
	}
	if err := db.GetIns().Model(&db.P11LimitFightPolicy{}).Where("id=1").Updates(map[string]any{"enabled": true, "published_at": now.Add(-2 * time.Minute).UnixMilli(), "start_at": now.Add(-time.Minute).UnixMilli(), "end_at": now.Add(time.Hour).UnixMilli()}).Error; err != nil {
		t.Fatal(err)
	}
	list := p11LimitCall(t, f, f.id, &pb.CSLimitPvpFindMatch{}, 8200, now).(*pb.SCLimitPvpFindMatch)
	if len(list.RankList) != 2 || list.GetMyrank().GetPvpIntegral() != 10 || list.GetFightTimes() != 0 || list.GetAllTimes() != 10 {
		t.Fatal("real original query/entry/free counters", list)
	}
	var match db.P11LimitFightMatch
	if err := db.GetIns().First(&match, "player_id=? AND opponent_id=?", f.id, b).Error; err != nil {
		t.Fatal(err)
	}
	input, err := p11LimitFrozenInput(&match, "finite-original-request")
	if err != nil || input.BattleType != 20 || input.MaxRound != 40 || input.Attacker.GetInst().GetFirstOrder() != 1 || input.Defender.GetInst().GetFirstOrder() != 0 || input.Attacker.GetPlayerId() != f.id || input.Defender.GetPlayerId() != b || len(input.Attacker.Formation) == 0 || input.Attacker.Formation[0].GetGuid() == "" || input.Attacker.Formation[0].GetSkillOrder() != "047_1,047_2,047_3" {
		t.Fatal("original mode20 complete frozen input", input, err)
	}
	// Certified original047 now has real public positive coverage. Keep this
	// rollback test as an explicit unsupported-input probe: only the isolated
	// frozen row gains an unknown skill, with its envelope hash recomputed so
	// rejection must retain the independent P6 source boundary too.
	unsupported := proto.Clone(input.Attacker).(*pb.BattlePlayer)
	unsupported.Formation[0].SkillOrder = proto.String("047_1,047_2,047_3,unverified_limit_skill")
	match.Attacker, err = proto.Marshal(unsupported)
	if err != nil {
		t.Fatal(err)
	}
	match.SourceHash = p11LimitFightMatchHash(&match)
	if err = db.GetIns().Save(&match).Error; err != nil {
		t.Fatal(err)
	}
	ownP, ownG := p10LuckyProfile(t, f.id)
	peerP, peerG := p10LuckyProfile(t, b)
	ownRaw, _ := json.Marshal(ownG)
	peerRaw, _ := json.Marshal(peerG)
	var recordsBefore int64
	db.GetIns().Model(&db.P6BattleRecord{}).Count(&recordsBefore)
	for _, stamp := range []int64{8300, 8300, 8301} {
		if _, err = f.s.p11LimitAt(f.ctx, f.id, req, stamp, now); err == nil {
			t.Fatal("unknown mode20 skill accepted resources/outcome")
		}
	}
	currentP, currentG := p10LuckyProfile(t, f.id)
	currentRaw, _ := json.Marshal(currentG)
	otherP, otherG := p10LuckyProfile(t, b)
	otherRaw, _ := json.Marshal(otherG)
	if !proto.Equal(ownP, currentP) || !proto.Equal(peerP, otherP) || string(ownRaw) != string(currentRaw) || string(peerRaw) != string(otherRaw) {
		t.Fatal("unsupported mode20 changed actual profiles")
	}
	var recordsAfter int64
	db.GetIns().Model(&db.P6BattleRecord{}).Count(&recordsAfter)
	if recordsAfter != recordsBefore {
		t.Fatal("unsupported capture committed", recordsBefore, recordsAfter)
	}
	db.GetIns().Model(&db.P11LimitFightSettlement{}).Count(&count)
	if count != 0 {
		t.Fatal("unsupported receipt committed", count)
	}
	var member db.P11LimitMember
	var daily db.P11LimitDaily
	db.GetIns().First(&member, "period_id=? AND player_id=?", match.PeriodID, f.id)
	db.GetIns().First(&daily, "period_id=? AND player_id=? AND day=?", match.PeriodID, f.id, dailyCopyDay(now))
	if member.Score != 10 || daily.Used != 0 || daily.Bought != 0 {
		t.Fatal("unsupported mode20 changed real points/daily", member, daily)
	}
	// Reopening retains the unsupported source and rejects it again without
	// producing an accepted result from historical rank or a known-mode proof.
	sqlDB, _ := db.GetIns().DB()
	sqlDB.Close()
	f.open(t)
	var cold db.P11LimitFightMatch
	if err = db.GetIns().First(&cold, "id=?", match.ID).Error; err != nil {
		t.Fatal(err)
	}
	if cold.SourceHash != match.SourceHash || string(cold.Attacker) != string(match.Attacker) || string(cold.Defender) != string(match.Defender) {
		t.Fatal("cold exact actor source changed")
	}
	if _, err = f.s.p11LimitAt(f.ctx, f.id, req, 8300, now); err == nil {
		t.Fatal("cold unsupported mode20 accepted")
	}
}
