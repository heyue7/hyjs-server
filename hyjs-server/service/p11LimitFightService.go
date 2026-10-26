package service

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"math"
	"math/big"
	"strings"
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

type p11LimitFightRules struct {
	Policy   db.P11LimitFightPolicy `json:"policy"`
	Energy   int32                  `json:"energy"`
	WinScore int32                  `json:"winScore"`
}

// InitializeP11LimitFight seeds no period, opponent, score or accepted outcome.
func (s *GameService) InitializeP11LimitFight(ctx context.Context) error {
	win, err := s.p10Setting("limitPvpFightSuccess")
	if err != nil || win != 5 {
		return errors.New(constUtil.P11LimitFightConflict)
	}
	tx := db.GetIns().WithContext(ctx)
	if err = tx.AutoMigrate(&db.P11LimitFightPolicy{}, &db.P11LimitFightOffer{}, &db.P11LimitFightMatch{}, &db.P11LimitFightSettlement{}); err != nil {
		return err
	}
	return tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&db.P11LimitFightPolicy{ID: 1, Version: "local-limit-type20-v1", Source: constUtil.P11LimitFightPolicySource, MatchMillis: 300000}).Error
}

func p11LimitFightMatchHash(m *db.P11LimitFightMatch) string {
	copy := *m
	copy.SourceHash = ""
	raw, err := json.Marshal(copy)
	if err != nil {
		return ""
	}
	return p6Digest(raw)
}

// p11LimitFightOffer freezes the operator version separately from the existing
// period hash. Older legitimate period reward snapshots retain their exact bytes.
func (s *GameService) p11LimitFightOffer(tx *gorm.DB, period *db.P11LimitPeriod, now time.Time) (*db.P11LimitFightOffer, *p11LimitFightRules, error) {
	var cfg db.P11LimitFightPolicy
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&cfg, 1).Error; err != nil {
		return nil, nil, err
	}
	if !cfg.Enabled {
		return nil, nil, nil
	}
	var offer db.P11LimitFightOffer
	err := tx.First(&offer, "version=?", cfg.Version).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		if cfg.ID != 1 || strings.TrimSpace(cfg.Version) == "" || len(cfg.Version) > 64 || cfg.Source != constUtil.P11LimitFightPolicySource || cfg.PublishedAt <= 0 || cfg.PublishedAt > cfg.StartAt || cfg.StartAt < period.StartsAt || cfg.EndAt > period.EndsAt || cfg.EndAt <= cfg.StartAt || cfg.EndAt-cfg.StartAt > int64(31*24*time.Hour/time.Millisecond) || cfg.MatchMillis < 1000 || cfg.MatchMillis > 300000 || now.UnixMilli() < cfg.StartAt || now.UnixMilli() >= cfg.EndAt {
			return nil, nil, errors.New(constUtil.P11LimitFightClosed)
		}
		win, e := s.p10Setting("limitPvpFightSuccess")
		if e != nil || win != 5 {
			return nil, nil, errors.New(constUtil.P11LimitFightConflict)
		}
		raw, e := json.Marshal(p11LimitFightRules{Policy: cfg, Energy: 2, WinScore: win})
		if e != nil {
			return nil, nil, e
		}
		offer = db.P11LimitFightOffer{Version: cfg.Version, Snapshot: string(raw), SourceHash: p6Digest(raw), CapturedAt: now.UnixMilli()}
		if e = tx.Create(&offer).Error; e != nil {
			return nil, nil, e
		}
	} else if err != nil {
		return nil, nil, err
	}
	var rules p11LimitFightRules
	if p6Digest([]byte(offer.Snapshot)) != offer.SourceHash || json.Unmarshal([]byte(offer.Snapshot), &rules) != nil || rules.Policy.Version != offer.Version || rules.Policy.Source != constUtil.P11LimitFightPolicySource || rules.Energy != 2 || rules.WinScore != 5 || rules.Policy.StartAt < period.StartsAt || rules.Policy.EndAt > period.EndsAt || now.UnixMilli() < rules.Policy.StartAt || now.UnixMilli() >= rules.Policy.EndAt || offer.CapturedAt < rules.Policy.StartAt || offer.CapturedAt >= rules.Policy.EndAt {
		return nil, nil, errors.New(constUtil.P11LimitFightConflict)
	}
	return &offer, &rules, nil
}

