package service

import (
	"encoding/json"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	pb "hyjs-server/model/protocol"
)

func TestP13SettlementParticipantExitTime(t *testing.T) {
	for _, test := range []struct {
		joined, left, cutoff int64
		valid                bool
	}{
		{10, 0, 100, true}, {10, 10, 100, true}, {10, 99, 100, true}, {10, 101, 100, true},
		{10, -1, 100, false}, {10, 9, 100, false}, {0, 0, 100, false}, {100, 0, 100, false},
	} {
		actor := &db.P13UnionFightFieldPlayer{JoinedAt: test.joined, LeftAt: test.left}
		if got := p13UFSettlementParticipantTimeValid(actor, test.cutoff); got != test.valid {
			t.Fatal(test, got)
		}
	}
}

func TestP13SettlementPreparationParticipationBoundary(t *testing.T) {
	for _, test := range []struct {
		left     int64
		eligible bool
	}{{0, true}, {99, false}, {100, false}, {101, true}} {
		actor := &db.P13UnionFightFieldPlayer{JoinedAt: 10, LeftAt: test.left}
		if got := p13UFSettlementParticipated(actor, 100); got != test.eligible {
			t.Fatal(test, got)
		}
	}
}

// Every branch uses actual registration, matching, entry and the original town
// exit. Combat branches use the real engine/worker; no HP or result is injected.
func TestP13SettlementRealEarlyLeaveZeroCombatMixedColdAwards(t *testing.T) {
	for _, mode := range []string{"zero", "combat", "mixed"} {
		t.Run(mode, func(t *testing.T) {
			f, id, enemy, clock, match := p13UFPlayerBattle172ActualSetup(t)
			for _, pid := range []int64{id, enemy} {
				var profile db.Profile
				if err := db.GetIns().First(&profile, pid).Error; err != nil {
					t.Fatal(err)
				}
				t.Cleanup(f.s.RegisterP10Client(f.ctx, pid, profile.Session, func(proto.Message) error { return nil }))
				if _, err := f.s.SyncP13UnionFightField(f.ctx, pid, false); err != nil {
					t.Fatal(err)
				}
			}
			terminal := ""
			drawPrestige := int32(0)
			if mode == "mixed" {
				terminal = constUtil.P13UFMixedTerminalRule
				drawPrestige = 5
			}
			published := match.StartAt - 60000
			if err := db.GetIns().Model(&db.P13UnionFightSettlementPolicy{}).Where("`group`=?", match.Group).Updates(map[string]any{
				"enabled": true, "version": "isolated-early-exit-v1", "source": "isolated real entry and engine; item311 finite award fixture only", "published_at": published - 1,
				"participation_rewards": "item_311,1", "victory_rewards": "item_311,2", "win_prestige": 50, "lose_prestige": 25,
				"personal_missions": "", "task_rewards": "", "transport_mvp_rewards": "", "terminal_rule": terminal, "draw_prestige": drawPrestige,
				"abort_rule": "", "finalization_grace_millis": 0,
			}).Error; err != nil {
				t.Fatal(err)
			}
			if err := f.s.p13UFSettlementFreeze(db.GetIns(), match, time.UnixMilli(published)); err != nil {
				t.Fatal(err)
			}
			var offer db.P13UnionFightSettlementOffer
			if err := db.GetIns().First(&offer, "match_id=?", match.ID).Error; err != nil {
				t.Fatal(err)
			}
			leaver := id
			var attempt db.P13UnionFightPlayerBattleAttempt
			if mode != "zero" {
				if _, handled, err := f.s.DispatchP13UnionFight(f.ctx, id, &pb.CSUFPlayerAskFight{TargetGuid: proto.Int64(enemy)}, 9901); err != nil || !handled {
					t.Fatal("real PK", err)
				}
				if err := db.GetIns().First(&attempt, "match_id=?", match.ID).Error; err != nil {
					t.Fatal(err)
				}
				clock.Store(attempt.DueAt)
				if err := f.s.AdvanceP13UnionFightPlayerBattles(f.ctx, time.UnixMilli(clock.Load())); err != nil {
					t.Fatal(err)
				}
				if err := db.GetIns().First(&attempt, "id=?", attempt.ID).Error; err != nil {
					t.Fatal(err)
				}
				if attempt.Status != constUtil.P13UFPlayerBattleSettled || attempt.RewardPlayerID == 0 || attempt.DeadPlayerID == 0 {
					t.Fatal("actual settled death required")
				}
				leaver = attempt.RewardPlayerID
			}
			var original db.P13UnionFightFieldPlayer
			if err := db.GetIns().First(&original, "match_id=? AND player_id=?", match.ID, leaver).Error; err != nil {
				t.Fatal(err)
			}
			leave := func() {
				t.Helper()
				if _, _, err := f.s.p13UFFieldLeaveExpected(f.ctx, leaver, &pb.CSTransferScene{Target: proto.String("town")}, time.UnixMilli(clock.Load()), match.ID); err != nil {
					t.Fatal("original exit", err)
				}
			}
			clock.Add(1000)
			leave()
			leave()
			var exited db.P13UnionFightFieldPlayer
			if err := db.GetIns().First(&exited, "match_id=? AND player_id=?", match.ID, leaver).Error; err != nil {
				t.Fatal(err)
			}
			if exited.JoinedAt != original.JoinedAt || exited.LeftAt != clock.Load() {
				t.Fatal("duplicate exit changed participation", original.JoinedAt, exited.JoinedAt, exited.LeftAt)
			}
			clock.Add(1000)
			if _, err := f.s.p13UFFieldEnter(f.ctx, leaver, time.UnixMilli(clock.Load())); err != nil {
				t.Fatal("real reentry", err)
			}
			var reentered db.P13UnionFightFieldPlayer
			if err := db.GetIns().First(&reentered, "match_id=? AND player_id=?", match.ID, leaver).Error; err != nil {
				t.Fatal(err)
			}
			if reentered.JoinedAt != original.JoinedAt || reentered.LeftAt != 0 {
				t.Fatal("reentry must retain original participation")
			}
			clock.Add(1000)
			leave()
			clock.Store(match.EndAt)
			var result *db.P13UnionFightSettlementResult
			var err error
			if mode == "zero" {
				result, err = f.s.p13UFSettleZeroFight(db.GetIns(), match, &offer, time.UnixMilli(clock.Load()))
			} else {
				result, err = f.s.p13UFSettlePlayerCombat(db.GetIns(), match, &offer, time.UnixMilli(clock.Load()))
			}
			if err != nil || result == nil {
				t.Fatal("legal early exit blocked entire round", err)
			}
			if mode != "zero" && (result.LeftKills+result.RightKills != 1 || result.WinnerGuildID != original.GuildID) {
				t.Fatal("departed winner's real kill omitted", result)
			}
			if mode == "mixed" && !p13UFMixedSchema(result.CombatProof) {
				t.Fatal("expected mixed proof")
			}
			var awards []p13UFSettlementAward
			if err = json.Unmarshal(result.Awards, &awards); err != nil || len(awards) != 2 {
				t.Fatal("both real participants retain published awards", err, len(awards))
			}
			found := false
			for _, award := range awards {
				if award.PlayerID == leaver {
					found = true
				}
			}
			if !found {
				t.Fatal("early leaver lost participation award")
			}
			frozen, marshalErr := json.Marshal(result)
			if marshalErr != nil {
				t.Fatal(marshalErr)
			}
			f.open(t)
			var restored db.P13UnionFightSettlementResult
			if err = db.GetIns().First(&restored, "match_id=?", match.ID).Error; err != nil {
				t.Fatal(err)
			}
			restoredJSON, _ := json.Marshal(&restored)
			if string(restoredJSON) != string(frozen) {
				t.Fatal("cold result changed")
			}
			result = &restored
			if mode != "zero" {
				if err = f.s.p13UFCombatSettlementVerify(db.GetIns(), match, result); err != nil {
					t.Fatal("cold certified history", err)
				}
			}
			if err = f.s.p13UFSettlementDeliver(db.GetIns(), result, time.UnixMilli(result.AwardAt)); err != nil {
				t.Fatal("cold original reward source", err)
			}
			if err = f.s.p13UFSettlementDeliver(db.GetIns(), result, time.UnixMilli(result.AwardAt+1)); err != nil {
				t.Fatal(err)
			}
			if got := p1Count(t, &db.RewardDelivery{}, "source_key=?", "p13-war:"+match.ID); got != 2 {
				t.Fatal("published participant delivery not once", got)
			}

		})
	}
}

