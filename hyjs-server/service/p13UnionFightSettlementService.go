package service

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"sort"
	"strings"
	"sync"
	"time"

	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	pb "hyjs-server/model/protocol"
)

// One bounded round-robin cursor per active connection avoids old unresolved
// legacy offers starving new rounds. A restarted worker safely begins again.
var p13UFSettlementCursor struct {
	sync.Mutex
	DB    *gorm.DB
	EndAt int64
	ID    string
}

type p13UFSettlementAward struct {
	PlayerID int64  `json:"playerId"`
	GuildID  string `json:"guildId"`
	Rewards  string `json:"rewards"`
	MVP      bool   `json:"mvp,omitempty"`
	Tasks    bool   `json:"tasks,omitempty"`
}

func (s *GameService) InitializeP13UnionFightSettlement(ctx context.Context) error {
	tx := db.GetIns().WithContext(ctx)
	if err := tx.AutoMigrate(&db.P13UnionFightSettlementPolicy{}, &db.P13UnionFightSettlementOffer{}, &db.P13UnionFightSettlementResult{}, &db.P13UnionFightSettlementIntent{}, &db.P13UnionFightSettlementPrestige{}, &db.P13UnionFightGuildMissionFact{}); err != nil {
		return err
	}
	for _, group := range []string{"A", "B", "C", "D"} {
		if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&db.P13UnionFightSettlementPolicy{Group: group, Source: constUtil.P13UnionFightSettlementSource}).Error; err != nil {
			return err
		}
	}
	if err := s.InitializeP13UnionFightWeekly(ctx); err != nil {
		return err
	}
	return s.InitializeP13UnionFightMonthly(ctx)
}

// p13UFSettlementFreeze requires an explicit local publication before the real match starts.
// Archived values are never enabled or copied into the current server by initialization.
func (s *GameService) p13UFSettlementFreeze(tx *gorm.DB, match *db.P13UnionFightMatch, now time.Time) error {
	if err := s.p13UFWeeklyFreeze(tx, match, now); err != nil {
		return err
	}
	var existing db.P13UnionFightSettlementOffer
	if err := tx.First(&existing, "match_id=?", match.ID).Error; err == nil {
		return nil
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}
	if now.UnixMilli() >= match.StartAt {
		return nil
	}
	var rule db.P13UnionFightSettlementPolicy
	if err := tx.First(&rule, "`group`=?", match.Group).Error; err != nil {
		return err
	}
	if !rule.Enabled {
		return nil
	}
	if !p13UFAbortRuleValid(&rule) || rule.TerminalRule != "" && rule.TerminalRule != constUtil.P13UFMixedTerminalRule || rule.TerminalRule == "" && rule.DrawPrestige != 0 || rule.TerminalRule == constUtil.P13UFMixedTerminalRule && rule.DrawPrestige != 5 {
		return errors.New(constUtil.P13UFMixedConflict)
	}
	if rule.Version == "" || strings.TrimSpace(rule.Source) == "" || rule.PublishedAt <= 0 || rule.PublishedAt > now.UnixMilli() || rule.PublishedAt >= match.StartAt || rule.ParticipationRewards == "" || rule.VictoryRewards == "" || rule.WinPrestige < 0 || rule.LosePrestige < 0 || rule.FactoryMissionPrestige < 0 || int64(rule.WinPrestige)+int64(rule.FactoryMissionPrestige) > math.MaxInt32 || int64(rule.LosePrestige)+int64(rule.FactoryMissionPrestige) > math.MaxInt32 {
		return errors.New("公会战本地结算奖档尚未提前完整发布")
	}
	original, err := s.p13UFFieldOriginal(match, tx)
	if err != nil {
		return err
	}
	if _, err := p13UFMissionCatalog(&rule); err != nil {
		return err
	}
	rewardLists := []string{rule.ParticipationRewards, rule.VictoryRewards}
	if rule.TaskRewards != "" {
		rewardLists = append(rewardLists, rule.TaskRewards)
	}
	if rule.TransportMVPRewards != "" {
		rewardLists = append(rewardLists, rule.TransportMVPRewards)
	}
	for _, rewards := range rewardLists {
		ok, err := original.openServerRewardSupported(rewards)
		if err != nil {
			return err
		}
		if !ok {
			return errors.New("公会战发布奖励资源尚未恢复")
		}
	}
	raw, err := json.Marshal(rule)
	if err != nil {
		return err
	}
	return tx.Create(&db.P13UnionFightSettlementOffer{AbortRule: rule.AbortRule, FinalizationGraceMillis: rule.FinalizationGraceMillis, MatchID: match.ID, Rules: raw, RulesHash: p6Digest(raw), RosterHash: match.SnapshotHash, FrozenAt: now.UnixMilli(), Source: constUtil.P13UnionFightSettlementSource}).Error
}

func p13UFSettlementRules(offer *db.P13UnionFightSettlementOffer, match *db.P13UnionFightMatch) (*db.P13UnionFightSettlementPolicy, error) {
	var rule db.P13UnionFightSettlementPolicy
	if offer.RosterHash != match.SnapshotHash || p6Digest(offer.Rules) != offer.RulesHash || json.Unmarshal(offer.Rules, &rule) != nil || !rule.Enabled || rule.Group != match.Group || rule.Version == "" || rule.PublishedAt <= 0 || rule.PublishedAt > offer.FrozenAt || offer.FrozenAt >= match.StartAt {
		return nil, errors.New("公会战冻结结算奖档来源无效")
	}
	if !p13UFAbortRuleValid(&rule) || rule.TerminalRule != "" && rule.TerminalRule != constUtil.P13UFMixedTerminalRule || rule.TerminalRule == "" && rule.DrawPrestige != 0 || rule.TerminalRule == constUtil.P13UFMixedTerminalRule && rule.DrawPrestige != 5 {
		return nil, errors.New(constUtil.P13UFMixedConflict)
	}
	if offer.AbortRule != rule.AbortRule || offer.FinalizationGraceMillis != rule.FinalizationGraceMillis {
		return nil, errors.New(constUtil.P13UFMixedConflict)
	}
	return &rule, nil
}

