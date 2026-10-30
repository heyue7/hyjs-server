package service

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
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

// This fixture creates the previous shared-boss schema only in a fresh P1
// database, with the desert event and its other frozen columns already present.
func TestGGOBossLines86LegacyMigrationPreservesEvent(t *testing.T) {
	f := p1Setup(t)
	if db.GetIns().Migrator().HasTable(&db.SharedFieldBoss55{}) {
		t.Fatal("legacy migration requires a fresh isolated P1 database")
	}
	ddl := `CREATE TABLE shared_field_boss55 (
		server_id BIGINT NOT NULL, scene VARBINARY(64) NOT NULL, pos_id VARBINARY(64) NOT NULL,
		uuid VARBINARY(128) NOT NULL, payload LONGBLOB NOT NULL, hp LONGBLOB NOT NULL,
		death_rules73 LONGBLOB NULL, ggo_event LONGBLOB NULL,
		respawn_at BIGINT NOT NULL, killer_id BIGINT NOT NULL,
		PRIMARY KEY (server_id, scene, pos_id))`
	if err := db.GetIns().Exec(ddl).Error; err != nil {
		t.Fatal(err)
	}
	now := time.Now().UnixMilli()
	cfg := ggoLineTestConfiguration(t)
	event := ggoBossEvent{Source: constUtil.GGOFieldBossSource, ID: "ggo-lines86-legacy", StartsAtMS: now - 1000, EndsAtMS: now + 3600000, Boss: cfg.Runtime.Boss}
	if event.Boss.HP < 2 {
		t.Fatal("invalid isolated boss HP fixture")
	}
	boss := &pb.SideBoss{Uuid: proto.String(event.ID), State: proto.Int32(1), CurHp: proto.Float64(0.5)}
	eventRaw, err := json.Marshal(event)
	if err != nil {
		t.Fatal(err)
	}
	payload, err := json.Marshal(boss)
	if err != nil {
		t.Fatal(err)
	}
	hp, err := json.Marshal(map[string]float64{ggoBossGUID(event.ID): float64(event.Boss.HP) / 2})
	if err != nil {
		t.Fatal(err)
	}
	rules := []byte(`{"id":"legacy-frozen-rules"}`)
	insert := `INSERT INTO shared_field_boss55
		(server_id,scene,pos_id,uuid,payload,hp,death_rules73,ggo_event,respawn_at,killer_id)
		VALUES (?,?,?,?,?,?,?,?,?,?)`
	if err = db.GetIns().Exec(insert, f.s.OriginalServerID(), "", "", "migration55", []byte(`{}`), []byte(`{}`), nil, nil, 0, 0).Error; err != nil {
		t.Fatal(err)
	}
	if err = db.GetIns().Exec(insert, f.s.OriginalServerID(), constUtil.GGOFieldScene, constUtil.GGOFieldBossPosition, event.ID, payload, hp, rules, eventRaw, 0, 0).Error; err != nil {
		t.Fatal(err)
	}
	check := func() {
		t.Helper()
		var row db.SharedFieldBoss55
		if err := sharedFieldBoss55Query(db.GetIns(), f.s.OriginalServerID(), constUtil.GGOFieldScene, constUtil.GGOFieldBossPosition, constUtil.GGOFieldBossStorageLine).Take(&row).Error; err != nil {
			t.Fatal(err)
		}
		if row.Line != constUtil.GGOFieldBossStorageLine || row.UUID != event.ID || row.RespawnAt != 0 || row.KillerID != 0 ||
			!bytes.Equal(row.GGOEvent, eventRaw) || !bytes.Equal(row.Payload, payload) || !bytes.Equal(row.HP, hp) || !bytes.Equal(row.DeathRules73, rules) {
			t.Fatal("migration changed the frozen desert boss row")
		}
		decoded, decodedBoss, decodedHP, err := ggoBossDecode(&row)
		if err != nil || decoded.ID != event.ID || decodedBoss.GetUuid() != event.ID || decodedHP[ggoBossGUID(event.ID)] != float64(event.Boss.HP)/2 {
			t.Fatal("migrated event cannot be decoded", err)
		}
		var count int64
		if err := db.GetIns().Model(&db.SharedFieldBoss55{}).Where("server_id=? AND scene=? AND pos_id=?", f.s.OriginalServerID(), constUtil.GGOFieldScene, constUtil.GGOFieldBossPosition).Count(&count).Error; err != nil || count != 1 {
			t.Fatal("migration duplicated the realm-wide event", count, err)
		}
	}
	for i := 0; i < 2; i++ {
		if err = f.s.InitializeSharedFieldBoss55(f.ctx); err != nil {
			t.Fatal("repeatable shared boss migration", i, err)
		}
		check()
	}
	f.restart(t)
	if err = f.s.InitializeSharedFieldBoss55(f.ctx); err != nil {
		t.Fatal("cold shared boss migration", err)
	}
	check()
}

