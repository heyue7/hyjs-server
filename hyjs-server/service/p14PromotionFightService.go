package service

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"math/big"
	"strconv"
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

type p14PromotionFightRules struct {
	MatchCost    int64 `json:"matchCost"`
	MaxRound     int64 `json:"maxRound"`
	WinScore     int64 `json:"winScore"`
	LossScore    int64 `json:"lossScore"`
	MinimumScore int64 `json:"minimumScore"`
}

func p14PromotionFightDecode(raw, hash string) (*p14PromotionFightRules, error) {
	var rules p14PromotionFightRules
	decoder := json.NewDecoder(strings.NewReader(raw))
	decoder.DisallowUnknownFields()
	if p6Digest([]byte(raw)) != hash || decoder.Decode(&rules) != nil || rules.MatchCost != 1 || rules.MaxRound != 40 || rules.WinScore != 15 || rules.LossScore != 0 || rules.MinimumScore != 1 {
		return nil, errors.New(constUtil.P14PromotionFightCorrupt)
	}
	var tail any
	if decoder.Decode(&tail) != io.EOF {
		return nil, errors.New(constUtil.P14PromotionFightCorrupt)
	}
	return &rules, nil
}

func p14PromotionFightOfferHash(row *db.P14PromotionFightOffer) string {
	copy := *row
	copy.SourceHash = ""
	raw, _ := json.Marshal(copy)
	return p6Digest(raw)
}

func p14PromotionFightHash(row *db.P14PromotionFight) string {
	copy := *row
	copy.SourceHash = ""
	raw, _ := json.Marshal(copy)
	return p6Digest(raw)
}

// InitializeP14PromotionFight creates only closed policy and immutable business
// ledgers. Neither qualification nor battle authority follows from this DDL.
func (s *GameService) InitializeP14PromotionFight(ctx context.Context) error {
	tx := db.GetIns().WithContext(ctx)
	if err := tx.AutoMigrate(&db.P14PromotionFightPolicy{}, &db.P14PromotionFightOffer{}, &db.P14PromotionFight{}); err != nil {
		return err
	}
	return tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&db.P14PromotionFightPolicy{ID: 1, Source: constUtil.P14PromotionFightSource}).Error
}

// p14PromotionFightTotals leaves the old noon grant state untouched. Because
// the original daily grant20 equals cap20, each real noon resets availability
// to20; only certified spends bound to that grant reduce the current balance.
func p14PromotionFightTotals(state *db.P14PromotionState, rows []db.P14PromotionFight) (int64, int64, error) {
	if state == nil || state.Score != 1 || state.Points != 0 && state.Points != 20 || state.Points == 0 && state.LastGrantAt != 0 || state.Points == 20 && state.LastGrantAt <= 0 {
		return 0, 0, errors.New(constUtil.P14PromotionFightCorrupt)
	}
	var (
		score       int64 = 1
		grantAt     int64
		used        int64
		currentUsed int64
		previousAt  int64
	)
	for i := range rows {
		row := &rows[i]
		if row.SeasonIndex != state.SeasonIndex || row.ServerID != state.ServerID || row.PlayerID != state.PlayerID || row.MatchIndex != int64(i+1) || row.QualificationHash != state.QualificationHash || row.GrantAt <= 0 || row.GrantAt < grantAt || row.GrantAt > state.LastGrantAt || row.CreatedAt < row.GrantAt || row.CreatedAt < previousAt || row.CreatedAt < state.EligibleAt || row.CreatedAt >= state.StageEnd || row.ScoreBefore != score || row.PointsAfter != row.PointsBefore-1 || row.ScoreDelta != 0 && row.ScoreDelta != 15 || row.ScoreAfter != row.ScoreBefore+row.ScoreDelta || row.ScoreAfter < 1 || row.ScoreAfter > math.MaxInt32 {
			return 0, 0, errors.New(constUtil.P14PromotionFightCorrupt)
		}
		if row.GrantAt != grantAt {
			grantAt, used = row.GrantAt, 0
		}
		if row.PointsBefore != 20-used || row.PointsBefore <= 0 {
			return 0, 0, errors.New(constUtil.P14PromotionFightCorrupt)
		}
		used++
		if row.GrantAt == state.LastGrantAt {
			currentUsed++
		}
		score, previousAt = row.ScoreAfter, row.CreatedAt
	}
	points := state.Points - currentUsed
	if points < 0 || points > 20 {
		return 0, 0, errors.New(constUtil.P14PromotionFightCorrupt)
	}
	return points, score, nil
}

