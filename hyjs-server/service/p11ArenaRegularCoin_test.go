package service

import (
	"bytes"
	"encoding/json"
	"errors"
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

func TestP11ArenaRegularCoinIndependentInclusiveSource(t *testing.T) {
	for _, reason := range []string{constUtil.P11ArenaPrizeDisabled, constUtil.P11ArenaPrizeClosed, constUtil.P11ArenaPrizeLevel, constUtil.P11ArenaPrizeDaily, constUtil.P11ArenaPrizeTotal} {
		source := &p11ArenaRegularCoinDraw{Source: constUtil.P11ArenaPrizeRegularCoinSource, Reason: reason, Values: []int64{0, 3000, 3000}}
		cards, draw, err := p11ArenaRegularCoinCards(reason, source)
		if err != nil || len(cards) != 3 || cards[0].GetNum() != 3000 || cards[1].GetNum() != 6000 || cards[2].GetNum() != 6000 {
			t.Fatal("inclusive independent repeated quantities", cards, err)
		}
		if draw == source || &draw.Values[0] == &source.Values[0] {
			t.Fatal("mutable frozen source shared")
		}
	}
	for _, value := range []int64{-1, 3001} {
		source := &p11ArenaRegularCoinDraw{Source: constUtil.P11ArenaPrizeRegularCoinSource, Reason: constUtil.P11ArenaPrizeDisabled, Values: []int64{0, value, 0}}
		if _, _, err := p11ArenaRegularCoinCards(source.Reason, source); err == nil {
			t.Fatal("out of range frozen draw accepted", value)
		}
	}
	if _, _, err := p11ArenaRegularCoinCards(constUtil.P11ArenaPrizeLost, nil); err == nil {
		t.Fatal("loss granted normal coins")
	}
	for _, source := range []*p11ArenaRegularCoinDraw{
		{Source: "unknown", Reason: constUtil.P11ArenaPrizeDisabled, Values: []int64{0, 1, 2}},
		{Source: constUtil.P11ArenaPrizeRegularCoinSource, Reason: constUtil.P11ArenaPrizeLost, Values: []int64{0, 1, 2}},
		{Source: constUtil.P11ArenaPrizeRegularCoinSource, Reason: constUtil.P11ArenaPrizeDisabled, Values: []int64{0, 1}},
	} {
		if _, _, err := p11ArenaRegularCoinCards(constUtil.P11ArenaPrizeDisabled, source); err == nil {
			t.Fatal("unknown source/shape accepted")
		}
	}
}

// Three real original requests exercise regular fallback, a valid custom pool,
// and its exhausted daily quota. Finite owned level20 actors are fixture inputs;
// original P6 determines the winner and retains its actual frozen seed/terminal.
func TestP11ArenaRegularCoinActualRequestCustomPoolRetriesColdAndLegacyZero(t *testing.T) {
	f, peer, other := p11ArenaSetup(t)
	p11FeedbackStartupTables(t, f.s)
	for _, id := range []int64{peer, other} {
		if err := updateProfile(f.ctx, id, func(p *pb.Player, _ *model.GameState) error { p.Level = proto.Int32(9); return nil }); err != nil {
			t.Fatal(err)
		}
	}
	if err := updateProfile(f.ctx, f.id, func(p *pb.Player, g *model.GameState) error {
		p.Level = proto.Int32(20)
		g.Warriors[0].Level = proto.Int32(20)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	for _, table := range []any{&db.P11ArenaPolicy{}, &db.P11ArenaFightPolicy{}} {
		if err := db.GetIns().Model(table).Where("id=1").Update("enabled", true).Error; err != nil {
			t.Fatal(err)
		}
	}
	fight := func(stamp int64) (*pb.SCPvpFightResult, *pb.CSPvpFightResult, db.P11ArenaPrizeAward) {
		t.Helper()
		listing, handled, err := f.s.DispatchP11Arena(f.ctx, f.id, &pb.CSPvpFindMatch{}, stamp)
		if err != nil || !handled {
			t.Fatal("original21059", err)
		}
		selected := listing[0].(*pb.SCPvpFindMatch).GetRankList()[0].GetInfo()
		request := &pb.CSPvpFightResult{PlayerId: selected.PlayerId, ServerId: selected.ServerId}
		before, _ := p10LuckyProfile(t, f.id)
		out, handled, err := f.s.DispatchP11Arena(f.ctx, f.id, request, stamp+1)
		if err != nil || !handled {
			t.Fatal("original23101", err)
		}
		response := out[0].(*pb.SCPvpFightResult)
		if response.GetWinPlayerId() != f.id {
			t.Fatal("finite stronger original actor did not win", response.GetWinPlayerId())
		}
		after, _ := p10LuckyProfile(t, f.id)
		if len(response.BoxList) != 3 || after.GetCoin()-before.GetCoin() != int64(response.BoxList[0].GetNum()) || after.GetEnergy() != before.GetEnergy()-2 {
			t.Fatal("first card actual coin/energy mismatch", response, before, after)
		}
		var sum int64
		for _, card := range response.BoxList {
			sum += int64(card.GetNum())
		}
		if after.GetCoin()-before.GetCoin() == sum {
			t.Fatal("preview rewards granted")
		}
		var award db.P11ArenaPrizeAward
		if err := db.GetIns().First(&award, "fight_id=?", response.GetFightId()).Error; err != nil {
			t.Fatal(err)
		}
		return response, request, award
	}
	first, request, award := fight(995001)
	if award.Status != constUtil.P11ArenaPrizeRegularCoin {
		t.Fatal("disabled policy still zero cards", award.Status)
	}
	for _, card := range first.BoxList {
		if card.GetId() != "coin" || card.GetNum() < 3000 || card.GetNum() > 6000 {
			t.Fatal("regular range", first.BoxList)
		}
	}
	var draw p11ArenaRegularCoinDraw
	if err := json.Unmarshal([]byte(award.DrawJSON), &draw); err != nil || draw.Source != constUtil.P11ArenaPrizeConfiguredCoinSource || draw.Min != 3000 || draw.Max != 6000 || draw.Reason != constUtil.P11ArenaPrizeDisabled {
		t.Fatal("frozen normal source", draw, err)
	}
	raw, _ := proto.Marshal(first)
	stable, _ := p10LuckyProfile(t, f.id)
	var wg sync.WaitGroup
	errs := make(chan error, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			out, _, err := f.s.DispatchP11Arena(f.ctx, f.id, request, 995002)
			if err == nil {
				got, _ := proto.Marshal(out[0])
				if !bytes.Equal(raw, got) {
					err = errors.New("same-stamp prize redrawn")
				}
			}
			errs <- err
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	connection, _ := db.GetIns().DB()
	connection.Close()
	f.open(t)
	f.s = &GameService{}
	p11FeedbackStartupTables(t, f.s)
	replay, _, err := f.s.DispatchP11Arena(f.ctx, f.id, request, 995002)
	if err != nil {
		t.Fatal("cold normal prize", err)
	}
	got, _ := proto.Marshal(replay[0])
	afterCold, _ := p10LuckyProfile(t, f.id)
	if !bytes.Equal(raw, got) || !proto.Equal(stable, afterCold) {
		t.Fatal("cold prize changed or regranted")
	}
	// A valid published custom pool retains its original values and quota.
	now := LocalPlayNow(f.ctx)
	local := now.In(time.FixedZone("UTC+8", 28800))
	end := time.Date(local.Year(), local.Month()+1, 1, 0, 0, 0, 0, local.Location())
	policy := db.P11ArenaPrizePolicy{ID: 1, Enabled: true, Version: "isolated-custom-coin7777", Source: constUtil.P11ArenaPrizeSource, PoolJSON: `[{"id":"coin","num":7777,"weight":1}]`, MinLevel: 10, DailyWins: 1, WindowWins: 2, PublishedAt: now.Add(-2 * time.Second).UnixMilli(), StartAt: now.Add(-time.Second).UnixMilli(), EndAt: end.UnixMilli()}
	if err := db.GetIns().Save(&policy).Error; err != nil {
		t.Fatal(err)
	}
	custom, _, customAward := fight(995011)
	if customAward.Status != constUtil.P11ArenaPrizeAwarded || custom.BoxList[0].GetNum() != 7777 {
		t.Fatal("valid operator pool overridden", customAward.Status, custom.BoxList)
	}
	capped, _, cappedAward := fight(995021)
	if cappedAward.Status != constUtil.P11ArenaPrizeRegularCoin {
		t.Fatal("quota fallback still empty", cappedAward.Status)
	}
	var cappedDraw p11ArenaRegularCoinDraw
	if err := json.Unmarshal([]byte(cappedAward.DrawJSON), &cappedDraw); err != nil || cappedDraw.Reason != constUtil.P11ArenaPrizeDaily {
		t.Fatal("quota fallback source", cappedDraw, err)
	}
	for _, card := range capped.BoxList {
		if card.GetNum() < 3000 || card.GetNum() > 6000 {
			t.Fatal("quota fallback range", card)
		}
	}
	// Construct an isolated historical zero-card receipt using the same real P6
	// terminal. Its legacy response must replay verbatim without any makeup grant.
	legacy := proto.Clone(first).(*pb.SCPvpFightResult)
	legacy.BoxList = []*pb.BaseResource{{Id: proto.String("coin"), Num: proto.Int32(0)}, {Id: proto.String("coin"), Num: proto.Int32(0)}, {Id: proto.String("coin"), Num: proto.Int32(0)}}
	var simps []*pb.SimpleItem
	for _, item := range legacy.Result.Simps {
		if item.GetId() != "coin" {
			simps = append(simps, item)
		}
	}
	legacy.Result.Simps = simps
	legacyRaw, _ := proto.MarshalOptions{Deterministic: true}.Marshal(legacy)
	boxes, _ := json.Marshal(legacy.BoxList)
	award.Status = constUtil.P11ArenaPrizeDisabled
	award.DrawJSON = "[]"
	award.BoxesJSON = string(boxes)
	award.GrantPB = nil
	award.FinalResponseHash = p6Digest(legacyRaw)
	award.SourceHash = p11ArenaPrizeAwardHash(award)
	if err := db.GetIns().Transaction(func(tx *gorm.DB) error {
		if err := tx.Save(&award).Error; err != nil {
			return err
		}
		return tx.Model(&db.P11ArenaFightSettlement{}).Where("fight_id=?", award.FightID).Updates(map[string]any{"response": legacyRaw, "response_hash": award.FinalResponseHash}).Error
	}); err != nil {
		t.Fatal(err)
	}
	beforeLegacy, _ := p10LuckyProfile(t, f.id)
	old, _, err := f.s.DispatchP11Arena(f.ctx, f.id, request, 995002)
	if err != nil {
		t.Fatal("historical zero receipt", err)
	}
	oldRaw, _ := proto.MarshalOptions{Deterministic: true}.Marshal(old[0])
	afterLegacy, _ := p10LuckyProfile(t, f.id)
	if !bytes.Equal(oldRaw, legacyRaw) || !proto.Equal(beforeLegacy, afterLegacy) {
		t.Fatal("historical zero receipt changed or backfilled")
	}
	var count int64
	db.GetIns().Model(&db.P11ArenaFightSettlement{}).Where("player_id=?", f.id).Count(&count)
	if count != 3 {
		t.Fatal("duplicate accepted fights", count)
	}
	t.Logf("original winner three independent coin cards=%d,%d,%d; actual first-card coin=%d; 8 retries+cold once; custom7777 retained; quota fallback; isolated old zero receipt unchanged", first.BoxList[0].GetNum(), first.BoxList[1].GetNum(), first.BoxList[2].GetNum(), first.BoxList[0].GetNum())
}
