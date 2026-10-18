package service

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

// This is the exact signed envelope observed in the prior real two-process
// validation. It contains no signing key or login session. It is read-only history.
//
//go:embed testdata/p14_legacy_stage7_rank.json
var p14LegacyStage7RankFixture []byte

func TestP14OriginalPromotionStageEnumAndActualLegacyRankReadOnly(t *testing.T) {
	if constUtil.P14PromotionStage != 6 || constUtil.P14Final16BeforeStage != 7 || constUtil.P14Final16Stage != 8 {
		t.Fatal("original UI.ServerBattleState/ServerBattlePromotion.Init contract")
	}
	var envelope model.P14PromotionRankImportRequest
	if json.Unmarshal(p14LegacyStage7RankFixture, &envelope) != nil {
		t.Fatal("actual legacy signed source")
	}
	var old p14PromotionRankSnapshot
	if json.Unmarshal(envelope.Snapshot, &old) != nil {
		t.Fatal("actual legacy snapshot")
	}
	row := db.P14PromotionRankVersion{SeasonIndex: old.Offer.SeasonIndex, ServerID: old.Offer.ServerID, Revision: old.Revision, Content: append([]byte(nil), envelope.Snapshot...), ContentHash: p6Digest(envelope.Snapshot), PreviousHash: old.PreviousHash, CapturedAt: old.ObservedAt}
	before := append([]byte(nil), row.Content...)
	snapshot, err := p14PromotionRankReadVersion(&row, time.UnixMilli(old.ObservedAt+1))
	if err != nil || snapshot.Revision != 2 || snapshot.Scores[0].Score != 16 || !bytes.Equal(row.Content, before) {
		t.Fatal("legacy stage7 observed match source rewritten", err)
	}
}

func TestP14OriginalPromotionStageSixPointsUploadPurposeAndRankWindowSevenClosed(t *testing.T) {
	f, ids, start, end := p14PromotionFormationSetup(t)
	if err := db.GetIns().Model(&db.P14PromotionPolicy{}).Where("id=1").Update("enabled", true).Error; err != nil {
		t.Fatal(err)
	}
	at := start.Add(time.Minute)
	info := p14PromotionInfo(t, f, ids[0], at)
	if info.GetTypeStage() != 6 || info.GetIsEnterCurStage() != 0 {
		t.Fatal("original promotion UI6 missing qualification")
	}
	if _, err := f.s.p14Register(f.ctx, ids[0], &pb.CSKFEnterFrightGame{}, 9101, at); err != nil {
		t.Fatal("original stage6 purpose2", err)
	}
	var frozen db.P14PromotionFormation
	if err := db.GetIns().First(&frozen, "season_index=7 AND server_id=1 AND player_id=?", ids[0]).Error; err != nil || frozen.StageStart != start.UnixMilli() || frozen.StageEnd != end.UnixMilli() {
		t.Fatal("wrong original purpose window", err)
	}
	policy := db.P14PromotionRankPolicy{Version: "original-stage6-fixture", Source: constUtil.P14PromotionRankSource, PublishedAt: start.Add(-time.Hour).UnixMilli()}
	offer, err := f.s.p14PromotionRankOfferTX(db.GetIns(), &policy, 7, at)
	if err != nil || offer.StageStart != start.UnixMilli() || offer.StageEnd != end.UnixMilli() {
		t.Fatal("rank producer chose Final16_Before", err)
	}
	if _, err := f.s.p14Query(f.ctx, ids[0], &pb.CSKFOpenFrightServerInfo{}, end); err == nil {
		t.Fatal("stage7 borrowed promotion admission without final proof")
	}
	if _, err = f.s.p14Register(f.ctx, ids[0], &pb.CSKFEnterFrightGame{}, 9102, end); err == nil {
		t.Fatal("stage7 started new purpose2")
	}
	if _, err = f.s.p14PromotionFightAt(f.ctx, ids[0], 9103, end); err == nil {
		t.Fatal("stage7 started new promotion match")
	}
	frames, err := f.s.p14Query(f.ctx, ids[0], &pb.CSKFPlayerInfo{PlayerId: proto.Int64(ids[0]), ServerId: proto.Int32(1), Type: proto.Int32(2)}, end)
	if err != nil || len(frames) != 1 {
		t.Fatal("frozen original stage6 cannot be read after cutoff", err)
	}
	var after db.P14PromotionFormation
	db.GetIns().First(&after, "season_index=7 AND server_id=1 AND player_id=?", ids[0])
	if after.SourceHash != frozen.SourceHash || !bytes.Equal(after.Battle, frozen.Battle) {
		t.Fatal("deadline rewrote frozen source")
	}
}

