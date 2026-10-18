package service

import (
	"errors"
	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	pb "hyjs-server/model/protocol"
	"strings"
	"time"
)

// p4PlunderActivityPeriod seeds one disabled original Type10 entry per month.
// The named local template preserves unknown private-server parameters.
func p4PlunderActivityPeriod(now time.Time) db.PeriodInstance {
	local := now.In(time.FixedZone("UTC+8", 8*3600))
	start := time.Date(local.Year(), local.Month(), 1, 0, 0, 0, 0, local.Location())
	end := start.AddDate(0, 1, 0)
	return db.PeriodInstance{ID: constUtil.P4PlunderActivityPrefix + local.Format("200601"), Enabled: false, Title: "夺宝次数福利", Kind: constUtil.P4PlunderActivityKind, Mode: "cumulative", Goal: constUtil.P4PlunderActivityDefaultGoal, Rewards: constUtil.P4PlunderActivityDefaultReward, StartAt: start.UnixMilli(), EndAt: end.UnixMilli(), RewardEndAt: end.UnixMilli(), Source: constUtil.P4PlunderActivitySource}
}

func p4PlunderActivityIdentity(cfg db.PeriodInstance) bool {
	if !strings.HasPrefix(cfg.ID, constUtil.P4PlunderActivityPrefix) || cfg.Kind != constUtil.P4PlunderActivityKind || cfg.Mode != "cumulative" || cfg.Source != constUtil.P4PlunderActivitySource || cfg.RewardEndAt != cfg.EndAt || cfg.Goal < 1 || cfg.RankPlaces != 0 {
		return false
	}
	local := time.UnixMilli(cfg.StartAt).In(time.FixedZone("UTC+8", 8*3600))
	end := time.Date(local.Year(), local.Month()+1, 1, 0, 0, 0, 0, local.Location())
	return cfg.ID == constUtil.P4PlunderActivityPrefix+local.Format("200601") && cfg.StartAt > 0 && cfg.StartAt < cfg.EndAt && cfg.EndAt <= end.UnixMilli()
}

func (s *GameService) seedP4PlunderActivityPeriod(tx *gorm.DB, now time.Time) error {
	cfg := p4PlunderActivityPeriod(now)
	if !p4PlunderActivityIdentity(cfg) {
		return errors.New(constUtil.P4PlunderActivityConflict)
	}
	if err := s.validateP1Period(cfg); err != nil {
		return err
	}
	return tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&cfg).Error
}

// recordP4VerifiedPlunderAttempt runs after the new accepted settlement is
// inserted in its resource transaction. Only an already open local publication
// gets a fact; retries and earlier generic attempts never backfill this activity.
func (s *GameService) recordP4VerifiedPlunderAttempt(tx *gorm.DB, row *db.P11PlunderFightSettlement, response *pb.SCFightTreasureStartResult, now time.Time) error {
	if !s.P1Ready || !s.P4Ready {
		return nil
	}
	bad := errors.New(constUtil.P4PlunderActivityConflict)
	if row == nil || response == nil || row.PlayerID <= 0 || row.FightID == "" || row.P6RecordID == "" || row.ScopeProof == "" || row.CreatedAt != now.UnixMilli() || row.EnergySpent != 2 || response.GetFightId() != row.FightID || response.GetMaxRoundNum() != 20 || (response.GetIsWin() == 1) != row.AttackerWon || response.GetIsWin() < 0 || response.GetIsWin() > 1 {
		return bad
	}
	raw, err := proto.MarshalOptions{Deterministic: true}.Marshal(response)
	if err != nil {
		return err
	}
	if p6Digest(raw) != row.ResponseHash || p6Digest(row.Terminal) != row.TerminalHash {
		return bad
	}
	var published []db.PeriodInstance
	if err = tx.Where("id LIKE ? AND kind=? AND enabled=?", constUtil.P4PlunderActivityPrefix+"%", constUtil.P4PlunderActivityKind, true).Find(&published).Error; err != nil {
		return err
	}
	var active *db.PeriodInstance
	for _, live := range published {
		cfg, e := s.freezeP1Period(tx, live, false)
		if e != nil {
			return e
		}
		if !p4PlunderActivityIdentity(cfg) {
			return bad
		}
		if now.UnixMilli() < cfg.StartAt || now.UnixMilli() >= cfg.EndAt {
			continue
		}
		if active != nil {
			return bad
		}
		active = &cfg
	}
	if active == nil {
		return nil
	}
	// Bind this activity to the accepted independent type3 witness already
	// committed inside the same resource transaction, before counting a fact.
	var witness db.P6BattleRecord
	if err = tx.Select("id", "player_id", "source", "source_fight", "status", "authoritative", "finished_at").Where("id=?", row.P6RecordID).First(&witness).Error; err != nil {
		return err
	}
	if row.ScopeProof != constUtil.P6PlunderNativeProof || witness.PlayerID != row.PlayerID || witness.Source != constUtil.P11PlunderFightSource || witness.SourceFight != row.FightID || witness.Status != constUtil.P6BattleComputed || !witness.Authoritative || witness.FinishedAt != row.CreatedAt {
		return bad
	}
	return s.RecordP1Event(tx, row.PlayerID, constUtil.P4PlunderActivityFactPrefix+active.ID+":"+row.FightID, constUtil.P4PlunderActivityKind, 1, now)
}

