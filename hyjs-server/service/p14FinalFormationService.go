package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strings"
	"time"

	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

func p14FinalFormationOfferHash(row *db.P14FinalFormationOffer) string {
	copy := *row
	copy.SourceHash = ""
	raw, _ := json.Marshal(copy)
	return p6Digest(raw)
}

func p14FinalFormationHash(row *db.P14FinalFormation) string {
	copy := *row
	copy.SourceHash = ""
	raw, _ := json.Marshal(copy)
	return p6Digest(raw)
}

func p14FinalFormationID(request model.P14SnapshotImportRequest) string {
	return p6Digest([]byte(fmt.Sprintf("p14/final/%d/%d/%s/%s/%d/%d/%d/%d", request.SeasonIndex, request.RoundStage, request.FinalQualificationHash, request.FinalOfferHash, request.SourceServerID, request.PlayerID, request.Revision, request.DestinationServerID)))
}

func p14FinalFormationValidateOffer(offer *db.P14FinalFormationOffer) error {
	if offer == nil || offer.SeasonIndex <= 0 || offer.RoundStage != constUtil.P14Final16Stage || offer.Source != constUtil.P14FinalFormationSource || strings.TrimSpace(offer.Version) == "" || len(offer.Version) > 128 || offer.PublishedAt <= 0 || offer.PublishedAt > offer.StageStart || len(offer.QualificationHash) != 64 || offer.StageStart <= 0 || offer.LockAt <= offer.StageStart || offer.LockAt != offer.FightStart-constUtil.P14FinalFormationLockMillis || offer.FightEnd <= offer.FightStart || offer.SourceHash != p14FinalFormationOfferHash(offer) {
		return errors.New(constUtil.P14FinalFormationCorrupt)
	}
	return nil
}

func p14FinalFormationCaptureAllowed(offer *db.P14FinalFormationOffer, capturedAt int64) error {
	if err := p14FinalFormationValidateOffer(offer); err != nil {
		return err
	}
	if capturedAt < offer.StageStart || capturedAt >= offer.LockAt {
		return errors.New(constUtil.P14FinalFormationClosed)
	}
	return nil
}

func (s *GameService) InitializeP14FinalFormation(ctx context.Context) error {
	tx := db.GetIns().WithContext(ctx)
	if err := tx.AutoMigrate(&db.P14FinalFormationPolicy{}, &db.P14FinalFormationOffer{}, &db.P14FinalFormation{}); err != nil {
		return err
	}
	return tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&db.P14FinalFormationPolicy{ID: 1, Source: constUtil.P14FinalFormationSource}).Error
}

// p14FinalFormationOfferTX freezes the actual first-final calendar and complete
// peer source. Later rounds wait for real winners; no bye or result is inferred.
func (s *GameService) p14FinalFormationOfferTX(ctx context.Context, tx *gorm.DB, season *db.P14SeasonPolicy, server, player int64, now time.Time) (*db.P14FinalFormationOffer, error) {
	qualified, err := s.p14FinalQualificationTX(ctx, tx, season.SeasonIndex, now)
	if err != nil {
		return nil, err
	}
	if qualified == nil || p14FinalQualificationEntered(qualified, server, player) != 1 {
		return nil, errors.New(constUtil.P14ResultsUnverified)
	}
	var certificate db.P14FinalQualification
	if err = tx.First(&certificate, "season_index=?", season.SeasonIndex).Error; err != nil {
		return nil, err
	}
	calendar, err := s.p14SeasonCalendar(tx, season.SeasonIndex)
	if err != nil {
		return nil, err
	}
	var start, beforeEnd, fightStart, fightEnd int64
	for _, slot := range calendar {
		if slot.Stage == constUtil.P14Final16BeforeStage {
			start, beforeEnd = slot.StartAt, slot.EndAt
		}
		if slot.Stage == constUtil.P14Final16Stage {
			fightStart, fightEnd = slot.StartAt, slot.EndAt
		}
	}
	if beforeEnd != fightStart || start <= 0 || fightStart-start <= constUtil.P14FinalFormationLockMillis {
		return nil, errors.New(constUtil.P14FinalFormationCorrupt)
	}
	var offer db.P14FinalFormationOffer
	err = tx.First(&offer, "season_index=? AND round_stage=?", season.SeasonIndex, constUtil.P14Final16Stage).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		var gate db.P14FinalFormationPolicy
		if err = tx.First(&gate, 1).Error; err != nil {
			return nil, err
		}
		if !season.Enabled || !gate.Enabled {
			return nil, errors.New(constUtil.P14FinalFormationClosed)
		}
		offer = db.P14FinalFormationOffer{SeasonIndex: season.SeasonIndex, RoundStage: constUtil.P14Final16Stage, Version: gate.Version, Source: gate.Source, PublishedAt: gate.PublishedAt, QualificationHash: certificate.ContentHash, StageStart: start, FightStart: fightStart, FightEnd: fightEnd, LockAt: fightStart - constUtil.P14FinalFormationLockMillis}
		offer.SourceHash = p14FinalFormationOfferHash(&offer)
		if err = p14FinalFormationValidateOffer(&offer); err != nil {
			return nil, err
		}
		if err = tx.Create(&offer).Error; err != nil {
			return nil, err
		}
	} else if err != nil {
		return nil, err
	}
	if err = p14FinalFormationValidateOffer(&offer); err != nil {
		return nil, err
	}
	if offer.QualificationHash != certificate.ContentHash || offer.StageStart != start || offer.FightStart != fightStart || offer.FightEnd != fightEnd {
		return nil, errors.New(constUtil.P14FinalFormationCorrupt)
	}
	return &offer, nil
}

