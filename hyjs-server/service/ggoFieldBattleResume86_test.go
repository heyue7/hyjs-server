package service

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"reflect"
	"sync"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

type ggoResume86Fixture struct {
	f       *p1Fixture
	ctx     context.Context
	session []byte
	req     model.GGOFieldBattleResumeRequest
	start   proto.Message
}

type ggoResume86Base struct {
	f       *p1Fixture
	ctx     context.Context
	session []byte
	event   *ggoBossEvent
}

// Use the real isolated native start and a published build threshold; no QA
// fixture or shared production role is touched by these tests.
func ggoResume86BaseSetup(t *testing.T) *ggoResume86Base {
	t.Helper()
	if os.Getenv("HYJS_P1_MYSQL_DATABASE") == "hyjs_test_p1_desert_v4_qa_28b" {
		t.Fatal("resume tests must use a separate managed test database")
	}
	f, ctx, session, event := ggoBossIntegrationFixture(t)
	return &ggoResume86Base{f: f, ctx: ctx, session: session, event: event}
}

func ggoResume86Setup(t *testing.T, kind int64) *ggoResume86Fixture {
	t.Helper()
	return ggoResume86SetupFromBase(t, kind, ggoResume86BaseSetup(t))
}

func ggoResume86SetupFromBase(t *testing.T, kind int64, base *ggoResume86Base) *ggoResume86Fixture {
	t.Helper()
	f, ctx, session, event := base.f, base.ctx, base.session, base.event
	var start proto.Message
	if kind == constUtil.GGOBattleResumeMonster {
		var node ggoMonsterNode
		var position *model.AdminFieldGGOPosition
		cfg := f.s.GGOFieldConfig
		f.s.GGOFieldRevision = "resume86"
		for i := range cfg.Runtime.MonsterTiers {
			cfg.Runtime.MonsterTiers[i].HP = 1
			cfg.Runtime.MonsterTiers[i].Attack = 1
		}
		if err := db.GetIns().Transaction(func(tx *gorm.DB) error {
			_, state, _, err := ggoLoadLine(tx, f.s.OriginalServerID(), 1, cfg, "resume86", time.Now().UnixMilli())
			if err != nil {
				return err
			}
			for _, candidate := range state.Monsters {
				for _, point := range cfg.Runtime.Points {
					if candidate.PointID == point.ID && point.Patrol == nil {
						node, position = candidate, point.Position
						return nil
					}
				}
			}
			return errors.New("no stationary monster fixture")
		}); err != nil {
			t.Fatal(err)
		}
		point, err := ggoWirePoint(position)
		if err != nil {
			t.Fatal(err)
		}
		if err = updateProfile(ctx, f.id, func(_ *pb.Player, game *model.GameState) error {
			game.Position.Current = point
			game.GGOField.ProtectionUntil = 0
			return nil
		}); err != nil {
			t.Fatal(err)
		}
		messages, err := f.s.fieldFightStart(ctx, f.id, &pb.CSSidePveFightStart{MonsterId: proto.String(node.UUID)})
		if err != nil || len(messages) != 1 {
			t.Fatal("native monster start", err)
		}
		start = messages[0]
	} else {
		// A shared fixture may cross a scheduled event boundary between cases.
		// Select the current event before placing the test role and starting.
		if err := db.GetIns().Transaction(func(tx *gorm.DB) error {
			_, current, _, _, err := f.s.ggoBossCurrent(tx, f.s.GGOFieldConfig, time.Now().UnixMilli(), true)
			if err == nil {
				event = current
			}
			return err
		}); err != nil || event == nil {
			t.Fatal("current boss event", err)
		}
		point, err := ggoWirePoint(event.Position)
		if err != nil {
			t.Fatal(err)
		}
		// The MySQL case reuses its authenticated role after the monster
		// subtest; move only its isolated fixture to the boss event.
		if err = updateProfile(ctx, f.id, func(_ *pb.Player, game *model.GameState) error {
			if game.Position == nil || game.Position.GetScene() != constUtil.GGOFieldScene || game.Field == nil ||
				game.Field.Side == nil || game.Field.Side.GetCurHp() <= 0 || game.GGOField == nil ||
				game.GGOField.MonsterFight != nil || (game.Field.Fight != nil && !game.Field.Fight.Settled) {
				return errors.New("prior fixture battle is not settled")
			}
			game.Position.Current = point
			return nil
		}); err != nil {
			t.Fatal(err)
		}
		messages, err := f.s.fieldBossFightStart(ctx, f.id, &pb.CSSidePveBossFightStart{Uuid: proto.String(event.ID)})
		if err != nil || len(messages) != 1 {
			t.Fatal("native boss start", err)
		}
		start = messages[0]
	}
	_, game := f.profile(t)
	_, hash, err := ggoResumeFrozenInput(game.Field.Fight)
	if err != nil {
		t.Fatal("frozen input", err)
	}
	var fightID string
	if kind == constUtil.GGOBattleResumeMonster {
		fightID = start.(*pb.SCSidePveFightStart).GetFightId()
	} else {
		fightID = start.(*pb.SCSidePveBossFightStart).GetFightId()
	}
	// A disabled published deployment retains the last economy/build policy;
	// recovery remains available when new GGO entries are disabled.
	configuration, err := f.s.adminOriginalField(constUtil.GGOFieldScene)
	if err != nil {
		t.Fatal(err)
	}
	configuration.GGO = f.s.GGOFieldEconomy
	raw, err := json.Marshal(configuration)
	if err != nil {
		t.Fatal(err)
	}
	deployment := db.AdminFieldDeployment{ServerID: f.s.OriginalServerID(), Scene: constUtil.GGOFieldScene,
		TemplateID: "resume86-test", Enabled: false, Configuration: string(raw), Revision: 1, UpdatedAt: 1}
	if err = db.GetIns().Save(&deployment).Error; err != nil {
		t.Fatal(err)
	}
	return &ggoResume86Fixture{f: f, ctx: ctx, session: session, start: start, req: model.GGOFieldBattleResumeRequest{
		GGOFieldAuth: model.GGOFieldAuth{PlayerID: f.id, SessionHex: hex.EncodeToString(session)},
		BattleType:   kind, ExpectedFightID: fightID, ExpectedInputHash: hash,
	}}
}

