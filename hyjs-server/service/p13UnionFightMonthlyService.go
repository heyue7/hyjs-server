package service

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"sort"
	"time"

	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	pb "hyjs-server/model/protocol"
)

type p13UFMonthlyTitleRule struct {
	ID       string `json:"id"`
	Effects  string `json:"effects"`
	Hours    int64  `json:"hours"`
	MaxRank  int32  `json:"maxRank"`
	Chairman bool   `json:"chairman"`
}
type p13UFMonthlyRules struct {
	Policy db.P13UnionFightMonthlyPolicy `json:"policy"`
	Titles []p13UFMonthlyTitleRule       `json:"titles"`
}
type p13UFMonthlyIdentity struct {
	PlayerID int64  `json:"playerId"`
	GuildID  string `json:"guildId"`
	Role     int32  `json:"role"`
	JoinedAt int64  `json:"joinedAt"`
}
type p13UFMonthlyGuild struct {
	GuildID      string `json:"guildId"`
	Name         string `json:"name"`
	ChairmanID   int64  `json:"chairmanId"`
	ChairmanName string `json:"chairmanName"`
}
type p13UFMonthlyAward struct {
	PlayerID int64                   `json:"playerId"`
	GuildID  string                  `json:"guildId"`
	Role     int32                   `json:"role"`
	Rank     int32                   `json:"rank"`
	Rewards  string                  `json:"rewards"`
	Titles   []p13UFMonthlyTitleRule `json:"titles"`
}

