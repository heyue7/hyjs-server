package service

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"strconv"
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

// InitializeP11ArenaFight publishes the independent, default-off rank-only
// policy before requests can consume energy or change any actual rank.
func (s *GameService) InitializeP11ArenaFight(ctx context.Context) error {
	tx := db.GetIns().WithContext(ctx)
	if err := tx.AutoMigrate(&db.P11ArenaFightPolicy{}, &db.P11ArenaFightSettlement{}); err != nil {
		return err
	}
	policy := db.P11ArenaFightPolicy{ID: constUtil.P11ArenaFightPolicyID, Energy: constUtil.P11ArenaEnergy, Source: constUtil.P11ArenaFightPolicySource}
	if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&policy).Error; err != nil {
		return err
	}
	return s.InitializeP11ArenaPrizes(ctx)
}

// p11ArenaScopedFightAt completes original modern23101 in one transaction.
// Profiles use the same ordered locks as friend/social operations; the shared
// arena policy serializes membership/rank transitions after those profile locks.
func (s *GameService) p11ArenaScopedFightAt(ctx context.Context, id int64, request *pb.CSPvpFightResult, stamp int64, now time.Time) ([]proto.Message, error) {
	if stamp <= 0 {
		return nil, errors.New(constUtil.P11ArenaFightStampRequired)
	}
	var response *pb.SCPvpFightResult
	var historical *db.P11ArenaFightSettlement
	err := db.GetIns().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := ServerGameClockGuardTransaction(tx); err != nil {
			return err
		}

		virtual := p11ArenaNPCID(request.GetPlayerId())
		lockTarget := request.GetPlayerId()
		if virtual {
			lockTarget = id
		}
		if _, err := p10LockProfiles(tx, id, lockTarget); err != nil {
			return err
		}
		if id == request.GetPlayerId() {
			return errors.New(constUtil.P11ArenaFightRefreshRequired)
		}
		requestRaw, err := proto.MarshalOptions{Deterministic: true}.Marshal(request)
		if err != nil {
			return err
		}
		requestHash := p6Digest(requestRaw)
		var previous db.P11ArenaFightSettlement
		err = tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&previous, "player_id=? AND stamp=?", id, stamp).Error
		if err == nil {
			if previous.OpponentID != request.GetPlayerId() || previous.RequestHash != requestHash {
				return errors.New(constUtil.P11ArenaFightReceiptConflict)
			}
			// The accepted row and its P6 source are immutable. Copy ownership
			// under profile locks, then release before decoding all frozen tables;
			// eight retries must not queue expensive read-only verification here.
			historical = &previous
			return nil
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		if s.P12WantedReady {
			for _, pid := range slices.Compact([]int64{id, lockTarget}) {
				if err := P12WantedRequireIdle(tx, pid, now); err != nil {
					return err
				}
			}
		}
		if s.P12FieldPVPReady {
			for _, pid := range slices.Compact([]int64{id, lockTarget}) {
				if err := P12FieldPVPRequireIdle(tx, pid, now); err != nil {
					return err
				}
			}
		}
		// A paid accepted receipt owns its frozen historical result. Blocking
		// prevents a new challenge but cannot charge again or erase that result.
		blocked, err := p10Blocked(tx, id, request.GetPlayerId())
		if err != nil {
			return err
		}
		if blocked {
			return errors.New(constUtil.P11ArenaFightRefreshRequired)
		}
		if err = lockProfilePeriods(tx); err != nil {
			return err
		}
		cfg, err := p11ArenaConfig(tx)
		if err != nil {
			return err
		}
		var policy db.P11ArenaFightPolicy
		if err = tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&policy, constUtil.P11ArenaFightPolicyID).Error; err != nil {
			return err
		}
		if !cfg.Enabled || !policy.Enabled || policy.Energy != constUtil.P11ArenaEnergy || policy.Source != constUtil.P11ArenaFightPolicySource {
			return errors.New(constUtil.P11ArenaClosed)
		}
		if err = s.p11SyncArenaMembers(tx, cfg, now); err != nil {
			return err
		}
		var ownerProfile, peerProfile db.Profile
		if err = tx.First(&ownerProfile, id).Error; err != nil {
			return err
		}
		if !virtual {
			if err = tx.First(&peerProfile, request.GetPlayerId()).Error; err != nil {
				return err
			}
		}
		owner, game, err := decodeProfileForCalendarUpdate(tx, &ownerProfile)
		if err != nil {
			return err
		}
		var peer *pb.Player
		if virtual {
			peer, _, err = s.p11ArenaNPCState(request.GetPlayerId() - constUtil.P11ArenaNPCIDBase)
		} else {
			peer, _, err = decodeProfile(&peerProfile)
		}
		if err != nil {
			return err
		}
		minimum, err := s.p10Setting("PVPLevel")
		if err != nil {
			return err
		}
		if owner.GetLevel() < minimum || !virtual && peer.GetLevel() < minimum {
			return errors.New(constUtil.P11ArenaFightRefreshRequired)
		}
		if owner.GetEnergy() < policy.Energy {
			return errors.New(constUtil.P11ArenaFightEnergyRequired)
		}
		var ownMember, defMember db.P11ArenaMember
		if err = tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&ownMember, "player_id=? AND active=?", id, true).Error; err != nil {
			return err
		}
		if virtual {
			rank, e := p11ArenaNPCSlot(tx, cfg, request.GetPlayerId())
			if e != nil {
				return e
			}
			projected, e := s.p11ArenaNPCMember(rank)
			if e != nil {
				return e
			}
			defMember = *projected
		} else if err = tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&defMember, "player_id=? AND active=?", request.GetPlayerId(), true).Error; err != nil {
			return err
		}
		var match db.P11ArenaMatch
		if err = tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("viewer_id=? AND opponent_id=? AND expires_at>?", id, request.GetPlayerId(), now.UnixMilli()).Order("created_at DESC, id DESC").First(&match).Error; err != nil {
			return errors.New(constUtil.P11ArenaFightRefreshRequired)
		}
		configHash, err := s.p11ArenaConfigurationHash()
		if err != nil {
			return err
		}
		if match.ConfigurationHash != configHash || match.CapabilityHash != P6CapabilityHash() || match.SnapshotHash != p11ArenaSnapshotHash(match) || match.ViewerRank != ownMember.Rank || match.OpponentRank != defMember.Rank || ownMember.Rank <= 0 || defMember.Rank <= 0 || ownMember.Rank == defMember.Rank {
			return errors.New(constUtil.P11ArenaFightRefreshRequired)
		}
		attacker, defender := &pb.BattlePlayer{}, &pb.BattlePlayer{}
		if proto.Unmarshal(match.Attacker, attacker) != nil || proto.Unmarshal(match.Defender, defender) != nil || attacker.GetPlayerId() != id || defender.GetPlayerId() != request.GetPlayerId() {
			return errors.New(constUtil.P11ArenaFightReceiptConflict)
		}
		// The original client selects a player, not the owner's avatar GUID.
		// Any nonempty owned formation frozen by matching is valid, including
		// a bench avatar; the accepted snapshot and P6 retain actor ownership.
		if len(attacker.Formation) == 0 || len(defender.Formation) == 0 {
			return errors.New(constUtil.P11ArenaFightRefreshRequired)
		}
		if err = p11ArenaGuidePrepare(ctx, owner, attacker, defender, false); err != nil {
			return err
		}
		fightID := uuid.NewString()
		rankBestBefore := p11ArenaRankRewardBoundary(ownMember)
		policyHash := p6Digest([]byte(policy.Source))
		input := model.FrozenBattleInput{Attacker: attacker, Defender: defender, BattleType: 2, Seed: match.Seed, MaxRound: 40, DefenderFirst: false, AppliedProperties: map[string][]string{"source": {p11ArenaGuideSourceLabel(ctx)}, "arenaMatch": {match.ID, match.SnapshotHash}, "arenaRequest": {requestHash}, "arenaPolicy": {policyHash}, "arenaBeforeRanks": {strconv.FormatInt(ownMember.Rank, 10), strconv.FormatInt(defMember.Rank, 10)}}}
		input.AppliedProperties["arenaRankReward"] = []string{constUtil.P11ArenaRankRewardSource, strconv.FormatInt(rankBestBefore, 10)}
		if virtual {
			input.AppliedProperties["arenaVirtualSlot"] = []string{constUtil.P11ArenaNPCSource, strconv.FormatInt(defMember.Rank, 10), defMember.SnapshotHash}
		}
		recordID, err := s.CaptureBattleReplay(ctx, tx, id, constUtil.P11ArenaFightSource, fightID, input)
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
		// Select an independently certified structural scope before computing.
		// The old one-actor path and its immutable receipts keep their proof.
		var terminal *model.CombatFinalHPOutcome
		if validateP6Arena047Scope(frozen) == nil {
			terminal, err = s.ComputeP6ArenaSettlement(ctx, frozen)
		} else if validateP6ActualArenaScope(frozen) == nil {
			terminal, err = s.ComputeP6ActualArenaSettlement(ctx, frozen)
		} else {
			terminal, err = s.ComputeP11GenericMechanismSettlement(ctx, frozen)
		}
		if err != nil {
			return err
		}
		if !terminal.Outcome.Authoritative {
			return errors.New(constUtil.P11ArenaClosed)
		}
		dailyAdmission, err := s.PrepareP11ArenaDailyMutation(ctx, tx, now)
		if err != nil {
			return err
		}
		oldOwn, oldDef := ownMember.Rank, defMember.Rank
		newOwn, newDef := oldOwn, oldDef
		if terminal.Outcome.Winner == 0 && oldOwn > oldDef {
			newOwn, newDef = oldDef, oldOwn
		}
		// Unique positive positions exchange through an uncommitted negative slot.
		// Other arena writers hold the same policy lock and cannot observe a half swap.
		if newOwn != oldOwn {
			moved := tx.Model(&db.P11ArenaMember{}).Where("player_id=? AND position=?", id, oldOwn).Update("position", -oldOwn)
			if moved.Error != nil {
				return moved.Error
			}
			if moved.RowsAffected != 1 {
				return errors.New(constUtil.P11ArenaFightRefreshRequired)
			}
			if !virtual {
				if err = tx.Model(&defMember).Update("position", newDef).Error; err != nil {
					return err
				}
			}
			if err = tx.Model(&ownMember).Update("position", newOwn).Error; err != nil {
				return err
			}
		}
		ownMember.Rank, defMember.Rank = newOwn, newDef
		winnerID := request.GetPlayerId()
		winnerMember := &defMember
		if terminal.Outcome.Winner == 0 {
			winnerID = id
			winnerMember = &ownMember
		}
		if !p11ArenaNPCID(winnerID) && (winnerMember.BestWonRank == 0 || winnerMember.Rank < winnerMember.BestWonRank) {
			if err = tx.Model(winnerMember).Update("best_won_rank", winnerMember.Rank).Error; err != nil {
				return err
			}
		}
		// The original single-fight energy2 cost follows validated real computation;
		// any capture/rank/profile/receipt failure rolls back the complete transaction.
		if err = p11ArenaGuidePrepare(ctx, owner, nil, nil, true); err != nil {
			return err
		}
		owner.Energy = proto.Int32(owner.GetEnergy() - policy.Energy)
		if err = saveProfile(tx, &ownerProfile, owner, game); err != nil {
			return err
		}
		myrank, err := p11ArenaMemberInfo(ownMember)
		if err != nil {
			return err
		}
		response = &pb.SCPvpFightResult{FightId: proto.String(fightID), RoomId: proto.String("map_arena_001"), RandSeed: proto.Int64(frozen.Seed), PlayerList: []*pb.BattlePlayer{proto.Clone(frozen.Attacker).(*pb.BattlePlayer), proto.Clone(frozen.Defender).(*pb.BattlePlayer)}, WinPlayerId: proto.Int64(winnerID), Myrank: myrank, Result: &pb.ResultObject{Simps: []*pb.SimpleItem{{Id: proto.String("energy"), Num: proto.Int32(-policy.Energy)}}}, MaxRoundNum: proto.Int32(40)}
		body, err := proto.MarshalOptions{Deterministic: true}.Marshal(response)
		if err != nil {
			return err
		}
		final, err := json.Marshal(terminal)
		if err != nil {
			return err
		}
		if err = p11StoreArenaFightP6(tx, &record, terminal.Outcome, now); err != nil {
			return err
		}
		row := db.P11ArenaFightSettlement{PlayerID: id, Stamp: stamp, OpponentID: request.GetPlayerId(), FightID: fightID, MatchID: match.ID, MatchHash: match.SnapshotHash, P6RecordID: recordID, RequestHash: requestHash, ScopeProof: terminal.Outcome.CapabilityHash, PolicyHash: policyHash, WinnerID: winnerID, AttackerOldRank: oldOwn, DefenderOldRank: oldDef, AttackerNewRank: newOwn, DefenderNewRank: newDef, EnergySpent: policy.Energy, Response: body, ResponseHash: p6Digest(body), Terminal: final, TerminalHash: p6Digest(final), CreatedAt: now.UnixMilli()}
		if err = tx.Create(&row).Error; err != nil {
			return err
		}
		if err := s.ApplyP11ArenaFightPrize(ctx, tx, &row, &ownerProfile, owner, game, response, now); err != nil {
			return err
		}
		return s.RecordP11ArenaDailyChallenge(ctx, tx, &row, dailyAdmission)
	})
	if err != nil {
		return nil, err
	}
	if historical != nil {
		response, err = p11ReadArenaFightReceipt(db.GetIns().WithContext(ctx), historical)
		if err != nil {
			return nil, err
		}
	}
	// Original AddResultObj may apply the old negative delta again on a retry.
	// The subsequent original SCPlayer supplies the current authoritative balance.
	player, _, err := s.resourceProfile(ctx, id)
	if err != nil {
		return nil, err
	}
	return []proto.Message{response, &pb.SCPlayer{Player: player}}, nil
}

