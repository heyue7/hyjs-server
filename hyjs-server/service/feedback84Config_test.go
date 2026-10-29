package service

import (
	"errors"
	"reflect"
	"testing"

	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
)

func feedback84ConfigFixture(t *testing.T) (*p1Fixture, *model.AdminPrincipal) {
	t.Helper()
	f, actor := reuseFixture(t, false)
	if err := f.s.InitializeClientUpdates(f.ctx); err != nil {
		t.Fatal(err)
	}
	if err := db.GetIns().Save(&db.ClientUpdatePolicy{ID: 1, Maintenance: true}).Error; err != nil {
		t.Fatal(err)
	}
	if err := f.s.InitializeP11PlunderFight(f.ctx); err != nil {
		t.Fatal(err)
	}
	// Subtests share one managed schema. Reset only this isolated publication's
	// operating row and audit, so each rejection starts from the actual default.
	if err := db.GetIns().Where("id=?", 1).Delete(&db.P11PlunderFightPolicy{}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.GetIns().Where("target=?", constUtil.Feedback84PolicyTarget).Delete(&db.AdminAudit{}).Error; err != nil {
		t.Fatal(err)
	}
	if err := f.s.InitializeP11PlunderFight(f.ctx); err != nil {
		t.Fatal(err)
	}
	if err := f.s.InitializePlunderPermanent40(f.ctx); err != nil {
		t.Fatal(err)
	}
	return f, actor
}

func TestFeedback84PublicationRejectsInvalidOrStaleClock(t *testing.T) {
	for _, mode := range []string{"invalid", "stale"} {
		t.Run(mode, func(t *testing.T) {
			f, actor := feedback84ConfigFixture(t)
			ctx, _, err := f.s.bindServerGameClockWorker75(f.ctx)
			if err != nil {
				t.Fatal(err)
			}
			var clock db.ServerGameClock
			if err := db.GetIns().First(&clock, "server_id=1").Error; err != nil {
				t.Fatal(err)
			}
			originalClock := clock
			defer func() {
				if err := db.GetIns().Save(&originalClock).Error; err != nil {
					t.Error(err)
				}
			}()
			revision := clock.Revision + 1
			if mode == "invalid" {
				revision = -1
			}
			if err := db.GetIns().Model(&clock).Update("revision", revision).Error; err != nil {
				t.Fatal(err)
			}
			if err := f.s.ApplyFeedback84Config(ctx, actor); err == nil {
				t.Fatal("invalid or stale clock published sweep")
			}
			var policy db.P11PlunderFightPolicy
			if err := db.GetIns().First(&policy, 1).Error; err != nil || policy.SweepEnabled || policy.Version != constUtil.P11PlunderPermanentVersion {
				t.Fatal("clock failure changed policy", err, policy)
			}
		})
	}
}

func TestFeedback84ExplicitSweepPublicationAndColdIdempotency(t *testing.T) {
	f, actor := feedback84ConfigFixture(t)
	var before db.P11PlunderFightPolicy
	if err := db.GetIns().First(&before, 1).Error; err != nil {
		t.Fatal(err)
	}
	var profile db.Profile
	if err := db.GetIns().First(&profile, f.id).Error; err != nil {
		t.Fatal(err)
	}
	snapshot := adminJSON(before)
	offer := db.P11PlunderFightOffer{Version: before.Version, Snapshot: string(snapshot), SourceHash: p6Digest(snapshot), CapturedAt: before.PublishedAt}
	if err := db.GetIns().Create(&offer).Error; err != nil {
		t.Fatal(err)
	}
	if err := f.s.ApplyFeedback84Config(f.ctx, actor); err != nil {
		t.Fatal(err)
	}
	var actual db.P11PlunderFightPolicy
	if err := db.GetIns().First(&actual, 1).Error; err != nil {
		t.Fatal(err)
	}
	expected := before
	expected.Version, expected.SweepEnabled = constUtil.Feedback84PlunderVersion, true
	if !reflect.DeepEqual(actual, expected) {
		t.Fatal("unrelated operating terms changed", actual)
	}
	var savedOffer db.P11PlunderFightOffer
	if err := db.GetIns().First(&savedOffer, "version=?", offer.Version).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := p11PlunderFightOfferRules(&savedOffer); err != nil {
		t.Fatal("old frozen offer no longer readable", err)
	}
	var afterProfile db.Profile
	if err := db.GetIns().First(&afterProfile, f.id).Error; err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(offer, savedOffer) || !reflect.DeepEqual(profile.Player, afterProfile.Player) || !reflect.DeepEqual(profile.Game, afterProfile.Game) {
		t.Fatal("historical offer or assets changed")
	}
	cold := &GameService{ServerID: 1, ServerClockReady: true}
	if err := cold.ApplyFeedback84Config(f.ctx, actor); err != nil {
		t.Fatal(err)
	}
	var count int64
	if err := db.GetIns().Model(&db.AdminAudit{}).Where("action=?", constUtil.Feedback84Migration).Count(&count).Error; err != nil || count != 1 {
		t.Fatal("repeat audit", count, err)
	}
	if err := db.GetIns().Model(&actual).Update("sweep_enabled", false).Error; err != nil {
		t.Fatal(err)
	}
	if err := cold.ApplyFeedback84Config(f.ctx, actor); !errors.Is(err, constUtil.ErrAdminConfigConflict) {
		t.Fatal("later closure reopened", err)
	}
}

func TestFeedback84PublicationRejectsDriftAndNonMaintenance(t *testing.T) {
	for _, mode := range []string{"maintenance", "policy", "audit"} {
		t.Run(mode, func(t *testing.T) {
			f, actor := feedback84ConfigFixture(t)
			switch mode {
			case "maintenance":
				if err := db.GetIns().Model(&db.ClientUpdatePolicy{}).Where("id=1").Update("maintenance", false).Error; err != nil {
					t.Fatal(err)
				}
			case "policy":
				if err := db.GetIns().Model(&db.P11PlunderFightPolicy{}).Where("id=1").Update("drop_ppm", 300000).Error; err != nil {
					t.Fatal(err)
				}
			case "audit":
				if err := db.GetIns().Create(&db.AdminAudit{Actor: "operator", Action: "closed", Target: constUtil.Feedback84PolicyTarget}).Error; err != nil {
					t.Fatal(err)
				}
			}
			var before db.P11PlunderFightPolicy
			if err := db.GetIns().First(&before, 1).Error; err != nil {
				t.Fatal(err)
			}
			if err := f.s.ApplyFeedback84Config(f.ctx, actor); !errors.Is(err, constUtil.ErrAdminConfigConflict) {
				t.Fatal("invalid publication accepted", err)
			}
			var after db.P11PlunderFightPolicy
			if err := db.GetIns().First(&after, 1).Error; err != nil || !reflect.DeepEqual(before, after) {
				t.Fatal("partial policy update", err)
			}
		})
	}
}
