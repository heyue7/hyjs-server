package service

import (
	"bytes"
	"compress/gzip"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/httpClient"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
	"io"
	"net/http"
	"strconv"
	"time"
)

// The original full terminal is preserved. Only frozen tables are omitted from
// transport; a peer must reconstruct the exact complete input hash using the
// same current engine/client/config before it may accept anything.
type p14FinalResultBody struct {
	Offer           db.P14FinalResultOffer     `json:"offer"`
	SettlementOffer db.P14FinalSettlementOffer `json:"settlementOffer"`
	Result          db.P14FinalSettlement      `json:"result"`
	Input           []byte                     `json:"input"`
	InputHash       string                     `json:"inputHash"`
	Terminal        []byte                     `json:"terminal"`
}
type p14FinalResultBlob struct {
	Codec       string `json:"codec"`
	DecodedSize int64  `json:"decodedSize"`
	ContentHash string `json:"contentHash"`
	Data        []byte `json:"data"`
}

func p14FinalResultOfferHash(row *db.P14FinalResultOffer) string {
	copy := *row
	copy.SourceHash = ""
	raw, _ := json.Marshal(copy)
	return p6Digest(raw)
}
func p14FinalResultReceiptHash(row *db.P14FinalResultReceipt) string {
	copy := *row
	copy.SourceHash = ""
	raw, _ := json.Marshal(copy)
	return p6Digest(raw)
}
func p14FinalResultDeliveryID(request *model.P14FinalResultImportRequest) string {
	return p6Digest([]byte(fmt.Sprintf("p14/final-result/%d/%d/%d/%s", request.ProtocolVersion, request.SourceServerID, request.DestinationServerID, p6Digest(request.Snapshot))))
}
func p14FinalResultEncode(body *p14FinalResultBody) (json.RawMessage, error) {
	raw, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	if int64(len(raw)) > constUtil.P14FinalResultMaxDecodedBytes {
		return nil, errors.New(constUtil.P14FinalResultCorrupt)
	}
	var buffer bytes.Buffer
	zipper := gzip.NewWriter(&buffer)
	if _, err = zipper.Write(raw); err != nil {
		return nil, err
	}
	if err = zipper.Close(); err != nil {
		return nil, err
	}
	return json.Marshal(p14FinalResultBlob{Codec: constUtil.P14FinalResultCodec, DecodedSize: int64(len(raw)), ContentHash: p6Digest(raw), Data: buffer.Bytes()})
}
func p14FinalResultDecode(raw json.RawMessage) (*p14FinalResultBody, error) {
	var blob p14FinalResultBlob
	if p14PromotionRankDecode(raw, &blob) != nil || blob.Codec != constUtil.P14FinalResultCodec || blob.DecodedSize <= 0 || blob.DecodedSize > constUtil.P14FinalResultMaxDecodedBytes || len(blob.ContentHash) != 64 {
		return nil, errors.New(constUtil.P14FinalResultCorrupt)
	}
	reader, err := gzip.NewReader(bytes.NewReader(blob.Data))
	if err != nil {
		return nil, err
	}
	defer reader.Close()
	decoded, err := io.ReadAll(io.LimitReader(reader, constUtil.P14FinalResultMaxDecodedBytes+1))
	if err != nil {
		return nil, err
	}
	var body p14FinalResultBody
	if int64(len(decoded)) != blob.DecodedSize || p6Digest(decoded) != blob.ContentHash || p14PromotionRankDecode(decoded, &body) != nil {
		return nil, errors.New(constUtil.P14FinalResultCorrupt)
	}
	return &body, nil
}
func p14FinalResultPolicyValid(policy *db.P14FinalResultPolicy, manifest *p14FinalSeriesManifest) bool {
	return policy != nil && manifest != nil && policy.ID == 1 && policy.Enabled && policy.ScopeProof != "" && policy.ScopeProof == constUtil.P6KFCanonicalNativeProof && policy.Version != "" && len(policy.Version) <= 128 && policy.Source == constUtil.P14FinalResultSource && policy.PublishedAt > 0 && policy.PublishedAt <= manifest.FormationOffer.StageStart
}
func (s *GameService) InitializeP14FinalResult(ctx context.Context) error {
	tx := db.GetIns().WithContext(ctx)
	if err := tx.AutoMigrate(&db.P14FinalResultPolicy{}, &db.P14FinalResultOffer{}, &db.P14FinalResultReceipt{}, &db.P14FinalResultDelivery{}); err != nil {
		return err
	}
	return tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&db.P14FinalResultPolicy{ID: 1, ScopeProof: constUtil.P6KFCanonicalNativeProof, Source: constUtil.P14FinalResultSource}).Error
}

