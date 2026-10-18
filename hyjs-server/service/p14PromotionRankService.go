package service

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
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

// Scores contain only this origin's accepted active challenges. Being chosen
// as another player's opponent never changes this ledger or creates a win.
type p14PromotionRankScore struct {
	PlayerID          int64  `json:"playerId"`
	Score             int64  `json:"score"`
	AcceptedMatches   int64  `json:"acceptedMatches"`
	LedgerHeadHash    string `json:"ledgerHeadHash"`
	LastMatchAt       int64  `json:"lastMatchAt"`
	ScopeProof        string `json:"scopeProof"`
	EngineVersion     string `json:"engineVersion,omitempty"`
	ClientVersion     string `json:"clientVersion,omitempty"`
	ConfigurationHash string `json:"configurationHash,omitempty"`
}
type p14PromotionRankSnapshot struct {
	Offer             db.P14PromotionRankOffer `json:"offer"`
	QualificationHash string                   `json:"qualificationHash"`
	Revision          int64                    `json:"revision"`
	PreviousHash      string                   `json:"previousHash"`
	ObservedAt        int64                    `json:"observedAt"`
	Closed            bool                     `json:"closed"`
	Scores            []p14PromotionRankScore  `json:"scores"`
}

func p14PromotionRankClock(ctx context.Context) time.Time {
	if clock, ok := ctx.Value(constUtil.P14CrossClockContextKey{}).(func() time.Time); ok {
		return clock()
	}
	return time.Now()
}
func p14PromotionRankDecode(raw []byte, target any) error {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return errors.New(constUtil.P14PromotionRankCorrupt)
	}
	var tail any
	if decoder.Decode(&tail) != io.EOF {
		return errors.New(constUtil.P14PromotionRankCorrupt)
	}
	return nil
}
func p14PromotionRankOfferHash(offer *db.P14PromotionRankOffer) string {
	copy := *offer
	copy.SourceHash = ""
	raw, _ := json.Marshal(copy)
	return p6Digest(raw)
}
func p14PromotionRankValidateOffer(offer *db.P14PromotionRankOffer, now time.Time) ([]int64, error) {
	var peers []int64
	if offer == nil || offer.SeasonIndex <= 0 || offer.ServerID <= 0 || strings.TrimSpace(offer.Version) == "" || len(offer.Version) > 128 || offer.Source != constUtil.P14PromotionRankSource || offer.RulesJSON != constUtil.P14PromotionRankRules || offer.RulesHash != p6Digest([]byte(offer.RulesJSON)) || offer.PublishedAt <= 0 || offer.PublishedAt > offer.StageStart || offer.PublishedAt > offer.FrozenAt || offer.FrozenAt > now.UnixMilli() || offer.StageEnd <= offer.StageStart || offer.StageEnd-offer.StageStart > constUtil.P14PromotionMaxStageMillis || offer.SourceHash != p14PromotionRankOfferHash(offer) || p14PromotionRankDecode([]byte(offer.PeerServersJSON), &peers) != nil || p6Digest([]byte(offer.PeerServersJSON)) != offer.CohortHash || len(peers) < 2 || len(peers) > constUtil.P14PromotionRankPeerLimit {
		return nil, errors.New(constUtil.P14PromotionRankCorrupt)
	}
	own := false
	for i, id := range peers {
		if id <= 0 || id > math.MaxInt32 || i > 0 && id <= peers[i-1] {
			return nil, errors.New(constUtil.P14PromotionRankCorrupt)
		}
		own = own || id == offer.ServerID
	}
	if !own {
		return nil, errors.New(constUtil.P14PromotionRankCorrupt)
	}
	return peers, nil
}