type ggoResume86Snapshot struct {
	player, game       string
	credit             int64
	lines              []db.GGOFieldLine
	receipts           []db.GGOFieldReceipt
	contributions      []db.FieldBossContribution73
	shared             []db.SharedFieldBoss55
	calendarRecoveries int64
}

func ggoResume86SnapshotFor(t *testing.T, f *p1Fixture) ggoResume86Snapshot {
	t.Helper()
	var (
		row db.Profile
		out ggoResume86Snapshot
	)
	if err := db.GetIns().First(&row, f.id).Error; err != nil || row.Player == nil || row.Game == nil {
		t.Fatal("profile snapshot", err)
	}
	out.player, out.game, out.credit = *row.Player, *row.Game, row.GGOCredit
	if err := db.GetIns().Where("server_id=?", f.s.OriginalServerID()).Order("line").Find(&out.lines).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.GetIns().Where("player_id=?", f.id).Order("request_id").Find(&out.receipts).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.GetIns().Where("player_id=?", f.id).Order("spawn_uuid").Find(&out.contributions).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.GetIns().Where("server_id=?", f.s.OriginalServerID()).Order("scene, pos_id, line").Find(&out.shared).Error; err != nil {
		t.Fatal(err)
	}
	if f.s.ServerClockReady {
		if err := db.GetIns().Model(&db.ServerGameClockRecovery{}).Where("server_id=? AND player_id=?", f.s.OriginalServerID(), f.id).Count(&out.calendarRecoveries).Error; err != nil {
			t.Fatal(err)
		}
	}
	return out
}

func ggoResume86EndRequest(t *testing.T, fixture *ggoResume86Fixture) proto.Message {
	t.Helper()
	_, game := fixture.f.profile(t)
	input, err := fieldBossFrozenInput(game.Field.Fight)
	if err != nil {
		t.Fatal(err)
	}
	terminal, _, err := fixture.f.s.ComputeP6GenericMechanismBattle(fixture.ctx, input, nil)
	if err != nil || terminal == nil || terminal.Outcome == nil || terminal.OriginalOperates == nil {
		t.Fatal("original battle replay", err)
	}
	result := int32(0)
	if terminal.Outcome.Winner == 0 {
		result = 1
	}
	if fixture.req.BattleType == constUtil.GGOBattleResumeBoss {
		return &pb.CSSidePveBossFightEnd{FightId: proto.String(fixture.req.ExpectedFightID), Uuid: proto.String(game.Field.Fight.GGOBoss.EventID), Result: proto.Int32(result), OptRecord: terminal.OriginalOperates}
	}
	reported := proto.Clone(input.Attacker).(*pb.BattlePlayer)
	for _, hero := range reported.Formation {
		found := false
		for _, actor := range terminal.FinalActors {
			if actor.Camp == 0 && actor.GUID == hero.GetGuid() {
				hero.Inst.CurHp = proto.Float64(actor.CurrentHP)
				if actor.RuntimePose != 0 {
					hero.Pose = proto.Int32(actor.RuntimePose)
				}
				found = true
				break
			}
		}
		if !found {
			t.Fatal("terminal actor missing")
		}
	}
	reported.Inst.Anger = proto.Float64(terminal.Outcome.Attacker.GetInst().GetAnger())
	return &pb.CSSidePveFightEnd{FightId: proto.String(fixture.req.ExpectedFightID), BattlePlayer: reported, Result: proto.Int32(result), OptRecord: terminal.OriginalOperates}
}

