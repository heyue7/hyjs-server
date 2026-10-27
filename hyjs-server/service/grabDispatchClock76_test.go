package service

import (
	"context"
	"testing"

	"google.golang.org/protobuf/proto"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	pb "hyjs-server/model/protocol"
)

func TestGrabDispatch76ListUsesSameGameDateAsOccupation(t *testing.T) {
	f, _, _ := p11ArenaSetup(t)
	if err := f.s.InitializeLocalPlayActivities(f.ctx); err != nil {
		t.Fatal(err)
	}
	if err := f.s.InitializeServerGameClock(f.ctx); err != nil {
		t.Fatal(err)
	}
	p11GrabPrepare(t, f, f.id)
	admin := adminWalletTestActor(t, f, constUtil.AdminRoleAdmin)
	ctx := wantedClock75Move(t, f, admin, -1)
	req := &pb.CSGrab{Index: proto.Int32(1)}
	ctx = context.WithValue(ctx, constUtil.GameRequestContextKey, req)
	before, _ := f.profile(t)
	occupied, err := f.s.p11GrabAt(ctx, f.id, req, 760101, LocalPlayNow(ctx))
	if err != nil {
		t.Fatal("original occupation", err)
	}
	if occupied[0].(*pb.SCGrab).GetGrabData().GetPlayerId() != f.id {
		t.Fatal("occupation missing")
	}
	after, _ := f.profile(t)
	if after.GetEnergy() != before.GetEnergy()-10 {
		t.Fatal("cost mismatch")
	}
	response, handled, err := f.s.DispatchP11GrabRandom(ctx, f.id, &pb.CSGetGrabData{}, 760102)
	if err != nil || !handled {
		t.Fatal("original list", handled, err)
	}
	list := response[0].(*pb.SCGetGrabData)
	if list.GetGrabData()[0].GetPlayerId() != f.id || list.GetGrabTimes() != 2 {
		t.Fatal("refresh prematurely expired paid slot or reset daily count", list.GetGrabData()[0], list.GetGrabTimes())
	}
	var slot db.P11GrabSlot
	if err = db.GetIns().First(&slot, "position=?", 1).Error; err != nil || slot.OwnerID == nil || *slot.OwnerID != f.id {
		t.Fatal("paid slot was erased", err)
	}
	last, _ := f.profile(t)
	if !proto.Equal(after, last) {
		t.Fatal("list changed player assets")
	}
}
