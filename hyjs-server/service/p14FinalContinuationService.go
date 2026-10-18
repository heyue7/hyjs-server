package service

import (
	"bytes"
	"compress/gzip"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
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

// The local fresh-upload rule consumes precisely this game's source rows. It
// never silently resets or carries HP/AP, or reuses Game1 as a future input.
type p14ContinuationProvenance struct {
	SeriesID           string                   `json:"seriesId"`
	ManifestHash       string                   `json:"manifestHash"`
	GameIndex          int32                    `json:"gameIndex"`
	OfferHash          string                   `json:"offerHash"`
	Rules              string                   `json:"rules"`
	PreviousResultHash string                   `json:"previousResultHash"`
	FormationOffer     db.P14PerGameOffer       `json:"formationOffer"`
	SealHashes         []string                 `json:"sealHashes"`
	Sources            []db.P14PerGameFormation `json:"sources"`
}

func p14FinalContinuationOfferHash(row *db.P14FinalContinuationOffer) string {
	copy := *row
	copy.SourceHash = ""
	raw, _ := json.Marshal(copy)
	return p6Digest(raw)
}
func p14FinalContinuationHash(row *db.P14FinalContinuationResult) string {
	copy := *row
	copy.SourceHash = ""
	raw, _ := json.Marshal(copy)
	return p6Digest(raw)
}
func p14FinalContinuationPolicyValid(policy *db.P14FinalContinuationPolicy, manifest *p14FinalSeriesManifest) bool {
	return policy != nil && manifest != nil && policy.ID == 1 && policy.Enabled && policy.ScopeProof != "" && policy.ScopeProof == constUtil.P6KFCanonicalNativeProof && policy.Source == constUtil.P14FinalContinuationSource && strings.TrimSpace(policy.Version) != "" && len(policy.Version) <= 128 && policy.PublishedAt > 0 && policy.PublishedAt <= manifest.FormationOffer.StageStart && policy.WrapperRule == constUtil.P6KFJointViewerCompatibility && policy.WrapperManifestHash == constUtil.P14FinalSettlementWrapperManifest && policy.WrapperDLLHash == constUtil.P14FinalSettlementWrapperDLL
}

// InitializeP14FinalContinuation adds independent, default-off game2..5 tables.
// Earlier157/161/165/166 rows and their rule versions remain byte-identical.
func (s *GameService) InitializeP14FinalContinuation(ctx context.Context) error {
	tx := db.GetIns().WithContext(ctx)
	if err := tx.AutoMigrate(&db.P14FinalContinuationPolicy{}, &db.P14FinalContinuationOffer{}, &db.P14FinalContinuationInput{}, &db.P14FinalContinuationResult{}, &db.P14FinalContinuationReceipt{}, &db.P14FinalContinuationDelivery{}); err != nil {
		return err
	}
	return tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&db.P14FinalContinuationPolicy{ID: 1, ScopeProof: constUtil.P6KFCanonicalNativeProof, Source: constUtil.P14FinalContinuationSource, WrapperRule: constUtil.P6KFJointViewerCompatibility, WrapperManifestHash: constUtil.P14FinalSettlementWrapperManifest, WrapperDLLHash: constUtil.P14FinalSettlementWrapperDLL}).Error
}

// Freeze the new fresh-upload operational rule independently of old Game1 policy.
func p14FinalContinuationOfferTX(tx *gorm.DB, series *db.P14FinalSeries, manifest *p14FinalSeriesManifest, gate *db.P14FinalContinuationPolicy) (*db.P14FinalContinuationOffer, error) {
	var offer db.P14FinalContinuationOffer
	err := tx.First(&offer, "series_id=?", series.ID).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		if !p14FinalContinuationPolicyValid(gate, manifest) {
			return nil, errors.New(constUtil.P14FinalContinuationPending)
		}
		policy, err := json.Marshal(gate)
		if err != nil {
			return nil, err
		}
		offer = db.P14FinalContinuationOffer{SeriesID: series.ID, SeriesManifestHash: series.ManifestHash, Policy: policy, Rules: constUtil.P14FinalContinuationRules}
		offer.SourceHash = p14FinalContinuationOfferHash(&offer)
		if err = tx.Create(&offer).Error; err != nil {
			return nil, err
		}
	} else if err != nil {
		return nil, err
	}
	var policy db.P14FinalContinuationPolicy
	if offer.SeriesManifestHash != series.ManifestHash || offer.Rules != constUtil.P14FinalContinuationRules || offer.SourceHash != p14FinalContinuationOfferHash(&offer) || p14PromotionRankDecode(offer.Policy, &policy) != nil || !p14FinalContinuationPolicyValid(&policy, manifest) {
		return nil, errors.New(constUtil.P14FinalContinuationCorrupt)
	}
	return &offer, nil
}