func TestP14OriginalPromotionLegacyFrozenFormationAndRankNeverReissued(t *testing.T) {
	f, ids, start, end := p14PromotionFormationSetup(t)
	if err := db.GetIns().Model(&db.P14PromotionPolicy{}).Where("id=1").Update("enabled", true).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := f.s.p14Register(f.ctx, ids[0], &pb.CSKFEnterFrightGame{}, 9201, start.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	var legacy db.P14PromotionFormation
	if err := db.GetIns().First(&legacy, "season_index=7 AND server_id=1 AND player_id=?", ids[0]).Error; err != nil {
		t.Fatal(err)
	}
	// This isolated compatibility fixture reuses a genuine formation payload and
	// explicitly models metadata saved by the legacy stage7 producer. It is no win.
	legacy.StageStart, legacy.StageEnd, legacy.CapturedAt = end.UnixMilli(), end.Add(time.Hour).UnixMilli(), end.Add(time.Minute).UnixMilli()
	legacy.SourceHash = p14PromotionFormationHash(&legacy)
	if err := db.GetIns().Save(&legacy).Error; err != nil {
		t.Fatal(err)
	}
	frames, err := f.s.p14Query(f.ctx, ids[0], &pb.CSKFPlayerInfo{PlayerId: proto.Int64(ids[0]), ServerId: proto.Int32(1), Type: proto.Int32(2)}, end.Add(2*time.Minute))
	if err != nil || len(frames) != 1 {
		t.Fatal("legacy frozen formation read lost", err)
	}
	var after db.P14PromotionFormation
	db.GetIns().First(&after, "season_index=7 AND server_id=1 AND player_id=?", ids[0])
	if after.SourceHash != legacy.SourceHash || after.CapturedAt != legacy.CapturedAt || !bytes.Equal(after.Battle, legacy.Battle) {
		t.Fatal("legacy formation rebranded")
	}
	if _, err = f.s.p14Register(f.ctx, ids[0], &pb.CSKFEnterFrightGame{}, 9202, end.Add(2*time.Minute)); err == nil {
		t.Fatal("legacy7 source reopened producer")
	}
	gate := db.P14PromotionRankPolicy{ID: 1, Enabled: true, Version: "legacy-audit-fixture", Source: constUtil.P14PromotionRankSource, PublishedAt: start.Add(-time.Hour).UnixMilli()}
	if err = db.GetIns().Save(&gate).Error; err != nil {
		t.Fatal(err)
	}
	offer, err := f.s.p14PromotionRankOfferTX(db.GetIns(), &gate, 7, end.Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	offer.StageStart, offer.StageEnd = legacy.StageStart, legacy.StageEnd
	offer.SourceHash = p14PromotionRankOfferHash(offer)
	if err = db.GetIns().Save(offer).Error; err != nil {
		t.Fatal(err)
	}
	before := offer.SourceHash
	if err = f.s.QueueP14PromotionRanks(f.ctx, end.Add(2*time.Minute)); err != nil {
		t.Fatal(err)
	}
	var versions int64
	db.GetIns().Model(&db.P14PromotionRankVersion{}).Count(&versions)
	db.GetIns().First(offer, "season_index=7 AND server_id=1")
	if versions != 0 || offer.SourceHash != before {
		t.Fatal("legacy7 rank source produced new version")
	}
}
