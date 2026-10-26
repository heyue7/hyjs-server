package service

import (
	"gorm.io/gorm"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	pb "hyjs-server/model/protocol"
	"testing"
	"time"
)

// TestP4CopyWindow validates every actual evening boundary and operator ownership.
// Boundary times exercise only configuration selection, never future production facts.
func TestP4CopyWindow(t *testing.T) {
	f := p4Setup(t)
	if err := f.s.InitializeP4CopyBenefits(f.ctx); err != nil {
		t.Fatal(err)
	}
	local := time.Now().In(time.FixedZone("UTC+8", 8*3600))
	day := time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, local.Location())
	key := constUtil.P4CopyWindowPrefix + local.Format("200601")
	var cfg db.P4CopyBenefitRule
	if err := db.GetIns().First(&cfg, "id=?", key).Error; err != nil {
		t.Fatal(err)
	}
	if cfg.Enabled || cfg.WeekdayMask != 127 || cfg.StartMinute != 1140 || cfg.EndMinute != 1260 || cfg.CurrencyFactor != 2 || cfg.DropFactor != 2 || cfg.Source != constUtil.P4CopyWindowSource {
		t.Fatal("declared evening instance", cfg)
	}
	// Publish this finite evening bonus only inside the isolated regression.
	if err := db.GetIns().Model(&cfg).Update("enabled", true).Error; err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		at     time.Time
		factor int64
	}{
		{day.Add(19*time.Hour - time.Millisecond), 1}, {day.Add(19 * time.Hour), 2}, {day.Add(21*time.Hour - time.Millisecond), 2}, {day.Add(21 * time.Hour), 1},
	} {
		if err := db.GetIns().Transaction(func(tx *gorm.DB) error {
			coin, exp, err := f.s.ApplyP4CopyCurrencyBonus(tx, tc.at, 100, 20)
			if err == nil && (coin != 100*tc.factor || exp != 20*tc.factor) {
				t.Fatalf("clock boundary %v got%d/%d", tc.at, coin, exp)
			}
			return err
		}); err != nil {
			t.Fatal(err)
		}
	}
	tabs, err := f.s.P4CopyBenefitsTabs(f.ctx, day.Add(20*time.Hour))
	if err != nil || len(tabs) != 1 || tabs[0].GetActivityId() != constUtil.P4CopyBenefitsActivityID || tabs[0].GetParam() != "2" {
		t.Fatal("original evening static page", tabs, err)
	}
	info, err := f.s.P4CopyBenefitsInfo(f.ctx, day.Add(20*time.Hour))
	if err != nil || info[0].(*pb.SCActivityInfo).GetInformation() != "今日19:00～21:00，主线金币和经验2倍，战利品数量2倍。" {
		t.Fatal("ordinary player wording", info, err)
	}
	if err := db.GetIns().Model(&cfg).Update("enabled", false).Error; err != nil {
		t.Fatal(err)
	}
	if err := f.s.InitializeP4CopyBenefits(f.ctx); err != nil {
		t.Fatal(err)
	}
	tabs, err = f.s.P4CopyBenefitsTabs(f.ctx, day.Add(20*time.Hour))
	if err != nil || len(tabs) != 0 {
		t.Fatal("operatoroff silently reopened", tabs, err)
	}
	// A new month stays disabled until explicitly published, with its own identity.
	next := time.Date(local.Year(), local.Month()+1, 1, 20, 0, 0, 0, local.Location())
	if err := f.s.seedP4CopyWindow(db.GetIns(), next); err != nil {
		t.Fatal(err)
	}
	var nextCfg db.P4CopyBenefitRule
	if err := db.GetIns().First(&nextCfg, "id=?", constUtil.P4CopyWindowPrefix+next.Format("200601")).Error; err != nil {
		t.Fatal(err)
	}
	if nextCfg.Enabled || nextCfg.StartAt != time.Date(next.Year(), next.Month(), 1, 0, 0, 0, 0, next.Location()).UnixMilli() {
		t.Fatal("next independent month")
	}
	if err := db.GetIns().Delete(&nextCfg).Error; err != nil {
		t.Fatal(err)
	}
	custom := nextCfg
	custom.ID = "operator-evening-off"
	custom.Enabled = false
	custom.Source = "explicit existing closed operator window"
	if err := db.GetIns().Create(&custom).Error; err != nil {
		t.Fatal(err)
	}
	if err := f.s.seedP4CopyWindow(db.GetIns(), next); err != nil {
		t.Fatal(err)
	}
	if p1Count(t, &db.P4CopyBenefitRule{}, "id=?", nextCfg.ID) != 0 {
		t.Fatal("existing operatoroff ignored")
	}
}