func p14FinalFormationRead(row *db.P14FinalFormation, offer *db.P14FinalFormationOffer) (*pb.FormationSimpleInfo, error) {
	if row == nil {
		return nil, errors.New(constUtil.P14FinalFormationCorrupt)
	}
	if err := p14FinalFormationCaptureAllowed(offer, row.CapturedAt); err != nil {
		return nil, err
	}
	frame := &pb.FormationSimpleInfo{}
	battle := &pb.BattlePlayer{}
	if row.SeasonIndex != offer.SeasonIndex || row.RoundStage != offer.RoundStage || row.Source != constUtil.P14FinalFormationSource || row.SourceHash != p14FinalFormationHash(row) || row.QualificationHash != offer.QualificationHash || row.OfferHash != offer.SourceHash || row.Revision <= 0 || row.ServerID <= 0 || row.PlayerID <= 0 || row.SnapshotHash != p14SnapshotDigest(row.Snapshot, row.Battle) || proto.Unmarshal(row.Snapshot, frame) != nil || proto.Unmarshal(row.Battle, battle) != nil || frame.GetFightPower() != row.FightPower || battle.GetPlayerId() != row.PlayerID || int64(battle.GetServerId()) != row.ServerID || len(battle.Formation) == 0 {
		return nil, errors.New(constUtil.P14FinalFormationCorrupt)
	}
	return frame, nil
}

