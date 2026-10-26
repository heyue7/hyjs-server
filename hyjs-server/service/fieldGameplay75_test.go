package service

import (
	"context"
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

// fieldGameplay75Fixture publishes the captured production catalogue through the
// original admin save/deploy APIs. Only role level, craft eligibility and fatigue
// are finite test preparation; resource nodes and rewards come from consumers.
func fieldGameplay75Fixture(t *testing.T) (*p1Fixture, *model.AdminPrincipal, []model.AdminFieldConfiguration) {
	t.Helper()
	f, actor := adminFieldRuntimeTestSetup(t)
	if f.mysql.Database != "" {
		t.Fatal("field75 regression requires its own temporary SQLite file")
	}
	f.ctx = context.WithValue(f.ctx, constUtil.GameServiceContextKey, f.s)
	for _, load := range []func(string) error{f.s.LoadP5Tables} {
		if err := load("../../analysis/tables"); err != nil {
			t.Fatal(err)
		}
	}
	for _, initialize := range []func(context.Context) error{f.s.InitializeP5, f.s.InitializeP8Gather, f.s.InitializeP8Life, f.s.InitGems, f.s.InitCooking, f.s.InitializeDailyCopy, f.s.InitializeFieldResources, f.s.InitializeSharedFieldBoss55, f.s.InitializeFieldBossDeath73, f.s.InitializeP13Team, f.s.InitializeAdminGameServers, f.s.InitializeServerGameClock} {
		if err := initialize(f.ctx); err != nil {
			t.Fatal(err)
		}
	}
	f.s.P8GatherReady = true
	if err := db.GetIns().AutoMigrate(&db.LocalPlayActivityOperation{}); err != nil {
		t.Fatal(err)
	}
	f.s.InitializeGameTableLookup()
	if err := db.GetIns().Model(&db.AdminUser{}).Where("id=?", actor.UserID).Update("role", constUtil.AdminRoleAdmin).Error; err != nil {
		t.Fatal(err)
	}
	actor.Role = constUtil.AdminRoleAdmin
	raw, err := readTestAnalysis75("../../analysis/growth74/live-verification.json")
	if err != nil {
		t.Fatal(err)
	}
	var snapshot struct {
		Fields []struct {
			Configuration model.AdminFieldConfiguration `json:"configuration"`
		} `json:"fields"`
	}
	if err = json.Unmarshal(raw, &snapshot); err != nil || len(snapshot.Fields) != 2 {
		t.Fatal("captured grass/snow configuration", err)
	}
	configs := []model.AdminFieldConfiguration{}
	for _, live := range snapshot.Fields {
		cfg := live.Configuration
		saved, e := f.s.AdminSaveField(f.ctx, actor, model.AdminFieldSaveRequest{Name: "已发布快照回归-" + cfg.Scene, Configuration: cfg})
		if e != nil {
			t.Fatal(cfg.Scene, e)
		}
		if _, e = f.s.AdminDeployField(f.ctx, actor, model.AdminFieldDeployRequest{ServerID: f.s.OriginalServerID(), Scene: cfg.Scene, TemplateID: saved.ID, TemplateVersion: saved.Version, Enabled: true}); e != nil {
			t.Fatal(e)
		}
		configs = append(configs, cfg)
	}
	if err = updateProfile(f.ctx, f.id, func(p *pb.Player, _ *model.GameState) error { p.Level = proto.Int32(40); return nil }); err != nil {
		t.Fatal(err)
	}
	if err = db.GetIns().Save(&db.GemCraftState{PlayerID: f.id, Level: 6}).Error; err != nil {
		t.Fatal(err)
	}
	if err = db.GetIns().Save(&db.CookingCraftState{PlayerID: f.id, Level: 6}).Error; err != nil {
		t.Fatal(err)
	}
	return f, actor, configs
}

func fieldGameplay75Runtime(t *testing.T, f *p1Fixture, body proto.Message) (*GameService, context.Context) {
	t.Helper()
	ctx := serverClockBindTest(t, f, f.id)
	runtime, err := f.s.fieldRuntimeForRequest(ctx, &model.GamePacket{Body: body})
	if err != nil {
		t.Fatal(err)
	}
	if runtime == f.s {
		t.Fatal("original field message bypassed published runtime")
	}
	return runtime, bindFieldRuntime(ctx, runtime)
}

func fieldGameplay75Clock(t *testing.T, f *p1Fixture, actor *model.AdminPrincipal, delta int64) {
	t.Helper()
	clock, err := f.s.AdminServerGameClock(f.ctx, actor)
	if err != nil {
		t.Fatal(err)
	}
	_, err = f.s.AdminChangeServerGameClock(f.ctx, actor, model.ServerGameClockChangeRequest{DeltaDays: delta, Revision: clock.Revision, RequestID: fmt.Sprintf("field75-%d-%d", delta, time.Now().UnixNano())})
	if err != nil {
		t.Fatal(err)
	}
}

func TestFieldGameplay75PublishedMineFishRealWaitColdAndClock(t *testing.T) {
	f, actor, configs := fieldGameplay75Fixture(t)
	for _, live := range configs {
		for _, kind := range []string{"mine", "fish"} {
			t.Run(live.Scene+"/"+kind, func(t *testing.T) {
				runtime, ctx := fieldGameplay75Runtime(t, f, &pb.CSTransferScene{})
				if _, err := runtime.fieldTransfer(ctx, f.id, &pb.CSTransferScene{Target: proto.String(live.Scene)}); err != nil {
					t.Fatal(err)
				}
				if _, err := runtime.fieldScene(ctx, f.id, false); err != nil {
					t.Fatal("published scene/resource projection", err)
				}
				rows, err := P8GatherNodes(db.GetIns(), f.id)
				if err != nil {
					t.Fatal(err)
				}
				var node db.P8ResourceNode
				var cfg *p8GatherConfig
				for _, candidate := range rows {
					if candidate.Scene != live.Scene {
						continue
					}
					parsed, e := p8DecodeConfig(candidate.Snapshot)
					if e != nil {
						t.Fatal(e)
					}
					if parsed.Kind == kind && candidate.Remaining > 0 {
						node, cfg = candidate, parsed
						break
					}
				}
				if cfg == nil {
					t.Fatal("no actual published node", live.Scene, kind)
				}
				if err = updateProfile(ctx, f.id, func(_ *pb.Player, g *model.GameState) error {
					g.Position.Current = proto.Clone(cfg.Point).(*pb.PointNew)
					return nil
				}); err != nil {
					t.Fatal(err)
				}
				startReq := &pb.CSFieldResourceStartGather{PosId: proto.String(node.PosID)}
				runtime, ctx = fieldGameplay75Runtime(t, f, startReq)
				start, handled, err := runtime.DispatchP8Gather(ctx, f.id, startReq, 0)
				if err != nil || !handled || start[0].(*pb.SCFieldResourceStartGather).GetResult() != 1 {
					t.Fatal("original TCP start", handled, err, start)
				}
				var accepted db.P8GatherAttempt
				if err = db.GetIns().First(&accepted, "player_id=?", f.id).Error; err != nil {
					t.Fatal(err)
				}
				if accepted.ReadyAt-accepted.StartedAt != cfg.GatherMS {
					t.Fatal("native gathering duration changed", accepted)
				}
				before, beforeGame := f.profile(t)
				fatigue := beforeGame.Field.Side.GetTiredness()
				for _, delta := range []int64{1, -1} {
					fieldGameplay75Clock(t, f, actor, delta)
					runtime, ctx = fieldGameplay75Runtime(t, f, &pb.CSFieldGetResource{})
					if time.Now().UnixMilli() < accepted.ReadyAt {
						if _, _, e := runtime.DispatchP8Gather(ctx, f.id, &pb.CSFieldGetResource{}, 0); e == nil {
							t.Fatal("calendar shift skipped real gathering wait", delta)
						}
					}
					var still db.P8GatherAttempt
					if e := db.GetIns().First(&still, "player_id=?", f.id).Error; e != nil || still.Token != accepted.Token || still.ReadyAt != accepted.ReadyAt || !still.Active {
						t.Fatal("calendar shifted accepted real timer", e, still)
					}
				}
				// Reopen the owned SQLite database as a dropped connection/cold request;
				// preserve the original accepted snapshot, token and wall-clock deadline.
				f.restart(t)
				runtime, ctx = fieldGameplay75Runtime(t, f, &pb.CSFieldResourceStartGather{})
				restarted, _, err := runtime.DispatchP8Gather(ctx, f.id, startReq, 0)
				if err != nil || restarted[0].(*pb.SCFieldResourceStartGather).GetResult() != 1 {
					t.Fatal("resume native start", err)
				}
				var resumed db.P8GatherAttempt
				if e := db.GetIns().First(&resumed, "player_id=?", f.id).Error; e != nil || resumed.Token != accepted.Token || resumed.ReadyAt != accepted.ReadyAt {
					t.Fatal("resume restarted timer or rerolled rewards", e)
				}
				if wait := time.Until(time.UnixMilli(accepted.ReadyAt)); wait > 0 {
					time.Sleep(wait + 10*time.Millisecond)
				}
				runtime, ctx = fieldGameplay75Runtime(t, f, &pb.CSFieldGetResource{})
				out, _, err := runtime.DispatchP8Gather(ctx, f.id, &pb.CSFieldGetResource{}, 0)
				if err != nil {
					t.Fatal("real waited claim", err)
				}
				claim := out[0].(*pb.SCFieldGetResource)
				if claim.GetCode() != 1 || len(claim.GetCost()) != 1 || claim.GetCost()[0].GetNum() != cfg.Fatigue {
					t.Fatal("actual resource claim", claim)
				}
				after, game := f.profile(t)
				if game.Field.Side.GetTiredness() != fatigue-cfg.Fatigue {
					t.Fatal("fatigue not consumed exactly once")
				}
				for _, r := range mustP3Resources75(t, accepted.Rewards) {
					if delta := fieldGameplay75Owned(after, game, r.GetId()) - fieldGameplay75Owned(before, beforeGame, r.GetId()); delta != int64(r.GetNum()) {
						t.Fatal("claim differs from accepted real roll", r.GetId(), delta, r.GetNum())
					}
				}
				var consumed db.P8ResourceNode
				if err = db.GetIns().First(&consumed, "player_id=? AND scene=? AND pos_id=?", f.id, node.Scene, node.PosID).Error; err != nil || consumed.Remaining != node.Remaining-1 {
					t.Fatal("node not consumed once", err, consumed)
				}
				again, _, err := runtime.DispatchP8Gather(ctx, f.id, &pb.CSFieldGetResource{}, 0)
				if err != nil || again[0].(*pb.SCFieldGetResource).GetCode() != constUtil.P8ResourceMissing {
					t.Fatal("duplicate native claim granted twice", err, again)
				}
				afterAgain, gameAgain := f.profile(t)
				if !proto.Equal(after, afterAgain) || gameAgain.Field.Side.GetTiredness() != game.Field.Side.GetTiredness() {
					t.Fatal("duplicate changed assets")
				}
				t.Logf("scene=%s kind=%s group=%s realWaitMs=%d frozenRewards=%s", live.Scene, kind, node.ResourceID, cfg.GatherMS, accepted.Rewards)
			})
		}
	}
}

func mustP3Resources75(t *testing.T, spec string) []*pb.BaseResource {
	t.Helper()
	r, err := p3Resources(spec)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func fieldGameplay75Owned(p *pb.Player, g *model.GameState, key string) int64 {
	amount := simpleOwned(p, g, key)
	for _, stack := range [][]*pb.SimpleItem{g.Gems, g.Cooking} {
		for _, item := range stack {
			if item.GetId() == key {
				amount += int64(item.GetNum())
			}
		}
	}
	return amount
}

func TestFieldGameplay75BossQuotaRoundTripKeepsLegacyAndClaimed(t *testing.T) {
	f, actor, _ := fieldGameplay75Fixture(t)
	ctx := serverClockBindTest(t, f, f.id)
	day := LocalPlayNow(ctx).Format("2006-01-02")
	before, _ := f.profile(t)
	// Finite pre-migration fixture: two already consumed non-death victories.
	// New death receipts must share that same global day budget.
	if err := updateProfile(ctx, f.id, func(_ *pb.Player, g *model.GameState) error {
		g.Field = &model.FieldState{Scene: "field_001", BossRewardDay: day, BossRewardClaims: 2}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	add := func(spawn, day string) {
		t.Helper()
		r := db.FieldBossContribution73{ServerID: f.s.OriginalServerID(), SpawnUUID: spawn, PlayerID: f.id, Damage: 1, Rules: []byte(`{}`), DailyLimit: 3, Reward: "coin,5", RewardDay: day, Rank: 1, Frozen: true}
		if err := db.GetIns().Create(&r).Error; err != nil {
			t.Fatal(err)
		}
	}
	collect := func() {
		t.Helper()
		ctx = serverClockBindTest(t, f, f.id)
		if err := f.s.CollectFieldBossPending(ctx, f.id); err != nil {
			t.Fatal(err)
		}
	}
	add("quota75-first", day)
	collect()
	p, g := f.profile(t)
	if p.GetCoin()-before.GetCoin() != 5 || g.Field.BossRewardClaims != 3 {
		t.Fatal("legacy budget was not merged", p.GetCoin(), g.Field)
	}
	fieldGameplay75Clock(t, f, actor, 1)
	ctx = serverClockBindTest(t, f, f.id)
	tomorrow := LocalPlayNow(ctx).Format("2006-01-02")
	add("quota75-tomorrow", tomorrow)
	collect()
	fieldGameplay75Clock(t, f, actor, -1)
	add("quota75-old-day", day)
	collect()
	p, g = f.profile(t)
	if p.GetCoin()-before.GetCoin() != 10 || g.Field.BossRewardDay != day || g.Field.BossRewardClaims != 3 {
		t.Fatal("date return reopened legacy plus claimed budget", p.GetCoin(), g.Field)
	}
	var refused db.FieldBossContribution73
	if err := db.GetIns().Where("spawn_uuid=?", "quota75-old-day").First(&refused).Error; err != nil || !refused.Claimed || refused.Rank != 0 || refused.RewardDay != day || refused.Reward != "coin,5" {
		t.Fatal("old receipt reward facts changed", err, refused)
	}
	f.restart(t)
	collect()
	p, g = f.profile(t)
	if p.GetCoin()-before.GetCoin() != 10 || g.Field.BossRewardClaims != 3 {
		t.Fatal("cold retry duplicated quota")
	}
	fieldGameplay75Clock(t, f, actor, 1)
	add("quota75-tomorrow-second", tomorrow)
	collect()
	p, g = f.profile(t)
	if p.GetCoin()-before.GetCoin() != 15 || g.Field.BossRewardClaims != 2 {
		t.Fatal("second date lost its own prior consumption", p.GetCoin(), g.Field.BossRewardClaims)
	}
}

func TestFieldGameplay75LegacyConsumerRecoversClaimedBeforeCache(t *testing.T) {
	f, _, _ := fieldGameplay75Fixture(t)
	ctx := serverClockBindTest(t, f, f.id)
	day := LocalPlayNow(ctx).Format("2006-01-02")
	for i := 0; i < 3; i++ {
		r := db.FieldBossContribution73{ServerID: f.s.OriginalServerID(), SpawnUUID: fmt.Sprint("prior-claim75-", i), PlayerID: f.id, Damage: 1, Rules: []byte(`{}`), DailyLimit: 3, Reward: "coin,5", RewardDay: day, Rank: 1, Frozen: true, Claimed: true}
		if err := db.GetIns().Create(&r).Error; err != nil {
			t.Fatal(err)
		}
	}
	// The contribution ledger predates the reversible day cache. Exercise the
	// original non-death resource consumer; no battle or reward is fabricated.
	limit := int64(3)
	if err := updateProfileWithTx(ctx, f.id, func(tx *gorm.DB, _ *pb.Player, g *model.GameState) error {
		g.Field = &model.FieldState{Scene: "field_002"}
		content, err := fieldBossResourceContent(ctx, g.Field, &model.FieldFight{BossRewardDailyLimit: &limit}, "coin,1", tx)
		if err != nil {
			return err
		}
		if content != "" || g.Field.BossRewardClaims != 3 {
			t.Fatal("legacy consumer ignored already claimed death rows", content, g.Field.BossRewardClaims)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

func TestFieldGameplay75SharedBossRealEngineRankTailCapAndClock(t *testing.T) {
	f, actor, configs := fieldGameplay75Fixture(t)
	policy, err := f.s.AdminFieldResources(f.ctx, actor)
	if err != nil {
		t.Fatal(err)
	}
	policy.BossDailyRewards = 3
	if _, err = f.s.AdminSaveFieldResources(f.ctx, actor, model.AdminFieldResourcesSaveRequest{AdminFieldResourcesResponse: *policy, Reason: "隔离跨图三次首领验收"}); err != nil {
		t.Fatal(err)
	}
	before, _ := f.profile(t)
	var lastReq *pb.CSSidePveBossFightEnd
	var lastResult proto.Message
	for i := 0; i < 4; i++ {
		live := configs[i%len(configs)]
		// Only shorten the owned fixture's dead-generation wall-clock deadline;
		// every new generation still uses original refresh/start/engine/end paths.
		if i >= 2 {
			if err := db.GetIns().Model(&db.SharedFieldBoss55{}).Where("server_id=? AND scene=? AND respawn_at>0", f.s.OriginalServerID(), live.Scene).Update("respawn_at", time.Now().Add(-time.Second).UnixMilli()).Error; err != nil {
				t.Fatal(err)
			}
		}
		runtime, ctx := fieldGameplay75Runtime(t, f, &pb.CSTransferScene{})
		if _, err = runtime.fieldTransfer(ctx, f.id, &pb.CSTransferScene{Target: proto.String(live.Scene)}); err != nil {
			t.Fatal(err)
		}
		if _, err = runtime.fieldScene(ctx, f.id, false); err != nil {
			t.Fatal(err)
		}
		_, game := f.profile(t)
		boss := game.Field.Bosses[0]
		if boss.GetState() != 1 {
			t.Fatal("actual refresh did not produce live boss", boss)
		}
		monster, err := runtime.fieldBossMonster(boss.GetUuid(), game, boss)
		if err != nil {
			t.Fatal(err)
		}
		leader := runtime.fieldBossLeaderInScene(boss.GetBossId(), live.Scene)
		hps := map[string]float64{}
		for _, hero := range monster.GetFormation() {
			hps[hero.GetGuid()] = 0
		}
		hps["field-boss-"+boss.GetUuid()+"-"+leader["Id"]] = 1
		hpRaw, _ := json.Marshal(hps)
		if err = db.GetIns().Model(&db.SharedFieldBoss55{}).Where("server_id=? AND scene=? AND uuid=?", f.s.OriginalServerID(), live.Scene, boss.GetUuid()).Update("hp", hpRaw).Error; err != nil {
			t.Fatal(err)
		}
		startReq := &pb.CSSidePveBossFightStart{Uuid: boss.Uuid}
		runtime, ctx = fieldGameplay75Runtime(t, f, startReq)
		started, err := runtime.fieldBossFightStart(ctx, f.id, startReq)
		if err != nil {
			t.Fatal("published shared death boss start", err)
		}
		_, game = f.profile(t)
		if game.Field.Fight.BossRewardDailyLimit == nil || *game.Field.Fight.BossRewardDailyLimit != 3 || !game.Field.Fight.BossDeathRules73.DeathSettlement {
			t.Fatal("published cap/rules did not freeze")
		}
		input, err := fieldBossFrozenInput(game.Field.Fight)
		if err != nil {
			t.Fatal(err)
		}
		terminal, err := runtime.ComputeOriginalP6ProtocolFinalHP(ctx, input, nil)
		if err != nil || terminal.Outcome.Winner != 0 {
			t.Fatal("native engine must defeat explicit one-HP remnant", err)
		}
		request := &pb.CSSidePveBossFightEnd{FightId: started[0].(*pb.SCSidePveBossFightStart).FightId, Uuid: boss.Uuid, Result: proto.Int32(1), OptRecord: terminal.OriginalOperates}
		if i == 0 {
			var changed model.AdminFieldConfiguration
			raw, _ := json.Marshal(live)
			if err = json.Unmarshal(raw, &changed); err != nil {
				t.Fatal(err)
			}
			for j := range changed.OrdinaryBossGroups {
				changed.OrdinaryBossGroups[j].Drops = []model.AdminFieldDrop{growth74Drop("diamond", 1)}
				changed.OrdinaryBossGroups[j].KillDrops = []model.AdminFieldDrop{growth74Drop("diamond", 1)}
				changed.OrdinaryBossGroups[j].RankDrops = nil
			}
			changed.MinLevel = 999
			fieldGameplay75Publish(t, f, actor, changed, "已接受首领之后改变未来奖励")
			runtime, ctx = fieldGameplay75Runtime(t, f, request)
		}
		ended, err := runtime.fieldBossFightEnd(ctx, f.id, request)
		if err != nil {
			t.Fatal("native authoritative death settlement", err)
		}
		if i == 0 {
			fieldGameplay75Publish(t, f, actor, live, "冻结首领奖励验证后恢复原快照")
		}
		var receipt db.FieldBossContribution73
		if err = db.GetIns().Where("server_id=? AND spawn_uuid=? AND player_id=?", f.s.OriginalServerID(), boss.GetUuid(), f.id).First(&receipt).Error; err != nil {
			t.Fatal(err)
		}
		if !receipt.Frozen || !receipt.Claimed || receipt.Damage != 1 {
			t.Fatal("real contribution receipt", receipt)
		}
		if i < 3 && receipt.Rank != 1 || i == 3 && receipt.Rank != 0 {
			t.Fatal("global cross-map daily cap", i, receipt.Rank)
		}
		// Each captured grass/snow rule sums participation + rank1 + tail to1300.
		_, game = f.profile(t)
		if i < 3 && fieldGameplay75RewardAmount(t, receipt.Reward, "diamond") != 1300 {
			t.Fatal("rank/tail/participation reward sum", receipt.Reward)
		}
		after, _ := f.profile(t)
		if after.GetDiamond()-before.GetDiamond() != int64(min(i+1, 3))*1300 || game.Field.BossRewardClaims != int64(min(i+1, 3)) {
			t.Fatal("daily cap changed delivered resources", i, after.GetDiamond()-before.GetDiamond(), game.Field.BossRewardClaims)
		}
		var shared db.SharedFieldBoss55
		if err = db.GetIns().Where("server_id=? AND scene=? AND uuid=?", f.s.OriginalServerID(), live.Scene, boss.GetUuid()).First(&shared).Error; err != nil {
			t.Fatal(err)
		}
		deadline := shared.RespawnAt
		if deadline <= time.Now().UnixMilli() || shared.UUID != boss.GetUuid() {
			t.Fatal("real death did not save refresh deadline")
		}
		for _, delta := range []int64{1, -1} {
			fieldGameplay75Clock(t, f, actor, delta)
			runtime, ctx = fieldGameplay75Runtime(t, f, &pb.CSSideResourceCheck{})
			if _, err = runtime.fieldScene(ctx, f.id, false); err != nil {
				t.Fatal(err)
			}
			var unchanged db.SharedFieldBoss55
			if err = db.GetIns().Where("server_id=? AND scene=? AND pos_id=?", f.s.OriginalServerID(), live.Scene, shared.PosID).First(&unchanged).Error; err != nil || unchanged.UUID != shared.UUID || unchanged.RespawnAt != deadline {
				t.Fatal("calendar move regenerated dead shared boss", delta, err, unchanged)
			}
			_, game = f.profile(t)
			if fieldBossFind(game.Field, boss.GetUuid()).GetState() != 3 {
				t.Fatal("calendar move revived dead boss")
			}
		}
		lastReq, lastResult = request, ended[0]
	}
	f.restart(t)
	runtime, ctx := fieldGameplay75Runtime(t, f, lastReq)
	beforeRetry, _ := f.profile(t)
	again, err := runtime.fieldBossFightEnd(ctx, f.id, lastReq)
	if err != nil || !proto.Equal(lastResult, again[0]) {
		t.Fatal("cold original end changed receipt", err)
	}
	afterRetry, _ := f.profile(t)
	if !proto.Equal(beforeRetry, afterRetry) {
		t.Fatal("cold end reissued resources")
	}
}

func fieldGameplay75Publish(t *testing.T, f *p1Fixture, actor *model.AdminPrincipal, cfg model.AdminFieldConfiguration, name string) {
	t.Helper()
	saved, err := f.s.AdminSaveField(f.ctx, actor, model.AdminFieldSaveRequest{Name: name, Configuration: cfg})
	if err != nil {
		t.Fatal(err)
	}
	deps, err := f.s.AdminFieldDeployments(f.ctx, actor, f.s.OriginalServerID())
	if err != nil {
		t.Fatal(err)
	}
	var version string
	for _, d := range deps.Items {
		if d.Scene == cfg.Scene {
			version = d.Version
		}
	}
	if _, err = f.s.AdminDeployField(f.ctx, actor, model.AdminFieldDeployRequest{ServerID: f.s.OriginalServerID(), Scene: cfg.Scene, TemplateID: saved.ID, TemplateVersion: saved.Version, Version: version, Enabled: true}); err != nil {
		t.Fatal(err)
	}
}

func fieldGameplay75RewardAmount(t *testing.T, spec, key string) int64 {
	t.Helper()
	var n int64
	for _, r := range mustP3Resources75(t, spec) {
		if r.GetId() == key {
			n += int64(r.GetNum())
		}
	}
	return n
}

func TestFieldGameplay75GrowthMultiplierActualClaimAndAcceptedFreeze(t *testing.T) {
	f, actor, originals := fieldGameplay75Fixture(t)
	if err := f.s.growth74Field(f.ctx, actor); err != nil {
		t.Fatal("original deployment applies reviewed growth quantities", err)
	}
	fields, err := f.s.AdminFields(f.ctx, actor)
	if err != nil {
		t.Fatal(err)
	}
	var target model.AdminFieldTemplateDTO
	for _, v := range fields.Items {
		if v.Configuration.Scene == "field_001" && growth74Source(v.Name) {
			target = v
			break
		}
	}
	if target.ID == "" {
		t.Fatal("growth scheme was not saved")
	}
	original := map[string]model.AdminFieldGatherGroup{}
	for _, g := range originals[0].GatherGroups {
		original[g.ID] = g
	}
	for _, g := range target.Configuration.GatherGroups {
		old := original[g.ID]
		if len(g.Drops) != len(old.Drops) {
			t.Fatal("growth drop types changed unexpectedly")
		}
		for j, d := range g.Drops {
			for k, q := range d.Quantities {
				n := old.Drops[j].Quantities[k].Amount
				want := n
				if n > 0 {
					want = (n*(20+g.Level*2) + 9) / 10
				}
				if q.Amount != want {
					t.Fatal("published 2.2+ level gradient mismatch", g.ID, n, q.Amount, want)
				}
			}
		}
	}
	runtime, ctx := fieldGameplay75Runtime(t, f, &pb.CSTransferScene{})
	if _, err = runtime.fieldTransfer(ctx, f.id, &pb.CSTransferScene{Target: proto.String("field_001")}); err != nil {
		t.Fatal(err)
	}
	if _, err = runtime.fieldScene(ctx, f.id, false); err != nil {
		t.Fatal(err)
	}
	rows, err := P8GatherNodes(db.GetIns(), f.id)
	if err != nil {
		t.Fatal(err)
	}
	var node db.P8ResourceNode
	var cfg *p8GatherConfig
	for _, r := range rows {
		c, e := p8DecodeConfig(r.Snapshot)
		if e != nil {
			t.Fatal(e)
		}
		if c.Kind == "mine" {
			node, cfg = r, c
			break
		}
	}
	if cfg == nil {
		t.Fatal("published mine missing")
	}
	var published model.AdminFieldGatherGroup
	for _, g := range target.Configuration.GatherGroups {
		if g.ID == node.ResourceID {
			published = g
			break
		}
	}
	if cfg.Reward != adminFieldDropText(published.Drops) {
		t.Fatal("consumer node used base rewards instead of actual deployment", cfg.Reward, published.Drops)
	}
	if err = updateProfile(ctx, f.id, func(_ *pb.Player, g *model.GameState) error {
		g.Position.Current = proto.Clone(cfg.Point).(*pb.PointNew)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if _, _, err = runtime.DispatchP8Gather(ctx, f.id, &pb.CSFieldResourceStartGather{PosId: proto.String(node.PosID)}, 0); err != nil {
		t.Fatal(err)
	}
	var accepted db.P8GatherAttempt
	if err = db.GetIns().First(&accepted, "player_id=?", f.id).Error; err != nil {
		t.Fatal(err)
	}
	before, beforeGame := f.profile(t)
	// Edit and close only the future deployment while this real timer is live.
	changed := target.Configuration
	for i := range changed.GatherGroups {
		for j := range changed.GatherGroups[i].Drops {
			for k := range changed.GatherGroups[i].Drops[j].Quantities {
				if changed.GatherGroups[i].Drops[j].Quantities[k].Amount > 0 {
					changed.GatherGroups[i].Drops[j].Quantities[k].Amount = 1
				}
			}
		}
	}
	changed.MinLevel = 999
	saved, err := f.s.AdminSaveField(f.ctx, actor, model.AdminFieldSaveRequest{ID: target.ID, Name: target.Name, Version: target.Version, Configuration: changed})
	if err != nil {
		t.Fatal(err)
	}
	deps, err := f.s.AdminFieldDeployments(f.ctx, actor, f.s.OriginalServerID())
	if err != nil {
		t.Fatal(err)
	}
	var version string
	for _, dep := range deps.Items {
		if dep.Scene == changed.Scene {
			version = dep.Version
		}
	}
	if _, err = f.s.AdminDeployField(f.ctx, actor, model.AdminFieldDeployRequest{ServerID: f.s.OriginalServerID(), Scene: changed.Scene, TemplateID: saved.ID, TemplateVersion: saved.Version, Version: version, Enabled: false}); err != nil {
		t.Fatal(err)
	}
	if wait := time.Until(time.UnixMilli(accepted.ReadyAt)); wait > 0 {
		time.Sleep(wait + 10*time.Millisecond)
	}
	runtime, ctx = fieldGameplay75Runtime(t, f, &pb.CSFieldGetResource{})
	out, _, err := runtime.DispatchP8Gather(ctx, f.id, &pb.CSFieldGetResource{}, 0)
	if err != nil || out[0].(*pb.SCFieldGetResource).GetCode() != 1 {
		t.Fatal("future closed deployment revoked accepted resource", err)
	}
	after, game := f.profile(t)
	for _, r := range mustP3Resources75(t, accepted.Rewards) {
		if delta := fieldGameplay75Owned(after, game, r.GetId()) - fieldGameplay75Owned(before, beforeGame, r.GetId()); delta != int64(r.GetNum()) {
			t.Fatal("growth reward frozen amount changed", r, delta)
		}
	}
	if _, _, err = runtime.DispatchP8Gather(ctx, f.id, &pb.CSFieldResourceStartGather{PosId: proto.String(node.PosID)}, 0); err == nil {
		t.Fatal("closed future deployment accepted a new gather")
	}
	t.Logf("actual published scaled node=%s rewards=%s waitMs=%d; later deployment rejected new start and retained accepted claim", node.ResourceID, accepted.Rewards, cfg.GatherMS)
}
