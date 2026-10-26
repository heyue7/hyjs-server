package service

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"google.golang.org/protobuf/proto"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	pb "hyjs-server/model/protocol"
	"math"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func dailyPlaybackSetup(t *testing.T) (*p1Fixture, int64) {
	t.Helper()
	f := &p1Fixture{ctx: context.Background(), path: filepath.Join(t.TempDir(), "daily-playback.db")}
	if name := os.Getenv("HYJS_DPLAY_MYSQL_DATABASE"); name != "" {
		if !strings.HasPrefix(name, "hyjs_test_dplay_") {
			t.Fatal("Daily playback isolation guard")
		}
		f.mysql = db.MySQLConfig{Host: os.Getenv("HYJS_DPLAY_MYSQL_HOST"), User: os.Getenv("HYJS_DPLAY_MYSQL_USER"), Password: os.Getenv("HYJS_DPLAY_MYSQL_PASSWORD"), Database: name}
	}
	f.open(t)
	t.Cleanup(func() { connection, _ := db.GetIns().DB(); connection.Close() })
	f.s = &GameService{}
	if err := f.s.LoadTables("../../analysis/tables"); err != nil {
		t.Fatal(err)
	}
	if err := f.s.InitializeDailyCopy(f.ctx); err != nil {
		t.Fatal(err)
	}
	if err := f.s.InitializeP6(f.ctx); err != nil {
		t.Fatal(err)
	}
	f.id = dailyCopyTestPlayer(t, f.ctx, "daily-playback-"+t.Name()+"-owner", 75)
	peer := dailyCopyTestPlayer(t, f.ctx, "daily-playback-"+t.Name()+"-peer", 75)
	return f, peer
}
func dailyPlaybackNative(t *testing.T, f *p1Fixture, name string, stamp int64) (db.DailyCopyFight, db.DailyCopyStartReceipt, *pb.SCDailyCopyFightStart, *pb.CSDailyCopyFightEnd) {
	t.Helper()
	raw, err := readTestAnalysis75("../../analysis/p6/" + name)
	if err != nil {
		t.Fatal(err)
	}
	var fixture struct {
		StartHex string `json:"start_hex"`
		EndHex   string `json:"end_hex"`
	}
	if err = json.Unmarshal(raw, &fixture); err != nil {
		t.Fatal(err)
	}
	startBytes, err := hex.DecodeString(fixture.StartHex)
	if err != nil {
		t.Fatal(err)
	}
	endBytes, err := hex.DecodeString(fixture.EndHex)
	if err != nil {
		t.Fatal(err)
	}
	start := &pb.SCDailyCopyFightStart{}
	end := &pb.CSDailyCopyFightEnd{}
	if proto.Unmarshal(startBytes, start) != nil || proto.Unmarshal(endBytes, end) != nil {
		t.Fatal("native protocol decode")
	}
	// 专库仅替换归属者整数ID，原队伍实例GUID、随机种子、操作、双方属性和终局保持原样。
	start.BattlePlayer.PlayerId = proto.Int64(f.id)
	kind, stamina := "TreasureExp", int32(10)
	if end.GetChapterId() == "28" {
		kind, stamina = "PetFood_dark", 30
	}
	startBytes, err = proto.Marshal(start)
	if err != nil {
		t.Fatal(err)
	}
	response, err := proto.Marshal(&pb.SCDailyCopyFightEnd{IsWin: proto.Int32(end.GetIsWin()), Get: &pb.ResultObject{}, Cost: []*pb.BaseResource{{Id: proto.String("stamina"), Num: proto.Int32(stamina)}}, LastTime: proto.Int32(0), StarNum: proto.Int32(0)})
	if err != nil {
		t.Fatal(err)
	}
	row := db.DailyCopyFight{FightID: start.GetFightId(), PlayerID: f.id, StartStamp: stamp, Day: "2026-10-13", Type: kind, ChapterID: end.GetChapterId(), Stamina: stamina, WinStars: 3, Reward: "coin,1", Start: startBytes, EndRequest: endBytes, Response: response, Settled: true}
	receipt := db.DailyCopyStartReceipt{PlayerID: f.id, Stamp: stamp, ChapterID: row.ChapterID, FightID: row.FightID}
	if err = db.GetIns().Where("fight_id=?", row.FightID).Delete(&db.DailyCopyFight{}).Error; err != nil {
		t.Fatal(err)
	}
	if err = db.GetIns().Where("player_id=? AND stamp=?", f.id, stamp).Delete(&db.DailyCopyStartReceipt{}).Error; err != nil {
		t.Fatal(err)
	}
	if err = db.GetIns().Create(&row).Error; err != nil {
		t.Fatal(err)
	}
	if err = db.GetIns().Create(&receipt).Error; err != nil {
		t.Fatal(err)
	}
	return row, receipt, start, end
}
func TestDailyPlaybackNativeTwoLossesOriginalProtocolConcurrentRestartNoWrites(t *testing.T) {
	f, peer := dailyPlaybackSetup(t)
	for index, name := range []string{"daily-id4-native-20261013.json", "daily-fooddark-id4-native-20261013.json"} {
		row, receipt, start, end := dailyPlaybackNative(t, f, name, int64(100+index))
		var beforeProfile db.Profile
		db.GetIns().First(&beforeProfile, f.id)
		var beforeRules []db.P6CombatRule
		db.GetIns().Order("id").Find(&beforeRules)
		var recordsBefore int64
		db.GetIns().Model(&db.P6BattleRecord{}).Count(&recordsBefore)
		var wg sync.WaitGroup
		replies := make(chan *pb.SCFightPlayback, 8)
		errs := make(chan error, 8)
		for i := 0; i < 8; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				out, err := f.s.dailyPlayback(f.ctx, f.id, row.FightID)
				errs <- err
				if err == nil {
					replies <- out[0].(*pb.SCFightPlayback)
				}
			}()
		}
		wg.Wait()
		close(errs)
		close(replies)
		for err := range errs {
			if err != nil {
				t.Fatal(err)
			}
		}
		expected := &pb.SCFightPlayback{Fightid: start.FightId, RandSeed: start.RandSeed, BattleType: proto.Int32(4), Attacker: start.BattlePlayer, Defender2: start.Monster, OptRecord: end.OptRecord, ServerResult: proto.Int32(1), ClientResult: proto.Int32(1), AttackerFactor: proto.Float64(1), DefenderFactor: proto.Float64(1), MaxRound: start.MaxRoundNum}
		for reply := range replies {
			if !proto.Equal(reply, expected) {
				t.Fatal("native daily playback changed original input/op/loss", reply)
			}
		}
		if _, err := f.s.dailyPlayback(f.ctx, peer, row.FightID); err == nil {
			t.Fatal("cross account replay exposed")
		}
		if _, err := f.s.dailyPlayback(f.ctx, f.id, "missing"); err == nil {
			t.Fatal("missing replay manufactured")
		}
		f.restart(t)
		out, err := f.s.dailyPlayback(f.ctx, f.id, row.FightID)
		if err != nil || !proto.Equal(out[0], expected) {
			t.Fatal("restart replay differs", err)
		}
		var afterProfile db.Profile
		db.GetIns().First(&afterProfile, f.id)
		a, _ := json.Marshal(beforeProfile)
		b, _ := json.Marshal(afterProfile)
		if string(a) != string(b) || *beforeProfile.Player != *afterProfile.Player || *beforeProfile.Game != *afterProfile.Game {
			t.Fatal("readonly replay changed resources/profile")
		}
		var afterRules []db.P6CombatRule
		db.GetIns().Order("id").Find(&afterRules)
		a, _ = json.Marshal(beforeRules)
		b, _ = json.Marshal(afterRules)
		if string(a) != string(b) {
			t.Fatal("replay changed P6 policy authority")
		}
		var recordsAfter int64
		db.GetIns().Model(&db.P6BattleRecord{}).Count(&recordsAfter)
		if recordsBefore != recordsAfter {
			t.Fatal("readonly replay created P6 history")
		}
		var afterRow db.DailyCopyFight
		db.GetIns().First(&afterRow, "fight_id=?", row.FightID)
		a, _ = json.Marshal(row)
		b, _ = json.Marshal(afterRow)
		if string(a) != string(b) {
			t.Fatal("replay rewrote daily snapshots")
		}
		var afterReceipt db.DailyCopyStartReceipt
		db.GetIns().First(&afterReceipt, "player_id=? AND stamp=?", f.id, receipt.Stamp)
		if afterReceipt != receipt {
			t.Fatal("replay rewrote start receipt")
		}
	}
}
func TestDailyPlaybackUnsettledMissingSnapshotsAndTripleConflictsReject(t *testing.T) {
	f, _ := dailyPlaybackSetup(t)
	original, receipt, _, _ := dailyPlaybackNative(t, f, "daily-id4-native-20261013.json", 201)
	for _, kind := range []string{"unsettled", "start", "end", "response", "receipt", "wrong-owner", "wrong-fight", "wrong-chapter", "response-win", "cost", "loss-gain", "loss-stars", "unknown-actor", "nan-time"} {
		t.Run(kind, func(t *testing.T) {
			row := original
			r := receipt
			start := &pb.SCDailyCopyFightStart{}
			end := &pb.CSDailyCopyFightEnd{}
			response := &pb.SCDailyCopyFightEnd{}
			proto.Unmarshal(row.Start, start)
			proto.Unmarshal(row.EndRequest, end)
			proto.Unmarshal(row.Response, response)
			switch kind {
			case "unsettled":
				row.Settled = false
			case "start":
				row.Start = nil
			case "end":
				row.EndRequest = nil
			case "response":
				row.Response = nil
			case "receipt":
				r.Stamp++
			case "wrong-owner":
				start.BattlePlayer.PlayerId = proto.Int64(f.id + 1)
				row.Start, _ = proto.Marshal(start)
			case "wrong-fight":
				end.FightId = proto.String("other")
				row.EndRequest, _ = proto.Marshal(end)
			case "wrong-chapter":
				end.ChapterId = proto.String("other")
				row.EndRequest, _ = proto.Marshal(end)
			case "response-win":
				response.IsWin = proto.Int32(1)
				row.Response, _ = proto.Marshal(response)
			case "cost":
				response.Cost[0].Num = proto.Int32(1)
				row.Response, _ = proto.Marshal(response)
			case "loss-gain":
				response.Get = &pb.ResultObject{Simps: []*pb.SimpleItem{{Id: proto.String("coin"), Num: proto.Int32(1)}}}
				row.Response, _ = proto.Marshal(response)
			case "loss-stars":
				response.StarNum = proto.Int32(1)
				row.Response, _ = proto.Marshal(response)
			case "unknown-actor":
				end.OptRecord.SkillOpts[0].ActorUid = proto.String("unknown")
				row.EndRequest, _ = proto.Marshal(end)
			case "nan-time":
				end.OptRecord.SkillOpts[0].RoundTime = proto.Float64(math.NaN())
				row.EndRequest, _ = proto.Marshal(end)
			}
			if _, err := dailyPlaybackProjection(&row, &r); err == nil {
				t.Fatal("invalid original triple accepted", kind)
			}
		})
	}
	ctx, cancel := context.WithCancel(f.ctx)
	cancel()
	if _, err := f.s.dailyPlayback(ctx, f.id, original.FightID); err == nil {
		t.Fatal("canceled replay context ignored")
	}
}
func TestDailyPlaybackOriginalSuccessfulBusinessAndExistingP6HashConflictNoFallback(t *testing.T) {
	f, _ := dailyPlaybackSetup(t)
	now := time.Date(2026, 10, 13, 12, 0, 0, 0, time.FixedZone("UTC+8", 8*3600))
	out, err := f.s.dailyCopyAt(f.ctx, f.id, &pb.CSDailyCopyFightStart{ChapterId: proto.String("5")}, 301, now)
	if err != nil {
		t.Fatal(err)
	}
	start := out[0].(*pb.SCDailyCopyFightStart)
	end := dailyCopyWin(start, "5")
	out, err = f.s.dailyCopyAt(f.ctx, f.id, end, 302, now)
	if err != nil {
		t.Fatal(err)
	}
	settled := out[0].(*pb.SCDailyCopyFightEnd)
	if settled.GetIsWin() != 1 {
		t.Fatal("actual legacy success receipt missing")
	}
	played, err := f.s.dailyPlayback(f.ctx, f.id, start.GetFightId())
	if err != nil {
		t.Fatal(err)
	}
	reply := played[0].(*pb.SCFightPlayback)
	if reply.GetServerResult() != 0 || reply.GetClientResult() != 0 || !proto.Equal(reply.OptRecord, end.OptRecord) {
		t.Fatal("actual success mapping")
	}
	// 原P6记录即使同fid并Hash损坏，也必须留在原冲突路径，不回退成另一来源的有效录像。
	if err = db.GetIns().Create(&db.P6BattleRecord{ID: fmt.Sprintf("daily-collision-%d", f.id), PlayerID: f.id, Source: "copy", SourceFight: start.GetFightId(), Status: constUtil.P6BattleObserved, Input: []byte("corrupt"), InputHash: "bad"}).Error; err != nil {
		t.Fatal(err)
	}
	if _, err = f.s.fightPlayback(f.ctx, f.id, &pb.CSFightPlayback{FightId: start.FightId, ServerId: proto.Int32(1)}); err == nil || !strings.Contains(err.Error(), constUtil.P6ReplayConflict) {
		t.Fatal("P6 hash conflict downgraded", err)
	}
}
