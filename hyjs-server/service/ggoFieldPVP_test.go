package service

import (
	"errors"
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

func TestGGOPVPSixColorTransferAndReceiptRollback(t *testing.T) {
	t.Setenv("HYJS_P1_MYSQL_DATABASE", "")
	f := p1Setup(t)
	if err := db.GetIns().AutoMigrate(&db.GGOFieldReceipt{}); err != nil {
		t.Fatal(err)
	}
	cfg := ggoLineTestConfiguration(t)
	f.s.GGOFieldConfig = cfg
	values, err := ggoChipValues(cfg)
	if err != nil {
		t.Fatal(err)
	}
	makeState := func(chips [6]int64) *model.GameState {
		return &model.GameState{GGOField: &model.GGOFieldState{Chips: chips, EconomyVersion: cfg.Runtime.EconomyVersion, ExchangeValues: values}, Field: &model.FieldState{Side: &pb.PlayerSide{CurHp: proto.Float64(1), State: proto.Int32(0)}}}
	}
	winner := makeState([6]int64{0, 1})
	loser := makeState([6]int64{5, 0, 0, 0, 0, 3})
	fightID := "11111111-1111-4111-8111-111111111111"
	err = db.GetIns().Transaction(func(tx *gorm.DB) error {
		return f.s.ggoSettlePVP(tx, 10001, 10002, winner, loser, fightID, time.Now().UnixMilli())
	})
	if err != nil {
		t.Fatal(err)
	}
	if winner.GGOField.Chips != ([6]int64{4, 1}) || loser.GGOField.Chips != ([6]int64{1, 0, 0, 0, 0, 3}) || loser.GGOField.PVPReturn == nil || loser.Field.Side.GetState() != constUtil.P8FieldDead {
		t.Fatal("chip transfer or return state", winner.GGOField, loser.GGOField)
	}
	var receipts []db.GGOFieldReceipt
	if err = db.GetIns().Where("player_id IN ?", []int64{10001, 10002}).Find(&receipts).Error; err != nil || len(receipts) != 2 {
		t.Fatal("missing pair of receipts", len(receipts), err)
	}
	aborted := errors.New("rollback test")
	winner = makeState([6]int64{})
	loser = makeState([6]int64{5, 0, 0, 0, 0, 3})
	err = db.GetIns().Transaction(func(tx *gorm.DB) error {
		if err := f.s.ggoSettlePVP(tx, 10003, 10004, winner, loser, "22222222-2222-4222-8222-222222222222", time.Now().UnixMilli()); err != nil {
			return err
		}
		return aborted
	})
	if !errors.Is(err, aborted) {
		t.Fatal(err)
	}
	var count int64
	if err = db.GetIns().Model(&db.GGOFieldReceipt{}).Where("player_id IN ?", []int64{10003, 10004}).Count(&count).Error; err != nil || count != 0 {
		t.Fatal("rolled-back PvP receipts persisted", count, err)
	}
}

func TestGGOPVPUsesPublishedDeathLossPercentage(t *testing.T) {
	t.Setenv("HYJS_P1_MYSQL_DATABASE", "")
	f := p1Setup(t)
	if err := db.GetIns().AutoMigrate(&db.GGOFieldReceipt{}); err != nil {
		t.Fatal(err)
	}
	cfg := ggoLineTestConfiguration(t)
	f.s.GGOFieldConfig = cfg
	values, err := ggoChipValues(cfg)
	if err != nil {
		t.Fatal(err)
	}
	for i, tc := range []struct {
		percent *int64
		want    int64
	}{{nil, 4}, {ggoInt64(0), 0}, {ggoInt64(25), 2}, {ggoInt64(100), 5}} {
		cfg.Rules.DeathLossPercent = tc.percent
		winner := &model.GameState{GGOField: &model.GGOFieldState{EconomyVersion: cfg.Runtime.EconomyVersion, ExchangeValues: values}}
		loser := &model.GameState{GGOField: &model.GGOFieldState{Chips: [6]int64{5, 0, 0, 0, 0, 3}, EconomyVersion: cfg.Runtime.EconomyVersion, ExchangeValues: values}, Field: &model.FieldState{Side: &pb.PlayerSide{CurHp: proto.Float64(1), State: proto.Int32(0)}}}
		fightID := fmt.Sprintf("10000000-0000-4000-8000-%012d", i+1)
		err = db.GetIns().Transaction(func(tx *gorm.DB) error {
			return f.s.ggoSettlePVP(tx, int64(10001+2*i), int64(10002+2*i), winner, loser, fightID, time.Now().UnixMilli())
		})
		if err != nil || winner.GGOField.Chips[0] != tc.want || loser.GGOField.Chips[0] != 5-tc.want || loser.GGOField.Chips[5] != 3 || loser.GGOField.PVPReturn == nil {
			t.Fatalf("percent=%v winner=%v loser=%v err=%v", tc.percent, winner.GGOField, loser.GGOField, err)
		}
	}
}
