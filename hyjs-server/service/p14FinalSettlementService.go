package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
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

func p14FinalSettlementOfferHash(row *db.P14FinalSettlementOffer) string {
	copy := *row
	copy.SourceHash = ""
	raw, _ := json.Marshal(copy)
	return p6Digest(raw)
}
func p14FinalSettlementHash(row *db.P14FinalSettlement) string {
	copy := *row
	copy.SourceHash = ""
	raw, _ := json.Marshal(copy)
	return p6Digest(raw)
}
func p14FinalSettlementPolicyValid(policy *db.P14FinalSettlementPolicy, manifest *p14FinalSeriesManifest) bool {
	return policy != nil && manifest != nil && policy.ID == 1 && policy.Enabled && policy.ScopeProof != "" && policy.ScopeProof == constUtil.P6KFCanonicalNativeProof && policy.Source == constUtil.P14FinalSettlementSource && strings.TrimSpace(policy.Version) != "" && len(policy.Version) <= 128 && policy.PublishedAt > 0 && policy.PublishedAt <= manifest.FormationOffer.StageStart && policy.WrapperRule == constUtil.P6KFJointViewerCompatibility && policy.WrapperManifestHash == constUtil.P14FinalSettlementWrapperManifest && policy.WrapperDLLHash == constUtil.P14FinalSettlementWrapperDLL
}

// InitializeP14FinalSettlement adds an opt-in local-wrapper capability. It does
// not promote historical ordinary17 or original PID-only captures to authority.
func (s *GameService) InitializeP14FinalSettlement(ctx context.Context) error {
	tx := db.GetIns().WithContext(ctx)
	if err := tx.AutoMigrate(&db.P14FinalSettlementPolicy{}, &db.P14FinalSettlementOffer{}, &db.P14FinalSettlement{}); err != nil {
		return err
	}
	return tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&db.P14FinalSettlementPolicy{ID: 1, ScopeProof: constUtil.P6KFCanonicalNativeProof, Source: constUtil.P14FinalSettlementSource, WrapperRule: constUtil.P6KFJointViewerCompatibility, WrapperManifestHash: constUtil.P14FinalSettlementWrapperManifest, WrapperDLLHash: constUtil.P14FinalSettlementWrapperDLL}).Error
}

// Freeze operational policy separately from the old161 series manifest. Those
// exact old bytes and old157 provenance remain unchanged and independently readable.
func p14FinalSettlementOfferTX(tx *gorm.DB, series *db.P14FinalSeries, manifest *p14FinalSeriesManifest, gate *db.P14FinalSettlementPolicy) (*db.P14FinalSettlementOffer, error) {
	var offer db.P14FinalSettlementOffer
	err := tx.First(&offer, "series_id=?", series.ID).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		if !p14FinalSettlementPolicyValid(gate, manifest) {
			return nil, errors.New(constUtil.P14FinalSettlementPending)
		}
		policy, err := json.Marshal(gate)
		if err != nil {
			return nil, err
		}
		offer = db.P14FinalSettlementOffer{SeriesID: series.ID, SeriesManifestHash: series.ManifestHash, Policy: policy, Rules: constUtil.P14FinalSettlementRules}
		offer.SourceHash = p14FinalSettlementOfferHash(&offer)
		if err = tx.Create(&offer).Error; err != nil {
			return nil, err
		}
	} else if err != nil {
		return nil, err
	}
	var policy db.P14FinalSettlementPolicy
	if offer.SeriesManifestHash != series.ManifestHash || offer.Rules != constUtil.P14FinalSettlementRules || offer.SourceHash != p14FinalSettlementOfferHash(&offer) || p14PromotionRankDecode(offer.Policy, &policy) != nil || !p14FinalSettlementPolicyValid(&policy, manifest) {
		return nil, errors.New(constUtil.P14FinalSettlementCorrupt)
	}
	return &offer, nil
}