// Every accepted record retains its full old input and terminal result. Reading
// it validates receipts; it never calls the current engine to rerun an old game.
func p14ReadFinalContinuationTX(tx *gorm.DB, row *db.P14FinalContinuationResult, slot *db.P14FinalSeriesSlot, manifest *p14FinalSeriesManifest) (*model.CombatFinalHPOutcome, error) {
	reject := func() (*model.CombatFinalHPOutcome, error) {
		return nil, errors.New(constUtil.P14FinalContinuationCorrupt)
	}
	if row == nil || slot == nil || (row.GameIndex < 2 || row.GameIndex > 5) || row.SeriesID != slot.SeriesID || row.GameIndex != slot.GameIndex || row.InputID != slot.InputID || row.InputHash != slot.InputHash || row.ProvenanceHash != slot.ProvenanceHash || row.SourceHash != p14FinalContinuationHash(row) || row.TerminalHash != p6Digest(row.Terminal) {
		return reject()
	}
	var offer db.P14FinalContinuationOffer
	if err := tx.First(&offer, "series_id=?", row.SeriesID).Error; err != nil {
		return nil, err
	}
	if offer.SourceHash != row.OfferHash || offer.SourceHash != p14FinalContinuationOfferHash(&offer) || offer.Rules != constUtil.P14FinalContinuationRules {
		return reject()
	}
	var frozenPolicy db.P14FinalContinuationPolicy
	if p14PromotionRankDecode(offer.Policy, &frozenPolicy) != nil || !p14FinalContinuationPolicyValid(&frozenPolicy, manifest) {
		return reject()
	}
	expectedCertificate, err := json.Marshal(row)
	if err != nil || !bytes.Equal(expectedCertificate, slot.Certificate) {
		return reject()
	}
	var encoded db.P14FinalContinuationResult
	if slot.State != constUtil.P14FinalSeriesSlotCertified || slot.CertificateHash != p6Digest(slot.Certificate) || p14PromotionRankDecode(slot.Certificate, &encoded) != nil || encoded.SourceHash != row.SourceHash {
		return reject()
	}
	var terminal model.CombatFinalHPOutcome
	if p14PromotionRankDecode(row.Terminal, &terminal) != nil || terminal.Outcome == nil || !terminal.Outcome.Authoritative || terminal.Outcome.CapabilityHash != row.ScopeProof {
		return reject()
	}
	var record db.P6BattleRecord
	if err := tx.First(&record, "id=?", row.P6RecordID).Error; err != nil {
		return nil, err
	}
	frozen, err := decodeP6Record(&record)
	if err != nil {
		return nil, err
	}
	if record.Status != constUtil.P6BattleComputed || !record.Authoritative || record.Source != constUtil.P14FinalContinuationBattleSource || record.SourceFight != row.InputID || record.PlayerID != frozen.Attacker.GetPlayerId() || record.EngineVersion != terminal.EngineVersion || record.ClientVersion != terminal.ClientVersion || record.ConfigurationHash != terminal.ConfigurationHash {
		return reject()
	}
	original := &pb.KFReplayData{}
	if proto.Unmarshal(slot.Input, original) != nil || !proto.Equal(original, frozen.KFReplay) {
		return reject()
	}
	server, player, err := P6KFCanonicalWinner(frozen, &terminal)
	if err != nil || server != row.WinnerServerID || player != row.WinnerPlayerID || terminal.Outcome.Winner != row.WinnerCamp {
		return reject()
	}
	operations, err := proto.MarshalOptions{Deterministic: true}.Marshal(terminal.Outcome.Operates)
	if err != nil {
		return nil, err
	}
	body, err := json.Marshal(terminal.Outcome)
	if err != nil {
		return nil, err
	}
	result := p6PlaybackResult(row.WinnerCamp)
	if !bytes.Equal(record.Operates, operations) || !bytes.Equal(record.Outcome, body) || record.OutcomeHash != p6Digest(body) || record.ClientResult != result || record.ServerResult != result || record.ReplayHash != p6Digest(append(append([]byte{}, operations...), byte(result))) {
		return reject()
	}
	return &terminal, nil
}

// Read every prior accepted local record or authenticated remote receipt. The
// returned common source-result hash is identical on both realms; local receipt
// and slot certificate hashes deliberately differ and never seed a new game.
func p14ContinuationHistoryTX(tx *gorm.DB, series *db.P14FinalSeries, manifest *p14FinalSeriesManifest, count int32) ([]int32, string, error) {
	if count < 1 || count > 5 {
		return nil, "", errors.New(constUtil.P14FinalContinuationCorrupt)
	}
	var winners []int32
	var last string
	for index := int32(1); index <= count; index++ {
		var slot db.P14FinalSeriesSlot
		if err := tx.First(&slot, "series_id=? AND game_index=?", series.ID, index).Error; err != nil {
			return nil, "", err
		}
		if slot.State != constUtil.P14FinalSeriesSlotCertified || slot.CertificateHash != p6Digest(slot.Certificate) || slot.InputHash != p6Digest(slot.Input) {
			return nil, "", errors.New(constUtil.P14FinalContinuationPending)
		}
		var terminal *model.CombatFinalHPOutcome
		var err error
		if index == 1 {
			var local db.P14FinalSettlement
			err = tx.First(&local, "series_id=? AND game_index=1", series.ID).Error
			if err == nil {
				terminal, err = p14ReadFinalSettlementTX(tx, &local, &slot, manifest)
				last = local.SourceHash
			} else if errors.Is(err, gorm.ErrRecordNotFound) {
				var remote db.P14FinalResultReceipt
				if err = tx.First(&remote, "series_id=? AND game_index=1", series.ID).Error; err == nil {
					_, terminal, err = p14ReadFinalResultReceipt(&remote)
					last = remote.SourceResultHash
					raw, _ := json.Marshal(remote)
					if !bytes.Equal(raw, slot.Certificate) {
						return nil, "", errors.New(constUtil.P14FinalContinuationCorrupt)
					}
				}
			}
		} else {
			var local db.P14FinalContinuationResult
			err = tx.First(&local, "series_id=? AND game_index=?", series.ID, index).Error
			if err == nil {
				terminal, err = p14ReadFinalContinuationTX(tx, &local, &slot, manifest)
				last = local.SourceHash
			} else if errors.Is(err, gorm.ErrRecordNotFound) {
				var remote db.P14FinalContinuationReceipt
				if err = tx.First(&remote, "series_id=? AND game_index=?", series.ID, index).Error; err == nil {
					_, terminal, err = p14ReadContinuationReceipt(&remote)
					last = remote.SourceResultHash
					raw, _ := json.Marshal(remote)
					if !bytes.Equal(raw, slot.Certificate) {
						return nil, "", errors.New(constUtil.P14FinalContinuationCorrupt)
					}
				}
			}
		}
		if err != nil {
			return nil, "", err
		}
		if terminal == nil || terminal.Outcome == nil || len(last) != 64 {
			return nil, "", errors.New(constUtil.P14FinalContinuationCorrupt)
		}
		winners = append(winners, terminal.Outcome.Winner)
	}
	if _, _, _, _, err := p14FinalSeriesProgress(winners); err != nil {
		return nil, "", err
	}
	return winners, last, nil
}