func p14FinalResultOfferTX(tx *gorm.DB, series *db.P14FinalSeries, manifest *p14FinalSeriesManifest) (*db.P14FinalResultOffer, error) {
	var offer db.P14FinalResultOffer
	err := tx.First(&offer, "series_id=?", series.ID).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		var policy db.P14FinalResultPolicy
		if err = tx.First(&policy, 1).Error; err != nil {
			return nil, err
		}
		if !p14FinalResultPolicyValid(&policy, manifest) {
			return nil, errors.New(constUtil.P14FinalResultPending)
		}
		raw, err := json.Marshal(policy)
		if err != nil {
			return nil, err
		}
		offer = db.P14FinalResultOffer{SeriesID: series.ID, SeriesManifestHash: series.ManifestHash, Policy: raw, Rules: constUtil.P14FinalResultRules}
		offer.SourceHash = p14FinalResultOfferHash(&offer)
		if err = tx.Create(&offer).Error; err != nil {
			return nil, err
		}
	} else if err != nil {
		return nil, err
	}
	var policy db.P14FinalResultPolicy
	if offer.SourceHash != p14FinalResultOfferHash(&offer) || offer.SeriesManifestHash != series.ManifestHash || offer.Rules != constUtil.P14FinalResultRules || p14PromotionRankDecode(offer.Policy, &policy) != nil || !p14FinalResultPolicyValid(&policy, manifest) {
		return nil, errors.New(constUtil.P14FinalResultCorrupt)
	}
	return &offer, nil
}

// QueueP14FinalResults reads only an accepted local canonical result. Source0
// authority and its full P6 record must still validate before an outbox is built.
func (s *GameService) QueueP14FinalResults(ctx context.Context, now time.Time) error {
	return db.GetIns().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := ServerGameClockGuardTransaction(tx); err != nil {
			return err
		}

		var season db.P14SeasonPolicy
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&season, 1).Error; err != nil {
			return err
		}
		var gate db.P14FinalResultPolicy
		if err := tx.First(&gate, 1).Error; err != nil {
			return err
		}
		if !season.Enabled || !gate.Enabled || constUtil.P6KFCanonicalNativeProof == "" {
			return nil
		}
		var results []db.P14FinalSettlement
		if err := tx.Where("game_index=1").Order("series_id").Find(&results).Error; err != nil {
			return err
		}
		for _, result := range results {
			var series db.P14FinalSeries
			if err := tx.First(&series, "id=?", result.SeriesID).Error; err != nil {
				return err
			}
			manifest, err := p14PerGameReadSeries(&series)
			if err != nil {
				return err
			}
			if series.SeasonIndex != season.SeasonIndex || manifest.Pair.LeftServerID != s.OriginalServerID() {
				continue
			}
			var slot db.P14FinalSeriesSlot
			if err = tx.First(&slot, "series_id=? AND game_index=1", series.ID).Error; err != nil {
				return err
			}
			if _, err = p14ReadFinalSettlementTX(tx, &result, &slot, manifest); err != nil {
				return err
			}
			offer, err := p14FinalResultOfferTX(tx, &series, manifest)
			if err != nil {
				return err
			}
			var settlementOffer db.P14FinalSettlementOffer
			if err = tx.First(&settlementOffer, "series_id=?", series.ID).Error; err != nil {
				return err
			}
			var record db.P6BattleRecord
			if err = tx.First(&record, "id=?", result.P6RecordID).Error; err != nil {
				return err
			}
			frozen, err := decodeP6Record(&record)
			if err != nil {
				return err
			}
			copy := *frozen
			copy.Tables = nil
			input, err := json.Marshal(&copy)
			if err != nil {
				return err
			}
			body := p14FinalResultBody{Offer: *offer, SettlementOffer: settlementOffer, Result: result, Input: input, InputHash: record.InputHash, Terminal: result.Terminal}
			snapshot, err := p14FinalResultEncode(&body)
			if err != nil {
				return err
			}
			var peers []db.P14TrustedPeer
			if err = tx.Where("enabled=1 AND server_id<>?", s.OriginalServerID()).Order("server_id").Find(&peers).Error; err != nil {
				return err
			}
			for _, peer := range peers {
				var existing db.P14FinalResultDelivery
				err = tx.First(&existing, "series_id=? AND game_index=1 AND destination_server_id=?", series.ID, peer.ServerID).Error
				if err == nil {
					if existing.SourceResultHash != result.SourceHash || existing.PayloadHash != p6Digest(existing.Payload) {
						return errors.New(constUtil.P14FinalResultCorrupt)
					}
					continue
				}
				if !errors.Is(err, gorm.ErrRecordNotFound) {
					return err
				}
				envelope := model.P14FinalResultImportRequest{ProtocolVersion: 1, SourceServerID: s.OriginalServerID(), DestinationServerID: peer.ServerID, Snapshot: snapshot}
				envelope.DeliveryID = p14FinalResultDeliveryID(&envelope)
				encoded, err := json.Marshal(envelope)
				if err != nil {
					return err
				}
				if len(encoded) > 1<<20 {
					return errors.New(constUtil.P14FinalResultCorrupt)
				}
				row := db.P14FinalResultDelivery{SeriesID: series.ID, GameIndex: 1, SourceResultHash: result.SourceHash, ID: envelope.DeliveryID, DestinationServerID: peer.ServerID, Payload: encoded, PayloadHash: p6Digest(encoded), CreatedAt: now.UnixMilli()}
				if err = tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&row).Error; err != nil {
					return err
				}
			}
		}
		return nil
	})
}

