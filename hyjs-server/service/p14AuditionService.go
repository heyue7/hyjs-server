package service

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"math/big"
	"time"

	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

func p14AuditionMatchHash(row *db.P14AuditionMatch) string {
	copy := *row
	copy.SourceHash, copy.OutcomeHash = "", ""
	copy.Applied, copy.WinnerID = false, 0
	raw, _ := json.Marshal(copy)
	return p6Digest(raw)
}
func p14AuditionRoundHash(row *db.P14AuditionRound) string {
	copy := *row
	copy.SourceHash = ""
	raw, _ := json.Marshal(copy)
	return p6Digest(raw)
}
func p14AuditionMatchesHash(rows []db.P14AuditionMatch) string {
	hashes := make([]string, 0, len(rows))
	for _, row := range rows {
		hashes = append(hashes, row.SourceHash)
	}
	raw, _ := json.Marshal(hashes)
	return p6Digest(raw)
}

// p14AuditionInput 每次验证当前数据库完整blob/SHA，只解析校验所需的原双方和元数据。
// 原Tables保持冻结RawMessage字节验证；完整机制验证已由Capture执行，实际计算仍使用P6完整解码。
func p14AuditionInput(record *db.P6BattleRecord) (*model.FrozenBattleInput, error) {
	raw, err := decodeP6StoredInput(record.Input)
	if err != nil {
		return nil, err
	}
	if p6Digest(raw) != record.InputHash {
		return nil, errors.New(constUtil.P6ReplayConflict)
	}
	var projection struct {
		Attacker          *pb.BattlePlayer  `json:"attacker"`
		Defender          *pb.BattlePlayer  `json:"defender"`
		Monster           *pb.BattleMonster `json:"monster"`
		Seed              int64             `json:"seed"`
		MaxRound          int32             `json:"maxRound"`
		BattleType        int32             `json:"battleType"`
		IsAttToDef        bool              `json:"isAttToDef"`
		DefenderFirst     bool              `json:"defenderFirst"`
		KFReplay          *pb.KFReplayData  `json:"kfReplay"`
		Tables            json.RawMessage   `json:"tables"`
		ConfigurationHash string            `json:"configurationHash"`
		EngineVersion     string            `json:"engineVersion"`
		ClientVersion     string            `json:"clientVersion"`
	}
	if err := json.Unmarshal(raw, &projection); err != nil {
		return nil, err
	}
	if p6Digest(projection.Tables) != record.ConfigurationHash || projection.ConfigurationHash != record.ConfigurationHash || projection.EngineVersion != record.EngineVersion || projection.ClientVersion != record.ClientVersion || projection.Seed != record.Seed {
		return nil, errors.New(constUtil.P6ReplayConflict)
	}
	input := &model.FrozenBattleInput{Attacker: projection.Attacker, Defender: projection.Defender, Monster: projection.Monster, Seed: projection.Seed, MaxRound: projection.MaxRound, BattleType: projection.BattleType, IsAttToDef: projection.IsAttToDef, DefenderFirst: projection.DefenderFirst, KFReplay: projection.KFReplay}
	if err := ValidateP6FirstOrder(input); err != nil {
		return nil, err
	}
	return input, nil
}

func p14AuditionQualificationHash(row *db.P14AuditionQualification) string {
	copy := *row
	copy.SourceHash = ""
	raw, _ := json.Marshal(copy)
	return p6Digest(raw)
}

