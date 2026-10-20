package service

import (
	"bytes"
	"testing"

	"hyjs-server/db"
	"hyjs-server/model"
)

// TestAdminTreasurePresentation32 keeps persisted legacy values intact while
// showing exactly the resource the existing gameplay consumer will award.
func TestAdminTreasurePresentation32(t *testing.T) {
	form := model.AdminActivityForm{Mode: "fortune-cat", Name: "藏宝箱", Milestones: []model.AdminActivityMilestone{{Goal: 100, Rewards: []model.AdminResourceAmount{{ResourceID: "diamond", Amount: 105}}}}}
	before := adminJSON(form)
	row := db.AdminActivityDefinition{ID: "treasure", Mode: form.Mode, Name: form.Name, FormJSON: before, Revision: 3}
	version := adminVersion(row)
	dto, err := adminActivityDefinitionDTO(row)
	if err != nil || dto.Form.Milestones[0].Rewards[0].ResourceID != "coin" || dto.Form.Milestones[0].Rewards[0].Amount != 105 || dto.Form.Milestones[0].Goal != 100 {
		t.Fatalf("effective reward mismatch: %+v %v", dto, err)
	}
	if !bytes.Equal(before, row.FormJSON) || !bytes.Equal(before, adminJSON(form)) || dto.Version != version {
		t.Fatal("presentation changed input bytes or version")
	}
	// The similarly named legacy mode and ordinary cumulative consumption keep
	// their distinct reward contracts, including actual diamond rewards.
	for _, mode := range []string{"treasure-box", "diamond-spent"} {
		form.Mode = mode
		shown := adminActivityPresentationForm(form)
		if shown.Milestones[0].Rewards[0].ResourceID != "diamond" || !bytes.Equal(before, row.FormJSON) {
			t.Fatal("unrelated mode changed", mode)
		}
	}
}