func p11StoreArenaFightP6(tx *gorm.DB, record *db.P6BattleRecord, outcome *model.CombatOutcome, now time.Time) error {
	input, err := decodeP6Record(record)
	if err != nil {
		return err
	}
	known := outcome != nil && (p6KnownArenaSettlementProof(input, outcome.CapabilityHash) || p6KnownActualArenaSettlementProof(input, outcome.CapabilityHash) || p11KnownGenericMechanismProof(input, outcome.CapabilityHash))
	if record.Status != constUtil.P6BattleCaptured || record.Source != constUtil.P11ArenaFightSource || !known || !outcome.Authoritative {
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
	saved := tx.Model(&db.P6BattleRecord{}).Where("id=? AND status=?", record.ID, constUtil.P6BattleCaptured).Updates(map[string]any{"status": constUtil.P6BattleComputed, "operates": operations, "outcome": body, "outcome_hash": p6Digest(body), "client_result": result, "server_result": result, "replay_hash": p6Digest(append(append([]byte(nil), operations...), byte(result))), "authoritative": true, "finished_at": now.UnixMilli()})
	if saved.Error != nil {
		return saved.Error
	}
	if saved.RowsAffected != 1 {
		return errors.New(constUtil.P6ReplayConflict)
	}
	return nil
}

// p11ReadArenaFightReceipt verifies immutable accepted facts without current
// roster/rank/config recomputation. Current requester ownership checks are above; new challenges additionally recheck blocking.
func p11ReadArenaFightReceipt(tx *gorm.DB, row *db.P11ArenaFightSettlement) (*pb.SCPvpFightResult, error) {
	reject := func() (*pb.SCPvpFightResult, error) { return nil, errors.New(constUtil.P11ArenaFightReceiptConflict) }
	if row == nil || p6Digest(row.Response) != row.ResponseHash || p6Digest(row.Terminal) != row.TerminalHash || row.EnergySpent != constUtil.P11ArenaEnergy || row.PolicyHash != p6Digest([]byte(constUtil.P11ArenaFightPolicySource)) {
		return reject()
	}
	var record db.P6BattleRecord
	// Accepted records are immutable; title qualification also uses this pure read.
	if err := tx.First(&record, "id=?", row.P6RecordID).Error; err != nil {
		return nil, err
	}
	input, err := decodeP6Record(&record)
	if err != nil {
		return nil, err
	}
	var terminal model.CombatFinalHPOutcome
	response := &pb.SCPvpFightResult{}
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
	winner := row.PlayerID
	if terminal.Outcome.Winner != 0 {
		winner = row.OpponentID
	}
	own, def := row.AttackerOldRank, row.DefenderOldRank
	if own <= 0 || def <= 0 || own == def {
		return reject()
	}
	if terminal.Outcome.Winner == 0 && own > def {
		own, def = def, own
	}
	meta := input.AppliedProperties
	if !slices.Equal(meta["arenaMatch"], []string{row.MatchID, row.MatchHash}) || !slices.Equal(meta["arenaRequest"], []string{row.RequestHash}) || !slices.Equal(meta["arenaPolicy"], []string{row.PolicyHash}) || !slices.Equal(meta["arenaBeforeRanks"], []string{strconv.FormatInt(row.AttackerOldRank, 10), strconv.FormatInt(row.DefenderOldRank, 10)}) {
		return reject()
	}
	if p11ArenaNPCID(row.OpponentID) {
		if row.OpponentID-constUtil.P11ArenaNPCIDBase != row.DefenderOldRank || len(meta["arenaVirtualSlot"]) != 3 || meta["arenaVirtualSlot"][0] != constUtil.P11ArenaNPCSource || meta["arenaVirtualSlot"][1] != strconv.FormatInt(row.DefenderOldRank, 10) {
			return reject()
		}
	}
	knownScope := p6KnownArenaSettlementProof(input, row.ScopeProof) || p6KnownActualArenaSettlementProof(input, row.ScopeProof) || p11KnownGenericMechanismProof(input, row.ScopeProof)
	if !knownScope || record.PlayerID != row.PlayerID || record.Source != constUtil.P11ArenaFightSource || record.SourceFight != row.FightID || record.Status != constUtil.P6BattleComputed || !record.Authoritative || !terminal.Outcome.Authoritative || row.ScopeProof != terminal.Outcome.CapabilityHash || record.OutcomeHash != p6Digest(body) || p6Digest(record.Outcome) != record.OutcomeHash || p6Digest(operations) != p6Digest(record.Operates) || record.ReplayHash != p6Digest(append(append([]byte(nil), operations...), byte(record.ServerResult))) || record.ClientResult != record.ServerResult || record.ServerResult != p6PlaybackResult(terminal.Outcome.Winner) || terminal.CalculationSourceSHA256 != p6Digest(source) || terminal.ConfigurationHash != record.ConfigurationHash || terminal.EngineVersion != record.EngineVersion || terminal.ClientVersion != record.ClientVersion || input.Attacker.GetPlayerId() != row.PlayerID || input.Defender.GetPlayerId() != row.OpponentID || row.WinnerID != winner || row.AttackerNewRank != own || row.DefenderNewRank != def {
		return reject()
	}
	if response.GetFightId() != row.FightID || response.GetWinPlayerId() != winner || response.GetRandSeed() != input.Seed || response.GetMaxRoundNum() != 40 || response.GetRoomId() != "map_arena_001" || len(response.PlayerList) != 2 || !proto.Equal(response.PlayerList[0], input.Attacker) || !proto.Equal(response.PlayerList[1], input.Defender) || response.GetMyrank().GetPvpRank() != own || response.GetMyrank().GetInfo().GetPlayerId() != row.PlayerID || len(response.GetResult().GetSimps()) < 1 || len(response.GetResult().GetWars()) != 0 || len(response.GetResult().GetEquips()) != 0 || len(response.GetResult().GetTreas()) != 0 {
		return reject()
	}
	cost := response.GetResult().GetSimps()[0]
	if cost.GetId() != "energy" || cost.GetNum() != -constUtil.P11ArenaEnergy {
		return reject()
	}
	if err = ValidateP11ArenaFightPrizeReceipt(tx, row, response); err != nil {
		return nil, err
	}
	if p11KnownGenericMechanismProof(input, row.ScopeProof) {
		err = validateP11GenericMechanismTerminal(input, &terminal)
	} else if p6KnownActualArenaSettlementProof(input, row.ScopeProof) {
		if err = validateP6ActualArenaScope(input); err == nil {
			err = validateP6ActualPartyTerminal(input, &terminal)
		}
	} else {
		err = validateP6Starter047Terminal(input, &terminal)
	}
	if err != nil {
		return nil, err
	}
	return response, nil
}

// P11ArenaVerifiedBestWonRank returns only a ranking witnessed by this domain's
// accepted real winner fact. Initial or manually populated rank fields alone
// never unlock a title. The call only reads; it never migrates or computes.
func P11ArenaVerifiedBestWonRank(tx *gorm.DB, id int64) (int64, string, error) {
	if !gameSchemaHasTable(tx, &db.P11ArenaMember{}) || !gameSchemaHasTable(tx, &db.P11ArenaFightSettlement{}) {
		return 0, "", nil
	}
	var member db.P11ArenaMember
	if err := tx.First(&member, "player_id=?", id).Error; errors.Is(err, gorm.ErrRecordNotFound) {
		return 0, "", nil
	} else if err != nil {
		return 0, "", err
	}
	if member.BestWonRank <= 0 {
		return 0, "", nil
	}
	var row db.P11ArenaFightSettlement
	err := tx.Where("winner_id=? AND ((player_id=? AND attacker_new_rank=?) OR (opponent_id=? AND defender_new_rank=?))", id, id, member.BestWonRank, id, member.BestWonRank).Order("created_at ASC, player_id ASC, stamp ASC").First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return 0, "", nil
	}
	if err != nil {
		return 0, "", err
	}
	if _, err = p11ReadArenaFightReceipt(tx, &row); err != nil {
		return 0, "", err
	}
	proof := p6Digest([]byte(row.P6RecordID + ":" + row.ResponseHash + ":" + row.TerminalHash + ":" + row.ScopeProof + ":" + row.PolicyHash))
	return member.BestWonRank, proof, nil
}
