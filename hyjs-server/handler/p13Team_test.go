package handler

import (
	"google.golang.org/protobuf/proto"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
	"strings"
	"testing"
)

func TestP13TeamOriginalIngressPairedServersTargetsAndNativeZeroStamp(t *testing.T) {
	valid := []proto.Message{&pb.CSCreateTeam{}, &pb.CSQueryTeam{}, &pb.CSQueryTeam{TeamId: proto.String("actual")}, &pb.CSJoinTeam{TeamId: proto.String("actual")}, &pb.CSExitTeam{TeamId: proto.String("actual")}, &pb.CSDisbandTeam{TeamId: proto.String("actual")}, &pb.CSQueryTeamTick{}, &pb.CSChangeTeamLeader{TeamId: proto.String("actual"), ToPlayerId: proto.Int64(2)}, &pb.CSTeamKick{TreamId: proto.String("actual"), ToPlayerId: proto.Int64(2)}, &pb.CSCheckTeam{ToPlayerId: proto.Int64(2), ToServerid: proto.Int32(1)}, &pb.CSTeamInvite{TeamId: proto.String("actual"), InvitePlayerIdList: []int64{2, 3}, InviteServerIdList: []int32{1, 1}}, &pb.CSTeamCreateAndInvite{InvitePlayerIdList: []int64{2}, InviteServerIdList: []int32{1}}}
	for _, request := range valid {
		if err := ValidateGameRequest(&model.GamePacket{ZoneId: 1, PlayerId: 1, Timestamp: 0, Body: request}); err != nil {
			t.Fatalf("valid original TCP %T: %v", request, err)
		}
	}
	invalid := []proto.Message{&pb.CSQueryTeam{TeamId: proto.String(" padded ")}, &pb.CSQueryTeam{TeamId: proto.String(strings.Repeat("a", 65))}, &pb.CSJoinTeam{}, &pb.CSExitTeam{}, &pb.CSDisbandTeam{}, &pb.CSChangeTeamLeader{TeamId: proto.String("actual"), ToPlayerId: proto.Int64(1)}, &pb.CSTeamKick{TreamId: proto.String("actual"), ToPlayerId: proto.Int64(0)}, &pb.CSCheckTeam{ToPlayerId: proto.Int64(2), ToServerid: proto.Int32(2)}, &pb.CSTeamInvite{TeamId: proto.String("actual"), InvitePlayerIdList: []int64{2}, InviteServerIdList: nil}, &pb.CSTeamCreateAndInvite{InvitePlayerIdList: []int64{2}, InviteServerIdList: []int32{2}}, &pb.CSTeamCreateAndInvite{InvitePlayerIdList: []int64{2, 2}, InviteServerIdList: []int32{1, 1}}, &pb.CSTeamCreateAndInvite{InvitePlayerIdList: []int64{1}, InviteServerIdList: []int32{1}}, &pb.CSTeamCreateAndInvite{InvitePlayerIdList: []int64{2, 3, 4, 5, 6}, InviteServerIdList: []int32{1, 1, 1, 1, 1}}}
	for _, request := range invalid {
		if err := ValidateGameRequest(&model.GamePacket{ZoneId: 1, PlayerId: 1, Timestamp: 0, Body: request}); err == nil {
			t.Fatalf("invalid %T accepted", request)
		}
	}
	if err := ValidateGameRequest(&model.GamePacket{ZoneId: 1, PlayerId: 1, Timestamp: -1, Body: &pb.CSCreateTeam{}}); err == nil {
		t.Fatal("negative native stamp accepted")
	}
}
