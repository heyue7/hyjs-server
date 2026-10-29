package service

import (
	"fmt"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

// The frozen PvE policy survives a later published configuration change and
// older pending fights without the field continue to use the 50% policy.
func TestGGOPVEDeathUsesFrozenPercentageAndLegacyDefault(t *testing.T) {
	t.Setenv("HYJS_P1_MYSQL_DATABASE", "")
	f := p1Setup(t)
	if err := db.GetIns().AutoMigrate(&db.GGOFieldReceipt{}, &db.P8LifeState{}); err != nil {
		t.Fatal(err)
	}
	cfg := ggoLineTestConfiguration(t)
	cfg.Rules.DeathLossPercent = ggoInt64(100)
	f.s.GGOFieldConfig = cfg
	values, err := ggoChipValues(cfg)
	if err != nil {
		t.Fatal(err)
	}
	for i, tc := range []struct {
		percent *int64
		want    int64
	}{{nil, 4}, {ggoInt64(0), 0}, {ggoInt64(25), 2}, {ggoInt64(100), 5}} {
		player, game := f.profile(t)
		game.GGOField = &model.GGOFieldState{Chips: [6]int64{5, 0, 0, 0, 0, 3}, EconomyVersion: cfg.Runtime.EconomyVersion, ExchangeValues: values}
		game.Position = &pb.Position{PlayerId: proto.Int64(f.id), Scene: proto.String(constUtil.GGOFieldScene), Current: &pb.PointNew{X: proto.Float32(20), Y: proto.Float32(0), Z: proto.Float32(20)}}
		game.Field = &model.FieldState{Scene: constUtil.GGOFieldScene, Side: &pb.PlayerSide{PlayerId: proto.Int64(f.id), CurHp: proto.Float64(0), State: proto.Int32(constUtil.P8FieldDead)}}
		fight := &model.GGOFieldMonsterFight{FightID: fmt.Sprintf("20000000-0000-4000-8000-%012d", i+1), ProtectedChips: 3, DeathLossPercent: tc.percent, SafeReturn: cfg.Runtime.SafeReturn, HealRatio: 0.5}
		var lost [6]int64
		err = db.GetIns().Transaction(func(tx *gorm.DB) error {
			var err error
			lost, err = f.s.ggoPVEDeathLocked(tx, f.id, player, game, fight, time.Now().UnixMilli())
			return err
		})
		if err != nil || lost != ([6]int64{tc.want}) || game.GGOField.Chips != ([6]int64{5 - tc.want, 0, 0, 0, 0, 3}) || !game.PositionLoading {
			t.Fatalf("percent=%v lost=%v chips=%v err=%v", tc.percent, lost, game.GGOField.Chips, err)
		}
	}
}