func p14ContinuationBuild(provenance *p14ContinuationProvenance, manifest *p14FinalSeriesManifest) (*db.P14FinalContinuationInput, error) {
	if provenance == nil || manifest == nil || provenance.GameIndex < 2 || provenance.GameIndex > 5 || len(provenance.Sources) != 2 || len(provenance.PreviousResultHash) != 64 || len(provenance.OfferHash) != 64 || len(provenance.SealHashes) == 0 || provenance.Rules != constUtil.P14FinalContinuationRules {
		return nil, errors.New(constUtil.P14FinalContinuationCorrupt)
	}
	var players []*pb.BattlePlayer
	for position, source := range provenance.Sources {
		server, player := manifest.Pair.LeftServerID, manifest.Pair.LeftPlayerID
		if position == 1 {
			server, player = manifest.Pair.RightServerID, manifest.Pair.RightPlayerID
		}
		if source.SeriesID != provenance.SeriesID || source.GameIndex != provenance.GameIndex || source.ServerID != server || source.PlayerID != player || source.SourceHash != p14PerGameFormationHash(&source) {
			return nil, errors.New(constUtil.P14FinalContinuationCorrupt)
		}
		actor := &pb.BattlePlayer{}
		if proto.Unmarshal(source.Battle, actor) != nil || actor.GetPlayerId() != player || int64(actor.GetServerId()) != server {
			return nil, errors.New(constUtil.P14FinalContinuationCorrupt)
		}
		players = append(players, actor)
	}
	raw, err := json.Marshal(provenance)
	if err != nil {
		return nil, err
	}
	id := p6Digest(append([]byte("p14/final-continuation/input/"), raw...))
	digest := sha256.Sum256([]byte(id))
	seed := int64(binary.BigEndian.Uint64(digest[:8]) & 0x7fffffffffffffff)
	if seed == 0 {
		seed = 1
	}
	replay := &pb.KFReplayData{WarId: proto.String(id), RandSeed: proto.Int64(seed), PlayerList: players, MaxRoundNum: proto.Int32(constUtil.P14FinalGameMaxRound)}
	encoded, err := proto.MarshalOptions{Deterministic: true}.Marshal(replay)
	if err != nil {
		return nil, err
	}
	return &db.P14FinalContinuationInput{SeriesID: provenance.SeriesID, GameIndex: provenance.GameIndex, ID: id, OfferHash: provenance.OfferHash, PreviousResultHash: provenance.PreviousResultHash, Replay: encoded, ReplayHash: p6Digest(encoded), Provenance: raw, ProvenanceHash: p6Digest(raw)}, nil
}

// Prepare both realms from the identical actual seals and preceding source result.
// A missing/offline peer stays pending. No elapsed timer supplies its formation.
func (s *GameService) p14ContinuationInputTX(ctx context.Context, tx *gorm.DB, series *db.P14FinalSeries, manifest *p14FinalSeriesManifest, offer *db.P14FinalContinuationOffer, now time.Time) (*db.P14FinalContinuationInput, []int32, error) {
	index := series.CurrentGame
	if index < 2 || index > 5 || series.State == constUtil.P14FinalSeriesComplete {
		return nil, nil, errors.New(constUtil.P14FinalContinuationPending)
	}
	winners, previous, err := p14ContinuationHistoryTX(tx, series, manifest, index-1)
	if err != nil {
		return nil, nil, err
	}
	left, right, next, state, err := p14FinalSeriesProgress(winners)
	if err != nil || state == constUtil.P14FinalSeriesComplete || next != index || left != series.LeftWins || right != series.RightWins || series.WinnerServerID != 0 || series.WinnerPlayerID != 0 {
		return nil, nil, errors.New(constUtil.P14FinalContinuationCorrupt)
	}
	var slot db.P14FinalSeriesSlot
	if err = tx.First(&slot, "series_id=? AND game_index=?", series.ID, index).Error; err != nil {
		return nil, nil, err
	}
	if slot.ScheduledAt != manifest.Slots[index-1] || slot.LockAt != slot.ScheduledAt-constUtil.P14FinalFormationLockMillis || now.UnixMilli() < slot.LockAt {
		return nil, nil, errors.New(constUtil.P14FinalContinuationPending)
	}
	players, seals, err := s.p14PerGameReadySourceTX(ctx, tx, series.ID, index, now)
	if err != nil {
		return nil, nil, err
	}
	var formationOffer db.P14PerGameOffer
	if err = tx.First(&formationOffer, "series_id=? AND game_index=?", series.ID, index).Error; err != nil {
		return nil, nil, err
	}
	provenance := &p14ContinuationProvenance{SeriesID: series.ID, ManifestHash: series.ManifestHash, GameIndex: index, OfferHash: offer.SourceHash, Rules: constUtil.P14FinalContinuationRules, PreviousResultHash: previous, FormationOffer: formationOffer, SealHashes: seals}
	fullConfig, err := s.p14ConfigurationHash()
	if err != nil {
		return nil, nil, err
	}
	for position, joint := range [][2]int64{{manifest.Pair.LeftServerID, manifest.Pair.LeftPlayerID}, {manifest.Pair.RightServerID, manifest.Pair.RightPlayerID}} {
		var source db.P14PerGameFormation
		if err = tx.First(&source, "series_id=? AND game_index=? AND server_id=? AND player_id=?", series.ID, index, joint[0], joint[1]).Error; err != nil {
			return nil, nil, err
		}
		actual := &pb.BattlePlayer{}
		if source.ConfigurationHash != fullConfig || source.EngineVersion != constUtil.P6EngineVersion || source.ClientVersion != constUtil.P6OriginalClientHash || proto.Unmarshal(source.Battle, actual) != nil || !proto.Equal(actual, players[position]) {
			return nil, nil, errors.New(constUtil.P14FinalContinuationPending)
		}
		provenance.Sources = append(provenance.Sources, source)
	}
	row, err := p14ContinuationBuild(provenance, manifest)
	if err != nil {
		return nil, nil, err
	}
	var saved db.P14FinalContinuationInput
	err = tx.First(&saved, "series_id=? AND game_index=?", series.ID, index).Error
	if err == nil {
		if series.State != constUtil.P14FinalSeriesWaitingResult || slot.State != constUtil.P14FinalSeriesSlotInputReady || slot.InputID != saved.ID || slot.InputHash != saved.ReplayHash || !bytes.Equal(slot.Input, saved.Replay) || slot.ProvenanceHash != saved.ProvenanceHash || len(slot.Certificate) != 0 || saved.ID != row.ID || !bytes.Equal(saved.Replay, row.Replay) || !bytes.Equal(saved.Provenance, row.Provenance) || saved.ReplayHash != row.ReplayHash || saved.ProvenanceHash != row.ProvenanceHash || saved.OfferHash != row.OfferHash || saved.PreviousResultHash != previous {
			return nil, nil, errors.New(constUtil.P14FinalContinuationCorrupt)
		}
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil, err
	} else {
		if slot.State != constUtil.P14FinalSeriesSlotPlanned || len(slot.Input) != 0 || len(slot.Certificate) != 0 || series.State != constUtil.P14FinalSeriesWaitingInput {
			return nil, nil, errors.New(constUtil.P14FinalContinuationCorrupt)
		}
		if err = tx.Create(row).Error; err != nil {
			return nil, nil, err
		}
		if err = tx.Model(&slot).Updates(map[string]any{"state": constUtil.P14FinalSeriesSlotInputReady, "input_id": row.ID, "input": row.Replay, "input_hash": row.ReplayHash, "provenance_hash": row.ProvenanceHash}).Error; err != nil {
			return nil, nil, err
		}
		if err = tx.Model(series).Updates(map[string]any{"state": constUtil.P14FinalSeriesWaitingResult, "updated_at": now.UnixMilli()}).Error; err != nil {
			return nil, nil, err
		}
	}
	return row, winners, nil
}

