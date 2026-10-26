package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
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

type p13UFWeeklyReward struct {
	Rank    int32  `json:"rank"`
	Rewards string `json:"rewards"`
}
type p13UFWeeklyRank struct {
	GuildID string `json:"guildId"`
	Score   int64  `json:"score"`
	Rank    int32  `json:"rank"`
	Preview []byte `json:"preview"`
}
type p13UFWeeklyAward struct {
	PlayerID int64  `json:"playerId"`
	GuildID  string `json:"guildId"`
	Rank     int32  `json:"rank"`
	Rewards  string `json:"rewards"`
}

func (s *GameService) InitializeP13UnionFightWeekly(ctx context.Context) error {
	// No current private prize values are seeded; an operator publishes an explicit future group.
	return db.GetIns().WithContext(ctx).AutoMigrate(&db.P13UnionFightWeeklyPolicy{}, &db.P13UnionFightWeeklyOffer{}, &db.P13UnionFightWeeklyResult{})
}
func p13UFWeeklyStart(week string) (time.Time, error) {
	at, err := time.ParseInLocation("2006-01-02", week, time.FixedZone("Asia/Shanghai", 8*3600))
	if err != nil || at.Weekday() != time.Monday {
		return time.Time{}, errors.New("公会战周奖周一日期无效")
	}
	return at, nil
}
func p13UFWeeklyRules(offer *db.P13UnionFightWeeklyOffer) (*db.P13UnionFightWeeklyPolicy, []p13UFWeeklyReward, error) {
	var policy db.P13UnionFightWeeklyPolicy
	if (offer.FirstMatchID == "") == (offer.FirstByeID == "") {
		return nil, nil, p13UFByeError()
	}
	if p6Digest(offer.Rules) != offer.RulesHash || json.Unmarshal(offer.Rules, &policy) != nil || policy.WeekStart != offer.WeekStart || policy.Group != offer.Group || !policy.Enabled || policy.Version == "" || policy.Source == "" || policy.PublishedAt <= 0 || policy.PublishedAt > offer.FrozenAt {
		return nil, nil, errors.New("公会战周奖冻结来源无效")
	}
	if _, err := p13UFWeeklyStart(policy.WeekStart); err != nil {
		return nil, nil, err
	}
	var defs []p13UFWeeklyReward
	if json.Unmarshal([]byte(policy.Rewards), &defs) != nil || len(defs) == 0 || len(defs) > 20 {
		return nil, nil, errors.New("公会战本地周奖目录无效")
	}
	seen := map[int32]bool{}
	for _, def := range defs {
		if def.Rank < 1 || def.Rank > 20 || seen[def.Rank] || def.Rewards == "" {
			return nil, nil, errors.New("公会战本地周奖名次/资源无效")
		}
		seen[def.Rank] = true
	}
	return &policy, defs, nil
}
func (s *GameService) p13UFWeeklyFreeze(tx *gorm.DB, match *db.P13UnionFightMatch, now time.Time) error {
	day, err := time.ParseInLocation("2006-01-02", match.Day, time.FixedZone("Asia/Shanghai", 8*3600))
	if err != nil {
		return err
	}
	week := p13UFWeek(day)
	weekStart, err := p13UFWeeklyStart(week)
	if err != nil {
		return err
	}
	var existing db.P13UnionFightWeeklyOffer
	err = tx.First(&existing, "week_start=? AND `group`=?", week, match.Group).Error
	if err == nil {
		return nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}
	// Once any real group match starts, later publication cannot retroactively create a week prize.
	var first db.P13UnionFightMatch
	if err = tx.Where("day>=? AND day<? AND `group`=?", week, weekStart.AddDate(0, 0, 7).Format("2006-01-02"), match.Group).Order("start_at,id").First(&first).Error; err != nil {
		return err
	}
	if now.UnixMilli() >= first.StartAt {
		return nil
	}
	var policy db.P13UnionFightWeeklyPolicy
	err = tx.First(&policy, "week_start=? AND `group`=?", week, match.Group).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	if !policy.Enabled {
		return nil
	}
	raw, err := json.Marshal(policy)
	if err != nil {
		return err
	}
	offer := db.P13UnionFightWeeklyOffer{WeekStart: week, Group: match.Group, Rules: raw, RulesHash: p6Digest(raw), FrozenAt: now.UnixMilli(), FirstMatchID: first.ID}
	_, defs, err := p13UFWeeklyRules(&offer)
	if err != nil {
		return err
	}
	original, err := s.p13UFFieldOriginal(match, tx)
	if err != nil {
		return err
	}
	for _, def := range defs {
		ok, err := original.openServerRewardSupported(def.Rewards)
		if err != nil {
			return err
		}
		if !ok {
			return errors.New("本地公会战周奖资源尚未恢复")
		}
	}
	return tx.Create(&offer).Error
}

