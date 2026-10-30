package service

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
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

// Run the original TCP framing, protobuf decoding, authenticated Dispatch,
// and response framing against the isolated fixture's actual profile sessions.
func bossHandshake86Call(t *testing.T, f *p1Fixture, id int64, opcode int32, body proto.Message, allowError ...bool) []proto.Message {
	t.Helper()
	payload, err := proto.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	raw := make([]byte, 28, 28+len(payload))
	binary.BigEndian.PutUint32(raw[0:4], uint32(24+len(payload)))
	binary.BigEndian.PutUint32(raw[4:8], 1)
	binary.BigEndian.PutUint32(raw[8:12], uint32(opcode))
	binary.BigEndian.PutUint32(raw[12:16], uint32(f.s.OriginalServerID()))
	binary.BigEndian.PutUint64(raw[16:24], uint64(id))
	binary.BigEndian.PutUint32(raw[24:28], uint32(len(payload)))
	raw = append(raw, payload...)
	packet, err := model.ParseGamePacket(raw, true)
	if err != nil {
		t.Fatal(err)
	}
	var profile db.Profile
	if err = db.GetIns().First(&profile, id).Error; err != nil {
		t.Fatal(err)
	}
	packet.Session = profile.Session
	messages := f.s.Dispatch(f.ctx, packet, "127.0.0.1")
	for _, message := range messages {
		if failure, ok := message.(*pb.ErrorMessage); ok && (len(allowError) == 0 || !allowError[0]) {
			t.Fatalf("native request %d: %s", opcode, failure.String())
		}
	}
	framed, err := model.EncodeGameResponses(messages)
	if err != nil {
		t.Fatal(err)
	}
	var decoded []proto.Message
	for len(framed) > 0 {
		if len(framed) < 8 {
			t.Fatal("truncated response")
		}
		n := int(binary.BigEndian.Uint32(framed[:4]))
		op := int32(binary.BigEndian.Uint32(framed[4:8]))
		if n < 0 || n > len(framed)-8 {
			t.Fatal("invalid response length")
		}
		msg, e := model.GameMessage(model.OpcodeNames[op], nil)
		if e != nil {
			t.Fatal(e)
		}
		if e = proto.Unmarshal(framed[8:8+n], msg); e != nil {
			t.Fatal(e)
		}
		decoded = append(decoded, msg)
		framed = framed[8+n:]
	}
	return decoded
}

func bossHandshake86Check(t *testing.T, f *p1Fixture, scene string, messages []proto.Message, expectedLine int32) *pb.SideBoss {
	t.Helper()
	var presence db.P10ScenePresence
	if err := db.GetIns().First(&presence, "player_id=?", f.id).Error; err != nil {
		t.Fatal(err)
	}
	_, game := fieldBoss33State(t, f.id)
	var boss *pb.SCSideBoss
	var monsters *pb.SCSideMonster
	var sides *pb.SCFieldPlayerState
	countdowns := make(map[int32]int64)
	for _, message := range messages {
		switch value := message.(type) {
		case *pb.SCSideBoss:
			boss = value
		case *pb.SCSideMonster:
			monsters = value
		case *pb.SCFieldPlayerState:
			sides = value
		case *pb.SCSideBossCountdown:
			countdowns[value.GetNature()] = value.GetTime()
		}
	}
	if presence.Scene != scene || presence.Line != expectedLine {
		t.Fatalf("presence scene=%s line=%d want=%d", presence.Scene, presence.Line, expectedLine)
	}
	if boss == nil || len(boss.Monsters) != 1 || len(countdowns) != 6 || game.Field == nil || monsters == nil || sides == nil {
		t.Fatal("original field projections missing")
	}
	pos := boss.Monsters[0].GetBossPosId()
	var final db.SharedFieldBoss55
	err := db.GetIns().Where("server_id=? AND scene=? AND line=? AND pos_id=?", f.s.OriginalServerID(), scene, presence.Line, pos).First(&final).Error
	t.Logf("reply: presence line=%d persisted boss line=%d final line generation exists=%t", presence.Line, game.Field.BossLine85, err == nil)
	if game.Field.BossLine85 != int64(presence.Line) {
		t.Fatalf("first reply projected persisted boss line %d, actual presence line %d", game.Field.BossLine85, presence.Line)
	}
	if err != nil {
		t.Fatal(err)
	}
	var authoritative pb.SideBoss
	if err := json.Unmarshal(final.Payload, &authoritative); err != nil {
		t.Fatal(err)
	}
	if !proto.Equal(boss.Monsters[0], &authoritative) || !proto.Equal(game.Field.Bosses[0], &authoritative) {
		t.Fatal("first response and persisted boss differ from the assigned line's generation")
	}
	if len(monsters.Monsters) != len(game.Field.Monsters) || len(sides.Player) != 1 || !proto.Equal(sides.Player[0], game.Field.Side) {
		t.Fatal("monster or player-side projection changed")
	}
	for i := range monsters.Monsters {
		if !proto.Equal(monsters.Monsters[i], game.Field.Monsters[i]) {
			t.Fatal("ordinary monster projection changed")
		}
	}
	want := fieldBossCountdownProjection(game.Field, time.Now())
	for _, message := range want {
		item := message.(*pb.SCSideBossCountdown)
		delta := countdowns[item.GetNature()] - item.GetTime()
		if delta < -1 || delta > 1 {
			t.Fatalf("countdown for nature %d differs from assigned boss", item.GetNature())
		}
	}
	return boss.Monsters[0]
}