// InitializeP14PromotionRank creates only a closed operator policy and empty
// durable version/outbox tables. It cannot create a score or qualifier.
func (s *GameService) InitializeP14PromotionRank(ctx context.Context) error {
	tx := db.GetIns().WithContext(ctx)
	if err := tx.AutoMigrate(&db.P14PromotionRankPolicy{}, &db.P14PromotionRankOffer{}, &db.P14PromotionRankVersion{}, &db.P14PromotionRankDelivery{}); err != nil {
		return err
	}
	return tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&db.P14PromotionRankPolicy{ID: 1, Source: constUtil.P14PromotionRankSource}).Error
}
func (s *GameService) p14PromotionRankOfferTX(tx *gorm.DB, policy *db.P14PromotionRankPolicy, season int32, now time.Time) (*db.P14PromotionRankOffer, error) {
	var offer db.P14PromotionRankOffer
	err := tx.First(&offer, "season_index=? AND server_id=?", season, s.OriginalServerID()).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		var ids []int64
		if err = tx.Model(&db.P14TrustedPeer{}).Where("enabled=1 AND server_id<>?", s.OriginalServerID()).Order("server_id").Pluck("server_id", &ids).Error; err != nil {
			return nil, err
		}
		ids = append(ids, s.OriginalServerID())
		sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
		raw, _ := json.Marshal(ids)
		slots, err := s.p14SeasonCalendar(tx, season)
		if err != nil {
			return nil, err
		}
		var start, end int64
		for _, slot := range slots {
			if slot.Stage == constUtil.P14PromotionStage {
				start, end = slot.StartAt, slot.EndAt
			}
		}
		offer = db.P14PromotionRankOffer{SeasonIndex: season, ServerID: s.OriginalServerID(), Version: policy.Version, Source: policy.Source, RulesJSON: constUtil.P14PromotionRankRules, RulesHash: p6Digest([]byte(constUtil.P14PromotionRankRules)), PeerServersJSON: string(raw), CohortHash: p6Digest(raw), StageStart: start, StageEnd: end, PublishedAt: policy.PublishedAt, FrozenAt: now.UnixMilli()}
		offer.SourceHash = p14PromotionRankOfferHash(&offer)
		if _, err = p14PromotionRankValidateOffer(&offer, now); err != nil {
			return nil, err
		}
		if err = tx.Create(&offer).Error; err != nil {
			return nil, err
		}
	} else if err != nil {
		return nil, err
	}
	_, err = p14PromotionRankValidateOffer(&offer, now)
	return &offer, err
}
func (s *GameService) p14PromotionRankQualification(ctx context.Context, tx *gorm.DB, season int32, server int64, now time.Time) ([]db.P14AuditionRoster, *db.P14AuditionQualification, error) {
	if server == s.OriginalServerID() {
		q, err := s.p14AuditionQualificationRead(ctx, tx, season, server)
		if err != nil {
			return nil, nil, err
		}
		_, roster, err := p14AuditionRead(tx, season, server)
		return roster, q, err
	}
	var saved db.P14ForeignQualification
	if err := tx.First(&saved, "season_index=? AND server_id=?", season, server).Error; err != nil {
		return nil, nil, err
	}
	var request model.P14QualificationImportRequest
	if saved.PayloadHash != p6Digest(saved.Payload) || p14PromotionRankDecode(saved.Payload, &request) != nil || request.SourceServerID != server || request.DestinationServerID != s.OriginalServerID() || request.SeasonIndex != season || request.DeliveryID != saved.DeliveryID {
		return nil, nil, errors.New(constUtil.P14PromotionRankCorrupt)
	}
	roster, boundary, q, err := p14QualificationProof(request, now)
	if err != nil {
		return nil, nil, err
	}
	if boundary.SourceHash != saved.BoundaryHash || q.SourceHash != saved.QualificationHash {
		return nil, nil, errors.New(constUtil.P14PromotionRankCorrupt)
	}
	return roster, q, nil
}
func p14PromotionRankInitialHead(qHash string, id int64) string {
	return p6Digest([]byte(qHash + "/promotion-score/initial/" + strconv.FormatInt(id, 10)))
}
func p14PromotionRankValidateScores(snapshot *p14PromotionRankSnapshot, q *db.P14AuditionQualification) error {
	var ids []int64
	if snapshot == nil || q == nil || snapshot.QualificationHash != q.SourceHash || snapshot.Offer.SeasonIndex != q.SeasonIndex || snapshot.Offer.ServerID != q.ServerID || snapshot.ObservedAt < q.QualifiedAt || q.QualifiedAt >= snapshot.Offer.StageEnd || snapshot.Revision < 1 || snapshot.Closed != (snapshot.ObservedAt >= snapshot.Offer.StageEnd) || p14PromotionRankDecode(q.Players, &ids) != nil || len(ids) == 0 || len(ids) != len(snapshot.Scores) {
		return errors.New(constUtil.P14PromotionRankCorrupt)
	}
	for i, entry := range snapshot.Scores {
		if entry.PlayerID != ids[i] || i > 0 && entry.PlayerID <= ids[i-1] || entry.AcceptedMatches < 0 || entry.AcceptedMatches > constUtil.P14PromotionRankMaxAccepted || entry.Score < 1 || entry.Score > 1+constUtil.P14PromotionRankWinDelta*entry.AcceptedMatches || (entry.Score-1)%constUtil.P14PromotionRankWinDelta != 0 || len(entry.LedgerHeadHash) != 64 || entry.LastMatchAt > snapshot.ObservedAt {
			return errors.New(constUtil.P14PromotionRankCorrupt)
		}
		if entry.AcceptedMatches == 0 {
			if entry.Score != 1 || entry.LastMatchAt != 0 || entry.ScopeProof != "" || entry.EngineVersion != "" || entry.ClientVersion != "" || entry.ConfigurationHash != "" || entry.LedgerHeadHash != p14PromotionRankInitialHead(q.SourceHash, entry.PlayerID) {
				return errors.New(constUtil.P14PromotionRankCorrupt)
			}
		} else if entry.LastMatchAt < snapshot.Offer.StageStart || entry.LastMatchAt >= snapshot.Offer.StageEnd || !p6KnownCrossStarterSettlementProof(&model.FrozenBattleInput{BattleType: 17, EngineVersion: entry.EngineVersion, ClientVersion: entry.ClientVersion, ConfigurationHash: entry.ConfigurationHash}, entry.ScopeProof) {
			return errors.New(constUtil.P14PromotionRankCorrupt)
		}
	}
	return nil
}
func p14PromotionRankReadVersion(row *db.P14PromotionRankVersion, now time.Time) (*p14PromotionRankSnapshot, error) {
	var snapshot p14PromotionRankSnapshot
	if row == nil || row.ContentHash != p6Digest(row.Content) || p14PromotionRankDecode(row.Content, &snapshot) != nil || snapshot.Offer.SeasonIndex != row.SeasonIndex || snapshot.Offer.ServerID != row.ServerID || snapshot.Revision != row.Revision || snapshot.PreviousHash != row.PreviousHash || snapshot.ObservedAt != row.CapturedAt || snapshot.ObservedAt > now.UnixMilli() || snapshot.ObservedAt < snapshot.Offer.FrozenAt {
		return nil, errors.New(constUtil.P14PromotionRankCorrupt)
	}
	if _, err := p14PromotionRankValidateOffer(&snapshot.Offer, now); err != nil {
		return nil, err
	}
	if snapshot.Revision == 1 && snapshot.PreviousHash != "" || snapshot.Revision > 1 && len(snapshot.PreviousHash) != 64 {
		return nil, errors.New(constUtil.P14PromotionRankCorrupt)
	}
	return &snapshot, nil
}

