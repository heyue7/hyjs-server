package service

import (
	"encoding/hex"
	"encoding/json"
	"math"
	"reflect"
	"testing"

	"google.golang.org/protobuf/proto"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

func TestGGOBossForfeitRecordBoundary(t *testing.T) {
	valid := &pb.CSSidePveBossFightEnd{OptRecord: &pb.BattleOperate{FinalRoundNum: proto.Int32(4)}}
	cases := []struct {
		name string
		edit func(*pb.CSSidePveBossFightEnd)
		want bool
	}{
		{"omitted zero values", func(*pb.CSSidePveBossFightEnd) {}, true},
		{"explicit zero values", func(r *pb.CSSidePveBossFightEnd) {
			r.Result = proto.Int32(0)
			r.OptRecord.AttackerTotalLeftHp = proto.Float64(0)
		}, true},
		{"win", func(r *pb.CSSidePveBossFightEnd) { r.Result = proto.Int32(1) }, false},
		{"missing record", func(r *pb.CSSidePveBossFightEnd) { r.OptRecord = nil }, false},
		{"empty record", func(r *pb.CSSidePveBossFightEnd) { r.OptRecord = &pb.BattleOperate{} }, false},
		{"round too large", func(r *pb.CSSidePveBossFightEnd) { r.OptRecord.FinalRoundNum = proto.Int32(22) }, false},
		{"surviving timeout", func(r *pb.CSSidePveBossFightEnd) {
			r.OptRecord.FinalRoundNum = proto.Int32(21)
			r.OptRecord.AttackerTotalLeftHp = proto.Float64(1)
		}, false},
		{"negative hp", func(r *pb.CSSidePveBossFightEnd) { r.OptRecord.AttackerTotalLeftHp = proto.Float64(-1) }, false},
		{"nan hp", func(r *pb.CSSidePveBossFightEnd) { r.OptRecord.AttackerTotalLeftHp = proto.Float64(math.NaN()) }, false},
		{"infinite hp", func(r *pb.CSSidePveBossFightEnd) { r.OptRecord.AttackerTotalLeftHp = proto.Float64(math.Inf(1)) }, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			request := proto.Clone(valid).(*pb.CSSidePveBossFightEnd)
			tc.edit(request)
			if got := ggoBossRequestedForfeit(request, 20); got != tc.want {
				t.Fatalf("forfeit=%v want=%v", got, tc.want)
			}
		})
	}
}