// prepareP11LimitMatches runs only after a fresh original FindMatch query in its
// existing profile/period transaction. Other players are MVCC reads, never locks
// acquired after the policy. A later fight locks both real profiles in ID order.
func (s *GameService) prepareP11LimitMatches(ctx context.Context, tx *gorm.DB, id int64, period *db.P11LimitPeriod, own *pb.LimitPvpRankPlayerInfo, peers []*pb.LimitPvpRankPlayerInfo, now time.Time) error {
	offer, rules, err := s.p11LimitFightOffer(tx, period, now)
	if err != nil || offer == nil {
		return err
	}
	var owner db.Profile
	if err = tx.First(&owner, id).Error; err != nil {
		return err
	}
	_, attacker, err := s.p11ArenaProjection(&owner, tx)
	if err != nil {
		return err
	}
	configuration, err := s.p11ArenaConfigurationHash()
	if err != nil {
		return err
	}
	for _, peer := range peers {
		if peer.GetInfo().GetPlayerId() == id {
			continue
		}
		var other db.Profile
		if err = tx.First(&other, peer.GetInfo().GetPlayerId()).Error; err != nil {
			return err
		}
		_, defender, e := s.p11ArenaProjection(&other, tx)
		if e != nil {
			return e
		}
		a := proto.Clone(attacker).(*pb.BattlePlayer)
		if e = SetP6FirstOrder(a, defender, false); e != nil {
			return e
		}
		ar, e := proto.MarshalOptions{Deterministic: true}.Marshal(a)
		if e != nil {
			return e
		}
		dr, e := proto.MarshalOptions{Deterministic: true}.Marshal(defender)
		if e != nil {
			return e
		}
		seed, e := rand.Int(rand.Reader, big.NewInt(math.MaxInt64))
		if e != nil {
			return e
		}
		row := db.P11LimitFightMatch{ID: uuid.NewString(), PeriodID: period.ID, PlayerID: id, OpponentID: other.Id, OwnScore: own.GetPvpIntegral(), PeerScore: peer.GetPvpIntegral(), OfferVersion: offer.Version, OfferHash: offer.SourceHash, PeriodHash: period.RulesHash, Attacker: ar, Defender: dr, Seed: seed.Int64(), ConfigurationHash: configuration, CapabilityHash: P6CapabilityHash(), CreatedAt: now.UnixMilli(), ExpiresAt: min(now.UnixMilli()+rules.Policy.MatchMillis, rules.Policy.EndAt)}
		row.SourceHash = p11LimitFightMatchHash(&row)
		if e = tx.Create(&row).Error; e != nil {
			return e
		}
	}
	return nil
}

// p11LimitFrozenInput keeps original mode20, complete Inst/actors and Max40.
// It is also the exact diagnostic boundary for independent original sampling.
func p11LimitFrozenInput(match *db.P11LimitFightMatch, requestHash string) (*model.FrozenBattleInput, error) {
	if match == nil || match.SourceHash != p11LimitFightMatchHash(match) {
		return nil, errors.New(constUtil.P11LimitFightConflict)
	}
	attacker, defender := &pb.BattlePlayer{}, &pb.BattlePlayer{}
	if proto.Unmarshal(match.Attacker, attacker) != nil || proto.Unmarshal(match.Defender, defender) != nil || attacker.GetPlayerId() != match.PlayerID || defender.GetPlayerId() != match.OpponentID || attacker.GetInst() == nil || defender.GetInst() == nil || attacker.GetInst().GetFirstOrder() != 1 || defender.GetInst().GetFirstOrder() != 0 {
		return nil, errors.New(constUtil.P11LimitFightConflict)
	}
	return &model.FrozenBattleInput{Attacker: attacker, Defender: defender, BattleType: 20, MaxRound: 40, Seed: match.Seed, DefenderFirst: false, AppliedProperties: map[string][]string{"source": {constUtil.P11LimitFightSource}, "limitPeriod": {match.PeriodID, match.PeriodHash}, "limitMatch": {match.ID, match.SourceHash}, "limitOffer": {match.OfferVersion, match.OfferHash}, "limitRequest": {requestHash}}}, nil
}

