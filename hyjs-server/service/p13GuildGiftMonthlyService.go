package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strconv"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"hyjs-server/constUtil"
	"hyjs-server/db"
)

type p13GuildGiftLevel struct {
	Level     int32  `json:"level"`
	Capacity  int32  `json:"capacity"`
	Threshold int64  `json:"threshold"`
	Common    string `json:"common"`
	Vice      string `json:"vice"`
	Chairman  string `json:"chairman"`
}

type p13GuildGiftIdentity struct {
	PlayerID int64 `json:"playerId"`
	Role     int32 `json:"role"`
	JoinedAt int64 `json:"joinedAt"`
}

func p13GuildGiftError() error { return errors.New(constUtil.P13GuildGiftConflict) }

func p13GuildGiftDigest(v any) string { raw, _ := json.Marshal(v); return p6Digest(raw) }

func p13GuildGiftMonth(now time.Time) (string, time.Time, time.Time) {
	local := now.In(time.FixedZone("Asia/Shanghai", 8*3600))
	start := time.Date(local.Year(), local.Month(), 1, 0, 0, 0, 0, local.Location())
	return start.Format("2006-01"), start, start.AddDate(0, 1, 0)
}

func p13GuildGiftRuleHash(v db.P13GuildGiftRule) string {
	v.SourceHash = ""
	return p13GuildGiftDigest(v)
}
func p13GuildGiftHeadHash(v db.P13GuildGiftHead) string {
	v.SourceHash = ""
	return p13GuildGiftDigest(v)
}
func p13GuildGiftArchiveHash(v db.P13GuildGiftArchive) string {
	v.SourceHash = ""
	return p13GuildGiftDigest(v)
}
func p13GuildGiftCreditHash(v db.P13GuildGiftCredit) string {
	v.SourceHash = ""
	return p13GuildGiftDigest(v)
}
func p13GuildGiftCutoffHash(v db.P13GuildGiftCutoff) string {
	v.SourceHash = ""
	return p13GuildGiftDigest(v)
}
func p13GuildGiftAwardHash(v db.P13GuildGiftAward) string {
	v.SourceHash = ""
	return p13GuildGiftDigest(v)
}

// InitializeP13GuildGiftMonthly records the explicitly approved future activation.
// It never converts an existing accumulated score into authenticated eligibility.
func (s *GameService) InitializeP13GuildGiftMonthly(ctx context.Context) error {
	d := db.GetIns().WithContext(ctx)
	if err := d.AutoMigrate(&db.P13GuildGiftPolicy{}, &db.P13GuildGiftRule{}, &db.P13GuildGiftHead{}, &db.P13GuildGiftArchive{}, &db.P13GuildGiftCredit{}, &db.P13GuildGiftCutoff{}, &db.P13GuildGiftAward{}); err != nil {
		return err
	}
	at, err := time.ParseInLocation("2006-01", constUtil.P13GuildGiftStartsMonth, time.FixedZone("Asia/Shanghai", 8*3600))
	if err != nil {
		return err
	}
	return d.Clauses(clause.OnConflict{DoNothing: true}).Create(&db.P13GuildGiftPolicy{ID: 1, Enabled: true, StartsAt: at.UnixMilli(), PublishedAt: time.Now().UnixMilli(), Rule: constUtil.P13GuildGiftRule, Source: constUtil.P13GuildGiftSource}).Error
}

func p13GuildGiftPolicy(tx *gorm.DB) (*db.P13GuildGiftPolicy, error) {
	// Older independent domains can intentionally omit ordinary guild schemas.
	if !gameSchemaHasTable(tx, &db.P13GuildGiftPolicy{}) {
		return nil, nil
	}
	var p db.P13GuildGiftPolicy
	if err := tx.First(&p, 1).Error; err != nil {
		return nil, err
	}
	start, err := time.ParseInLocation("2006-01", constUtil.P13GuildGiftStartsMonth, time.FixedZone("Asia/Shanghai", 8*3600))
	if err != nil || p.ID != 1 || p.Rule != constUtil.P13GuildGiftRule || p.Source != constUtil.P13GuildGiftSource || p.StartsAt != start.UnixMilli() || p.PublishedAt <= 0 {
		return nil, p13GuildGiftError()
	}
	return &p, nil
}

