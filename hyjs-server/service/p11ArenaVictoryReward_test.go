package service

import (
	"bytes"
	"encoding/json"
	"errors"
	"math"
	"strings"
	"sync"
	"testing"

	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

// The frozen facts are pure data: no database, no clock and no operator pool.
func TestP11ArenaVictoryRewardFrozenFacts(t *testing.T) {
	for level, want := range map[int64][2]int64{
		1:                 {constUtil.P11ArenaVictoryPopularity, 2},
		20:                {constUtil.P11ArenaVictoryPopularity, 40},
		math.MaxInt32 / 2: {constUtil.P11ArenaVictoryPopularity, math.MaxInt32 - 1},
	} {
		popularity, exp, err := p11ArenaVictoryRewardAmounts(level)
		if err != nil || popularity != want[0] || exp != want[1] {
			t.Fatal("level basis", level, popularity, exp, err)
		}
	}
	for _, level := range []int64{0, -1, math.MaxInt32/2 + 1} {
		if _, _, err := p11ArenaVictoryRewardAmounts(level); err == nil {
			t.Fatal("implausible level accepted", level)
		}
	}
	row := &db.P11ArenaFightSettlement{PlayerID: 7, WinnerID: 7}
	// An award written before this source keeps its old shape and hash input.
	legacy := db.P11ArenaPrizeAward{FightID: "legacy"}
	expected := &pb.ResultObject{}
	if err := p11ArenaVictoryRewardFacts(row, &legacy, expected); err != nil || len(expected.Simps) != 0 {
		t.Fatal("legacy award projected new facts", err, expected)
	}
	raw, err := json.Marshal(legacy)
	if err != nil || strings.Contains(string(raw), "victoryReward") {
		t.Fatal("legacy award hash gained new fields", err, string(raw))
	}
	award := db.P11ArenaPrizeAward{FightID: "current", PlayerLevel: 20, VictoryRewardSource: constUtil.P11ArenaVictoryRewardSource, VictoryRewardLevel: 20, VictoryRewardPopularity: 50, VictoryRewardExp: 40}
	if err = p11ArenaVictoryRewardFacts(row, &award, expected); err != nil || len(expected.Simps) != 2 || expected.Simps[0].GetId() != "popularity" || expected.Simps[0].GetNum() != 50 || expected.Simps[1].GetId() != "exp" || expected.Simps[1].GetNum() != 40 {
		t.Fatal("frozen victory projection", err, expected)
	}
	for _, corrupt := range []func(*db.P11ArenaPrizeAward, *db.P11ArenaFightSettlement){
		func(a *db.P11ArenaPrizeAward, _ *db.P11ArenaFightSettlement) { a.VictoryRewardPopularity = 51 },
		func(a *db.P11ArenaPrizeAward, _ *db.P11ArenaFightSettlement) { a.VictoryRewardExp = 42 },
		func(a *db.P11ArenaPrizeAward, _ *db.P11ArenaFightSettlement) { a.VictoryRewardLevel = 19 },
		func(a *db.P11ArenaPrizeAward, _ *db.P11ArenaFightSettlement) {
			a.VictoryRewardLevel = math.MaxInt32/2 + 1
		},
		func(a *db.P11ArenaPrizeAward, _ *db.P11ArenaFightSettlement) { a.VictoryRewardLevel = 0 },
		func(a *db.P11ArenaPrizeAward, _ *db.P11ArenaFightSettlement) { a.VictoryRewardPopularity = 0 },
		func(a *db.P11ArenaPrizeAward, _ *db.P11ArenaFightSettlement) { a.VictoryRewardSource = "local-guess" },
		func(_ *db.P11ArenaPrizeAward, r *db.P11ArenaFightSettlement) { r.WinnerID = 9 },
	} {
		changed, changedRow := award, *row
		corrupt(&changed, &changedRow)
		if err = p11ArenaVictoryRewardFacts(&changedRow, &changed, &pb.ResultObject{}); err == nil {
			t.Fatal("corrupt victory facts accepted", changed)
		}
	}
	// A source-less row with leftovers is corruption, not a legacy receipt.
	partial := award
	partial.VictoryRewardSource = ""
	if err = p11ArenaVictoryRewardFacts(row, &partial, &pb.ResultObject{}); err == nil {
		t.Fatal("source-less victory facts accepted")
	}
}

// One real original arena request wins at exp1199 so the confirmed 40 experience
// crosses the recovered UserExp level20 threshold and grants the original stamina
// and energy level-up values. Eight identical retries, a real challenger loss and
// a cold service replay must all keep the frozen facts without regranting.
func TestP11ArenaVictoryFameExperienceActualWinLossRetryCold(t *testing.T) {
	f, peer, _ := p11ArenaSetup(t)
	p11FeedbackStartupTables(t, f.s)
	// The original hero level follows the character level, so both finite actors
	// keep the same level and the explicit training below decides each outcome.
	if err := updateProfile(f.ctx, f.id, func(p *pb.Player, g *model.GameState) error {
		p.Level = proto.Int32(20)
		p.Exp = proto.Int64(1199)
		g.Warriors[0].Level = proto.Int32(20)
		g.Warriors[0].Exp = proto.Int64(0)
		g.Warriors[0].Training = &pb.Training{Hp: proto.Int32(400000), Att: proto.Int32(40000), Pdef: proto.Int32(4000), Mdef: proto.Int32(4000)}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	for _, table := range []any{&db.P11ArenaPolicy{}, &db.P11ArenaFightPolicy{}} {
		if err := db.GetIns().Model(table).Where("id=1").Update("enabled", true).Error; err != nil {
			t.Fatal(err)
		}
	}
	fight := func(stamp int64, pick func(*pb.PvpRankPlayerInfo) bool) (*pb.SCPvpFightResult, *pb.CSPvpFightResult, *pb.Player) {
		t.Helper()
		listing, handled, err := f.s.DispatchP11Arena(f.ctx, f.id, &pb.CSPvpFindMatch{}, stamp)
		if err != nil || !handled {
			t.Fatal("original21059", err)
		}
		var selected *pb.PvpRankPlayerInfo
		for _, candidate := range listing[0].(*pb.SCPvpFindMatch).GetRankList() {
			if selected == nil && pick(candidate) {
				selected = candidate
			}
		}
		if selected == nil {
			t.Fatal("listed actual opponent missing")
		}
		request := &pb.CSPvpFightResult{PlayerId: proto.Int64(selected.GetInfo().GetPlayerId()), ServerId: proto.Int32(selected.GetInfo().GetServerId())}
		before, _ := p10LuckyProfile(t, f.id)
		out, handled, err := f.s.DispatchP11Arena(f.ctx, f.id, request, stamp+1)
		if err != nil || !handled {
			t.Fatal("original23101", err)
		}
		return out[0].(*pb.SCPvpFightResult), request, before
	}
	// Fail after all reward mutation, at the award insert, to verify the shared
	// transaction rolls profile, rank and settlement facts back together.
	if _, _, err := f.s.DispatchP11Arena(f.ctx, f.id, &pb.CSPvpFindMatch{}, 996990); err != nil {
		t.Fatal(err)
	}
	rollbackBefore, _ := p10LuckyProfile(t, f.id)
	var rankBefore db.P11ArenaMember
	if err := db.GetIns().First(&rankBefore, "player_id=?", f.id).Error; err != nil {
		t.Fatal(err)
	}
	callback := "arena_victory_owned_rollback"
	if err := db.GetIns().Callback().Create().Before("gorm:create").Register(callback, func(tx *gorm.DB) {
		if tx.Statement.Table == "p11_arena_prize_awards" {
			tx.AddError(errors.New("injected award failure"))
		}
	}); err != nil {
		t.Fatal(err)
	}
	_, _, rollbackErr := f.s.DispatchP11Arena(f.ctx, f.id, &pb.CSPvpFightResult{PlayerId: proto.Int64(peer), ServerId: proto.Int32(1)}, 996991)
	if err := db.GetIns().Callback().Create().Remove(callback); err != nil {
		t.Fatal(err)
	}
	if rollbackErr == nil {
		t.Fatal("injected settlement failure accepted")
	}
	rollbackAfter, _ := p10LuckyProfile(t, f.id)
	var rankAfter db.P11ArenaMember
	if err := db.GetIns().First(&rankAfter, "player_id=?", f.id).Error; err != nil {
		t.Fatal(err)
	}
	var rolledCount int64
	if err := db.GetIns().Model(&db.P11ArenaFightSettlement{}).Where("player_id=?", f.id).Count(&rolledCount).Error; err != nil {
		t.Fatal(err)
	}
	if !proto.Equal(rollbackBefore, rollbackAfter) || rankBefore.Rank != rankAfter.Rank || rolledCount != 0 {
		t.Fatal("settlement failure leaked reward/rank/fight", rolledCount)
	}
	defenderBefore, _ := p10LuckyProfile(t, peer)
	first, request, before := fight(997001, func(candidate *pb.PvpRankPlayerInfo) bool { return candidate.GetInfo().GetPlayerId() == peer })
	if first.GetWinPlayerId() != f.id {
		t.Fatal("finite stronger original actor did not win", first.GetWinPlayerId())
	}
	after, _ := p10LuckyProfile(t, f.id)
	defenderAfter, _ := p10LuckyProfile(t, peer)
	if defenderBefore.GetPopularity() != defenderAfter.GetPopularity() || defenderBefore.GetExp() != defenderAfter.GetExp() {
		t.Fatal("defender received challenger victory reward")
	}
	if after.GetPopularity() != before.GetPopularity()+int32(constUtil.P11ArenaVictoryPopularity) {
		t.Fatal("victory fame", before.GetPopularity(), after.GetPopularity())
	}
	// 1199+40 crosses the level20 threshold: level21, exp39, +30 stamina, +10
	// energy, while the original fight already spent the two energy points.
	if after.GetLevel() != 21 || after.GetExp() != 39 || after.GetStamina() != before.GetStamina()+30 || after.GetEnergy() != before.GetEnergy()+8 {
		t.Fatal("real experience/level-up semantics", before, after)
	}
	var coin, popularity, exp int32
	for _, item := range first.GetResult().GetSimps() {
		switch item.GetId() {
		case "coin":
			coin += item.GetNum()
		case "popularity":
			popularity += item.GetNum()
		case "exp":
			exp += item.GetNum()
		}
	}
	if coin != first.BoxList[0].GetNum() || popularity != 50 || exp != 40 {
		t.Fatal("original ResultObject projection", first.GetResult(), first.BoxList)
	}
	var award db.P11ArenaPrizeAward
	if err := db.GetIns().First(&award, "fight_id=?", first.GetFightId()).Error; err != nil {
		t.Fatal(err)
	}
	if award.Status != constUtil.P11ArenaPrizeRegularCoin || award.VictoryRewardSource != constUtil.P11ArenaVictoryRewardSource || award.VictoryRewardLevel != 20 || award.VictoryRewardPopularity != 50 || award.VictoryRewardExp != 40 || award.PlayerLevel != 20 {
		t.Fatal("frozen victory award facts", award)
	}
	// A historical award has no victory facts. Its exact old projection remains
	// valid and validation never grants resources while rebuilding the receipt.
	legacyPlayer, _ := p10LuckyProfile(t, f.id)
	legacyRollback := errors.New("legacy verification rollback")
	legacyErr := db.GetIns().Transaction(func(tx *gorm.DB) error {
		legacyAward := award
		legacyAward.VictoryRewardSource = ""
		legacyAward.VictoryRewardLevel, legacyAward.VictoryRewardPopularity, legacyAward.VictoryRewardExp = 0, 0, 0
		legacyResponse := proto.Clone(first).(*pb.SCPvpFightResult)
		legacyResponse.Result.Simps = nil
		for _, item := range first.Result.Simps {
			if item.GetId() != "popularity" && item.GetId() != "exp" {
				legacyResponse.Result.Simps = append(legacyResponse.Result.Simps, proto.Clone(item).(*pb.SimpleItem))
			}
		}
		legacyRaw, e := proto.MarshalOptions{Deterministic: true}.Marshal(legacyResponse)
		if e != nil {
			return e
		}
		legacyAward.FinalResponseHash = p6Digest(legacyRaw)
		legacyAward.SourceHash = p11ArenaPrizeAwardHash(legacyAward)
		if e = tx.Save(&legacyAward).Error; e != nil {
			return e
		}
		var legacyRow db.P11ArenaFightSettlement
		if e = tx.First(&legacyRow, "fight_id=?", first.GetFightId()).Error; e != nil {
			return e
		}
		legacyRow.Response, legacyRow.ResponseHash = legacyRaw, legacyAward.FinalResponseHash
		if e = tx.Save(&legacyRow).Error; e != nil {
			return e
		}
		if e = ValidateP11ArenaFightPrizeReceipt(tx, &legacyRow, legacyResponse); e != nil {
			return e
		}
		return legacyRollback
	})
	if !errors.Is(legacyErr, legacyRollback) {
		t.Fatal("historical exact receipt rejected", legacyErr)
	}
	legacyAfter, _ := p10LuckyProfile(t, f.id)
	if !proto.Equal(legacyPlayer, legacyAfter) {
		t.Fatal("legacy validation granted resources")
	}
	// GrantPB still means only the original flip card.
	grant := &pb.ResultObject{}
	if err := proto.Unmarshal(award.GrantPB, grant); err != nil || len(grant.Simps) != 1 || grant.Simps[0].GetId() != "coin" || len(grant.Wars)+len(grant.Equips)+len(grant.Treas) != 0 {
		t.Fatal("GrantPB mixed the new reward", grant, err)
	}
	raw, err := proto.Marshal(first)
	if err != nil {
		t.Fatal(err)
	}
	stable, _ := p10LuckyProfile(t, f.id)
	var wg sync.WaitGroup
	errs := make(chan error, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			out, _, e := f.s.DispatchP11Arena(f.ctx, f.id, request, 997002)
			if e == nil {
				got, _ := proto.Marshal(out[0])
				if !bytes.Equal(raw, got) {
					e = errors.New("same-stamp victory reward redrawn")
				}
			}
			errs <- e
		}()
	}
	wg.Wait()
	close(errs)
	for e := range errs {
		if e != nil {
			t.Fatal(e)
		}
	}
	afterRetries, _ := p10LuckyProfile(t, f.id)
	if !proto.Equal(stable, afterRetries) {
		t.Fatal("victory retries changed player resources")
	}
	// Restore the challenger to its untrained baseline before the loss scenario;
	// leaving both actors equally trained makes the outcome seed-dependent.
	if err := updateProfile(f.ctx, f.id, func(_ *pb.Player, g *model.GameState) error {
		g.Warriors[0].Training = nil
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	// The re-enrolled peer keeps the original hero level rule and is given finite
	// explicit training so the deterministic P6 simulation ends in its favour.
	if err := updateProfile(f.ctx, peer, func(p *pb.Player, g *model.GameState) error {
		p.Level = proto.Int32(20)
		g.Warriors[0].Training = &pb.Training{Hp: proto.Int32(400000), Att: proto.Int32(40000), Pdef: proto.Int32(4000), Mdef: proto.Int32(4000)}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	// Match candidates are rank-windowed, so move the re-enrolled peer beside the
	// challenger under the same policy lock every actual rank writer uses.
	var ownMember db.P11ArenaMember
	if err := db.GetIns().First(&ownMember, "player_id=?", f.id).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.GetIns().Transaction(func(tx *gorm.DB) error {
		if _, err := p11ArenaConfig(tx); err != nil {
			return err
		}
		return tx.Model(&db.P11ArenaMember{}).Where("player_id=?", peer).Update("position", ownMember.Rank+1).Error
	}); err != nil {
		t.Fatal(err)
	}
	loss, lossRequest, lossBefore := fight(997011, func(candidate *pb.PvpRankPlayerInfo) bool { return candidate.GetInfo().GetPlayerId() == peer })
	if loss.GetWinPlayerId() != peer {
		t.Fatal("weakened finite challenger won", loss.GetWinPlayerId(), lossRequest)
	}
	lossAfter, _ := p10LuckyProfile(t, f.id)
	if lossAfter.GetPopularity() != lossBefore.GetPopularity() || lossAfter.GetExp() != lossBefore.GetExp() || lossAfter.GetLevel() != lossBefore.GetLevel() || lossAfter.GetStamina() != lossBefore.GetStamina() {
		t.Fatal("loss granted fame or experience", lossBefore, lossAfter)
	}
	for _, item := range loss.GetResult().GetSimps() {
		if item.GetId() == "popularity" || item.GetId() == "exp" {
			t.Fatal("loss projected victory rewards", loss.GetResult())
		}
	}
	var lossAward db.P11ArenaPrizeAward
	if err := db.GetIns().First(&lossAward, "fight_id=?", loss.GetFightId()).Error; err != nil {
		t.Fatal(err)
	}
	if lossAward.Status != constUtil.P11ArenaPrizeLost || lossAward.VictoryRewardSource != "" || lossAward.VictoryRewardLevel != 0 || lossAward.VictoryRewardPopularity != 0 || lossAward.VictoryRewardExp != 0 {
		t.Fatal("loss stored victory facts", lossAward)
	}
	var lossRow db.P11ArenaFightSettlement
	if err := db.GetIns().First(&lossRow, "fight_id=?", loss.GetFightId()).Error; err != nil {
		t.Fatal(err)
	}
	if err := ValidateP11ArenaFightPrizeReceipt(db.GetIns(), &lossRow, loss); err != nil {
		t.Fatal("loss receipt", err)
	}
	// Cold service reuses the persisted award row and never pays twice.
	connection, _ := db.GetIns().DB()
	connection.Close()
	f.open(t)
	f.s = &GameService{}
	p11FeedbackStartupTables(t, f.s)
	replay, _, err := f.s.DispatchP11Arena(f.ctx, f.id, request, 997002)
	if err != nil {
		t.Fatal("cold victory receipt", err)
	}
	got, _ := proto.Marshal(replay[0])
	afterCold, _ := p10LuckyProfile(t, f.id)
	if !bytes.Equal(raw, got) || !proto.Equal(lossAfter, afterCold) {
		t.Fatal("cold victory receipt changed or regranted")
	}
	var count int64
	db.GetIns().Model(&db.P11ArenaFightSettlement{}).Where("player_id=?", f.id).Count(&count)
	if count != 2 {
		t.Fatal("duplicate accepted fights", count)
	}
	t.Logf("actual win level20 exp1199 -> level%d exp%d fame50 exp40 stamina+30 energy+10; 8 retries+cold once; weaker challenger lost without facts", after.GetLevel(), after.GetExp())
}