// Every accepted record retains its full old input and terminal result. Reading
// it validates receipts; it never calls the current engine to rerun an old game.
func p14ReadFinalSettlementTX(tx *gorm.DB, row *db.P14FinalSettlement, slot *db.P14FinalSeriesSlot, manifest *p14FinalSeriesManifest) (*model.CombatFinalHPOutcome, error) {
	reject := func() (*model.CombatFinalHPOutcome, error) {
		return nil, errors.New(constUtil.P14FinalSettlementCorrupt)
	}
	if row == nil || slot == nil || row.GameIndex != 1 || row.SeriesID != slot.SeriesID || row.GameIndex != slot.GameIndex || row.InputID != slot.InputID || row.InputHash != slot.InputHash || row.ProvenanceHash != slot.ProvenanceHash || row.SourceHash != p14FinalSettlementHash(row) || row.TerminalHash != p6Digest(row.Terminal) {
		return reject()
	}
	var offer db.P14FinalSettlementOffer
	if err := tx.First(&offer, "series_id=?", row.SeriesID).Error; err != nil {
		return nil, err
	}
	if offer.SourceHash != row.OfferHash || offer.SourceHash != p14FinalSettlementOfferHash(&offer) || offer.Rules != constUtil.P14FinalSettlementRules {
		return reject()
	}
	var frozenPolicy db.P14FinalSettlementPolicy
	if p14PromotionRankDecode(offer.Policy, &frozenPolicy) != nil || !p14FinalSettlementPolicyValid(&frozenPolicy, manifest) {
		return reject()
	}
	expectedCertificate, err := json.Marshal(row)
	if err != nil || !bytes.Equal(expectedCertificate, slot.Certificate) {
		return reject()
	}
	var encoded db.P14FinalSettlement
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
	if record.Status != constUtil.P6BattleComputed || !record.Authoritative || record.Source != constUtil.P14FinalSettlementBattleSource || record.SourceFight != row.InputID || record.PlayerID != frozen.Attacker.GetPlayerId() || record.EngineVersion != terminal.EngineVersion || record.ClientVersion != terminal.ClientVersion || record.ConfigurationHash != terminal.ConfigurationHash {
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

// Metadata versions are checked before Capture can freeze current tables. An
// old registration or audit may not silently acquire today's engine label.
func p14FinalSettlementSourceVersion(member *p14FinalSealMember, input *model.FrozenBattleInput, sourceConfiguration string) error {
	var source struct {
		EngineVersion     string `json:"engineVersion"`
		ClientVersion     string `json:"clientVersion"`
		ConfigurationHash string `json:"configurationHash"`
	}
	if member == nil || json.Unmarshal(member.Metadata, &source) != nil || source.EngineVersion != constUtil.P6EngineVersion || source.ClientVersion != input.ClientVersion || source.ConfigurationHash != sourceConfiguration {
		return errors.New(constUtil.P14FinalSettlementPending)
	}
	return nil
}

// SettleP14FinalGameOne computes one real input with the independent canonical
// KF capability. Only the canonical left realm owns the P6 record. The second
// realm receives a separately signed result in the following federation domain.
// A viewer cannot create another win. No caller supplies a winner or client result.
func (s *GameService) SettleP14FinalGameOne(ctx context.Context, seriesID string, now time.Time) error {
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
			return errors.New(constUtil.P14FinalSettlementPending)
		}
		var slot db.P14FinalSeriesSlot
		if err = tx.First(&slot, "series_id=? AND game_index=1", series.ID).Error; err != nil {
			return err
		}
		var accepted db.P14FinalSettlement
		err = tx.First(&accepted, "series_id=? AND game_index=1", series.ID).Error
		if err == nil {
			_, err = p14ReadFinalSettlementTX(tx, &accepted, &slot, manifest)
			return err
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		var gate db.P14FinalSettlementPolicy
		if err = tx.First(&gate, 1).Error; err != nil {
			return err
		}
		if !season.Enabled || !gate.Enabled || season.SeasonIndex != series.SeasonIndex || constUtil.P6KFCanonicalNativeProof == "" || now.UnixMilli() < slot.ScheduledAt {
			return errors.New(constUtil.P14FinalSettlementPending)
		}
		if series.CurrentGame != 1 || series.LeftWins != 0 || series.RightWins != 0 || series.WinnerPlayerID != 0 || series.WinnerServerID != 0 || series.State != constUtil.P14FinalSeriesWaitingResult || slot.ScheduledAt != manifest.Slots[0] || slot.LockAt != manifest.Slots[0]-constUtil.P14FinalFormationLockMillis || slot.State != constUtil.P14FinalSeriesSlotInputReady || len(slot.Certificate) != 0 || slot.InputHash != p6Digest(slot.Input) || slot.InputID != manifest.FirstInputID || slot.InputHash != manifest.FirstInputHash || slot.ProvenanceHash != manifest.FirstProvenanceHash {
			return errors.New(constUtil.P14FinalSettlementCorrupt)
		}
		var game db.P14FinalGame
		if err = tx.First(&game, "id=?", slot.InputID).Error; err != nil {
			return err
		}
		planned, _, err := p14FinalSeriesPlan(&game, &manifest.FormationOffer, &manifest.Policy, now)
		if err != nil || planned.ID != series.ID || !bytes.Equal(game.Replay, slot.Input) {
			return errors.New(constUtil.P14FinalSettlementCorrupt)
		}
		var provenance p14FinalGameProvenance
		if p14PromotionRankDecode(game.Provenance, &provenance) != nil {
			return errors.New(constUtil.P14FinalSettlementCorrupt)
		}
		replay := &pb.KFReplayData{}
		if proto.Unmarshal(slot.Input, replay) != nil || len(replay.PlayerList) != 2 || replay.PlayerList[0].GetPlayerId() != manifest.Pair.LeftPlayerID || int64(replay.PlayerList[0].GetServerId()) != manifest.Pair.LeftServerID || replay.PlayerList[1].GetPlayerId() != manifest.Pair.RightPlayerID || int64(replay.PlayerList[1].GetServerId()) != manifest.Pair.RightServerID {
			return errors.New(constUtil.P14FinalSettlementCorrupt)
		}
		input, err := s.FreezeP6KFReplayRealmInput(replay, manifest.Pair.LeftServerID, manifest.Pair.LeftPlayerID)
		if err != nil {
			return err
		}
		// P14 source metadata hashes the complete GameTables. P6 independently
		// freezes its mathematical subset; those two hash domains stay distinct.
		sourceConfiguration, err := s.p14ConfigurationHash()
		if err != nil {
			return err
		}
		for _, member := range []*p14FinalSealMember{&provenance.Left, &provenance.Right} {
			if err = p14FinalSettlementSourceVersion(member, input, sourceConfiguration); err != nil {
				return err
			}
		}
		offer, err := p14FinalSettlementOfferTX(tx, &series, manifest, &gate)
		if err != nil {
			return err
		}
		input.AppliedProperties = map[string][]string{"p14FinalCanonical": {series.ID, series.ManifestHash, offer.SourceHash, slot.InputHash, slot.ProvenanceHash, constUtil.P14FinalSettlementWrapperManifest, constUtil.P14FinalSettlementWrapperDLL}}
		recordID, err := s.CaptureBattleReplay(ctx, tx, manifest.Pair.LeftPlayerID, constUtil.P14FinalSettlementBattleSource, slot.InputID, *input)
		if err != nil {
			return err
		}
		var record db.P6BattleRecord
		if err = tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&record, "id=?", recordID).Error; err != nil {
			return err
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
		if winner < 0 || winner > 1 || (winner == 0 && (server != manifest.Pair.LeftServerID || player != manifest.Pair.LeftPlayerID)) || (winner == 1 && (server != manifest.Pair.RightServerID || player != manifest.Pair.RightPlayerID)) {
			return errors.New(constUtil.P14FinalSettlementCorrupt)
		}
		if record.Status != constUtil.P6BattleCaptured {
			return errors.New(constUtil.P14FinalSettlementCorrupt)
		}
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
			return errors.New(constUtil.P14FinalSettlementCorrupt)
		}
		raw, err := json.Marshal(terminal)
		if err != nil {
			return err
		}
		accepted = db.P14FinalSettlement{SeriesID: series.ID, GameIndex: 1, OfferHash: offer.SourceHash, InputID: slot.InputID, InputHash: slot.InputHash, ProvenanceHash: slot.ProvenanceHash, P6RecordID: record.ID, ScopeProof: terminal.Outcome.CapabilityHash, WinnerCamp: winner, WinnerServerID: server, WinnerPlayerID: player, Terminal: raw, TerminalHash: p6Digest(raw), CreatedAt: now.UnixMilli()}
		accepted.SourceHash = p14FinalSettlementHash(&accepted)
		if err = tx.Create(&accepted).Error; err != nil {
			return err
		}
		certificate, err := json.Marshal(&accepted)
		if err != nil {
			return err
		}
		if err = tx.Model(&slot).Updates(map[string]any{"state": constUtil.P14FinalSeriesSlotCertified, "certificate": certificate, "certificate_hash": p6Digest(certificate)}).Error; err != nil {
			return err
		}
		left, right, next, state, err := p14FinalSeriesProgress([]int32{winner})
		if err != nil {
			return err
		}
		return tx.Model(&series).Updates(map[string]any{"left_wins": left, "right_wins": right, "current_game": next, "state": state, "updated_at": now.UnixMilli()}).Error
	})
}

