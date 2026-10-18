package service

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"reflect"
	"sort"
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

// Metadata preserves the exact existing producer row; the separately carried
// binary fields restore its original source hash without mutating old rows.
type p14FinalSealMember struct {
	Kind      int32           `json:"kind"`
	PlayerID  int64           `json:"playerId"`
	Metadata  json.RawMessage `json:"metadata"`
	Formation []byte          `json:"formation"`
	Battle    []byte          `json:"battle"`
}

type p14FinalSealSnapshot struct {
	SeasonIndex int32                     `json:"seasonIndex"`
	RoundStage  int32                     `json:"roundStage"`
	ServerID    int64                     `json:"serverId"`
	Policy      db.P14FinalGamePolicy     `json:"policy"`
	RulesHash   string                    `json:"rulesHash"`
	DrawHash    string                    `json:"drawHash"`
	Offer       db.P14FinalFormationOffer `json:"offer"`
	SealedAt    int64                     `json:"sealedAt"`
	Members     []p14FinalSealMember      `json:"members"`
}

type p14FinalGameProvenance struct {
	SeasonIndex       int32                 `json:"seasonIndex"`
	RoundStage        int32                 `json:"roundStage"`
	GameIndex         int32                 `json:"gameIndex"`
	Pair              p14FinalDrawPair      `json:"pair"`
	DrawHash          string                `json:"drawHash"`
	QualificationHash string                `json:"qualificationHash"`
	Policy            db.P14FinalGamePolicy `json:"policy"`
	RulesHash         string                `json:"rulesHash"`
	// Ordered seal digests bind every actual originating realm, not only the pair.
	SealHashes []string           `json:"sealHashes"`
	Left       p14FinalSealMember `json:"left"`
	Right      p14FinalSealMember `json:"right"`
}

func p14FinalGamePolicyValid(policy *db.P14FinalGamePolicy, offer *db.P14FinalFormationOffer) bool {
	return policy != nil && offer != nil && policy.ID == 1 && policy.Enabled && policy.Source == constUtil.P14FinalGameSource && strings.TrimSpace(policy.Version) != "" && len(policy.Version) <= 128 && policy.PublishedAt > 0 && policy.PublishedAt <= offer.StageStart
}

func p14FinalSealDeliveryID(request model.P14FinalSealImportRequest) string {
	return p6Digest([]byte(fmt.Sprintf("p14/final-seal/%d/%d/%d/%s", request.ProtocolVersion, request.SourceServerID, request.DestinationServerID, p6Digest(request.Snapshot))))
}

func p14FinalDrawOrigins(draw *p14FinalDrawSnapshot) []int64 {
	seen := map[int64]bool{}
	var ids []int64
	for _, entry := range draw.Entrants {
		if !seen[entry.ServerID] {
			seen[entry.ServerID] = true
			ids = append(ids, entry.ServerID)
		}
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	return ids
}

// InitializeP14FinalGame migrates only the independent first-game domain. The
// policy remains off until operators publish its explicit local rule version.
func (s *GameService) InitializeP14FinalGame(ctx context.Context) error {
	tx := db.GetIns().WithContext(ctx)
	if err := tx.AutoMigrate(&db.P14FinalGamePolicy{}, &db.P14FinalSeal{}, &db.P14FinalSealDelivery{}, &db.P14FinalGame{}); err != nil {
		return err
	}
	return tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&db.P14FinalGamePolicy{ID: 1, Source: constUtil.P14FinalGameSource}).Error
}

