package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"hyjs-server/constUtil"
	"hyjs-server/db"
)

type p11ArenaDailyRules struct {
	Policy           db.P11ArenaDailyPolicy `json:"policy"`
	OriginalRewards  string                 `json:"originalRewards"`
	DeleteRewardDays int64                  `json:"deleteRewardDays"`
}
type p11ArenaDailyRank struct {
	PlayerID   int64  `json:"playerId"`
	Rank       int64  `json:"rank"`
	Challenges int64  `json:"challenges"`
	RankProof  string `json:"rankProof"`
	Rewards    string `json:"rewards"`
}

// LoadP11ArenaDailyTables restores the seven original tiers without introducing
// new quantities or a private-server cutoff that the recovered tables omit.
func (s *GameService) LoadP11ArenaDailyTables(root string) error {
	if err := s.loadP7Table(root, "PvpRankReward"); err != nil {
		return err
	}
	var lines []string
	for _, r := range s.Tables["PvpRankReward"] {
		if r["id"] != "" {
			lines = append(lines, r["id"]+"|"+r["rank"]+"|"+r["reward"])
		}
	}
	if strings.Join(lines, "\n") != constUtil.P11ArenaDailyOriginalRewards {
		return errors.New(constUtil.P11ArenaDailyConfigError)
	}
	return nil
}

// InitializeP11ArenaDaily publishes the user-authorized permanent midnight source
// only for the untouched default. Existing operator windows and accepted facts are retained.
func (s *GameService) InitializeP11ArenaDaily(ctx context.Context) error {
	tx := db.GetIns().WithContext(ctx)
	if err := tx.AutoMigrate(&db.P11ArenaDailyPolicy{}, &db.P11ArenaDailyOffer{}, &db.P11ArenaDailySnapshot{}, &db.P11ArenaDailyAward{}, &db.P11ArenaDailyChallenge{}); err != nil {
		return err
	}
	policy := db.P11ArenaDailyPolicy{ID: constUtil.P11ArenaDailyPolicyID, Version: "local-arena-daily-rank-v1", Source: constUtil.P11ArenaDailySource, MinChallenges: 1}
	if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&policy).Error; err != nil {
		return err
	}
	return tx.Transaction(func(tx *gorm.DB) error {
		var current db.P11ArenaDailyPolicy
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&current, 1).Error; err != nil {
			return err
		}
		// Only the untouched historical default adopts the newly authorized midnight source.
		if current.Permanent || current.Enabled || current.Version != "local-arena-daily-rank-v1" || current.Source != constUtil.P11ArenaDailySource || current.PublishedAt != 0 || current.StartAt != 0 || current.EndAt != 0 || current.CutoffSecond != 0 || current.MinChallenges != 1 {
			return nil
		}
		now := time.Now()
		start, _ := p11ArenaDailyWindow(now, 0)
		return tx.Model(&current).Updates(map[string]any{"enabled": true, "permanent": true, "version": constUtil.P11ArenaDailyPermanentVersion, "source": constUtil.P11ArenaDailyPermanentSource, "published_at": now.UnixMilli(), "start_at": start.UnixMilli(), "end_at": int64(0), "cutoff_second": int64(0)}).Error
	})
}

