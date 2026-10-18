package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"hyjs-server/constUtil"
	"hyjs-server/db"
)

type p11LimitDailyRules struct {
	Policy         db.P11LimitDailyPolicy `json:"policy"`
	OriginalScores string                 `json:"originalScores"`
}

type p11LimitDailyRank struct {
	PlayerID   int64  `json:"playerId"`
	Rank       int64  `json:"rank"`
	Score      int64  `json:"score"`
	Challenges int64  `json:"challenges"`
	Eligible   bool   `json:"eligible"`
	Bonus      int64  `json:"bonus"`
	ScoreProof string `json:"scoreProof"`
}

type p11LimitDailyVerification struct {
	accepted  map[string]*db.P11LimitFightSettlement
	snapshots map[string]*db.P11LimitDailySnapshot
}

// InitializeP11LimitDaily checks the exact seven recovered rows and creates only
// the optional defaultoff operator source. It never publishes a historical day.
func (s *GameService) LoadP11LimitDailyTables(root string) error {
	if err := s.loadP7Table(root, "LimitPvpDailyScore"); err != nil {
		return err
	}
	return s.validateP11LimitDailyTables()
}

func (s *GameService) validateP11LimitDailyTables() error {
	var lines []string
	for _, r := range s.Tables["LimitPvpDailyScore"] {
		lines = append(lines, r["id"]+"|"+r["minRank"]+"|"+r["maxRank"]+"|"+r["rewardScore"])
	}
	if strings.Join(lines, "\n") != constUtil.P11LimitDailyOriginalScores {
		return errors.New(constUtil.P11LimitDailyConfigError)
	}
	return nil
}

func (s *GameService) InitializeP11LimitDaily(ctx context.Context) error {
	if err := s.validateP11LimitDailyTables(); err != nil {
		return err
	}
	tx := db.GetIns().WithContext(ctx)
	if err := tx.AutoMigrate(&db.P11LimitDailyPolicy{}, &db.P11LimitDailyOffer{}, &db.P11LimitDailyChallenge{}, &db.P11LimitDailySnapshot{}, &db.P11LimitDailyAward{}); err != nil {
		return err
	}
	return tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&db.P11LimitDailyPolicy{ID: 1, Version: "local-limit-daily-score-v1", Source: constUtil.P11LimitDailySource, MinChallenges: 1}).Error
}

func p11LimitDailyNow(ctx context.Context, requested time.Time) time.Time {
	if clock, ok := ctx.Value(constUtil.P11LimitDailyClockContextKey{}).(func() time.Time); ok && clock != nil {
		return clock()
	}
	actual := time.Now()
	if requested.After(actual) {
		return requested
	}
	return actual
}

func p11LimitDailyOfferHash(value db.P11LimitDailyOffer) string {
	value.SourceHash = ""
	raw, _ := json.Marshal(value)
	return p6Digest(raw)
}
func p11LimitDailyChallengeHash(value db.P11LimitDailyChallenge) string {
	value.SourceHash = ""
	raw, _ := json.Marshal(value)
	return p6Digest(raw)
}
func p11LimitDailySnapshotHash(value db.P11LimitDailySnapshot) string {
	value.SourceHash = ""
	raw, _ := json.Marshal(value)
	return p6Digest(raw)
}
func p11LimitDailyAwardHash(value db.P11LimitDailyAward) string {
	value.SourceHash = ""
	raw, _ := json.Marshal(value)
	return p6Digest(raw)
}

func p11LimitDailyPolicyValid(p *db.P11LimitDailyPolicy) bool {
	if p == nil || p.ID != 1 || strings.TrimSpace(p.Version) == "" || len(p.Version) > 64 || p.Source != constUtil.P11LimitDailySource || p.PublishedAt <= 0 || p.PublishedAt > p.StartAt || p.EndAt <= p.StartAt || p.EndAt-p.StartAt > 31*86400000 || (p.EndAt-p.StartAt)%86400000 != 0 || p.CutoffSecond < 0 || p.CutoffSecond >= 86400 || p.MinChallenges < 1 || p.MinChallenges > 100 {
		return false
	}
	start, _ := p11ArenaDailyWindow(time.UnixMilli(p.StartAt), p.CutoffSecond)
	return start.UnixMilli() == p.StartAt
}

