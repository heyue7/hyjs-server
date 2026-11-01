package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
	"hyjs-server/db"
	pb "hyjs-server/model/protocol"
)

type p13ReinviteFailReceiptKey struct{}

func p13ReinviteOne(t *testing.T, rec *p10SceneRecorder, team string, inviter int64) *pb.SCTeamCreateAndInvite {
	t.Helper()
	invites := p13TeamInvites(rec.drain())
	if len(invites) != 1 || invites[0].GetTeam().GetTeamId() != team || invites[0].GetInvitePlayerId() != inviter {
		t.Fatalf("expected one invitation for team %q from %d: %+v", team, inviter, invites)
	}
	return invites[0]
}

func p13ReinvitePending(t *testing.T, team string, target int64, message *pb.SCTeamCreateAndInvite) p13TeamPush {
	t.Helper()
	var invitation db.P13TeamInvite
	if err := db.GetIns().First(&invitation, "team_id=? AND target_id=?", team, target).Error; err != nil {
		t.Fatal(err)
	}
	return p13TeamPush{target: target, teamID: team, generation: invitation.TargetGeneration, invitation: true,
		authorityVersion: invitation.AuthorityVersion, inviterGeneration: invitation.InviterGeneration, message: message}
}

func TestP13TeamReinviteAfterDeclineOrLostDeliveryAndReceiptReplay(t *testing.T) {
	f, ids, recs := p13TeamSetup(t)
	p13TeamOpen(t)
	now := time.Now()
	request := &pb.CSTeamCreateAndInvite{InvitePlayerIdList: []int64{ids[1]}, InviteServerIdList: []int32{1}}
	created := p13TeamCall(t, f, ids[0], request, 0, now).(*pb.SCTeamCreateAndInvite)
	team := created.GetTeam().GetTeamId()
	if team == "" {
		t.Fatal("native invitation did not create a team")
	}
	p13ReinviteOne(t, recs[1], team, ids[0])
	var original db.P13TeamInvite
	if err := db.GetIns().First(&original, "team_id=? AND target_id=?", team, ids[1]).Error; err != nil {
		t.Fatal(err)
	}
	// Both a native decline and a lost popup leave no response at the server.
	// Each new zero-stamp TCP request must deliver again without a new invite row.
	for i := 0; i < 2; i++ {
		result := p13TeamCall(t, f, ids[0], request, 0, now.Add(time.Duration(i+1)*time.Second)).(*pb.SCTeamCreateAndInvite)
		if result.GetTeam().GetTeamId() != team {
			t.Fatal("reinvitation created another team", result)
		}
		p13ReinviteOne(t, recs[1], team, ids[0])
	}
	var current db.P13TeamInvite
	if err := db.GetIns().First(&current, "team_id=? AND target_id=?", team, ids[1]).Error; err != nil || current != original {
		t.Fatalf("reinvitation changed stored identity: before=%+v after=%+v err=%v", original, current, err)
	}
	var count int64
	if err := db.GetIns().Model(&db.P13TeamInvite{}).Where("team_id=? AND target_id=?", team, ids[1]).Count(&count).Error; err != nil || count != 1 {
		t.Fatalf("reinvitation duplicated stored invitation: count=%d err=%v", count, err)
	}

	// A new positive stamp redelivers; an identical receipt replay has no push.
	first := p13TeamCall(t, f, ids[0], request, 55, now)
	p13ReinviteOne(t, recs[1], team, ids[0])
	replayed := p13TeamCall(t, f, ids[0], request, 55, now.Add(time.Second))
	if !proto.Equal(first, replayed) || len(p13TeamInvites(recs[1].drain())) != 0 {
		t.Fatal("receipt replay changed the response or resent an invitation")
	}
	conflict := &pb.CSTeamCreateAndInvite{InvitePlayerIdList: []int64{ids[2]}, InviteServerIdList: []int32{1}}
	if _, err := f.s.p13TeamAt(f.ctx, ids[0], conflict, 55, now); err == nil {
		t.Fatal("different request reused a positive receipt stamp")
	}
	if len(p13TeamInvites(recs[2].drain())) != 0 {
		t.Fatal("conflicting receipt pushed an invitation")
	}

	join := &pb.CSJoinTeam{TeamId: proto.String(team)}
	p13TeamCall(t, f, ids[1], join, 0, now)
	p13TeamCall(t, f, ids[1], join, 0, now)
	members, err := p13TeamMembers(db.GetIns(), team)
	if err != nil || len(members) != 2 || members[0].PlayerID != ids[0] || members[1].PlayerID != ids[1] {
		t.Fatalf("accept or retry created duplicate membership: members=%+v err=%v", members, err)
	}
	player, _ := p10LuckyProfile(t, ids[1])
	if player.GetTeamId() != team {
		t.Fatal("accepted team did not reach the actual player profile", player)
	}
	if _, err := f.s.p13TeamAt(f.ctx, ids[0], request, 0, now); err == nil {
		t.Fatal("already joined player accepted another invitation")
	}
	if len(p13TeamInvites(recs[1].drain())) != 0 {
		t.Fatal("already joined player received another invitation")
	}
}