// A trusted head cannot fall back into legacy arithmetic after a clock rewind.
// Request paths select this guild's primary key; the worker checks at most one
// indexed head before its otherwise no-op preactivation return.
func p13GuildGiftPrestartGuard(tx *gorm.DB, guildID string) error {
	query := tx.Model(&db.P13GuildGiftHead{}).Select("guild_id").Limit(1)
	if guildID != "" {
		query = query.Where("guild_id=?", guildID)
	}
	var head db.P13GuildGiftHead
	err := query.Take(&head).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	if calendar75Adjusted(tx) {
		// A trusted test rewind pauses this forward-only monthly ledger. Its
		// head and earned entitlements remain intact; ordinary guild play works.
		return nil
	}
	return p13GuildGiftError()
}

func p13GuildGiftTrackedBeforeStart75(tx *gorm.DB, guildID string) (bool, error) {
	var head db.P13GuildGiftHead
	err := tx.First(&head, "guild_id=?", guildID).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	_, _, err = p13GuildGiftVerifyHead(tx, &head)
	return true, err
}

func p13GuildGiftReadRule(tx *gorm.DB, month string) (*db.P13GuildGiftRule, []p13GuildGiftLevel, error) {
	var rule db.P13GuildGiftRule
	if err := tx.First(&rule, "month=?", month).Error; err != nil {
		return nil, nil, err
	}
	start, err := time.ParseInLocation("2006-01", month, time.FixedZone("Asia/Shanghai", 8*3600))
	var levels []p13GuildGiftLevel
	if err != nil || rule.StartAt != start.UnixMilli() || rule.EndAt != start.AddDate(0, 1, 0).UnixMilli() || rule.FrozenAt < rule.StartAt || rule.FrozenAt >= rule.EndAt || rule.Credit <= 0 || rule.LevelsHash != p6Digest(rule.Levels) || rule.SourceHash != p13GuildGiftRuleHash(rule) || rule.Source != constUtil.P13GuildGiftSource || json.Unmarshal(rule.Levels, &levels) != nil || len(levels) != 10 {
		return nil, nil, p13GuildGiftError()
	}
	for i, l := range levels {
		if l.Level != int32(i+1) || l.Capacity < 1 || l.Capacity > constUtil.P13GuildGiftMaxMembers || l.Threshold < 1 || l.Common == "" || l.Vice == "" || l.Chairman == "" {
			return nil, nil, p13GuildGiftError()
		}
	}
	var config []map[string]string
	var login map[string]string
	if rule.ConfigHash != p6Digest(rule.Config) || rule.LoginConfigHash != p6Digest(rule.LoginConfig) || json.Unmarshal(rule.Config, &config) != nil || len(config) != 10 || json.Unmarshal(rule.LoginConfig, &login) != nil || login["StaticName"] != "UnionLoginActiveScore" {
		return nil, nil, p13GuildGiftError()
	}
	credit, e := strconv.ParseInt(login["NameValue"], 10, 32)
	if e != nil || credit != int64(rule.Credit) {
		return nil, nil, p13GuildGiftError()
	}
	seen := map[int]bool{}
	for _, r := range config {
		level, e := strconv.Atoi(r["Level"])
		if e != nil || level < 1 || level > 10 || seen[level] {
			return nil, nil, p13GuildGiftError()
		}
		seen[level] = true
		l := levels[level-1]
		if r["ActiveScore"] != strconv.FormatInt(l.Threshold, 10) || r["MaxMemberNum"] != strconv.Itoa(int(l.Capacity)) || r["CommonReward"] != l.Common || r["ViceReward"] != l.Vice || r["ChairmanReward"] != l.Chairman {
			return nil, nil, p13GuildGiftError()
		}
	}
	return &rule, levels, nil
}