func p11ArenaDailyNow(ctx context.Context, requested time.Time) time.Time {
	if clock, ok := ctx.Value(constUtil.P11ArenaDailyClockContextKey{}).(func() time.Time); ok && clock != nil {
		return clock()
	}
	if ServerGameClockContextBound(ctx) {
		revision, _ := ctx.Value(constUtil.ServerGameClockRevisionKey).(int64)
		if revision > 0 {
			return LocalPlayNow(ctx)
		}
	}
	actual := time.Now()
	if requested.After(actual) {
		return requested
	}
	return actual
}
func p11ArenaDailyWindow(at time.Time, cutoff int64) (time.Time, time.Time) {
	local := at.In(time.FixedZone("UTC+8", 8*3600))
	start := time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, local.Location()).Add(time.Duration(cutoff) * time.Second)
	if local.Before(start) {
		start = start.AddDate(0, 0, -1)
	}
	return start, start.AddDate(0, 0, 1)
}
func p11ArenaDailyPolicyValid(p *db.P11ArenaDailyPolicy) bool {
	if p != nil && p.Permanent {
		if p.ID != 1 || p.Version != constUtil.P11ArenaDailyPermanentVersion || p.Source != constUtil.P11ArenaDailyPermanentSource || p.PublishedAt <= 0 || p.EndAt != 0 || p.CutoffSecond != 0 || p.MinChallenges < 1 || p.MinChallenges > 100 {
			return false
		}
		start, end := p11ArenaDailyWindow(time.UnixMilli(p.PublishedAt), 0)
		return p.StartAt == start.UnixMilli() && p.PublishedAt >= p.StartAt && p.PublishedAt < end.UnixMilli()
	}

	if p == nil || p.ID != 1 || strings.TrimSpace(p.Version) == "" || len(p.Version) > 64 || p.Source != constUtil.P11ArenaDailySource || p.PublishedAt <= 0 || p.PublishedAt > p.StartAt || p.StartAt >= p.EndAt || p.CutoffSecond < 0 || p.CutoffSecond >= 86400 || p.MinChallenges < 1 || p.MinChallenges > 100 {
		return false
	}
	start, end := p11ArenaDailyWindow(time.UnixMilli(p.StartAt), p.CutoffSecond)
	if start.UnixMilli() != p.StartAt || p.EndAt-p.StartAt > constUtil.P11ArenaDailyWindowLimit*86400000 || (p.EndAt-p.StartAt)%86400000 != 0 || end.Year() < 2020 || end.Year() > 2100 {
		return false
	}
	return true
}
func p11ArenaDailyOfferHash(o db.P11ArenaDailyOffer) string {
	o.SourceHash = ""
	b, _ := json.Marshal(o)
	return p6Digest(b)
}
func p11ArenaDailySnapshotHash(o db.P11ArenaDailySnapshot) string {
	o.SourceHash = ""
	b, _ := json.Marshal(o)
	return p6Digest(b)
}
func p11ArenaDailyChallengeHash(o db.P11ArenaDailyChallenge) string {
	o.SourceHash = ""
	b, _ := json.Marshal(o)
	return p6Digest(b)
}
func p11ArenaDailyAwardHash(o db.P11ArenaDailyAward) string {
	o.SourceHash = ""
	b, _ := json.Marshal(o)
	return p6Digest(b)
}
func p11ArenaDailyOfferRules(o *db.P11ArenaDailyOffer) (*p11ArenaDailyRules, error) {
	var r p11ArenaDailyRules
	if o == nil || o.SourceHash != p11ArenaDailyOfferHash(*o) || json.Unmarshal([]byte(o.RulesJSON), &r) != nil || !r.Policy.Enabled || !p11ArenaDailyPolicyValid(&r.Policy) || r.OriginalRewards != constUtil.P11ArenaDailyOriginalRewards || r.DeleteRewardDays != 14 {
		return nil, errors.New(constUtil.P11ArenaDailyConflict)
	}
	start, end := p11ArenaDailyWindow(time.UnixMilli(o.StartAt), r.Policy.CutoffSecond)
	if o.Day != start.Format("20060102") || start.UnixMilli() != o.StartAt || end.UnixMilli() != o.EndAt || o.StartAt < r.Policy.StartAt || (!r.Policy.Permanent && o.EndAt > r.Policy.EndAt) || o.CapturedAt < max(o.StartAt, r.Policy.PublishedAt) || o.CapturedAt >= o.EndAt {
		return nil, errors.New(constUtil.P11ArenaDailyConflict)
	}
	return &r, nil
}
func p11ArenaDailyReward(rank int64) string {
	for _, line := range strings.Split(constUtil.P11ArenaDailyOriginalRewards, "\n") {
		p := strings.Split(line, "|")
		bounds := strings.Split(p[1], "-")
		first, _ := strconv.ParseInt(bounds[0], 10, 64)
		last := first
		if len(bounds) == 2 {
			last, _ = strconv.ParseInt(bounds[1], 10, 64)
		}
		if rank >= first && rank <= last {
			return p[2]
		}
	}
	return ""
}

