package service

import (
	"encoding/json"
	"fmt"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/go-sql-driver/mysql"
	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

func p6UnusedWinConditionLists() [][]*pb.BattleWinCondition {
	return [][]*pb.BattleWinCondition{
		{{}},
		{{WinKey: proto.Int32(0), WinValue: proto.Int32(1)}, {WinKey: proto.Int32(1), WinValue: proto.Int32(0)}},
		{{WinKey: proto.Int32(-2147483648), WinValue: proto.Int32(2147483647)}, {WinKey: proto.Int32(3), WinValue: proto.Int32(-1)}},
	}
}

func TestP6OriginalUnusedWinConditionTransportKeepsDefaultRules(t *testing.T) {
	s := p6OriginalTables(t)
	input := p6Input(t, s, p6Actor("hero_047", "subject", "047_1", 1, 1000000), p6Actor("hero_047", "opponent", "047_1", 1, 1000000), 1)
	base, err := s.ComputeFrozenBattle(t.Context(), input, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, conditions := range p6UnusedWinConditionLists() {
		input.WinConditions = conditions
		before, _ := json.Marshal(input)
		out, err := s.ComputeFrozenBattle(t.Context(), input, nil)
		if err != nil || out.Winner != base.Winner || !proto.Equal(out.Operates, base.Operates) || !reflect.DeepEqual(out.Events, base.Events) || out.RandomDraws != base.RandomDraws {
			t.Fatal("unconsumed original protobuf fields invented victory behavior", err)
		}
		after, _ := json.Marshal(input)
		if string(before) != string(after) {
			t.Fatal("original unused victory fields normalized")
		}
	}
	input.WinConditions = []*pb.BattleWinCondition{nil}
	if _, err = s.ComputeFrozenBattle(t.Context(), input, nil); err == nil {
		t.Fatal("nil frozen protocol condition accepted")
	}
}

func TestP6MySQLUnusedWinConditionColdOriginalTransport(t *testing.T) {
	dsn := os.Getenv("HYJS_MYSQL_P6_DSN")
	if dsn == "" {
		t.Skip("isolated original victory transport MySQL disabled")
	}
	config, err := mysql.ParseDSN(dsn)
	if err != nil || !strings.HasPrefix(config.DBName, "hyjs_test_p6_") {
		t.Fatal("dedicated original victory transport schema required")
	}
	ctx := t.Context()
	open := func() {
		t.Helper()
		if err := db.InitGameMySQL(ctx, db.MySQLConfig{Host: config.Addr, User: config.User, Password: config.Passwd, Database: config.DBName}); err != nil {
			t.Fatal(err)
		}
	}
	open()
	conn, _ := db.GetIns().DB()
	defer func() { conn.Close() }()
	s := &GameService{}
	if err = s.LoadTables("../../analysis/tables"); err != nil {
		t.Fatal(err)
	}
	if err = s.InitializeP6(ctx); err != nil {
		t.Fatal(err)
	}
	id := dailyCopyTestPlayer(t, ctx, "p6-unused-victory", 10)
	var records []struct {
		id         string
		conditions []*pb.BattleWinCondition
		outcome    *model.CombatOutcome
	}
	for index, conditions := range p6UnusedWinConditionLists() {
		input := p6Input(t, s, p6Actor("hero_047", "subject", "047_1", 1, 1000000), p6Actor("hero_047", "opponent", "047_1", 1, 1000000), 1)
		input.Attacker.PlayerId = proto.Int64(id)
		input.WinConditions = conditions
		fight := fmt.Sprintf("unused-victory-%d", index)
		var recordID string
		var outcome *model.CombatOutcome
		if err = p6Tx(ctx, id, func(tx *gorm.DB) error {
			var e error
			recordID, e = s.CaptureBattleReplay(ctx, tx, id, "p6_unused_victory", fight, *input)
			if e != nil {
				return e
			}
			outcome, e = s.ComputeBattleReplay(ctx, tx, id, "p6_unused_victory", fight, nil)
			return e
		}); err != nil || outcome.Winner != 1 || outcome.Authoritative {
			t.Fatal("unused original victory list blocked default computed replay", err)
		}
		records = append(records, struct {
			id         string
			conditions []*pb.BattleWinCondition
			outcome    *model.CombatOutcome
		}{recordID, conditions, outcome})
	}
	if err = conn.Close(); err != nil {
		t.Fatal(err)
	}
	open()
	conn, _ = db.GetIns().DB()
	for _, record := range records {
		replies, e := s.fightPlayback(ctx, id, &pb.CSFightPlayback{FightId: proto.String(record.id)})
		if e != nil || len(replies) != 1 {
			t.Fatal("cold original unused victory transport", e)
		}
		wire, e := proto.Marshal(replies[0].(*pb.SCFightPlayback))
		if e != nil {
			t.Fatal(e)
		}
		decoded := &pb.SCFightPlayback{}
		if e = proto.Unmarshal(wire, decoded); e != nil || !p6ConditionsEqual(record.conditions, decoded.GetWinConditions()) || !proto.Equal(record.outcome.Operates, decoded.OptRecord) || decoded.GetServerResult() != 1 {
			t.Fatal("cold original unused victory fields/default loss changed", e)
		}
	}
}
