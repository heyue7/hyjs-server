package service

import (
	"bytes"
	"context"
	"crypto/hmac"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"

	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/httpClient"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

type p14PerGameSealBody struct {
	Offer    db.P14PerGameOffer       `json:"offer"`
	ServerID int64                    `json:"serverId"`
	SealedAt int64                    `json:"sealedAt"`
	Members  []db.P14PerGameFormation `json:"members"`
}
type p14PerGameSnapshot struct {
	Kind      int32                   `json:"kind"`
	Offer     db.P14PerGameOffer      `json:"offer"`
	Formation *db.P14PerGameFormation `json:"formation,omitempty"`
	Seal      *p14PerGameSealBody     `json:"seal,omitempty"`
}

func p14PerGameOfferHash(row *db.P14PerGameOffer) string {
	copy := *row
	copy.SourceHash = ""
	raw, _ := json.Marshal(copy)
	return p6Digest(raw)
}
func p14PerGameFormationHash(row *db.P14PerGameFormation) string {
	copy := *row
	copy.SourceHash = ""
	raw, _ := json.Marshal(copy)
	return p6Digest(raw)
}
func p14PerGameDeliveryID(request model.P14PerGameImportRequest) string {
	return p6Digest([]byte(fmt.Sprintf("p14/per-game/%d/%d/%d/%s", request.ProtocolVersion, request.SourceServerID, request.DestinationServerID, p6Digest(request.Snapshot))))
}

// Derive the next game solely from the frozen server calendar. A locked game
// never rolls forward early into another upload window. No client field picks it.
func p14PerGameCurrentSlot(manifest *p14FinalSeriesManifest, at int64) (int32, error) {
	if manifest == nil || len(manifest.Slots) != int(constUtil.P14FinalSeriesMaxGames) {
		return 0, errors.New(constUtil.P14PerGameCorrupt)
	}
	if at < manifest.Slots[0] {
		return 0, nil
	}
	for index := 1; index < len(manifest.Slots); index++ {
		if at < manifest.Slots[index] {
			if at >= manifest.Slots[index]-constUtil.P14FinalFormationLockMillis {
				return 0, errors.New(constUtil.P14PerGameClosed)
			}
			return int32(index + 1), nil
		}
	}
	return 0, errors.New(constUtil.P14PerGameClosed)
}
func p14PerGameMember(manifest *p14FinalSeriesManifest, server, player int64) bool {
	pair := manifest.Pair
	return pair.LeftServerID == server && pair.LeftPlayerID == player || pair.RightServerID == server && pair.RightPlayerID == player
}
func p14PerGameOrigins(manifest *p14FinalSeriesManifest) []int64 {
	if manifest.Pair.LeftServerID == manifest.Pair.RightServerID {
		return []int64{manifest.Pair.LeftServerID}
	}
	if manifest.Pair.LeftServerID < manifest.Pair.RightServerID {
		return []int64{manifest.Pair.LeftServerID, manifest.Pair.RightServerID}
	}
	return []int64{manifest.Pair.RightServerID, manifest.Pair.LeftServerID}
}
func p14PerGameReadSeries(row *db.P14FinalSeries) (*p14FinalSeriesManifest, error) {
	var manifest p14FinalSeriesManifest
	if row == nil || row.ManifestHash != p6Digest(row.Manifest) || p14PromotionRankDecode(row.Manifest, &manifest) != nil || manifest.Rules != constUtil.P14FinalSeriesRules || len(manifest.Slots) != 5 || manifest.FirstInputID != row.GameOneID {
		return nil, errors.New(constUtil.P14PerGameCorrupt)
	}
	return &manifest, nil
}
func p14PerGameReadOffer(offer *db.P14PerGameOffer, series *db.P14FinalSeries, manifest *p14FinalSeriesManifest) error {
	var policy db.P14PerGamePolicy
	if offer == nil || offer.SeriesID != series.ID || offer.SeriesManifestHash != series.ManifestHash || offer.GameIndex < 2 || offer.GameIndex > 5 || p14PromotionRankDecode(offer.Policy, &policy) != nil || !policy.Enabled || policy.Source != constUtil.P14PerGameSource || strings.TrimSpace(policy.Version) == "" || len(policy.Version) > 128 || policy.PublishedAt <= 0 || policy.PublishedAt > manifest.FormationOffer.StageStart || offer.OpenAt != manifest.Slots[offer.GameIndex-2] || offer.ScheduledAt != manifest.Slots[offer.GameIndex-1] || offer.LockAt != offer.ScheduledAt-constUtil.P14FinalFormationLockMillis || offer.OpenAt >= offer.LockAt || offer.SourceHash != p14PerGameOfferHash(offer) {
		return errors.New(constUtil.P14PerGameCorrupt)
	}
	return nil
}
func p14PerGameReadFormation(row *db.P14PerGameFormation, offer *db.P14PerGameOffer, manifest *p14FinalSeriesManifest) error {
	frame := &pb.FormationSimpleInfo{}
	battle := &pb.BattlePlayer{}
	if row == nil || row.SeriesID != offer.SeriesID || row.GameIndex != offer.GameIndex || row.OfferHash != offer.SourceHash || !p14PerGameMember(manifest, row.ServerID, row.PlayerID) || row.Revision <= 0 || row.CapturedAt < offer.OpenAt || row.CapturedAt >= offer.LockAt || row.SnapshotHash != p14SnapshotDigest(row.Snapshot, row.Battle) || row.SourceHash != p14PerGameFormationHash(row) || proto.Unmarshal(row.Snapshot, frame) != nil || proto.Unmarshal(row.Battle, battle) != nil || frame.GetFightPower() != row.FightPower || battle.GetPlayerId() != row.PlayerID || int64(battle.GetServerId()) != row.ServerID || len(battle.Formation) == 0 || len(row.ConfigurationHash) != 64 || row.EngineVersion == "" || row.ClientVersion == "" {
		return errors.New(constUtil.P14PerGameCorrupt)
	}
	return nil
}

func (s *GameService) InitializeP14PerGame(ctx context.Context) error {
	tx := db.GetIns().WithContext(ctx)
	if err := tx.AutoMigrate(&db.P14PerGamePolicy{}, &db.P14PerGameOffer{}, &db.P14PerGameFormation{}, &db.P14PerGameSeal{}, &db.P14PerGameDelivery{}); err != nil {
		return err
	}
	return tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&db.P14PerGamePolicy{ID: 1, Source: constUtil.P14PerGameSource}).Error
}