// InitializeP13UnionFightMonthly creates no enabled policy or retrospective offer.
func (s *GameService) InitializeP13UnionFightMonthly(ctx context.Context) error {
	return db.GetIns().WithContext(ctx).AutoMigrate(&db.P13UnionFightMonthlyPolicy{}, &db.P13UnionFightMonthlyOffer{}, &db.P13UnionFightMonthlyCutoff{}, &db.P13UnionFightMonthlyResult{}, &db.P13UnionFightMonthlyTitle{})
}
func p13UFMonth(month string) (time.Time, time.Time, error) {
	start, err := time.ParseInLocation("2006-01", month, time.FixedZone("Asia/Shanghai", 8*3600))
	if err != nil || start.Format("2006-01") != month {
		return time.Time{}, time.Time{}, errors.New("公会战月份无效")
	}
	return start, start.AddDate(0, 1, 0), nil
}
func p13UFMonthlyReadRules(offer *db.P13UnionFightMonthlyOffer) (*p13UFMonthlyRules, []p13UFWeeklyReward, error) {
	var rules p13UFMonthlyRules
	start, end, err := p13UFMonth(offer.Month)
	if err != nil {
		return nil, nil, err
	}
	if p6Digest(offer.Rules) != offer.RulesHash || json.Unmarshal(offer.Rules, &rules) != nil || rules.Policy.Month != offer.Month || !rules.Policy.Enabled || rules.Policy.Version == "" || rules.Policy.Source == "" || rules.Policy.PublishedAt <= 0 || rules.Policy.PublishedAt > offer.FrozenAt || offer.FrozenAt >= start.UnixMilli() || offer.StartAt != start.UnixMilli() || offer.CutoffAt != end.UnixMilli() {
		return nil, nil, errors.New("公会战月赛提前冻结来源无效")
	}
	var rewards []p13UFWeeklyReward
	if json.Unmarshal([]byte(rules.Policy.Rewards), &rewards) != nil || len(rewards) > 1000 {
		return nil, nil, errors.New("本地月赛奖档无效")
	}
	seen := map[int32]bool{}
	for _, r := range rewards {
		if r.Rank < 1 || r.Rank > 1000 || r.Rewards == "" || seen[r.Rank] {
			return nil, nil, errors.New("本地月赛名次资源无效")
		}
		seen[r.Rank] = true
	}
	titleSeen := map[string]bool{}
	for _, r := range rules.Titles {
		if titleSeen[r.ID] || r.Effects == "" || r.Hours <= 0 || r.Hours > 87600 {
			return nil, nil, errors.New("原月赛称号冻结无效")
		}
		if r.ID == "title_028" {
			if !rules.Policy.MemberTitle || r.MaxRank != 20 || r.Chairman {
				return nil, nil, p5Error()
			}
		} else if r.ID == "title_029" {
			if !rules.Policy.ChairmanTitle || r.MaxRank != 5 || !r.Chairman {
				return nil, nil, p5Error()
			}
		} else {
			return nil, nil, p5Error()
		}
		titleSeen[r.ID] = true
	}
	if titleSeen["title_028"] != rules.Policy.MemberTitle || titleSeen["title_029"] != rules.Policy.ChairmanTitle {
		return nil, nil, p5Error()
	}
	return &rules, rewards, nil
}
func (s *GameService) p13UFMonthlyArm(tx *gorm.DB, now time.Time) error {
	var policies []db.P13UnionFightMonthlyPolicy
	if err := tx.Where("enabled=?", true).Order("month").Find(&policies).Error; err != nil {
		return err
	}
	for _, policy := range policies {
		start, end, err := p13UFMonth(policy.Month)
		if err != nil {
			return err
		}
		// Publication after the actual boundary never reconstructs a historical month.
		if !now.Before(start) {
			continue
		}
		var n int64
		if err = tx.Model(&db.P13UnionFightMonthlyOffer{}).Where("month=?", policy.Month).Count(&n).Error; err != nil {
			return err
		}
		if n > 0 {
			continue
		}
		rules := p13UFMonthlyRules{Policy: policy}
		for _, choice := range []struct {
			id       string
			enabled  bool
			rank     int32
			chairman bool
		}{{"title_028", policy.MemberTitle, 20, false}, {"title_029", policy.ChairmanTitle, 5, true}} {
			if !choice.enabled {
				continue
			}
			row := s.table("Title", "titleId", choice.id)
			hours := int64(tableNumber(row, "effectTime"))
			if row == nil || hours <= 0 || hours > 87600 || row["effects"] == "" {
				return p5Error()
			}
			rules.Titles = append(rules.Titles, p13UFMonthlyTitleRule{ID: choice.id, Effects: row["effects"], Hours: hours, MaxRank: choice.rank, Chairman: choice.chairman})
		}
		raw, err := json.Marshal(rules)
		if err != nil {
			return err
		}
		offer := db.P13UnionFightMonthlyOffer{Month: policy.Month, StartAt: start.UnixMilli(), CutoffAt: end.UnixMilli(), Rules: raw, RulesHash: p6Digest(raw), FrozenAt: now.UnixMilli()}
		_, rewards, err := p13UFMonthlyReadRules(&offer)
		if err != nil {
			return err
		}
		for _, r := range rewards {
			ok, e := s.openServerRewardSupported(r.Rewards)
			if e != nil {
				return e
			}
			if !ok {
				return errors.New("本地月赛奖励资源未恢复")
			}
		}
		if err = tx.Create(&offer).Error; err != nil {
			return err
		}
	}
	return nil
}
func p13UFMonthlyCutoffHash(row *db.P13UnionFightMonthlyCutoff) string {
	raw, _ := json.Marshal(struct {
		Month, Offer     string
		Cutoff, Captured int64
		Members, Guilds  []byte
	}{row.Month, row.OfferHash, row.CutoffAt, row.CapturedAt, row.Members, row.Guilds})
	return p6Digest(raw)
}

