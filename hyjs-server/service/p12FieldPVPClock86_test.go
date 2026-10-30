package service

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
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

// Offset is supplied only through the authenticated calendar context. The
// At methods take an injectable wall instant, so these tests never sleep.
func p12Clock86Context(ctx context.Context, owner, days int64) context.Context {
	ctx = context.WithValue(ctx, constUtil.LocalPlayBoundKey, true)
	ctx = context.WithValue(ctx, constUtil.LocalPlayOwnerKey, owner)
	return context.WithValue(ctx, constUtil.LocalPlayOffsetDaysKey, days)
}

func TestP12Clock86LegacyRetryAndUnknownDeadlines(t *testing.T) {
	t.Setenv("HYJS_P12_PK_MYSQL_DATABASE", "")
	for _, stamp := range []int64{0, 860010} {
		t.Run(fmt.Sprint(stamp), func(t *testing.T) {
			f, victim, _, sessions := p12FieldPVPSetup(t)
			p12FieldPVPEnable(t)
			f.s.P12FieldPVPReady = true
			p12Clock86Injure(t, f, victim)
			now := time.Now()
			ctx := p12Clock86Context(f.ctx, f.id, 10)
			req := &pb.CSSidePvpFightResult{PlayerId: proto.Int64(victim)}
			first, err := f.s.p12FieldPVPFightAt(ctx, f.id, sessions[f.id], req, stamp, now)
			if err != nil || first[0].(*pb.SCSidePvpFightResult).GetCode() != 1 {
				t.Fatal("prepare accepted receipt", err)
			}
			// Model a pre-version receipt and legacy mutable markers while keeping
			// all original response, replay, hash and civil-time bytes untouched.
			if err = db.GetIns().Model(&db.P12FieldPVPReceipt{}).Where("player_id=?", f.id).Updates(map[string]any{"clock_version": 0, "real_started_at": 0, "real_busy_until": 0}).Error; err != nil {
				t.Fatal(err)
			}
			if err = db.GetIns().Model(&db.P12FieldPVPState{}).Where("player_id IN ?", []int64{f.id, victim}).Updates(map[string]any{"clock_version": 0, "real_busy_until": 0, "real_jail_until": 0}).Error; err != nil {
				t.Fatal(err)
			}
			var before db.P12FieldPVPReceipt
			if err = db.GetIns().First(&before, "player_id=?", f.id).Error; err != nil {
				t.Fatal(err)
			}
			snapshot, _ := json.Marshal(before)
			retry, err := f.s.p12FieldPVPFightAt(ctx, f.id, sessions[f.id], req, stamp, now.Add(24*time.Hour))
			if err != nil || !proto.Equal(first[0], retry[0]) {
				t.Fatalf("legacy idempotent retry: %v", err)
			}
			if stamp > 0 {
				retry, err = f.s.p12FieldPVPFightAt(ctx, f.id, sessions[f.id], req, 0, now.Add(24*time.Hour))
				if err != nil || !proto.Equal(first[0], retry[0]) {
					t.Fatalf("legacy unnumbered replay of numbered receipt: %v", err)
				}
			}
			var after db.P12FieldPVPReceipt
			if err = db.GetIns().First(&after, "player_id=?", f.id).Error; err != nil {
				t.Fatal(err)
			}
			current, _ := json.Marshal(after)
			if !bytes.Equal(snapshot, current) || !bytes.Equal(before.Response, after.Response) || !bytes.Equal(before.Terminal, after.Terminal) || !bytes.Equal(before.Replay, after.Replay) || !bytes.Equal(before.MV, after.MV) {
				t.Fatal("legacy receipt was rewritten")
			}
			if err = P12FieldPVPRequireIdle(db.GetIns(), victim, now.Add(1000*24*time.Hour)); err == nil {
				t.Fatal("unknown legacy deadline was treated as expired")
			}
			if err = f.s.TickP12FieldPVPPresentation(ctx, now.Add(1000*24*time.Hour)); err != nil {
				t.Fatal(err)
			}
			var state db.P12FieldPVPState
			if err = db.GetIns().First(&state, "player_id=?", victim).Error; err != nil || state.BusyUntil == 0 {
				t.Fatal("worker consumed unknown legacy marker", err)
			}
			if err = serverGameClockPendingBattles(db.GetIns(), now.Add(1000*24*time.Hour), 0); err == nil {
				t.Fatal("calendar change ignored unknown legacy marker")
			}
			corruptions := map[string]func(*db.P12FieldPVPReceipt){
				"policy":   func(r *db.P12FieldPVPReceipt) { r.PolicyHash = "foreign" },
				"request":  func(r *db.P12FieldPVPReceipt) { r.RequestHash = "foreign" },
				"scope":    func(r *db.P12FieldPVPReceipt) { r.ScopeProof = "foreign" },
				"owner":    func(r *db.P12FieldPVPReceipt) { r.PlayerID++ },
				"opponent": func(r *db.P12FieldPVPReceipt) { r.OpponentID++ },
				"winner":   func(r *db.P12FieldPVPReceipt) { r.WinnerID++ },
				"created":  func(r *db.P12FieldPVPReceipt) { r.CreatedAt++ },
				"deadline": func(r *db.P12FieldPVPReceipt) { r.BusyUntil++ },
				"version":  func(r *db.P12FieldPVPReceipt) { r.RealBusyUntil = 1 },
				"response": func(r *db.P12FieldPVPReceipt) { r.Response = append([]byte(nil), r.Response...); r.Response[0] ^= 1 },
				"terminal": func(r *db.P12FieldPVPReceipt) { r.Terminal = append([]byte(nil), r.Terminal...); r.Terminal[0] ^= 1 },
				"replay":   func(r *db.P12FieldPVPReceipt) { r.Replay = append([]byte(nil), r.Replay...); r.Replay[0] ^= 1 },
				"mv":       func(r *db.P12FieldPVPReceipt) { r.MV = append([]byte(nil), r.MV...); r.MV[0] ^= 1 },
			}
			for name, mutate := range corruptions {
				corrupt := before
				mutate(&corrupt)
				if _, _, err = f.s.p12FieldPVPReadReceipt(db.GetIns(), &corrupt); err == nil {
					t.Fatalf("legacy receipt accepted corrupt %s", name)
				}
			}
		})
	}
}