// p14PromotionRankAdvance reads only accepted rows after the previous immutable
// version. Old scores were already proven; count/max-index checks detect gaps
// without repeatedly decompressing every historical battle on each tick.
func p14PromotionRankAdvance(tx *gorm.DB, q *db.P14AuditionQualification, offer *db.P14PromotionRankOffer, previous *p14PromotionRankSnapshot, now time.Time) ([]p14PromotionRankScore, error) {
	var ids []int64
	if p14PromotionRankDecode(q.Players, &ids) != nil {
		return nil, errors.New(constUtil.P14PromotionRankCorrupt)
	}
	prior := map[int64]p14PromotionRankScore{}
	if previous != nil {
		for _, entry := range previous.Scores {
			prior[entry.PlayerID] = entry
		}
	}
	result := make([]p14PromotionRankScore, 0, len(ids))
	for _, id := range ids {
		entry, ok := prior[id]
		if !ok {
			entry = p14PromotionRankScore{PlayerID: id, Score: 1, LedgerHeadHash: p14PromotionRankInitialHead(q.SourceHash, id)}
		}
		var total, maxIndex int64
		if err := tx.Model(&db.P14PromotionFight{}).Where("season_index=? AND server_id=? AND player_id=?", q.SeasonIndex, q.ServerID, id).Count(&total).Error; err != nil {
			return nil, err
		}
		if err := tx.Model(&db.P14PromotionFight{}).Select("COALESCE(MAX(match_index),0)").Where("season_index=? AND server_id=? AND player_id=?", q.SeasonIndex, q.ServerID, id).Scan(&maxIndex).Error; err != nil {
			return nil, err
		}
		if total < entry.AcceptedMatches || maxIndex != total {
			return nil, errors.New(constUtil.P14PromotionRankCorrupt)
		}
		var rows []db.P14PromotionFight
		if err := tx.Where("season_index=? AND server_id=? AND player_id=? AND match_index>?", q.SeasonIndex, q.ServerID, id, entry.AcceptedMatches).Order("match_index").Find(&rows).Error; err != nil {
			return nil, err
		}
		for _, fight := range rows {
			delta := int64(0)
			if fight.WinnerID == id {
				delta = constUtil.P14PromotionRankWinDelta
			}
			if fight.MatchIndex != entry.AcceptedMatches+1 || fight.QualificationHash != q.SourceHash || fight.ScoreBefore != entry.Score || fight.ScoreDelta != delta || fight.ScoreAfter != entry.Score+delta || fight.CreatedAt < entry.LastMatchAt || fight.CreatedAt > now.UnixMilli() || fight.CreatedAt < offer.StageStart || fight.CreatedAt >= offer.StageEnd {
				return nil, errors.New(constUtil.P14PromotionRankCorrupt)
			}
			if _, err := p14ReadPromotionFight(tx, &fight); err != nil {
				return nil, err
			}
			entry.AcceptedMatches++
			entry.Score = fight.ScoreAfter
			entry.LastMatchAt = fight.CreatedAt
			entry.ScopeProof = fight.ScopeProof
			// Preserve accepted engine/proof identity; a later source version never
			// relabels historical captures or recomputes their old battles.
			var record db.P6BattleRecord
			if err := tx.First(&record, "id=?", fight.P6RecordID).Error; err != nil {
				return nil, err
			}
			entry.EngineVersion, entry.ClientVersion, entry.ConfigurationHash = record.EngineVersion, record.ClientVersion, record.ConfigurationHash
			entry.LedgerHeadHash = p6Digest([]byte(entry.LedgerHeadHash + "/" + fight.SourceHash))
		}
		if entry.AcceptedMatches != total {
			return nil, errors.New(constUtil.P14PromotionRankCorrupt)
		}
		result = append(result, entry)
	}
	return result, nil
}

