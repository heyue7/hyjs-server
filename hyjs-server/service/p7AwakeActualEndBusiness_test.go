package service

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"google.golang.org/protobuf/proto"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	pb "hyjs-server/model/protocol"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// 仅向本测试的新隔离库导入原机已付pending输入。没有库存、会话、接受结果或胜利夹具。
func p7AwakeActualPaidFixture(t *testing.T) (*p1Fixture, *pb.CSAwakeCopyFightEnd, time.Time) {
	t.Helper()
	f := p7AwakeBusinessFixture(t)
	if p1Count(t, &db.Profile{}, "id=?", 4) != 0 {
		t.Fatal("actual phone fixture requires fresh isolated schema with profile4 unused")
	}
	var owner db.Profile
	if err := db.GetIns().First(&owner, f.id).Error; err != nil {
		t.Fatal(err)
	}
	player := &pb.Player{}
	if owner.Player == nil || json.Unmarshal([]byte(*owner.Player), player) != nil {
		t.Fatal("isolated owner snapshot")
	}
	player.PlayerId = proto.Int64(4)
	playerRaw, marshalErr := json.Marshal(player)
	if marshalErr != nil {
		t.Fatal(marshalErr)
	}
	playerJSON := string(playerRaw)
	owner.Player = &playerJSON
	owner.Id = 4
	owner.Uid = "isolated-captured-awake-paid-owner4"
	owner.Session = []byte("fixture-only-004")
	if err := db.GetIns().Create(&owner).Error; err != nil {
		t.Fatal(err)
	}
	f.id = 4
	folder := "../../analysis/playable-plan-20261017/awake-original-end-capture-r1"
	raw, err := os.ReadFile(filepath.Join(folder, "paid-pending-fixture.json"))
	if err != nil || p6Digest(raw) != "3f44e322658eac3572f2b2204566f693b033a2c0206187efddc6d3f9a8a64ad7" {
		t.Fatal("actual paid pending source pin", err)
	}
	var snapshot struct {
		PlayerID int64 `json:"playerId"`
		Tables   []struct {
			Name    string   `json:"name"`
			Columns []string `json:"columns"`
			Row     []any    `json:"row"`
		} `json:"tables"`
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	if err = decoder.Decode(&snapshot); err != nil || snapshot.PlayerID != 4 || len(snapshot.Tables) != 4 {
		t.Fatal("finite actual pending fixture", err)
	}
	allowed := map[string]bool{"p7_awake_runs": true, "p7_awake_explorations": true, "p7_awake_fights": true, "p6_battle_records": true}
	for _, table := range snapshot.Tables {
		if !allowed[table.Name] || len(table.Columns) != len(table.Row) {
			t.Fatal("fixture table boundary")
		}
		delete(allowed, table.Name)
		values := make([]any, len(table.Row))
		columns := make([]string, len(table.Columns))
		for index, column := range table.Columns {
			for _, r := range column {
				if !(r == '_' || r >= 'a' && r <= 'z') {
					t.Fatal("fixture SQL column")
				}
			}
			columns[index] = "`" + column + "`"
			switch value := table.Row[index].(type) {
			case nil:
				values[index] = nil
			case json.Number:
				values[index], err = value.Int64()
				if err != nil {
					t.Fatal(err)
				}
			case map[string]any:
				data, decodeErr := hex.DecodeString(value["hex"].(string))
				if decodeErr != nil {
					t.Fatal(decodeErr)
				}
				if value["binary"] == true {
					values[index] = data
				} else {
					values[index] = string(data)
				}
			default:
				t.Fatal("unexpected fixture value")
			}
		}
		placeholders := strings.TrimSuffix(strings.Repeat("?,", len(values)), ",")
		if err = db.GetIns().Exec("INSERT INTO "+table.Name+" ("+strings.Join(columns, ",")+") VALUES ("+placeholders+")", values...).Error; err != nil {
			t.Fatal(err)
		}
	}
	body, err := os.ReadFile(filepath.Join(folder, "original-End.pb"))
	if err != nil || p6Digest(body) != "4696d6ac79fcaf3fd906e38ef77b1482a7a81dd1b143068ae17093ed62580b21" {
		t.Fatal("actual original End pin", err)
	}
	end := &pb.CSAwakeCopyFightEnd{}
	if err = proto.Unmarshal(body, end); err != nil {
		t.Fatal(err)
	}
	var run db.P7AwakeRun
	if err = db.GetIns().First(&run, "player_id=?", 4).Error; err != nil {
		t.Fatal(err)
	}
	return f, end, time.UnixMilli(run.StartAt + 60000)
}

// 直接消费捕获的真实HTTP body，验证公开End事务、并发幂等及冷回执；输入不是自算的“测试客户端”。
func TestP7AwakeActualPhoneManualEndBusinessColdOnce(t *testing.T) {
	if constUtil.P7Awake16ManualProof == "" {
		t.Fatal("actual phone strict replay proof must be installed before business verification")
	}
	f, end, now := p7AwakeActualPaidFixture(t)
	var group sync.WaitGroup
	replies := make([][]proto.Message, 2)
	failures := make([]error, 2)
	for i := range replies {
		group.Add(1)
		go func(index int) {
			defer group.Done()
			replies[index], failures[index] = f.s.p7AwakeFightEnd(f.ctx, f.id, proto.Clone(end).(*pb.CSAwakeCopyFightEnd), now)
		}(i)
	}
	group.Wait()
	for i := range replies {
		if failures[i] != nil || len(replies[i]) != 1 {
			t.Fatal("actual End transaction", i, failures[i])
		}
	}
	if !proto.Equal(replies[0][0], replies[1][0]) {
		t.Fatal("concurrent End receipt differs")
	}
	accepted := replies[0][0].(*pb.SCAwakeCopyFightEnd)
	if accepted.GetIsWinServer() != 1 || accepted.GetRefreshData().GetScore() != 80 {
		t.Fatal("actual result/monster score", accepted)
	}
	var fight db.P7AwakeFight
	if err := db.GetIns().First(&fight, "id=?", end.GetFightId()).Error; err != nil || fight.Status != constUtil.P7AwakeFightAccepted {
		t.Fatal("End not accepted", err)
	}
	var record db.P6BattleRecord
	if err := db.GetIns().First(&record, "id=?", fight.ReplayID).Error; err != nil || !record.Authoritative || record.Status != constUtil.P6BattleComputed {
		t.Fatal("strict manual result not bound to frozen record", err)
	}
	f.restart(t)
	retry, err := f.s.p7AwakeFightEnd(f.ctx, f.id, end, now)
	if err != nil || !proto.Equal(retry[0], accepted) {
		t.Fatal("cold original End receipt", err)
	}
	fake := proto.Clone(end).(*pb.CSAwakeCopyFightEnd)
	fake.IsWinClient = proto.Int32(0)
	if _, err = f.s.p7AwakeFightEnd(f.ctx, f.id, fake, now); err == nil {
		t.Fatal("cold false winner changed original receipt")
	}
	if _, err = f.s.p7AwakeFightEnd(f.ctx, f.id+1, end, now); err == nil {
		t.Fatal("another account received actual End receipt")
	}
	// 原机pending包含真实EnterStamp；新结算请求必须晚于该实例，不能沿用小测试戳。
	// 同时保留旧实例保护的反例，确保没有为夹具放宽生产门禁。
	if _, err = f.s.p7AwakeExplore(f.ctx, f.id, &pb.CSAwakeCopySettle{}, now, 995001); err == nil {
		t.Fatal("old settlement timestamp consumed actual current exploration")
	}
	var exploration db.P7AwakeExploration
	if err = db.GetIns().First(&exploration, "run_id=?", fight.RunID).Error; err != nil {
		t.Fatal(err)
	}
	settleStamp := now.UnixMilli()
	if settleStamp <= exploration.EnterStamp {
		settleStamp = exploration.EnterStamp + 1
	}
	settled, err := f.s.p7AwakeExplore(f.ctx, f.id, &pb.CSAwakeCopySettle{}, now, settleStamp)
	if err != nil || settled[0].(*pb.SCAwakeCopySettle).GetScore() != 80 || len(settled[0].(*pb.SCAwakeCopySettle).ScoreRewards) != 0 {
		t.Fatal("real partial exploration settlement", err)
	}
	cold, err := f.s.p7AwakeExplore(f.ctx, f.id, &pb.CSAwakeCopySettle{}, now, settleStamp)
	if err != nil || !proto.Equal(settled[0], cold[0]) {
		t.Fatal("partial result repeated", err)
	}
}
