package service

import (
	"encoding/json"
	"errors"
	"math"
	"sort"
	"time"

	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	pb "hyjs-server/model/protocol"
)

type p13UFCombatLedger struct {
	ActualKills  map[int64]int32
	ActualDeaths map[int64]int32
	ActualScores map[int64]float64
	LastCars     map[int64]*pb.InstUFCariot
	Kills        [2]int32
	Reached      [2]int64
	Hash         string
	PlayerKills  map[int64]int32
	PlayerDeaths map[int64]int32
	PlayerScores map[int64]float64
	Proof        []byte
	Last         map[int64]*pb.InstFightable
}

// This comparator consumes proven events, never the calculation's winner flag.
func p13UFCombatWinner(kills [2]int32, reached [2]int64, powers []int64) (int, string) {
	if kills[0] != kills[1] {
		if kills[0] > kills[1] {
			return 0, "kills"
		}
		return 1, "kills"
	}
	if reached[0] != reached[1] {
		if reached[0] < reached[1] {
			return 0, "first-reaching"
		}
		return 1, "first-reaching"
	}
	if powers[0] != powers[1] {
		if powers[0] < powers[1] {
			return 0, "lower-power"
		}
		return 1, "lower-power"
	}
	return -1, ""
}

// Reject unclosed effects without relaxing the peaceful branch's NoP6 guard.
func p13UFCombatUnclosed(tx *gorm.DB, match *db.P13UnionFightMatch, rows []db.P13UnionFightPlayerBattleAttempt, ids []int64) (bool, error) {
	for _, table := range []any{&db.P13UnionFightBattleAuditAttempt{}, &db.P13UnionFightGuardAuditAttempt{}} {
		var n int64
		if e := tx.Model(table).Where("match_id=?", match.ID).Count(&n).Error; e != nil {
			return true, e
		}
		if n > 0 {
			return true, nil
		}
	}
	var n int64
	if e := tx.Model(&db.P13UnionFightItemState{}).Where("match_id=? AND (normal_until>0 OR diamond_until>0)", match.ID).Count(&n).Error; e != nil {
		return true, e
	}
	if n > 0 {
		return true, nil
	}
	if e := tx.Model(&db.P13UnionFightGateState{}).Where("match_id=? AND frame_hash<>initial_hash", match.ID).Count(&n).Error; e != nil {
		return true, e
	}
	if n > 0 {
		return true, nil
	}
	// Items may heal or revive and later expire. Their durable receipt still blocks
	// this narrower branch even if the current cooldown or HP looks pristine.
	if e := tx.Model(&db.P13GuildReceipt{}).Where("player_id IN ? AND operation=? AND created_at>=? AND created_at<=?", ids, constUtil.P13UnionFightItemUseOperation, match.StartAt, match.EndAt).Count(&n).Error; e != nil {
		return true, e
	}
	if n > 0 {
		return true, nil
	}
	var intents []db.P13UnionFightSettlementIntent
	if e := tx.Where("match_id=?", match.ID).Find(&intents).Error; e != nil {
		return true, e
	}
	accepted := map[[3]int64]int64{}
	for _, row := range rows {
		key := [3]int64{row.PlayerID, row.TargetID, row.ClientStamp}
		accepted[key] = max(accepted[key], row.CreatedAt)
	}
	for _, intent := range intents {
		created, found := accepted[[3]int64{intent.PlayerID, intent.TargetID, intent.Stamp}]
		if !found || intent.ObservedAt < match.StartAt || intent.ObservedAt > created || (intent.Source != constUtil.P13UnionFightSettlementSource && intent.Source != constUtil.P13UFPlayerBattleSource) {
			return true, nil
		}
	}
	bound := map[string]bool{}
	for _, row := range rows {
		bound[row.P6RecordID] = true
	}
	// Unscoped history is never silently ignored. The maintenance backfill
	// verifies its input SHA before assigning match identity.
	if e := tx.Model(&db.P6BattleRecord{}).Where("player_id IN ? AND source IN ? AND p13_match_id=''", ids, p13P6MatchSources()).Count(&n).Error; e != nil {
		return true, e
	}
	if n > 0 {
		return true, nil
	}
	var records []struct{ ID string }
	if e := tx.Model(&db.P6BattleRecord{}).Select("id").Where("p13_match_id=? AND source IN ?", match.ID, p13P6MatchSources()).Find(&records).Error; e != nil {
		return true, e
	}
	for _, record := range records {
		if !bound[record.ID] {
			return true, nil
		}
	}
	return false, nil
}

