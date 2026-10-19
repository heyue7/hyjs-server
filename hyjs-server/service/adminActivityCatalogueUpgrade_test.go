package service

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

// TestAdminActivityCatalogueUpgrade preserves real consumer/history bytes while
// removing generated aliases and upgrading only untouched legacy definitions.
func TestAdminActivityCatalogueUpgrade(t *testing.T) {
	f, actor := adminModesFixture(t)
	if err := f.s.InitializeAdminActivitySchedules(f.ctx); err != nil {
		t.Fatal(err)
	}
	var source db.PeriodInstance
	if err := db.GetIns().Where("kind=?", constUtil.P4KindCollect).First(&source).Error; err != nil {
		t.Fatal(err)
	}
	id := "import_" + p6Digest([]byte("period:" + source.ID))[:24]
	var upgraded db.AdminActivityDefinition
	if err := db.GetIns().First(&upgraded, "id=?", id).Error; err != nil {
		t.Fatal(err)
	}
	upgraded.Mode = "unsupported"
	upgraded.Usable = false
	if err := db.GetIns().Save(&upgraded).Error; err != nil {
		t.Fatal(err)
	}
	aliases := []db.AdminActivityDefinition{}
	for _, key := range []string{"period:totalLogin_p1_lp-owned_three", "commerce:p4:ordinary:202611", "pool:recruit_1", "pool:recruit_2"} {
		aliases = append(aliases, db.AdminActivityDefinition{ID: "import_" + p6Digest([]byte(key))[:24], Name: "旧自动入口", Mode: "unsupported", SourceActivityID: key, FormJSON: adminJSON(model.AdminActivityForm{Mode: "unsupported"}), Revision: 1})
	}
	if err := db.GetIns().Create(&aliases).Error; err != nil {
		t.Fatal(err)
	}
	// An operator's explicit template edit remains authoritative.
	manual := aliases[0]
	manual.ID = "template_operator_owned"
	manual.Name = "运营保留模板"
	if err := db.GetIns().Create(&manual).Error; err != nil {
		t.Fatal(err)
	}
	before, _ := json.Marshal(source)
	profileBefore, _ := f.profile(t)
	if err := f.s.InitializeAdminActivitySchedules(f.ctx); err != nil {
		t.Fatal(err)
	}
	first := []db.AdminActivityDefinition{}
	if err := db.GetIns().Order("id").Find(&first).Error; err != nil {
		t.Fatal(err)
	}
	for _, a := range aliases {
		var row db.AdminActivityDefinition
		if err := db.GetIns().First(&row, "id=?", a.ID).Error; err != nil || !row.Archived {
			t.Fatal("obsolete alias not archived", a.SourceActivityID, err)
		}
	}
	if err := db.GetIns().First(&upgraded, "id=?", id).Error; err != nil || upgraded.Mode != "hero-collection" || !upgraded.Usable {
		t.Fatal("real supported activity stayed unavailable", err, upgraded.Mode)
	}
	if err := f.s.InitializeAdminActivitySchedules(f.ctx); err != nil {
		t.Fatal(err)
	}
	second := []db.AdminActivityDefinition{}
	if err := db.GetIns().Order("id").Find(&second).Error; err != nil {
		t.Fatal(err)
	}
	a, _ := json.Marshal(first)
	b, _ := json.Marshal(second)
	if string(a) != string(b) {
		t.Fatal("startup cleanup not idempotent")
	}
	var after db.PeriodInstance
	if err := db.GetIns().First(&after, "id=?", source.ID).Error; err != nil {
		t.Fatal(err)
	}
	afterBytes, _ := json.Marshal(after)
	if string(before) != string(afterBytes) {
		t.Fatal("source activity rewritten")
	}
	profileAfter, _ := f.profile(t)
	if !proto.Equal(profileBefore, profileAfter) {
		t.Fatal("catalogue migration changed player")
	}
	listed, err := f.s.AdminActivityDefinitions(f.ctx, actor)
	if err != nil {
		t.Fatal(err)
	}
	foundManual := false
	for _, row := range listed {
		if row.ID == manual.ID {
			foundManual = true
		}
		if adminActivityRedundantSource(row.SourceActivityID) && row.ID != manual.ID {
			t.Fatal("obsolete aliases still listed")
		}
	}
	if !foundManual {
		t.Fatal("operator-owned template removed")
	}
}