func (s *GameService) p14PerGameOfferTX(ctx context.Context, tx *gorm.DB, series *db.P14FinalSeries, index int32) (*db.P14PerGameOffer, *p14FinalSeriesManifest, error) {
	manifest, err := p14PerGameReadSeries(series)
	if err != nil {
		return nil, nil, err
	}
	if index < 2 || index > 5 {
		return nil, nil, errors.New(constUtil.P14PerGameClosed)
	}
	var offer db.P14PerGameOffer
	err = tx.First(&offer, "series_id=? AND game_index=?", series.ID, index).Error
	if err == nil {
		return &offer, manifest, p14PerGameReadOffer(&offer, series, manifest)
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil, err
	}
	var policy db.P14PerGamePolicy
	if err = tx.First(&policy, 1).Error; err != nil {
		return nil, nil, err
	}
	if !policy.Enabled {
		return nil, nil, errors.New(constUtil.P14PerGameClosed)
	}
	raw, err := json.Marshal(policy)
	if err != nil {
		return nil, nil, err
	}
	offer = db.P14PerGameOffer{SeriesID: series.ID, GameIndex: index, SeriesManifestHash: series.ManifestHash, Policy: raw, OpenAt: manifest.Slots[index-2], LockAt: manifest.Slots[index-1] - constUtil.P14FinalFormationLockMillis, ScheduledAt: manifest.Slots[index-1]}
	offer.SourceHash = p14PerGameOfferHash(&offer)
	if err = p14PerGameReadOffer(&offer, series, manifest); err != nil {
		return nil, nil, err
	}
	if err = tx.Create(&offer).Error; err != nil {
		return nil, nil, err
	}
	return &offer, manifest, nil
}

func (s *GameService) p14PerGameQueueTX(tx *gorm.DB, body p14PerGameSnapshot, now time.Time) error {
	raw, err := json.Marshal(body)
	if err != nil {
		return err
	}
	var peers []db.P14TrustedPeer
	if err = tx.Where("enabled=1 AND server_id<>?", s.OriginalServerID()).Order("server_id").Find(&peers).Error; err != nil {
		return err
	}
	for _, peer := range peers {
		if len(peer.SigningKey) < 32 || peer.Endpoint == "" {
			return errors.New(constUtil.P14PerGameCorrupt)
		}
		envelope := model.P14PerGameImportRequest{ProtocolVersion: 1, SourceServerID: s.OriginalServerID(), DestinationServerID: peer.ServerID, Snapshot: raw}
		envelope.DeliveryID = p14PerGameDeliveryID(envelope)
		encoded, err := json.Marshal(envelope)
		if err != nil {
			return err
		}
		if err = tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&db.P14PerGameDelivery{ID: envelope.DeliveryID, DestinationServerID: peer.ServerID, Payload: encoded, PayloadHash: p6Digest(encoded), CreatedAt: now.UnixMilli()}).Error; err != nil {
			return err
		}
	}
	return nil
}