func (s *GameService) p13UFCombatLedgerRead(tx *gorm.DB, match *db.P13UnionFightMatch, guilds map[int64]string, ids []int64) (*p13UFCombatLedger, error) {
	var rows []db.P13UnionFightPlayerBattleAttempt
	if e := tx.Select("id,match_id,player_id,target_id,client_stamp,created_at,p6_record_id").Where("match_id=?", match.ID).Order("due_at,id").Find(&rows).Error; e != nil {
		return nil, e
	}
	if len(rows) == 0 {
		return nil, nil
	}
	blocked, e := p13UFCombatUnclosed(tx, match, rows, ids)
	if e != nil || blocked {
		return nil, e
	}
	ledger := &p13UFCombatLedger{Reached: [2]int64{match.StartAt, match.StartAt}, PlayerKills: map[int64]int32{}, PlayerDeaths: map[int64]int32{}, PlayerScores: map[int64]float64{}, Last: map[int64]*pb.InstFightable{}}
	digests := []string{}
	proof := p13UFCombatProof{Schema: constUtil.P13UFCombatProofSchema, MatchSHA: match.SnapshotHash, LeftPreviewSHA: p6Digest(match.LeftPreview), RightPreviewSHA: p6Digest(match.RightPreview), Players: append([]int64(nil), ids...)}
	var offer db.P13UnionFightPlayerBattleOffer
	if e := tx.First(&offer, "match_id=?", match.ID).Error; e != nil {
		return nil, e
	}
	for batchStart := 0; batchStart < len(rows); batchStart += 4 {
		batchEnd := min(batchStart+4, len(rows))
		batchIDs := []string{}
		recordIDs := []string{}
		for _, row := range rows[batchStart:batchEnd] {
			batchIDs = append(batchIDs, row.ID)
			recordIDs = append(recordIDs, row.P6RecordID)
		}
		var batch []db.P13UnionFightPlayerBattleAttempt
		if e := tx.Where("id IN ?", batchIDs).Order("due_at,id").Find(&batch).Error; e != nil {
			return nil, e
		}
		if len(batch) != len(batchIDs) {
			return nil, errors.New("公会战结算缺来源行")
		}
		var records []db.P6BattleRecord
		if e := tx.Where("id IN ?", recordIDs).Find(&records).Error; e != nil {
			return nil, e
		}
		byID := map[string]*db.P6BattleRecord{}
		for i := range records {
			byID[records[i].ID] = &records[i]
		}
		for i := range batch {
			row := &batch[i]
			if row.Status != constUtil.P13UFPlayerBattleSettled || row.CreatedAt < match.StartAt || row.DueAt > match.EndAt || row.DueAt <= row.CreatedAt || row.SettledAt < row.DueAt {
				return nil, nil
			}
			record := byID[row.P6RecordID]
			if record == nil || record.P13MatchID != match.ID {
				return nil, errors.New("公会战 P6 归属未认证")
			}
			input, out, before, rules, e := s.p13UFPlayerBattleVerifyLoaded(tx.Statement.Context, row, record, &offer)
			if e != nil {
				return nil, e
			}
			frames, e := p13UFPlayerBattleFrames(before)
			if e != nil {
				return nil, e
			}
			after, dead, reward, revive, e := p13UFPlayerBattleApply(frames, input, out, rules)
			if e != nil {
				return nil, e
			}
			var actual []*pb.InstUFPlayer
			if json.Unmarshal(row.After, &actual) != nil || len(actual) != 2 || row.DeadPlayerID != dead || row.RewardPlayerID != reward || row.ReviveSeconds != revive {
				return nil, errors.New("公会战真实终末映射冲突")
			}
			for j, frame := range actual {
				left := float32(0)
				if frame.GetGuid() == dead {
					left = float32(max(int64(0), row.DueAt+row.ReviveSeconds*1000-row.SettledAt)) / 1000
				}
				wanted := after[j]
				if frame.GetGuid() != wanted.GetGuid() || frame.GetCamp() != wanted.GetCamp() || !proto.Equal(frame.FightInfo, wanted.FightInfo) || frame.GetKillNum() != wanted.GetKillNum() || frame.GetDeadNum() != wanted.GetDeadNum() || frame.GetOwnScore() != wanted.GetOwnScore() || frame.GetOwnToken() != wanted.GetOwnToken() || frame.GetState() != wanted.GetState() || frame.GetTotalTime() != wanted.GetTotalTime() || frame.GetLeftTime() != left {
					return nil, errors.New("公会战保存终末与P6不一致")
				}
			}
			proof.Events = append(proof.Events, p13UFCombatProofEvent{Attempt: p13UFCombatAttemptMetadata(row), Record: p13UFCombatRecordMetadata(record)})
			for j, side := range before {
				expectedGuild := match.LeftGuildID
				if side.Camp == 1 {
					expectedGuild = match.RightGuildID
				}
				if guilds[side.PlayerID] != expectedGuild || side.GuildID != expectedGuild {
					return nil, errors.New("公会战击杀来源不属于冻结阵营")
				}
				frame := frames[j]
				if frame.GetKillNum() != ledger.PlayerKills[side.PlayerID] || frame.GetDeadNum() != ledger.PlayerDeaths[side.PlayerID] {
					return nil, errors.New("公会战击杀前态账本不连续")
				}
				if previous := ledger.Last[side.PlayerID]; previous != nil {
					if !proto.Equal(previous, frame.FightInfo) {
						return nil, nil
					}
				} else {
					hp := frame.GetFightInfo()
					if hp == nil || len(hp.HpPos) == 0 || len(hp.HpPos) != len(hp.MaxHpPos) || hp.GetAnger() != 0 {
						return nil, nil
					}
					for k, value := range hp.HpPos {
						if value <= 0 || value != hp.MaxHpPos[k] || math.IsNaN(value) || math.IsInf(value, 0) {
							return nil, nil
						}
					}
				}
				ledger.Last[side.PlayerID] = proto.Clone(after[j].FightInfo).(*pb.InstFightable)
			}
			if dead != 0 {
				if ledger.PlayerKills[reward] == math.MaxInt32 || ledger.PlayerDeaths[dead] == math.MaxInt32 {
					return nil, errors.New("公会战击杀账本溢出")
				}
				camp := 0
				if guilds[reward] == match.RightGuildID {
					camp = 1
				}
				if ledger.Kills[camp] == math.MaxInt32 {
					return nil, errors.New("公会战阵营击杀溢出")
				}
				ledger.Kills[camp]++
				ledger.Reached[camp] = row.DueAt
				ledger.PlayerKills[reward]++
				ledger.PlayerDeaths[dead]++
				ledger.PlayerScores[reward] += rules.KillScore
			}
			digests = append(digests, row.ID+":"+row.SourceHash)
		}
	}
	// Stable order explicitly closes both simultaneous events and their evidence.
	sort.Strings(digests)
	raw, _ := json.Marshal(struct {
		Match   string
		Roster  string
		Events  []string
		Kills   [2]int32
		Reached [2]int64
	}{match.ID, match.SnapshotHash, digests, ledger.Kills, ledger.Reached})
	ledger.Hash = p6Digest(raw)
	ledger.Proof, _ = json.Marshal(proof)
	return ledger, nil
}

