package service

import (
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

func TestGGOMonsterPendingOrdinaryRewardKeepsDeliveredChip(t *testing.T) {
	s := &GameService{}
	player := &pb.Player{Coin: proto.Int64(math.MaxInt64)}
	game := &model.GameState{GGOField: &model.GGOFieldState{}}
	pending := &ggoPendingMonster{FightID: "fight", Reward: "coin,10", Chips: [6]int64{0, 0, 1}, EconomyVersion: "v4", ExchangeValues: [6]int64{3, 9, 30, 90, 300, 900}}
	receipt := &model.GGOFieldMonsterReceipt{FightID: pending.FightID, Status: "settled", DeliveryStatus: "pending_delivery"}
	if _, full, err := s.ggoMonsterAward(player, game, pending); err != nil || !full {
		t.Fatal("ordinary reward must remain pending", full, err)
	}
	if err := s.ggoMonsterAwardPendingChips(player, game, pending, receipt); err != nil {
		t.Fatal(err)
	}
	if game.GGOField.Chips != ([6]int64{0, 0, 1}) || game.GGOField.Revision != 1 || receipt.Chips != ([6]int64{0, 0, 1}) || receipt.TerminalRevision != 1 || pending.Chips != ([6]int64{}) || pending.Reward != "coin,10" {
		t.Fatal("partial delivery", game.GGOField, pending, receipt)
	}
	if err := s.ggoMonsterAwardPendingChips(player, game, pending, receipt); err != nil || game.GGOField.Chips[2] != 1 || receipt.Chips[2] != 1 {
		t.Fatal("repeat partial delivery", game.GGOField, receipt, err)
	}
	player.Coin = proto.Int64(0)
	claimed, full, err := s.ggoMonsterAward(player, game, pending)
	if err != nil || full || player.GetCoin() != 10 || claimed.Chips != ([6]int64{}) || game.GGOField.Chips[2] != 1 || game.GGOField.Revision != 1 {
		t.Fatal("ordinary claim duplicated chip", claimed, full, err, game.GGOField)
	}
}

// Runs the original fight start/end with the original engine, using a frozen
// deterministic fixture for a full ordinary wallet and one awarded chip.
func TestGGOMonsterPendingChipTransactionAndRetry(t *testing.T) {
	f, ctx, session, _ := ggoBossIntegrationFixture(t)
	cfg := f.s.GGOFieldConfig
	f.s.GGOFieldRevision = "monster-chip-fixture"
	for i := range cfg.Runtime.MonsterTiers {
		cfg.Runtime.MonsterTiers[i].HP = 1
		cfg.Runtime.MonsterTiers[i].Attack = 1
	}
	var node ggoMonsterNode
	var position *model.AdminFieldGGOPosition
	if err := db.GetIns().Transaction(func(tx *gorm.DB) error {
		_, state, _, err := ggoLoadLine(tx, f.s.OriginalServerID(), 1, cfg, f.s.GGOFieldRevision, time.Now().UnixMilli())
		if err != nil {
			return err
		}
		for _, candidate := range state.Monsters {
			for _, point := range cfg.Runtime.Points {
				if point.ID == candidate.PointID && point.Patrol == nil {
					node, position = candidate, point.Position
					return nil
				}
			}
		}
		return errors.New("stationary monster fixture missing")
	}); err != nil {
		t.Fatal(err)
	}
	point, err := ggoWirePoint(position)
	if err != nil {
		t.Fatal(err)
	}
	if err = updateProfile(ctx, f.id, func(p *pb.Player, g *model.GameState) error {
		p.Coin = proto.Int64(math.MaxInt64)
		g.Position.Current = point
		g.GGOField.ProtectionUntil = 0
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	messages, err := f.s.fieldFightStart(ctx, f.id, &pb.CSSidePveFightStart{MonsterId: proto.String(node.UUID)})
	if err != nil {
		t.Fatal("native start", err)
	}
	start := messages[0].(*pb.SCSidePveFightStart)
	if err = updateProfile(ctx, f.id, func(_ *pb.Player, g *model.GameState) error {
		g.GGOField.MonsterFight.Reward = "coin,10"
		g.GGOField.MonsterFight.RewardNumerator = 1
		g.GGOField.MonsterFight.RewardDenominator = 1
		g.GGOField.MonsterFight.Chips = [6]int64{0, 0, 1}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	_, before := f.profile(t)
	input, err := fieldBossFrozenInput(before.Field.Fight)
	if err != nil {
		t.Fatal(err)
	}
	terminal, _, err := f.s.ComputeP6GenericMechanismBattle(ctx, input, nil)
	if err != nil || terminal == nil || terminal.OriginalOperates == nil || terminal.Outcome.Winner != 0 {
		t.Fatal("fixture must win the original engine battle", terminal, err)
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
			t.Fatal("missing final actor")
		}
	}
	reported.Inst.Anger = proto.Float64(terminal.Outcome.Attacker.GetInst().GetAnger())
	end := &pb.CSSidePveFightEnd{FightId: start.FightId, BattlePlayer: reported, Result: proto.Int32(1), OptRecord: terminal.OriginalOperates}
	forced := errors.New("rollback monster chip delivery")
	err = updateProfileWithTx(ctx, f.id, func(tx *gorm.DB, p *pb.Player, g *model.GameState) error {
		if _, _, _, err := f.s.ggoMonsterEndLocked(tx, f.id, p, g, end); err != nil {
			return err
		}
		return forced
	})
	if !errors.Is(err, forced) {
		t.Fatal("forced rollback", err)
	}
	_, rolledBack := f.profile(t)
	if rolledBack.GGOField.Chips != before.GGOField.Chips {
		t.Fatal("rollback changed chip balance", rolledBack.GGOField.Chips)
	}
	var count int64
	if err = db.GetIns().Model(&db.GGOFieldReceipt{}).Where("player_id=? AND operation IN ?", f.id, []string{constUtil.GGOFieldOpMonsterReady, constUtil.GGOFieldOpMonsterResult}).Count(&count).Error; err != nil || count != 0 {
		t.Fatal("rollback persisted receipts", count, err)
	}
	ended, err := f.s.fieldFightEnd(ctx, f.id, end)
	if err != nil {
		t.Fatal("native end", err)
	}
	replayed, err := f.s.fieldFightEnd(ctx, f.id, end)
	if err != nil || !proto.Equal(ended[0], replayed[0]) {
		t.Fatal("native end replay", err)
	}
	var receipt model.GGOFieldMonsterReceipt
	var ready ggoPendingMonster
	if found, err := ggoFindBusinessReceipt(db.GetIns(), f.id, constUtil.GGOFieldOpMonsterResult, start.GetFightId(), &receipt); err != nil || !found || receipt.DeliveryStatus != "pending_delivery" || receipt.Chips != ([6]int64{0, 0, 1}) {
		t.Fatal("partial result receipt", found, receipt, err)
	}
	if found, err := ggoFindBusinessReceipt(db.GetIns(), f.id, constUtil.GGOFieldOpMonsterReady, start.GetFightId(), &ready); err != nil || !found || ready.Reward != "coin,10" || ready.Chips != ([6]int64{}) {
		t.Fatal("ordinary ready receipt", found, ready, err)
	}
	var ledger db.GGOFieldReceipt
	var delta [6]int64
	if err = db.GetIns().Where("player_id=? AND operation=? AND business_key=?", f.id, constUtil.GGOFieldOpMonsterResult, start.GetFightId()).Take(&ledger).Error; err != nil || json.Unmarshal(ledger.ChipDelta, &delta) != nil || delta != ([6]int64{0, 0, 1}) {
		t.Fatal("partial delivery ledger delta", delta, err)
	}
	_, after := f.profile(t)
	if after.GGOField.Chips[2] != before.GGOField.Chips[2]+1 {
		t.Fatal("chip was not delivered exactly once", after.GGOField.Chips)
	}
	if err = updateProfile(ctx, f.id, func(p *pb.Player, _ *model.GameState) error { p.Coin = proto.Int64(0); return nil }); err != nil {
		t.Fatal(err)
	}
	database := db.GetIns()
	sqlDB, err := database.DB()
	if err != nil {
		t.Fatal(err)
	}
	f.s.adminFieldRuntimeCache.Store(&adminFieldRuntimeSnapshot{serverID: f.s.OriginalServerID(), database: database, sqlDatabase: sqlDB, signature: adminFieldDeploymentSignature(nil), tables: f.s.Tables, ggo: cfg, ggoEconomy: cfg, ggoRevision: f.s.GGOFieldRevision})
	auth := model.GGOFieldAuth{PlayerID: f.id, SessionHex: hex.EncodeToString(session)}
	claim := model.GGOFieldMonsterClaimRequest{GGOFieldMutation: model.GGOFieldMutation{GGOFieldAuth: auth, RequestID: "00000000-0000-4000-8000-000000000001"}, FightID: start.GetFightId()}
	if out, err := f.s.GGOFieldMonsterClaim(ctx, claim, session); err != nil || out.Chips != ([6]int64{}) {
		t.Fatal("ordinary pending claim", out, err)
	}
	claim.RequestID = "00000000-0000-4000-8000-000000000002"
	if out, err := f.s.GGOFieldMonsterClaim(ctx, claim, session); err != nil || out.Chips != ([6]int64{}) {
		t.Fatal("ordinary claim replay", out, err)
	}
	player, after := f.profile(t)
	if player.GetCoin() != 10 || after.GGOField.Chips[2] != before.GGOField.Chips[2]+1 {
		t.Fatal("claim duplicated chip or ordinary reward", player.GetCoin(), after.GGOField.Chips)
	}

	// Existing pre-upgrade pending rows still contain their undelivered chip.
	oldFight := "111111111111111111111111"
	oldPending := ggoPendingMonster{FightID: oldFight, Reward: "coin,5", Chips: [6]int64{0, 1}, EconomyVersion: cfg.Runtime.EconomyVersion, ExchangeValues: ready.ExchangeValues}
	oldResult := model.GGOFieldMonsterReceipt{FightID: oldFight, Status: "settled", DeliveryStatus: "pending_delivery", InputHash: p6Digest([]byte("input")), EndRequestHash: p6Digest([]byte("end")), NativeEnd: receipt.NativeEnd}
	if err = db.GetIns().Transaction(func(tx *gorm.DB) error {
		now := time.Now().UnixMilli()
		if err := ggoSaveReceipt(tx, f.id, ggoSystemReceiptID("monster-ready", oldFight), constUtil.GGOFieldOpMonsterReady, p6Digest([]byte(oldFight)), oldPending.EconomyVersion, &oldFight, [6]int64{}, 0, now, oldPending); err != nil {
			return err
		}
		return ggoSaveReceipt(tx, f.id, ggoSystemReceiptID("monster-result", oldFight), constUtil.GGOFieldOpMonsterResult, oldResult.EndRequestHash, oldPending.EconomyVersion, &oldFight, [6]int64{}, 0, now, oldResult)
	}); err != nil {
		t.Fatal("legacy pending fixture", err)
	}
	claim.FightID = oldFight
	claim.RequestID = "00000000-0000-4000-8000-000000000003"
	if out, err := f.s.GGOFieldMonsterClaim(ctx, claim, session); err != nil || out.Chips != ([6]int64{0, 1}) {
		t.Fatal("legacy pending claim", out, err)
	}
	claim.RequestID = "00000000-0000-4000-8000-000000000004"
	if out, err := f.s.GGOFieldMonsterClaim(ctx, claim, session); err != nil || out.Chips != ([6]int64{0, 1}) {
		t.Fatal("legacy claim replay", out, err)
	}
	player, after = f.profile(t)
	if player.GetCoin() != 15 || after.GGOField.Chips[1] != before.GGOField.Chips[1]+1 || after.GGOField.Chips[2] != before.GGOField.Chips[2]+1 {
		t.Fatal("legacy claim duplicated assets", player.GetCoin(), after.GGOField.Chips)
	}
}

func TestGGOMonsterPendingChipKeepsFrozenOutcomeOnEconomyConflict(t *testing.T) {
	s := &GameService{}
	player := &pb.Player{}
	game := &model.GameState{GGOField: &model.GGOFieldState{Chips: [6]int64{1}, EconomyVersion: "old", ExchangeValues: [6]int64{3, 9, 30, 90, 300, 900}, Revision: 4}}
	pending := &ggoPendingMonster{FightID: "fight", Reward: "coin,10", Chips: [6]int64{0, 1}, EconomyVersion: "new", ExchangeValues: [6]int64{3, 9, 30, 90, 300, 900}}
	receipt := &model.GGOFieldMonsterReceipt{FightID: pending.FightID, DeliveryStatus: "pending_delivery"}
	if err := s.ggoMonsterAwardPendingChips(player, game, pending, receipt); err != nil || pending.Chips != ([6]int64{0, 1}) || receipt.Chips != ([6]int64{}) || game.GGOField.Chips != ([6]int64{1}) || game.GGOField.Revision != 4 {
		t.Fatal("conflicted chip must remain pending", pending, receipt, game.GGOField, err)
	}
}
