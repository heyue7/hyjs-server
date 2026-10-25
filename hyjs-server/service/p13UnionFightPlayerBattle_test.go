package service

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"google.golang.org/protobuf/proto"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
	"math"
	"os"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func p13UFPlayerBattle172Rules() *p13UFPlayerBattleRules {
	return &p13UFPlayerBattleRules{Policy: db.P13UnionFightPlayerBattlePolicy{ID: 1, Enabled: true, Version: "test-local-ordinary-v1", Source: "finite isolated source; original revive nth mapping unknown; local ordinal1cap40 explicitly published", PublishedAt: 1, StartsAt: 1, EndsAt: 1 << 62, ReviveRule: constUtil.P13UFPlayerBattleReviveRule}, MatchID: "isolated-real-source-test", MatchSHA: p6Digest([]byte("match-source")), Engine: constUtil.P6EngineVersion, Client: constUtil.P6OriginalClientHash, Configuration: p6Digest([]byte("tables")), CapabilityProof: constUtil.P6UFPlayerNativeProof, MaxRounds: 40, AttackerFirst: true, AttackerFactor: 1, DefenderFactor: 1, FullHPOnly: true, AttackCost: 0, DurationMillis: 5000, KillScore: 5, KillToken: 5, ArriveRange: .8, ReviveSeconds: []int64{5, 10, 15, 20, 25, 30, 35, 40}}
}

func p13UFPlayerBattle172Frames(players []*pb.BattlePlayer) []*pb.InstUFPlayer {
	var out []*pb.InstUFPlayer
	for i, player := range players {
		// Deliberately use the opposite warcamp from enginecamp: rewards map identity.
		frame := &pb.InstUFPlayer{Guid: player.PlayerId, ServerId: player.ServerId, Camp: proto.Int32(int32(1 - i)), State: proto.Int32(0), OwnToken: proto.Float64(11), OwnScore: proto.Float64(7), FightInfo: &pb.InstFightable{Guid: player.PlayerId, Anger: player.GetInst().Anger}}
		for _, hero := range player.Formation {
			frame.FightInfo.Index = append(frame.FightInfo.Index, hero.GetPose())
			frame.FightInfo.HpPos = append(frame.FightInfo.HpPos, hero.GetInst().GetCurHp())
			frame.FightInfo.MaxHpPos = append(frame.FightInfo.MaxHpPos, hero.GetProp().GetHp())
		}
		out = append(out, frame)
	}
	return out
}