func (s *GameService) p13GuildGiftFreezeRule(tx *gorm.DB, now time.Time) (*db.P13GuildGiftRule, error) {
	month, start, end := p13GuildGiftMonth(now)
	loaded, _, err := p13GuildGiftReadRule(tx, month)
	if err == nil {
		return loaded, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}
	tables, err := adminGuildRuntimeTablesTx(tx, s.Tables)
	if err != nil {
		return nil, err
	}
	original := s.withRuntimeTables(tables)
	credit, err := original.p10Setting("UnionLoginActiveScore")
	if err != nil || credit <= 0 {
		return nil, p13GuildGiftError()
	}
	levels := make([]p13GuildGiftLevel, 0, 10)
	for level := 1; level <= 10; level++ {
		r := original.table("UnionConfig", "Level", strconv.Itoa(level))
		threshold, e := strconv.ParseInt(r["ActiveScore"], 10, 32)
		capacity, e2 := strconv.ParseInt(r["MaxMemberNum"], 10, 32)
		if e != nil || e2 != nil || threshold <= 0 || capacity <= 0 || capacity > constUtil.P13GuildGiftMaxMembers {
			return nil, p13GuildGiftError()
		}
		for _, key := range []string{"CommonReward", "ViceReward", "ChairmanReward"} {
			ok, e := original.openServerRewardSupported(r[key])
			if e != nil {
				return nil, e
			}
			if !ok {
				return nil, p13GuildGiftError()
			}
		}
		levels = append(levels, p13GuildGiftLevel{Level: int32(level), Capacity: int32(capacity), Threshold: threshold, Common: r["CommonReward"], Vice: r["ViceReward"], Chairman: r["ChairmanReward"]})
	}
	raw, err := json.Marshal(levels)
	if err != nil {
		return nil, err
	}
	config, err := json.Marshal(original.Tables["UnionConfig"])
	if err != nil {
		return nil, err
	}
	login, err := json.Marshal(original.table("GameConfigure", "StaticName", "UnionLoginActiveScore"))
	if err != nil {
		return nil, err
	}
	rule := db.P13GuildGiftRule{Month: month, StartAt: start.UnixMilli(), EndAt: end.UnixMilli(), Credit: credit, Levels: raw, LevelsHash: p6Digest(raw), Config: config, ConfigHash: p6Digest(config), LoginConfig: login, LoginConfigHash: p6Digest(login), FrozenAt: now.UnixMilli(), Source: constUtil.P13GuildGiftSource}
	rule.SourceHash = p13GuildGiftRuleHash(rule)
	if err = tx.Create(&rule).Error; err != nil {
		return nil, err
	}
	verified, _, err := p13GuildGiftReadRule(tx, month)
	return verified, err
}

func p13GuildGiftVerifyCredit(c *db.P13GuildGiftCredit, rule *db.P13GuildGiftRule) error {
	if c == nil || rule == nil || c.ID != p6Digest([]byte(fmt.Sprintf("guild-gift-credit:%d:%s", c.PlayerID, c.Day))) || c.PlayerID <= 0 || c.GuildID == "" || c.Month != rule.Month || c.RuleHash != rule.SourceHash || c.Amount != rule.Credit || c.BeforeScore < 0 || c.BeforeScore > math.MaxInt32-int64(c.Amount) || c.AfterScore != c.BeforeScore+int64(c.Amount) || c.OccurredAt < rule.StartAt || c.OccurredAt >= rule.EndAt || c.Day != p13GuildDay(time.UnixMilli(c.OccurredAt)) || c.JoinedAt <= 0 || c.JoinedAt > c.OccurredAt || c.SourceHash != p13GuildGiftCreditHash(*c) || (c.PreviousID == "") != (c.PreviousHash == "") {
		return p13GuildGiftError()
	}
	return nil
}