// P13MonthlyBeforeGuildMutation runs under the existing guild policy lock, before
// any post-boundary membership, chairman, join or dissolution mutation. The same
// hook at the worker boundary covers idle/offline accounts without moving cutoff.
func (s *GameService) P13MonthlyBeforeGuildMutation(tx *gorm.DB, now time.Time) error {
	if !gameSchemaHasTable(tx, &db.P13UnionFightMonthlyOffer{}) {
		return nil
	}
	if err := s.p13UFMonthlyArm(tx, now); err != nil {
		return err
	}
	var offers []db.P13UnionFightMonthlyOffer
	if err := tx.Where("cutoff_at<=?", now.UnixMilli()).Order("month").Find(&offers).Error; err != nil {
		return err
	}
	for _, offer := range offers {
		var existing db.P13UnionFightMonthlyCutoff
		err := tx.First(&existing, "month=?", offer.Month).Error
		if err == nil {
			if existing.OfferHash != offer.RulesHash || existing.CutoffAt != offer.CutoffAt || existing.SnapshotHash != p13UFMonthlyCutoffHash(&existing) {
				return errors.New("月末身份归档无效")
			}
			continue
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		if _, _, err = p13UFMonthlyReadRules(&offer); err != nil {
			return err
		}
		var members []db.P13GuildMember
		if err = tx.Where("guild_id<>?", "").Order("guild_id,player_id").Find(&members).Error; err != nil {
			return err
		}
		identities := []p13UFMonthlyIdentity{}
		guildIDs := map[string]bool{}
		for _, m := range members {
			if m.PlayerID <= 0 || m.JoinedAt <= 0 || m.JoinedAt >= offer.CutoffAt || m.Role < constUtil.P13GuildMember || m.Role > constUtil.P13GuildChairman {
				return errors.New("月末实际公会身份无效")
			}
			identities = append(identities, p13UFMonthlyIdentity{m.PlayerID, m.GuildID, m.Role, m.JoinedAt})
			guildIDs[m.GuildID] = true
		}
		names := []p13UFMonthlyGuild{}
		for id := range guildIDs {
			var guild db.P13Guild
			if err = tx.First(&guild, "id=?", id).Error; err != nil {
				return err
			}
			view := p13UFMonthlyGuild{GuildID: id, Name: guild.Name}
			for _, m := range identities {
				if m.GuildID == id && m.Role == constUtil.P13GuildChairman {
					if view.ChairmanID != 0 {
						return errors.New("月末公会会长身份不唯一")
					}
					view.ChairmanID = m.PlayerID
					actor, e := s.p10Player(tx, m.PlayerID)
					if e != nil {
						return e
					}
					view.ChairmanName = actor.GetPlayerName()
				}
			}
			if view.Name == "" || view.ChairmanID <= 0 || view.ChairmanName == "" {
				return errors.New("月末公会会长来源缺失")
			}
			names = append(names, view)
		}
		sort.Slice(names, func(i, j int) bool { return names[i].GuildID < names[j].GuildID })
		rawMembers, err := json.Marshal(identities)
		if err != nil {
			return err
		}
		rawNames, err := json.Marshal(names)
		if err != nil {
			return err
		}
		frozen := db.P13UnionFightMonthlyCutoff{Month: offer.Month, OfferHash: offer.RulesHash, CutoffAt: offer.CutoffAt, CapturedAt: now.UnixMilli(), Members: rawMembers, Guilds: rawNames, Source: constUtil.P13UnionFightMonthlySource}
		frozen.SnapshotHash = p13UFMonthlyCutoffHash(&frozen)
		if err = tx.Create(&frozen).Error; err != nil {
			return err
		}
	}
	return nil
}

// p13UFMonthlyRanks derives month totals from real match facts, while reconciling
// the complete touched weeks against the spend-independent prestige ledger.
// Completeness applies only to this month's accepted entries and both rounds.
func p13UFMonthlyRanks(tx *gorm.DB, offer *db.P13UnionFightMonthlyOffer) ([]p13UFWeeklyRank, bool, error) {
	start, end, err := p13UFMonth(offer.Month)
	if err != nil {
		return nil, false, err
	}
	firstWeek, _ := time.ParseInLocation("2006-01-02", p13UFWeek(start), start.Location())
	lastWeek, _ := time.ParseInLocation("2006-01-02", p13UFWeek(end.Add(-time.Millisecond)), start.Location())
	var matches []db.P13UnionFightMatch
	if err = tx.Where("day>=? AND day<?", firstWeek.Format("2006-01-02"), lastWeek.AddDate(0, 0, 7).Format("2006-01-02")).Order("start_at,id").Find(&matches).Error; err != nil {
		return nil, false, err
	}
	var (
		monthScores  = map[string]int64{}
		weekScores   = map[string]map[string]int64{}
		previews     = map[string][]byte{}
		coverage     = map[string]map[int32]bool{}
		complete     = true
		monthMatches int
	)
	// Reconcile every intersecting week, including orphan legacy ledger rows
	// whose real terminal/factory source is absent from the match archive.
	for week := firstWeek; !week.After(lastWeek); week = week.AddDate(0, 0, 7) {
		weekScores[week.Format("2006-01-02")] = map[string]int64{}
	}
	for _, match := range matches {
		day, e := time.ParseInLocation("2006-01-02", match.Day, start.Location())
		if e != nil {
			return nil, false, e
		}
		inMonth := !day.Before(start) && day.Before(end)
		if inMonth {
			monthMatches++
			for _, guild := range []string{match.LeftGuildID, match.RightGuildID} {
				key := match.Day + "/" + guild
				if coverage[key] == nil {
					coverage[key] = map[int32]bool{}
				}
				if coverage[key][match.Round] || match.Round < 1 || match.Round > 2 {
					return nil, false, errors.New("月赛真实轮次重复或无效")
				}
				coverage[key][match.Round] = true
			}
		}
		var roundOffer db.P13UnionFightSettlementOffer
		e = tx.First(&roundOffer, "match_id=?", match.ID).Error
		if errors.Is(e, gorm.ErrRecordNotFound) {
			if inMonth {
				complete = false
			}
			continue
		}
		if e != nil {
			return nil, false, e
		}
		rule, e := p13UFSettlementRules(&roundOffer, &match)
		if e != nil {
			return nil, false, e
		}
		var result db.P13UnionFightSettlementResult
		e = tx.First(&result, "match_id=?", match.ID).Error
		terminal := e == nil
		if e != nil && !errors.Is(e, gorm.ErrRecordNotFound) {
			return nil, false, e
		}
		if inMonth && !terminal {
			complete = false
		}
		if terminal && (result.OfferHash != roundOffer.RulesHash || result.EndAt != match.EndAt || result.WinnerGuildID == result.LoserGuildID || result.WinnerGuildID != match.LeftGuildID && result.WinnerGuildID != match.RightGuildID || result.LoserGuildID != match.LeftGuildID && result.LoserGuildID != match.RightGuildID) {
			return nil, false, errors.New("月赛缺真实完整终局来源")
		}
		week := p13UFWeek(day)
		if weekScores[week] == nil {
			weekScores[week] = map[string]int64{}
		}
		for _, guild := range []string{match.LeftGuildID, match.RightGuildID} {
			extra, e := p13UFGuildMissionPrestige(tx, &match, guild)
			if e != nil {
				return nil, false, e
			}
			delta := int64(extra)
			if terminal {
				if guild == result.WinnerGuildID {
					if result.WinTaskPrestige != extra {
						return nil, false, errors.New("月赛胜方工厂事实与终局不一致")
					}
					delta += int64(rule.WinPrestige)
				} else {
					if result.LoseTaskPrestige != extra {
						return nil, false, errors.New("月赛负方工厂事实与终局不一致")
					}
					delta += int64(rule.LosePrestige)
				}
			}
			if delta < 0 || weekScores[week][guild] > math.MaxInt32-delta {
				return nil, false, errors.New("月赛真实威望溢出")
			}
			weekScores[week][guild] += delta
			if inMonth {
				if monthScores[guild] > math.MaxInt32-delta {
					return nil, false, errors.New("月赛累计威望溢出")
				}
				monthScores[guild] += delta
				raw := match.LeftPreview
				if guild == match.RightGuildID {
					raw = match.RightPreview
				}
				var view pb.UFInfoPreView
				if proto.Unmarshal(raw, &view) != nil || view.GetUnionId() != guild || view.GetUnionName() == "" || view.GetMasterName() == "" || view.GetServerId() <= 0 {
					return nil, false, errors.New("月赛原榜单身份字段缺失")
				}
				previews[guild] = raw
			}
		}
	}
	var entries []db.P13UnionFightEntry
	if err = tx.Where("day>=? AND day<?", start.AddDate(0, 0, -1).Format("2006-01-02"), end.AddDate(0, 0, -1).Format("2006-01-02")).Find(&entries).Error; err != nil {
		return nil, false, err
	}
	declaredCoverage := map[string]bool{}
	for _, entry := range entries {
		if entry.FrozenAt <= 0 {
			complete = false
			continue
		}
		if entry.EligibleCount == 0 {
			continue
		}
		declared, e := time.ParseInLocation("2006-01-02", entry.Day, start.Location())
		if e != nil {
			return nil, false, e
		}
		coverageKey := declared.AddDate(0, 0, 1).Format("2006-01-02") + "/" + entry.GuildID
		declaredCoverage[coverageKey] = true
		rounds := coverage[coverageKey]
		if !entry.MatchProcessed || !rounds[1] || !rounds[2] {
			complete = false
		}
	}
	for key := range coverage {
		if !declaredCoverage[key] {
			complete = false
		}
	}
	// A missing actual terminal/round stays pending before a complete ledger claim.
	if complete {
		for week, scores := range weekScores {
			var ledgers []db.P13UnionFightSettlementPrestige
			if err = tx.Where("week_start=?", week).Find(&ledgers).Error; err != nil {
				return nil, false, err
			}
			for _, row := range ledgers {
				if row.Score < 0 || row.Score != scores[row.GuildID] {
					return nil, false, errors.New("月赛真实威望与周账本来源不守恒")
				}
				delete(scores, row.GuildID)
			}
			for _, score := range scores {
				if score != 0 {
					return nil, false, errors.New("月赛真实事实缺周威望账本")
				}
			}
		}
	}
	ranks := []p13UFWeeklyRank{}
	for guild, score := range monthScores {
		if score > 0 {
			ranks = append(ranks, p13UFWeeklyRank{GuildID: guild, Score: score, Preview: previews[guild]})
		}
	}
	sort.Slice(ranks, func(i, j int) bool {
		if ranks[i].Score != ranks[j].Score {
			return ranks[i].Score > ranks[j].Score
		}
		return ranks[i].GuildID < ranks[j].GuildID
	})
	for i := range ranks {
		ranks[i].Rank = int32(i + 1)
		if i > 0 && ranks[i].Score == ranks[i-1].Score {
			ranks[i].Rank = ranks[i-1].Rank
		}
	}
	return ranks, complete && monthMatches > 0, nil
}
func p13UFMonthlyResultHash(row *db.P13UnionFightMonthlyResult) string {
	raw, _ := json.Marshal(struct {
		Month, Offer, Cutoff string
		Ranks, Awards        []byte
		Settled              int64
		Source               string
	}{row.Month, row.OfferHash, row.CutoffHash, row.Rankings, row.Awards, row.SettledAt, row.Source})
	return p6Digest(raw)
}
func (s *GameService) p13UFMonthlySettle(tx *gorm.DB, offer *db.P13UnionFightMonthlyOffer, now time.Time) (*db.P13UnionFightMonthlyResult, error) {
	if now.UnixMilli() < offer.CutoffAt {
		return nil, nil
	}
	var existing db.P13UnionFightMonthlyResult
	err := tx.First(&existing, "month=?", offer.Month).Error
	if err == nil {
		if existing.ResultHash != p13UFMonthlyResultHash(&existing) || existing.OfferHash != offer.RulesHash {
			return nil, errors.New("月赛终局归档无效")
		}
		return &existing, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}
	rules, rewards, err := p13UFMonthlyReadRules(offer)
	if err != nil {
		return nil, err
	}
	var cutoff db.P13UnionFightMonthlyCutoff
	if err = tx.First(&cutoff, "month=?", offer.Month).Error; errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	} else if err != nil {
		return nil, err
	}
	if cutoff.SnapshotHash != p13UFMonthlyCutoffHash(&cutoff) || cutoff.OfferHash != offer.RulesHash || cutoff.CutoffAt != offer.CutoffAt {
		return nil, errors.New("月赛截止身份来源无效")
	}
	ranks, complete, err := p13UFMonthlyRanks(tx, offer)
	if err != nil || !complete {
		return nil, err
	}
	var (
		members []p13UFMonthlyIdentity
		guilds  []p13UFMonthlyGuild
	)
	if json.Unmarshal(cutoff.Members, &members) != nil || json.Unmarshal(cutoff.Guilds, &guilds) != nil {
		return nil, errors.New("月末身份快照损坏")
	}
	byGuild := map[string]p13UFMonthlyGuild{}
	for _, g := range guilds {
		byGuild[g.GuildID] = g
	}
	byRank := map[int32]string{}
	for _, reward := range rewards {
		byRank[reward.Rank] = reward.Rewards
	}
	awards := []p13UFMonthlyAward{}
	for i := range ranks {
		rank := &ranks[i]
		if guild, exists := byGuild[rank.GuildID]; exists {
			var view pb.UFInfoPreView
			if err = proto.Unmarshal(rank.Preview, &view); err != nil {
				return nil, err
			}
			view.UnionName = proto.String(guild.Name)
			view.MasterName = proto.String(guild.ChairmanName)
			rank.Preview, err = proto.Marshal(&view)
			if err != nil {
				return nil, err
			}
		}
		for _, member := range members {
			if member.GuildID != rank.GuildID {
				continue
			}
			award := p13UFMonthlyAward{PlayerID: member.PlayerID, GuildID: member.GuildID, Role: member.Role, Rank: rank.Rank, Rewards: byRank[rank.Rank]}
			for _, title := range rules.Titles {
				if rank.Rank <= title.MaxRank && (!title.Chairman || member.Role == constUtil.P13GuildChairman) {
					award.Titles = append(award.Titles, title)
				}
			}
			if award.Rewards != "" || len(award.Titles) > 0 {
				awards = append(awards, award)
			}
		}
	}
	sort.Slice(awards, func(i, j int) bool { return awards[i].PlayerID < awards[j].PlayerID })
	rawRanks, err := json.Marshal(ranks)
	if err != nil {
		return nil, err
	}
	rawAwards, err := json.Marshal(awards)
	if err != nil {
		return nil, err
	}
	result := db.P13UnionFightMonthlyResult{Month: offer.Month, OfferHash: offer.RulesHash, CutoffHash: cutoff.SnapshotHash, Rankings: rawRanks, Awards: rawAwards, SettledAt: now.UnixMilli(), Source: constUtil.P13UnionFightMonthlySource + "; " + rules.Policy.Version + "; " + rules.Policy.Source}
	result.ResultHash = p13UFMonthlyResultHash(&result)
	if err = tx.Create(&result).Error; err != nil {
		return nil, err
	}
	return &result, nil
}
func (s *GameService) TickP13UnionFightMonthly(ctx context.Context, now time.Time) error {
	return db.GetIns().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := ServerGameClockGuardTransaction(tx); err != nil {
			return err
		}

		var gate db.P13GuildPolicy
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&gate, 1).Error; err != nil {
			return err
		}
		if err := s.P13MonthlyBeforeGuildMutation(tx, now); err != nil {
			return err
		}
		var offers []db.P13UnionFightMonthlyOffer
		if err := tx.Where("cutoff_at<=?", now.UnixMilli()).Order("month").Find(&offers).Error; err != nil {
			return err
		}
		for _, offer := range offers {
			result, err := s.p13UFMonthlySettle(tx, &offer, now)
			if err != nil {
				return err
			}
			if result == nil || result.DeliveredAt > 0 {
				continue
			}
			var awards []p13UFMonthlyAward
			if err = json.Unmarshal(result.Awards, &awards); err != nil {
				return err
			}
			for _, award := range awards {
				if award.Rewards != "" {
					if _, err = s.EnqueueReward(tx, award.PlayerID, "p13-month:"+offer.Month, "公会战月奖励", result.Source, award.Rewards, 0); err != nil {
						return err
					}
				}
				for _, title := range award.Titles {
					expires := offer.CutoffAt + title.Hours*3600000
					if expires < offer.CutoffAt {
						return p5Error()
					}
					row := db.P13UnionFightMonthlyTitle{Month: offer.Month, PlayerID: award.PlayerID, TitleID: title.ID, GuildID: award.GuildID, Rank: award.Rank, Role: award.Role, GrantedAt: offer.CutoffAt, ExpiresAt: expires, Effects: title.Effects, ResultHash: result.ResultHash, Source: result.Source}
					if err = tx.Create(&row).Error; err != nil {
						return err
					}
				}
			}
			if err = tx.Model(result).Update("delivered_at", now.UnixMilli()).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

// P13UnionFightMonthlyRank serves the original rankType2/subType0 entry. Live
// standings contain only observed prestige; a closed incomplete month stays pending.
func (s *GameService) P13UnionFightMonthlyRank(ctx context.Context, _ int64, b *pb.CSUFRankInfo, now time.Time) ([]proto.Message, error) {
	if b.GetRankType() != 2 || b.GetSubType() != 0 {
		return nil, errors.New("原公会战月榜类型无效")
	}
	if err := s.TickP13UnionFightMonthly(ctx, now); err != nil {
		return nil, err
	}
	out := &pb.SCUFRankInfo{RankType: b.RankType, SubType: b.SubType, RankIno: &pb.RankInfo{Type: proto.Int32(2)}}
	err := db.GetIns().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := ServerGameClockGuardTransaction(tx); err != nil {
			return err
		}

		var gate db.P13GuildPolicy
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&gate, 1).Error; err != nil {
			return err
		}
		var offer db.P13UnionFightMonthlyOffer
		if err := tx.Where("month<=?", now.In(time.FixedZone("Asia/Shanghai", 8*3600)).Format("2006-01")).Order("month DESC").First(&offer).Error; errors.Is(err, gorm.ErrRecordNotFound) {
			return errors.New("公会战月赛尚未提前发布")
		} else if err != nil {
			return err
		}
		if _, _, err := p13UFMonthlyReadRules(&offer); err != nil {
			return err
		}
		var ranks []p13UFWeeklyRank
		var result db.P13UnionFightMonthlyResult
		err := tx.First(&result, "month=?", offer.Month).Error
		if err == nil {
			if result.ResultHash != p13UFMonthlyResultHash(&result) {
				return errors.New("月榜冻结结果无效")
			}
			if err = json.Unmarshal(result.Rankings, &ranks); err != nil {
				return err
			}
		} else if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		} else {
			if now.UnixMilli() >= offer.CutoffAt {
				return errors.New("月赛尚有真实场次或轮次未终局，暂未结算")
			}
			ranks, _, err = p13UFMonthlyRanks(tx, &offer)
			if err != nil {
				return err
			}
		}
		for _, rank := range ranks {
			if rank.Rank > 1000 {
				break
			}
			var preview pb.UFInfoPreView
			if err = proto.Unmarshal(rank.Preview, &preview); err != nil {
				return err
			}
			out.RankIno.List = append(out.RankIno.List, &pb.RankRowInfo{Rank: proto.Int32(rank.Rank), Name: preview.MasterName, UnionName: preview.UnionName, Values: []int32{int32(rank.Score), preview.GetServerId()}})
		}
		return nil
	})
	return []proto.Message{out}, err
}