func p11LimitDailyOfferRules(offer *db.P11LimitDailyOffer) (*p11LimitDailyRules, error) {
	var rules p11LimitDailyRules
	if offer == nil || offer.SourceHash != p11LimitDailyOfferHash(*offer) || json.Unmarshal([]byte(offer.RulesJSON), &rules) != nil || !rules.Policy.Enabled || !p11LimitDailyPolicyValid(&rules.Policy) || rules.OriginalScores != constUtil.P11LimitDailyOriginalScores {
		return nil, errors.New(constUtil.P11LimitDailyConflict)
	}
	start, end := p11ArenaDailyWindow(time.UnixMilli(offer.StartAt), rules.Policy.CutoffSecond)
	if offer.ID != p6Digest([]byte(offer.PeriodID+"/"+offer.Day)) || offer.Day != start.Format("20060102") || offer.StartAt != start.UnixMilli() || offer.EndAt != end.UnixMilli() || offer.StartAt < rules.Policy.StartAt || offer.EndAt > rules.Policy.EndAt || offer.CapturedAt < offer.StartAt || offer.CapturedAt >= offer.EndAt {
		return nil, errors.New(constUtil.P11LimitDailyConflict)
	}
	return &rules, nil
}

func p11LimitDailyBonus(rank int64) int64 {
	for _, line := range strings.Split(constUtil.P11LimitDailyOriginalScores, "\n") {
		parts := strings.Split(line, "|")
		low, _ := strconv.ParseInt(parts[1], 10, 64)
		high, _ := strconv.ParseInt(parts[2], 10, 64)
		bonus, _ := strconv.ParseInt(parts[3], 10, 64)
		if rank >= low && rank <= high {
			return bonus
		}
	}
	return 0
}

// PrepareP11LimitDailyMutation is called inside the existing LimitPolicy row
// lock before any member-point mutation. It first seals due old windows even if
// the operator has since closed or replaced the live publication.
func (s *GameService) PrepareP11LimitDailyMutation(ctx context.Context, tx *gorm.DB, period *db.P11LimitPeriod, requested time.Time) (time.Time, error) {
	now := p11LimitDailyNow(ctx, requested)
	if !tx.Migrator().HasTable(&db.P11LimitDailyPolicy{}) {
		return now, nil
	}
	var cfg db.P11LimitDailyPolicy
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&cfg, 1).Error; err != nil {
		return now, err
	}
	var due []db.P11LimitDailyOffer
	closed := tx.Model(&db.P11LimitDailySnapshot{}).Select("id")
	if err := tx.Where("end_at<=? AND id NOT IN (?)", now.UnixMilli(), closed).Order("end_at,id").Limit(31).Find(&due).Error; err != nil {
		return now, err
	}
	for i := range due {
		if err := s.p11LimitDailyClose(tx, &due[i], now); err != nil {
			return now, err
		}
	}
	if !cfg.Enabled || period == nil {
		return now, nil
	}
	if !p11LimitDailyPolicyValid(&cfg) || cfg.StartAt < period.StartsAt || cfg.EndAt >= period.EndsAt {
		return now, errors.New(constUtil.P11LimitDailyConfigError)
	}
	start, end := p11ArenaDailyWindow(now, cfg.CutoffSecond)
	if start.UnixMilli() < cfg.StartAt || end.UnixMilli() > cfg.EndAt {
		return now, nil
	}
	day := start.Format("20060102")
	id := p6Digest([]byte(period.ID + "/" + day))
	var existing db.P11LimitDailyOffer
	err := tx.First(&existing, "id=?", id).Error
	if err == nil {
		_, err = p11LimitDailyOfferRules(&existing)
		return now, err
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return now, err
	}
	var overlap int64
	if err = tx.Model(&db.P11LimitDailyOffer{}).Where("period_id=? AND start_at<? AND end_at>?", period.ID, end.UnixMilli(), start.UnixMilli()).Count(&overlap).Error; err != nil {
		return now, err
	}
	if overlap != 0 {
		return now, nil
	}
	raw, err := json.Marshal(p11LimitDailyRules{Policy: cfg, OriginalScores: constUtil.P11LimitDailyOriginalScores})
	if err != nil {
		return now, err
	}
	offer := db.P11LimitDailyOffer{ID: id, PeriodID: period.ID, Day: day, StartAt: start.UnixMilli(), EndAt: end.UnixMilli(), RulesJSON: string(raw), PeriodHash: period.RulesHash, CapturedAt: now.UnixMilli()}
	offer.SourceHash = p11LimitDailyOfferHash(offer)
	return now, tx.Create(&offer).Error
}

