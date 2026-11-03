package service

import (
	"bytes"
	"context"
	cryptorand "crypto/rand"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"math"
	mathrand "math/rand"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
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

type sameEventPlayer struct {
	id    int64
	ctx   context.Context
	power int32
	team  int
	start *pb.SCSidePveBossFightStart
	input *model.FrozenBattleInput
}

func sameEventEvidence(t *testing.T, name string, raw []byte) {
	t.Helper()
	root := os.Getenv("HYJS_GGO_SAME_EVENT_EVIDENCE")
	if root == "" {
		return
	}
	info, err := os.Lstat(root)
	if err != nil || !filepath.IsAbs(root) || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 || info.Mode().Perm()&0077 != 0 || filepath.Base(name) != name {
		t.Fatalf("evidence directory must be an existing private directory: %v", err)
	}
	path := filepath.Join(root, name)
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = file.Write(raw); err != nil {
		_ = file.Close()
		t.Fatal(err)
	}
	if err = file.Close(); err != nil {
		t.Fatal(err)
	}
	t.Logf("private diagnostic evidence: %s", path)
}

func sameEventFailureEvidence(t *testing.T, s *GameService, p *sameEventPlayer, event *ggoBossEvent, before float64, request *pb.CSSidePveBossFightEnd, terminal *model.CombatFinalHPOutcome, endErr error) {
	t.Helper()
	var automaticEngine, replayEngine *combatRoundEngine
	auto, autoErr := s.computeFrozenBattleRecordObserved(p.ctx, p.input, nil, func(engine *combatRoundEngine) { automaticEngine = engine }, nil, true)
	_, replayErr := s.computeFrozenBattleRecordObserved(p.ctx, p.input, request.OptRecord, func(engine *combatRoundEngine) { replayEngine = engine }, nil, true)
	first := map[string]any{}
	var replayEvents []model.CombatEvent
	var replayOperations *pb.BattleOperate
	if automaticEngine != nil && replayEngine != nil {
		replayEvents = replayEngine.events
		replayOperations = replayEngine.operate
		count := min(len(automaticEngine.events), len(replayEvents))
		for i := 0; i < count; i++ {
			if !reflect.DeepEqual(automaticEngine.events[i], replayEvents[i]) {
				first = map[string]any{"index": i, "automatic": automaticEngine.events[i], "playback": replayEvents[i]}
				break
			}
		}
		if len(first) == 0 && len(automaticEngine.events) != len(replayEvents) {
			first = map[string]any{"index": count, "automaticEventCount": len(automaticEngine.events), "playbackEventCount": len(replayEvents)}
		}
	}
	after, _ := sameEventHP(t, s.OriginalServerID(), 1, event.ID)
	if before != after {
		t.Fatal("failed End changed shared HP")
	}
	errorText := func(err error) string {
		if err == nil {
			return ""
		}
		return err.Error()
	}
	startPB, err := (proto.MarshalOptions{Deterministic: true}).Marshal(p.start)
	if err != nil {
		t.Fatal(err)
	}
	endPB, err := (proto.MarshalOptions{Deterministic: true}).Marshal(request)
	if err != nil {
		t.Fatal(err)
	}
	evidence := map[string]any{"scope": "isolated synthetic native request; not a device capture", "team": p.team,
		"playerId": p.id, "actualFormationPower": p.power, "event": event, "beforeHP": before, "afterFailedEndHP": after,
		"nativeStart": p.start, "nativeStartPB": startPB, "frozenInput": p.input, "automaticFinal": terminal, "nativeEndRequest": request, "nativeEndPB": endPB,
		"nativeEndError": errorText(endErr), "observedAutomaticError": errorText(autoErr), "observedPlaybackError": errorText(replayErr),
		"observedAutomatic": auto, "playbackEvents": replayEvents, "playbackOperations": replayOperations, "firstEventDifference": first}
	raw, err := json.MarshalIndent(evidence, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	sameEventEvidence(t, fmt.Sprintf("player-%d-failure.json", p.id), raw)
	if raw, err := json.Marshal(first); err == nil {
		t.Logf("first automatic/playback event difference: %s", raw)
	}
}

// Only the child process runs this fixture. The production Start still obtains
// its seed and fight ID from crypto/rand; a private reader supplies fixed bytes
// for this test so the original engine inputs can be replayed exactly.
func sameEventStart(t *testing.T, s *GameService, p *sameEventPlayer, eventID string, seed int64) {
	t.Helper()
	var first [4]byte
	binary.BigEndian.PutUint32(first[:], uint32(seed))
	previous := cryptorand.Reader
	cryptorand.Reader = io.MultiReader(bytes.NewReader(first[:]), mathrand.New(mathrand.NewSource(seed)))
	defer func() { cryptorand.Reader = previous }()
	messages, err := s.fieldBossFightStart(p.ctx, p.id, &pb.CSSidePveBossFightStart{Uuid: proto.String(eventID)})
	if err != nil || len(messages) == 0 {
		t.Fatalf("native Start player=%d: %v", p.id, err)
	}
	var ok bool
	p.start, ok = messages[0].(*pb.SCSidePveBossFightStart)
	if !ok || p.start.GetCode() != 1 || p.start.GetRandSeed() != seed || p.start.GetMaxround() != 20 {
		t.Fatalf("unexpected native Start player=%d seed=%d", p.id, p.start.GetRandSeed())
	}
	var row db.Profile
	if err := db.GetIns().First(&row, p.id).Error; err != nil {
		t.Fatal(err)
	}
	_, game, err := decodeProfile(&row)
	if err != nil || game.Field == nil || game.Field.Fight == nil || game.Field.Fight.GGOBoss == nil {
		t.Fatalf("load frozen fight player=%d: %v", p.id, err)
	}
	if game.Field.Fight.GGOBoss.Line != 1 || game.Field.Fight.GGOBoss.EventID != eventID {
		t.Fatalf("wrong frozen line or event for player %d", p.id)
	}
	p.input, err = fieldBossFrozenInput(game.Field.Fight)
	if err != nil || p.input.Seed != seed || p.input.MaxRound != 20 || p.input.BattleType != constUtil.FieldBossBattleType {
		t.Fatalf("wrong frozen engine input player=%d: %v", p.id, err)
	}
}

func sameEventHP(t *testing.T, serverID, line int64, eventID string) (float64, db.SharedFieldBoss55) {
	t.Helper()
	var row db.SharedFieldBoss55
	if err := sharedFieldBoss55Query(db.GetIns(), serverID, constUtil.GGOFieldScene, constUtil.GGOFieldBossPosition, line).Take(&row).Error; err != nil {
		t.Fatal(err)
	}
	event, _, hp, err := ggoBossDecode(&row)
	if err != nil || event.ID != eventID {
		t.Fatalf("line %d event changed: %v", line, err)
	}
	return hp[ggoBossGUID(eventID)], row
}

func sameEventGiveFormation(t *testing.T, s *GameService, p *sameEventPlayer, template *model.GameState, power int32) {
	t.Helper()
	err := updateProfile(p.ctx, p.id, func(player *pb.Player, g *model.GameState) error {
		g.Warriors, g.Equipment, g.Treasures, g.Formation = template.Warriors, template.Equipment, template.Treasures, template.Formation
		g.TacticalFormations, g.TacticalSkillId = template.TacticalFormations, template.TacticalSkillId
		player.Level = proto.Int32(100)
		g.Field.Side.BattlePosHps = nil
		g.Field.Side.CurHp, g.Field.Side.Anger = proto.Float64(1), proto.Float64(0)
		actual, err := s.formationFightPower(player, g)
		if err != nil || math.Abs(float64(actual)-500000) > 10000 {
			return fmt.Errorf("original formation power=%d template=%d: %v", actual, power, err)
		}
		player.FightPower = proto.Int32(actual)
		p.power = actual
		return nil
	})
	if err != nil {
		t.Fatalf("install finite formation for player=%d: %v", p.id, err)
	}
}

// 15 real native starts overlap the same live event; their ends are serialized
// against its shared HP row. The five-hero 500k formation belongs to each
// player; three formation templates are reused across three actual teams.
// This test is engine + SQLite evidence, not a mobile or MySQL concurrency test.
func TestGGOBossCalibrationSameEvent(t *testing.T) {
	if os.Getenv("HYJS_GGO_SAME_EVENT_CHILD") != "1" {
		cmd := exec.Command(os.Args[0], "-test.run=^TestGGOBossCalibrationSameEvent$", "-test.v", "-test.timeout=10m")
		for _, value := range os.Environ() {
			if !strings.HasPrefix(value, "HYJS_GGO_SAME_EVENT_CHILD=") && !strings.HasPrefix(value, "HYJS_P1_MYSQL_DATABASE=") && !strings.HasPrefix(value, "HYJS_GAME_ACCOUNTS_DATABASE=") {
				cmd.Env = append(cmd.Env, value)
			}
		}
		cmd.Env = append(cmd.Env, "HYJS_GGO_SAME_EVENT_CHILD=1", "HYJS_P1_MYSQL_DATABASE=", "HYJS_GAME_ACCOUNTS_DATABASE=")
		output, err := cmd.CombinedOutput()
		sameEventEvidence(t, "test.log", output)
		t.Logf("isolated calibration process:\n%s", output)
		if err != nil {
			t.Fatalf("isolated calibration failed: %v", err)
		}
		return
	}
	t.Setenv("HYJS_P1_MYSQL_DATABASE", "")
	t.Setenv("HYJS_GAME_ACCOUNTS_DATABASE", "")
	f, ctx, _, event := ggoBossIntegrationFixture(t)
	if db.GetIns().Dialector.Name() != "sqlite" {
		t.Fatal("calibration requires a private SQLite fixture")
	}
	if err := db.GetIns().AutoMigrate(&db.P13Team{}); err != nil {
		t.Fatal(err)
	}
	if err := f.s.InitializeAdminSproutSettings(ctx); err != nil {
		t.Fatal(err)
	}
	for _, initialize := range []func(context.Context) error{f.s.InitializeOpenServerActivity, f.s.InitializeDailyTasks} {
		if err := initialize(ctx); err != nil {
			t.Fatal(err)
		}
	}
	f.s.P5Ready = true
	// Preserve the published default combat and award rules. A longer synthetic
	// event window only keeps the private test live while 15 inputs are prepared.
	event.ID = "ggo-calibration-same-event-line-1"
	event.Line = 1
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
		t.Fatal("install the fixed same-line event", err)
	}
	var second *ggoBossEvent
	if err := db.GetIns().Transaction(func(tx *gorm.DB) error {
		var err error
		_, second, _, _, err = f.s.ggoBossCurrent(tx, f.s.GGOFieldConfig, time.Now().UnixMilli(), true, 2)
		return err
	}); err != nil || second.ID == event.ID {
		t.Fatal("independent line fixture", err)
	}
	otherHP, _ := sameEventHP(t, f.s.OriginalServerID(), 2, second.ID)
	ids := [][]string{{"hero_001", "hero_002", "hero_003", "hero_004", "hero_005"}, {"hero_006", "hero_009", "hero_015", "hero_017", "hero_043"}, {"hero_065", "hero_066", "hero_067", "hero_074", "hero_075"}}
	players := make([]sameEventPlayer, 15)
	players[0] = sameEventPlayer{id: f.id, ctx: ctx, team: 1}
	contexts := map[int64]context.Context{f.id: ctx}
	for i := 1; i < len(players); i++ {
		id, playerCtx := ggoBossLines86Peer(t, f, ctx, event, 1)
		players[i] = sameEventPlayer{id: id, ctx: playerCtx, team: i/5 + 1}
		contexts[id] = playerCtx
	}
	for team := 0; team < 3; team++ {
		_, template, power := desertBossCalibrationTeam(t, f.s, ids[team])
		teamID := fmt.Sprintf("calibration-team-%d", team+1)
		if err := db.GetIns().Create(&db.P13Team{ID: teamID, LeaderID: players[team*5].id, MaxSize: 5, AuthorityVersion: 1, Revision: 1, CreatedAt: time.Now().UnixMilli()}).Error; err != nil {
			t.Fatal(err)
		}
		for i := team * 5; i < team*5+5; i++ {
			if err := db.GetIns().Create(&db.P13TeamMember{PlayerID: players[i].id, TeamID: teamID, Generation: 1, JoinedAt: time.Now().UnixMilli()}).Error; err != nil {
				t.Fatal(err)
			}
			sameEventGiveFormation(t, f.s, &players[i], template, power)
		}
	}
	for i := range players {
		sameEventStart(t, f.s, &players[i], event.ID, int64(280000+i))
		if players[i].input.Monster.GetFormation()[0].GetInst().GetCurHp() != float64(event.Boss.HP) {
			t.Fatal("all 15 accepted starts must precede the first End")
		}
	}

	// The deterministic P6 output and original 30085 End both use the actual
	// accepted input. End updates one shared row and writes its real receipt.
	fullRounds := 0
	var effectiveTotal float64
	var maxPlayback, maxAutomatic time.Duration
	var playbackCount, automaticCount int
	for i := range players {
		p := &players[i]
		terminal, _, err := f.s.ComputeP6GenericMechanismBattle(p.ctx, p.input, nil)
		if err != nil || terminal == nil || terminal.Outcome == nil || terminal.OriginalOperates == nil {
			t.Fatalf("original engine player=%d: %v", p.id, err)
		}
		if terminal.Outcome.FinalRound == 21 {
			fullRounds++
		}
		result := int32(0)
		if terminal.Outcome.Winner == 0 {
			result = 1
		}
		request := &pb.CSSidePveBossFightEnd{FightId: p.start.FightId, Uuid: proto.String(event.ID), Result: proto.Int32(result), OptRecord: terminal.OriginalOperates}
		probeStart := time.Now()
		playback, playbackErr := f.s.ComputeOriginalP6ProtocolFinalHP(p.ctx, p.input, request.OptRecord)
		probeElapsed := time.Since(probeStart)
		path := "automatic-exact"
		if playbackErr == nil {
			path = "playback"
			if playback == nil || playback.Outcome == nil || !reflect.DeepEqual(playback.FinalActors, terminal.FinalActors) ||
				playback.Outcome.Attacker.GetInst().GetAnger() != terminal.Outcome.Attacker.GetInst().GetAnger() {
				t.Fatalf("original playback and automatic actor HP/anger differ for player %d", p.id)
			}
		}
		var autoBossHP float64
		var bossActors int
		for _, actor := range terminal.FinalActors {
			if actor.Camp == 1 && actor.GUID == p.input.Monster.Formation[0].GetGuid() {
				autoBossHP = actor.CurrentHP
				bossActors++
			}
		}
		if bossActors != 1 {
			t.Fatal("automatic terminal does not contain exactly one matching boss")
		}
		before, _ := sameEventHP(t, f.s.OriginalServerID(), 1, event.ID)
		type rejectedRequest struct {
			name string
			body *pb.CSSidePveBossFightEnd
		}
		var rejectedRequests []rejectedRequest
		if i == 10 {
			if len(request.OptRecord.GetSkillOpts()) < 2 {
				t.Fatal("original eleventh player record has too few skill operations")
			}
			badSkill := proto.Clone(request).(*pb.CSSidePveBossFightEnd)
			badSkill.OptRecord.SkillOpts[0].SkillIndex = proto.Int32(badSkill.OptRecord.SkillOpts[0].GetSkillIndex() + 1)
			badHP := proto.Clone(request).(*pb.CSSidePveBossFightEnd)
			badHP.OptRecord.DefenderTotalLeftHp = proto.Float64(badHP.OptRecord.GetDefenderTotalLeftHp() + 1)
			badResult := proto.Clone(request).(*pb.CSSidePveBossFightEnd)
			badResult.Result = proto.Int32(1 - badResult.GetResult())
			badTime := proto.Clone(request).(*pb.CSSidePveBossFightEnd)
			badTime.OptRecord.SkillOpts[0].RoundTime = proto.Float64(1200)
			badIdentity := proto.Clone(request).(*pb.CSSidePveBossFightEnd)
			badIdentity.OptRecord.SkillOpts[0].ActorUid = proto.String("no-such-player-actor")
			badOrder := proto.Clone(request).(*pb.CSSidePveBossFightEnd)
			last := len(badOrder.OptRecord.SkillOpts) - 1
			badOrder.OptRecord.SkillOpts[0], badOrder.OptRecord.SkillOpts[last] = badOrder.OptRecord.SkillOpts[last], badOrder.OptRecord.SkillOpts[0]
			missing := proto.Clone(request).(*pb.CSSidePveBossFightEnd)
			missing.OptRecord.SkillOpts = missing.OptRecord.SkillOpts[1:]
			added := proto.Clone(request).(*pb.CSSidePveBossFightEnd)
			added.OptRecord.SkillOpts = append(added.OptRecord.SkillOpts, proto.Clone(added.OptRecord.SkillOpts[0]).(*pb.BattleSkillOperate))
			rejectedRequests = []rejectedRequest{{"skill", badSkill}, {"hp", badHP}, {"result", badResult}, {"time", badTime}, {"identity", badIdentity}, {"order", badOrder}, {"missing", missing}, {"added", added}}
		}
		endStart := time.Now()
		messages, err := f.s.fieldBossFightEnd(p.ctx, p.id, request)
		endElapsed := time.Since(endStart)
		if err != nil || len(messages) == 0 || messages[0].(*pb.SCSidePveBossFightEnd).GetCode() != 1 {
			sameEventFailureEvidence(t, f.s, p, event, before, request, terminal, err)
			t.Fatalf("native End player=%d: %v", p.id, err)
		}
		after, _ := sameEventHP(t, f.s.OriginalServerID(), 1, event.ID)
		var receipt db.GGOFieldReceipt
		if err := db.GetIns().Where("player_id=? AND request_id=?", p.id, ggoSystemReceiptID("boss-fight", p.start.GetFightId())).Take(&receipt).Error; err != nil {
			t.Fatal(err)
		}
		var settled model.GGOFieldBossBattleReceipt
		if err := json.Unmarshal(receipt.Response, &settled); err != nil || settled.EventID != event.ID || math.Abs(settled.Damage-(before-after)) > 0.001 {
			t.Fatalf("shared HP and receipt disagree player=%d: %v", p.id, err)
		}
		wantDamage := math.Min(before, math.Max(0, p.input.Monster.Formation[0].GetInst().GetCurHp()-autoBossHP))
		if settled.Damage != wantDamage {
			t.Fatalf("receipt damage differs from independent original automatic terminal player=%d got=%.12f want=%.12f", p.id, settled.Damage, wantDamage)
		}
		native := messages[0].(*pb.SCSidePveBossFightEnd)
		if native.GetFightResult() != result || len(native.GetBattlePos()) != len(native.GetHps()) {
			t.Fatal("native returned result or actor HP shape differs", p.id)
		}
		hps := map[int32]float64{}
		for j, pos := range native.GetBattlePos() {
			if _, exists := hps[pos]; exists {
				t.Fatal("native returned duplicate actor position", p.id, pos)
			}
			hps[pos] = native.GetHps()[j]
		}
		var attackerHP float64
		for _, actor := range terminal.FinalActors {
			if actor.Camp == 0 {
				attackerHP += actor.CurrentHP
			}
		}
		wantDefeated := result == 0 || attackerHP <= 0
		if settled.Defeated != wantDefeated || settled.Forfeit {
			t.Fatal("native outcome and death receipt differ", p.id, result, attackerHP, settled.Defeated, settled.Forfeit)
		}
		if settled.Defeated {
			var profile db.Profile
			if err := db.GetIns().First(&profile, p.id).Error; err != nil {
				t.Fatal("load death recovery", p.id, err)
			}
			_, game, err := decodeProfile(&profile)
			if err != nil || game.Field == nil || game.Field.Fight == nil || game.Field.Fight.GGOBoss == nil || game.GGOField == nil {
				t.Fatal("load frozen death recovery", p.id, err)
			}
			frozen := game.Field.Fight.GGOBoss
			point, err := ggoWirePoint(frozen.SafeReturn)
			if err != nil || settled.SafeReturn == nil || settled.SafeReturn.X == nil || settled.SafeReturn.Y == nil || settled.SafeReturn.Z == nil ||
				*settled.SafeReturn.X != float64(point.GetX()) || *settled.SafeReturn.Y != float64(point.GetY()) || *settled.SafeReturn.Z != float64(point.GetZ()) ||
				!game.PositionLoading || !proto.Equal(game.Position.GetCurrent(), point) || settled.PositionRevision < 1 || settled.PositionRevision != game.GGOField.PositionRevision {
				t.Fatal("death recovery position or receipt differs", p.id, err)
			}
			if native.GetAnger() != 0 || native.GetState() != constUtil.P8FieldIdle || frozen.HealRatio <= 0 || frozen.HealRatio > 1 || len(hps) != len(p.input.Attacker.GetFormation()) {
				t.Fatal("death recovery anger, state or formation differs", p.id)
			}
			for _, hero := range p.input.Attacker.GetFormation() {
				if got, ok := hps[hero.GetPose()]; !ok || got != frozen.HealRatio {
					t.Fatal("native returned actor HP differs from frozen death recovery", p.id, hero.GetPose())
				}
			}
		} else {
			if native.GetAnger() != terminal.Outcome.Attacker.GetInst().GetAnger() || settled.SafeReturn != nil || settled.PositionRevision != 0 {
				t.Fatal("successful outcome has unexpected anger or death return", p.id)
			}
			for _, actor := range terminal.FinalActors {
				if actor.Camp != 0 {
					continue
				}
				pos := actor.Pose
				if actor.RuntimePose != 0 {
					pos = actor.RuntimePose
				}
				if got, ok := hps[pos]; !ok || got != actor.CurrentHP/actor.MaxHP {
					t.Fatal("native returned actor HP differs from original terminal", p.id, pos)
				}
			}
		}
		replay, err := f.s.fieldBossFightEnd(p.ctx, p.id, request)
		if err != nil || len(replay) == 0 || !proto.Equal(messages[0], replay[0]) {
			t.Fatalf("native End replay player=%d: %v", p.id, err)
		}
		var receiptCount int64
		if err := db.GetIns().Model(&db.GGOFieldReceipt{}).Where("player_id=? AND request_id=?", p.id, ggoSystemReceiptID("boss-fight", p.start.GetFightId())).Count(&receiptCount).Error; err != nil || receiptCount != 1 {
			t.Fatal("native End replay duplicated its receipt", p.id, receiptCount, err)
		}
		for _, bad := range rejectedRequests {
			if _, err := f.s.fieldBossFightEnd(p.ctx, p.id, bad.body); err == nil {
				t.Fatal("modified retry reused the original receipt", bad.name)
			}
			stillAfter, _ := sameEventHP(t, f.s.OriginalServerID(), 1, event.ID)
			if stillAfter != after {
				t.Fatal("modified retry changed shared HP")
			}
		}
		effectiveTotal += settled.Damage
		if path == "playback" {
			playbackCount++
			maxPlayback = max(maxPlayback, endElapsed)
		} else {
			automaticCount++
			maxAutomatic = max(maxAutomatic, endElapsed)
		}
		t.Logf("team=%d member=%d player=%d actualFormationPower=%d seed=%d finalRound=%d winner=%d before=%.3f damage=%.3f after=%.3f path=%s playbackProbeMs=%.3f endMs=%.3f", p.team, i%5+1, p.id, p.power, p.input.Seed, terminal.Outcome.FinalRound, terminal.Outcome.Winner, before, settled.Damage, after, path, float64(probeElapsed)/float64(time.Millisecond), float64(endElapsed)/float64(time.Millisecond))
	}
	t.Logf("15-player native End duration: playback count=%d maxMs=%.3f automatic-exact count=%d maxMs=%.3f; SQLite service path, no network", playbackCount, float64(maxPlayback)/float64(time.Millisecond), automaticCount, float64(maxAutomatic)/float64(time.Millisecond))
	remaining, row := sameEventHP(t, f.s.OriginalServerID(), 1, event.ID)
	if math.Abs(effectiveTotal+remaining-float64(event.Boss.HP)) > 0.01 {
		t.Fatal("cumulative shared HP does not conserve damage")
	}
	otherAfter, _ := sameEventHP(t, f.s.OriginalServerID(), 2, second.ID)
	if otherAfter != otherHP {
		t.Fatal("line one damage changed line two HP")
	}
	t.Logf("15-player checkpoint: configuredHP=%d effectiveDamage=%.3f remaining=%.3f fullTenOwnRounds=%d/15 killed=%t", event.Boss.HP, effectiveTotal, remaining, fullRounds, row.KillerID != 0)
	if row.KillerID == 0 {
		var premature int64
		if err := db.GetIns().Model(&db.FieldBossContribution73{}).Where("server_id=? AND spawn_uuid=? AND frozen=?", f.s.OriginalServerID(), event.ID, true).Count(&premature).Error; err != nil || premature != 0 {
			t.Fatal("surviving boss must not freeze awards", premature, err)
		}
		t.Log("boss survived the 15-player checkpoint; a separate additional player may finish it for the reward contract check")
		// The extra battle is reported separately and never counted toward the
		// 15-player calibration. It uses the real remaining HP and native End.
		id, cleanupCtx := ggoBossLines86Peer(t, f, ctx, event, 1)
		contexts[id] = cleanupCtx
		cleanup := sameEventPlayer{id: id, ctx: cleanupCtx, team: 0}
		_, template, power := desertBossCalibrationTeam(t, f.s, ids[0])
		sameEventGiveFormation(t, f.s, &cleanup, template, power)
		sameEventStart(t, f.s, &cleanup, event.ID, 290000)
		if cleanup.input.Monster.GetFormation()[0].GetInst().GetCurHp() != remaining {
			t.Fatal("additional real battle did not freeze the actual remaining HP")
		}
		terminal, _, err := f.s.ComputeP6GenericMechanismBattle(cleanup.ctx, cleanup.input, nil)
		if err != nil || terminal == nil || terminal.Outcome == nil || terminal.OriginalOperates == nil {
			t.Fatal("additional real battle computation", err)
		}
		result := int32(0)
		if terminal.Outcome.Winner == 0 {
			result = 1
		}
		request := &pb.CSSidePveBossFightEnd{FightId: cleanup.start.FightId, Uuid: proto.String(event.ID), Result: proto.Int32(result), OptRecord: terminal.OriginalOperates}
		messages, err := f.s.fieldBossFightEnd(cleanup.ctx, cleanup.id, request)
		if err != nil || len(messages) == 0 || messages[0].(*pb.SCSidePveBossFightEnd).GetCode() != 1 {
			t.Fatalf("additional real battle End: %v", err)
		}
		after, current := sameEventHP(t, f.s.OriginalServerID(), 1, event.ID)
		t.Logf("additional player=%d actualFormationPower=%d finalRound=%d before=%.3f after=%.3f", cleanup.id, cleanup.power, terminal.Outcome.FinalRound, remaining, after)
		row = current
		if after != 0 || row.KillerID != cleanup.id {
			t.Fatal("additional real battle did not kill the boss; reward verification remains incomplete")
		}
	}
	var members []db.FieldBossContribution73
	if err := db.GetIns().Where("server_id=? AND spawn_uuid=? AND damage>0", f.s.OriginalServerID(), event.ID).Order("damage DESC, id ASC, player_id ASC").Find(&members).Error; err != nil {
		t.Fatal(err)
	}
	if len(members) == 0 || event.Boss.ParticipationThresholdBps != 300 {
		t.Fatal("missing contributors or 3 percent rule")
	}
	for i, member := range members {
		var award ggoBossAward
		if !member.Frozen || member.Rank != int64(i+1) || json.Unmarshal(member.GGOChips, &award) != nil || award.EconomyVersion != event.EconomyVersion || award.ExchangeValues != event.ExchangeValues {
			t.Fatalf("incorrect independent frozen award for player %d", member.PlayerID)
		}
		base, err := ggoBossChipReward(event, member.Damage, member.Rank, false)
		if err != nil {
			t.Fatal(err)
		}
		var extra int64
		for j := range base {
			if award.Chips[j] < base[j] {
				t.Fatal("frozen award lost participation or rank chips")
			}
			extra += award.Chips[j] - base[j]
		}
		wantExtra := int64(0)
		if member.PlayerID == row.KillerID {
			wantExtra = 1
		}
		if extra != wantExtra {
			t.Fatalf("tail reward duplicated or assigned to wrong player: %d", member.PlayerID)
		}
		playerCtx, ok := contexts[member.PlayerID]
		if !ok {
			t.Fatal("unexpected event contributor", member.PlayerID)
		}
		var profile db.Profile
		if err := db.GetIns().First(&profile, member.PlayerID).Error; err != nil {
			t.Fatal(err)
		}
		_, beforeGame, err := decodeProfile(&profile)
		if err != nil || beforeGame.GGOField == nil {
			t.Fatal("contributor profile unavailable", err)
		}
		beforeChips := beforeGame.GGOField.Chips
		if _, err := f.s.CollectFieldBossPendingRewards(playerCtx, member.PlayerID); err != nil {
			t.Fatalf("claim own frozen award player=%d: %v", member.PlayerID, err)
		}
		if err := db.GetIns().First(&profile, member.PlayerID).Error; err != nil {
			t.Fatal(err)
		}
		_, afterGame, err := decodeProfile(&profile)
		if err != nil || afterGame.GGOField == nil {
			t.Fatal("claimed contributor profile unavailable", err)
		}
		if !member.Claimed {
			for j, count := range award.Chips {
				if afterGame.GGOField.Chips[j] != beforeChips[j]+count {
					t.Fatalf("independent chip delivery player=%d color=%d", member.PlayerID, j)
				}
			}
		}
		var claimed db.FieldBossContribution73
		if err := db.GetIns().First(&claimed, "id=?", member.ID).Error; err != nil || !claimed.Claimed || !claimed.GGOChipsClaimed {
			t.Fatal("award claim state missing", member.PlayerID, err)
		}
		var awardReceipts int64
		if err := db.GetIns().Model(&db.GGOFieldReceipt{}).Where("player_id=? AND operation=? AND business_key=?", member.PlayerID, constUtil.GGOFieldBossAwardOperation, event.ID).Count(&awardReceipts).Error; err != nil || awardReceipts != 1 {
			t.Fatal("award delivery receipt missing or duplicated", member.PlayerID, awardReceipts, err)
		}
		again, err := f.s.CollectFieldBossPendingRewards(playerCtx, member.PlayerID)
		if err != nil || again != nil {
			t.Fatal("award was delivered more than once", member.PlayerID, err)
		}
		t.Logf("award player=%d damage=%.3f rank=%d meets3percent=%t tail=%t chips=%v", member.PlayerID, member.Damage, member.Rank, member.Damage >= float64(event.Boss.HP)*0.03, member.PlayerID == row.KillerID, award.Chips)
	}
	otherAfter, _ = sameEventHP(t, f.s.OriginalServerID(), 2, second.ID)
	if otherAfter != otherHP {
		t.Fatal("line one death changed line two HP")
	}
}

// Opt-in diagnostic: validate the original local host's recorded protobuf
// against the exact frozen server input. A mismatch remains a failing test.
func TestGGOBossCalibrationNativeRecordedReplay(t *testing.T) {
	root := os.Getenv("HYJS_GGO_SAME_EVENT_NATIVE_REPLAY")
	if root == "" {
		t.Skip("requires explicit private original-engine evidence")
	}
	raw, err := os.ReadFile(filepath.Join(root, "rerun-pb", "player-12-failure.json"))
	if err != nil {
		t.Fatal(err)
	}
	var evidence struct {
		FrozenInput model.FrozenBattleInput `json:"frozenInput"`
		NativeEndPB []byte                  `json:"nativeEndPB"`
	}
	if err := json.Unmarshal(raw, &evidence); err != nil {
		t.Fatal(err)
	}
	var end pb.CSSidePveBossFightEnd
	if err := proto.Unmarshal(evidence.NativeEndPB, &end); err != nil {
		t.Fatal(err)
	}
	raw, err = os.ReadFile(filepath.Join(root, "native-attempt-1", "original-final.pb"))
	if err != nil {
		t.Fatal(err)
	}
	var original pb.BattleOperate
	if err := proto.Unmarshal(raw, &original); err != nil || !proto.Equal(&original, end.OptRecord) {
		t.Fatal("original host PB differs from accepted source operations", err)
	}
	input := &evidence.FrozenInput
	if input.Seed != 280010 || input.MaxRound != 20 || input.BattleType != constUtil.FieldBossBattleType {
		t.Fatal("wrong frozen battle identity")
	}
	s := &GameService{Tables: input.Tables}
	t.Logf("native operation SHA=%s seed=%d round=%d attackerHP=%.15g defenderHP=%.15g", p6Digest(raw), input.Seed, original.GetFinalRoundNum(), original.GetAttackerTotalLeftHp(), original.GetDefenderTotalLeftHp())
	terminal, _, err := s.ComputeP6GenericMechanismBattle(t.Context(), input, &original)
	if err != nil {
		t.Fatalf("strict replay of original host record: %v", err)
	}
	if !proto.Equal(terminal.OriginalOperates, &original) {
		t.Fatal("strict replay changed the original operation record")
	}
}
