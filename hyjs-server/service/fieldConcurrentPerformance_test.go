package service

import (
	"reflect"
	"sync"
	"testing"

	"hyjs-server/model"
)

// Concurrent first readers reuse one published snapshot instead of allocating
// identical map tables for every login in the same burst.
func TestAdminFieldRuntimeConcurrentColdReaders(t *testing.T) {
	f, actor := adminFieldRuntimeTestSetup(t)
	catalog, err := f.s.AdminFields(f.ctx, actor)
	if err != nil {
		t.Fatal(err)
	}
	var template model.AdminFieldTemplateDTO
	for _, row := range catalog.Items {
		if row.Configuration.Scene == "field_001" {
			template = row
		}
	}
	if template.ID == "" {
		t.Fatal("forest template missing")
	}
	if _, err := f.s.AdminDeployField(f.ctx, actor, model.AdminFieldDeployRequest{ServerID: 1, Scene: "field_001", TemplateID: template.ID, TemplateVersion: template.Version, Enabled: true}); err != nil {
		t.Fatal(err)
	}
	f.s.adminFieldRuntimeCache.Store(nil)
	start := make(chan struct{})
	results := make(chan uintptr, 32)
	var wg sync.WaitGroup
	for i := 0; i < cap(results); i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			rows, err := f.s.AdminFieldRuntimeTables(f.ctx, 1)
			if err != nil {
				t.Errorf("concurrent cold reader: %v", err)
				return
			}
			if field := adminFieldRuntimeTestRow(rows, "Field", "FieldId", "field_001"); field["adminEnabled"] != "true" {
				t.Error("concurrent reader observed an incomplete field publication")
				return
			}
			results <- reflect.ValueOf(rows).Pointer()
		}()
	}
	close(start)
	wg.Wait()
	close(results)
	if len(results) != cap(results) {
		t.Fatal("cold reader did not complete")
	}
	var first uintptr
	for pointer := range results {
		if first == 0 {
			first = pointer
		}
		if pointer != first {
			t.Fatal("identical concurrent cold readers constructed separate snapshots")
		}
	}
}
