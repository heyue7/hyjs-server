package service

import (
	"testing"
	"time"

	"gorm.io/gorm"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

func TestP1GatherPowerEntryRetriesOnlyWhenNeeded(t *testing.T) {
	f := p4Setup(t)
	ctx := p1GatherContext(f)
	if err := f.s.RecordP4Login(ctx, f.id); err != nil {
		t.Fatal(err)
	}
	period := p1Instance(t, f, "p1:catalog-power-entry", constUtil.P4KindPower, "single", 100, time.Now().Add(time.Hour), 0)
	var modes []bool
	run := func() error {
		return p1GatherTestSharedProfile(f.s, ctx, f.id, func(tx *gorm.DB, _ *pb.Player, _ *model.GameState) error {
			modes = append(modes, p1SharedRead(tx))
			return f.s.recordP4PowerEntry(tx, f.id, 42, time.Now())
		})
	}
	if err := run(); err != nil || len(modes) != 2 || !modes[0] || modes[1] {
		t.Fatalf("new power entry did not roll back and retry: %v %v", modes, err)
	}
	var progress db.PeriodProgress
	if err := db.GetIns().First(&progress, "instance_id=? AND player_id=?", period.ID, f.id).Error; err != nil || progress.Value != 42 ||
		p1Count(t, &db.EventFact{}, "player_id=? AND source_key=?", f.id, "power-entry:"+period.ID) != 1 {
		t.Fatalf("power entry was lost or duplicated: %+v %v", progress, err)
	}
	modes = nil
	if err := run(); err != nil || len(modes) != 1 || !modes[0] {
		t.Fatalf("already registered power entry left the snapshot fast path: %v %v", modes, err)
	}
}
