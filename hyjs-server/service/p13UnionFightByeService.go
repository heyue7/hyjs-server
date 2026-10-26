package service

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"reflect"
	"sort"
	"strings"
	"time"

	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	pb "hyjs-server/model/protocol"
)

type p13UFByeCandidate struct {
	GuildID  string                    `json:"guildId"`
	Power    int64                     `json:"power"`
	Entry    db.P13UnionFightEntry     `json:"entry"`
	Group    db.P13UnionFightGroup     `json:"group"`
	Fighters []db.P13UnionFightFighter `json:"fighters"`
	Eligible []int64                   `json:"eligible"`
}
type p13UFByeEntryIdentity struct {
	GuildID, Day, Group, Configuration, Client, Engine, Source string
	FrozenAt                                                   int64
	EligibleCount                                              int32
}

func p13UFByeEntryDirectory(tx *gorm.DB, day, week, group string) ([]p13UFByeEntryIdentity, error) {
	result := []p13UFByeEntryIdentity{}
	last := ""
	for {
		var rows []p13UFByeEntryIdentity
		q := tx.Table("p13_union_fight_entries AS e").Select("e.guild_id,e.day,g.`group`,e.configuration_hash AS configuration,e.client_version AS client,e.engine_version AS engine,e.source,e.frozen_at,e.eligible_count").Joins("JOIN p13_union_fight_groups AS g ON g.guild_id=e.guild_id AND g.week_start=?", week).Where("e.day=? AND e.frozen_at>0 AND e.eligible_count>0 AND g.`group`=?", day, group)
		if last != "" {
			q = q.Where("e.guild_id>?", last)
		}
		if err := q.Order("e.guild_id").Limit(128).Scan(&rows).Error; err != nil {
			return nil, err
		}
		result = append(result, rows...)
		if len(rows) < 128 {
			break
		}
		last = rows[len(rows)-1].GuildID
	}
	return result, nil
}

type p13UFByeMatch struct {
	ID, Left, Right, Hash, Configuration, Client, Engine string
	Round                                                int32
	StartAt, EndAt                                       int64
}
type p13UFByeSnapshot struct {
	Rule            string                           `json:"rule"`
	RegistrationDay string                           `json:"registrationDay"`
	Week            string                           `json:"week"`
	Schedule        db.P13UnionFightSchedule         `json:"schedule"`
	Candidates      []p13UFByeCandidate              `json:"candidates"`
	EntryDirectory  []p13UFByeEntryIdentity          `json:"entryDirectory"`
	Matches         []p13UFByeMatch                  `json:"matches"`
	Roster          []p13UFMatchFighter              `json:"roster"`
	Preview         []byte                           `json:"preview"`
	Policy          db.P13UnionFightSettlementPolicy `json:"policy"`
	MatchedAt       int64                            `json:"matchedAt"`
}
type p13UFByeSeal struct {
	ID   string `json:"id"`
	Hash string `json:"hash"`
}

func p13UFByeError() error { return errors.New(constUtil.P13UnionFightByeConflict) }
func p13UFByeID(day, guild string, round int32) string {
	return p6Digest([]byte(fmt.Sprintf("%s/%s/%s/%d", constUtil.P13UnionFightByeRule, day, guild, round)))
}
func p13UFByeHash(row *db.P13UnionFightBye) string {
	copy := *row
	copy.Snapshot = nil
	copy.SourceHash = ""
	raw, _ := json.Marshal(copy)
	return p6Digest(raw)
}
func p13UFByeEntry(row db.P13UnionFightEntry) db.P13UnionFightEntry {
	row.MatchProcessed = false
	row.FrozenTables = nil
	return row
}
func p13UFByeMatchIdentity(row db.P13UnionFightMatch) p13UFByeMatch {
	return p13UFByeMatch{row.ID, row.LeftGuildID, row.RightGuildID, row.SnapshotHash, row.ConfigurationHash, row.ClientVersion, row.EngineVersion, row.Round, row.StartAt, row.EndAt}
}