// p14AuditionQualificationBuild 从逐场权威回放重建五败计数和最后存活者，缺少任何一场不授予资格。
func (s *GameService) p14AuditionQualificationBuild(ctx context.Context, tx *gorm.DB, boundary *db.P14AuditionBoundary, roster []db.P14AuditionRoster) (*db.P14AuditionQualification, error) {
	var rounds []db.P14AuditionRound
	if err := tx.Where("season_index=? AND server_id=?", boundary.SeasonIndex, boundary.ServerID).Order("round_index").Find(&rounds).Error; err != nil {
		return nil, err
	}
	if len(rounds) == 0 {
		if len(roster) == 0 || len(roster) > constUtil.P14AuditionPromotionLimit {
			return nil, errors.New(constUtil.P14AuditionAuthorityPending)
		}
		ids := make([]int64, 0, len(roster))
		for _, row := range roster {
			ids = append(ids, row.PlayerID)
		}
		players, err := json.Marshal(ids)
		if err != nil {
			return nil, err
		}
		return &db.P14AuditionQualification{SeasonIndex: boundary.SeasonIndex, ServerID: boundary.ServerID, BoundaryHash: boundary.SourceHash, Players: players, ResultsHash: p6Digest([]byte(boundary.SourceHash + "/" + constUtil.P14AuditionAutoSource))}, nil
	}
	losses := map[int64]int64{}
	var sources []string
	for i := range rounds {
		round := &rounds[i]
		if round.RoundIndex != int64(i+1) {
			return nil, errors.New("海选晋级来源缺失完整历史轮次")
		}
		matches, err := s.p14AuditionRoundRead(ctx, tx, round, boundary, roster)
		if err != nil {
			return nil, err
		}
		var participants []int64
		if err := json.Unmarshal(round.Participants, &participants); err != nil {
			return nil, err
		}
		eligible := map[int64]bool{}
		for _, row := range roster {
			if losses[row.PlayerID] < constUtil.P14AuditionLossLimit {
				eligible[row.PlayerID] = true
			}
		}
		if len(eligible) <= constUtil.P14AuditionPromotionLimit || len(participants) != len(eligible) {
			return nil, errors.New("海选历史当轮名单遗漏存活者或终局后仍比赛")
		}
		for _, id := range participants {
			if !eligible[id] {
				return nil, errors.New("淘汰玩家进入海选后续轮次")
			}
		}
		for _, match := range matches {
			if !match.Applied {
				return nil, errors.New(constUtil.P14AuditionAuthorityPending)
			}
			if losses[match.AttackerID] >= constUtil.P14AuditionLossLimit || losses[match.DefenderID] >= constUtil.P14AuditionLossLimit {
				return nil, errors.New("已五败淘汰者出现在后续海选比赛")
			}
			loser := match.AttackerID
			if match.WinnerID == loser {
				loser = match.DefenderID
			}
			losses[loser]++
			sources = append(sources, match.SourceHash+"/"+match.OutcomeHash)
		}
	}
	var ids []int64
	for _, row := range roster {
		if losses[row.PlayerID] < constUtil.P14AuditionLossLimit {
			ids = append(ids, row.PlayerID)
		}
	}
	if len(ids) == 0 || len(ids) > constUtil.P14AuditionPromotionLimit {
		return nil, errors.New("原海选尚未产生最后十名实际存活者")
	}
	players, err := json.Marshal(ids)
	if err != nil {
		return nil, err
	}
	raw, err := json.Marshal(sources)
	if err != nil {
		return nil, err
	}
	return &db.P14AuditionQualification{SeasonIndex: boundary.SeasonIndex, ServerID: boundary.ServerID, BoundaryHash: boundary.SourceHash, RoundCount: int64(len(rounds)), Players: players, ResultsHash: p6Digest(raw)}, nil
}

func (s *GameService) p14AuditionQualificationRead(ctx context.Context, tx *gorm.DB, season int32, server int64) (*db.P14AuditionQualification, error) {
	var saved db.P14AuditionQualification
	if err := tx.First(&saved, "season_index=? AND server_id=?", season, server).Error; err != nil {
		return nil, err
	}
	boundary, roster, err := p14AuditionRead(tx, season, server)
	if err != nil {
		return nil, err
	}
	derived, err := s.p14AuditionQualificationBuild(ctx, tx, boundary, roster)
	if err != nil {
		return nil, err
	}
	if err := p14AuditionAutoRecoveryVerify(boundary, &saved); err != nil {
		return nil, err
	}
	if saved.QualifiedAt < boundary.RegisterEnd || saved.SourceHash != p14AuditionQualificationHash(&saved) || saved.BoundaryHash != derived.BoundaryHash || saved.RoundCount != derived.RoundCount || string(saved.Players) != string(derived.Players) || saved.ResultsHash != derived.ResultsHash {
		return nil, errors.New("海选晋级名单与实际权威结果不一致")
	}
	return &saved, nil
}

