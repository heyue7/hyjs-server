package service

import (
	"bytes"
	"errors"
	"testing"

	"gorm.io/gorm"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
)

func TestGGOClientBuildRequiresCurrentSessionBinding(t *testing.T) {
	t.Setenv("HYJS_P1_MYSQL_DATABASE", "")
	f := p1Setup(t)
	if err := db.GetIns().AutoMigrate(&db.ClientUpdateSession{}); err != nil {
		t.Fatal(err)
	}
	cfg := ggoLineTestConfiguration(t)
	cfg.Runtime.MinClientBuild = 2026102904
	var row db.Profile
	if err := db.GetIns().First(&row, f.id).Error; err != nil {
		t.Fatal(err)
	}
	check := func(session []byte) error {
		return db.GetIns().Transaction(func(tx *gorm.DB) error { return ggoCheckClientBuildLocked(tx, f.id, session, cfg) })
	}
	assertUpdate := func(err error) {
		var biz *model.BizCode
		if !errors.As(err, &biz) || biz.Code != constUtil.GGOFieldClientUpdateRequired {
			t.Fatal("missing update rejection", err)
		}
	}
	assertUpdate(check(row.Session))
	binding := db.ClientUpdateSession{PlayerID: f.id, SessionHash: clientUpdateSessionHash(row.Session), BuildCode: 2026103101}
	if err := db.GetIns().Save(&binding).Error; err != nil {
		t.Fatal(err)
	}
	assertUpdate(check(row.Session))
	binding.BuildCode = 2026103102
	if err := db.GetIns().Save(&binding).Error; err != nil {
		t.Fatal(err)
	}
	if err := check(row.Session); err != nil {
		t.Fatal(err)
	}
	cfg.Runtime.MinClientBuild = 2026103103
	assertUpdate(check(row.Session))
	cfg.Runtime.MinClientBuild = 2026102904
	different := bytes.Repeat([]byte{42}, 16)
	if bytes.Equal(different, row.Session) {
		different[0]++
	}
	assertUpdate(check(different))
	cfg.Runtime.MinClientBuild = 0
	assertUpdate(check(row.Session))
	if model.ValidAdminFieldGGORuntime(cfg) {
		t.Fatal("enabled draft without minimum build accepted")
	}
	cfg.Runtime.Enabled = false
	if !model.ValidAdminFieldGGORuntime(cfg) {
		t.Fatal("disabled draft should retain unknown minimum build")
	}
}