// Source comparisons use the actual stored byte digest, never the editable
// configuration_hash column alone. Every query is bounded to128 guild keys.
func p13UFByeSources(tx *gorm.DB, day, week string, ids []string) (map[string]db.P13UnionFightEntry, map[string]db.P13UnionFightGroup, map[string][]db.P13UnionFightFighter, error) {
	entries := map[string]db.P13UnionFightEntry{}
	groups := map[string]db.P13UnionFightGroup{}
	fighters := map[string][]db.P13UnionFightFighter{}
	for begin := 0; begin < len(ids); begin += 128 {
		end := min(begin+128, len(ids))
		keys := ids[begin:end]
		var erows []struct {
			db.P13UnionFightEntry
			ActualHash string
		}
		selectEntry := "*"
		if tx.Dialector.Name() == "mysql" {
			selectEntry = "guild_id,day,applied_at,deadline,frozen_at,eligible_count,adjust_times_used,match_processed,configuration_hash,engine_version,client_version,source,LOWER(SHA2(frozen_tables,256)) AS actual_hash"
		}
		if err := tx.Model(&db.P13UnionFightEntry{}).Select(selectEntry).Where("day=? AND guild_id IN ?", day, keys).Find(&erows).Error; err != nil {
			return nil, nil, nil, err
		}
		for _, v := range erows {
			actual := v.ActualHash
			if tx.Dialector.Name() != "mysql" {
				actual = p6Digest(v.FrozenTables)
			}
			if actual != v.ConfigurationHash {
				return nil, nil, nil, p13UFByeError()
			}
			entries[v.GuildID] = p13UFByeEntry(v.P13UnionFightEntry)
		}
		var grows []db.P13UnionFightGroup
		if err := tx.Where("week_start=? AND guild_id IN ?", week, keys).Find(&grows).Error; err != nil {
			return nil, nil, nil, err
		}
		for _, v := range grows {
			groups[v.GuildID] = v
		}
		lastGuild := ""
		lastPlayer := int64(0)
		for {
			var frows []struct {
				db.P13UnionFightFighter
				ActualHash string
			}
			q := tx.Model(&db.P13UnionFightFighter{}).Select("p13_union_fight_fighters.*").Where("day=? AND guild_id IN ? AND applied_at>0", day, keys)
			if lastGuild != "" {
				q = q.Where("guild_id>? OR (guild_id=? AND player_id>?)", lastGuild, lastGuild, lastPlayer)
			}
			if tx.Dialector.Name() == "mysql" {
				q = q.Select("guild_id,day,player_id,role,applied_at,first_on,auto_fight,name,fight_power,snapshot_hash,updated_at,battle_adjust_times,default_items,LOWER(SHA2(battle,256)) AS actual_hash")
			}
			if err := q.Order("guild_id,player_id").Limit(128).Find(&frows).Error; err != nil {
				return nil, nil, nil, err
			}
			for _, v := range frows {
				actual := v.ActualHash
				if tx.Dialector.Name() != "mysql" {
					actual = p6Digest(v.Battle)
				}
				if actual != v.SnapshotHash {
					return nil, nil, nil, p13UFByeError()
				}
				v.Battle = nil
				fighters[v.GuildID] = append(fighters[v.GuildID], v.P13UnionFightFighter)
			}
			if len(frows) < 128 {
				break
			}
			last := frows[len(frows)-1]
			lastGuild, lastPlayer = last.GuildID, last.PlayerID
		}
	}
	return entries, groups, fighters, nil
}
func p13UFByeMatches(tx *gorm.DB, day, group string) ([]p13UFByeMatch, error) {
	result := []p13UFByeMatch{}
	cursor := ""
	for {
		var rows []db.P13UnionFightMatch
		q := tx.Select("id,left_guild_id,right_guild_id,snapshot_hash,configuration_hash,client_version,engine_version,round,start_at,end_at").Where("day=? AND `group`=?", day, group)
		if cursor != "" {
			q = q.Where("id>?", cursor)
		}
		if err := q.Order("id").Limit(128).Find(&rows).Error; err != nil {
			return nil, err
		}
		for _, row := range rows {
			result = append(result, p13UFByeMatchIdentity(row))
		}
		if len(rows) < 128 {
			break
		}
		cursor = rows[len(rows)-1].ID
	}
	return result, nil
}
func p13UFByePolicyValid(rule *db.P13UnionFightSettlementPolicy, at, start int64) bool {
	return at < start && rule.Enabled && rule.Version != "" && strings.TrimSpace(rule.Source) != "" && rule.PublishedAt > 0 && rule.PublishedAt <= at && rule.PublishedAt < start && rule.WinPrestige >= 0 && rule.LosePrestige >= 0 && rule.FactoryMissionPrestige >= 0 && int64(rule.WinPrestige)+int64(rule.FactoryMissionPrestige) <= math.MaxInt32 && int64(rule.LosePrestige)+int64(rule.FactoryMissionPrestige) <= math.MaxInt32 && rule.ParticipationRewards != "" && rule.VictoryRewards != "" && p13UFAbortRuleValid(rule) && (rule.TerminalRule == "" && rule.DrawPrestige == 0 || rule.TerminalRule == constUtil.P13UFMixedTerminalRule && rule.DrawPrestige == 5)
}

