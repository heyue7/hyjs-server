package service

import (
	"encoding/json"
	"fmt"
	"reflect"
	"testing"
	"time"

	"gorm.io/gorm"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
)

// The audited calendar was a mixture of growth templates and saved native shop
// catalogues. Only this isolated fixture reconstructs that historical shape.
func growth75MixedPublicationFixture(t *testing.T) (*p1Fixture, *model.AdminPrincipal, []db.AdminActivityOccurrence, []db.AdminActivityOccurrence) {
	t.Helper()
	f, actor := growth75DeploymentFixture(t, 820)
	if err := db.GetIns().Model(&db.ClientUpdatePolicy{}).Where("id=1").Updates(map[string]any{"maintenance": true, "maintenance_epoch": 9, "maintenance_start_at": 123}).Error; err != nil {
		t.Fatal(err)
	}
	definitions := map[string]db.AdminActivityDefinition{}
	originals := map[string]db.P4CommerceRule{}
	for _, key := range []string{"point", "guild"} {
		source, err := f.s.AdminOrdinaryShopSource(db.GetIns(), key)
		if err != nil {
			t.Fatal("original saved shop source", key, err)
		}
		if !source.Enabled {
			shop, err := f.s.AdminShop(f.ctx, key)
			if err != nil {
				t.Fatal(err)
			}
			settings := shop.Settings
			settings.Enabled = true
			if _, err = f.s.AdminSaveShopSettings(f.ctx, actor, key, model.AdminShopSettingsSaveRequest{AdminShopSettings: settings, Version: shop.Version}); err != nil {
				t.Fatal("explicit original source enable", key, err)
			}
			if source, err = f.s.AdminOrdinaryShopSource(db.GetIns(), key); err != nil || !source.Enabled {
				t.Fatal("saved source remained unavailable", key, err)
			}
		}
		form, err := f.s.adminShopScheduleSourceForm(db.GetIns(), key)
		if err != nil || form.Source != "商店已保存目录" || form.Mode != "shop-window" || form.Options.ShopKey != key {
			t.Fatal("original shop scheduling form", key, err)
		}
		form.RepeatPolicy = "repeat"
		definition := db.AdminActivityDefinition{ID: "lineage75_" + key, Name: form.Name, Mode: "shop-window", FormJSON: adminJSON(form), DefaultDays: 7, ClaimDays: 1, Usable: true, Enabled: true, Revision: 1, UpdatedAtMillis: constUtil.Growth75SourcePublishedAt - 1}
		if err := db.GetIns().Create(&definition).Error; err != nil {
			t.Fatal(err)
		}
		definitions[key], originals[key] = definition, source
	}
	var shops []db.AdminActivityOccurrence
	for index := 0; index < 33; index++ {
		key := "point"
		if index >= 26 {
			key = "guild"
		}
		definition := definitions[key]
		var form model.AdminActivityForm
		if err := json.Unmarshal(definition.FormJSON, &form); err != nil {
			t.Fatal(err)
		}
		suffix := fmt.Sprintf("%016x%08x", index+1, 33-index)
		consumer := "shop_schedule_" + key + "_" + suffix[:16]
		source := originals[key]
		source.ID = consumer
		source.Frozen = nil
		source.StartAt = time.Now().AddDate(0, 0, index*7).UnixMilli()
		source.EndAt = source.StartAt + 7*86400000
		source.ClaimEndAt = source.EndAt + 86400000
		form.StartAt, form.EndAt, form.ClaimEndAt = source.StartAt, source.EndAt, source.ClaimEndAt
		row := db.AdminActivityOccurrence{ID: "occ_" + suffix, ServerID: 1, TemplateID: definition.ID, TemplateVersion: adminVersion(definition), Name: form.Name, Mode: "shop-window", Timing: "calendar", ConsumerKey: consumer, ConsumerID: "commerce:" + consumer, StartAt: source.StartAt, EndAt: source.EndAt, ClaimEndAt: source.ClaimEndAt, Enabled: true, FormJSON: adminJSON(form), Revision: 1}
		if err := db.GetIns().Transaction(func(tx *gorm.DB) error {
			if err := tx.Where("id=?", fmt.Sprintf("growth75-proof-%04d", index)).Delete(&db.AdminActivityOccurrence{}).Error; err != nil {
				return err
			}
			if err := tx.Create(&source).Error; err != nil {
				return err
			}
			return tx.Create(&row).Error
		}); err != nil {
			t.Fatal("actual native scheduled catalogue", key, err)
		}
		shops = append(shops, row)
	}
	archived := make([]db.AdminActivityOccurrence, 53)
	for index := range archived {
		archived[index] = db.AdminActivityOccurrence{ID: fmt.Sprintf("lineage75_archived_%02d", index), ServerID: 1, ConsumerKey: fmt.Sprintf("lineage75_archived_key_%02d", index), ConsumerID: fmt.Sprintf("period:lineage75_archived_key_%02d", index), Mode: "login-total", Archived: true, Revision: 2, FormJSON: adminJSON(model.AdminActivityForm{Mode: "login-total", Source: "成长投放V12 / archived historical calendar"})}
	}
	if err := db.GetIns().CreateInBatches(&archived, 53).Error; err != nil {
		t.Fatal(err)
	}
	if p1Count(t, &db.AdminActivityOccurrence{}, "server_id=? AND archived=?", 1, false) != 820 || p1Count(t, &db.P4CommerceRule{}, "id LIKE ?", "shop_schedule_%") != 33 {
		t.Fatal("mixed audit fixture must contain exactly820 live rows and33 real native shop rules")
	}
	return f, actor, shops, archived
}