// TestAdminAdditionalActivityNativeModes checks typed rules against actual
// original messages, then claims a real recorded collection goal once across a cold restart.
func TestAdminAdditionalActivityNativeModes(t *testing.T) {
	f, actor := adminModesFixture(t)
	now := time.Now()
	for _, mode := range []string{"arena-wins", "plunder-count", "hero-collection", "trial-stars", "power-race"} {
		form := adminModeForm(mode)
		form.Milestones = []model.AdminActivityMilestone{{Goal: 2, Rewards: []model.AdminResourceAmount{{ResourceID: "coin", Amount: 17}}}}
		if mode == "power-race" {
			form.Options.RankPlaces = 3
			form.Options.RankRewards = []model.AdminResourceAmount{{ResourceID: "diamond", Amount: 4}}
		}
		saved, err := f.s.AdminSaveTypedActivity(f.ctx, actor, "", model.AdminTypedActivityRequest{AdminActivityForm: form, Enabled: true})
		if err != nil {
			t.Fatal(mode, err)
		}
		_, key, _ := strings.Cut(saved.ID, ":")
		var rule db.PeriodInstance
		if err := db.GetIns().First(&rule, "id=?", key).Error; err != nil {
			t.Fatal(err)
		}
		expected := "cumulative"
		if mode == "trial-stars" || mode == "power-race" {
			expected = "single"
		}
		if rule.Mode != expected {
			t.Fatal("wrong progress semantics", mode, rule.Mode)
		}
		if mode == "power-race" && (rule.RankPlaces != 3 || rule.RankRewards != "diamond,4") {
			t.Fatal("rank reward fields lost")
		}
		if _, err := f.s.P1Activity(f.ctx, f.id, &pb.CSActivityInfo{ActivityId: proto.String(key)}); err != nil {
			t.Fatal("native activity info", mode, err)
		}
		if mode == "hero-collection" {
			f.event(t, "owned:catalogue-one", constUtil.P4KindCollect, 1, now)
			f.event(t, "owned:catalogue-two", constUtil.P4KindCollect, 1, now)
			req := &pb.CSActivityReward{ActivityId: proto.String(key), Index: proto.Int32(1), GetResources: []*pb.BaseResource{{Id: proto.String("coin"), Num: proto.Int32(17)}}}
			_, err := f.s.P1Activity(f.ctx, f.id, req)
			if err != nil {
				t.Fatal("native goal claim", err)
			}
			p, _ := f.profile(t)
			coin := p.GetCoin()
			f.restart(t)
			_, err = f.s.P1Activity(f.ctx, f.id, req)
			if err != nil {
				t.Fatal("native cold receipt", err)
			}
			p, _ = f.profile(t)
			if p.GetCoin() != coin {
				t.Fatal("repeat issued more currency")
			}
		}
	}
}

// TestAdminConfiguredBattleActivityFacts uses accepted native settlements, not
// fabricated counters, to check the newly typed non-monthly period identities.
func TestAdminConfiguredBattleActivityFacts(t *testing.T) {
	for _, mode := range []string{"arena-wins", "plunder-count"} {
		t.Run(mode, func(t *testing.T) {
			var f *p1Fixture
			var opponent int64
			if mode == "arena-wins" {
				f, opponent = p11ArenaScopedSetup(t)
			} else {
				f, opponent, _, _ = p11PlunderFightSetup(t)
			}
			if err := f.s.InitializeP4(f.ctx); err != nil {
				t.Fatal(err)
			}
			for _, initialize := range []func() error{func() error { return AdminInitialize(f.ctx, "") }, func() error { return InitializeAdminOperations(f.ctx) }, func() error { return InitializeAdminActivityModes(f.ctx) }} {
				if err := initialize(); err != nil {
					t.Fatal(err)
				}
			}
			actorRow := db.AdminUser{Username: "native-" + p6Digest([]byte(t.Name()))[:20], PasswordHash: "unused-fixture", Role: constUtil.AdminRoleOperator, Enabled: true}
			if err := db.GetIns().Create(&actorRow).Error; err != nil {
				t.Fatal(err)
			}
			actor := &model.AdminPrincipal{UserID: actorRow.ID, Username: actorRow.Username, Role: actorRow.Role}
			form := adminModeForm(mode)
			form.Milestones = []model.AdminActivityMilestone{{Goal: 1, Rewards: []model.AdminResourceAmount{{ResourceID: "item_005", Amount: 1}}}}
			saved, err := f.s.AdminSaveTypedActivity(f.ctx, actor, "", model.AdminTypedActivityRequest{AdminActivityForm: form, Enabled: true})
			if err != nil {
				t.Fatal(err)
			}
			_, key, _ := strings.Cut(saved.ID, ":")
			winnerID := f.id
			if mode == "arena-wins" {
				winnerID = opponent
				if err := updateProfile(f.ctx, opponent, func(p *pb.Player, g *model.GameState) error { p.Level = proto.Int32(100); return nil }); err != nil {
					t.Fatal(err)
				}
				for _, table := range []any{&db.P11ArenaPolicy{}, &db.P11ArenaFightPolicy{}} {
					if err := db.GetIns().Model(table).Where("id=1").Update("enabled", true).Error; err != nil {
						t.Fatal(err)
					}
				}
				if _, err := f.s.p11ArenaQuery(f.ctx, opponent, &pb.CSPvpFindMatch{}, 962001, time.Now()); err != nil {
					t.Fatal(err)
				}
				out, err := f.s.p11ArenaScopedFightAt(f.ctx, opponent, &pb.CSPvpFightResult{PlayerId: proto.Int64(f.id), ServerId: proto.Int32(1)}, 962002, time.Now())
				if err != nil || out[0].(*pb.SCPvpFightResult).GetWinPlayerId() != opponent {
					t.Fatal("native win", err)
				}
			} else {
				p11PlunderFiniteItems(t, f, opponent, "frag_trea_001_1,1")
				p11PlunderPublish(t, time.Now())
				if _, err := f.s.p11PlunderListAt(f.ctx, f.id, "frag_trea_001_1", time.Now()); err != nil {
					t.Fatal(err)
				}
				if _, err := f.s.p11PlunderFightAt(f.ctx, f.id, &pb.CSFightTreasureStartResult{FightPlayerId: proto.Int64(opponent), TargetFlagId: proto.String("frag_trea_001_1")}, 962003, time.Now()); err != nil {
					t.Fatal("native plunder", err)
				}
			}
			out, err := f.s.P1Activity(f.ctx, winnerID, &pb.CSActivityInfo{ActivityId: proto.String(key)})
			if err != nil {
				t.Fatal(err)
			}
			items := out[0].(*pb.SCActivityInfo).GetItemList()
			if len(items) != 1 || items[0].GetStatus() != 1 || items[0].GetLeft() != 1 {
				t.Fatal("accepted native fact did not reach operator activity", mode, items)
			}
		})
	}
}