// p13UFObserveFight persists the authenticated field request before the audit path can
// reject, roll back or leave a capture pending. Any such request excludes peaceful victory.
func (s *GameService) p13UFObserveFight(ctx context.Context, id int64, b *pb.CSUFPlayerAskFight, stamp int64, now time.Time) error {
	return db.GetIns().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := ServerGameClockGuardTransaction(tx); err != nil {
			return err
		}

		var lock db.P13GuildPolicy
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&lock, 1).Error; err != nil {
			return err
		}
		var actor db.P13UnionFightFieldPlayer
		if err := tx.Where("player_id=? AND left_at=0", id).Order("joined_at DESC").First(&actor).Error; errors.Is(err, gorm.ErrRecordNotFound) {
			return nil
		} else if err != nil {
			return err
		}
		var match db.P13UnionFightMatch
		if err := tx.First(&match, "id=?", actor.MatchID).Error; err != nil {
			return err
		}
		if now.UnixMilli() < match.StartAt || now.UnixMilli() >= match.EndAt {
			return nil
		}
		var settled int64
		if err := tx.Model(&db.P13UnionFightSettlementResult{}).Where("match_id=?", match.ID).Count(&settled).Error; err != nil {
			return err
		}
		if settled > 0 {
			return errors.New("公会战已经结算")
		}
		row := db.P13UnionFightSettlementIntent{MatchID: match.ID, PlayerID: id, TargetID: b.GetTargetGuid(), Stamp: stamp, ObservedAt: now.UnixMilli(), Source: constUtil.P13UnionFightSettlementSource}
		return tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&row).Error
	})
}

func p13UFSettlementNoP6(tx *gorm.DB, match *db.P13UnionFightMatch, ids []int64) (bool, error) {
	for _, table := range []any{&db.P13UnionFightSettlementIntent{}, &db.P13UnionFightBattleAuditAttempt{}, &db.P13UnionFightGuardAuditAttempt{}} {
		var count int64
		if err := tx.Model(table).Where("match_id=?", match.ID).Count(&count).Error; err != nil {
			return false, err
		}
		if count > 0 {
			return false, nil
		}
	}
	// The first supported terminal branch also excludes persistent item effects:
	// healing must not erase a previous HP change, and bombs must not turn a
	// changed battlefield into an apparently untouched zero-combat round.
	var affected int64
	if err := tx.Model(&db.P13UnionFightItemState{}).Where("match_id=? AND (normal_until>0 OR diamond_until>0)", match.ID).Count(&affected).Error; err != nil {
		return false, err
	}
	if affected > 0 {
		return false, nil
	}
	if err := tx.Model(&db.P13UnionFightGateState{}).Where("match_id=? AND frame_hash<>initial_hash", match.ID).Count(&affected).Error; err != nil {
		return false, err
	}
	if affected > 0 {
		return false, nil
	}
	// Match-bound AppliedProperties also find captured records whose attempt transaction
	// never bound an ID. Other wars or unrelated P6 modes for these accounts do not count.
	var records []db.P6BattleRecord
	if err := tx.Where("player_id IN ? AND source IN ?", ids, []string{constUtil.P13UnionFightBattleAuditP6Source, constUtil.P13UnionFightGuardAuditP6Source}).Find(&records).Error; err != nil {
		return false, err
	}
	for _, record := range records {
		raw, err := decodeP6StoredInput(record.Input)
		if err != nil {
			if record.CreatedAt >= match.StartAt && record.CreatedAt <= match.EndAt {
				return false, nil
			}
			continue
		}
		var input struct {
			AppliedProperties map[string][]string `json:"appliedProperties"`
		}
		if json.Unmarshal(raw, &input) != nil {
			return false, nil
		}
		for _, facts := range input.AppliedProperties {
			for _, fact := range facts {
				if strings.Contains(fact, ";match="+match.ID+";") {
					return false, nil
				}
			}
		}
	}
	return true, nil
}