// Called only for new, unprocessed matching candidates under the established
// guild-policy lock. Old processed registrations are never revisited.
func (s *GameService) p13UFByeProduce(tx *gorm.DB, schedule *db.P13UnionFightSchedule, registrationDay, week, group string, bucket []*p13UFMatchCandidate, now time.Time) error {
	if len(bucket) == 0 || len(bucket)%2 == 0 {
		return nil
	}
	if !schedule.Enabled || schedule.Version == "" || schedule.PublishedAt <= 0 || schedule.PublishedAt > now.UnixMilli() || now.UnixMilli() >= schedule.FirstStartAt || p13UFCustomScheduleValid(schedule, schedule.PreparationMillis) != nil {
		return nil
	}
	tail := bucket[len(bucket)-1]
	var rule db.P13UnionFightSettlementPolicy
	err := tx.First(&rule, "`group`=?", group).Error
	if errors.Is(err, gorm.ErrRecordNotFound) || err == nil && (!rule.Enabled || rule.PublishedAt > now.UnixMilli() || rule.PublishedAt >= schedule.FirstStartAt) {
		return nil
	}
	if err != nil {
		return err
	}
	if !p13UFByePolicyValid(&rule, now.UnixMilli(), schedule.FirstStartAt) {
		return p13UFByeError()
	}
	if _, err = p13UFMissionCatalog(&rule); err != nil {
		return err
	}
	ids := []string{}
	for _, c := range bucket {
		ids = append(ids, c.guild.ID)
	}
	entries, groups, fighters, err := p13UFByeSources(tx, registrationDay, week, ids)
	if err != nil {
		return err
	}
	snapshot := p13UFByeSnapshot{Rule: constUtil.P13UnionFightByeRule, RegistrationDay: registrationDay, Week: week, Schedule: *schedule, Policy: rule, MatchedAt: now.UnixMilli()}
	for _, c := range bucket {
		entry, ok := entries[c.guild.ID]
		if !ok || entry.FrozenAt <= 0 || entry.EligibleCount <= 0 {
			return p13UFByeError()
		}
		g, ok := groups[c.guild.ID]
		if !ok || g.Group != group {
			return p13UFByeError()
		}
		candidate := p13UFByeCandidate{GuildID: c.guild.ID, Power: c.power, Entry: entry, Group: g, Fighters: fighters[c.guild.ID]}
		for _, f := range c.fighters {
			candidate.Eligible = append(candidate.Eligible, f.PlayerID)
		}
		snapshot.Candidates = append(snapshot.Candidates, candidate)
	}
	snapshot.EntryDirectory, err = p13UFByeEntryDirectory(tx, registrationDay, week, group)
	if err != nil {
		return err
	}
	snapshot.Matches, err = p13UFByeMatches(tx, schedule.Day, group)
	if err != nil {
		return err
	}
	for _, fighter := range tail.fighters {
		snapshot.Roster = append(snapshot.Roster, p13UFMatchFighter{P13UnionFightFighter: fighter, BattlePayload: append([]byte(nil), fighter.Battle...)})
	}
	snapshot.Preview, err = s.p13UFGuildPreview(tx, tail.guild, tail.power)
	if err != nil {
		return err
	}
	raw, err := json.Marshal(snapshot)
	if err != nil {
		return err
	}
	for i, at := range []int64{schedule.FirstStartAt, schedule.SecondStartAt} {
		row := db.P13UnionFightBye{ID: p13UFByeID(schedule.Day, tail.guild.ID, int32(i+1)), GuildID: tail.guild.ID, Day: schedule.Day, Round: int32(i + 1), Group: group, StartAt: at, EndAt: at + schedule.DurationMillis, CreatedAt: now.UnixMilli(), WinPrestige: rule.WinPrestige, Snapshot: raw, SnapshotHash: p6Digest(raw), Source: constUtil.P13UnionFightByeSource}
		row.SourceHash = p13UFByeHash(&row)
		if _, err := p13UFByeRead(&row); err != nil {
			return err
		}
		if err := tx.Create(&row).Error; err != nil {
			return err
		}
	}
	var ledger db.P13UnionFightSettlementPrestige
	err = tx.First(&ledger, "week_start=? AND guild_id=?", week, tail.guild.ID).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		ledger = db.P13UnionFightSettlementPrestige{WeekStart: week, GuildID: tail.guild.ID}
	} else if err != nil {
		return err
	}
	delta := int64(rule.WinPrestige) * 2
	if ledger.Score < 0 || ledger.Score > math.MaxInt32-delta {
		return p13UFByeError()
	}
	ledger.Score += delta
	if err = tx.Save(&ledger).Error; err != nil {
		return err
	}
	return s.p13UFByeWeeklyFreeze(tx, &snapshot, group, now)
}