func p14PromotionRankDeliveryID(request model.P14PromotionRankImportRequest) string {
	return p6Digest([]byte(fmt.Sprintf("p14/rank/%d/%d/%s", request.SourceServerID, request.DestinationServerID, p6Digest(request.Snapshot))))
}
func p14PromotionRankSameProgress(previous, current *p14PromotionRankSnapshot) bool {
	if previous == nil || current == nil {
		return false
	}
	a, _ := json.Marshal(previous.Scores)
	b, _ := json.Marshal(current.Scores)
	return previous.Closed == current.Closed && bytes.Equal(a, b)
}
func p14PromotionRankValidateAdvance(previous, current *p14PromotionRankSnapshot) error {
	if previous == nil {
		if current.Revision == 1 && current.PreviousHash == "" {
			return nil
		}
		return errors.New(constUtil.P14PromotionRankCorrupt)
	}
	if current.Revision != previous.Revision+1 || current.ObservedAt < previous.ObservedAt || current.Offer.SourceHash != previous.Offer.SourceHash || current.QualificationHash != previous.QualificationHash || previous.Closed || p14PromotionRankSameProgress(previous, current) || len(previous.Scores) != len(current.Scores) {
		return errors.New(constUtil.P14PromotionRankCorrupt)
	}
	for i, entry := range current.Scores {
		prior := previous.Scores[i]
		if entry.PlayerID != prior.PlayerID || entry.AcceptedMatches < prior.AcceptedMatches || entry.Score < prior.Score || entry.Score-prior.Score > constUtil.P14PromotionRankWinDelta*(entry.AcceptedMatches-prior.AcceptedMatches) || entry.LastMatchAt < prior.LastMatchAt {
			return errors.New(constUtil.P14PromotionRankCorrupt)
		}
		if entry.AcceptedMatches == prior.AcceptedMatches {
			a, _ := json.Marshal(entry)
			b, _ := json.Marshal(prior)
			if !bytes.Equal(a, b) {
				return errors.New(constUtil.P14PromotionRankCorrupt)
			}
		} else if entry.LedgerHeadHash == prior.LedgerHeadHash {
			return errors.New(constUtil.P14PromotionRankCorrupt)
		}
	}
	return nil
}

