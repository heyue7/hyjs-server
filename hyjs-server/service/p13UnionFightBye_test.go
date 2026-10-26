package service

import (
	"bytes"
	"encoding/json"
	"fmt"
	"google.golang.org/protobuf/proto"
	pb "hyjs-server/model/protocol"
	"strings"
	"testing"
	"time"

	"hyjs-server/db"
)

// Existing archives must retain their byte-for-byte digest when no bye exists.
func TestP13UFByeArchiveHashCompatibility(t *testing.T) {
	weekly := db.P13UnionFightWeeklyResult{WeekStart: "2026-10-19", Group: "A", OfferHash: "offer", Rankings: []byte("[]"), Awards: []byte("[]")}
	old, _ := json.Marshal(struct {
		Week, Group, Offer string
		Rankings, Awards   []byte
	}{weekly.WeekStart, weekly.Group, weekly.OfferHash, weekly.Rankings, weekly.Awards})
	if p13UFWeeklyResultHash(&weekly) != p6Digest(old) {
		t.Fatal("legacy weekly digest changed")
	}
	before := p13UFWeeklyResultHash(&weekly)
	weekly.ByeDirectory = []byte(`[{"id":"bye","hash":"source"}]`)
	if p13UFWeeklyResultHash(&weekly) == before {
		t.Fatal("weekly digest omitted bye source")
	}
	monthly := db.P13UnionFightMonthlyResult{Month: "2026-10", OfferHash: "offer", CutoffHash: "cutoff", Rankings: []byte("[]"), Awards: []byte("[]"), SettledAt: 123, Source: "source"}
	old, _ = json.Marshal(struct {
		Month, Offer, Cutoff string
		Ranks, Awards        []byte
		Settled              int64
		Source               string
	}{monthly.Month, monthly.OfferHash, monthly.CutoffHash, monthly.Rankings, monthly.Awards, monthly.SettledAt, monthly.Source})
	if p13UFMonthlyResultHash(&monthly) != p6Digest(old) {
		t.Fatal("legacy monthly digest changed")
	}
	before = p13UFMonthlyResultHash(&monthly)
	monthly.ByeDirectory = weekly.ByeDirectory
	if p13UFMonthlyResultHash(&monthly) == before {
		t.Fatal("monthly digest omitted bye source")
	}
}

func TestP13UFByeDirectorySealsEveryFactIncludingZeroScore(t *testing.T) {
	rows := []db.P13UnionFightBye{{ID: "z", SourceHash: "second", WinPrestige: 0}, {ID: "a", SourceHash: "first", WinPrestige: 0}}
	if string(p13UFByeDirectory(rows)) != `[{"id":"a","hash":"first"},{"id":"z","hash":"second"}]` {
		t.Fatal("bye identities/source digests missing or unsorted")
	}
	if p13UFByeDirectory(nil) != nil {
		t.Fatal("legacy directory must remain nil")
	}
	rows[1].SourceHash = "tampered"
	if string(p13UFByeDirectory(rows)) == `[{"id":"a","hash":"first"},{"id":"z","hash":"second"}]` {
		t.Fatal("source mutation omitted")
	}
}

func TestP13UFByeIdentityBindsRoundGuildAndDay(t *testing.T) {
	base := p13UFByeID("2026-10-26", "guild", 1)
	for _, other := range []string{p13UFByeID("2026-10-26", "guild", 2), p13UFByeID("2026-10-26", "other", 1), p13UFByeID("2026-10-27", "guild", 1)} {
		if base == other {
			t.Fatal("bye identity collision")
		}
	}
}

func TestP13UFByePolicyUsesPublishedPrestigeIncludingZero(t *testing.T) {
	rule := db.P13UnionFightSettlementPolicy{Enabled: true, Version: "v1", Source: "published", PublishedAt: 100, ParticipationRewards: "[]", VictoryRewards: "[]"}
	if !p13UFByePolicyValid(&rule, 200, 300) {
		t.Fatal("legally published zero prestige rejected")
	}
	for _, value := range []int32{0, 3, 27} {
		rule.WinPrestige = value
		if !p13UFByePolicyValid(&rule, 200, 300) {
			t.Fatalf("published prestige %d rejected", value)
		}
	}
	rule.PublishedAt = 201
	if p13UFByePolicyValid(&rule, 200, 300) {
		t.Fatal("future publication accepted")
	}
	rule.PublishedAt = 100
	if p13UFByePolicyValid(&rule, 300, 300) {
		t.Fatal("match at start accepted")
	}
}