func p13UFByeRead(row *db.P13UnionFightBye) (*p13UFByeSnapshot, error) {
	if row == nil || row.Source != constUtil.P13UnionFightByeSource || row.SourceHash != p13UFByeHash(row) || row.SnapshotHash != p6Digest(row.Snapshot) || row.ID != p13UFByeID(row.Day, row.GuildID, row.Round) || row.Round < 1 || row.Round > 2 {
		return nil, p13UFByeError()
	}
	var b p13UFByeSnapshot
	if json.Unmarshal(row.Snapshot, &b) != nil || b.Rule != constUtil.P13UnionFightByeRule || b.Schedule.Day != row.Day || b.MatchedAt != row.CreatedAt || !p13UFByePolicyValid(&b.Policy, row.CreatedAt, b.Schedule.FirstStartAt) || b.Policy.Group != row.Group || b.Policy.WinPrestige != row.WinPrestige || len(b.Candidates) == 0 || len(b.Candidates)%2 != 1 || len(b.Roster) == 0 || b.RegistrationDay == "" {
		return nil, p13UFByeError()
	}
	day, err := time.ParseInLocation("2006-01-02", row.Day, time.FixedZone("Asia/Shanghai", 8*3600))
	if err != nil || p13UFWeek(day) != b.Week || b.Schedule.DurationMillis <= 0 || b.Schedule.SecondStartAt < b.Schedule.FirstStartAt+b.Schedule.DurationMillis {
		return nil, p13UFByeError()
	}
	at := b.Schedule.FirstStartAt
	if row.Round == 2 {
		at = b.Schedule.SecondStartAt
	}
	if row.StartAt != at || row.EndAt != at+b.Schedule.DurationMillis {
		return nil, p13UFByeError()
	}
	if !b.Schedule.Enabled || b.Schedule.PublishedAt > b.MatchedAt || b.MatchedAt >= b.Schedule.FirstStartAt || p13UFCustomScheduleValid(&b.Schedule, b.Schedule.PreparationMillis) != nil {
		return nil, p13UFByeError()
	}
	tail := b.Candidates[len(b.Candidates)-1]
	if tail.GuildID != row.GuildID || len(tail.Eligible) != len(b.Roster) {
		return nil, p13UFByeError()
	}
	seen := map[string]bool{}
	for i, c := range b.Candidates {
		if seen[c.GuildID] || c.GuildID == "" || c.Power < 0 || c.Entry.GuildID != c.GuildID || c.Entry.Day != b.RegistrationDay || c.Entry.MatchProcessed || c.Entry.FrozenAt <= 0 || c.Entry.EligibleCount <= 0 || c.Group.WeekStart != b.Week || c.Group.GuildID != c.GuildID || c.Group.Group != row.Group || len(c.Eligible) == 0 {
			return nil, p13UFByeError()
		}
		seen[c.GuildID] = true
		if i > 0 {
			p := b.Candidates[i-1]
			if p.Power < c.Power || p.Power == c.Power && p.GuildID > c.GuildID {
				return nil, p13UFByeError()
			}
		}
	}
	total := int64(0)
	players := map[int64]bool{}
	for _, f := range b.Roster {
		if f.GuildID != row.GuildID || f.Day != b.RegistrationDay || f.PlayerID <= 0 || f.AppliedAt <= 0 || players[f.PlayerID] || p6Digest(f.BattlePayload) != f.SnapshotHash || f.FightPower < 0 || total > math.MaxInt64-f.FightPower {
			return nil, p13UFByeError()
		}
		players[f.PlayerID] = true
		total += f.FightPower
	}
	for _, id := range tail.Eligible {
		if !players[id] {
			return nil, p13UFByeError()
		}
	}
	if total != tail.Power {
		return nil, p13UFByeError()
	}
	var preview pb.UFInfoPreView
	if proto.Unmarshal(b.Preview, &preview) != nil || preview.GetUnionId() != row.GuildID || preview.GetFightPower() != tail.Power || preview.GetServerId() <= 0 || preview.GetUnionName() == "" {
		return nil, p13UFByeError()
	}
	return &b, nil
}