// Original End with a client-reported zero-HP loss is a self-funded forfeit
// even when the independent server terminal wins with surviving heroes.
func TestGGOBossForfeitKeepsServerDamageAndColdReceipt(t *testing.T) {
	t.Setenv("HYJS_P1_MYSQL_DATABASE", "")
	t.Setenv("HYJS_GAME_ACCOUNTS_DATABASE", "")
	f, ctx, session, event := ggoBossIntegrationFixture(t)
	messages, err := f.s.fieldBossFightStart(ctx, f.id, &pb.CSSidePveBossFightStart{Uuid: proto.String(event.ID)})
	if err != nil {
		t.Fatal(err)
	}
	start := messages[0].(*pb.SCSidePveBossFightStart)
	if err = updateProfile(ctx, f.id, func(_ *pb.Player, game *model.GameState) error {
		game.GGOField.Chips = [6]int64{10}
		game.GGOField.PositionRevision = 7
		fight := game.Field.Fight
		input, err := fieldBossFrozenInput(fight)
		if err != nil {
			return err
		}
		for _, hero := range input.Attacker.Formation {
			hero.Prop.Att = proto.Float64(1e9)
		}
		for _, enemy := range input.Monster.Formation {
			enemy.Inst.CurHp = proto.Float64(1)
			enemy.Prop.Att = proto.Float64(1)
		}
		input.MaxRound = 20
		fight.BossStart.Maxround, fight.Start.Maxround = proto.Int32(20), proto.Int32(20)
		fight.BossStart.BattlePlayer, fight.Start.BattlePlayer = input.Attacker, input.Attacker
		fight.BossStart.BattleMonsterList, fight.Start.BattleMonsterList = []*pb.BattleMonster{input.Monster}, []*pb.BattleMonster{input.Monster}
		fight.GGOBoss.ProtectedChips = 0
		fight.GGOBoss.DeathLossPercent = proto.Int64(50)
		raw, err := json.Marshal(input)
		if err != nil {
			return err
		}
		fight.BossInput, fight.BossInputStored, fight.BossInputHash = input, nil, ""
		fight.GGOBoss.InputHash = p6Digest(raw)
		return nil
	}); err != nil {
		t.Fatal("prepare isolated automatic win", err)
	}
	_, before := f.profile(t)
	input, err := fieldBossFrozenInput(before.Field.Fight)
	if err != nil {
		t.Fatal(err)
	}
	terminal, _, err := f.s.ComputeP6GenericMechanismBattle(ctx, input, nil)
	if err != nil {
		t.Fatal(err)
	}
	var remaining float64
	for _, actor := range terminal.FinalActors {
		if actor.Camp == 0 {
			remaining += actor.CurrentHP
		}
	}
	if terminal.Outcome.Winner != 0 || remaining <= 0 {
		t.Fatal("fixture must win with survivors", terminal.Outcome.Winner, remaining)
	}
	wantDamage, err := ggoBossLeaderDamage(before.Field.Fight, input, terminal)
	if err != nil || wantDamage <= 0 {
		t.Fatal("expected authoritative damage", wantDamage, err)
	}
	clientRecord := proto.Clone(terminal.OriginalOperates).(*pb.BattleOperate)
	clientRecord.AttackerTotalLeftHp = nil // protobuf-net omits default zero.
	request := &pb.CSSidePveBossFightEnd{FightId: start.FightId, Uuid: proto.String(event.ID), OptRecord: clientRecord}
	ended, err := f.s.fieldBossFightEnd(ctx, f.id, request)
	if err != nil || len(ended) == 0 || ended[0].(*pb.SCSidePveBossFightEnd).GetFightResult() != 0 {
		t.Fatal("native forfeit result", err)
	}
	_, after := f.profile(t)
	if after.GGOField.Chips != ([6]int64{5}) || after.GGOField.PositionRevision != 8 || !after.PositionLoading {
		t.Fatal("forfeit did not apply loss and safe return", after.GGOField.Chips, after.GGOField.PositionRevision)
	}
	read := func() *model.GGOFieldBossBattleReceipt {
		t.Helper()
		receipt, e := f.s.GGOFieldBossReceipt(ctx, model.GGOFieldBossReceiptRequest{GGOFieldAuth: model.GGOFieldAuth{PlayerID: f.id, SessionHex: hex.EncodeToString(session)}, FightID: start.GetFightId()}, session)
		if e != nil {
			t.Fatal(e)
		}
		return receipt
	}
	receipt := read()
	p := after.Position.GetCurrent()
	if !receipt.Defeated || !receipt.Forfeit || receipt.Lost != ([6]int64{5}) || receipt.SafeReturn == nil || receipt.PositionRevision != 8 || *receipt.SafeReturn.X != float64(p.GetX()) || *receipt.SafeReturn.Y != float64(p.GetY()) || *receipt.SafeReturn.Z != float64(p.GetZ()) || receipt.Damage != wantDamage || receipt.RewardStatus != "pending_boss_defeat" || len(receipt.RewardResources) != 0 || receipt.RewardChips != nil {
		t.Fatal("forfeit receipt or reward changed", receipt)
	}
	f.restart(t)
	replay, err := f.s.fieldBossFightEnd(ctx, f.id, request)
	if err != nil || !proto.Equal(ended[0], replay[0]) || !reflect.DeepEqual(receipt, read()) {
		t.Fatal("cold forfeit replay changed", err)
	}
	_, cold := f.profile(t)
	if cold.GGOField.Chips != ([6]int64{5}) || cold.GGOField.PositionRevision != 8 {
		t.Fatal("replay repeated loss or safe return")
	}
}

func TestGGOBossSurvivingTimeoutAppliesDeathAndKeepsDamage(t *testing.T) {
	t.Setenv("HYJS_P1_MYSQL_DATABASE", "")
	t.Setenv("HYJS_GAME_ACCOUNTS_DATABASE", "")
	ggoBossSurvivingTimeoutAppliesDeathAndKeepsDamage(t, false)
}

func TestGGOBossTimeoutDeathMySQL(t *testing.T) {
	ggoBossRound2MySQLGuard(t)
	ggoBossSurvivingTimeoutAppliesDeathAndKeepsDamage(t, true)
}

