package service

import (
	"bufio"
	"context"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

type copyNativeSourceState struct {
	Round    int32 `json:"round"`
	Attacker struct {
		Actors []struct {
			UID       string `json:"uid"`
			HeroID    string `json:"heroId"`
			Attribute struct {
				HP float64 `json:"hp"`
			} `json:"attribute"`
		} `json:"actors"`
	} `json:"attacker"`
	Defender struct {
		Actors []struct {
			UID       string `json:"uid"`
			HeroID    string `json:"heroId"`
			Attribute struct {
				HP float64 `json:"hp"`
			} `json:"attribute"`
		} `json:"actors"`
	} `json:"defender"`
}

// readCopyOriginalNativeOutcome only accepts the identity from the original
// frozen reply and the actual original automatic-AI result; no victory is made up.
func readCopyOriginalNativeOutcome(t *testing.T, path string, start *pb.SCCopyFightStart) (*pb.CSCopyFightEnd, string) {
	t.Helper()
	raw, err := readTestAnalysis75(path)
	if err != nil {
		t.Fatal(err)
	}
	scanner := bufio.NewScanner(strings.NewReader(string(raw)))
	scanner.Buffer(make([]byte, 65536), 32<<20)
	var last copyNativeSourceState
	var operate pb.BattleOperate
	var inputCount, resultCount int
	var win bool
	for scanner.Scan() {
		var event struct {
			Kind        string                `json:"kind"`
			Scope       string                `json:"scope"`
			FightID     string                `json:"fightId"`
			Seed        int64                 `json:"seed"`
			BattleType  int32                 `json:"battleType"`
			Round       int32                 `json:"round"`
			Time        float64               `json:"time"`
			State       copyNativeSourceState `json:"state"`
			AttackerWon *bool                 `json:"attackerWon"`
			Data        struct {
				Camp       int32  `json:"camp"`
				ActorUID   string `json:"actorUid"`
				SkillIndex int32  `json:"skillIndex"`
			} `json:"data"`
		}
		if err := json.Unmarshal(scanner.Bytes(), &event); err != nil {
			t.Fatal(err)
		}
		switch event.Kind {
		case "input":
			inputCount++
			if event.FightID != start.GetFightId() || event.Seed != start.GetRandSeed() || event.BattleType != 1 || !strings.Contains(event.Scope, "automatic") {
				t.Fatal("original native identity/AI mode mismatch")
			}
		case "state":
			last = event.State
		case "skill_press":
			operate.SkillOpts = append(operate.SkillOpts, &pb.BattleSkillOperate{RoundNum: proto.Int32(event.Round), RoundTime: proto.Float64(event.Time), Camp: proto.Int32(event.Data.Camp), ActorUid: proto.String(event.Data.ActorUID), SkillIndex: proto.Int32(event.Data.SkillIndex)})
		case "result":
			resultCount++
			if event.AttackerWon == nil {
				t.Fatal("missing original boolean result")
			}
			win = *event.AttackerWon
		}
	}
	if err := scanner.Err(); err != nil {
		t.Fatal(err)
	}
	if inputCount != 1 || resultCount != 1 || last.Round < 1 || len(last.Attacker.Actors) != len(start.GetBattlePlayer().GetFormation()) {
		t.Fatal("incomplete original native evidence")
	}
	party := proto.Clone(start.GetBattlePlayer()).(*pb.BattlePlayer)
	seen := map[string]bool{}
	var attackerHp, defenderHp float64
	for _, native := range last.Attacker.Actors {
		found := false
		for _, actor := range party.Formation {
			if "0_"+actor.GetGuid() != native.UID {
				continue
			}
			if seen[native.UID] || native.HeroID != actor.GetId() || !combatFinite(native.Attribute.HP) || native.Attribute.HP < 0 || native.Attribute.HP > actor.GetProp().GetHp() {
				t.Fatal("original exact UID/HP mismatch")
			}
			actor.Inst.CurHp = proto.Float64(native.Attribute.HP)
			seen[native.UID] = true
			attackerHp += native.Attribute.HP
			found = true
			break
		}
		if !found {
			t.Fatal("original actor UID missing from frozen party", native.UID)
		}
	}
	for _, actor := range last.Defender.Actors {
		if !combatFinite(actor.Attribute.HP) || actor.Attribute.HP < 0 {
			t.Fatal("invalid original defender HP")
		}
		defenderHp += actor.Attribute.HP
	}
	operate.FinalRoundNum = proto.Int32(last.Round)
	operate.AttackerTotalLeftHp = proto.Float64(attackerHp)
	operate.DefenderTotalLeftHp = proto.Float64(defenderHp)
	result := int32(0)
	if win {
		result = 1
	}
	return &pb.CSCopyFightEnd{FightId: start.FightId, Result: proto.Int32(result), BattlePlayer: party, OptRecord: &operate}, p6Digest(raw)
}

// TestCopyOriginalPaidStateRecovery restores the entire real paid Player/Game.
// Authentication is recreated only in the isolated fresh database. Original
// battle identity and every protobuf field must match before the saved native
// winner can settle; ordinary awards are fresh actual draws, never inserted.
func TestCopyOriginalPaidStateRecovery(t *testing.T) {
	captureOnly := os.Getenv("HYJS_COPY_BOOK_RECOVERY_CAPTURE_MYSQL") == "1"
	if os.Getenv("HYJS_COPY_BOOK_RECOVERY_SQLITE") != "1" && !captureOnly {
		t.Skip("explicit genuine paid snapshot/original native winner recovery")
	}
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "copy-paid-recovery.db")
	open := func() {
		t.Helper()
		var err error
		if captureOnly {
			schema := os.Getenv("HYJS_P5_TALENT_MYSQL_DATABASE")
			if !strings.HasPrefix(schema, "hyjs_test_p5_talent_") || schema != os.Getenv("HYJS_MYSQL_DATABASE") {
				t.Fatal("dedicated original source recovery schema required")
			}
			err = db.InitGameMySQL(ctx, db.MySQLConfig{Host: os.Getenv("HYJS_MYSQL_HOST"), User: os.Getenv("HYJS_MYSQL_USER"), Password: os.Getenv("HYJS_MYSQL_PASSWORD"), Database: schema})
		} else {
			err = db.InitGame(ctx, path)
		}
		if err != nil {
			t.Fatal(err)
		}
	}
	open()
	defer func() { pool, _ := db.GetIns().DB(); pool.Close() }()
	s := &GameService{}
	for _, load := range []func(string) error{s.LoadTables, s.LoadP5Tables} {
		if err := load("../../analysis/tables"); err != nil {
			t.Fatal(err)
		}
	}
	for _, init := range []func(context.Context) error{s.InitializeP1, s.InitializeP5, s.InitializeOpenServerActivity, s.InitializeDailyTasks, s.InitializeCopyDrops, s.InitializeP6, s.InitializeDailyCopy, s.InitCooking} {
		if err := init(ctx); err != nil {
			t.Fatal(err)
		}
	}
	if checkpoint := os.Getenv("HYJS_COPY_BOOK_CONTINUATION_FILE"); captureOnly && checkpoint != "" {
		restoreCopySourceCheckpoint(t, ctx, s, checkpoint, os.Getenv("HYJS_COPY_BOOK_CONTINUATION_SHA256"))
		return
	}
	folder := "../../analysis/p6/copy-book-native-chain-20261014"
	raw, err := readTestAnalysis75(filepath.Join(folder, "000-paid-profile-state.json"))
	if err != nil {
		t.Fatal(err)
	}
	if p6Digest(raw) != "a6a060ababa6a89103d397487567de228c7bb9a8a02cb27889fdece868de6bc0" {
		t.Fatal("actual immutable paid snapshot SHA changed")
	}
	var snapshot struct {
		Player pb.Player       `json:"player"`
		Game   model.GameState `json:"game"`
	}
	if err := json.Unmarshal(raw, &snapshot); err != nil {
		t.Fatal(err)
	}
	if snapshot.Game.Run == nil || snapshot.Game.Run.Fight == nil || snapshot.Game.Run.Fight.Start == nil {
		t.Fatal("missing genuine pending fight")
	}
	start := snapshot.Game.Run.Fight.Start
	if start.GetFightId() != "5469211627dd34fbbfaf2518" || start.GetRandSeed() != 636902327 {
		t.Fatal("genuine original identity changed")
	}
	original, err := readTestAnalysis75(filepath.Join(folder, "000.pb"))
	if err != nil {
		t.Fatal(err)
	}
	if p6Digest(original) != "38e5a69bb2e466dcefebaebd2806a8709df8946f76a034492a39a758d1bf6e3f" {
		t.Fatal("original wire SHA changed")
	}
	restored := &pb.SCFightPlayback{Fightid: start.FightId, RandSeed: start.RandSeed, BattleType: proto.Int32(1), Attacker: start.BattlePlayer, Defender2: start.GetBattleMonsterList()[0], MaxRound: start.MaxRound, AttackerFactor: proto.Float64(1), DefenderFactor: proto.Float64(1)}
	var expected pb.SCFightPlayback
	if err := proto.Unmarshal(original, &expected); err != nil || !proto.Equal(restored, &expected) {
		t.Fatal("complete frozen wire did not survive whole paid state recovery", err)
	}
	a, err := LoginAccount(ctx, "original-native-book-recovery", "isolated-test-only")
	if err != nil {
		t.Fatal(err)
	}
	if a.Id != snapshot.Player.GetPlayerId() {
		t.Fatal("recovered owner ID must be identical")
	}
	playerJSON, _ := json.Marshal(&snapshot.Player)
	gameJSON, _ := json.Marshal(&snapshot.Game)
	if err := db.GetIns().Model(&db.Profile{}).Where("id=?", a.Id).Updates(map[string]any{"player": string(playerJSON), "game": string(gameJSON)}).Error; err != nil {
		t.Fatal(err)
	}
	// Only missing frozen metadata is rebuilt from the same original table and
	// declared strategy, preserving the actual paid state and original start frame.
	if err := db.GetIns().Transaction(func(tx *gorm.DB) error {
		if err := s.captureCopyDrops(tx, a.Id, start.GetFightId(), s.table("CopyTask", "TaskId", "168")); err != nil {
			return err
		}
		return s.CaptureOriginalCopyReplay(ctx, tx, a.Id, start)
	}); err != nil {
		t.Fatal(err)
	}
	msg, err := s.copyFightStart(ctx, a.Id, &pb.CSCopyFightStart{ChapterId: proto.String("8"), MissionId: proto.String("42"), TaskId: proto.String("168")})
	if err != nil || !proto.Equal(msg[0], start) {
		t.Fatal("real recovered original start frame", err)
	}
	if captureOnly {
		// The disposable HTTP caller will submit the actual original outcome.
		// Return before any settlement, preserving the zero-book paid checkpoint.
		t.Log("whole original paid state and pending start restored; metadata captured; zero books; actual HTTP outcome still required")
		return
	}
	finish, originalSHA := readCopyOriginalNativeOutcome(t, filepath.Join(folder, "000.native.jsonl"), start)
	if finish.GetResult() != 1 {
		t.Fatal("original case did not win")
	}
	reqCtx := context.WithValue(context.WithValue(ctx, constUtil.GameServiceContextKey, s), constUtil.GameRequestContextKey, finish)
	out, err := s.copyFightEnd(reqCtx, a.Id, finish)
	if err != nil {
		t.Fatal("original actual winner recovery settlement", err)
	}
	before := out[0]
	var row db.Profile
	if err := db.GetIns().First(&row, a.Id).Error; err != nil {
		t.Fatal(err)
	}
	p, g, err := decodeProfile(&row)
	if err != nil {
		t.Fatal(err)
	}
	gained := before.(*pb.SCCopyFightEnd).GetGain()
	amount := map[string]int64{}
	for _, item := range gained.GetSimps() {
		amount[item.GetId()] += int64(item.GetNum())
	}
	for _, key := range []string{"item_439", "item_440"} {
		if simpleOwned(p, g, key) != amount[key] || simpleOwned(&snapshot.Player, &snapshot.Game, key) != 0 {
			t.Fatal("material was prepared or recovery awarded more than the actual native response")
		}
	}
	coldBeforeP, coldBeforeG := *row.Player, *row.Game
	pool, _ := db.GetIns().DB()
	pool.Close()
	open()
	replay, err := s.copyFightEnd(reqCtx, a.Id, finish)
	if err != nil || !proto.Equal(before, replay[0]) {
		t.Fatal("original recovery cold receipt", err)
	}
	if err := db.GetIns().First(&row, a.Id).Error; err != nil || *row.Player != coldBeforeP || *row.Game != coldBeforeG {
		t.Fatal("recovery cold duplicated original reward", err)
	}
	dailyNow := time.Date(2026, 10, 14, 12, 0, 0, 0, time.FixedZone("UTC+8", 8*3600))
	dailyMessages, err := s.dailyCopyAt(ctx, a.Id, &pb.CSDailyCopyFightStart{ChapterId: proto.String("49")}, 200, dailyNow)
	if err != nil {
		t.Fatal("actual pending daily source checkpoint", err)
	}
	dailyStart := dailyMessages[0].(*pb.SCDailyCopyFightStart)
	if err := db.GetIns().First(&row, a.Id).Error; err != nil {
		t.Fatal(err)
	}
	p, g, err = decodeProfile(&row)
	if err != nil {
		t.Fatal(err)
	}
	coldBeforeP, coldBeforeG = *row.Player, *row.Game
	// A bounded source batch keeps every actual frozen row so a fresh
	// isolated database can continue the same books without replaying cultivation.
	var metadata []map[string]any
	for _, table := range []string{"copy_drop_snapshots", "p6_battle_records"} {
		var rows []map[string]any
		projection := "*, HEX(input) AS input"
		if table == "p6_battle_records" {
			projection += ", HEX(operates) AS operates, HEX(outcome) AS outcome"
		}
		if err := db.GetIns().Table(table).Select(projection).Where("player_id=?", a.Id).Find(&rows).Error; err != nil {
			t.Fatal(err)
		}
		for _, stored := range rows {
			metadata = append(metadata, stored)
		}
	}
	dailyMetadata := map[string][]map[string]any{}
	for _, table := range []string{"daily_copy_states", "daily_copy_fights", "daily_copy_start_receipts"} {
		var rows []map[string]any
		projection := "*"
		if table == "daily_copy_fights" {
			projection += ", HEX(start_snapshot) AS start_snapshot, HEX(end_request) AS end_request, HEX(response) AS response"
		}
		if err := db.GetIns().Table(table).Select(projection).Where("player_id=?", a.Id).Find(&rows).Error; err != nil {
			t.Fatal(err)
		}
		dailyMetadata[table] = rows
	}
	directory, err := os.MkdirTemp("../../analysis/p6", ".copy-checkpoint-isolated-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(directory) })
	checkpointPath := filepath.Join(directory, "actual-completed-paid-state.json")
	checkpointJSON, err := json.Marshal(map[string]any{"schema": "actual-paid-copy-state-with-frozen-source-metadata-v1", "player": p, "game": g, "sourceMetadataRows": metadata, "dailyMetadata": dailyMetadata, "credentialsIncluded": false})
	if err != nil || os.WriteFile(checkpointPath, checkpointJSON, 0600) != nil {
		t.Fatal("actual source checkpoint write", err)
	}
	pool, _ = db.GetIns().DB()
	pool.Close()
	if err := db.InitGame(ctx, filepath.Join(t.TempDir(), "copy-completed-continuation.db")); err != nil {
		t.Fatal(err)
	}
	for _, init := range []func(context.Context) error{s.InitializeP1, s.InitializeP5, s.InitializeOpenServerActivity, s.InitializeDailyTasks, s.InitializeCopyDrops, s.InitializeP6, s.InitializeDailyCopy, s.InitCooking} {
		if err := init(ctx); err != nil {
			t.Fatal(err)
		}
	}
	restoreCopySourceCheckpoint(t, ctx, s, checkpointPath, p6Digest(checkpointJSON))
	if err := db.GetIns().First(&row, a.Id).Error; err != nil || *row.Player != coldBeforeP || *row.Game != coldBeforeG {
		t.Fatal("whole paid source checkpoint changed during exact continuation recovery", err)
	}
	dailyResume, err := s.dailyCopyAt(ctx, a.Id, &pb.CSDailyCopyFightStart{ChapterId: proto.String("49")}, 201, dailyNow)
	if err != nil || !proto.Equal(dailyResume[0], dailyStart) {
		t.Fatal("actual original daily pending start changed during source continuation", err)
	}
	var quota db.DailyCopyState
	if err := db.GetIns().First(&quota, "player_id=? AND day=? AND type=?", a.Id, dailyCopyDay(dailyNow), "PetEquip").Error; err != nil || quota.Used != 1 || quota.Limit != 3 {
		t.Fatal("complete source recovery reset original actual daily quota", err)
	}
	replay, err = s.copyFightEnd(reqCtx, a.Id, finish)
	if err != nil || !proto.Equal(before, replay[0]) {
		t.Fatal("restored complete source metadata and original receipt", err)
	}
	t.Logf("whole genuine paid state and original wire match; originalID=%s seed=%d originalOutcomeSHA=%s real random medicine=%d certificate=%d; source native HP/operation used, no paidprep or book insertion", start.GetFightId(), start.GetRandSeed(), originalSHA, amount["item_439"], amount["item_440"])
}

// restoreCopySourceCheckpoint restores completed actual source states so another
// bounded batch can continue without repeating paid cultivation or adding books.
func restoreCopySourceCheckpoint(t *testing.T, ctx context.Context, s *GameService, path, expectedSHA string) {
	t.Helper()
	root, err := filepath.Abs("../../analysis/p6")
	if err != nil {
		t.Fatal(err)
	}
	root, err = filepath.EvalSymlinks(root)
	if err != nil {
		t.Fatal(err)
	}
	actual, err := filepath.Abs(path)
	if err == nil {
		actual, err = filepath.EvalSymlinks(actual)
	}
	if err != nil || !strings.HasPrefix(actual, root+string(os.PathSeparator)) || len(expectedSHA) != 64 {
		t.Fatal("credential-free local source checkpoint and exact manifest SHA required")
	}
	raw, err := readTestAnalysis75(actual)
	if err != nil || p6Digest(raw) != expectedSHA {
		t.Fatal("actual source checkpoint SHA mismatch", err)
	}
	var checkpoint struct {
		Schema             string                       `json:"schema"`
		Player             pb.Player                    `json:"player"`
		Game               model.GameState              `json:"game"`
		SourceMetadataRows []json.RawMessage            `json:"sourceMetadataRows"`
		DailyMetadata      map[string][]json.RawMessage `json:"dailyMetadata"`
		Credentials        bool                         `json:"credentialsIncluded"`
	}
	if json.Unmarshal(raw, &checkpoint) != nil || checkpoint.Schema != "actual-paid-copy-state-with-frozen-source-metadata-v1" || checkpoint.Credentials || checkpoint.Player.GetPlayerId() != 1 || checkpoint.Game.Run == nil || checkpoint.Game.Run.Fight == nil || !checkpoint.Game.Run.Fight.Settled {
		t.Fatal("only whole actual completed source checkpoints may start another source batch")
	}
	a, err := LoginAccount(ctx, "original-native-book-recovery", "isolated-test-only")
	if err != nil || a.Id != checkpoint.Player.GetPlayerId() {
		t.Fatal("actual source owner mismatch", err)
	}
	playerJSON, _ := json.Marshal(&checkpoint.Player)
	gameJSON, _ := json.Marshal(&checkpoint.Game)
	if err := db.GetIns().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&db.Profile{}).Where("id=?", a.Id).Updates(map[string]any{"player": string(playerJSON), "game": string(gameJSON)}).Error; err != nil {
			return err
		}
		for _, encoded := range checkpoint.SourceMetadataRows {
			var row map[string]any
			decoder := json.NewDecoder(strings.NewReader(string(encoded)))
			decoder.UseNumber()
			if err := decoder.Decode(&row); err != nil {
				return err
			}
			for key, value := range row {
				if number, ok := value.(json.Number); ok {
					integer, err := number.Int64()
					if err != nil {
						return err
					}
					row[key] = integer
				}
				if key == "input" || key == "operates" || key == "outcome" {
					if value == nil {
						continue
					}
					text, ok := value.(string)
					if !ok {
						t.Fatal("invalid actual checkpoint binary column")
					}
					decoded, err := hex.DecodeString(text)
					if err != nil {
						return err
					}
					row[key] = decoded
				}
			}
			if row["player_id"] != a.Id {
				t.Fatal("source metadata owner differs from whole paid profile")
			}
			if fightID, ok := row["fight_id"].(string); ok {
				input, ok := row["input"].([]byte)
				if !ok || p6Digest(input) != row["input_hash"] {
					t.Fatal("actual frozen drop checksum mismatch")
				}
				if err := tx.Table("copy_drop_snapshots").Create(row).Error; err != nil {
					return err
				}
				if fightID == "" {
					t.Fatal("missing actual source identity")
				}
			} else {
				if row["source"] != "copy" {
					t.Fatal("only actual copy source metadata is restored")
				}
				if err := tx.Table("p6_battle_records").Create(row).Error; err != nil {
					return err
				}
				var restored db.P6BattleRecord
				if err := tx.First(&restored, "id=?", row["id"]).Error; err != nil {
					return err
				}
				if _, err := decodeP6Record(&restored); err != nil {
					return err
				}
			}
		}
		// Preserve original daily quotas and exact starts/receipts when a bounded
		// source batch continues. Fresh authentication never resets consumed days.
		for _, table := range []string{"daily_copy_states", "daily_copy_fights", "daily_copy_start_receipts"} {
			for _, encoded := range checkpoint.DailyMetadata[table] {
				var row map[string]any
				decoder := json.NewDecoder(strings.NewReader(string(encoded)))
				decoder.UseNumber()
				if err := decoder.Decode(&row); err != nil {
					return err
				}
				for key, value := range row {
					if number, ok := value.(json.Number); ok {
						integer, err := number.Int64()
						if err != nil {
							return err
						}
						row[key] = integer
					}
					if key == "start_snapshot" || key == "end_request" || key == "response" {
						if value == nil {
							continue
						}
						text, ok := value.(string)
						if !ok {
							t.Fatal("invalid actual daily checkpoint bytes")
						}
						decoded, err := hex.DecodeString(text)
						if err != nil {
							return err
						}
						row[key] = decoded
					}
				}
				if row["player_id"] != a.Id {
					t.Fatal("actual daily checkpoint owner mismatch")
				}
				if err := tx.Table(table).Create(row).Error; err != nil {
					return err
				}
				if table == "daily_copy_fights" {
					var restored db.DailyCopyFight
					if err := tx.First(&restored, "fight_id=?", row["fight_id"]).Error; err != nil {
						return err
					}
					if _, err := decodeDailyCopyStart(&restored); err != nil {
						return err
					}
				}
			}
		}
		return nil
	}); err != nil {
		t.Fatal("actual complete source metadata recovery", err)
	}
	t.Log("whole completed source Player/Game and exact frozen copy/P6 bytes recovered; no paidprep or book insertion; further original AI outcomes still required")
}
