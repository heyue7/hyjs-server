package service

import (
	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
	"slices"
	"sync"
	"testing"
)

func TestCopyBoxAllOriginalReachableContents(t *testing.T) {
	f := p4Setup(t)
	if e := f.s.LoadP5Tables("../../analysis/tables"); e != nil {
		t.Fatal(e)
	}
	reachable, blocked, entries := 0, 0, 0
	for _, row := range f.s.Tables["CopyBox"] {
		input, e := f.s.copyBoxInput(row)
		if f.s.table("CopyMission", "MissionId", row["MissionId"]) == nil {
			if e == nil {
				t.Fatal("orphan mission allowed")
			}
			blocked++
			continue
		}
		if e != nil {
			t.Fatal(row["BoxId"], e)
		}
		reachable++
		p := &pb.Player{Level: proto.Int32(100), EquipBagSize: proto.Int32(1000), WarriorBagSize: proto.Int32(1000)}
		g := &model.GameState{}
		gain, e := f.s.openServerGain(p, g, input.Reward)
		if e != nil {
			t.Fatal(row["BoxId"], e)
		}
		entries += len(gain.GetSimps()) + len(gain.GetEquips()) + len(gain.GetWars())
		if p.GetCoin() != 80000 || input.Box.GetRes() != "box" || input.Box.GetIsOpen() != 0 {
			t.Fatal("original coin/model/open", row["BoxId"])
		}
	}
	// Equip quantity2 expands into exact separate GUIDs, so 203 original entries produce 211 instances.
	if reachable != 79 || blocked != 15 || entries != 211 {
		t.Fatal(reachable, blocked, entries)
	}
	t.Log("79 reachable original chests/203 entries/211 instances;15 orphan missions preserved blocked")
}
func TestCopyBoxFrozenRollbackConcurrentClaimAndCold(t *testing.T) {
	f := p4Setup(t)
	if e := f.s.LoadP5Tables("../../analysis/tables"); e != nil {
		t.Fatal(e)
	}
	if e := f.s.InitializeP5(f.ctx); e != nil {
		t.Fatal(e)
	}
	request := &pb.CSCopyBox{BoxId: proto.String("box_6")}
	if _, e := f.s.copyBoxOpen(f.ctx, f.id, request, 1); e == nil {
		t.Fatal("history only source")
	}
	var box *pb.CopyBoxInfo
	if e := updateProfileWithTx(f.ctx, f.id, func(tx *gorm.DB, p *pb.Player, g *model.GameState) error {
		p.Level = proto.Int32(100)
		p.EquipBagSize = proto.Int32(0)
		g.Run = &model.CopyRun{Chapter: "2", Mission: "6", Passed: []string{}}
		boxes, e := f.s.CaptureCopyBoxes(tx, f.id, "2", "6")
		if e == nil {
			box = boxes[0]
		}
		return e
	}); e != nil {
		t.Fatal(e)
	}
	if box.GetBoxId() != "box_6" || box.GetX() != 7 || box.GetY() != 1 || box.GetZ() != 28 || box.GetAngle() != 90 {
		t.Fatal("original scene projection", box)
	}
	if _, e := f.s.copyBoxOpen(f.ctx, f.id, request, 1); e == nil {
		t.Fatal("incomplete source")
	}
	if e := updateProfile(f.ctx, f.id, func(p *pb.Player, g *model.GameState) error {
		g.Run.Complete = true
		for _, task := range f.s.tasks("6") {
			g.Run.Passed = append(g.Run.Passed, task["TaskId"])
		}
		g.Run.Fight = &model.CopyFight{Settled: true, Result: &pb.SCCopyFightEnd{FightResult: proto.Int32(1)}}
		return nil
	}); e != nil {
		t.Fatal(e)
	}
	beforeP, beforeG := f.profile(t)
	if _, e := f.s.copyBoxOpen(f.ctx, f.id, request, 1); e == nil {
		t.Fatal("capacity bypass")
	}
	afterP, afterG := f.profile(t)
	if !proto.Equal(beforeP, afterP) || len(afterG.Equipment) != len(beforeG.Equipment) {
		t.Fatal("failed grant profile changed")
	}
	var claim db.CopyBoxClaim
	if e := db.GetIns().First(&claim, "player_id=? AND box_id=?", f.id, "box_6").Error; e != nil || claim.Claimed {
		t.Fatal("failed grant claimed", e)
	}
	if e := updateProfile(f.ctx, f.id, func(p *pb.Player, g *model.GameState) error { p.EquipBagSize = proto.Int32(60); return nil }); e != nil {
		t.Fatal(e)
	}
	// Hot original table changes do not rewrite the actual entry snapshot.
	f.s.table("CopyBox", "BoxId", "box_6")["DropAward"] = "coin,1"
	var wg sync.WaitGroup
	results := make(chan []proto.Message, 8)
	errs := make(chan error, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); v, e := f.s.copyBoxOpen(f.ctx, f.id, request, 1); results <- v; errs <- e }()
	}
	wg.Wait()
	close(results)
	close(errs)
	for e := range errs {
		if e != nil {
			t.Fatal(e)
		}
	}
	var response *pb.SCCopyBox
	for r := range results {
		if response == nil {
			response = r[0].(*pb.SCCopyBox)
		} else if !proto.Equal(response, r[0]) {
			t.Fatal("duplicate reroll")
		}
	}
	if len(response.GetCurrentResource()) != 2 || !slices.ContainsFunc(response.GetCurrentResource(), func(v *pb.BaseResource) bool { return v.GetId() == "equip_002" && v.GetGuid() != "" }) {
		t.Fatal("original resource GUID", response)
	}
	p, g := f.profile(t)
	if p.GetCoin() != beforeP.GetCoin()+80000 || len(g.Equipment) != len(beforeG.Equipment)+1 {
		t.Fatal("eight source grants")
	}
	if _, e := f.s.copyBoxOpen(f.ctx, f.id, request, 2); e == nil {
		t.Fatal("second distinct claim")
	}
	if e := db.GetIns().Transaction(func(tx *gorm.DB) error {
		boxes, e := f.s.CaptureCopyBoxes(tx, f.id, "2", "6")
		if e == nil && boxes[0].GetIsOpen() != 1 {
			t.Fatal("claimed scene state")
		}
		return e
	}); e != nil {
		t.Fatal(e)
	}
	if e := db.GetIns().Model(&db.CopyBoxRule{}).Where("id=?", 1).Update("enabled", false).Error; e != nil {
		t.Fatal(e)
	}
	sql, _ := db.GetIns().DB()
	sql.Close()
	f.open(t)
	if e := f.s.InitializeP5(f.ctx); e != nil {
		t.Fatal(e)
	}
	replay, e := f.s.copyBoxOpen(f.ctx, f.id, request, 1)
	if e != nil || !proto.Equal(response, replay[0]) {
		t.Fatal("cold old off receipt", e)
	}
	var rule db.CopyBoxRule
	db.GetIns().First(&rule, 1)
	if rule.Enabled {
		t.Fatal("operator off reenabled")
	}
	t.Log("entry frozen coordinates/reward, incomplete/history rejection, capacity rollback,8once,one lifetime claim,open scene,cold old receipt/off")
}
