package service

import (
	"hyjs-server/constUtil"
	"hyjs-server/model"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Verify every original title's exported art and the readable target after an actual saved override.
func TestAdminOriginalTitleArtworkAndReadableTargets(t *testing.T) {
	adminWalletTestEnvironment(t)
	f := p1Setup(t)
	if err := f.s.LoadP5Tables("../../analysis/tables"); err != nil {
		t.Fatal(err)
	}
	if err := InitializeAdminGameContent(f.ctx); err != nil {
		t.Fatal(err)
	}
	if err := f.s.LoadAdminCatalogue("../../analysis/tables", "../../admin-web/public/media/content-manifest.json"); err != nil {
		t.Fatal(err)
	}
	actor := adminWalletTestActor(t, f, constUtil.AdminRoleOperator)
	list, err := f.s.AdminTitles(f.ctx, actor)
	if err != nil {
		t.Fatal(err)
	}
	if len(list.Items) != 71 || len(list.Chapters) == 0 {
		t.Fatal("original title/chapters missing")
	}
	var target model.AdminTitleDTO
	for _, row := range list.Items {
		if !strings.HasPrefix(row.IconURL, "/admin/media/items/title_") || row.Requirement == "" {
			t.Fatal("missing original art or qualification", row.ID)
		}
		path := filepath.Join("../../admin-web/public", strings.TrimPrefix(row.IconURL, "/admin/"))
		if _, err := os.Stat(path); err != nil {
			t.Fatal(err)
		}
		if row.Type == 1 {
			target = row
		}
	}
	target.Param = "3"
	saved, err := f.s.AdminSaveTitle(f.ctx, actor, target.ID, target)
	if err != nil {
		t.Fatal(err)
	}
	if saved.Requirement != "同时上阵 3 名角色" || saved.IconURL != target.IconURL {
		t.Fatal("current target label or original art changed", saved)
	}
	if _, err = f.s.AdminSaveTitle(f.ctx, actor, target.ID, target); err == nil {
		t.Fatal("stale edit accepted")
	}
}