// Enrollment, frozen rosters, grouping, matching, reward queues and cold reads
// all use their real services. No Match, Bye, terminal or score is inserted.
func TestP13UFByeRealMatcherRewardsColdArchive(t *testing.T) {
	for _, tc := range []struct {
		name     string
		count    int
		prestige int32
	}{{"bye-only-10", 1, 10}, {"bye-only-zero", 1, 0}, {"paired-and-odd", 3, 10}} {
		t.Run(tc.name, func(t *testing.T) {
			f, ids := p13GuildSetup(t)
			p13GuildOpen(t, true)
			p13UFOpen(t)
			if err := f.s.LoadP5Tables("../../analysis/tables"); err != nil {
				t.Fatal(err)
			}
			for _, table := range []any{&db.P13UnionFightBye{}, &db.P13UnionFightSchedule{}, &db.P13UnionFightEntry{}, &db.P13UnionFightFighter{}, &db.P13UnionFightGroup{}, &db.P13UnionFightMatch{}, &db.P13UnionFightFieldPlayer{}, &db.P13UnionFightSettlementOffer{}, &db.P13UnionFightSettlementResult{}, &db.P13UnionFightSettlementPrestige{}, &db.P13UnionFightWeeklyPolicy{}, &db.P13UnionFightWeeklyOffer{}, &db.P13UnionFightWeeklyResult{}, &db.P13UnionFightMonthlyPolicy{}, &db.P13UnionFightMonthlyOffer{}, &db.P13UnionFightMonthlyCutoff{}, &db.P13UnionFightMonthlyResult{}, &db.P13UnionFightMonthlyTitle{}} {
				if err := db.GetIns().Where("1=1").Delete(table).Error; err != nil {
					t.Fatal(err)
				}
			}
			if err := db.GetIns().Model(&db.P13UnionFightPolicy{}).Where("id=1").Updates(map[string]any{"custom_calendar_enabled": true, "match_enabled": true}).Error; err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() {
				db.GetIns().Model(&db.P13UnionFightPolicy{}).Where("id=1").Update("custom_calendar_enabled", false)
				db.GetIns().Model(&db.P13UnionFightSettlementPolicy{}).Where("1=1").Update("enabled", false)
			})
			publish := p13CalendarAt(t, "2026-10-31 12:00")
			monthRule := db.P13UnionFightMonthlyPolicy{Month: "2026-11", Enabled: true, Version: "genuine-bye-month-v1", Source: "isolated explicitly published bye reward fixture", PublishedAt: publish.UnixMilli() - 1, Rewards: `[{"rank":1,"rewards":"item_311,4"}]`, MemberTitle: true, ChairmanTitle: true}
			if err := db.GetIns().Create(&monthRule).Error; err != nil {
				t.Fatal(err)
			}
			if err := f.s.TickP13UnionFightMonthly(f.ctx, publish); err != nil {
				t.Fatal(err)
			}
			at := p13CalendarAt(t, "2026-11-02 12:00")
			schedule := db.P13UnionFightSchedule{Day: "2026-11-03", RegistrationDay: "2026-11-02", Enabled: true, Version: "genuine-bye-match-v1", PublishedAt: publish.UnixMilli(), EnrollmentStartAt: at.UnixMilli(), EnrollmentEndAt: p13CalendarAt(t, "2026-11-02 19:00").UnixMilli(), FirstStartAt: p13CalendarAt(t, "2026-11-03 20:00").UnixMilli(), SecondStartAt: p13CalendarAt(t, "2026-11-03 21:00").UnixMilli(), DurationMillis: 1800000, PreparationMillis: 300000, Source: "explicit isolated real matcher bye fixture"}
			if err := db.GetIns().Create(&schedule).Error; err != nil {
				t.Fatal(err)
			}
			if err := db.GetIns().Model(&db.P13UnionFightSettlementPolicy{}).Where("`group`=?", "A").Updates(map[string]any{"enabled": true, "version": "genuine-bye-settlement-v1", "source": "explicit isolated win prestige publication", "published_at": publish.UnixMilli(), "win_prestige": tc.prestige, "lose_prestige": 0, "factory_mission_prestige": 0, "participation_rewards": "item_311,1", "victory_rewards": "item_311,2", "personal_missions": "", "task_rewards": "", "transport_mvp_rewards": "", "terminal_rule": "", "draw_prestige": 0, "abort_rule": "", "finalization_grace_millis": 0}).Error; err != nil {
				t.Fatal(err)
			}
			weeklyPolicy := db.P13UnionFightWeeklyPolicy{WeekStart: "2026-11-02", Group: "A", Enabled: true, Version: "genuine-bye-week-v1", Source: "explicit isolated bye weekly award", PublishedAt: publish.UnixMilli(), Rewards: `[{"rank":1,"rewards":"item_311,8"}]`}
			if err := db.GetIns().Create(&weeklyPolicy).Error; err != nil {
				t.Fatal(err)
			}
			guildOwners := map[string]int64{}
			for i, id := range ids[:tc.count] {
				p13GuildCall(t, f, id, &pb.CSUnionCreate{Name: proto.String(fmt.Sprintf("真实轮空%d", i))}, 1, at)
				guild := p13GuildID(t, f, id, at)
				guildOwners[guild] = id
				if err := db.GetIns().Model(&db.P13Guild{}).Where("id=?", guild).Update("level", 2).Error; err != nil {
					t.Fatal(err)
				}
				p13UFCall(t, f, id, &pb.CSUFUnionApply{}, 2, at)
				p13UFCall(t, f, id, &pb.CSUFPlayerApply{}, 3, at.Add(time.Second))
				// A retained weekly group is a finite grouping prerequisite. The matcher
				// still owns candidate sorting, both real pair rounds and the odd tail.
				if err := db.GetIns().Create(&db.P13UnionFightGroup{WeekStart: "2026-11-02", GuildID: guild, Group: "A", AssignedAt: at.UnixMilli(), Source: "explicit preassigned weekly group fixture"}).Error; err != nil {
					t.Fatal(err)
				}
			}
			matchedAt := time.UnixMilli(schedule.EnrollmentEndAt)
			if err := f.s.TickP13UnionFight(f.ctx, matchedAt); err != nil {
				t.Fatal(err)
			}
			if err := f.s.MatchP13UnionFight(f.ctx, matchedAt); err != nil {
				t.Fatal(err)
			}
			var byes []db.P13UnionFightBye
			if err := db.GetIns().Order("round").Find(&byes).Error; err != nil || len(byes) != 2 {
				t.Fatal("matcher must make exactly two bye facts", len(byes), err)
			}
			expectedMatches := int64(0)
			if tc.count == 3 {
				expectedMatches = 2
			}
			if got := p1Count(t, &db.P13UnionFightMatch{}, "1=1"); got != expectedMatches {
				t.Fatal("fictive or missing real pair match", got, expectedMatches)
			}
			for _, bye := range byes {
				if bye.WinPrestige != tc.prestige || bye.GuildID != byes[0].GuildID {
					t.Fatal("wrong frozen policy/tail", bye)
				}
			}
			byeGuild := byes[0].GuildID
			owner := guildOwners[byeGuild]
			previews, _, err := f.s.p13UFMatchPreviews(db.GetIns(), byeGuild, schedule.Day, owner)
			if err != nil || len(previews) != 2 {
				t.Fatal("real bye preview", previews, err)
			}
			for _, preview := range previews {
				if preview.GetIsRoundBye() != 1 || preview.GetFightResult() != 1 || preview.GetEnemyUnion() != nil {
					t.Fatal("original bye preview shape", preview)
				}
			}
			if err := db.GetIns().Model(&db.P13UnionFightFieldPolicy{}).Where("id=1").Update("enabled", true).Error; err != nil {
				t.Fatal(err)
			}
			if _, err := f.s.p13UFFieldEnter(f.ctx, owner, time.UnixMilli(schedule.FirstStartAt)); err == nil || !strings.Contains(err.Error(), "真实对局") {
				t.Fatal("bye entry did not reject absent real match", err)
			}
			for _, table := range []any{&db.P13UnionFightFieldPlayer{}, &db.P13UnionFightSettlementResult{}} {
				if got := p1Count(t, table, "1=1"); got != 0 {
					t.Fatal("bye manufactured scene or terminal", got)
				}
			}
			var ledger db.P13UnionFightSettlementPrestige
			if err := db.GetIns().First(&ledger, "week_start=? AND guild_id=?", "2026-11-02", byeGuild).Error; err != nil || ledger.Score != int64(tc.prestige)*2 {
				t.Fatal("real frozen bye prestige ledger", ledger, err)
			}
			sunday := p13CalendarAt(t, "2026-11-08 12:00")
			if err := f.s.TickP13UnionFightWeekly(f.ctx, sunday); err != nil {
				t.Fatal(err)
			}
			if tc.count == 3 {
				// The real pair has not fought. Its missing terminal must keep the whole
				// group pending even though the true odd tail is fully covered.
				if p1Count(t, &db.P13UnionFightWeeklyResult{}, "week_start=? AND `group`=?", "2026-11-02", "A") != 0 {
					t.Fatal("unfinished real pair certified weekly rewards")
				}
				var offer db.P13UnionFightMonthlyOffer
				if err := db.GetIns().First(&offer, "month=?", "2026-11").Error; err != nil {
					t.Fatal(err)
				}
				if _, complete, err := p13UFMonthlyRanks(db.GetIns(), &offer); err != nil || complete {
					t.Fatal("unfinished pair monthly coverage", complete, err)
				}
				return
			}
			var weekResult db.P13UnionFightWeeklyResult
			if err := db.GetIns().First(&weekResult, "week_start=? AND `group`=?", "2026-11-02", "A").Error; err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(weekResult.ByeDirectory, p13UFByeDirectory(byes)) {
				t.Fatal("weekly archive omitted exact bye directory")
			}
			expectedAwards := int64(0)
			if tc.prestige > 0 {
				expectedAwards = 1
			}
			if got := p1Count(t, &db.RewardDelivery{}, "source_key=? AND player_id=?", "p13-week:2026-11-02:A", owner); got != expectedAwards {
				t.Fatal("weekly actual recipient count", got)
			}
			cutoff := p13CalendarAt(t, "2026-12-01 00:00")
			if err := f.s.TickP13UnionFightMonthly(f.ctx, cutoff); err != nil {
				t.Fatal(err)
			}
			var monthResult db.P13UnionFightMonthlyResult
			if err := db.GetIns().First(&monthResult, "month=?", "2026-11").Error; err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(monthResult.ByeDirectory, p13UFByeDirectory(byes)) {
				t.Fatal("monthly archive omitted exact bye directory")
			}
			if got := p1Count(t, &db.RewardDelivery{}, "source_key=? AND player_id=?", "p13-month:2026-11", owner); got != expectedAwards {
				t.Fatal("monthly actual recipient count", got)
			}
			if got := p1Count(t, &db.P13UnionFightMonthlyTitle{}, "month=?", "2026-11"); got != expectedAwards*2 {
				t.Fatal("monthly actual chairman/member titles", got)
			}
			// A new service instance and reopened connection must consume stored facts.
			sql, _ := db.GetIns().DB()
			if err := sql.Close(); err != nil {
				t.Fatal(err)
			}
			f.open(t)
			cold := &GameService{}
			if err := cold.LoadTables("../../analysis/tables"); err != nil {
				t.Fatal(err)
			}
			if err := cold.LoadP13GuildTables("../../analysis/tables"); err != nil {
				t.Fatal(err)
			}
			if err := cold.LoadP5Tables("../../analysis/tables"); err != nil {
				t.Fatal(err)
			}
			for i := 0; i < 2; i++ {
				if err := cold.TickP13UnionFightWeekly(f.ctx, cutoff); err != nil {
					t.Fatal(err)
				}
				if err := cold.TickP13UnionFightMonthly(f.ctx, cutoff); err != nil {
					t.Fatal(err)
				}
			}
			var weekAfter db.P13UnionFightWeeklyResult
			var monthAfter db.P13UnionFightMonthlyResult
			if err := db.GetIns().First(&weekAfter, "week_start=? AND `group`=?", "2026-11-02", "A").Error; err != nil {
				t.Fatal(err)
			}
			if err := db.GetIns().First(&monthAfter, "month=?", "2026-11").Error; err != nil {
				t.Fatal(err)
			}
			if weekAfter.ResultHash != weekResult.ResultHash || monthAfter.ResultHash != monthResult.ResultHash || !bytes.Equal(weekAfter.ByeDirectory, weekResult.ByeDirectory) || !bytes.Equal(monthAfter.ByeDirectory, monthResult.ByeDirectory) {
				t.Fatal("cold retry rewrote frozen archive")
			}
			for _, source := range []string{"p13-week:2026-11-02:A", "p13-month:2026-11"} {
				if got := p1Count(t, &db.RewardDelivery{}, "source_key=? AND player_id=?", source, owner); got != expectedAwards {
					t.Fatal("cold duplicate reward", source, got)
				}
			}
			if tc.prestige > 0 {
				messages, err := cold.RewardCenter(f.ctx, owner, &pb.CSQueryUserReward{})
				if err != nil {
					t.Fatal(err)
				}
				var rewardIDs []int64
				for _, item := range messages[0].(*pb.SCQueryUserReward).GetRewardItemInfoList() {
					rewardIDs = append(rewardIDs, item.GetRewardId())
				}
				before := p13WeeklyBadgeCount(t, owner)
				for i := 0; i < 2; i++ {
					if _, err := cold.RewardCenter(f.ctx, owner, &pb.CSAddRewardsToPlayer{RewardIds: rewardIDs}); err != nil {
						t.Fatal(err)
					}
				}
				if p13WeeklyBadgeCount(t, owner)-before != 12 {
					t.Fatal("bye weekly/monthly rewards missing or duplicated")
				}
			}
			if err := db.GetIns().Model(&db.P13UnionFightBye{}).Where("id=?", byes[0].ID).Update("win_prestige", tc.prestige+1).Error; err != nil {
				t.Fatal(err)
			}
			if err := cold.TickP13UnionFightWeekly(f.ctx, cutoff); err == nil {
				t.Fatal("weekly cold archive accepted tampered bye")
			}
			if err := cold.TickP13UnionFightMonthly(f.ctx, cutoff); err == nil {
				t.Fatal("monthly cold archive accepted tampered bye")
			}
		})
	}
}
