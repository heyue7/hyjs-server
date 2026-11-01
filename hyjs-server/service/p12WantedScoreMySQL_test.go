package service

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"math"
	"math/big"
	"os"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/go-sql-driver/mysql"
	"github.com/google/uuid"
	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

type wantedScoreMySQLActorKey struct{}

type wantedScoreMySQLFight struct {
	playerID int64
	input    model.FrozenBattleInput
	terminal model.CombatFinalHPOutcome
	request  *pb.CSRebelFightEnd
	paid     *pb.Player
	legacy   bool
}

type wantedScoreMySQLObserved struct {
	playerID int64
	instance db.P12WantedInstance
}

type wantedScoreMySQLResult struct {
	playerID int64
	reply    *pb.SCRebelFightEnd
	err      error
}

func wantedScoreMySQLProfile(t *testing.T, id int64) *pb.Player {
	t.Helper()
	var row db.Profile
	if err := db.GetIns().First(&row, id).Error; err != nil {
		t.Fatal(err)
	}
	player, _, err := decodeProfile(&row)
	if err != nil {
		t.Fatal(err)
	}
	return player
}

// Two independent profiles use the same controlled captured roster; the second
// has its pet slot removed. This fixture does not claim natural acquisition.
func wantedScoreMySQLSecondPlayer(t *testing.T, f *p1Fixture) int64 {
	t.Helper()
	var peer db.Profile
	if err := db.GetIns().Where("id<>? AND uid LIKE ?", f.id, "p7-"+t.Name()+"%").First(&peer).Error; err != nil {
		t.Fatal("isolated second profile", err)
	}
	owner, roster := f.profile(t)
	if err := updateProfile(f.ctx, peer.Id, func(p *pb.Player, g *model.GameState) error {
		p.Level = proto.Int32(owner.GetLevel())
		p.HeroWarId = proto.String(owner.GetHeroWarId())
		g.Warriors = nil
		for _, warrior := range roster.Warriors {
			g.Warriors = append(g.Warriors, proto.Clone(warrior).(*pb.Warrior))
		}
		g.Equipment = nil
		for _, equipment := range roster.Equipment {
			g.Equipment = append(g.Equipment, proto.Clone(equipment).(*pb.Equipment))
		}
		g.Formation = nil
		for _, source := range roster.Formation {
			slot := proto.Clone(source).(*pb.FormationItem)
			things := slot.Things[:0]
			for _, thing := range slot.Things {
				if thing.GetPos() != 7 {
					things = append(things, thing)
				}
			}
			slot.Things = things
			g.Formation = append(g.Formation, slot)
		}
		return nil
	}); err != nil {
		t.Fatal("isolated peer formation", err)
	}
	return peer.Id
}

func wantedScoreMySQLSharedInstance(t *testing.T, f *p1Fixture, now time.Time) *db.P12WantedInstance {
	t.Helper()
	var policy db.P12WantedPolicy
	if err := db.GetIns().First(&policy, 1).Error; err != nil {
		t.Fatal(err)
	}
	var definition p12WantedDefinition
	if err := json.Unmarshal([]byte(policy.DefinitionJSON), &definition); err != nil {
		t.Fatal(err)
	}
	var wantedID string
	for _, row := range definition.Tables["WantedLevel"] {
		if row["Level"] == "35" && row["Difficulty"] == "3" {
			wantedID = row["WantedID"]
			break
		}
	}
	if wantedID == "" {
		t.Fatal("missing original level-35 difficulty-3 boss")
	}
	_, bodies, maximum, err := f.s.p12WantedMonster(definition, wantedID, nil)
	if err != nil || len(bodies) != 4 {
		t.Fatal("original shared four-body boss", err)
	}
	hp := make(map[string]float64, len(bodies))
	for _, body := range bodies {
		hp[body.GetWantedEnemyId()] = body.GetHp()
	}
	rawHP, err := json.Marshal(hp)
	if err != nil {
		t.Fatal(err)
	}
	instance := &db.P12WantedInstance{
		ID: "score-mysql-" + uuid.NewString(), ServerID: f.s.OriginalServerID(),
		FinderID: f.id, FinderName: "isolated score lock fixture", WantedID: wantedID,
		Level: 35, Difficulty: 3, State: constUtil.P12WantedActive,
		CreatedAt: now.UnixMilli(), ExpiresAt: now.Add(30 * time.Minute).UnixMilli(), SharedAt: now.UnixMilli(),
		CurrentHP: maximum, MaxHP: maximum, MonsterHPJSON: string(rawHP),
		DefinitionJSON: policy.DefinitionJSON, DefinitionHash: policy.DefinitionHash,
	}
	if err := db.GetIns().Create(instance).Error; err != nil {
		t.Fatal(err)
	}
	return instance
}