// RecordP11LimitDailyChallenge journals only a newly accepted mode20 settlement
// in the same serialized transaction, after its immutable original reply exists.
func (s *GameService) RecordP11LimitDailyChallenge(ctx context.Context, tx *gorm.DB, row *db.P11LimitFightSettlement, admitted time.Time) error {
	if !tx.Migrator().HasTable(&db.P11LimitDailyPolicy{}) {
		return nil
	}
	if row == nil || row.PlayerID <= 0 || row.FightID == "" || row.PeriodID == "" || row.ScopeProof == "" || row.ResponseHash != p6Digest(row.Response) || row.TerminalHash != p6Digest(row.Terminal) || admitted.UnixMilli() < row.CreatedAt {
		return errors.New(constUtil.P11LimitDailyConflict)
	}
	fact := db.P11LimitDailyChallenge{FightID: row.FightID, PeriodID: row.PeriodID, PlayerID: row.PlayerID, OpponentID: row.OpponentID, AcceptedAt: admitted.UnixMilli(), ResponseHash: row.ResponseHash, TerminalHash: row.TerminalHash, P6RecordID: row.P6RecordID, ScopeProof: row.ScopeProof}
	if err := tx.WithContext(ctx).Create(&fact).Error; err != nil {
		return err
	}
	fact.SourceHash = p11LimitDailyChallengeHash(fact)
	return tx.Model(&fact).Update("source_hash", fact.SourceHash).Error
}

func (v *p11LimitDailyVerification) acceptedRow(tx *gorm.DB, fact *db.P11LimitDailyChallenge) (*db.P11LimitFightSettlement, error) {
	if found, ok := v.accepted[fact.FightID]; ok {
		return found, nil
	}
	if fact.ID < 1 || fact.SourceHash != p11LimitDailyChallengeHash(*fact) {
		return nil, errors.New(constUtil.P11LimitDailyConflict)
	}
	var row db.P11LimitFightSettlement
	if err := tx.First(&row, "fight_id=?", fact.FightID).Error; err != nil {
		return nil, err
	}
	if row.PeriodID != fact.PeriodID || row.PlayerID != fact.PlayerID || row.OpponentID != fact.OpponentID || row.ResponseHash != fact.ResponseHash || row.TerminalHash != fact.TerminalHash || row.P6RecordID != fact.P6RecordID || row.ScopeProof != fact.ScopeProof || row.CreatedAt > fact.AcceptedAt {
		return nil, errors.New(constUtil.P11LimitDailyConflict)
	}
	reply, err := p11ReadLimitFightReceipt(tx, &row)
	if err != nil {
		return nil, err
	}
	if (row.ScoreDelta != 0 && row.ScoreDelta != 5) || (row.ScoreDelta == 5 && reply.GetWinPlayerId() != row.PlayerID) || (row.ScoreDelta == 0 && reply.GetWinPlayerId() != row.OpponentID) {
		return nil, errors.New(constUtil.P11LimitDailyConflict)
	}
	v.accepted[fact.FightID] = &row
	return &row, nil
}