func (s *GameService) p14FinalSealCaptureMember(ctx context.Context, tx *gorm.DB, season int32, server, player int64, now time.Time) (p14FinalSealMember, error) {
	member := p14FinalSealMember{PlayerID: player}
	var final db.P14FinalFormation
	err := tx.First(&final, "season_index=? AND round_stage=? AND server_id=? AND player_id=?", season, constUtil.P14Final16Stage, server, player).Error
	if err == nil {
		member.Kind = constUtil.P14FinalSealFinal
		member.Metadata, _ = json.Marshal(final)
		member.Formation = final.Snapshot
		member.Battle = final.Battle
		return member, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return member, err
	}
	var promotion db.P14PromotionFormation
	err = tx.First(&promotion, "season_index=? AND server_id=? AND player_id=?", season, server, player).Error
	if err == nil {
		member.Kind = constUtil.P14FinalSealPromotion
		member.Metadata, _ = json.Marshal(promotion)
		member.Formation = promotion.Snapshot
		member.Battle = promotion.Battle
		return member, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return member, err
	}
	roster, _, err := s.p14PromotionRankQualification(ctx, tx, season, server, now)
	if err != nil {
		return member, err
	}
	for _, row := range roster {
		if row.PlayerID == player && row.ServerID == server {
			member.Kind = constUtil.P14FinalSealRoster
			member.Metadata, _ = json.Marshal(row)
			member.Formation = row.Snapshot
			member.Battle = row.Battle
			return member, nil
		}
	}
	return member, errors.New(constUtil.P14FinalGameCorrupt)
}

// p14FinalSealMemberRead validates original source rows without recapturing a
// mutable profile or changing its engine, first order, HP, formation or GUIDs.
func (s *GameService) p14FinalSealMemberRead(ctx context.Context, tx *gorm.DB, body *p14FinalSealSnapshot, member p14FinalSealMember, now time.Time) (*pb.BattlePlayer, error) {
	battle := &pb.BattlePlayer{}
	frame := &pb.FormationSimpleInfo{}
	if member.PlayerID <= 0 || proto.Unmarshal(member.Battle, battle) != nil || proto.Unmarshal(member.Formation, frame) != nil || battle.GetPlayerId() != member.PlayerID || int64(battle.GetServerId()) != body.ServerID || len(battle.Formation) == 0 {
		return nil, errors.New(constUtil.P14FinalGameCorrupt)
	}
	switch member.Kind {
	case constUtil.P14FinalSealFinal:
		var row db.P14FinalFormation
		if p14PromotionRankDecode(member.Metadata, &row) != nil {
			return nil, errors.New(constUtil.P14FinalGameCorrupt)
		}
		row.Snapshot = member.Formation
		row.Battle = member.Battle
		if row.PlayerID != member.PlayerID || row.ServerID != body.ServerID || row.SeasonIndex != body.SeasonIndex {
			return nil, errors.New(constUtil.P14FinalGameCorrupt)
		}
		if _, err := p14FinalFormationRead(&row, &body.Offer); err != nil {
			return nil, err
		}
		// A source already observed at a higher revision cannot be hidden by a seal.
		var known db.P14FinalFormation
		err := tx.First(&known, "season_index=? AND round_stage=? AND server_id=? AND player_id=?", row.SeasonIndex, row.RoundStage, row.ServerID, row.PlayerID).Error
		if err == nil && (known.Revision > row.Revision || known.Revision == row.Revision && known.SourceHash != row.SourceHash) {
			return nil, errors.New(constUtil.P14FinalGameCorrupt)
		}
		if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, err
		}
	case constUtil.P14FinalSealPromotion:
		var row db.P14PromotionFormation
		if p14PromotionRankDecode(member.Metadata, &row) != nil {
			return nil, errors.New(constUtil.P14FinalGameCorrupt)
		}
		row.Snapshot = member.Formation
		row.Battle = member.Battle
		if row.PlayerID != member.PlayerID || row.ServerID != body.ServerID || row.SeasonIndex != body.SeasonIndex || row.CapturedAt >= body.Offer.StageStart {
			return nil, errors.New(constUtil.P14FinalGameCorrupt)
		}
		if _, err := p14PromotionFormationRead(&row); err != nil {
			return nil, err
		}
		_, q, err := s.p14PromotionRankQualification(ctx, tx, body.SeasonIndex, body.ServerID, now)
		if err != nil {
			return nil, err
		}
		if row.QualificationHash != q.SourceHash {
			return nil, errors.New(constUtil.P14FinalGameCorrupt)
		}
		var season db.P14SeasonPolicy
		if err := tx.First(&season, 1).Error; err != nil {
			return nil, err
		}
		stage, _, err := s.p14SeasonStage(tx, &season, time.UnixMilli(row.CapturedAt))
		if err != nil || season.SeasonIndex != body.SeasonIndex || stage != constUtil.P14PromotionStage {
			return nil, errors.New(constUtil.P14FinalGameCorrupt)
		}
		var known db.P14PromotionFormation
		err = tx.First(&known, "season_index=? AND server_id=? AND player_id=?", row.SeasonIndex, row.ServerID, row.PlayerID).Error
		if err == nil && (known.Revision > row.Revision || known.Revision == row.Revision && known.SourceHash != row.SourceHash) {
			return nil, errors.New(constUtil.P14FinalGameCorrupt)
		}
		if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, err
		}
	case constUtil.P14FinalSealRoster:
		var row db.P14AuditionRoster
		if p14PromotionRankDecode(member.Metadata, &row) != nil || !bytes.Equal(row.Snapshot, member.Formation) || !bytes.Equal(row.Battle, member.Battle) {
			return nil, errors.New(constUtil.P14FinalGameCorrupt)
		}
		roster, _, err := s.p14PromotionRankQualification(ctx, tx, body.SeasonIndex, body.ServerID, now)
		if err != nil {
			return nil, err
		}
		matched := false
		for _, original := range roster {
			if original.PlayerID == member.PlayerID && reflect.DeepEqual(original, row) {
				matched = true
				break
			}
		}
		if !matched {
			return nil, errors.New(constUtil.P14FinalGameCorrupt)
		}
	default:
		return nil, errors.New(constUtil.P14FinalGameCorrupt)
	}
	// A fallback cannot conceal any real purpose3 capture that has already arrived.
	if member.Kind != constUtil.P14FinalSealFinal {
		var count int64
		if err := tx.Model(&db.P14FinalFormation{}).Where("season_index=? AND round_stage=? AND server_id=? AND player_id=?", body.SeasonIndex, body.RoundStage, body.ServerID, member.PlayerID).Count(&count).Error; err != nil {
			return nil, err
		}
		if count != 0 {
			return nil, errors.New(constUtil.P14FinalGameCorrupt)
		}
	}
	return battle, nil
}

