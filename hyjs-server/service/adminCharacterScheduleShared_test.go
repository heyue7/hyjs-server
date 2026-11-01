package service

import (
	"context"
	"errors"
	"fmt"
	"os"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
)

type characterScheduleProbeKey struct{}

type characterScheduleProbe struct {
	mu          sync.Mutex
	modes       []bool
	ownerWrites []bool
	entered     chan<- struct{}
	release     <-chan struct{}
	blockOnce   sync.Once
}

func (p *characterScheduleProbe) snapshot() ([]bool, []bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([]bool(nil), p.modes...), append([]bool(nil), p.ownerWrites...)
}

// Observe the real GORM transactions after they have acquired their schedule
// rows. The barrier holds their existing database locks, not a synthetic mutex.
func installCharacterScheduleProbe(t *testing.T) {
	t.Helper()
	database := db.GetIns()
	queryName := "test:character_schedule_shared_query"
	createName := "test:character_schedule_shared_owner"
	if err := database.Callback().Query().After("gorm:query").Register(queryName, func(tx *gorm.DB) {
		probe, _ := tx.Statement.Context.Value(characterScheduleProbeKey{}).(*characterScheduleProbe)
		if probe == nil || tx.Error != nil || tx.Statement.Table != "admin_activity_occurrences" || !strings.Contains(strings.ToLower(tx.Statement.SQL.String()), "timing") {
			return
		}
		if _, ok := tx.Statement.Dest.(*[]db.AdminActivityOccurrence); !ok {
			return
		}
		shared := p1SharedRead(tx)
		probe.mu.Lock()
		probe.modes = append(probe.modes, shared)
		probe.mu.Unlock()
		if shared && probe.entered != nil && probe.release != nil {
			probe.blockOnce.Do(func() {
				probe.entered <- struct{}{}
				select {
				case <-probe.release:
				case <-tx.Statement.Context.Done():
				}
			})
		}
	}); err != nil {
		t.Fatal(err)
	}
	if err := database.Callback().Create().After("gorm:create").Register(createName, func(tx *gorm.DB) {
		probe, _ := tx.Statement.Context.Value(characterScheduleProbeKey{}).(*characterScheduleProbe)
		if probe == nil || tx.Error != nil || tx.RowsAffected != 1 || tx.Statement.Table != "admin_activity_template_owners" {
			return
		}
		probe.mu.Lock()
		probe.ownerWrites = append(probe.ownerWrites, p1SharedRead(tx))
		probe.mu.Unlock()
	}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = database.Callback().Query().Remove(queryName)
		_ = database.Callback().Create().Remove(createName)
	})
}