// p14AuditionRoundRead 校验整轮配对/完整名册与P6输入，不把丢失比赛当成已完赛。
func (s *GameService) p14AuditionRoundRead(ctx context.Context, tx *gorm.DB, round *db.P14AuditionRound, boundary *db.P14AuditionBoundary, roster []db.P14AuditionRoster) ([]db.P14AuditionMatch, error) {
	if round.BoundaryHash != boundary.SourceHash || round.SourceHash != p14AuditionRoundHash(round) || round.RoundIndex < 1 {
		return nil, errors.New("海选随机轮次与真实截止名册不一致")
	}
	var ids []int64
	if json.Unmarshal(round.Participants, &ids) != nil {
		return nil, errors.New("海选当轮完整参赛名单损坏")
	}
	byID := make(map[int64]*db.P14AuditionRoster, len(roster))
	for i := range roster {
		byID[roster[i].PlayerID] = &roster[i]
	}
	seen := map[int64]bool{}
	for _, id := range ids {
		if byID[id] == nil || seen[id] {
			return nil, errors.New("海选随机名单包含外服或重复角色")
		}
		seen[id] = true
	}
	var matches []db.P14AuditionMatch
	if err := tx.Where("season_index=? AND server_id=? AND round_index=?", round.SeasonIndex, round.ServerID, round.RoundIndex).Order("pair_index").Find(&matches).Error; err != nil {
		return nil, err
	}
	if len(matches) != len(ids)/2 || p14AuditionMatchesHash(matches) != round.MatchHash {
		return nil, errors.New("海选当轮完整比赛数量或SHA损坏")
	}
	for i := range matches {
		row := &matches[i]
		if row.PairIndex != int64(i+1) || row.AttackerID != ids[2*i] || row.DefenderID != ids[2*i+1] || row.AttackerHash != byID[row.AttackerID].SourceHash || row.DefenderHash != byID[row.DefenderID].SourceHash || row.SourceHash != p14AuditionMatchHash(row) {
			return nil, errors.New("海选实际双方/顺序/来源SHA损坏")
		}
		var record db.P6BattleRecord
		if err := tx.First(&record, "id=?", row.ReplayID).Error; err != nil {
			return nil, err
		}
		input, err := p14AuditionInput(&record)
		if err != nil {
			return nil, err
		}
		if record.PlayerID != row.AttackerID || record.Source != constUtil.P14AuditionBattleSource || record.SourceFight != row.ID || record.InputHash != row.InputHash || input.Seed != row.Seed || input.BattleType != 17 || input.MaxRound != 40 || input.IsAttToDef || input.KFReplay != nil {
			return nil, errors.New("海选P6记录归属或原战斗参数损坏")
		}
		a, b, err := p14AuditionPlayers(byID[row.AttackerID], byID[row.DefenderID])
		if err != nil {
			return nil, err
		}
		if !proto.Equal(a, input.Attacker) || !proto.Equal(b, input.Defender) {
			return nil, errors.New("海选P6输入已偏离真实截止阵容")
		}
		if row.Applied {
			outcome, err := s.ComputeBattleReplay(ctx, tx, row.AttackerID, constUtil.P14AuditionBattleSource, row.ID, nil)
			if err != nil {
				return nil, err
			}
			winner := row.DefenderID
			if outcome.Winner == 0 {
				winner = row.AttackerID
			}
			if !outcome.Authoritative || !record.Authoritative || row.WinnerID != winner || row.OutcomeHash != record.OutcomeHash {
				return nil, errors.New(constUtil.P14AuditionAuthorityPending)
			}
		} else if row.WinnerID != 0 || row.OutcomeHash != "" {
			return nil, errors.New("未权威结算的海选出现赢家")
		}
	}
	return matches, nil
}