func p14FinalSealHeaderValidate(body *p14FinalSealSnapshot, draw *p14FinalDrawSnapshot, drawHash string, now time.Time) error {
	if body == nil || draw == nil || body.SeasonIndex != draw.SeasonIndex || body.RoundStage != constUtil.P14Final16Stage || body.ServerID <= 0 || body.ServerID > 32 || body.DrawHash != drawHash || body.RulesHash != p6Digest([]byte(constUtil.P14FinalGameRules)) || !p14FinalGamePolicyValid(&body.Policy, &body.Offer) || body.SealedAt < body.Offer.LockAt || body.SealedAt > now.UnixMilli() || body.SealedAt >= body.Offer.FightEnd || body.Offer.SeasonIndex != body.SeasonIndex || body.Offer.QualificationHash != draw.QualificationHash {
		return errors.New(constUtil.P14FinalGameCorrupt)
	}
	if err := p14FinalFormationValidateOffer(&body.Offer); err != nil {
		return err
	}
	var ids []int64
	for _, entry := range draw.Entrants {
		if entry.ServerID == body.ServerID {
			ids = append(ids, entry.PlayerID)
		}
	}
	if len(ids) == 0 || len(ids) != len(body.Members) {
		return errors.New(constUtil.P14FinalGameCorrupt)
	}
	for i, member := range body.Members {
		if member.PlayerID != ids[i] {
			return errors.New(constUtil.P14FinalGameCorrupt)
		}
	}
	return nil
}

