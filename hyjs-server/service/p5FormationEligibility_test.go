package service

import (
	"context"
	"encoding/json"
	"fmt"
	"google.golang.org/protobuf/proto"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
	"path/filepath"
	"strings"
	"testing"
)

func TestP5OriginalGreenFormationDeniedWithoutMutation(t *testing.T) {
	ctx := context.Background()
	if err := db.InitGame(ctx, filepath.Join(t.TempDir(), "green-formation.db")); err != nil {
		t.Fatal(err)
	}
	defer func() { pool, _ := db.GetIns().DB(); pool.Close() }()
	s := &GameService{}
	for _, load := range []func(string) error{s.LoadTables, s.LoadP5Tables} {
		if err := load("../../analysis/tables"); err != nil {
			t.Fatal(err)
		}
	}
	for _, init := range []func(context.Context) error{s.InitializeP1, s.InitializeP5, s.InitializeGameplayPools, s.InitializeP2, s.InitializeOpenServerActivity, s.InitializeDailyTasks} {
		if err := init(ctx); err != nil {
			t.Fatal(err)
		}
	}
	s.P5Ready = true
	account, err := LoginAccount(ctx, "original-green-eligibility", "isolated-test-only")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = CreatePlayer(ctx, account.Id, "原绿色卡资格", "hero_043"); err != nil {
		t.Fatal(err)
	}
	var cards []*pb.Warrior
	if err = updateProfile(ctx, account.Id, func(p *pb.Player, g *model.GameState) error {
		p.Level = proto.Int32(100)
		p.WarriorBagSize = proto.Int32(1000)
		parts := []string{}
		for number := 56; number <= 61; number++ {
			parts = append(parts, fmt.Sprintf("hero_%03d,1", number))
		}
		parts = append(parts, "hero_032,1")
		gain, err := s.GrantFixedReward(p, g, strings.Join(parts, ";"))
		if err == nil {
			cards = gain.GetWars()
		}
		return err
	}); err != nil {
		t.Fatal(err)
	}
	before := func() string {
		t.Helper()
		var row db.Profile
		if err := db.GetIns().First(&row, account.Id).Error; err != nil {
			t.Fatal(err)
		}
		data, _ := json.Marshal(row)
		return string(data)
	}
	for _, card := range cards[:6] {
		if err := s.P5FormationHeroAllowed(card.GetId()); err == nil {
			t.Fatal("original green qualification accepted", card.GetId())
		}
		saved := before()
		if _, err := s.formationOn(ctx, account.Id, &pb.CSFormationOn{Id: card.Id, Guid: card.Guid, FormatPos: proto.Int32(2)}); err == nil {
			t.Fatal("original green actual formation request accepted", card.GetId())
		}
		if before() != saved {
			t.Fatal("denied green formation changed inventory or formation", card.GetId())
		}
	}
	blue := cards[6]
	if err := s.P5FormationHeroAllowed(blue.GetId()); err != nil {
		t.Fatal("original quality13 qualification", err)
	}
	if _, err := s.formationOn(ctx, account.Id, &pb.CSFormationOn{Id: blue.Id, Guid: blue.Guid, FormatPos: proto.Int32(2)}); err != nil {
		t.Fatal("original blue actual formation request", err)
	}
}
