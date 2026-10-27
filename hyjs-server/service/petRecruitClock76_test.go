package service

import (
	"context"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

func TestPetRecruit76NegativeGameDateUsesEventClock(t *testing.T) {
	f, admin := wantedClock75Setup(t)
	if err := f.s.LoadP9Tables("../../analysis/tables"); err != nil {
		t.Fatal(err)
	}
	for _, init := range []func(context.Context) error{f.s.InitializeP9Pets, f.s.InitializeP9PetNext, f.s.InitializeP9PetIntimacy, f.s.InitializeP9CopyIntimacy} {
		if err := init(f.ctx); err != nil {
			t.Fatal(err)
		}
	}
	f.s.P1Ready = true
	if err := updateProfile(f.ctx, f.id, func(p *pb.Player, _ *model.GameState) error {
		p.Level = proto.Int32(35)
		p.Diamond = proto.Int64(1000)
		p.PetBagSize = proto.Int32(100)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	ctx := wantedClock75Move(t, f, admin, -1)
	first := &pb.CSPetFirstRecruit{}
	firstOut, err := f.s.p9PetFirst(context.WithValue(ctx, constUtil.GameRequestContextKey, first), f.id, 760010, first)
	if err != nil {
		t.Fatal("original first pet", err)
	}
	pet := firstOut[0].(*pb.SCPetFirstRecruit).GetGet().GetPets()[0]
	req := &pb.CSPetRecruit{Free: proto.Int32(0), NumType: proto.Int32(1)}
	requestCtx := context.WithValue(ctx, constUtil.GameRequestContextKey, req)
	before, _ := f.profile(t)
	out, err := f.s.p9PetRecruit(requestCtx, f.id, req, 760011)
	if err != nil {
		t.Fatal("paid original recruit on negative game day", err)
	}
	after, _ := f.profile(t)
	if after.GetDiamond() != before.GetDiamond()-150 {
		t.Fatal("incorrect original cost")
	}
	var fact db.EventFact
	if err = db.GetIns().Where("player_id=? AND source_key=? AND kind=?", f.id, "p9:recruit:760011", constUtil.P9RecruitEvent).First(&fact).Error; err != nil {
		t.Fatal(err)
	}
	if fact.Day != p1Day(LocalPlayNow(ctx)) || fact.OccurredAt > LocalPlayNow(ctx).Add(time.Minute).UnixMilli() {
		t.Fatal("recruit event not on game clock", fact)
	}
	again, err := f.s.p9PetRecruit(requestCtx, f.id, req, 760011)
	last, _ := f.profile(t)
	if err != nil || !proto.Equal(out[0], again[0]) || !proto.Equal(after, last) {
		t.Fatal("same request repeated paid recruit", err)
	}
	interaction := &pb.CSPetHudong{Guid: pet.Guid, Type: proto.Int32(0)}
	interactCtx := context.WithValue(ctx, constUtil.GameRequestContextKey, interaction)
	interacted, err := f.s.p9PetHudongAt(interactCtx, f.id, interaction, 760012, time.Now())
	if err != nil {
		t.Fatal("original interaction on negative game day", err)
	}
	var hudong db.EventFact
	if err = db.GetIns().Where("player_id=? AND source_key=? AND kind=?", f.id, "p9:hudong:760012", "pet_hudong").First(&hudong).Error; err != nil {
		t.Fatal(err)
	}
	if hudong.Day != p1Day(LocalPlayNow(ctx)) {
		t.Fatal("interaction event on wrong day", hudong)
	}
	againInteraction, err := f.s.p9PetHudongAt(interactCtx, f.id, interaction, 760012, time.Now())
	if err != nil || !proto.Equal(interacted[0], againInteraction[0]) {
		t.Fatal("interaction replay changed original response", err)
	}
}
