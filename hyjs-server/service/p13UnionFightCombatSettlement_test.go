package service

import (
	"encoding/json"
	"gorm.io/gorm"
	"strings"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	pb "hyjs-server/model/protocol"
)

func TestP13UFCombatSettlementOriginalWinnerOrder(t *testing.T) {
	cases := []struct {
		name    string
		kills   [2]int32
		reached [2]int64
		powers  []int64
		winner  int
		reason  string
	}{
		{"one actual kill outranks power", [2]int32{1, 0}, [2]int64{200, 100}, []int64{1000, 1}, 0, "kills"},
		{"same kills earlier effect", [2]int32{2, 2}, [2]int64{200, 300}, []int64{1000, 1}, 0, "first-reaching"},
		{"simultaneous lower power", [2]int32{2, 2}, [2]int64{200, 200}, []int64{1000, 1}, 1, "lower-power"},
		{"exact tie remains unresolved", [2]int32{2, 2}, [2]int64{200, 200}, []int64{1000, 1000}, -1, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			winner, reason := p13UFCombatWinner(c.kills, c.reached, c.powers)
			if winner != c.winner || reason != c.reason {
				t.Fatalf("winner=%d reason=%s", winner, reason)
			}
		})
	}
}

// The real ordinary14 producer supplies HP, death and reward identities. This
// fixture publishes only the existing explicit isolated award example.
func TestP13UFCombatSettlementActualPKTerminalColdAndDelivery(t *testing.T) {
	f, id, enemy, clock, match := p13UFPlayerBattle172ActualSetup(t)
	for _, pid := range []int64{id, enemy} {
		var row db.Profile
		if e := db.GetIns().First(&row, pid).Error; e != nil {
			t.Fatal(e)
		}
		t.Cleanup(f.s.RegisterP10Client(f.ctx, pid, row.Session, func(proto.Message) error { return nil }))
		if _, e := f.s.SyncP13UnionFightField(f.ctx, pid, false); e != nil {
			t.Fatal(e)
		}
	}
	published := match.StartAt - 60000
	if e := db.GetIns().Model(&db.P13UnionFightSettlementPolicy{}).Where("`group`=?", match.Group).Updates(map[string]any{"enabled": true, "version": "isolated-certified-combat-v1", "source": "isolated prepublished original item311 example, no production publication", "published_at": published - 1, "participation_rewards": "item_311,1", "victory_rewards": "item_311,2", "win_prestige": 50, "lose_prestige": 25, "personal_missions": "", "task_rewards": "", "transport_mvp_rewards": ""}).Error; e != nil {
		t.Fatal(e)
	}
	if e := f.s.p13UFSettlementFreeze(db.GetIns(), match, time.UnixMilli(published)); e != nil {
		t.Fatal(e)
	}
	var offer db.P13UnionFightSettlementOffer
	if e := db.GetIns().First(&offer, "match_id=?", match.ID).Error; e != nil {
		t.Fatal(e)
	}
	if _, handled, e := f.s.DispatchP13UnionFight(f.ctx, id, &pb.CSUFPlayerAskFight{TargetGuid: proto.Int64(enemy)}, 900); e != nil || !handled {
		t.Fatal("real start", e)
	}
	var attempt db.P13UnionFightPlayerBattleAttempt
	if e := db.GetIns().First(&attempt, "match_id=?", match.ID).Error; e != nil {
		t.Fatal(e)
	}
	// Pending remains unresolved even after a caller asks for terminal time.
	if result, e := f.s.p13UFSettlePlayerCombat(db.GetIns(), match, &offer, time.UnixMilli(match.EndAt)); e != nil || result != nil {
		t.Fatal("pending settled", result, e)
	}
	clock.Store(attempt.DueAt)
	if e := f.s.AdvanceP13UnionFightPlayerBattles(f.ctx, time.UnixMilli(clock.Load())); e != nil {
		t.Fatal(e)
	}
	if e := db.GetIns().First(&attempt, "id=?", attempt.ID).Error; e != nil {
		t.Fatal(e)
	}
	if attempt.Status != constUtil.P13UFPlayerBattleSettled || attempt.DeadPlayerID == 0 || attempt.RewardPlayerID == 0 {
		t.Fatal("real death required", attempt.Status)
	}
	// Corrupted persisted evidence cannot become a round result.
	originalHash := attempt.SourceHash
	if e := db.GetIns().Model(&attempt).Update("source_hash", "broken").Error; e != nil {
		t.Fatal(e)
	}
	if result, e := f.s.p13UFSettlePlayerCombat(db.GetIns(), match, &offer, time.UnixMilli(match.EndAt)); e == nil || result != nil {
		t.Fatal("bad SHA accepted")
	}
	if e := db.GetIns().Model(&attempt).Update("source_hash", originalHash).Error; e != nil {
		t.Fatal(e)
	}
	audit := db.P13UnionFightSettlementIntent{MatchID: match.ID, PlayerID: id, TargetID: enemy, Stamp: 901, ObservedAt: attempt.CreatedAt, Source: constUtil.P13UnionFightSettlementSource}
	if e := db.GetIns().Create(&audit).Error; e != nil {
		t.Fatal(e)
	}
	if result, e := f.s.p13UFSettlePlayerCombat(db.GetIns(), match, &offer, time.UnixMilli(match.EndAt)); e != nil || result != nil {
		t.Fatal("unknown intent accepted", e)
	}
	if e := db.GetIns().Delete(&audit).Error; e != nil {
		t.Fatal(e)
	}
	// An audit row is a separate unknown source, even when its target is a player.
	guard := db.P13UnionFightGuardAuditAttempt{MatchID: match.ID, PlayerID: id, Stamp: 902}
	if e := db.GetIns().Create(&guard).Error; e != nil {
		t.Fatal(e)
	}
	if result, e := f.s.p13UFSettlePlayerCombat(db.GetIns(), match, &offer, time.UnixMilli(match.EndAt)); e != nil || result != nil {
		t.Fatal("guard audit accepted", e)
	}
	if e := db.GetIns().Delete(&guard).Error; e != nil {
		t.Fatal(e)
	}
	clock.Store(match.EndAt)
	result, e := f.s.p13UFSettlePlayerCombat(db.GetIns(), match, &offer, time.UnixMilli(clock.Load()))
	if e != nil || result == nil {
		t.Fatal("actual combat terminal", e)
	}
	winnerGuild := ""
	var winnerActor db.P13UnionFightFieldPlayer
	if e = db.GetIns().First(&winnerActor, "match_id=? AND player_id=?", match.ID, attempt.RewardPlayerID).Error; e != nil {
		t.Fatal(e)
	}
	winnerGuild = winnerActor.GuildID
	if result.WinnerGuildID != winnerGuild || result.LeftKills+result.RightKills != 1 || result.Reason != "kills" || result.LedgerHash == "" {
		t.Fatal("kill terminal identity", result)
	}
	encoded, _ := json.Marshal(result)
	if e = f.s.p13UFCombatSettlementVerify(db.GetIns(), match, result); e != nil {
		t.Fatal("new source verification", e)
	}
	// A lightweight proof verifies retained metadata without rereading P6 input,
	// operates or outcome. This callback observes actual executed query SQL.
	var heavyReads []string
	callback := "p13-proof-no-p6-blobs"
	if e = db.GetIns().Callback().Query().After("gorm:query").Register(callback, func(tx *gorm.DB) {
		if tx.Statement.Table != "p6_battle_records" {
			return
		}
		query := strings.ToLower(tx.Statement.SQL.String())
		if strings.Contains(query, "select *") || strings.Contains(query, "`input`") || strings.Contains(query, "`outcome`") || strings.Contains(query, "`operates`") {
			heavyReads = append(heavyReads, query)
		}
	}); e != nil {
		t.Fatal(e)
	}
	if e = f.s.p13UFCombatSettlementVerify(db.GetIns(), match, result); e != nil {
		t.Fatal(e)
	}
	if e = db.GetIns().Callback().Query().Remove(callback); e != nil {
		t.Fatal(e)
	}
	if len(heavyReads) != 0 {
		t.Fatal("downstream reread P6 payload", heavyReads)
	}
	// Missing or mutated source metadata still invalidates the frozen proof.
	var p6 db.P6BattleRecord
	if e = db.GetIns().First(&p6, "id=?", attempt.P6RecordID).Error; e != nil {
		t.Fatal(e)
	}
	originalReplay := p6.ReplayHash
	if e = db.GetIns().Model(&p6).Update("replay_hash", "broken").Error; e != nil {
		t.Fatal(e)
	}
	if e = f.s.p13UFCombatSettlementVerify(db.GetIns(), match, result); e == nil {
		t.Fatal("changed source metadata accepted")
	}
	if e = db.GetIns().Model(&p6).Update("replay_hash", originalReplay).Error; e != nil {
		t.Fatal(e)
	}
	// Verified historical association can be resumed and cannot silently default.
	if e = db.GetIns().Model(&p6).Update("p13_match_id", "").Error; e != nil {
		t.Fatal(e)
	}
	if e = f.s.p13UFCombatSettlementVerify(db.GetIns(), match, result); e == nil {
		t.Fatal("missing match association accepted")
	}
	predecessor := []byte(p6.ID)
	for i := len(predecessor) - 1; i >= 0; i-- {
		n := strings.IndexByte("0123456789abcdef", predecessor[i])
		if n > 0 {
			predecessor[i] = "0123456789abcdef"[n-1]
			break
		}
		predecessor[i] = 'f'
	}
	originalInputHash := p6.InputHash
	if e = db.GetIns().Model(&p6).Update("input_hash", "broken").Error; e != nil {
		t.Fatal(e)
	}
	failedCursor, failedCount, failedErr := BackfillP13P6MatchScopeBatch(f.ctx, string(predecessor), 1)
	if failedErr == nil || failedCount != 0 || failedCursor != string(predecessor) {
		t.Fatal("damaged backfill lost checkpoint", failedCursor, failedCount, failedErr)
	}
	if e = db.GetIns().Model(&p6).Update("input_hash", originalInputHash).Error; e != nil {
		t.Fatal(e)
	}
	cursor, backfillCount, backfillErr := BackfillP13P6MatchScopeBatch(f.ctx, string(predecessor), 1)
	if backfillErr != nil || backfillCount != 1 || cursor != p6.ID {
		t.Fatal("verified backfill", cursor, backfillCount, backfillErr)
	}
	if e = f.s.p13UFCombatSettlementVerify(db.GetIns(), match, result); e != nil {
		t.Fatal("backfilled proof", e)
	}
	t.Run("missing ledger source rejects", func(t *testing.T) {
		tx := db.GetIns().Begin()
		if tx.Error != nil {
			t.Fatal(tx.Error)
		}
		defer tx.Rollback()
		if e := tx.Delete(&db.P13UnionFightPlayerBattleAttempt{}, "id=?", attempt.ID).Error; e != nil {
			t.Fatal(e)
		}
		if e := f.s.p13UFCombatSettlementVerify(tx, match, result); e == nil {
			t.Fatal("missing ledger accepted")
		}
	})
	// Reopening the same DB verifies a historical terminal without another battle.
	f.restart(t)
	result, e = f.s.p13UFSettlePlayerCombat(db.GetIns(), match, &offer, time.UnixMilli(clock.Load()))
	if e != nil {
		t.Fatal(e)
	}
	restored, _ := json.Marshal(result)
	if string(encoded) != string(restored) {
		t.Fatal("cold result changed")
	}
	if e = f.s.p13UFSettlementDeliver(db.GetIns(), result, time.UnixMilli(result.AwardAt-1)); e != nil {
		t.Fatal(e)
	}
	if result.DeliveredAt != 0 {
		t.Fatal("early delivery")
	}
	if e = f.s.p13UFSettlementDeliver(db.GetIns(), result, time.UnixMilli(result.AwardAt)); e != nil {
		t.Fatal(e)
	}
	var count int64
	if e = db.GetIns().Model(&db.RewardDelivery{}).Where("source_key=?", "p13-war:"+match.ID).Count(&count).Error; e != nil {
		t.Fatal(e)
	}
	if count != 2 {
		t.Fatal("two actual participants", count)
	}
	if e = f.s.p13UFSettlementDeliver(db.GetIns(), result, time.UnixMilli(result.AwardAt+1)); e != nil {
		t.Fatal(e)
	}
	var after int64
	if e = db.GetIns().Model(&db.RewardDelivery{}).Where("source_key=?", "p13-war:"+match.ID).Count(&after).Error; e != nil || after != count {
		t.Fatal("duplicate delivery", after, e)
	}
	result.LeftKills++
	if e = f.s.p13UFCombatSettlementVerify(db.GetIns(), match, result); e == nil {
		t.Fatal("tampered terminal accepted")
	}
}