// Original81030 remains unchanged. A live series and current server slot select
// the independent next-game row. Game1/old purpose3 signing is left untouched.
func (s *GameService) P14FinalFormationUploadCurrent(ctx context.Context, tx *gorm.DB, season *db.P14SeasonPolicy, player *pb.Player, state *model.GameState, now time.Time) (int32, error) {
	if !tx.Migrator().HasTable(&db.P14PerGamePolicy{}) {
		old, err := s.p14FinalFormationUpload(ctx, tx, season, player, state, now)
		if err != nil {
			return 0, err
		}
		return old.FightPower, nil
	}
	var gate db.P14PerGamePolicy
	if err := tx.First(&gate, 1).Error; err != nil {
		return 0, err
	}
	var seriesRows []db.P14FinalSeries
	if gate.Enabled {
		if err := tx.Where("season_index=? AND round_stage=?", season.SeasonIndex, constUtil.P14Final16Stage).Find(&seriesRows).Error; err != nil {
			return 0, err
		}
	}
	for position := range seriesRows {
		series := &seriesRows[position]
		manifest, err := p14PerGameReadSeries(series)
		if err != nil {
			return 0, err
		}
		if !p14PerGameMember(manifest, s.OriginalServerID(), player.GetPlayerId()) {
			continue
		}
		index, err := p14PerGameCurrentSlot(manifest, now.UnixMilli())
		if err != nil {
			return 0, err
		}
		if index == 0 {
			break
		}
		if !season.Enabled || series.State == constUtil.P14FinalSeriesComplete {
			return 0, errors.New(constUtil.P14PerGameClosed)
		}
		offer, manifest, err := s.p14PerGameOfferTX(ctx, tx, series, index)
		if err != nil {
			return 0, err
		}
		var sealed int64
		if err := tx.Model(&db.P14PerGameSeal{}).Where("series_id=? AND game_index=? AND server_id=?", series.ID, index, s.OriginalServerID()).Count(&sealed).Error; err != nil {
			return 0, err
		}
		if sealed != 0 {
			return 0, errors.New(constUtil.P14PerGameClosed)
		}
		var previous db.P14PerGameFormation
		err = tx.First(&previous, "series_id=? AND game_index=? AND server_id=? AND player_id=?", series.ID, index, s.OriginalServerID(), player.GetPlayerId()).Error
		if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return 0, err
		}
		if err == nil {
			if err = p14PerGameReadFormation(&previous, offer, manifest); err != nil || previous.CapturedAt > now.UnixMilli() {
				return 0, errors.New(constUtil.P14PerGameCorrupt)
			}
		}
		if previous.Revision == math.MaxInt64 {
			return 0, errors.New(constUtil.P14PerGameCorrupt)
		}
		battle, err := s.battlePlayer(player, state)
		if err != nil {
			return 0, err
		}
		frame, err := s.p10FormationProfile(tx, player.GetPlayerId())
		if err != nil {
			return 0, err
		}
		rawBattle, err := proto.Marshal(battle)
		if err != nil {
			return 0, err
		}
		rawFrame, err := proto.Marshal(frame)
		if err != nil {
			return 0, err
		}
		configHash, err := s.p14ConfigurationHash()
		if err != nil {
			return 0, err
		}
		row := db.P14PerGameFormation{SeriesID: series.ID, GameIndex: index, ServerID: s.OriginalServerID(), PlayerID: player.GetPlayerId(), Revision: previous.Revision + 1, CapturedAt: now.UnixMilli(), FightPower: frame.GetFightPower(), Snapshot: rawFrame, Battle: rawBattle, SnapshotHash: p14SnapshotDigest(rawFrame, rawBattle), ConfigurationHash: configHash, EngineVersion: constUtil.P6EngineVersion, ClientVersion: constUtil.P6OriginalClientHash, OfferHash: offer.SourceHash}
		row.SourceHash = p14PerGameFormationHash(&row)
		if err = p14PerGameReadFormation(&row, offer, manifest); err != nil {
			return 0, err
		}
		if err = tx.Save(&row).Error; err != nil {
			return 0, err
		}
		if err = s.p14PerGameQueueTX(tx, p14PerGameSnapshot{Kind: constUtil.P14PerGameFormationKind, Offer: *offer, Formation: &row}, now); err != nil {
			return 0, err
		}
		return row.FightPower, nil
	}
	old, err := s.p14FinalFormationUpload(ctx, tx, season, player, state, now)
	if err != nil {
		return 0, err
	}
	return old.FightPower, nil
}

