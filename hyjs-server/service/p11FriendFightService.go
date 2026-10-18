package service

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"math"
	"math/big"
	"time"

	"github.com/google/uuid"
	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

// p11FriendScopedFightAt handles the original single-request friend duel. Both
// actual rosters, computed winner, source/terminal audit and response commit in
// one transaction. The existing Arena/Duel switches default off; unknown teams
// roll back before any public result. Diagnostic preparation remains separate.
func (s *GameService) p11FriendScopedFightAt(ctx context.Context, id int64, request *pb.CSFriendFight, stamp int64, now time.Time) ([]proto.Message, error) {
	if stamp <= 0 {
		return nil, errors.New(constUtil.P11FriendScopedStampRequired)
	}
	var response *pb.SCFriendFight
	var historical *db.P11FriendFightSettlement
	err := db.GetIns().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := ServerGameClockGuardTransaction(tx); err != nil {
			return err
		}

		if _, err := p10LockProfiles(tx, id, request.GetPlayerId()); err != nil {
			return err
		}
		friends, err := p10Friends(tx, id, request.GetPlayerId())
		if err != nil {
			return err
		}
		blocked, err := p10Blocked(tx, id, request.GetPlayerId())
		if err != nil {
			return err
		}
		if id == request.GetPlayerId() || !friends || blocked {
			return errors.New(constUtil.P11FriendScopedPermission)
		}
		rawRequest, err := proto.MarshalOptions{Deterministic: true}.Marshal(request)
		if err != nil {
			return err
		}
		var previous db.P11FriendFightSettlement
		err = tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&previous, "player_id=? AND stamp=?", id, stamp).Error
		if err == nil {
			if previous.OpponentID != request.GetPlayerId() || previous.RequestHash != p6Digest(rawRequest) {
				return errors.New(constUtil.P11FriendScopedReceiptConflict)
			}
			// Current friend/block ACL and request ownership were checked under
			// ordered profiles. Verify immutable frozen source after releasing
			// those locks so concurrent retries cannot queue full-table decoding.
			historical = &previous
			return nil
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		if s.P12WantedReady {
			for _, pid := range []int64{id, request.GetPlayerId()} {
				if err := P12WantedRequireIdle(tx, pid, now); err != nil {
					return err
				}
			}
		}
		if s.P12FieldPVPReady {
			for _, pid := range []int64{id, request.GetPlayerId()} {
				if err := P12FieldPVPRequireIdle(tx, pid, now); err != nil {
					return err
				}
			}
		}
		cfg, err := p11ArenaConfig(tx)
		if err != nil {
			return err
		}
		if !cfg.Enabled || !cfg.DuelEnabled {
			return errors.New(constUtil.P11ArenaClosed)
		}
		var owner, opponent db.Profile
		if err = tx.First(&owner, id).Error; err != nil {
			return err
		}
		if err = tx.First(&opponent, request.GetPlayerId()).Error; err != nil {
			return err
		}
		_, attacker, err := s.p11ArenaProjection(&owner, tx)
		if err != nil {
			return err
		}
		_, defender, err := s.p11ArenaProjection(&opponent, tx)
		if err != nil {
			return err
		}
		defenderFirst := attacker.GetInst().GetFirstOrder() == 0 && defender.GetInst().GetFirstOrder() == 1
		if err = SetP6FirstOrder(attacker, defender, defenderFirst); err != nil {
			return err
		}
		seed, err := rand.Int(rand.Reader, big.NewInt(math.MaxInt64))
		if err != nil {
			return err
		}
		fightID := uuid.NewString()
		p6ID, err := s.CaptureBattleReplay(ctx, tx, id, constUtil.P11FriendScopedSource, fightID, model.FrozenBattleInput{Attacker: attacker, Defender: defender, BattleType: 11, Seed: seed.Int64(), MaxRound: 40, DefenderFirst: defenderFirst, AppliedProperties: map[string][]string{"source": {"public original CSFriendFight; actual locked profile roster; no costs/ranks/rewards"}}})
		if err != nil {
			return err
		}
		var record db.P6BattleRecord
		if err = tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&record, "id=?", p6ID).Error; err != nil {
			return err
		}
		frozen, err := decodeP6Record(&record)
		if err != nil {
			return err
		}
		var terminal *model.CombatFinalHPOutcome
		if validateP6LinearPhysicalScope(frozen) == nil {
			terminal, err = s.ComputeP6ScopedSettlement(ctx, frozen)
		} else if validateP6Starter047Scope(frozen) == nil {
			terminal, err = s.ComputeP6StarterSettlement(ctx, frozen)
		} else if validateP6OwnedPartnerScope(frozen) == nil {
			terminal, err = s.ComputeP6OwnedPartnerSettlement(ctx, frozen)
		} else {
			terminal, err = s.ComputeP6LegacyPartySettlement(ctx, frozen)
			// A separately verified complete-party scope can admit a real
			// ultimate/long battle. Empty proof or unknown branches still
			// return the unsupported error inside this same transaction.
			if err != nil && err.Error() == constUtil.CombatUnsupported && validateP6ActualPartyScope(frozen) == nil {
				terminal, err = s.ComputeP6ActualPartySettlement(ctx, frozen)
			}
		}
		if err != nil && err.Error() == constUtil.CombatUnsupported {
			terminal, err = s.ComputeP11GenericMechanismSettlement(ctx, frozen)
		}
		if err != nil {
			return err
		}
		if !terminal.Outcome.Authoritative || frozen.BattleType != 11 {
			return errors.New(constUtil.P11ArenaClosed)
		}
		winnerID := request.GetPlayerId()
		if terminal.Outcome.Winner == 0 {
			winnerID = id
		}
		response = &pb.SCFriendFight{FightId: proto.String(fightID), RoomId: proto.String(constUtil.P11FriendScopedRoom), RandSeed: proto.Int64(frozen.Seed), PlayerList: []*pb.BattlePlayer{proto.Clone(frozen.Attacker).(*pb.BattlePlayer), proto.Clone(frozen.Defender).(*pb.BattlePlayer)}, WinPlayerId: proto.Int64(winnerID), MaxRoundNum: proto.Int32(frozen.MaxRound)}
		body, err := proto.MarshalOptions{Deterministic: true}.Marshal(response)
		if err != nil {
			return err
		}
		final, err := json.Marshal(terminal)
		if err != nil {
			return err
		}
		if err = p11StoreScopedFriendP6(tx, &record, terminal.Outcome, now); err != nil {
			return err
		}
		row := db.P11FriendFightSettlement{PlayerID: id, Stamp: stamp, OpponentID: request.GetPlayerId(), FightID: fightID, P6RecordID: p6ID, RequestHash: p6Digest(rawRequest), ScopeProof: terminal.Outcome.CapabilityHash, Response: body, ResponseHash: p6Digest(body), Terminal: final, TerminalHash: p6Digest(final), CreatedAt: now.UnixMilli()}
		return tx.Create(&row).Error
	})
	if err != nil {
		return nil, err
	}
	if historical != nil {
		response, err = p11ReadScopedFriendReceipt(db.GetIns().WithContext(ctx), historical)
		if err != nil {
			return nil, err
		}
	}
	return []proto.Message{response}, nil
}

