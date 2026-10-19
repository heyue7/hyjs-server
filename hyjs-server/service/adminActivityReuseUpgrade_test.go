package service

import (
	"encoding/json"
	"google.golang.org/protobuf/proto"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
	"strings"
	"testing"
)

func TestAdminReuseUpgradeOldCharacterInstances(t *testing.T) {
	f, actor := reuseFixture(t, false)
	form := adminModeForm("login-rewards")
	form.Milestones = []model.AdminActivityMilestone{{Goal: 1, Rewards: []model.AdminResourceAmount{{ResourceID: "coin", Amount: 3}}}}
	definition, err := f.s.AdminSaveActivityDefinition(f.ctx, actor, "", model.AdminActivityDefinitionSaveRequest{Name: "升级前角色活动", Form: form, DefaultDays: 1, Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	first := reusePublish(t, f, actor, *definition, 1, 1, "character-day")
	second := reusePublish(t, f, actor, *definition, 2, 1, "character-day")
	var firstReceipt proto.Message
	for day := 0; day < 2; day++ {
		if day > 0 {
			reuseClockDay(t, f, actor)
		}
		bound := serverClockBindTest(t, f, f.id)
		if err = f.s.PrepareAdminCharacterActivitySchedules(bound, f.id); err != nil {
			t.Fatal(err)
		}
		fixture := *f
		fixture.ctx = bound
		fixture.event(t, "upgrade-login-"+p1Day(LocalPlayNow(bound)), constUtil.P1EventLoginDay, 1, LocalPlayNow(bound))
		output, e := f.s.p4Welfare(bound, f.id, &pb.CSGetLoginReward{}, int64(71001+day), LocalPlayNow(bound))
		if e != nil {
			t.Fatal(e)
		}
		if day == 0 {
			firstReceipt = output[0]
		}
	}
	// Recreate the exact upgrade boundary: old issued/source JSON has no policy,
	// and the newly introduced owner table has no rows. Both prior periods have
	// real native states/receipts; stopped/expired source rows remain evidence.
	for _, source := range []model.AdminActivityOccurrenceDTO{first, second} {
		var issued db.AdminCharacterActivityInstance
		if err = db.GetIns().First(&issued, "player_id=? AND occurrence_id=?", f.id, source.ID).Error; err != nil {
			t.Fatal(err)
		}
		var snapshot map[string]any
		if err = json.Unmarshal(issued.FormJSON, &snapshot); err != nil {
			t.Fatal(err)
		}
		delete(snapshot, "repeatPolicy")
		raw, _ := json.Marshal(snapshot)
		if err = db.GetIns().Model(&issued).Update("form_json", raw).Error; err != nil {
			t.Fatal(err)
		}
		if err = db.GetIns().Model(&db.AdminActivityOccurrence{}).Where("id=?", source.ID).Update("form_json", raw).Error; err != nil {
			t.Fatal(err)
		}
		reuseToggle(t, f, actor, source.ID, false)
	}
	var stateBefore []db.P4WelfareState
	if err = db.GetIns().Where("player_id=?", f.id).Order("rule_id").Find(&stateBefore).Error; err != nil {
		t.Fatal(err)
	}
	before, _ := json.Marshal(stateBefore)
	if err = db.GetIns().Where("player_id=?", f.id).Delete(&db.AdminActivityTemplateOwner{}).Error; err != nil {
		t.Fatal(err)
	}
	form.RepeatPolicy = "once"
	definition, err = f.s.AdminSaveActivityDefinition(f.ctx, actor, definition.ID, model.AdminActivityDefinitionSaveRequest{Name: definition.Name, Form: form, DefaultDays: 1, Enabled: true, Version: definition.Version})
	if err != nil {
		t.Fatal(err)
	}
	reusePublish(t, f, actor, *definition, 3, 1, "character-day")
	reuseClockDay(t, f, actor)
	f.restart(t)
	if err = f.s.InitializeAdminActivitySchedules(f.ctx); err != nil {
		t.Fatal(err)
	}
	bound := serverClockBindTest(t, f, f.id)
	if err = f.s.PrepareAdminCharacterActivitySchedules(bound, f.id); err != nil {
		t.Fatal(err)
	}
	if got := p1Count(t, &db.AdminCharacterActivityInstance{}, "player_id=? AND mode=?", f.id, "login-rewards"); got != 2 {
		t.Fatal("old participants received a new once period", got)
	}
	var owner db.AdminActivityTemplateOwner
	if err = db.GetIns().First(&owner, "server_id=? AND player_id=? AND template_id=?", 1, f.id, definition.ID).Error; err != nil || owner.OccurrenceID != first.ID {
		t.Fatal("old stable first acceptance was not recovered", owner, err)
	}
	// An old disabled instance entrance also repairs ownership without opening it.
	if err = db.GetIns().Where("player_id=?", f.id).Delete(&db.AdminActivityTemplateOwner{}).Error; err != nil {
		t.Fatal(err)
	}
	var issued db.AdminCharacterActivityInstance
	if err = db.GetIns().First(&issued, "player_id=? AND occurrence_id=?", f.id, second.ID).Error; err != nil {
		t.Fatal(err)
	}
	allowed, err := adminCharacterActivityAccess(db.GetIns().WithContext(bound), issued.ConsumerKey)
	if err != nil || allowed {
		t.Fatal("old disabled instance access", allowed, err)
	}
	if err = db.GetIns().First(&owner, "server_id=? AND player_id=? AND template_id=?", 1, f.id, definition.ID).Error; err != nil || owner.OccurrenceID != first.ID {
		t.Fatal("access recovered wrong first period", owner, err)
	}
	var stateAfter []db.P4WelfareState
	if err = db.GetIns().Where("player_id=?", f.id).Order("rule_id").Find(&stateAfter).Error; err != nil {
		t.Fatal(err)
	}
	after, _ := json.Marshal(stateAfter)
	if string(before) != string(after) {
		t.Fatal("upgrade changed native claimed state bytes")
	}
	output, err := f.s.p4Welfare(bound, f.id, &pb.CSGetLoginReward{}, 71001, LocalPlayNow(bound))
	if err != nil || !proto.Equal(output[0], firstReceipt) {
		t.Fatal("cold old receipt changed", err)
	}
}

func TestAdminReuseUpgradeOldAbsoluteConsumer(t *testing.T) {
	f, actor := reuseFixture(t, false)
	if err := updateProfile(f.ctx, f.id, func(p *pb.Player, _ *model.GameState) error { p.Diamond = proto.Int64(20); return nil }); err != nil {
		t.Fatal(err)
	}
	form := adminModeForm("exchange")
	form.Products = []model.AdminActivityProduct{{ID: "upgrade-absolute", Name: "普通原兑换", Index: 1, Rewards: []model.AdminResourceAmount{{ResourceID: "coin", Amount: 7}}, Cost: model.AdminResourceAmount{ResourceID: "diamond", Amount: 1}, DailyLimit: 2, TotalLimit: 2, Stock: 2, MinLevel: 1, DiscountBps: 10000, Weight: 1}}
	definition, err := f.s.AdminSaveActivityDefinition(f.ctx, actor, "", model.AdminActivityDefinitionSaveRequest{Name: "旧绝对期兑换", Form: form, DefaultDays: 1, Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	first := reusePublish(t, f, actor, *definition, 1, 1, "calendar")
	bound := serverClockBindTest(t, f, f.id)
	receipt, err := reuseTrade(t, f, bound, first, 72001)
	if err != nil {
		t.Fatal(err)
	}
	_, oldKey, _ := strings.Cut(first.ConsumerID, ":")
	var state db.P4CommerceState
	if err = db.GetIns().First(&state, "player_id=? AND rule_id=?", f.id, oldKey).Error; err != nil {
		t.Fatal(err)
	}
	original := append([]byte{}, state.Payload...)
	reuseToggle(t, f, actor, first.ID, false)
	if err = db.GetIns().Where("player_id=?", f.id).Delete(&db.AdminActivityTemplateOwner{}).Error; err != nil {
		t.Fatal(err)
	}
	form.RepeatPolicy = "once"
	definition, err = f.s.AdminSaveActivityDefinition(f.ctx, actor, definition.ID, model.AdminActivityDefinitionSaveRequest{Name: definition.Name, Form: form, DefaultDays: 1, Enabled: true, Version: definition.Version})
	if err != nil {
		t.Fatal(err)
	}
	second := reusePublish(t, f, actor, *definition, 2, 1, "calendar")
	reuseClockDay(t, f, actor)
	bound = serverClockBindTest(t, f, f.id)
	if _, err = reuseTrade(t, f, bound, second, 72002); err == nil {
		t.Fatal("proven old absolute participant accepted new once period")
	}
	var owner db.AdminActivityTemplateOwner
	// The denied transaction rolls back its lazy marker; make the idempotent
	// upgrade recovery explicit to inspect it without changing any native state.
	if _, err = adminAcceptActivityTemplate(db.GetIns().WithContext(bound), f.id, db.AdminActivityOccurrence{ID: second.ID, ServerID: 1, TemplateID: definition.ID, Mode: "exchange", FormJSON: adminJSON(second.Form)}); err != nil {
		t.Fatal(err)
	}
	if err = db.GetIns().First(&owner, "server_id=? AND player_id=? AND template_id=?", 1, f.id, definition.ID).Error; err != nil || owner.OccurrenceID != first.ID {
		t.Fatal("absolute evidence missing", owner, err)
	}
	if err = db.GetIns().First(&state, "player_id=? AND rule_id=?", f.id, oldKey).Error; err != nil || string(state.Payload) != string(original) {
		t.Fatal("absolute migration rewrote purchase state", err)
	}
	// A role with no native proof is not bulk-marked as an old participant.
	peer, err := LoginAccount(f.ctx, "upgrade-unrelated-role", "fixture-password")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = CreatePlayer(f.ctx, peer.Id, "未参与旧活动", "hero_043"); err != nil {
		t.Fatal(err)
	}
	peerBound := serverClockBindTest(t, f, peer.Id)
	_, newKey, _ := strings.Cut(second.ConsumerID, ":")
	if _, err = f.s.p4Commerce(peerBound, peer.Id, &pb.CSActivityShopInfo{ActivityId: proto.String(newKey)}, 0, LocalPlayNow(peerBound)); err != nil {
		t.Fatal("unrelated role incorrectly marked participated", err)
	}
	f.restart(t)
	again, err := reuseTrade(t, f, bound, first, 72001)
	if err != nil || !proto.Equal(again[0], receipt[0]) {
		t.Fatal("absolute old receipt changed", err)
	}
}