func p11LimitDailySum(total, delta int64) (int64, error) {
	if total < 0 || delta < 0 || total > math.MaxInt64-delta {
		return 0, errors.New(constUtil.P11LimitDailyConflict)
	}
	return total + delta, nil
}

// pointProof traces the actual current period balance, including legitimate
// daily entry facts, accepted independent battles, and earlier frozen awards.
// One cutoff verification cache prevents decoding the same battle repeatedly.
func (v *p11LimitDailyVerification) pointProof(tx *gorm.DB, period *db.P11LimitPeriod, member *db.P11LimitMember, capturedAt, end int64) (string, error) {
	var total int64
	sources := []string{period.ID, strconv.FormatInt(member.PlayerID, 10)}
	var days []db.P11LimitDaily
	if err := tx.Where("period_id=? AND player_id=? AND created_at<?", period.ID, member.PlayerID, end).Order("day").Find(&days).Error; err != nil {
		return "", err
	}
	for _, day := range days {
		var fact db.EventFact
		key := fmt.Sprintf("p11-limit-entry:%s:%d:%s", period.ID, member.PlayerID, day.Day)
		if err := tx.First(&fact, "player_id=? AND source_key=? AND kind=?", member.PlayerID, key, "limit_arena_daily_entry").Error; err != nil {
			return "", err
		}
		if day.LoginScore != 10 || day.CreatedAt < period.StartsAt || fact.Amount != int64(day.LoginScore) || fact.OccurredAt != day.CreatedAt || fact.Day != day.Day {
			return "", errors.New(constUtil.P11LimitDailyConflict)
		}
		var err error
		total, err = p11LimitDailySum(total, int64(day.LoginScore))
		if err != nil {
			return "", err
		}
		raw, _ := json.Marshal(fact)
		sources = append(sources, p6Digest(raw))
	}
	// Legacy immutable accepted receipts may prove an existing period balance.
	// They never create retrospective admission facts or daily qualification.
	var fights []db.P11LimitDailyChallenge
	if err := tx.Where("period_id=? AND player_id=?", period.ID, member.PlayerID).Order("id").Find(&fights).Error; err != nil {
		return "", err
	}
	journal := map[string]*db.P11LimitDailyChallenge{}
	for i := range fights {
		journal[fights[i].FightID] = &fights[i]
	}
	var rows []db.P11LimitFightSettlement
	if err := tx.Where("period_id=? AND player_id=? AND created_at<?", period.ID, member.PlayerID, end).Order("created_at,stamp").Find(&rows).Error; err != nil {
		return "", err
	}
	for i := range rows {
		row := &rows[i]
		source := ""
		if fact, ok := journal[row.FightID]; ok {
			if fact.AcceptedAt >= end {
				continue
			}
			accepted, err := v.acceptedRow(tx, fact)
			if err != nil {
				return "", err
			}
			row = accepted
			source = fact.SourceHash
		} else {
			if row.CreatedAt >= capturedAt {
				return "", errors.New(constUtil.P11LimitDailyConflict)
			}
			reply, err := p11ReadLimitFightReceipt(tx, row)
			if err != nil {
				return "", err
			}
			if (row.ScoreDelta != 0 && row.ScoreDelta != 5) || (row.ScoreDelta == 5 && reply.GetWinPlayerId() != row.PlayerID) || (row.ScoreDelta == 0 && reply.GetWinPlayerId() != row.OpponentID) {
				return "", errors.New(constUtil.P11LimitDailyConflict)
			}
			v.accepted[row.FightID] = row
			source = p6Digest([]byte(row.ResponseHash + "/" + row.TerminalHash + "/" + row.ScopeProof))
		}
		var err error
		total, err = p11LimitDailySum(total, int64(row.ScoreDelta))
		if err != nil {
			return "", err
		}
		sources = append(sources, source)
	}
	var awards []db.P11LimitDailyAward
	if err := tx.Where("period_id=? AND player_id=?", period.ID, member.PlayerID).Order("created_at,offer_id").Find(&awards).Error; err != nil {
		return "", err
	}
	for _, award := range awards {
		if award.SourceHash != p11LimitDailyAwardHash(award) || award.Bonus != p11LimitDailyBonus(award.Rank) || award.Bonus < 1 || award.AfterScore != award.BeforeScore+award.Bonus {
			return "", errors.New(constUtil.P11LimitDailyConflict)
		}
		snap, ok := v.snapshots[award.OfferID]
		if !ok {
			snap = &db.P11LimitDailySnapshot{}
			if err := tx.First(snap, "id=?", award.OfferID).Error; err != nil {
				return "", err
			}
			if snap.Status != constUtil.P11LimitDailyComplete || snap.SourceHash != p11LimitDailySnapshotHash(*snap) {
				return "", errors.New(constUtil.P11LimitDailyConflict)
			}
			v.snapshots[award.OfferID] = snap
		}
		if snap.PeriodID != period.ID || snap.EndAt > end || snap.SourceHash != award.SnapshotHash {
			return "", errors.New(constUtil.P11LimitDailyConflict)
		}
		var ranks []p11LimitDailyRank
		if json.Unmarshal([]byte(snap.RanksJSON), &ranks) != nil {
			return "", errors.New(constUtil.P11LimitDailyConflict)
		}
		found := false
		for _, rank := range ranks {
			if rank.PlayerID == member.PlayerID && rank.Eligible && rank.Rank == award.Rank && rank.Bonus == award.Bonus && rank.Score == award.BeforeScore {
				found = true
				break
			}
		}
		if !found {
			return "", errors.New(constUtil.P11LimitDailyConflict)
		}
		var err error
		total, err = p11LimitDailySum(total, award.Bonus)
		if err != nil {
			return "", err
		}
		sources = append(sources, award.SourceHash)
	}
	if total != member.Score {
		return "", errors.New(constUtil.P11LimitDailyConflict)
	}
	return p6Digest([]byte(strings.Join(sources, "\n"))), nil
}