// A full directory check groups both round facts, then batch-loads immutable
// signup and fighter sources. No current membership is retroactively applied.
func p13UFByeVerifyBatch(tx *gorm.DB, row *db.P13UnionFightBye, b *p13UFByeSnapshot) error {
	var schedule db.P13UnionFightSchedule
	if err := tx.First(&schedule, "day=?", row.Day).Error; err != nil {
		return err
	}
	if !reflect.DeepEqual(schedule, b.Schedule) {
		return p13UFByeError()
	}
	ids := []string{}
	for _, c := range b.Candidates {
		ids = append(ids, c.GuildID)
	}
	entries, groups, fighters, err := p13UFByeSources(tx, b.RegistrationDay, b.Week, ids)
	if err != nil {
		return err
	}
	for _, c := range b.Candidates {
		if !reflect.DeepEqual(entries[c.GuildID], c.Entry) || !reflect.DeepEqual(groups[c.GuildID], c.Group) || !reflect.DeepEqual(fighters[c.GuildID], c.Fighters) {
			return p13UFByeError()
		}
	}
	directory, err := p13UFByeEntryDirectory(tx, b.RegistrationDay, b.Week, row.Group)
	if err != nil {
		return err
	}
	if !reflect.DeepEqual(directory, b.EntryDirectory) {
		return p13UFByeError()
	}
	actual, err := p13UFByeMatches(tx, row.Day, row.Group)
	if err != nil {
		return err
	}
	if !reflect.DeepEqual(actual, b.Matches) {
		return p13UFByeError()
	}
	// Only the true final odd element is a bye. An incompatible paired pair is
	// still unresolved and cannot be transformed into two free wins.
	for i := 0; i+1 < len(b.Candidates); i += 2 {
		left, right := b.Candidates[i], b.Candidates[i+1]
		if left.Entry.ConfigurationHash != right.Entry.ConfigurationHash || left.Entry.ClientVersion != right.Entry.ClientVersion || left.Entry.EngineVersion != right.Entry.EngineVersion {
			continue
		}
		for round := int32(1); round <= 2; round++ {
			n := 0
			for _, m := range actual {
				if m.Left == left.GuildID && m.Right == right.GuildID && m.Round == round {
					n++
				}
			}
			if n != 1 {
				return p13UFByeError()
			}
		}
	}
	var rounds []db.P13UnionFightBye
	if err = tx.Where("day=? AND `group`=?", row.Day, row.Group).Order("round,id").Limit(3).Find(&rounds).Error; err != nil {
		return err
	}
	if len(rounds) != 2 {
		return p13UFByeError()
	}
	for i, v := range rounds {
		if v.GuildID != row.GuildID || v.Round != int32(i+1) || v.SnapshotHash != row.SnapshotHash || !bytes.Equal(v.Snapshot, row.Snapshot) {
			return p13UFByeError()
		}
		if _, err = p13UFByeRead(&v); err != nil {
			return err
		}
	}
	for _, m := range actual {
		if m.Left == row.GuildID || m.Right == row.GuildID {
			return p13UFByeError()
		}
	}
	return nil
}
func p13UFByes(tx *gorm.DB, start, end, group string) ([]db.P13UnionFightBye, error) {
	result := []db.P13UnionFightBye{}
	last := ""
	for {
		var rows []db.P13UnionFightBye
		q := tx.Where("day>=? AND day<?", start, end)
		if group != "" {
			q = q.Where("`group`=?", group)
		}
		if last != "" {
			q = q.Where("id>?", last)
		}
		if err := q.Order("id").Limit(128).Find(&rows).Error; err != nil {
			return nil, err
		}
		result = append(result, rows...)
		if len(rows) < 128 {
			break
		}
		last = rows[len(rows)-1].ID
	}
	verified := map[string]bool{}
	for i := range result {
		row := &result[i]
		b, err := p13UFByeRead(row)
		if err != nil {
			return nil, err
		}
		key := row.Day + "/" + row.Group
		if !verified[key] {
			if err = p13UFByeVerifyBatch(tx, row, b); err != nil {
				return nil, err
			}
			verified[key] = true
		}
	}
	return result, nil
}
func p13UFByeSeals(rows []db.P13UnionFightBye) []p13UFByeSeal {
	var seals []p13UFByeSeal
	for _, row := range rows {
		seals = append(seals, p13UFByeSeal{row.ID, row.SourceHash})
	}
	sort.Slice(seals, func(i, j int) bool { return seals[i].ID < seals[j].ID })
	return seals
}
func p13UFByeDirectory(rows []db.P13UnionFightBye) []byte {
	if len(rows) == 0 {
		return nil
	}
	raw, _ := json.Marshal(p13UFByeSeals(rows))
	return raw
}
func p13UFByeDirectoryVerify(tx *gorm.DB, raw []byte, start, end, group string) error {
	var expected []p13UFByeSeal
	if len(raw) > 0 && (json.Unmarshal(raw, &expected) != nil || len(expected) == 0) {
		return p13UFByeError()
	}
	actual, err := p13UFByes(tx, start, end, group)
	if err != nil {
		return err
	}
	if !reflect.DeepEqual(p13UFByeSeals(actual), expected) {
		return p13UFByeError()
	}
	return nil
}
func p13UFNextDay(day string) string {
	at, err := time.Parse("2006-01-02", day)
	if err != nil {
		return day
	}
	return at.AddDate(0, 0, 1).Format("2006-01-02")
}

