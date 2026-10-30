package service

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"math"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

// The finite role and native presence frame are isolated test inputs. This
// exercises the real start/end services and original protocol replay, not a phone.
func ggoBossIntegrationFixture(t *testing.T) (*p1Fixture, context.Context, []byte, *ggoBossEvent) {
	t.Helper()
	f, id, ctx, _ := terminalFixture(t)
	f.id = id // terminalFixture creates a distinct owned role; bind all native calls to it.
	for _, load := range []func(string) error{f.s.LoadP5Tables, f.s.LoadP8Tables, f.s.LoadP12WorldBossTables} {
		if err := load("../../analysis/tables"); err != nil {
			t.Fatal(err)
		}
	}
	if err := f.s.InstallGGOItems(); err != nil {
		t.Fatal(err)
	}
	if err := db.GetIns().AutoMigrate(&db.P4CommerceRule{}, &db.ShopState{}, &db.P4CommerceStock{}, &db.AdminFieldDeployment{}, &db.P10ScenePresence{}, &db.ClientUpdateSession{}, &db.P13TeamMember{}); err != nil {
		t.Fatal(err)
	}
	for _, init := range []func(context.Context) error{f.s.InitializeP5, f.s.InitializeDailyCopy, f.s.InitGems, f.s.InitCooking, f.s.InitializeP8Gather, f.s.InitializeP8Life, f.s.InitializeSharedFieldBoss55, f.s.InitializeFieldBossDeath73, f.s.InitializeFieldResources, f.s.InitializeGGO} {
		if err := init(ctx); err != nil {
			t.Fatal(err)
		}
	}
	cfg := ggoLineTestConfiguration(t)
	// Keep an event live at any real test time; two hours of legal schedule span.
	cfg.Runtime.Boss.DayIntervalMinutes = 60
	cfg.Runtime.Boss.EveningIntervalMinutes = 60
	cfg.Runtime.Boss.ActiveSeconds = 3600
	cfg.Runtime.Boss.PartyRounds = 1
	cfg.Runtime.Boss.Attack = 1
	cfg.Runtime.Boss.HP = 5000000
	f.s.GGOIntegrationReady = true
	f.s.GGOFieldConfig = cfg
	f.s.GGOFieldEconomy = cfg
	values, err := ggoChipValues(cfg)
	if err != nil {
		t.Fatal(err)
	}
	var event *ggoBossEvent
	var point *pb.PointNew
	if err = db.GetIns().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		_, e, b, _, err := f.s.ggoBossCurrent(tx, cfg, time.Now().UnixMilli(), true)
		event = e
		if b != nil {
			point = b.CurrentPoint
		}
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if err = updateProfile(ctx, id, func(p *pb.Player, g *model.GameState) error {
		p.Level = proto.Int32(40)
		g.Position = &pb.Position{PlayerId: proto.Int64(id), Scene: proto.String(constUtil.GGOFieldScene), Current: point}
		g.PositionLoading = false
		g.Field = &model.FieldState{Scene: constUtil.GGOFieldScene, Side: &pb.PlayerSide{PlayerId: proto.Int64(id), State: proto.Int32(0), CurHp: proto.Float64(1), Anger: proto.Float64(0)}}
		g.GGOField = &model.GGOFieldState{EconomyVersion: cfg.Runtime.EconomyVersion, ExchangeValues: values, Chips: [6]int64{10, 0, 0, 1}}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	var profile db.Profile
	if err = db.GetIns().First(&profile, id).Error; err != nil {
		t.Fatal(err)
	}
	session := append([]byte(nil), profile.Session...)
	binding := db.ClientUpdateSession{PlayerID: id, SessionHash: clientUpdateSessionHash(session), BuildCode: cfg.Runtime.MinClientBuild}
	if err = db.GetIns().Save(&binding).Error; err != nil {
		t.Fatal(err)
	}
	frame, err := proto.Marshal(&pb.MovePlayer{Info: &pb.MovePlayerInfo{PlayerId: proto.Int64(id)}})
	if err != nil {
		t.Fatal(err)
	}
	if err = db.GetIns().Save(&db.P10ScenePresence{PlayerID: id, Scene: constUtil.GGOFieldScene, Line: 1, SessionHash: p6Digest(session), Frame: frame}).Error; err != nil {
		t.Fatal(err)
	}
	key := p10ClientKey{f.s.p10ClientOwner(), id}
	sink := &p10ClientSink{session: session, send: func(proto.Message) error { return nil }}
	p10ClientSinks.Store(key, sink)
	t.Cleanup(func() { p10ClientSinks.CompareAndDelete(key, sink); f.s.ggoStopNoticeTimers() })
	ctx = context.WithValue(ctx, ggoNativeSessionKey{}, session)
	ctx = context.WithValue(ctx, constUtil.GameServiceContextKey, f.s)
	return f, ctx, session, event
}

func TestGGOBossNativePartialEndRetryAndCold(t *testing.T) {
	f, ctx, session, event := ggoBossIntegrationFixture(t)
	startMessages, err := f.s.fieldBossFightStart(ctx, f.id, &pb.CSSidePveBossFightStart{Uuid: proto.String(event.ID)})
	if err != nil {
		t.Fatal("native single-boss start", err)
	}
	start := startMessages[0].(*pb.SCSidePveBossFightStart)
	if len(start.BattleMonsterList) != 1 || len(start.BattleMonsterList[0].Formation) != 1 || start.GetMaxround() != 2 {
		t.Fatal("single boss or round contract", start)
	}
	var row db.Profile
	if err = db.GetIns().First(&row, f.id).Error; err != nil {
		t.Fatal(err)
	}
	_, g, err := decodeProfile(&row)
	if err != nil {
		t.Fatal(err)
	}
	input, err := fieldBossFrozenInput(g.Field.Fight)
	if err != nil {
		t.Fatal(err)
	}
	terminal, _, err := f.s.ComputeP6GenericMechanismBattle(ctx, input, nil)
	if err != nil || terminal == nil || terminal.OriginalOperates == nil {
		t.Fatal("real type12 replay", err)
	}
	result := int32(0)
	if terminal.Outcome.Winner == 0 {
		result = 1
	}
	request := &pb.CSSidePveBossFightEnd{FightId: start.FightId, Uuid: proto.String(event.ID), Result: proto.Int32(result), OptRecord: terminal.OriginalOperates}
	forged := proto.Clone(request).(*pb.CSSidePveBossFightEnd)
	forged.OptRecord = proto.Clone(request.OptRecord).(*pb.BattleOperate)
	forged.OptRecord.DefenderTotalLeftHp = proto.Float64(forged.OptRecord.GetDefenderTotalLeftHp() + 1)
	invalid := proto.Clone(request).(*pb.CSSidePveBossFightEnd)
	invalid.Uuid = proto.String(event.ID + "-other")
	if _, err = f.s.fieldBossFightEnd(ctx, f.id, invalid); err == nil {
		t.Fatal("wrong event accepted")
	}
	ended, err := f.s.fieldBossFightEnd(ctx, f.id, request)
	if err != nil {
		t.Fatal("native end", err)
	}
	native := ended[0].(*pb.SCSidePveBossFightEnd)
	if native.GetCode() != 1 {
		t.Fatal(native)
	}
	if err = db.GetIns().First(&row, f.id).Error; err != nil {
		t.Fatal(err)
	}
	_, g, err = decodeProfile(&row)
	if err != nil {
		t.Fatal(err)
	}
	if !g.Field.Fight.Settled || g.GGOField.Chips != ([6]int64{10, 0, 0, 1}) || g.Field.Side.GetCurHp() <= 0 {
		t.Fatal("surviving partial battle changed carried chips")
	}
	var shared db.SharedFieldBoss55
	if err = sharedFieldBoss55Query(db.GetIns(), f.s.OriginalServerID(), constUtil.GGOFieldScene, constUtil.GGOFieldBossPosition).First(&shared).Error; err != nil {
		t.Fatal(err)
	}
	_, _, hp, err := ggoBossDecode(&shared)
	if err != nil {
		t.Fatal(err)
	}
	if hp[ggoBossGUID(event.ID)] >= float64(event.Boss.HP) || hp[ggoBossGUID(event.ID)] <= 0 {
		t.Fatal("partial authoritative damage", hp)
	}
	replay, err := f.s.fieldBossFightEnd(ctx, f.id, request)
	if err != nil || !proto.Equal(native, replay[0]) {
		t.Fatal("exact native repeat", err)
	}
	if _, err = f.s.fieldBossFightEnd(ctx, f.id, forged); err == nil {
		t.Fatal("changed retry accepted")
	}
	f.restart(t)
	replay, err = f.s.fieldBossFightEnd(ctx, f.id, request)
	if err != nil || !proto.Equal(native, replay[0]) {
		t.Fatal("cold native receipt", err)
	}
	receipt, err := f.s.GGOFieldBossReceipt(ctx, model.GGOFieldBossReceiptRequest{GGOFieldAuth: model.GGOFieldAuth{PlayerID: f.id, SessionHex: hex.EncodeToString(session)}, FightID: start.GetFightId()}, session)
	if err != nil || receipt.State != "settled" || receipt.InputHash == "" || receipt.EndRequestHash == "" || len(receipt.NativeEnd) == 0 {
		t.Fatal("authenticated receipt", receipt, err)
	}
	raw, _ := json.Marshal(receipt)
	if !bytes.Contains(raw, []byte(`"status":"settled"`)) {
		t.Fatal("client status contract", string(raw))
	}
}

func TestGGOBossSharedLastHitRollbackAndRewards(t *testing.T) {
	f, ctx, _, event := ggoBossIntegrationFixture(t)
	monster, err := f.s.ggoBossMonster(event, 100)
	if err != nil {
		t.Fatal(err)
	}
	// Seed a prior valid participant with the same frozen event; the pending
	// source still uses one contribution row per player and one shared HP row.
	if err = db.GetIns().Model(&db.SharedFieldBoss55{}).Where("server_id=? AND scene=? AND pos_id=?", f.s.OriginalServerID(), constUtil.GGOFieldScene, constUtil.GGOFieldBossPosition).Update("hp", adminJSON(map[string]float64{ggoBossGUID(event.ID): 100})).Error; err != nil {
		t.Fatal(err)
	}
	rules, _ := json.Marshal(event)
	if err = db.GetIns().Create(&db.FieldBossContribution73{ServerID: f.s.OriginalServerID(), SpawnUUID: event.ID, PlayerID: f.id, Damage: float64(event.Boss.HP) - 100, Rules: rules, DailyLimit: 3}).Error; err != nil {
		t.Fatal(err)
	}
	fight := &model.FieldFight{GGOBoss: &model.GGOFieldBossFight{EventID: event.ID, EndsAtMS: event.EndsAtMS, Boss: &pb.SideBoss{Uuid: proto.String(event.ID)}}}
	input := &model.FrozenBattleInput{Monster: monster}
	terminal := &model.CombatFinalHPOutcome{FinalActors: []model.CombatFinalActorHP{{Camp: 1, GUID: ggoBossGUID(event.ID), CurrentHP: 0, MaxHP: float64(event.Boss.HP)}}}
	forced := errors.New("rollback boss death")
	apply := func(abort bool) error {
		return db.GetIns().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			_, damage, err := f.s.ggoBossApplyDamageLocked(tx, f.id, fight, input, terminal, time.Now().UnixMilli())
			if err != nil {
				return err
			}
			if math.Abs(damage-100) > .00001 {
				return errors.New("last hit was not clipped")
			}
			if abort {
				return forced
			}
			return nil
		})
	}
	if err = apply(true); !errors.Is(err, forced) {
		t.Fatal(err)
	}
	var frozen int64
	if err = db.GetIns().Model(&db.FieldBossContribution73{}).Where("spawn_uuid=? AND frozen=?", event.ID, true).Count(&frozen).Error; err != nil || frozen != 0 {
		t.Fatal("death rollback", frozen, err)
	}
	if err = apply(false); err != nil {
		t.Fatal(err)
	}
	var member db.FieldBossContribution73
	if err = db.GetIns().Where("spawn_uuid=? AND player_id=?", event.ID, f.id).First(&member).Error; err != nil {
		t.Fatal(err)
	}
	var award ggoBossAward
	if !member.Frozen || member.Rank != 1 || json.Unmarshal(member.GGOChips, &award) != nil || award.Chips[2] != 2 {
		t.Fatal("frozen one event award", member)
	}
	gain, err := f.s.CollectFieldBossPendingRewards(ctx, f.id)
	if err != nil || gain == nil {
		t.Fatal("actual resource and chip delivery", gain, err)
	}
	if again, err := f.s.CollectFieldBossPendingRewards(ctx, f.id); err != nil || again != nil {
		t.Fatal("award replay", again, err)
	}
	var profile db.Profile
	if err = db.GetIns().First(&profile, f.id).Error; err != nil {
		t.Fatal(err)
	}
	_, g, err := decodeProfile(&profile)
	if err != nil {
		t.Fatal(err)
	}
	if g.Field.BossRewardClaims != 1 || g.GGOField.Chips[2] != 2 {
		t.Fatal("shared quota or chip delivery", g.Field.BossRewardClaims, g.GGOField.Chips)
	}
}