// p11LimitFightAt requires the real query snapshot and independent mode20
// terminal. A missing certificate rolls back captured replay, energy, daily used,
// cumulative points, receipt and P1 facts together; no other battle proof applies.
func (s *GameService) p11LimitFightAt(ctx context.Context, id int64, request *pb.CSLimitPvpFightResult, stamp int64, now time.Time) ([]proto.Message, error) {
	if stamp <= 0 || id == request.GetPlayerId() {
		return nil, errors.New(constUtil.P11LimitFightRefresh)
	}
	var response *pb.SCLimitPvpFightResult
	err := db.GetIns().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := ServerGameClockGuardTransaction(tx); err != nil {
			return err
		}

		if _, e := p10LockProfiles(tx, id, request.GetPlayerId()); e != nil {
			return e
		}
		raw, e := proto.MarshalOptions{Deterministic: true}.Marshal(request)
		if e != nil {
			return e
		}
		var old db.P11LimitFightSettlement
		e = tx.First(&old, "player_id=? AND stamp=?", id, stamp).Error
		if e == nil {
			if old.RequestHash != p6Digest(raw) || old.OpponentID != request.GetPlayerId() {
				return errors.New(constUtil.P11LimitFightConflict)
			}
			response, e = p11ReadLimitFightReceipt(tx, &old)
			return e
		}
		if !errors.Is(e, gorm.ErrRecordNotFound) {
			return e
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
		period, periodRules, e := s.p11LimitPeriod(tx, now)
		if e != nil {
			return e
		}
		if _, e = s.PrepareP11LimitDailyMutation(ctx, tx, period, now); e != nil {
			return e
		}
		offer, rules, e := s.p11LimitFightOffer(tx, period, now)
		if e != nil {
			return e
		}
		if offer == nil {
			return errors.New(constUtil.P11LimitFightClosed)
		}
		var owner, other db.Profile
		if e = tx.First(&owner, id).Error; e != nil {
			return e
		}
		if e = tx.First(&other, request.GetPlayerId()).Error; e != nil {
			return e
		}
		player, game, e := decodeProfileForCalendarUpdate(tx, &owner)
		if e != nil {
			return e
		}
		peer, _, e := decodeProfileForCalendarUpdate(tx, &other)
		if e != nil {
			return e
		}
		minLevel, e := s.p10Setting("PVPLevel")
		if e != nil {
			return e
		}
		if player.GetLevel() < minLevel || peer.GetLevel() < minLevel || player.GetEnergy() < rules.Energy {
			return errors.New(constUtil.P11LimitFightRefresh)
		}
		blocked, e := p10Blocked(tx, id, other.Id)
		if e != nil {
			return e
		}
		if blocked {
			return errors.New(constUtil.P11LimitFightRefresh)
		}
		var member, opponent db.P11LimitMember
		if e = tx.First(&member, "period_id=? AND player_id=?", period.ID, id).Error; e != nil {
			return e
		}
		if e = tx.First(&opponent, "period_id=? AND player_id=?", period.ID, other.Id).Error; e != nil {
			return e
		}
		scoreDiff := member.Score - opponent.Score
		if scoreDiff < 0 {
			scoreDiff = -scoreDiff
		}
		levelDiff := int64(player.GetLevel()) - int64(peer.GetLevel())
		if levelDiff < 0 {
			levelDiff = -levelDiff
		}
		if member.Score < 0 || opponent.Score < 0 || scoreDiff > int64(periodRules.MatchScore) || levelDiff > int64(periodRules.MatchLevel) {
			return errors.New(constUtil.P11LimitFightRefresh)
		}
		var daily db.P11LimitDaily
		if e = tx.First(&daily, "period_id=? AND player_id=? AND day=?", period.ID, id, dailyCopyDay(now)).Error; e != nil {
			return errors.New(constUtil.P11LimitFightRefresh)
		}
		if daily.Used < 0 || daily.Bought < 0 || daily.Bought > periodRules.BuyMax || daily.Used >= periodRules.FreeTimes+daily.Bought {
			return errors.New(constUtil.P11LimitFightRefresh)
		}
		var match db.P11LimitFightMatch
		if e = tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("period_id=? AND player_id=? AND opponent_id=? AND created_at<=? AND expires_at>?", period.ID, id, other.Id, now.UnixMilli(), now.UnixMilli()).Order("created_at DESC,id DESC").First(&match).Error; e != nil {
			return errors.New(constUtil.P11LimitFightRefresh)
		}
		configuration, e := s.p11ArenaConfigurationHash()
		if e != nil {
			return e
		}
		if match.OwnScore != member.Score || match.PeerScore != opponent.Score || match.PeriodHash != period.RulesHash || match.OfferVersion != offer.Version || match.OfferHash != offer.SourceHash || match.ConfigurationHash != configuration || match.CapabilityHash != P6CapabilityHash() {
			return errors.New(constUtil.P11LimitFightRefresh)
		}
		input, e := p11LimitFrozenInput(&match, p6Digest(raw))
		if e != nil {
			return e
		}
		_, actualA, e := s.p11ArenaProjection(&owner, tx)
		if e != nil {
			return e
		}
		_, actualD, e := s.p11ArenaProjection(&other, tx)
		if e != nil {
			return e
		}
		if e = SetP6FirstOrder(actualA, actualD, false); e != nil {
			return e
		}
		if !proto.Equal(actualA, input.Attacker) || !proto.Equal(actualD, input.Defender) {
			return errors.New(constUtil.P11LimitFightRefresh)
		}
		var used int64
		if e = tx.Model(&db.P11LimitFightSettlement{}).Where("match_id=?", match.ID).Count(&used).Error; e != nil {
			return e
		}
		if used != 0 {
			return errors.New(constUtil.P11LimitFightRefresh)
		}
		fightID := uuid.NewString()
		recordID, e := s.CaptureBattleReplay(ctx, tx, id, constUtil.P11LimitFightSource, fightID, *input)
		if e != nil {
			return e
		}
		var record db.P6BattleRecord
		if e = tx.First(&record, "id=?", recordID).Error; e != nil {
			return e
		}
		frozen, e := decodeP6Record(&record)
		if e != nil {
			return e
		}
		terminal, e := s.ComputeP6LimitSettlement(ctx, frozen)
		if e != nil {
			return e
		}
		if terminal == nil || terminal.Outcome == nil || !terminal.Outcome.Authoritative || !p6KnownLimitSettlementProof(frozen, terminal.Outcome.CapabilityHash) || p11SettlementScope(frozen, terminal.Outcome.CapabilityHash, validateP6LimitScope) != nil || validateP6LimitTerminal(frozen, terminal) != nil {
			return errors.New(constUtil.P11LimitFightClosed)
		}
		// Linearize the accepted result after computation. A cutoff can close
		// an old ranking first; changed point eligibility requires a real new query.
		admitted, e := s.PrepareP11LimitDailyMutation(ctx, tx, period, now)
		if e != nil {
			return e
		}
		var actualMember, actualOpponent db.P11LimitMember
		if e = tx.First(&actualMember, "period_id=? AND player_id=?", period.ID, id).Error; e != nil {
			return e
		}
		if e = tx.First(&actualOpponent, "period_id=? AND player_id=?", period.ID, other.Id).Error; e != nil {
			return e
		}
		if dailyCopyDay(admitted) != daily.Day || actualMember.Score != member.Score || actualOpponent.Score != opponent.Score {
			return errors.New(constUtil.P11LimitFightRefresh)
		}
		// Only an independently accepted challenger win produces the original5.
		// Defender score/loss deductions are unknown and explicitly local0.
		beforeScore, beforeUsed := member.Score, daily.Used
		delta := int32(0)
		winner := other.Id
		if terminal.Outcome.Winner == 0 {
			delta = rules.WinScore
			winner = id
		} else if terminal.Outcome.Winner != 1 {
			return errors.New(constUtil.P11LimitFightConflict)
		}
		if member.Score > math.MaxInt64-int64(delta) {
			return errors.New(constUtil.P11LimitFightConflict)
		}
		member.Score += int64(delta)
		member.UpdatedAt = now.UnixMilli()
		daily.Used++
		player.Energy = proto.Int32(player.GetEnergy() - rules.Energy)
		if e = tx.Save(&member).Error; e != nil {
			return e
		}
		if e = tx.Save(&daily).Error; e != nil {
			return e
		}
		if e = saveProfile(tx, &owner, player, game); e != nil {
			return e
		}
		mine, _, e := s.p11LimitList(tx, period.ID, id, periodRules, false)
		if e != nil {
			return e
		}
		response = &pb.SCLimitPvpFightResult{FightId: proto.String(fightID), RoomId: proto.String("map_arena_001"), RandSeed: proto.Int64(frozen.Seed), PlayerList: []*pb.BattlePlayer{proto.Clone(frozen.Attacker).(*pb.BattlePlayer), proto.Clone(frozen.Defender).(*pb.BattlePlayer)}, WinPlayerId: proto.Int64(winner), Myrank: mine, Intergral: proto.Int32(delta), MaxRoundNum: proto.Int32(40)}
		body, e := proto.MarshalOptions{Deterministic: true}.Marshal(response)
		if e != nil {
			return e
		}
		final, e := json.Marshal(terminal)
		if e != nil {
			return e
		}
		if e = p11StoreLimitFightP6(tx, &record, frozen, terminal, now); e != nil {
			return e
		}
		row := db.P11LimitFightSettlement{PlayerID: id, Stamp: stamp, OpponentID: other.Id, PeriodID: period.ID, RequestHash: p6Digest(raw), MatchID: match.ID, MatchHash: match.SourceHash, FightID: fightID, P6RecordID: recordID, ScopeProof: terminal.Outcome.CapabilityHash, EnergySpent: rules.Energy, ScoreDelta: delta, BeforeScore: beforeScore, AfterScore: member.Score, Day: daily.Day, BeforeUsed: beforeUsed, AfterUsed: daily.Used, Response: body, ResponseHash: p6Digest(body), Terminal: final, TerminalHash: p6Digest(final), CreatedAt: now.UnixMilli()}
		if e = tx.Create(&row).Error; e != nil {
			return e
		}
		if e = s.RecordP11LimitDailyChallenge(ctx, tx, &row, admitted); e != nil {
			return e
		}
		if s.P1Ready {
			if e = s.RecordP1Event(tx, id, "p11-limit-accepted:"+fightID, "limit_arena_accepted", 1, now); e != nil {
				return e
			}
			if delta > 0 {
				return s.RecordP1Event(tx, id, "p11-limit-win:"+fightID, "limit_arena_verified_win", 1, now)
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	current, _, err := s.resourceProfile(ctx, id)
	if err != nil {
		return nil, err
	}
	return []proto.Message{response, &pb.SCPlayer{Player: current}}, nil
}

func p11StoreLimitFightP6(tx *gorm.DB, record *db.P6BattleRecord, input *model.FrozenBattleInput, terminal *model.CombatFinalHPOutcome, now time.Time) error {
	if record.Status != constUtil.P6BattleCaptured || record.Source != constUtil.P11LimitFightSource || !p6KnownLimitSettlementProof(input, terminal.Outcome.CapabilityHash) {
		return errors.New(constUtil.P11LimitFightConflict)
	}
	operations, err := proto.MarshalOptions{Deterministic: true}.Marshal(terminal.Outcome.Operates)
	if err != nil {
		return err
	}
	body, err := json.Marshal(terminal.Outcome)
	if err != nil {
		return err
	}
	result := p6PlaybackResult(terminal.Outcome.Winner)
	saved := tx.Model(&db.P6BattleRecord{}).Where("id=? AND status=?", record.ID, constUtil.P6BattleCaptured).Updates(map[string]any{"status": constUtil.P6BattleComputed, "operates": operations, "outcome": body, "outcome_hash": p6Digest(body), "client_result": result, "server_result": result, "replay_hash": p6Digest(append(append([]byte(nil), operations...), byte(result))), "authoritative": true, "finished_at": now.UnixMilli()})
	if saved.Error != nil {
		return saved.Error
	}
	if saved.RowsAffected != 1 {
		return errors.New(constUtil.P11LimitFightConflict)
	}
	return nil
}

func p11ReadLimitFightReceipt(tx *gorm.DB, row *db.P11LimitFightSettlement) (*pb.SCLimitPvpFightResult, error) {
	if p6Digest(row.Response) != row.ResponseHash || p6Digest(row.Terminal) != row.TerminalHash || row.EnergySpent != 2 || row.AfterUsed != row.BeforeUsed+1 || row.AfterScore != row.BeforeScore+int64(row.ScoreDelta) {
		return nil, errors.New(constUtil.P11LimitFightConflict)
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
	response := &pb.SCLimitPvpFightResult{}
	if record.Status != constUtil.P6BattleComputed || !record.Authoritative || record.Source != constUtil.P11LimitFightSource || !p6KnownLimitSettlementProof(input, row.ScopeProof) || p11SettlementScope(input, row.ScopeProof, validateP6LimitScope) != nil || json.Unmarshal(row.Terminal, &terminal) != nil || validateP6LimitTerminal(input, &terminal) != nil || proto.Unmarshal(row.Response, response) != nil || response.GetFightId() != row.FightID || response.GetIntergral() != row.ScoreDelta {
		return nil, errors.New(constUtil.P11LimitFightConflict)
	}
	return response, nil
}