func (s *GameService) p13UFSettlePlayerCombat(tx *gorm.DB, match *db.P13UnionFightMatch, offer *db.P13UnionFightSettlementOffer, now time.Time) (*db.P13UnionFightSettlementResult, error) {
	mixed, mixedErr := p13UFMixedNeeded(tx, match)
	if mixedErr != nil {
		return nil, mixedErr
	}
	frozenRule, ruleErr := p13UFSettlementRules(offer, match)
	if ruleErr != nil {
		return nil, ruleErr
	}
	if mixed || frozenRule.TerminalRule == constUtil.P13UFMixedTerminalRule {
		return s.p13UFSettleMixedCombat(tx, match, offer, now)
	}
	if now.UnixMilli() < match.EndAt {
		return nil, nil
	}
	var saved db.P13UnionFightSettlementResult
	if e := tx.First(&saved, "match_id=?", match.ID).Error; e == nil {
		return &saved, nil
	} else if !errors.Is(e, gorm.ErrRecordNotFound) {
		return nil, e
	}
	rule, e := p13UFSettlementRules(offer, match)
	if e != nil {
		return nil, e
	}
	guilds, ids, powers, e := p13UFCombatRoster(tx, match)
	if e != nil {
		return nil, e
	}
	if guilds == nil {
		return nil, nil
	}
	ledger, e := s.p13UFCombatLedgerRead(tx, match, guilds, ids)
	if e != nil || ledger == nil {
		return nil, e
	}
	winner, reason := p13UFCombatWinner(ledger.Kills, ledger.Reached, powers)
	if winner < 0 {
		return nil, nil
	}
	var actors []db.P13UnionFightFieldPlayer
	if e = tx.Where("match_id=?", match.ID).Order("player_id").Find(&actors).Error; e != nil {
		return nil, e
	}
	awards := []p13UFSettlementAward{}
	joined := map[string]int{}
	seenActors := map[int64]bool{}
	winnerGuild, loserGuild := match.LeftGuildID, match.RightGuildID
	if winner == 1 {
		winnerGuild, loserGuild = loserGuild, winnerGuild
	}
	for _, actor := range actors {
		seenActors[actor.PlayerID] = true
		if actor.GuildID != guilds[actor.PlayerID] || !p13UFSettlementParticipantTimeValid(&actor, match.EndAt) {
			return nil, nil
		}
		frame, e := p13UFFieldReadActor(&actor)
		if e != nil {
			return nil, e
		}
		expectedCamp := int64(0)
		if actor.GuildID == match.RightGuildID {
			expectedCamp = 1
		}
		if actor.Camp != expectedCamp || frame.GetCamp() != int32(expectedCamp) {
			return nil, nil
		}
		_, missionScore, complete, e := p13UFMissionState(tx, match, &actor, rule)
		if e != nil {
			return nil, e
		}
		if frame.GetKillNum() != ledger.PlayerKills[actor.PlayerID] || frame.GetDeadNum() != ledger.PlayerDeaths[actor.PlayerID] || frame.GetOwnScore() != float64(missionScore)+ledger.PlayerScores[actor.PlayerID] || frame.GetState() == 1 || frame.GetState() == 6 || frame.GetState() == 8 {
			return nil, nil
		}
		if last := ledger.Last[actor.PlayerID]; last != nil {
			if !proto.Equal(last, frame.FightInfo) {
				return nil, nil
			}
		} else {
			hp := frame.GetFightInfo()
			if hp == nil || len(hp.HpPos) == 0 || len(hp.HpPos) != len(hp.MaxHpPos) || hp.GetAnger() != 0 {
				return nil, nil
			}
			for i, value := range hp.HpPos {
				if value <= 0 || value != hp.MaxHpPos[i] || math.IsNaN(value) || math.IsInf(value, 0) {
					return nil, nil
				}
			}
		}
		if !p13UFSettlementParticipated(&actor, match.StartAt) {
			continue
		}
		joined[actor.GuildID]++
		rewards := rule.ParticipationRewards
		if actor.GuildID == winnerGuild {
			rewards += ";" + rule.VictoryRewards
		}
		tasks := rule.TaskRewards != "" && complete >= 2
		if tasks {
			rewards += ";" + rule.TaskRewards
		}
		awards = append(awards, p13UFSettlementAward{PlayerID: actor.PlayerID, GuildID: actor.GuildID, Rewards: rewards, Tasks: tasks})
	}
	for id := range ledger.Last {
		if !seenActors[id] {
			return nil, nil
		}
	}
	if joined[match.LeftGuildID] == 0 || joined[match.RightGuildID] == 0 {
		return nil, nil
	}
	result := db.P13UnionFightSettlementResult{LeftKills: ledger.Kills[0], RightKills: ledger.Kills[1], LeftReachedAt: ledger.Reached[0], RightReachedAt: ledger.Reached[1], Reason: reason, LedgerHash: ledger.Hash, CombatProof: ledger.Proof}
	return s.p13UFFinishSettlement(tx, match, offer, rule, actors, awards, winnerGuild, loserGuild, powers, now, result)
}

