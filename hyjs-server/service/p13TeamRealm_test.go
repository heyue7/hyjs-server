package service

import (
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	pb "hyjs-server/model/protocol"
)

func TestP13TeamConfiguredRealmOriginalInviteJoinAndMemberIdentity(t *testing.T) {
	f, ids, recorders := p13TeamSetup(t)
	f.s.ServerID = 2
	p13TeamOpen(t)
	now := time.Now()
	for _, bad := range []int32{0, 1, 3} {
		if _, err := f.s.p13TeamQuery(f.ctx, ids[0], &pb.CSCheckTeam{ToPlayerId: proto.Int64(ids[1]), ToServerid: proto.Int32(bad)}); err == nil {
			t.Fatal("foreign numeric player ID accepted as local team candidate", bad)
		}
		if _, err := f.s.p13TeamAt(f.ctx, ids[0], &pb.CSTeamCreateAndInvite{InvitePlayerIdList: []int64{ids[1]}, InviteServerIdList: []int32{bad}}, 10+int64(bad), now); err == nil {
			t.Fatal("foreign team invitation accepted", bad)
		}
	}
	available := p13TeamCall(t, f, ids[0], &pb.CSCheckTeam{ToPlayerId: proto.Int64(ids[1]), ToServerid: proto.Int32(2)}, 0, now).(*pb.SCCheckTeam)
	if available.GetCheckResult() != 1 {
		t.Fatal("real local online player unavailable", available)
	}
	created := p13TeamCall(t, f, ids[0], &pb.CSTeamCreateAndInvite{InvitePlayerIdList: []int64{ids[1]}, InviteServerIdList: []int32{2}}, 20, now).(*pb.SCTeamCreateAndInvite)
	teamID := created.GetTeam().GetTeamId()
	if teamID == "" || created.GetTeam().GetMemberList()[0].GetServerId() != 2 {
		t.Fatal("real realm missing from created original team", created)
	}
	invites := p13TeamInvites(recorders[1].drain())
	if len(invites) != 1 || invites[0].GetTeam().GetMemberList()[0].GetServerId() != 2 {
		t.Fatal("target original push lost configured realm", invites)
	}
	p13TeamCall(t, f, ids[1], &pb.CSJoinTeam{TeamId: proto.String(teamID)}, 21, now)
	p13TeamCall(t, f, ids[0], &pb.CSTeamInvite{TeamId: proto.String(teamID), InvitePlayerIdList: []int64{ids[2]}, InviteServerIdList: []int32{2}}, 22, now)
	p13TeamCall(t, f, ids[2], &pb.CSJoinTeam{TeamId: proto.String(teamID)}, 23, now)
	p13TeamCall(t, f, ids[0], &pb.CSChangeTeamLeader{TeamId: proto.String(teamID), ToPlayerId: proto.Int64(ids[1])}, 24, now)
	query := p13TeamCall(t, f, ids[1], &pb.CSQueryTeam{TeamId: proto.String(teamID)}, 0, now).(*pb.SCQueryTeam)
	if len(query.GetTeam().GetMemberList()) != 3 || query.GetTeam().GetMemberList()[0].GetPlayerId() != ids[1] {
		t.Fatal("actual realm2 membership/leader changed", query)
	}
	for _, member := range query.GetTeam().GetMemberList() {
		if member.GetServerId() != 2 {
			t.Fatal("member relabeled as another realm", member)
		}
	}
	p13TeamCall(t, f, ids[1], &pb.CSTeamKick{TreamId: proto.String(teamID), ToPlayerId: proto.Int64(ids[2])}, 25, now)
	p13TeamCall(t, f, ids[0], &pb.CSExitTeam{TeamId: proto.String(teamID)}, 26, now)
	p13TeamCall(t, f, ids[1], &pb.CSDisbandTeam{TeamId: proto.String(teamID)}, 27, now)
	for _, id := range ids[:3] {
		p, _ := p10LuckyProfile(t, id)
		if p.GetTeamId() != "" {
			t.Fatal("real local team cleanup lost", id)
		}
	}
}