// ImportP14FinalResult verifies a trusted origin's exact raw HMAC, genuine
// source0 joint and independently certified terminal. It performs no calculation
// and never creates a local P6BattleRecord for that external owner's PID.
func (s *GameService) ImportP14FinalResult(ctx context.Context, request *model.P14FinalResultImportRequest, raw []byte, signature string) (*model.P14FinalResultImportResponse, error) {
	if request == nil {
		return nil, errors.New(constUtil.P14FinalResultCorrupt)
	}
	now := LocalPlayNow(ctx)
	if clock, ok := ctx.Value(constUtil.P14CrossClockContextKey{}).(func() time.Time); ok {
		now = clock()
	}
	var signed model.P14FinalResultImportRequest
	if p14PromotionRankDecode(raw, &signed) != nil || signed.ProtocolVersion != request.ProtocolVersion || signed.DeliveryID != request.DeliveryID || signed.SourceServerID != request.SourceServerID || signed.DestinationServerID != request.DestinationServerID || !bytes.Equal(signed.Snapshot, request.Snapshot) {
		return nil, errors.New(constUtil.P14FinalResultCorrupt)
	}
	response := &model.P14FinalResultImportResponse{DeliveryID: request.DeliveryID}
	err := db.GetIns().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := ServerGameClockGuardTransaction(tx); err != nil {
			return err
		}

		var season db.P14SeasonPolicy
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&season, 1).Error; err != nil {
			return err
		}
		if request.ProtocolVersion != 1 || request.SourceServerID == s.OriginalServerID() || request.DestinationServerID != s.OriginalServerID() || request.DeliveryID != p14FinalResultDeliveryID(request) {
			return errors.New(constUtil.P14FinalResultCorrupt)
		}
		var peer db.P14TrustedPeer
		if err := tx.First(&peer, "server_id=? AND enabled=1", request.SourceServerID).Error; err != nil {
			return err
		}
		supplied, err := hex.DecodeString(signature)
		mac := hmac.New(sha256.New, peer.SigningKey)
		_, _ = mac.Write(raw)
		if len(peer.SigningKey) < 32 || err != nil || !hmac.Equal(mac.Sum(nil), supplied) {
			return errors.New(constUtil.P14FinalResultCorrupt)
		}
		var previous db.P14FinalResultReceipt
		err = tx.First(&previous, "delivery_id=?", request.DeliveryID).Error
		if err == nil {
			if previous.PayloadHash != p6Digest(raw) {
				return errors.New(constUtil.P14FinalResultCorrupt)
			}
			if _, _, err = p14ReadFinalResultReceipt(&previous); err != nil {
				return err
			}
			response.Confirmed = true
			return nil
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		body, err := p14FinalResultDecode(request.Snapshot)
		if err != nil {
			return err
		}
		var series db.P14FinalSeries
		if err = tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&series, "id=?", body.Result.SeriesID).Error; err != nil {
			return err
		}
		manifest, err := p14PerGameReadSeries(&series)
		if err != nil {
			return err
		}
		if request.SourceServerID != manifest.Pair.LeftServerID || series.CurrentGame != 1 || series.State != constUtil.P14FinalSeriesWaitingResult || series.LeftWins != 0 || series.RightWins != 0 || series.WinnerPlayerID != 0 || series.WinnerServerID != 0 {
			return errors.New(constUtil.P14FinalResultCorrupt)
		}
		var slot db.P14FinalSeriesSlot
		if err = tx.First(&slot, "series_id=? AND game_index=1", series.ID).Error; err != nil {
			return err
		}
		if slot.State != constUtil.P14FinalSeriesSlotInputReady || len(slot.Certificate) != 0 || body.Result.CreatedAt < slot.ScheduledAt || body.Result.CreatedAt > now.UnixMilli() || body.Result.GameIndex != 1 || body.Result.InputID != slot.InputID || body.Result.InputHash != slot.InputHash || body.Result.ProvenanceHash != slot.ProvenanceHash || slot.InputHash != p6Digest(slot.Input) {
			return errors.New(constUtil.P14FinalResultCorrupt)
		}
		if body.Offer.SeriesID != series.ID || body.Offer.SeriesManifestHash != series.ManifestHash || body.Offer.Rules != constUtil.P14FinalResultRules || body.Offer.SourceHash != p14FinalResultOfferHash(&body.Offer) {
			return errors.New(constUtil.P14FinalResultCorrupt)
		}
		var sourcePolicy db.P14FinalResultPolicy
		if p14PromotionRankDecode(body.Offer.Policy, &sourcePolicy) != nil || !p14FinalResultPolicyValid(&sourcePolicy, manifest) {
			return errors.New(constUtil.P14FinalResultCorrupt)
		}
		var localPolicy db.P14FinalResultPolicy
		if err = tx.First(&localPolicy, 1).Error; err != nil {
			return err
		}
		localPolicy.Enabled = true
		localRaw, _ := json.Marshal(localPolicy)
		if !bytes.Equal(localRaw, body.Offer.Policy) {
			return errors.New(constUtil.P14FinalResultPending)
		}
		var localOffer db.P14FinalResultOffer
		err = tx.First(&localOffer, "series_id=?", series.ID).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			if err = tx.Create(&body.Offer).Error; err != nil {
				return err
			}
		} else if err != nil {
			return err
		} else if localOffer.SourceHash != body.Offer.SourceHash {
			return errors.New(constUtil.P14FinalResultCorrupt)
		}
		if body.SettlementOffer.SeriesID != series.ID || body.SettlementOffer.SeriesManifestHash != series.ManifestHash || body.SettlementOffer.SourceHash != body.Result.OfferHash || body.SettlementOffer.SourceHash != p14FinalSettlementOfferHash(&body.SettlementOffer) || body.SettlementOffer.Rules != constUtil.P14FinalSettlementRules {
			return errors.New(constUtil.P14FinalResultCorrupt)
		}
		var settlementPolicy db.P14FinalSettlementPolicy
		if p14PromotionRankDecode(body.SettlementOffer.Policy, &settlementPolicy) != nil || !p14FinalSettlementPolicyValid(&settlementPolicy, manifest) {
			return errors.New(constUtil.P14FinalResultCorrupt)
		}
		body.Result.Terminal = body.Terminal
		if body.Result.SourceHash != p14FinalSettlementHash(&body.Result) || body.Result.TerminalHash != p6Digest(body.Terminal) {
			return errors.New(constUtil.P14FinalResultCorrupt)
		}
		var transported model.FrozenBattleInput
		if p14PromotionRankDecode(body.Input, &transported) != nil || transported.Tables != nil || transported.EngineVersion != constUtil.P6EngineVersion || transported.ClientVersion != constUtil.P6OriginalClientHash {
			return errors.New(constUtil.P14FinalResultPending)
		}
		sourceConfig := transported.ConfigurationHash
		frozen, err := s.FreezeBattleInput(transported)
		if err != nil {
			return err
		}
		frozenRaw, err := json.Marshal(frozen)
		if err != nil {
			return err
		}
		original := &pb.KFReplayData{}
		if sourceConfig != frozen.ConfigurationHash || p6Digest(frozenRaw) != body.InputHash || proto.Unmarshal(slot.Input, original) != nil || !proto.Equal(original, frozen.KFReplay) {
			return errors.New(constUtil.P14FinalResultCorrupt)
		}
		var terminal model.CombatFinalHPOutcome
		if p14PromotionRankDecode(body.Terminal, &terminal) != nil {
			return errors.New(constUtil.P14FinalResultCorrupt)
		}
		server, player, err := P6KFCanonicalWinner(frozen, &terminal)
		if err != nil {
			return err
		}
		if body.Result.ScopeProof != terminal.Outcome.CapabilityHash || body.Result.WinnerCamp != terminal.Outcome.Winner || body.Result.WinnerServerID != server || body.Result.WinnerPlayerID != player {
			return errors.New(constUtil.P14FinalResultCorrupt)
		}
		stored, err := encodeP6StoredInput(frozenRaw)
		if err != nil {
			return err
		}
		resultRaw, err := json.Marshal(&body.Result)
		if err != nil {
			return err
		}
		row := db.P14FinalResultReceipt{SeriesID: series.ID, GameIndex: 1, DeliveryID: request.DeliveryID, SourceServerID: request.SourceServerID, PayloadHash: p6Digest(raw), SourceResult: resultRaw, SourceResultHash: body.Result.SourceHash, Input: stored, InputHash: body.InputHash, Terminal: body.Terminal, TerminalHash: p6Digest(body.Terminal), OfferHash: body.Offer.SourceHash, WinnerServerID: server, WinnerPlayerID: player, ReceivedAt: now.UnixMilli()}
		row.SourceHash = p14FinalResultReceiptHash(&row)
		if err = tx.Create(&row).Error; err != nil {
			return err
		}
		certificate, err := json.Marshal(row)
		if err != nil {
			return err
		}
		if err = tx.Model(&slot).Updates(map[string]any{"state": constUtil.P14FinalSeriesSlotCertified, "certificate": certificate, "certificate_hash": p6Digest(certificate)}).Error; err != nil {
			return err
		}
		left, right, next, state, err := p14FinalSeriesProgress([]int32{terminal.Outcome.Winner})
		if err != nil {
			return err
		}
		if err = tx.Model(&series).Updates(map[string]any{"left_wins": left, "right_wins": right, "current_game": next, "state": state, "updated_at": row.ReceivedAt}).Error; err != nil {
			return err
		}
		response.Confirmed = true
		return nil
	})
	return response, err
}