// p13UFWeeklyRanks projects real settled prestige and original frozen guild names.
// Equal numerical prestige shares a competition rank under this named local policy.
func p13UFWeeklyRanks(tx *gorm.DB, week, group string) ([]p13UFWeeklyRank, error) {
	start, err := p13UFWeeklyStart(week)
	if err != nil {
		return nil, err
	}
	var groups []db.P13UnionFightGroup
	if err = tx.Where("week_start=? AND `group`=?", week, group).Find(&groups).Error; err != nil {
		return nil, err
	}
	ranks := []p13UFWeeklyRank{}
	for _, g := range groups {
		var ledger db.P13UnionFightSettlementPrestige
		err = tx.First(&ledger, "week_start=? AND guild_id=?", week, g.GuildID).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			continue
		}
		if err != nil {
			return nil, err
		}
		if ledger.Score < 0 || ledger.Score > math.MaxInt32 {
			return nil, errors.New("真实公会威望超出原整数范围")
		}
		if ledger.Score == 0 {
			continue
		}
		var match db.P13UnionFightMatch
		err = tx.Where("day>=? AND day<? AND `group`=? AND (left_guild_id=? OR right_guild_id=?)", week, start.AddDate(0, 0, 7).Format("2006-01-02"), group, g.GuildID, g.GuildID).Order("start_at desc,id").First(&match).Error
		if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, err
		}
		raw := match.LeftPreview
		if errors.Is(err, gorm.ErrRecordNotFound) {
			byes, e := p13UFByes(tx, week, start.AddDate(0, 0, 7).Format("2006-01-02"), group)
			if e != nil {
				return nil, e
			}
			for i := range byes {
				if byes[i].GuildID == g.GuildID {
					b, e := p13UFByeRead(&byes[i])
					if e != nil {
						return nil, e
					}
					raw = b.Preview
				}
			}
			if len(raw) == 0 {
				return nil, p13UFByeError()
			}
		}
		if match.RightGuildID == g.GuildID {
			raw = match.RightPreview
		}
		var preview pb.UFInfoPreView
		if proto.Unmarshal(raw, &preview) != nil || preview.GetUnionId() != g.GuildID || preview.GetServerId() <= 0 || preview.GetUnionName() == "" {
			return nil, errors.New("公会战周榜冻结身份无效")
		}
		ranks = append(ranks, p13UFWeeklyRank{GuildID: g.GuildID, Score: ledger.Score, Preview: raw})
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
	return ranks, nil
}
func (s *GameService) p13UFWeeklyQuery(ctx context.Context, id int64, b *pb.CSUFRankInfo, now time.Time) ([]proto.Message, error) {
	if b.GetRankType() != 1 || b.GetSubType() < 1 || b.GetSubType() > 4 {
		return nil, errors.New("该原公会战排行类型尚未恢复")
	}
	var frame *pb.SCUFRankInfo
	err := db.GetIns().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := ServerGameClockGuardTransaction(tx); err != nil {
			return err
		}

		if err := p13UFNeedEnabled(tx); err != nil {
			return err
		}
		start, _, _, _ := p13UFClock(now)
		week := p13UFWeek(start)
		group := string(rune('A' + b.GetSubType() - 1))
		ranks, err := p13UFWeeklyRanks(tx, week, group)
		if err != nil {
			return err
		}
		info := &pb.RankInfo{Type: proto.Int32(1)}
		for _, rank := range ranks {
			if rank.Rank > 20 {
				break
			}
			var preview pb.UFInfoPreView
			if err = proto.Unmarshal(rank.Preview, &preview); err != nil {
				return err
			}
			info.List = append(info.List, &pb.RankRowInfo{Rank: proto.Int32(rank.Rank), Name: preview.MasterName, UnionName: preview.UnionName, Values: []int32{int32(rank.Score), preview.GetServerId()}})
		}
		frame = &pb.SCUFRankInfo{RankType: b.RankType, SubType: b.SubType, RankIno: info}
		return nil
	})
	return []proto.Message{frame}, err
}
func p13UFWeeklyResultHash(result *db.P13UnionFightWeeklyResult) string {
	raw, _ := json.Marshal(struct {
		Week, Group, Offer string
		Rankings, Awards   []byte
		ByeDirectory       []byte `json:",omitempty"`
	}{result.WeekStart, result.Group, result.OfferHash, result.Rankings, result.Awards, result.ByeDirectory})
	return p6Digest(raw)
}
func (s *GameService) p13UFWeeklySettle(tx *gorm.DB, offer *db.P13UnionFightWeeklyOffer, now time.Time) (*db.P13UnionFightWeeklyResult, error) {
	start, err := p13UFWeeklyStart(offer.WeekStart)
	if err != nil {
		return nil, err
	}
	if now.Before(start.AddDate(0, 0, 6)) {
		return nil, nil
	}
	var saved db.P13UnionFightWeeklyResult
	err = tx.First(&saved, "week_start=? AND `group`=?", offer.WeekStart, offer.Group).Error
	if err == nil {
		if saved.ResultHash != p13UFWeeklyResultHash(&saved) {
			return nil, errors.New("公会战周奖结算归档无效")
		}
		if err = p13UFByeDirectoryVerify(tx, saved.ByeDirectory, offer.WeekStart, start.AddDate(0, 0, 7).Format("2006-01-02"), offer.Group); err != nil {
			return nil, err
		}
		return &saved, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}
	if err = p13UFByeWeeklyAnchor(tx, offer); err != nil {
		return nil, err
	}
	policy, defs, err := p13UFWeeklyRules(offer)
	if err != nil {
		return nil, err
	}
	var matches []db.P13UnionFightMatch
	if err = tx.Where("day>=? AND day<? AND `group`=?", offer.WeekStart, start.AddDate(0, 0, 7).Format("2006-01-02"), offer.Group).Order("start_at,id").Find(&matches).Error; err != nil {
		return nil, err
	}
	byes, err := p13UFByes(tx, offer.WeekStart, start.AddDate(0, 0, 7).Format("2006-01-02"), offer.Group)
	if err != nil {
		return nil, err
	}
	if len(matches) == 0 && len(byes) == 0 {
		return nil, nil
	}
	// Every actually accepted eligible registration needs both original rounds.
	// Odd unmatched guilds, missing calendars and unfinished freezing remain unresolved.
	coverage, err := p13UFCalendarCoverage(tx, start, start.AddDate(0, 0, 7), matches)
	if err != nil {
		return nil, err
	}
	groupByGuild := map[string]string{}
	lastGuild := ""
	for {
		if tx.Statement.Context.Err() != nil {
			return nil, tx.Statement.Context.Err()
		}
		var groups []db.P13UnionFightGroup
		query := tx.Select("guild_id,`group`").Where("week_start=?", offer.WeekStart)
		if lastGuild != "" {
			query = query.Where("guild_id>?", lastGuild)
		}
		if err = query.Order("guild_id").Limit(128).Find(&groups).Error; err != nil {
			return nil, err
		}
		for _, row := range groups {
			groupByGuild[row.GuildID] = row.Group
		}
		if len(groups) < 128 {
			break
		}
		lastGuild = groups[len(groups)-1].GuildID
	}
	for _, row := range coverage {
		entry := row.Entry
		group, assigned := groupByGuild[entry.GuildID]
		if assigned && group != offer.Group {
			continue
		}
		if entry.FrozenAt <= 0 {
			return nil, nil
		}
		if entry.EligibleCount == 0 {
			continue
		}
		if !assigned || !entry.MatchProcessed {
			return nil, nil
		}
		rounds := map[int32]bool{}
		for _, match := range matches {
			if match.Day == row.MatchDay && (match.LeftGuildID == entry.GuildID || match.RightGuildID == entry.GuildID) {
				if rounds[match.Round] || match.Round < 1 || match.Round > 2 {
					return nil, p13UFByeError()
				}
				rounds[match.Round] = true
			}
		}
		for _, bye := range byes {
			if bye.Day == row.MatchDay && bye.GuildID == entry.GuildID {
				if rounds[bye.Round] {
					return nil, p13UFByeError()
				}
				rounds[bye.Round] = true
			}
		}
		if !rounds[1] || !rounds[2] {
			return nil, nil
		}
	}
	scores := map[string]int64{}
	participants := map[string]map[int64]bool{}
	for _, match := range matches {
		if now.UnixMilli() < match.EndAt {
			return nil, nil
		}
		var result db.P13UnionFightSettlementResult
		err = tx.First(&result, "match_id=?", match.ID).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		if err != nil {
			return nil, err
		}
		var roundOffer db.P13UnionFightSettlementOffer
		if err = tx.First(&roundOffer, "match_id=?", match.ID).Error; err != nil {
			return nil, err
		}
		if result.ResultKind == constUtil.P13UFTechnicalAbortKind {
			if err = s.p13UFCombatSettlementVerify(tx, &match, &result); err != nil {
				return nil, err
			}
			continue
		}
		rule, err := p13UFSettlementRules(&roundOffer, &match)
		if err != nil {
			return nil, err
		}
		if result.OfferHash != roundOffer.RulesHash || !p13UFMixedRoundIdentity(&match, &result, rule) {
			return nil, errors.New("周奖缺真实完整场次终局")
		}
		if err = s.p13UFCombatSettlementVerify(tx, &match, &result); err != nil {
			return nil, err
		}
		if result.ResultKind == constUtil.P13UFTechnicalAbortKind {
			continue
		}
		for _, guild := range []string{match.LeftGuildID, match.RightGuildID} {
			extra, err := p13UFGuildMissionPrestige(tx, &match, guild)
			if err != nil {
				return nil, err
			}
			base, snapshotExtra, prestigeErr := p13UFMixedRoundPrestige(&match, &result, rule, guild)
			if prestigeErr != nil {
				return nil, prestigeErr
			}
			if snapshotExtra != extra {
				return nil, errors.New("周奖工厂威望与真实三态终局不一致")
			}
			scores[guild] += base + int64(extra)
		}
		var actual []p13UFSettlementAward
		if err = json.Unmarshal(result.Awards, &actual); err != nil {
			return nil, err
		}
		for _, actor := range actual {
			if actor.GuildID != match.LeftGuildID && actor.GuildID != match.RightGuildID {
				return nil, errors.New("公会战周奖参与身份无效")
			}
			if participants[actor.GuildID] == nil {
				participants[actor.GuildID] = map[int64]bool{}
			}
			participants[actor.GuildID][actor.PlayerID] = true
		}
	}
	currentByeMembers, err := p13UFByeCurrentMembers(tx, byes)
	if err != nil {
		return nil, err
	}
	for i := range byes {
		bye := &byes[i]
		if now.UnixMilli() < bye.EndAt {
			return nil, nil
		}
		b, e := p13UFByeRead(bye)
		if e != nil {
			return nil, e
		}
		scores[bye.GuildID] += int64(bye.WinPrestige)
		if participants[bye.GuildID] == nil {
			participants[bye.GuildID] = map[int64]bool{}
		}
		for _, f := range b.Roster {
			if currentByeMembers[f.PlayerID] == bye.GuildID {
				participants[bye.GuildID][f.PlayerID] = true
			}
		}
	}
	ranks, err := p13UFWeeklyRanks(tx, offer.WeekStart, offer.Group)
	if err != nil {
		return nil, err
	}
	// Earliest real entered guild anchors this local weekly recipient rule across guild changes.
	var entered []struct{ db.P13UnionFightFieldPlayer }
	if err = tx.Table("p13_union_fight_field_players AS a").Select("a.*").Joins("JOIN p13_union_fight_matches AS m ON m.id=a.match_id").Where("m.day>=? AND m.day<?", offer.WeekStart, start.AddDate(0, 0, 7).Format("2006-01-02")).Order("a.joined_at,a.match_id").Scan(&entered).Error; err != nil {
		return nil, err
	}
	firstGuild := map[int64]string{}
	for _, actor := range entered {
		if firstGuild[actor.PlayerID] == "" {
			firstGuild[actor.PlayerID] = actor.GuildID
		}
	}
	for i := range byes {
		b, e := p13UFByeRead(&byes[i])
		if e != nil {
			return nil, e
		}
		for _, f := range b.Roster {
			if firstGuild[f.PlayerID] == "" && currentByeMembers[f.PlayerID] == byes[i].GuildID {
				firstGuild[f.PlayerID] = byes[i].GuildID
			}
		}
	}
	byRank := map[int32]string{}
	for _, def := range defs {
		byRank[def.Rank] = def.Rewards
	}
	awards := []p13UFWeeklyAward{}
	for _, rank := range ranks {
		if rank.Score != scores[rank.GuildID] {
			return nil, errors.New("周榜威望与实际终局总和不一致")
		}
		if rewards := byRank[rank.Rank]; rewards != "" {
			for id := range participants[rank.GuildID] {
				if firstGuild[id] == rank.GuildID {
					awards = append(awards, p13UFWeeklyAward{PlayerID: id, GuildID: rank.GuildID, Rank: rank.Rank, Rewards: rewards})
				}
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
	saved = db.P13UnionFightWeeklyResult{ByeDirectory: p13UFByeDirectory(byes), WeekStart: offer.WeekStart, Group: offer.Group, OfferHash: offer.RulesHash, Rankings: rawRanks, Awards: rawAwards, SettledAt: now.UnixMilli(), Source: constUtil.P13UnionFightWeeklySource + "; " + policy.Version + "; " + policy.Source}
	saved.ResultHash = p13UFWeeklyResultHash(&saved)
	if err = tx.Create(&saved).Error; err != nil {
		return nil, err
	}
	return &saved, nil
}
func (s *GameService) TickP13UnionFightWeekly(ctx context.Context, now time.Time) error {
	var offers []db.P13UnionFightWeeklyOffer
	if err := db.GetIns().WithContext(ctx).Order("week_start,`group`").Find(&offers).Error; err != nil {
		return err
	}
	for _, offer := range offers {
		if err := db.GetIns().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			if err := ServerGameClockGuardTransaction(tx); err != nil {
				return err
			}

			var lock db.P13GuildPolicy
			if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&lock, 1).Error; err != nil {
				return err
			}
			result, err := s.p13UFWeeklySettle(tx, &offer, now)
			if err != nil || result == nil {
				return err
			}
			if result.DeliveredAt > 0 {
				return nil
			}
			var awards []p13UFWeeklyAward
			if err = json.Unmarshal(result.Awards, &awards); err != nil {
				return err
			}
			for _, award := range awards {
				if _, err = s.EnqueueReward(tx, award.PlayerID, fmt.Sprintf("p13-week:%s:%s", offer.WeekStart, offer.Group), "公会战周奖励", "本周公会战排名奖励，按本周已发布的奖档发放。", award.Rewards, 0); err != nil {
					return err
				}
			}
			return tx.Model(result).Update("delivered_at", now.UnixMilli()).Error
		}); err != nil {
			return err
		}
	}
	return nil
}