func TestP12Clock86ReceiptRetryIgnoresCurrentPeerClock(t *testing.T) {
	t.Setenv("HYJS_P12_PK_MYSQL_DATABASE", "")
	f, victim, _, sessions := p12FieldPVPSetup(t)
	p12FieldPVPEnable(t)
	f.s.P12FieldPVPReady = true
	p12Clock86Injure(t, f, victim)
	wall := time.Now()
	request := &pb.CSSidePvpFightResult{PlayerId: proto.Int64(victim)}
	first, err := f.s.p12FieldPVPFightAt(f.ctx, f.id, sessions[f.id], request, 860022, wall)
	if err != nil || first[0].(*pb.SCSidePvpFightResult).GetCode() != 1 {
		t.Fatalf("prepare accepted receipt: %v", err)
	}
	if err = f.s.InitializeLocalPlay(f.ctx); err != nil {
		t.Fatal(err)
	}
	f.s.LocalPlayToolsEnabled = true
	if err = db.GetIns().Model(&db.LocalPlayPolicy{}).Where("id=1").Update("enabled", true).Error; err != nil {
		t.Fatal(err)
	}
	if err = db.GetIns().Model(&db.Profile{}).Where("id=?", victim).Update("uid", fmt.Sprintf("%sclock86-invalid-%d", constUtil.P3DemoAccountPrefix, victim)).Error; err != nil {
		t.Fatal(err)
	}
	if err = db.GetIns().Create(&db.LocalPlayClockState{PlayerID: victim, OffsetDays: 3651}).Error; err != nil {
		t.Fatal(err)
	}
	retry, err := f.s.p12FieldPVPFightAt(f.ctx, f.id, sessions[f.id], request, 860022, wall.Add(time.Minute))
	if err != nil || !proto.Equal(first[0], retry[0]) {
		t.Fatalf("valid immutable retry depends on a later peer clock: %v", err)
	}
	if _, err = f.s.p12FieldPVPFightAt(f.ctx, f.id, sessions[f.id], request, 860023, wall.Add(time.Minute)); err == nil {
		t.Fatal("new fight accepted an invalid peer clock")
	}
}