func wantedScoreMySQLStart(t *testing.T, f *p1Fixture, playerID int64, instance *db.P12WantedInstance, stamp int64, now time.Time, legacy bool) wantedScoreMySQLFight {
	t.Helper()
	request := &pb.CSRebelFightStart{RebelId: proto.String(instance.ID), Type: proto.Int32(1)}
	ctx := context.WithValue(f.ctx, constUtil.GameRequestContextKey, request)
	started, err := f.s.p12WantedStart(ctx, playerID, request, stamp, now)
	if err != nil {
		t.Fatal("real paid start", playerID, err)
	}
	start := started[0].(*pb.SCRebelFightStart)
	var row db.P12WantedFight
	if err := db.GetIns().First(&row, "id=?", start.GetFightId()).Error; err != nil {
		t.Fatal(err)
	}
	fight := wantedScoreMySQLFight{playerID: playerID, paid: wantedScoreMySQLProfile(t, playerID), legacy: legacy}
	if json.Unmarshal([]byte(row.InputJSON), &fight.input) != nil || json.Unmarshal([]byte(row.TerminalJSON), &fight.terminal) != nil {
		t.Fatal("missing certified start input or terminal")
	}
	indices := fight.input.AppliedProperties["wantedSettlementIndices"]
	if len(indices) != 4 || indices[1] != "10000" || indices[2] != constUtil.P12WantedScoreRuleV2 || indices[3] != "3" {
		t.Fatal("new fight did not freeze difficulty pool", indices)
	}
	if legacy {
		// Recreate an already-paid v1 snapshot in the disposable database and
		// re-certify its terminal against that exact historical input.
		fight.input.AppliedProperties["wantedSettlementIndices"] = []string{indices[0], "50", constUtil.P12WantedRewardRule}
		terminal, err := f.s.ComputeP12WantedProtocolFinalHP(f.ctx, &fight.input)
		if err != nil {
			t.Fatal("certify historical v1 pending fight", err)
		}
		fight.terminal = *terminal
		inputJSON, err := json.Marshal(&fight.input)
		if err != nil {
			t.Fatal(err)
		}
		terminalJSON, err := json.Marshal(&fight.terminal)
		if err != nil {
			t.Fatal(err)
		}
		if err := db.GetIns().Model(&row).Updates(map[string]any{
			"input_json": string(inputJSON), "input_hash": p6Digest(inputJSON),
			"terminal_json": string(terminalJSON), "terminal_hash": p6Digest(terminalJSON),
		}).Error; err != nil {
			t.Fatal(err)
		}
	}
	if fight.terminal.Outcome == nil || fight.terminal.OriginalOperates == nil {
		t.Fatal("certified terminal missing")
	}
	result := int32(0)
	if fight.terminal.Outcome.Winner == 0 {
		result = 1
	}
	fight.request = &pb.CSRebelFightEnd{FightId: start.FightId, Result: proto.Int32(result), OptRecord: fight.terminal.OriginalOperates}
	return fight
}

// Reference the documented exact-float quantization independently of the
// settlement helper, then calculate the per-body effective clipped loss.
func wantedScoreMySQLUnits(value float64) *big.Int {
	scaled := new(big.Rat).Mul(new(big.Rat).SetFloat64(value), big.NewRat(1000000, 1))
	numerator := new(big.Int).Add(scaled.Num(), new(big.Int).Sub(scaled.Denom(), big.NewInt(1)))
	return numerator.Quo(numerator, scaled.Denom())
}