// Store the new opt-in authoritative computation only in its own captured source.
// Existing generic/diagnostic records are never recomputed or promoted here.
func p11StoreScopedFriendP6(tx *gorm.DB, record *db.P6BattleRecord, outcome *model.CombatOutcome, now time.Time) error {
	if record.Status != constUtil.P6BattleCaptured || record.Source != constUtil.P11FriendScopedSource || !outcome.Authoritative || !((constUtil.P6LinearPhysicalNativeProof != "" && outcome.CapabilityHash == constUtil.P6LinearPhysicalNativeProof) || (constUtil.P6StarterNativeProof != "" && outcome.CapabilityHash == constUtil.P6StarterNativeProof) || (constUtil.P6OwnedPartnerNativeProof != "" && outcome.CapabilityHash == constUtil.P6OwnedPartnerNativeProof) || (constUtil.P6LegacyPartyNativeProof != "" && outcome.CapabilityHash == constUtil.P6LegacyPartyNativeProof) || (constUtil.P6ActualPartyNativeProof != "" && outcome.CapabilityHash == constUtil.P6ActualPartyNativeProof) || outcome.CapabilityHash == constUtil.P11GenericFriendCompatibilityProof) {
		return errors.New(constUtil.P6ReplayConflict)
	}
	operations, err := proto.MarshalOptions{Deterministic: true}.Marshal(outcome.Operates)
	if err != nil {
		return err
	}
	body, err := json.Marshal(outcome)
	if err != nil {
		return err
	}
	result := p6PlaybackResult(outcome.Winner)
	change := map[string]any{"status": constUtil.P6BattleComputed, "operates": operations, "outcome": body, "outcome_hash": p6Digest(body), "client_result": result, "server_result": result, "replay_hash": p6Digest(append(append([]byte(nil), operations...), byte(result))), "authoritative": true, "finished_at": now.UnixMilli()}
	saved := tx.Model(&db.P6BattleRecord{}).Where("id=? AND status=?", record.ID, constUtil.P6BattleCaptured).Updates(change)
	if saved.Error != nil {
		return saved.Error
	}
	if saved.RowsAffected != 1 {
		return errors.New(constUtil.P6ReplayConflict)
	}
	return nil
}