// p14ReadFinalResultReceipt is a pure receipt verifier. Frozen remote tables
// and terminal remain available for cold reads without a local P6 owner or rerun.
func p14ReadFinalResultReceipt(row *db.P14FinalResultReceipt) (*model.FrozenBattleInput, *model.CombatFinalHPOutcome, error) {
	reject := func() (*model.FrozenBattleInput, *model.CombatFinalHPOutcome, error) {
		return nil, nil, errors.New(constUtil.P14FinalResultCorrupt)
	}
	if row == nil || row.SourceHash != p14FinalResultReceiptHash(row) || row.TerminalHash != p6Digest(row.Terminal) {
		return reject()
	}
	raw, err := decodeP6StoredInput(row.Input)
	if err != nil || p6Digest(raw) != row.InputHash {
		return reject()
	}
	var input model.FrozenBattleInput
	var terminal model.CombatFinalHPOutcome
	var result db.P14FinalSettlement
	if p14PromotionRankDecode(raw, &input) != nil || p14PromotionRankDecode(row.Terminal, &terminal) != nil || p14PromotionRankDecode(row.SourceResult, &result) != nil {
		return reject()
	}
	result.Terminal = row.Terminal
	if result.SourceHash != row.SourceResultHash || result.SourceHash != p14FinalSettlementHash(&result) || result.SeriesID != row.SeriesID || result.GameIndex != row.GameIndex || input.KFReplay == nil || len(input.KFReplay.PlayerList) != 2 || int64(input.KFReplay.PlayerList[0].GetServerId()) != row.SourceServerID {
		return reject()
	}
	server, player, err := P6KFCanonicalWinner(&input, &terminal)
	if err != nil || row.WinnerServerID != server || row.WinnerPlayerID != player || result.WinnerServerID != server || result.WinnerPlayerID != player || terminal.Outcome.Winner != result.WinnerCamp || terminal.Outcome.CapabilityHash != result.ScopeProof {
		return reject()
	}
	return &input, &terminal, nil
}