func TestFieldBossLines86HandshakeFirstReply(t *testing.T) {
	for _, scene := range []string{"field_001", "field_002"} {
		t.Run(scene, func(t *testing.T) { bossHandshake86Exercise(t, scene) })
	}
}

func bossHandshake86Exercise(t *testing.T, scene string) {
	t.Helper()
	f := p1Setup(t)
	f.ctx = context.WithValue(f.ctx, constUtil.GameServiceContextKey, f.s)
	fieldBossLines85Initialize(t, f, false)
	account, err := LoginAccount(f.ctx, fmt.Sprintf("boss86-peer-%d", time.Now().UnixNano()), "isolated-test-password")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = CreatePlayer(f.ctx, account.Id, "线路占用", "hero_043"); err != nil {
		t.Fatal(err)
	}
	for _, id := range []int64{f.id, account.Id} {
		if err := updateProfile(f.ctx, id, func(p *pb.Player, _ *model.GameState) error { p.Level = proto.Int32(100); return nil }); err != nil {
			t.Fatal(err)
		}
	}
	if err := db.GetIns().Model(&db.P10ScenePolicy{}).Where("id=?", 1).Updates(map[string]any{"enabled": true, "line_count": 2, "max_players": 100}).Error; err != nil {
		t.Fatal(err)
	}
	closeFirst := p10SceneConnect(t, f, f.id, &p10SceneRecorder{})
	p10SceneConnect(t, f, account.Id, &p10SceneRecorder{})
	bossHandshake86Call(t, f, account.Id, 30032, &pb.CSTransferScene{Target: proto.String(scene)})
	bossHandshake86Call(t, f, account.Id, 30024, &pb.CSCurrentPosition{ShowArround: proto.Int32(1)})
	// The alternate line has a dead boss and a real deadline, making both
	// the boss identity and all six countdowns distinguishable from line one.
	pos := splitNonEmpty(f.s.fieldSceneRow(scene)["bossPosIds"])[0]
	if err := db.GetIns().Transaction(func(tx *gorm.DB) error {
		row, boss, hp, err := f.s.sharedFieldBoss55Locked(tx, scene, pos, 2, time.Now().UnixMilli())
		if err != nil {
			return err
		}
		boss.State, boss.CurHp = proto.Int32(3), proto.Float64(0)
		row.RespawnAt = time.Now().Add(time.Hour).UnixMilli()
		return saveSharedFieldBoss55(tx, row, boss, hp)
	}); err != nil {
		t.Fatal(err)
	}
	if err := db.GetIns().Model(&db.P10ScenePolicy{}).Where("id=?", 1).Update("max_players", 1).Error; err != nil {
		t.Fatal(err)
	}
	bossHandshake86Call(t, f, f.id, 30032, &pb.CSTransferScene{Target: proto.String(scene)})
	loading := bossHandshake86Call(t, f, f.id, 30053, &pb.CSLoginScene{})
	var loadingScene, loadingMonsters, loadingSide bool
	for _, message := range loading {
		switch message.(type) {
		case *pb.SCSideBoss, *pb.SCSideBossCountdown:
			t.Fatal("loading scene sent an unassigned boss or countdown")
		case *pb.SCLoginScene:
			loadingScene = true
		case *pb.SCSideMonster:
			loadingMonsters = true
		case *pb.SCFieldPlayerState:
			loadingSide = true
		}
	}
	if !loadingScene || !loadingMonsters || !loadingSide {
		t.Fatal("loading scene lost a non-boss native projection")
	}
	messages := bossHandshake86Call(t, f, f.id, 30024, &pb.CSCurrentPosition{ShowArround: proto.Int32(1)})
	line2 := bossHandshake86Check(t, f, scene, messages, 2)
	if line2.GetState() != 3 {
		t.Fatal("full-line reallocation missed the alternate line's dead boss")
	}
	if err := db.GetIns().Model(&db.P10ScenePolicy{}).Where("id=?", 1).Updates(map[string]any{"line_count": 1, "max_players": 100}).Error; err != nil {
		t.Fatal(err)
	}
	line1 := bossHandshake86Check(t, f, scene, bossHandshake86Call(t, f, f.id, 30053, &pb.CSLoginScene{}), 1)
	if line1.GetState() != 1 || line1.GetUuid() == line2.GetUuid() {
		t.Fatal("line-count reduction did not restore the line-one boss")
	}
	// A cold socket registration applies the current line policy before the
	// original scene login; no client-chosen line or fabricated response is used.
	closeFirst()
	if err := db.GetIns().Model(&db.P10ScenePresence{}).Where("player_id=?", f.id).Update("line", 2).Error; err != nil {
		t.Fatal(err)
	}
	withoutSocket := bossHandshake86Call(t, f, f.id, 30053, &pb.CSLoginScene{})
	for _, message := range withoutSocket {
		switch message.(type) {
		case *pb.SCSideBoss, *pb.SCSideBossCountdown:
			t.Fatal("cold login without a current socket sent an unassigned boss")
		}
	}
	p10SceneConnect(t, f, f.id, &p10SceneRecorder{})
	bossHandshake86Check(t, f, scene, bossHandshake86Call(t, f, f.id, 30053, &pb.CSLoginScene{}), 1)
	if err := db.GetIns().Model(&db.P10ScenePolicy{}).Where("id=?", 1).Update("max_players", 1).Error; err != nil {
		t.Fatal(err)
	}
	full := bossHandshake86Call(t, f, f.id, 30024, &pb.CSCurrentPosition{ShowArround: proto.Int32(1)}, true)
	if len(full) != 1 {
		t.Fatal("failed authoritative allocation must not return a boss projection")
	}
	if _, ok := full[0].(*pb.ErrorMessage); !ok {
		t.Fatal("full-line allocation error was swallowed")
	}
	if err := db.GetIns().Model(&db.P10ScenePolicy{}).Where("id=?", 1).Updates(map[string]any{"line_count": 2, "max_players": 100}).Error; err != nil {
		t.Fatal(err)
	}
	start, _ := fieldBossLines85Start(t, f, f.id, line1.GetUuid())
	_, before := fieldBoss33State(t, f.id)
	if err := db.GetIns().Model(&db.P10ScenePresence{}).Where("player_id=?", f.id).Update("line", 2).Error; err != nil {
		t.Fatal(err)
	}
	pending := bossHandshake86Call(t, f, f.id, 30024, &pb.CSCurrentPosition{ShowArround: proto.Int32(1)})
	_, after := fieldBoss33State(t, f.id)
	beforeFight, err := json.Marshal(before.Field.Fight)
	if err != nil {
		t.Fatal(err)
	}
	afterFight, err := json.Marshal(after.Field.Fight)
	if err != nil || string(beforeFight) != string(afterFight) || after.Field.Fight.BossLine85 != 1 || after.Field.Fight.BossUUID != line1.GetUuid() || start.GetFightId() == "" {
		t.Fatal("in-flight frozen boss line or input changed", err)
	}
	for _, message := range pending {
		switch message.(type) {
		case *pb.SCSideBoss, *pb.SCSideBossCountdown:
			t.Fatal("pending fight sent a new field boss projection")
		}
	}
	// The selective write keeps unknown fields and an actual compressed roster
	// byte-identical. Roll back this synthetic save inside the isolated test.
	synthetic := []byte(`{"futureTop":{"large":9007199254740993},"field":{"scene":"field_001","futureField":{"large":9007199254740993}},"warriors":[` + strings.Repeat(`{"id":"hero_043","name":"isolated-compression-fixture"},`, 300) + `{"id":"hero_043"}]}`)
	compressed, err := model.CompactProfileGame86(synthetic)
	if err != nil {
		t.Fatal(err)
	}
	var original map[string]json.RawMessage
	if err := json.Unmarshal(compressed, &original); err != nil || len(original["warriors"]) == 0 || original["warriors"][0] != '{' {
		t.Fatal("compressed roster fixture invalid", err)
	}
	var originalField map[string]json.RawMessage
	if err := json.Unmarshal(original["field"], &originalField); err != nil {
		t.Fatal(err)
	}
	rollback := errors.New("discard synthetic projection fixture")
	err = db.GetIns().Transaction(func(tx *gorm.DB) error {
		saved := string(compressed)
		row := db.Profile{Id: f.id, Game: &saved}
		field := &model.FieldState{Scene: "field_001", BossLine85: 2, RespawnAt: map[string]int64{"test": 123}}
		if err := fieldBossSaveProjection85(tx, &row, field); err != nil {
			return err
		}
		if err := tx.First(&row, f.id).Error; err != nil {
			return err
		}
		var current, currentField map[string]json.RawMessage
		if err := json.Unmarshal([]byte(*row.Game), &current); err != nil {
			return err
		}
		if err := json.Unmarshal(current["field"], &currentField); err != nil {
			return err
		}
		if !bytes.Equal(original["warriors"], current["warriors"]) || !bytes.Equal(original["futureTop"], current["futureTop"]) || !bytes.Equal(originalField["futureField"], currentField["futureField"]) {
			return errors.New("unknown fields or compressed roster changed")
		}
		return rollback
	})
	if !errors.Is(err, rollback) {
		t.Fatal("selective projection write", err)
	}
}