func p14PromotionFightReadOffer(row *db.P14PromotionFightOffer, now time.Time) (*p14PromotionFightRules, error) {
	if row == nil || row.SeasonIndex <= 0 || row.ServerID <= 0 || strings.TrimSpace(row.Version) == "" || len(row.Version) > 128 || row.Source != constUtil.P14PromotionFightSource || row.PublishedAt <= 0 || row.PublishedAt > row.CapturedAt || row.CapturedAt > now.UnixMilli() || row.SourceHash != p14PromotionFightOfferHash(row) {
		return nil, errors.New(constUtil.P14PromotionFightCorrupt)
	}
	return p14PromotionFightDecode(row.RulesJSON, row.RulesHash)
}

func p14PromotionFightFreezeOffer(tx *gorm.DB, policy *db.P14PromotionFightPolicy, season int32, server int64, now time.Time) (*db.P14PromotionFightOffer, *p14PromotionFightRules, error) {
	var offer db.P14PromotionFightOffer
	err := tx.First(&offer, "season_index=? AND server_id=?", season, server).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		offer = db.P14PromotionFightOffer{SeasonIndex: season, ServerID: server, Version: policy.Version, Source: policy.Source, RulesJSON: policy.RulesJSON, RulesHash: policy.RulesHash, PublishedAt: policy.PublishedAt, CapturedAt: now.UnixMilli()}
		offer.SourceHash = p14PromotionFightOfferHash(&offer)
		if _, err = p14PromotionFightReadOffer(&offer, now); err != nil {
			return nil, nil, err
		}
		if err = tx.Create(&offer).Error; err != nil {
			return nil, nil, err
		}
	} else if err != nil {
		return nil, nil, err
	}
	rules, err := p14PromotionFightReadOffer(&offer, now)
	return &offer, rules, err
}

func p14StorePromotionFightP6(tx *gorm.DB, record *db.P6BattleRecord, terminal *model.CombatFinalHPOutcome, now time.Time) error {
	if terminal == nil || terminal.Outcome == nil || record.Status != constUtil.P6BattleCaptured || record.Source != constUtil.P14PromotionFightBattleSource || !terminal.Outcome.Authoritative || constUtil.P6CrossStarterNativeProof == "" || terminal.Outcome.CapabilityHash != constUtil.P6CrossStarterNativeProof {
		return errors.New(constUtil.P14PromotionFightPending)
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
		return errors.New(constUtil.P14PromotionFightCorrupt)
	}
	return nil
}

