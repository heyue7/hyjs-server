package service

import (
	"errors"
	"strings"
	"time"

	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	pb "hyjs-server/model/protocol"
)

// p4ArenaWinPeriod is a named local publication template. Existing P1 snapshots
// own the frozen goal, reward and cutoff. No private-server values are invented.
func p4ArenaWinPeriod(now time.Time) db.PeriodInstance {
	local := now.In(time.FixedZone("UTC+8", 8*3600))
	start := time.Date(local.Year(), local.Month(), 1, 0, 0, 0, 0, local.Location())
	end := start.AddDate(0, 1, 0)
	return db.PeriodInstance{ID: constUtil.P4ArenaWinPeriodPrefix + local.Format("200601"), Enabled: false, Title: "竞技胜场福利", Kind: constUtil.P4ArenaWinKind, Mode: "cumulative", Goal: constUtil.P4ArenaWinDefaultGoal, Rewards: constUtil.P4ArenaWinDefaultReward, StartAt: start.UnixMilli(), EndAt: end.UnixMilli(), RewardEndAt: end.UnixMilli(), Source: constUtil.P4ArenaWinSource}
}

// seedP4ArenaWinPeriod uses only existing period tables and preserves an
// operator's publication. A single no-grace month avoids duplicate Type9 tabs.
func (s *GameService) seedP4ArenaWinPeriod(tx *gorm.DB, now time.Time) error {
	cfg := p4ArenaWinPeriod(now)
	if !p4ArenaWinFrozenIdentity(cfg) {
		return errors.New(constUtil.P4ArenaWinConflict)
	}
	if err := s.validateP1Period(cfg); err != nil {
		return err
	}
	return tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&cfg).Error
}

// recordP4VerifiedArenaWin is called only immediately after the existing strict
// p11ReadArenaFightReceipt and whole-response equality check, in that same TX.
// The original paid settlement is still committed atomically with this fact;
// a later grant/save failure rolls everything back. Historical retry paths skip it.
func (s *GameService) recordP4VerifiedArenaWin(tx *gorm.DB, row *db.P11ArenaFightSettlement, verified *pb.SCPvpFightResult, now time.Time) error {
	if !s.P1Ready || !s.P4Ready {
		return nil
	}
	if row == nil || verified == nil || row.PlayerID <= 0 || row.FightID == "" || row.P6RecordID == "" || row.CreatedAt != now.UnixMilli() || row.EnergySpent != constUtil.P11ArenaEnergy || verified.GetFightId() != row.FightID || verified.GetWinPlayerId() != row.WinnerID {
		return errors.New(constUtil.P4ArenaWinConflict)
	}
	raw, err := proto.MarshalOptions{Deterministic: true}.Marshal(verified)
	if err != nil {
		return err
	}
	if p6Digest(raw) != row.ResponseHash || p6Digest(row.Terminal) != row.TerminalHash || row.ScopeProof == "" || row.PolicyHash != p6Digest([]byte(constUtil.P11ArenaFightPolicySource)) {
		return errors.New(constUtil.P4ArenaWinConflict)
	}
	if row.WinnerID != row.PlayerID {
		return nil
	}
	var periods []db.PeriodInstance
	if err = tx.Where("id LIKE ? AND enabled=?", constUtil.P4ArenaWinPeriodPrefix+"%", true).Find(&periods).Error; err != nil {
		return err
	}
	for _, live := range periods {
		frozen, e := s.freezeP1Period(tx, live, false)
		if e != nil {
			return e
		}
		if !p4ArenaWinFrozenIdentity(frozen) {
			return errors.New(constUtil.P4ArenaWinConflict)
		}
	}
	return s.RecordP1Event(tx, row.PlayerID, constUtil.P4ArenaWinFactPrefix+row.FightID, constUtil.P4ArenaWinKind, 1, now)
}