func TestGrowth75PublicationMixedNativeShopsAppliesAndRetries(t *testing.T) {
	f, _, shops, archived := growth75MixedPublicationFixture(t)
	if err := f.s.ApplyGrowth75(f.ctx); err != nil {
		t.Fatal("mixed787+26+7 publication apply", err)
	}
	if got := p1Count(t, &db.AdminActivityOccurrence{}, "server_id=? AND archived=? AND first_published_at=?", 1, false, constUtil.Growth75SourcePublishedAt); got != 820 {
		t.Fatal("audited whole set was not restored", got)
	}
	for _, key := range []string{"point", "guild"} {
		var definition db.AdminActivityDefinition
		if err := db.GetIns().First(&definition, "id=?", "lineage75_"+key).Error; err != nil || definition.Revision <= 1 || definition.UpdatedAtMillis <= constUtil.Growth75SourcePublishedAt {
			t.Fatal("economic synchronization did not exercise legitimate newer template lineage", key, definition.Revision, definition.UpdatedAtMillis, err)
		}
	}
	for _, shop := range shops {
		var native db.P4CommerceRule
		var current db.AdminActivityOccurrence
		if err := db.GetIns().First(&native, "id=?", shop.ConsumerKey).Error; err != nil {
			t.Fatal(err)
		}
		if err := db.GetIns().First(&current, "id=?", shop.ID).Error; err != nil || current.ConsumerKey != shop.ConsumerKey || current.ConsumerID != shop.ConsumerID || current.Revision <= 1 {
			t.Fatal("economic synchronization changed native consumer identity or did not revise snapshot", err)
		}
		var form model.AdminActivityForm
		if err := json.Unmarshal(current.FormJSON, &form); err != nil || native.Kind != form.Options.ShopKey || native.ShopType != map[string]int32{"point": 5, "guild": 8}[native.Kind] {
			t.Fatal("native scheduled shop type was replaced", native.Kind, native.ShopType, err)
		}
	}
	var first []db.AdminActivityOccurrence
	if err := db.GetIns().Order("id").Find(&first).Error; err != nil {
		t.Fatal(err)
	}
	if err := f.s.ApplyGrowth75(f.ctx); err != nil {
		t.Fatal("own economic revisions rejected verified publication retry", err)
	}
	var second []db.AdminActivityOccurrence
	if err := db.GetIns().Order("id").Find(&second).Error; err != nil || !reflect.DeepEqual(first, second) {
		t.Fatal("second apply changed published/archived source snapshots", err)
	}
	for _, old := range archived {
		var current db.AdminActivityOccurrence
		if err := db.GetIns().First(&current, "id=?", old.ID).Error; err != nil || !reflect.DeepEqual(old, current) {
			t.Fatal("archived historical row entered publication or was rewritten", old.ID, err)
		}
	}
	for action, count := range map[string]int64{"growth75.activity.publication": 1, "calendar75.anchors": 1, "growth75.client.compatibility": 1} {
		if got := p1Count(t, &db.AdminAudit{}, "action=?", action); got != count {
			t.Fatal("replayed publication or release", action, got)
		}
	}
	var policy db.ClientUpdatePolicy
	if err := db.GetIns().First(&policy, 1).Error; err != nil || !policy.Maintenance || policy.MaintenanceEpoch != 9 || policy.MaintenanceStartAt != 123 {
		t.Fatal("mixed apply changed maintenance boundary", err)
	}
}