func p14PerGameSealValidate(body *p14PerGameSealBody, series *db.P14FinalSeries, manifest *p14FinalSeriesManifest, now time.Time) error {
	if body == nil || body.SealedAt < body.Offer.LockAt || body.SealedAt > now.UnixMilli() || body.ServerID <= 0 || p14PerGameReadOffer(&body.Offer, series, manifest) != nil {
		return errors.New(constUtil.P14PerGameCorrupt)
	}
	expected := map[int64]bool{}
	for _, joint := range [][2]int64{{manifest.Pair.LeftServerID, manifest.Pair.LeftPlayerID}, {manifest.Pair.RightServerID, manifest.Pair.RightPlayerID}} {
		if joint[0] == body.ServerID {
			expected[joint[1]] = true
		}
	}
	if len(expected) == 0 || len(body.Members) != len(expected) {
		return errors.New(constUtil.P14PerGameCorrupt)
	}
	var last int64
	for index := range body.Members {
		member := &body.Members[index]
		if member.ServerID != body.ServerID || !expected[member.PlayerID] || member.PlayerID <= last || p14PerGameReadFormation(member, &body.Offer, manifest) != nil {
			return errors.New(constUtil.P14PerGameCorrupt)
		}
		delete(expected, member.PlayerID)
		last = member.PlayerID
	}
	return nil
}

// Old/out-of-order deliveries may acknowledge history but cannot overwrite a
// higher actual revision or contradict the source's already-frozen game seal.
func p14PerGameApplyFormation(tx *gorm.DB, row *db.P14PerGameFormation, offer *db.P14PerGameOffer, manifest *p14FinalSeriesManifest) error {
	if err := p14PerGameReadFormation(row, offer, manifest); err != nil {
		return err
	}
	var previous db.P14PerGameFormation
	err := tx.First(&previous, "series_id=? AND game_index=? AND server_id=? AND player_id=?", row.SeriesID, row.GameIndex, row.ServerID, row.PlayerID).Error
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}
	if err == nil {
		if err = p14PerGameReadFormation(&previous, offer, manifest); err != nil {
			return err
		}
		if previous.Revision > row.Revision {
			return nil
		}
		if previous.Revision == row.Revision {
			if previous.SourceHash != row.SourceHash {
				return errors.New(constUtil.P14PerGameCorrupt)
			}
			return nil
		}
		if previous.CapturedAt > row.CapturedAt {
			return errors.New(constUtil.P14PerGameCorrupt)
		}
	}
	var sealed db.P14PerGameSeal
	err = tx.First(&sealed, "series_id=? AND game_index=? AND server_id=?", row.SeriesID, row.GameIndex, row.ServerID).Error
	if err == nil {
		var body p14PerGameSealBody
		if sealed.ContentHash != p6Digest(sealed.Content) || p14PromotionRankDecode(sealed.Content, &body) != nil {
			return errors.New(constUtil.P14PerGameCorrupt)
		}
		found := false
		for _, member := range body.Members {
			if member.PlayerID == row.PlayerID && member.SourceHash == row.SourceHash {
				found = true
			}
		}
		if !found {
			return errors.New(constUtil.P14PerGameCorrupt)
		}
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}
	return tx.Save(row).Error
}