// Apply only independently verified winners. Completion explicitly identifies the
// winning joint identity and leaves unplayed slots unneeded with no certificate.
func p14ContinuationProgressTX(tx *gorm.DB, series *db.P14FinalSeries, manifest *p14FinalSeriesManifest, winners []int32, now time.Time) error {
	left, right, next, state, err := p14FinalSeriesProgress(winners)
	if err != nil {
		return err
	}
	changes := map[string]any{"left_wins": left, "right_wins": right, "current_game": next, "state": state, "updated_at": now.UnixMilli()}
	if state == constUtil.P14FinalSeriesComplete {
		server, player := manifest.Pair.LeftServerID, manifest.Pair.LeftPlayerID
		if right == constUtil.P14FinalSeriesWinsRequired {
			server, player = manifest.Pair.RightServerID, manifest.Pair.RightPlayerID
		}
		changes["winner_server_id"], changes["winner_player_id"] = server, player
		if err = tx.Model(&db.P14FinalSeriesSlot{}).Where("series_id=? AND game_index>? AND state=? AND length(certificate)=0", series.ID, len(winners), constUtil.P14FinalSeriesSlotPlanned).Update("state", constUtil.P14FinalSeriesSlotUnneeded).Error; err != nil {
			return err
		}
	}
	return tx.Model(series).Updates(changes).Error
}

