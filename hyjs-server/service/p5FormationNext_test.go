package service

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"google.golang.org/protobuf/proto"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

// TestP5FormationNext 验证任意角色替换主战位，并保持旧主角和同ID红卡独立实例。
func TestP5FormationNext(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "p5-next.db")
	schema := os.Getenv("HYJS_P5_NEXT_MYSQL_DATABASE")
	open := func() {
		t.Helper()
		var err error
		if schema != "" {
			if !strings.HasPrefix(schema, "hyjs_test_p5next_") {
				t.Fatal("P5 next database rejected")
			}
			err = db.InitGameMySQL(ctx, db.MySQLConfig{Host: os.Getenv("HYJS_MYSQL_HOST"), User: os.Getenv("HYJS_MYSQL_USER"), Password: os.Getenv("HYJS_MYSQL_PASSWORD"), Database: schema})
		} else {
			err = db.InitGame(ctx, path)
		}
		if err != nil {
			t.Fatal(err)
		}
	}
	open()
	defer func() { pool, _ := db.GetIns().DB(); pool.Close() }()
	s := &GameService{P5Ready: true}
	if err := s.LoadTables("../../analysis/tables"); err != nil {
		t.Fatal(err)
	}
	if err := s.LoadP5Tables("../../analysis/tables"); err != nil {
		t.Fatal(err)
	}
	if err := s.InitializeP5(ctx); err != nil {
		t.Fatal(err)
	}
	if err := s.InitializeP1(ctx); err != nil {
		t.Fatal(err)
	}
	row, err := LoginAccount(ctx, "p5-next-main", "test-password")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = CreatePlayer(ctx, row.Id, "主角阵容原消息", "hero_001"); err != nil {
		t.Fatal(err)
	}
	if err = updateProfile(ctx, row.Id, func(p *pb.Player, g *model.GameState) error {
		p.Level = proto.Int32(11)
		_, err := s.GrantFixedReward(p, g, "hero_020,1;hero_001,1;equip_001,1;coin,1000000")
		return err
	}); err != nil {
		t.Fatal(err)
	}
	playerID := row.Id
	state := func() (*pb.Player, *model.GameState, db.Profile) {
		t.Helper()
		var saved db.Profile
		if err := db.GetIns().First(&saved, playerID).Error; err != nil {
			t.Fatal(err)
		}
		p, g, err := decodeProfile(&saved)
		if err != nil {
			t.Fatal(err)
		}
		return p, g, saved
	}
	call := func(b *pb.CSFormationOn, stamp int64) []proto.Message {
		t.Helper()
		messages, handled, err := s.DispatchP5Next(ctx, playerID, b, stamp)
		if err != nil || !handled {
			t.Fatalf("P5 next frame handled=%v err=%v", handled, err)
		}
		return messages
	}
	p, g, _ := state()
	mainGUID := p.GetHeroWarId()
	main, err := p5Warrior(g, mainGUID)
	if err != nil {
		t.Fatal(err)
	}
	mainSnapshot := proto.Clone(main)
	var selected, duplicate *pb.Warrior
	for _, w := range g.Warriors {
		if w.GetId() == "hero_020" {
			selected = w
		}
		if w.GetId() == "hero_001" && w.GetGuid() != mainGUID {
			duplicate = w
		}
	}
	if selected == nil || duplicate == nil {
		t.Fatal("missing separate owned heroes")
	}
	equip := g.Equipment[0]
	if _, err := s.equipmentFormationOn(ctx, playerID, &pb.CSFormationOn{Id: proto.String(equip.GetEquipId()), Guid: proto.String(equip.GetGuid()), FormatPos: proto.Int32(1)}); err != nil {
		t.Fatal(err)
	}
	request := &pb.CSFormationOn{Id: proto.String(selected.GetId()), Guid: proto.String(selected.GetGuid()), FormatPos: proto.Int32(1)}
	if _, handled, err := s.DispatchP5Next(ctx, playerID, request, 101); !handled || err != nil {
		t.Fatal("valid single-member replacement denied", err)
	}
	if err := updateProfile(ctx, playerID, func(p *pb.Player, _ *model.GameState) error { p.Level = proto.Int32(12); return nil }); err != nil {
		t.Fatal(err)
	}
	var wait sync.WaitGroup
	errs := make(chan error, 6)
	for n := 0; n < 6; n++ {
		wait.Add(1)
		go func() { defer wait.Done(); _, _, err := s.DispatchP5Next(ctx, playerID, request, 102); errs <- err }()
	}
	wait.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal("parallel native replacement", err)
		}
	}
	first := call(request, 102)
	result := first[0].(*pb.SCFormationOn)
	if formationHero(result.GetFormationItem()).GetGuid() != selected.GetGuid() || len(result.GetFormationItem().GetThings()) != 2 {
		t.Fatal("native replacement dropped loadout", result)
	}
	p, g, _ = state()
	main, err = p5Warrior(g, mainGUID)
	if err != nil || p.GetHeroWarId() != mainGUID || !proto.Equal(main, mainSnapshot) {
		t.Fatal("main identity or growth changed", err)
	}
	ownedEquip, err := s.equipmentRecord(g, equip.GetGuid())
	if err != nil || ownedEquip.GetLastHeroId() != selected.GetId() {
		t.Fatal("equipment owner not updated", err, ownedEquip)
	}
	if err := P5GuardInstance(p, g, mainGUID); err == nil {
		t.Fatal("off-formation original main lost recycle protection")
	}
	if err := P5GuardInstance(p, g, duplicate.GetGuid()); err != nil {
		t.Fatal("same-ID different-GUID red hero incorrectly protected", err)
	}
	if err := updateProfile(ctx, playerID, func(p *pb.Player, _ *model.GameState) error { p.Level = proto.Int32(25); return nil }); err != nil {
		t.Fatal(err)
	}
	if _, _, err := s.DispatchP5Next(ctx, playerID, &pb.CSHelperOn{Guid: proto.String(mainGUID), Pos: proto.Int32(1)}, 201); err == nil {
		t.Fatal("original main entered assistance after leaving formation")
	}
	if _, handled, err := s.DispatchP5Next(ctx, playerID, &pb.CSHelperOn{Guid: proto.String(duplicate.GetGuid()), Pos: proto.Int32(1)}, 202); !handled || err != nil {
		t.Fatal("same-ID independent red hero cannot assist", err)
	}
	if _, err := s.p5HelperOff(ctx, playerID, &pb.CSHelperOff{Pos: proto.Int32(1)}, 203); err != nil {
		t.Fatal(err)
	}
	if _, _, err := s.DispatchP5Next(ctx, playerID, &pb.CSFormationOn{Id: proto.String(duplicate.GetId()), Guid: proto.String(duplicate.GetGuid()), FormatPos: proto.Int32(1)}, 102); err == nil {
		t.Fatal("receipt collision accepted changed main selection")
	}
	pool, _ := db.GetIns().DB()
	pool.Close()
	open()
	if err := s.InitializeP5(ctx); err != nil {
		t.Fatal(err)
	}
	replayed := call(request, 102)
	if !proto.Equal(first[0], replayed[0]) {
		t.Fatal("native replacement replay changed after reconnect")
	}
	returnMain := &pb.CSFormationOn{Id: proto.String(main.GetId()), Guid: proto.String(mainGUID), FormatPos: proto.Int32(1)}
	call(returnMain, 103)
	p, g, _ = state()
	if formationHero(g.Formation[0]).GetGuid() != mainGUID || p.GetHeroWarId() != mainGUID {
		t.Fatal("original main could not return")
	}
	call(request, 104)
	_, g, _ = state()
	// 用下一组原请求创建新投入，换阵容主人不改变账本指纹，卸下仍可重置退款。
	if err := updateProfile(ctx, playerID, func(p *pb.Player, g *model.GameState) error {
		p.Level = proto.Int32(16)
		p.Diamond = proto.Int64(100)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.p5StrengthenOne(ctx, playerID, &pb.CSEquipStrengthenOneKey{Guid: proto.String(equip.GetGuid())}, 105); err != nil {
		t.Fatal(err)
	}
	call(returnMain, 106)
	if _, err := s.formationOff(ctx, playerID, &pb.CSFormationOff{Id: proto.String(equip.GetEquipId()), Guid: proto.String(equip.GetGuid()), FormatPos: proto.Int32(1)}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.p5EquipmentReset(ctx, playerID, &pb.CSEquipReBuild{Guid: proto.String(equip.GetGuid())}, 107); err != nil {
		t.Fatal("investment rejected after formation owner change", err)
	}
	p, g, _ = state()
	// 当前重置合同把装备退为原合成碎片，并退还投入，不保留原装备GUID。
	if _, err = s.equipmentRecord(g, equip.GetGuid()); err == nil || p.GetCoin() != 1000000 || p.GetDiamond() != 50 {
		t.Fatal("investment refund lost after native main replacement", err, p)
	}
	fragments, err := s.p5WarriorFragmentReward(equip.GetEquipId(), 1)
	if err != nil {
		t.Fatal(err)
	}
	for _, fragment := range fragments {
		if simpleOwned(p, g, fragment.GetId()) < int64(fragment.GetNum()) {
			t.Fatal("original equipment fragments missing")
		}
	}
}
