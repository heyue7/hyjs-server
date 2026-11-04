package service

import (
	"encoding/base64"
	"errors"
	"strings"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
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
	if len(packet.BossInfoList) != 5 || len(page.PendingIDs) != 4 || page.SelectedBossID != stages[0].TaskID || page.Offset != 0 || page.NextOffset != 5 {
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
	if packet.BossInfoList[0].GetIsOpen() != 1 {
		t.Fatal("selected original enemy not admitted", packet.BossInfoList[0])
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

	// A visible unselected enemy can be damaged or have an unchecked historical
	// prerequisite. Neither its blob nor its eligibility may block another choice.
	if stages[3].ConditionTaskID == "" {
		t.Fatal("fixture lacks original prerequisite")
	}
	for _, update := range []struct {
		id     string
		values map[string]any
	}{
		{stages[1].ID, map[string]any{"min_level": 23}},
		{stages[2].ID, map[string]any{"enemies": []byte("invalid enemy")}},
		{stages[3].ID, map[string]any{"condition_task_id": stages[3].ConditionTaskID}},
		{stages[4].ID, map[string]any{"enabled": false}},
	} {
		if err := db.GetIns().Model(&db.P7RushStageRule{}).Where("id=?", update.id).Updates(update.values).Error; err != nil {
			t.Fatal(err)
		}
	}
	if err := db.GetIns().Model(&db.P7RushState{}).Where("player_id=?", f.id).Update("high_floor", 100).Error; err != nil {
		t.Fatal(err)
	}
	var summaries []string
	var fullIDs []string
	historyReads := 0
	observe := false
	historyErr := errors.New("selected prerequisite history read")
	name := "p7-elite-page-demand"
	if err := db.GetIns().Callback().Query().Before("gorm:query").Register(name+"-history", func(tx *gorm.DB) {
		if observe && tx.Statement.Table == "p7_rush_fights" {
			historyReads++
			tx.AddError(historyErr)
		}
	}); err != nil {
		t.Fatal(err)
	}
	defer db.GetIns().Callback().Query().Remove(name + "-history")
	if err := db.GetIns().Callback().Query().After("gorm:query").Register(name+"-stage", func(tx *gorm.DB) {
		if !observe || tx.Statement.Table != "p7_rush_stage_rules" {
			return
		}
		switch rows := tx.Statement.Dest.(type) {
		case *[]db.P7RushStageRule:
			summaries = append(summaries, tx.Statement.SQL.String())
			for _, row := range *rows {
				if len(row.Enemies) != 0 {
					t.Error("summary loaded an enemy blob")
				}
			}
		case *db.P7RushStageRule:
			fullIDs = append(fullIDs, rows.ID)
		}
	}); err != nil {
		t.Fatal(err)
	}
	defer db.GetIns().Callback().Query().Remove(name + "-stage")
	observe = true
	page, err = f.s.P7ElitePage(f.ctx, f.id, owner.Session, nil, 5, "", "")
	observe = false
	if err != nil {
		t.Fatal("unselected entries affected the page", err)
	}
	packet = p7DecodeElitePage(t, page)
	if len(summaries) != 1 || len(fullIDs) != 1 || fullIDs[0] != stages[0].ID || historyReads != 0 {
		t.Fatalf("wrong stage reads: summaries=%v full=%v history=%d", summaries, fullIDs, historyReads)
	}
	selectList, _, ok := strings.Cut(strings.ToLower(summaries[0]), " from ")
	if !ok || strings.Contains(selectList, "*") || strings.Contains(selectList, "enemies") {
		t.Fatalf("summary fetched the full rule: %s", summaries[0])
	}
	if page.SelectedBossID != stages[0].TaskID || len(page.PendingIDs) != 2 || page.PendingIDs[0] != stages[2].TaskID || page.PendingIDs[1] != stages[3].TaskID {
		t.Fatal("incorrect pending selection", page)
	}
	for i, row := range packet.BossInfoList {
		if i != 0 && row.GetIsOpen() != 0 {
			t.Fatal("unselected or ineligible row claimed open", row)
		}
	}
	broken, err := f.s.P7ElitePage(f.ctx, f.id, owner.Session, nil, 5, stages[2].TaskID, "")
	if err != nil {
		t.Fatal("selected invalid enemy", err)
	}
	brokenPacket := p7DecodeElitePage(t, broken)
	if len(brokenPacket.BossInfoList) != 1 || brokenPacket.BossInfoList[0].GetIsOpen() != 0 {
		t.Fatal("selected invalid enemy was admitted", broken, err)
	}
	observe = true
	_, err = f.s.P7ElitePage(f.ctx, f.id, owner.Session, nil, 5, stages[3].TaskID, "")
	observe = false
	if !errors.Is(err, historyErr) || historyReads != 1 {
		t.Fatal("selected historical prerequisite error was not returned", historyReads, err)
	}
	summaries, fullIDs, historyReads = nil, nil, 0
	offset := int64(5)
	observe = true
	next, err := f.s.P7ElitePage(f.ctx, f.id, owner.Session, &offset, 5, "", stages[0].TaskID)
	observe = false
	if err != nil || next.SelectedBossID != stages[0].TaskID || len(fullIDs) != 0 || historyReads != 0 {
		t.Fatal("next window rechecked an old selection", next, fullIDs, historyReads, err)
	}
	if err := db.GetIns().Model(&db.P7RushStageRule{}).Where("id=?", stages[0].ID).Update("enabled", false).Error; err != nil {
		t.Fatal(err)
	}
	page, err = f.s.P7ElitePage(f.ctx, f.id, owner.Session, nil, 5, "", "")
	if err != nil || page.SelectedBossID != stages[2].TaskID || len(page.PendingIDs) != 1 || page.PendingIDs[0] != stages[3].TaskID {
		t.Fatal("default did not select the first basic candidate", page, err)
	}
	if err := db.GetIns().Model(&db.P7RushStageRule{}).Where("id IN ?", []string{stages[2].ID, stages[3].ID}).Update("enabled", false).Error; err != nil {
		t.Fatal(err)
	}
	page, err = f.s.P7ElitePage(f.ctx, f.id, owner.Session, nil, 5, "", "")
	if err != nil || page.SelectedBossID != stages[0].TaskID || len(page.PendingIDs) != 0 {
		t.Fatal("empty candidate window did not select the first row", page, err)
	}
	// A damaged reward outside the requested window cannot be read or parsed.
	if err := db.GetIns().Model(&db.P7RushStageRule{}).Where("id=?", stages[5].ID).Update("first_reward", "invalid-reward").Error; err != nil {
		t.Fatal(err)
	}
	if _, err = f.s.P7ElitePage(f.ctx, f.id, owner.Session, nil, 5, "", ""); err != nil {
		t.Fatal("unrequested row affected first page", err)
	}
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