// SettleP14FinalContinuation uses no client winner. Only canonical left owns a
// new P6 record; a peer receives the signed terminal through the independent route.
func (s *GameService) SettleP14FinalContinuation(ctx context.Context, seriesID string, now time.Time) error {
	return db.GetIns().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := ServerGameClockGuardTransaction(tx); err != nil {
			return err
		}

		var season db.P14SeasonPolicy
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&season, 1).Error; err != nil {
			return err
		}
		var series db.P14FinalSeries
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&series, "id=?", seriesID).Error; err != nil {
			return err
		}
		manifest, err := p14PerGameReadSeries(&series)
		if err != nil {
			return err
		}
		if s.OriginalServerID() != manifest.Pair.LeftServerID {
			return errors.New(constUtil.P14FinalContinuationPending)
		}
		if series.State == constUtil.P14FinalSeriesComplete {
			return nil
		}
		var gate db.P14FinalContinuationPolicy
		if err = tx.First(&gate, 1).Error; err != nil {
			return err
		}
		if !season.Enabled || !gate.Enabled || season.SeasonIndex != series.SeasonIndex || constUtil.P6KFCanonicalNativeProof == "" {
			return errors.New(constUtil.P14FinalContinuationPending)
		}
		offer, err := p14FinalContinuationOfferTX(tx, &series, manifest, &gate)
		if err != nil {
			return err
		}
		source, winners, err := s.p14ContinuationInputTX(ctx, tx, &series, manifest, offer, now)
		if err != nil {
			return err
		}
		index := source.GameIndex
		if now.UnixMilli() < manifest.Slots[index-1] {
			return nil
		}
		replay := &pb.KFReplayData{}
		if proto.Unmarshal(source.Replay, replay) != nil {
			return errors.New(constUtil.P14FinalContinuationCorrupt)
		}
		input, err := s.FreezeP6KFReplayRealmInput(replay, manifest.Pair.LeftServerID, manifest.Pair.LeftPlayerID)
		if err != nil {
			return err
		}
		input.AppliedProperties = map[string][]string{"p14Continuation": {series.ID, series.ManifestHash, offer.SourceHash, source.ReplayHash, source.ProvenanceHash, source.PreviousResultHash, constUtil.P14FinalSettlementWrapperManifest, constUtil.P14FinalSettlementWrapperDLL}}
		recordID, err := s.CaptureBattleReplay(ctx, tx, manifest.Pair.LeftPlayerID, constUtil.P14FinalContinuationBattleSource, source.ID, *input)
		if err != nil {
			return err
		}
		var record db.P6BattleRecord
		if err = tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&record, "id=?", recordID).Error; err != nil {
			return err
		}
		if record.Status != constUtil.P6BattleCaptured {
			return errors.New(constUtil.P14FinalContinuationCorrupt)
		}
		frozen, err := decodeP6Record(&record)
		if err != nil {
			return err
		}
		terminal, err := s.ComputeP6KFCanonicalSettlement(ctx, frozen)
		if err != nil {
			return err
		}
		server, player, err := P6KFCanonicalWinner(frozen, terminal)
		if err != nil {
			return err
		}
		winner := terminal.Outcome.Winner
		operations, err := proto.MarshalOptions{Deterministic: true}.Marshal(terminal.Outcome.Operates)
		if err != nil {
			return err
		}
		outcome, err := json.Marshal(terminal.Outcome)
		if err != nil {
			return err
		}
		result := p6PlaybackResult(winner)
		stored := tx.Model(&db.P6BattleRecord{}).Where("id=? AND status=?", record.ID, constUtil.P6BattleCaptured).Updates(map[string]any{"status": constUtil.P6BattleComputed, "operates": operations, "outcome": outcome, "outcome_hash": p6Digest(outcome), "client_result": result, "server_result": result, "replay_hash": p6Digest(append(append([]byte{}, operations...), byte(result))), "authoritative": true, "finished_at": now.UnixMilli()})
		if stored.Error != nil {
			return stored.Error
		}
		if stored.RowsAffected != 1 {
			return errors.New(constUtil.P14FinalContinuationCorrupt)
		}
		raw, err := json.Marshal(terminal)
		if err != nil {
			return err
		}
		accepted := db.P14FinalContinuationResult{SeriesID: series.ID, GameIndex: index, OfferHash: offer.SourceHash, InputID: source.ID, InputHash: source.ReplayHash, ProvenanceHash: source.ProvenanceHash, P6RecordID: record.ID, ScopeProof: terminal.Outcome.CapabilityHash, WinnerCamp: winner, WinnerServerID: server, WinnerPlayerID: player, Terminal: raw, TerminalHash: p6Digest(raw), CreatedAt: now.UnixMilli()}
		accepted.SourceHash = p14FinalContinuationHash(&accepted)
		if err = tx.Create(&accepted).Error; err != nil {
			return err
		}
		certificate, err := json.Marshal(&accepted)
		if err != nil {
			return err
		}
		if err = tx.Model(&db.P14FinalSeriesSlot{}).Where("series_id=? AND game_index=?", series.ID, index).Updates(map[string]any{"state": constUtil.P14FinalSeriesSlotCertified, "certificate": certificate, "certificate_hash": p6Digest(certificate)}).Error; err != nil {
			return err
		}
		return p14ContinuationProgressTX(tx, &series, manifest, append(winners, winner), now)
	})
}

// The original full terminal is preserved. Only frozen tables are omitted from
// transport; a peer must reconstruct the exact complete input hash using the
// same current engine/client/config before it may accept anything.
type p14ContinuationResultBody struct {
	Offer     db.P14FinalContinuationOffer  `json:"offer"`
	Result    db.P14FinalContinuationResult `json:"result"`
	Input     []byte                        `json:"input"`
	InputHash string                        `json:"inputHash"`
	Terminal  []byte                        `json:"terminal"`
}
type p14ContinuationResultBlob struct {
	Codec       string `json:"codec"`
	DecodedSize int64  `json:"decodedSize"`
	ContentHash string `json:"contentHash"`
	Data        []byte `json:"data"`
}

