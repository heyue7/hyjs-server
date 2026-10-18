package service

import (
	"bytes"
	"context"
	"encoding/json"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	"os"
	"strings"
	"testing"
	"time"
)

func TestP14FinalResultCompressedSnapshotBoundsAndIntegrity(t *testing.T) {
	body := &p14FinalResultBody{Input: []byte("actual input transported separately from tables"), Terminal: []byte("finite codec integrity fixture, no battle result")}
	encoded, err := p14FinalResultEncode(body)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := p14FinalResultDecode(encoded)
	if err != nil || !bytes.Equal(decoded.Input, body.Input) || !bytes.Equal(decoded.Terminal, body.Terminal) {
		t.Fatal("codec changed bytes", err)
	}
	var blob p14FinalResultBlob
	if json.Unmarshal(encoded, &blob) != nil {
		t.Fatal("blob")
	}
	for _, change := range []func(*p14FinalResultBlob){
		func(b *p14FinalResultBlob) { b.DecodedSize = constUtil.P14FinalResultMaxDecodedBytes + 1 },
		func(b *p14FinalResultBlob) { b.ContentHash = strings.Repeat("0", 64) },
		func(b *p14FinalResultBlob) { b.Codec = "unknown" },
		func(b *p14FinalResultBlob) { b.Data = []byte("invalid gzip") },
	} {
		copy := blob
		change(&copy)
		raw, _ := json.Marshal(copy)
		if _, err = p14FinalResultDecode(raw); err == nil {
			t.Fatal("corrupt or oversized source accepted")
		}
	}
}