func ggoResume86End(fixture *ggoResume86Fixture, end proto.Message) ([]proto.Message, error) {
	if fixture.req.BattleType == constUtil.GGOBattleResumeMonster {
		return fixture.f.s.fieldFightEnd(fixture.ctx, fixture.f.id, end.(*pb.CSSidePveFightEnd))
	}
	return fixture.f.s.fieldBossFightEnd(fixture.ctx, fixture.f.id, end.(*pb.CSSidePveBossFightEnd))
}

func TestGGOBattleResume86NativeAndSettled(t *testing.T) {
	var shared *ggoResume86Base
	if os.Getenv("HYJS_P1_MYSQL_DATABASE") != "" {
		// The first setup migrates global accounts in this schema. Keep its
		// authenticated role for both cases instead of asking the legacy
		// fixture to auto-register another account after that migration.
		shared = ggoResume86BaseSetup(t)
	}
	for _, kind := range []int64{constUtil.GGOBattleResumeMonster, constUtil.GGOBattleResumeBoss} {
		name := "monster"
		if kind == constUtil.GGOBattleResumeBoss {
			name = "boss"
		}
		t.Run(name, func(t *testing.T) {
			var fixture *ggoResume86Fixture
			if shared == nil {
				fixture = ggoResume86Setup(t, kind)
			} else {
				fixture = ggoResume86SetupFromBase(t, kind, shared)
			}
			if kind == constUtil.GGOBattleResumeBoss {
				// An accepted boss battle can outlive the public event; its
				// original input remains resumable but no late damage is granted.
				if err := updateProfile(fixture.ctx, fixture.f.id, func(_ *pb.Player, game *model.GameState) error {
					game.Field.Fight.GGOBoss.EndsAtMS = time.Now().Add(-time.Second).UnixMilli()
					return nil
				}); err != nil {
					t.Fatal(err)
				}
			}
			if err := fixture.f.s.InitializeServerGameClock(fixture.ctx); err != nil {
				t.Fatal(err)
			}
			before := ggoResume86SnapshotFor(t, fixture.f)
			if before.calendarRecoveries != 0 {
				t.Fatal("fixture already has a calendar recovery")
			}
			wantStart, err := proto.Marshal(fixture.start)
			if err != nil {
				t.Fatal(err)
			}
			for i := 0; i < 3; i++ {
				out, err := fixture.f.s.GGOFieldBattleResume(fixture.ctx, fixture.req, fixture.session)
				if err != nil || out.Status != constUtil.GGOBattleResumeActive || out.FightID != fixture.req.ExpectedFightID ||
					out.InputHash != fixture.req.ExpectedInputHash || !bytes.Equal(out.NativeStart, wantStart) || len(out.NativeEnd) != 0 {
					t.Fatal("active original start", out, err)
				}
				if kind == constUtil.GGOBattleResumeMonster && out.MonsterUUID == "" || kind == constUtil.GGOBattleResumeBoss && out.EventID == "" {
					t.Fatal("target identity missing")
				}
			}
			if after := ggoResume86SnapshotFor(t, fixture.f); !reflect.DeepEqual(before, after) {
				t.Fatal("resume modified profile, ledger, shared state or calendar recovery")
			}
			badRequests := []model.GGOFieldBattleResumeRequest{fixture.req, fixture.req, fixture.req, fixture.req}
			badRequests[0].ExpectedFightID = "aaaaaaaaaaaaaaaaaaaaaaaa"
			badRequests[1].ExpectedInputHash = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
			badRequests[2].BattleType = constUtil.GGOBattleResumeMonster + constUtil.GGOBattleResumeBoss - kind
			badRequests[3].PlayerID += 1000000
			for _, bad := range badRequests {
				if out, err := fixture.f.s.GGOFieldBattleResume(fixture.ctx, bad, fixture.session); err == nil || out != nil {
					t.Fatal("wrong fight identity or role disclosed data")
				}
			}
			wrongSession := append([]byte(nil), fixture.session...)
			wrongSession[0] ^= 1
			bad := fixture.req
			bad.SessionHex = hex.EncodeToString(wrongSession)
			if out, err := fixture.f.s.GGOFieldBattleResume(fixture.ctx, bad, wrongSession); err == nil || out != nil {
				t.Fatal("wrong session accepted")
			}
			server := fixture.f.s.ServerID
			fixture.f.s.ServerID = 2
			if out, err := fixture.f.s.GGOFieldBattleResume(fixture.ctx, fixture.req, fixture.session); err == nil || out != nil {
				t.Fatal("wrong server accepted")
			}
			fixture.f.s.ServerID = server
			if err := db.GetIns().Where("player_id=?", fixture.f.id).Delete(&db.ClientUpdateSession{}).Error; err != nil {
				t.Fatal(err)
			}
			if out, err := fixture.f.s.GGOFieldBattleResume(fixture.ctx, fixture.req, fixture.session); err == nil || out != nil {
				t.Fatal("missing current build accepted")
			}
			binding := db.ClientUpdateSession{PlayerID: fixture.f.id, SessionHash: clientUpdateSessionHash(fixture.session), BuildCode: fixture.f.s.GGOFieldEconomy.Runtime.MinClientBuild}
			if err := db.GetIns().Save(&binding).Error; err != nil {
				t.Fatal(err)
			}
			if err := updateProfile(fixture.ctx, fixture.f.id, func(_ *pb.Player, game *model.GameState) error {
				game.Position.Scene = proto.String("town_main")
				return nil
			}); err != nil {
				t.Fatal(err)
			}
			if out, err := fixture.f.s.GGOFieldBattleResume(fixture.ctx, fixture.req, fixture.session); err == nil || out != nil {
				t.Fatal("wrong scene accepted")
			}
			if err := updateProfile(fixture.ctx, fixture.f.id, func(_ *pb.Player, game *model.GameState) error {
				game.Position.Scene = proto.String(constUtil.GGOFieldScene)
				return nil
			}); err != nil {
				t.Fatal(err)
			}
			end := ggoResume86EndRequest(t, fixture)
			messages, err := ggoResume86End(fixture, end)
			if err != nil || len(messages) == 0 {
				t.Fatal("native end", err)
			}
			if kind == constUtil.GGOBattleResumeBoss {
				var receipt model.GGOFieldBossBattleReceipt
				found, err := ggoFindBusinessReceipt(db.GetIns(), fixture.f.id, constUtil.GGOFieldBossFightOperation, fixture.req.ExpectedFightID, &receipt)
				if err != nil || !found || receipt.Damage != 0 {
					t.Fatal("expired boss gained event damage", err)
				}
			}
			wantEnd, err := proto.Marshal(messages[0])
			if err != nil {
				t.Fatal(err)
			}
			settled := ggoResume86SnapshotFor(t, fixture.f)
			for i := 0; i < 3; i++ {
				out, err := fixture.f.s.GGOFieldBattleResume(fixture.ctx, fixture.req, fixture.session)
				if err != nil || out.Status != constUtil.GGOBattleResumeSettled || len(out.NativeStart) != 0 ||
					!bytes.Equal(out.NativeEnd, wantEnd) || !ggoResumeHex(out.EndRequestHash, 64) {
					t.Fatal("settled original receipt", out, err)
				}
			}
			if after := ggoResume86SnapshotFor(t, fixture.f); !reflect.DeepEqual(settled, after) {
				t.Fatal("settled recovery modified profile, award or ledger")
			}
		})
	}
}