func p14ContinuationReceiptHash(row *db.P14FinalContinuationReceipt) string {
	copy := *row
	copy.SourceHash = ""
	raw, _ := json.Marshal(copy)
	return p6Digest(raw)
}
func p14ContinuationResultDeliveryID(request *model.P14ContinuationResultImportRequest) string {
	return p6Digest([]byte(fmt.Sprintf("p14/final-continuation/%d/%d/%d/%s", request.ProtocolVersion, request.SourceServerID, request.DestinationServerID, p6Digest(request.Snapshot))))
}
func p14ContinuationResultEncode(body *p14ContinuationResultBody) (json.RawMessage, error) {
	raw, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	if int64(len(raw)) > constUtil.P14FinalResultMaxDecodedBytes {
		return nil, errors.New(constUtil.P14FinalContinuationCorrupt)
	}
	var buffer bytes.Buffer
	zipper := gzip.NewWriter(&buffer)
	if _, err = zipper.Write(raw); err != nil {
		return nil, err
	}
	if err = zipper.Close(); err != nil {
		return nil, err
	}
	return json.Marshal(p14ContinuationResultBlob{Codec: constUtil.P14FinalResultCodec, DecodedSize: int64(len(raw)), ContentHash: p6Digest(raw), Data: buffer.Bytes()})
}
func p14ContinuationResultDecode(raw json.RawMessage) (*p14ContinuationResultBody, error) {
	var blob p14ContinuationResultBlob
	if p14PromotionRankDecode(raw, &blob) != nil || blob.Codec != constUtil.P14FinalResultCodec || blob.DecodedSize <= 0 || blob.DecodedSize > constUtil.P14FinalResultMaxDecodedBytes || len(blob.ContentHash) != 64 {
		return nil, errors.New(constUtil.P14FinalContinuationCorrupt)
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
	var body p14ContinuationResultBody
	if int64(len(decoded)) != blob.DecodedSize || p6Digest(decoded) != blob.ContentHash || p14PromotionRankDecode(decoded, &body) != nil {
		return nil, errors.New(constUtil.P14FinalContinuationCorrupt)
	}
	return &body, nil
}

// QueueP14ContinuationResults reads only an accepted local canonical result. Source0
// authority and its full P6 record must still validate before an outbox is built.
func (s *GameService) QueueP14ContinuationResults(ctx context.Context, now time.Time) error {
	return db.GetIns().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := ServerGameClockGuardTransaction(tx); err != nil {
			return err
		}

		var season db.P14SeasonPolicy
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&season, 1).Error; err != nil {
			return err
		}
		var gate db.P14FinalContinuationPolicy
		if err := tx.First(&gate, 1).Error; err != nil {
			return err
		}
		if !season.Enabled || !gate.Enabled || constUtil.P6KFCanonicalNativeProof == "" {
			return nil
		}
		var results []db.P14FinalContinuationResult
		if err := tx.Where("game_index BETWEEN 2 AND 5").Order("series_id").Find(&results).Error; err != nil {
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
			if err = tx.First(&slot, "series_id=? AND game_index=?", series.ID, result.GameIndex).Error; err != nil {
				return err
			}
			if _, err = p14ReadFinalContinuationTX(tx, &result, &slot, manifest); err != nil {
				return err
			}
			var offer db.P14FinalContinuationOffer
			if err = tx.First(&offer, "series_id=?", series.ID).Error; err != nil {
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
			body := p14ContinuationResultBody{Offer: offer, Result: result, Input: input, InputHash: record.InputHash, Terminal: result.Terminal}
			snapshot, err := p14ContinuationResultEncode(&body)
			if err != nil {
				return err
			}
			var peers []db.P14TrustedPeer
			if err = tx.Where("enabled=1 AND server_id<>?", s.OriginalServerID()).Order("server_id").Find(&peers).Error; err != nil {
				return err
			}
			for _, peer := range peers {
				var existing db.P14FinalContinuationDelivery
				err = tx.First(&existing, "series_id=? AND game_index=? AND destination_server_id=?", series.ID, result.GameIndex, peer.ServerID).Error
				if err == nil {
					if existing.SourceResultHash != result.SourceHash || existing.PayloadHash != p6Digest(existing.Payload) {
						return errors.New(constUtil.P14FinalContinuationCorrupt)
					}
					continue
				}
				if !errors.Is(err, gorm.ErrRecordNotFound) {
					return err
				}
				envelope := model.P14ContinuationResultImportRequest{ProtocolVersion: 1, SourceServerID: s.OriginalServerID(), DestinationServerID: peer.ServerID, Snapshot: snapshot}
				envelope.DeliveryID = p14ContinuationResultDeliveryID(&envelope)
				encoded, err := json.Marshal(envelope)
				if err != nil {
					return err
				}
				if len(encoded) > 1<<20 {
					return errors.New(constUtil.P14FinalContinuationCorrupt)
				}
				row := db.P14FinalContinuationDelivery{SeriesID: series.ID, GameIndex: result.GameIndex, SourceResultHash: result.SourceHash, ID: envelope.DeliveryID, DestinationServerID: peer.ServerID, Payload: encoded, PayloadHash: p6Digest(encoded), CreatedAt: now.UnixMilli()}
				if err = tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&row).Error; err != nil {
					return err
				}
			}
		}
		return nil
	})
}