// p14FinalFormationUpload uses the same authenticated, freshly hydrated profile
// transaction as original81030 and records its signed purpose3 outbox atomically.
func (s *GameService) p14FinalFormationUpload(ctx context.Context, tx *gorm.DB, policy *db.P14SeasonPolicy, player *pb.Player, game *model.GameState, now time.Time) (*db.P14FinalFormation, error) {
	server, id := s.OriginalServerID(), player.GetPlayerId()
	offer, err := s.p14FinalFormationOfferTX(ctx, tx, policy, server, id, now)
	if err != nil {
		return nil, err
	}
	var gate db.P14FinalFormationPolicy
	if err = tx.First(&gate, 1).Error; err != nil {
		return nil, err
	}
	if !policy.Enabled || !gate.Enabled {
		return nil, errors.New(constUtil.P14FinalFormationClosed)
	}
	if err = p14FinalFormationCaptureAllowed(offer, now.UnixMilli()); err != nil {
		return nil, err
	}
	var previous db.P14FinalFormation
	err = tx.First(&previous, "season_index=? AND round_stage=? AND server_id=? AND player_id=?", policy.SeasonIndex, offer.RoundStage, server, id).Error
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}
	if err == nil {
		if _, err = p14FinalFormationRead(&previous, offer); err != nil || previous.CapturedAt > now.UnixMilli() {
			return nil, errors.New(constUtil.P14FinalFormationCorrupt)
		}
	}
	if previous.Revision == math.MaxInt64 {
		return nil, errors.New(constUtil.P14FinalFormationCorrupt)
	}
	battle, err := s.battlePlayer(player, game)
	if err != nil {
		return nil, err
	}
	frame, err := s.p10FormationProfile(tx, id)
	if err != nil {
		return nil, err
	}
	encodedBattle, err := proto.Marshal(battle)
	if err != nil {
		return nil, err
	}
	encodedFrame, err := proto.Marshal(frame)
	if err != nil {
		return nil, err
	}
	configHash, err := s.p14ConfigurationHash()
	if err != nil {
		return nil, err
	}
	row := db.P14FinalFormation{SeasonIndex: policy.SeasonIndex, RoundStage: offer.RoundStage, ServerID: server, PlayerID: id, Revision: previous.Revision + 1, CapturedAt: now.UnixMilli(), FightPower: frame.GetFightPower(), Snapshot: encodedFrame, Battle: encodedBattle, SnapshotHash: p14SnapshotDigest(encodedFrame, encodedBattle), ClientVersion: constUtil.P6OriginalClientHash, EngineVersion: constUtil.P6EngineVersion, ConfigurationHash: configHash, QualificationHash: offer.QualificationHash, OfferHash: offer.SourceHash, Source: constUtil.P14FinalFormationSource}
	row.SourceHash = p14FinalFormationHash(&row)
	if _, err = p14FinalFormationRead(&row, offer); err != nil {
		return nil, err
	}
	if err = tx.Save(&row).Error; err != nil {
		return nil, err
	}
	var peers []db.P14TrustedPeer
	if err = tx.Where("enabled=1 AND server_id<>?", server).Find(&peers).Error; err != nil {
		return nil, err
	}
	if len(peers) == 0 {
		return nil, errors.New(constUtil.P14FinalFormationCorrupt)
	}
	for _, peer := range peers {
		if len(peer.SigningKey) < 32 || peer.Endpoint == "" {
			return nil, errors.New(constUtil.P14FinalFormationCorrupt)
		}
		request := model.P14SnapshotImportRequest{ProtocolVersion: constUtil.P14FinalFormationProtocolVersion, SourceServerID: server, DestinationServerID: peer.ServerID, SeasonIndex: row.SeasonIndex, PlayerID: id, Revision: row.Revision, CapturedAt: row.CapturedAt, FightPower: row.FightPower, Formation: row.Snapshot, Battle: row.Battle, SnapshotHash: row.SnapshotHash, ConfigurationHash: row.ConfigurationHash, ClientVersion: row.ClientVersion, EngineVersion: row.EngineVersion, RoundStage: row.RoundStage, FinalQualificationHash: row.QualificationHash, FinalOfferHash: row.OfferHash}
		request.DeliveryID = p14FinalFormationID(request)
		raw, err := json.Marshal(request)
		if err != nil {
			return nil, err
		}
		if err = tx.Create(&db.P14Delivery{ID: request.DeliveryID, DestinationServerID: peer.ServerID, Payload: raw, CreatedAt: now.UnixMilli()}).Error; err != nil {
			return nil, err
		}
	}
	return &row, nil
}