func p13GuildGiftVerifyHead(tx *gorm.DB, head *db.P13GuildGiftHead) (*db.P13GuildGiftRule, []p13GuildGiftLevel, error) {
	if head == nil || head.GuildID == "" || head.SourceHash != p13GuildGiftHeadHash(*head) || head.Score < 0 || head.Score > math.MaxInt32 || head.CreditCount < 0 {
		return nil, nil, p13GuildGiftError()
	}
	rule, levels, err := p13GuildGiftReadRule(tx, head.Month)
	if err != nil {
		return nil, nil, err
	}
	if head.RuleHash != rule.SourceHash || head.RetiredAt < 0 || (head.RetiredAt == 0 && head.DueAt != rule.EndAt) || (head.RetiredAt > 0 && (head.DueAt != 0 || head.RetiredAt < rule.StartAt)) || head.CreditCount > math.MaxInt32/int64(rule.Credit) || head.Score != head.CreditCount*int64(rule.Credit) {
		return nil, nil, p13GuildGiftError()
	}
	var archive db.P13GuildGiftArchive
	if err = tx.First(&archive, "guild_id=?", head.GuildID).Error; err != nil {
		return nil, nil, err
	}
	var old db.P13Guild
	if archive.SourceHash != p13GuildGiftArchiveHash(archive) || archive.SourceHash != head.ArchiveHash || archive.GuildHash != p6Digest(archive.GuildRaw) || archive.Source != constUtil.P13GuildGiftSource || archive.CollectedAt <= 0 || json.Unmarshal(archive.GuildRaw, &old) != nil || old.ID != head.GuildID || old.ActiveScore != archive.Score {
		return nil, nil, p13GuildGiftError()
	}
	if head.CreditCount == 0 {
		if head.Score != 0 || head.LastCreditID != "" || head.LastCreditHash != "" || head.LastCreditAt != 0 {
			return nil, nil, p13GuildGiftError()
		}
	} else {
		var last db.P13GuildGiftCredit
		if err = tx.First(&last, "id=?", head.LastCreditID).Error; err != nil {
			return nil, nil, err
		}
		if err = p13GuildGiftVerifyCredit(&last, rule); err != nil {
			return nil, nil, err
		}
		if last.GuildID != head.GuildID || last.AfterScore != head.Score || last.SourceHash != head.LastCreditHash || last.OccurredAt != head.LastCreditAt || last.OccurredAt < rule.FrozenAt {
			return nil, nil, p13GuildGiftError()
		}
	}
	return rule, levels, nil
}

func p13GuildGiftReward(level p13GuildGiftLevel, role int32) (string, error) {
	switch role {
	case constUtil.P13GuildMember:
		return level.Common, nil
	case constUtil.P13GuildVice:
		return level.Vice, nil
	case constUtil.P13GuildChairman:
		return level.Chairman, nil
	}
	return "", p13GuildGiftError()
}

func (s *GameService) p13GuildGiftClose(tx *gorm.DB, guild *db.P13Guild, head *db.P13GuildGiftHead, now time.Time) error {
	rule, levels, err := p13GuildGiftVerifyHead(tx, head)
	if err != nil {
		return err
	}
	if now.UnixMilli() < rule.EndAt || guild.Level < 1 || guild.Level > 10 {
		return p13GuildGiftError()
	}
	level := levels[guild.Level-1]
	var members []db.P13GuildMember
	if err = tx.Where("guild_id=?", guild.ID).Order("player_id").Limit(constUtil.P13GuildGiftMaxMembers + 1).Find(&members).Error; err != nil {
		return err
	}
	if len(members) == 0 || len(members) > int(level.Capacity) {
		return p13GuildGiftError()
	}
	roster := make([]p13GuildGiftIdentity, 0, len(members))
	chairman := 0
	for _, m := range members {
		if m.PlayerID <= 0 || m.JoinedAt <= 0 || m.JoinedAt >= rule.EndAt || m.Role < 0 || m.Role > 2 {
			return p13GuildGiftError()
		}
		if m.Role == constUtil.P13GuildChairman {
			chairman++
		}
		roster = append(roster, p13GuildGiftIdentity{PlayerID: m.PlayerID, Role: m.Role, JoinedAt: m.JoinedAt})
	}
	if chairman != 1 {
		return p13GuildGiftError()
	}
	raw, err := json.Marshal(roster)
	if err != nil {
		return err
	}
	headRaw, err := json.Marshal(head)
	if err != nil {
		return err
	}
	cut := db.P13GuildGiftCutoff{ID: p6Digest([]byte("guild-gift-cutoff:" + guild.ID + ":" + head.Month)), GuildID: guild.ID, Month: head.Month, CutoffAt: rule.EndAt, CapturedAt: now.UnixMilli(), Level: guild.Level, Score: head.Score, Qualified: head.Score >= level.Threshold, RuleHash: rule.SourceHash, HeadHash: head.SourceHash, HeadRaw: headRaw, LastCreditID: head.LastCreditID, LastCreditHash: head.LastCreditHash, Roster: raw}
	cut.SourceHash = p13GuildGiftCutoffHash(cut)
	if err = tx.Create(&cut).Error; err != nil {
		return err
	}
	if cut.Qualified {
		awards := make([]db.P13GuildGiftAward, 0, len(roster))
		for _, m := range roster {
			reward, e := p13GuildGiftReward(level, m.Role)
			if e != nil {
				return e
			}
			a := db.P13GuildGiftAward{ID: p6Digest([]byte(fmt.Sprintf("guild-gift-award:%s:%d", cut.ID, m.PlayerID))), PlayerID: m.PlayerID, GuildID: guild.ID, Month: head.Month, Role: m.Role, CutoffID: cut.ID, CutoffHash: cut.SourceHash, Rewards: reward}
			a.SourceHash = p13GuildGiftAwardHash(a)
			awards = append(awards, a)
		}
		// At most the original 62 members; one batch insert, no per-player P1 work.
		if err = tx.Create(&awards).Error; err != nil {
			return err
		}
	}
	return nil
}