// Actual unmodified original source families exercise timeout, either winner,
// and multiple actor deaths. No winner or damage is fabricated for a positive case.
func TestP13UFPlayerBattle172WholePlayerDeathAndTimeout(t *testing.T) {
	s := p6OriginalTables(t)
	for _, index := range []int{0, 1, 2, 3} {
		t.Run(fmt.Sprint(index), func(t *testing.T) {
			raw, e := os.ReadFile(fmt.Sprintf("../../analysis/p6/original-uf14-player-factor1-inputs-v191/%03d.SCUFFightRecordReplay.pb", index))
			if e != nil {
				t.Fatal(e)
			}
			typed := &pb.SCUFFightRecordReplay{}
			if proto.Unmarshal(raw, typed) != nil {
				t.Fatal("source")
			}
			input, e := s.FreezeP6UFPlayerInput(typed)
			if e != nil {
				t.Fatal(e)
			}
			terminal, e := s.ComputeP6UFPlayerSettlement(t.Context(), input)
			if e != nil {
				t.Fatal(e)
			}
			before := p13UFPlayerBattle172Frames(typed.PlayerList)
			saved, _ := json.Marshal(before)
			after, dead, killer, revive, e := p13UFPlayerBattleApply(before, input, terminal, p13UFPlayerBattle172Rules())
			if e != nil {
				t.Fatal(e)
			}
			if index == 0 {
				if dead != 0 || killer != 0 || revive != 0 || after[0].GetKillNum() != 0 || after[1].GetKillNum() != 0 || after[0].GetOwnToken() != 11 || after[1].GetOwnScore() != 7 {
					t.Fatal("timeout awarded a kill")
				}
			} else {
				won := int(terminal.Outcome.Winner)
				lost := 1 - won
				if dead != before[lost].GetGuid() || killer != before[won].GetGuid() || revive != 5 || after[lost].GetDeadNum() != 1 || after[won].GetKillNum() != 1 || after[won].GetOwnScore() != 12 || after[won].GetOwnToken() != 16 || after[lost].GetState() != 3 || after[won].GetState() != 0 {
					t.Fatal("whole player identity/reward mapping")
				}
				for _, hp := range after[lost].FightInfo.HpPos {
					if hp != 0 {
						t.Fatal("death before all actual actorHP0")
					}
				}
				if index == 3 && len(after[lost].FightInfo.HpPos) != 2 {
					t.Fatal("party source did not exercise two actor deaths")
				}
			}
			for i, frame := range after {
				expected := terminal.Outcome.Attacker
				if i == 1 {
					expected = terminal.Outcome.Defender
				}
				if frame.FightInfo.GetAnger() != expected.GetInst().GetAnger() {
					t.Fatal("final AP was reset or inferred")
				}
			}
			now, _ := json.Marshal(before)
			if !bytes.Equal(saved, now) {
				t.Fatal("before-state mutated")
			}
			// Unknown policy/effects and incomplete HP mappings cannot acquire war effects.
			bad := p13UFPlayerBattle172Rules()
			bad.Policy.ReviveRule = ""
			if _, _, _, _, e = p13UFPlayerBattleApply(before, input, terminal, bad); e == nil {
				t.Fatal("unknown private revive lookup accepted")
			}
			bad = p13UFPlayerBattle172Rules()
			bad.KillScore = 50
			if _, _, _, _, e = p13UFPlayerBattleApply(before, input, terminal, bad); e != nil {
				t.Fatal("valid published score50 rejected", e)
			}
			bad.KillScore = math.NaN()
			if _, _, _, _, e = p13UFPlayerBattleApply(before, input, terminal, bad); e == nil {
				t.Fatal("nonfinite published score accepted")
			}
			damaged := proto.Clone(before[0]).(*pb.InstUFPlayer)
			damaged.FightInfo.HpPos[0]--
			if _, _, _, _, e = p13UFPlayerBattleApply([]*pb.InstUFPlayer{damaged, before[1]}, input, terminal, p13UFPlayerBattle172Rules()); e == nil {
				t.Fatal("partial initial HP accepted")
			}
		})
	}
}

func TestP13UFPlayerBattle172FrozenFrameNoMissingHPOrAPProjection(t *testing.T) {
	raw, e := os.ReadFile("../../analysis/p6/original-uf14-player-factor1-inputs-v191/001.SCUFFightRecordReplay.pb")
	if e != nil {
		t.Fatal(e)
	}
	typed := &pb.SCUFFightRecordReplay{}
	if proto.Unmarshal(raw, typed) != nil {
		t.Fatal("source")
	}
	battle := typed.PlayerList[0]
	frame := p13UFPlayerBattle172Frames(typed.PlayerList)[0]
	b, _ := proto.Marshal(battle)
	f, _ := proto.Marshal(frame)
	actor := &db.P13UnionFightFieldPlayer{PlayerID: battle.GetPlayerId(), Camp: int64(frame.GetCamp()), Battle: b, BattleHash: p6Digest(b), Frame: f, FrameHash: p6Digest(f)}
	copy, e := p13UFPlayerBattleSource(actor, frame, battle, 1)
	if e != nil || !proto.Equal(copy, battle) {
		t.Fatal("fullHP source", e)
	}
	for _, kind := range []string{"injured", "maximum-ulp", "ap", "missing-pose", "battle-sha"} {
		changed := proto.Clone(frame).(*pb.InstUFPlayer)
		row := *actor
		switch kind {
		case "injured":
			changed.FightInfo.HpPos[0]--
		case "maximum-ulp":
			changed.FightInfo.MaxHpPos[0] += .000000001
		case "ap":
			changed.FightInfo.Anger = proto.Float64(1)
		case "missing-pose":
			changed.FightInfo.Index = nil
		case "battle-sha":
			row.BattleHash = "unknown"
		}
		if _, e = p13UFPlayerBattleSource(&row, changed, battle, 1); e == nil {
			t.Fatal("unknown source admitted", kind)
		}
	}
	after, _ := proto.Marshal(battle)
	if !bytes.Equal(b, after) {
		t.Fatal("original source hero/HP/skill/initiative mutated")
	}
}

