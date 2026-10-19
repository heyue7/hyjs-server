package service

import (
	"errors"
	"google.golang.org/protobuf/proto"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
	"strings"
	"testing"
)

func TestKingRecruitParallelWorldsNativeSelectionAndCold(t *testing.T) {
	f, actor, base := adminKingFixture(t)
	if err := f.s.InitializeKingRecruitWorlds(f.ctx); err != nil {
		t.Fatal(err)
	}
	session := []byte("king-world-test1")
	if err := db.GetIns().Model(&db.Profile{}).Where("id=?", f.id).Update("session", session).Error; err != nil {
		t.Fatal(err)
	}
	slots := []model.AdminActivityOccurrenceDTO{}
	for world := int64(1); world <= 3; world++ {
		form := base
		form.Options.KingWorld = world
		form.Prizes = []model.AdminActivityPrize{base.Prizes[world-1]}
		form.Name = []string{"SAO王者", "ALO王者", "GGO王者"}[world-1]
		definition := adminKingDefinition(t, f, actor, form)
		slots = append(slots, reusePublish(t, f, actor, definition, 1, 1, "calendar"))
	}
	bound := serverClockBindTest(t, f, f.id)
	state, err := f.s.KingRecruitWorlds(bound, f.id, session, "", "")
	if err != nil || len(state.Worlds) != 3 {
		t.Fatal("three simultaneous native worlds", state, err)
	}
	before, _ := f.profile(t)
	for index, slot := range slots {
		_, key, _ := strings.Cut(slot.ConsumerID, ":")
		state, err = f.s.KingRecruitWorlds(bound, f.id, session, key, state.Version)
		if err != nil || state.SelectedPoolID != key {
			t.Fatal("world select", err)
		}
		info := adminKingInfo(t, f, bound)
		if info.GetSpace() != int32(index+1) || len(info.GetRewardList()) != 1 || info.GetRewardList()[0] != base.Prizes[index].ResourceID {
			t.Fatal("native selected header", info)
		}
		preview, err := f.s.kingRecruitPreview(bound, f.id, &pb.CSWarriorRecruitGetPreview{CostType: proto.Int32(3)})
		if err != nil || preview[0].(*pb.SCWarriorRecruitGetPreview).GetRewards()[0] != info.GetRewardList()[0] {
			t.Fatal("world preview", err)
		}
		paid, err := f.s.warriorRecruit(bound, f.id, p2Recruit(1, 0), 97001+int64(index))
		if err != nil || paid[0].(*pb.SCWarriorRecruit).GetGet().GetWars()[0].GetId() != info.GetRewardList()[0] {
			t.Fatal("world actual draw", err)
		}
		retry, err := f.s.warriorRecruit(bound, f.id, p2Recruit(1, 0), 97001+int64(index))
		if err != nil || !proto.Equal(retry[0], paid[0]) {
			t.Fatal("world native retry", err)
		}
	}
	after, _ := f.profile(t)
	if before.GetDiamond()-after.GetDiamond() != 900 {
		t.Fatal("world switching changed price or duplicated retry")
	}
	info := adminKingInfo(t, f, bound)
	if info.GetLuck() != 30 || info.GetLeftTimes() != 47 {
		t.Fatal("world switching reset global counters", info)
	}
	if _, err = f.s.KingRecruitWorlds(bound, f.id, session, state.SelectedPoolID, "stale"); !errors.Is(err, constUtil.ErrAdminConfigConflict) {
		t.Fatal("stale world catalogue accepted", err)
	}
	if _, err = f.s.KingRecruitWorlds(bound, f.id, []byte("invalid-session!"), "", ""); err == nil {
		t.Fatal("foreign session selected world")
	}
	// Closing the selected pool cannot silently spend in another still-open world.
	reuseToggle(t, f, actor, slots[2].ID, false)
	if _, err = f.s.warriorRecruit(bound, f.id, p2Recruit(1, 0), 97004); !errors.Is(err, constUtil.ErrKingRecruitActivityClosed) {
		t.Fatal("stale selected world fell through", err)
	}
	if _, err = f.s.warriorRecruit(bound, f.id, p2Recruit(1, 0), 97003); err != nil {
		t.Fatal("closed world lost committed native receipt", err)
	}
	if _, err = (&GameService{ServerID: 2, Tables: f.s.Tables}).KingRecruitWorlds(bound, f.id, session, "", ""); !errors.Is(err, constUtil.ErrAdminPermission) {
		t.Fatal("cross-realm session accepted", err)
	}
	state, err = f.s.KingRecruitWorlds(bound, f.id, session, "", "")
	if err != nil || len(state.Worlds) != 2 || state.SelectedPoolID == "" {
		t.Fatal("remaining open worlds inaccessible", err)
	}
	f.restart(t)
	bound = serverClockBindTest(t, f, f.id)
	state, err = f.s.KingRecruitWorlds(bound, f.id, session, "", "")
	if err != nil || len(state.Worlds) != 2 || adminKingInfo(t, f, bound).GetLuck() != 30 {
		t.Fatal("cold selection/counter", err)
	}
}

func TestKingRecruitWorldScheduleIsolationAndRewards(t *testing.T) {
	f, actor, base := adminKingFixture(t)
	base.Options.KingWorld = 1
	base.Prizes = base.Prizes[:1]
	one := adminKingDefinition(t, f, actor, base)
	slot := reusePublish(t, f, actor, one, 1, 1, "calendar")
	// Same world cannot be published twice over the same time.
	second := adminKingDefinition(t, f, actor, base)
	calendar, err := f.s.AdminActivitySchedule(f.ctx, actor, 1)
	if err != nil {
		t.Fatal(err)
	}
	item := slot
	item.ID = ""
	item.Version = ""
	item.TemplateID = second.ID
	item.TemplateVersion = second.Version
	preview, err := f.s.AdminPreviewActivitySchedule(f.ctx, actor, 1, model.AdminActivitySchedulePreviewRequest{Version: calendar.Version, Items: append(calendar.Items, item)})
	if err != nil || preview.Valid {
		t.Fatal("overlapping same-world schedule accepted", err)
	}
	// A cross-world prize cannot be hidden behind a selected-world option.
	base.Prizes = append(base.Prizes, model.AdminActivityPrize{ID: 3, ResourceID: "hero_005", Amount: 1, Weight: 1, Guaranteed: true})
	if _, err = f.s.AdminSaveActivityDefinition(f.ctx, actor, "", model.AdminActivityDefinitionSaveRequest{Name: "跨世界错误", Form: base, DefaultDays: 1, Enabled: true}); err == nil {
		t.Fatal("foreign-world reward accepted")
	}
}
