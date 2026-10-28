package service

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

// TestP9OrdinaryRecruitLifecycle validates the declared local pool separately
// from the original recovered price/free/coin and the independent first gift.
func TestP9OrdinaryRecruitLifecycle(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "p9-recruit.db")
	open := func() {
		t.Helper()
		var err error
		if name := os.Getenv("HYJS_P9_MYSQL_DATABASE"); name != "" {
			if !strings.HasPrefix(name, "hyjs_test_p9_") {
				t.Fatal("isolated p9 database required")
			}
			err = db.InitGameMySQL(ctx, db.MySQLConfig{Host: os.Getenv("HYJS_MYSQL_HOST"), User: os.Getenv("HYJS_MYSQL_USER"), Password: os.Getenv("HYJS_MYSQL_PASSWORD"), Database: name})
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
	for _, load := range []func(string) error{s.LoadTables, s.LoadP9Tables} {
		if err := load("../../analysis/tables"); err != nil {
			t.Fatal(err)
		}
	}
	for _, init := range []func(context.Context) error{s.InitializeP1, s.InitializeP9Pets} {
		if err := init(ctx); err != nil {
			t.Fatal(err)
		}
	}
	var rule db.P9PetRecruitRule
	if err := db.GetIns().First(&rule, "id=?", constUtil.P9RecruitRuleID).Error; err != nil {
		t.Fatal(err)
	}
	input, err := s.p9RecruitInput(rule)
	if err != nil {
		t.Fatal(err)
	}
	group := map[string]int64{}
	for _, option := range input.Pool {
		group[s.table("Pet", "id", option.PetID)["quality"]] += option.Weight
	}
	if len(input.Pool) != 59 || group["18"] != group["20"]*99 || group["23"] != 0 {
		t.Fatal("non-red 99/1 declared quality weights", len(input.Pool), group)
	}
	a, err := LoginAccount(ctx, "p9-ordinary", "isolated-only")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := CreatePlayer(ctx, a.Id, "普通精灵链", "hero_043"); err != nil {
		t.Fatal(err)
	}
	if err := updateProfile(ctx, a.Id, func(p *pb.Player, g *model.GameState) error {
		p.Level = proto.Int32(34)
		p.PetBagSize = proto.Int32(100)
		p.Diamond = proto.Int64(2000)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	read := func() (*pb.Player, *model.GameState) {
		t.Helper()
		var row db.Profile
		if err := db.GetIns().First(&row, a.Id).Error; err != nil {
			t.Fatal(err)
		}
		p, g, e := decodeProfile(&row)
		if e != nil {
			t.Fatal(e)
		}
		return p, g
	}
	query := func() *pb.PetRecruit {
		t.Helper()
		out, e := s.P9PetRecruitInfo(ctx, a.Id)
		if e != nil {
			t.Fatal(e)
		}
		return out[0].(*pb.SCPetRecruitGet).GetRecruit()
	}
	if _, err := s.p9PetRecruit(ctx, a.Id, &pb.CSPetRecruit{Free: proto.Int32(1), NumType: proto.Int32(1)}, 1); err == nil {
		t.Fatal("ordinary bypassed first gift")
	}
	if _, err := s.p9PetFirst(ctx, a.Id, 2, &pb.CSPetFirstRecruit{}); err != nil {
		t.Fatal(err)
	}
	info := query()
	if info.GetNum() != 1 || info.GetMustTimes() != 9 || info.GetReward() != "" || info.GetNextFree() <= time.Now().UnixMilli() {
		t.Fatal("original free/pity query", info)
	}
	free := &pb.CSPetRecruit{Free: proto.Int32(1), NumType: proto.Int32(1)}
	var wg sync.WaitGroup
	out := make(chan proto.Message, 8)
	errs := make(chan error, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			msgs, e := s.p9PetRecruit(ctx, a.Id, free, 3)
			errs <- e
			if e == nil {
				out <- msgs[0]
			}
		}()
	}
	wg.Wait()
	close(out)
	close(errs)
	for e := range errs {
		if e != nil {
			t.Fatal(e)
		}
	}
	var first proto.Message
	for reply := range out {
		if first == nil {
			first = reply
		}
		if !proto.Equal(first, reply) {
			t.Fatal("same free frame rerolled")
		}
	}
	if _, err := s.p9PetRecruit(ctx, a.Id, &pb.CSPetRecruit{Free: proto.Int32(0), NumType: proto.Int32(1)}, 3); err == nil {
		t.Fatal("same ordinary receipt allowed request collision")
	}
	freeReply := first.(*pb.SCPetRecruit)
	if len(freeReply.GetGet().GetPets()) != 1 || len(freeReply.GetGetList()) != 1 || len(freeReply.GetCost()) != 0 || freeReply.GetGet().GetSimps()[0].GetNum() != 2000 {
		t.Fatal("original free card/coin result")
	}
	p, g := read()
	if p.GetDiamond() != 2000 || p.GetCoin() != 2000 || query().GetNum() != 0 {
		t.Fatal("free balance/cooldown")
	}
	beforeP := proto.Clone(p)
	beforeG, _ := json.Marshal(g)
	if _, err := s.p9PetRecruit(ctx, a.Id, free, 4); err == nil {
		t.Fatal("second free draw accepted")
	}
	p, g = read()
	raw, _ := json.Marshal(g)
	if !proto.Equal(beforeP, p) || string(raw) != string(beforeG) {
		t.Fatal("invalid free partly changed profile")
	}
	// An already-started account follows the edited pool without resetting pity or replaying rewards.
	var existing db.P9PetRecruitProgress
	if err := db.GetIns().First(&existing, "player_id=?", a.Id).Error; err != nil {
		t.Fatal(err)
	}
	oldDraws, oldSince := existing.Draws, existing.SinceGuarantee
	legacy := *input
	legacy.Pool = []p9RecruitOption{{PetID: "pet_009", Weight: 70}, {PetID: "pet_046", Weight: 1, Guarantee: true}}
	legacy.Source = constUtil.P9RecruitLegacySource
	legacyRaw, _ := json.Marshal(legacy)
	if err := db.GetIns().Model(&existing).Updates(map[string]any{"input": legacyRaw, "input_hash": p6Digest(legacyRaw)}).Error; err != nil {
		t.Fatal(err)
	}
	if query().GetNum() != 0 {
		t.Fatal("pool change reset free quota")
	}
	if err := db.GetIns().First(&existing, "player_id=?", a.Id).Error; err != nil {
		t.Fatal(err)
	}
	var adopted p9RecruitInput
	if json.Unmarshal(existing.Input, &adopted) != nil || len(adopted.Pool) != 59 || existing.Draws != oldDraws || existing.SinceGuarantee != oldSince {
		t.Fatal("existing progress was reset or retained red pool")
	}
	replay, err := s.p9PetRecruit(ctx, a.Id, free, 3)
	if err != nil || !proto.Equal(replay[0], first) {
		t.Fatal("pool change altered committed receipt", err)
	}
	// Invalid live configuration fails closed; restoring it preserves quota and history.
	if err := db.GetIns().Model(&rule).Update("pool", []byte(`[]`)).Error; err != nil {
		t.Fatal(err)
	}
	five := &pb.CSPetRecruit{Free: proto.Int32(0), NumType: proto.Int32(2)}
	if _, err := s.p9PetRecruit(ctx, a.Id, five, 4); err == nil {
		t.Fatal("invalid live pool accepted")
	}
	restorePool, _ := json.Marshal(input.Pool)
	if err := db.GetIns().Model(&rule).Update("pool", restorePool).Error; err != nil {
		t.Fatal(err)
	}
	result, err := s.p9PetRecruit(ctx, a.Id, five, 4)
	if err != nil {
		t.Fatal(err)
	}
	paid := result[0].(*pb.SCPetRecruit)
	if len(paid.GetGet().GetPets()) != 5 || len(paid.GetGetList()) != 5 || paid.GetCost()[0].GetNum() != 700 || paid.GetGet().GetSimps()[0].GetNum() != 10000 {
		t.Fatal("original five price/result")
	}
	pets, err := P9PetList(db.GetIns(), a.Id)
	if err != nil || len(pets) != 7 {
		t.Fatal("actual unique inventory", len(pets), err)
	}
	// Explicit isolation preparation puts this player at the genuine next guaranteed draw.
	if err := db.GetIns().Model(&db.P9PetRecruitProgress{}).Where("player_id=?", a.Id).Update("since_guarantee", 9).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.GetIns().Model(&db.P9PetState{}).Where("player_id=?", a.Id).Update("must_times", 0).Error; err != nil {
		t.Fatal(err)
	}
	forced, err := s.p9PetRecruit(ctx, a.Id, &pb.CSPetRecruit{Free: proto.Int32(0), NumType: proto.Int32(1)}, 5)
	if err != nil {
		t.Fatal(err)
	}
	pet := forced[0].(*pb.SCPetRecruit).GetGet().GetPets()[0]
	quality := s.table("Pet", "id", pet.GetId())["quality"]
	if quality != "20" || query().GetMustTimes() != 9 {
		t.Fatal("guaranteed local quality did not reset")
	}
	if err := db.GetIns().Model(&db.P9PetState{}).Where("player_id=?", a.Id).Update("next_free", time.Now().Add(-50*time.Hour).UnixMilli()).Error; err != nil {
		t.Fatal(err)
	}
	renewed := query()
	if renewed.GetNum() != 1 || renewed.GetNextFree() <= time.Now().UnixMilli() {
		t.Fatal("offline periods did not cap/rebase")
	}
	// With distinct frames only the single replenished free slot can succeed.
	success := 0
	errorsOut := make(chan error, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func(stamp int64) { defer wg.Done(); _, e := s.p9PetRecruit(ctx, a.Id, free, stamp); errorsOut <- e }(int64(10 + i))
	}
	wg.Wait()
	close(errorsOut)
	for e := range errorsOut {
		if e == nil {
			success++
		}
	}
	if success != 1 {
		t.Fatal("distinct free races duplicated quota", success)
	}
	if err := db.GetIns().Model(&rule).Update("enabled", false).Error; err != nil {
		t.Fatal(err)
	}
	if query().GetNum() != 0 {
		t.Fatal("disabled pool advertised free")
	}
	if _, err := s.p9PetRecruit(ctx, a.Id, five, 20); err == nil {
		t.Fatal("disabled pool awarded")
	}
	pool, _ := db.GetIns().DB()
	pool.Close()
	open()
	cold, e := s.p9PetRecruit(ctx, a.Id, five, 4)
	if e != nil || !proto.Equal(paid, cold[0]) {
		t.Fatal("disabled cold old paid receipt changed", e)
	}
	t.Log("59 non-red original IDs / declared99:1 pool, independent first gift, original150/700 prices and2000coin/card, eight-way exact/free races, current pool, preserved next10 pity, offline cap and disabled cold receipt passed")
}