// ImportP14PerGame accepts a separate exact-raw HMAC envelope. The old157 seal
// and v1/2/3 formation contracts retain their original bytes and interpretation.
func (s *GameService) ImportP14PerGame(ctx context.Context, request model.P14PerGameImportRequest, raw []byte, signature string) (*model.P14PerGameImportResponse, error) {
	now := p14PromotionRankClock(ctx)
	var signed model.P14PerGameImportRequest
	if p14PromotionRankDecode(raw, &signed) != nil {
		return nil, errors.New(constUtil.P14PerGameCorrupt)
	}
	interpreted, _ := json.Marshal(request)
	exact, _ := json.Marshal(signed)
	if !bytes.Equal(interpreted, exact) {
		return nil, errors.New(constUtil.P14PerGameCorrupt)
	}
	reply := &model.P14PerGameImportResponse{DeliveryID: request.DeliveryID}
	err := db.GetIns().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := ServerGameClockGuardTransaction(tx); err != nil {
			return err
		}

		var season db.P14SeasonPolicy
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&season, 1).Error; err != nil {
			return err
		}
		var peer db.P14TrustedPeer
		if err := tx.First(&peer, "server_id=? AND enabled=1", request.SourceServerID).Error; err != nil {
			return errors.New(constUtil.P14PerGameCorrupt)
		}
		actual, err := hex.DecodeString(signature)
		expected, _ := hex.DecodeString(P14SignPayload(peer.SigningKey, raw))
		if err != nil || len(peer.SigningKey) < 32 || !hmac.Equal(actual, expected) || request.ProtocolVersion != constUtil.P14PerGameProtocolVersion || request.SourceServerID == s.OriginalServerID() || request.DestinationServerID != s.OriginalServerID() || request.DeliveryID != p14PerGameDeliveryID(request) {
			return errors.New(constUtil.P14PerGameCorrupt)
		}
		var receipt db.P14ImportReceipt
		err = tx.First(&receipt, "id=?", request.DeliveryID).Error
		if err == nil {
			if receipt.PayloadHash != p6Digest(raw) || receipt.SourceServerID != request.SourceServerID {
				return errors.New(constUtil.P14PerGameCorrupt)
			}
			reply.Confirmed = true
			return nil
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		var body p14PerGameSnapshot
		if p14PromotionRankDecode(request.Snapshot, &body) != nil {
			return errors.New(constUtil.P14PerGameCorrupt)
		}
		var series db.P14FinalSeries
		if err = tx.First(&series, "id=? AND season_index=?", body.Offer.SeriesID, season.SeasonIndex).Error; err != nil {
			return err
		}
		manifest, err := p14PerGameReadSeries(&series)
		if err != nil {
			return err
		}
		if err = p14PerGameReadOffer(&body.Offer, &series, manifest); err != nil {
			return err
		}
		var offer db.P14PerGameOffer
		err = tx.First(&offer, "series_id=? AND game_index=?", series.ID, body.Offer.GameIndex).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			// A closed switch can fulfill the same previously published local version.
			// An unknown or changed policy cannot silently become a new offer.
			var local db.P14PerGamePolicy
			if err = tx.First(&local, 1).Error; err != nil {
				return err
			}
			local.Enabled = true
			policyRaw, _ := json.Marshal(local)
			if !bytes.Equal(policyRaw, body.Offer.Policy) {
				return errors.New(constUtil.P14PerGameClosed)
			}
			offer = body.Offer
			if err = tx.Create(&offer).Error; err != nil {
				return err
			}
		} else if err != nil {
			return err
		} else if offer.SourceHash != body.Offer.SourceHash {
			return errors.New(constUtil.P14PerGameCorrupt)
		}
		switch body.Kind {
		case constUtil.P14PerGameFormationKind:
			if body.Formation == nil || body.Seal != nil || body.Formation.ServerID != request.SourceServerID || body.Formation.CapturedAt > now.UnixMilli() {
				return errors.New(constUtil.P14PerGameCorrupt)
			}
			configHash, err := s.p14ConfigurationHash()
			if err != nil {
				return err
			}
			if body.Formation.ConfigurationHash != configHash || body.Formation.EngineVersion != constUtil.P6EngineVersion || body.Formation.ClientVersion != constUtil.P6OriginalClientHash {
				return errors.New(constUtil.P14PerGameCorrupt)
			}
			if err = p14PerGameApplyFormation(tx, body.Formation, &offer, manifest); err != nil {
				return err
			}
		case constUtil.P14PerGameSealKind:
			if body.Seal == nil || body.Formation != nil || body.Seal.ServerID != request.SourceServerID || body.Seal.Offer.SourceHash != offer.SourceHash {
				return errors.New(constUtil.P14PerGameCorrupt)
			}
			if err = p14PerGameSealValidate(body.Seal, &series, manifest, now); err != nil {
				return err
			}
			var previous db.P14PerGameSeal
			encoded, _ := json.Marshal(body.Seal)
			digest := p6Digest(encoded)
			err = tx.First(&previous, "series_id=? AND game_index=? AND server_id=?", series.ID, offer.GameIndex, request.SourceServerID).Error
			if err == nil {
				if previous.ContentHash != digest {
					return errors.New(constUtil.P14PerGameCorrupt)
				}
			} else if !errors.Is(err, gorm.ErrRecordNotFound) {
				return err
			} else {
				for index := range body.Seal.Members {
					member := &body.Seal.Members[index]
					if err = p14PerGameApplyFormation(tx, member, &offer, manifest); err != nil {
						return err
					}
					var observed db.P14PerGameFormation
					if err = tx.First(&observed, "series_id=? AND game_index=? AND server_id=? AND player_id=?", series.ID, offer.GameIndex, member.ServerID, member.PlayerID).Error; err != nil {
						return err
					}
					if observed.SourceHash != member.SourceHash {
						return errors.New(constUtil.P14PerGameCorrupt)
					}
				}
				if err = tx.Create(&db.P14PerGameSeal{SeriesID: series.ID, GameIndex: offer.GameIndex, ServerID: request.SourceServerID, Content: encoded, ContentHash: digest, SealedAt: body.Seal.SealedAt}).Error; err != nil {
					return err
				}
			}
		default:
			return errors.New(constUtil.P14PerGameCorrupt)
		}
		if err = tx.Create(&db.P14ImportReceipt{ID: request.DeliveryID, PayloadHash: p6Digest(raw), SourceServerID: request.SourceServerID, ImportedAt: now.UnixMilli()}).Error; err != nil {
			return err
		}
		reply.Confirmed = true
		return nil
	})
	if err != nil {
		return nil, err
	}
	return reply, nil
}

