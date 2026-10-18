package service

import (
	"encoding/json"
	"fmt"
	"math"
	"sync"
	"testing"

	"google.golang.org/protobuf/proto"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

func TestBagCapacityLegacyColdAndFullTransaction(t *testing.T) {
	f := p1Setup(t)
	p, g := f.profile(t)
	if p.GetWarriorBagSize() != 1000 || p.GetEquipBagSize() != 1000 || p.GetPetBagSize() != 1000 || g.BagCapacityScale != 10 {
		t.Fatal("fresh role did not receive all three scaled bag limits")
	}
	// 模拟旧角色已有VIP增加的105栏与120个真实实例，恢复时不改任何实例。
	p.WarriorBagSize = proto.Int32(105)
	p.EquipBagSize = proto.Int32(108)
	p.PetBagSize = proto.Int32(111)
	p.Coin = proto.Int64(100)
	g.BagCapacityScale = 0
	for len(g.Warriors) < 120 {
		g.Warriors = append(g.Warriors, &pb.Warrior{Id: proto.String("hero_020"), Guid: proto.String(fmt.Sprintf("owned-%d", len(g.Warriors))), Level: proto.Int32(1)})
	}
	PrepareWarriors(g)
	ownedBefore, err := json.Marshal(g.Warriors)
	if err != nil {
		t.Fatal(err)
	}
	playerRaw, _ := json.Marshal(p)
	gameRaw, _ := json.Marshal(g)
	if err = db.GetIns().Model(&db.Profile{}).Where("id=?", f.id).Updates(map[string]any{"player": string(playerRaw), "game": string(gameRaw)}).Error; err != nil {
		t.Fatal(err)
	}
	var baseline db.Profile
	if err = db.GetIns().First(&baseline, f.id).Error; err != nil {
		t.Fatal(err)
	}
	if _, _, err = decodeProfileForUpdate(&baseline); err != nil {
		t.Fatal(err)
	}
	decodedBaseline, _, err := decodeProfile(&baseline)
	if err != nil || decodedBaseline.GetWarriorBagSize() != 1050 || decodedBaseline.GetEquipBagSize() != 1080 || decodedBaseline.GetPetBagSize() != 1110 {
		t.Fatal("resource baseline lost its matching capacity-version marker", err)
	}
	// 并发的正常事务必须共用角色行锁，容量和版本原子保存且不重复放大。
	var wg sync.WaitGroup
	errs := make(chan error, 4)
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			errs <- updateProfile(f.ctx, f.id, func(current *pb.Player, _ *model.GameState) error {
				if current.GetWarriorBagSize() != 1050 || current.GetEquipBagSize() != 1080 || current.GetPetBagSize() != 1110 {
					return fmt.Errorf("legacy capacity: %d", current.GetWarriorBagSize())
				}
				return nil
			})
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	f.restart(t)
	p, g = f.profile(t)
	ownedAfter, _ := json.Marshal(g.Warriors)
	if p.GetWarriorBagSize() != 1050 || g.BagCapacityScale != 10 || string(ownedBefore) != string(ownedAfter) || p.GetEquipBagSize() != 1080 || p.GetPetBagSize() != 1110 {
		t.Fatal("cold migration repeated or changed inventory/other bags")
	}
	if err = updateProfile(f.ctx, f.id, func(current *pb.Player, game *model.GameState) error {
		for len(game.Warriors) < int(current.GetWarriorBagSize()) {
			game.Warriors = append(game.Warriors, &pb.Warrior{Id: proto.String("hero_020"), Guid: proto.String(fmt.Sprintf("full-%d", len(game.Warriors))), Level: proto.Int32(1)})
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	// 扩容后仍保留原满包保护，前置扣费须和失败发奖一起回滚。
	err = updateProfile(f.ctx, f.id, func(current *pb.Player, game *model.GameState) error {
		current.Coin = proto.Int64(current.GetCoin() - 10)
		_, err := f.s.GrantFixedReward(current, game, "hero_020,1")
		return err
	})
	if err == nil {
		t.Fatal("full hero bag accepted a paid reward")
	}
	f.restart(t)
	p, g = f.profile(t)
	if p.GetCoin() != 100 || len(g.Warriors) != 1050 || p.GetWarriorBagSize() != 1050 {
		t.Fatal("full bag failure lost currency, capacity or owned characters")
	}
}

func TestBagCapacityOverflowAndVersionGuard(t *testing.T) {
	p := &pb.Player{WarriorBagSize: proto.Int32(math.MaxInt32), EquipBagSize: proto.Int32(103), PetBagSize: proto.Int32(109)}
	g := &model.GameState{}
	if err := prepareBagCapacity(p, g); err != nil || p.GetWarriorBagSize() != math.MaxInt32 || p.GetEquipBagSize() != 1030 || p.GetPetBagSize() != 1090 {
		t.Fatal("capacity overflow reduced existing storage", err)
	}
	before := proto.Clone(p).(*pb.Player)
	if err := prepareBagCapacity(p, g); err != nil || !proto.Equal(before, p) {
		t.Fatal("migration was not idempotent", err)
	}
	g.BagCapacityScale = constUtil.BagCapacityScale + 1
	if err := prepareBagCapacity(p, g); err == nil || !proto.Equal(before, p) {
		t.Fatal("unknown stored capacity version was silently converted")
	}
}

func TestBagCapacityUnreleasedFivefoldMigration(t *testing.T) {
	p := &pb.Player{WarriorBagSize: proto.Int32(525), EquipBagSize: proto.Int32(105), PetBagSize: proto.Int32(100)}
	g := &model.GameState{WarriorBagCapacityScale: 5}
	if err := prepareBagCapacity(p, g); err != nil || p.GetWarriorBagSize() != 1050 || p.GetEquipBagSize() != 1050 || p.GetPetBagSize() != 1000 || g.BagCapacityScale != 10 || g.WarriorBagCapacityScale != 0 {
		t.Fatal("unreleased hero-only scale was applied twice", err)
	}
	before := proto.Clone(p).(*pb.Player)
	if err := prepareBagCapacity(p, g); err != nil || !proto.Equal(before, p) {
		t.Fatal("fivefold migration repeated", err)
	}
	p = &pb.Player{WarriorBagSize: proto.Int32(500), EquipBagSize: proto.Int32(550), PetBagSize: proto.Int32(600)}
	g = &model.GameState{BagCapacityScale: 5}
	if err := prepareBagCapacity(p, g); err != nil || p.GetWarriorBagSize() != 1000 || p.GetEquipBagSize() != 1100 || p.GetPetBagSize() != 1200 {
		t.Fatal("internal common fivefold scale was not converted exactly", err)
	}
}