func (s *GameService) p13UFSettleZeroFight(tx *gorm.DB, match *db.P13UnionFightMatch, offer *db.P13UnionFightSettlementOffer, now time.Time) (*db.P13UnionFightSettlementResult, error) {
	mixed, mixedErr := p13UFMixedNeeded(tx, match)
	if mixedErr != nil {
		return nil, mixedErr
	}
	if mixed {
		return nil, nil
	}
	frozenRule, ruleErr := p13UFSettlementRules(offer, match)
	if ruleErr != nil {
		return nil, ruleErr
	}
	if frozenRule.TerminalRule == constUtil.P13UFMixedTerminalRule {
		return nil, nil
	}

	if now.UnixMilli() < match.EndAt {
		return nil, nil
	}
	var saved db.P13UnionFightSettlementResult
	if err := tx.First(&saved, "match_id=?", match.ID).Error; err == nil {
		return &saved, nil
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}
	rule, err := p13UFSettlementRules(offer, match)
	if err != nil {
		return nil, err
	}
	if p6Digest(match.Roster) != match.SnapshotHash {
		return nil, errors.New("公会战真实报名名单损坏")
	}
	var roster p13UFMatchRoster
	if err = json.Unmarshal(match.Roster, &roster); err != nil {
		return nil, err
	}
	var left, right pb.UFInfoPreView
	if proto.Unmarshal(match.LeftPreview, &left) != nil || proto.Unmarshal(match.RightPreview, &right) != nil || left.GetUnionId() != match.LeftGuildID || right.GetUnionId() != match.RightGuildID {
		return nil, errors.New("公会战冻结双方资料无效")
	}
	ids := []int64{}
	guildByPlayer := map[int64]string{}
	powers := []int64{0, 0}
	for camp, fighters := range [][]p13UFMatchFighter{roster.Left, roster.Right} {
		if len(fighters) == 0 {
			return nil, nil
		}
		guild := match.LeftGuildID
		if camp == 1 {
			guild = match.RightGuildID
		}
		for _, item := range fighters {
			fighter, err := p13UFReadMatchFighter(tx, item)
			if err != nil {
				return nil, err
			}
			if fighter.PlayerID <= 0 || guildByPlayer[fighter.PlayerID] != "" || fighter.GuildID != guild || fighter.AppliedAt <= 0 || fighter.FightPower < 0 || powers[camp] > math.MaxInt64-fighter.FightPower {
				return nil, errors.New("公会战冻结报名身份或总战力无效")
			}
			member, err := p13GuildMember(tx, fighter.PlayerID)
			if err != nil {
				return nil, err
			}
			if member.GuildID != guild {
				return nil, nil
			}
			powers[camp] += fighter.FightPower
			ids = append(ids, fighter.PlayerID)
			guildByPlayer[fighter.PlayerID] = guild
		}
	}
	if powers[0] <= 0 || powers[1] <= 0 || powers[0] != left.GetFightPower() || powers[1] != right.GetFightPower() {
		return nil, errors.New("公会战真实冻结总战力与原预览不一致")
	}
	if powers[0] == powers[1] {
		return nil, nil
	}
	noP6, err := p13UFSettlementNoP6(tx, match, ids)
	if err != nil || !noP6 {
		return nil, err
	}
	var actors []db.P13UnionFightFieldPlayer
	if err = tx.Where("match_id=?", match.ID).Order("player_id").Find(&actors).Error; err != nil {
		return nil, err
	}
	joined := map[string]int{}
	awards := []p13UFSettlementAward{}
	winner, loser := match.LeftGuildID, match.RightGuildID
	if powers[1] < powers[0] {
		winner, loser = loser, winner
	}
	for _, actor := range actors {
		if actor.GuildID != guildByPlayer[actor.PlayerID] || !p13UFSettlementParticipantTimeValid(&actor, match.EndAt) {
			return nil, nil
		}
		frame, err := p13UFFieldReadActor(&actor)
		if err != nil {
			return nil, err
		}
		_, missionScore, missionComplete, err := p13UFMissionState(tx, match, &actor, rule)
		if err != nil {
			return nil, err
		}
		if frame.GetKillNum() != 0 || frame.GetDeadNum() != 0 || frame.GetOwnScore() != float64(missionScore) || frame.GetState() == 1 || frame.GetState() == 6 || frame.GetState() == 3 || frame.GetState() == 8 {
			return nil, nil
		}
		fight := frame.GetFightInfo()
		if len(fight.GetHpPos()) == 0 || len(fight.GetHpPos()) != len(fight.GetMaxHpPos()) {
			return nil, nil
		}
		for i, hp := range fight.GetHpPos() {
			if hp <= 0 || math.IsNaN(hp) || math.IsInf(hp, 0) || hp != fight.GetMaxHpPos()[i] {
				return nil, nil
			}
		}
		if !p13UFSettlementParticipated(&actor, match.StartAt) {
			continue
		}
		joined[actor.GuildID]++
		rewards := rule.ParticipationRewards
		if actor.GuildID == winner {
			rewards += ";" + rule.VictoryRewards
		}
		tasks := rule.TaskRewards != "" && missionComplete >= 2
		if tasks {
			rewards += ";" + rule.TaskRewards
		}
		awards = append(awards, p13UFSettlementAward{PlayerID: actor.PlayerID, GuildID: actor.GuildID, Rewards: rewards, Tasks: tasks})
	}
	if joined[match.LeftGuildID] == 0 || joined[match.RightGuildID] == 0 {
		return nil, nil
	}
	return s.p13UFFinishSettlement(tx, match, offer, rule, actors, awards, winner, loser, powers, now, db.P13UnionFightSettlementResult{})
}