// Read the accepted immutable receipt without current roster/table recomputation.
// The caller has already rechecked live owner/friend/blacklist permissions.
func p11ReadScopedFriendReceipt(tx *gorm.DB, row *db.P11FriendFightSettlement) (*pb.SCFriendFight, error) {
	reject := func() (*pb.SCFriendFight, error) { return nil, errors.New(constUtil.P11FriendScopedReceiptConflict) }
	if row == nil || p6Digest(row.Response) != row.ResponseHash || p6Digest(row.Terminal) != row.TerminalHash {
		return reject()
	}
	var record db.P6BattleRecord
	if err := tx.First(&record, "id=?", row.P6RecordID).Error; err != nil {
		return nil, err
	}
	input, err := decodeP6Record(&record)
	if err != nil {
		return nil, err
	}
	var terminal model.CombatFinalHPOutcome
	response := &pb.SCFriendFight{}
	if json.Unmarshal(row.Terminal, &terminal) != nil || proto.Unmarshal(row.Response, response) != nil || terminal.Outcome == nil {
		return reject()
	}
	body, err := json.Marshal(terminal.Outcome)
	if err != nil {
		return nil, err
	}
	operations, err := proto.MarshalOptions{Deterministic: true}.Marshal(terminal.Outcome.Operates)
	if err != nil {
		return nil, err
	}
	closure := *input
	closure.Tables = nil
	source, err := json.Marshal(&closure)
	if err != nil {
		return nil, err
	}
	expectedWinner := row.PlayerID
	if terminal.Outcome.Winner != 0 {
		expectedWinner = row.OpponentID
	}
	if !(p6KnownScopedSettlementProof(input, row.ScopeProof) || p6KnownStarterSettlementProof(input, row.ScopeProof) || p6KnownOwnedPartnerSettlementProof(input, row.ScopeProof) || p6KnownLegacyPartySettlementProof(input, row.ScopeProof) || p6KnownActualPartySettlementProof(input, row.ScopeProof) || p11KnownGenericMechanismProof(input, row.ScopeProof)) || record.PlayerID != row.PlayerID || record.Source != constUtil.P11FriendScopedSource || record.SourceFight != row.FightID || record.Status != constUtil.P6BattleComputed || !record.Authoritative || !terminal.Outcome.Authoritative || row.ScopeProof != terminal.Outcome.CapabilityHash || record.OutcomeHash != p6Digest(body) || p6Digest(record.Outcome) != record.OutcomeHash || p6Digest(operations) != p6Digest(record.Operates) || record.ReplayHash != p6Digest(append(append([]byte(nil), operations...), byte(record.ServerResult))) || record.ClientResult != record.ServerResult || record.ServerResult != p6PlaybackResult(terminal.Outcome.Winner) || terminal.CalculationSourceSHA256 != p6Digest(source) || terminal.ConfigurationHash != record.ConfigurationHash || terminal.EngineVersion != record.EngineVersion || terminal.ClientVersion != record.ClientVersion || input.BattleType != 11 || input.Attacker.GetPlayerId() != row.PlayerID || input.Defender.GetPlayerId() != row.OpponentID {
		return reject()
	}
	if response.GetFightId() != row.FightID || response.GetWinPlayerId() != expectedWinner || response.GetRandSeed() != input.Seed || response.GetMaxRoundNum() != input.MaxRound || response.GetRoomId() != constUtil.P11FriendScopedRoom || len(response.PlayerList) != 2 || !proto.Equal(response.PlayerList[0], input.Attacker) || !proto.Equal(response.PlayerList[1], input.Defender) {
		return reject()
	}
	if p11KnownGenericMechanismProof(input, row.ScopeProof) {
		err = validateP11GenericMechanismTerminal(input, &terminal)
	} else if p6KnownActualPartySettlementProof(input, row.ScopeProof) {
		err = validateP6ActualPartyScope(input)
		if err == nil {
			err = validateP6ActualPartyTerminal(input, &terminal)
		}
	} else if p6KnownLegacyPartySettlementProof(input, row.ScopeProof) {
		err = validateP6LegacyPartyScope(input)
		if err == nil {
			err = validateP6LegacyPartyTerminal(input, &terminal)
		}
	} else if p6KnownOwnedPartnerSettlementProof(input, row.ScopeProof) {
		err = validateP6OwnedPartnerTerminal(input, &terminal)
	} else if p6KnownStarterSettlementProof(input, row.ScopeProof) {
		err = validateP6Starter047Terminal(input, &terminal)
	} else {
		err = validateP6LinearPhysicalTerminal(input, &terminal)
	}
	if err != nil {
		return nil, err
	}
	return response, nil
}
