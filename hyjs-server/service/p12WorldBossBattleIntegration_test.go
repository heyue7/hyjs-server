package service

import (
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
	"hyjs-server/db"
	pb "hyjs-server/model/protocol"
)

// TestP12WorldBossBattleSceneChangesOnlyAfterFinish 将原场景和排行查询接到同一真实结算事实。
func TestP12WorldBossBattleSceneChangesOnlyAfterFinish(t *testing.T) {
	f, _, boss, now := p12WorldBossBattleSetup(t)
	session := p12WorldBossBattlePrepare(t, f, f.id)
	project := func() *pb.WorldBoss {
		t.Helper()
		var messages []proto.Message
		err := db.GetIns().Transaction(func(tx *gorm.DB) error {
			var err error
			messages, err = f.s.p12WorldBossFieldProjection(tx, "field_002", now)
			return err
		})
		if err != nil || len(messages) != 1 {
			t.Fatal("original field entity missing", err, len(messages))
		}
		return messages[0].(*pb.SCWorldBoss).GetWorldBoss()
	}
	before := project()
	start, err := f.s.startP12WorldBossBattleAt(f.ctx, f.id, boss, 0, now)
	if err != nil {
		t.Fatal(err)
	}
	during := project()
	if during.GetCurHp() != before.GetCurHp() || during.GetTotalHp() != before.GetTotalHp() {
		t.Fatal("precomputation changed visible shared HP")
	}
	if err := db.GetIns().Transaction(func(tx *gorm.DB) error { return f.s.ensureNoActiveDailyCopy(tx, f.id, now) }); err == nil {
		t.Fatal("a pending boss did not block a competing battle")
	}
	finish, err := f.s.finishP12WorldBossBattleAt(f.ctx, f.id, session, start.GetFightId(), now.Add(time.Minute))
	if err != nil || finish.AppliedDamage <= 0 {
		t.Fatal("real finish did not commit calculated damage", err)
	}
	after := project()
	if after.GetCurHp() != before.GetCurHp()-float64(finish.AppliedDamage) || after.GetCurHp() != float64(finish.CurrentHP) {
		t.Fatal("field projection not bound to committed shared HP")
	}
	rank := p12WorldBossCall(t, f, f.id, &pb.CSWorldBossRank{}, now).(*pb.SCWorldBossRank)
	if rank.GetMyDamageInfo().GetDamage() != float64(finish.AppliedDamage) || rank.GetBossHpPercent() != after.GetCurHp()/after.GetTotalHp() {
		t.Fatal("original rank path differs from committed field entity")
	}
	if err := db.GetIns().Transaction(func(tx *gorm.DB) error { return f.s.ensureNoActiveDailyCopy(tx, f.id, now) }); err != nil {
		t.Fatal("committed boss kept blocking subsequent gameplay", err)
	}
}