// QueueP14PerGameSeals freezes only this origin's real uploads after each own
// lock. Missing uploads remain pending; Game1/promotion rows are never copied.
func (s *GameService) QueueP14PerGameSeals(ctx context.Context, now time.Time) error {
	return db.GetIns().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := ServerGameClockGuardTransaction(tx); err != nil {
			return err
		}

		var season db.P14SeasonPolicy
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&season, 1).Error; err != nil {
			return err
		}
		var gate db.P14PerGamePolicy
		if err := tx.First(&gate, 1).Error; err != nil {
			return err
		}
		if !season.Enabled || !gate.Enabled {
			return nil
		}
		var rows []db.P14FinalSeries
		if err := tx.Where("season_index=? AND round_stage=?", season.SeasonIndex, constUtil.P14Final16Stage).Order("group_id,pair_index").Find(&rows).Error; err != nil {
			return err
		}
		for position := range rows {
			series := &rows[position]
			if series.State == constUtil.P14FinalSeriesComplete {
				continue
			}
			manifest, err := p14PerGameReadSeries(series)
			if err != nil {
				return err
			}
			var members []int64
			for _, joint := range [][2]int64{{manifest.Pair.LeftServerID, manifest.Pair.LeftPlayerID}, {manifest.Pair.RightServerID, manifest.Pair.RightPlayerID}} {
				if joint[0] == s.OriginalServerID() {
					members = append(members, joint[1])
				}
			}
			if len(members) == 0 {
				continue
			}
			for index := int32(2); index <= 5; index++ {
				if now.UnixMilli() < manifest.Slots[index-1]-constUtil.P14FinalFormationLockMillis {
					continue
				}
				var existing db.P14PerGameSeal
				err := tx.First(&existing, "series_id=? AND game_index=? AND server_id=?", series.ID, index, s.OriginalServerID()).Error
				if err == nil {
					if existing.ContentHash != p6Digest(existing.Content) {
						return errors.New(constUtil.P14PerGameCorrupt)
					}
					continue
				}
				if !errors.Is(err, gorm.ErrRecordNotFound) {
					return err
				}
				offer, manifest, err := s.p14PerGameOfferTX(ctx, tx, series, index)
				if err != nil {
					return err
				}
				var actual []db.P14PerGameFormation
				if err = tx.Where("series_id=? AND game_index=? AND server_id=? AND player_id IN ?", series.ID, index, s.OriginalServerID(), members).Order("player_id").Find(&actual).Error; err != nil {
					return err
				}
				if len(actual) != len(members) {
					continue
				}
				body := p14PerGameSealBody{Offer: *offer, ServerID: s.OriginalServerID(), SealedAt: now.UnixMilli(), Members: actual}
				if err = p14PerGameSealValidate(&body, series, manifest, now); err != nil {
					return err
				}
				encoded, err := json.Marshal(body)
				if err != nil {
					return err
				}
				if err = tx.Create(&db.P14PerGameSeal{SeriesID: series.ID, GameIndex: index, ServerID: body.ServerID, Content: encoded, ContentHash: p6Digest(encoded), SealedAt: body.SealedAt}).Error; err != nil {
					return err
				}
				if err = s.p14PerGameQueueTX(tx, p14PerGameSnapshot{Kind: constUtil.P14PerGameSealKind, Offer: *offer, Seal: &body}, now); err != nil {
					return err
				}
			}
		}
		return nil
	})
}