// QueueP14PromotionRanks derives a durable source version from genuine local
// qualifiers and newly accepted active-spend facts. An origin seal states only
// that its own phase7 window ended; no global32 qualification is generated.
func (s *GameService) QueueP14PromotionRanks(ctx context.Context, now time.Time) error {
	return db.GetIns().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := ServerGameClockGuardTransaction(tx); err != nil {
			return err
		}

		var season db.P14SeasonPolicy
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&season, 1).Error; err != nil {
			return err
		}
		var policy db.P14PromotionRankPolicy
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&policy, 1).Error; err != nil {
			return err
		}
		if !policy.Enabled || !season.Enabled || season.SeasonIndex <= 0 {
			return nil
		}
		if policy.Source != constUtil.P14PromotionRankSource {
			return errors.New(constUtil.P14PromotionRankCorrupt)
		}
		_, q, err := s.p14PromotionRankQualification(ctx, tx, season.SeasonIndex, s.OriginalServerID(), now)
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil
		}
		if err != nil {
			return err
		}
		// Another transaction may have committed after this tick captured its
		// single clock value. Wait for the next tick without publishing stale time.
		if q.QualifiedAt > now.UnixMilli() {
			return nil
		}
		var existingOffer db.P14PromotionRankOffer
		if readErr := tx.First(&existingOffer, "season_index=? AND server_id=?", season.SeasonIndex, s.OriginalServerID()).Error; readErr == nil {
			// Legacy stage7 snapshots are audit history. The enum correction never
			// rewrites their source hash or publishes a new legacy score version.
			stage, _, stageErr := s.p14SeasonStage(tx, &season, time.UnixMilli(existingOffer.StageStart))
			if stageErr != nil {
				return stageErr
			}
			if stage == constUtil.P14LegacyPromotionStage {
				return nil
			}
			if stage != constUtil.P14PromotionStage {
				return errors.New(constUtil.P14PromotionRankCorrupt)
			}
			if existingOffer.FrozenAt > now.UnixMilli() {
				return nil
			}
		} else if !errors.Is(readErr, gorm.ErrRecordNotFound) {
			return readErr
		}
		offer, err := s.p14PromotionRankOfferTX(tx, &policy, season.SeasonIndex, now)
		if err != nil {
			return err
		}
		// Qualification observed at or after this frozen window cannot publish
		// promotion scores. Keep the genuine certificate and prior raw history.
		if q.QualifiedAt >= offer.StageEnd {
			return nil
		}
		var latest db.P14PromotionRankVersion
		err = tx.Where("season_index=? AND server_id=?", season.SeasonIndex, s.OriginalServerID()).Order("revision DESC").First(&latest).Error
		var previous *p14PromotionRankSnapshot
		if err == nil {
			if latest.CapturedAt > now.UnixMilli() {
				return nil
			}
			previous, err = p14PromotionRankReadVersion(&latest, now)
			if err != nil {
				return err
			}
			if err = p14PromotionRankValidateScores(previous, q); err != nil {
				return err
			}
			if previous.Offer.SourceHash != offer.SourceHash {
				return errors.New(constUtil.P14PromotionRankCorrupt)
			}
		} else if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		var newestMatchAt int64
		if err := tx.Model(&db.P14PromotionFight{}).Select("COALESCE(MAX(created_at),0)").Where("season_index=? AND server_id=?", season.SeasonIndex, s.OriginalServerID()).Scan(&newestMatchAt).Error; err != nil {
			return err
		}
		if newestMatchAt > now.UnixMilli() {
			return nil
		}
		scores, err := p14PromotionRankAdvance(tx, q, offer, previous, now)
		if err != nil {
			return err
		}
		snapshot := p14PromotionRankSnapshot{Offer: *offer, QualificationHash: q.SourceHash, Revision: latest.Revision + 1, PreviousHash: latest.ContentHash, ObservedAt: now.UnixMilli(), Closed: now.UnixMilli() >= offer.StageEnd, Scores: scores}
		if p14PromotionRankSameProgress(previous, &snapshot) {
			return nil
		}
		if err = p14PromotionRankValidateScores(&snapshot, q); err != nil {
			return err
		}
		if err = p14PromotionRankValidateAdvance(previous, &snapshot); err != nil {
			return err
		}
		content, err := json.Marshal(snapshot)
		if err != nil {
			return err
		}
		version := db.P14PromotionRankVersion{SeasonIndex: season.SeasonIndex, ServerID: s.OriginalServerID(), Revision: snapshot.Revision, Content: content, ContentHash: p6Digest(content), PreviousHash: snapshot.PreviousHash, CapturedAt: snapshot.ObservedAt}
		if err = tx.Create(&version).Error; err != nil {
			return err
		}
		peers, err := p14PromotionRankValidateOffer(offer, now)
		if err != nil {
			return err
		}
		for _, peer := range peers {
			if peer == s.OriginalServerID() {
				continue
			}
			request := model.P14PromotionRankImportRequest{ProtocolVersion: constUtil.P14PromotionRankProtocol, SourceServerID: s.OriginalServerID(), DestinationServerID: peer, Snapshot: content}
			request.DeliveryID = p14PromotionRankDeliveryID(request)
			payload, err := json.Marshal(request)
			if err != nil {
				return err
			}
			delivery := db.P14PromotionRankDelivery{ID: request.DeliveryID, SeasonIndex: season.SeasonIndex, SourceServerID: s.OriginalServerID(), DestinationServerID: peer, Revision: version.Revision, Payload: payload, PayloadHash: p6Digest(payload), CreatedAt: now.UnixMilli()}
			if err = tx.Create(&delivery).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

// ImportP14PromotionRank authenticates the exact raw body using the configured
// peer key, then commits only the next qualified version. An immutable version
// itself is the duplicate receipt; no additional mirror receipt table is used.
func (s *GameService) ImportP14PromotionRank(ctx context.Context, request model.P14PromotionRankImportRequest, raw []byte, signature string) (*model.P14PromotionRankImportResponse, error) {
	now := p14PromotionRankClock(ctx)
	var decoded model.P14PromotionRankImportRequest
	if p14PromotionRankDecode(raw, &decoded) != nil {
		return nil, errors.New(constUtil.P14PromotionRankCorrupt)
	}
	parsed, _ := json.Marshal(decoded)
	supplied, _ := json.Marshal(request)
	if !bytes.Equal(parsed, supplied) || request.ProtocolVersion != constUtil.P14PromotionRankProtocol || request.SourceServerID == s.OriginalServerID() || request.DestinationServerID != s.OriginalServerID() || request.DeliveryID != p14PromotionRankDeliveryID(request) {
		return nil, errors.New(constUtil.P14PromotionRankCorrupt)
	}
	var peer db.P14TrustedPeer
	if err := db.GetIns().WithContext(ctx).First(&peer, "server_id=? AND enabled=1", request.SourceServerID).Error; err != nil {
		return nil, err
	}
	mac := hmac.New(sha256.New, peer.SigningKey)
	mac.Write(raw)
	given, err := hex.DecodeString(signature)
	if len(peer.SigningKey) < 32 || err != nil || !hmac.Equal(mac.Sum(nil), given) {
		return nil, errors.New(constUtil.P14PromotionRankCorrupt)
	}
	var incoming p14PromotionRankSnapshot
	if p14PromotionRankDecode(request.Snapshot, &incoming) != nil || incoming.Offer.ServerID != request.SourceServerID {
		return nil, errors.New(constUtil.P14PromotionRankCorrupt)
	}
	version := db.P14PromotionRankVersion{SeasonIndex: incoming.Offer.SeasonIndex, ServerID: incoming.Offer.ServerID, Revision: incoming.Revision, Content: append([]byte(nil), request.Snapshot...), ContentHash: p6Digest(request.Snapshot), PreviousHash: incoming.PreviousHash, CapturedAt: incoming.ObservedAt, ImportedAt: now.UnixMilli()}
	if _, err = p14PromotionRankReadVersion(&version, now); err != nil {
		return nil, err
	}
	err = db.GetIns().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := ServerGameClockGuardTransaction(tx); err != nil {
			return err
		}

		var season db.P14SeasonPolicy
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&season, 1).Error; err != nil {
			return err
		}
		var saved db.P14PromotionRankVersion
		err := tx.First(&saved, "season_index=? AND server_id=? AND revision=?", version.SeasonIndex, version.ServerID, version.Revision).Error
		if err == nil {
			if saved.ContentHash != version.ContentHash || !bytes.Equal(saved.Content, version.Content) {
				return errors.New(constUtil.P14PromotionRankCorrupt)
			}
			_, readErr := p14PromotionRankReadVersion(&saved, now)
			return readErr
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		var policy db.P14PromotionRankPolicy
		if err = tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&policy, 1).Error; err != nil {
			return err
		}
		if !policy.Enabled || policy.Source != constUtil.P14PromotionRankSource {
			return errors.New(constUtil.P14PromotionRankClosed)
		}
		own, err := s.p14PromotionRankOfferTX(tx, &policy, version.SeasonIndex, now)
		if err != nil {
			return err
		}
		if own.Version != incoming.Offer.Version || own.RulesHash != incoming.Offer.RulesHash || own.CohortHash != incoming.Offer.CohortHash || own.StageStart != incoming.Offer.StageStart || own.StageEnd != incoming.Offer.StageEnd {
			return errors.New(constUtil.P14PromotionRankCorrupt)
		}
		peers, err := p14PromotionRankValidateOffer(own, now)
		if err != nil {
			return err
		}
		found := false
		for _, id := range peers {
			found = found || id == request.SourceServerID
		}
		if !found {
			return errors.New(constUtil.P14PromotionRankCorrupt)
		}
		_, q, err := s.p14PromotionRankQualification(ctx, tx, version.SeasonIndex, version.ServerID, now)
		if err != nil {
			return err
		}
		if err = p14PromotionRankValidateScores(&incoming, q); err != nil {
			return err
		}
		var latest db.P14PromotionRankVersion
		err = tx.Where("season_index=? AND server_id=?", version.SeasonIndex, version.ServerID).Order("revision DESC").First(&latest).Error
		var previous *p14PromotionRankSnapshot
		if err == nil {
			previous, err = p14PromotionRankReadVersion(&latest, now)
			if err != nil {
				return err
			}
			if incoming.PreviousHash != latest.ContentHash {
				return errors.New(constUtil.P14PromotionRankCorrupt)
			}
		} else if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		if err = p14PromotionRankValidateAdvance(previous, &incoming); err != nil {
			return err
		}
		return tx.Create(&version).Error
	})
	if err != nil {
		return nil, err
	}
	return &model.P14PromotionRankImportResponse{DeliveryID: request.DeliveryID, Confirmed: true}, nil
}

