package service

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

func TestP9HudongOriginalTypesWindowCDRewardsReplayAndCold(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "hudong.db")
	if err := db.InitGame(ctx, path); err != nil {
		t.Fatal(err)
	}
	defer func() { sql, _ := db.GetIns().DB(); sql.Close() }()
	s := &GameService{}
	for _, load := range []func(string) error{s.LoadTables, s.LoadP9Tables} {
		if err := load("../../analysis/tables"); err != nil {
			t.Fatal(err)
		}
	}
	for _, init := range []func(context.Context) error{s.InitializeP1, s.InitializeP9Pets, s.InitializeP9PetNext, s.InitializeP9PetIntimacy, s.InitializeP9CopyIntimacy} {
		if err := init(ctx); err != nil {
			t.Fatal(err)
		}
	}
	account, err := LoginAccount(ctx, "hudong-native-types", "isolated-only")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := CreatePlayer(ctx, account.Id, "原三种互动", "hero_043"); err != nil {
		t.Fatal(err)
	}
	if err := updateProfile(ctx, account.Id, func(p *pb.Player, _ *model.GameState) error { p.Level = proto.Int32(34); return nil }); err != nil {
		t.Fatal(err)
	}
	first, err := s.p9PetFirst(ctx, account.Id, 1, &pb.CSPetFirstRecruit{})
	if err != nil {
		t.Fatal(err)
	}
	pet := first[0].(*pb.SCPetFirstRecruit).GetGet().GetPets()[0]
	shown, err := s.p9PetHudongShow(ctx, account.Id, &pb.CSPetHudongShow{Guid: pet.Guid})
	if err != nil {
		t.Fatal(err)
	}
	pet = shown[0].(*pb.SCPetHudongShow).GetPet()
	favorite := pet.GetEat().GetCritTrend()
	if favorite < 0 || favorite > 2 || len(pet.GetEat().GetHdTimeList()) != 3 {
		t.Fatal("original preference/counter projection")
	}
	now := time.Now()
	stamp := int64(2)
	request := &pb.CSPetHudong{Guid: pet.Guid, Type: proto.Int32(favorite)}
	var wg sync.WaitGroup
	errs := make(chan error, 8)
	replies := make(chan *pb.SCPetHudong, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			out, err := s.p9PetHudongAt(ctx, account.Id, request, stamp, now)
			errs <- err
			if err == nil {
				replies <- out[0].(*pb.SCPetHudong)
			}
		}()
	}
	wg.Wait()
	close(errs)
	close(replies)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	var result *pb.SCPetHudong
	for r := range replies {
		if result == nil {
			result = r
		}
		if !proto.Equal(result, r) {
			t.Fatal("same frame rerolled interaction")
		}
	}
	if result.GetPet().GetEat().GetExp() != 6 || result.GetPet().GetEat().GetHdTimeList()[favorite] != 1 || result.GetPet().GetEat().GetCritedValue() != 6 {
		t.Fatal("favorite double/source counts")
	}
	sql, _ := db.GetIns().DB()
	sql.Close()
	if err := db.InitGame(ctx, path); err != nil {
		t.Fatal(err)
	}
	cold, err := s.p9PetHudongAt(ctx, account.Id, request, stamp, now.Add(time.Second))
	if err != nil || !proto.Equal(cold[0], result) {
		t.Fatal("cold same interaction", err)
	}
	// The domain clock fixture advances multiple six-hour boundaries. P1 refuses
	// future-dated facts; native HTTP below separately verifies real-time facts.
	s.P1Ready = false
	stamp++
	if _, err := s.p9PetHudongAt(ctx, account.Id, request, stamp, now.Add(time.Second)); err == nil {
		t.Fatal("server5second cooldown bypass")
	}
	at := now
	call := func(kind int32) *pb.Pet {
		t.Helper()
		at = at.Add(6 * time.Second)
		stamp++
		out, err := s.p9PetHudongAt(ctx, account.Id, &pb.CSPetHudong{Guid: pet.Guid, Type: proto.Int32(kind)}, stamp, at)
		if err != nil {
			t.Fatal(err)
		}
		return out[0].(*pb.SCPetHudong).GetPet()
	}
	for kind := int32(0); kind < 3; kind++ {
		initial := int32(0)
		if kind == favorite {
			initial = 1
		}
		for n := initial; n < 5; n++ {
			pet = call(kind)
		}
	}
	if pet.GetEat().GetExp() != 60 {
		t.Fatal("all three first-five grants", pet.GetEat().GetExp())
	}
	for kind := int32(0); kind < 3; kind++ {
		for n := 0; n < 5; n++ {
			pet = call(kind)
			if pet.GetEat().GetCritedValue() != 0 {
				t.Fatal("attempt6..10 grew intimacy")
			}
		}
	}
	if pet.GetEat().GetExp() != 60 {
		t.Fatal("zero phase changed experience")
	}
	stamp++
	if _, err := s.p9PetHudongAt(ctx, account.Id, request, stamp, at.Add(6*time.Second)); err == nil {
		t.Fatal("attempt11 accepted")
	}
	oldStamp := int64(2)
	if _, err := s.p9PetHudongAt(ctx, account.Id, request, oldStamp, at); err == nil {
		t.Fatal("old complete pet projection returned after later attempts")
	}
	var state db.P9PetHudongState
	if err := db.GetIns().First(&state, "guid=?", pet.GetGuid()).Error; err != nil {
		t.Fatal(err)
	}
	if state.Earned != 60 || state.Applied != 60 || state.Capped != 0 {
		t.Fatal("free experience journal conservation")
	}
	// Live changes apply on the next anchored window, current input remains frozen.
	if err := db.GetIns().Model(&db.P9PetHudongRule{}).Where("id=?", constUtil.P9HudongRuleID).Update("input", []byte(`{}`)).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.GetIns().Transaction(func(tx *gorm.DB) error {
		owned, err := p9OwnedPet(tx, account.Id, pet.GetGuid())
		if err != nil {
			return err
		}
		_, _, err = s.p9HudongState(tx, account.Id, owned, at)
		return err
	}); err != nil {
		t.Fatal("hot edit changed current frozen preference", err)
	}
	var cfg p9HudongConfig
	if err := json.Unmarshal(state.Input, &cfg); err != nil {
		t.Fatal(err)
	}
	if err := db.GetIns().Model(&db.P9PetHudongRule{}).Where("id=?", constUtil.P9HudongRuleID).Update("input", state.Input).Error; err != nil {
		t.Fatal(err)
	}
	next := time.UnixMilli(state.WindowAt + 6*3600000 + 10000)
	stamp++
	out, err := s.p9PetHudongAt(ctx, account.Id, &pb.CSPetHudong{Guid: pet.Guid, Type: proto.Int32(0)}, stamp, next)
	if err != nil {
		t.Fatal(err)
	}
	updated := out[0].(*pb.SCPetHudong).GetPet()
	if updated.GetEat().GetHdTimeList()[0] != 1 || updated.GetEat().GetHdTimeList()[1] != 0 || updated.GetEat().GetHdTimeList()[2] != 0 || updated.GetEat().GetExp() <= 60 {
		t.Fatal("six-hour resets counts only")
	}
	// The final original layer caps excess free experience without minting refunds.
	rollback := errors.New("rollback isolated boundary allocation")
	if err := db.GetIns().Transaction(func(tx *gorm.DB) error {
		boundary := proto.Clone(updated).(*pb.Pet)
		boundary.Guid = proto.String("hudong-terminal-isolated")
		boundary.RandPropList = nil
		boundary.Eat = &pb.PetEat{Level: proto.Int32(6), Exp: proto.Int32(12406)}
		applied, capped, err := s.p9HudongGain(tx, account.Id, boundary, 6)
		if err != nil || applied != 2 || capped != 4 || boundary.GetEat().GetLevel() != 7 || boundary.GetEat().GetExp() != 0 || len(boundary.GetRandPropList()) != 3 {
			t.Fatalf("original quality20 terminal cap: applied=%d capped=%d pet=%v err=%v", applied, capped, boundary, err)
		}
		if _, _, err := s.p9HudongGain(tx, account.Id, boundary, 3); err == nil {
			t.Fatal("max intimacy accepted more free XP")
		}
		return rollback
	}); !errors.Is(err, rollback) {
		t.Fatal(err)
	}
	if err := db.GetIns().Model(&db.P9PetHudongRule{}).Where("id=?", constUtil.P9HudongRuleID).Update("enabled", false).Error; err != nil {
		t.Fatal(err)
	}
	stamp++
	if _, err := s.p9PetHudongAt(ctx, account.Id, request, stamp, next.Add(6*time.Second)); err == nil {
		t.Fatal("disabled fresh interaction")
	}
	t.Log("original0/1/2, favorite3x2, first5 and zero6..10,5s CD, all30 attempts, window freeze, receipt epochs/cold and shutdown passed")
}
