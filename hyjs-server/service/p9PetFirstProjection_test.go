package service

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"google.golang.org/protobuf/proto"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

func TestP9FirstNativeProjection(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "p9-first-native.db")
	schema := os.Getenv("HYJS_P9_MYSQL_DATABASE")
	open := func() {
		var err error
		if schema == "" {
			err = db.InitGame(ctx, path)
		} else {
			if !strings.HasPrefix(schema, "hyjs_test_p9_") {
				t.Fatal("P9 isolation")
			}
			err = db.InitGameMySQL(ctx, db.MySQLConfig{Host: os.Getenv("HYJS_MYSQL_HOST"), User: os.Getenv("HYJS_MYSQL_USER"), Password: os.Getenv("HYJS_MYSQL_PASSWORD"), Database: schema})
		}
		if err != nil {
			t.Fatal(err)
		}
	}
	open()
	defer func() { pool, _ := db.GetIns().DB(); pool.Close() }()
	s := &GameService{}
	if err := s.LoadTables("../../analysis/tables"); err != nil {
		t.Fatal(err)
	}
	if err := s.LoadP9Tables("../../analysis/tables"); err != nil {
		t.Fatal(err)
	}
	for _, init := range []func(context.Context) error{s.InitializeP1, s.InitializeP9Pets, s.InitializeP9PetNext, s.InitializeP9PetRecycle} {
		if err := init(ctx); err != nil {
			t.Fatal(err)
		}
	}
	row, err := LoginAccount(ctx, "p9-first-native-projection", "isolated-password")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = CreatePlayer(ctx, row.Id, "原按钮首抽", "hero_043"); err != nil {
		t.Fatal(err)
	}
	level := func(n int32) {
		t.Helper()
		if err := updateProfile(ctx, row.Id, func(p *pb.Player, g *model.GameState) error {
			p.Level = proto.Int32(n)
			p.NoviceGuideSchedule = proto.Int32(3)
			return nil
		}); err != nil {
			t.Fatal(err)
		}
	}
	get := func() *pb.SCPetRecruitGet {
		t.Helper()
		out, err := s.P9PetRecruitInfo(ctx, row.Id)
		if err != nil {
			t.Fatal(err)
		}
		return out[0].(*pb.SCPetRecruitGet)
	}
	level(33)
	if get().GetRecruit().GetReward() != "" {
		t.Fatal("pre34 hint")
	}
	for _, n := range []int32{34, 75} {
		level(n)
		info := get().GetRecruit()
		if info.GetReward() != "pet_002,1" || info.GetNum() != 0 || info.GetMustTimes() != 0 || info.GetNextFree() != 0 {
			t.Fatalf("original first preview %+v", info)
		}
	}
	pets, err := P9PetList(db.GetIns(), row.Id)
	if err != nil || len(pets) != 0 {
		t.Fatal("preview granted instance")
	}
	if _, _, err = s.DispatchP9Pets(ctx, row.Id, &pb.CSPetRecruit{Free: proto.Int32(0), NumType: proto.Int32(1)}, 991000); err == nil {
		t.Fatal("ordinary recruited first reward")
	}
	out, _, err := s.DispatchP9Pets(ctx, row.Id, &pb.CSPetFirstRecruit{}, 991001)
	if err != nil {
		t.Fatal(err)
	}
	pet := out[0].(*pb.SCPetFirstRecruit).GetGet().GetPets()[0]
	if get().GetRecruit().GetReward() != "" {
		t.Fatal("first hint remained")
	}
	if _, _, err = s.DispatchP9PetRecycle(ctx, row.Id, &pb.CSPetSell{Guids: []string{pet.GetGuid()}}, 991002); err != nil {
		t.Fatal(err)
	}
	if get().GetRecruit().GetReward() != "" {
		t.Fatal("all pets sold restored first")
	}
	pool, _ := db.GetIns().DB()
	pool.Close()
	open()
	if get().GetRecruit().GetReward() != "" {
		t.Fatal("restart restored first")
	}
	var profile db.Profile
	if err = db.GetIns().First(&profile, row.Id).Error; err != nil {
		t.Fatal(err)
	}
	p, _, err := decodeProfile(&profile)
	if err != nil || p.GetNoviceGuideSchedule() != 3 {
		t.Fatal("first projection modified guide schedule")
	}
	if _, _, err = s.DispatchP9Pets(ctx, row.Id, &pb.CSPetFirstRecruit{}, 991003); err == nil {
		t.Fatal("second first original frame")
	}
}