// P13GuildGiftBeforeMutation closes only this guild. Caller holds GuildPolicy.
// The half-open month includes its last millisecond; first next-month activity
// freezes the old membership/role/level before any new mutation or login credit.
func (s *GameService) P13GuildGiftBeforeMutation(tx *gorm.DB, guildID string, now time.Time) (bool, error) {
	if guildID == "" {
		return false, nil
	}
	policy, err := p13GuildGiftPolicy(tx)
	if err != nil || policy == nil {
		return false, err
	}
	if now.UnixMilli() < policy.StartsAt {
		if calendar75Adjusted(tx) {
			return p13GuildGiftTrackedBeforeStart75(tx, guildID)
		}
		return false, p13GuildGiftPrestartGuard(tx, guildID)
	}
	var guild db.P13Guild
	if err = tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&guild, "id=?", guildID).Error; err != nil {
		return true, err
	}
	month, _, _ := p13GuildGiftMonth(now)
	var head db.P13GuildGiftHead
	err = tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&head, "guild_id=?", guildID).Error
	if err == nil {
		var existingRule *db.P13GuildGiftRule
		if existingRule, _, err = p13GuildGiftVerifyHead(tx, &head); err != nil {
			return true, err
		}
		if now.UnixMilli() < existingRule.FrozenAt || now.UnixMilli() < head.LastCreditAt {
			if calendar75Adjusted(tx) && max(existingRule.FrozenAt, head.LastCreditAt) <= calendar75SourceHorizon(tx, now).UnixMilli() {
				return true, nil
			}
			return true, p13GuildGiftError()
		}
		if head.RetiredAt != 0 {
			return true, p13GuildGiftError()
		}
		if month < head.Month {
			if calendar75Adjusted(tx) && existingRule.FrozenAt <= calendar75SourceHorizon(tx, now).UnixMilli() {
				return true, nil
			}
			return true, p13GuildGiftError()
		}
		if month == head.Month {
			if int64(guild.ActiveScore) != head.Score {
				return true, p13GuildGiftError()
			}
			return true, nil
		}
		if err = s.p13GuildGiftClose(tx, &guild, &head, now); err != nil {
			return true, err
		}
	} else if errors.Is(err, gorm.ErrRecordNotFound) {
		raw, e := json.Marshal(guild)
		if e != nil {
			return true, e
		}
		archive := db.P13GuildGiftArchive{GuildID: guildID, Score: guild.ActiveScore, GuildRaw: raw, GuildHash: p6Digest(raw), CollectedAt: now.UnixMilli(), Source: constUtil.P13GuildGiftSource}
		archive.SourceHash = p13GuildGiftArchiveHash(archive)
		if err = tx.Create(&archive).Error; err != nil {
			return true, err
		}
		head = db.P13GuildGiftHead{GuildID: guildID, ArchiveHash: archive.SourceHash}
	} else {
		return true, err
	}
	// Missed empty months are never manufactured. Only the real old bucket closes.
	rule, err := s.p13GuildGiftFreezeRule(tx, now)
	if err != nil {
		return true, err
	}
	head.Month, head.DueAt, head.Score, head.CreditCount = rule.Month, rule.EndAt, 0, 0
	head.LastCreditID, head.LastCreditHash, head.RuleHash = "", "", rule.SourceHash
	head.LastCreditAt = 0
	head.SourceHash = p13GuildGiftHeadHash(head)
	if err = tx.Save(&head).Error; err != nil {
		return true, err
	}
	return true, tx.Model(&db.P13Guild{}).Where("id=?", guildID).Update("active_score", 0).Error
}

