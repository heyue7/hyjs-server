package service

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

// This fixture uses actual CreatePlayer/P5 builders. Only level25, current injury,
// a finite coin wallet and current-position readiness are declared prerequisites;
// it does not replace actors, skills, talents, native results or settlement.
func p12FieldPVPSetup(t *testing.T) (*p1Fixture, int64, int64, map[int64][]byte) {
	t.Helper()
	f := &p1Fixture{ctx: context.Background(), path: filepath.Join(t.TempDir(), "p12-pk.db")}
	if name := os.Getenv("HYJS_P12_PK_MYSQL_DATABASE"); name != "" {
		if !strings.HasPrefix(name, "hyjs_test_p12_pk_") {
			t.Fatal("P12 PK schema isolation")
		}
		f.mysql = db.MySQLConfig{Host: os.Getenv("HYJS_P12_PK_MYSQL_HOST"), User: os.Getenv("HYJS_P12_PK_MYSQL_USER"), Password: os.Getenv("HYJS_P12_PK_MYSQL_PASSWORD"), Database: name}
	}
	f.open(t)
	t.Cleanup(func() { pool, _ := db.GetIns().DB(); pool.Close() })
	f.s = &GameService{}
	for _, load := range []func(string) error{f.s.LoadTables, f.s.LoadP5Tables, f.s.LoadP8Tables, f.s.LoadP12FieldPVPTables} {
		if err := load("../../analysis/tables"); err != nil {
			t.Fatal(err)
		}
	}
	for _, init := range []func(context.Context) error{f.s.InitializeP1, f.s.InitializeP5, f.s.InitializeP6, f.s.InitializeP8Life, f.s.InitializeP10Social, f.s.InitializeP10Scene, f.s.InitializeP12FieldPVP, f.s.InitializeSharedFieldBoss55} {
		if err := init(f.ctx); err != nil {
			t.Fatal(err)
		}
	}
	if err := db.GetIns().AutoMigrate(&db.DailyCopyFight{}); err != nil {
		t.Fatal(err)
	}
	for _, table := range []any{&db.P12FieldPVPReceipt{}, &db.P12FieldPVPState{}, &db.P12FieldPVPPolicy{}, &db.P10ScenePresence{}} {
		if err := db.GetIns().Where("1=1").Delete(table).Error; err != nil {
			t.Fatal(err)
		}
	}
	if err := f.s.InitializeP12FieldPVP(f.ctx); err != nil {
		t.Fatal(err)
	}
	if err := db.GetIns().Model(&db.P10ScenePolicy{}).Where("id=1").Update("enabled", true).Error; err != nil {
		t.Fatal(err)
	}
	var ids []int64
	sessions := map[int64][]byte{}
	for i := 0; i < 3; i++ {
		row, err := LoginAccount(f.ctx, fmt.Sprintf("p12-pk-%s-%d-%d", t.Name(), i, time.Now().UnixNano()), "isolated-pk-password")
		if err != nil {
			t.Fatal(err)
		}
		if _, err = CreatePlayer(f.ctx, row.Id, fmt.Sprintf("PK隔离%d", i), "hero_047"); err != nil {
			t.Fatal(err)
		}
		if err = updateProfile(f.ctx, row.Id, func(p *pb.Player, g *model.GameState) error {
			p.Level = proto.Int32(25)
			p.Coin = proto.Int64(100000)
			g.Position = &pb.Position{PlayerId: p.PlayerId, Scene: proto.String("field_002"), Current: &pb.PointNew{X: proto.Float32(68 + float32(i)), Y: proto.Float32(15), Z: proto.Float32(84)}}
			g.PositionLoading = false
			g.Field = &model.FieldState{Scene: "field_002", Side: &pb.PlayerSide{PlayerId: p.PlayerId, State: proto.Int32(0), CurHp: proto.Float64(1), Anger: proto.Float64(0)}}
			return nil
		}); err != nil {
			t.Fatal(err)
		}
		sessions[row.Id] = append([]byte(nil), row.Session...)
		ids = append(ids, row.Id)
		p10SceneConnect(t, f, row.Id, &p10SceneRecorder{})
	}
	f.id = ids[0]
	return f, ids[1], ids[2], sessions
}
func p12FieldPVPEnable(t *testing.T) {
	t.Helper()
	if err := db.GetIns().Model(&db.P12FieldPVPPolicy{}).Where("id=1").Update("enabled", true).Error; err != nil {
		t.Fatal(err)
	}
}
func p12FieldPVPProfiles(t *testing.T, ids ...int64) []byte {
	t.Helper()
	var rows []db.Profile
	if err := db.GetIns().Where("id IN ?", ids).Order("id").Find(&rows).Error; err != nil {
		t.Fatal(err)
	}
	body, _ := json.Marshal(rows)
	var texts []string
	for _, row := range rows {
		texts = append(texts, *row.Player, *row.Game)
	}
	saved, _ := json.Marshal(texts)
	return append(body, saved...)
}
func p12FieldPVPResponse(t *testing.T, out []proto.Message, err error) *pb.SCSidePvpFightResult {
	t.Helper()
	if err != nil || len(out) < 1 {
		t.Fatal("PK request", err)
	}
	return out[0].(*pb.SCSidePvpFightResult)
}
func TestP12FieldPVPBusinessAtomicTerminalRetryColdAndMV(t *testing.T) {
	f, b, c, sessions := p12FieldPVPSetup(t)
	now := time.Now()
	p12FieldPVPEnable(t)
	// A real bounded injury is part of the saved field side; no result is supplied.
	if err := updateProfile(f.ctx, b, func(p *pb.Player, g *model.GameState) error { g.Field.Side.CurHp = proto.Float64(.01); return nil }); err != nil {
		t.Fatal(err)
	}
	request := &pb.CSSidePvpFightResult{PlayerId: proto.Int64(b), FightType: proto.Int32(0)}
	var wg sync.WaitGroup
	results := make(chan *pb.SCSidePvpFightResult, 8)
	errs := make(chan error, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			out, err := f.s.p12FieldPVPFightAt(f.ctx, f.id, sessions[f.id], request, 0, now)
			if err != nil {
				errs <- err
				return
			}
			results <- out[0].(*pb.SCSidePvpFightResult)
		}()
	}
	wg.Wait()
	close(results)
	close(errs)
	for err := range errs {
		t.Fatal(err)
	}
	var first *pb.SCSidePvpFightResult
	for response := range results {
		if response.GetCode() != 1 {
			t.Fatal("legal field did not compute", response)
		}
		if first == nil {
			first = response
		} else if !proto.Equal(first, response) {
			t.Fatal("same TCP busy-window request replay changed")
		}
	}
	if first == nil {
		t.Fatal("missing response")
	}
	var receipts []db.P12FieldPVPReceipt
	if err := db.GetIns().Where("player_id=?", f.id).Find(&receipts).Error; err != nil || len(receipts) != 1 {
		t.Fatal("duplicate settlement", len(receipts), err)
	}
	row := receipts[0]
	_, replay, err := f.s.p12FieldPVPReadReceipt(db.GetIns(), &row)
	if err != nil {
		t.Fatal(err)
	}
	if row.Stamp != nil || replay.AttPlayer.Formation[0].GetInst().GetCurHp() <= 0 || replay.DefPlayer.Formation[0].GetInst().GetCurHp() != replay.DefPlayer.Formation[0].GetProp().GetHp()*.01 {
		t.Fatal("original zero stamp/current injury lost")
	}
	var term model.CombatFinalHPOutcome
	if json.Unmarshal(row.Terminal, &term) != nil {
		t.Fatal("terminal decode")
	}
	for camp, id := range []int64{f.id, b} {
		var profile db.Profile
		if err = db.GetIns().First(&profile, id).Error; err != nil {
			t.Fatal(err)
		}
		p, g, e := decodeProfile(&profile)
		if e != nil {
			t.Fatal(e)
		}
		if g.Field.Side.GetCurHp() != term.FinalActors[camp].CurrentHP/term.FinalActors[camp].MaxHP {
			t.Fatal("terminal HP not applied", camp)
		}
		terminalParty := term.Outcome.Attacker
		if camp == 1 {
			terminalParty = term.Outcome.Defender
		}
		if g.Field.Side.GetAnger() != terminalParty.GetInst().GetAnger() {
			t.Fatal("terminal AP not applied")
		}
		expected := int64(100000)
		if id == f.id {
			expected += row.LootCoin
		} else {
			expected -= row.LootCoin
		}
		if p.GetCoin() != expected {
			t.Fatal("atomic coin delta", p.GetCoin(), expected)
		}
	}
	if first.GetWinPlayerId() == f.id && row.LootCoin != 2000 {
		t.Fatal("original2percent loot", row.LootCoin)
	}
	if err = P12FieldPVPRequireIdle(db.GetIns(), b, now.Add(time.Second)); err == nil {
		t.Fatal("other modes bypassed presentation busy lease")
	}
	reverse := &pb.CSSidePvpFightResult{PlayerId: proto.Int64(f.id), FightType: proto.Int32(0)}
	out, err := f.s.p12FieldPVPFightAt(f.ctx, b, sessions[b], reverse, 0, now.Add(time.Second))
	if err != nil || out[0].(*pb.SCSidePvpFightResult).GetCode() != 2002 {
		t.Fatal("reverse busy request", err)
	}
	before := p12FieldPVPProfiles(t, f.id, b)
	out, err = f.s.p12FieldPVPFightAt(f.ctx, f.id, sessions[f.id], request, 0, now.Add(2*time.Second))
	if !proto.Equal(first, p12FieldPVPResponse(t, out, err)) || !bytes.Equal(before, p12FieldPVPProfiles(t, f.id, b)) {
		t.Fatal("retry applied state twice")
	}
	f.restart(t)
	out, err = f.s.p12FieldPVPFightAt(f.ctx, f.id, sessions[f.id], request, 0, now.Add(3*time.Second))
	if !proto.Equal(first, p12FieldPVPResponse(t, out, err)) {
		t.Fatal("cold replay")
	}
	for _, id := range []int64{f.id, b} {
		list, e := f.s.p12FieldPVPMV(f.ctx, id, sessions[id], &pb.CSSideGetPvpMVList{})
		if e != nil || len(list[0].(*pb.SCSideGetPvpMVList).GetMvs()) != 1 {
			t.Fatal("participant MV list", e)
		}
		mv, e := f.s.p12FieldPVPMV(f.ctx, id, sessions[id], &pb.CSSideGetPvpMV{FightId: first.FightId})
		if e != nil || !proto.Equal(mv[0].(*pb.SCSideGetPvpMV).Data, replay) {
			t.Fatal("frozen actual replay", e)
		}
	}
	if _, err = f.s.p12FieldPVPMV(f.ctx, c, sessions[c], &pb.CSSideGetPvpMV{FightId: first.FightId}); err == nil {
		t.Fatal("foreign replay read")
	}
	bad := append([]byte(nil), sessions[f.id]...)
	bad[0] ^= 1
	if _, err = f.s.p12FieldPVPFightAt(f.ctx, f.id, bad, request, 0, now.Add(time.Second)); err == nil {
		t.Fatal("stale session replay")
	}
	corrupt := row
	corrupt.Terminal = append([]byte(nil), row.Terminal...)
	corrupt.Terminal[0] ^= 1
	if _, _, err = f.s.p12FieldPVPReadReceipt(db.GetIns(), &corrupt); err == nil {
		t.Fatal("corrupt terminal accepted")
	}
}
func TestP12FieldPVPBusinessClosedSafeDistanceBusyAndScopeNoMutation(t *testing.T) {
	f, b, _, sessions := p12FieldPVPSetup(t)
	now := time.Now()
	request := &pb.CSSidePvpFightResult{PlayerId: proto.Int64(b)}
	before := p12FieldPVPProfiles(t, f.id, b)
	if _, err := f.s.p12FieldPVPFightAt(f.ctx, f.id, sessions[f.id], request, 0, now); err == nil {
		t.Fatal("default-off bypass")
	}
	if !bytes.Equal(before, p12FieldPVPProfiles(t, f.id, b)) {
		t.Fatal("closed changed profiles")
	}
	p12FieldPVPEnable(t)
	for _, scenario := range []string{"safe", "first-map", "distance", "loading", "private-run", "dead", "other-line", "unsupported"} {
		t.Run(scenario, func(t *testing.T) {
			var profile db.Profile
			if err := db.GetIns().First(&profile, b).Error; err != nil {
				t.Fatal(err)
			}
			p, g, err := decodeProfile(&profile)
			if err != nil {
				t.Fatal(err)
			}
			originalP := proto.Clone(p).(*pb.Player)
			originalG, _ := json.Marshal(g)
			switch scenario {
			case "safe":
				g.Position.Current.X = proto.Float32(59)
				g.Position.Current.Z = proto.Float32(77)
			case "first-map":
				g.Position.Scene = proto.String("field_001")
				g.Field.Scene = "field_001"
			case "distance":
				g.Position.Current.X = proto.Float32(100)
			case "loading":
				g.PositionLoading = true
			case "private-run":
				g.Run = &model.CopyRun{Complete: false}
			case "dead":
				g.Field.Side.CurHp = proto.Float64(0)
				g.Field.Side.State = proto.Int32(5)
			case "other-line":
				if err = db.GetIns().Model(&db.P10ScenePresence{}).Where("player_id=?", b).Update("line", 2).Error; err != nil {
					t.Fatal(err)
				}
			case "unsupported":
				hero := f.s.table("Hero", "id", "hero_004")
				originalSkill := hero["SkillId1"]
				hero["SkillId1"] = "fixture75-unpublished-skill"
				t.Cleanup(func() { hero["SkillId1"] = originalSkill })
				for _, w := range g.Warriors {
					w.Id = proto.String("hero_004")
				}
			}
			if err = saveProfile(db.GetIns(), &profile, p, g); err != nil {
				t.Fatal(err)
			}
			before := p12FieldPVPProfiles(t, f.id, b)
			out, err := f.s.p12FieldPVPFightAt(f.ctx, f.id, sessions[f.id], request, 0, now)
			if err == nil && out[0].(*pb.SCSidePvpFightResult).GetCode() == 1 {
				t.Fatal("invalid PK succeeded")
			}
			if !bytes.Equal(before, p12FieldPVPProfiles(t, f.id, b)) {
				t.Fatal("rejected request mutated profiles")
			}
			if n := p1Count(t, &db.P12FieldPVPReceipt{}, "player_id=?", f.id); n != 0 {
				t.Fatal("rejected request receipt")
			}
			var restored model.GameState
			if json.Unmarshal(originalG, &restored) != nil {
				t.Fatal("restore")
			}
			if err = saveProfile(db.GetIns(), &profile, originalP, &restored); err != nil {
				t.Fatal(err)
			}
			if err = db.GetIns().Model(&db.P10ScenePresence{}).Where("player_id=?", b).Update("line", 1).Error; err != nil {
				t.Fatal(err)
			}
		})
	}
}
func TestP12FieldPVPBusinessPenaltyDailyCapsRedeemAndNaturalRelease(t *testing.T) {
	f, b, _, sessions := p12FieldPVPSetup(t)
	now := time.Now()
	p12FieldPVPEnable(t)
	if coin, err := f.s.p12FieldPVPLoot(99999, 499999); err != nil || coin != 1 {
		t.Fatal("original daily cap", coin, err)
	}
	if coin, err := f.s.p12FieldPVPLoot(1000000000, 0); err != nil || coin != 50000 {
		t.Fatal("original per-fight cap", coin, err)
	}
	// Persisted jail state represents a prior accepted red death for focused
	// redemption tests; no fabricated fight receipt or winner is created.
	state := db.P12FieldPVPState{PlayerID: b, Kills: 6, Day: now.In(time.FixedZone("Asia/Shanghai", 8*3600)).Format("2006-01-02"), DailyKills: 60, LootCoin: 500000, JailUntil: now.Add(time.Hour).UnixMilli(), Source: constUtil.P12FieldPVPSource}
	if err := db.GetIns().Save(&state).Error; err != nil {
		t.Fatal(err)
	}
	if err := updateProfile(f.ctx, b, func(p *pb.Player, g *model.GameState) error {
		g.Field.Side.Kills = proto.Int32(6)
		g.Field.Side.CurHp = proto.Float64(0)
		g.Field.Side.State = proto.Int32(5)
		p.RedeemTime = proto.Int64(state.JailUntil)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	query, err := f.s.p12FieldPVPRedeemAt(f.ctx, b, sessions[b], &pb.CSPlayerRedeem{Type: proto.Int32(0)}, 0, now)
	if err != nil || query[0].(*pb.SCPlayerRedeem).GetCost()[0].GetNum() != 10000 {
		t.Fatal("first original redemption", err)
	}
	if err = P12FieldPVPRequireIdle(db.GetIns(), b, now); err == nil {
		t.Fatal("jail bypass")
	}
	paid, err := f.s.p12FieldPVPRedeemAt(f.ctx, b, sessions[b], &pb.CSPlayerRedeem{Type: proto.Int32(1)}, 0, now)
	if err != nil || paid[0].(*pb.SCPlayerRedeem).GetRedeemTime() != 0 {
		t.Fatal("paid release", err)
	}
	var row db.Profile
	if err = db.GetIns().First(&row, b).Error; err != nil {
		t.Fatal(err)
	}
	p, g, err := decodeProfile(&row)
	if err != nil {
		t.Fatal(err)
	}
	if p.GetCoin() != 90000 || p.GetRedeemTime() != 0 || g.Field.Side.GetCurHp() <= 0 || g.Field.Side.GetState() != 0 || g.Position.GetScene() != "town_main" || !g.PositionLoading {
		t.Fatal("release state")
	}
	if err = db.GetIns().First(&state, "player_id=?", b).Error; err != nil {
		t.Fatal(err)
	}
	if state.RedeemCount != 1 || state.Kills != 0 || state.DailyKills != 60 || state.LootCoin != 500000 {
		t.Fatal("redemption washed daily caps")
	}
	// Retry when no remaining sentence costs nothing, including original TCP0.
	if _, err = f.s.p12FieldPVPRedeemAt(f.ctx, b, sessions[b], &pb.CSPlayerRedeem{Type: proto.Int32(1)}, 0, now); err != nil {
		t.Fatal(err)
	}
	f.restart(t)
	state.Kills = 11
	state.JailUntil = now.Add(time.Minute).UnixMilli()
	if err = db.GetIns().Save(&state).Error; err != nil {
		t.Fatal(err)
	}
	cost, err := f.s.p12FieldPVPRedeemCost(&state)
	if err != nil || cost != 11*8334 {
		t.Fatal("cold persistent subsequent redemption", cost, err)
	}
	state.Day = "2000-01-01"
	if err = db.GetIns().Save(&state).Error; err != nil {
		t.Fatal(err)
	}
	err = db.GetIns().Transaction(func(tx *gorm.DB) error {
		var profile db.Profile
		if err := tx.First(&profile, b).Error; err != nil {
			return err
		}
		p, g, err := decodeProfileForUpdate(&profile)
		if err != nil {
			return err
		}
		fresh, err := p12FieldPVPState(tx, p, g, now)
		if err != nil {
			return err
		}
		if fresh.Kills != 11 || fresh.DailyKills != 0 || fresh.LootCoin != 0 || fresh.JailUntil != state.JailUntil {
			return fmt.Errorf("day rollover washed persistent penalty")
		}
		if err = tx.Save(fresh).Error; err != nil {
			return err
		}
		if err = f.s.SyncP12FieldPVPPenalty(tx, p, g, now.Add(2*time.Minute)); err != nil {
			return err
		}
		return saveProfile(tx, &profile, p, g)
	})
	if err != nil {
		t.Fatal(err)
	}
	if err = P12FieldPVPRequireIdle(db.GetIns(), b, now.Add(2*time.Minute)); err != nil {
		t.Fatal("natural expired jail remained locked", err)
	}
}

func TestP12FieldPVPBusinessJudicialActualTerminalAndOriginalPrisonHandshake(t *testing.T) {
	f, b, _, sessions := p12FieldPVPSetup(t)
	now := time.Now()
	p12FieldPVPEnable(t)
	if err := updateProfile(f.ctx, b, func(p *pb.Player, g *model.GameState) error {
		g.Field.Side.Kills = proto.Int32(6)
		g.Field.Side.CurHp = proto.Float64(.01)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	before := p12FieldPVPProfiles(t, f.id, b)
	if _, err := f.s.p12FieldPVPFightAt(f.ctx, f.id, sessions[f.id], &pb.CSSidePvpFightResult{PlayerId: proto.Int64(b), FightType: proto.Int32(0)}, 0, now); err == nil {
		t.Fatal("judicial target accepted as ordinary white target")
	}
	if !bytes.Equal(before, p12FieldPVPProfiles(t, f.id, b)) {
		t.Fatal("failed target class mutated profile")
	}
	out, err := f.s.p12FieldPVPFightAt(f.ctx, f.id, sessions[f.id], &pb.CSSidePvpFightResult{PlayerId: proto.Int64(b), FightType: proto.Int32(1)}, 0, now)
	response := p12FieldPVPResponse(t, out, err)
	if response.GetCode() != 1 || response.GetWinPlayerId() != f.id || len(response.Looted) != 0 || len(response.Result.GetSimps()) != 0 {
		t.Fatal("actual judicial terminal/zero coin", response)
	}
	var state db.P12FieldPVPState
	if err = db.GetIns().First(&state, "player_id=?", b).Error; err != nil {
		t.Fatal(err)
	}
	if state.Kills != 6 || state.JailUntil != state.BusyUntil+6*180000 {
		t.Fatal("judicial sentence did not retain original jailFlag", state.Kills, state.JailUntil, state.BusyUntil)
	}
	point := &pb.PointNew{X: proto.Float32(float32(constUtil.P12FieldPVPPrisonX)), Y: proto.Float32(-2), Z: proto.Float32(float32(constUtil.P12FieldPVPPrisonZ))}
	if ok, err := ShouldAllowP12PrisonTransfer(db.GetIns(), b, "town_main", point, now.Add(time.Second)); err == nil || ok {
		t.Fatal("prison transition interrupted original3+3 presentation")
	}
	if ok, err := ShouldAllowP12PrisonTransfer(db.GetIns(), b, "town_main", point, now.Add(7*time.Second)); err != nil || !ok {
		t.Fatal("original scripted prison destination rejected", err)
	}
	if ok, err := ShouldAllowP12PrisonTransfer(db.GetIns(), b, "field_002", point, now.Add(7*time.Second)); err != nil || ok {
		t.Fatal("prison destination forged")
	}
	err = db.GetIns().Transaction(func(tx *gorm.DB) error {
		var row db.Profile
		if err := tx.First(&row, b).Error; err != nil {
			return err
		}
		p, g, err := decodeProfileForUpdate(&row)
		if err != nil {
			return err
		}
		if err = f.s.SyncP12FieldPVPPenalty(tx, p, g, now.Add(7*time.Second)); err != nil {
			return err
		}
		if g.Position.GetScene() != "town_main" || !p12FieldPVPPrisonPoint(g.Position.GetCurrent()) || g.Field.Side.GetKills() != 6 || g.Field.Side.GetState() != 0 || g.Field.Side.GetCurHp() <= 0 {
			return fmt.Errorf("cold prison position/jailFlag/life recovery")
		}
		return saveProfile(tx, &row, p, g)
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.s.p12FieldPVPRedeemAt(f.ctx, b, sessions[b], &pb.CSPlayerRedeem{Type: proto.Int32(1)}, 0, now.Add(7*time.Second)); err != nil {
		t.Fatal(err)
	}
	if err = P12FieldPVPRequireIdle(db.GetIns(), b, now.Add(7*time.Second)); err != nil {
		t.Fatal("paid prison still locked", err)
	}
}