// ImportP14ContinuationResult verifies a trusted origin's exact raw HMAC, genuine
// source0 joint and independently certified terminal. It performs no calculation
// and never creates a local P6BattleRecord for that external owner's PID.
func (s *GameService) ImportP14ContinuationResult(ctx context.Context, request *model.P14ContinuationResultImportRequest, raw []byte, signature string) (*model.P14ContinuationResultImportResponse, error) {
	if request == nil {
		return nil, errors.New(constUtil.P14FinalContinuationCorrupt)
	}
	now := LocalPlayNow(ctx)
	if clock, ok := ctx.Value(constUtil.P14CrossClockContextKey{}).(func() time.Time); ok {
		now = clock()
	}
	var signed model.P14ContinuationResultImportRequest
	if p14PromotionRankDecode(raw, &signed) != nil || signed.ProtocolVersion != request.ProtocolVersion || signed.DeliveryID != request.DeliveryID || signed.SourceServerID != request.SourceServerID || signed.DestinationServerID != request.DestinationServerID || !bytes.Equal(signed.Snapshot, request.Snapshot) {
		return nil, errors.New(constUtil.P14FinalContinuationCorrupt)
	}
	response := &model.P14ContinuationResultImportResponse{DeliveryID: request.DeliveryID}
	err := db.GetIns().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := ServerGameClockGuardTransaction(tx); err != nil {
			return err
		}

		var season db.P14SeasonPolicy
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&season, 1).Error; err != nil {
			return err
		}
		if request.ProtocolVersion != 1 || request.SourceServerID == s.OriginalServerID() || request.DestinationServerID != s.OriginalServerID() || request.DeliveryID != p14ContinuationResultDeliveryID(request) {
			return errors.New(constUtil.P14FinalContinuationCorrupt)
		}
		var peer db.P14TrustedPeer
		if err := tx.First(&peer, "server_id=? AND enabled=1", request.SourceServerID).Error; err != nil {
			return err
		}
		supplied, err := hex.DecodeString(signature)
		mac := hmac.New(sha256.New, peer.SigningKey)
		_, _ = mac.Write(raw)
		if len(peer.SigningKey) < 32 || err != nil || !hmac.Equal(mac.Sum(nil), supplied) {
			return errors.New(constUtil.P14FinalContinuationCorrupt)
		}
		var previous db.P14FinalContinuationReceipt
		err = tx.First(&previous, "delivery_id=?", request.DeliveryID).Error
		if err == nil {
			if previous.PayloadHash != p6Digest(raw) {
				return errors.New(constUtil.P14FinalContinuationCorrupt)
			}
			if _, _, err = p14ReadContinuationReceipt(&previous); err != nil {
				return err
			}
			response.Confirmed = true
			return nil
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		body, err := p14ContinuationResultDecode(request.Snapshot)
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
		index := body.Result.GameIndex
		if request.SourceServerID != manifest.Pair.LeftServerID || index < 2 || index > 5 || series.CurrentGame != index || series.State == constUtil.P14FinalSeriesComplete {
			return errors.New(constUtil.P14FinalContinuationCorrupt)
		}
		if body.Offer.SeriesID != series.ID || body.Offer.SeriesManifestHash != series.ManifestHash || body.Offer.Rules != constUtil.P14FinalContinuationRules || body.Offer.SourceHash != p14FinalContinuationOfferHash(&body.Offer) {
			return errors.New(constUtil.P14FinalContinuationCorrupt)
		}
		var sourcePolicy db.P14FinalContinuationPolicy
		if p14PromotionRankDecode(body.Offer.Policy, &sourcePolicy) != nil || !p14FinalContinuationPolicyValid(&sourcePolicy, manifest) {
			return errors.New(constUtil.P14FinalContinuationCorrupt)
		}
		var localPolicy db.P14FinalContinuationPolicy
		if err = tx.First(&localPolicy, 1).Error; err != nil {
			return err
		}
		localPolicy.Enabled = true
		localRaw, _ := json.Marshal(localPolicy)
		if !bytes.Equal(localRaw, body.Offer.Policy) {
			return errors.New(constUtil.P14FinalContinuationPending)
		}
		var localOffer db.P14FinalContinuationOffer
		err = tx.First(&localOffer, "series_id=?", series.ID).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			if err = tx.Create(&body.Offer).Error; err != nil {
				return err
			}
		} else if err != nil {
			return err
		} else if localOffer.SourceHash != body.Offer.SourceHash {
			return errors.New(constUtil.P14FinalContinuationCorrupt)
		}
		source, winners, err := s.p14ContinuationInputTX(ctx, tx, &series, manifest, &body.Offer, now)
		if err != nil {
			return err
		}
		var slot db.P14FinalSeriesSlot
		if err = tx.First(&slot, "series_id=? AND game_index=?", series.ID, index).Error; err != nil {
			return err
		}
		if source.ID != slot.InputID || slot.State != constUtil.P14FinalSeriesSlotInputReady || len(slot.Certificate) != 0 || body.Result.CreatedAt < slot.ScheduledAt || body.Result.CreatedAt > now.UnixMilli() || body.Result.InputID != slot.InputID || body.Result.InputHash != slot.InputHash || body.Result.ProvenanceHash != slot.ProvenanceHash || slot.InputHash != p6Digest(slot.Input) || body.Result.OfferHash != body.Offer.SourceHash {
			return errors.New(constUtil.P14FinalContinuationCorrupt)
		}
		body.Result.Terminal = body.Terminal
		if body.Result.SourceHash != p14FinalContinuationHash(&body.Result) || body.Result.TerminalHash != p6Digest(body.Terminal) {
			return errors.New(constUtil.P14FinalContinuationCorrupt)
		}
		var transported model.FrozenBattleInput
		if p14PromotionRankDecode(body.Input, &transported) != nil || transported.Tables != nil || transported.EngineVersion != constUtil.P6EngineVersion || transported.ClientVersion != constUtil.P6OriginalClientHash {
			return errors.New(constUtil.P14FinalContinuationPending)
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
			return errors.New(constUtil.P14FinalContinuationCorrupt)
		}
		var terminal model.CombatFinalHPOutcome
		if p14PromotionRankDecode(body.Terminal, &terminal) != nil {
			return errors.New(constUtil.P14FinalContinuationCorrupt)
		}
		server, player, err := P6KFCanonicalWinner(frozen, &terminal)
		if err != nil {
			return err
		}
		if body.Result.ScopeProof != terminal.Outcome.CapabilityHash || body.Result.WinnerCamp != terminal.Outcome.Winner || body.Result.WinnerServerID != server || body.Result.WinnerPlayerID != player {
			return errors.New(constUtil.P14FinalContinuationCorrupt)
		}
		stored, err := encodeP6StoredInput(frozenRaw)
		if err != nil {
			return err
		}
		resultRaw, err := json.Marshal(&body.Result)
		if err != nil {
			return err
		}
		row := db.P14FinalContinuationReceipt{SeriesID: series.ID, GameIndex: index, DeliveryID: request.DeliveryID, SourceServerID: request.SourceServerID, PayloadHash: p6Digest(raw), SourceResult: resultRaw, SourceResultHash: body.Result.SourceHash, Input: stored, InputHash: body.InputHash, Terminal: body.Terminal, TerminalHash: p6Digest(body.Terminal), OfferHash: body.Offer.SourceHash, WinnerServerID: server, WinnerPlayerID: player, ReceivedAt: now.UnixMilli()}
		row.SourceHash = p14ContinuationReceiptHash(&row)
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
		if err = p14ContinuationProgressTX(tx, &series, manifest, append(winners, terminal.Outcome.Winner), now); err != nil {
			return err
		}
		response.Confirmed = true
		return nil
	})
	return response, err
}