// Retire an actual dissolved guild's open bucket without inventing a month-end
// roster. Already frozen cutoffs and P1 entitlements survive dissolution.
func (s *GameService) P13GuildGiftRetire(tx *gorm.DB, guildID string, now time.Time) error {
	tracked, err := s.P13GuildGiftBeforeMutation(tx, guildID, now)
	if err != nil || !tracked {
		return err
	}
	var head db.P13GuildGiftHead
	if err = tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&head, "guild_id=?", guildID).Error; err != nil {
		return err
	}
	head.RetiredAt, head.DueAt = now.UnixMilli(), 0
	if calendar75Adjusted(tx) {
		rule, _, e := p13GuildGiftReadRule(tx, head.Month)
		if e != nil {
			return e
		}
		head.RetiredAt = max(head.RetiredAt, rule.StartAt, head.LastCreditAt)
	}
	head.SourceHash = p13GuildGiftHeadHash(head)
	return tx.Save(&head).Error
}

func (s *GameService) p13GuildGiftCredit(tx *gorm.DB, guild *db.P13Guild, member *db.P13GuildMember, now time.Time) (bool, error) {
	policy, err := p13GuildGiftPolicy(tx)
	if err != nil || policy == nil {
		return false, err
	}
	if now.UnixMilli() < policy.StartsAt {
		if calendar75Adjusted(tx) {
			return p13GuildGiftTrackedBeforeStart75(tx, guild.ID)
		}
		return false, p13GuildGiftPrestartGuard(tx, guild.ID)
	}
	var head db.P13GuildGiftHead
	if err = tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&head, "guild_id=?", guild.ID).Error; err != nil {
		return true, err
	}
	rule, _, err := p13GuildGiftVerifyHead(tx, &head)
	if err != nil {
		return true, err
	}
	month, _, _ := p13GuildGiftMonth(now)
	if calendar75Adjusted(tx) && (month < head.Month || now.UnixMilli() < head.LastCreditAt || now.UnixMilli() < rule.FrozenAt || member.JoinedAt > now.UnixMilli()) {
		if max(head.LastCreditAt, rule.FrozenAt, member.JoinedAt) <= calendar75SourceHorizon(tx, now).UnixMilli() && int64(guild.ActiveScore) == head.Score && member.GuildID == guild.ID {
			return true, nil
		}
	}
	if head.Month != month || int64(guild.ActiveScore) != head.Score || member.GuildID != guild.ID || now.UnixMilli() < head.LastCreditAt || now.UnixMilli() < rule.FrozenAt {
		return true, p13GuildGiftError()
	}
	if !policy.Enabled {
		return true, nil
	}
	var old db.P13GuildGiftCredit
	err = tx.First(&old, "player_id=? AND day=?", member.PlayerID, p13GuildDay(now)).Error
	if err == nil {
		oldRule, _, e := p13GuildGiftReadRule(tx, old.Month)
		if e != nil {
			return true, e
		}
		return true, p13GuildGiftVerifyCredit(&old, oldRule)
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return true, err
	}
	if head.Score > math.MaxInt32-int64(rule.Credit) || head.CreditCount == math.MaxInt64 {
		return true, p13GuildGiftError()
	}
	c := db.P13GuildGiftCredit{ID: p6Digest([]byte(fmt.Sprintf("guild-gift-credit:%d:%s", member.PlayerID, p13GuildDay(now)))), PlayerID: member.PlayerID, Day: p13GuildDay(now), GuildID: guild.ID, Month: month, JoinedAt: member.JoinedAt, Amount: rule.Credit, BeforeScore: head.Score, AfterScore: head.Score + int64(rule.Credit), PreviousID: head.LastCreditID, PreviousHash: head.LastCreditHash, RuleHash: rule.SourceHash, OccurredAt: now.UnixMilli()}
	c.SourceHash = p13GuildGiftCreditHash(c)
	if err = p13GuildGiftVerifyCredit(&c, rule); err != nil {
		return true, err
	}
	if err = tx.Create(&c).Error; err != nil {
		return true, err
	}
	head.Score, head.LastCreditID, head.LastCreditHash = c.AfterScore, c.ID, c.SourceHash
	head.LastCreditAt = c.OccurredAt
	head.CreditCount++
	head.SourceHash = p13GuildGiftHeadHash(head)
	if err = tx.Save(&head).Error; err != nil {
		return true, err
	}
	guild.ActiveScore = int32(head.Score)
	return true, nil
}