// p4ReconcileArenaWinProgress consumes only actual accepted challenge facts in
// this requested open, unsettled window. Closed months and old untracked paid
// battles never gain synthetic facts. P1 delivery uniqueness remains unchanged.
func (s *GameService) p4ReconcileArenaWinProgress(tx *gorm.DB, progress *db.PeriodProgress, cfg db.PeriodInstance, now int64) error {
	if cfg.Kind != constUtil.P4ArenaWinKind || !strings.HasPrefix(cfg.ID, constUtil.P4ArenaWinPeriodPrefix) || now < cfg.StartAt || now >= cfg.EndAt {
		return nil
	}
	if !p4ArenaWinFrozenIdentity(cfg) {
		return errors.New(constUtil.P4ArenaWinConflict)
	}
	if !tx.Migrator().HasTable(&db.P11ArenaFightSettlement{}) || !tx.Migrator().HasTable(&db.P6BattleRecord{}) {
		return nil
	}
	var settled int64
	if err := tx.Model(&db.PeriodSettlement{}).Where("instance_id=?", cfg.ID).Count(&settled).Error; err != nil {
		return err
	}
	if settled > 0 {
		return nil
	}
	var stats struct {
		Total   int64
		LastDay string
	}
	// Match exact immutable writer identity, creation time, real challenger winner
	// and a computed P6 source. Queries and arbitrary old numeric ranks cannot join.
	query := tx.Table("p1_event_facts AS facts").Select("COUNT(*) AS total, COALESCE(MAX(facts.day), '') AS last_day").Joins("JOIN p11_arena_fight_settlements AS wins ON wins.player_id=facts.player_id AND wins.fight_id=SUBSTR(facts.source_key,?)", len(constUtil.P4ArenaWinFactPrefix)+1).Joins("JOIN p6_battle_records AS records ON records.id=wins.p6_record_id AND records.player_id=wins.player_id AND records.source_fight=wins.fight_id").Where("facts.player_id=? AND facts.kind=? AND facts.source_key LIKE ? AND facts.amount=1 AND facts.occurred_at>=? AND facts.occurred_at<? AND facts.occurred_at<=?", progress.PlayerID, constUtil.P4ArenaWinKind, constUtil.P4ArenaWinFactPrefix+"%", cfg.StartAt, cfg.EndAt, now).Where("wins.winner_id=wins.player_id AND wins.created_at=facts.occurred_at AND wins.energy_spent=? AND wins.policy_hash=? AND records.source=? AND records.status=? AND records.authoritative=? AND records.finished_at=wins.created_at", constUtil.P11ArenaEnergy, p6Digest([]byte(constUtil.P11ArenaFightPolicySource)), constUtil.P11ArenaFightSource, constUtil.P6BattleComputed, true)
	if err := query.Scan(&stats).Error; err != nil {
		return err
	}
	total := stats.Total
	if total <= progress.Value {
		return nil
	}
	if _, err := s.freezeP1Period(tx, cfg, true); err != nil {
		return err
	}
	progress.Value = total
	progress.LastDay = max(progress.LastDay, stats.LastDay)
	if progress.RewardID == 0 && total >= cfg.Goal {
		delivery, err := s.EnqueueRewardNotice(tx, progress.PlayerID, "activity:"+cfg.ID, cfg.Title, cfg.Source, cfg.Rewards, cfg.RewardEndAt)
		if err != nil {
			return err
		}
		progress.RewardID = delivery.ID
	}
	return tx.Save(progress).Error
}

// p4ArenaWinFrozenIdentity allows pure tests and support tools to verify the
// same original Type9 prefix and finite no-grace local window without a DB read.
func p4ArenaWinFrozenIdentity(cfg db.PeriodInstance) bool {
	if !strings.HasPrefix(cfg.ID, constUtil.P4ArenaWinPeriodPrefix) || cfg.Kind != constUtil.P4ArenaWinKind || cfg.Mode != "cumulative" || cfg.Source != constUtil.P4ArenaWinSource || cfg.EndAt != cfg.RewardEndAt {
		return false
	}
	local := time.UnixMilli(cfg.StartAt).In(time.FixedZone("UTC+8", 8*3600))
	end := time.Date(local.Year(), local.Month()+1, 1, 0, 0, 0, 0, local.Location())
	return cfg.ID == constUtil.P4ArenaWinPeriodPrefix+local.Format("200601") && cfg.StartAt > 0 && cfg.StartAt < cfg.EndAt && cfg.EndAt <= end.UnixMilli() && cfg.Goal > 0 && cfg.RankPlaces == 0
}
