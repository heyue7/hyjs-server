package service

import (
	"errors"
	"fmt"
	"os"
	"sync"
	"testing"

	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
)

// The real InnoDB path must accept one unchanged row and serialize edits based
// on the same version. SQLite cannot demonstrate either MySQL behavior.
func TestRecruitRuntimeMySQL33SingleFieldAndConcurrentVersion(t *testing.T) {
	if os.Getenv("HYJS_P1_MYSQL_DATABASE") == "" {
		t.Skip("isolated MySQL allocation required")
	}
	f := p1Setup(t)
	if err := f.s.InitializeGameplayPools(f.ctx); err != nil {
		t.Fatal(err)
	}
	if err := f.s.InitializeP2(f.ctx); err != nil {
		t.Fatal(err)
	}
	if err := f.s.InitializeRecruitRuntime(f.ctx); err != nil {
		t.Fatal(err)
	}
	if err := AdminInitialize(f.ctx, ""); err != nil {
		t.Fatal(err)
	}
	operator := db.AdminUser{Username: fmt.Sprintf("mysql-recruit33-%d", f.id), PasswordHash: "unused-test", Role: constUtil.AdminRoleOperator, Enabled: true}
	if err := db.GetIns().Create(&operator).Error; err != nil {
		t.Fatal(err)
	}
	actor := &model.AdminPrincipal{UserID: operator.ID, Username: operator.Username, Role: operator.Role}
	current, err := f.s.AdminRecruitSettings(f.ctx, actor)
	if err != nil {
		t.Fatal(err)
	}
	items := append([]model.AdminRecruitSetting(nil), current.Items...)
	items[1].DailyLimit = current.Items[1].DailyLimit + 1
	saved, err := f.s.AdminSaveRecruitSettings(f.ctx, actor, model.AdminRecruitSettingsSaveRequest{Version: current.Version, Reason: "single field real MySQL", Items: items})
	if err != nil {
		t.Fatal("single field update with unchanged orange row", err)
	}
	if saved.Items[1].DailyLimit != items[1].DailyLimit {
		t.Fatal("daily limit not persisted", saved)
	}
	var initialAudits int64
	if err := db.GetIns().Model(&db.AdminAudit{}).Where("action=? AND target=?", constUtil.RecruitRuntimeAuditAction, constUtil.RecruitRuntimeAuditTarget).Count(&initialAudits).Error; err != nil {
		t.Fatal(err)
	}
	start := make(chan struct{})
	failures := make(chan error, 8)
	var workers sync.WaitGroup
	for i := 0; i < 8; i++ {
		workers.Add(1)
		go func(index int) {
			defer workers.Done()
			<-start
			next := append([]model.AdminRecruitSetting(nil), saved.Items...)
			next[1].DailyLimit = saved.Items[1].DailyLimit + int64(index+1)
			_, callErr := f.s.AdminSaveRecruitSettings(f.ctx, actor, model.AdminRecruitSettingsSaveRequest{Version: saved.Version, Reason: "race same version", Items: next})
			failures <- callErr
		}(i)
	}
	close(start)
	workers.Wait()
	close(failures)
	success, conflicts := 0, 0
	for callErr := range failures {
		if callErr == nil {
			success++
		} else if errors.Is(callErr, constUtil.ErrAdminConfigConflict) {
			conflicts++
		} else {
			t.Errorf("unexpected concurrent error: %v", callErr)
		}
	}
	if success != 1 || conflicts != 7 {
		t.Fatalf("same version overwrote edits: success=%d conflict=%d", success, conflicts)
	}
	var count int64
	if err := db.GetIns().Model(&db.AdminAudit{}).Where("action=? AND target=?", constUtil.RecruitRuntimeAuditAction, constUtil.RecruitRuntimeAuditTarget).Count(&count).Error; err != nil || count != initialAudits+1 {
		t.Fatal("committed settings audit count", count, err)
	}
}