// DeliverP14PromotionRanks retries immutable versions in source revision order.
// An unavailable peer leaves the outbox pending, so a later cold start resumes.
func (s *GameService) DeliverP14PromotionRanks(ctx context.Context) error {
	tx := db.GetIns().WithContext(ctx)
	var gate db.P14PromotionRankPolicy
	if err := tx.First(&gate, 1).Error; err != nil {
		return err
	}
	if !gate.Enabled {
		return nil
	}
	if gate.Source != constUtil.P14PromotionRankSource {
		return errors.New(constUtil.P14PromotionRankCorrupt)
	}
	// One oldest version per peer prevents an offline destination from filling
	// every batch. Retry counts rotate the bounded cohort fairly across ticks.
	var peers []int64
	if err := tx.Model(&db.P14PromotionRankDelivery{}).Where("confirmed=0").Distinct("destination_server_id").Order("destination_server_id").Pluck("destination_server_id", &peers).Error; err != nil {
		return err
	}
	if len(peers) > constUtil.P14PromotionRankPeerLimit {
		return errors.New(constUtil.P14PromotionRankCorrupt)
	}
	var rows []db.P14PromotionRankDelivery
	for _, id := range peers {
		var row db.P14PromotionRankDelivery
		if err := tx.Where("confirmed=0 AND destination_server_id=?", id).Order("season_index,revision").First(&row).Error; err != nil {
			return err
		}
		rows = append(rows, row)
	}
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].AttemptCount != rows[j].AttemptCount {
			return rows[i].AttemptCount < rows[j].AttemptCount
		}
		if rows[i].CreatedAt != rows[j].CreatedAt {
			return rows[i].CreatedAt < rows[j].CreatedAt
		}
		return rows[i].DestinationServerID < rows[j].DestinationServerID
	})
	if len(rows) > constUtil.P14PromotionRankBatch {
		rows = rows[:constUtil.P14PromotionRankBatch]
	}
	for _, row := range rows {
		if err := ctx.Err(); err != nil {
			return err
		}
		if row.PayloadHash != p6Digest(row.Payload) {
			return errors.New(constUtil.P14PromotionRankCorrupt)
		}
		var peer db.P14TrustedPeer
		if err := tx.First(&peer, "server_id=? AND enabled=1", row.DestinationServerID).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				continue
			}
			return err
		}
		if len(peer.SigningKey) < 32 || peer.Endpoint == "" {
			continue
		}
		cli := httpClient.HttpInner("p14-promotion-rank-"+strconv.FormatInt(peer.ServerID, 10)).SetTimeout(3*time.Second).AddHeader("X-HYJS-P14-Signature", P14SignPayload(peer.SigningKey, row.Payload))
		status, raw, callErr := cli.PostWithContext(ctx, peer.Endpoint+constUtil.P14PromotionRankPath, row.Payload)
		if err := tx.Model(&db.P14PromotionRankDelivery{}).Where("id=? AND confirmed=0", row.ID).UpdateColumn("attempt_count", gorm.Expr("attempt_count+1")).Error; err != nil {
			return err
		}
		if callErr != nil || status != http.StatusOK {
			continue
		}
		var reply struct {
			Code int64                                `json:"code"`
			Data model.P14PromotionRankImportResponse `json:"data"`
		}
		if json.Unmarshal(raw, &reply) != nil || reply.Code != 0 || !reply.Data.Confirmed || reply.Data.DeliveryID != row.ID {
			continue
		}
		if err := tx.Model(&db.P14PromotionRankDelivery{}).Where("id=? AND confirmed=0", row.ID).Updates(map[string]any{"confirmed": true, "confirmed_at": p14PromotionRankClock(ctx).UnixMilli()}).Error; err != nil {
			return err
		}
	}
	return nil
}

