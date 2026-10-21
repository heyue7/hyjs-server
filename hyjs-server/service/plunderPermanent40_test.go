package service

import (
	"encoding/json"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"testing"
	"time"
)

func TestPlunderPermanent40MigrationFrozenOfferAndMaintenance(t *testing.T) {
	f := feedback40Database(t, "plunder40.db")
	t.Cleanup(func() {
		connection, err := db.GetIns().DB()
		if err == nil {
			connection.Close()
		}
	})
	f.s = &GameService{}
	if err := db.GetIns().AutoMigrate(&db.P11PlunderFightPolicy{}, &db.P11PlunderFightOffer{}); err != nil {
		t.Fatal(err)
	}
	if err := AdminInitialize(f.ctx, ""); err != nil {
		t.Fatal(err)
	}
	original := db.P11PlunderFightPolicy{ID: 1, Version: "local-plunder-type3-v1", MatchCount: 3, ScanLimit: 512, LevelDifference: 20, MatchMillis: 300000, ChanceTier: 3, DropPPM: 250000, Source: constUtil.P11PlunderFightPolicySource}
	if err := db.GetIns().Create(&original).Error; err != nil {
		t.Fatal(err)
	}
	// Operator changes and previously published closures must remain untouched.
	for _, change := range []func(*db.P11PlunderFightPolicy){
		func(c *db.P11PlunderFightPolicy) { c.Version = "operator-policy" },
		func(c *db.P11PlunderFightPolicy) { c.PublishedAt = 1; c.StartAt = 1; c.EndAt = 2 },
		func(c *db.P11PlunderFightPolicy) { c.DropPPM = 1000000 },
	} {
		custom := original
		change(&custom)
		if err := db.GetIns().Save(&custom).Error; err != nil {
			t.Fatal(err)
		}
		if err := f.s.InitializePlunderPermanent40(f.ctx); err != nil {
			t.Fatal(err)
		}
		var kept db.P11PlunderFightPolicy
		db.GetIns().First(&kept, 1)
		if kept != custom {
			t.Fatal("operator policy changed", kept, custom)
		}
	}
	if err := db.GetIns().Save(&original).Error; err != nil {
		t.Fatal(err)
	}
	if err := f.s.InitializePlunderPermanent40(f.ctx); err != nil {
		t.Fatal(err)
	}
	var cfg db.P11PlunderFightPolicy
	if err := db.GetIns().First(&cfg, 1).Error; err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	if !cfg.Permanent || !cfg.Enabled || cfg.SweepEnabled || cfg.DropPPM != original.DropPPM || !p11PlunderPolicyOpen(&cfg, now.AddDate(1, 0, 0).UnixMilli()) {
		t.Fatal("permanent default", cfg)
	}
	var audit db.AdminAudit
	if err := db.GetIns().Where("action=?", constUtil.P11PlunderPermanentMigration).First(&audit).Error; err != nil {
		t.Fatal(err)
	}
	var detail struct {
		Before  db.P11PlunderFightPolicy
		Applied bool
	}
	if err := json.Unmarshal([]byte(audit.Detail), &detail); err != nil || !detail.Applied || detail.Before != original {
		t.Fatal("backup mismatch", detail, err)
	}
	offer, _, err := p11PlunderFightOffer(db.GetIns(), now)
	if err != nil {
		t.Fatal(err)
	}
	oldSnapshot := offer.Snapshot
	if p11PlunderMatchExpires(&cfg, now.UnixMilli()) != now.UnixMilli()+300000 {
		t.Fatal("permanent candidate expiry")
	}
	if err := db.GetIns().Model(&cfg).Update("enabled", false).Error; err != nil {
		t.Fatal(err)
	}
	if err = f.s.InitializePlunderPermanent40(f.ctx); err != nil {
		t.Fatal(err)
	}
	if _, _, err = p11PlunderFightOffer(db.GetIns(), now); err == nil || err.Error() != constUtil.P11PlunderFightUnavailable {
		t.Fatal("maintenance reopened", err)
	}
	var count int64
	db.GetIns().Model(&db.AdminAudit{}).Where("action=?", constUtil.P11PlunderPermanentMigration).Count(&count)
	if count != 1 {
		t.Fatal("migration repeated", count)
	}
	db.GetIns().First(offer, "version=?", offer.Version)
	if offer.Snapshot != oldSnapshot {
		t.Fatal("old offer mutated")
	}
	// A deliberately restored original row retains the applied marker and stays closed.
	if err = db.GetIns().Save(&original).Error; err != nil {
		t.Fatal(err)
	}
	if err = f.s.InitializePlunderPermanent40(f.ctx); err != nil {
		t.Fatal(err)
	}
	db.GetIns().First(&cfg, 1)
	if cfg.Enabled {
		t.Fatal("marker ignored")
	}
}

func TestPlunderPermanent40FiniteCompatibility(t *testing.T) {
	now := time.Now().Truncate(time.Millisecond)
	cfg := db.P11PlunderFightPolicy{ID: 1, Enabled: true, Version: "finite", PublishedAt: now.Add(-time.Hour).UnixMilli(), StartAt: now.Add(-time.Hour).UnixMilli(), EndAt: now.Add(time.Hour).UnixMilli(), MatchCount: 3, ScanLimit: 512, LevelDifference: 20, MatchMillis: 300000, ChanceTier: 3, DropPPM: 250000, Source: constUtil.P11PlunderFightPolicySource}
	raw, _ := json.Marshal(cfg)
	offer := db.P11PlunderFightOffer{Version: cfg.Version, Snapshot: string(raw), SourceHash: p6Digest(raw), CapturedAt: now.UnixMilli()}
	if _, err := p11PlunderFightOfferRules(&offer); err != nil {
		t.Fatal("legacy snapshot", err)
	}
	if !p11PlunderPolicyOpen(&cfg, now.UnixMilli()) || p11PlunderPolicyOpen(&cfg, cfg.EndAt) {
		t.Fatal("finite boundary")
	}
	if p11PlunderMatchExpires(&cfg, cfg.EndAt-1) != cfg.EndAt {
		t.Fatal("finite expiry")
	}
	cfg.Permanent = true
	if p11PlunderFightPolicyValid(&cfg) {
		t.Fatal("permanent finite end accepted")
	}
	cfg.EndAt = 0
	if !p11PlunderFightPolicyValid(&cfg) || p11PlunderPolicyOpen(&cfg, cfg.StartAt-1) {
		t.Fatal("permanent start boundary")
	}
	cfg.Enabled = false
	if p11PlunderPolicyOpen(&cfg, now.UnixMilli()) {
		t.Fatal("disabled permanent")
	}
}
