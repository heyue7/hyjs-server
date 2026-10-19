package service

import (
	"hyjs-server/model"
	"testing"
)

func TestAdminActivityCostCataloguePaginationAndLegalFacets(t *testing.T) {
	f, actor := adminModesFixture(t)
	all, err := f.s.AdminActivityCostResources(f.ctx, actor, "", model.AdminItemsRequest{Page: 1, PageSize: 1 << 30})
	if err != nil {
		t.Fatal(err)
	}
	if all.Total <= 100 || int64(len(all.Items)) != all.Total {
		t.Fatal("legal catalogue was truncated before pagination", all.Total, len(all.Items))
	}
	page, err := f.s.AdminActivityCostResources(f.ctx, actor, "", model.AdminItemsRequest{Page: 2, PageSize: 3})
	if err != nil {
		t.Fatal(err)
	}
	if page.Total != all.Total || len(page.Items) != 3 {
		t.Fatal("legal cost total/page incorrect", page.Total, len(page.Items))
	}
	for i, item := range page.Items {
		if item.ID != all.Items[i+3].ID {
			t.Fatal("currency-prioritized order changed across pages")
		}
		if _, err = f.s.p4ParseCost(item.ID + ",1"); err != nil {
			t.Fatal("unsupported resource entered cost selector", item.ID)
		}
	}
	selected, err := f.s.AdminActivityCostResources(f.ctx, actor, "", model.AdminItemsRequest{Category: "currency", IDs: []string{"coin", "diamond", "hero_043"}, Page: 2, PageSize: 1})
	if err != nil {
		t.Fatal(err)
	}
	if selected.Total != 2 || len(selected.Items) != 1 || selected.Items[0].ID != "diamond" {
		t.Fatal("exact IDs/category were applied after page cut", selected)
	}
	if len(selected.Facets.Categories) == 0 || len(selected.Facets.Qualities) == 0 {
		t.Fatal("legal-cost facets missing")
	}
	unsupported, err := f.s.AdminActivityCostResources(f.ctx, actor, "", model.AdminItemsRequest{IDs: []string{"hero_043"}, Page: 1, PageSize: 25})
	if err != nil || unsupported.Total != 0 || len(unsupported.Items) != 0 {
		t.Fatal("non-cost character resource was offered", unsupported, err)
	}
}