func TestP12Clock86LeaseStartsAfterCompute(t *testing.T) {
	t.Setenv("HYJS_P12_PK_MYSQL_DATABASE", "")
	f, victim, _, sessions := p12FieldPVPSetup(t)
	p12FieldPVPEnable(t)
	f.s.P12FieldPVPReady = true
	p12Clock86Injure(t, f, victim)
	start := time.Now()
	accepted := start.Add(30 * time.Second)
	calls := 0
	clock := func() time.Time {
		calls++
		if calls == 1 {
			return start
		}
		return accepted
	}
	ctx := p12Clock86Context(f.ctx, f.id, 10)
	_, err := f.s.p12FieldPVPFightClocked(ctx, f.id, sessions[f.id], &pb.CSSidePvpFightResult{PlayerId: proto.Int64(victim)}, 860011, clock)
	if err != nil {
		t.Fatal(err)
	}
	var receipt db.P12FieldPVPReceipt
	if err = db.GetIns().First(&receipt, "player_id=?", f.id).Error; err != nil || receipt.RealStartedAt != accepted.UnixMilli() || receipt.RealBusyUntil != accepted.Add(6*time.Second).UnixMilli() || receipt.CreatedAt != accepted.AddDate(0, 0, 10).UnixMilli() {
		t.Fatalf("physical lease and calendar timestamp: %+v %v", receipt, err)
	}
	if err = P12FieldPVPRequireIdle(db.GetIns(), victim, accepted.Add(5999*time.Millisecond)); err == nil {
		t.Fatal("lease ended before six seconds")
	}
	if err = P12FieldPVPRequireIdle(db.GetIns(), victim, accepted.Add(6*time.Second)); err != nil {
		t.Fatalf("lease lasted more than six seconds: %v", err)
	}
	corrupt := receipt
	corrupt.RealBusyUntil++
	if _, _, err = f.s.p12FieldPVPReadReceipt(db.GetIns(), &corrupt); err == nil {
		t.Fatal("versioned receipt accepted a modified real deadline")
	}
}

func TestP12Clock86CrossModeGuardKeepsCalendarData(t *testing.T) {
	t.Setenv("HYJS_P12_PK_MYSQL_DATABASE", "")
	for _, offset := range []int64{-10, 10} {
		t.Run(fmt.Sprint(offset), func(t *testing.T) {
			f, _, _, _ := p12FieldPVPSetup(t)
			f.s.P12FieldPVPReady = true
			wall := time.Now()
			ctx := p12Clock86Context(f.ctx, f.id, offset)
			game := LocalPlayAt(ctx, wall)
			state := db.P12FieldPVPState{PlayerID: f.id, Day: "2000-01-01", DailyKills: 7, LootCoin: 1000, BusyUntil: game.Add(6 * time.Second).UnixMilli(), RealBusyUntil: wall.Add(6 * time.Second).UnixMilli(), ClockVersion: db.P12FieldPVPWallClock, Source: constUtil.P12FieldPVPSource}
			if err := db.GetIns().Save(&state).Error; err != nil {
				t.Fatal(err)
			}
			if err := f.s.ensureNoActiveDailyCopy(db.GetIns().WithContext(ctx), f.id, game); err == nil || err.Error() != constUtil.P12FieldPVPBusy {
				t.Fatalf("calendar offset bypassed physical PVP lease: %v", err)
			}
			var unchanged db.P12FieldPVPState
			if err := db.GetIns().First(&unchanged, "player_id=?", f.id).Error; err != nil || unchanged.Day != state.Day || unchanged.DailyKills != state.DailyKills || unchanged.LootCoin != state.LootCoin {
				t.Fatalf("cross-mode guard changed daily counters: %+v %v", unchanged, err)
			}
		})
	}
}

func TestP12Clock86ArenaGuardUsesWallForBothPlayers(t *testing.T) {
	t.Setenv("HYJS_P12_PK_MYSQL_DATABASE", "")
	for _, offset := range []int64{-10, 10} {
		t.Run(fmt.Sprint(offset), func(t *testing.T) {
			f, peer := p11ArenaScopedSetup(t)
			if err := db.GetIns().AutoMigrate(&db.P12FieldPVPState{}); err != nil {
				t.Fatal(err)
			}
			f.s.P12FieldPVPReady = true
			wall := time.Now()
			ctx := p12Clock86Context(f.ctx, f.id, offset)
			game := LocalPlayAt(ctx, wall)
			request := &pb.CSPvpFightResult{PlayerId: proto.Int64(peer), ServerId: proto.Int32(1)}
			for i, id := range []int64{f.id, peer} {
				state := db.P12FieldPVPState{PlayerID: id, Day: "2000-01-01", DailyKills: 7, LootCoin: 1000, BusyUntil: game.Add(6 * time.Second).UnixMilli(), RealBusyUntil: wall.Add(6 * time.Second).UnixMilli(), ClockVersion: db.P12FieldPVPWallClock, Source: constUtil.P12FieldPVPSource}
				if err := db.GetIns().Save(&state).Error; err != nil {
					t.Fatal(err)
				}
				_, err := f.s.p11ArenaScopedFightAt(ctx, f.id, request, int64(860030+i), game)
				if err == nil || err.Error() != constUtil.P12FieldPVPBusy {
					t.Fatalf("arena bypassed player %d real PVP lease: %v", id, err)
				}
				if err = db.GetIns().Model(&state).Updates(map[string]any{"busy_until": game.AddDate(0, 0, 20).UnixMilli(), "real_busy_until": wall.Add(-time.Second).UnixMilli()}).Error; err != nil {
					t.Fatal(err)
				}
				_, err = f.s.p11ArenaScopedFightAt(ctx, f.id, request, int64(860030+i), game)
				if err != nil && err.Error() == constUtil.P12FieldPVPBusy {
					t.Fatalf("arena kept player %d busy after real expiry", id)
				}
				if err = db.GetIns().First(&state, "player_id=?", id).Error; err != nil || state.Day != "2000-01-01" || state.DailyKills != 7 || state.LootCoin != 1000 {
					t.Fatalf("arena guard modified player %d daily data: %+v %v", id, state, err)
				}
			}
		})
	}
}

