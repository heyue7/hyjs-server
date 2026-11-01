package service

import (
	"context"
	"errors"
	"os"
	"sync"
	"testing"
	"time"

	"gorm.io/gorm"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

// The public generic template-copy route must wait for the admitted gather;
// the next player then observes and freezes the newly published instance.
func TestP1GatherCatalogAdminCopyMySQL(t *testing.T) {
	if os.Getenv("HYJS_P1_MYSQL_DATABASE") == "" {
		t.Skip("requires an isolated hyjs_test_p1_ MySQL database")
	}
	f := p1GatherFixture(t)
	peer := p1GatherSecondPlayer(t, f)
	if err := AdminInitialize(f.ctx, ""); err != nil {
		t.Fatal(err)
	}
	if err := InitializeAdminOperations(f.ctx); err != nil {
		t.Fatal(err)
	}
	operator := db.AdminUser{Username: "p1-catalog-isolated-operator", PasswordHash: "unused-fixture", Role: constUtil.AdminRoleOperator, Enabled: true}
	if err := db.GetIns().Create(&operator).Error; err != nil {
		t.Fatal(err)
	}
	principal := &model.AdminPrincipal{UserID: operator.ID, Username: operator.Username, Role: operator.Role}
	template := p1Instance(t, f, "p1_catalog_template", constUtil.P8EventMineGather, "cumulative", 1, time.Now().Add(time.Hour), 0)
	if err := db.GetIns().Model(&template).Update("enabled", false).Error; err != nil {
		t.Fatal(err)
	}
	template.Enabled = false
	copy := template
	copy.ID += "_next"
	request := model.AdminActivityCreateRequest{TemplateID: "period:" + template.ID, NewID: copy.ID, Enabled: true, Config: adminJSON(copy)}
	ctx, cancel := context.WithTimeout(p1GatherContext(f), 15*time.Second)
	defer cancel()
	entered, release := make(chan struct{}), make(chan struct{})
	var releaseOnce sync.Once
	defer releaseOnce.Do(func() { close(release) })
	gatherDone := make(chan error, 1)
	go func() {
		gatherDone <- p1GatherTestSharedProfile(f.s, ctx, f.id, func(tx *gorm.DB, _ *pb.Player, _ *model.GameState) error {
			if err := f.s.recordP1GatherEvent(tx, f.id, &db.P8GatherAttempt{PlayerID: f.id, Token: "catalog-before-publish"}, constUtil.P8EventMineGather, false, time.Now()); err != nil {
				return err
			}
			close(entered)
			select {
			case <-release:
				return nil
			case <-ctx.Done():
				return ctx.Err()
			}
		})
	}()
	select {
	case <-entered:
	case err := <-gatherDone:
		t.Fatalf("gather stopped before publication: %v", err)
	case <-ctx.Done():
		t.Fatal("gather timed out")
	}
	publishStarted := make(chan struct{})
	publishDone := make(chan error, 1)
	go func() {
		close(publishStarted)
		_, err := f.s.AdminCreateActivity(ctx, principal, request)
		publishDone <- err
	}()
	<-publishStarted
	select {
	case err := <-publishDone:
		t.Fatalf("publisher passed an admitted gather: %v", err)
	case <-time.After(300 * time.Millisecond):
	case <-ctx.Done():
		t.Fatal("publisher timed out")
	}
	releaseOnce.Do(func() { close(release) })
	for _, done := range []<-chan error{gatherDone, publishDone} {
		select {
		case err := <-done:
			if err != nil {
				t.Fatal(err)
			}
		case <-ctx.Done():
			t.Fatal("transaction did not finish after gather release")
		}
	}
	if p1Count(t, &db.PeriodProgress{}, "instance_id=? AND player_id=?", copy.ID, f.id) != 0 {
		t.Fatal("gather admitted before publication counted in the new period")
	}
	var modes []bool
	if err := p1GatherTestSharedProfile(f.s, ctx, peer, func(tx *gorm.DB, _ *pb.Player, _ *model.GameState) error {
		modes = append(modes, p1SharedRead(tx))
		return f.s.recordP1GatherEvent(tx, peer, &db.P8GatherAttempt{PlayerID: peer, Token: "catalog-after-publish"}, constUtil.P8EventMineGather, true, time.Now())
	}); err != nil {
		t.Fatal(err)
	}
	var progress db.PeriodProgress
	if len(modes) != 2 || !modes[0] || modes[1] || db.GetIns().First(&progress, "instance_id=? AND player_id=?", copy.ID, peer).Error != nil || progress.Value != 1 {
		t.Fatalf("post-publication gather missed the new frozen instance: modes=%v progress=%+v", modes, progress)
	}
	if _, err := f.s.AdminCreateActivity(ctx, principal, request); !errors.Is(err, constUtil.ErrAdminConfigConflict) {
		t.Fatalf("duplicate publication did not retain its conflict: %v", err)
	}
}
