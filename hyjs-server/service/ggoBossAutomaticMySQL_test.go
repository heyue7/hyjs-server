package service

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
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

// The two records were emitted by the original Assembly-CSharp automatic
// driver for the synthetic eleventh and twelfth actors. Their full input and
// source hashes are kept outside Git; neither sample represents a phone run.
type ggoAutomaticMySQLCase struct {
	id, seed              int64
	inputFile, inputSHA   string
	recordFile, recordSHA string
	player                sameEventPlayer
	evidence              struct {
		PlayerID int64                     `json:"playerId"`
		Input    *model.FrozenBattleInput  `json:"frozenInput"`
		End      *pb.CSSidePveBossFightEnd `json:"nativeEndRequest"`
	}
	record    *pb.BattleOperate
	request   *pb.CSSidePveBossFightEnd
	inputHash string
	session   []byte
	response  *pb.SCSidePveBossFightEnd
	damage    float64
	settled   bool
}

func ggoAutomaticMySQLRead(t *testing.T, root, name, wantSHA string, maximum int64) []byte {
	t.Helper()
	path := filepath.Join(root, name)
	info, err := os.Stat(path)
	if err != nil || !info.Mode().IsRegular() || info.Size() <= 0 || info.Size() > maximum {
		t.Fatalf("invalid bounded original-engine evidence %s: %v", name, err)
	}
	raw, err := os.ReadFile(path)
	if err != nil || p6Digest(raw) != wantSHA {
		t.Fatalf("original-engine evidence digest mismatch for %s: %v", name, err)
	}
	return raw
}

func ggoAutomaticMySQLLoad(t *testing.T, root string, c *ggoAutomaticMySQLCase) {
	t.Helper()
	raw := ggoAutomaticMySQLRead(t, root, c.inputFile, c.inputSHA, 32<<20)
	if err := json.Unmarshal(raw, &c.evidence); err != nil || c.evidence.PlayerID != c.id || c.evidence.Input == nil || c.evidence.End == nil || c.evidence.Input.Seed != c.seed {
		t.Fatalf("invalid pinned synthetic input for player %d: %v", c.id, err)
	}
	recordRaw := ggoAutomaticMySQLRead(t, root, c.recordFile, c.recordSHA, 1<<20)
	c.record = &pb.BattleOperate{}
	if err := proto.Unmarshal(recordRaw, c.record); err != nil {
		t.Fatalf("invalid original protobuf for player %d: %v", c.id, err)
	}
	canonical, err := (proto.MarshalOptions{Deterministic: true}).Marshal(c.record)
	if err != nil || !bytes.Equal(canonical, recordRaw) || !proto.Equal(c.record, c.evidence.End.GetOptRecord()) || len(c.record.GetSkillOpts()) < 2 {
		t.Fatalf("original protobuf and pinned input disagree for player %d: %v", c.id, err)
	}
}

func ggoAutomaticMySQLAssertState(t *testing.T, f *p1Fixture, event *ggoBossEvent, cases []*ggoAutomaticMySQLCase, wantHP float64) {
	t.Helper()
	hp, _ := sameEventHP(t, f.s.OriginalServerID(), 1, event.ID)
	if math.Abs(hp-wantHP) > 0.00001 {
		t.Fatalf("shared HP changed: got %.12f want %.12f", hp, wantHP)
	}
	for _, c := range cases {
		var members []db.FieldBossContribution73
		if err := db.GetIns().Where("server_id=? AND spawn_uuid=? AND player_id=?", f.s.OriginalServerID(), event.ID, c.id).Find(&members).Error; err != nil || len(members) != 1 || math.Abs(members[0].Damage-c.damage) > 0.00001 {
			t.Fatalf("contribution changed for player %d: rows=%d err=%v", c.id, len(members), err)
		}
		var count int64
		if err := db.GetIns().Model(&db.GGOFieldReceipt{}).Where("player_id=? AND request_id=? AND operation=?", c.id, ggoSystemReceiptID("boss-fight", c.player.start.GetFightId()), constUtil.GGOFieldBossFightOperation).Count(&count).Error; err != nil || count != ggoAutomaticMySQLCount(c.settled) {
			t.Fatalf("receipt count changed for player %d: count=%d err=%v", c.id, count, err)
		}
	}
}

func ggoAutomaticMySQLCount(v bool) int64 {
	if v {
		return 1
	}
	return 0
}