// p4ReconcilePlunderActivityProgress reads only source-bound real accepted
// challenges carrying this exact publication identity. It never adopts generic
// prepublication history, query snapshots or another activity's battle result.
func (s *GameService) p4ReconcilePlunderActivityProgress(tx *gorm.DB, progress *db.PeriodProgress, cfg db.PeriodInstance, now int64) error {
	if cfg.Kind != constUtil.P4PlunderActivityKind || !strings.HasPrefix(cfg.ID, constUtil.P4PlunderActivityPrefix) || now < cfg.StartAt || now >= cfg.EndAt {
		return nil
	}
	if !p4PlunderActivityIdentity(cfg) {
		return errors.New(constUtil.P4PlunderActivityConflict)
	}
	if !tx.Migrator().HasTable(&db.P11PlunderFightSettlement{}) || !tx.Migrator().HasTable(&db.P6BattleRecord{}) {
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
	prefix := constUtil.P4PlunderActivityFactPrefix + cfg.ID + ":"
	query := tx.Table("p1_event_facts AS facts").Select("COUNT(*) AS total, COALESCE(MAX(facts.day),'') AS last_day").Joins("JOIN p11_plunder_fight_settlements AS attempts ON attempts.player_id=facts.player_id AND attempts.fight_id=SUBSTR(facts.source_key,?)", len(prefix)+1).Joins("JOIN p6_battle_records AS records ON records.id=attempts.p6_record_id AND records.player_id=attempts.player_id AND records.source_fight=attempts.fight_id").Where("facts.player_id=? AND facts.kind=? AND facts.source_key LIKE ? AND facts.amount=1 AND facts.occurred_at>=? AND facts.occurred_at<? AND facts.occurred_at<=?", progress.PlayerID, cfg.Kind, prefix+"%", cfg.StartAt, cfg.EndAt, now).Where("attempts.created_at=facts.occurred_at AND attempts.energy_spent=2 AND attempts.scope_proof<>'' AND records.source=? AND records.status=? AND records.authoritative=? AND records.finished_at=attempts.created_at", constUtil.P11PlunderFightSource, constUtil.P6BattleComputed, true)
	if err := query.Scan(&stats).Error; err != nil {
		return err
	}
	if stats.Total <= progress.Value {
		return nil
	}
	if _, err := s.freezeP1Period(tx, cfg, true); err != nil {
		return err
	}
	progress.Value = stats.Total
	progress.LastDay = max(progress.LastDay, stats.LastDay)
	if progress.RewardID == 0 && stats.Total >= cfg.Goal {
		delivery, err := s.EnqueueRewardNotice(tx, progress.PlayerID, "activity:"+cfg.ID, cfg.Title, cfg.Source, cfg.Rewards, cfg.RewardEndAt)
		if err != nil {
			return err
		}
		progress.RewardID = delivery.ID
	}
	return tx.Save(progress).Error
}