func TestP12Clock86PeerOwnCalendarAndGGOProtection(t *testing.T) {
	t.Setenv("HYJS_P12_PK_MYSQL_DATABASE", "")
	f, victim, _, sessions := p12FieldPVPSetup(t)
	p12FieldPVPEnable(t)
	f.s.P12FieldPVPReady, f.s.LocalPlayToolsEnabled = true, true
	if err := f.s.InitializeLocalPlay(f.ctx); err != nil {
		t.Fatal(err)
	}
	if err := db.GetIns().Model(&db.LocalPlayPolicy{}).Where("id=1").Update("enabled", true).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.GetIns().Model(&db.Profile{}).Where("id=?", victim).Update("uid", fmt.Sprintf("%sclock86-%d", constUtil.P3DemoAccountPrefix, victim)).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.GetIns().Create(&db.LocalPlayClockState{PlayerID: victim, OffsetDays: -10}).Error; err != nil {
		t.Fatal(err)
	}
	wall := time.Now()
	peerDay := dailyCopyDay(wall.AddDate(0, 0, -10))
	pending := db.DailyCopyFight{FightID: "clock86-peer-copy", PlayerID: victim, StartStamp: 860012, Day: peerDay, Start: []byte{1}}
	if err := db.GetIns().Create(&pending).Error; err != nil {
		t.Fatal(err)
	}
	ctx := p12Clock86Context(f.ctx, f.id, 10)
	req := &pb.CSSidePvpFightResult{PlayerId: proto.Int64(victim)}
	frames, err := f.s.p12FieldPVPFightAt(ctx, f.id, sessions[f.id], req, 860013, wall)
	if err != nil || frames[0].(*pb.SCSidePvpFightResult).GetCode() != 2002 {
		t.Fatalf("peer's own active daily copy did not block PK: %v", err)
	}
	if err = db.GetIns().Model(&pending).Update("settled", true).Error; err != nil {
		t.Fatal(err)
	}
	frames, err = f.s.p12FieldPVPFightAt(ctx, f.id, sessions[f.id], req, 860013, wall)
	if err != nil || frames[0].(*pb.SCSidePvpFightResult).GetCode() != 1 {
		t.Fatalf("settled peer copy blocked PK: %v", err)
	}
	var profile db.Profile
	if err = db.GetIns().First(&profile, victim).Error; err != nil {
		t.Fatal(err)
	}
	player, game, err := decodeProfile(&profile)
	if err != nil {
		t.Fatal(err)
	}
	f.s.GGOFieldConfig = ggoLineTestConfiguration(t)
	game.Position.Scene = proto.String(constUtil.GGOFieldScene)
	game.Position.Current = &pb.PointNew{X: proto.Float32(20), Y: proto.Float32(0), Z: proto.Float32(20)}
	game.GGOField = &model.GGOFieldState{ProtectionUntil: wall.Add(3 * time.Second).UnixMilli()}
	code, err := f.s.p12FieldPVPQualifyClocks(db.GetIns().WithContext(ctx), player, game, wall, wall.AddDate(0, 0, 10))
	if err != nil || code != 2007 {
		t.Fatalf("game offset bypassed GGO physical protection: code=%d err=%v", code, err)
	}
}