// p14AuditionPlayers 保留真实截止阵容，双方原先手都为空时采用原回调的进攻先手fallback。
func p14AuditionPlayers(a, b *db.P14AuditionRoster) (*pb.BattlePlayer, *pb.BattlePlayer, error) {
	var attacker, defender pb.BattlePlayer
	if proto.Unmarshal(a.Battle, &attacker) != nil || proto.Unmarshal(b.Battle, &defender) != nil {
		return nil, nil, errors.New("海选原双方阵容损坏")
	}
	defenderFirst := attacker.GetInst().GetFirstOrder() == 0 && defender.GetInst().GetFirstOrder() == 1
	if err := SetP6FirstOrder(&attacker, &defender, defenderFirst); err != nil {
		return nil, nil, err
	}
	return &attacker, &defender, nil
}

// PrepareP14AuditionRound 随机配对真实存活本服成员并冻结P6；没有权威旧轮结果不能开下一轮。
func (s *GameService) PrepareP14AuditionRound(ctx context.Context, now time.Time) (*db.P14AuditionRound, error) {
	var result *db.P14AuditionRound
	err := db.GetIns().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := ServerGameClockGuardTransaction(tx); err != nil {
			return err
		}

		var policy db.P14SeasonPolicy
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&policy, 1).Error; err != nil {
			return err
		}
		var gate db.P14AuditionMatchPolicy
		if err := tx.First(&gate, 1).Error; err != nil {
			return err
		}
		if !gate.Enabled {
			return errors.New("海选真实匹配尚未启用")
		}
		stage, _, err := s.p14SeasonStage(tx, &policy, now)
		if err != nil {
			return err
		}
		if stage != constUtil.P14AuditionStage {
			return errors.New("当前不在原海选比赛阶段")
		}
		boundary, roster, err := p14AuditionRead(tx, policy.SeasonIndex, s.OriginalServerID())
		if err != nil {
			return err
		}
		losses := map[int64]int64{}
		var rounds []db.P14AuditionRound
		if err := tx.Where("season_index=? AND server_id=?", policy.SeasonIndex, s.OriginalServerID()).Order("round_index").Find(&rounds).Error; err != nil {
			return err
		}
		for i := range rounds {
			round := &rounds[i]
			if round.RoundIndex != int64(i+1) {
				return errors.New("海选历史轮次存在缺口")
			}
			matches, err := s.p14AuditionRoundRead(ctx, tx, round, boundary, roster)
			if err != nil {
				return err
			}
			pending := false
			for _, match := range matches {
				if !match.Applied {
					pending = true
					continue
				}
				loser := match.AttackerID
				if match.WinnerID == loser {
					loser = match.DefenderID
				}
				losses[loser]++
			}
			if pending {
				if i != len(rounds)-1 {
					return errors.New("未完海选轮次后出现新对局")
				}
				result = round
				return nil
			}
		}
		active := make([]db.P14AuditionRoster, 0, len(roster))
		currentConfig, err := s.p14ConfigurationHash()
		if err != nil {
			return err
		}
		for _, row := range roster {
			if losses[row.PlayerID] >= constUtil.P14AuditionLossLimit {
				continue
			}
			if row.ConfigurationHash != currentConfig || row.EngineVersion != constUtil.P6EngineVersion || row.ClientVersion != constUtil.P6OriginalClientHash {
				return errors.New("海选截止阵容版本与当前P6不兼容")
			}
			active = append(active, row)
		}
		// 原报名不足自动晋级分支只依据本服截止名册，不生成虚构对局、轮空胜场或胜负。
		if len(active) <= constUtil.P14AuditionPromotionLimit {
			if len(active) > 0 {
				qualified, err := s.p14AuditionQualificationBuild(ctx, tx, boundary, roster)
				if err != nil {
					return err
				}
				var previous db.P14AuditionQualification
				err = tx.First(&previous, "season_index=? AND server_id=?", policy.SeasonIndex, s.OriginalServerID()).Error
				if err == nil {
					_, err = s.p14AuditionQualificationRead(ctx, tx, policy.SeasonIndex, s.OriginalServerID())
					return err
				}
				if !errors.Is(err, gorm.ErrRecordNotFound) {
					return err
				}
				qualified.QualifiedAt = now.UnixMilli()
				qualified.SourceHash = p14AuditionQualificationHash(qualified)
				return tx.Create(qualified).Error
			}
			return nil
		}
		for i := len(active) - 1; i > 0; i-- {
			n, err := rand.Int(rand.Reader, big.NewInt(int64(i+1)))
			if err != nil {
				return err
			}
			active[i], active[n.Int64()] = active[n.Int64()], active[i]
		}
		ids := make([]int64, 0, len(active))
		for _, row := range active {
			ids = append(ids, row.PlayerID)
		}
		participants, err := json.Marshal(ids)
		if err != nil {
			return err
		}
		round := &db.P14AuditionRound{SeasonIndex: policy.SeasonIndex, ServerID: s.OriginalServerID(), RoundIndex: int64(len(rounds) + 1), BoundaryHash: boundary.SourceHash, Participants: participants, CreatedAt: now.UnixMilli()}
		matches := make([]db.P14AuditionMatch, 0, len(active)/2)
		for i := 0; i+1 < len(active); i += 2 {
			a, b, err := p14AuditionPlayers(&active[i], &active[i+1])
			if err != nil {
				return err
			}
			seed, err := rand.Int(rand.Reader, big.NewInt(math.MaxInt64))
			if err != nil {
				return err
			}
			row := db.P14AuditionMatch{SeasonIndex: round.SeasonIndex, ServerID: round.ServerID, RoundIndex: round.RoundIndex, PairIndex: int64(i/2 + 1), AttackerID: a.GetPlayerId(), DefenderID: b.GetPlayerId(), AttackerHash: active[i].SourceHash, DefenderHash: active[i+1].SourceHash, Seed: seed.Int64(), CreatedAt: now.UnixMilli()}
			row.ID = p6Digest([]byte(fmt.Sprintf("p14-audition/%d/%d/%d/%d", row.SeasonIndex, row.ServerID, row.RoundIndex, row.PairIndex)))
			input := model.FrozenBattleInput{Attacker: a, Defender: b, Seed: row.Seed, MaxRound: 40, BattleType: 17, DefenderFirst: a.GetInst().GetFirstOrder() == 0 && b.GetInst().GetFirstOrder() == 1}
			row.ReplayID, err = s.CaptureBattleReplay(ctx, tx, row.AttackerID, constUtil.P14AuditionBattleSource, row.ID, input)
			if err != nil {
				return err
			}
			var record db.P6BattleRecord
			if err := tx.First(&record, "id=?", row.ReplayID).Error; err != nil {
				return err
			}
			row.InputHash = record.InputHash
			row.SourceHash = p14AuditionMatchHash(&row)
			if err := tx.Create(&row).Error; err != nil {
				return err
			}
			matches = append(matches, row)
		}
		round.MatchHash = p14AuditionMatchesHash(matches)
		round.SourceHash = p14AuditionRoundHash(round)
		if err := tx.Create(round).Error; err != nil {
			return err
		}
		result = round
		return nil
	})
	return result, err
}