// p14ReadContinuationReceipt is a pure receipt verifier. Frozen remote tables
// and terminal remain available for cold reads without a local P6 owner or rerun.
func p14ReadContinuationReceipt(row *db.P14FinalContinuationReceipt) (*model.FrozenBattleInput, *model.CombatFinalHPOutcome, error) {
	reject := func() (*model.FrozenBattleInput, *model.CombatFinalHPOutcome, error) {
		return nil, nil, errors.New(constUtil.P14FinalContinuationCorrupt)
	}
	if row == nil || row.SourceHash != p14ContinuationReceiptHash(row) || row.TerminalHash != p6Digest(row.Terminal) {
		return reject()
	}
	raw, err := decodeP6StoredInput(row.Input)
	if err != nil || p6Digest(raw) != row.InputHash {
		return reject()
	}
	var input model.FrozenBattleInput
	var terminal model.CombatFinalHPOutcome
	var result db.P14FinalContinuationResult
	if p14PromotionRankDecode(raw, &input) != nil || p14PromotionRankDecode(row.Terminal, &terminal) != nil || p14PromotionRankDecode(row.SourceResult, &result) != nil {
		return reject()
	}
	result.Terminal = row.Terminal
	if result.SourceHash != row.SourceResultHash || result.SourceHash != p14FinalContinuationHash(&result) || result.SeriesID != row.SeriesID || result.GameIndex != row.GameIndex || input.KFReplay == nil || len(input.KFReplay.PlayerList) != 2 || int64(input.KFReplay.PlayerList[0].GetServerId()) != row.SourceServerID {
		return reject()
	}
	server, player, err := P6KFCanonicalWinner(&input, &terminal)
	if err != nil || row.WinnerServerID != server || row.WinnerPlayerID != player || result.WinnerServerID != server || result.WinnerPlayerID != player || terminal.Outcome.Winner != result.WinnerCamp || terminal.Outcome.CapabilityHash != result.ScopeProof {
		return reject()
	}
	return &input, &terminal, nil
}

// DeliverP14ContinuationResults fulfills already frozen outboxes even if operators
// close new forwarding. Offline peers remain pending and acquire no fake loss.
func (s *GameService) DeliverP14ContinuationResults(ctx context.Context) error {
	tx := db.GetIns().WithContext(ctx)
	var destinations []int64
	if err := tx.Model(&db.P14FinalContinuationDelivery{}).Where("confirmed=0").Distinct("destination_server_id").Order("destination_server_id").Pluck("destination_server_id", &destinations).Error; err != nil {
		return err
	}
	if len(destinations) > 32 {
		return errors.New(constUtil.P14FinalContinuationCorrupt)
	}
	for _, destination := range destinations {
		if err := ctx.Err(); err != nil {
			return err
		}
		var row db.P14FinalContinuationDelivery
		if err := tx.Where("confirmed=0 AND destination_server_id=?", destination).Order("created_at,id").First(&row).Error; err != nil {
			return err
		}
		if row.PayloadHash != p6Digest(row.Payload) {
			return errors.New(constUtil.P14FinalContinuationCorrupt)
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
		client := httpClient.HttpInner("p14-final-continuation-"+strconv.FormatInt(destination, 10)).SetTimeout(3*time.Second).AddHeader("X-HYJS-P14-Signature", P14SignPayload(peer.SigningKey, row.Payload))
		status, raw, callErr := client.PostWithContext(ctx, peer.Endpoint+constUtil.P14FinalContinuationImportPath, row.Payload)
		if err = tx.Model(&db.P14FinalContinuationDelivery{}).Where("id=? AND confirmed=0", row.ID).Updates(map[string]any{"attempt_count": gorm.Expr("attempt_count+1"), "last_attempt_at": p14PromotionRankClock(ctx).UnixMilli()}).Error; err != nil {
			return err
		}
		if callErr != nil || status != http.StatusOK {
			continue
		}
		var reply struct {
			Code int64                                     `json:"code"`
			Data model.P14ContinuationResultImportResponse `json:"data"`
		}
		if json.Unmarshal(raw, &reply) != nil || reply.Code != 0 || !reply.Data.Confirmed || reply.Data.DeliveryID != row.ID {
			continue
		}
		if err = tx.Model(&db.P14FinalContinuationDelivery{}).Where("id=? AND confirmed=0", row.ID).Update("confirmed", true).Error; err != nil {
			return err
		}
	}
	return nil
}

// TickP14ContinuationResults shares the existing11-worker lifecycle and internal clock.
func (s *GameService) TickP14ContinuationResults(ctx context.Context, now time.Time) error {
	if clock, ok := ctx.Value(constUtil.P14CrossClockContextKey{}).(func() time.Time); ok {
		now = clock()
	}
	if err := s.QueueP14ContinuationResults(ctx, now); err != nil {
		return err
	}
	return s.DeliverP14ContinuationResults(ctx)
}

// QueueP14FinalContinuations tries only each series' current genuine slot. A
// missing seal, unqualified structure, or offline origin cannot advance it.
func (s *GameService) QueueP14FinalContinuations(ctx context.Context, now time.Time) error {
	tx := db.GetIns().WithContext(ctx)
	var gate db.P14FinalContinuationPolicy
	if err := tx.First(&gate, 1).Error; err != nil {
		return err
	}
	if !gate.Enabled {
		return nil
	}
	var rows []db.P14FinalSeries
	if err := tx.Where("current_game BETWEEN 2 AND 5 AND state IN ?", []int32{constUtil.P14FinalSeriesWaitingInput, constUtil.P14FinalSeriesWaitingResult}).Order("id").Find(&rows).Error; err != nil {
		return err
	}
	for _, row := range rows {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := s.SettleP14FinalContinuation(ctx, row.ID, now); err != nil {
			switch err.Error() {
			case constUtil.P14FinalContinuationPending, constUtil.P14FinalSeriesPending, constUtil.P14PerGameClosed, constUtil.CombatUnsupported, constUtil.CombatInvalid:
				continue
			default:
				return err
			}
		}
	}
	return nil
}

// TickP14FinalContinuations reuses the existing Cross worker and internal clock.
// No new goroutine or public time/winner parameter is introduced.
func (s *GameService) TickP14FinalContinuations(ctx context.Context, now time.Time) error {
	if clock, ok := ctx.Value(constUtil.P14CrossClockContextKey{}).(func() time.Time); ok {
		now = clock()
	}
	if err := s.QueueP14FinalContinuations(ctx, now); err != nil {
		return err
	}
	return s.TickP14ContinuationResults(ctx, now)
}
