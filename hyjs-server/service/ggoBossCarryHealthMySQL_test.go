package service

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"math"
	"os"
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

// Runs only in a freshly leased, isolated local-services P1 database. Two
// real native Ends overlap, then a separately prepared 1-HP leader verifies
// actual native death -> frozen award -> recipient resource/chip delivery.
func TestGGOBossCarryHealthAndDeathAwardsMySQL(t *testing.T) {
	if os.Getenv("HYJS_GGO_HEALTH_MYSQL") != "1" {
		t.Skip("requires a freshly leased local MySQL database")
	}
	if !strings.HasPrefix(os.Getenv("HYJS_P1_MYSQL_DATABASE"), "hyjs_test_p1_") || os.Getenv("HYJS_GAME_ACCOUNTS_DATABASE") != "" {
		t.Fatal("isolated P1 database required")
	}
	f, ctx, firstSession, event := ggoBossIntegrationFixture(t)
	if db.GetIns().Dialector.Name() != "mysql" {
		t.Fatal("MySQL test is not using MySQL")
	}
	pool, err := db.GetIns().DB()
	if err != nil {
		t.Fatal(err)
	}
	pool.SetMaxOpenConns(6)
	pool.SetMaxIdleConns(2)
	if err = db.GetIns().AutoMigrate(&db.P13Team{}); err != nil {
		t.Fatal(err)
	}
	f.s.P5Ready = true
	// Large finite HP keeps both actual settlements on the same live event.
	event.Boss.HP, event.Boss.PartyRounds = 1000000000, 10
	f.s.GGOFieldConfig.Runtime.Boss.HP, f.s.GGOFieldConfig.Runtime.Boss.PartyRounds = event.Boss.HP, event.Boss.PartyRounds
	if err = db.GetIns().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		row, current, boss, _, err := f.s.ggoBossCurrent(tx, f.s.GGOFieldConfig, time.Now().UnixMilli(), true, 1)
		if err != nil {
			return err
		}
		current.Boss.HP, current.Boss.PartyRounds = event.Boss.HP, event.Boss.PartyRounds
		boss.CurHp = proto.Float64(1)
		return saveGGOBossState(tx, row, current, boss, map[string]float64{ggoBossGUID(event.ID): float64(event.Boss.HP)})
	}); err != nil {
		t.Fatal(err)
	}
	peerID, peerCtx := ggoBossLines86Peer(t, f, ctx, event, 1)
	_, template, power := desertBossCalibrationTeam(t, f.s, []string{"hero_065", "hero_066", "hero_067", "hero_074", "hero_075"})
	type participant struct {
		id      int64
		ctx     context.Context
		session []byte
		start   *pb.SCSidePveBossFightStart
		request *pb.CSSidePveBossFightEnd
		damage  map[string]float64
	}
	var peerProfile db.Profile
	if err = db.GetIns().First(&peerProfile, peerID).Error; err != nil {
		t.Fatal(err)
	}
	players := []*participant{{id: f.id, ctx: ctx, session: firstSession}, {id: peerID, ctx: peerCtx, session: append([]byte(nil), peerProfile.Session...)}}
	for _, p := range players {
		fixture := &sameEventPlayer{id: p.id, ctx: p.ctx}
		sameEventGiveFormation(t, f.s, fixture, template, power)
		messages, err := f.s.fieldBossFightStart(p.ctx, p.id, &pb.CSSidePveBossFightStart{Uuid: proto.String(event.ID)})
		if err != nil {
			t.Fatal("native concurrent start", p.id, err)
		}
		p.start = messages[0].(*pb.SCSidePveBossFightStart)
		var row db.Profile
		if err = db.GetIns().First(&row, p.id).Error; err != nil {
			t.Fatal(err)
		}
		_, game, err := decodeProfile(&row)
		if err != nil {
			t.Fatal(err)
		}
		input, err := fieldBossFrozenInput(game.Field.Fight)
		if err != nil {
			t.Fatal(err)
		}
		terminal, _, err := f.s.ComputeP6GenericMechanismBattle(p.ctx, input, nil)
		if err != nil {
			t.Fatal("original engine terminal", err)
		}
		p.damage, err = ggoBossActorDamage(game.Field.Fight, input, terminal)
		if err != nil {
			t.Fatal(err)
		}
		result := int32(0)
		if terminal.Outcome.Winner == 0 {
			result = 1
		}
		p.request = &pb.CSSidePveBossFightEnd{FightId: p.start.FightId, Uuid: proto.String(event.ID), Result: proto.Int32(result), OptRecord: terminal.OriginalOperates}
	}
	leader := ggoBossGUID(event.ID)
	var attendantDamage float64
	for _, p := range players {
		for _, pose := range []string{"3", "7", "9"} {
			attendantDamage += p.damage[leader+"-"+pose]
		}
	}
	if attendantDamage <= 0 {
		t.Fatal("fixture did not produce actual attendant damage")
	}
	start := make(chan struct{})
	errs := make(chan error, len(players))
	var wg sync.WaitGroup
	for _, player := range players {
		wg.Add(1)
		go func(p *participant) {
			defer wg.Done()
			<-start
			_, err := f.s.fieldBossFightEnd(p.ctx, p.id, p.request)
			errs <- err
		}(player)
	}
	close(start)
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal("parallel native End", err)
		}
	}
	var shared db.SharedFieldBoss55
	if err = sharedFieldBoss55Query(db.GetIns(), f.s.OriginalServerID(), constUtil.GGOFieldScene, constUtil.GGOFieldBossPosition, 1).Take(&shared).Error; err != nil {
		t.Fatal(err)
	}
	_, _, hp, err := ggoBossDecode(&shared)
	if err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{leader, leader + "-3", leader + "-7", leader + "-9"} {
		maximum := float64(573045)
		if key == leader {
			maximum = float64(event.Boss.HP)
		}
		want := math.Max(0, maximum-players[0].damage[key]-players[1].damage[key])
		if math.Abs(hp[key]-want) > .001 {
			t.Fatal("parallel shared health", key, hp[key], want)
		}
	}
	var rawHP map[string]float64
	if err = json.Unmarshal(shared.HP, &rawHP); err != nil || len(rawHP) != 1 || rawHP[leader] != hp[leader] {
		t.Fatal("legacy one-key HP persistence", rawHP, err)
	}
	for _, p := range players {
		var count int64
		if err = db.GetIns().Model(&db.GGOFieldReceipt{}).Where("player_id=? AND request_id=?", p.id, ggoSystemReceiptID("boss-fight", p.start.GetFightId())).Count(&count).Error; err != nil || count != 1 {
			t.Fatal("concurrent receipt", p.id, count, err)
		}
		if _, err = f.s.fieldBossFightEnd(p.ctx, p.id, p.request); err != nil {
			t.Fatal("concurrent end retry", p.id, err)
		}
	}
	// The test deliberately reduces the isolated remaining leader HP and clears
	// attendants so a real next battle produces a deterministic original kill.
	if err = db.GetIns().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		row, current, boss, currentHP, err := f.s.ggoBossCurrent(tx, f.s.GGOFieldConfig, time.Now().UnixMilli(), true, 1)
		if err != nil {
			return err
		}
		currentHP[leader], currentHP[leader+"-3"], currentHP[leader+"-7"], currentHP[leader+"-9"] = 1, 0, 0, 0
		boss.CurHp = proto.Float64(1 / float64(event.Boss.HP))
		return saveGGOBossState(tx, row, current, boss, currentHP)
	}); err != nil {
		t.Fatal(err)
	}
	killerID, killerCtx := ggoBossLines86Peer(t, f, ctx, event, 1)
	sameEventGiveFormation(t, f.s, &sameEventPlayer{id: killerID, ctx: killerCtx}, template, power)
	var killerBefore db.Profile
	if err = db.GetIns().First(&killerBefore, killerID).Error; err != nil {
		t.Fatal(err)
	}
	beforePlayer, beforeGame, err := decodeProfile(&killerBefore)
	if err != nil {
		t.Fatal(err)
	}
	messages, err := f.s.fieldBossFightStart(killerCtx, killerID, &pb.CSSidePveBossFightStart{Uuid: proto.String(event.ID)})
	if err != nil {
		t.Fatal("killer native start", err)
	}
	killerStart := messages[0].(*pb.SCSidePveBossFightStart)
	var inBattle db.Profile
	if err = db.GetIns().First(&inBattle, killerID).Error; err != nil {
		t.Fatal(err)
	}
	_, battleGame, err := decodeProfile(&inBattle)
	if err != nil {
		t.Fatal(err)
	}
	input, err := fieldBossFrozenInput(battleGame.Field.Fight)
	if err != nil {
		t.Fatal(err)
	}
	terminal, _, err := f.s.ComputeP6GenericMechanismBattle(killerCtx, input, nil)
	if err != nil {
		t.Fatal(err)
	}
	actorDamage, err := ggoBossActorDamage(battleGame.Field.Fight, input, terminal)
	if err != nil || actorDamage[leader] != 1 {
		t.Fatal("original engine did not defeat isolated 1-HP boss", actorDamage[leader], err)
	}
	kill := &pb.CSSidePveBossFightEnd{FightId: killerStart.FightId, Uuid: proto.String(event.ID), Result: proto.Int32(1), OptRecord: terminal.OriginalOperates}
	if _, err = f.s.fieldBossFightEnd(killerCtx, killerID, kill); err != nil {
		t.Fatal("native boss-kill End", err)
	}
	var killerAfter db.Profile
	if err = db.GetIns().First(&killerAfter, killerID).Error; err != nil {
		t.Fatal(err)
	}
	afterPlayer, afterGame, err := decodeProfile(&killerAfter)
	if err != nil || afterPlayer.GetDiamond()-beforePlayer.GetDiamond() < 1500 || afterPlayer.GetCoin()-beforePlayer.GetCoin() < 8000000 || afterGame.GGOField.Chips[2]-beforeGame.GGOField.Chips[2] < 1 {
		t.Fatal("actual native death did not deliver participant resources/chips", err)
	}
	view, err := f.s.GGOFieldBossReceipt(killerCtx, model.GGOFieldBossReceiptRequest{GGOFieldAuth: model.GGOFieldAuth{PlayerID: killerID, SessionHex: hex.EncodeToString(killerAfter.Session)}, FightID: killerStart.GetFightId()}, killerAfter.Session)
	if err != nil || view.RewardStatus != "delivered" || view.RewardChips == nil || len(view.RewardResources) == 0 {
		t.Fatal("killer delivery receipt", view, err)
	}
	for _, p := range players {
		var member db.FieldBossContribution73
		if err = db.GetIns().Where("server_id=? AND spawn_uuid=? AND player_id=?", f.s.OriginalServerID(), event.ID, p.id).Take(&member).Error; err != nil {
			t.Fatal(err)
		}
		if member.Damage <= 0 {
			continue
		}
		if !member.Frozen || member.Claimed {
			t.Fatal("contributor reward before recipient collection", p.id)
		}
		if gain, err := f.s.CollectFieldBossPendingRewardsWithSession(p.ctx, p.id, p.session); err != nil || gain == nil {
			t.Fatal("contributor award collection", p.id, err)
		}
		if err = db.GetIns().First(&member, member.ID).Error; err != nil || !member.Claimed || !member.GGOChipsClaimed {
			t.Fatal("contributor durable delivery", p.id, err)
		}
		if gain, err := f.s.CollectFieldBossPendingRewardsWithSession(p.ctx, p.id, p.session); err != nil || gain != nil {
			t.Fatal("contributor duplicate delivery", p.id, err)
		}
	}
	if _, err = f.s.fieldBossFightEnd(killerCtx, killerID, kill); err != nil {
		t.Fatal("killer native receipt replay", err)
	}
}