func wantedScoreMySQLExpected(t *testing.T, fight wantedScoreMySQLFight, instance db.P12WantedInstance) (map[string]float64, float64, int64) {
	t.Helper()
	before := map[string]float64{}
	if err := json.Unmarshal([]byte(instance.MonsterHPJSON), &before); err != nil {
		t.Fatal(err)
	}
	after := make(map[string]float64, len(before))
	actors := map[string]*pb.BattleHero{}
	for _, actor := range fight.input.Monster.Formation {
		actors[actor.GetGuid()] = actor
	}
	var applied float64
	for _, final := range fight.terminal.FinalActors {
		if final.Camp != 1 {
			continue
		}
		actor := actors[final.GUID]
		if actor == nil || !strings.HasPrefix(final.GUID, "wanted-") {
			t.Fatal("unexpected certified boss body", final.GUID)
		}
		key := "wanted_" + strings.TrimPrefix(final.GUID, "wanted-")
		old, ok := before[key]
		if !ok {
			t.Fatal("missing authoritative body", key)
		}
		damage := math.Min(old, math.Max(0, actor.GetInst().GetCurHp()-final.CurrentHP))
		after[key] = old - damage
		applied += damage
	}
	if len(after) != 4 || applied <= 0 {
		t.Fatal("each participant must remove real shared HP", applied, after)
	}
	if fight.legacy {
		return after, applied, int64(math.Floor(applied / 50))
	}
	var lost, maximum big.Int
	for _, actor := range fight.input.Monster.Formation {
		key := "wanted_" + strings.TrimPrefix(actor.GetGuid(), "wanted-")
		oldUnits := wantedScoreMySQLUnits(before[key])
		newUnits := wantedScoreMySQLUnits(after[key])
		lost.Add(&lost, new(big.Int).Sub(oldUnits, newUnits))
		maximum.Add(&maximum, wantedScoreMySQLUnits(actor.GetProp().GetHp()))
	}
	ratio := new(big.Rat).SetFrac(&lost, &maximum)
	ratio.Mul(ratio, big.NewRat(10000, 1))
	return after, applied, new(big.Int).Quo(ratio.Num(), ratio.Denom()).Int64()
}

func wantedScoreMySQLTerminalDamage(t *testing.T, fight wantedScoreMySQLFight) map[string]float64 {
	t.Helper()
	actors := map[string]*pb.BattleHero{}
	for _, actor := range fight.input.Monster.Formation {
		actors[actor.GetGuid()] = actor
	}
	damage := map[string]float64{}
	for _, final := range fight.terminal.FinalActors {
		if final.Camp != 1 {
			continue
		}
		actor := actors[final.GUID]
		if actor == nil || !strings.HasPrefix(final.GUID, "wanted-") {
			t.Fatal("unexpected certified boss body", final.GUID)
		}
		key := "wanted_" + strings.TrimPrefix(final.GUID, "wanted-")
		damage[key] = math.Max(0, actor.GetInst().GetCurHp()-final.CurrentHP)
	}
	if len(damage) != 4 {
		t.Fatal("certified damage missing a boss body")
	}
	return damage
}

// After both genuine paid starts, set a controlled intervening shared-HP
// prerequisite in this disposable fixture. At least one body now has less HP
// than the two frozen terminals together, making overdraw clipping observable.
func wantedScoreMySQLPrepareOverlap(t *testing.T, instance *db.P12WantedInstance, fights []wantedScoreMySQLFight) (float64, bool) {
	t.Helper()
	hp := map[string]float64{}
	if err := json.Unmarshal([]byte(instance.MonsterHPJSON), &hp); err != nil {
		t.Fatal(err)
	}
	damageA := wantedScoreMySQLTerminalDamage(t, fights[0])
	damageB := wantedScoreMySQLTerminalDamage(t, fights[1])
	var rawTotal float64
	for key := range hp {
		rawTotal += damageA[key] + damageB[key]
	}
	prepared := false
	for _, actor := range fights[0].input.Monster.Formation {
		key := "wanted_" + strings.TrimPrefix(actor.GetGuid(), "wanted-")
		a, b := damageA[key], damageB[key]
		if a <= 0 || b <= 0 {
			continue
		}
		remaining := math.Min(hp[key], math.Max(a, b)+math.Min(a, b)/2)
		if remaining <= 0 || remaining >= a+b {
			continue
		}
		hp[key] = remaining
		prepared = true
		break
	}
	if !prepared {
		return 0, false
	}
	var total float64
	for _, actor := range fights[0].input.Monster.Formation {
		key := "wanted_" + strings.TrimPrefix(actor.GetGuid(), "wanted-")
		total += hp[key]
	}
	rawHP, err := json.Marshal(hp)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.GetIns().Model(instance).Updates(map[string]any{"monster_hp_json": string(rawHP), "current_hp": total}).Error; err != nil {
		t.Fatal(err)
	}
	instance.MonsterHPJSON, instance.CurrentHP = string(rawHP), total
	return rawTotal, true
}

