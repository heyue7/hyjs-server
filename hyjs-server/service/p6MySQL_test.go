package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/go-sql-driver/mysql"
	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
	"os"
	"strings"
	"sync"
	"testing"
)

// This entry only accepts a pre-created dedicated schema; the external harness drops it.
func TestP6MySQLConcurrentComputeAndAtomicResources(t *testing.T) {
	dsn := os.Getenv("HYJS_MYSQL_P6_DSN")
	if dsn == "" {
		t.Skip("isolated P6 MySQL disabled")
	}
	config, err := mysql.ParseDSN(dsn)
	if err != nil || !strings.HasPrefix(config.DBName, "hyjs_test_p6_") {
		t.Fatal("P6 isolated database required")
	}
	ctx := context.Background()
	if err = db.InitGameMySQL(ctx, db.MySQLConfig{Host: config.Addr, User: config.User, Password: config.Passwd, Database: config.DBName}); err != nil {
		t.Fatal(err)
	}
	conn, _ := db.GetIns().DB()
	defer func() { conn.Close() }()
	s := &GameService{}
	if err = s.LoadTables("../../analysis/tables"); err != nil {
		t.Fatal(err)
	}
	if err = s.InitializeP6(ctx); err != nil {
		t.Fatal(err)
	}
	id := dailyCopyTestPlayer(t, ctx, "p6-go-concurrent", 10)
	f := p6Input(t, s, p6Actor("hero_047", "player", "047_1", 1, 1000), p6Actor("hero_047", "enemy", "047_1", 1, 1000), 12)
	f.Attacker.PlayerId = proto.Int64(id)
	var wg sync.WaitGroup
	ids := make([]string, 6)
	failures := make([]error, 6)
	for i := range ids {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			failures[index] = p6Tx(ctx, id, func(tx *gorm.DB) error {
				var e error
				ids[index], e = s.CaptureBattleReplay(ctx, tx, id, "isolated", "concurrent", *f)
				return e
			})
		}(i)
	}
	wg.Wait()
	for i, e := range failures {
		if e != nil || ids[i] != ids[0] {
			t.Fatal("capture concurrent replay", e)
		}
	}
	var count int64
	db.GetIns().Model(&db.P6BattleRecord{}).Where("player_id=? AND source_fight=?", id, "concurrent").Count(&count)
	if count != 1 {
		t.Fatal(count)
	}
	outcomes := make([]*model.CombatOutcome, 6)
	db.GetIns().Model(&db.P6CombatRule{}).Where("id=1").Update("authoritative_enabled", true)
	for i := range ids {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			failures[index] = p6Tx(ctx, id, func(tx *gorm.DB) error {
				var e error
				outcomes[index], e = s.ComputeBattleReplay(ctx, tx, id, "isolated", "concurrent", nil)
				return e
			})
		}(i)
	}
	wg.Wait()
	for i, e := range failures {
		if e != nil || outcomes[i].Authoritative || !proto.Equal(outcomes[i].Operates, outcomes[0].Operates) {
			t.Fatal("SQL flag bypassed native gate or computed receipt changed", e)
		}
	}
	var before db.Profile
	db.GetIns().First(&before, id)
	rollback := errors.New("reward rejected")
	err = updateProfileWithTx(ctx, id, func(tx *gorm.DB, p *pb.Player, g *model.GameState) error {
		p.Coin = proto.Int64(p.GetCoin() + 500)
		if _, e := s.CaptureBattleReplay(ctx, tx, id, "isolated", "rollback", *f); e != nil {
			return e
		}
		return rollback
	})
	if !errors.Is(err, rollback) {
		t.Fatal(err)
	}
	var after db.Profile
	db.GetIns().First(&after, id)
	if *before.Player != *after.Player || *before.Game != *after.Game {
		t.Fatal("resource rollback changed profile")
	}
	db.GetIns().Model(&db.P6BattleRecord{}).Where("player_id=? AND source_fight=?", id, "rollback").Count(&count)
	if count != 0 {
		t.Fatal("rollback capture survived")
	}
	periodicActor := p6Actor("hero_008", "periodic-source", "008_1", 1, 12000)
	periodicActor.TalentList = []string{"tal_291"}
	periodicInput := p6Input(t, s, periodicActor, p6Actor("hero_047", "periodic-target", "047_1", 1, 12000), 12)
	periodicInput.Attacker.PlayerId = proto.Int64(id)
	var periodicRecordID string
	var periodicOutcome *model.CombatOutcome
	if err = p6Tx(ctx, id, func(tx *gorm.DB) error {
		var e error
		periodicRecordID, e = s.CaptureBattleReplay(ctx, tx, id, "isolated", "periodic-persist", *periodicInput)
		if e != nil {
			return e
		}
		periodicOutcome, e = s.ComputeBattleReplay(ctx, tx, id, "isolated", "periodic-persist", nil)
		return e
	}); err != nil {
		t.Fatal(err)
	}
	counterActor := p6Actor("hero_047", "counter-defender", "047_1", 1, 12000)
	counterActor.TalentList = []string{"tal_353"}
	counterInput := p6Input(t, s, p6Actor("hero_047", "counter-attacker", "047_1", 1, 12000), counterActor, 12)
	counterInput.Attacker.PlayerId = proto.Int64(id)
	var counterRecordID string
	var counterOutcome *model.CombatOutcome
	if err = p6Tx(ctx, id, func(tx *gorm.DB) error {
		var e error
		counterRecordID, e = s.CaptureBattleReplay(ctx, tx, id, "isolated", "counter-persist", *counterInput)
		if e != nil {
			return e
		}
		counterOutcome, e = s.ComputeBattleReplay(ctx, tx, id, "isolated", "counter-persist", nil)
		return e
	}); err != nil {
		t.Fatal(err)
	}
	reviveActor := p6Actor("hero_047", "revive-defender", "047_1", 1, 100)
	reviveActor.Inst.CurHp = proto.Float64(20)
	reviveActor.TalentList = []string{"tal_282"}
	reviveInput := p6Input(t, s, p6Actor("hero_047", "revive-attacker", "047_1", 1, 1000), reviveActor, 8)
	reviveInput.Attacker.PlayerId = proto.Int64(id)
	var reviveRecordID string
	var reviveOutcome *model.CombatOutcome
	if err = p6Tx(ctx, id, func(tx *gorm.DB) error {
		var e error
		reviveRecordID, e = s.CaptureBattleReplay(ctx, tx, id, "isolated", "revive-persist", *reviveInput)
		if e != nil {
			return e
		}
		reviveOutcome, e = s.ComputeBattleReplay(ctx, tx, id, "isolated", "revive-persist", nil)
		return e
	}); err != nil {
		t.Fatal(err)
	}
	projectileInput := p6Input(t, s, p6Actor("hero_052", "projectile-attacker", "052_1", 1, 40000), p6Actor("hero_047", "projectile-target", "047_1", 1, 40000), 12)
	projectileInput.Attacker.PlayerId = proto.Int64(id)
	var projectileRecordID string
	var projectileOutcome *model.CombatOutcome
	if err = p6Tx(ctx, id, func(tx *gorm.DB) error {
		var e error
		projectileRecordID, e = s.CaptureBattleReplay(ctx, tx, id, "isolated", "projectile-persist", *projectileInput)
		if e != nil {
			return e
		}
		projectileOutcome, e = s.ComputeBattleReplay(ctx, tx, id, "isolated", "projectile-persist", nil)
		return e
	}); err != nil {
		t.Fatal(err)
	}
	awakeInput := p6Input(t, s, p6AwakenActor(), p6Actor("hero_047", "awake-target", "047_1", 1, 40000), 12)
	awakeInput.Attacker.PlayerId = proto.Int64(id)
	var awakeRecordID string
	var awakeOutcome *model.CombatOutcome
	if err = p6Tx(ctx, id, func(tx *gorm.DB) error {
		var e error
		awakeRecordID, e = s.CaptureBattleReplay(ctx, tx, id, "isolated", "awake-persist", *awakeInput)
		if e != nil {
			return e
		}
		awakeOutcome, e = s.ComputeBattleReplay(ctx, tx, id, "isolated", "awake-persist", nil)
		return e
	}); err != nil {
		t.Fatal(err)
	}
	bleedingInput := p6Input(t, s, p6BleedingActor(), p6Actor("hero_047", "bleeding-target", "047_1", 1, 40000), 12)
	bleedingInput.Attacker.PlayerId = proto.Int64(id)
	var bleedingRecordID string
	var bleedingOutcome *model.CombatOutcome
	if err = p6Tx(ctx, id, func(tx *gorm.DB) error {
		var e error
		bleedingRecordID, e = s.CaptureBattleReplay(ctx, tx, id, "isolated", "bleeding-persist", *bleedingInput)
		if e != nil {
			return e
		}
		bleedingOutcome, e = s.ComputeBattleReplay(ctx, tx, id, "isolated", "bleeding-persist", nil)
		return e
	}); err != nil {
		t.Fatal(err)
	}
	kiritoInput := p6Input(t, s, p6KiritoAwakenActor(), p6Actor("hero_047", "kirito-target", "047_1", 1, 80000), 18)
	kiritoInput.Attacker.PlayerId = proto.Int64(id)
	var kiritoRecordID string
	var kiritoOutcome *model.CombatOutcome
	if err = p6Tx(ctx, id, func(tx *gorm.DB) error {
		var e error
		kiritoRecordID, e = s.CaptureBattleReplay(ctx, tx, id, "isolated", "kirito-persist", *kiritoInput)
		if e != nil {
			return e
		}
		kiritoOutcome, e = s.ComputeBattleReplay(ctx, tx, id, "isolated", "kirito-persist", nil)
		return e
	}); err != nil {
		t.Fatal(err)
	}
	asunaInput := p6Input(t, s, p6AsunaActor(), p6Actor("hero_047", "asuna-target", "047_1", 1, 80000), 18)
	asunaInput.Attacker.PlayerId = proto.Int64(id)
	var asunaRecordID string
	var asunaOutcome *model.CombatOutcome
	if err = p6Tx(ctx, id, func(tx *gorm.DB) error {
		var e error
		asunaRecordID, e = s.CaptureBattleReplay(ctx, tx, id, "isolated", "asuna-persist", *asunaInput)
		if e != nil {
			return e
		}
		asunaOutcome, e = s.ComputeBattleReplay(ctx, tx, id, "isolated", "asuna-persist", nil)
		return e
	}); err != nil {
		t.Fatal(err)
	}
	weakInput := p6WeakReleaseInput(t, s)
	weakInput.Attacker.PlayerId = proto.Int64(id)
	var weakRecordID string
	var weakOutcome *model.CombatOutcome
	if err = p6Tx(ctx, id, func(tx *gorm.DB) error {
		var e error
		weakRecordID, e = s.CaptureBattleReplay(ctx, tx, id, "isolated", "weak-release-persist", *weakInput)
		if e != nil {
			return e
		}
		weakOutcome, e = s.ComputeBattleReplay(ctx, tx, id, "isolated", "weak-release-persist", nil)
		if e != nil {
			return e
		}
		replayed, replayErr := s.ComputeFrozenBattle(ctx, weakInput, weakOutcome.Operates)
		if replayErr != nil || !proto.Equal(replayed.Operates, weakOutcome.Operates) || replayed.Authoritative {
			return fmt.Errorf("weak-release persisted calculation diverged: %v", replayErr)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	areaInput := p6AreaBuffInput(t, s)
	areaInput.Attacker.PlayerId = proto.Int64(id)
	var areaRecordID string
	var areaOutcome *model.CombatOutcome
	if err = p6Tx(ctx, id, func(tx *gorm.DB) error {
		var e error
		areaRecordID, e = s.CaptureBattleReplay(ctx, tx, id, "isolated", "area-hit-buff-persist", *areaInput)
		if e != nil {
			return e
		}
		areaOutcome, e = s.ComputeBattleReplay(ctx, tx, id, "isolated", "area-hit-buff-persist", nil)
		return e
	}); err != nil {
		t.Fatal(err)
	}
	bowInput := p6CompleteBowInput(t, s)
	bowInput.Attacker.PlayerId = proto.Int64(id)
	var bowRecordID string
	var bowOutcome *model.CombatOutcome
	if err = p6Tx(ctx, id, func(tx *gorm.DB) error {
		var e error
		bowRecordID, e = s.CaptureBattleReplay(ctx, tx, id, "isolated", "complete-bow-persist", *bowInput)
		if e != nil {
			return e
		}
		bowOutcome, e = s.ComputeBattleReplay(ctx, tx, id, "isolated", "complete-bow-persist", nil)
		if e != nil {
			return e
		}
		replayed, replayErr := s.ComputeFrozenBattle(ctx, bowInput, bowOutcome.Operates)
		if replayErr != nil || !proto.Equal(replayed.Operates, bowOutcome.Operates) || !proto.Equal(replayed.Attacker, bowOutcome.Attacker) || !proto.Equal(replayed.Defender, bowOutcome.Defender) {
			return fmt.Errorf("complete bow persisted replay diverged: %v", replayErr)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	imposInput := p6OriginalImposBossInput(t, s)
	imposInput.Attacker.PlayerId = proto.Int64(id)
	var imposRecordID string
	var imposOutcome *model.CombatOutcome
	if err = p6Tx(ctx, id, func(tx *gorm.DB) error {
		var e error
		imposRecordID, e = s.CaptureBattleReplay(ctx, tx, id, "isolated", "impos-boss-persist", *imposInput)
		if e != nil {
			return e
		}
		imposOutcome, e = s.ComputeBattleReplay(ctx, tx, id, "isolated", "impos-boss-persist", nil)
		return e
	}); err != nil {
		t.Fatal(err)
	}
	petAreaInput := p6PetAreaTalentInput(t, s)
	petAreaInput.Attacker.PlayerId = proto.Int64(id)
	var petAreaRecordID string
	var petAreaOutcome *model.CombatOutcome
	if err = p6Tx(ctx, id, func(tx *gorm.DB) error {
		var e error
		petAreaRecordID, e = s.CaptureBattleReplay(ctx, tx, id, "isolated", "pet-area-talent-persist", *petAreaInput)
		if e != nil {
			return e
		}
		petAreaOutcome, e = s.ComputeBattleReplay(ctx, tx, id, "isolated", "pet-area-talent-persist", nil)
		return e
	}); err != nil {
		t.Fatal(err)
	}
	randomInput := p6RandomBuffInput(t, s)
	randomInput.Attacker.PlayerId = proto.Int64(id)
	var randomRecordID string
	var randomOutcome *model.CombatOutcome
	if err = p6Tx(ctx, id, func(tx *gorm.DB) error {
		var e error
		randomRecordID, e = s.CaptureBattleReplay(ctx, tx, id, "isolated", "random-buff-persist", *randomInput)
		if e != nil {
			return e
		}
		randomOutcome, e = s.ComputeBattleReplay(ctx, tx, id, "isolated", "random-buff-persist", nil)
		if e != nil {
			return e
		}
		replayed, replayErr := s.ComputeFrozenBattle(ctx, randomInput, randomOutcome.Operates)
		if replayErr != nil || !proto.Equal(replayed.Operates, randomOutcome.Operates) || !proto.Equal(replayed.Attacker, randomOutcome.Attacker) || !proto.Equal(replayed.Defender, randomOutcome.Defender) || replayed.RandomDraws != randomOutcome.RandomDraws {
			return fmt.Errorf("random child persisted calculation diverged: %v", replayErr)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	hpTransferInput := p6HPTransferInput(t, s)
	hpTransferInput.Attacker.PlayerId = proto.Int64(id)
	var hpTransferRecordID string
	var hpTransferOutcome *model.CombatOutcome
	if err = p6Tx(ctx, id, func(tx *gorm.DB) error {
		var e error
		hpTransferRecordID, e = s.CaptureBattleReplay(ctx, tx, id, "isolated", "hp-transfer-persist", *hpTransferInput)
		if e != nil {
			return e
		}
		hpTransferOutcome, e = s.ComputeBattleReplay(ctx, tx, id, "isolated", "hp-transfer-persist", nil)
		if e != nil {
			return e
		}
		replayed, replayErr := s.ComputeFrozenBattle(ctx, hpTransferInput, hpTransferOutcome.Operates)
		if replayErr != nil || !proto.Equal(replayed.Operates, hpTransferOutcome.Operates) || !proto.Equal(replayed.Attacker, hpTransferOutcome.Attacker) || !proto.Equal(replayed.Defender, hpTransferOutcome.Defender) || replayed.RandomDraws != hpTransferOutcome.RandomDraws {
			return fmt.Errorf("HP staircase transfer persisted calculation diverged: %v", replayErr)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	sourceShieldInput := p6SourceShieldInput(t, s)
	sourceShieldInput.Attacker.PlayerId = proto.Int64(id)
	var sourceShieldRecordID string
	var sourceShieldOutcome *model.CombatOutcome
	if err = p6Tx(ctx, id, func(tx *gorm.DB) error {
		var e error
		sourceShieldRecordID, e = s.CaptureBattleReplay(ctx, tx, id, "isolated", "source-shield-persist", *sourceShieldInput)
		if e != nil {
			return e
		}
		sourceShieldOutcome, e = s.ComputeBattleReplay(ctx, tx, id, "isolated", "source-shield-persist", nil)
		if e != nil {
			return e
		}
		replayed, replayErr := s.ComputeFrozenBattle(ctx, sourceShieldInput, sourceShieldOutcome.Operates)
		if replayErr != nil || !proto.Equal(replayed.Operates, sourceShieldOutcome.Operates) || !proto.Equal(replayed.Attacker, sourceShieldOutcome.Attacker) || !proto.Equal(replayed.Defender, sourceShieldOutcome.Defender) || replayed.RandomDraws != sourceShieldOutcome.RandomDraws {
			return fmt.Errorf("source-attack shield persisted calculation diverged: %v", replayErr)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	maxHPPercentInput := p6MaxHPPercentInput(t, s)
	maxHPPercentInput.Attacker.PlayerId = proto.Int64(id)
	var maxHPPercentRecordID string
	var maxHPPercentOutcome *model.CombatOutcome
	if err = p6Tx(ctx, id, func(tx *gorm.DB) error {
		var e error
		maxHPPercentRecordID, e = s.CaptureBattleReplay(ctx, tx, id, "isolated", "maxhp-percent-persist", *maxHPPercentInput)
		if e != nil {
			return e
		}
		maxHPPercentOutcome, e = s.ComputeBattleReplay(ctx, tx, id, "isolated", "maxhp-percent-persist", nil)
		if e != nil {
			return e
		}
		replayed, replayErr := s.ComputeFrozenBattle(ctx, maxHPPercentInput, maxHPPercentOutcome.Operates)
		if replayErr != nil || !proto.Equal(replayed.Operates, maxHPPercentOutcome.Operates) || !proto.Equal(replayed.Attacker, maxHPPercentOutcome.Attacker) || !proto.Equal(replayed.Defender, maxHPPercentOutcome.Defender) || replayed.RandomDraws != maxHPPercentOutcome.RandomDraws {
			return fmt.Errorf("maxHP percent persisted calculation diverged: %v", replayErr)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	multibarInput := p6OriginalMultiBarInput(t, s)
	multibarInput.Attacker.PlayerId = proto.Int64(id)
	var multibarRecordID string
	var multibarOutcome *model.CombatOutcome
	if err = p6Tx(ctx, id, func(tx *gorm.DB) error {
		var e error
		multibarRecordID, e = s.CaptureBattleReplay(ctx, tx, id, "isolated", "multibar-persist", *multibarInput)
		if e != nil {
			return e
		}
		multibarOutcome, e = s.ComputeBattleReplay(ctx, tx, id, "isolated", "multibar-persist", nil)
		return e
	}); err != nil {
		t.Fatal(err)
	}
	manualInput, manualOperate := p6ManualWaitInput(t, s)
	manualInput.Attacker.PlayerId = proto.Int64(id)
	var manualRecordID string
	var manualOutcome *model.CombatOutcome
	if err = p6Tx(ctx, id, func(tx *gorm.DB) error {
		var e error
		manualRecordID, e = s.CaptureBattleReplay(ctx, tx, id, "isolated", "manual-persist", *manualInput)
		if e != nil {
			return e
		}
		manualOutcome, e = s.ComputeBattleReplay(ctx, tx, id, "isolated", "manual-persist", manualOperate)
		if e != nil {
			return e
		}
		nativeRetry := proto.Clone(manualOperate).(*pb.BattleOperate)
		for _, op := range nativeRetry.SkillOpts {
			if op.GetCamp() == 0 {
				op.Camp = nil
			}
			if op.GetSkillIndex() == 0 {
				op.SkillIndex = nil
			}
			if op.GetRoundTime() == 0 {
				op.RoundTime = nil
			}
		}
		retried, retryErr := s.ComputeBattleReplay(ctx, tx, id, "isolated", "manual-persist", nativeRetry)
		if retryErr != nil {
			return retryErr
		}
		if !proto.Equal(retried.Operates, manualOutcome.Operates) {
			return errors.New("native omitted-default retry changed stored operation")
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	var guardRecords []struct {
		id    string
		input *model.FrozenBattleInput
		out   *model.CombatOutcome
	}
	if os.Getenv("HYJS_P6_NATIVE_DIAGNOSTIC") == "1" {
		for _, sample := range p6GuardNativeSamples() {
			name := fmt.Sprintf("guard-main12-task%d-id4-native-20261013", sample.task)
			original, nativeOperate := p6NativeMainFixture(t, name+".json", name+"-input.json", sample.id)
			nativeService := &GameService{Tables: original.Tables}
			nativeInput, freezeErr := nativeService.FreezeBattleInput(original)
			if freezeErr != nil {
				t.Fatal(freezeErr)
			}
			// Ownership alone changes to the isolated account; native seed/panel/operations remain exact.
			nativeInput.Attacker.PlayerId = proto.Int64(id)
			var nativeRecordID string
			var nativeOutcome *model.CombatOutcome
			if err = p6Tx(ctx, id, func(tx *gorm.DB) error {
				var e error
				nativeRecordID, e = s.CaptureBattleReplay(ctx, tx, id, "isolated", name, *nativeInput)
				if e != nil {
					return e
				}
				nativeOutcome, e = s.ComputeBattleReplay(ctx, tx, id, "isolated", name, nativeOperate)
				return e
			}); err != nil {
				t.Fatal(err)
			}
			if nativeOutcome.Authoritative || !p6OriginalOperateEqual(nativeOutcome.Operates, nativeOperate) {
				t.Fatal("native guardian record changed or opened authority", sample.task)
			}
			guardRecords = append(guardRecords, struct {
				id    string
				input *model.FrozenBattleInput
				out   *model.CombatOutcome
			}{nativeRecordID, nativeInput, nativeOutcome})
		}
	}
	var nativeMainRecordID string
	var nativeMainOutcome *model.CombatOutcome
	var nativeTask5RecordID string
	var nativeTask5Outcome *model.CombatOutcome
	if os.Getenv("HYJS_P6_NATIVE_DIAGNOSTIC") == "1" {
		original, nativeOperate := p6NativeRedMainFixture(t)
		nativeService := &GameService{Tables: original.Tables}
		nativeInput, freezeErr := nativeService.FreezeBattleInput(original)
		if freezeErr != nil {
			t.Fatal(freezeErr)
		}
		// Map only record ownership to the isolated account; retain actual native combat values.
		nativeInput.Attacker.PlayerId = proto.Int64(id)
		if err = p6Tx(ctx, id, func(tx *gorm.DB) error {
			var e error
			nativeMainRecordID, e = s.CaptureBattleReplay(ctx, tx, id, "isolated", "actual-main-persist", *nativeInput)
			if e != nil {
				return e
			}
			nativeMainOutcome, e = s.ComputeBattleReplay(ctx, tx, id, "isolated", "actual-main-persist", nativeOperate)
			return e
		}); err != nil {
			t.Fatal(err)
		}
		if nativeMainOutcome.Authoritative || !p6OriginalOperateEqual(nativeMainOutcome.Operates, nativeOperate) {
			t.Fatal("real native projection changed or opened authority")
		}
		original, nativeOperate = p6NativeMainFixture(t, "red-main-task5-id4-native-20261013.json", "red-main-task5-id4-native-input-20261013.json", "5ca6c5562082cb222d35e2a50efc8c793757f56a5fc8a5bf022d1a38fdb6a3f5")
		nativeService = &GameService{Tables: original.Tables}
		nativeInput, freezeErr = nativeService.FreezeBattleInput(original)
		if freezeErr != nil {
			t.Fatal(freezeErr)
		}
		nativeInput.Attacker.PlayerId = proto.Int64(id)
		if err = p6Tx(ctx, id, func(tx *gorm.DB) error {
			var e error
			nativeTask5RecordID, e = s.CaptureBattleReplay(ctx, tx, id, "isolated", "actual-task5-persist", *nativeInput)
			if e != nil {
				return e
			}
			nativeTask5Outcome, e = s.ComputeBattleReplay(ctx, tx, id, "isolated", "actual-task5-persist", nativeOperate)
			return e
		}); err != nil {
			t.Fatal(err)
		}
		if nativeTask5Outcome.Authoritative || !p6OriginalOperateEqual(nativeTask5Outcome.Operates, nativeOperate) {
			t.Fatal("real grown Asuna native record changed or opened authority")
		}
	}
	var transferRecords []struct {
		id    string
		input *model.FrozenBattleInput
		out   *model.CombatOutcome
	}
	for index, input := range p6OriginalTransferWholeInputs(t, s) {
		input.Attacker.PlayerId = proto.Int64(id)
		fight := fmt.Sprintf("property-transfer-%d", index)
		var recordID string
		var out *model.CombatOutcome
		if err = p6Tx(ctx, id, func(tx *gorm.DB) error {
			var e error
			recordID, e = s.CaptureBattleReplay(ctx, tx, id, "isolated", fight, *input)
			if e != nil {
				return e
			}
			out, e = s.ComputeBattleReplay(ctx, tx, id, "isolated", fight, nil)
			return e
		}); err != nil {
			t.Fatal(err)
		}
		replayed, e := s.ComputeFrozenBattle(ctx, input, out.Operates)
		if e != nil || out.Authoritative || !proto.Equal(out.Attacker, replayed.Attacker) || !proto.Equal(out.Defender, replayed.Defender) || !proto.Equal(out.Operates, replayed.Operates) || out.RandomDraws != replayed.RandomDraws {
			t.Fatal("property-transfer persisted computation diverged", e)
		}
		transferRecords = append(transferRecords, struct {
			id    string
			input *model.FrozenBattleInput
			out   *model.CombatOutcome
		}{recordID, input, out})
	}
	var blockRecords []struct {
		id    string
		input *model.FrozenBattleInput
		out   *model.CombatOutcome
	}
	for index, input := range p6BlockWholeInputs(t, s) {
		input.Attacker.PlayerId = proto.Int64(id)
		fight := fmt.Sprintf("native-block-%d", index)
		var recordID string
		var out *model.CombatOutcome
		if err = p6Tx(ctx, id, func(tx *gorm.DB) error {
			var e error
			recordID, e = s.CaptureBattleReplay(ctx, tx, id, "isolated", fight, *input)
			if e != nil {
				return e
			}
			out, e = s.ComputeBattleReplay(ctx, tx, id, "isolated", fight, nil)
			return e
		}); err != nil {
			t.Fatal(err)
		}
		replayed, e := s.ComputeFrozenBattle(ctx, input, out.Operates)
		if e != nil || out.Authoritative || !proto.Equal(out.Attacker, replayed.Attacker) || !proto.Equal(out.Defender, replayed.Defender) || !proto.Equal(out.Operates, replayed.Operates) || out.RandomDraws != replayed.RandomDraws {
			t.Fatal("native block persisted computation diverged", e)
		}
		blockRecords = append(blockRecords, struct {
			id    string
			input *model.FrozenBattleInput
			out   *model.CombatOutcome
		}{recordID, input, out})
	}
	var roundHPRecords []struct {
		id    string
		input *model.FrozenBattleInput
		out   *model.CombatOutcome
	}
	for index, input := range p6RoundMaxHPWholeInputs(t, s) {
		input.Attacker.PlayerId = proto.Int64(id)
		fight := fmt.Sprintf("native-round-maxhp-%d", index)
		var recordID string
		var out *model.CombatOutcome
		if err = p6Tx(ctx, id, func(tx *gorm.DB) error {
			var e error
			recordID, e = s.CaptureBattleReplay(ctx, tx, id, "isolated", fight, *input)
			if e != nil {
				return e
			}
			out, e = s.ComputeBattleReplay(ctx, tx, id, "isolated", fight, nil)
			return e
		}); err != nil {
			t.Fatal(err)
		}
		replayed, e := s.ComputeFrozenBattle(ctx, input, out.Operates)
		if e != nil || out.Authoritative || !proto.Equal(out.Attacker, replayed.Attacker) || !proto.Equal(out.Defender, replayed.Defender) || !proto.Equal(out.Operates, replayed.Operates) || out.RandomDraws != replayed.RandomDraws {
			t.Fatal("native round-maxHP persisted computation diverged", e)
		}
		roundHPRecords = append(roundHPRecords, struct {
			id    string
			input *model.FrozenBattleInput
			out   *model.CombatOutcome
		}{recordID, input, out})
	}
	inertInput := p6Native722Input(t, s)
	inertInput.Attacker.PlayerId = proto.Int64(id)
	var inertRecordID string
	var inertOutcome *model.CombatOutcome
	if err = p6Tx(ctx, id, func(tx *gorm.DB) error {
		var e error
		inertRecordID, e = s.CaptureBattleReplay(ctx, tx, id, "isolated", "native-buff722-persist", *inertInput)
		if e != nil {
			return e
		}
		inertOutcome, e = s.ComputeBattleReplay(ctx, tx, id, "isolated", "native-buff722-persist", nil)
		return e
	}); err != nil {
		t.Fatal(err)
	}
	if replayed, e := s.ComputeFrozenBattle(ctx, inertInput, inertOutcome.Operates); e != nil || inertOutcome.Authoritative || !proto.Equal(inertOutcome.Attacker, replayed.Attacker) || !proto.Equal(inertOutcome.Defender, replayed.Defender) || !proto.Equal(inertOutcome.Operates, replayed.Operates) || inertOutcome.RandomDraws != replayed.RandomDraws {
		t.Fatal("original buff722 persisted replay diverged", e)
	}
	var dispelRecords []struct {
		id    string
		input *model.FrozenBattleInput
		out   *model.CombatOutcome
	}
	for index, input := range p6DispelWholeInputs(t, s) {
		input.Attacker.PlayerId = proto.Int64(id)
		fight := fmt.Sprintf("native-dispel-%d", index)
		var recordID string
		var out *model.CombatOutcome
		if err = p6Tx(ctx, id, func(tx *gorm.DB) error {
			var e error
			recordID, e = s.CaptureBattleReplay(ctx, tx, id, "isolated", fight, *input)
			if e != nil {
				return e
			}
			out, e = s.ComputeBattleReplay(ctx, tx, id, "isolated", fight, nil)
			return e
		}); err != nil {
			t.Fatal(err)
		}
		replayed, e := s.ComputeFrozenBattle(ctx, input, out.Operates)
		if e != nil || out.Authoritative || !proto.Equal(out.Attacker, replayed.Attacker) || !proto.Equal(out.Defender, replayed.Defender) || !proto.Equal(out.Operates, replayed.Operates) || out.RandomDraws != replayed.RandomDraws {
			t.Fatal("native dispel persisted computation diverged", e)
		}
		dispelRecords = append(dispelRecords, struct {
			id    string
			input *model.FrozenBattleInput
			out   *model.CombatOutcome
		}{recordID, input, out})
	}
	var angerCompoundRecords []struct {
		id    string
		input *model.FrozenBattleInput
		out   *model.CombatOutcome
	}
	for index, input := range p6AngerCompoundWholeInputs(t, s) {
		input.Attacker.PlayerId = proto.Int64(id)
		fight := fmt.Sprintf("native-anger-compound-%d", index)
		var recordID string
		var out *model.CombatOutcome
		if err = p6Tx(ctx, id, func(tx *gorm.DB) error {
			var e error
			recordID, e = s.CaptureBattleReplay(ctx, tx, id, "isolated", fight, *input)
			if e != nil {
				return e
			}
			out, e = s.ComputeBattleReplay(ctx, tx, id, "isolated", fight, nil)
			return e
		}); err != nil {
			t.Fatal(err)
		}
		replayed, e := s.ComputeFrozenBattle(ctx, input, out.Operates)
		if e != nil || out.Authoritative || !proto.Equal(out.Attacker, replayed.Attacker) || !proto.Equal(out.Defender, replayed.Defender) || !proto.Equal(out.Operates, replayed.Operates) || out.RandomDraws != replayed.RandomDraws {
			t.Fatal("native anger compound persisted computation diverged", e)
		}
		angerCompoundRecords = append(angerCompoundRecords, struct {
			id    string
			input *model.FrozenBattleInput
			out   *model.CombatOutcome
		}{recordID, input, out})
	}
	var damageTransferRecords []struct {
		id    string
		input *model.FrozenBattleInput
		out   *model.CombatOutcome
	}
	for index, input := range p6DamageTransferWholeInputs(t, s) {
		input.Attacker.PlayerId = proto.Int64(id)
		fight := fmt.Sprintf("native-damage-transfer-%d", index)
		var recordID string
		var out *model.CombatOutcome
		if err = p6Tx(ctx, id, func(tx *gorm.DB) error {
			var e error
			recordID, e = s.CaptureBattleReplay(ctx, tx, id, "isolated", fight, *input)
			if e != nil {
				return e
			}
			out, e = s.ComputeBattleReplay(ctx, tx, id, "isolated", fight, nil)
			return e
		}); err != nil {
			t.Fatal(err)
		}
		replayed, e := s.ComputeFrozenBattle(ctx, input, out.Operates)
		if e != nil || out.Authoritative || !proto.Equal(out.Attacker, replayed.Attacker) || !proto.Equal(out.Defender, replayed.Defender) || !proto.Equal(out.Operates, replayed.Operates) || out.RandomDraws != replayed.RandomDraws {
			t.Fatal("native damage-transfer persisted computation diverged", e)
		}
		damageTransferRecords = append(damageTransferRecords, struct {
			id    string
			input *model.FrozenBattleInput
			out   *model.CombatOutcome
		}{recordID, input, out})
	}
	var lastAttackRecords []struct {
		id    string
		input *model.FrozenBattleInput
		out   *model.CombatOutcome
	}
	for index, input := range p6LastAttackWholeInputs(t, s) {
		input.Attacker.PlayerId = proto.Int64(id)
		fight := fmt.Sprintf("native-last-attack-%d", index)
		var recordID string
		var out *model.CombatOutcome
		if err = p6Tx(ctx, id, func(tx *gorm.DB) error {
			var e error
			recordID, e = s.CaptureBattleReplay(ctx, tx, id, "isolated", fight, *input)
			if e != nil {
				return e
			}
			out, e = s.ComputeBattleReplay(ctx, tx, id, "isolated", fight, nil)
			return e
		}); err != nil {
			t.Fatal(err)
		}
		replayed, e := s.ComputeFrozenBattle(ctx, input, out.Operates)
		if e != nil || out.Authoritative || !proto.Equal(out.Attacker, replayed.Attacker) || !proto.Equal(out.Defender, replayed.Defender) || !proto.Equal(out.Operates, replayed.Operates) || out.RandomDraws != replayed.RandomDraws {
			t.Fatal("native last-attack persisted computation diverged", e)
		}
		lastAttackRecords = append(lastAttackRecords, struct {
			id    string
			input *model.FrozenBattleInput
			out   *model.CombatOutcome
		}{recordID, input, out})
	}
	var sameDamageRecords []struct {
		id    string
		input *model.FrozenBattleInput
		out   *model.CombatOutcome
	}
	for index, input := range p6SameDamageWholeInputs(t, s) {
		input.Attacker.PlayerId = proto.Int64(id)
		fight := fmt.Sprintf("native-same-damage-%d", index)
		var recordID string
		var out *model.CombatOutcome
		if err = p6Tx(ctx, id, func(tx *gorm.DB) error {
			var e error
			recordID, e = s.CaptureBattleReplay(ctx, tx, id, "isolated", fight, *input)
			if e != nil {
				return e
			}
			out, e = s.ComputeBattleReplay(ctx, tx, id, "isolated", fight, nil)
			return e
		}); err != nil {
			t.Fatal(err)
		}
		replayed, e := s.ComputeFrozenBattle(ctx, input, out.Operates)
		if e != nil || out.Authoritative || !proto.Equal(out.Attacker, replayed.Attacker) || !proto.Equal(out.Defender, replayed.Defender) || !proto.Equal(out.Operates, replayed.Operates) || out.RandomDraws != replayed.RandomDraws {
			t.Fatal("native same-damage persisted computation diverged", e)
		}
		sameDamageRecords = append(sameDamageRecords, struct {
			id    string
			input *model.FrozenBattleInput
			out   *model.CombatOutcome
		}{recordID, input, out})
	}
	var skillReplacementRecords []struct {
		id    string
		input *model.FrozenBattleInput
		out   *model.CombatOutcome
	}
	for index, input := range p6SkillReplacementWholeInputs(t, s) {
		input.Attacker.PlayerId = proto.Int64(id)
		fight := fmt.Sprintf("native-skill-replacement-%d", index)
		var recordID string
		var out *model.CombatOutcome
		if err = p6Tx(ctx, id, func(tx *gorm.DB) error {
			var e error
			recordID, e = s.CaptureBattleReplay(ctx, tx, id, "isolated", fight, *input)
			if e != nil {
				return e
			}
			out, e = s.ComputeBattleReplay(ctx, tx, id, "isolated", fight, nil)
			return e
		}); err != nil {
			t.Fatal(err)
		}
		replayed, e := s.ComputeFrozenBattle(ctx, input, out.Operates)
		if e != nil || out.Authoritative || !proto.Equal(out.Attacker, replayed.Attacker) || !proto.Equal(out.Defender, replayed.Defender) || !proto.Equal(out.Operates, replayed.Operates) || out.RandomDraws != replayed.RandomDraws {
			t.Fatal("native skill-replacement persisted computation diverged", e)
		}
		skillReplacementRecords = append(skillReplacementRecords, struct {
			id    string
			input *model.FrozenBattleInput
			out   *model.CombatOutcome
		}{recordID, input, out})
	}
	var reboundRecords []struct {
		id    string
		input *model.FrozenBattleInput
		out   *model.CombatOutcome
	}
	for index, input := range p6ReboundWholeInputs(t, s) {
		input.Attacker.PlayerId = proto.Int64(id)
		fight := fmt.Sprintf("native-rebound-%d", index)
		var recordID string
		var out *model.CombatOutcome
		if err = p6Tx(ctx, id, func(tx *gorm.DB) error {
			var e error
			recordID, e = s.CaptureBattleReplay(ctx, tx, id, "isolated", fight, *input)
			if e != nil {
				return e
			}
			out, e = s.ComputeBattleReplay(ctx, tx, id, "isolated", fight, nil)
			return e
		}); err != nil {
			t.Fatal(err)
		}
		replayed, e := s.ComputeFrozenBattle(ctx, input, out.Operates)
		if e != nil || out.Authoritative || !proto.Equal(out.Attacker, replayed.Attacker) || !proto.Equal(out.Defender, replayed.Defender) || !proto.Equal(out.Operates, replayed.Operates) || out.RandomDraws != replayed.RandomDraws {
			t.Fatal("native rebound persisted computation diverged", e)
		}
		reboundRecords = append(reboundRecords, struct {
			id    string
			input *model.FrozenBattleInput
			out   *model.CombatOutcome
		}{recordID, input, out})
	}
	var shareRecords []struct {
		id    string
		input *model.FrozenBattleInput
		out   *model.CombatOutcome
	}
	for index, input := range p6ShareWholeInputs(t, s) {
		input.Attacker.PlayerId = proto.Int64(id)
		fight := fmt.Sprintf("native-share-%d", index)
		var recordID string
		var out *model.CombatOutcome
		if err = p6Tx(ctx, id, func(tx *gorm.DB) error {
			var e error
			recordID, e = s.CaptureBattleReplay(ctx, tx, id, "isolated", fight, *input)
			if e != nil {
				return e
			}
			out, e = s.ComputeBattleReplay(ctx, tx, id, "isolated", fight, nil)
			return e
		}); err != nil {
			t.Fatal(err)
		}
		replayed, e := s.ComputeFrozenBattle(ctx, input, out.Operates)
		if e != nil || out.Authoritative || !proto.Equal(out.Attacker, replayed.Attacker) || !proto.Equal(out.Defender, replayed.Defender) || !proto.Equal(out.Operates, replayed.Operates) || out.RandomDraws != replayed.RandomDraws {
			t.Fatal("native share persisted computation diverged", e)
		}
		shareRecords = append(shareRecords, struct {
			id    string
			input *model.FrozenBattleInput
			out   *model.CombatOutcome
		}{recordID, input, out})
	}
	var unmappedProbeRecords []struct {
		id    string
		input *model.FrozenBattleInput
		out   *model.CombatOutcome
	}
	for index, input := range p6UnmappedMechanismProbeInputs(t, s) {
		input.Attacker.PlayerId = proto.Int64(id)
		probeService := &GameService{Tables: input.Tables}
		fight := fmt.Sprintf("unmapped-mechanism-probe-%d", index)
		var recordID string
		var out *model.CombatOutcome
		if err = p6Tx(ctx, id, func(tx *gorm.DB) error {
			var e error
			recordID, e = probeService.CaptureBattleReplay(ctx, tx, id, "isolated", fight, *input)
			if e != nil {
				return e
			}
			out, e = probeService.ComputeBattleReplay(ctx, tx, id, "isolated", fight, nil)
			return e
		}); err != nil {
			t.Fatal(err)
		}
		replayed, e := probeService.ComputeFrozenBattle(ctx, input, out.Operates)
		if e != nil || out.Authoritative || !proto.Equal(out.Attacker, replayed.Attacker) || !proto.Equal(out.Defender, replayed.Defender) || !proto.Equal(out.Operates, replayed.Operates) || out.RandomDraws != replayed.RandomDraws {
			t.Fatal("unmapped mechanism probe persisted computation diverged", e)
		}
		unmappedProbeRecords = append(unmappedProbeRecords, struct {
			id    string
			input *model.FrozenBattleInput
			out   *model.CombatOutcome
		}{recordID, input, out})
	}
	var hpAPProbeRecords []struct {
		id    string
		input *model.FrozenBattleInput
		out   *model.CombatOutcome
	}
	for index, input := range p6HPAPProbeInputs(t, s) {
		input.Attacker.PlayerId = proto.Int64(id)
		probeService := &GameService{Tables: input.Tables}
		fight := fmt.Sprintf("unmapped-hp-ap-probe-%d", index)
		var recordID string
		var out *model.CombatOutcome
		if err = p6Tx(ctx, id, func(tx *gorm.DB) error {
			var e error
			recordID, e = probeService.CaptureBattleReplay(ctx, tx, id, "isolated", fight, *input)
			if e != nil {
				return e
			}
			out, e = probeService.ComputeBattleReplay(ctx, tx, id, "isolated", fight, nil)
			return e
		}); err != nil {
			t.Fatal(err)
		}
		replayed, e := probeService.ComputeFrozenBattle(ctx, input, out.Operates)
		if e != nil || out.Authoritative || !proto.Equal(out.Attacker, replayed.Attacker) || !proto.Equal(out.Defender, replayed.Defender) || !proto.Equal(out.Operates, replayed.Operates) || out.RandomDraws != replayed.RandomDraws {
			t.Fatal("unmapped HP/AP probe persisted computation diverged", e)
		}
		hpAPProbeRecords = append(hpAPProbeRecords, struct {
			id    string
			input *model.FrozenBattleInput
			out   *model.CombatOutcome
		}{recordID, input, out})
	}
	var roundExtraProbeRecords []struct {
		id    string
		input *model.FrozenBattleInput
		out   *model.CombatOutcome
	}
	for index, input := range p6RoundExtraProbeInputs(t, s) {
		input.Attacker.PlayerId = proto.Int64(id)
		probeService := &GameService{Tables: input.Tables}
		fight := fmt.Sprintf("unmapped-round-extra-probe-%d", index)
		var recordID string
		var out *model.CombatOutcome
		if err = p6Tx(ctx, id, func(tx *gorm.DB) error {
			var e error
			recordID, e = probeService.CaptureBattleReplay(ctx, tx, id, "isolated", fight, *input)
			if e != nil {
				return e
			}
			out, e = probeService.ComputeBattleReplay(ctx, tx, id, "isolated", fight, nil)
			return e
		}); err != nil {
			t.Fatal(err)
		}
		replayed, e := probeService.ComputeFrozenBattle(ctx, input, out.Operates)
		if e != nil || out.Authoritative || !proto.Equal(out.Attacker, replayed.Attacker) || !proto.Equal(out.Defender, replayed.Defender) || !proto.Equal(out.Operates, replayed.Operates) || out.RandomDraws != replayed.RandomDraws {
			t.Fatal("unmapped round-extra probe persisted computation diverged", e)
		}
		roundExtraProbeRecords = append(roundExtraProbeRecords, struct {
			id    string
			input *model.FrozenBattleInput
			out   *model.CombatOutcome
		}{recordID, input, out})
	}
	var hpChangeRecords []struct {
		id    string
		input *model.FrozenBattleInput
		out   *model.CombatOutcome
	}
	// Persist both camps of each actual high-tier emergency-heal actor with all original skills.
	for index, input := range p6HPChangeWholeInputs(t, s) {
		input.Attacker.PlayerId = proto.Int64(id)
		fight := fmt.Sprintf("native-hp-change-%d", index)
		var recordID string
		var out *model.CombatOutcome
		if err = p6Tx(ctx, id, func(tx *gorm.DB) error {
			var e error
			recordID, e = s.CaptureBattleReplay(ctx, tx, id, "isolated", fight, *input)
			if e != nil {
				return e
			}
			out, e = s.ComputeBattleReplay(ctx, tx, id, "isolated", fight, nil)
			return e
		}); err != nil {
			t.Fatal(err)
		}
		found := false
		for _, ev := range out.Events {
			found = found || ev.Kind == "buff_load" && (ev.Buff == "buff_418" || ev.Buff == "buff_829")
		}
		replayed, e := s.ComputeFrozenBattle(ctx, input, out.Operates)
		if e != nil || !found || out.Authoritative || !proto.Equal(out.Attacker, replayed.Attacker) || !proto.Equal(out.Defender, replayed.Defender) || !proto.Equal(out.Operates, replayed.Operates) || out.RandomDraws != replayed.RandomDraws {
			t.Fatal("native actual emergency recovery persisted computation diverged", e)
		}
		hpChangeRecords = append(hpChangeRecords, struct {
			id    string
			input *model.FrozenBattleInput
			out   *model.CombatOutcome
		}{recordID, input, out})
	}
	var conditionAndNativeRecords []struct {
		id         string
		input      *model.FrozenBattleInput
		out        *model.CombatOutcome
		goldenCase string
	}
	// Actual 10 mapped high-tier heroes exercise all original skills against 1/3 enemies.
	newInputs := p6RemainingConditionWholeInputs(t, s)
	// Preserve actual weak-count and maximum-missing-HP owners from both camps.
	newInputs = append(newInputs, p6LiveSelectionWholeInputs(t, s)...)
	newInputs = append(newInputs, p6OriginalIdleNPCWholeInputs(t)...)
	provided := map[int]*pb.BattleOperate{}
	nativeCases := map[int]string{}
	if os.Getenv("HYJS_P6_NATIVE_DIAGNOSTIC") == "1" {
		for _, tc := range []struct{ name, fixture string }{{"solo", "daily-id4-native-20261013.json"}, {"food", "daily-fooddark-id4-native-20261013.json"}} {
			input, operations := p6NativeDailyInput(t, tc.fixture)
			provided[len(newInputs)], nativeCases[len(newInputs)] = operations, tc.name
			newInputs = append(newInputs, input)
		}
	}
	for index, input := range newInputs {
		input.Attacker.PlayerId = proto.Int64(id)
		fight := fmt.Sprintf("native-condition-and-daily-%d", index)
		var recordID string
		var out *model.CombatOutcome
		if err = p6Tx(ctx, id, func(tx *gorm.DB) error {
			var e error
			recordID, e = s.CaptureBattleReplay(ctx, tx, id, "isolated", fight, *input)
			if e != nil {
				return e
			}
			out, e = s.ComputeBattleReplay(ctx, tx, id, "isolated", fight, provided[index])
			return e
		}); err != nil {
			t.Fatal(err)
		}
		replayed, e := s.ComputeFrozenBattle(ctx, input, out.Operates)
		if e != nil || out.Authoritative || !proto.Equal(out.Attacker, replayed.Attacker) || !proto.Equal(out.Monster, replayed.Monster) || !proto.Equal(out.Defender, replayed.Defender) || !proto.Equal(out.Operates, replayed.Operates) || out.RandomDraws != replayed.RandomDraws {
			t.Fatal("actual original remaining conditions/native Daily persisted computation diverged", e)
		}
		if nativeCases[index] != "" {
			p6AssertNativePlaybackGolden(t, nativeCases[index], out)
		}
		conditionAndNativeRecords = append(conditionAndNativeRecords, struct {
			id         string
			input      *model.FrozenBattleInput
			out        *model.CombatOutcome
			goldenCase string
		}{recordID, input, out, nativeCases[index]})
	}
	// Preserve historical schema snapshots in MySQL, then read them after connection restart.
	legacyOperate := proto.Clone(outcomes[0].Operates).(*pb.BattleOperate)
	for _, op := range legacyOperate.SkillOpts {
		op.ActorUid = proto.String(fmt.Sprintf("%d_%s", op.GetCamp(), op.GetActorUid()))
	}
	legacyIDs := []string{}
	for _, version := range []string{"apk-10056262-p6-v2", "apk-10056262-p6-v3"} {
		var legacyID string
		if err = p6Tx(ctx, id, func(tx *gorm.DB) error {
			var e error
			legacyID, e = s.CaptureBattleReplay(ctx, tx, id, "isolated", version, *f)
			if e != nil {
				return e
			}
			if e = s.FinishObservedBattleReplay(ctx, tx, id, "isolated", version, legacyOperate, outcomes[0].Winner == 0); e != nil {
				return e
			}
			var record db.P6BattleRecord
			if e = tx.First(&record, "id=?", legacyID).Error; e != nil {
				return e
			}
			var input model.FrozenBattleInput
			storedInput, decodeErr := decodeP6StoredInput(record.Input)
			if decodeErr != nil {
				return decodeErr
			}
			if e = json.Unmarshal(storedInput, &input); e != nil {
				return e
			}
			input.EngineVersion = version
			if record.Input, e = json.Marshal(&input); e != nil {
				return e
			}
			record.EngineVersion, record.InputHash = version, p6Digest(record.Input)
			return tx.Save(&record).Error
		}); err != nil {
			t.Fatal(err)
		}
		legacyIDs = append(legacyIDs, legacyID)
	}
	// Reopen all DB connections to model process restart; frozen content must survive it.
	if err = p6Tx(ctx, id, func(tx *gorm.DB) error {
		_, e := s.CaptureBattleReplay(ctx, tx, id, "isolated", "resume-after-restart", *f)
		return e
	}); err != nil {
		t.Fatal(err)
	}
	if err = conn.Close(); err != nil {
		t.Fatal(err)
	}
	if err = db.InitGameMySQL(ctx, db.MySQLConfig{Host: config.Addr, User: config.User, Password: config.Passwd, Database: config.DBName}); err != nil {
		t.Fatal(err)
	}
	conn, _ = db.GetIns().DB()
	s.table("SkillCast", "SkillCastId", "895")["ExportFactor"] = "999"
	if err = p6Tx(ctx, id, func(tx *gorm.DB) error {
		frozen, e := s.RecoverFrozenBattle(ctx, tx, id, "isolated", "resume-after-restart")
		if e == nil && (frozen.ConfigurationHash != f.ConfigurationHash || !proto.Equal(frozen.Attacker, f.Attacker)) {
			t.Fatal("restart changed frozen input")
		}
		return e
	}); err != nil {
		t.Fatal(err)
	}
	if err = p6Tx(ctx, id, func(tx *gorm.DB) error {
		replayed, e := s.ComputeBattleReplay(ctx, tx, id, "isolated", "concurrent", nil)
		if e == nil && !proto.Equal(replayed.Operates, outcomes[0].Operates) {
			t.Fatal("restart changed computed receipt")
		}
		return e
	}); err != nil {
		t.Fatal(err)
	}
	playback, err := s.fightPlayback(ctx, id, &pb.CSFightPlayback{FightId: proto.String(periodicRecordID), ServerId: proto.Int32(1)})
	if err != nil || !proto.Equal(playback[0].(*pb.SCFightPlayback).OptRecord, periodicOutcome.Operates) {
		t.Fatal("periodic persisted playback lost", err)
	}
	playback, err = s.fightPlayback(ctx, id, &pb.CSFightPlayback{FightId: proto.String(counterRecordID), ServerId: proto.Int32(1)})
	if err != nil || !proto.Equal(playback[0].(*pb.SCFightPlayback).OptRecord, counterOutcome.Operates) {
		t.Fatal("counterstrike persisted playback lost", err)
	}
	playback, err = s.fightPlayback(ctx, id, &pb.CSFightPlayback{FightId: proto.String(reviveRecordID), ServerId: proto.Int32(1)})
	if err != nil || !proto.Equal(playback[0].(*pb.SCFightPlayback).OptRecord, reviveOutcome.Operates) {
		t.Fatal("revival persisted playback lost", err)
	}
	playback, err = s.fightPlayback(ctx, id, &pb.CSFightPlayback{FightId: proto.String(projectileRecordID), ServerId: proto.Int32(1)})
	if err != nil || !proto.Equal(playback[0].(*pb.SCFightPlayback).OptRecord, projectileOutcome.Operates) {
		t.Fatal("projectile persisted playback lost", err)
	}
	playback, err = s.fightPlayback(ctx, id, &pb.CSFightPlayback{FightId: proto.String(awakeRecordID), ServerId: proto.Int32(1)})
	if err != nil || !proto.Equal(playback[0].(*pb.SCFightPlayback).OptRecord, awakeOutcome.Operates) || !proto.Equal(playback[0].(*pb.SCFightPlayback).Attacker, awakeInput.Attacker) {
		t.Fatal("awake persisted playback lost native slots or snapshot", err)
	}
	playback, err = s.fightPlayback(ctx, id, &pb.CSFightPlayback{FightId: proto.String(bleedingRecordID), ServerId: proto.Int32(1)})
	if err != nil || !proto.Equal(playback[0].(*pb.SCFightPlayback).OptRecord, bleedingOutcome.Operates) || !proto.Equal(playback[0].(*pb.SCFightPlayback).Attacker, bleedingInput.Attacker) {
		t.Fatal("bleeding persisted playback lost bleeding snapshot", err)
	}
	playback, err = s.fightPlayback(ctx, id, &pb.CSFightPlayback{FightId: proto.String(kiritoRecordID), ServerId: proto.Int32(1)})
	if err != nil || !proto.Equal(playback[0].(*pb.SCFightPlayback).OptRecord, kiritoOutcome.Operates) || !proto.Equal(playback[0].(*pb.SCFightPlayback).Attacker, kiritoInput.Attacker) {
		t.Fatal("kirito persisted playback lost kirito snapshot", err)
	}
	playback, err = s.fightPlayback(ctx, id, &pb.CSFightPlayback{FightId: proto.String(asunaRecordID), ServerId: proto.Int32(1)})
	if err != nil || !proto.Equal(playback[0].(*pb.SCFightPlayback).OptRecord, asunaOutcome.Operates) || !proto.Equal(playback[0].(*pb.SCFightPlayback).Attacker, asunaInput.Attacker) {
		t.Fatal("asuna persisted playback lost asuna snapshot", err)
	}
	playback, err = s.fightPlayback(ctx, id, &pb.CSFightPlayback{FightId: proto.String(weakRecordID), ServerId: proto.Int32(1)})
	if err != nil || !proto.Equal(playback[0].(*pb.SCFightPlayback).OptRecord, weakOutcome.Operates) || !proto.Equal(playback[0].(*pb.SCFightPlayback).Attacker, weakInput.Attacker) || !proto.Equal(playback[0].(*pb.SCFightPlayback).Defender1, weakInput.Defender) {
		t.Fatal("held-target persisted playback lost original snapshot or release flow", err)
	}
	playback, err = s.fightPlayback(ctx, id, &pb.CSFightPlayback{FightId: proto.String(areaRecordID), ServerId: proto.Int32(1)})
	if err != nil || !proto.Equal(playback[0].(*pb.SCFightPlayback).OptRecord, areaOutcome.Operates) || !proto.Equal(playback[0].(*pb.SCFightPlayback).Attacker, areaInput.Attacker) || !proto.Equal(playback[0].(*pb.SCFightPlayback).Defender1, areaInput.Defender) {
		t.Fatal("area-hit Buff persisted playback lost original snapshot or target scope", err)
	}
	playback, err = s.fightPlayback(ctx, id, &pb.CSFightPlayback{FightId: proto.String(bowRecordID), ServerId: proto.Int32(1)})
	if err != nil || !proto.Equal(playback[0].(*pb.SCFightPlayback).OptRecord, bowOutcome.Operates) || !proto.Equal(playback[0].(*pb.SCFightPlayback).Attacker, bowInput.Attacker) || !proto.Equal(playback[0].(*pb.SCFightPlayback).Defender1, bowInput.Defender) {
		t.Fatal("complete bow persisted playback lost original skill sequence or snapshot", err)
	}
	playback, err = s.fightPlayback(ctx, id, &pb.CSFightPlayback{FightId: proto.String(imposRecordID), ServerId: proto.Int32(1)})
	if err != nil || !proto.Equal(playback[0].(*pb.SCFightPlayback).OptRecord, imposOutcome.Operates) || !proto.Equal(playback[0].(*pb.SCFightPlayback).Attacker, imposInput.Attacker) || !proto.Equal(playback[0].(*pb.SCFightPlayback).Defender2, imposInput.Monster) {
		t.Fatal("immune-boss persisted playback lost original snapshot or operation", err)
	}
	var imposRecord db.P6BattleRecord
	if err = db.GetIns().WithContext(ctx).First(&imposRecord, "id=?", imposRecordID).Error; err != nil {
		t.Fatal(err)
	}
	var imposFrozen model.FrozenBattleInput
	imposStoredInput, decodeErr := decodeP6StoredInput(imposRecord.Input)
	if decodeErr != nil {
		t.Fatal(decodeErr)
	}
	if err = json.Unmarshal(imposStoredInput, &imposFrozen); err != nil || (&GameService{Tables: imposFrozen.Tables}).table("Hero", "id", "monster_035")["ImposState"] != "2" || imposFrozen.ConfigurationHash != imposInput.ConfigurationHash || imposRecord.Authoritative {
		t.Fatal("stored original immunity or authority gate changed", err)
	}
	playback, err = s.fightPlayback(ctx, id, &pb.CSFightPlayback{FightId: proto.String(petAreaRecordID), ServerId: proto.Int32(1)})
	if err != nil || !proto.Equal(playback[0].(*pb.SCFightPlayback).OptRecord, petAreaOutcome.Operates) || !proto.Equal(playback[0].(*pb.SCFightPlayback).Attacker, petAreaInput.Attacker) || !proto.Equal(playback[0].(*pb.SCFightPlayback).Defender1, petAreaInput.Defender) {
		t.Fatal("original guardian area talent playback lost original actor or operation", err)
	}
	playback, err = s.fightPlayback(ctx, id, &pb.CSFightPlayback{FightId: proto.String(randomRecordID), ServerId: proto.Int32(1)})
	if err != nil || !proto.Equal(playback[0].(*pb.SCFightPlayback).OptRecord, randomOutcome.Operates) || !proto.Equal(playback[0].(*pb.SCFightPlayback).Attacker, randomInput.Attacker) || !proto.Equal(playback[0].(*pb.SCFightPlayback).Defender1, randomInput.Defender) {
		t.Fatal("original random child playback lost actual actor/child operation", err)
	}
	playback, err = s.fightPlayback(ctx, id, &pb.CSFightPlayback{FightId: proto.String(hpTransferRecordID), ServerId: proto.Int32(1)})
	if err != nil || !proto.Equal(playback[0].(*pb.SCFightPlayback).OptRecord, hpTransferOutcome.Operates) || !proto.Equal(playback[0].(*pb.SCFightPlayback).Attacker, hpTransferInput.Attacker) || !proto.Equal(playback[0].(*pb.SCFightPlayback).Defender1, hpTransferInput.Defender) {
		t.Fatal("original HP staircase transfer playback lost actual actor/child operation", err)
	}
	playback, err = s.fightPlayback(ctx, id, &pb.CSFightPlayback{FightId: proto.String(sourceShieldRecordID), ServerId: proto.Int32(1)})
	if err != nil || !proto.Equal(playback[0].(*pb.SCFightPlayback).OptRecord, sourceShieldOutcome.Operates) || !proto.Equal(playback[0].(*pb.SCFightPlayback).Attacker, sourceShieldInput.Attacker) || !proto.Equal(playback[0].(*pb.SCFightPlayback).Defender1, sourceShieldInput.Defender) {
		t.Fatal("original source-attack shield playback lost actual actor/child operation", err)
	}
	playback, err = s.fightPlayback(ctx, id, &pb.CSFightPlayback{FightId: proto.String(maxHPPercentRecordID), ServerId: proto.Int32(1)})
	if err != nil || !proto.Equal(playback[0].(*pb.SCFightPlayback).OptRecord, maxHPPercentOutcome.Operates) || !proto.Equal(playback[0].(*pb.SCFightPlayback).Attacker, maxHPPercentInput.Attacker) || !proto.Equal(playback[0].(*pb.SCFightPlayback).Defender1, maxHPPercentInput.Defender) {
		t.Fatal("original maxHP percent playback lost actual actor/child operation", err)
	}
	playback, err = s.fightPlayback(ctx, id, &pb.CSFightPlayback{FightId: proto.String(multibarRecordID), ServerId: proto.Int32(1)})
	if err != nil || !proto.Equal(playback[0].(*pb.SCFightPlayback).OptRecord, multibarOutcome.Operates) || !proto.Equal(playback[0].(*pb.SCFightPlayback).Attacker, multibarInput.Attacker) || !proto.Equal(playback[0].(*pb.SCFightPlayback).Defender2, multibarInput.Monster) {
		t.Fatal("multibar persisted playback lost multibar snapshot", err)
	}
	playback, err = s.fightPlayback(ctx, id, &pb.CSFightPlayback{FightId: proto.String(manualRecordID), ServerId: proto.Int32(1)})
	if err != nil || !proto.Equal(playback[0].(*pb.SCFightPlayback).OptRecord, manualOutcome.Operates) || !proto.Equal(playback[0].(*pb.SCFightPlayback).Attacker, manualInput.Attacker) {
		t.Fatal("manual persisted playback lost manual snapshot", err)
	}
	for _, record := range guardRecords {
		playback, err = s.fightPlayback(ctx, id, &pb.CSFightPlayback{FightId: proto.String(record.id), ServerId: proto.Int32(1)})
		if err != nil || !proto.Equal(playback[0].(*pb.SCFightPlayback).OptRecord, record.out.Operates) || !proto.Equal(playback[0].(*pb.SCFightPlayback).Attacker, record.input.Attacker) || !proto.Equal(playback[0].(*pb.SCFightPlayback).Defender2, record.input.Monster) {
			t.Fatal("native guardian snapshot/operation lost after reconnect", err)
		}
	}
	if nativeMainRecordID != "" {
		playback, err = s.fightPlayback(ctx, id, &pb.CSFightPlayback{FightId: proto.String(nativeMainRecordID), ServerId: proto.Int32(1)})
		if err != nil || !proto.Equal(playback[0].(*pb.SCFightPlayback).OptRecord, nativeMainOutcome.Operates) {
			t.Fatal("real native computed playback lost on reconnect", err)
		}
	}
	if nativeTask5RecordID != "" {
		playback, err = s.fightPlayback(ctx, id, &pb.CSFightPlayback{FightId: proto.String(nativeTask5RecordID), ServerId: proto.Int32(1)})
		if err != nil || !proto.Equal(playback[0].(*pb.SCFightPlayback).OptRecord, nativeTask5Outcome.Operates) {
			t.Fatal("real grown Asuna native playback lost on reconnect", err)
		}
	}
	for _, record := range transferRecords {
		playback, err = s.fightPlayback(ctx, id, &pb.CSFightPlayback{FightId: proto.String(record.id), ServerId: proto.Int32(1)})
		if err != nil || !proto.Equal(playback[0].(*pb.SCFightPlayback).OptRecord, record.out.Operates) || !proto.Equal(playback[0].(*pb.SCFightPlayback).Attacker, record.input.Attacker) || !proto.Equal(playback[0].(*pb.SCFightPlayback).Defender1, record.input.Defender) {
			t.Fatal("original property-transfer playback lost snapshot/operations on reconnect", err)
		}
	}
	for _, record := range blockRecords {
		playback, err = s.fightPlayback(ctx, id, &pb.CSFightPlayback{FightId: proto.String(record.id), ServerId: proto.Int32(1)})
		if err != nil || !proto.Equal(playback[0].(*pb.SCFightPlayback).OptRecord, record.out.Operates) || !proto.Equal(playback[0].(*pb.SCFightPlayback).Attacker, record.input.Attacker) || !proto.Equal(playback[0].(*pb.SCFightPlayback).Defender1, record.input.Defender) {
			t.Fatal("original block playback lost snapshot/operations on reconnect", err)
		}
	}
	for _, record := range roundHPRecords {
		playback, err = s.fightPlayback(ctx, id, &pb.CSFightPlayback{FightId: proto.String(record.id), ServerId: proto.Int32(1)})
		if err != nil || !proto.Equal(playback[0].(*pb.SCFightPlayback).OptRecord, record.out.Operates) || !proto.Equal(playback[0].(*pb.SCFightPlayback).Attacker, record.input.Attacker) || !proto.Equal(playback[0].(*pb.SCFightPlayback).Defender1, record.input.Defender) {
			t.Fatal("original round-maxHP playback lost snapshot/operations on reconnect", err)
		}
	}
	playback, err = s.fightPlayback(ctx, id, &pb.CSFightPlayback{FightId: proto.String(inertRecordID), ServerId: proto.Int32(1)})
	if err != nil || !proto.Equal(playback[0].(*pb.SCFightPlayback).OptRecord, inertOutcome.Operates) || !proto.Equal(playback[0].(*pb.SCFightPlayback).Attacker, inertInput.Attacker) || !proto.Equal(playback[0].(*pb.SCFightPlayback).Defender1, inertInput.Defender) {
		t.Fatal("original buff722 playback lost native class0 snapshot on reconnect", err)
	}
	for _, record := range dispelRecords {
		playback, err = s.fightPlayback(ctx, id, &pb.CSFightPlayback{FightId: proto.String(record.id), ServerId: proto.Int32(1)})
		if err != nil || !proto.Equal(playback[0].(*pb.SCFightPlayback).OptRecord, record.out.Operates) || !proto.Equal(playback[0].(*pb.SCFightPlayback).Attacker, record.input.Attacker) || !proto.Equal(playback[0].(*pb.SCFightPlayback).Defender1, record.input.Defender) {
			t.Fatal("original dispel playback lost snapshot/operations on reconnect", err)
		}
	}
	for _, record := range angerCompoundRecords {
		playback, err = s.fightPlayback(ctx, id, &pb.CSFightPlayback{FightId: proto.String(record.id), ServerId: proto.Int32(1)})
		if err != nil || !proto.Equal(playback[0].(*pb.SCFightPlayback).OptRecord, record.out.Operates) || !proto.Equal(playback[0].(*pb.SCFightPlayback).Attacker, record.input.Attacker) || !proto.Equal(playback[0].(*pb.SCFightPlayback).Defender1, record.input.Defender) {
			t.Fatal("original anger compound playback lost snapshot/operations on reconnect", err)
		}
	}
	for _, record := range damageTransferRecords {
		playback, err = s.fightPlayback(ctx, id, &pb.CSFightPlayback{FightId: proto.String(record.id), ServerId: proto.Int32(1)})
		if err != nil || !proto.Equal(playback[0].(*pb.SCFightPlayback).OptRecord, record.out.Operates) || !proto.Equal(playback[0].(*pb.SCFightPlayback).Attacker, record.input.Attacker) || !proto.Equal(playback[0].(*pb.SCFightPlayback).Defender1, record.input.Defender) {
			t.Fatal("original damage-transfer playback lost snapshot/operations on reconnect", err)
		}
	}

	for _, record := range lastAttackRecords {
		playback, err = s.fightPlayback(ctx, id, &pb.CSFightPlayback{FightId: proto.String(record.id), ServerId: proto.Int32(1)})
		if err != nil || !proto.Equal(playback[0].(*pb.SCFightPlayback).OptRecord, record.out.Operates) || !proto.Equal(playback[0].(*pb.SCFightPlayback).Attacker, record.input.Attacker) || !proto.Equal(playback[0].(*pb.SCFightPlayback).Defender1, record.input.Defender) {
			t.Fatal("original last-attack playback lost snapshot/operations on reconnect", err)
		}
	}

	for _, record := range sameDamageRecords {
		playback, err = s.fightPlayback(ctx, id, &pb.CSFightPlayback{FightId: proto.String(record.id), ServerId: proto.Int32(1)})
		if err != nil || !proto.Equal(playback[0].(*pb.SCFightPlayback).OptRecord, record.out.Operates) || !proto.Equal(playback[0].(*pb.SCFightPlayback).Attacker, record.input.Attacker) || !proto.Equal(playback[0].(*pb.SCFightPlayback).Defender1, record.input.Defender) {
			t.Fatal("original same-damage playback lost snapshot/operations on reconnect", err)
		}
	}

	for _, record := range skillReplacementRecords {
		playback, err = s.fightPlayback(ctx, id, &pb.CSFightPlayback{FightId: proto.String(record.id), ServerId: proto.Int32(1)})
		if err != nil || !proto.Equal(playback[0].(*pb.SCFightPlayback).OptRecord, record.out.Operates) || !proto.Equal(playback[0].(*pb.SCFightPlayback).Attacker, record.input.Attacker) || !proto.Equal(playback[0].(*pb.SCFightPlayback).Defender1, record.input.Defender) {
			t.Fatal("original skill-replacement playback lost snapshot/operations on reconnect", err)
		}
	}

	for _, record := range reboundRecords {
		playback, err = s.fightPlayback(ctx, id, &pb.CSFightPlayback{FightId: proto.String(record.id), ServerId: proto.Int32(1)})
		if err != nil || !proto.Equal(playback[0].(*pb.SCFightPlayback).OptRecord, record.out.Operates) || !proto.Equal(playback[0].(*pb.SCFightPlayback).Attacker, record.input.Attacker) || !proto.Equal(playback[0].(*pb.SCFightPlayback).Defender1, record.input.Defender) {
			t.Fatal("original rebound playback lost snapshot/operations on reconnect", err)
		}
	}

	for _, record := range shareRecords {
		playback, err = s.fightPlayback(ctx, id, &pb.CSFightPlayback{FightId: proto.String(record.id), ServerId: proto.Int32(1)})
		if err != nil || !proto.Equal(playback[0].(*pb.SCFightPlayback).OptRecord, record.out.Operates) || !proto.Equal(playback[0].(*pb.SCFightPlayback).Attacker, record.input.Attacker) || !proto.Equal(playback[0].(*pb.SCFightPlayback).Defender1, record.input.Defender) {
			t.Fatal("original share playback lost snapshot/operations on reconnect", err)
		}
	}

	for _, record := range unmappedProbeRecords {
		playback, err = s.fightPlayback(ctx, id, &pb.CSFightPlayback{FightId: proto.String(record.id), ServerId: proto.Int32(1)})
		if err != nil || !proto.Equal(playback[0].(*pb.SCFightPlayback).OptRecord, record.out.Operates) || !proto.Equal(playback[0].(*pb.SCFightPlayback).Attacker, record.input.Attacker) || !proto.Equal(playback[0].(*pb.SCFightPlayback).Defender1, record.input.Defender) {
			t.Fatal("original unmapped mechanism probe playback lost snapshot/operations on reconnect", err)
		}
	}

	for _, record := range hpAPProbeRecords {
		playback, err = s.fightPlayback(ctx, id, &pb.CSFightPlayback{FightId: proto.String(record.id), ServerId: proto.Int32(1)})
		if err != nil || !proto.Equal(playback[0].(*pb.SCFightPlayback).OptRecord, record.out.Operates) || !proto.Equal(playback[0].(*pb.SCFightPlayback).Attacker, record.input.Attacker) || !proto.Equal(playback[0].(*pb.SCFightPlayback).Defender1, record.input.Defender) {
			t.Fatal("original unmapped HP/AP probe playback lost snapshot/operations on reconnect", err)
		}
	}
	for _, record := range roundExtraProbeRecords {
		playback, err = s.fightPlayback(ctx, id, &pb.CSFightPlayback{FightId: proto.String(record.id), ServerId: proto.Int32(1)})
		if err != nil || !proto.Equal(playback[0].(*pb.SCFightPlayback).OptRecord, record.out.Operates) || !proto.Equal(playback[0].(*pb.SCFightPlayback).Attacker, record.input.Attacker) || !proto.Equal(playback[0].(*pb.SCFightPlayback).Defender1, record.input.Defender) {
			t.Fatal("original unmapped round-extra probe playback lost snapshot/operations on reconnect", err)
		}
	}
	for _, record := range hpChangeRecords {
		playback, err = s.fightPlayback(ctx, id, &pb.CSFightPlayback{FightId: proto.String(record.id), ServerId: proto.Int32(1)})
		if err != nil || !proto.Equal(playback[0].(*pb.SCFightPlayback).OptRecord, record.out.Operates) || !proto.Equal(playback[0].(*pb.SCFightPlayback).Attacker, record.input.Attacker) || !proto.Equal(playback[0].(*pb.SCFightPlayback).Defender1, record.input.Defender) {
			t.Fatal("original actual emergency recovery playback lost snapshots/operations on reconnect", err)
		}
	}
	for _, record := range conditionAndNativeRecords {
		playback, err = s.fightPlayback(ctx, id, &pb.CSFightPlayback{FightId: proto.String(record.id), ServerId: proto.Int32(1)})
		if err != nil || !proto.Equal(playback[0].(*pb.SCFightPlayback).OptRecord, record.out.Operates) || !proto.Equal(playback[0].(*pb.SCFightPlayback).Attacker, record.input.Attacker) || !proto.Equal(playback[0].(*pb.SCFightPlayback).Defender1, record.input.Defender) || !proto.Equal(playback[0].(*pb.SCFightPlayback).Defender2, record.input.Monster) {
			t.Fatal("actual original remaining conditions/native Daily replay lost snapshots/operations on reconnect", err)
		}
	}
	for _, legacyID := range legacyIDs {
		playback, err = s.fightPlayback(ctx, id, &pb.CSFightPlayback{FightId: proto.String(legacyID), ServerId: proto.Int32(1)})
		if err != nil || !proto.Equal(playback[0].(*pb.SCFightPlayback).OptRecord, legacyOperate) {
			t.Fatal("legacy native completed playback lost", legacyID, err)
		}
	}
}