// The profile row serializes resume and the real native End transaction on InnoDB.
func TestGGOBattleResume86ConcurrentEndMySQL(t *testing.T) {
	if os.Getenv("HYJS_P1_MYSQL_DATABASE") == "" {
		t.Skip("requires managed isolated InnoDB fixture")
	}
	fixture := ggoResume86Setup(t, constUtil.GGOBattleResumeBoss)
	if db.GetIns().Dialector.Name() != "mysql" {
		t.Skip("requires managed isolated InnoDB fixture")
	}
	end := ggoResume86EndRequest(t, fixture)
	wantStart, err := proto.Marshal(fixture.start)
	if err != nil {
		t.Fatal(err)
	}
	type observation struct {
		out *model.GGOFieldBattleResumeResponse
		err error
	}
	const readers = 8
	results := make(chan observation, readers)
	ended := make(chan observation, 1)
	start := make(chan struct{})
	ready := make(chan struct{}, readers+1)
	locker := db.GetIns().Begin()
	if locker.Error != nil {
		t.Fatal(locker.Error)
	}
	defer locker.Rollback()
	var locked db.Profile
	if err := locker.Clauses(clause.Locking{Strength: "UPDATE"}).First(&locked, fixture.f.id).Error; err != nil {
		t.Fatal(err)
	}
	var wait sync.WaitGroup
	for i := 0; i < readers; i++ {
		wait.Add(1)
		go func() {
			defer wait.Done()
			<-start
			ready <- struct{}{}
			out, err := fixture.f.s.GGOFieldBattleResume(fixture.ctx, fixture.req, fixture.session)
			results <- observation{out: out, err: err}
		}()
	}
	go func() {
		<-start
		ready <- struct{}{}
		messages, err := ggoResume86End(fixture, end)
		if err != nil || len(messages) == 0 {
			ended <- observation{err: err}
			return
		}
		wire, err := proto.Marshal(messages[0])
		ended <- observation{out: &model.GGOFieldBattleResumeResponse{NativeEnd: wire}, err: err}
	}()
	close(start)
	for i := 0; i < readers+1; i++ {
		<-ready
	}
	time.Sleep(100 * time.Millisecond)
	select {
	case <-results:
		t.Fatal("resume passed a held InnoDB profile row lock")
	case <-ended:
		t.Fatal("native end passed a held InnoDB profile row lock")
	default:
	}
	if err := locker.Commit().Error; err != nil {
		t.Fatal(err)
	}
	wait.Wait()
	close(results)
	endResult := <-ended
	if endResult.err != nil || endResult.out == nil {
		t.Fatal("native end during resume", endResult.err)
	}
	for result := range results {
		if result.err != nil || result.out == nil || result.out.FightID != fixture.req.ExpectedFightID || result.out.InputHash != fixture.req.ExpectedInputHash {
			t.Fatal("concurrent response identity", result.err)
		}
		switch result.out.Status {
		case constUtil.GGOBattleResumeActive:
			if !bytes.Equal(result.out.NativeStart, wantStart) || len(result.out.NativeEnd) != 0 {
				t.Fatal("concurrent active reply differs")
			}
		case constUtil.GGOBattleResumeSettled:
			if len(result.out.NativeStart) != 0 || !bytes.Equal(result.out.NativeEnd, endResult.out.NativeEnd) {
				t.Fatal("concurrent receipt differs")
			}
		default:
			t.Fatal("invalid concurrent status")
		}
	}
	before := ggoResume86SnapshotFor(t, fixture.f)
	out, err := fixture.f.s.GGOFieldBattleResume(fixture.ctx, fixture.req, fixture.session)
	if err != nil || out.Status != constUtil.GGOBattleResumeSettled || !bytes.Equal(out.NativeEnd, endResult.out.NativeEnd) {
		t.Fatal("post-race receipt", err)
	}
	if after := ggoResume86SnapshotFor(t, fixture.f); !reflect.DeepEqual(before, after) {
		t.Fatal("post-race retry changed state")
	}
	if count := p1Count(t, &db.GGOFieldReceipt{}, "player_id=? AND operation=?", fixture.f.id, constUtil.GGOFieldBossFightOperation); count != 1 {
		t.Fatal("race created multiple end receipts", count)
	}
}

