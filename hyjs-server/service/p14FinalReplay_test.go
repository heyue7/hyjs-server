package service

import (
	"bytes"
	"context"
	"encoding/json"
	"google.golang.org/protobuf/proto"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	pb "hyjs-server/model/protocol"
	"os"
	"testing"
	"time"
)

func p14ReplayDifferentIDFixture(t *testing.T) (*p1Fixture, *p1Fixture, *db.P14FinalSeries, *p14FinalSeriesManifest, time.Time) {
	t.Helper()
	a, _, _, _ := p14Setup(t)
	a.s.ServerID = 1
	sql, _ := db.GetIns().DB()
	sql.Close()
	b, ids, _, _ := p14Setup(t)
	b.id = ids[1]
	b.s.ServerID = 2
	start := time.Now().Add(time.Minute).UnixMilli()
	qualification := p6Digest([]byte("explicit domain qualification fixture, not an actual signup"))
	offer := &db.P14FinalFormationOffer{SeasonIndex: 61, RoundStage: 8, Version: "domain-current-profile-v1", Source: constUtil.P14FinalFormationSource, PublishedAt: start - 1, QualificationHash: qualification, StageStart: start, FightStart: start + 3600000, FightEnd: start + 7*3600000, LockAt: start + 3600000 - 300000}
	offer.SourceHash = p14FinalFormationOfferHash(offer)
	members := []p14FinalSealMember{}
	for _, f := range []*p1Fixture{a, b} {
		p14PerGameSwitch(t, f)
		for _, init := range []func(context.Context) error{f.s.InitializeP6, f.s.InitializeP14FinalSettlement} {
			if err := init(f.ctx); err != nil {
				t.Fatal(err)
			}
		}
		player, state := f.profile(t)
		if err := f.s.RefreshP5GuildPrayerPower(f.ctx, db.GetIns(), player, state, time.UnixMilli(start+1000)); err != nil {
			t.Fatal(err)
		}
		battle, err := f.s.battlePlayer(player, state)
		if err != nil {
			t.Fatal(err)
		}
		frame, err := f.s.p10FormationProfile(db.GetIns(), f.id)
		if err != nil {
			t.Fatal(err)
		}
		battleRaw, _ := proto.Marshal(battle)
		frameRaw, _ := proto.Marshal(frame)
		configuration, err := f.s.p14ConfigurationHash()
		if err != nil {
			t.Fatal(err)
		}
		row := db.P14FinalFormation{SeasonIndex: 61, RoundStage: 8, ServerID: f.s.OriginalServerID(), PlayerID: f.id, Revision: 1, CapturedAt: start + 1000, FightPower: frame.GetFightPower(), Snapshot: frameRaw, Battle: battleRaw, SnapshotHash: p14SnapshotDigest(frameRaw, battleRaw), ClientVersion: constUtil.P6OriginalClientHash, EngineVersion: constUtil.P6EngineVersion, ConfigurationHash: configuration, QualificationHash: qualification, OfferHash: offer.SourceHash, Source: constUtil.P14FinalFormationSource}
		row.SourceHash = p14FinalFormationHash(&row)
		metadata, _ := json.Marshal(row)
		members = append(members, p14FinalSealMember{Kind: 3, PlayerID: f.id, Metadata: metadata, Formation: frameRaw, Battle: battleRaw})
	}
	gamePolicy := db.P14FinalGamePolicy{ID: 1, Enabled: true, Version: "domain-current-first-input-v1", Source: constUtil.P14FinalGameSource, PublishedAt: start - 1}
	provenance := p14FinalGameProvenance{SeasonIndex: 61, RoundStage: 8, GameIndex: 1, Pair: p14FinalDrawPair{Group: "A", Index: 0, LeftServerID: 1, LeftPlayerID: a.id, RightServerID: 2, RightPlayerID: b.id}, DrawHash: p6Digest([]byte("explicit domain draw fixture")), QualificationHash: qualification, Policy: gamePolicy, RulesHash: p6Digest([]byte(constUtil.P14FinalGameRules)), SealHashes: []string{p6Digest(members[0].Metadata), p6Digest(members[1].Metadata)}, Left: members[0], Right: members[1]}
	game, err := p14FinalGameBuild(&provenance)
	if err != nil {
		t.Fatal(err)
	}
	game.CreatedAt = offer.FightStart
	schedule := &db.P14FinalSeriesPolicy{ID: 1, Enabled: true, Version: "domain-current-schedule-v1", Source: constUtil.P14FinalSeriesSource, PublishedAt: start - 1}
	at := time.UnixMilli(offer.FightStart + 1)
	series, slots, err := p14FinalSeriesPlan(game, offer, schedule, at)
	if err != nil {
		t.Fatal(err)
	}
	var manifest p14FinalSeriesManifest
	if json.Unmarshal(series.Manifest, &manifest) != nil {
		t.Fatal("manifest")
	}
	for _, f := range []*p1Fixture{a, b} {
		p14PerGameSwitch(t, f)
		tx := db.GetIns()
		if err = tx.Model(&db.P14SeasonPolicy{}).Where("id=1").Updates(map[string]any{"enabled": true, "season_index": 61}).Error; err != nil {
			t.Fatal(err)
		}
		for _, row := range []any{game, offer, series, &slots} {
			if err = tx.Create(row).Error; err != nil {
				t.Fatal(err)
			}
		}
		if f.s.OriginalServerID() == 2 {
			if err = tx.Model(&db.P14TrustedPeer{}).Where("server_id=2").Update("server_id", 1).Error; err != nil {
				t.Fatal(err)
			}
		}
	}
	return a, b, series, &manifest, at
}