// p14AuditionResult 投影真实海选终局；自动晋级的胜败计数均为0，诊断计算不能形成终局。
func (s *GameService) p14AuditionResult(ctx context.Context, id int64, now time.Time) ([]proto.Message, error) {
	var reply *pb.SCKuaFuMatchResult
	err := db.GetIns().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := ServerGameClockGuardTransaction(tx); err != nil {
			return err
		}

		var policy db.P14SeasonPolicy
		if err := tx.First(&policy, 1).Error; err != nil {
			return err
		}
		stage, _, err := s.p14SeasonStage(tx, &policy, now)
		if err != nil {
			return err
		}
		if stage < 4 || stage > constUtil.P14LegacyPromotionStage {
			return errors.New("当前不是原海选终局查询阶段")
		}
		qualified, err := s.p14AuditionQualificationRead(ctx, tx, policy.SeasonIndex, s.OriginalServerID())
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return errors.New(constUtil.P14AuditionAuthorityPending)
		}
		if err != nil {
			return err
		}
		// A prepublished downtime recovery first materializes its zero-match
		// result under the legacy stage7 mapping. Keep only its frozen read path.
		if stage == constUtil.P14LegacyPromotionStage && qualified.RecoverySource != constUtil.P14AuditionAutoRecoverySource {
			return errors.New("当前不是原海选终局查询阶段")
		}
		var ids []int64
		if err := json.Unmarshal(qualified.Players, &ids); err != nil {
			return err
		}
		passed := int32(0)
		for _, player := range ids {
			if player == id {
				passed = 1
			}
		}
		var entrant db.P14AuditionRoster
		if err := tx.First(&entrant, "season_index=? AND server_id=? AND player_id=?", policy.SeasonIndex, s.OriginalServerID(), id).Error; err != nil {
			return errors.New("角色未进入本服真实海选名册")
		}
		var matches []db.P14AuditionMatch
		if err := tx.Where("season_index=? AND server_id=? AND (attacker_id=? OR defender_id=?)", policy.SeasonIndex, s.OriginalServerID(), id, id).Find(&matches).Error; err != nil {
			return err
		}
		wins, failures := int32(0), int32(0)
		for _, match := range matches {
			if !match.Applied {
				return errors.New(constUtil.P14AuditionAuthorityPending)
			}
			if match.WinnerID == id {
				wins++
			} else {
				failures++
			}
		}
		reply = &pb.SCKuaFuMatchResult{Result: proto.Int32(passed), MatchResult: &pb.KuaFuMatchResult{TypeStage: proto.Int32(1), WinTimes: proto.Int32(wins), FailTimes: proto.Int32(failures)}}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return []proto.Message{reply}, nil
}