func TestP13TeamReinviteRollbackAndReconnect(t *testing.T) {
	f, ids, recs := p13TeamSetup(t)
	p13TeamOpen(t)
	now := time.Now()
	request := &pb.CSTeamCreateAndInvite{InvitePlayerIdList: []int64{ids[1]}, InviteServerIdList: []int32{1}}
	team := p13TeamCall(t, f, ids[0], request, 0, now).(*pb.SCTeamCreateAndInvite).GetTeam().GetTeamId()
	p13ReinviteOne(t, recs[1], team, ids[0])
	var original db.P13TeamInvite
	if err := db.GetIns().First(&original, "team_id=? AND target_id=?", team, ids[1]).Error; err != nil {
		t.Fatal(err)
	}
	key := p10ClientKey{f.s.p10ClientOwner(), ids[3]}
	value, ok := p10LiveClient(key)
	if !ok {
		t.Fatal("test target did not connect")
	}
	p10MarkDisconnected(value.(*p10ClientSink), false)
	// The valid first invitation is queued and the second is inserted before
	// the offline last target forces the complete transaction to roll back.
	mixed := &pb.CSTeamCreateAndInvite{InvitePlayerIdList: []int64{ids[1], ids[2], ids[3]}, InviteServerIdList: []int32{1, 1, 1}}
	if _, err := f.s.p13TeamAt(f.ctx, ids[0], mixed, 0, now); err == nil {
		t.Fatal("offline target did not abort invitation transaction")
	}
	for i := 1; i <= 3; i++ {
		if len(p13TeamInvites(recs[i].drain())) != 0 {
			t.Fatal("rolled-back transaction sent an invitation", ids[i])
		}
	}
	var current db.P13TeamInvite
	if err := db.GetIns().First(&current, "team_id=? AND target_id=?", team, ids[1]).Error; err != nil || current != original {
		t.Fatalf("rollback changed existing invitation: %+v %v", current, err)
	}
	var count int64
	if err := db.GetIns().Model(&db.P13TeamInvite{}).Where("team_id=? AND target_id=?", team, ids[2]).Count(&count).Error; err != nil || count != 0 {
		t.Fatalf("rollback kept a newly inserted invitation: count=%d err=%v", count, err)
	}

	// Receipt failure occurs after both invitations are queued; neither the
	// queued push nor the newly inserted row may escape the transaction.
	injected := errors.New("injected team receipt failure")
	database := db.GetIns()
	const callback = "test:p13-reinvite-receipt-failure"
	if err := database.Callback().Create().Before("gorm:create").Register(callback, func(tx *gorm.DB) {
		if tx.Statement.Table == "p13_team_receipts" && tx.Statement.Context.Value(p13ReinviteFailReceiptKey{}) == true {
			tx.AddError(injected)
		}
	}); err != nil {
		t.Fatal(err)
	}
	registered := true
	t.Cleanup(func() {
		if registered {
			_ = database.Callback().Create().Remove(callback)
		}
	})
	withReceipt := &pb.CSTeamCreateAndInvite{InvitePlayerIdList: []int64{ids[1], ids[2]}, InviteServerIdList: []int32{1, 1}}
	failedCtx := context.WithValue(f.ctx, p13ReinviteFailReceiptKey{}, true)
	_, receiptErr := f.s.p13TeamAt(failedCtx, ids[0], withReceipt, 91, now)
	if err := database.Callback().Create().Remove(callback); err != nil {
		t.Fatal(err)
	}
	registered = false
	if !errors.Is(receiptErr, injected) {
		t.Fatalf("receipt failure was not returned: %v", receiptErr)
	}
	for i := 1; i <= 2; i++ {
		if len(p13TeamInvites(recs[i].drain())) != 0 {
			t.Fatal("failed receipt sent a queued invitation", ids[i])
		}
	}
	if err := database.Model(&db.P13TeamReceipt{}).Where("player_id=? AND stamp=?", ids[0], 91).Count(&count).Error; err != nil || count != 0 {
		t.Fatalf("failed receipt persisted: count=%d err=%v", count, err)
	}
	if err := database.Model(&db.P13TeamInvite{}).Where("team_id=? AND target_id=?", team, ids[2]).Count(&count).Error; err != nil || count != 0 {
		t.Fatalf("failed receipt kept a new invitation: count=%d err=%v", count, err)
	}
	first := p13TeamCall(t, f, ids[0], withReceipt, 91, now)
	p13ReinviteOne(t, recs[1], team, ids[0])
	p13ReinviteOne(t, recs[2], team, ids[0])
	replayed := p13TeamCall(t, f, ids[0], withReceipt, 91, now)
	if !proto.Equal(first, replayed) || len(p13TeamInvites(recs[1].drain())) != 0 || len(p13TeamInvites(recs[2].drain())) != 0 {
		t.Fatal("receipt replay after rollback changed response or resent invitations")
	}

	// A pending invitation survives disconnect. A new request is rejected until
	// a real socket with the current session is registered again.
	key = p10ClientKey{f.s.p10ClientOwner(), ids[1]}
	value, ok = p10LiveClient(key)
	if !ok {
		t.Fatal("invited player did not connect")
	}
	p10MarkDisconnected(value.(*p10ClientSink), false)
	if _, err := f.s.p13TeamAt(f.ctx, ids[0], request, 0, now); err == nil {
		t.Fatal("offline player was reinvited")
	}
	if len(p13TeamInvites(recs[1].drain())) != 0 {
		t.Fatal("offline player received an invitation")
	}
	var profile db.Profile
	if err := db.GetIns().First(&profile, ids[1]).Error; err != nil {
		t.Fatal(err)
	}
	reconnected := &p10SceneRecorder{}
	detach := f.s.RegisterP10Client(f.ctx, ids[1], profile.Session, reconnected.send)
	t.Cleanup(detach)
	p13TeamCall(t, f, ids[0], request, 0, now)
	p13ReinviteOne(t, reconnected, team, ids[0])
	if len(p13TeamInvites(recs[1].drain())) != 0 {
		t.Fatal("reinvitation reached the disconnected socket")
	}
}