// Retain the original membership/withdrawal qualification for this first branch.
func p13UFCombatRoster(tx *gorm.DB, match *db.P13UnionFightMatch) (map[int64]string, []int64, []int64, error) {
	if p6Digest(match.Roster) != match.SnapshotHash {
		return nil, nil, nil, errors.New("公会战冻结名单损坏")
	}
	var roster p13UFMatchRoster
	var left, right pb.UFInfoPreView
	if json.Unmarshal(match.Roster, &roster) != nil || proto.Unmarshal(match.LeftPreview, &left) != nil || proto.Unmarshal(match.RightPreview, &right) != nil || left.GetUnionId() != match.LeftGuildID || right.GetUnionId() != match.RightGuildID {
		return nil, nil, nil, errors.New("公会战冻结预览损坏")
	}
	guilds := map[int64]string{}
	ids := []int64{}
	powers := []int64{0, 0}
	allIDs := []int64{}
	legacyIDs := []int64{}
	for _, side := range [][]p13UFMatchFighter{roster.Left, roster.Right} {
		for _, item := range side {
			allIDs = append(allIDs, item.PlayerID)
			if len(item.BattlePayload) == 0 {
				legacyIDs = append(legacyIDs, item.PlayerID)
			}
		}
	}
	members := map[int64]string{}
	for start := 0; start < len(allIDs); start += 128 {
		var batch []db.P13GuildMember
		if e := tx.Select("player_id,guild_id").Where("player_id IN ?", allIDs[start:min(start+128, len(allIDs))]).Find(&batch).Error; e != nil {
			return nil, nil, nil, e
		}
		for _, m := range batch {
			members[m.PlayerID] = m.GuildID
		}
	}
	archived := map[int64]db.P13UnionFightFighter{}
	frozenGuilds := map[string]bool{}
	if len(legacyIDs) > 0 {
		var entries []db.P13UnionFightEntry
		if e := tx.Select("guild_id").Where("day=? AND guild_id IN ? AND frozen_at>0", match.Day, []string{match.LeftGuildID, match.RightGuildID}).Find(&entries).Error; e != nil {
			return nil, nil, nil, e
		}
		for _, entry := range entries {
			frozenGuilds[entry.GuildID] = true
		}
		for start := 0; start < len(legacyIDs); start += 128 {
			var batch []db.P13UnionFightFighter
			if e := tx.Select("player_id,guild_id,snapshot_hash,battle").Where("day=? AND guild_id IN ? AND player_id IN ?", match.Day, []string{match.LeftGuildID, match.RightGuildID}, legacyIDs[start:min(start+128, len(legacyIDs))]).Find(&batch).Error; e != nil {
				return nil, nil, nil, e
			}
			for _, fighter := range batch {
				if _, exists := archived[fighter.PlayerID]; exists {
					return nil, nil, nil, errors.New("公会战归档身份重复")
				}
				archived[fighter.PlayerID] = fighter
			}
		}
	}
	for camp, side := range [][]p13UFMatchFighter{roster.Left, roster.Right} {
		if len(side) == 0 {
			return nil, nil, nil, nil
		}
		guild := match.LeftGuildID
		if camp == 1 {
			guild = match.RightGuildID
		}
		for _, item := range side {
			fighter := item.P13UnionFightFighter
			battle := item.BattlePayload
			if len(battle) == 0 {
				saved, ok := archived[fighter.PlayerID]
				if !ok || !frozenGuilds[guild] || saved.GuildID != guild || saved.SnapshotHash != fighter.SnapshotHash {
					return nil, nil, nil, errors.New("公会战旧冻结归档不一致")
				}
				battle = saved.Battle
			}
			if p6Digest(battle) != fighter.SnapshotHash {
				return nil, nil, nil, errors.New("公会战冻结Battle字节损坏")
			}
			if fighter.PlayerID <= 0 || guilds[fighter.PlayerID] != "" || fighter.GuildID != guild || fighter.AppliedAt <= 0 || fighter.FightPower < 0 || powers[camp] > math.MaxInt64-fighter.FightPower {
				return nil, nil, nil, errors.New("公会战冻结身份无效")
			}
			if members[fighter.PlayerID] != guild {
				return nil, nil, nil, nil
			}
			guilds[fighter.PlayerID] = guild
			ids = append(ids, fighter.PlayerID)
			powers[camp] += fighter.FightPower
		}
	}
	if powers[0] <= 0 || powers[1] <= 0 || powers[0] != left.GetFightPower() || powers[1] != right.GetFightPower() {
		return nil, nil, nil, errors.New("公会战冻结总战力不一致")
	}
	return guilds, ids, powers, nil
}

// Old peaceful rows have no combat metadata and retain their original contract.
func (s *GameService) p13UFCombatSettlementVerify(tx *gorm.DB, match *db.P13UnionFightMatch, result *db.P13UnionFightSettlementResult) error {
	return p13UFCombatProofVerify(tx, match, result)
}

// Monthly ranking is a pure table consumer. Receipt verification reads frozen
// P6 tables and does not need a live GameService table catalog.
func p13UFCombatSettlementVerify(tx *gorm.DB, match *db.P13UnionFightMatch, result *db.P13UnionFightSettlementResult) error {
	return (&GameService{}).p13UFCombatSettlementVerify(tx, match, result)
}