// p14FinalFormationImport validates the signed capture time, complete shared
// qualification and exact round offer even when delivery follows the lock.
func (s *GameService) p14FinalFormationImport(ctx context.Context, tx *gorm.DB, policy *db.P14SeasonPolicy, request model.P14SnapshotImportRequest, now time.Time) error {
	if request.SeasonIndex != policy.SeasonIndex || request.CapturedAt > now.UnixMilli() || request.Revision <= 0 || request.RoundStage != constUtil.P14Final16Stage {
		return errors.New(constUtil.P14FinalFormationCorrupt)
	}
	offer, err := s.p14FinalFormationOfferTX(ctx, tx, policy, request.SourceServerID, request.PlayerID, now)
	if err != nil {
		return err
	}
	configHash, err := s.p14ConfigurationHash()
	if err != nil {
		return err
	}
	if request.ClientVersion != constUtil.P6OriginalClientHash || request.EngineVersion != constUtil.P6EngineVersion || request.ConfigurationHash != configHash {
		return errors.New(constUtil.P14FinalFormationCorrupt)
	}
	row := db.P14FinalFormation{SeasonIndex: request.SeasonIndex, RoundStage: request.RoundStage, ServerID: request.SourceServerID, PlayerID: request.PlayerID, Revision: request.Revision, CapturedAt: request.CapturedAt, FightPower: request.FightPower, Snapshot: append([]byte(nil), request.Formation...), Battle: append([]byte(nil), request.Battle...), SnapshotHash: request.SnapshotHash, ClientVersion: request.ClientVersion, EngineVersion: request.EngineVersion, ConfigurationHash: request.ConfigurationHash, QualificationHash: request.FinalQualificationHash, OfferHash: request.FinalOfferHash, Source: constUtil.P14FinalFormationSource}
	row.SourceHash = p14FinalFormationHash(&row)
	if _, err = p14FinalFormationRead(&row, offer); err != nil {
		return err
	}
	var previous db.P14FinalFormation
	err = tx.First(&previous, "season_index=? AND round_stage=? AND server_id=? AND player_id=?", row.SeasonIndex, row.RoundStage, row.ServerID, row.PlayerID).Error
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}
	if err == nil {
		if _, err = p14FinalFormationRead(&previous, offer); err != nil {
			return err
		}
		if previous.Revision == row.Revision && previous.SourceHash != row.SourceHash || row.Revision > previous.Revision && row.CapturedAt < previous.CapturedAt {
			return errors.New(constUtil.P14FinalFormationCorrupt)
		}
	}
	if errors.Is(err, gorm.ErrRecordNotFound) || row.Revision > previous.Revision {
		return tx.Save(&row).Error
	}
	return nil
}

func (s *GameService) p14FinalFormationQuery(ctx context.Context, tx *gorm.DB, policy *db.P14SeasonPolicy, server, player int64, now time.Time) (*pb.FormationSimpleInfo, error) {
	offer, err := s.p14FinalFormationOfferTX(ctx, tx, policy, server, player, now)
	if err != nil {
		return nil, err
	}
	stage, _, err := s.p14SeasonStage(tx, policy, now)
	if err != nil || stage != constUtil.P14Final16BeforeStage && stage != constUtil.P14Final16Stage {
		return nil, errors.New(constUtil.P14ResultsUnverified)
	}
	var row db.P14FinalFormation
	err = tx.First(&row, "season_index=? AND round_stage=? AND server_id=? AND player_id=?", policy.SeasonIndex, offer.RoundStage, server, player).Error
	if err == nil {
		if row.CapturedAt > now.UnixMilli() {
			return nil, errors.New(constUtil.P14FinalFormationCorrupt)
		}
		return p14FinalFormationRead(&row, offer)
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}
	// Without a final upload, retain the actual frozen promotion formation. The
	// first final never silently recaptures today's mutable public profile.
	var promotion db.P14PromotionFormation
	err = tx.First(&promotion, "season_index=? AND server_id=? AND player_id=?", policy.SeasonIndex, server, player).Error
	if err == nil {
		_, qualified, err := s.p14PromotionRankQualification(ctx, tx, policy.SeasonIndex, server, now)
		if err != nil {
			return nil, err
		}
		stage, _, err := s.p14SeasonStage(tx, policy, time.UnixMilli(promotion.CapturedAt))
		if err != nil || stage != constUtil.P14PromotionStage || promotion.CapturedAt > now.UnixMilli() || promotion.QualificationHash != qualified.SourceHash {
			return nil, errors.New(constUtil.P14FinalFormationCorrupt)
		}
		return p14PromotionFormationRead(&promotion)
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}
	roster, _, err := s.p14PromotionRankQualification(ctx, tx, policy.SeasonIndex, server, now)
	if err != nil {
		return nil, err
	}
	for _, frozen := range roster {
		if frozen.ServerID == server && frozen.PlayerID == player {
			frame := &pb.FormationSimpleInfo{}
			if err = proto.Unmarshal(frozen.Snapshot, frame); err != nil {
				return nil, err
			}
			if frame.GetFightPower() != frozen.FightPower {
				return nil, errors.New(constUtil.P14FinalFormationCorrupt)
			}
			return frame, nil
		}
	}
	return nil, errors.New(constUtil.P14FinalFormationCorrupt)
}
