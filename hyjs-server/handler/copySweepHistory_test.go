package handler

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
	"hyjs-server/service"
)

// The same original signed HTTP/Protobuf test runs with an explicitly leased
// hyjs_test_sweep_* MySQL database, or an isolated temporary SQLite database.
func TestCopySweepHistoryOriginalSignedHTTP(t *testing.T) {
	ctx := context.Background()
	name := os.Getenv("HYJS_COPY_SWEEP_MYSQL_DATABASE")
	var err error
	if name == "" {
		err = db.InitGame(ctx, filepath.Join(t.TempDir(), "sweep-history.db"))
	} else {
		if !strings.HasPrefix(name, "hyjs_test_sweep_") {
			t.Fatal("sweep test requires its own leased database")
		}
		err = db.InitGameMySQL(ctx, db.MySQLConfig{Host: os.Getenv("HYJS_COPY_SWEEP_MYSQL_HOST"), User: os.Getenv("HYJS_COPY_SWEEP_MYSQL_USER"), Password: os.Getenv("HYJS_COPY_SWEEP_MYSQL_PASSWORD"), Database: name})
	}
	if err != nil {
		t.Fatal("isolated database initialization failed", err)
	}
	connection, _ := db.GetIns().DB()
	t.Cleanup(func() { connection.Close() })
	s := &service.GameService{}
	if err := s.LoadTables("../../analysis/tables"); err != nil {
		t.Fatal(err)
	}
	for _, init := range []func(context.Context) error{s.InitializeGameplayPools, s.InitializeP1, s.InitializeP6, s.InitCooking, s.InitializeDailyCopy, s.InitializeCopyDrops, s.InitializeDailyTasks, s.InitializeP9CopyIntimacy, s.InitializeCopySweepWitness} {
		if err := init(ctx); err != nil {
			t.Fatal(err)
		}
	}
	s.P9PetsReady, s.CopySweepHistoryComplete = true, true
	account, err := service.LoginAccount(ctx, "sweep-history-wire", "isolated-password")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.CreatePlayer(ctx, account.Id, "扫荡原协议", "hero_043"); err != nil {
		t.Fatal(err)
	}
	var row db.Profile
	if err := db.GetIns().First(&row, account.Id).Error; err != nil {
		t.Fatal(err)
	}
	var player pb.Player
	var game model.GameState
	if err := json.Unmarshal([]byte(*row.Player), &player); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal([]byte(*row.Game), &game); err != nil {
		t.Fatal(err)
	}
	player.Level, player.Stamina, player.Exp = proto.Int32(100), proto.Int32(1000), proto.Int64(0)
	game.Copy.PassedCopyInfo = []*pb.CopySimpleInfo{{ChapterId: proto.String("1"), MissionId: proto.String("1"), Star: proto.Int32(5)}, {ChapterId: proto.String("1"), MissionId: proto.String("2"), Star: proto.Int32(5)}, {ChapterId: proto.String("3"), MissionId: proto.String("12"), Star: proto.Int32(5)}}
	playerRaw, _ := json.Marshal(&player)
	gameRaw, _ := json.Marshal(&game)
	if err := db.GetIns().Model(&row).Updates(map[string]any{"player": string(playerRaw), "game": string(gameRaw)}).Error; err != nil {
		t.Fatal(err)
	}
	previous := gameService
	ConfigureGame(s)
	t.Cleanup(func() { gameService = previous })
	req := &pb.CSCopySweep{ChapterId: proto.String("1"), MissionId: proto.String("1")}
	other := &pb.CSCopySweep{ChapterId: proto.String("1"), MissionId: proto.String("2")}
	call := func(stamp int64, session []byte, request *pb.CSCopySweep) proto.Message {
		return feedback84WantedHTTP(t, account.Id, stamp, session, request)
	}
	start := time.Now()
	if _, ok := call(10000, account.Session, req).(*pb.SCCopySweep); !ok {
		t.Fatal("first original sweep did not settle")
	}
	firstDuration := time.Since(start)
	start = time.Now()
	low, ok := call(1000, account.Session, req).(*pb.SCCopySweep)
	if !ok {
		t.Fatal("lower original timestamp did not settle")
	}
	t.Logf("same request input original HTTP: first=%s lower=%s", firstDuration, time.Since(start))
	if again, ok := call(1000, account.Session, req).(*pb.SCCopySweep); !ok || !proto.Equal(low, again) {
		t.Fatal("latest signed request did not replay")
	}
	if _, ok := call(1000, account.Session, other).(*pb.ErrorMessage); !ok {
		t.Fatal("same timestamp across missions accepted")
	}
	oldSession := append([]byte(nil), account.Session...)
	account, err = service.LoginAccount(ctx, "sweep-history-wire", "isolated-password")
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := call(1000, oldSession, req).(*pb.ErrorMessage); !ok {
		t.Fatal("rotated session accepted")
	}
	if again, ok := call(1000, account.Session, req).(*pb.SCCopySweep); !ok || !proto.Equal(low, again) {
		t.Fatal("rewrapped latest request did not preserve its receipt")
	}
	if _, ok := call(10000, account.Session, req).(*pb.ErrorMessage); !ok {
		t.Fatal("rewrapped older request was treated as a new sweep")
	}
	thirdChapter := &pb.CSCopySweep{ChapterId: proto.String("3"), MissionId: proto.String("12")}
	readProfile := func() (*pb.Player, *model.GameState) {
		t.Helper()
		var profile db.Profile
		if err := db.GetIns().First(&profile, account.Id).Error; err != nil {
			t.Fatal(err)
		}
		var current pb.Player
		var state model.GameState
		if err := json.Unmarshal([]byte(*profile.Player), &current); err != nil {
			t.Fatal(err)
		}
		if err := json.Unmarshal([]byte(*profile.Game), &state); err != nil {
			t.Fatal(err)
		}
		return &current, &state
	}
	itemCount := func(state *model.GameState, id string) int64 {
		var count int64
		for _, item := range state.Items {
			if item.GetId() == id {
				count += int64(item.GetNum())
			}
		}
		return count
	}
	var last *pb.SCCopySweep
	var lastRequest *pb.CSCopySweep
	var elapsed time.Duration
	for index := 0; index < 10; index++ {
		beforeP, beforeG := readProfile()
		request := proto.Clone(thirdChapter).(*pb.CSCopySweep)
		if index == 9 {
			request.ProtoReflect().SetUnknown([]byte{0xa0, 0x06, 0x01})
		}
		start = time.Now()
		result, ok := call(20001+int64(index), account.Session, request).(*pb.SCCopySweep)
		elapsed += time.Since(start)
		if !ok || len(result.GetCost()) != 1 || result.GetCost()[0].GetId() != "stamina" || result.GetCost()[0].GetNum() != 10 {
			t.Fatal("third-chapter original sweep cost", index)
		}
		afterP, afterG := readProfile()
		gain := map[string]int64{}
		for _, item := range result.GetGain().GetSimps() {
			gain[item.GetId()] += int64(item.GetNum())
		}
		if afterP.GetStamina() != beforeP.GetStamina()-10 || afterP.GetCoin() != beforeP.GetCoin()+gain["coin"] || afterP.GetExp() != beforeP.GetExp()+gain["exp"] || itemCount(afterG, "item_124") != itemCount(beforeG, "item_124")+gain["item_124"] || len(afterG.Warriors) != len(beforeG.Warriors)+len(result.GetGain().GetWars()) {
			t.Fatal("third-chapter paid cost or actual drop mismatch", index)
		}
		var challenge int32 = -1
		for _, passed := range afterG.Copy.GetPassedCopyInfo() {
			if passed.GetChapterId() == "3" && passed.GetMissionId() == "12" {
				challenge = passed.GetChallengeTimes()
			}
		}
		var grants int64
		if err := db.GetIns().Model(&db.P9PetCopyGrant{}).Where("player_id=? AND source_key LIKE ?", account.Id, "sweep:%").Count(&grants).Error; err != nil || challenge != int32(index+1) || grants != int64(index+3) {
			t.Fatal("third-chapter challenge or grant count", index, challenge, grants, err)
		}
		last = result
		lastRequest = request
	}
	t.Logf("third-chapter ten original signed HTTP sweeps: total=%s mean=%s", elapsed, elapsed/10)
	beforeReplay, beforeReplayGame := readProfile()
	if repeated, ok := call(20010, account.Session, lastRequest).(*pb.SCCopySweep); !ok || !proto.Equal(last, repeated) {
		t.Fatal("third-chapter latest receipt changed")
	}
	changedUnknown := proto.Clone(lastRequest).(*pb.CSCopySweep)
	changedUnknown.ProtoReflect().SetUnknown([]byte{0xa0, 0x06, 0x02})
	if _, ok := call(20010, account.Session, changedUnknown).(*pb.ErrorMessage); !ok {
		t.Fatal("different protobuf extension reused a timestamp")
	}
	afterReplay, afterReplayGame := readProfile()
	if !proto.Equal(beforeReplay, afterReplay) || len(beforeReplayGame.Warriors) != len(afterReplayGame.Warriors) || itemCount(beforeReplayGame, "item_124") != itemCount(afterReplayGame, "item_124") {
		t.Fatal("third-chapter replay duplicated cost or drops")
	}
	if err := db.GetIns().First(&row, account.Id).Error; err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal([]byte(*row.Player), &player); err != nil {
		t.Fatal(err)
	}
	var grants int64
	if err := db.GetIns().Model(&db.P9PetCopyGrant{}).Where("player_id=? AND source_key LIKE ?", account.Id, "sweep:%").Count(&grants).Error; err != nil || grants != 12 || player.GetStamina() != 880 {
		t.Fatal("original HTTP cost or grant count", grants, player.GetStamina(), err)
	}
}