// PrepareP11ArenaDailyMutation runs while the caller holds the existing arena
// policy row lock, before roster or rank changes. A second call after computation
// provides the actual admission time even when a request crossed the cutoff.
func (s *GameService) PrepareP11ArenaDailyMutation(ctx context.Context, tx *gorm.DB, requested time.Time) (time.Time, error) {
	now := p11ArenaDailyNow(ctx, requested)
	tx = tx.WithContext(ctx)
	if !gameSchemaHasTable(tx, &db.P11ArenaDailyPolicy{}) {
		return now, nil
	}
	var policy db.P11ArenaDailyPolicy
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&policy, 1).Error; err != nil {
		return now, err
	}
	var due []db.P11ArenaDailyOffer
	closed := tx.Model(&db.P11ArenaDailySnapshot{}).Select("day")
	if err := tx.Where("end_at<=? AND day NOT IN (?)", now.UnixMilli(), closed).Order("end_at, day").Limit(int(constUtil.P11ArenaDailyWindowLimit)).Find(&due).Error; err != nil {
		return now, err
	}
	for i := range due {
		if err := s.p11ArenaDailyClose(tx, &due[i], now); err != nil {
			return now, err
		}
	}
	if !policy.Enabled {
		return now, nil
	}
	if !p11ArenaDailyPolicyValid(&policy) {
		return now, errors.New(constUtil.P11ArenaDailyConfigError)
	}
	// The permanent source remains published while an administrator revisits an
	// earlier game day. Freeze only a bounded copy; its original row stays intact.
	if policy.Permanent && calendar75Adjusted(tx) && policy.PublishedAt > now.UnixMilli() && policy.PublishedAt <= calendar75SourceHorizon(tx, now).UnixMilli() {
		start, _ := p11ArenaDailyWindow(now, policy.CutoffSecond)
		policy.PublishedAt, policy.StartAt = now.UnixMilli(), start.UnixMilli()
	}
	start, end := p11ArenaDailyWindow(now, policy.CutoffSecond)
	if start.UnixMilli() < policy.StartAt || (!policy.Permanent && end.UnixMilli() > policy.EndAt) || now.UnixMilli() < policy.PublishedAt {
		return now, nil
	}
	var prior db.P11ArenaDailyOffer
	err := tx.First(&prior, "day=?", start.Format("20060102")).Error
	if err == nil {
		_, err = p11ArenaDailyOfferRules(&prior)
		return now, err
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return now, err
	}
	// An operator changing the current cutoff cannot create a second overlapping
	// reward window. The first published offer keeps its complete original term.
	var overlapping int64
	if err := tx.Model(&db.P11ArenaDailyOffer{}).Where("start_at<? AND end_at>?", end.UnixMilli(), start.UnixMilli()).Count(&overlapping).Error; err != nil {
		return now, err
	}
	if overlapping > 0 {
		return now, nil
	}
	rules := p11ArenaDailyRules{Policy: policy, OriginalRewards: constUtil.P11ArenaDailyOriginalRewards, DeleteRewardDays: 14}
	raw, err := json.Marshal(rules)
	if err != nil {
		return now, err
	}
	offer := db.P11ArenaDailyOffer{Day: start.Format("20060102"), StartAt: start.UnixMilli(), EndAt: end.UnixMilli(), RulesJSON: string(raw), CapturedAt: now.UnixMilli()}
	offer.SourceHash = p11ArenaDailyOfferHash(offer)
	return now, tx.Create(&offer).Error
}