// Both terminal branches share the frozen award, prestige and inbox transaction.
func (s *GameService) p13UFFinishSettlement(tx *gorm.DB, match *db.P13UnionFightMatch, offer *db.P13UnionFightSettlementOffer, rule *db.P13UnionFightSettlementPolicy, actors []db.P13UnionFightFieldPlayer, awards []p13UFSettlementAward, winner, loser string, powers []int64, now time.Time, saved db.P13UnionFightSettlementResult) (*db.P13UnionFightSettlementResult, error) {
	if p13UFAbortEnabled(rule) {
		deadline, err := p13UFAbortDeadline(match, rule)
		if err != nil {
			return nil, err
		}
		if now.UnixMilli() >= deadline || p13UFNow(tx.Statement.Context).UnixMilli() >= deadline {
			return nil, errors.New("公会战最终封账期限已到")
		}
	}
	mvp, err := p13UFTransportMVP(tx, match, actors, awards, rule)
	if err != nil {
		return nil, err
	}
	prestigeWinner, prestigeLoser := winner, loser
	if saved.ResultKind == "draw" {
		if rule.TerminalRule != constUtil.P13UFMixedTerminalRule || rule.DrawPrestige != 5 || winner != "" || loser != "" {
			return nil, errors.New(constUtil.P13UFMixedConflict)
		}
		prestigeWinner, prestigeLoser = match.LeftGuildID, match.RightGuildID
	}
	winTasks, err := p13UFGuildMissionPrestige(tx, match, prestigeWinner)
	if err != nil {
		return nil, err
	}
	loseTasks, err := p13UFGuildMissionPrestige(tx, match, prestigeLoser)
	if err != nil {
		return nil, err
	}
	day, err := time.ParseInLocation("2006-01-02", match.Day, time.FixedZone("Asia/Shanghai", 8*3600))
	if err != nil {
		return nil, err
	}
	awardAt := day.Add(23*time.Hour + 30*time.Minute).UnixMilli()
	if awardAt < match.EndAt {
		return nil, nil
	}
	raw, err := json.Marshal(awards)
	if err != nil {
		return nil, err
	}
	base := db.P13UnionFightSettlementResult{WinTaskPrestige: winTasks, LoseTaskPrestige: loseTasks, MatchID: match.ID, WinnerGuildID: winner, LoserGuildID: loser, LeftPower: powers[0], RightPower: powers[1], EndAt: match.EndAt, SettledAt: now.UnixMilli(), AwardAt: awardAt, OfferHash: offer.RulesHash, Awards: raw, MVP: mvp, Source: constUtil.P13UnionFightSettlementSource + "; publication=" + rule.Version + "; " + rule.Source}
	base.ResultKind, base.TerminalAt = saved.ResultKind, saved.TerminalAt
	base.LeftKills, base.RightKills = saved.LeftKills, saved.RightKills
	base.LeftReachedAt, base.RightReachedAt = saved.LeftReachedAt, saved.RightReachedAt
	base.Reason, base.LedgerHash = saved.Reason, saved.LedgerHash
	base.CombatProof = saved.CombatProof
	if saved.Reason != "" {
		base.Source = constUtil.P13UnionFightCombatSettlementSource + "; publication=" + rule.Version + "; " + rule.Source
	}
	if p13UFMixedSchema(saved.CombatProof) {
		base.Source = constUtil.P13UnionFightMixedSettlementSource + "; publication=" + rule.Version + "; " + rule.Source
	}

	saved = base
	if winTasks > 0 || loseTasks > 0 {
		saved.Source += "; " + constUtil.P13UnionFightGuildMissionSource
	}
	if rule.PersonalMissions != "" {
		saved.Source += "; " + constUtil.P13UnionFightMissionSource
	}
	if rule.TransportMVPRewards != "" {
		saved.Source += "; " + constUtil.P13UnionFightTransportSource
	}
	if saved.Reason != "" {
		saved.CombatProofHash = p13UFCombatProofDigest(&saved)
	}
	if err = tx.Create(&saved).Error; err != nil {
		return nil, err
	}
	prestigeRows := []struct {
		guild string
		delta int32
	}{{winner, rule.WinPrestige}, {loser, rule.LosePrestige}}
	if saved.ResultKind == "draw" {
		prestigeRows = []struct {
			guild string
			delta int32
		}{{match.LeftGuildID, rule.DrawPrestige}, {match.RightGuildID, rule.DrawPrestige}}
	}
	if p13UFAbortEnabled(rule) {
		for i := range prestigeRows {
			extra := winTasks
			if prestigeRows[i].guild == prestigeLoser {
				extra = loseTasks
			}
			if int64(prestigeRows[i].delta)+int64(extra) > math.MaxInt32 {
				return nil, errors.New("公会战任务威望溢出")
			}
			prestigeRows[i].delta += extra
		}
	}
	for _, item := range prestigeRows {
		var prestige db.P13UnionFightSettlementPrestige
		key := p13UFWeek(day)
		err = tx.First(&prestige, "week_start=? AND guild_id=?", key, item.guild).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			prestige = db.P13UnionFightSettlementPrestige{WeekStart: key, GuildID: item.guild}
		} else if err != nil {
			return nil, err
		}
		if prestige.Score < 0 || prestige.Score > math.MaxInt32-int64(item.delta) {
			return nil, errors.New("公会战威望溢出")
		}
		prestige.Score += int64(item.delta)
		if err = tx.Save(&prestige).Error; err != nil {
			return nil, err
		}
	}
	if p13UFAbortEnabled(rule) {
		deadline, err := p13UFAbortDeadline(match, rule)
		if err != nil {
			return nil, err
		}
		if p13UFNow(tx.Statement.Context).UnixMilli() >= deadline {
			return nil, errors.New("公会战最终封账期限已到")
		}
	}
	return &saved, nil
}

// p13UFTransportMVP uses an observed full field lifecycle and actual deposited
// ore, never current spendable balances. The published local v1 policy awards
// only a unique positive leader; unknown tied/zero maxima have no MVP grant.
func p13UFTransportMVP(tx *gorm.DB, match *db.P13UnionFightMatch, actors []db.P13UnionFightFieldPlayer, awards []p13UFSettlementAward, rule *db.P13UnionFightSettlementPolicy) ([]byte, error) {
	if rule.TransportMVPRewards == "" {
		return nil, nil
	}
	var scope db.P13UnionFightTransportScope
	var field db.P13UnionFightField
	if err := tx.First(&scope, "match_id=?", match.ID).Error; err != nil {
		return nil, err
	}
	if err := tx.First(&field, "match_id=?", match.ID).Error; err != nil {
		return nil, err
	}
	if scope.Source != constUtil.P13UnionFightTransportSource || scope.StartedAt <= 0 || scope.StartedAt != field.CreatedAt {
		return nil, errors.New("公会战矿运MVP缺完整场次观察来源")
	}
	byID := map[int64]*db.P13UnionFightFieldPlayer{}
	for i := range actors {
		byID[actors[i].PlayerID] = &actors[i]
	}
	var stats []db.P13UnionFightTransportStat
	if err := tx.Where("match_id=?", match.ID).Order("ore DESC,player_id").Find(&stats).Error; err != nil {
		return nil, err
	}
	var winner *db.P13UnionFightTransportStat
	for i := range stats {
		stat := &stats[i]
		actor := byID[stat.PlayerID]
		if actor == nil || actor.GuildID != stat.GuildID || stat.Ore <= 0 || stat.Deposits <= 0 || stat.UpdatedAt < scope.StartedAt || stat.UpdatedAt >= match.EndAt {
			return nil, errors.New("公会战矿运累计不属于真实本场参与者")
		}
		if i == 0 {
			winner = stat
		} else if stat.Ore == stats[0].Ore {
			winner = nil
		}
	}
	if winner == nil || winner.Ore > math.MaxInt32 {
		return nil, nil
	}
	actor := byID[winner.PlayerID]
	frame, err := p13UFFieldReadActor(actor)
	if err != nil {
		return nil, err
	}
	for i := range awards {
		if awards[i].PlayerID == winner.PlayerID {
			awards[i].MVP = true
			awards[i].Rewards += ";" + rule.TransportMVPRewards
		}
	}
	return proto.Marshal(&pb.UFFightResult{MvpInfos: []*pb.UFFightResultPlayer{{PlayerId: proto.Int64(winner.PlayerID), ServerId: frame.ServerId, UnionId: proto.String(winner.GuildID), NickName: frame.NickName, MainHeroId: frame.HeroId, Type: proto.Int32(constUtil.P13UnionFightTransportMVPSlot), Value: proto.Int32(int32(winner.Ore))}}})
}

