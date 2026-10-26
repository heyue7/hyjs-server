package service

import (
	"encoding/base64"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

func p7DecodeElitePage(t *testing.T, response *model.P7ElitePageResponse) *pb.SCGetRushEliteBossInfo {
	t.Helper()
	raw, err := base64.StdEncoding.DecodeString(response.Packet)
	if err != nil {
		t.Fatal(err)
	}
	packet := &pb.SCGetRushEliteBossInfo{}
	if err = proto.Unmarshal(raw, packet); err != nil {
		t.Fatal(err)
	}
	return packet
}

func TestP7ElitePageWindowPendingSingleSelectionAndNoSpend(t *testing.T) {
	f := p7SectionFixture(t)
	var owner db.Profile
	if err := db.GetIns().First(&owner, f.id).Error; err != nil {
		t.Fatal(err)
	}
	// Finite configuration fixture: five distinct catalogue IDs share the
	// existing first enemy, without creating wins, progress, rewards or history.
	var stages []db.P7RushStageRule
	if err := db.GetIns().Where("kind=?", constUtil.P7RushElite).Order("task_id,id").Limit(6).Find(&stages).Error; err != nil || len(stages) < 6 {
		t.Fatal("existing catalogue", len(stages), err)
	}
	first := stages[0]
	var actual db.P7RushStageRule
	if err := db.GetIns().Where("kind=? AND source=?", constUtil.P7RushTrial, constUtil.P7SectionStageSource).Order("task_id,id").First(&actual).Error; err != nil {
		t.Fatal(err)
	}
	first.Enemies, first.Scene, first.MaxRound, first.WinKey, first.WinValue = actual.Enemies, actual.Scene, actual.MaxRound, actual.WinKey, actual.WinValue

	for i := range 5 {
		copy := first
		copy.ID, copy.TaskID, copy.ConditionTaskID = stages[i].ID, stages[i].TaskID, ""
		copy.Enabled, copy.MinLevel, copy.Source = true, 1, constUtil.P7SectionStageSource
		if err := db.GetIns().Save(&copy).Error; err != nil {
			t.Fatal(err)
		}
	}
	before, gameBefore := f.profile(t)
	start := time.Now()
	page, err := f.s.P7ElitePage(f.ctx, f.id, owner.Session, nil, 5, "", "")
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("first visible page: %s", time.Since(start))
	packet := p7DecodeElitePage(t, page)
	if len(packet.BossInfoList) != 5 || len(page.PendingIDs) != 4 || page.SelectedBossID != stages[4].TaskID || page.Offset != 0 || page.NextOffset != 5 {
		t.Fatalf("window=%+v packet=%v", page, packet)
	}
	pending := map[string]bool{}
	for _, id := range page.PendingIDs {
		pending[id] = true
	}
	for _, item := range packet.BossInfoList {
		if pending[item.GetEliteTaskId()] && item.GetIsOpen() != 0 {
			t.Fatal("unverified entry claimed open", item)
		}
	}
	if packet.BossInfoList[4].GetIsOpen() != 1 {
		t.Fatal("selected original enemy not admitted", packet.BossInfoList[4])
	}
	id := stages[1].TaskID
	detail, err := f.s.P7ElitePage(f.ctx, f.id, owner.Session, nil, 5, id, "")
	if err != nil {
		t.Fatal(err)
	}
	item := p7DecodeElitePage(t, detail)
	if len(item.BossInfoList) != 1 || item.BossInfoList[0].GetEliteTaskId() != id || item.BossInfoList[0].GetIsOpen() != 1 || len(detail.PendingIDs) != 0 || detail.Offset != -1 || detail.SelectedBossID != id {
		t.Fatal("single selection", detail, item)
	}
	// A damaged reward outside the requested window cannot be read or parsed.
	if err := db.GetIns().Model(&db.P7RushStageRule{}).Where("id=?", stages[5].ID).Update("first_reward", "invalid-reward").Error; err != nil {
		t.Fatal(err)
	}
	if _, err = f.s.P7ElitePage(f.ctx, f.id, owner.Session, nil, 5, "", ""); err != nil {
		t.Fatal("unrequested row affected first page", err)
	}
	offset := int64(5)
	if _, err = f.s.P7ElitePage(f.ctx, f.id, owner.Session, &offset, 5, "", ""); err == nil {
		t.Fatal("requested damaged reward was ignored")
	}
	if _, err = f.s.P7ElitePage(f.ctx, f.id, []byte("bad-session-0000"), nil, 5, "", ""); err == nil {
		t.Fatal("other session accepted")
	}
	after, gameAfter := f.profile(t)
	if !proto.Equal(before, after) || !p6CopySeriesJSONEqual(t, gameBefore, gameAfter) {
		t.Fatal("page or rejected query changed assets")
	}
}

func TestP7ElitePageTailMatchesOriginalClosedRows(t *testing.T) {
	f := p7SectionFixture(t)
	var owner db.Profile
	if err := db.GetIns().First(&owner, f.id).Error; err != nil {
		t.Fatal(err)
	}
	all, err := f.s.P7RushQuery(f.ctx, f.id, &pb.CSGetRushEliteBossInfo{}, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	original := all[0].(*pb.SCGetRushEliteBossInfo)
	for from := int64(0); from < int64(len(original.BossInfoList)); from += 5 {
		view, err := f.s.P7ElitePage(f.ctx, f.id, owner.Session, &from, 5, "", "")
		if err != nil {
			t.Fatal(err)
		}
		page := p7DecodeElitePage(t, view)
		to := min(int(from)+5, len(original.BossInfoList))
		if len(page.BossInfoList) != to-int(from) || view.Total != int64(len(original.BossInfoList)) || len(view.PendingIDs) != 0 {
			t.Fatal("page boundary", view, page)
		}
		for i, item := range page.BossInfoList {
			if !proto.Equal(item, original.BossInfoList[int(from)+i]) {
				t.Fatal("original closed catalogue row differs", item, original.BossInfoList[int(from)+i])
			}
		}
		if to == len(original.BossInfoList) && view.NextOffset != -1 {
			t.Fatal("last page still advertises more", view)
		}
	}
}