func TestGGOBattleResume86FrozenInputCompatibility(t *testing.T) {
	input := &model.FrozenBattleInput{Attacker: &pb.BattlePlayer{PlayerId: proto.Int64(7)}, Monster: &pb.BattleMonster{}, BattleType: 6, Seed: 42, MaxRound: 10}
	raw, err := json.Marshal(input)
	if err != nil {
		t.Fatal(err)
	}
	hash := p6Digest(raw)
	plain := &model.FieldFight{BossInput: input}
	if _, got, err := ggoResumeFrozenInput(plain); err != nil || got != hash {
		t.Fatal("uncompressed legacy input", err)
	}
	stored, err := encodeP6StoredInput(raw)
	if err != nil {
		t.Fatal(err)
	}
	compressed := &model.FieldFight{BossInputStored: stored, BossInputHash: hash}
	if _, got, err := ggoResumeFrozenInput(compressed); err != nil || got != hash {
		t.Fatal("compressed input", err)
	}
	compressed.BossInputHash = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	if _, _, err := ggoResumeFrozenInput(compressed); err == nil {
		t.Fatal("corrupt input hash accepted")
	}
	plain.BossInputStored = stored
	if _, _, err := ggoResumeFrozenInput(plain); err == nil {
		t.Fatal("conflicting input representations accepted")
	}
}