func (s *GameService) p14FinalSealValidate(ctx context.Context, tx *gorm.DB, body *p14FinalSealSnapshot, draw *p14FinalDrawSnapshot, drawHash string, now time.Time) error {
	if err := p14FinalSealHeaderValidate(body, draw, drawHash, now); err != nil {
		return err
	}
	for _, member := range body.Members {
		if _, err := s.p14FinalSealMemberRead(ctx, tx, body, member, now); err != nil {
			return err
		}
	}
	return nil
}

// QueueP14FinalSeals runs under the same season row lock as original81030.
// Every source freezes its last accepted local captures only after the real
// five-minute boundary. Purpose3 and seal transport may arrive in either order.
func (s *GameService) QueueP14FinalSeals(ctx context.Context, now time.Time) error {
	return db.GetIns().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := ServerGameClockGuardTransaction(tx); err != nil {
			return err
		}

		var season db.P14SeasonPolicy
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&season, 1).Error; err != nil {
			return err
		}
		var gate db.P14FinalGamePolicy
		if err := tx.First(&gate, 1).Error; err != nil {
			return err
		}
		if !gate.Enabled || !season.Enabled {
			return nil
		}
		stage, _, err := s.p14SeasonStage(tx, &season, now)
		if err != nil {
			return err
		}
		if stage != constUtil.P14Final16BeforeStage && stage != constUtil.P14Final16Stage {
			return nil
		}
		draw, err := s.p14FinalDrawTX(ctx, tx, &season, now)
		if err != nil {
			return err
		}
		var ownIDs []int64
		for _, entry := range draw.Entrants {
			if entry.ServerID == s.OriginalServerID() {
				ownIDs = append(ownIDs, entry.PlayerID)
			}
		}
		if len(ownIDs) == 0 {
			return nil
		}
		offer, err := s.p14FinalFormationOfferTX(ctx, tx, &season, s.OriginalServerID(), ownIDs[0], now)
		if err != nil {
			return err
		}
		if now.UnixMilli() < offer.LockAt {
			return nil
		}
		var saved db.P14FinalSeal
		err = tx.First(&saved, "season_index=? AND round_stage=? AND server_id=?", season.SeasonIndex, offer.RoundStage, s.OriginalServerID()).Error
		if err == nil {
			return nil
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		var drawRow db.P14FinalDraw
		if err = tx.First(&drawRow, "season_index=?", season.SeasonIndex).Error; err != nil {
			return err
		}
		body := p14FinalSealSnapshot{SeasonIndex: season.SeasonIndex, RoundStage: offer.RoundStage, ServerID: s.OriginalServerID(), Policy: gate, RulesHash: p6Digest([]byte(constUtil.P14FinalGameRules)), DrawHash: drawRow.ContentHash, Offer: *offer, SealedAt: now.UnixMilli()}
		for _, id := range ownIDs {
			member, err := s.p14FinalSealCaptureMember(ctx, tx, season.SeasonIndex, s.OriginalServerID(), id, now)
			if err != nil {
				return err
			}
			body.Members = append(body.Members, member)
		}
		if err = s.p14FinalSealValidate(ctx, tx, &body, draw, drawRow.ContentHash, now); err != nil {
			return err
		}
		raw, err := json.Marshal(body)
		if err != nil {
			return err
		}
		row := db.P14FinalSeal{SeasonIndex: body.SeasonIndex, RoundStage: body.RoundStage, ServerID: body.ServerID, Content: raw, ContentHash: p6Digest(raw), ReceivedAt: now.UnixMilli()}
		if err = tx.Create(&row).Error; err != nil {
			return err
		}
		// Deliver to every original qualification peer, including realms without a
		// finalist, so their spectator list does not require a fabricated local seal.
		var peers []db.P14TrustedPeer
		if err = tx.Where("enabled=1 AND server_id<>?", s.OriginalServerID()).Find(&peers).Error; err != nil {
			return err
		}
		for _, peer := range peers {
			request := model.P14FinalSealImportRequest{ProtocolVersion: constUtil.P14FinalGameProtocol, SourceServerID: body.ServerID, DestinationServerID: peer.ServerID, Snapshot: raw}
			request.DeliveryID = p14FinalSealDeliveryID(request)
			payload, err := json.Marshal(request)
			if err != nil {
				return err
			}
			if err = tx.Create(&db.P14FinalSealDelivery{ID: request.DeliveryID, DestinationServerID: peer.ServerID, Payload: payload, PayloadHash: p6Digest(payload), CreatedAt: now.UnixMilli()}).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

// ImportP14FinalSeal authenticates the exact raw envelope and retains one
// immutable source seal. The origin's capture time remains separate from arrival.
func (s *GameService) ImportP14FinalSeal(ctx context.Context, request model.P14FinalSealImportRequest, raw []byte, signature string) (*model.P14FinalSealImportResponse, error) {
	now := p14PromotionRankClock(ctx)
	var decoded model.P14FinalSealImportRequest
	if p14PromotionRankDecode(raw, &decoded) != nil {
		return nil, errors.New(constUtil.P14FinalGameCorrupt)
	}
	a, _ := json.Marshal(decoded)
	b, _ := json.Marshal(request)
	if !bytes.Equal(a, b) || request.ProtocolVersion != constUtil.P14FinalGameProtocol || request.SourceServerID == s.OriginalServerID() || request.SourceServerID <= 0 || request.SourceServerID > 32 || request.DestinationServerID != s.OriginalServerID() || request.DeliveryID != p14FinalSealDeliveryID(request) {
		return nil, errors.New(constUtil.P14FinalGameCorrupt)
	}
	var peer db.P14TrustedPeer
	if err := db.GetIns().WithContext(ctx).First(&peer, "server_id=? AND enabled=1", request.SourceServerID).Error; err != nil {
		return nil, err
	}
	mac := hmac.New(sha256.New, peer.SigningKey)
	mac.Write(raw)
	given, err := hex.DecodeString(signature)
	if len(peer.SigningKey) < 32 || err != nil || !hmac.Equal(mac.Sum(nil), given) {
		return nil, errors.New(constUtil.P14FinalGameCorrupt)
	}
	var incoming p14FinalSealSnapshot
	if p14PromotionRankDecode(request.Snapshot, &incoming) != nil || incoming.ServerID != request.SourceServerID {
		return nil, errors.New(constUtil.P14FinalGameCorrupt)
	}
	err = db.GetIns().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := ServerGameClockGuardTransaction(tx); err != nil {
			return err
		}

		var season db.P14SeasonPolicy
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&season, 1).Error; err != nil {
			return err
		}
		draw, err := s.p14FinalDrawTX(ctx, tx, &season, now)
		if err != nil {
			return err
		}
		var drawRow db.P14FinalDraw
		if err = tx.First(&drawRow, "season_index=?", incoming.SeasonIndex).Error; err != nil {
			return err
		}
		if err = s.p14FinalSealValidate(ctx, tx, &incoming, draw, drawRow.ContentHash, now); err != nil {
			return err
		}
		localOffer, err := s.p14FinalFormationOfferTX(ctx, tx, &season, incoming.ServerID, incoming.Members[0].PlayerID, now)
		if err != nil {
			return err
		}
		if localOffer.SourceHash != incoming.Offer.SourceHash {
			return errors.New(constUtil.P14FinalGameCorrupt)
		}
		var saved db.P14FinalSeal
		err = tx.First(&saved, "season_index=? AND round_stage=? AND server_id=?", incoming.SeasonIndex, incoming.RoundStage, incoming.ServerID).Error
		if err == nil {
			if saved.ContentHash != p6Digest(request.Snapshot) || !bytes.Equal(saved.Content, request.Snapshot) {
				return errors.New(constUtil.P14FinalGameCorrupt)
			}
			return nil
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		var gate db.P14FinalGamePolicy
		if err = tx.First(&gate, 1).Error; err != nil {
			return err
		}
		if !season.Enabled || !gate.Enabled {
			return errors.New(constUtil.P14FinalGamePending)
		}
		if gate != incoming.Policy {
			return errors.New(constUtil.P14FinalGameCorrupt)
		}
		return tx.Create(&db.P14FinalSeal{SeasonIndex: incoming.SeasonIndex, RoundStage: incoming.RoundStage, ServerID: incoming.ServerID, Content: request.Snapshot, ContentHash: p6Digest(request.Snapshot), ReceivedAt: now.UnixMilli()}).Error
	})
	if err != nil {
		return nil, err
	}
	return &model.P14FinalSealImportResponse{DeliveryID: request.DeliveryID, Confirmed: true}, nil
}