// RecordP11ArenaDailyChallenge attaches a serialized fact only to the new modern
// settlement after its original prize callback finalized the committed response.
// The caller already validated the accepted P6 source in that same transaction.
func (s *GameService) RecordP11ArenaDailyChallenge(ctx context.Context, tx *gorm.DB, row *db.P11ArenaFightSettlement, admitted time.Time) error {
	tx = tx.WithContext(ctx)
	if !gameSchemaHasTable(tx, &db.P11ArenaDailyPolicy{}) {
		return nil
	}
	if row == nil || row.FightID == "" || row.PlayerID <= 0 || row.OpponentID <= 0 || admitted.UnixMilli() <= 0 || row.ResponseHash != p6Digest(row.Response) || row.TerminalHash != p6Digest(row.Terminal) || row.ScopeProof == "" || row.P6RecordID == "" {
		return errors.New(constUtil.P11ArenaDailyConflict)
	}
	fact := db.P11ArenaDailyChallenge{FightID: row.FightID, PlayerID: row.PlayerID, OpponentID: row.OpponentID, AcceptedAt: admitted.UnixMilli(), ResponseHash: row.ResponseHash, TerminalHash: row.TerminalHash, P6RecordID: row.P6RecordID, ScopeProof: row.ScopeProof}
	// The auto-increment sequence is allocated while the same arena lock still
	// serializes rank transitions. Hash it after insertion, before the commit.
	if err := tx.Create(&fact).Error; err != nil {
		return err
	}
	fact.SourceHash = p11ArenaDailyChallengeHash(fact)
	return tx.Model(&fact).Update("source_hash", fact.SourceHash).Error
}

func p11ArenaDailyAccepted(tx *gorm.DB, fact *db.P11ArenaDailyChallenge) (*db.P11ArenaFightSettlement, error) {
	bad := errors.New(constUtil.P11ArenaDailyConflict)
	if fact.ID <= 0 || fact.SourceHash != p11ArenaDailyChallengeHash(*fact) {
		return nil, bad
	}
	var row db.P11ArenaFightSettlement
	if err := tx.First(&row, "fight_id=?", fact.FightID).Error; err != nil {
		return nil, err
	}
	if fact.PlayerID != row.PlayerID || fact.OpponentID != row.OpponentID || fact.ResponseHash != row.ResponseHash || fact.TerminalHash != row.TerminalHash || fact.P6RecordID != row.P6RecordID || fact.ScopeProof != row.ScopeProof {
		return nil, bad
	}
	if _, err := p11ReadArenaFightReceipt(tx, &row); err != nil {
		return nil, err
	}
	return &row, nil
}