func TestGrowth75PublicationMixedLineageRejectsBeforeWrites(t *testing.T) {
	f, _, shops, _ := growth75MixedPublicationFixture(t)
	shop := shops[0]
	var definition db.AdminActivityDefinition
	if err := db.GetIns().First(&definition, "id=?", shop.TemplateID).Error; err != nil {
		t.Fatal(err)
	}
	row := func(column string, value any) func() func() {
		return func() func() {
			if err := db.GetIns().Model(&db.AdminActivityOccurrence{}).Where("id=?", shop.ID).Update(column, value).Error; err != nil {
				t.Fatal(err)
			}
			return func() {
				id := shop.ID
				if column == "id" {
					id = value.(string)
				}
				if err := db.GetIns().Where("id=?", id).Delete(&db.AdminActivityOccurrence{}).Error; err != nil {
					t.Fatal(err)
				}
				if err := db.GetIns().Create(&shop).Error; err != nil {
					t.Fatal(err)
				}
			}
		}
	}
	def := func(column string, value any) func() func() {
		return func() func() {
			if err := db.GetIns().Model(&db.AdminActivityDefinition{}).Where("id=?", definition.ID).Update(column, value).Error; err != nil {
				t.Fatal(err)
			}
			return func() {
				if err := db.GetIns().Save(&definition).Error; err != nil {
					t.Fatal(err)
				}
			}
		}
	}
	changeForm := func(raw []byte, update func(*model.AdminActivityForm)) []byte {
		var form model.AdminActivityForm
		if err := json.Unmarshal(raw, &form); err != nil {
			t.Fatal(err)
		}
		update(&form)
		return adminJSON(form)
	}
	cases := []struct {
		name   string
		change func() func()
	}{
		{"source-exact", row("form_json", changeForm(shop.FormJSON, func(f *model.AdminActivityForm) { f.Source += " duplicate" }))},
		{"row-mode", row("mode", "recharge-total")},
		{"form-mode", row("form_json", changeForm(shop.FormJSON, func(f *model.AdminActivityForm) { f.Mode = "recharge-total" }))},
		{"shop-key", row("form_json", changeForm(shop.FormJSON, func(f *model.AdminActivityForm) { f.Options.ShopKey = "score" }))},
		{"consumer-key", row("consumer_key", shop.ConsumerKey+"bad")},
		{"consumer-domain", row("consumer_id", "period:"+shop.ConsumerKey)},
		{"short-occurrence-id", row("id", "occ_x")},
		{"template-missing", row("template_id", "missing_lineage75")},
		{"definition-mode", def("mode", "recharge-total")},
		{"definition-source", def("form_json", changeForm(definition.FormJSON, func(f *model.AdminActivityForm) { f.Source = "different saved catalogue" }))},
		{"definition-shop", def("form_json", changeForm(definition.FormJSON, func(f *model.AdminActivityForm) { f.Options.ShopKey = "guild" }))},
		{"definition-unverified-revision", def("revision", int64(2))},
		{"definition-missing-time", def("updated_at_millis", int64(0))},
		{"definition-published-too-late", def("updated_at_millis", constUtil.Growth75SourcePublishedAt+1)},
		{"foreign-realm", row("server_id", int64(2))},
		{"archived-cannot-fill820", row("archived", true)},
		{"conflicting-publication", row("first_published_at", constUtil.Growth75SourcePublishedAt-1)},
		{"extra821", func() func() {
			extra := db.AdminActivityOccurrence{ID: "lineage75_extra", ServerID: 1, ConsumerKey: "lineage75_extra_key", ConsumerID: "period:lineage75_extra_key", FormJSON: adminJSON(model.AdminActivityForm{Source: constUtil.Growth75SourceLabel})}
			if err := db.GetIns().Create(&extra).Error; err != nil {
				t.Fatal(err)
			}
			return func() {
				if err := db.GetIns().Where("id=?", extra.ID).Delete(&db.AdminActivityOccurrence{}).Error; err != nil {
					t.Fatal(err)
				}
			}
		}},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			restore := test.change()
			defer restore()
			if err := f.s.ApplyGrowth75(f.ctx); err == nil {
				t.Fatal("unverified mixed publication was applied")
			}
			if got := p1Count(t, &db.AdminActivityOccurrence{}, "first_published_at=?", constUtil.Growth75SourcePublishedAt); got != 0 {
				t.Fatal("rejected lineage wrote a publication floor", got)
			}
			if p1Count(t, &db.AdminAudit{}, "action=?", "growth75.activity.publication") != 0 || p1Count(t, &db.ClientUpdateRelease{}, "build_code=?", constUtil.Growth75Build) != 0 {
				t.Fatal("rejected lineage published an audit or release")
			}
		})
	}
	if err := f.s.growth75VerifyPublication(f.ctx); err != nil {
		t.Fatal("negative fixture was not restored to the original valid mixed set", err)
	}
}