// Bye-only groups freeze the same pre-start weekly source without inventing a
// Match ID. A real earlier/equal match remains the established anchor.
func (s *GameService) p13UFByeWeeklyFreeze(tx *gorm.DB, b *p13UFByeSnapshot, group string, now time.Time) error {
	var existing db.P13UnionFightWeeklyOffer
	if err := tx.First(&existing, "week_start=? AND `group`=?", b.Week, group).Error; err == nil {
		return nil
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}
	start, err := p13UFWeeklyStart(b.Week)
	if err != nil {
		return err
	}
	end := start.AddDate(0, 0, 7).Format("2006-01-02")
	var first db.P13UnionFightMatch
	err = tx.Where("day>=? AND day<? AND `group`=?", b.Week, end, group).Order("start_at,id").First(&first).Error
	if err == nil && first.StartAt <= b.Schedule.FirstStartAt {
		return s.p13UFWeeklyFreeze(tx, &first, now)
	}
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}
	byes, err := p13UFByes(tx, b.Week, end, group)
	if err != nil {
		return err
	}
	sort.Slice(byes, func(i, j int) bool {
		if byes[i].StartAt != byes[j].StartAt {
			return byes[i].StartAt < byes[j].StartAt
		}
		return byes[i].ID < byes[j].ID
	})
	if len(byes) == 0 || now.UnixMilli() >= byes[0].StartAt {
		return nil
	}
	var rule db.P13UnionFightWeeklyPolicy
	if err = tx.First(&rule, "week_start=? AND `group`=?", b.Week, group).Error; errors.Is(err, gorm.ErrRecordNotFound) {
		return nil
	} else if err != nil {
		return err
	}
	if !rule.Enabled {
		return nil
	}
	raw, err := json.Marshal(rule)
	if err != nil {
		return err
	}
	offer := db.P13UnionFightWeeklyOffer{WeekStart: b.Week, Group: group, Rules: raw, RulesHash: p6Digest(raw), FrozenAt: now.UnixMilli(), FirstByeID: byes[0].ID}
	_, defs, err := p13UFWeeklyRules(&offer)
	if err != nil {
		return err
	}
	for _, def := range defs {
		ok, e := s.openServerRewardSupported(def.Rewards)
		if e != nil {
			return e
		}
		if !ok {
			return p13UFByeError()
		}
	}
	return tx.Create(&offer).Error
}
func p13UFByeWeeklyAnchor(tx *gorm.DB, offer *db.P13UnionFightWeeklyOffer) error {
	if offer.FirstByeID == "" {
		return nil
	}
	if offer.FirstMatchID != "" {
		return p13UFByeError()
	}
	start, err := p13UFWeeklyStart(offer.WeekStart)
	if err != nil {
		return err
	}
	byes, err := p13UFByes(tx, offer.WeekStart, start.AddDate(0, 0, 7).Format("2006-01-02"), offer.Group)
	if err != nil {
		return err
	}
	sort.Slice(byes, func(i, j int) bool {
		if byes[i].StartAt != byes[j].StartAt {
			return byes[i].StartAt < byes[j].StartAt
		}
		return byes[i].ID < byes[j].ID
	})
	if len(byes) == 0 || byes[0].ID != offer.FirstByeID || offer.FrozenAt >= byes[0].StartAt {
		return p13UFByeError()
	}
	var earlier int64
	if err = tx.Model(&db.P13UnionFightMatch{}).Where("day>=? AND day<? AND `group`=? AND start_at<=?", offer.WeekStart, start.AddDate(0, 0, 7).Format("2006-01-02"), offer.Group, byes[0].StartAt).Count(&earlier).Error; err != nil {
		return err
	}
	if earlier != 0 {
		return p13UFByeError()
	}
	return nil
}