// DeliverP14FinalSeals reuses the existing P14 worker/context and internal HTTP
// client. One oldest unconfirmed seal per peer keeps offline retry work bounded.
func (s *GameService) DeliverP14FinalSeals(ctx context.Context) error {
	tx := db.GetIns().WithContext(ctx)
	var gate db.P14FinalGamePolicy
	if err := tx.First(&gate, 1).Error; err != nil {
		return err
	}
	if !gate.Enabled {
		return nil
	}
	var ids []int64
	if err := tx.Model(&db.P14FinalSealDelivery{}).Where("confirmed=0").Distinct("destination_server_id").Order("destination_server_id").Pluck("destination_server_id", &ids).Error; err != nil {
		return err
	}
	if len(ids) > 32 {
		return errors.New(constUtil.P14FinalGameCorrupt)
	}
	for _, id := range ids {
		if err := ctx.Err(); err != nil {
			return err
		}
		var row db.P14FinalSealDelivery
		if err := tx.Where("confirmed=0 AND destination_server_id=?", id).Order("created_at,id").First(&row).Error; err != nil {
			return err
		}
		if row.PayloadHash != p6Digest(row.Payload) {
			return errors.New(constUtil.P14FinalGameCorrupt)
		}
		var peer db.P14TrustedPeer
		err := tx.First(&peer, "server_id=? AND enabled=1", id).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			continue
		}
		if err != nil {
			return err
		}
		if len(peer.SigningKey) < 32 || peer.Endpoint == "" {
			continue
		}
		cli := httpClient.HttpInner("p14-final-seal-"+strconv.FormatInt(id, 10)).SetTimeout(3*time.Second).AddHeader("X-HYJS-P14-Signature", P14SignPayload(peer.SigningKey, row.Payload))
		status, raw, callErr := cli.PostWithContext(ctx, peer.Endpoint+constUtil.P14FinalGameImportPath, row.Payload)
		if err = tx.Model(&db.P14FinalSealDelivery{}).Where("id=? AND confirmed=0", row.ID).UpdateColumn("attempt_count", gorm.Expr("attempt_count+1")).Error; err != nil {
			return err
		}
		if callErr != nil || status != http.StatusOK {
			continue
		}
		var reply struct {
			Code int64                            `json:"code"`
			Data model.P14FinalSealImportResponse `json:"data"`
		}
		if json.Unmarshal(raw, &reply) != nil || reply.Code != 0 || !reply.Data.Confirmed || reply.Data.DeliveryID != row.ID {
			continue
		}
		if err = tx.Model(&db.P14FinalSealDelivery{}).Where("id=? AND confirmed=0", row.ID).Updates(map[string]any{"confirmed": true, "confirmed_at": p14PromotionRankClock(ctx).UnixMilli()}).Error; err != nil {
			return err
		}
	}
	return nil
}