func ggoAutomaticMySQLResumeRequest(c *ggoAutomaticMySQLCase) model.GGOFieldBattleResumeRequest {
	return model.GGOFieldBattleResumeRequest{
		GGOFieldAuth: model.GGOFieldAuth{PlayerID: c.id, SessionHex: hex.EncodeToString(c.session)},
		BattleType:   constUtil.GGOBattleResumeBoss, ExpectedFightID: c.player.start.GetFightId(), ExpectedInputHash: c.inputHash,
	}
}

func ggoAutomaticMySQLReconnect(t *testing.T, f *p1Fixture, c *ggoAutomaticMySQLCase) []byte {
	t.Helper()
	var profile db.Profile
	if err := db.GetIns().First(&profile, c.id).Error; err != nil {
		t.Fatal(err)
	}
	old := append([]byte(nil), c.session...)
	login, err := LoginAccount(c.player.ctx, profile.Uid, "isolated-test-password")
	if err != nil || login == nil || login.Id != c.id || len(login.Session) != 16 || bytes.Equal(old, login.Session) {
		t.Fatalf("rotate isolated session for player %d: %v", c.id, err)
	}
	c.session = append([]byte(nil), login.Session...)
	c.player.ctx = context.WithValue(c.player.ctx, ggoNativeSessionKey{}, c.session)
	// The isolated native fixture supplies the same build and presence bindings
	// that a connected client would establish after login.
	if err = db.GetIns().Save(&db.ClientUpdateSession{PlayerID: c.id, SessionHash: clientUpdateSessionHash(c.session), BuildCode: f.s.GGOFieldConfig.Runtime.MinClientBuild}).Error; err != nil {
		t.Fatal(err)
	}
	if err = db.GetIns().Model(&db.P10ScenePresence{}).Where("player_id=? AND scene=? AND line=?", c.id, constUtil.GGOFieldScene, 1).Update("session_hash", p6Digest(c.session)).Error; err != nil {
		t.Fatal(err)
	}
	key := p10ClientKey{f.s.p10ClientOwner(), c.id}
	sink := &p10ClientSink{session: c.session, send: func(proto.Message) error { return nil }}
	p10ClientSinks.Store(key, sink)
	t.Cleanup(func() { p10ClientSinks.CompareAndDelete(key, sink) })
	return old
}