// p14AuditionAutoReplayInfo 原自动晋级没有战报；只有已核实的零场资格可返回原autoPass空列表。
func (s *GameService) p14AuditionAutoReplayInfo(ctx context.Context, id int64, request *pb.CSKFReplayInfo, now time.Time) ([]proto.Message, error) {
	if request.GetType() != 1 || request.GetReplayFlag() != 0 || request.GetRegId() != "" || request.GetPageIndex() != 0 {
		return nil, errors.New(constUtil.P14ResultsUnverified)
	}
	err := db.GetIns().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := ServerGameClockGuardTransaction(tx); err != nil {
			return err
		}

		var policy db.P14SeasonPolicy
		if err := tx.First(&policy, 1).Error; err != nil {
			return err
		}
		stage, _, err := s.p14SeasonStage(tx, &policy, now)
		if err != nil {
			return err
		}
		if stage < 4 || stage > 6 {
			return errors.New("当前没有真实本服海选战报")
		}
		qualified, err := s.p14AuditionQualificationRead(ctx, tx, policy.SeasonIndex, s.OriginalServerID())
		if err != nil {
			return err
		}
		if qualified.RoundCount != 0 {
			return errors.New("真实海选对局录像分页仍待恢复")
		}
		var member db.P14AuditionRoster
		return tx.First(&member, "season_index=? AND server_id=? AND player_id=?", policy.SeasonIndex, s.OriginalServerID(), id).Error
	})
	if err != nil {
		return nil, err
	}
	return []proto.Message{&pb.SCKFReplayInfo{Type: proto.Int32(1), SubType: proto.Int32(request.GetSubType()), PageIndex: proto.Int32(0), PageNum: proto.Int32(1)}}, nil
}