func p14FinalGameBuild(provenance *p14FinalGameProvenance) (*db.P14FinalGame, error) {
	if provenance == nil || provenance.SeasonIndex <= 0 || provenance.Policy.Source != constUtil.P14FinalGameSource || provenance.RulesHash != p6Digest([]byte(constUtil.P14FinalGameRules)) || len(provenance.DrawHash) != 64 || len(provenance.QualificationHash) != 64 || provenance.GameIndex != 1 || provenance.RoundStage != constUtil.P14Final16Stage || len(provenance.SealHashes) == 0 || provenance.Left.PlayerID != provenance.Pair.LeftPlayerID || provenance.Right.PlayerID != provenance.Pair.RightPlayerID {
		return nil, errors.New(constUtil.P14FinalGameCorrupt)
	}
	left, right := &pb.BattlePlayer{}, &pb.BattlePlayer{}
	if proto.Unmarshal(provenance.Left.Battle, left) != nil || proto.Unmarshal(provenance.Right.Battle, right) != nil || len(left.Formation) == 0 || len(right.Formation) == 0 || provenance.Pair.LeftServerID == provenance.Pair.RightServerID && provenance.Pair.LeftPlayerID == provenance.Pair.RightPlayerID || left.GetPlayerId() != provenance.Pair.LeftPlayerID || right.GetPlayerId() != provenance.Pair.RightPlayerID || int64(left.GetServerId()) != provenance.Pair.LeftServerID || int64(right.GetServerId()) != provenance.Pair.RightServerID {
		return nil, errors.New(constUtil.P14FinalGameCorrupt)
	}
	source, err := json.Marshal(provenance)
	if err != nil {
		return nil, err
	}
	id := p6Digest(append([]byte("p14/final-game1/"), source...))
	digest := sha256.Sum256([]byte(id))
	seed := int64(binary.BigEndian.Uint64(digest[:8]) & 0x7fffffffffffffff)
	if seed == 0 {
		seed = 1
	}
	// This is the original ordered two-player replay input only. Omitted outcome
	// fields deliberately preserve no played-game fact and no synthesized winner.
	replay := &pb.KFReplayData{WarId: proto.String(id), RandSeed: proto.Int64(seed), PlayerList: []*pb.BattlePlayer{left, right}, MaxRoundNum: proto.Int32(constUtil.P14FinalGameMaxRound)}
	encoded, err := proto.Marshal(replay)
	if err != nil {
		return nil, err
	}
	return &db.P14FinalGame{ID: id, SeasonIndex: provenance.SeasonIndex, RoundStage: provenance.RoundStage, GroupID: provenance.Pair.Group, PairIndex: provenance.Pair.Index, GameIndex: 1, Replay: encoded, ReplayHash: p6Digest(encoded), Provenance: source, ProvenanceHash: p6Digest(source)}, nil
}