// Prepare actual accounts/guild declarations/paid registrations and original
// current handshakes. Only starting level/assets/calendar are finite preparation;
// no player battle/HP/winner/P6 record is inserted by this fixture.
func p13UFPlayerBattle172ActualSetup(t *testing.T) (*p1Fixture, int64, int64, *atomic.Int64, *db.P13UnionFightMatch) {
	t.Helper()
	f, ids := p13GuildSetup(t)
	p13GuildOpen(t, false)
	p13UFOpen(t)
	if e := f.s.InitializeP6(f.ctx); e != nil {
		t.Fatal(e)
	}
	if e := f.s.InitializeP13UnionFightPlayerBattle(f.ctx); e != nil {
		t.Fatal(e)
	}
	var closed db.P13UnionFightPlayerBattlePolicy
	if e := db.GetIns().First(&closed, 1).Error; e != nil || closed.Enabled {
		t.Fatal("new player battle policy not defaultoff", e)
	}
	for i := 0; i < 5; i++ {
		login, e := LoginAccount(f.ctx, fmt.Sprintf("ordinary172-%s-%d-%d", t.Name(), i, time.Now().UnixNano()), "isolated-local-pass")
		if e != nil {
			t.Fatal(e)
		}
		if _, e = CreatePlayer(f.ctx, login.Id, fmt.Sprintf("实源17%d", i), "hero_047"); e != nil {
			t.Fatal(e)
		}
		ids = append(ids, login.Id)
	}
	for i, id := range ids {
		level := int32(30)
		if i == 0 {
			level = 50
		}
		if e := updateProfile(f.ctx, id, func(p *pb.Player, g *model.GameState) error {
			p.Level = proto.Int32(level)
			p.Coin = proto.Int64(20000000)
			p.Diamond = proto.Int64(1000)
			return nil
		}); e != nil {
			t.Fatal(e)
		}
	}
	now := p13UFMondays()
	if e := db.GetIns().Model(&db.P13UnionFightPolicy{}).Where("id=1").Update("match_enabled", true).Error; e != nil {
		t.Fatal(e)
	}
	if e := db.GetIns().Model(&db.P13UnionFightFieldPolicy{}).Where("id=1").Updates(map[string]any{"enabled": true, "preparation_millis": 300000, "left_revive_point_id": 1, "right_revive_point_id": 3, "initial_guard_hp": 1000, "initial_gate_hp": 10000, "move_tolerance": 3, "source": "finite isolated normal war field; playerHP comes only from actual frozen P5"}).Error; e != nil {
		t.Fatal(e)
	}
	if e := db.GetIns().Model(&db.P13UnionFightPlayerBattlePolicy{}).Where("id=1").Updates(map[string]any{"enabled": true, "version": "actual172-local-v1", "source": "prepublished isolated ordinary14 factor1 raw1/0; no attack fee; private revive nth unknown explicitlylocal ordinal1cap40", "published_at": now.UnixMilli(), "starts_at": now.UnixMilli(), "ends_at": now.Add(72 * time.Hour).UnixMilli(), "revive_rule": constUtil.P13UFPlayerBattleReviveRule}).Error; e != nil {
		t.Fatal(e)
	}
	for i, id := range ids {
		p13GuildCall(t, f, id, &pb.CSUnionCreate{Name: proto.String(fmt.Sprintf("普通战会%d", i))}, int64(100+i), now)
		guild := p13GuildID(t, f, id, now)
		if e := db.GetIns().Model(&db.P13Guild{}).Where("id=?", guild).Update("level", 2).Error; e != nil {
			t.Fatal(e)
		}
		p13UFCall(t, f, id, &pb.CSUFUnionApply{}, int64(200+i), now)
		p13UFCall(t, f, id, &pb.CSUFPlayerApply{}, int64(300+i), now)
	}
	next := now.Add(12 * time.Hour)
	first := next.Add(20 * time.Hour).UnixMilli()
	if e := db.GetIns().Create(&db.P13UnionFightSchedule{Day: "2026-10-20", FirstStartAt: first, SecondStartAt: first + 600000, DurationMillis: 300000, Source: "finite isolated published calendar; original server dates unknown"}).Error; e != nil {
		t.Fatal(e)
	}
	if e := f.s.TickP13UnionFight(f.ctx, next); e != nil {
		t.Fatal(e)
	}
	guild := p13GuildID(t, f, ids[0], now)
	var match db.P13UnionFightMatch
	if e := db.GetIns().First(&match, "round=1 AND (left_guild_id=? OR right_guild_id=?)", guild, guild).Error; e != nil {
		t.Fatal(e)
	}
	var roster p13UFMatchRoster
	if json.Unmarshal(match.Roster, &roster) != nil {
		t.Fatal("actual roster")
	}
	enemy := int64(0)
	for _, side := range [][]p13UFMatchFighter{roster.Left, roster.Right} {
		for _, entry := range side {
			if entry.GuildID != guild {
				enemy = entry.PlayerID
				break
			}
		}
	}
	if enemy <= 0 {
		t.Fatal("actual drawn enemy missing")
	}
	clock := &atomic.Int64{}
	clock.Store(first - 60000)
	f.ctx = context.WithValue(f.ctx, constUtil.P13UnionFightClockContextKey{}, func() time.Time { return time.UnixMilli(clock.Load()) })
	for _, id := range []int64{ids[0], enemy} {
		if _, e := f.s.p13UFFieldEnter(f.ctx, id, time.UnixMilli(clock.Load())); e != nil {
			t.Fatal(e)
		}
		if _, e := f.s.p13UFFieldFrames(f.ctx, id, &pb.CSCurrentPosition{}, time.UnixMilli(clock.Load())); e != nil {
			t.Fatal(e)
		}
	}
	// Real original movement, with enough elapsed clock allowance, reaches the
	// actual enemy position. No participant row position or HP is injected.
	clock.Store(first + 60000)
	var target db.P13UnionFightFieldPlayer
	if e := db.GetIns().First(&target, "match_id=? AND player_id=?", match.ID, enemy).Error; e != nil {
		t.Fatal(e)
	}
	if _, e := f.s.p13UFFieldMove(f.ctx, ids[0], &pb.CSMove{PointNew: &pb.PointNew{X: proto.Float32(target.X), Y: proto.Float32(target.Y), Z: proto.Float32(target.Z)}}, time.UnixMilli(clock.Load())); e != nil {
		t.Fatal(e)
	}
	return f, ids[0], enemy, clock, &match
}