// p14ReadPromotionFight reads a previously accepted result without recomputing
// its old engine, rerolling an opponent, or charging another match point.
func p14ReadPromotionFight(tx *gorm.DB, row *db.P14PromotionFight) (*pb.SCKuafuMatchPlayer, error) {
	reject := func() (*pb.SCKuafuMatchPlayer, error) { return nil, errors.New(constUtil.P14PromotionFightCorrupt) }
	if row == nil || row.SourceHash != p14PromotionFightHash(row) || p6Digest(row.Response) != row.ResponseHash || p6Digest(row.Terminal) != row.TerminalHash || row.OpponentServerID == row.ServerID || row.OpponentID == row.PlayerID || row.Stamp <= 0 {
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
	response := &pb.SCKuafuMatchPlayer{}
	if json.Unmarshal(row.Terminal, &terminal) != nil || proto.Unmarshal(row.Response, response) != nil || terminal.Outcome == nil {
		return reject()
	}
	if !p6KnownCrossStarterSettlementProof(input, row.ScopeProof) || !terminal.Outcome.Authoritative || !record.Authoritative || row.ScopeProof != terminal.Outcome.CapabilityHash || record.Status != constUtil.P6BattleComputed || record.PlayerID != row.PlayerID || record.Source != constUtil.P14PromotionFightBattleSource || record.SourceFight != row.ID || record.EngineVersion != terminal.EngineVersion || record.ConfigurationHash != terminal.ConfigurationHash || record.ClientVersion != terminal.ClientVersion || record.ServerResult != record.ClientResult || record.ServerResult != p6PlaybackResult(terminal.Outcome.Winner) {
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
	if record.OutcomeHash != p6Digest(body) || p6Digest(record.Outcome) != record.OutcomeHash || !bytes.Equal(record.Operates, operations) || record.ReplayHash != p6Digest(append(append([]byte(nil), operations...), byte(record.ServerResult))) || terminal.CalculationSourceSHA256 != p6Digest(source) || input.Attacker.GetPlayerId() != row.PlayerID || int64(input.Attacker.GetServerId()) != row.ServerID || input.Defender.GetPlayerId() != row.OpponentID || int64(input.Defender.GetServerId()) != row.OpponentServerID {
		return reject()
	}
	winner := row.OpponentID
	if terminal.Outcome.Winner == 0 {
		winner = row.PlayerID
	}
	if winner != row.WinnerID || response.GetState() != 0 || response.GetFightId() != row.ID || response.GetWinPlayerId() != winner || response.GetRandomSeed() != input.Seed || response.GetMaxRoundNum() != 40 || response.GetLastMatchNum() != int32(row.PointsAfter) || response.GetChangeScore() != int32(row.ScoreDelta) || response.RoomId != nil || len(response.PlayerList) != 2 || !proto.Equal(response.PlayerList[0], input.Attacker) || !proto.Equal(response.PlayerList[1], input.Defender) {
		return reject()
	}
	meta, err := json.Marshal(input.AppliedProperties["p14PromotionFight"])
	if err != nil {
		return nil, err
	}
	expected, _ := json.Marshal([]string{row.ID, row.OfferHash, row.AttackerFormationHash, row.DefenderFormationHash, row.GrantHash, strconv.FormatInt(row.Stamp, 10), strconv.FormatInt(row.ScoreBefore, 10)})
	if !bytes.Equal(meta, expected) {
		return reject()
	}
	if err = validateP6CrossStarterTerminal(input, &terminal); err != nil {
		return nil, err
	}
	return response, nil
}

func p14PromotionFightRows(tx *gorm.DB, state *db.P14PromotionState, now time.Time) ([]db.P14PromotionFight, error) {
	var rows []db.P14PromotionFight
	if !tx.Migrator().HasTable(&db.P14PromotionFight{}) {
		return rows, nil
	}
	if err := tx.Where("season_index=? AND server_id=? AND player_id=?", state.SeasonIndex, state.ServerID, state.PlayerID).Order("match_index").Find(&rows).Error; err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return rows, nil
	}
	var offer db.P14PromotionFightOffer
	if err := tx.First(&offer, "season_index=? AND server_id=?", state.SeasonIndex, state.ServerID).Error; err != nil {
		return nil, err
	}
	rules, err := p14PromotionFightReadOffer(&offer, now)
	if err != nil {
		return nil, err
	}
	for i := range rows {
		row := &rows[i]
		var grant db.P14PromotionGrant
		if err = tx.First(&grant, "season_index=? AND server_id=? AND player_id=? AND grant_at=?", row.SeasonIndex, row.ServerID, row.PlayerID, row.GrantAt).Error; err != nil {
			return nil, err
		}
		delta := rules.LossScore
		if row.WinnerID == row.PlayerID {
			delta = rules.WinScore
		}
		if row.CreatedAt > now.UnixMilli() || row.OfferHash != offer.SourceHash || row.GrantHash != grant.SourceHash || row.ScoreDelta != delta {
			return nil, errors.New(constUtil.P14PromotionFightCorrupt)
		}
		if _, err = p14ReadPromotionFight(tx, row); err != nil {
			return nil, err
		}
	}
	return rows, nil
}

func (s *GameService) p14PromotionFightBalanceTX(tx *gorm.DB, state *db.P14PromotionState, now time.Time) (int64, int64, error) {
	rows, err := p14PromotionFightRows(tx, state, now)
	if err != nil {
		return 0, 0, err
	}
	return p14PromotionFightTotals(state, rows)
}

// p14PromotionFightAt is one atomic original challenge. New typed17 proof is
// checked before persisting points or score; unsupported/pending calculations
// roll back the capture and every newly observed grant in this transaction.
func (s *GameService) p14PromotionFightAt(ctx context.Context, id, stamp int64, now time.Time) ([]proto.Message, error) {
	var response *pb.SCKuafuMatchPlayer
	err := db.GetIns().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := ServerGameClockGuardTransaction(tx); err != nil {
			return err
		}

		var season db.P14SeasonPolicy
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&season, 1).Error; err != nil {
			return err
		}
		var prior []db.P14PromotionFight
		if err := tx.Where("server_id=? AND player_id=? AND stamp=?", s.OriginalServerID(), id, stamp).Find(&prior).Error; err != nil {
			return err
		}
		if len(prior) > 1 {
			return errors.New(constUtil.P14PromotionFightCorrupt)
		}
		if len(prior) == 1 {
			var err error
			response, err = p14ReadPromotionFight(tx, &prior[0])
			return err
		}
		var gate db.P14PromotionFightPolicy
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&gate, 1).Error; err != nil {
			return err
		}
		if !gate.Enabled {
			return errors.New(constUtil.P14PromotionFightClosed)
		}
		if gate.Source != constUtil.P14PromotionFightSource {
			return errors.New(constUtil.P14PromotionFightCorrupt)
		}
		stage, _, err := s.p14SeasonStage(tx, &season, now)
		if err != nil {
			return err
		}
		if stage != constUtil.P14PromotionStage {
			return errors.New(constUtil.P14PromotionFightClosed)
		}
		entered, _, err := s.p14PromotionStatusTX(ctx, tx, &season, id, now)
		if err != nil {
			return err
		}
		if entered != 1 {
			return errors.New(constUtil.P14PromotionFightClosed)
		}
		var state db.P14PromotionState
		if err = tx.First(&state, "season_index=? AND server_id=? AND player_id=?", season.SeasonIndex, s.OriginalServerID(), id).Error; err != nil {
			return err
		}
		rows, err := p14PromotionFightRows(tx, &state, now)
		if err != nil {
			return err
		}
		points, score, err := p14PromotionFightTotals(&state, rows)
		if err != nil {
			return err
		}
		if points < 1 || score > math.MaxInt32-15 {
			return errors.New("跨服晋级匹配点不足或积分已达本地上限")
		}
		var local db.P14PromotionFormation
		if err = tx.First(&local, "season_index=? AND server_id=? AND player_id=?", season.SeasonIndex, s.OriginalServerID(), id).Error; err != nil {
			return errors.New("请先通过原入口更新当前晋级阵容")
		}
		config, err := s.p14ConfigurationHash()
		if err != nil {
			return err
		}
		readPlayer := func(row *db.P14PromotionFormation) (*pb.BattlePlayer, error) {
			qualification, _, _, err := s.p14PromotionFormationEligibility(ctx, tx, &season, row.ServerID, row.PlayerID, now)
			if err != nil {
				return nil, err
			}
			if row.QualificationHash != qualification || row.EngineVersion != constUtil.P6EngineVersion || row.ClientVersion != constUtil.P6OriginalClientHash || row.ConfigurationHash != config || row.CapturedAt > now.UnixMilli() {
				return nil, errors.New("晋级阵容需由原服按当前版本重新上传")
			}
			if _, err = p14PromotionFormationRead(row); err != nil {
				return nil, err
			}
			player := &pb.BattlePlayer{}
			if err = proto.Unmarshal(row.Battle, player); err != nil {
				return nil, err
			}
			return player, nil
		}
		attacker, err := readPlayer(&local)
		if err != nil {
			return err
		}
		var candidates []db.P14PromotionFormation
		if err = tx.Where("season_index=? AND server_id<>? AND player_id<>?", season.SeasonIndex, s.OriginalServerID(), id).Order("server_id,player_id").Find(&candidates).Error; err != nil {
			return err
		}
		if len(candidates) == 0 {
			return errors.New("当前没有满足真实联合身份条件的跨服对手")
		}
		choice, err := rand.Int(rand.Reader, big.NewInt(int64(len(candidates))))
		if err != nil {
			return err
		}
		peer := &candidates[choice.Int64()]
		defender, err := readPlayer(peer)
		if err != nil {
			return err
		}
		offer, rules, err := p14PromotionFightFreezeOffer(tx, &gate, season.SeasonIndex, s.OriginalServerID(), now)
		if err != nil {
			return err
		}
		var grant db.P14PromotionGrant
		if err = tx.First(&grant, "season_index=? AND server_id=? AND player_id=? AND grant_at=?", state.SeasonIndex, state.ServerID, state.PlayerID, state.LastGrantAt).Error; err != nil {
			return err
		}
		seed, err := rand.Int(rand.Reader, big.NewInt(math.MaxInt32-1))
		if err != nil {
			return err
		}
		fightID := p6Digest([]byte(fmt.Sprintf("p14/promotion/%d/%d/%d/%d", season.SeasonIndex, s.OriginalServerID(), id, stamp)))
		input := model.FrozenBattleInput{Attacker: attacker, Defender: defender, Seed: seed.Int64() + 1, MaxRound: 40, BattleType: 17, DefenderFirst: attacker.GetInst().GetFirstOrder() == 0 && defender.GetInst().GetFirstOrder() == 1, ClientVersion: local.ClientVersion, AppliedProperties: map[string][]string{"p14PromotionFight": {fightID, offer.SourceHash, local.SourceHash, peer.SourceHash, grant.SourceHash, strconv.FormatInt(stamp, 10), strconv.FormatInt(score, 10)}}}
		recordID, err := s.CaptureBattleReplay(ctx, tx, id, constUtil.P14PromotionFightBattleSource, fightID, input)
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
		terminal, err := s.ComputeP6CrossStarterSettlement(ctx, frozen)
		if err != nil {
			return err
		}
		if terminal == nil || terminal.Outcome == nil || !terminal.Outcome.Authoritative || !p6KnownCrossStarterSettlementProof(frozen, terminal.Outcome.CapabilityHash) {
			return errors.New(constUtil.P14PromotionFightPending)
		}
		winner, delta := peer.PlayerID, rules.LossScore
		if terminal.Outcome.Winner == 0 {
			winner, delta = id, rules.WinScore
		}
		response = &pb.SCKuafuMatchPlayer{State: proto.Int32(0), FightId: proto.String(fightID), WinPlayerId: proto.Int64(winner), PlayerList: []*pb.BattlePlayer{proto.Clone(frozen.Attacker).(*pb.BattlePlayer), proto.Clone(frozen.Defender).(*pb.BattlePlayer)}, RandomSeed: proto.Int64(frozen.Seed), MaxRoundNum: proto.Int32(40), LastMatchNum: proto.Int32(int32(points - rules.MatchCost)), ChangeScore: proto.Int32(int32(delta))}
		body, err := proto.MarshalOptions{Deterministic: true}.Marshal(response)
		if err != nil {
			return err
		}
		final, err := json.Marshal(terminal)
		if err != nil {
			return err
		}
		if err = p14StorePromotionFightP6(tx, &record, terminal, now); err != nil {
			return err
		}
		row := db.P14PromotionFight{ID: fightID, SeasonIndex: season.SeasonIndex, ServerID: s.OriginalServerID(), PlayerID: id, MatchIndex: int64(len(rows)) + 1, Stamp: stamp, OpponentServerID: peer.ServerID, OpponentID: peer.PlayerID, AttackerFormationHash: local.SourceHash, DefenderFormationHash: peer.SourceHash, QualificationHash: state.QualificationHash, OfferHash: offer.SourceHash, GrantAt: grant.GrantAt, GrantHash: grant.SourceHash, PointsBefore: points, PointsAfter: points - rules.MatchCost, ScoreBefore: score, ScoreAfter: score + delta, ScoreDelta: delta, WinnerID: winner, P6RecordID: recordID, ScopeProof: terminal.Outcome.CapabilityHash, Response: body, ResponseHash: p6Digest(body), Terminal: final, TerminalHash: p6Digest(final), CreatedAt: now.UnixMilli()}
		row.SourceHash = p14PromotionFightHash(&row)
		if _, _, err = p14PromotionFightTotals(&state, append(rows, row)); err != nil {
			return err
		}
		return tx.Create(&row).Error
	})
	if err != nil {
		return nil, err
	}
	return []proto.Message{response}, nil
}