// FreezeP14FinalGameOne creates only a first-game input after every actual
// source seal exists. Later games and results require independent KF evidence.
func (s *GameService) FreezeP14FinalGameOne(ctx context.Context, group string, pairIndex int32, now time.Time) (*db.P14FinalGame, error) {
	var result *db.P14FinalGame
	err := db.GetIns().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := ServerGameClockGuardTransaction(tx); err != nil {
			return err
		}

		var season db.P14SeasonPolicy
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&season, 1).Error; err != nil {
			return err
		}
		// Existing games are returned unchanged without relabeling their old engine.
		var saved db.P14FinalGame
		err := tx.First(&saved, "season_index=? AND round_stage=? AND group_id=? AND pair_index=? AND game_index=1", season.SeasonIndex, constUtil.P14Final16Stage, group, pairIndex).Error
		if err == nil {
			if saved.ReplayHash != p6Digest(saved.Replay) || saved.ProvenanceHash != p6Digest(saved.Provenance) {
				return errors.New(constUtil.P14FinalGameCorrupt)
			}
			result = &saved
			return nil
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		var gate db.P14FinalGamePolicy
		if err = tx.First(&gate, 1).Error; err != nil {
			return err
		}
		if !gate.Enabled || !season.Enabled {
			return errors.New(constUtil.P14FinalGamePending)
		}
		stage, _, err := s.p14SeasonStage(tx, &season, now)
		if err != nil {
			return err
		}
		if stage != constUtil.P14Final16Stage {
			return errors.New(constUtil.P14FinalGamePending)
		}
		draw, err := s.p14FinalDrawTX(ctx, tx, &season, now)
		if err != nil {
			return err
		}
		var drawRow db.P14FinalDraw
		if err = tx.First(&drawRow, "season_index=?", season.SeasonIndex).Error; err != nil {
			return err
		}
		var pair *p14FinalDrawPair
		for _, candidate := range draw.Pairs {
			if candidate.Group == group && candidate.Index == pairIndex {
				copy := candidate
				pair = &copy
				break
			}
		}
		if pair == nil {
			return errors.New(constUtil.P14FinalGamePending)
		}
		provenance := p14FinalGameProvenance{SeasonIndex: season.SeasonIndex, RoundStage: constUtil.P14Final16Stage, GameIndex: 1, Pair: *pair, DrawHash: drawRow.ContentHash, QualificationHash: draw.QualificationHash, Policy: gate, RulesHash: p6Digest([]byte(constUtil.P14FinalGameRules))}
		for _, origin := range p14FinalDrawOrigins(draw) {
			var row db.P14FinalSeal
			err = tx.First(&row, "season_index=? AND round_stage=? AND server_id=?", season.SeasonIndex, constUtil.P14Final16Stage, origin).Error
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return errors.New(constUtil.P14FinalGamePending)
			}
			if err != nil {
				return err
			}
			var seal p14FinalSealSnapshot
			if row.ContentHash != p6Digest(row.Content) || p14PromotionRankDecode(row.Content, &seal) != nil || seal.Policy != gate {
				return errors.New(constUtil.P14FinalGameCorrupt)
			}
			if err = s.p14FinalSealValidate(ctx, tx, &seal, draw, drawRow.ContentHash, now); err != nil {
				return err
			}
			provenance.SealHashes = append(provenance.SealHashes, row.ContentHash)
			for _, member := range seal.Members {
				if origin == pair.LeftServerID && member.PlayerID == pair.LeftPlayerID {
					provenance.Left = member
				}
				if origin == pair.RightServerID && member.PlayerID == pair.RightPlayerID {
					provenance.Right = member
				}
			}
		}
		game, err := p14FinalGameBuild(&provenance)
		if err != nil {
			return err
		}
		game.CreatedAt = now.UnixMilli()
		if err = tx.Create(game).Error; err != nil {
			return err
		}
		result = game
		return nil
	})
	return result, err
}