type p14PromotionRankProjection struct {
	ServerID, PlayerID, Score int64
	Power                     int32
	Name                      string
}

func p14PromotionRankSort(rows []p14PromotionRankProjection) {
	sort.Slice(rows, func(i, j int) bool {
		a, b := rows[i], rows[j]
		if a.Score != b.Score {
			return a.Score > b.Score
		}
		if a.Power != b.Power {
			return a.Power > b.Power
		}
		if a.ServerID != b.ServerID {
			return a.ServerID < b.ServerID
		}
		return a.PlayerID < b.PlayerID
	})
}

// p14PromotionRankReadAt requires every frozen cohort member's qualified score
// version. Missing remote facts produce a pending error, never a local-only
// global rank. Even a complete list does not create final32 qualification.
func (s *GameService) p14PromotionRankReadAt(ctx context.Context, request *pb.CSKFRankInfo, now time.Time) ([]proto.Message, error) {
	if err := s.QueueP14PromotionRanks(ctx, now); err != nil {
		return nil, err
	}
	var response *pb.SCKFRankInfo
	err := db.GetIns().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := ServerGameClockGuardTransaction(tx); err != nil {
			return err
		}

		var gate db.P14PromotionRankPolicy
		if err := tx.First(&gate, 1).Error; err != nil {
			return err
		}
		if !gate.Enabled || gate.Source != constUtil.P14PromotionRankSource {
			return errors.New(constUtil.P14PromotionRankClosed)
		}
		var policy db.P14SeasonPolicy
		if err := tx.First(&policy, 1).Error; err != nil {
			return err
		}
		season := request.GetPeakMatchIndex()
		if season == 0 {
			season = policy.SeasonIndex
		}
		var own db.P14PromotionRankOffer
		if err := tx.First(&own, "season_index=? AND server_id=?", season, s.OriginalServerID()).Error; err != nil {
			return errors.New(constUtil.P14PromotionRankClosed)
		}
		peers, err := p14PromotionRankValidateOffer(&own, now)
		if err != nil {
			return err
		}
		var projection []p14PromotionRankProjection
		for _, server := range peers {
			var latest db.P14PromotionRankVersion
			if err := tx.Where("season_index=? AND server_id=?", season, server).Order("revision DESC").First(&latest).Error; err != nil {
				if errors.Is(err, gorm.ErrRecordNotFound) {
					return errors.New(constUtil.P14PromotionRankClosed)
				}
				return err
			}
			snapshot, err := p14PromotionRankReadVersion(&latest, now)
			if err != nil {
				return err
			}
			if snapshot.Offer.Version != own.Version || snapshot.Offer.RulesHash != own.RulesHash || snapshot.Offer.CohortHash != own.CohortHash || snapshot.Offer.StageStart != own.StageStart || snapshot.Offer.StageEnd != own.StageEnd {
				return errors.New(constUtil.P14PromotionRankCorrupt)
			}
			roster, q, err := s.p14PromotionRankQualification(ctx, tx, season, server, now)
			if err != nil {
				return err
			}
			if err = p14PromotionRankValidateScores(snapshot, q); err != nil {
				return err
			}
			rosterByID := map[int64]db.P14AuditionRoster{}
			for _, item := range roster {
				rosterByID[item.PlayerID] = item
			}
			for _, entry := range snapshot.Scores {
				row, ok := rosterByID[entry.PlayerID]
				if !ok {
					return errors.New(constUtil.P14PromotionRankCorrupt)
				}
				formation := &pb.FormationSimpleInfo{}
				if proto.Unmarshal(row.Snapshot, formation) != nil || formation.GetFightPower() != row.FightPower {
					return errors.New(constUtil.P14PromotionRankCorrupt)
				}
				projection = append(projection, p14PromotionRankProjection{ServerID: server, PlayerID: entry.PlayerID, Score: entry.Score, Power: row.FightPower, Name: formation.GetName()})
			}
		}
		p14PromotionRankSort(projection)
		info := &pb.RankInfo{Type: proto.Int32(2)}
		for i, row := range projection {
			info.List = append(info.List, &pb.RankRowInfo{Pid: proto.Int64(row.PlayerID), Rank: proto.Int32(int32(i + 1)), Name: proto.String(row.Name), Values: []int32{row.Power, int32(row.ServerID), int32(row.Score)}})
		}
		response = &pb.SCKFRankInfo{Type: proto.Int32(2), SubType: proto.Int32(0), RankIno: info}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return []proto.Message{response}, nil
}

// ServeP14PromotionRank is one bounded ctx worker for this independent default
// closed domain. All actual scores are read from accepted business ledgers.
func (s *GameService) ServeP14PromotionRank(ctx context.Context) error {
	ticker := time.NewTicker(constUtil.P14DeliveryInterval)
	defer ticker.Stop()
	for {
		if err := s.QueueP14PromotionRanks(ctx, LocalPlayAt(ctx, p14PromotionRankClock(ctx))); err != nil {
			return err
		}
		if err := s.DeliverP14PromotionRanks(ctx); err != nil {
			return err
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}