// Participation survives a lawful scene exit. JoinedAt is the first real entry;
// reentry retains it and clears LeftAt. Exit metadata never invalidates other
// participants or removes already certified combat history from the terminal.
func p13UFSettlementParticipantTimeValid(actor *db.P13UnionFightFieldPlayer, cutoff int64) bool {
	return actor != nil && actor.JoinedAt > 0 && actor.JoinedAt < cutoff && actor.LeftAt >= 0 && (actor.LeftAt == 0 || actor.LeftAt >= actor.JoinedAt)
}

// A preparation-only visit remains in the authenticated actor directory, but
// cannot supply battle participation or an award when it ended before kickoff.
func p13UFSettlementParticipated(actor *db.P13UnionFightFieldPlayer, startAt int64) bool {
	return actor != nil && !(actor.LeftAt > 0 && actor.LeftAt <= startAt)
}

func (s *GameService) p13UFSettlementDeliver(tx *gorm.DB, result *db.P13UnionFightSettlementResult, now time.Time) error {
	if result.ResultKind == constUtil.P13UFTechnicalAbortKind {
		return nil
	}
	if result.DeliveredAt > 0 || now.UnixMilli() < result.AwardAt {
		return nil
	}
	if result.Reason != "" || result.LedgerHash != "" || len(result.CombatProof) > 0 || result.CombatProofHash != "" || strings.HasPrefix(result.Source, constUtil.P13UnionFightCombatSettlementSource) {
		var match db.P13UnionFightMatch
		if err := tx.First(&match, "id=?", result.MatchID).Error; err != nil {
			return err
		}
		if err := s.p13UFCombatSettlementVerify(tx, &match, result); err != nil {
			return err
		}
	}
	var awards []p13UFSettlementAward
	if err := json.Unmarshal(result.Awards, &awards); err != nil {
		return err
	}
	for _, award := range awards {
		if _, err := s.EnqueueReward(tx, award.PlayerID, "p13-war:"+result.MatchID, "公会战结算", "公会战结算奖励，按本场已发布的奖档发放。", award.Rewards, 0); err != nil {
			return err
		}
	}
	return tx.Model(result).Update("delivered_at", now.UnixMilli()).Error
}

// TickP13UnionFightSettlement shares the existing guild worker and its transaction lock.
// A nonempty combat history or undefined tie stays unresolved and produces no resources.
func (s *GameService) TickP13UnionFightSettlement(ctx context.Context, now time.Time) error {
	return errors.Join(s.p13UFSettlementPrepublish(ctx, now), s.TickP13UnionFightFinalization(ctx, now), s.p13UFSettlementMaintenanceDelivery(ctx, now))
}

// TickP13UnionFightSettlementMaintenance never advances the terminal cursor.
func (s *GameService) TickP13UnionFightSettlementMaintenance(ctx context.Context, now time.Time) error {
	return errors.Join(s.p13UFSettlementPrepublish(ctx, now), s.p13UFSettlementMaintenanceDelivery(ctx, now))
}

func (s *GameService) p13UFSettlementPrepublish(ctx context.Context, now time.Time) error {
	var failures []error
	// Separate bounded prepublication, terminal and delivery directories. Old
	// missing/unconfigured histories cannot force a full roster/blob scan.
	var future []db.P13UnionFightMatch
	if err := db.GetIns().WithContext(ctx).Table("p13_union_fight_matches AS m").Select("m.id").Joins("LEFT JOIN p13_union_fight_settlement_offers AS o ON o.match_id=m.id").Where("o.match_id IS NULL AND m.start_at>?", now.UnixMilli()).Order("m.start_at,m.id").Limit(128).Scan(&future).Error; err != nil {
		return err
	}
	for _, candidate := range future {
		err := db.GetIns().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			if err := ServerGameClockGuardTransaction(tx); err != nil {
				return err
			}
			var gate db.P13GuildPolicy
			if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&gate, 1).Error; err != nil {
				return err
			}
			var match db.P13UnionFightMatch
			if err := tx.First(&match, "id=?", candidate.ID).Error; err != nil {
				return err
			}
			return s.p13UFSettlementFreeze(tx, &match, now)
		})
		if err != nil {
			failures = append(failures, err)
		}
	}
	return errors.Join(failures...)
}