// Run only against a newly allocated, managed P1 MySQL database. This test
// performs service calls and an authenticated session rotation; its test sink
// is not a socket, mobile client, or MySQL concurrency/load test.
func TestGGOBossAutomaticOriginalReplayMySQL(t *testing.T) {
	if os.Getenv("HYJS_GGO_AUTOMATIC_MYSQL") != "1" {
		t.Skip("requires an explicitly leased fresh P1 MySQL database and original-engine evidence")
	}
	database := os.Getenv("HYJS_P1_MYSQL_DATABASE")
	if !strings.HasPrefix(database, "hyjs_test_p1_") || database == "hyjs_test_p1_desert_v4_qa_28b" || os.Getenv("HYJS_GAME_ACCOUNTS_DATABASE") != "" {
		t.Fatal("requires a fresh isolated P1 database; QA and external accounts databases are forbidden")
	}
	root := os.Getenv("HYJS_GGO_AUTOMATIC_ORACLE_ROOT")
	if !filepath.IsAbs(root) {
		t.Fatal("HYJS_GGO_AUTOMATIC_ORACLE_ROOT must be an absolute read-only evidence directory")
	}
	cases := []*ggoAutomaticMySQLCase{
		{id: 12, seed: 280010, inputFile: "rerun-pb/player-12-failure.json", inputSHA: "2440431bde6016e1a59228b9ee14f069d46f26ef29f2569ceda9e5fdf6d0f6e5", recordFile: "native-attempt-1/original-final.pb", recordSHA: "c367e92c4af7819c2f58554ea5055478d702694e441fe830b2f2c02abf4dc8c2"},
		{id: 13, seed: 280011, inputFile: "compatible-r1/player-13-failure.json", inputSHA: "5478ca65604822ba91730c698e089de565aa5022ce367f63f89e4ed54b053ac7", recordFile: "native-attempt-2/original-final.pb", recordSHA: "20895a190cbb366c0570cfc3cd7cea556ffacfed8ccaa30b58c49fb96d6f1acb"},
	}
	for _, c := range cases {
		ggoAutomaticMySQLLoad(t, root, c)
	}
	f, ctx, _, event := ggoBossIntegrationFixture(t)
	if db.GetIns().Dialector.Name() != "mysql" || f.id != 2 {
		t.Fatal("original fixture requires a fresh MySQL database with first test role ID 2")
	}
	if err := db.GetIns().AutoMigrate(&db.P13Team{}); err != nil {
		t.Fatal(err)
	}
	for _, initialize := range []func(context.Context) error{f.s.InitializeAdminSproutSettings, f.s.InitializeOpenServerActivity, f.s.InitializeDailyTasks} {
		if err := initialize(ctx); err != nil {
			t.Fatal(err)
		}
	}
	f.s.P5Ready = true
	event.ID, event.Line = "ggo-calibration-same-event-line-1", 1
	event.Boss = defaultGGORuntime().Boss
	event.StartsAtMS, event.EndsAtMS = time.Now().Add(-time.Minute).UnixMilli(), time.Now().Add(time.Hour).UnixMilli()
	f.s.GGOFieldConfig.Runtime.Boss = event.Boss
	if err := db.GetIns().Transaction(func(tx *gorm.DB) error {
		var row db.SharedFieldBoss55
		if err := sharedFieldBoss55Query(tx, f.s.OriginalServerID(), constUtil.GGOFieldScene, constUtil.GGOFieldBossPosition, 1).Take(&row).Error; err != nil {
			return err
		}
		_, boss, _, err := ggoBossDecode(&row)
		if err != nil {
			return err
		}
		row.UUID = event.ID
		row.GGOEvent, err = json.Marshal(event)
		if err != nil {
			return err
		}
		boss.Uuid, boss.State, boss.CurHp = proto.String(event.ID), proto.Int32(1), proto.Float64(1)
		return saveSharedFieldBoss55(tx, &row, boss, map[string]float64{ggoBossGUID(event.ID): float64(event.Boss.HP)})
	}); err != nil {
		t.Fatal("install synthetic shared event", err)
	}
	for expectedID := int64(3); expectedID <= 13; expectedID++ {
		id, peerCtx := ggoBossLines86Peer(t, f, ctx, event, 1)
		if id != expectedID {
			t.Fatalf("fresh database identity sequence: got %d want %d", id, expectedID)
		}
		for _, c := range cases {
			if c.id == id {
				c.player = sameEventPlayer{id: id, ctx: peerCtx, team: 3}
			}
		}
	}
	_, formation, power := desertBossCalibrationTeam(t, f.s, []string{"hero_065", "hero_066", "hero_067", "hero_074", "hero_075"})
	for _, c := range cases {
		sameEventGiveFormation(t, f.s, &c.player, formation, power)
		sameEventStart(t, f.s, &c.player, event.ID, c.seed)
		current, err := json.Marshal(c.player.input)
		original, originalErr := json.Marshal(c.evidence.Input)
		if err != nil || originalErr != nil || !bytes.Equal(current, original) {
			t.Fatalf("full accepted input differs from pinned original for player %d: %v / %v", c.id, err, originalErr)
		}
		var profile db.Profile
		if err = db.GetIns().First(&profile, c.id).Error; err != nil {
			t.Fatal(err)
		}
		_, game, err := decodeProfile(&profile)
		if err != nil || game.Field == nil || game.Field.Fight == nil || game.Field.Fight.GGOBoss == nil {
			t.Fatalf("read persisted frozen input for player %d: %v", c.id, err)
		}
		_, c.inputHash, err = ggoResumeFrozenInput(game.Field.Fight)
		if err != nil || c.inputHash != p6Digest(current) || game.Field.Fight.GGOBoss.InputHash != c.inputHash {
			t.Fatalf("frozen input binding for player %d: %v", c.id, err)
		}
		c.session = append([]byte(nil), profile.Session...)
		c.request = &pb.CSSidePveBossFightEnd{FightId: c.player.start.FightId, Uuid: proto.String(event.ID), Result: proto.Int32(c.evidence.End.GetResult()), OptRecord: proto.Clone(c.record).(*pb.BattleOperate)}
	}
	wantHP := float64(event.Boss.HP)
	ggoAutomaticMySQLAssertState(t, f, event, cases, wantHP)
	configuration, err := f.s.adminOriginalField(constUtil.GGOFieldScene)
	if err != nil {
		t.Fatal(err)
	}
	configuration.GGO = f.s.GGOFieldEconomy
	configurationRaw, err := json.Marshal(configuration)
	if err != nil {
		t.Fatal(err)
	}
	if err = db.GetIns().Save(&db.AdminFieldDeployment{ServerID: f.s.OriginalServerID(), Scene: constUtil.GGOFieldScene, TemplateID: "automatic-mysql-test", Enabled: false, Configuration: string(configurationRaw), Revision: 1, UpdatedAt: 1}).Error; err != nil {
		t.Fatal(err)
	}
	// Reopen the pool and issue new real login sessions before recovering either
	// accepted fight. Handshake and presence are supplied by the isolated sink.
	f.restart(t)
	for _, c := range cases {
		old := ggoAutomaticMySQLReconnect(t, f, c)
		request := ggoAutomaticMySQLResumeRequest(c)
		oldRequest := request
		oldRequest.SessionHex = hex.EncodeToString(old)
		if out, err := f.s.GGOFieldBattleResume(c.player.ctx, oldRequest, old); err == nil || out != nil {
			t.Fatal("old session recovered an accepted fight", c.id)
		}
		active, err := f.s.GGOFieldBattleResume(c.player.ctx, request, c.session)
		var start pb.SCSidePveBossFightStart
		if err != nil || active == nil || active.Status != "active" || active.InputHash != c.inputHash || active.EventID != event.ID || proto.Unmarshal(active.NativeStart, &start) != nil || !proto.Equal(&start, c.player.start) {
			t.Fatalf("active recovery after login player=%d: %v", c.id, err)
		}
		wrong := request
		wrong.ExpectedInputHash = strings.Repeat("0", 64)
		if out, err := f.s.GGOFieldBattleResume(c.player.ctx, wrong, c.session); err == nil || out != nil {
			t.Fatal("wrong frozen input hash recovered an accepted fight", c.id)
		}
	}
	for _, c := range cases {
		terminal, _, err := f.s.ComputeP6GenericMechanismBattle(c.player.ctx, c.player.input, nil)
		if err != nil || terminal == nil || terminal.Outcome == nil || terminal.OriginalOperates == nil {
			t.Fatalf("automatic result player=%d: %v", c.id, err)
		}
		record, err := (proto.MarshalOptions{Deterministic: true}).Marshal(terminal.OriginalOperates)
		original, originalErr := (proto.MarshalOptions{Deterministic: true}).Marshal(c.record)
		if err != nil || originalErr != nil || !bytes.Equal(record, original) {
			t.Fatalf("automatic record differs from original assembly player=%d: %v / %v", c.id, err, originalErr)
		}
		_, replayErr := f.s.ComputeOriginalP6ProtocolFinalHP(c.player.ctx, c.player.input, c.record)
		if replayErr == nil || (!strings.HasPrefix(replayErr.Error(), constUtil.P6ReplayConflict+": 原录像终局或完整技能记录不符") && !strings.HasPrefix(replayErr.Error(), "录像操作不符合原技能时序:")) {
			t.Fatalf("sample did not reach the verified exact-automatic fallback for player %d: %v", c.id, replayErr)
		}
		// A corrupted stored input digest must fail before damage or a receipt.
		setStoredHash := func(value string) {
			t.Helper()
			if err := updateProfile(c.player.ctx, c.id, func(_ *pb.Player, g *model.GameState) error {
				if g.Field == nil || g.Field.Fight == nil || g.Field.Fight.BossInputHash == "" {
					return fmt.Errorf("missing stored fight for player %d", c.id)
				}
				g.Field.Fight.BossInputHash = value
				return nil
			}); err != nil {
				t.Fatal(err)
			}
		}
		setStoredHash(strings.Repeat("0", 64))
		if _, err = f.s.fieldBossFightEnd(c.player.ctx, c.id, c.request); err == nil {
			t.Fatal("corrupt frozen input accepted", c.id)
		}
		ggoAutomaticMySQLAssertState(t, f, event, cases, wantHP)
		setStoredHash(c.inputHash)
		badSkill := proto.Clone(c.request).(*pb.CSSidePveBossFightEnd)
		badSkill.OptRecord.SkillOpts[0].SkillIndex = proto.Int32(badSkill.OptRecord.SkillOpts[0].GetSkillIndex() + 1)
		badHP := proto.Clone(c.request).(*pb.CSSidePveBossFightEnd)
		badHP.OptRecord.DefenderTotalLeftHp = proto.Float64(badHP.OptRecord.GetDefenderTotalLeftHp() + 1)
		badResult := proto.Clone(c.request).(*pb.CSSidePveBossFightEnd)
		badResult.Result = proto.Int32(1 - badResult.GetResult())
		badEvent := proto.Clone(c.request).(*pb.CSSidePveBossFightEnd)
		badEvent.Uuid = proto.String(event.ID + "-other")
		other := cases[0]
		if other == c {
			other = cases[1]
		}
		badInput := proto.Clone(c.request).(*pb.CSSidePveBossFightEnd)
		badInput.OptRecord = proto.Clone(other.record).(*pb.BattleOperate)
		bad := map[string]*pb.CSSidePveBossFightEnd{"skill": badSkill, "hp": badHP, "result": badResult, "event": badEvent, "other-input-record": badInput}
		for name, changed := range bad {
			if _, err := f.s.fieldBossFightEnd(c.player.ctx, c.id, changed); err == nil {
				t.Fatalf("modified %s accepted before settlement for player %d", name, c.id)
			}
			ggoAutomaticMySQLAssertState(t, f, event, cases, wantHP)
		}
		var terminalBossHP float64
		var bosses int
		for _, actor := range terminal.FinalActors {
			if actor.Camp == 1 && actor.GUID == c.player.input.Monster.Formation[0].GetGuid() {
				terminalBossHP, bosses = actor.CurrentHP, bosses+1
			}
		}
		if bosses != 1 {
			t.Fatal("original terminal has no unique boss", c.id)
		}
		wantDamage := math.Min(wantHP, math.Max(0, c.player.input.Monster.Formation[0].GetInst().GetCurHp()-terminalBossHP))
		messages, err := f.s.fieldBossFightEnd(c.player.ctx, c.id, c.request)
		if err != nil || len(messages) == 0 {
			t.Fatalf("original automatic End player=%d: %v", c.id, err)
		}
		var ok bool
		c.response, ok = messages[0].(*pb.SCSidePveBossFightEnd)
		if !ok || c.response.GetCode() != 1 {
			t.Fatalf("invalid End response for player %d", c.id)
		}
		c.damage, c.settled = wantDamage, true
		wantHP -= wantDamage
		ggoAutomaticMySQLAssertState(t, f, event, cases, wantHP)
		for name, changed := range bad {
			if _, err := f.s.fieldBossFightEnd(c.player.ctx, c.id, changed); err == nil {
				t.Fatalf("modified %s reused a receipt for player %d", name, c.id)
			}
			ggoAutomaticMySQLAssertState(t, f, event, cases, wantHP)
		}
		t.Logf("pinned original automatic player=%d seed=%d damage=%.3f remaining=%.3f strictPlayback=%q", c.id, c.seed, c.damage, wantHP, replayErr.Error())
	}
	f.restart(t)
	for _, c := range cases {
		resumed, err := f.s.GGOFieldBattleResume(c.player.ctx, ggoAutomaticMySQLResumeRequest(c), c.session)
		var restored pb.SCSidePveBossFightEnd
		if err != nil || resumed == nil || resumed.Status != "settled" || resumed.InputHash != c.inputHash || resumed.EventID != event.ID || proto.Unmarshal(resumed.NativeEnd, &restored) != nil || !proto.Equal(&restored, c.response) {
			t.Fatalf("settled recovery after pool reopen player=%d: %v", c.id, err)
		}
		replayed, err := f.s.fieldBossFightEnd(c.player.ctx, c.id, c.request)
		if err != nil || len(replayed) == 0 || !proto.Equal(replayed[0], c.response) {
			t.Fatalf("replayed End after pool reopen player=%d: %v", c.id, err)
		}
		requestHash, err := ggoBossEndHash(c.request)
		if err != nil {
			t.Fatal(err)
		}
		receipt, err := f.s.GGOFieldBossReceipt(c.player.ctx, model.GGOFieldBossReceiptRequest{GGOFieldAuth: model.GGOFieldAuth{PlayerID: c.id, SessionHex: hex.EncodeToString(c.session)}, FightID: c.player.start.GetFightId()}, c.session)
		if err != nil || receipt == nil || receipt.State != "settled" || receipt.InputHash != c.inputHash || receipt.EndRequestHash != requestHash || math.Abs(receipt.Damage-c.damage) > 0.00001 || !bytes.Equal(receipt.NativeEnd, resumed.NativeEnd) {
			t.Fatalf("authenticated immutable receipt player=%d: %v", c.id, err)
		}
		ggoAutomaticMySQLAssertState(t, f, event, cases, wantHP)
	}
}