func TestP12Clock86PrisonUsesRealDeadlineAndKeepsDailyCounters(t *testing.T) {
	t.Setenv("HYJS_P12_PK_MYSQL_DATABASE", "")
	for _, offset := range []int64{-10, 10} {
		t.Run(fmt.Sprint(offset), func(t *testing.T) {
			f, prisoner, _, sessions := p12FieldPVPSetup(t)
			p12FieldPVPEnable(t)
			f.s.P12FieldPVPReady, f.s.P8LifeReady = true, true
			wall := time.Now()
			ctx := p12Clock86Context(f.ctx, prisoner, offset)
			game := LocalPlayAt(ctx, wall)
			state := db.P12FieldPVPState{PlayerID: prisoner, Kills: 6, Day: dailyCopyDay(game), DailyKills: 7, LootCoin: 1000, JailUntil: game.Add(time.Minute).UnixMilli(), RealJailUntil: wall.Add(time.Minute).UnixMilli(), ClockVersion: db.P12FieldPVPWallClock, Source: constUtil.P12FieldPVPSource}
			if err := db.GetIns().Save(&state).Error; err != nil {
				t.Fatal(err)
			}
			if err := updateProfile(f.ctx, prisoner, func(p *pb.Player, g *model.GameState) error {
				p.RedeemTime = proto.Int64(state.JailUntil)
				g.Field.Side.Kills = proto.Int32(6)
				g.Field.Side.CurHp = proto.Float64(0)
				g.Field.Side.State = proto.Int32(constUtil.P8FieldDead)
				return nil
			}); err != nil {
				t.Fatal(err)
			}
			if err := P12FieldPVPRequireIdle(db.GetIns(), prisoner, wall.Add(59*time.Second)); err == nil {
				t.Fatal("jail expired before its real deadline")
			}
			if err := P12FieldPVPRequireIdle(db.GetIns(), prisoner, wall.Add(time.Minute)); err != nil {
				t.Fatalf("jail lasted beyond its real deadline: %v", err)
			}
			quote, err := f.s.p12FieldPVPRedeemAt(ctx, prisoner, sessions[prisoner], &pb.CSPlayerRedeem{Type: proto.Int32(0)}, 0, wall)
			if err != nil || quote[0].(*pb.SCPlayerRedeem).GetRedeemTime() != state.JailUntil || quote[0].(*pb.SCPlayerRedeem).GetCost()[0].GetNum() != 10000 {
				t.Fatalf("redemption quote or legacy wire timestamp changed: %v", err)
			}
			paid, err := f.s.p12FieldPVPRedeemAt(ctx, prisoner, sessions[prisoner], &pb.CSPlayerRedeem{Type: proto.Int32(1)}, 0, wall)
			if err != nil || paid[0].(*pb.SCPlayerRedeem).GetRedeemTime() != 0 {
				t.Fatalf("paid release failed: %v", err)
			}
			if err = db.GetIns().First(&state, "player_id=?", prisoner).Error; err != nil || state.RealJailUntil != 0 || state.JailUntil != 0 || state.DailyKills != 7 || state.LootCoin != 1000 || state.RedeemCount != 1 {
				t.Fatalf("release altered daily counters or left jail: %+v %v", state, err)
			}
		})
	}
}

