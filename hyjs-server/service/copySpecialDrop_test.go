package service

import (
	"encoding/json"
	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
	"testing"
)

// Full original rows verify local cadence only. Real settlement and sweep are separately HTTP tested.
func TestCopySpecialAllOriginalCadenceAndConservation(t *testing.T) {
	f := p4Setup(t)
	if e := f.s.InitializeCopyDrops(f.ctx); e != nil {
		t.Fatal(e)
	}
	tasks, entries, gear := 0, 0, 0
	for _, task := range f.s.Tables["CopyTask"] {
		input, e := f.s.copySpecialInput(db.GetIns(), task)
		if e != nil {
			t.Fatal(task["TaskId"], e)
		}
		if input == nil {
			continue
		}
		tasks++
		entries += len(input.Entries)
		p := &pb.Player{EquipBagSize: proto.Int32(1000), WarriorBagSize: proto.Int32(1000), Level: proto.Int32(100)}
		g := &model.GameState{}
		for n := 0; n < 4; n++ {
			var gain *pb.ResultObject
			e = db.GetIns().Transaction(func(tx *gorm.DB) error {
				var e error
				gain, _, e = f.s.applyCopySpecialInput(tx, int64(3000+tasks), p, g, input)
				return e
			})
			if e != nil {
				t.Fatal(task["TaskId"], e)
			}
			if n < 3 && (len(gain.GetSimps()) != 0 || len(gain.GetEquips()) != 0) {
				t.Fatal("unearned local cadence reward")
			}
			if n == 3 {
				if len(gain.GetSimps())+len(gain.GetEquips()) != len(input.Entries) {
					t.Fatal("every original entry once", task["TaskId"], gain)
				}
				gear += len(gain.GetEquips())
			}
		}
		var rows []db.CopySpecialState
		if e = db.GetIns().Where("player_id=?", 3000+tasks).Find(&rows).Error; e != nil {
			t.Fatal(e)
		}
		for _, state := range rows {
			if state.Successes != 4 || state.Credit != 0 || state.Spent != 4 || state.Granted != 1 || len(state.LastInputHash) != 64 {
				t.Fatal("credit conservation", state)
			}
		}
	}
	if tasks != 79 || entries != 102 || gear != 4 {
		t.Fatal("full79 tasks/102 entries/4 original equips", tasks, entries, gear)
	}
	t.Log("full79/102 entries, local four-success cadence, 4equip/98frag, credit+spent=successes")
}

func TestCopySpecialFrozenCycleCapacityAndLegacy(t *testing.T) {
	f := p4Setup(t)
	if e := f.s.InitializeCopyDrops(f.ctx); e != nil {
		t.Fatal(e)
	}
	task := f.s.table("CopyTask", "TaskId", "20")
	input, e := f.s.copySpecialInput(db.GetIns(), task)
	if e != nil {
		t.Fatal(e)
	}
	p := &pb.Player{EquipBagSize: proto.Int32(0), Level: proto.Int32(100)}
	g := &model.GameState{}
	for n := 0; n < 3; n++ {
		if e = db.GetIns().Transaction(func(tx *gorm.DB) error { _, _, e := f.s.applyCopySpecialInput(tx, f.id, p, g, input); return e }); e != nil {
			t.Fatal(e)
		}
	}
	if e = db.GetIns().Transaction(func(tx *gorm.DB) error { _, _, e := f.s.applyCopySpecialInput(tx, f.id, p, g, input); return e }); e == nil {
		t.Fatal("shared equipment capacity bypass")
	}
	var state db.CopySpecialState
	if e = db.GetIns().First(&state, "player_id=? AND task_id=?", f.id, "20").Error; e != nil {
		t.Fatal(e)
	}
	if state.Successes != 3 || state.Credit != 3 || state.Granted != 0 {
		t.Fatal("failed grant committed credit")
	}
	if e = db.GetIns().Model(&db.CopySpecialRule{}).Where("id=?", constUtil.CopySpecialRuleID).Update("cycle", 9).Error; e != nil {
		t.Fatal(e)
	}
	p.EquipBagSize = proto.Int32(60)
	var gain *pb.ResultObject
	var resources []*pb.BaseResource
	if e = db.GetIns().Transaction(func(tx *gorm.DB) error {
		var e error
		gain, resources, e = f.s.applyCopySpecialInput(tx, f.id, p, g, input)
		return e
	}); e != nil {
		t.Fatal(e)
	}
	if len(gain.GetEquips()) != 1 || len(resources) != 1 || resources[0].GetGuid() != gain.GetEquips()[0].GetGuid() {
		t.Fatal("frozen4 retained exact original GUID")
	}
	fresh, e := f.s.copySpecialInput(db.GetIns(), task)
	if e != nil || fresh.Cycle != 9 {
		t.Fatal("new rule cadence", e)
	}
	if e = db.GetIns().First(&state, "player_id=? AND task_id=?", f.id, "20").Error; e != nil {
		t.Fatal(e)
	}
	before, _ := json.Marshal(state)
	if e = db.GetIns().Transaction(func(tx *gorm.DB) error { _, _, e := f.s.applyCopySpecialInput(tx, f.id, p, g, nil); return e }); e != nil {
		t.Fatal(e)
	}
	if e = db.GetIns().First(&state, "player_id=? AND task_id=?", f.id, "20").Error; e != nil {
		t.Fatal(e)
	}
	after, _ := json.Marshal(state)
	if string(before) != string(after) {
		t.Fatal("legacy noSpecial changed progress")
	}
	// Invalid live triples cannot silently change unrelated source rewards.
	invalid := map[string]string{"TaskId": "20", "SpecialOdds": "equip_001,1:0,200"}
	if _, e = f.s.copySpecialInput(db.GetIns(), invalid); e == nil {
		t.Fatal("malformed original triple")
	}
	if e = db.GetIns().Model(&db.CopySpecialRule{}).Where("id=?", constUtil.CopySpecialRuleID).Update("enabled", false).Error; e != nil {
		t.Fatal(e)
	}
	off, e := f.s.copySpecialInput(db.GetIns(), task)
	if e != nil || off.Enabled {
		t.Fatal("operator off", e)
	}
	if e = f.s.InitializeCopyDrops(f.ctx); e != nil {
		t.Fatal(e)
	}
	off, e = f.s.copySpecialInput(db.GetIns(), task)
	if e != nil || off.Enabled || off.Cycle != 9 {
		t.Fatal("restart seed overwrote operator")
	}
	t.Log("frozen4 vsnew9, capacity rollback, exact GUID currentResource, legacy nil no reward, bad tuple fail, operatoroff preserved")
}
