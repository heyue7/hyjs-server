package service

import (
	"reflect"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
	"hyjs-server/db"
	pb "hyjs-server/model/protocol"
)

func TestP13UFPlayerAdmissionLoadsEachSourceOnce(t *testing.T) {
	f, id, enemy, clock, match := p13UFPartyAdmissionActualSetup(t)
	counts := map[string]int{}
	var captured map[string]int
	qname, cname := "p13_admission_loaded_reads", "p13_admission_loaded_capture"
	if err := db.GetIns().Callback().Query().After("gorm:query").Register(qname, func(tx *gorm.DB) { counts[tx.Statement.Table]++ }); err != nil {
		t.Fatal(err)
	}
	defer db.GetIns().Callback().Query().Remove(qname)
	if err := db.GetIns().Callback().Create().After("gorm:create").Register(cname, func(tx *gorm.DB) {
		if tx.Statement.Table == "p13_union_fight_player_battle_attempts" {
			captured = map[string]int{}
			for table, n := range counts {
				captured[table] = n
			}
		}
	}); err != nil {
		t.Fatal(err)
	}
	defer db.GetIns().Callback().Create().Remove(cname)
	replies, handled, err := f.s.p13UFTryPlayerBattle(f.ctx, id, &pb.CSUFPlayerAskFight{TargetGuid: proto.Int64(enemy)}, 0, time.UnixMilli(clock.Load()))
	if err != nil || !handled || len(replies) != 2 || captured == nil {
		t.Fatal("actual party admission", handled, err)
	}
	// Counts are frozen at the durable attempt insert, before postcommit broadcasts.
	for table, want := range map[string]int{"profiles": 2, "p13_union_fight_matches": 2, "p13_union_fight_fields": 2, "p13_union_fight_player_battle_offers": 1} {
		if captured[table] != want {
			t.Fatal("admission repeated authenticated source", table, captured[table], want, captured)
		}
	}
	var attempt db.P13UnionFightPlayerBattleAttempt
	if err := db.GetIns().First(&attempt, "match_id=? AND player_id=?", match.ID, id).Error; err != nil {
		t.Fatal(err)
	}
	if _, _, _, _, err := f.s.p13UFPlayerBattleVerify(db.GetIns().WithContext(f.ctx), &attempt); err != nil {
		t.Fatal("loaded capture full source verification", err)
	}
}

func TestP13UFFieldBattleLoadedMatchesPublicAndKeepsRejections(t *testing.T) {
	f, id, _, clock, _ := p13UFPartyAdmissionActualSetup(t)
	err := db.GetIns().WithContext(f.ctx).Transaction(func(tx *gorm.DB) error {
		now := time.UnixMilli(clock.Load())
		actor, match, _, frame, err := f.s.p13UFFieldCurrent(tx, id, now)
		if err != nil {
			return err
		}
		var profile db.Profile
		if err = tx.First(&profile, id).Error; err != nil {
			return err
		}
		_, game, err := decodeProfileForCalendarUpdate(tx, &profile)
		if err != nil {
			return err
		}
		got, sources, err := f.s.p13UFFieldBattleLoaded(tx, actor, match, frame, game, now)
		if err != nil {
			return err
		}
		want, originalSources, err := f.s.p13UFFieldBattle(tx, id, now)
		if err != nil {
			return err
		}
		if !proto.Equal(got, want) || !reflect.DeepEqual(sources, originalSources) {
			t.Fatal("loaded projection changed original attributes/source list")
		}
		invalid := proto.Clone(frame).(*pb.InstUFPlayer)
		invalid.FightInfo.HpPos[0] = invalid.FightInfo.MaxHpPos[0] + 1
		if _, _, err := f.s.p13UFFieldBattleLoaded(tx, actor, match, invalid, game, now); err == nil {
			t.Fatal("loaded projection accepted illegal current HP")
		}
		game.PositionLoading = true
		if _, _, err := f.s.p13UFFieldBattleLoaded(tx, actor, match, frame, game, now); err == nil {
			t.Fatal("loaded projection skipped Current handshake")
		}
		game.PositionLoading = false
		broken := *actor
		broken.Battle = append(append([]byte(nil), actor.Battle...), 0)
		if _, _, err := f.s.p13UFFieldBattleLoaded(tx, &broken, match, frame, game, now); err == nil {
			t.Fatal("loaded projection skipped frozen Battle SHA")
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