// ComputeP14AuditionMatch 持久真实整场计算和回放；未权威结果不写赢家、不增加失败、不授予晋级。
func (s *GameService) ComputeP14AuditionMatch(ctx context.Context, matchID string) (*model.CombatOutcome, error) {
	var result *model.CombatOutcome
	err := db.GetIns().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := ServerGameClockGuardTransaction(tx); err != nil {
			return err
		}

		var policy db.P14SeasonPolicy
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&policy, 1).Error; err != nil {
			return err
		}
		var match db.P14AuditionMatch
		if err := tx.First(&match, "id=? AND server_id=?", matchID, s.OriginalServerID()).Error; err != nil {
			return err
		}
		boundary, roster, err := p14AuditionRead(tx, match.SeasonIndex, match.ServerID)
		if err != nil {
			return err
		}
		var round db.P14AuditionRound
		if err := tx.First(&round, "season_index=? AND server_id=? AND round_index=?", match.SeasonIndex, match.ServerID, match.RoundIndex).Error; err != nil {
			return err
		}
		if _, err := s.p14AuditionRoundRead(ctx, tx, &round, boundary, roster); err != nil {
			return err
		}
		result, err = s.ComputeBattleReplay(ctx, tx, match.AttackerID, constUtil.P14AuditionBattleSource, match.ID, nil)
		if err != nil {
			return err
		}
		if !result.Authoritative {
			return nil
		}
		var record db.P6BattleRecord
		if err := tx.First(&record, "id=?", match.ReplayID).Error; err != nil {
			return err
		}
		if !record.Authoritative || record.InputHash != match.InputHash || result.Winner < 0 || result.Winner > 1 {
			return errors.New(constUtil.P14AuditionAuthorityPending)
		}
		match.WinnerID = match.DefenderID
		if result.Winner == 0 {
			match.WinnerID = match.AttackerID
		}
		match.Applied, match.OutcomeHash = true, record.OutcomeHash
		return tx.Save(&match).Error
	})
	return result, err
}

// TickP14Audition 复用实际P14生命周期；每个捕获对局仅计算一次，重启恢复原随机轮次与未完成记录。
func (s *GameService) TickP14Audition(ctx context.Context, now time.Time) error {
	tx := db.GetIns().WithContext(ctx)
	var gate db.P14AuditionMatchPolicy
	if err := tx.First(&gate, 1).Error; err != nil {
		return err
	}
	if !gate.Enabled {
		return nil
	}
	var policy db.P14SeasonPolicy
	if err := tx.First(&policy, 1).Error; err != nil {
		return err
	}
	stage, _, err := s.p14SeasonStage(tx, &policy, now)
	if err != nil {
		return err
	}
	if stage != constUtil.P14AuditionStage {
		if stage >= 5 && stage <= constUtil.P14PromotionStage {
			return s.recoverP14AuditionAuto(ctx, now)
		}
		return nil
	}
	round, err := s.PrepareP14AuditionRound(ctx, now)
	if err != nil || round == nil {
		return err
	}
	var matches []db.P14AuditionMatch
	if err := tx.Where("season_index=? AND server_id=? AND round_index=?", round.SeasonIndex, round.ServerID, round.RoundIndex).Order("pair_index").Find(&matches).Error; err != nil {
		return err
	}
	for _, match := range matches {
		if err := ctx.Err(); err != nil {
			return err
		}
		var record db.P6BattleRecord
		if err := tx.Select("id", "status", "authoritative").First(&record, "id=?", match.ReplayID).Error; err != nil {
			return err
		}
		if record.Status == constUtil.P6BattleComputed && (match.Applied || !record.Authoritative) {
			continue
		}
		if _, err := s.ComputeP14AuditionMatch(ctx, match.ID); err != nil {
			return err
		}
	}
	return nil
}