func characterSchedulePublish(t *testing.T, f *p1Fixture, actor *model.AdminPrincipal, mode, policy string, startDay, days int64) model.AdminActivityOccurrenceDTO {
	t.Helper()
	form := adminModeForm(mode)
	form.Name = "共享锁测试-" + mode
	form.RepeatPolicy = policy
	form.Milestones = []model.AdminActivityMilestone{{Goal: 2, Rewards: []model.AdminResourceAmount{{ResourceID: "coin", Amount: 7}}}}
	definition, err := f.s.AdminSaveActivityDefinition(f.ctx, actor, "", model.AdminActivityDefinitionSaveRequest{Name: form.Name, Form: form, DefaultDays: days, Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	return reusePublish(t, f, actor, *definition, startDay, days, "character-day")
}

func characterScheduleIssued(t *testing.T, id int64, occurrenceID string) db.AdminCharacterActivityInstance {
	t.Helper()
	var issued db.AdminCharacterActivityInstance
	if err := db.GetIns().First(&issued, "player_id=? AND occurrence_id=?", id, occurrenceID).Error; err != nil {
		t.Fatal(err)
	}
	return issued
}

func TestAdminCharacterScheduleSharedSQLite(t *testing.T) {
	t.Setenv("HYJS_P4_MYSQL_DATABASE", "")
	f, actor := reuseFixture(t, false)
	occurrence := characterSchedulePublish(t, f, actor, "login-total", "repeat", 1, 14)
	installCharacterScheduleProbe(t)
	ctx := serverClockBindTest(t, f, f.id)
	probe := &characterScheduleProbe{}
	ctx = context.WithValue(ctx, characterScheduleProbeKey{}, probe)
	factsBefore := p1Count(t, &db.EventFact{}, "player_id=?", f.id)
	if err := f.s.PrepareAdminCharacterActivitySchedules(ctx, f.id); err != nil {
		t.Fatal(err)
	}
	modes, writes := probe.snapshot()
	if !reflect.DeepEqual(modes, []bool{true}) || !reflect.DeepEqual(writes, []bool{true}) {
		t.Fatalf("new personal consumer should use one owner transaction: modes=%v owner writes=%v", modes, writes)
	}
	issued := characterScheduleIssued(t, f.id, occurrence.ID)
	if issued.Mode != "login-total" || p1Count(t, &db.PeriodInstance{}, "id=?", issued.ConsumerKey) != 1 ||
		p1Count(t, &db.AdminActivityTemplateOwner{}, "server_id=? AND player_id=? AND template_id=?", f.s.OriginalServerID(), f.id, occurrence.TemplateID) != 1 ||
		p1Count(t, &db.EventFact{}, "player_id=?", f.id) != factsBefore {
		t.Fatal("retry changed owner, consumer, or facts")
	}
	var owner db.AdminActivityTemplateOwner
	if err := db.GetIns().First(&owner, "server_id=? AND player_id=? AND template_id=?", f.s.OriginalServerID(), f.id, occurrence.TemplateID).Error; err != nil || owner.OccurrenceID != occurrence.ID {
		t.Fatalf("first-owner identity changed: %+v %v", owner, err)
	}
	probe = &characterScheduleProbe{}
	ctx = context.WithValue(ctx, characterScheduleProbeKey{}, probe)
	if err := f.s.PrepareAdminCharacterActivitySchedules(ctx, f.id); err != nil {
		t.Fatal(err)
	}
	modes, writes = probe.snapshot()
	if !reflect.DeepEqual(modes, []bool{true}) || len(writes) != 0 || !reflect.DeepEqual(issued, characterScheduleIssued(t, f.id, occurrence.ID)) {
		t.Fatalf("issued consumer should remain unchanged in one shared attempt: modes=%v owner writes=%v", modes, writes)
	}
	// A pre-policy issued consumer can legitimately lack its owner row. The
	// original historical recovery remains an allowed per-player shared write.
	if err := db.GetIns().Where("server_id=? AND player_id=? AND template_id=?", f.s.OriginalServerID(), f.id, occurrence.TemplateID).Delete(&db.AdminActivityTemplateOwner{}).Error; err != nil {
		t.Fatal(err)
	}
	probe = &characterScheduleProbe{}
	ctx = context.WithValue(ctx, characterScheduleProbeKey{}, probe)
	if err := f.s.PrepareAdminCharacterActivitySchedules(ctx, f.id); err != nil {
		t.Fatal(err)
	}
	modes, writes = probe.snapshot()
	if !reflect.DeepEqual(modes, []bool{true}) || !reflect.DeepEqual(writes, []bool{true}) ||
		p1Count(t, &db.AdminActivityTemplateOwner{}, "server_id=? AND player_id=? AND template_id=? AND occurrence_id=?", f.s.OriginalServerID(), f.id, occurrence.TemplateID, occurrence.ID) != 1 ||
		!reflect.DeepEqual(issued, characterScheduleIssued(t, f.id, occurrence.ID)) {
		t.Fatalf("historical owner backfill changed issued rules: modes=%v owner writes=%v", modes, writes)
	}
	wrongOwner := context.WithValue(ctx, constUtil.ServerGameClockOwnerKey, f.id+1)
	if err := f.s.PrepareAdminCharacterActivitySchedules(wrongOwner, f.id); !errors.Is(err, constUtil.ErrAdminInput) {
		t.Fatalf("wrong calendar owner accepted: %v", err)
	}

	// A disabled schedule is skipped, and a later re-enable retains the issued snapshot.
	reuseToggle(t, f, actor, occurrence.ID, false)
	if err := f.s.PrepareAdminCharacterActivitySchedules(ctx, f.id); err != nil {
		t.Fatal(err)
	}
	reuseToggle(t, f, actor, occurrence.ID, true)
	if err := f.s.PrepareAdminCharacterActivitySchedules(ctx, f.id); err != nil || !reflect.DeepEqual(issued, characterScheduleIssued(t, f.id, occurrence.ID)) {
		t.Fatalf("schedule re-enable changed an issued snapshot: %v", err)
	}
	if err := db.GetIns().Model(&db.AdminActivityOccurrence{}).Where("id=?", occurrence.ID).Update("start_day", 367).Error; err != nil {
		t.Fatal(err)
	}
	if err := f.s.PrepareAdminCharacterActivitySchedules(ctx, f.id); !errors.Is(err, constUtil.ErrAdminConfiguration) {
		t.Fatalf("invalid schedule day accepted: %v", err)
	}
	if err := db.GetIns().Model(&db.AdminActivityOccurrence{}).Where("id=?", occurrence.ID).Update("start_day", 1).Error; err != nil {
		t.Fatal(err)
	}
	stale := ctx
	reuseClockDay(t, f, actor)
	if err := f.s.PrepareAdminCharacterActivitySchedules(stale, f.id); !errors.Is(err, constUtil.ErrAdminConfigConflict) {
		t.Fatalf("stale server clock accepted: %v", err)
	}
}

func TestAdminCharacterScheduleSharedOnceEligibilitySQLite(t *testing.T) {
	t.Setenv("HYJS_P4_MYSQL_DATABASE", "")
	f, actor := reuseFixture(t, false)
	form := adminModeForm("login-rewards")
	form.Name = "共享锁单次资格"
	form.RepeatPolicy = "once"
	form.Milestones = []model.AdminActivityMilestone{{Goal: 1, Rewards: []model.AdminResourceAmount{{ResourceID: "coin", Amount: 7}}}}
	definition, err := f.s.AdminSaveActivityDefinition(f.ctx, actor, "", model.AdminActivityDefinitionSaveRequest{Name: form.Name, Form: form, DefaultDays: 1, Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	first := reusePublish(t, f, actor, *definition, 1, 1, "character-day")
	second := reusePublish(t, f, actor, *definition, 2, 1, "character-day")
	ctx := serverClockBindTest(t, f, f.id)
	if err := f.s.PrepareAdminCharacterActivitySchedules(ctx, f.id); err != nil {
		t.Fatal(err)
	}
	issued := characterScheduleIssued(t, f.id, first.ID)
	reuseClockDay(t, f, actor)
	ctx = serverClockBindTest(t, f, f.id)
	installCharacterScheduleProbe(t)
	probe := &characterScheduleProbe{}
	ctx = context.WithValue(ctx, characterScheduleProbeKey{}, probe)
	if err := f.s.PrepareAdminCharacterActivitySchedules(ctx, f.id); err != nil {
		t.Fatal(err)
	}
	modes, writes := probe.snapshot()
	if !reflect.DeepEqual(modes, []bool{true}) || len(writes) != 0 ||
		p1Count(t, &db.AdminCharacterActivityInstance{}, "player_id=? AND occurrence_id=?", f.id, second.ID) != 0 ||
		!reflect.DeepEqual(issued, characterScheduleIssued(t, f.id, first.ID)) {
		t.Fatalf("once-only owner gained a second consumer: modes=%v owner writes=%v", modes, writes)
	}
	var owner db.AdminActivityTemplateOwner
	if err := db.GetIns().First(&owner, "server_id=? AND player_id=? AND template_id=?", f.s.OriginalServerID(), f.id, definition.ID).Error; err != nil || owner.OccurrenceID != first.ID {
		t.Fatalf("once-only first owner changed: %+v %v", owner, err)
	}
}

// Run only with a fresh, allocated P4-prefixed MySQL schema. SQLite cannot
// exercise InnoDB shared-lock compatibility or concurrent publisher ordering.
func TestAdminCharacterScheduleSharedMySQL(t *testing.T) {
	if os.Getenv("HYJS_P4_MYSQL_DATABASE") == "" {
		t.Skip("requires a fresh isolated hyjs_test_p4_ MySQL database")
	}
	f, actor := reuseFixture(t, false)
	database := db.GetIns()
	sqlDB, err := database.DB()
	if err != nil {
		t.Fatal(err)
	}
	sqlDB.SetMaxOpenConns(8)
	sqlDB.SetMaxIdleConns(8)
	other, err := LoginAccount(f.ctx, fmt.Sprintf("schedule-shared-%d", time.Now().UnixNano()), "isolated-password")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = CreatePlayer(f.ctx, other.Id, "共享日程乙", "hero_043"); err != nil {
		t.Fatal(err)
	}
	ids := []int64{f.id, other.Id}
	contexts := make(map[int64]context.Context, len(ids))
	for _, id := range ids {
		contexts[id] = serverClockBindTest(t, f, id)
	}
	occurrence := characterSchedulePublish(t, f, actor, "login-total", "repeat", 1, 14)
	for _, id := range ids {
		if err := f.s.PrepareAdminCharacterActivitySchedules(contexts[id], id); err != nil {
			t.Fatal(err)
		}
	}
	before := map[int64]db.AdminCharacterActivityInstance{}
	for _, id := range ids {
		before[id] = characterScheduleIssued(t, id, occurrence.ID)
	}
	installCharacterScheduleProbe(t)
	calendar, err := f.s.AdminActivitySchedule(f.ctx, actor, 1)
	if err != nil {
		t.Fatal(err)
	}
	for i := range calendar.Items {
		if calendar.Items[i].ID == occurrence.ID {
			calendar.Items[i].Enabled = false
			calendar.Items[i].Lane++
		}
	}
	ctx, cancel := context.WithTimeout(f.ctx, 15*time.Second)
	defer cancel()
	entered := make(chan struct{}, len(ids))
	release := make(chan struct{})
	var releaseOnce sync.Once
	defer releaseOnce.Do(func() { close(release) })
	finished := make(chan error, len(ids))
	probes := make([]*characterScheduleProbe, 0, len(ids))
	for _, id := range ids {
		id := id
		probe := &characterScheduleProbe{entered: entered, release: release}
		probes = append(probes, probe)
		request := context.WithValue(contexts[id], characterScheduleProbeKey{}, probe)
		request, requestCancel := context.WithTimeout(request, 15*time.Second)
		defer requestCancel()
		go func() { finished <- f.s.PrepareAdminCharacterActivitySchedules(request, id) }()
	}
	for range ids {
		select {
		case <-entered:
		case e := <-finished:
			t.Fatalf("a shared request finished before both entered: %v", e)
		case <-ctx.Done():
			t.Fatal("two roles could not hold shared P1 and schedule locks concurrently")
		}
	}
	adminStarted := make(chan struct{})
	adminFinished := make(chan error, 1)
	go func() {
		close(adminStarted)
		_, e := f.s.AdminSaveActivitySchedule(ctx, actor, 1, model.AdminActivityScheduleSaveRequest{Version: calendar.Version, Items: calendar.Items})
		adminFinished <- e
	}()
	<-adminStarted
	select {
	case e := <-adminFinished:
		t.Fatalf("admin disabled or modified the schedule during shared preparation: %v", e)
	case <-time.After(250 * time.Millisecond):
	}
	releaseOnce.Do(func() { close(release) })
	for range ids {
		select {
		case e := <-finished:
			if e != nil {
				t.Fatalf("shared role preparation failed: %v", e)
			}
		case <-ctx.Done():
			t.Fatal("shared preparation did not finish")
		}
	}
	select {
	case e := <-adminFinished:
		if e != nil {
			t.Fatalf("admin schedule update failed after release: %v", e)
		}
	case <-ctx.Done():
		t.Fatal("admin schedule update remained blocked")
	}
	for i, id := range ids {
		modes, writes := probes[i].snapshot()
		if !reflect.DeepEqual(modes, []bool{true}) || len(writes) != 0 || !reflect.DeepEqual(before[id], characterScheduleIssued(t, id, occurrence.ID)) {
			t.Fatalf("role %d existing consumer changed: modes=%v owner writes=%v", id, modes, writes)
		}
	}
	var disabled db.AdminActivityOccurrence
	if err := database.First(&disabled, "id=?", occurrence.ID).Error; err != nil || disabled.Enabled || disabled.Lane != occurrence.Lane+1 {
		t.Fatalf("admin disable/modify not committed: %+v %v", disabled, err)
	}
	reuseToggle(t, f, actor, occurrence.ID, true)

	// A second eligible period and two same-player requests exercise the actual
	// rollback/retry boundary and the profile's existing per-player serialization.
	second := characterSchedulePublish(t, f, actor, "recruit-count", "repeat", 1, 14)
	start := make(chan struct{})
	sameDone := make(chan error, 2)
	secondProbes := []*characterScheduleProbe{{}, {}}
	for _, probe := range secondProbes {
		request := context.WithValue(contexts[f.id], characterScheduleProbeKey{}, probe)
		go func() {
			<-start
			sameDone <- f.s.PrepareAdminCharacterActivitySchedules(request, f.id)
		}()
	}
	close(start)
	for range secondProbes {
		select {
		case e := <-sameDone:
			if e != nil {
				t.Fatalf("same-player concurrent preparation failed: %v", e)
			}
		case <-ctx.Done():
			t.Fatal("same-player preparations did not finish")
		}
	}
	var sharedWrites, exclusiveWrites int
	for _, probe := range secondProbes {
		_, writes := probe.snapshot()
		for _, shared := range writes {
			if shared {
				sharedWrites++
			} else {
				exclusiveWrites++
			}
		}
	}
	issued := characterScheduleIssued(t, f.id, second.ID)
	if sharedWrites != 1 || exclusiveWrites != 0 || p1Count(t, &db.AdminCharacterActivityInstance{}, "player_id=? AND occurrence_id=?", f.id, second.ID) != 1 ||
		p1Count(t, &db.AdminActivityTemplateOwner{}, "server_id=? AND player_id=? AND template_id=?", f.s.OriginalServerID(), f.id, second.TemplateID) != 1 ||
		p1Count(t, &db.PeriodInstance{}, "id=?", issued.ConsumerKey) != 1 {
		t.Fatalf("new consumer retry was not atomic: shared owner writes=%d exclusive owner writes=%d", sharedWrites, exclusiveWrites)
	}
	var owner db.AdminActivityTemplateOwner
	if err := database.First(&owner, "server_id=? AND player_id=? AND template_id=?", f.s.OriginalServerID(), f.id, second.TemplateID).Error; err != nil || owner.OccurrenceID != second.ID {
		t.Fatalf("new consumer owner mismatch: %+v %v", owner, err)
	}
	if p1Count(t, &db.EventFact{}, "player_id=? AND kind=?", f.id, constUtil.P1EventRecruit) != 0 {
		t.Fatal("schedule creation fabricated a gameplay fact")
	}
	for attempt := 0; attempt < 2; attempt++ {
		if err := database.WithContext(contexts[f.id]).Transaction(func(tx *gorm.DB) error {
			var profile db.Profile
			if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&profile, f.id).Error; err != nil {
				return err
			}
			return f.s.RecordP1Event(tx, f.id, "schedule-shared-real-recruit", constUtil.P1EventRecruit, 1, LocalPlayNow(tx.Statement.Context))
		}); err != nil {
			t.Fatalf("original fact attempt %d: %v", attempt, err)
		}
	}
	var progress db.PeriodProgress
	if err := database.First(&progress, "instance_id=? AND player_id=?", issued.ConsumerKey, f.id).Error; err != nil || progress.Value != 1 ||
		p1Count(t, &db.EventFact{}, "player_id=? AND kind=? AND source_key=?", f.id, constUtil.P1EventRecruit, "schedule-shared-real-recruit") != 1 {
		t.Fatalf("issued consumer lost fact/progress idempotence: %+v %v", progress, err)
	}
}