func TestGrowth75PublicationMixedRetryRequiresCompletedBackfillReceipt(t *testing.T) {
	f, actor, _, _ := growth75MixedPublicationFixture(t)
	if err := f.s.BackfillGrowth75PublishedAt(f.ctx, actor); err != nil {
		t.Fatal(err)
	}
	if err := f.s.ApplyGrowth75Economy(f.ctx, actor); err != nil {
		t.Fatal(err)
	}
	if err := f.s.growth75VerifyPublication(f.ctx); err != nil {
		t.Fatal("economic revision has a completed audited publication receipt", err)
	}
	var audit db.AdminAudit
	if err := db.GetIns().Where("action=?", "growth75.activity.publication").First(&audit).Error; err != nil {
		t.Fatal(err)
	}
	for _, wrong := range []string{"restored=819; sourceAudit=321; realPublishedAt=1793026861377;", "restored=820; sourceAudit=320; realPublishedAt=1793026861377;", "restored=820; sourceAudit=321; realPublishedAt=1793026861376;"} {
		if err := db.GetIns().Model(&db.AdminAudit{}).Where("id=?", audit.ID).Update("detail", wrong).Error; err != nil {
			t.Fatal(err)
		}
		if err := f.s.growth75VerifyPublication(f.ctx); err == nil {
			t.Fatal("newer template bypassed original publication with an incomplete receipt", wrong)
		}
	}
	if err := db.GetIns().Save(&audit).Error; err != nil {
		t.Fatal(err)
	}
	if err := f.s.ApplyGrowth75(f.ctx); err != nil {
		t.Fatal("actual completed backfill receipt did not permit a resumable apply", err)
	}
	if n := p1Count(t, &db.AdminAudit{}, "action=?", "growth75.activity.publication"); n != 1 {
		t.Fatal("retry regenerated publication receipt", n)
	}
}
