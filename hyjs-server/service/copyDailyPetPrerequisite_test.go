package service

import (
	"context"
	"os"
	"strconv"
	"strings"
	"testing"

	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

// TestCopyDailyPetPrerequisite prepares only a finite non-source dependency on
// the already restored real party. No379..390 material is inserted or awarded.
func TestCopyDailyPetPrerequisite(t *testing.T) {
	if os.Getenv("HYJS_COPY_DAILY_PET_PREREQUISITE") != "1" {
		t.Skip("explicit finite non-source pet prerequisite for original daily source audit")
	}
	ctx := context.Background()
	schema := os.Getenv("HYJS_P5_TALENT_MYSQL_DATABASE")
	if !strings.HasPrefix(schema, "hyjs_test_p5_talent_") || schema != os.Getenv("HYJS_MYSQL_DATABASE") {
		t.Fatal("dedicated existing actual source database required")
	}
	if err := db.InitGameMySQL(ctx, db.MySQLConfig{Host: os.Getenv("HYJS_MYSQL_HOST"), User: os.Getenv("HYJS_MYSQL_USER"), Password: os.Getenv("HYJS_MYSQL_PASSWORD"), Database: schema}); err != nil {
		t.Fatal(err)
	}
	defer func() { pool, _ := db.GetIns().DB(); pool.Close() }()
	s := &GameService{}
	for _, load := range []func(string) error{s.LoadTables, s.LoadP5Tables, s.LoadP9Tables} {
		if err := load("../../analysis/tables"); err != nil {
			t.Fatal(err)
		}
	}
	for _, init := range []func(context.Context) error{s.InitializeP1, s.InitializeP9Pets, s.InitializeP9PetNext} {
		if err := init(ctx); err != nil {
			t.Fatal(err)
		}
	}
	var target *pb.Pet
	var count int64
	if err := updateProfileWithTx(ctx, 1, func(tx *gorm.DB, p *pb.Player, g *model.GameState) error {
		var err error
		pets, err := s.P9GrantPet(tx, p, "pet_047", 1)
		if err != nil {
			return err
		}
		target = pets[0]
		var total int64
		for level := int32(1); level < 10; level++ {
			exp, err := s.p9LevelExp(target, level)
			if err != nil {
				return err
			}
			total += exp
		}
		itemExp, err := strconv.ParseInt(s.table("Item", "itemId", "item_124")["exp"], 10, 64)
		if err != nil || itemExp < 1 {
			return p5Error()
		}
		count = (total + itemExp - 1) / itemExp
		_, err = s.GrantFixedReward(p, g, "item_374,8;item_124,"+strconv.FormatInt(count, 10))
		return err
	}); err != nil {
		t.Fatal(err)
	}
	stamp := int64(1800002550000)
	level, handled, err := s.DispatchP9PetNext(ctx, 1, &pb.CSPetLevelup{Guid: target.Guid, Cost: []*pb.BaseResource{{Id: proto.String("item_124"), Num: proto.Int32(int32(count))}}}, stamp)
	if err != nil || !handled || level[0].(*pb.SCPetLevelup).GetPet().GetLevel() < 10 {
		t.Fatal("actual original level prerequisite", err)
	}
	for step := int64(1); step <= 2; step++ {
		out, handled, err := s.DispatchP9PetEquip(ctx, 1, &pb.CSPetEquipStrengthOne{PetGuid: target.Guid, EquipId: proto.String("petequip_002"), Num: proto.Int32(5)}, stamp+step)
		if err != nil || !handled || out[0].(*pb.SCPetEquipStrengthOne).GetPetEquip().GetStrengthLevel() != int32(step*5) {
			t.Fatal("actual original10 strength prerequisite", err)
		}
	}
	t.Logf("finite original pet047 constructor; actual original level>=10 using%d item124 and strength10 paid; only item3748 non-source prepared; source379..390 untouched", count)
}