// TickP14FinalGame is invoked by the existing P14 worker, preserving its stop
// lifecycle. Pending genuine qualification or source delivery is retried later.
func (s *GameService) TickP14FinalGame(ctx context.Context, now time.Time) error {
	// Only the existing explicit internal test context can override this tick.
	if clock, ok := ctx.Value(constUtil.P14CrossClockContextKey{}).(func() time.Time); ok {
		now = clock()
	}
	if err := s.QueueP14FinalSeals(ctx, now); err != nil {
		if err.Error() != constUtil.P14ResultsUnverified && err.Error() != constUtil.P14FinalGamePending {
			return err
		}
	}
	if err := s.DeliverP14FinalSeals(ctx); err != nil {
		return err
	}
	tx := db.GetIns().WithContext(ctx)
	var gate db.P14FinalGamePolicy
	if err := tx.First(&gate, 1).Error; err != nil {
		return err
	}
	if !gate.Enabled {
		return nil
	}
	var season db.P14SeasonPolicy
	if err := tx.First(&season, 1).Error; err != nil {
		return err
	}
	if !season.Enabled {
		return nil
	}
	stage, _, err := s.p14SeasonStage(tx, &season, now)
	if err != nil {
		return err
	}
	if stage != constUtil.P14Final16Stage {
		return nil
	}
	var drawRow db.P14FinalDraw
	err = tx.First(&drawRow, "season_index=?", season.SeasonIndex).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	var draw p14FinalDrawSnapshot
	if drawRow.ContentHash != p6Digest(drawRow.Content) || p14PromotionRankDecode(drawRow.Content, &draw) != nil {
		return errors.New(constUtil.P14FinalGameCorrupt)
	}
	for _, pair := range draw.Pairs {
		if _, err = s.FreezeP14FinalGameOne(ctx, pair.Group, pair.Index, now); err != nil && err.Error() != constUtil.P14FinalGamePending && err.Error() != constUtil.P14ResultsUnverified {
			return err
		}
	}
	return nil
}