// QueueP14FinalSettlements uses independent per-series transactions. An unknown
// actor shape remains pending without a receipt, loss, timer forfeit or prize.
func (s *GameService) QueueP14FinalSettlements(ctx context.Context, now time.Time) error {
	tx := db.GetIns().WithContext(ctx)
	var gate db.P14FinalSettlementPolicy
	if err := tx.First(&gate, 1).Error; err != nil {
		return err
	}
	if !gate.Enabled || constUtil.P6KFCanonicalNativeProof == "" {
		return nil
	}
	var rows []db.P14FinalSeries
	if err := tx.Where("current_game=1 AND state=?", constUtil.P14FinalSeriesWaitingResult).Order("id").Find(&rows).Error; err != nil {
		return err
	}
	for _, row := range rows {
		if err := ctx.Err(); err != nil {
			return err
		}
		err := s.SettleP14FinalGameOne(ctx, row.ID, now)
		if err != nil && err.Error() != constUtil.P14FinalSettlementPending && err.Error() != constUtil.CombatUnsupported && err.Error() != constUtil.CombatInvalid {
			return err
		}
	}
	return nil
}

// TickP14FinalSettlements reuses the existing P14 worker and private test clock.
// Normal request bodies and headers have no clock injection route.
func (s *GameService) TickP14FinalSettlements(ctx context.Context, now time.Time) error {
	if clock, ok := ctx.Value(constUtil.P14CrossClockContextKey{}).(func() time.Time); ok {
		now = clock()
	}
	return s.QueueP14FinalSettlements(ctx, now)
}