// TickP13UnionFightFinalization is the bounded terminal-only worker entry point.
func (s *GameService) TickP13UnionFightFinalization(ctx context.Context, now time.Time) error {
	var failures []error
	var candidates []db.P13UnionFightMatch
	p13UFSettlementCursor.Lock()
	if p13UFSettlementCursor.DB != db.GetIns() {
		p13UFSettlementCursor.DB = db.GetIns()
		p13UFSettlementCursor.EndAt = 0
		p13UFSettlementCursor.ID = ""
	}
	q := db.GetIns().WithContext(ctx).Table("p13_union_fight_matches AS m").Select("m.id,m.end_at").Joins("JOIN p13_union_fight_settlement_offers AS o ON o.match_id=m.id").Joins("LEFT JOIN p13_union_fight_settlement_results AS r ON r.match_id=m.id").Where("r.match_id IS NULL AND m.start_at<=?", now.UnixMilli())
	if p13UFSettlementCursor.ID != "" {
		q = q.Where("m.end_at>? OR (m.end_at=? AND m.id>?)", p13UFSettlementCursor.EndAt, p13UFSettlementCursor.EndAt, p13UFSettlementCursor.ID)
	}
	err := q.Order("m.end_at,m.id").Limit(128).Scan(&candidates).Error
	if err == nil && len(candidates) > 0 {
		last := candidates[len(candidates)-1]
		p13UFSettlementCursor.EndAt = last.EndAt
		p13UFSettlementCursor.ID = last.ID
	}
	if err == nil && len(candidates) < 128 {
		p13UFSettlementCursor.EndAt = 0
		p13UFSettlementCursor.ID = ""
	}
	p13UFSettlementCursor.Unlock()
	if err != nil {
		return err
	}
	// No terminal source and a future logical End cannot produce a result.
	// Authenticate the bounded small base directory before taking the global gate.
	scopes := make([]string, 0, len(candidates))
	for _, candidate := range candidates {
		if now.UnixMilli() < candidate.EndAt {
			scopes = append(scopes, candidate.ID)
		}
	}
	terminals, terminalErr := p13UFMixedTerminalBatch(db.GetIns().WithContext(ctx), scopes)
	if terminalErr != nil {
		// An invalid early-base proof stays fail-closed. End/deadline candidates
		// still reach their existing full verifier or immutable abort seal.
		failures = append(failures, terminalErr)
	}
	for _, candidate := range candidates {
		if now.UnixMilli() < candidate.EndAt && (terminalErr != nil || terminals[candidate.ID] == 0) {
			continue
		}
		if err := s.p13UFSettleMatchCandidate(ctx, candidate.ID, now); err != nil {
			failures = append(failures, err)
		}
	}
	return errors.Join(failures...)
}

// All worker and request finalization uses this exact fence/transaction path.
func (s *GameService) p13UFSettleMatchCandidate(ctx context.Context, matchID string, now time.Time) error {
	// Fence precedes database locks; publication never takes this writer while
	// holding a guild lock. It remains held through the actual commit.
	var aborted *db.P13UnionFightSettlementResult
	p13UFFinalizationFence.Lock()
	err := db.GetIns().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := ServerGameClockGuardTransaction(tx); err != nil {
			return err
		}
		var gate db.P13GuildPolicy
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&gate, 1).Error; err != nil {
			return err
		}
		var match db.P13UnionFightMatch
		if err := tx.Clauses(clause.Locking{Strength: "SHARE"}).First(&match, "id=?", matchID).Error; err != nil {
			return err
		}
		var existing db.P13UnionFightSettlementResult
		err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&existing, "match_id=?", match.ID).Error
		if err == nil {
			return nil
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		var offer db.P13UnionFightSettlementOffer
		if err = tx.Clauses(clause.Locking{Strength: "SHARE"}).First(&offer, "match_id=?", match.ID).Error; err != nil {
			return err
		}
		abortRule, err := p13UFAbortFrozenRule(&offer, &match)
		if err != nil {
			return err
		}
		effectiveNow := now
		if abortRule != nil && p13UFNow(ctx).After(effectiveNow) {
			effectiveNow = p13UFNow(ctx)
		}
		if abortRule != nil {
			deadline, err := p13UFAbortDeadline(&match, abortRule)
			if err != nil {
				return err
			}
			if effectiveNow.UnixMilli() >= deadline {
				aborted, err = s.p13UFSealTechnicalAbort(tx, &match, &offer, abortRule, effectiveNow)
				return err
			}
		}
		result, err := s.p13UFSettleZeroFight(tx, &match, &offer, effectiveNow)
		if err == nil && result == nil {
			result, err = s.p13UFSettlePlayerCombat(tx, &match, &offer, effectiveNow)
		}
		if err != nil || result == nil {
			return err
		}
		return s.p13UFSettlementDeliver(tx, result, effectiveNow)
	})
	p13UFFinalizationFence.Unlock()
	if err == nil && aborted != nil {
		s.p13UFTechnicalAbortPublish(ctx, aborted)
	}

	return err
}

func (s *GameService) p13UFSettlementMaintenanceDelivery(ctx context.Context, now time.Time) error {
	var failures []error
	var deliveries []db.P13UnionFightSettlementResult
	if err := db.GetIns().WithContext(ctx).Select("match_id").Where("delivered_at=0 AND award_at<=? AND result_kind<>?", now.UnixMilli(), constUtil.P13UFTechnicalAbortKind).Order("award_at,match_id").Limit(128).Find(&deliveries).Error; err != nil {
		return err
	}
	for _, row := range deliveries {
		err := db.GetIns().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			if err := ServerGameClockGuardTransaction(tx); err != nil {
				return err
			}
			var gate db.P13GuildPolicy
			if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&gate, 1).Error; err != nil {
				return err
			}
			var result db.P13UnionFightSettlementResult
			if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&result, "match_id=?", row.MatchID).Error; err != nil {
				return err
			}
			return s.p13UFSettlementDeliver(tx, &result, now)
		})
		if err != nil {
			failures = append(failures, err)
		}
	}
	if err := s.TickP13UnionFightWeekly(ctx, now); err != nil {
		failures = append(failures, err)
	}
	if err := s.TickP13UnionFightMonthly(ctx, now); err != nil {
		failures = append(failures, err)
	}
	return errors.Join(failures...)
}