func TestP13TeamReinvitePendingPushRequiresOriginalAuthorityAndGenerations(t *testing.T) {
	f, ids, recs := p13TeamSetup(t)
	p13TeamOpen(t)
	now := time.Now()
	request := &pb.CSTeamCreateAndInvite{InvitePlayerIdList: []int64{ids[1], ids[2]}, InviteServerIdList: []int32{1, 1}}
	team := p13TeamCall(t, f, ids[0], request, 0, now).(*pb.SCTeamCreateAndInvite).GetTeam().GetTeamId()
	p13ReinviteOne(t, recs[1], team, ids[0])
	oldMessage := p13ReinviteOne(t, recs[2], team, ids[0])
	oldPush := p13ReinvitePending(t, team, ids[2], oldMessage)

	// An unchanged committed invitation is still eligible for delivery.
	f.s.p13TeamPublish(f.ctx, ids[0], []p13TeamPush{oldPush})
	p13ReinviteOne(t, recs[2], team, ids[0])
	p13TeamCall(t, f, ids[1], &pb.CSJoinTeam{TeamId: proto.String(team)}, 0, now)
	p13TeamCall(t, f, ids[0], &pb.CSChangeTeamLeader{TeamId: proto.String(team), ToPlayerId: proto.Int64(ids[1])}, 0, now)
	if _, err := f.s.p13TeamAt(f.ctx, ids[2], &pb.CSJoinTeam{TeamId: proto.String(team)}, 0, now); err == nil {
		t.Fatal("old leader's invitation remained valid after transfer")
	}
	if _, err := f.s.p13TeamAt(f.ctx, ids[0], request, 0, now); err == nil {
		t.Fatal("previous leader retained invite authority")
	}

	newRequest := &pb.CSTeamInvite{TeamId: proto.String(team), InvitePlayerIdList: []int64{ids[2]}, InviteServerIdList: []int32{1}}
	p13TeamCall(t, f, ids[1], newRequest, 0, now)
	p13ReinviteOne(t, recs[2], team, ids[1])
	f.s.p13TeamPublish(f.ctx, ids[0], []p13TeamPush{oldPush})
	if len(p13TeamInvites(recs[2].drain())) != 0 {
		t.Fatal("old leader's pending push borrowed the new invitation")
	}

	// Returning leadership to the same character still changes the authority
	// version; a freshly valid invitation must not revive the old frame.
	p13TeamCall(t, f, ids[1], &pb.CSChangeTeamLeader{TeamId: proto.String(team), ToPlayerId: proto.Int64(ids[0])}, 0, now)
	p13TeamCall(t, f, ids[0], newRequest, 0, now)
	currentMessage := p13ReinviteOne(t, recs[2], team, ids[0])
	currentPush := p13ReinvitePending(t, team, ids[2], currentMessage)
	f.s.p13TeamPublish(f.ctx, ids[0], []p13TeamPush{oldPush})
	if len(p13TeamInvites(recs[2].drain())) != 0 {
		t.Fatal("old authority version revived after leadership returned")
	}
	staleInviter := currentPush
	staleInviter.inviterGeneration--
	f.s.p13TeamPublish(f.ctx, ids[0], []p13TeamPush{staleInviter})
	if len(p13TeamInvites(recs[2].drain())) != 0 {
		t.Fatal("stale inviter generation was delivered")
	}

	// A disconnect between commit and publication cannot receive the frame.
	// After reauthentication the same still-valid invite uses the new socket.
	key := p10ClientKey{f.s.p10ClientOwner(), ids[2]}
	value, ok := p10LiveClient(key)
	if !ok {
		t.Fatal("pending invitation target did not connect")
	}
	p10MarkDisconnected(value.(*p10ClientSink), false)
	f.s.p13TeamPublish(f.ctx, ids[0], []p13TeamPush{currentPush})
	if len(p13TeamInvites(recs[2].drain())) != 0 {
		t.Fatal("pending invitation reached a disconnected socket")
	}
	var profile db.Profile
	if err := db.GetIns().First(&profile, ids[2]).Error; err != nil {
		t.Fatal(err)
	}
	reconnected := &p10SceneRecorder{}
	detach := f.s.RegisterP10Client(f.ctx, ids[2], profile.Session, reconnected.send)
	t.Cleanup(detach)
	f.s.p13TeamPublish(f.ctx, ids[0], []p13TeamPush{currentPush})
	p13ReinviteOne(t, reconnected, team, ids[0])
	if len(p13TeamInvites(recs[2].drain())) != 0 {
		t.Fatal("pending invitation reached the replaced socket")
	}
	recs[2] = reconnected

	// Join and leave changes the target generation even if the same leader
	// invites this character again immediately.
	p13TeamCall(t, f, ids[2], &pb.CSJoinTeam{TeamId: proto.String(team)}, 0, now)
	p13TeamCall(t, f, ids[2], &pb.CSExitTeam{TeamId: proto.String(team)}, 0, now)
	p13TeamCall(t, f, ids[0], newRequest, 0, now)
	p13ReinviteOne(t, recs[2], team, ids[0])
	f.s.p13TeamPublish(f.ctx, ids[0], []p13TeamPush{currentPush})
	if len(p13TeamInvites(recs[2].drain())) != 0 {
		t.Fatal("old target generation was delivered")
	}
}