// p13UFMonthlyTitle reads only a delivered immutable cutoff entitlement. Current
// membership and current operator policy cannot move or revoke the earned award.
func p13UFMonthlyTitle(tx *gorm.DB, id int64, title string, now time.Time) (*db.P13UnionFightMonthlyTitle, error) {
	if !gameSchemaHasTable(tx, &db.P13UnionFightMonthlyTitle{}) {
		return nil, nil
	}
	var row db.P13UnionFightMonthlyTitle
	err := tx.Where("player_id=? AND title_id=? AND granted_at<=? AND expires_at>?", id, title, now.UnixMilli(), now.UnixMilli()).Order("granted_at DESC").First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var result db.P13UnionFightMonthlyResult
	if err = tx.First(&result, "month=?", row.Month).Error; err != nil {
		return nil, err
	}
	if result.DeliveredAt <= 0 || result.ResultHash != p13UFMonthlyResultHash(&result) || row.ResultHash != result.ResultHash {
		return nil, errors.New("月赛称号终局来源无效")
	}
	var offer db.P13UnionFightMonthlyOffer
	if err = tx.First(&offer, "month=?", row.Month).Error; err != nil {
		return nil, err
	}
	if row.GrantedAt != offer.CutoffAt || result.OfferHash != offer.RulesHash || row.Source != result.Source {
		return nil, p5Error()
	}
	var awards []p13UFMonthlyAward
	if err = json.Unmarshal(result.Awards, &awards); err != nil {
		return nil, err
	}
	for _, award := range awards {
		if award.PlayerID != id || award.GuildID != row.GuildID || award.Rank != row.Rank || award.Role != row.Role {
			continue
		}
		for _, rule := range award.Titles {
			if rule.ID == title && rule.Effects == row.Effects && row.ExpiresAt == row.GrantedAt+rule.Hours*3600000 {
				return &row, nil
			}
		}
	}
	return nil, errors.New("月赛称号与截止实际身份不一致")
}