// Current membership and realm are eligibility at the weekly cutoff, not a
// replacement for the frozen matcher roster. Offline roles can still qualify.
func p13UFByeCurrentMembers(tx *gorm.DB, byes []db.P13UnionFightBye) (map[int64]string, error) {
	ids := []int64{}
	seen := map[int64]bool{}
	for i := range byes {
		b, err := p13UFByeRead(&byes[i])
		if err != nil {
			return nil, err
		}
		for _, f := range b.Roster {
			if !seen[f.PlayerID] {
				seen[f.PlayerID] = true
				ids = append(ids, f.PlayerID)
			}
		}
	}
	result := map[int64]string{}
	for begin := 0; begin < len(ids); begin += 128 {
		end := min(begin+128, len(ids))
		var rows []struct {
			PlayerID  int64
			GuildID   string
			ProfileID int64
			ServerID  int64
			HasPlayer bool
		}
		if err := tx.Table("p13_guild_members AS m").Select("m.player_id,m.guild_id,p.id AS profile_id,p.server_id,p.player IS NOT NULL AS has_player").Joins("JOIN profiles AS p ON p.id=m.player_id").Where("m.player_id IN ?", ids[begin:end]).Find(&rows).Error; err != nil {
			return nil, err
		}
		for _, row := range rows {
			if row.ProfileID == row.PlayerID && row.HasPlayer && row.ServerID > 0 {
				result[row.PlayerID] = row.GuildID
			}
		}
	}
	return result, nil
}