// The real path computes once, reserves both actual players, applies only at
// five seconds, restores cold receipts with policyoff, and awards war currency.
func TestP13UFPlayerBattle172ActualStartPendingDeathAndCold(t *testing.T) {
	f, id, enemy, clock, match := p13UFPlayerBattle172ActualSetup(t)
	now := func() time.Time { return time.UnixMilli(clock.Load()) }
	for _, pid := range []int64{id, enemy} {
		var row db.Profile
		if e := db.GetIns().First(&row, pid).Error; e != nil {
			t.Fatal(e)
		}
		t.Cleanup(f.s.RegisterP10Client(f.ctx, pid, row.Session, func(proto.Message) error { return nil }))
		if _, e := f.s.SyncP13UnionFightField(f.ctx, pid, false); e != nil {
			t.Fatal(e)
		}
	}
	var profiles []db.Profile
	if e := db.GetIns().Where("id IN ?", []int64{id, enemy}).Order("id").Find(&profiles).Error; e != nil {
		t.Fatal(e)
	}
	walletBefore, _ := json.Marshal(profiles)
	request := &pb.CSUFPlayerAskFight{TargetGuid: proto.Int64(enemy)}
	// The actual original socket timestamp is zero. It is never rewritten into
	// an allegedly supplied positive client retry key.
	replies, handled, e := f.s.DispatchP13UnionFight(f.ctx, id, request, 0)
	if e != nil || !handled || len(replies) != 2 {
		t.Fatal("original socket0 Start", handled, e)
	}
	var row db.P13UnionFightPlayerBattleAttempt
	if e = db.GetIns().First(&row, "player_id=?", id).Error; e != nil {
		t.Fatal(e)
	}
	if row.CalculationStage != constUtil.P13UFPlayerBattleCaptured || len(row.Terminal) != 0 || row.ScopeProof != "" || row.TerminalHash != "" {
		t.Fatal("Begin must precede terminal computation")
	}
	var captured db.P6BattleRecord
	if e = db.GetIns().First(&captured, "id=?", row.P6RecordID).Error; e != nil || captured.Status != constUtil.P6BattleCaptured || captured.Authoritative {
		t.Fatal("durable uncomputed source missing", e)
	}
	if row.ClientStamp != 0 || row.Stamp <= 0 || row.Status != constUtil.P13UFPlayerBattlePending {
		t.Fatal("socket0 identity or pending journal")
	}
	var beforeActor db.P13UnionFightFieldPlayer
	if e = db.GetIns().First(&beforeActor, "match_id=? AND player_id=?", match.ID, id).Error; e != nil {
		t.Fatal(e)
	}
	beforeFrame, e := p13UFFieldReadActor(&beforeActor)
	if e != nil || beforeFrame.GetState() != 1 {
		t.Fatal("original Begin state", e)
	}
	if _, handled, e = f.s.DispatchP13UnionFight(f.ctx, id, request, 0); e != nil || !handled {
		t.Fatal("same natural pending retry", e)
	}
	if _, _, e = f.s.DispatchP13UnionFight(f.ctx, enemy, &pb.CSUFPlayerAskFight{TargetGuid: proto.Int64(id)}, 91); e == nil {
		t.Fatal("second battle entered actual pending pair")
	}
	if _, e = f.s.p13UFFieldMove(f.ctx, id, &pb.CSMove{PointNew: &pb.PointNew{X: proto.Float32(beforeActor.X), Y: proto.Float32(beforeActor.Y), Z: proto.Float32(beforeActor.Z)}}, now()); e == nil {
		t.Fatal("pending player moved")
	}
	p13UFPlayerBattle172ResourceActionsDenied(t, f, id, now())
	clock.Add(4999)
	if e = f.s.AdvanceP13UnionFightPlayerBattles(f.ctx, now()); e != nil {
		t.Fatal(e)
	}
	if e = db.GetIns().First(&row, "id=?", row.ID).Error; e != nil || row.Status != constUtil.P13UFPlayerBattlePending {
		t.Fatal("early terminal", e)
	}
	// New policy/source shutdown does not change a previously accepted terminal.
	if e = db.GetIns().Model(&db.P13UnionFightPlayerBattlePolicy{}).Where("id=1").Update("enabled", false).Error; e != nil {
		t.Fatal(e)
	}
	clock.Add(1)
	if e = f.s.AdvanceP13UnionFightPlayerBattles(f.ctx, now()); e != nil {
		t.Fatal(e)
	}
	if e = db.GetIns().First(&row, "id=?", row.ID).Error; e != nil || row.Status != constUtil.P13UFPlayerBattleSettled {
		t.Fatal("actual random terminal", e)
	}
	terminal := model.CombatFinalHPOutcome{}
	if json.Unmarshal(row.Terminal, &terminal) != nil || terminal.Outcome == nil || !terminal.Outcome.Authoritative || terminal.Outcome.CapabilityHash != constUtil.P6UFPlayerNativeProof {
		t.Fatal("independent14 evidence")
	}
	won, lost := int(terminal.Outcome.Winner), 1-int(terminal.Outcome.Winner)
	participants := []int64{id, enemy}
	if row.DeadPlayerID != participants[lost] || row.RewardPlayerID != participants[won] || row.ReviveSeconds != 5 {
		t.Fatal("actual terminal identity/death", row.DeadPlayerID, row.RewardPlayerID)
	}
	var frames []*pb.InstUFPlayer
	if json.Unmarshal(row.After, &frames) != nil {
		t.Fatal("after")
	}
	if frames[won].GetOwnScore() != 5 || frames[won].GetOwnToken() != 5 || frames[won].GetKillNum() != 1 || frames[lost].GetDeadNum() != 1 || frames[lost].GetState() != 3 {
		t.Fatal("war-only reward/death counters")
	}
	var afterProfiles []db.Profile
	if e = db.GetIns().Where("id IN ?", []int64{id, enemy}).Order("id").Find(&afterProfiles).Error; e != nil {
		t.Fatal(e)
	}
	walletAfter, _ := json.Marshal(afterProfiles)
	if !bytes.Equal(walletBefore, walletAfter) {
		t.Fatal("ordinary attack charged unproved currency or mutated profile")
	}
	saved, _ := json.Marshal(row)
	if e = f.s.AdvanceP13UnionFightPlayerBattles(f.ctx, now()); e != nil {
		t.Fatal(e)
	}
	f.open(t)
	if e = db.GetIns().First(&row, "id=?", row.ID).Error; e != nil {
		t.Fatal(e)
	}
	cold, _ := json.Marshal(row)
	if !bytes.Equal(saved, cold) {
		t.Fatal("cold timer re-applied terminal/reward")
	}
	if _, e = f.s.p13UFPlayerBattleReceipt(db.GetIns(), &row, id); e != nil {
		t.Fatal("policyoff cold receipt", e)
	}
	var count int64
	if e = db.GetIns().Model(&db.P6BattleRecord{}).Where("source=?", constUtil.P13UFPlayerBattleSource).Count(&count).Error; e != nil || count != 1 {
		t.Fatal("calculated more than once", count, e)
	}
	// A genuinely injured survivor is not silently filled for the next attempt.
	if e = db.GetIns().Model(&db.P13UnionFightPlayerBattlePolicy{}).Where("id=1").Update("enabled", true).Error; e != nil {
		t.Fatal(e)
	}
	if _, _, e = f.s.DispatchP13UnionFight(f.ctx, id, request, 92); e == nil {
		t.Fatal("injured/dead pair re-admitted")
	}
	var actor db.P13UnionFightFieldPlayer
	if e = db.GetIns().First(&actor, "match_id=? AND player_id=?", match.ID, row.DeadPlayerID).Error; e != nil {
		t.Fatal(e)
	}
	frame, e := p13UFFieldReadActor(&actor)
	if e != nil {
		t.Fatal(e)
	}
	clock.Add(6000)
	p13UFPlayerBattle172ResourceActionsDenied(t, f, row.DeadPlayerID, now())
	persistedFrame := append([]byte(nil), actor.Frame...)
	_, peers, peerErr := f.s.p13UFFieldPeers(db.GetIns().WithContext(f.ctx), match.ID, row.RewardPlayerID)
	if peerErr != nil || len(peers) != 1 || peers[0].GetGuid() != row.DeadPlayerID || peers[0].GetState() != 3 || peers[0].GetLeftTime() != 0 {
		t.Fatal("cold peer restarted death countdown", peerErr, peers)
	}
	if e = db.GetIns().First(&actor, "match_id=? AND player_id=?", match.ID, row.DeadPlayerID).Error; e != nil || !bytes.Equal(actor.Frame, persistedFrame) {
		t.Fatal("peer projection mutated persistent frame", e)
	}
	if e = p13UFPlayerBattleProject(db.GetIns(), &actor, frame, now()); e != nil || frame.GetLeftTime() != 0 || frame.GetState() != 3 {
		t.Fatal("unproved automatic revival/heal", e)
	}
	for _, hp := range frame.GetFightInfo().HpPos {
		if hp != 0 {
			t.Fatal("timer manufactured HP")
		}
	}
	// Immutable accepted replay source still has no preset winner in its start.
	start := &pb.SCUFFightRecordReplay{}
	if proto.Unmarshal(row.StartPB, start) != nil || start.WinPlayerId != nil || start.GetFightId() != row.FightID || start.GetFightId() == strconv.FormatInt(row.Stamp, 10) {
		t.Fatal("pre-result source identity")
	}
}