func (s *GameService) p11LimitDailyClose(tx *gorm.DB, offer *db.P11LimitDailyOffer, now time.Time) error {
	rules, err := p11LimitDailyOfferRules(offer)
	if err != nil {
		return err
	}
	var period db.P11LimitPeriod
	if err = tx.First(&period, "id=?", offer.PeriodID).Error; err != nil {
		return err
	}
	if period.RulesHash != offer.PeriodHash || period.RulesHash != p6Digest([]byte(period.RulesJSON)) || offer.StartAt < period.StartsAt || offer.EndAt >= period.EndsAt {
		return errors.New(constUtil.P11LimitDailyConflict)
	}
	snapshot := db.P11LimitDailySnapshot{ID: offer.ID, PeriodID: offer.PeriodID, Day: offer.Day, StartAt: offer.StartAt, EndAt: offer.EndAt, OfferHash: offer.SourceHash, Status: constUtil.P11LimitDailyComplete, RanksJSON: "[]", CapturedAt: now.UnixMilli()}
	var facts []db.P11LimitDailyChallenge
	if err = tx.Where("period_id=? AND accepted_at>=? AND accepted_at<?", offer.PeriodID, max(offer.StartAt, offer.CapturedAt), offer.EndAt).Order("id").Find(&facts).Error; err != nil {
		return err
	}
	counts := map[int64]int64{}
	verifier := p11LimitDailyVerification{accepted: map[string]*db.P11LimitFightSettlement{}, snapshots: map[string]*db.P11LimitDailySnapshot{}}
	for i := range facts {
		if _, err = verifier.acceptedRow(tx, &facts[i]); err != nil {
			snapshot.Status = constUtil.P11LimitDailyPending
			snapshot.Reason = "accepted-mode20-proof-missing"
			break
		}
		counts[facts[i].PlayerID]++
	}
	var members []db.P11LimitMember
	if err = tx.Where("period_id=? AND created_at<?", period.ID, offer.EndAt).Order("score DESC,player_id").Find(&members).Error; err != nil {
		return err
	}
	ranks := make([]p11LimitDailyRank, 0, len(members))
	for index := range members {
		member := &members[index]
		rank := p11LimitDailyRank{PlayerID: member.PlayerID, Rank: int64(index + 1), Score: member.Score, Challenges: counts[member.PlayerID]}
		if snapshot.Status == constUtil.P11LimitDailyComplete {
			rank.ScoreProof, err = verifier.pointProof(tx, &period, member, offer.CapturedAt, offer.EndAt)
			if err != nil {
				snapshot.Status = constUtil.P11LimitDailyPending
				snapshot.Reason = "period-score-source-unverified"
			}
		}
		rank.Eligible = counts[member.PlayerID] >= rules.Policy.MinChallenges
		if rank.Eligible {
			rank.Bonus = p11LimitDailyBonus(rank.Rank)
		}
		ranks = append(ranks, rank)
	}
	// Pending captures the actual cutoff order too. Later live points cannot be
	// substituted for this window, and no subset receives a partial award.
	raw, err := json.Marshal(ranks)
	if err != nil {
		return err
	}
	snapshot.RanksJSON = string(raw)
	snapshot.SourceHash = p11LimitDailySnapshotHash(snapshot)
	if err = tx.Create(&snapshot).Error; err != nil {
		return err
	}
	if snapshot.Status != constUtil.P11LimitDailyComplete {
		return nil
	}
	for _, rank := range ranks {
		if !rank.Eligible || rank.Bonus < 1 {
			continue
		}
		after, err := p11LimitDailySum(rank.Score, rank.Bonus)
		if err != nil {
			return err
		}
		key := fmt.Sprintf("p11-limit-daily:%s:%d", offer.ID, rank.PlayerID)
		// Point delivery is an actual transaction now; its historical cutoff
		// remains separately frozen in offer/snapshot, and P1 receives no future event.
		deliveredAt := time.Now()
		award := db.P11LimitDailyAward{OfferID: offer.ID, PlayerID: rank.PlayerID, PeriodID: offer.PeriodID, Day: offer.Day, Rank: rank.Rank, Bonus: rank.Bonus, BeforeScore: rank.Score, AfterScore: after, SnapshotHash: snapshot.SourceHash, SourceKey: key, CreatedAt: deliveredAt.UnixMilli()}
		award.SourceHash = p11LimitDailyAwardHash(award)
		changed := tx.Model(&db.P11LimitMember{}).Where("period_id=? AND player_id=? AND score=?", period.ID, rank.PlayerID, rank.Score).Update("score", after)
		if changed.Error != nil {
			return changed.Error
		}
		if changed.RowsAffected != 1 {
			return errors.New(constUtil.P11LimitDailyConflict)
		}
		if err = tx.Create(&award).Error; err != nil {
			return err
		}
		if s.P1Ready {
			if err = s.RecordP1Event(tx, rank.PlayerID, key, "limit_arena_daily_score", rank.Bonus, deliveredAt); err != nil {
				return err
			}
		}
	}
	return nil
}

// TickP11LimitDaily follows the same period serialization lock as requests;
// offline closure touches only period points and existing P1 source facts.
func (s *GameService) TickP11LimitDaily(ctx context.Context, now time.Time) error {
	return db.GetIns().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := ServerGameClockGuardTransaction(tx); err != nil {
			return err
		}

		var policy db.P11LimitPolicy
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&policy, 1).Error; err != nil {
			return err
		}
		_, err := s.PrepareP11LimitDailyMutation(ctx, tx, nil, now)
		return err
	})
}

func (s *GameService) ServeP11LimitDaily(ctx context.Context) error {
	if err := s.TickP11LimitDaily(ctx, LocalPlayNow(ctx)); err != nil {
		return err
	}
	ticker := time.NewTicker(constUtil.P11LimitDailyWorkerInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case now := <-ticker.C:
			if err := s.TickP11LimitDaily(ctx, LocalPlayAt(ctx, now)); err != nil {
				if ctx.Err() != nil {
					return nil
				}
				return err
			}
		}
	}
}