// P13GuildGiftVerifyAward authenticates the frozen recipient and original role
// reward independently of current membership, current tables or guild survival.
func P13GuildGiftVerifyAward(tx *gorm.DB, a *db.P13GuildGiftAward) error {
	if a == nil || a.PlayerID <= 0 || a.SourceHash != p13GuildGiftAwardHash(*a) || a.DeliveredAt < 0 || (a.DeliveredAt == 0) != (a.DeliveryID == 0) {
		return p13GuildGiftError()
	}
	var cut db.P13GuildGiftCutoff
	if err := tx.First(&cut, "id=?", a.CutoffID).Error; err != nil {
		return err
	}
	rule, levels, err := p13GuildGiftReadRule(tx, cut.Month)
	if err != nil {
		return err
	}
	if cut.ID != p6Digest([]byte("guild-gift-cutoff:"+cut.GuildID+":"+cut.Month)) || cut.SourceHash != p13GuildGiftCutoffHash(cut) || cut.SourceHash != a.CutoffHash || cut.GuildID != a.GuildID || cut.Month != a.Month || cut.RuleHash != rule.SourceHash || cut.CutoffAt != rule.EndAt || cut.CapturedAt < cut.CutoffAt || cut.Level < 1 || cut.Level > 10 || !cut.Qualified || cut.Score < levels[cut.Level-1].Threshold || cut.Score > math.MaxInt32 || len(cut.HeadHash) != 64 || len(cut.LastCreditID) != 64 || len(cut.LastCreditHash) != 64 {
		return p13GuildGiftError()
	}
	var frozen db.P13GuildGiftHead
	if json.Unmarshal(cut.HeadRaw, &frozen) != nil || frozen.SourceHash != cut.HeadHash || frozen.GuildID != cut.GuildID || frozen.Month != cut.Month || frozen.Score != cut.Score || frozen.LastCreditID != cut.LastCreditID || frozen.LastCreditHash != cut.LastCreditHash || frozen.RetiredAt != 0 {
		return p13GuildGiftError()
	}
	if _, _, err = p13GuildGiftVerifyHead(tx, &frozen); err != nil {
		return err
	}
	var last db.P13GuildGiftCredit
	if err = tx.First(&last, "id=?", cut.LastCreditID).Error; err != nil {
		return err
	}
	if err = p13GuildGiftVerifyCredit(&last, rule); err != nil {
		return err
	}
	if last.SourceHash != cut.LastCreditHash || last.AfterScore != cut.Score || last.GuildID != cut.GuildID {
		return p13GuildGiftError()
	}
	var roster []p13GuildGiftIdentity
	if json.Unmarshal(cut.Roster, &roster) != nil || len(roster) == 0 || len(roster) > int(levels[cut.Level-1].Capacity) {
		return p13GuildGiftError()
	}
	found := false
	chairman := 0
	previous := int64(0)
	for _, m := range roster {
		if m.PlayerID <= previous || m.JoinedAt <= 0 || m.JoinedAt >= cut.CutoffAt || m.Role < 0 || m.Role > 2 {
			return p13GuildGiftError()
		}
		previous = m.PlayerID
		if m.Role == constUtil.P13GuildChairman {
			chairman++
		}
		if m.PlayerID == a.PlayerID {
			expected, e := p13GuildGiftReward(levels[cut.Level-1], m.Role)
			if e != nil {
				return e
			}
			found = m.Role == a.Role && a.Rewards == expected
		}
	}
	if !found || chairman != 1 || a.ID != p6Digest([]byte(fmt.Sprintf("guild-gift-award:%s:%d", cut.ID, a.PlayerID))) {
		return p13GuildGiftError()
	}
	return nil
}