// A real preparation visit is not battle participation. A second genuinely
// entered guildmate can still participate; the departed identity is retained.
func TestP13SettlementRealPreparationExitEligibilityAndGuildmate(t *testing.T) {
	for _, withGuildmate := range []bool{false, true} {
		name := "only-preparation"
		if withGuildmate {
			name = "guildmate-stays"
		}
		t.Run(name, func(t *testing.T) {
			f, ids, _, clock, match := p13UFFieldSetup(t)
			if err := f.s.InitializeP6(f.ctx); err != nil {
				t.Fatal(err)
			}
			if err := db.GetIns().Model(&db.P13UnionFightFieldPolicy{}).Where("id=1").Updates(map[string]any{"enabled": true, "preparation_millis": 300000, "left_revive_point_id": 1, "right_revive_point_id": 3, "initial_guard_hp": 1000, "initial_gate_hp": 10000, "move_tolerance": 3, "source": "isolated real preparation exit fixture"}).Error; err != nil {
				t.Fatal(err)
			}
			if err := db.GetIns().Model(&db.P13UnionFightSettlementPolicy{}).Where("`group`=?", match.Group).Updates(map[string]any{"enabled": true, "version": "isolated-preparation-exit-v1", "source": "isolated actual roster and entry, finite original item311 award fixture", "published_at": clock.Load() - 1, "participation_rewards": "item_311,1", "victory_rewards": "item_311,2", "win_prestige": 50, "lose_prestige": 25, "personal_missions": "", "task_rewards": "", "transport_mvp_rewards": "", "terminal_rule": "", "draw_prestige": 0, "abort_rule": "", "finalization_grace_millis": 0}).Error; err != nil {
				t.Fatal(err)
			}
			if err := f.s.p13UFSettlementFreeze(db.GetIns(), match, time.UnixMilli(clock.Load())); err != nil {
				t.Fatal(err)
			}
			var roster p13UFMatchRoster
			if err := json.Unmarshal(match.Roster, &roster); err != nil {
				t.Fatal(err)
			}
			own, other := roster.Left, roster.Right
			found := false
			for _, fighter := range own {
				if fighter.PlayerID == ids[0] {
					found = true
				}
			}
			if !found {
				own, other = other, own
			}
			if len(own) < 2 || len(other) < 1 {
				t.Fatal("real multiperson guild roster missing")
			}
			teammate := int64(0)
			for _, fighter := range own {
				if fighter.PlayerID != ids[0] {
					teammate = fighter.PlayerID
					break
				}
			}
			entrants := []int64{ids[0], other[0].PlayerID}
			if withGuildmate {
				entrants = append(entrants, teammate)
			}
			for _, id := range entrants {
				if _, err := f.s.p13UFFieldEnter(f.ctx, id, time.UnixMilli(clock.Load())); err != nil {
					t.Fatal(err)
				}
			}
			clock.Add(1000)
			if _, _, err := f.s.p13UFFieldLeaveExpected(f.ctx, ids[0], &pb.CSTransferScene{Target: proto.String("town")}, time.UnixMilli(clock.Load()), match.ID); err != nil {
				t.Fatal(err)
			}
			var departed db.P13UnionFightFieldPlayer
			if err := db.GetIns().First(&departed, "match_id=? AND player_id=?", match.ID, ids[0]).Error; err != nil {
				t.Fatal(err)
			}
			if departed.LeftAt <= 0 || departed.LeftAt > match.StartAt || !p13UFSettlementParticipantTimeValid(&departed, match.EndAt) || p13UFSettlementParticipated(&departed, match.StartAt) {
				t.Fatal("expected lawful preparation-only visit")
			}
			clock.Store(match.EndAt)
			var offer db.P13UnionFightSettlementOffer
			if err := db.GetIns().First(&offer, "match_id=?", match.ID).Error; err != nil {
				t.Fatal(err)
			}
			result, err := f.s.p13UFSettleZeroFight(db.GetIns(), match, &offer, time.UnixMilli(clock.Load()))
			if err != nil {
				t.Fatal(err)
			}
			if !withGuildmate {
				if result != nil || p1Count(t, &db.P13UnionFightSettlementResult{}, "match_id=?", match.ID) != 0 || p1Count(t, &db.RewardDelivery{}, "source_key=?", "p13-war:"+match.ID) != 0 {
					t.Fatal("preparation-only camp fabricated terminal or award")
				}
				return
			}
			if result == nil {
				t.Fatal("preparation leaver invalidated actual guildmate participation")
			}
			var awards []p13UFSettlementAward
			if err = json.Unmarshal(result.Awards, &awards); err != nil || len(awards) != 2 {
				t.Fatal("only two real battle participants", err, len(awards))
			}
			gotTeammate := false
			for _, award := range awards {
				if award.PlayerID == ids[0] {
					t.Fatal("preparation-only player awarded")
				}
				if award.PlayerID == teammate {
					gotTeammate = true
				}
			}
			if !gotTeammate {
				t.Fatal("remaining guildmate omitted")
			}
			if got := p1Count(t, &db.P13UnionFightFieldPlayer{}, "match_id=?", match.ID); got != 3 {
				t.Fatal("preparation actor removed from authenticated directory", got)
			}
			f.open(t)
			if err = f.s.p13UFSettlementDeliver(db.GetIns(), result, time.UnixMilli(result.AwardAt)); err != nil {
				t.Fatal(err)
			}
			if got := p1Count(t, &db.RewardDelivery{}, "source_key=?", "p13-war:"+match.ID); got != 2 {
				t.Fatal("incorrect participant deliveries", got)
			}
		})
	}
}