func (s *GameService) p11ArenaDailyClose(tx *gorm.DB, offer *db.P11ArenaDailyOffer, now time.Time) error {
	rules, err := p11ArenaDailyOfferRules(offer)
	if err != nil {
		return err
	}
	snapshot := db.P11ArenaDailySnapshot{Day: offer.Day, StartAt: offer.StartAt, EndAt: offer.EndAt, OfferHash: offer.SourceHash, Status: constUtil.P11ArenaDailyComplete, RanksJSON: "[]", CapturedAt: now.UnixMilli()}
	var facts []db.P11ArenaDailyChallenge
	if err = tx.Where("accepted_at>=? AND accepted_at<?", max(offer.StartAt, offer.CapturedAt), offer.EndAt).Order("id").Find(&facts).Error; err != nil {
		return err
	}
	counts := map[int64]int64{}
	for _, fact := range facts {
		counts[fact.PlayerID]++
	}
	var members []db.P11ArenaMember
	membership := tx.Where("position>0 AND position<=1000")
	if !calendar75Adjusted(tx) {
		membership = membership.Where("created_at<?", offer.EndAt)
	}
	if err = membership.Order("position").Find(&members).Error; err != nil {
		return err
	}
	// Validate each immutable battle at most once in this cutoff transaction.
	accepted := map[int64]*db.P11ArenaFightSettlement{}
	checked := func(fact *db.P11ArenaDailyChallenge) (*db.P11ArenaFightSettlement, error) {
		if row, ok := accepted[fact.ID]; ok {
			return row, nil
		}
		row, e := p11ArenaDailyAccepted(tx, fact)
		if e == nil {
			accepted[fact.ID] = row
		}
		return row, e
	}
	ranks := make([]p11ArenaDailyRank, 0)
	pending := func(reason string) { snapshot.Status = constUtil.P11ArenaDailyPending; snapshot.Reason = reason }
	for _, member := range members {
		if counts[member.PlayerID] < rules.Policy.MinChallenges {
			continue
		}
		for i := range facts {
			if facts[i].PlayerID == member.PlayerID {
				if _, err = checked(&facts[i]); err != nil {
					pending("accepted-challenge-proof-missing")
					break
				}
			}
		}
		if snapshot.Status == constUtil.P11ArenaDailyPending {
			break
		}
		var latest db.P11ArenaDailyChallenge
		if err = tx.Where("accepted_at<? AND (player_id=? OR opponent_id=?)", offer.EndAt, member.PlayerID, member.PlayerID).Order("id DESC").First(&latest).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				pending("current-rank-proof-missing")
				break
			}
			return err
		}
		row, e := checked(&latest)
		if e != nil {
			pending("current-rank-proof-invalid")
			break
		}
		rank := row.AttackerNewRank
		if row.OpponentID == member.PlayerID {
			rank = row.DefenderNewRank
		}
		if rank != member.Rank {
			pending("current-rank-not-supported-by-last-transition")
			break
		}
		var n int64
		if err = tx.Model(&db.Profile{}).Where("id=?", member.PlayerID).Count(&n).Error; err != nil {
			return err
		}
		if n != 1 {
			pending("eligible-player-profile-missing")
			break
		}
		ranks = append(ranks, p11ArenaDailyRank{PlayerID: member.PlayerID, Rank: rank, Challenges: counts[member.PlayerID], RankProof: latest.SourceHash, Rewards: p11ArenaDailyReward(rank)})
	}
	if snapshot.Status == constUtil.P11ArenaDailyComplete {
		raw, e := json.Marshal(ranks)
		if e != nil {
			return e
		}
		snapshot.RanksJSON = string(raw)
	}
	snapshot.SourceHash = p11ArenaDailySnapshotHash(snapshot)
	if err = tx.Create(&snapshot).Error; err != nil {
		return err
	}
	if snapshot.Status != constUtil.P11ArenaDailyComplete {
		return nil
	}
	for _, rank := range ranks {
		key := fmt.Sprintf("arena-daily:%s:%d", offer.Day, rank.PlayerID)
		delivery, e := s.EnqueueRewardNotice(tx, rank.PlayerID, key, "每日竞技排名发奖", fmt.Sprintf("竞技场排名第%d名奖励", rank.Rank), rank.Rewards, offer.EndAt+rules.DeleteRewardDays*86400000)
		if e != nil {
			return e
		}
		award := db.P11ArenaDailyAward{Day: offer.Day, PlayerID: rank.PlayerID, Rank: rank.Rank, Rewards: rank.Rewards, SnapshotHash: snapshot.SourceHash, RankProof: rank.RankProof, DeliveryID: delivery.ID, SourceKey: key, CreatedAt: now.UnixMilli()}
		award.SourceHash = p11ArenaDailyAwardHash(award)
		if err = tx.Create(&award).Error; err != nil {
			return err
		}
	}
	return nil
}

// TickP11ArenaDaily closes only already-frozen windows under the same arena
// serialization lock. Offline rewards use the existing P1 once-only delivery.
func (s *GameService) TickP11ArenaDaily(ctx context.Context, now time.Time) error {
	ctx, bound, err := s.bindServerGameClockWorker75(ctx)
	if err != nil {
		return err
	}
	if bound {
		now = LocalPlayNow(ctx)
	}
	return db.GetIns().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := ServerGameClockGuardTransaction(tx); err != nil {
			return err
		}

		var arena db.P11ArenaPolicy
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&arena, 1).Error; err != nil {
			return err
		}
		_, err := s.PrepareP11ArenaDailyMutation(ctx, tx, now)
		return err
	})
}
func (s *GameService) ServeP11ArenaDaily(ctx context.Context) error {
	if err := s.TickP11ArenaDaily(ctx, LocalPlayNow(ctx)); err != nil {
		return err
	}
	ticker := time.NewTicker(constUtil.P11ArenaDailyWorkerInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case now := <-ticker.C:
			if err := s.TickP11ArenaDaily(ctx, LocalPlayAt(ctx, now)); err != nil {
				if ctx.Err() != nil {
					return nil
				}
				return err
			}
		}
	}
}