// These two file fixtures reuse the preserved real157 input and separately named
// scheduling policy. They prove actual computation/import/cold ownership and
// never claim a newly played signup or server qualification in this domain test.
func TestP14FinalResultActualCanonicalOriginSignedPeerColdNoP6Alias(t *testing.T) {
	if os.Getenv("HYJS_P13_MYSQL_DATABASE") != "" {
		t.Skip("two-file domain fixture; managed dual-process HTTP separate")
	}
	if constUtil.P6KFCanonicalNativeProof == "" {
		t.Skip("independent original KF certificate is required for positive source")
	}
	a, b, series, manifest, at := p14CurrentFinalSettlementFixture(t)
	p14PerGameSwitch(t, a)
	for _, init := range []func(context.Context) error{a.s.InitializeP6, a.s.InitializeP14FinalSettlement, a.s.InitializeP14FinalResult} {
		if err := init(a.ctx); err != nil {
			t.Fatal(err)
		}
	}
	tx := db.GetIns()
	for _, table := range []any{&db.P14FinalSettlementPolicy{}, &db.P14FinalResultPolicy{}} {
		if err := tx.Model(table).Where("id=1").Updates(map[string]any{"enabled": true, "version": "domain-published-canonical-result-v1", "published_at": manifest.FormationOffer.StageStart - 1}).Error; err != nil {
			t.Fatal(err)
		}
	}
	if err := a.s.SettleP14FinalGameOne(a.ctx, series.ID, at); err != nil {
		var gate db.P14FinalSettlementPolicy
		tx.First(&gate, 1)
		t.Fatalf("%v, gate valid=%v scope=%s realm=%d expected=%d", err, p14FinalSettlementPolicyValid(&gate, manifest), gate.ScopeProof, a.s.OriginalServerID(), manifest.Pair.LeftServerID)
	}
	if err := a.s.QueueP14FinalResults(a.ctx, at); err != nil {
		t.Fatal(err)
	}
	var delivery db.P14FinalResultDelivery
	if err := tx.First(&delivery).Error; err != nil {
		t.Fatal(err)
	}
	var request model.P14FinalResultImportRequest
	if json.Unmarshal(delivery.Payload, &request) != nil {
		t.Fatal("envelope")
	}
	var result db.P14FinalSettlement
	if err := tx.First(&result).Error; err != nil {
		t.Fatal(err)
	}
	sql, _ := db.GetIns().DB()
	sql.Close()
	p14PerGameSwitch(t, b)
	for _, init := range []func(context.Context) error{b.s.InitializeP6, b.s.InitializeP14FinalSettlement, b.s.InitializeP14FinalResult} {
		if err := init(b.ctx); err != nil {
			t.Fatal(err)
		}
	}
	b.ctx = context.WithValue(b.ctx, constUtil.P14CrossClockContextKey{}, func() time.Time { return at.Add(time.Second) })
	tx = db.GetIns()
	if err := tx.Model(&db.P14FinalResultPolicy{}).Where("id=1").Updates(map[string]any{"enabled": true, "version": "domain-published-canonical-result-v1", "published_at": manifest.FormationOffer.StageStart - 1}).Error; err != nil {
		t.Fatal(err)
	}
	key := []byte(strings.Repeat("isolated-peer-key-", 2))
	count := func(table any) int64 {
		var n int64
		if err := tx.Model(table).Count(&n).Error; err != nil {
			t.Fatal(err)
		}
		return n
	}
	if _, err := b.s.ImportP14FinalResult(b.ctx, &request, delivery.Payload, strings.Repeat("0", 64)); err == nil || count(&db.P14FinalResultReceipt{}) != 0 || count(&db.P6BattleRecord{}) != 0 {
		t.Fatal("bad origin signature wrote a result or aliased owner")
	}
	// A genuine accepted result with only the independent capability replaced by
	// ordinary17 must fail even under a valid isolated origin signature.
	body, err := p14FinalResultDecode(request.Snapshot)
	if err != nil {
		t.Fatal(err)
	}
	var terminal model.CombatFinalHPOutcome
	if json.Unmarshal(body.Terminal, &terminal) != nil {
		t.Fatal("terminal")
	}
	terminal.Outcome.CapabilityHash = constUtil.P6CrossStarterNativeProof
	body.Terminal, _ = json.Marshal(&terminal)
	body.Result.TerminalHash = p6Digest(body.Terminal)
	body.Result.ScopeProof = terminal.Outcome.CapabilityHash
	body.Result.SourceHash = p14FinalSettlementHash(&body.Result)
	bad := request
	bad.Snapshot, err = p14FinalResultEncode(body)
	if err != nil {
		t.Fatal(err)
	}
	bad.DeliveryID = p14FinalResultDeliveryID(&bad)
	badRaw, _ := json.Marshal(&bad)
	if _, err = b.s.ImportP14FinalResult(b.ctx, &bad, badRaw, P14SignPayload(key, badRaw)); err == nil || count(&db.P14FinalResultReceipt{}) != 0 || count(&db.P14FinalResultOffer{}) != 0 || count(&db.P6BattleRecord{}) != 0 {
		t.Fatal("ordinary17 substituted for KF or failed import left writes", err)
	}
	response, err := b.s.ImportP14FinalResult(b.ctx, &request, delivery.Payload, P14SignPayload(key, delivery.Payload))
	if err != nil || !response.Confirmed {
		t.Fatal("genuine canonical terminal import", err)
	}
	if count(&db.P14FinalResultReceipt{}) != 1 || count(&db.P14FinalSettlement{}) != 0 || count(&db.P6BattleRecord{}) != 0 {
		t.Fatal("peer invented source record or duplicate receipt")
	}
	var state db.P14FinalSeries
	if err = tx.First(&state, "id=?", series.ID).Error; err != nil || state.CurrentGame != 2 || state.LeftWins+state.RightWins != 1 || state.WinnerServerID != 0 || state.WinnerPlayerID != 0 {
		t.Fatal("peer series did not apply exactly one certified game", err)
	}
	var saved db.P14FinalResultReceipt
	if err = tx.First(&saved).Error; err != nil || saved.WinnerServerID != result.WinnerServerID || saved.WinnerPlayerID != result.WinnerPlayerID {
		t.Fatal("same PID lost canonical realm", err)
	}
	if _, _, err = p14ReadFinalResultReceipt(&saved); err != nil {
		t.Fatal(err)
	}
	b.restart(t)
	tx = db.GetIns()
	if err = tx.Model(&db.P14FinalResultPolicy{}).Where("id=1").Update("enabled", false).Error; err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 3; i++ {
		reply, err := b.s.ImportP14FinalResult(b.ctx, &request, delivery.Payload, P14SignPayload(key, delivery.Payload))
		if err != nil || !reply.Confirmed {
			t.Fatal("cold exact retry", err)
		}
	}
	if count(&db.P14FinalResultReceipt{}) != 1 || count(&db.P6BattleRecord{}) != 0 {
		t.Fatal("cold retry duplicated a win or P6 owner")
	}
	var later []db.P14FinalSeriesSlot
	if err = tx.Where("series_id=? AND game_index>1", series.ID).Find(&later).Error; err != nil {
		t.Fatal(err)
	}
	for _, slot := range later {
		if len(slot.Input) != 0 || len(slot.Certificate) != 0 {
			t.Fatal("next game fabricated")
		}
	}
}
