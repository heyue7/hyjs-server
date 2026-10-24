package service

import (
	"context"
	"strings"
	"testing"

	"google.golang.org/protobuf/proto"
	"hyjs-server/constUtil"
	pb "hyjs-server/model/protocol"
)

func TestP4NativeBoonInfoOriginal57053AndActualRotation(t *testing.T) {
	f, id := localPlayActivitiesFixture(t)
	before, beforeGame := f.profile(t)
	if _, err := f.s.LocalPlayActivitiesApply(f.ctx, id, "open", "CopyBenefits", "e1000000-0000-4000-8000-000000000001"); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{constUtil.P4CopyBenefitsActivityID, "warriorWeightImprove"} {
		messages, handled, err := f.s.DispatchP4(f.ctx, id, &pb.CSCopyBoonInfo{ActivityId: proto.String(key)}, 1)
		if err != nil || !handled || len(messages) != 1 {
			t.Fatal("original57053 was not dispatched", key, err)
		}
		info, ok := messages[0].(*pb.SCCopyBoonInfo)
		if !ok || info.GetInformation() == "" {
			t.Fatal("original57054 response missing", key)
		}
		if key == constUtil.P4CopyBenefitsActivityID && info.GetActivityId() != "copyDouble" {
			t.Fatal("original main-copy navigation key", info.GetActivityId())
		}
		if key == "warriorWeightImprove" && (!strings.Contains(info.GetInformation(), "%") || info.GetActivityId() != "warriorWeightImprove") {
			t.Fatal("actual pool probability or safe original navigation missing", info)
		}
	}
	tabs, err := f.s.P4ActivityTabs(f.ctx, LocalPlayNow(f.ctx))
	if err != nil {
		t.Fatal(err)
	}
	count := 0
	for _, tab := range tabs {
		if tab.GetActivityId() == "warriorWeightImprove" {
			count++
		}
	}
	if count != 0 {
		t.Fatal("strong recruit must stay in the shop role-recruit entry and out of activity tabs", count)
	}
	after, afterGame := f.profile(t)
	if !proto.Equal(before, after) || len(beforeGame.Warriors) != len(afterGame.Warriors) || len(beforeGame.Equipment) != len(afterGame.Equipment) {
		t.Fatal("native description query changed original inventory")
	}
	privateKey := "allCopyDouble_p4_" + localPlayCycleID(t, f, id, "CopyBenefits")
	foreign := context.WithValue(f.ctx, constUtil.LocalPlayOwnerKey, id+1)
	if _, err := f.s.P4BoonInfo(foreign, &pb.CSCopyBoonInfo{ActivityId: proto.String(privateKey)}, LocalPlayNow(f.ctx)); err == nil {
		t.Fatal("foreign personal benefit description accepted")
	}
	if _, err := f.s.P4BoonInfo(f.ctx, &pb.CSCopyBoonInfo{ActivityId: proto.String("unknown_boon")}, LocalPlayNow(f.ctx)); err == nil {
		t.Fatal("unknown activity reported as supported")
	}
}