// DeliverP14PerGame shares the existing bounded worker. Exact payload bytes
// remain in its own outbox until the peer durably confirms that delivery.
func (s *GameService) DeliverP14PerGame(ctx context.Context) error {
	tx := db.GetIns().WithContext(ctx)
	var gate db.P14PerGamePolicy
	if err := tx.First(&gate, 1).Error; err != nil {
		return err
	}
	if !gate.Enabled {
		return nil
	}
	var destinations []int64
	if err := tx.Model(&db.P14PerGameDelivery{}).Where("confirmed=0").Distinct("destination_server_id").Order("destination_server_id").Pluck("destination_server_id", &destinations).Error; err != nil {
		return err
	}
	if len(destinations) > 32 {
		return errors.New(constUtil.P14PerGameCorrupt)
	}
	for _, destination := range destinations {
		if err := ctx.Err(); err != nil {
			return err
		}
		var row db.P14PerGameDelivery
		if err := tx.Where("confirmed=0 AND destination_server_id=?", destination).Order("created_at,id").First(&row).Error; err != nil {
			return err
		}
		if row.PayloadHash != p6Digest(row.Payload) {
			return errors.New(constUtil.P14PerGameCorrupt)
		}
		var peer db.P14TrustedPeer
		err := tx.First(&peer, "server_id=? AND enabled=1", destination).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			continue
		}
		if err != nil {
			return err
		}
		if len(peer.SigningKey) < 32 || peer.Endpoint == "" {
			continue
		}
		cli := httpClient.HttpInner("p14-per-game-"+strconv.FormatInt(destination, 10)).SetTimeout(3*time.Second).AddHeader("X-HYJS-P14-Signature", P14SignPayload(peer.SigningKey, row.Payload))
		status, raw, callErr := cli.PostWithContext(ctx, peer.Endpoint+constUtil.P14PerGameImportPath, row.Payload)
		if err = tx.Model(&db.P14PerGameDelivery{}).Where("id=? AND confirmed=0", row.ID).UpdateColumn("attempt_count", gorm.Expr("attempt_count+1")).Error; err != nil {
			return err
		}
		if callErr != nil || status != http.StatusOK {
			continue
		}
		var reply struct {
			Code int64                          `json:"code"`
			Data model.P14PerGameImportResponse `json:"data"`
		}
		if json.Unmarshal(raw, &reply) != nil || reply.Code != 0 || !reply.Data.Confirmed || reply.Data.DeliveryID != row.ID {
			continue
		}
		if err = tx.Model(&db.P14PerGameDelivery{}).Where("id=? AND confirmed=0", row.ID).Updates(map[string]any{"confirmed": true, "confirmed_at": p14PromotionRankClock(ctx).UnixMilli()}).Error; err != nil {
			return err
		}
	}
	return nil
}
func (s *GameService) TickP14PerGame(ctx context.Context, now time.Time) error {
	if clock, ok := ctx.Value(constUtil.P14CrossClockContextKey{}).(func() time.Time); ok {
		now = clock()
	}
	if err := s.QueueP14PerGameSeals(ctx, now); err != nil {
		return err
	}
	return s.DeliverP14PerGame(ctx)
}