func wantedScoreMySQLRunRace(t *testing.T, f *p1Fixture, instance *db.P12WantedInstance, fights []wantedScoreMySQLFight, rawDamage float64, now time.Time) []wantedScoreMySQLResult {
	t.Helper()
	database := db.GetIns()
	reached := make(chan int64, 2)
	acquired := make(chan wantedScoreMySQLObserved, 2)
	callback := "wanted-score-mysql-instance-lock"
	match := func(tx *gorm.DB) (int64, bool) {
		playerID, ok := tx.Statement.Context.Value(wantedScoreMySQLActorKey{}).(int64)
		lock, locked := tx.Statement.Clauses["FOR"].Expression.(clause.Locking)
		return playerID, ok && tx.Statement.Table == "p12_wanted_instances" && locked && lock.Strength == "UPDATE"
	}
	if err := database.Callback().Query().Before("gorm:query").Register(callback+":before", func(tx *gorm.DB) {
		if playerID, ok := match(tx); ok {
			reached <- playerID
		}
	}); err != nil {
		t.Fatal(err)
	}
	if err := database.Callback().Query().After("gorm:query").Register(callback+":after", func(tx *gorm.DB) {
		if playerID, ok := match(tx); ok && tx.Error == nil {
			if row, valid := tx.Statement.Dest.(*db.P12WantedInstance); valid {
				acquired <- wantedScoreMySQLObserved{playerID: playerID, instance: *row}
			}
		}
	}); err != nil {
		database.Callback().Query().Remove(callback + ":before")
		t.Fatal(err)
	}
	holder := database.Begin()
	if holder.Error != nil {
		database.Callback().Query().Remove(callback + ":after")
		database.Callback().Query().Remove(callback + ":before")
		t.Fatal(holder.Error)
	}
	ctx, cancel := context.WithTimeout(f.ctx, 45*time.Second)
	var wg sync.WaitGroup
	defer func() {
		cancel()
		holder.Rollback()
		wg.Wait()
		database.Callback().Query().Remove(callback + ":after")
		database.Callback().Query().Remove(callback + ":before")
	}()
	var locked db.P12WantedInstance
	if err := holder.Clauses(clause.Locking{Strength: "UPDATE"}).First(&locked, "id=?", instance.ID).Error; err != nil {
		t.Fatal("hold instance row", err)
	}
	// A distinct MySQL connection must fail NOWAIT against this exact key.
	var probe struct{ ID string }
	err := database.WithContext(ctx).Raw("SELECT id FROM p12_wanted_instances WHERE id=? FOR UPDATE NOWAIT", instance.ID).Scan(&probe).Error
	var mysqlErr *mysql.MySQLError
	if !errors.As(err, &mysqlErr) || mysqlErr.Number != 3572 {
		t.Fatal("InnoDB did not hold the targeted instance row", err)
	}
	results := make(chan wantedScoreMySQLResult, 2)
	for _, fight := range fights {
		fight := fight
		wg.Add(1)
		go func() {
			defer wg.Done()
			requestCtx := context.WithValue(ctx, wantedScoreMySQLActorKey{}, fight.playerID)
			requestCtx = context.WithValue(requestCtx, constUtil.GameRequestContextKey, fight.request)
			messages, err := f.s.p12WantedEnd(requestCtx, fight.playerID, fight.request, now)
			result := wantedScoreMySQLResult{playerID: fight.playerID, err: err}
			if err == nil && len(messages) > 0 {
				result.reply, _ = messages[0].(*pb.SCRebelFightEnd)
			}
			results <- result
		}()
	}
	select {
	case <-reached:
	case <-time.After(20 * time.Second):
		t.Fatal("neither real settlement reached its instance FOR UPDATE")
	}
	select {
	case result := <-results:
		t.Fatal("settlement returned while instance was held", result.err)
	case <-time.After(250 * time.Millisecond):
	}
	if err := holder.Commit().Error; err != nil {
		t.Fatal("release instance lock", err)
	}
	got := make(map[int64]wantedScoreMySQLResult, 2)
	for len(got) < 2 {
		select {
		case result := <-results:
			if result.err != nil || result.reply == nil {
				t.Fatal("real concurrent End failed", result.playerID, result.err)
			}
			got[result.playerID] = result
		case <-ctx.Done():
			t.Fatal("concurrent settlements did not complete", ctx.Err())
		}
	}
	observed := make([]wantedScoreMySQLObserved, 0, 2)
	for len(observed) < 2 {
		select {
		case event := <-acquired:
			observed = append(observed, event)
		case <-ctx.Done():
			t.Fatal("missing real instance-lock observations", ctx.Err())
		}
	}
	if observed[0].playerID == observed[1].playerID || observed[0].instance.ID != instance.ID || observed[1].instance.ID != instance.ID || observed[0].instance.MonsterHPJSON != instance.MonsterHPJSON {
		t.Fatal("unexpected instance lock order or initial snapshot", observed[0].playerID, observed[1].playerID, observed[0].instance.ID, observed[1].instance.ID)
	}
	byPlayer := map[int64]wantedScoreMySQLFight{}
	for _, fight := range fights {
		byPlayer[fight.playerID] = fight
	}
	var total int64
	var totalApplied float64
	var priorAfter map[string]float64
	orderedResults := make([]wantedScoreMySQLResult, 0, 2)
	for _, event := range observed {
		fight, ok := byPlayer[event.playerID]
		if !ok {
			t.Fatal("lock observed for unexpected participant", event.playerID)
		}
		if priorAfter != nil {
			current := map[string]float64{}
			if json.Unmarshal([]byte(event.instance.MonsterHPJSON), &current) != nil || !reflect.DeepEqual(current, priorAfter) {
				t.Fatal("second transaction did not read first committed body HP", current, priorAfter)
			}
		}
		after, applied, score := wantedScoreMySQLExpected(t, fight, event.instance)
		result := got[event.playerID]
		if result.reply.GetDamage() != applied || int64(result.reply.GetScoreAdd()) != score || score <= 0 {
			t.Fatal("actual damage or individually earned score differs", event.playerID, applied, score, result.reply)
		}
		current := wantedScoreMySQLProfile(t, event.playerID)
		if int64(current.GetScore())-int64(fight.paid.GetScore()) != score || current.GetWantedAttTimes() != fight.paid.GetWantedAttTimes() {
			t.Fatal("score or paid attempts were not committed once", event.playerID)
		}
		var saved db.P12WantedFight
		if err := database.First(&saved, "id=?", fight.request.GetFightId()).Error; err != nil || !saved.Committed || saved.Score != score || saved.AppliedDamage != applied {
			t.Fatal("fight receipt differs", event.playerID, err)
		}
		if n := p1Count(t, &db.EventFact{}, "player_id=? AND source_key=? AND kind=?", event.playerID, "wanted:attack:"+saved.ID, "wanted_attack"); n != 1 {
			t.Fatal("attack event was not recorded exactly once", event.playerID, n)
		}
		priorAfter = after
		total += score
		totalApplied += applied
		orderedResults = append(orderedResults, result)
	}
	if totalApplied >= rawDamage {
		t.Fatal("shared HP contention did not exercise actual overdraw clipping", totalApplied, rawDamage)
	}
	var final db.P12WantedInstance
	if err := database.First(&final, "id=?", instance.ID).Error; err != nil {
		t.Fatal(err)
	}
	last := map[string]float64{}
	if json.Unmarshal([]byte(final.MonsterHPJSON), &last) != nil || !reflect.DeepEqual(last, priorAfter) {
		t.Fatal("persisted shared HP does not equal both clipped deductions", last, priorAfter)
	}
	if !fights[0].legacy && !fights[1].legacy && total > 10000 {
		t.Fatal("v2 participants exceeded their frozen boss score pool", total)
	}
	t.Logf("instance lock serialized roles; scores=%d,%d v1/v2=%t/%t", orderedResults[0].reply.GetScoreAdd(), orderedResults[1].reply.GetScoreAdd(), fights[0].legacy, fights[1].legacy)
	return orderedResults
}