func (s *GameService) p13GuildGiftDeliver(tx *gorm.DB, id string, now time.Time) error {
	var a db.P13GuildGiftAward
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&a, "id=?", id).Error; err != nil {
		return err
	}
	if err := P13GuildGiftVerifyAward(tx, &a); err != nil {
		return err
	}
	if a.DeliveredAt > 0 {
		return nil
	}
	key := "p13-guild-gift:" + a.Month + ":" + a.GuildID
	content := constUtil.P13GuildGiftSource + ";cutoff=" + a.CutoffHash
	delivery, err := s.EnqueueReward(tx, a.PlayerID, key, "公会月礼包", content, a.Rewards, 0)
	if err != nil {
		return err
	}
	if delivery.PlayerID != a.PlayerID || delivery.SourceKey != key || delivery.Title != "公会月礼包" || delivery.Content != content || delivery.Rewards != a.Rewards || delivery.ExpiresAt != 0 {
		return p13GuildGiftError()
	}
	a.DeliveredAt, a.DeliveryID = now.UnixMilli(), delivery.ID
	a.SourceHash = p13GuildGiftAwardHash(a)
	return tx.Save(&a).Error
}

// TickP13GuildGiftMonthly uses bounded discovery and per-guild/per-award TXs.
// Use the same LocalPlayNow context as login/admin; no war-specific clock.
func (s *GameService) TickP13GuildGiftMonthly(ctx context.Context, now time.Time) error {
	ctx, bound, err := s.bindServerGameClockWorker75(ctx)
	if err != nil {
		return err
	}
	if bound {
		now = LocalPlayNow(ctx)
	}
	d := db.GetIns().WithContext(ctx)
	p, err := p13GuildGiftPolicy(d)
	if err != nil || p == nil {
		return err
	}
	if now.UnixMilli() < p.StartsAt {
		return p13GuildGiftPrestartGuard(d, "")
	}
	var ids []string
	if err = d.Model(&db.P13GuildGiftHead{}).Select("guild_id").Where("due_at>0 AND due_at<=?", now.UnixMilli()).Order("due_at,guild_id").Limit(constUtil.P13GuildGiftBatch).Scan(&ids).Error; err != nil {
		return err
	}
	remaining := constUtil.P13GuildGiftBatch - len(ids)
	if remaining > 0 {
		var fresh []string
		if err = d.Model(&db.P13Guild{}).Select("id").Where("NOT EXISTS (SELECT 1 FROM p13_guild_gift_heads WHERE p13_guild_gift_heads.guild_id=p13_guilds.id)").Order("created_at,id").Limit(remaining).Scan(&fresh).Error; err != nil {
			return err
		}
		ids = append(ids, fresh...)
	}
	for _, id := range ids {
		if err = ctx.Err(); err != nil {
			return err
		}
		if err = d.Transaction(func(tx *gorm.DB) error {
			if e := ServerGameClockGuardTransaction(tx); e != nil {
				return e
			}
			var gate db.P13GuildPolicy
			if e := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&gate, 1).Error; e != nil {
				return e
			}
			var count int64
			if e := tx.Model(&db.P13Guild{}).Where("id=?", id).Count(&count).Error; e != nil {
				return e
			}
			if count == 0 {
				return nil
			}
			_, e := s.P13GuildGiftBeforeMutation(tx, id, now)
			return e
		}); err != nil {
			return err
		}
	}
	var awards []string
	if err = d.Model(&db.P13GuildGiftAward{}).Select("id").Where("delivered_at=0").Order("id").Limit(constUtil.P13GuildGiftBatch).Scan(&awards).Error; err != nil {
		return err
	}
	for _, id := range awards {
		if err = ctx.Err(); err != nil {
			return err
		}
		if err = d.Transaction(func(tx *gorm.DB) error {
			if e := ServerGameClockGuardTransaction(tx); e != nil {
				return e
			}
			return s.p13GuildGiftDeliver(tx, id, now)
		}); err != nil {
			return err
		}
	}
	return nil
}

// ServeP13GuildGiftMonthly returns operational errors to ServeBackgroundWorker,
// which owns backoff/retry and bounded diagnostics for durable workers.
func (s *GameService) ServeP13GuildGiftMonthly(ctx context.Context) error {
	if err := s.TickP13GuildGiftMonthly(ctx, LocalPlayNow(ctx)); err != nil {
		if ctx.Err() != nil {
			return nil
		}
		return err
	}
	ticker := time.NewTicker(constUtil.P13GuildGiftInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			if err := s.TickP13GuildGiftMonthly(ctx, LocalPlayNow(ctx)); err != nil {
				if ctx.Err() != nil {
					return nil
				}
				return err
			}
		}
	}
}