// DeliverP14FinalResults fulfills already frozen outboxes even if operators
// close new forwarding. Offline peers remain pending and acquire no fake loss.
func (s *GameService) DeliverP14FinalResults(ctx context.Context) error {
	tx := db.GetIns().WithContext(ctx)
	var destinations []int64
	if err := tx.Model(&db.P14FinalResultDelivery{}).Where("confirmed=0").Distinct("destination_server_id").Order("destination_server_id").Pluck("destination_server_id", &destinations).Error; err != nil {
		return err
	}
	if len(destinations) > 32 {
		return errors.New(constUtil.P14FinalResultCorrupt)
	}
	for _, destination := range destinations {
		if err := ctx.Err(); err != nil {
			return err
		}
		var row db.P14FinalResultDelivery
		if err := tx.Where("confirmed=0 AND destination_server_id=?", destination).Order("created_at,id").First(&row).Error; err != nil {
			return err
		}
		if row.PayloadHash != p6Digest(row.Payload) {
			return errors.New(constUtil.P14FinalResultCorrupt)
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
		client := httpClient.HttpInner("p14-final-result-"+strconv.FormatInt(destination, 10)).SetTimeout(3*time.Second).AddHeader("X-HYJS-P14-Signature", P14SignPayload(peer.SigningKey, row.Payload))
		status, raw, callErr := client.PostWithContext(ctx, peer.Endpoint+constUtil.P14FinalResultImportPath, row.Payload)
		if err = tx.Model(&db.P14FinalResultDelivery{}).Where("id=? AND confirmed=0", row.ID).Updates(map[string]any{"attempt_count": gorm.Expr("attempt_count+1"), "last_attempt_at": p14PromotionRankClock(ctx).UnixMilli()}).Error; err != nil {
			return err
		}
		if callErr != nil || status != http.StatusOK {
			continue
		}
		var reply struct {
			Code int64                              `json:"code"`
			Data model.P14FinalResultImportResponse `json:"data"`
		}
		if json.Unmarshal(raw, &reply) != nil || reply.Code != 0 || !reply.Data.Confirmed || reply.Data.DeliveryID != row.ID {
			continue
		}
		if err = tx.Model(&db.P14FinalResultDelivery{}).Where("id=? AND confirmed=0", row.ID).Update("confirmed", true).Error; err != nil {
			return err
		}
	}
	return nil
}

// TickP14FinalResults shares the existing11-worker lifecycle and internal clock.
func (s *GameService) TickP14FinalResults(ctx context.Context, now time.Time) error {
	if clock, ok := ctx.Value(constUtil.P14CrossClockContextKey{}).(func() time.Time); ok {
		now = clock()
	}
	if err := s.QueueP14FinalResults(ctx, now); err != nil {
		return err
	}
	return s.DeliverP14FinalResults(ctx)
}
