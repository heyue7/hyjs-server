package service

import (
	"context"
	"encoding/json"
	"google.golang.org/protobuf/proto"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
	"testing"
)

func TestP7RushWarmHistoryCommitsPerSource(t *testing.T) {
	f, _, request := p7ScopedFixture(t, constUtil.P7RushTrial)
	p7DurableHistoryFixture(t, f.ctx)
	for _, table := range []any{&db.P7RushScopedPolicy{}, &db.P7RushManualPolicy{}} {
		if err := db.GetIns().Model(table).Where("id=1").Update("enabled", true).Error; err != nil {
			t.Fatal(err)
		}
	}
	start, err := f.s.p7Start(f.ctx, f.id, request, 83001, false)
	if err != nil {
		t.Fatal(err)
	}
	var manual db.P7RushManualCalculation
	if err = db.GetIns().First(&manual, "fight_id=?", start.GetFightId()).Error; err != nil {
		t.Fatal(err)
	}
	body, err := decodeP6StoredInput(manual.Series)
	if err != nil {
		t.Fatal(err)
	}
	var series model.FrozenP6RushSeries
	if err = json.Unmarshal(body, &series); err != nil {
		t.Fatal(err)
	}
	input, err := cloneP6CalculationInput(series.Template)
	if err != nil {
		t.Fatal(err)
	}
	input.DiagnosticAIDriverPolicy = constUtil.P6DiagnosticManualAutoStart
	input.DiagnosticManualAutoStartRoundTime = proto.Float64(30.100000000000158)
	calculated, err := f.s.ComputeOriginalP6ProtocolFinalHP(f.ctx, input, nil)
	if err != nil {
		t.Fatal(err)
	}
	result := int32(0)
	if calculated.Outcome.Winner == 0 {
		result = 1
	}
	if _, err = f.s.p7End(f.ctx, f.id, &pb.CSRushFightEnd{FightId: start.FightId, Result: proto.Int32(result), OptRecord: calculated.OriginalOperates}); err != nil {
		t.Fatal(err)
	}
	if err = db.GetIns().Where("fight_id=?", start.GetFightId()).Delete(&db.P7RushHistoryCertificate{}).Error; err != nil {
		t.Fatal(err)
	}
	// A later damaged source must fail without rolling back the previous valid proof.
	var bad db.P7RushFight
	if err = db.GetIns().First(&bad, "id=?", start.GetFightId()).Error; err != nil {
		t.Fatal(err)
	}
	bad.ID = "zzzz-invalid-later-source"
	bad.EndRequest = []byte("invalid-end")
	if err = db.GetIns().Create(&bad).Error; err != nil {
		t.Fatal(err)
	}
	if err = db.GetIns().First(&manual, "fight_id=?", start.GetFightId()).Error; err != nil {
		t.Fatal(err)
	}
	manual.FightID = bad.ID
	if err = db.GetIns().Create(&manual).Error; err != nil {
		t.Fatal(err)
	}
	before, beforeGame := f.profile(t)
	if _, err = WarmPendingP7RushHistory(f.ctx); err == nil {
		t.Fatal("damaged later source was accepted")
	}
	if p7HistoryCertificateCount(t, start.GetFightId()) != 1 || p7HistoryCertificateCount(t, bad.ID) != 0 {
		t.Fatal("valid proof rolled back or invalid proof published")
	}
	after, afterGame := f.profile(t)
	if !proto.Equal(before, after) || !p6CopySeriesJSONEqual(t, beforeGame, afterGame) {
		t.Fatal("warm changed profile or resources")
	}
	db.GetIns().Where("fight_id=?", bad.ID).Delete(&db.P7RushManualCalculation{})
	db.GetIns().Where("id=?", bad.ID).Delete(&db.P7RushFight{})
	if count, err := WarmPendingP7RushHistory(f.ctx); err != nil || count != 0 {
		t.Fatal("completed warm not resumable", count, err)
	}
	if err = WarmP7RushHistory(f.ctx, f.id); err != nil {
		t.Fatal("warm valid source", err)
	}
	foreign := &GameService{ServerID: 2}
	foreignCtx := context.WithValue(f.ctx, constUtil.GameServiceContextKey, foreign)
	if err = WarmP7RushHistory(foreignCtx, f.id); err == nil {
		t.Fatal("foreign owner accepted")
	}
}
