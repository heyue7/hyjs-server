package service

import (
	"context"
	"math"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

// The PVE end uses the original field service with a finite client-terminal
// fixture. PK is computed by the actual P12 service, without a supplied winner.
func TestP12FieldPVPReviveClockAfterSettledPVE(t *testing.T) {
	t.Setenv("HYJS_P12_PK_MYSQL_DATABASE", "")
	for _, scenario := range []struct {
		name          string
		refreshFirst  bool
		unconsumedPVE bool
		victimAttacks bool
		paidRevive    bool
	}{
		{name: "direct-free-revive"},
		{name: "heartbeat-before-free-revive", refreshFirst: true},
		{name: "unconsumed-pve-direct", unconsumedPVE: true},
		{name: "unconsumed-pve-heartbeat", refreshFirst: true, unconsumedPVE: true},
		{name: "attacker-defeated", victimAttacks: true},
		{name: "paid-revive", paidRevive: true},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			f, other, _, sessions := p12FieldPVPSetup(t)
			victim, attacker, target, winner := other, f.id, other, f.id
			if scenario.victimAttacks {
				victim, winner = f.id, other
			}
			p12FieldPVPEnable(t)
			f.s.P12FieldPVPReady, f.s.P8LifeReady = true, true
			for _, init := range []func(context.Context) error{f.s.InitializeP8Gather, f.s.InitializeDailyCopy} {
				if err := init(f.ctx); err != nil {
					t.Fatal(err)
				}
			}
			if err := updateProfile(f.ctx, victim, func(p *pb.Player, _ *model.GameState) error {
				p.WarriorBagSize = proto.Int32(1000)
				p.Diamond = proto.Int64(100)
				return nil
			}); err != nil {
				t.Fatal(err)
			}
			if _, err := f.s.fieldScene(f.ctx, victim, false); err != nil {
				t.Fatal("prepare snow scene", err)
			}
			readVictim := func() (*pb.Player, *model.GameState, db.P8LifeState) {
				t.Helper()
				var profile db.Profile
				if err := db.GetIns().First(&profile, victim).Error; err != nil {
					t.Fatal(err)
				}
				player, game, err := decodeProfile(&profile)
				if err != nil {
					t.Fatal(err)
				}
				var life db.P8LifeState
				if err = db.GetIns().First(&life, "player_id=?", victim).Error; err != nil {
					t.Fatal(err)
				}
				return player, game, life
			}
			_, game, _ := readVictim()
			if len(game.Field.Monsters) == 0 {
				t.Fatal("snow monster fixture unavailable")
			}
			startFrames, err := f.s.fieldFightStart(f.ctx, victim, &pb.CSSidePveFightStart{MonsterId: game.Field.Monsters[0].Uuid})
			if err != nil {
				t.Fatal("accept PVE", err)
			}
			start := startFrames[0].(*pb.SCSidePveFightStart)
			party := proto.Clone(start.GetBattlePlayer()).(*pb.BattlePlayer)
			var totalHP float64
			for _, hero := range party.GetFormation() {
				totalHP += hero.GetInst().GetCurHp()
			}
			end := &pb.CSSidePveFightEnd{FightId: start.FightId, Result: proto.Int32(1), BattlePlayer: party, OptRecord: &pb.BattleOperate{FinalRoundNum: proto.Int32(1), AttackerTotalLeftHp: proto.Float64(totalHP), DefenderTotalLeftHp: proto.Float64(0)}}
			if scenario.unconsumedPVE {
				// Model an older settlement created before P8 life tracking was ready.
				f.s.P8LifeReady = false
			}
			if _, err = f.s.fieldFightEnd(f.ctx, victim, end); err != nil {
				t.Fatal("settle finite PVE client-terminal fixture", err)
			}
			f.s.P8LifeReady = true
			_, game, life := readVictim()
			if game.Field.Fight == nil || !game.Field.Fight.Settled || game.Field.Fight.Start.GetFightId() != start.GetFightId() || (life.LastFightID == start.GetFightId()) == scenario.unconsumedPVE {
				t.Fatal("unexpected PVE settlement marker", life.LastFightID, scenario.unconsumedPVE)
			}
			// Finite initial injury makes the actual P12 calculation produce death.
			if err = updateProfile(f.ctx, victim, func(_ *pb.Player, game *model.GameState) error {
				game.Field.Side.CurHp = proto.Float64(.01)
				game.Field.Side.BattlePos3 = proto.Float64(.01)
				game.Field.Side.BattlePosHps = []*pb.BattlePosHp{{BattlePos: proto.Int32(3), HpPercent: proto.Float64(.01)}}
				return nil
			}); err != nil {
				t.Fatal(err)
			}
			acceptedAt := time.Now()
			pkRequest := &pb.CSSidePvpFightResult{PlayerId: proto.Int64(target)}
			frames, err := f.s.p12FieldPVPFightAt(f.ctx, attacker, sessions[attacker], pkRequest, 872010, acceptedAt)
			if err != nil {
				t.Fatal("accept actual PK", err)
			}
			result := frames[0].(*pb.SCSidePvpFightResult)
			_, game, life = readVictim()
			if result.GetWinPlayerId() != winner || game.Field.Side.GetCurHp() != 0 || game.Field.Side.GetState() != constUtil.P8FieldDead || life.DeathAt != acceptedAt.UnixMilli() || life.LastFightID != start.GetFightId() {
				t.Fatal("actual PK did not preserve the PVE marker and victim death", result.GetWinPlayerId(), game.Field.Side.GetCurHp(), game.Field.Side.GetState(), life.DeathAt, life.LastFightID)
			}
			var pk db.P12FieldPVPState
			if err = db.GetIns().First(&pk, "player_id=?", victim).Error; err != nil || pk.LastFightID != result.GetFightId() {
				t.Fatal("P12 PK identity missing", err)
			}
			beforePKReplay := p12FieldPVPProfiles(t, attacker, target)
			againPK, err := f.s.p12FieldPVPFightAt(f.ctx, attacker, sessions[attacker], pkRequest, 872010, acceptedAt.Add(time.Second))
			if err != nil || !proto.Equal(result, againPK[0]) || string(beforePKReplay) != string(p12FieldPVPProfiles(t, attacker, target)) {
				t.Fatal("PK receipt replay changed its outcome or profiles", err)
			}
			if scenario.refreshFirst {
				f.restart(t)
				_, coldGame, coldLife := readVictim()
				if coldGame.Field.Side.GetState() != constUtil.P8FieldDead || coldLife.DeathAt != acceptedAt.UnixMilli() || coldLife.LastFightID != start.GetFightId() {
					t.Fatal("PK death or PVE marker changed after restart", coldLife)
				}
			}
			// Advance only this isolated fixture's lease/clock to test both sides
			// of the ten-second server boundary without a wall-clock sleep.
			if err = db.GetIns().Model(&pk).Update("busy_until", 0).Error; err != nil {
				t.Fatal(err)
			}
			setAge := func(ms int64) int64 {
				t.Helper()
				deathAt := time.Now().UnixMilli() - ms
				if err := db.GetIns().Model(&db.P8LifeState{}).Where("player_id=?", victim).Update("death_at", deathAt).Error; err != nil {
					t.Fatal(err)
				}
				return deathAt
			}
			setAge(9000)
			if _, err = f.s.p8SafeRevive(f.ctx, victim, &pb.CSTransfer2Safe{}, 872011); err == nil {
				t.Fatal("free revival accepted before the ten-second boundary")
			}
			if scenario.paidRevive {
				_, before, _ := readVictim()
				previousPoint := proto.Clone(before.Position.Current).(*pb.PointNew)
				paid, err := f.s.p8Resurrect(f.ctx, victim, &pb.CSResurrection{}, 872013)
				if err != nil {
					t.Fatal("paid PK revival", err)
				}
				player, current, paidLife := readVictim()
				reply := paid[0].(*pb.SCResurrenttion)
				if len(reply.Cost) != 1 || reply.Cost[0].GetId() != "diamond" || reply.Cost[0].GetNum() != 50 || player.GetDiamond() != 50 || !proto.Equal(current.Position.Current, previousPoint) || current.Field.Side.GetCurHp() != 1 || current.Field.Side.GetState() != constUtil.P8FieldIdle || paidLife.DeathAt != 0 || paidLife.LastFightID != start.GetFightId() {
					t.Fatal("paid PK revival contract changed", reply, player.GetDiamond(), current.Position, current.Field.Side, paidLife)
				}
				snapshot := p12FieldPVPProfiles(t, attacker, target)
				f.restart(t)
				again, err := f.s.p8Resurrect(f.ctx, victim, &pb.CSResurrection{}, 872013)
				if err != nil || !proto.Equal(reply, again[0]) || string(snapshot) != string(p12FieldPVPProfiles(t, attacker, target)) {
					t.Fatal("paid PK revival repeated after restart", err)
				}
				return
			}
			deathAt := setAge(11000)
			if scenario.refreshFirst {
				if _, err = f.s.fieldHeartbeat(f.ctx, victim); err != nil {
					t.Fatal("ordinary heartbeat", err)
				}
				_, _, life = readVictim()
				if life.DeathAt != deathAt {
					t.Fatalf("ordinary heartbeat reset PK death clock: got %d, want %d", life.DeathAt, deathAt)
				}
			}
			out, err := f.s.p8SafeRevive(f.ctx, victim, &pb.CSTransfer2Safe{}, 872012)
			if err != nil {
				t.Fatal("free revival rejected after ten seconds", err)
			}
			reply := out[0].(*pb.SCTransfer2Safe)
			player, game, life := readVictim()
			point, err := fieldPoint(f.s.table("Field", "FieldId", "field_002")["Resurrection"])
			if err != nil || reply.GetResult() != 1 || !proto.Equal(game.Position.GetCurrent(), point) || math.Abs(game.Field.Side.GetCurHp()-.1) > 1e-8 || game.Field.Side.GetState() != constUtil.P8FieldIdle || player.GetDiamond() != 100 || life.DeathAt != 0 || life.LastFightID != start.GetFightId() {
				t.Fatal("free safe revival changed contract", err, reply, game.Position, game.Field.Side, player.GetDiamond(), life)
			}
			beforeReplay := p12FieldPVPProfiles(t, attacker, target)
			again, err := f.s.p8SafeRevive(f.ctx, victim, &pb.CSTransfer2Safe{}, 872012)
			if err != nil || !proto.Equal(reply, again[0]) || string(beforeReplay) != string(p12FieldPVPProfiles(t, attacker, target)) {
				t.Fatal("same free receipt repeated effects", err)
			}
			f.restart(t)
			cold, err := f.s.p8SafeRevive(f.ctx, victim, &pb.CSTransfer2Safe{}, 872012)
			if err != nil || !proto.Equal(reply, cold[0]) || string(beforeReplay) != string(p12FieldPVPProfiles(t, attacker, target)) {
				t.Fatal("cold free receipt changed state", err)
			}
			_, game, _ = readVictim()
			var liveMonster *pb.SideMonster
			for _, monster := range game.Field.Monsters {
				if monster.GetState() == 1 {
					liveMonster = monster
					break
				}
			}
			if liveMonster == nil {
				t.Fatal("missing second finite PVE fixture")
			}
			nextFrames, err := f.s.fieldFightStart(f.ctx, victim, &pb.CSSidePveFightStart{MonsterId: liveMonster.Uuid})
			if err != nil {
				t.Fatal("start later PVE", err)
			}
			next := nextFrames[0].(*pb.SCSidePveFightStart)
			deadParty := proto.Clone(next.GetBattlePlayer()).(*pb.BattlePlayer)
			for _, hero := range deadParty.GetFormation() {
				hero.Inst.CurHp = proto.Float64(0)
			}
			loss := &pb.CSSidePveFightEnd{FightId: next.FightId, Result: proto.Int32(0), BattlePlayer: deadParty, OptRecord: &pb.BattleOperate{FinalRoundNum: proto.Int32(1), AttackerTotalLeftHp: proto.Float64(0), DefenderTotalLeftHp: proto.Float64(1)}}
			if _, err = f.s.fieldFightEnd(f.ctx, victim, loss); err != nil {
				t.Fatal("settle later finite PVE death fixture", err)
			}
			_, game, life = readVictim()
			if game.Field.Side.GetState() != constUtil.P8FieldDead || game.Field.Side.GetCurHp() != 0 || life.DeathAt <= 0 || life.LastFightID != next.GetFightId() {
				t.Fatal("new PVE death did not advance the original life clock", game.Field.Side, life)
			}
			beforeOldReceipt := p12FieldPVPProfiles(t, attacker, target)
			if _, err = f.s.p8SafeRevive(f.ctx, victim, &pb.CSTransfer2Safe{}, 872012); err == nil || string(beforeOldReceipt) != string(p12FieldPVPProfiles(t, attacker, target)) {
				t.Fatal("old PK revival receipt changed a later PVE death", err)
			}
		})
	}
}