func p13UFSettlementFrame(result *db.P13UnionFightSettlementResult, rule *db.P13UnionFightSettlementPolicy, id int64) (*pb.SCUnionFightResult, error) {
	if result.ResultKind == constUtil.P13UFTechnicalAbortKind {
		return nil, errors.New(constUtil.P13UFTechnicalAbortMessage)
	}
	var awards []p13UFSettlementAward
	if err := json.Unmarshal(result.Awards, &awards); err != nil {
		return nil, err
	}
	var own *p13UFSettlementAward
	for i := range awards {
		if awards[i].PlayerID == id {
			own = &awards[i]
			break
		}
	}
	if own == nil {
		return nil, errors.New("未实际参加这场公会战")
	}
	frame := &pb.UFFightResult{FightId: proto.String(result.MatchID), WinUnionId: proto.String(result.WinnerGuildID), LoseUnionId: proto.String(result.LoserGuildID), WinUnionReward: proto.Int32(rule.WinPrestige + result.WinTaskPrestige), LoseUnionReward: proto.Int32(rule.LosePrestige + result.LoseTaskPrestige)}
	if len(result.MVP) > 0 {
		var stored pb.UFFightResult
		if err := proto.Unmarshal(result.MVP, &stored); err != nil {
			return nil, err
		}
		frame.MvpInfos = stored.MvpInfos
	}
	for index, reward := range []string{rule.ParticipationRewards, rule.VictoryRewards} {
		resources, err := p1RewardResources(reward)
		if err != nil {
			return nil, err
		}
		items := []*pb.SimpleItem{}
		for _, item := range resources {
			items = append(items, &pb.SimpleItem{Id: item.Id, Num: item.Num})
		}
		success := int32(1)
		if index == 1 && own.GuildID != result.WinnerGuildID {
			success = 0
			items = nil
		}
		frame.SelfAchieves = append(frame.SelfAchieves, &pb.UFFightResultSelfAchieve{Type: proto.Int32(int32(index)), IsSuccess: proto.Int32(success), Rewards: items})
	}
	if rule.TransportMVPRewards != "" {
		award := &pb.UFFightResultSelfAchieve{Type: proto.Int32(2), IsSuccess: proto.Int32(0)}
		if own.MVP {
			award.IsSuccess = proto.Int32(1)
			resources, err := p1RewardResources(rule.TransportMVPRewards)
			if err != nil {
				return nil, err
			}
			for _, item := range resources {
				award.Rewards = append(award.Rewards, &pb.SimpleItem{Id: item.Id, Num: item.Num})
			}
		}
		frame.SelfAchieves = append(frame.SelfAchieves, award)
	}
	if rule.TaskRewards != "" {
		award := &pb.UFFightResultSelfAchieve{Type: proto.Int32(3), IsSuccess: proto.Int32(0)}
		if own.Tasks {
			award.IsSuccess = proto.Int32(1)
			resources, err := p1RewardResources(rule.TaskRewards)
			if err != nil {
				return nil, err
			}
			for _, item := range resources {
				award.Rewards = append(award.Rewards, &pb.SimpleItem{Id: item.Id, Num: item.Num})
			}
		}
		frame.SelfAchieves = append(frame.SelfAchieves, award)
	}
	return &pb.SCUnionFightResult{Result: frame}, nil
}

// A request may help only its actual latest entered match, using the worker path.
func (s *GameService) p13UFFinalizeEnteredMatch(ctx context.Context, matchID string, now time.Time) error {
	var candidates []db.P13UnionFightMatch
	err := db.GetIns().WithContext(ctx).Table("p13_union_fight_matches AS m").Select("m.id,m.end_at").Joins("JOIN p13_union_fight_settlement_offers AS o ON o.match_id=m.id").Joins("LEFT JOIN p13_union_fight_settlement_results AS r ON r.match_id=m.id").Where("m.id=? AND m.start_at<=? AND r.match_id IS NULL", matchID, now.UnixMilli()).Limit(1).Scan(&candidates).Error
	if err != nil || len(candidates) == 0 {
		return err
	}
	if now.UnixMilli() < candidates[0].EndAt {
		terminal, err := p13UFMixedTerminalBatch(db.GetIns().WithContext(ctx), []string{matchID})
		if err != nil {
			return err
		}
		if terminal[matchID] == 0 {
			return nil
		}
	}
	return s.p13UFSettleMatchCandidate(ctx, matchID, now)
}

// Field membership provides the bounded result directory, never a global history scan.
func p13UFEnteredSettlementResults(tx *gorm.DB, id int64) ([]db.P13UnionFightSettlementResult, map[string]db.P13UnionFightMatch, map[string]db.P13UnionFightSettlementOffer, error) {
	results := []db.P13UnionFightSettlementResult{}
	matches := map[string]db.P13UnionFightMatch{}
	offers := map[string]db.P13UnionFightSettlementOffer{}
	cursor := ""
	for {
		var actors []db.P13UnionFightFieldPlayer
		if err := tx.Select("match_id").Where("player_id=? AND match_id>?", id, cursor).Order("match_id").Limit(128).Find(&actors).Error; err != nil {
			return nil, nil, nil, err
		}
		ids := make([]string, 0, len(actors))
		for _, actor := range actors {
			ids = append(ids, actor.MatchID)
			cursor = actor.MatchID
		}
		if len(ids) > 0 {
			var batch []db.P13UnionFightSettlementResult
			if err := tx.Where("match_id IN ?", ids).Find(&batch).Error; err != nil {
				return nil, nil, nil, err
			}
			results = append(results, batch...)
			var ms []db.P13UnionFightMatch
			if err := tx.Where("id IN ?", ids).Find(&ms).Error; err != nil {
				return nil, nil, nil, err
			}
			for _, m := range ms {
				matches[m.ID] = m
			}
			var os []db.P13UnionFightSettlementOffer
			if err := tx.Where("match_id IN ?", ids).Find(&os).Error; err != nil {
				return nil, nil, nil, err
			}
			for _, o := range os {
				offers[o.MatchID] = o
			}
		}
		if len(actors) < 128 {
			break
		}
	}
	sort.Slice(results, func(i, j int) bool {
		if results[i].EndAt != results[j].EndAt {
			return results[i].EndAt > results[j].EndAt
		}
		return results[i].MatchID < results[j].MatchID
	})
	return results, matches, offers, nil
}