// The real domain fixture creates both IDs normally. It does not rename an ID
// in frozen input merely to avoid the original log UI's PID-only comparison.
func TestP14FinalReplayActualDifferentPlayersCertifiedAndPlannedCold(t *testing.T) {
	if os.Getenv("HYJS_P13_MYSQL_DATABASE") != "" {
		t.Skip("two-file domain fixture")
	}
	a, _, series, manifest, at := p14ReplayDifferentIDFixture(t)
	p14PerGameSwitch(t, a)
	for _, init := range []func(context.Context) error{a.s.InitializeP14FinalResult, a.s.InitializeP14PerGame, a.s.InitializeP14FinalContinuation} {
		if err := init(a.ctx); err != nil {
			t.Fatal(err)
		}
	}
	if err := db.GetIns().Model(&db.P14FinalSettlementPolicy{}).Where("id=1").Updates(map[string]any{"enabled": true, "version": "actual-log-domain-v1", "published_at": manifest.FormationOffer.StageStart - 1}).Error; err != nil {
		t.Fatal(err)
	}
	if err := a.s.SettleP14FinalGameOne(a.ctx, series.ID, at); err != nil {
		t.Fatal(err)
	}
	request := &pb.CSKFReplayInfo{Type: proto.Int32(3), SubType: proto.Int32(8), RegId: proto.String("61_1_1")}
	// The exact original click syntax is decimal season_server_player.
	request.RegId = proto.String("61_1_1")
	messages, err := a.s.P14FinalReplayInfoAt(a.ctx, a.id, request, at)
	if err != nil {
		t.Fatal(err)
	}
	info := messages[0].(*pb.SCKFReplayInfo)
	if len(info.Replays) != 5 || info.Replays[0].GetState() != 0 || info.Replays[0].GetWarId() != series.GameOneID || info.Replays[0].GetWinPlayerId() == 0 {
		t.Fatal("actual completed game missing")
	}
	for _, row := range info.Replays[1:] {
		if row.GetState() != 1 || row.GetWarId() != "" || row.GetWinPlayerId() != 0 {
			t.Fatal("planned input pretends to be played")
		}
	}
	var before int64
	db.GetIns().Model(&db.P6BattleRecord{}).Count(&before)
	frames, err := a.s.P14FinalReplayAt(a.ctx, a.id, &pb.CSKFReplay{Type: proto.Int32(3), WarId: proto.String(series.GameOneID)}, at)
	if err != nil {
		t.Fatal(err)
	}
	var slot db.P14FinalSeriesSlot
	if err = db.GetIns().First(&slot, "series_id=? AND game_index=1", series.ID).Error; err != nil {
		t.Fatal(err)
	}
	raw, _ := proto.Marshal(frames[0].(*pb.SCKFReplay).GetData())
	if !bytes.Equal(raw, slot.Input) {
		t.Fatal("playback changed actual original source")
	}
	if _, err = a.s.P14FinalReplayAt(a.ctx, a.id+1, &pb.CSKFReplay{Type: proto.Int32(3), WarId: proto.String(series.GameOneID)}, at); err == nil {
		t.Fatal("uncertified spectator view accepted")
	}
	a.restart(t)
	cold, err := a.s.P14FinalReplayInfoAt(a.ctx, a.id, request, at)
	if err != nil || !proto.Equal(cold[0], info) {
		t.Fatal("cold log changed", err)
	}
	var after int64
	db.GetIns().Model(&db.P6BattleRecord{}).Count(&after)
	if after != before {
		t.Fatal("read replay created P6 owner or recomputed")
	}
}

func TestP14FinalReplaySamePIDRejectsOriginalAmbiguousWinnerUI(t *testing.T) {
	if os.Getenv("HYJS_P13_MYSQL_DATABASE") != "" {
		t.Skip("two-file fixture")
	}
	a, _, _, _, at := p14ContinuationFixture(t)
	p14PerGameSwitch(t, a)
	if _, err := a.s.P14FinalReplayInfoAt(a.ctx, a.id, &pb.CSKFReplayInfo{Type: proto.Int32(3), SubType: proto.Int32(8), RegId: proto.String("61_1_1")}, at); err == nil {
		t.Fatal("PID-only original label forged a realm winner")
	}
}