// p14PerGameReadySourceTX exposes only the two genuine ordered future inputs
// after all actual pair origins have sealed that game. It produces no game ID,
// seed, winner, HP carry, score or advancement before the prior KF certificate.
func (s *GameService) p14PerGameReadySourceTX(ctx context.Context, tx *gorm.DB, seriesID string, index int32, now time.Time) ([]*pb.BattlePlayer, []string, error) {
	var series db.P14FinalSeries
	if err := tx.WithContext(ctx).First(&series, "id=?", seriesID).Error; err != nil {
		return nil, nil, err
	}
	offer, manifest, err := s.p14PerGameOfferTX(ctx, tx, &series, index)
	if err != nil {
		return nil, nil, err
	}
	if now.UnixMilli() < offer.LockAt {
		return nil, nil, errors.New(constUtil.P14FinalSeriesPending)
	}
	found := map[string]*pb.BattlePlayer{}
	var hashes []string
	for _, origin := range p14PerGameOrigins(manifest) {
		var seal db.P14PerGameSeal
		err := tx.First(&seal, "series_id=? AND game_index=? AND server_id=?", series.ID, index, origin).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil, errors.New(constUtil.P14FinalSeriesPending)
		}
		if err != nil {
			return nil, nil, err
		}
		var body p14PerGameSealBody
		if seal.ContentHash != p6Digest(seal.Content) || p14PromotionRankDecode(seal.Content, &body) != nil || body.Offer.SourceHash != offer.SourceHash {
			return nil, nil, errors.New(constUtil.P14PerGameCorrupt)
		}
		if err = p14PerGameSealValidate(&body, &series, manifest, now); err != nil {
			return nil, nil, err
		}
		for position := range body.Members {
			member := &body.Members[position]
			var observed db.P14PerGameFormation
			if err = tx.First(&observed, "series_id=? AND game_index=? AND server_id=? AND player_id=?", series.ID, index, origin, member.PlayerID).Error; err != nil {
				return nil, nil, err
			}
			if observed.SourceHash != member.SourceHash {
				return nil, nil, errors.New(constUtil.P14PerGameCorrupt)
			}
			player := &pb.BattlePlayer{}
			if proto.Unmarshal(member.Battle, player) != nil {
				return nil, nil, errors.New(constUtil.P14PerGameCorrupt)
			}
			found[strconv.FormatInt(origin, 10)+"/"+strconv.FormatInt(member.PlayerID, 10)] = player
		}
		hashes = append(hashes, seal.ContentHash)
	}
	left := found[strconv.FormatInt(manifest.Pair.LeftServerID, 10)+"/"+strconv.FormatInt(manifest.Pair.LeftPlayerID, 10)]
	right := found[strconv.FormatInt(manifest.Pair.RightServerID, 10)+"/"+strconv.FormatInt(manifest.Pair.RightPlayerID, 10)]
	if left == nil || right == nil {
		return nil, nil, errors.New(constUtil.P14PerGameCorrupt)
	}
	return []*pb.BattlePlayer{left, right}, hashes, nil
}

// P14FinalFormationQueryCurrent displays a real next-game upload when one
// exists, including its locked window. With no such upload, the old first-game
// display remains a read-only fallback and is never sealed as a next-game input.
func (s *GameService) P14FinalFormationQueryCurrent(ctx context.Context, tx *gorm.DB, season *db.P14SeasonPolicy, server, player int64, now time.Time) (*pb.FormationSimpleInfo, error) {
	if tx.Migrator().HasTable(&db.P14PerGameFormation{}) {
		var rows []db.P14FinalSeries
		if err := tx.Where("season_index=? AND round_stage=?", season.SeasonIndex, constUtil.P14Final16Stage).Find(&rows).Error; err != nil {
			return nil, err
		}
		for position := range rows {
			series := &rows[position]
			manifest, err := p14PerGameReadSeries(series)
			if err != nil {
				return nil, err
			}
			if !p14PerGameMember(manifest, server, player) || now.UnixMilli() < manifest.Slots[0] {
				continue
			}
			var row db.P14PerGameFormation
			err = tx.Where("series_id=? AND server_id=? AND player_id=? AND captured_at<=?", series.ID, server, player, now.UnixMilli()).Order("game_index DESC,revision DESC").First(&row).Error
			if errors.Is(err, gorm.ErrRecordNotFound) {
				continue
			}
			if err != nil {
				return nil, err
			}
			var offer db.P14PerGameOffer
			if err = tx.First(&offer, "series_id=? AND game_index=?", series.ID, row.GameIndex).Error; err != nil {
				return nil, err
			}
			if p14PerGameReadOffer(&offer, series, manifest) != nil || p14PerGameReadFormation(&row, &offer, manifest) != nil {
				return nil, errors.New(constUtil.P14PerGameCorrupt)
			}
			frame := &pb.FormationSimpleInfo{}
			if proto.Unmarshal(row.Snapshot, frame) != nil {
				return nil, errors.New(constUtil.P14PerGameCorrupt)
			}
			return frame, nil
		}
	}
	return s.p14FinalFormationQuery(ctx, tx, season, server, player, now)
}