func (s *GameService) p13UFSettlementQuery(ctx context.Context, id int64, request proto.Message, now time.Time) ([]proto.Message, error) {
	var latest db.P13UnionFightFieldPlayer
	if _, ok := request.(*pb.CSUFFightResult); ok {
		if err := db.GetIns().WithContext(ctx).Select("match_id,joined_at").Where("player_id=?", id).Order("joined_at DESC,match_id DESC").First(&latest).Error; err != nil {
			return nil, err
		}
		if err := s.p13UFFinalizeEnteredMatch(ctx, latest.MatchID, now); err != nil {
			return nil, err
		}
	}
	var response proto.Message
	err := db.GetIns().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := ServerGameClockGuardTransaction(tx); err != nil {
			return err
		}

		var results []db.P13UnionFightSettlementResult
		var resultMatches map[string]db.P13UnionFightMatch
		var resultOffers map[string]db.P13UnionFightSettlementOffer
		if _, ok := request.(*pb.CSUFFightResult); ok {
			var result db.P13UnionFightSettlementResult
			if err := tx.First(&result, "match_id=?", latest.MatchID).Error; err != nil {
				if errors.Is(err, gorm.ErrRecordNotFound) {
					return errors.New("公会战尚无可核实的完整终局")
				}
				return err
			}
			results = append(results, result)
		} else {
			var err error
			results, resultMatches, resultOffers, err = p13UFEnteredSettlementResults(tx, id)
			if err != nil {
				return err
			}
		}
		if _, ok := request.(*pb.CSUFFightResult); ok {
			// The original empty request belongs to the player's latest entered
			// match. A pending second round must not replay the first round's win.
			for _, result := range results {
				if result.MatchID != latest.MatchID {
					continue
				}
				if result.ResultKind == constUtil.P13UFTechnicalAbortKind {
					var match db.P13UnionFightMatch
					if err := tx.First(&match, "id=?", result.MatchID).Error; err != nil {
						return err
					}
					if err := p13UFTechnicalAbortVerify(tx, &match, &result); err != nil {
						return err
					}
					var proof p13UFAbortProof
					if json.Unmarshal(result.CombatProof, &proof) != nil {
						return errors.New("公会战中止参与目录损坏")
					}
					for _, participant := range proof.Participants {
						if participant.PlayerID == id {
							response = &pb.SCUFMessage{Msg: proto.String(constUtil.P13UFTechnicalAbortMessage)}
							return nil
						}
					}
					continue
				}
				var awards []p13UFSettlementAward
				if json.Unmarshal(result.Awards, &awards) != nil {
					return errors.New("公会战奖励名单损坏")
				}
				found := false
				for _, award := range awards {
					found = found || award.PlayerID == id
				}
				if !found {
					continue
				}
				var match db.P13UnionFightMatch
				var offer db.P13UnionFightSettlementOffer
				if err := tx.First(&match, "id=?", result.MatchID).Error; err != nil {
					return err
				}
				if err := tx.First(&offer, "match_id=?", result.MatchID).Error; err != nil {
					return err
				}
				rule, err := p13UFSettlementRules(&offer, &match)
				if err != nil {
					return err
				}
				if err := s.p13UFCombatSettlementVerify(tx, &match, &result); err != nil {
					return err
				}
				frame, err := p13UFSettlementFrame(&result, rule, id)
				response = frame
				return err
			}
			return errors.New("公会战尚无可核实的完整终局")
		}
		member, err := p13GuildMember(tx, id)
		if err != nil {
			return err
		}
		if member.GuildID == "" {
			return errors.New("当前没有公会")
		}
		reply := &pb.SCUFUnionResultInfo{}
		for _, result := range results {
			match, ok := resultMatches[result.MatchID]
			if !ok {
				return gorm.ErrRecordNotFound
			}
			if member.GuildID != match.LeftGuildID && member.GuildID != match.RightGuildID {
				continue
			}
			if result.ResultKind == constUtil.P13UFTechnicalAbortKind {
				if err := p13UFTechnicalAbortVerify(tx, &match, &result); err != nil {
					return err
				}
				continue
			}
			var own, enemy pb.UFInfoPreView
			ownRaw, enemyRaw := match.LeftPreview, match.RightPreview
			if member.GuildID == match.RightGuildID {
				ownRaw, enemyRaw = enemyRaw, ownRaw
			}
			if proto.Unmarshal(ownRaw, &own) != nil || proto.Unmarshal(enemyRaw, &enemy) != nil {
				return errors.New("公会战原结果资料损坏")
			}
			offer, ok := resultOffers[match.ID]
			if !ok {
				return gorm.ErrRecordNotFound
			}
			rule, err := p13UFSettlementRules(&offer, &match)
			if err != nil {
				return err
			}
			if err := s.p13UFCombatSettlementVerify(tx, &match, &result); err != nil {
				return err
			}
			won := int32(0)
			a, b := rule.LosePrestige+result.LoseTaskPrestige, rule.WinPrestige+result.WinTaskPrestige
			if member.GuildID == result.WinnerGuildID {
				won = 1
				a, b = b, a
			}
			ownKills, enemyKills := result.LeftKills, result.RightKills
			if member.GuildID == match.RightGuildID {
				ownKills, enemyKills = enemyKills, ownKills
			}
			reply.Reports = append(reply.Reports, &pb.UFFightReport{FightId: proto.String(match.ID), WeekId: proto.String(match.Day), RoundId: proto.Int32(match.Round), FightEndTime: proto.Int64(match.EndAt), Result: proto.Int32(won), UnionId1: own.UnionId, UnionId2: enemy.UnionId, UnionName1: own.UnionName, UnionName2: enemy.UnionName, ServerId1: own.ServerId, ServerId2: enemy.ServerId, KillNum1: proto.Int32(ownKills), KillNum2: proto.Int32(enemyKills), AddPrestige1: proto.Int32(a), AddPrestige2: proto.Int32(b)})
		}
		response = reply
		return nil
	})
	if err != nil {
		return nil, err
	}
	return []proto.Message{response}, nil
}