// These calls previously admitted state1 or state3 and mutated state/token,
// stranding a certified pending terminal or bypassing the death movement guard.
func p13UFPlayerBattle172ResourceActionsDenied(t *testing.T, f *p1Fixture, id int64, now time.Time) {
	t.Helper()
	if _, err := f.s.p13UFResourceBegin(f.ctx, id, &pb.CSUFPlayerCollect{ResGuid: proto.Int64(1)}, now); err == nil || !strings.Contains(err.Error(), "原普通战斗") {
		t.Fatal("collect bypassed real battle lifecycle", err)
	}
	if _, err := f.s.p13UFResourceDeposit(f.ctx, id, &pb.CSUFPlayerUpResource{NpcId: proto.String("1")}, 19501, now); err == nil || !strings.Contains(err.Error(), "原普通战斗") {
		t.Fatal("deposit bypassed real battle lifecycle", err)
	}
}

func TestP13UFPlayerBattle172PositiveRetryOnlyCurrentSceneAndClock(t *testing.T) {
	f, strong, weak, clock, _ := p13UFPlayerBattle172ActualSetup(t)
	now := func() time.Time { return time.UnixMilli(clock.Load()) }
	request := &pb.CSUFPlayerAskFight{TargetGuid: proto.Int64(strong)}
	if _, handled, err := f.s.DispatchP13UnionFight(f.ctx, weak, request, 19511); err != nil || !handled {
		t.Fatal("positive actual start", err)
	}
	clock.Add(5000)
	if err := f.s.AdvanceP13UnionFightPlayerBattles(f.ctx, now()); err != nil {
		t.Fatal(err)
	}
	clock.Add(6000)
	replies, handled, err := f.s.p13UFPlayerBattleRetry(f.ctx, weak, request, 19511)
	if err != nil || !handled || len(replies) < 2 {
		t.Fatal("same accepted current-world retry", err, replies)
	}
	for _, message := range replies {
		switch frame := message.(type) {
		case *pb.SCUFPlayerFightEnd:
			if frame.GetGuid() != weak {
				t.Fatal("old opponent end replayed")
			}
		case *pb.SCUFPlayerFightEndDetail:
			if frame.GetPlayer().GetGuid() != weak || frame.GetPlayer().GetLeftTime() != 0 {
				t.Fatal("old death countdown replayed", frame)
			}
		case *pb.SCUFPlayerDead:
			if len(frame.Guid) != 1 || frame.Guid[0] != weak || len(frame.ReviveTime) != 1 || frame.ReviveTime[0] != 0 {
				t.Fatal("old death clock/opponent replayed", frame)
			}
		}
	}
	if _, err = f.s.p13UFFieldLeave(f.ctx, weak, &pb.CSTransferScene{Target: proto.String("town")}, now()); err != nil {
		t.Fatal("real leave", err)
	}
	replies, handled, err = f.s.p13UFPlayerBattleRetry(f.ctx, weak, request, 19511)
	if err != nil || !handled || len(replies) != 0 {
		t.Fatal("historical battle rewound another current scene", err, replies)
	}
	var count int64
	if err = db.GetIns().Model(&db.P6BattleRecord{}).Where("source=?", constUtil.P13UFPlayerBattleSource).Count(&count).Error; err != nil || count != 1 {
		t.Fatal("retry recalculated", err, count)
	}
}