func TestP12Clock86LegacyPrisonReadsStayAvailableAndBlocked(t *testing.T) {
	t.Setenv("HYJS_P12_PK_MYSQL_DATABASE", "")
	f, prisoner, _, _ := p12FieldPVPSetup(t)
	f.s.P12FieldPVPReady = true
	state := db.P12FieldPVPState{PlayerID: prisoner, Kills: 6, JailUntil: 1, Source: constUtil.P12FieldPVPSource}
	if err := db.GetIns().Save(&state).Error; err != nil {
		t.Fatal(err)
	}
	if err := updateProfile(f.ctx, prisoner, func(p *pb.Player, g *model.GameState) error {
		p.RedeemTime = proto.Int64(state.JailUntil)
		g.Position.Scene = proto.String("town_main")
		g.Position.Current = &pb.PointNew{X: proto.Float32(float32(constUtil.P12FieldPVPPrisonX)), Y: proto.Float32(float32(constUtil.P12FieldPVPPrisonY)), Z: proto.Float32(float32(constUtil.P12FieldPVPPrisonZ))}
		g.Field.Side.Kills = proto.Int32(6)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	var profile db.Profile
	if err := db.GetIns().First(&profile, prisoner).Error; err != nil {
		t.Fatal(err)
	}
	player, game, err := decodeProfile(&profile)
	if err != nil {
		t.Fatal(err)
	}
	before := string(p12FieldPVPProfiles(t, prisoner))
	wall := time.Now()
	view, shown, err := P12FieldPVPPrisonSide(db.GetIns(), player, game, wall)
	if err != nil || !shown || view.GetKills() != 6 {
		t.Fatalf("legacy prison projection unavailable: %v", err)
	}
	if _, err = f.s.ProjectP12FieldPVPSide(db.GetIns(), prisoner, view, wall); err != nil {
		t.Fatalf("legacy prison without presentation blocks reads: %v", err)
	}
	if err = P12FieldPVPRequireIdle(db.GetIns(), prisoner, wall); err == nil || err.Error() != constUtil.P12FieldPVPJailed {
		t.Fatalf("legacy prison action was not blocked: %v", err)
	}
	if err = f.s.SyncP12FieldPVPPenalty(db.GetIns(), player, game, wall); err != nil || before != string(p12FieldPVPProfiles(t, prisoner)) {
		t.Fatalf("legacy prison read changed persisted data: %v", err)
	}
}

func TestP12Clock86GGOStatusRealLeaseAndLegacyRecovery(t *testing.T) {
	t.Setenv("HYJS_P12_PK_MYSQL_DATABASE", "")
	for _, offset := range []int64{-10, 10} {
		t.Run(fmt.Sprint(offset), func(t *testing.T) {
			f, attacker, defender, sessions := p12FieldPVPSetup(t)
			p12FieldPVPEnable(t)
			if err := db.GetIns().AutoMigrate(&db.AdminFieldDeployment{}, &db.GGOFieldReceipt{}, &db.ClientUpdateSession{}, &db.P8GatherAttempt{}); err != nil {
				t.Fatal(err)
			}
			cfg := ggoLineTestConfiguration(t)
			values, err := ggoChipValues(cfg)
			if err != nil {
				t.Fatal(err)
			}
			f.s.GGOIntegrationReady, f.s.P12FieldPVPReady = true, true
			f.s.GGOFieldConfig, f.s.GGOFieldEconomy, f.s.GGOFieldRevision = cfg, cfg, "clock86-release"
			database := db.GetIns()
			pool, err := database.DB()
			if err != nil {
				t.Fatal(err)
			}
			f.s.adminFieldRuntimeCache.Store(&adminFieldRuntimeSnapshot{serverID: f.s.OriginalServerID(), database: database, sqlDatabase: pool, signature: adminFieldDeploymentSignature(nil), tables: f.s.Tables, ggo: cfg, ggoEconomy: cfg, ggoRevision: "clock86-release"})
			for i, id := range []int64{attacker, defender} {
				if err = updateProfile(f.ctx, id, func(_ *pb.Player, g *model.GameState) error {
					g.Position = &pb.Position{PlayerId: proto.Int64(id), Scene: proto.String(constUtil.GGOFieldScene), Current: &pb.PointNew{X: proto.Float32(20 + float32(i)), Y: proto.Float32(0), Z: proto.Float32(20)}}
					g.PositionLoading = false
					hp := 1.0
					if i == 1 {
						hp = .01
					}
					g.Field = &model.FieldState{Scene: constUtil.GGOFieldScene, Side: &pb.PlayerSide{PlayerId: proto.Int64(id), State: proto.Int32(0), CurHp: proto.Float64(hp), Anger: proto.Float64(0)}}
					g.GGOField = &model.GGOFieldState{Chips: [6]int64{10, 2, 0, 0, 0, 3}, EconomyVersion: cfg.Runtime.EconomyVersion, ExchangeValues: values}
					return nil
				}); err != nil {
					t.Fatal(err)
				}
				binding := db.ClientUpdateSession{PlayerID: id, SessionHash: clientUpdateSessionHash(sessions[id]), BuildCode: 1}
				if err = database.Save(&binding).Error; err != nil {
					t.Fatal(err)
				}
				if err = f.s.SyncP10Scene(f.ctx, id, false); err != nil {
					t.Fatal(err)
				}
			}
			ctx := p12Clock86Context(f.ctx, attacker, offset)
			request := &pb.CSSidePvpFightResult{PlayerId: proto.Int64(defender)}
			frames, err := f.s.p12FieldPVPFightClocked(ctx, attacker, sessions[attacker], request, 860020, time.Now)
			if err != nil || frames[0].(*pb.SCSidePvpFightResult).GetCode() != 1 {
				t.Fatalf("GGO PK: %v", err)
			}
			var receipt db.P12FieldPVPReceipt
			if err = database.First(&receipt, "player_id=? AND stamp=?", attacker, 860020).Error; err != nil {
				t.Fatal(err)
			}
			raw, err := proto.MarshalOptions{Deterministic: true}.Marshal(request)
			if err != nil {
				t.Fatal(err)
			}
			query := model.GGOFieldPVPStatusRequest{GGOFieldAuth: model.GGOFieldAuth{PlayerID: attacker, SessionHex: hex.EncodeToString(sessions[attacker])}, Stamp: 860020, NativeRequest: raw}
			status, err := f.s.GGOFieldPVPStatus(ctx, query, sessions[attacker])
			if err != nil || status.Status != "settled" || !status.PresentationCurrent || status.PresentationUntilMS != receipt.RealBusyUntil || len(status.NativeState) == 0 {
				t.Fatalf("GGO did not expose real lease: %+v %v", status, err)
			}
			if err = database.Model(&db.P12FieldPVPReceipt{}).Where("player_id=?", attacker).Updates(map[string]any{"clock_version": 0, "real_started_at": 0, "real_busy_until": 0}).Error; err != nil {
				t.Fatal(err)
			}
			if err = database.Model(&db.P12FieldPVPState{}).Where("player_id IN ?", []int64{attacker, defender}).Updates(map[string]any{"clock_version": 0, "real_busy_until": 0, "real_jail_until": 0}).Error; err != nil {
				t.Fatal(err)
			}
			legacy, err := f.s.GGOFieldPVPStatus(ctx, query, sessions[attacker])
			if err != nil || legacy.Status != "settled" || legacy.PresentationCurrent || legacy.PresentationUntilMS != 0 || !bytes.Equal(status.NativeResponse, legacy.NativeResponse) {
				t.Fatalf("legacy GGO result recovery guessed a deadline: %+v %v", legacy, err)
			}
		})
	}
}

func p12Clock86Injure(t *testing.T, f *p1Fixture, id int64) {
	t.Helper()
	if err := updateProfile(f.ctx, id, func(_ *pb.Player, g *model.GameState) error {
		g.Field.Side.CurHp = proto.Float64(.01)
		g.Field.Side.BattlePos3 = proto.Float64(.01)
		g.Field.Side.BattlePosHps = []*pb.BattlePosHp{{BattlePos: proto.Int32(3), HpPercent: proto.Float64(.01)}}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

func TestP12Clock86SnowCalendarAndFreeRevive(t *testing.T) {
	t.Setenv("HYJS_P12_PK_MYSQL_DATABASE", "")
	for _, offset := range []int64{-10, 10} {
		t.Run(fmt.Sprint(offset), func(t *testing.T) {
			f, victim, _, sessions := p12FieldPVPSetup(t)
			p12FieldPVPEnable(t)
			f.s.P12FieldPVPReady, f.s.P8LifeReady = true, true
			if err := f.s.InitializeP8Gather(f.ctx); err != nil {
				t.Fatal(err)
			}
			p12Clock86Injure(t, f, victim)
			ctx := p12Clock86Context(f.ctx, f.id, offset)
			wall := time.Now().Add(-11 * time.Second)
			req := &pb.CSSidePvpFightResult{PlayerId: proto.Int64(victim)}
			frames, err := f.s.p12FieldPVPFightAt(ctx, f.id, sessions[f.id], req, 860001, wall)
			if err != nil || frames[0].(*pb.SCSidePvpFightResult).GetWinPlayerId() != f.id {
				t.Fatalf("actual PK: %v", err)
			}
			var state db.P12FieldPVPState
			if err := db.GetIns().First(&state, "player_id=?", f.id).Error; err != nil {
				t.Fatal(err)
			}
			wantDay := wall.AddDate(0, 0, int(offset)).In(time.FixedZone("Asia/Shanghai", 8*3600)).Format("2006-01-02")
			if state.Day != wantDay {
				t.Fatalf("PVP daily day=%s want=%s", state.Day, wantDay)
			}
			var life db.P8LifeState
			if err := db.GetIns().First(&life, "player_id=?", victim).Error; err != nil || life.DeathAt != wall.UnixMilli() {
				t.Fatalf("PK death must use wall time: %+v %v", life, err)
			}
			if _, err := f.s.p8SafeRevive(ctx, victim, &pb.CSTransfer2Safe{}, 860002); err != nil {
				t.Fatalf("free revive after 10 real seconds: %v", err)
			}
			var profile db.Profile
			if err := db.GetIns().First(&profile, victim).Error; err != nil {
				t.Fatal(err)
			}
			_, game, err := decodeProfile(&profile)
			if err != nil || game.Position.GetScene() != "field_002" || game.Position.GetCurrent().GetX() != 59 || game.Position.GetCurrent().GetY() != 15 || game.Position.GetCurrent().GetZ() != 77 || game.Field.Side.GetState() != constUtil.P8FieldIdle || game.Field.Side.GetCurHp() != .1 {
				t.Fatalf("free revive did not return to snow safe area: %v", err)
			}
		})
	}
}

func TestP12Clock86DispatchPresentationAndPeerDay(t *testing.T) {
	t.Setenv("HYJS_P12_PK_MYSQL_DATABASE", "")
	for _, offset := range []int64{-10, 10} {
		t.Run(fmt.Sprint(offset), func(t *testing.T) {
			f, victim, _, sessions := p12FieldPVPSetup(t)
			p12FieldPVPEnable(t)
			f.s.P12FieldPVPReady, f.s.P8LifeReady = true, true
			p12Clock86Injure(t, f, victim)
			peerDay := time.Now().AddDate(0, 0, -int(offset)).In(time.FixedZone("Asia/Shanghai", 8*3600)).Format("2006-01-02")
			peer := db.P12FieldPVPState{PlayerID: victim, Day: peerDay, DailyKills: 7, LootCoin: 1000, Source: constUtil.P12FieldPVPSource}
			if err := db.GetIns().Save(&peer).Error; err != nil {
				t.Fatal(err)
			}
			ctx := p12Clock86Context(f.ctx, f.id, offset)
			start := time.Now()
			frames, handled, err := f.s.DispatchP12FieldPVP(ctx, f.id, sessions[f.id], &pb.CSSidePvpFightResult{PlayerId: proto.Int64(victim)}, 860003)
			finish := time.Now()
			if !handled || err != nil || frames[0].(*pb.SCSidePvpFightResult).GetCode() != 1 {
				t.Fatalf("dispatch: handled=%v error=%v", handled, err)
			}
			if err = db.GetIns().First(&peer, "player_id=?", victim).Error; err != nil || peer.Day != peerDay || peer.DailyKills != 7 || peer.LootCoin != 1000 {
				t.Fatalf("opponent calendar was rewritten: %+v %v", peer, err)
			}
			var life db.P8LifeState
			if err = db.GetIns().First(&life, "player_id=?", victim).Error; err != nil || life.DeathAt < start.UnixMilli() || life.DeathAt > finish.UnixMilli() {
				t.Fatalf("death outside wall window: %+v %v", life, err)
			}
			if err = P12FieldPVPRequireIdle(db.GetIns(), victim, start.Add(time.Second)); err == nil {
				t.Fatal("presentation lease expired too early")
			}
			if err = f.s.TickP12FieldPVPPresentation(ctx, finish.Add(7*time.Second)); err != nil {
				t.Fatal(err)
			}
			if err = P12FieldPVPRequireIdle(db.GetIns(), victim, finish.Add(7*time.Second)); err != nil {
				t.Fatalf("presentation lease outlived six real seconds: %v", err)
			}
		})
	}
}

func TestP12Clock86RollbackDoesNotPersistTimeOrSettlement(t *testing.T) {
	t.Setenv("HYJS_P12_PK_MYSQL_DATABASE", "")
	f, victim, _, sessions := p12FieldPVPSetup(t)
	p12FieldPVPEnable(t)
	f.s.P12FieldPVPReady, f.s.P8LifeReady = true, true
	p12Clock86Injure(t, f, victim)
	before := string(p12FieldPVPProfiles(t, f.id, victim))
	const callback = "test:p12-clock86-receipt-failure"
	if err := db.GetIns().Callback().Create().Before("gorm:create").Register(callback, func(tx *gorm.DB) {
		if tx.Statement.Table == "p12_field_pvp_receipts" {
			tx.AddError(fmt.Errorf("injected receipt failure"))
		}
	}); err != nil {
		t.Fatal(err)
	}
	defer db.GetIns().Callback().Create().Remove(callback)
	_, err := f.s.p12FieldPVPFightAt(p12Clock86Context(f.ctx, f.id, 10), f.id, sessions[f.id], &pb.CSSidePvpFightResult{PlayerId: proto.Int64(victim)}, 860004, time.Now())
	if err == nil || before != string(p12FieldPVPProfiles(t, f.id, victim)) {
		t.Fatal("failed receipt must roll back profiles", err)
	}
	for _, table := range []any{&db.P12FieldPVPState{}, &db.P12FieldPVPReceipt{}, &db.P6BattleRecord{}} {
		var count int64
		if err := db.GetIns().Model(table).Count(&count).Error; err != nil || count != 0 {
			t.Fatalf("failed receipt persisted domain data: %T count=%d error=%v", table, count, err)
		}
	}
}