// Requires a fresh, task-owned hyjs_test_p7_* MySQL schema supplied by the
// local-services manager. No service, database or credentials are created here.
func TestP12WantedScoreMySQLSharedBossAndLegacy(t *testing.T) {
	name := os.Getenv("HYJS_P7_MYSQL_DATABASE")
	if name == "" {
		t.Skip("requires an independently leased InnoDB database")
	}
	if !strings.HasPrefix(name, "hyjs_test_p7_") || os.Getenv("HYJS_GAME_ACCOUNTS_DATABASE") != "" {
		t.Fatal("requires task-owned P7 test schema and no external accounts schema")
	}
	if os.Getenv("HYJS_P7_MYSQL_HOST") != "127.0.0.1:3306" {
		t.Fatal("requires the allocated local MySQL endpoint 127.0.0.1:3306")
	}
	// p7Setup intentionally resets fixture tables; require a brand-new schema
	// before it can create or delete anything.
	config := mysql.NewConfig()
	config.Net, config.Addr, config.DBName = "tcp", os.Getenv("HYJS_P7_MYSQL_HOST"), name
	config.User, config.Passwd = os.Getenv("HYJS_P7_MYSQL_USER"), os.Getenv("HYJS_P7_MYSQL_PASSWORD")
	config.Timeout, config.ReadTimeout, config.WriteTimeout = 5*time.Second, 10*time.Second, 10*time.Second
	if config.User == "" || config.Passwd == "" {
		t.Fatal("missing isolated MySQL test credentials")
	}
	preflight, err := sql.Open("mysql", config.FormatDSN())
	if err != nil {
		t.Fatal("open isolated MySQL preflight", err)
	}
	var existing int
	err = preflight.QueryRow("SELECT COUNT(*) FROM information_schema.TABLES WHERE TABLE_SCHEMA=DATABASE()").Scan(&existing)
	closeErr := preflight.Close()
	if err != nil || closeErr != nil || existing != 0 {
		t.Fatal("test requires an empty task-owned schema", existing, err, closeErr)
	}
	f := p12WantedPersistenceFixture(t)
	if db.GetIns().Dialector.Name() != "mysql" {
		t.Fatal("actual InnoDB is required")
	}
	sqlDB, err := db.GetIns().DB()
	if err != nil {
		t.Fatal(err)
	}
	sqlDB.SetMaxOpenConns(6)
	sqlDB.SetMaxIdleConns(3)
	var engines []struct{ TableName, Engine string }
	if err := db.GetIns().Raw("SELECT TABLE_NAME,ENGINE FROM information_schema.TABLES WHERE TABLE_SCHEMA=DATABASE() AND TABLE_NAME IN ('profiles','p12_wanted_instances','p12_wanted_fights')").Scan(&engines).Error; err != nil {
		t.Fatal(err)
	}
	if len(engines) != 3 {
		t.Fatal("missing InnoDB tables", engines)
	}
	for _, table := range engines {
		if table.Engine != "InnoDB" {
			t.Fatal("transaction table is not InnoDB", table.TableName, table.Engine)
		}
	}
	peer := wantedScoreMySQLSecondPlayer(t, f)
	now := time.Now()
	for _, id := range []int64{f.id, peer} {
		if _, err := f.s.p12WantedList(f.ctx, id, now); err != nil {
			t.Fatal("initialize original wanted attempts", id, err)
		}
	}
	type replay struct {
		fight    wantedScoreMySQLFight
		receipt  *pb.SCRebelFightEnd
		instance string
	}
	var replays []replay
	for round, mixed := range []bool{false, true} {
		var instance *db.P12WantedInstance
		var fights []wantedScoreMySQLFight
		var rawDamage float64
		var prepared bool
		for attempt := 0; attempt < 4; attempt++ {
			instance = wantedScoreMySQLSharedInstance(t, f, now)
			stampOffset := int64(round*10 + attempt)
			fights = []wantedScoreMySQLFight{
				wantedScoreMySQLStart(t, f, f.id, instance, 945000+stampOffset, now, mixed),
				wantedScoreMySQLStart(t, f, peer, instance, 946000+stampOffset, now, false),
			}
			rawDamage, prepared = wantedScoreMySQLPrepareOverlap(t, instance, fights)
			if prepared {
				break
			}
			// Real target selection may yield disjoint bodies. Complete those
			// paid fights normally, then draw another bounded fixture.
			for _, fight := range fights {
				ctx := context.WithValue(f.ctx, constUtil.GameRequestContextKey, fight.request)
				if _, err := f.s.p12WantedEnd(ctx, fight.playerID, fight.request, now); err != nil {
					t.Fatal("complete non-overlapping fixture", err)
				}
			}
			t.Log("certified attacks had disjoint boss bodies; retrying bounded fixture")
		}
		if !prepared {
			t.Fatal("four real fight pairs had no overlapping body damage")
		}
		results := wantedScoreMySQLRunRace(t, f, instance, fights, rawDamage, now)
		for _, result := range results {
			for _, fight := range fights {
				if fight.playerID == result.playerID {
					replays = append(replays, replay{fight: fight, receipt: result.reply, instance: instance.ID})
				}
			}
		}
	}
	before := map[int64]*pb.Player{f.id: wantedScoreMySQLProfile(t, f.id), peer: wantedScoreMySQLProfile(t, peer)}
	instances := map[string]db.P12WantedInstance{}
	for _, item := range replays {
		if _, ok := instances[item.instance]; !ok {
			var row db.P12WantedInstance
			if err := db.GetIns().First(&row, "id=?", item.instance).Error; err != nil {
				t.Fatal(err)
			}
			instances[item.instance] = row
		}
	}
	f.restart(t)
	coldDB, err := db.GetIns().DB()
	if err != nil {
		t.Fatal(err)
	}
	coldDB.SetMaxOpenConns(3)
	coldDB.SetMaxIdleConns(2)
	for _, item := range replays {
		ctx := context.WithValue(f.ctx, constUtil.GameRequestContextKey, item.fight.request)
		result, err := f.s.p12WantedEnd(ctx, item.fight.playerID, item.fight.request, now)
		if err != nil || len(result) == 0 || !proto.Equal(result[0], item.receipt) {
			t.Fatal("cold identical End did not replay original receipt", item.fight.playerID, err)
		}
		conflict := proto.Clone(item.fight.request).(*pb.CSRebelFightEnd)
		conflict.Result = proto.Int32(1 - conflict.GetResult())
		conflictCtx := context.WithValue(f.ctx, constUtil.GameRequestContextKey, conflict)
		if _, err := f.s.p12WantedEnd(conflictCtx, item.fight.playerID, conflict, now); err == nil {
			t.Fatal("different replay accepted", item.fight.playerID)
		}
		for _, event := range []struct{ prefix, kind string }{{"wanted:attack:", "wanted_attack"}, {"wanted:damage:", "wanted_damage"}} {
			if n := p1Count(t, &db.EventFact{}, "player_id=? AND source_key=? AND kind=?", item.fight.playerID, event.prefix+item.fight.request.GetFightId(), event.kind); n != 1 {
				t.Fatal("cold replay changed event count", item.fight.playerID, event.kind, n)
			}
		}
	}
	for id, expected := range before {
		if actual := wantedScoreMySQLProfile(t, id); !proto.Equal(actual, expected) {
			t.Fatal("cold receipt replay changed profile", id)
		}
	}
	for id, expected := range instances {
		var actual db.P12WantedInstance
		if err := db.GetIns().First(&actual, "id=?", id).Error; err != nil || actual.MonsterHPJSON != expected.MonsterHPJSON || actual.CurrentHP != expected.CurrentHP {
			t.Fatal("cold receipt replay changed shared HP", id, err)
		}
	}
	if len(replays) != 4 {
		t.Fatalf("expected four committed fight receipts, got %d", len(replays))
	}
}