// A server-confirmed round limit applies the death return even with survivors.
func ggoBossSurvivingTimeoutAppliesDeathAndKeepsDamage(t *testing.T, requireMySQL bool) {
	t.Helper()
	f, ctx, session, event := ggoBossIntegrationFixture(t)
	if requireMySQL && db.GetIns().Dialector.Name() != "mysql" {
		t.Fatal("requires a MySQL game connection")
	}
	messages, err := f.s.fieldBossFightStart(ctx, f.id, &pb.CSSidePveBossFightStart{Uuid: proto.String(event.ID)})
	if err != nil {
		t.Fatal(err)
	}
	start := messages[0].(*pb.SCSidePveBossFightStart)
	if err = updateProfile(ctx, f.id, func(_ *pb.Player, game *model.GameState) error {
		game.GGOField.Chips = [6]int64{10}
		game.GGOField.PositionRevision = 7
		fight := game.Field.Fight
		input, e := fieldBossFrozenInput(fight)
		if e != nil {
			return e
		}
		for _, hero := range input.Attacker.Formation {
			hero.Prop.Hp = proto.Float64(1e9)
			hero.Inst.CurHp = proto.Float64(1e9)
			hero.Prop.Att = proto.Float64(100000)
			hero.Prop.Hit = proto.Float64(100)
		}
		for _, enemy := range input.Monster.Formation {
			enemy.Prop.Att = proto.Float64(1)
			enemy.Prop.Pdef = proto.Float64(1)
			enemy.Prop.Mdef = proto.Float64(1)
			enemy.Prop.Dodge = proto.Float64(0)
			if enemy.GetGuid() != ggoBossGUID(event.ID) {
				enemy.Inst.CurHp = proto.Float64(1)
			}
		}
		// The current four-member event needs time to clear the attendants and
		// damage the leader; the large leader and attacker HP still force timeout.
		input.MaxRound = 8
		fight.BossStart.Maxround, fight.Start.Maxround = proto.Int32(input.MaxRound), proto.Int32(input.MaxRound)
		fight.BossStart.BattlePlayer, fight.Start.BattlePlayer = input.Attacker, input.Attacker
		fight.BossStart.BattleMonsterList, fight.Start.BattleMonsterList = []*pb.BattleMonster{input.Monster}, []*pb.BattleMonster{input.Monster}
		fight.GGOBoss.ProtectedChips = 0
		fight.GGOBoss.DeathLossPercent = proto.Int64(50)
		raw, e := json.Marshal(input)
		if e != nil {
			return e
		}
		fight.BossInput, fight.BossInputStored, fight.BossInputHash = input, nil, ""
		fight.GGOBoss.InputHash = p6Digest(raw)
		return nil
	}); err != nil {
		t.Fatal("prepare isolated timeout", err)
	}
	_, before := f.profile(t)
	input, err := fieldBossFrozenInput(before.Field.Fight)
	if err != nil {
		t.Fatal(err)
	}
	terminal, _, err := f.s.ComputeP6GenericMechanismBattle(ctx, input, nil)
	if err != nil || terminal == nil || terminal.OriginalOperates == nil || terminal.Outcome.Winner != 1 || terminal.OriginalOperates.GetFinalRoundNum() != input.MaxRound+1 || terminal.OriginalOperates.GetAttackerTotalLeftHp() <= 0 {
		t.Fatal("fixture must time out with survivors", terminal, err)
	}
	wantDamage, err := ggoBossLeaderDamage(before.Field.Fight, input, terminal)
	if err != nil || wantDamage <= 0 || wantDamage >= float64(event.Boss.HP) {
		t.Fatal("fixture must deal partial authoritative damage", wantDamage, err)
	}
	wantReturn, err := ggoWirePoint(before.Field.Fight.GGOBoss.SafeReturn)
	if err != nil || proto.Equal(before.Position.GetCurrent(), wantReturn) {
		t.Fatal("fixture must begin away from the frozen return point", err)
	}
	request := &pb.CSSidePveBossFightEnd{FightId: start.FightId, Uuid: proto.String(event.ID), OptRecord: terminal.OriginalOperates}
	var ended []proto.Message
	if requireMySQL {
		type endReply struct {
			messages []proto.Message
			err      error
		}
		const callers = 4
		gate := make(chan struct{})
		replies := make(chan endReply, callers)
		for i := 0; i < callers; i++ {
			body := proto.Clone(request).(*pb.CSSidePveBossFightEnd)
			go func() {
				<-gate
				messages, e := f.s.fieldBossFightEnd(ctx, f.id, body)
				replies <- endReply{messages: messages, err: e}
			}()
		}
		close(gate)
		results := make([]endReply, callers)
		for i := 0; i < callers; i++ {
			results[i] = <-replies
		}
		for i, reply := range results {
			if reply.err != nil || len(reply.messages) == 0 {
				t.Fatal("concurrent native timeout", i, reply.err)
			}
			if ended == nil {
				ended = reply.messages
			} else if !proto.Equal(ended[0], reply.messages[0]) {
				t.Fatal("concurrent native timeout receipts differ", i)
			}
		}
	} else {
		ended, err = f.s.fieldBossFightEnd(ctx, f.id, request)
		if err != nil || len(ended) == 0 {
			t.Fatal("native timeout", err)
		}
	}
	native, ok := ended[0].(*pb.SCSidePveBossFightEnd)
	if !ok || native.GetCode() != 1 || native.GetFightResult() != 0 {
		t.Fatal("native timeout must report failure")
	}
	readReceipt := func() *model.GGOFieldBossBattleReceipt {
		t.Helper()
		receipt, e := f.s.GGOFieldBossReceipt(ctx, model.GGOFieldBossReceiptRequest{GGOFieldAuth: model.GGOFieldAuth{PlayerID: f.id, SessionHex: hex.EncodeToString(session)}, FightID: start.GetFightId()}, session)
		if e != nil || receipt == nil {
			t.Fatal("read timeout receipt", e)
		}
		return receipt
	}
	receipt := readReceipt()
	if !receipt.Defeated || receipt.Forfeit || receipt.Lost != ([6]int64{5}) || receipt.SafeReturn == nil || receipt.SafeReturn.X == nil || receipt.SafeReturn.Y == nil || receipt.SafeReturn.Z == nil ||
		*receipt.SafeReturn.X != float64(wantReturn.GetX()) || *receipt.SafeReturn.Y != float64(wantReturn.GetY()) || *receipt.SafeReturn.Z != float64(wantReturn.GetZ()) ||
		receipt.PositionRevision != 8 || receipt.Damage != wantDamage || receipt.RewardStatus != "pending_boss_defeat" || len(receipt.RewardResources) != 0 || receipt.RewardChips != nil {
		t.Fatal("timeout receipt lost death return or damage", receipt)
	}
	assertCommitted := func() {
		t.Helper()
		_, after := f.profile(t)
		if !after.Field.Fight.Settled || after.GGOField.Chips != ([6]int64{5}) || after.GGOField.PositionRevision != 8 || !after.PositionLoading || !proto.Equal(after.Position.GetCurrent(), wantReturn) {
			t.Fatal("timeout death loss or safe return changed", after.GGOField.Chips, after.GGOField.PositionRevision)
		}
		var member db.FieldBossContribution73
		if e := db.GetIns().Where("server_id=? AND spawn_uuid=? AND player_id=?", f.s.OriginalServerID(), event.ID, f.id).Take(&member).Error; e != nil || member.Damage != wantDamage {
			t.Fatal("timeout contribution changed", member.Damage, wantDamage, e)
		}
		var shared db.SharedFieldBoss55
		if e := sharedFieldBoss55Query(db.GetIns(), f.s.OriginalServerID(), constUtil.GGOFieldScene, constUtil.GGOFieldBossPosition).Take(&shared).Error; e != nil {
			t.Fatal("read shared boss", e)
		}
		_, _, hp, e := ggoBossDecode(&shared)
		if e != nil || hp[ggoBossGUID(event.ID)] != float64(event.Boss.HP)-wantDamage {
			t.Fatal("timeout shared damage changed", hp, e)
		}
		for _, check := range []struct {
			requestID string
			operation string
		}{
			{ggoSystemReceiptID("pve-loss", start.GetFightId()), constUtil.GGOFieldOpPVELoss},
			{ggoSystemReceiptID("boss-fight", start.GetFightId()), constUtil.GGOFieldBossFightOperation},
		} {
			var count int64
			if e := db.GetIns().Model(&db.GGOFieldReceipt{}).Where("player_id=? AND request_id=? AND operation=?", f.id, check.requestID, check.operation).Count(&count).Error; e != nil || count != 1 {
				t.Fatal("timeout receipt count", check.operation, count, e)
			}
		}
	}
	assertCommitted()
	for _, cold := range []bool{false, true} {
		if cold {
			f.restart(t)
		}
		replay, e := f.s.fieldBossFightEnd(ctx, f.id, request)
		if e != nil || len(replay) == 0 || !proto.Equal(ended[0], replay[0]) || !reflect.DeepEqual(receipt, readReceipt()) {
			t.Fatal("timeout replay changed receipt", cold, e)
		}
		assertCommitted()
	}
}
