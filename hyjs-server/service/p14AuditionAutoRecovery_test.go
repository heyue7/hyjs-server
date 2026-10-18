package service

import (
	"encoding/json"
	"sync"
	"testing"
	"time"

	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

func TestP14AutoRecoveryRealSignupOfflineAcrossAuditionPrepublishedColdAndOnce(t *testing.T) {
	// The helper signs up three real accounts and freezes at stage3. It never
	// visits stage4 or prepares a round, matching a service outage over that window.
	f, audition := p14AuditionMatchSetup(t, 3)
	at := audition.Add(2 * time.Hour)
	if err := f.s.InitializeP14Qualification(f.ctx); err != nil {
		t.Fatal(err)
	}
	if err := db.GetIns().Model(&db.P14AuditionMatchPolicy{}).Where("id=1").Update("enabled", true).Error; err != nil {
		t.Fatal(err)
	}
	if err := f.s.TickP14Season(f.ctx, at); err != nil {
		t.Fatal(err)
	}
	var count int64
	db.GetIns().Model(&db.P14AuditionQualification{}).Count(&count)
	if count != 0 {
		t.Fatal("default recovery policy fabricated retrospective qualification")
	}
	var boundary db.P14AuditionBoundary
	db.GetIns().First(&boundary, "season_index=7 AND server_id=1")
	if err := db.GetIns().Model(&db.P14AuditionMatchPolicy{}).Where("id=1").Updates(map[string]any{"auto_recovery_enabled": true, "auto_recovery_published_at": audition.Add(-2 * time.Minute).UnixMilli(), "auto_recovery_source": constUtil.P14AuditionAutoRecoverySource}).Error; err != nil {
		t.Fatal(err)
	}
	workers := 1
	if f.mysql.Database != "" {
		workers = 8
	}
	var wg sync.WaitGroup
	errs := make(chan error, workers)
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); errs <- f.s.TickP14Season(f.ctx, at) }()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	qualified, err := f.s.p14AuditionQualificationRead(f.ctx, db.GetIns(), 7, 1)
	if err != nil || qualified.QualifiedAt != at.UnixMilli() || qualified.RoundCount != 0 || qualified.RecoverySource != constUtil.P14AuditionAutoRecoverySource {
		t.Fatal("actual observation, never historical win", err)
	}
	db.GetIns().Model(&db.P14AuditionQualification{}).Count(&count)
	if count != 1 {
		t.Fatal("duplicate recovery", count)
	}
	for _, table := range []any{&db.P14AuditionRound{}, &db.P14AuditionMatch{}} {
		db.GetIns().Model(table).Count(&count)
		if count != 0 {
			t.Fatal("automatic capacity recovery fabricated battles")
		}
	}
	if err := db.GetIns().Model(&db.P14QualificationFederationPolicy{}).Where("id=1").Update("enabled", true).Error; err != nil {
		t.Fatal(err)
	}
	if err := f.s.QueueP14Qualifications(f.ctx, at); err != nil {
		t.Fatal(err)
	}
	var delivery db.P14QualificationDelivery
	if err := db.GetIns().First(&delivery).Error; err != nil {
		t.Fatal(err)
	}
	var request model.P14QualificationImportRequest
	if json.Unmarshal(delivery.Payload, &request) != nil {
		t.Fatal("actual recovered outbox")
	}
	if _, _, peerQualified, err := p14QualificationProof(request, at); err != nil || peerQualified.RecoverySource != qualified.RecoverySource {
		t.Fatal("foreign certificate retains explicit recovery policy", err)
	}
	var players []int64
	if json.Unmarshal(qualified.Players, &players) != nil || len(players) != 3 {
		t.Fatal("actual recovered players")
	}
	frames, err := f.s.p14AuditionResult(f.ctx, players[0], at)
	if err != nil {
		t.Fatal("recovered original promotion stage6 original result", err)
	}
	result := frames[0].(*pb.SCKuaFuMatchResult)
	if result.GetResult() != 1 || result.GetMatchResult().GetWinTimes() != 0 || result.GetMatchResult().GetFailTimes() != 0 {
		t.Fatal("recovery fabricated battle stats")
	}
	beforeHash := qualified.SourceHash
	sqlDB, _ := db.GetIns().DB()
	if err := sqlDB.Close(); err != nil {
		t.Fatal(err)
	}
	f.open(t)
	if err := db.GetIns().Model(&db.P14AuditionMatchPolicy{}).Where("id=1").Update("auto_recovery_enabled", false).Error; err != nil {
		t.Fatal(err)
	}
	qualified, err = f.s.p14AuditionQualificationRead(f.ctx, db.GetIns(), 7, 1)
	if err != nil || qualified.SourceHash != beforeHash {
		t.Fatal("closed policy erased genuine already recovered qualification", err)
	}
	var after db.P14AuditionBoundary
	db.GetIns().First(&after, "season_index=7 AND server_id=1")
	if after.SourceHash != boundary.SourceHash {
		t.Fatal("recovery changed the actual deadline roster")
	}
}

func TestP14AutoRecoveryLatePublicationRefusedAndLargeRosterStaysPending(t *testing.T) {
	f, audition := p14AuditionMatchSetup(t, 3)
	at := audition.Add(2 * time.Hour)
	if err := db.GetIns().Model(&db.P14AuditionMatchPolicy{}).Where("id=1").Updates(map[string]any{"enabled": true, "auto_recovery_enabled": true, "auto_recovery_published_at": audition.UnixMilli(), "auto_recovery_source": constUtil.P14AuditionAutoRecoverySource}).Error; err != nil {
		t.Fatal(err)
	}
	if err := f.s.TickP14Season(f.ctx, at); err == nil {
		t.Fatal("late rule rewrote an elapsed audition")
	}
	var count int64
	db.GetIns().Model(&db.P14AuditionQualification{}).Count(&count)
	if count != 0 {
		t.Fatal("late policy left partial qualification")
	}
}

func TestP14AutoRecoveryElevenRealSignupCannotBypassAuthority(t *testing.T) {
	f, audition := p14AuditionMatchSetup(t, 11)
	if err := db.GetIns().Model(&db.P14AuditionMatchPolicy{}).Where("id=1").Updates(map[string]any{"enabled": true, "auto_recovery_enabled": true, "auto_recovery_published_at": audition.Add(-2 * time.Minute).UnixMilli(), "auto_recovery_source": constUtil.P14AuditionAutoRecoverySource}).Error; err != nil {
		t.Fatal(err)
	}
	if err := f.s.TickP14Season(f.ctx, audition.Add(2*time.Hour)); err != nil {
		t.Fatal(err)
	}
	var count int64
	db.GetIns().Model(&db.P14AuditionQualification{}).Count(&count)
	if count != 0 {
		t.Fatal("eleven real entries fabricated finalten")
	}
}