// The second role is a finite isolated fixture; real native Boss Start/End
// still determines damage and receipts. No player or server state is shared
// outside the P1 test database.
func ggoBossLines86Peer(t *testing.T, f *p1Fixture, ctx context.Context, event *ggoBossEvent, line int32) (int64, context.Context) {
	t.Helper()
	username := fmt.Sprintf("ggo-lines86-%d", time.Now().UnixNano())
	password := "isolated-test-password"
	registered, err := RegisterGameAccount(ctx, username, password, "沙漠线路验证")
	if err != nil || registered == nil || !registered.Registered {
		t.Fatal("register isolated peer account", err)
	}
	account, err := LoginAccount(ctx, registered.Username, password)
	if err != nil {
		t.Fatal("login isolated peer account", err)
	}
	if account.Id == f.id || account.AccountID <= 0 {
		t.Fatal("peer must use a distinct registered account")
	}
	if _, err = CreatePlayer(ctx, account.Id, "沙漠线路验证", "hero_043"); err != nil {
		t.Fatal(err)
	}
	point, err := ggoWirePoint(event.Position)
	if err != nil {
		t.Fatal(err)
	}
	values, err := ggoChipValues(f.s.GGOFieldConfig)
	if err != nil {
		t.Fatal(err)
	}
	if err = updateProfile(ctx, account.Id, func(player *pb.Player, game *model.GameState) error {
		player.Level = proto.Int32(40)
		game.Position = &pb.Position{PlayerId: proto.Int64(account.Id), Scene: proto.String(constUtil.GGOFieldScene), Current: point}
		game.PositionLoading = false
		game.Field = &model.FieldState{Scene: constUtil.GGOFieldScene, Side: &pb.PlayerSide{PlayerId: proto.Int64(account.Id), State: proto.Int32(0), CurHp: proto.Float64(1), Anger: proto.Float64(0)}}
		game.GGOField = &model.GGOFieldState{EconomyVersion: f.s.GGOFieldConfig.Runtime.EconomyVersion, ExchangeValues: values}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	var profile db.Profile
	if err = db.GetIns().First(&profile, account.Id).Error; err != nil {
		t.Fatal(err)
	}
	session := append([]byte(nil), profile.Session...)
	if len(session) != 16 {
		t.Fatal("fixture session has the wrong length")
	}
	if err = db.GetIns().Save(&db.ClientUpdateSession{PlayerID: account.Id, SessionHash: clientUpdateSessionHash(session), BuildCode: f.s.GGOFieldConfig.Runtime.MinClientBuild}).Error; err != nil {
		t.Fatal(err)
	}
	frame, err := proto.Marshal(&pb.MovePlayer{Info: &pb.MovePlayerInfo{PlayerId: proto.Int64(account.Id)}})
	if err != nil {
		t.Fatal(err)
	}
	if err = db.GetIns().Save(&db.P10ScenePresence{PlayerID: account.Id, Scene: constUtil.GGOFieldScene, Line: line, SessionHash: p6Digest(session), Frame: frame}).Error; err != nil {
		t.Fatal(err)
	}
	key := p10ClientKey{f.s.p10ClientOwner(), account.Id}
	sink := &p10ClientSink{session: session, send: func(proto.Message) error { return nil }}
	p10ClientSinks.Store(key, sink)
	t.Cleanup(func() { p10ClientSinks.CompareAndDelete(key, sink) })
	return account.Id, context.WithValue(ctx, ggoNativeSessionKey{}, session)
}

func ggoBossLines86EndRequest(t *testing.T, f *p1Fixture, ctx context.Context, id int64, eventID string, fullHP int64) *pb.CSSidePveBossFightEnd {
	t.Helper()
	replies, err := f.s.fieldBossFightStart(ctx, id, &pb.CSSidePveBossFightStart{Uuid: proto.String(eventID)})
	if err != nil {
		t.Fatal("native desert boss Start", id, err)
	}
	start := replies[0].(*pb.SCSidePveBossFightStart)
	if len(start.BattleMonsterList) != 1 || len(start.BattleMonsterList[0].Formation) != 1 || start.BattleMonsterList[0].Formation[0].GetInst().GetCurHp() != float64(fullHP) {
		t.Fatal("native Start must freeze the full boss", id)
	}
	var profile db.Profile
	if err = db.GetIns().First(&profile, id).Error; err != nil {
		t.Fatal(err)
	}
	_, game, err := decodeProfile(&profile)
	if err != nil {
		t.Fatal(err)
	}
	input, err := fieldBossFrozenInput(game.Field.Fight)
	if err != nil {
		t.Fatal(err)
	}
	terminal, _, err := f.s.ComputeP6GenericMechanismBattle(ctx, input, nil)
	if err != nil || terminal == nil || terminal.Outcome == nil || terminal.OriginalOperates == nil {
		t.Fatal("original type12 battle calculation", id, err)
	}
	result := int32(0)
	if terminal.Outcome.Winner == 0 {
		result = 1
	}
	return &pb.CSSidePveBossFightEnd{FightId: start.FightId, Uuid: proto.String(eventID), Result: proto.Int32(result), OptRecord: terminal.OriginalOperates}
}

func TestGGOBossLines86IndependentSharedWithinLineAndColdReceipts(t *testing.T) {
	f, firstCtx, _, firstEvent := ggoBossIntegrationFixture(t)
	peerID, peerCtx := ggoBossLines86Peer(t, f, firstCtx, firstEvent, 1)
	otherID, otherCtx := ggoBossLines86Peer(t, f, firstCtx, firstEvent, 2)
	var secondEvent *ggoBossEvent
	if err := db.GetIns().Transaction(func(tx *gorm.DB) error {
		_, event, _, _, err := f.s.ggoBossCurrent(tx, f.s.GGOFieldConfig, firstEvent.StartsAtMS+1, false, 2)
		secondEvent = event
		return err
	}); err != nil {
		t.Fatal("create second line event", err)
	}
	if firstEvent.ID == secondEvent.ID || firstEvent.Line != 1 || secondEvent.Line != 2 {
		t.Fatal("events must be distinct and independent by line", firstEvent, secondEvent)
	}
	players := []struct {
		id    int64
		ctx   context.Context
		line  int32
		event *ggoBossEvent
	}{{f.id, firstCtx, 1, firstEvent}, {peerID, peerCtx, 1, firstEvent}, {otherID, otherCtx, 2, secondEvent}}
	for _, player := range players {
		var presence db.P10ScenePresence
		if err := db.GetIns().First(&presence, "player_id=?", player.id).Error; err != nil || presence.Scene != constUtil.GGOFieldScene || presence.Line != player.line {
			t.Fatal("isolated presence line", player.id, presence.Line, err)
		}
		check, err := f.s.fieldBossCheck(player.ctx, player.id, &pb.CSSideBossCheck{Uuid: proto.String(player.event.ID)})
		if err != nil || len(check) != 1 {
			t.Fatal("native boss check", player.id, err)
		}
		boss := check[0].(*pb.SCSideBossCheck)
		if boss.GetCode() != 1 || boss.GetMonster().GetUuid() != player.event.ID || boss.GetMonster().GetCurHp() != 1 {
			t.Fatal("player did not see its own line's full event", player.id, boss)
		}
		foreign := firstEvent.ID
		if player.line == 1 {
			foreign = secondEvent.ID
		}
		check, err = f.s.fieldBossCheck(player.ctx, player.id, &pb.CSSideBossCheck{Uuid: proto.String(foreign)})
		if err != nil || len(check) != 1 || check[0].(*pb.SCSideBossCheck).GetCode() != 2003 {
			t.Fatal("foreign line event visible to native Check", player.id, err)
		}
		if _, err = f.s.fieldBossFightStart(player.ctx, player.id, &pb.CSSidePveBossFightStart{Uuid: proto.String(foreign)}); err == nil {
			t.Fatal("foreign line event accepted for Start", player.id)
		}
	}

	// Both line-one fights freeze the same full HP; line two freezes its own.
	requests := make([]*pb.CSSidePveBossFightEnd, len(players))
	for i, player := range players {
		requests[i] = ggoBossLines86EndRequest(t, f, player.ctx, player.id, player.event.ID, player.event.Boss.HP)
		var profile db.Profile
		if err := db.GetIns().First(&profile, player.id).Error; err != nil {
			t.Fatal(err)
		}
		_, game, err := decodeProfile(&profile)
		if err != nil || game.Field.Fight.GGOBoss.Line != int64(player.line) {
			t.Fatal("accepted fight did not freeze presence line", player.id, err)
		}
	}
	// A line change cannot redirect the already accepted battle or its replay.
	if err := db.GetIns().Model(&db.P10ScenePresence{}).Where("player_id=?", f.id).Update("line", 2).Error; err != nil {
		t.Fatal(err)
	}
	retry, err := f.s.fieldBossFightStart(firstCtx, f.id, &pb.CSSidePveBossFightStart{Uuid: proto.String(firstEvent.ID)})
	if err != nil || retry[0].(*pb.SCSidePveBossFightStart).GetFightId() != requests[0].GetFightId() {
		t.Fatal("accepted Start changed after switching line", err)
	}
	type endResult struct {
		messages []proto.Message
		err      error
	}
	results := make([]endResult, len(players))
	if db.GetIns().Dialector.Name() == "mysql" {
		done := make(chan int, len(players))
		for i := range players {
			go func(index int) {
				results[index].messages, results[index].err = f.s.fieldBossFightEnd(players[index].ctx, players[index].id, requests[index])
				done <- index
			}(i)
		}
		for range players {
			<-done
		}
	} else {
		for i, player := range players {
			results[i].messages, results[i].err = f.s.fieldBossFightEnd(player.ctx, player.id, requests[i])
		}
	}
	for i, result := range results {
		if result.err != nil || len(result.messages) == 0 || result.messages[0].(*pb.SCSidePveBossFightEnd).GetCode() != 1 {
			t.Fatal("native boss End", players[i].id, result.err)
		}
	}
	var before []db.SharedFieldBoss55
	for line, event := range map[int64]*ggoBossEvent{1: firstEvent, 2: secondEvent} {
		var row db.SharedFieldBoss55
		if err := sharedFieldBoss55Query(db.GetIns(), f.s.OriginalServerID(), constUtil.GGOFieldScene, constUtil.GGOFieldBossPosition, line).Take(&row).Error; err != nil {
			t.Fatal(err)
		}
		_, _, hp, err := ggoBossDecode(&row)
		if err != nil {
			t.Fatal(err)
		}
		remaining := hp[ggoBossGUID(event.ID)]
		if remaining <= 0 || remaining >= float64(event.Boss.HP) {
			t.Fatal("line needs actual partial damage", line, remaining)
		}
		var contributions []db.FieldBossContribution73
		if err = db.GetIns().Where("server_id=? AND spawn_uuid=?", f.s.OriginalServerID(), event.ID).Find(&contributions).Error; err != nil {
			t.Fatal(err)
		}
		want := 1
		if line == 1 {
			want = 2
		}
		if len(contributions) != want {
			t.Fatal("contributions crossed event boundary", line, len(contributions))
		}
		var damage float64
		for _, contribution := range contributions {
			if contribution.Damage <= 0 || contribution.Frozen {
				t.Fatal("invalid partial contribution", line, contribution.PlayerID, contribution.Damage)
			}
			damage += contribution.Damage
		}
		if math.Abs(float64(event.Boss.HP)-remaining-damage) > 0.000001 {
			t.Fatal("line HP and contribution diverged", line, remaining, damage)
		}
		before = append(before, row)
		var view *model.GGOFieldBossView
		if err = db.GetIns().Transaction(func(tx *gorm.DB) error {
			view, err = f.s.ggoBossSnapshotLocked(tx, f.s.GGOFieldConfig, time.Now().UnixMilli(), line)
			return err
		}); err != nil || view.EventID != event.ID || view.HP != int64(math.Ceil(remaining)) {
			t.Fatal("line snapshot did not use its event", line, view, err)
		}
	}
	var eventRows int64
	if err = db.GetIns().Model(&db.SharedFieldBoss55{}).Where("server_id=? AND scene=? AND pos_id=?", f.s.OriginalServerID(), constUtil.GGOFieldScene, constUtil.GGOFieldBossPosition).Count(&eventRows).Error; err != nil || eventRows != 2 {
		t.Fatal("each line needs its own event row", eventRows, err)
	}
	// The changed player now sees line two; a same-line peer still sees line one.
	for _, item := range []struct {
		id    int64
		ctx   context.Context
		event *ggoBossEvent
	}{{f.id, firstCtx, secondEvent}, {peerID, peerCtx, firstEvent}, {otherID, otherCtx, secondEvent}} {
		check, checkErr := f.s.fieldBossCheck(item.ctx, item.id, &pb.CSSideBossCheck{Uuid: proto.String(item.event.ID)})
		if checkErr != nil || len(check) != 1 || check[0].(*pb.SCSideBossCheck).GetCode() != 1 {
			t.Fatal("current line check", item.id, checkErr)
		}
	}
	for i, player := range players {
		replay, replayErr := f.s.fieldBossFightEnd(player.ctx, player.id, requests[i])
		if replayErr != nil || len(replay) == 0 || !proto.Equal(replay[0], results[i].messages[0]) {
			t.Fatal("same native End must replay the receipt", player.id, replayErr)
		}
	}
	f.restart(t)
	for i, player := range players {
		replay, replayErr := f.s.fieldBossFightEnd(player.ctx, player.id, requests[i])
		if replayErr != nil || len(replay) == 0 || !proto.Equal(replay[0], results[i].messages[0]) {
			t.Fatal("cold native End must replay the receipt", player.id, replayErr)
		}
	}
	for _, row := range before {
		var after db.SharedFieldBoss55
		if err = sharedFieldBoss55Query(db.GetIns(), f.s.OriginalServerID(), constUtil.GGOFieldScene, constUtil.GGOFieldBossPosition, row.Line).Take(&after).Error; err != nil || !bytes.Equal(after.HP, row.HP) || !bytes.Equal(after.GGOEvent, row.GGOEvent) {
			t.Fatal("cold replay changed line event", row.Line, err)
		}
	}
	game := &model.GameState{Field: &model.FieldState{Scene: constUtil.GGOFieldScene, BossLine85: 2, Bosses: []*pb.SideBoss{{Uuid: proto.String(secondEvent.ID)}}}}
	if err = db.GetIns().Transaction(func(tx *gorm.DB) error { return f.s.refreshSharedFieldBosses55(tx, otherID, game) }); err != nil {
		t.Fatal("ordinary projector entered desert", err)
	}
	if len(game.Field.Bosses) != 1 || game.Field.Bosses[0].GetUuid() != secondEvent.ID || game.Field.BossLine85 != 2 {
		t.Fatal("ordinary projector rewrote desert projection")
	}
}

// A saved pre-line event and an already accepted battle remain on line one,
// even when the reconnected player's new presence has moved to line two.
func TestGGOBossLines86LegacyInFlightUsesLineOne(t *testing.T) {
	f, ctx, _, current := ggoBossIntegrationFixture(t)
	legacy := *current
	legacy.ID = "ggo-lines86-inflight-legacy"
	legacy.Line = 0
	if err := db.GetIns().Transaction(func(tx *gorm.DB) error {
		var row db.SharedFieldBoss55
		if err := sharedFieldBoss55Query(tx, f.s.OriginalServerID(), constUtil.GGOFieldScene, constUtil.GGOFieldBossPosition, 1).Take(&row).Error; err != nil {
			return err
		}
		_, boss, _, err := ggoBossDecode(&row)
		if err != nil {
			return err
		}
		row.UUID = legacy.ID
		row.GGOEvent, err = json.Marshal(&legacy)
		if err != nil {
			return err
		}
		boss.Uuid = proto.String(legacy.ID)
		return saveSharedFieldBoss55(tx, &row, boss, map[string]float64{ggoBossGUID(legacy.ID): float64(legacy.Boss.HP)})
	}); err != nil {
		t.Fatal("install legacy event", err)
	}
	request := ggoBossLines86EndRequest(t, f, ctx, f.id, legacy.ID, legacy.Boss.HP)
	if err := updateProfile(ctx, f.id, func(_ *pb.Player, game *model.GameState) error {
		game.Field.Fight.GGOBoss.Line = 0 // The old serialized fight had no line field.
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if err := db.GetIns().Model(&db.P10ScenePresence{}).Where("player_id=?", f.id).Update("line", 2).Error; err != nil {
		t.Fatal(err)
	}
	var second *ggoBossEvent
	if err := db.GetIns().Transaction(func(tx *gorm.DB) error {
		_, event, _, _, err := f.s.ggoBossCurrent(tx, f.s.GGOFieldConfig, legacy.StartsAtMS+1, false, 2)
		second = event
		return err
	}); err != nil || second == nil || second.ID == legacy.ID {
		t.Fatal("create distinct line two after legacy event", err)
	}
	f.restart(t)
	ended, err := f.s.fieldBossFightEnd(ctx, f.id, request)
	if err != nil || len(ended) == 0 || ended[0].(*pb.SCSidePveBossFightEnd).GetCode() != 1 {
		t.Fatal("cold legacy in-flight End", err)
	}
	for line, event := range map[int64]*ggoBossEvent{1: &legacy, 2: second} {
		var row db.SharedFieldBoss55
		if err = sharedFieldBoss55Query(db.GetIns(), f.s.OriginalServerID(), constUtil.GGOFieldScene, constUtil.GGOFieldBossPosition, line).Take(&row).Error; err != nil {
			t.Fatal(err)
		}
		decoded, _, hp, err := ggoBossDecode(&row)
		if err != nil || decoded.ID != event.ID {
			t.Fatal("legacy or independent event changed", line, err)
		}
		value := hp[ggoBossGUID(event.ID)]
		if line == 1 && (value <= 0 || value >= float64(event.Boss.HP)) || line == 2 && value != float64(event.Boss.HP) {
			t.Fatal("legacy damage used the wrong line", line, value)
		}
	}
	replay, err := f.s.fieldBossFightEnd(ctx, f.id, request)
	if err != nil || len(replay) == 0 || !proto.Equal(ended[0], replay[0]) {
		t.Fatal("legacy in-flight receipt retry", err)
	}
}

func TestGGOBossLines86IdentityAndLegacyDecode(t *testing.T) {
	first := ggoBossEventID(1, 1, 123456)
	if first != ggoBossEventID(1, 1, 123456) || first == ggoBossEventID(1, 2, 123456) || first == ggoBossEventID(2, 1, 123456) || first == ggoBossEventID(1, 1, 123457) {
		t.Fatal("event identity does not isolate server, line and window")
	}
	event := ggoBossEvent{Source: constUtil.GGOFieldBossSource, ID: first, Line: 1, StartsAtMS: 1, EndsAtMS: 2, Boss: model.AdminFieldGGOBoss{HP: 100}}
	raw, _ := json.Marshal(event)
	boss, _ := json.Marshal(&pb.SideBoss{Uuid: proto.String(event.ID)})
	hp, _ := json.Marshal(map[string]float64{ggoBossGUID(event.ID): 100})
	row := &db.SharedFieldBoss55{Line: 1, UUID: event.ID, GGOEvent: raw, Payload: boss, HP: hp}
	if _, _, _, err := ggoBossDecode(row); err != nil {
		t.Fatal("valid line event", err)
	}
	row.Line = 2
	if _, _, _, err := ggoBossDecode(row); err == nil {
		t.Fatal("a different line's frozen event was accepted")
	}
	event.Line = 0
	row.GGOEvent, _ = json.Marshal(event)
	if _, _, _, err := ggoBossDecode(row); err == nil {
		t.Fatal("a legacy event was accepted on line two")
	}
	row.Line = 1
	if _, _, _, err := ggoBossDecode(row); err != nil {
		t.Fatal("legacy line-one event rejected", err)
	}
	for _, tc := range []struct {
		frozen, want int64
		valid        bool
	}{{0, 1, true}, {1, 1, true}, {2, 2, true}, {-1, 0, false}, {int64(math.MaxInt32) + 1, 0, false}} {
		line, err := ggoBossFrozenLine(&model.GGOFieldBossFight{Line: tc.frozen})
		if (err == nil) != tc.valid || line != tc.want {
			t.Fatal("frozen line", tc, line, err)
		}
	}
}

// Concurrent last hits serialize on the line's row; a kill freezes only that
// event's contributions, rank and tail rewards.
func TestGGOBossLines86LastHitAndAwardsIndependent(t *testing.T) {
	f, ctx, _, first := ggoBossIntegrationFixture(t)
	peerID, _ := ggoBossLines86Peer(t, f, ctx, first, 1)
	var second *ggoBossEvent
	if err := db.GetIns().Transaction(func(tx *gorm.DB) error {
		_, event, _, _, err := f.s.ggoBossCurrent(tx, f.s.GGOFieldConfig, first.StartsAtMS+1, false, 2)
		second = event
		return err
	}); err != nil {
		t.Fatal(err)
	}
	events := map[int64]*ggoBossEvent{1: first, 2: second}
	for line, event := range events {
		if err := db.GetIns().Model(&db.SharedFieldBoss55{}).Where("server_id=? AND scene=? AND line=? AND pos_id=?", f.s.OriginalServerID(), constUtil.GGOFieldScene, line, constUtil.GGOFieldBossPosition).Updates(map[string]any{"hp": adminJSON(map[string]float64{ggoBossGUID(event.ID): 100})}).Error; err != nil {
			t.Fatal(err)
		}
		rules, _ := json.Marshal(event)
		members := []db.FieldBossContribution73{
			{ServerID: f.s.OriginalServerID(), SpawnUUID: event.ID, PlayerID: f.id, Damage: 1, Rules: rules, DailyLimit: 3},
			{ServerID: f.s.OriginalServerID(), SpawnUUID: event.ID, PlayerID: peerID, Damage: float64(event.Boss.HP) - 101, Rules: rules, DailyLimit: 3},
		}
		if err := db.GetIns().Create(&members).Error; err != nil {
			t.Fatal(err)
		}
	}
	type outcome struct {
		damage float64
		err    error
	}
	apply := func(line, id int64) outcome {
		event := events[line]
		monster, err := f.s.ggoBossMonster(event, 100)
		if err != nil {
			return outcome{err: err}
		}
		fight := &model.FieldFight{GGOBoss: &model.GGOFieldBossFight{Line: line, EventID: event.ID, EndsAtMS: event.EndsAtMS, Boss: &pb.SideBoss{Uuid: proto.String(event.ID)}}}
		input := &model.FrozenBattleInput{Monster: monster}
		terminal := &model.CombatFinalHPOutcome{FinalActors: []model.CombatFinalActorHP{{Camp: 1, GUID: ggoBossGUID(event.ID), CurrentHP: 0, MaxHP: float64(event.Boss.HP)}}}
		var damage float64
		err = db.GetIns().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			_, damage, err = f.s.ggoBossApplyDamageLocked(tx, id, fight, input, terminal, time.Now().UnixMilli())
			return err
		})
		return outcome{damage: damage, err: err}
	}
	var results [2]outcome
	if db.GetIns().Dialector.Name() == "mysql" {
		done := make(chan int, 2)
		for i, id := range []int64{f.id, peerID} {
			go func(index int, playerID int64) { results[index] = apply(1, playerID); done <- index }(i, id)
		}
		<-done
		<-done
	} else {
		results[0], results[1] = apply(1, f.id), apply(1, peerID)
	}
	if results[0].err != nil || results[1].err != nil || results[0].damage+results[1].damage != 100 || (results[0].damage != 0 && results[1].damage != 0) {
		t.Fatal("one last hit must own the remaining HP", results)
	}
	var secondRow db.SharedFieldBoss55
	if err := sharedFieldBoss55Query(db.GetIns(), f.s.OriginalServerID(), constUtil.GGOFieldScene, constUtil.GGOFieldBossPosition, 2).Take(&secondRow).Error; err != nil {
		t.Fatal(err)
	}
	_, _, hp, err := ggoBossDecode(&secondRow)
	if err != nil || hp[ggoBossGUID(second.ID)] != 100 || secondRow.KillerID != 0 {
		t.Fatal("first line kill changed second line", secondRow, err)
	}
	var frozen int64
	if err = db.GetIns().Model(&db.FieldBossContribution73{}).Where("spawn_uuid=? AND frozen=?", second.ID, true).Count(&frozen).Error; err != nil || frozen != 0 {
		t.Fatal("first line kill froze second line awards", frozen, err)
	}
	if result := apply(2, f.id); result.err != nil || result.damage != 100 {
		t.Fatal("second line last hit", result)
	}
	for line, event := range events {
		var row db.SharedFieldBoss55
		if err = sharedFieldBoss55Query(db.GetIns(), f.s.OriginalServerID(), constUtil.GGOFieldScene, constUtil.GGOFieldBossPosition, line).Take(&row).Error; err != nil || row.KillerID == 0 {
			t.Fatal("missing killer on line", line, err)
		}
		var members []db.FieldBossContribution73
		if err = db.GetIns().Where("server_id=? AND spawn_uuid=?", f.s.OriginalServerID(), event.ID).Order("reward_rank").Find(&members).Error; err != nil || len(members) != 2 {
			t.Fatal("line members", line, len(members), err)
		}
		if members[0].PlayerID != peerID || members[0].Rank != 1 || members[1].PlayerID != f.id || members[1].Rank != 2 {
			t.Fatal("rank crossed line or did not follow damage", line, members)
		}
		for _, member := range members {
			var award ggoBossAward
			if !member.Frozen || json.Unmarshal(member.GGOChips, &award) != nil {
				t.Fatal("line award was not frozen", line, member.PlayerID)
			}
			base, err := ggoBossChipReward(event, member.Damage, member.Rank, false)
			if err != nil {
				t.Fatal(err)
			}
			var extra int64
			for i := range base {
				extra += award.Chips[i] - base[i]
			}
			want := int64(0)
			if member.PlayerID == row.KillerID {
				want = 1
			}
			if extra != want {
				t.Fatal("tail reward crossed event or duplicated", line, member.PlayerID, extra, want)
			}
		}
	}
}

// An ordinary field victory and desert events on different lines consume the
// same character/day quota; changing lines does not grant additional claims.
func TestGGOBossLines86DailyQuotaSharedAcrossScenesAndLines(t *testing.T) {
	f, ctx, _, event := ggoBossIntegrationFixture(t)
	award := ggoBossAward{EconomyVersion: event.EconomyVersion, ExchangeValues: event.ExchangeValues, Chips: [6]int64{0, 0, 1}}
	raw, err := json.Marshal(award)
	if err != nil {
		t.Fatal(err)
	}
	day := LocalPlayNow(ctx).Format("2006-01-02")
	ids := []string{"ordinary-lines86", ggoBossEventID(f.s.OriginalServerID(), 1, event.StartsAtMS), ggoBossEventID(f.s.OriginalServerID(), 2, event.StartsAtMS), ggoBossEventID(f.s.OriginalServerID(), 2, event.StartsAtMS+1)}
	for i, spawn := range ids {
		member := db.FieldBossContribution73{ServerID: f.s.OriginalServerID(), SpawnUUID: spawn, PlayerID: f.id, Damage: 1, Rules: []byte("{}"), DailyLimit: 3, RewardDay: day, Rank: 1, Frozen: true}
		if i > 0 {
			member.GGOChips = raw
		}
		if err = db.GetIns().Create(&member).Error; err != nil {
			t.Fatal(err)
		}
	}
	if _, err = f.s.CollectFieldBossPendingRewards(ctx, f.id); err != nil {
		t.Fatal("collect daily quota", err)
	}
	_, game := f.profile(t)
	if game.Field.BossRewardClaims != 3 || game.GGOField.Chips[2] != 2 {
		t.Fatal("daily quota reset across scenes or lines", game.Field.BossRewardClaims, game.GGOField.Chips)
	}
	var final db.FieldBossContribution73
	if err = db.GetIns().Where("server_id=? AND spawn_uuid=? AND player_id=?", f.s.OriginalServerID(), ids[3], f.id).Take(&final).Error; err != nil || !final.Claimed || !final.GGOChipsClaimed || final.Rank != 0 {
		t.Fatal("fourth award exceeded daily cap", final, err)
	}
	f.restart(t)
	if _, err = f.s.CollectFieldBossPendingRewards(ctx, f.id); err != nil {
		t.Fatal("cold quota replay", err)
	}
	_, game = f.profile(t)
	if game.Field.BossRewardClaims != 3 || game.GGOField.Chips[2] != 2 {
		t.Fatal("cold replay changed quota or chips", game.Field.BossRewardClaims, game.GGOField.Chips)
	}
}

// Advancing one line's scheduled window never replaces the other line's row,
// and an accepted old fight cannot apply damage to the replacement event.
func TestGGOBossLines86RefreshAndExpiredFight(t *testing.T) {
	f, ctx, _, old1 := ggoBossIntegrationFixture(t)
	var old2 *ggoBossEvent
	if err := db.GetIns().Transaction(func(tx *gorm.DB) error {
		_, event, _, _, err := f.s.ggoBossCurrent(tx, f.s.GGOFieldConfig, old1.StartsAtMS+1, false, 2)
		old2 = event
		return err
	}); err != nil {
		t.Fatal(err)
	}
	var before2 db.SharedFieldBoss55
	if err := sharedFieldBoss55Query(db.GetIns(), f.s.OriginalServerID(), constUtil.GGOFieldScene, constUtil.GGOFieldBossPosition, 2).Take(&before2).Error; err != nil {
		t.Fatal(err)
	}
	nextNow := old1.StartsAtMS + constUtil.GGOFieldBossRespawnMS + 1
	if err := sharedFieldBoss55Query(db.GetIns().Model(&db.SharedFieldBoss55{}), f.s.OriginalServerID(), constUtil.GGOFieldScene, constUtil.GGOFieldBossPosition, 1).
		Updates(map[string]any{"killer_id": f.id, "respawn_at": nextNow}).Error; err != nil {
		t.Fatal(err)
	}
	var next1 *ggoBossEvent
	if err := db.GetIns().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		_, event, _, _, err := f.s.ggoBossCurrent(tx, f.s.GGOFieldConfig, nextNow, false, 1)
		next1 = event
		return err
	}); err != nil || next1 == nil || next1.ID == old1.ID {
		t.Fatal("line one did not advance", err)
	}
	var after2 db.SharedFieldBoss55
	if err := sharedFieldBoss55Query(db.GetIns(), f.s.OriginalServerID(), constUtil.GGOFieldScene, constUtil.GGOFieldBossPosition, 2).Take(&after2).Error; err != nil || after2.UUID != old2.ID || !bytes.Equal(after2.GGOEvent, before2.GGOEvent) || !bytes.Equal(after2.HP, before2.HP) {
		t.Fatal("line one refresh changed line two", err)
	}
	var damage float64
	if err := db.GetIns().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		fight := &model.FieldFight{GGOBoss: &model.GGOFieldBossFight{Line: 1, EventID: old1.ID, EndsAtMS: old1.EndsAtMS, Boss: &pb.SideBoss{Uuid: proto.String(old1.ID)}}}
		_, dealt, err := f.s.ggoBossApplyDamageLocked(tx, f.id, fight, nil, nil, nextNow)
		damage = dealt
		return err
	}); err != nil || damage != 0 {
		t.Fatal("expired battle damaged replacement", damage, err)
	}
	if err := sharedFieldBoss55Query(db.GetIns().Model(&db.SharedFieldBoss55{}), f.s.OriginalServerID(), constUtil.GGOFieldScene, constUtil.GGOFieldBossPosition, 2).
		Updates(map[string]any{"killer_id": f.id, "respawn_at": nextNow}).Error; err != nil {
		t.Fatal(err)
	}
	var next2 *ggoBossEvent
	if err := db.GetIns().Transaction(func(tx *gorm.DB) error {
		_, event, _, _, err := f.s.ggoBossCurrent(tx, f.s.GGOFieldConfig, nextNow, false, 2)
		next2 = event
		return err
	}); err != nil || next2 == nil || next2.ID == old2.ID || next2.ID == next1.ID {
		t.Fatal("line two did not advance independently", err)
	}
	f.restart(t)
	for line, want := range map[int64]*ggoBossEvent{1: next1, 2: next2} {
		if err := db.GetIns().Transaction(func(tx *gorm.DB) error {
			_, event, _, hp, err := f.s.ggoBossCurrent(tx, f.s.GGOFieldConfig, nextNow, false, line)
			if err == nil && (event.ID != want.ID || hp[ggoBossGUID(event.ID)] != float64(event.Boss.HP)) {
				return fmt.Errorf("line %d replacement lost on cold load", line)
			}
			return err
		}); err != nil {
			t.Fatal(err)
		}
	}
}
