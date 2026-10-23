package service

import (
	"bytes"
	"encoding/json"
	"errors"
	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
	"testing"
)

func TestP7RushStoredFingerprintIncludesHiddenBlobs(t *testing.T) {
	row := db.P6BattleRecord{ID: "same", Input: []byte{1}, Operates: []byte{2}, Outcome: []byte{3}, InputHash: "same-declared-hash"}
	before, err := p7RushStoredRowFingerprint(&row)
	if err != nil {
		t.Fatal(err)
	}
	for _, field := range []*[]byte{&row.Input, &row.Operates, &row.Outcome} {
		old := append([]byte(nil), (*field)...)
		(*field)[0] ^= 255
		after, err := p7RushStoredRowFingerprint(&row)
		if err != nil || after == before {
			t.Fatal("hidden source bytes omitted", err)
		}
		*field = old
	}
}

// One real original scoped fight supplies an accepted source without private account fixtures.
func TestP7RushSettledReadMemoPreservesSourceAndTransaction(t *testing.T) {
	f, _, request := p7ScopedFixture(t, constUtil.P7RushTrial)
	if err := db.GetIns().Model(&db.P7RushScopedPolicy{}).Where("id=?", 1).Update("enabled", true).Error; err != nil {
		t.Fatal(err)
	}
	start, err := f.s.p7Start(f.ctx, f.id, request, 55501, false)
	if err != nil {
		t.Fatal(err)
	}
	var row db.P7RushScopedCalculation
	if err = db.GetIns().First(&row, "fight_id=?", start.GetFightId()).Error; err != nil {
		t.Fatal(err)
	}
	var terminal model.CombatFinalHPOutcome
	if err = json.Unmarshal(row.Terminal, &terminal); err != nil {
		t.Fatal(err)
	}
	result := int32(0)
	if terminal.Outcome.Winner == 0 {
		result = 1
	}
	end := &pb.CSRushFightEnd{FightId: start.FightId, Result: proto.Int32(result), OptRecord: terminal.OriginalOperates}
	if _, err = f.s.p7End(f.ctx, f.id, end); err != nil {
		t.Fatal(err)
	}
	var fight db.P7RushFight
	if err = db.GetIns().First(&fight, "id=?", start.GetFightId()).Error; err != nil {
		t.Fatal(err)
	}
	ctx := p7WithRushReadRequestCache(f.ctx)
	cache := ctx.Value(constUtil.P7RushReadRequestCacheKey{}).(*p7RushReadRequestCache)
	rollback := errors.New("owned mutation rollback")
	err = db.GetIns().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		first, scoped, e := p7ReadScopedRush(tx, &fight, end)
		if e != nil || !scoped {
			t.Fatalf("original accepted read: %v", e)
		}
		again, _, e := p7ReadScopedRush(tx, &fight, end)
		if e != nil || cache.hits != 1 {
			t.Fatalf("same-source memo: %v", e)
		}
		a, _ := json.Marshal(first)
		b, _ := json.Marshal(again)
		if !bytes.Equal(a, b) {
			t.Fatal("memo changed original accepted outcome")
		}
		again.Winner = 2
		third, _, e := p7ReadScopedRush(tx, &fight, end)
		if e != nil || third.Winner != first.Winner {
			t.Fatal("caller mutated memo")
		}
		var record db.P6BattleRecord
		if e = tx.First(&record, "id=?", fight.ReplayID).Error; e != nil {
			return e
		}
		original := append([]byte(nil), record.Input...)
		changed := append([]byte(nil), original...)
		changed[0] ^= 255
		if e = tx.Model(&record).Update("input", changed).Error; e != nil {
			return e
		}
		if _, _, e = p7ReadScopedRush(tx, &fight, end); e == nil {
			t.Fatal("actual hidden input mutation reused memo")
		}
		if e = tx.Model(&record).Update("input", original).Error; e != nil {
			return e
		}
		wrong := proto.Clone(end).(*pb.CSRushFightEnd)
		wrong.Result = proto.Int32(1 - result)
		if _, _, e = p7ReadScopedRush(tx, &fight, wrong); e == nil {
			t.Fatal("changed End reused memo")
		}
		forged := fight
		forged.PlayerID++
		if _, _, e = p7ReadScopedRush(tx, &forged, end); e == nil {
			t.Fatal("changed owner reused memo")
		}
		pending := fight
		pending.Status = constUtil.P7RushComputed
		if candidate, _ := p7RushSettledReadFingerprint(tx, &pending, end); candidate != nil {
			t.Fatal("pending memo enabled")
		}
		freshCtx := p7WithRushReadRequestCache(f.ctx)
		fresh := freshCtx.Value(constUtil.P7RushReadRequestCacheKey{}).(*p7RushReadRequestCache)
		if _, _, e = p7ReadScopedRush(tx.WithContext(freshCtx), &fight, end); e != nil || fresh.hits != 0 || fresh.misses != 1 {
			t.Fatal("request cache crossed request boundary", e)
		}
		return rollback
	})
	if !errors.Is(err, rollback) {
		t.Fatal(err)
	}
	hits, misses := cache.hits, cache.misses
	if err = db.GetIns().WithContext(ctx).Transaction(func(tx *gorm.DB) error { _, _, e := p7ReadScopedRush(tx, &fight, end); return e }); err != nil {
		t.Fatal(err)
	}
	if cache.hits != hits || cache.misses != misses+1 {
		t.Fatal("request cache crossed SQL transaction boundary")
	}
}
