package service

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"
	"unicode"

	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

func p11ArenaRound2MySQLGuard(t *testing.T) {
	t.Helper()
	name := os.Getenv("HYJS_P11_MYSQL_DATABASE")
	if name == "" {
		t.Skip("requires an isolated hyjs_test_p11_ MySQL database")
	}
	if !strings.HasPrefix(name, "hyjs_test_p11_") || len(name) == len("hyjs_test_p11_") || os.Getenv("HYJS_GAME_ACCOUNTS_DATABASE") != "" {
		t.Fatal("requires an isolated game and accounts fixture")
	}
}

type p11ArenaRound2QueryKey struct{}
type p11ArenaRound2QueryTag struct {
	phase string
	id    int64
}

type p11ArenaRound2ProfileRead struct {
	sql string
	ids []int64
}

type p11ArenaRound2Audit struct {
	mu                sync.Mutex
	profileReads      map[string][]p11ArenaRound2ProfileRead
	policyShare       map[string]int
	policyExclusive   map[string]int
	policyWrites      map[string]int
	allocatorLock     map[string]int
	allocatorWrites   map[string]int
	allocatorValues   map[string][]int64
	unexpectedProfile []string
}

func newP11ArenaRound2Audit() *p11ArenaRound2Audit {
	return &p11ArenaRound2Audit{
		profileReads:    map[string][]p11ArenaRound2ProfileRead{},
		policyShare:     map[string]int{},
		policyExclusive: map[string]int{},
		policyWrites:    map[string]int{},
		allocatorLock:   map[string]int{},
		allocatorWrites: map[string]int{},
		allocatorValues: map[string][]int64{},
	}
}

func p11ArenaRound2FullProfileSelect(statement string) bool {
	s := strings.ToLower(statement)
	parts := strings.SplitN(s, " from ", 2)
	if len(parts) != 2 || strings.HasPrefix(strings.TrimSpace(parts[0]), "select count(") {
		return false
	}
	if strings.Contains(parts[0], "*") {
		return true
	}
	fields := strings.FieldsFunc(parts[0], func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r) && r != '_'
	})
	for _, field := range fields {
		if field == "player" || field == "game" {
			return true
		}
	}
	return false
}

func TestP11ArenaConcurrencyRound2MySQLStablePolicyReaders(t *testing.T) {
	p11ArenaRound2MySQLGuard(t)
	f, peer, unrelated := p11ArenaSetup(t)
	database := db.GetIns()
	now := time.Now()
	// All players are already enrolled before the allocator is held. The peer's
	// actual candidate at 9999 is selected; the locked profile at 1000 is not.
	p11ArenaQuery(t, f, peer, &pb.CSPvpFindMatch{}, 62001, now)
	for id, rank := range map[int64]int64{f.id: 9999, peer: 10000, unrelated: 1000} {
		updated := database.Model(&db.P11ArenaMember{}).Where("player_id=? AND active=?", id, true).Update("position", rank)
		if updated.Error != nil || updated.RowsAffected != 1 {
			t.Fatalf("set independent fixture rank: rows=%d err=%v", updated.RowsAffected, updated.Error)
		}
	}
	ctx, cancel := context.WithTimeout(f.ctx, 20*time.Second)
	var held *gorm.DB
	type queryResult struct {
		id    int64
		reply []proto.Message
		err   error
	}
	done := make(chan queryResult, 4)
	pending := 0
	audit := newP11ArenaRound2Audit()
	const queryBefore = "test:p11-round2-query-before"
	const queryAfter = "test:p11-round2-query-after"
	const updateBefore = "test:p11-round2-update-before"
	var registeredBefore, registeredAfter, registeredUpdate bool
	defer func() {
		if held != nil {
			_ = held.Rollback().Error
		}
		cancel()
		for pending > 0 {
			<-done
			pending--
		}
		if registeredUpdate {
			_ = database.Callback().Update().Remove(updateBefore)
		}
		if registeredAfter {
			_ = database.Callback().Query().Remove(queryAfter)
		}
		if registeredBefore {
			_ = database.Callback().Query().Remove(queryBefore)
		}
	}()
	if err := database.Callback().Query().Before("gorm:query").Register(queryBefore, func(tx *gorm.DB) {
		tag, ok := tx.Statement.Context.Value(p11ArenaRound2QueryKey{}).(p11ArenaRound2QueryTag)
		if !ok {
			return
		}
		locking, hasLock := tx.Statement.Clauses["FOR"].Expression.(clause.Locking)
		audit.mu.Lock()
		defer audit.mu.Unlock()
		switch tx.Statement.Table {
		case "p11_arena_policies":
			if hasLock && locking.Strength == "SHARE" {
				audit.policyShare[tag.phase]++
			}
			if hasLock && locking.Strength == "UPDATE" {
				audit.policyExclusive[tag.phase]++
			}
		case "p11_arena_rank_allocators":
			if hasLock {
				audit.allocatorLock[tag.phase]++
			}
		}
	}); err != nil {
		t.Fatal(err)
	}
	registeredBefore = true
	if err := database.Callback().Query().After("gorm:query").Register(queryAfter, func(tx *gorm.DB) {
		tag, ok := tx.Statement.Context.Value(p11ArenaRound2QueryKey{}).(p11ArenaRound2QueryTag)
		if !ok || tx.Error != nil {
			return
		}
		audit.mu.Lock()
		defer audit.mu.Unlock()
		if tx.Statement.Table == "p11_arena_rank_allocators" {
			if allocator, ok := tx.Statement.Dest.(*db.P11ArenaRankAllocator); ok && tx.RowsAffected == 1 {
				audit.allocatorValues[tag.phase] = append(audit.allocatorValues[tag.phase], allocator.NextRank)
			}
		}
		statement := tx.Statement.SQL.String()
		if tx.Statement.Table != "profiles" || !p11ArenaRound2FullProfileSelect(statement) {
			return
		}
		read := p11ArenaRound2ProfileRead{sql: statement}
		switch dest := tx.Statement.Dest.(type) {
		case *db.Profile:
			if tx.RowsAffected > 0 {
				read.ids = append(read.ids, dest.Id)
			}
		case *[]db.Profile:
			for _, row := range *dest {
				read.ids = append(read.ids, row.Id)
			}
		default:
			audit.unexpectedProfile = append(audit.unexpectedProfile, fmt.Sprintf("%s: %T", tag.phase, tx.Statement.Dest))
		}
		audit.profileReads[tag.phase] = append(audit.profileReads[tag.phase], read)
	}); err != nil {
		t.Fatal(err)
	}
	registeredAfter = true
	if err := database.Callback().Update().Before("gorm:update").Register(updateBefore, func(tx *gorm.DB) {
		tag, ok := tx.Statement.Context.Value(p11ArenaRound2QueryKey{}).(p11ArenaRound2QueryTag)
		if !ok {
			return
		}
		audit.mu.Lock()
		defer audit.mu.Unlock()
		switch tx.Statement.Table {
		case "p11_arena_policies":
			audit.policyWrites[tag.phase]++
		case "p11_arena_rank_allocators":
			audit.allocatorWrites[tag.phase]++
		}
	}); err != nil {
		t.Fatal(err)
	}
	registeredUpdate = true
	startQuery := func(phase string, id, stamp int64) {
		pending++
		go func() {
			tagged := context.WithValue(ctx, p11ArenaRound2QueryKey{}, p11ArenaRound2QueryTag{phase: phase, id: id})
			reply, err := f.s.p11ArenaQuery(tagged, id, &pb.CSPvpFindMatch{}, stamp, now)
			done <- queryResult{id: id, reply: reply, err: err}
		}()
	}
	collect := func(count int) []queryResult {
		t.Helper()
		results := make([]queryResult, 0, count)
		timer := time.NewTimer(2 * time.Second)
		defer timer.Stop()
		for len(results) < count {
			select {
			case result := <-done:
				pending--
				if result.err != nil || len(result.reply) == 0 {
					t.Fatalf("independent arena query %d: %v", result.id, result.err)
				}
				response, ok := result.reply[0].(*pb.SCPvpFindMatch)
				if !ok || response.GetMyrank().GetInfo().GetPlayerId() != result.id || len(response.GetRankList()) == 0 {
					t.Fatalf("independent arena query %d returned no valid match", result.id)
				}
				results = append(results, result)
			case <-timer.C:
				t.Fatal("independent arena queries waited for the held row")
			case <-ctx.Done():
				t.Fatalf("arena query context ended: %v", ctx.Err())
			}
		}
		return results
	}
	begin := func() {
		t.Helper()
		held = database.WithContext(ctx).Begin(&sql.TxOptions{Isolation: sql.LevelReadCommitted})
		if held.Error != nil {
			t.Fatal(held.Error)
		}
	}
	release := func() {
		t.Helper()
		if err := held.Rollback().Error; err != nil {
			t.Fatal(err)
		}
		held = nil
	}

	begin()
	if _, err := p11ArenaConfig(held); err != nil {
		t.Fatal(err)
	}
	startQuery("policy", peer, 62002)
	collect(1)
	release()

	begin()
	var allocator db.P11ArenaRankAllocator
	if err := held.Clauses(clause.Locking{Strength: "UPDATE"}).First(&allocator, "policy_id=?", constUtil.P11ArenaPolicyID).Error; err != nil {
		t.Fatal(err)
	}
	committedNextRank := allocator.NextRank
	if err := held.Model(&allocator).Update("next_rank", committedNextRank+1).Error; err != nil {
		t.Fatal(err)
	}
	for i, id := range []int64{f.id, peer, unrelated} {
		startQuery("allocator", id, int64(62010+i))
	}
	collect(3)
	release()

	begin()
	var lockedProfile db.Profile
	if err := held.Select("id").Clauses(clause.Locking{Strength: "UPDATE"}).First(&lockedProfile, unrelated).Error; err != nil {
		t.Fatal(err)
	}
	startQuery("profile", peer, 62020)
	profileResult := collect(1)[0]
	release()
	response := profileResult.reply[0].(*pb.SCPvpFindMatch)
	selected := map[int64]bool{}
	for _, candidate := range response.GetRankList() {
		id := candidate.GetInfo().GetPlayerId()
		if !p11ArenaNPCID(id) {
			selected[id] = true
		}
	}
	if !selected[f.id] || selected[unrelated] {
		t.Fatalf("fixture did not select the intended real opponent: %v", selected)
	}

	// A newly eligible profile can be discovered while its own transaction is
	// active. Its registration must use the committed profile snapshot, even
	// though its profile row stays locked until after the other player's query.
	var previousMember db.P11ArenaMember
	if err := database.First(&previousMember, "player_id=?", unrelated).Error; err != nil {
		t.Fatal(err)
	}
	if previousMember.BestWonRank != 0 || previousMember.RankRewardBest != constUtil.P11ArenaVirtualSlots+1 {
		t.Fatal("missing-member fixture already has earned rank progress")
	}
	var beforeRegistration db.P11ArenaRankAllocator
	if err := database.First(&beforeRegistration, "policy_id=?", constUtil.P11ArenaPolicyID).Error; err != nil {
		t.Fatal(err)
	}
	if beforeRegistration.NextRank <= 10002 {
		t.Fatal("newly allocated member would be inside the peer's nearby ranks")
	}
	deleted := database.Where("player_id=?", unrelated).Delete(&db.P11ArenaMember{})
	if deleted.Error != nil || deleted.RowsAffected != 1 {
		t.Fatalf("create missing eligible member fixture: rows=%d err=%v", deleted.RowsAffected, deleted.Error)
	}
	begin()
	lockedProfile = db.Profile{}
	if err := held.Select("id").Clauses(clause.Locking{Strength: "UPDATE"}).First(&lockedProfile, unrelated).Error; err != nil {
		t.Fatal(err)
	}
	startQuery("missing-eligible", peer, 62021)
	collect(1)
	var registered []db.P11ArenaMember
	if err := database.Where("player_id=?", unrelated).Find(&registered).Error; err != nil {
		t.Fatal(err)
	}
	if len(registered) != 1 {
		t.Fatalf("missing eligible profile has %d member rows, want one", len(registered))
	}
	if !registered[0].Active || registered[0].Rank != beforeRegistration.NextRank || registered[0].BestWonRank != previousMember.BestWonRank || registered[0].RankRewardBest != previousMember.RankRewardBest {
		t.Fatalf("missing eligible profile registration changed rank progress: active=%t rank=%d best=%d rewardBest=%d", registered[0].Active, registered[0].Rank, registered[0].BestWonRank, registered[0].RankRewardBest)
	}
	var afterRegistration db.P11ArenaRankAllocator
	if err := database.First(&afterRegistration, "policy_id=?", constUtil.P11ArenaPolicyID).Error; err != nil {
		t.Fatal(err)
	}
	if afterRegistration.NextRank != beforeRegistration.NextRank+1 {
		t.Fatalf("missing eligible registration did not allocate exactly one rank: before=%d after=%d", beforeRegistration.NextRank, afterRegistration.NextRank)
	}
	release()

	audit.mu.Lock()
	defer audit.mu.Unlock()
	for _, phase := range []string{"policy", "allocator", "profile"} {
		if audit.policyShare[phase] == 0 || audit.policyExclusive[phase] != 0 || audit.policyWrites[phase] != 0 || audit.allocatorLock[phase] != 0 || audit.allocatorWrites[phase] != 0 {
			t.Fatalf("%s query took a policy/allocator write lock: share=%d policyX=%d policyWrites=%d allocatorLocks=%d allocatorWrites=%d", phase, audit.policyShare[phase], audit.policyExclusive[phase], audit.policyWrites[phase], audit.allocatorLock[phase], audit.allocatorWrites[phase])
		}
	}
	if got := audit.allocatorValues["allocator"]; len(got) != 3 || got[0] != committedNextRank || got[1] != committedNextRank || got[2] != committedNextRank {
		t.Fatalf("allocator readers did not all see the committed cursor: %v", got)
	}
	if len(audit.unexpectedProfile) != 0 {
		t.Fatalf("unrecognized profile query destinations: %v", audit.unexpectedProfile)
	}
	seen := map[int64]int{}
	for _, read := range audit.profileReads["profile"] {
		if !strings.Contains(strings.ToLower(read.sql), " where ") {
			t.Fatalf("full profile read had no WHERE: %s", read.sql)
		}
		for _, id := range read.ids {
			seen[id]++
		}
	}
	want := map[int64]int{peer: 1}
	for id := range selected {
		want[id]++
	}
	if !reflect.DeepEqual(seen, want) {
		t.Fatalf("full profile reads differ from owner and selected real opponents: got=%v want=%v", seen, want)
	}
}

type p11ArenaRound2FightKey struct{}

type p11ArenaRound2FightResult struct {
	id    int64
	stamp int64
	reply []proto.Message
	err   error
	stage string
}

// Both real requests are stopped after their actual empty-slot query so that
// rank arbitration, rather than request start order, decides the outcome.
func p11ArenaRound2FightBoth(t *testing.T, f *p1Fixture, now time.Time, targets, stamps map[int64]int64, holdID int64) []p11ArenaRound2FightResult {
	t.Helper()
	if len(targets) != 2 || holdID != 0 && targets[holdID] == 0 {
		t.Fatal("fight coordination requires two targets and a valid holder")
	}
	database := db.GetIns()
	ctx, cancel := context.WithTimeout(f.ctx, 40*time.Second)
	gates := map[int64]chan struct{}{}
	gateOnce := map[int64]*sync.Once{}
	for id := range targets {
		gates[id], gateOnce[id] = make(chan struct{}), &sync.Once{}
	}
	releaseGate := func(id int64) { gateOnce[id].Do(func() { close(gates[id]) }) }
	rankHeld, holdGate := make(chan struct{}), make(chan struct{})
	arrived := make(chan int64, 2)
	done := make(chan p11ArenaRound2FightResult, 2)
	var releaseHold, heldOnce sync.Once
	var once, stages sync.Map
	lastStage := func(id int64) string {
		if stage, ok := stages.Load(id); ok {
			return stage.(string)
		}
		return "no SQL recorded"
	}
	allStages := func() string {
		parts := make([]string, 0, len(targets))
		for id := range targets {
			parts = append(parts, fmt.Sprintf("%d=%s", id, lastStage(id)))
		}
		return strings.Join(parts, "; ")
	}
	recordStage := func(operation string) func(*gorm.DB) {
		return func(tx *gorm.DB) {
			id, ok := tx.Statement.Context.Value(p11ArenaRound2FightKey{}).(int64)
			if !ok {
				return
			}
			table := tx.Statement.Table
			if table == "" && tx.Statement.Schema != nil {
				table = tx.Statement.Schema.Table
			}
			if table == "" {
				table = "<unknown>"
			}
			stages.Store(id, operation+" "+table)
		}
	}
	pending := 0
	const callback = "test:p11-round2-virtual-slot-read"
	const updateCallback = "test:p11-round2-virtual-slot-held"
	const queryStageCallback = "test:p11-round2-query-stage"
	const createStageCallback = "test:p11-round2-create-stage"
	const updateStageCallback = "test:p11-round2-update-stage"
	var registered, updateRegistered, queryStageRegistered, createStageRegistered, updateStageRegistered bool
	defer func() {
		for id := range gates {
			releaseGate(id)
		}
		releaseHold.Do(func() { close(holdGate) })
		cancel()
		for pending > 0 {
			<-done
			pending--
		}
		if registered {
			_ = database.Callback().Query().Remove(callback)
		}
		if updateRegistered {
			_ = database.Callback().Update().Remove(updateCallback)
		}
		if queryStageRegistered {
			_ = database.Callback().Query().Remove(queryStageCallback)
		}
		if createStageRegistered {
			_ = database.Callback().Create().Remove(createStageCallback)
		}
		if updateStageRegistered {
			_ = database.Callback().Update().Remove(updateStageCallback)
		}
	}()
	if err := database.Callback().Query().Before("gorm:query").Register(queryStageCallback, recordStage("query")); err != nil {
		t.Fatal(err)
	}
	queryStageRegistered = true
	if err := database.Callback().Create().Before("gorm:create").Register(createStageCallback, recordStage("create")); err != nil {
		t.Fatal(err)
	}
	createStageRegistered = true
	if err := database.Callback().Update().Before("gorm:update").Register(updateStageCallback, recordStage("update")); err != nil {
		t.Fatal(err)
	}
	updateStageRegistered = true
	if err := database.Callback().Query().After("gorm:query").Register(callback, func(tx *gorm.DB) {
		id, ok := tx.Statement.Context.Value(p11ArenaRound2FightKey{}).(int64)
		if !ok || tx.Statement.Table != "p11_arena_members" || tx.Error != nil || !strings.Contains(strings.ToLower(tx.Statement.SQL.String()), "count(") {
			return
		}
		rank, ok := targets[id]
		if !ok {
			return
		}
		matched := false
		for _, value := range tx.Statement.Vars {
			if n, ok := value.(int64); ok && n == rank {
				matched = true
			}
		}
		count, ok := tx.Statement.Dest.(*int64)
		if !matched || !ok || *count != 0 {
			return
		}
		value, _ := once.LoadOrStore(id, &sync.Once{})
		value.(*sync.Once).Do(func() {
			arrived <- id
			select {
			case <-gates[id]:
			case <-ctx.Done():
				tx.AddError(ctx.Err())
			}
		})
	}); err != nil {
		t.Fatal(err)
	}
	registered = true
	if holdID != 0 {
		if err := database.Callback().Update().After("gorm:update").Register(updateCallback, func(tx *gorm.DB) {
			id, ok := tx.Statement.Context.Value(p11ArenaRound2FightKey{}).(int64)
			if !ok || id != holdID || tx.Statement.Table != "p11_arena_members" || tx.Error != nil || tx.RowsAffected != 1 {
				return
			}
			values, ok := tx.Statement.Dest.(map[string]any)
			if !ok {
				return
			}
			position, ok := values["position"].(int64)
			if !ok || position != targets[holdID] {
				return
			}
			heldOnce.Do(func() {
				close(rankHeld)
				select {
				case <-holdGate:
				case <-ctx.Done():
					tx.AddError(ctx.Err())
				}
			})
		}); err != nil {
			t.Fatal(err)
		}
		updateRegistered = true
	}
	for id, rank := range targets {
		id, rank := id, rank
		stamp := stamps[id]
		pending++
		go func() {
			tagged := context.WithValue(ctx, p11ArenaRound2FightKey{}, id)
			request := &pb.CSPvpFightResult{PlayerId: proto.Int64(constUtil.P11ArenaNPCIDBase + rank), ServerId: proto.Int32(1)}
			reply, err := f.s.p11ArenaScopedFightAt(tagged, id, request, stamp, now)
			done <- p11ArenaRound2FightResult{id: id, stamp: stamp, reply: reply, err: err, stage: lastStage(id)}
		}()
	}
	seen := map[int64]bool{}
	timer := time.NewTimer(10 * time.Second)
	defer timer.Stop()
	for len(seen) < 2 {
		select {
		case id := <-arrived:
			seen[id] = true
		case result := <-done:
			pending--
			t.Fatalf("fight %d ended before both empty-slot checks: %v; stages: %s", result.id, result.err, allStages())
		case <-timer.C:
			t.Fatalf("both fights did not reach independent empty-slot checks; stages: %s", allStages())
		case <-ctx.Done():
			t.Fatalf("fight coordination ended: %v; stages: %s", ctx.Err(), allStages())
		}
	}
	results := make([]p11ArenaRound2FightResult, 0, 2)
	if holdID == 0 {
		for id := range gates {
			releaseGate(id)
		}
	} else {
		other := int64(0)
		for id := range targets {
			if id != holdID {
				other = id
			}
		}
		if other == 0 || targets[other] == targets[holdID] {
			t.Fatal("held-rank scenario requires distinct positions")
		}
		releaseGate(holdID)
		select {
		case <-rankHeld:
		case result := <-done:
			pending--
			t.Fatalf("holder ended before updating its rank: id=%d err=%v; stages: %s", result.id, result.err, allStages())
		case <-ctx.Done():
			t.Fatalf("holder did not reach its rank update: %v; stages: %s", ctx.Err(), allStages())
		}
		releaseGate(other)
		select {
		case result := <-done:
			pending--
			if result.id != other || result.err != nil || len(result.reply) == 0 {
				t.Fatalf("other NPC fight failed while rank %d was held: id=%d err=%v; stages: %s", targets[holdID], result.id, result.err, allStages())
			}
			results = append(results, result)
		case <-ctx.Done():
			t.Fatalf("other NPC fight waited for an independent held rank: %v; stages: %s", ctx.Err(), allStages())
		}
		releaseHold.Do(func() { close(holdGate) })
	}
	for len(results) < 2 {
		select {
		case result := <-done:
			pending--
			results = append(results, result)
		case <-ctx.Done():
			t.Fatalf("fights did not finish: %v; stages: %s", ctx.Err(), allStages())
		}
	}
	return results
}

type p11ArenaRound2ProfileState struct {
	player, game string
	energy       int32
}

func p11ArenaRound2Profile(t *testing.T, id int64) p11ArenaRound2ProfileState {
	t.Helper()
	var row db.Profile
	if err := db.GetIns().First(&row, id).Error; err != nil {
		t.Fatal(err)
	}
	player, _, err := decodeProfile(&row)
	if err != nil || row.Player == nil || row.Game == nil {
		t.Fatalf("read fight profile: %v", err)
	}
	return p11ArenaRound2ProfileState{player: *row.Player, game: *row.Game, energy: player.GetEnergy()}
}

func p11ArenaRound2Offer(t *testing.T, f *p1Fixture, id, stamp int64, now time.Time) *pb.SCPvpFindMatch {
	t.Helper()
	return p11ArenaQuery(t, f, id, &pb.CSPvpFindMatch{}, stamp, now).(*pb.SCPvpFindMatch)
}

func p11ArenaRound2NPCOffer(offer *pb.SCPvpFindMatch, rank int64) bool {
	for _, candidate := range offer.GetRankList() {
		if candidate.GetPvpRank() == rank && candidate.GetInfo().GetPlayerId() == constUtil.P11ArenaNPCIDBase+rank {
			return true
		}
	}
	return false
}

func TestP11ArenaConcurrencyRound2MySQLVirtualRankArbitration(t *testing.T) {
	p11ArenaRound2MySQLGuard(t)
	if constUtil.P6ArenaNativeProof == "" {
		t.Fatal("original arena combat certificate is required")
	}
	f, first, second := p11ArenaSetup(t)
	database := db.GetIns()
	now := time.Now()
	for _, id := range []int64{first, second} {
		if err := updateProfile(f.ctx, id, func(p *pb.Player, g *model.GameState) error {
			if len(g.Warriors) != 1 || g.Warriors[0].GetId() != "hero_047" {
				return errors.New("original one-hero arena fixture unavailable")
			}
			p.Level, p.Energy = proto.Int32(100), proto.Int32(100)
			g.Warriors[0].Level = proto.Int32(100)
			return nil
		}); err != nil {
			t.Fatal(err)
		}
	}
	for _, table := range []any{&db.P11ArenaPolicy{}, &db.P11ArenaFightPolicy{}} {
		if err := database.Model(table).Where("id=1").Update("enabled", true).Error; err != nil {
			t.Fatal(err)
		}
	}
	for id, rank := range map[int64]int64{f.id: 1000, first: 100, second: 101} {
		updated := database.Model(&db.P11ArenaMember{}).Where("player_id=? AND active=?", id, true).Update("position", rank)
		if updated.Error != nil || updated.RowsAffected != 1 {
			t.Fatalf("set rank fixture: rows=%d err=%v", updated.RowsAffected, updated.Error)
		}
	}
	firstOffer := p11ArenaRound2Offer(t, f, first, 62101, now)
	secondOffer := p11ArenaRound2Offer(t, f, second, 62102, now)
	const commonRank int64 = 98
	if !p11ArenaRound2NPCOffer(firstOffer, commonRank) || !p11ArenaRound2NPCOffer(secondOffer, commonRank) {
		t.Fatal("both actual match offers must contain the same empty NPC rank")
	}
	before := map[int64]p11ArenaRound2ProfileState{first: p11ArenaRound2Profile(t, first), second: p11ArenaRound2Profile(t, second)}
	initialRanks := map[int64]int64{first: 100, second: 101}
	stamps := map[int64]int64{first: 62111, second: 62112}
	results := p11ArenaRound2FightBoth(t, f, now, map[int64]int64{first: commonRank, second: commonRank}, stamps, 0)
	var winner, loser int64
	for _, result := range results {
		if result.err != nil {
			if result.err.Error() != constUtil.P11ArenaFightRefreshRequired || loser != 0 || len(result.reply) != 0 {
				t.Fatalf("loser did not receive the refresh result: id=%d err=%v stage=%s", result.id, result.err, result.stage)
			}
			loser = result.id
			continue
		}
		if winner != 0 || len(result.reply) == 0 {
			t.Fatal("same NPC seat produced multiple successful settlements")
		}
		response, ok := result.reply[0].(*pb.SCPvpFightResult)
		if !ok || response.GetWinPlayerId() != result.id || response.GetMyrank().GetPvpRank() != commonRank {
			t.Fatal("same-seat winner did not actually occupy the NPC rank")
		}
		winner = result.id
	}
	if winner == 0 || loser == 0 || winner == loser {
		t.Fatal("same NPC seat did not produce exactly one winner and one refresh")
	}
	var rows []db.P11ArenaMember
	if err := database.Where("position=?", commonRank).Find(&rows).Error; err != nil || len(rows) != 1 || rows[0].PlayerID != winner {
		t.Fatalf("contested NPC rank has wrong owner: rows=%d err=%v", len(rows), err)
	}
	var losingMember db.P11ArenaMember
	if err := database.First(&losingMember, "player_id=?", loser).Error; err != nil || losingMember.Rank != initialRanks[loser] {
		t.Fatalf("failed challenge changed the loser's rank: rank=%d err=%v", losingMember.Rank, err)
	}
	afterLoser := p11ArenaRound2Profile(t, loser)
	if afterLoser != before[loser] {
		t.Fatal("failed challenge changed the loser's profile or resources")
	}
	afterWinner := p11ArenaRound2Profile(t, winner)
	if afterWinner.energy != before[winner].energy-constUtil.P11ArenaEnergy {
		t.Fatal("successful challenge did not charge the original energy cost once")
	}
	for _, id := range []int64{winner, loser} {
		var settlements, records int64
		if err := database.Model(&db.P11ArenaFightSettlement{}).Where("player_id=? AND stamp=?", id, stamps[id]).Count(&settlements).Error; err != nil {
			t.Fatal(err)
		}
		if err := database.Model(&db.P6BattleRecord{}).Where("player_id=? AND source=?", id, constUtil.P11ArenaFightSource).Count(&records).Error; err != nil {
			t.Fatal(err)
		}
		want := int64(0)
		if id == winner {
			want = 1
		}
		if settlements != want || records != want {
			t.Fatalf("same-seat fight left incorrect settlement/capture counts: %d %d", settlements, records)
		}
	}
	var winnerSettlement db.P11ArenaFightSettlement
	if err := database.First(&winnerSettlement, "player_id=? AND stamp=?", winner, stamps[winner]).Error; err != nil {
		t.Fatal(err)
	}
	var proof db.P6BattleRecord
	if err := database.First(&proof, "id=?", winnerSettlement.P6RecordID).Error; err != nil || !proof.Authoritative || winnerSettlement.ScopeProof == "" {
		t.Fatalf("same-seat settlement lacks original authoritative combat: %v", err)
	}

	// Refresh the two real offers after the contested seat, then choose distinct
	// empty ranks that the original matcher actually returned for each player.
	newOffers := map[int64]*pb.SCPvpFindMatch{
		first:  p11ArenaRound2Offer(t, f, first, 62121, now),
		second: p11ArenaRound2Offer(t, f, second, 62122, now),
	}
	targets := map[int64]int64{}
	used := map[int64]bool{}
	for _, id := range []int64{first, second} {
		offer := newOffers[id]
		for _, candidate := range offer.GetRankList() {
			rank := candidate.GetPvpRank()
			if rank > 0 && rank < offer.GetMyrank().GetPvpRank() && p11ArenaNPCID(candidate.GetInfo().GetPlayerId()) && !used[rank] {
				targets[id], used[rank] = rank, true
				break
			}
		}
		if targets[id] == 0 {
			t.Fatal("no distinct higher NPC rank in the actual match offer")
		}
	}
	beforeDistinct := map[int64]p11ArenaRound2ProfileState{first: p11ArenaRound2Profile(t, first), second: p11ArenaRound2Profile(t, second)}
	stamps = map[int64]int64{first: 62131, second: 62132}
	results = p11ArenaRound2FightBoth(t, f, now, targets, stamps, first)
	for _, result := range results {
		if result.err != nil || len(result.reply) == 0 {
			t.Fatalf("independent NPC fight failed: id=%d err=%v stage=%s", result.id, result.err, result.stage)
		}
		response, ok := result.reply[0].(*pb.SCPvpFightResult)
		if !ok || response.GetWinPlayerId() != result.id || response.GetMyrank().GetPvpRank() != targets[result.id] {
			t.Fatalf("independent NPC fight did not occupy its own offered rank: %d", result.id)
		}
		after := p11ArenaRound2Profile(t, result.id)
		if after.energy != beforeDistinct[result.id].energy-constUtil.P11ArenaEnergy {
			t.Fatalf("independent NPC fight has wrong energy cost: %d", result.id)
		}
		var settlement db.P11ArenaFightSettlement
		if err := database.First(&settlement, "player_id=? AND stamp=?", result.id, result.stamp).Error; err != nil {
			t.Fatal(err)
		}
		proof = db.P6BattleRecord{}
		if err := database.First(&proof, "id=?", settlement.P6RecordID).Error; err != nil || !proof.Authoritative || settlement.ScopeProof == "" {
			t.Fatalf("independent NPC fight lacks original authoritative combat: %v", err)
		}
	}
}

func TestP11ArenaConcurrencyRound2MySQLIndependentAfterFirstReceipt(t *testing.T) {
	p11ArenaRound2MySQLGuard(t)
	if constUtil.P6ArenaNativeProof == "" {
		t.Fatal("original arena combat certificate is required")
	}
	f, first, second := p11ArenaSetup(t)
	database := db.GetIns()
	now := time.Now()
	for _, id := range []int64{first, second} {
		if err := updateProfile(f.ctx, id, func(p *pb.Player, g *model.GameState) error {
			if len(g.Warriors) != 1 || g.Warriors[0].GetId() != "hero_047" {
				return errors.New("original one-hero arena fixture unavailable")
			}
			p.Level, p.Energy = proto.Int32(100), proto.Int32(100)
			g.Warriors[0].Level = proto.Int32(100)
			return nil
		}); err != nil {
			t.Fatal(err)
		}
	}
	for _, table := range []any{&db.P11ArenaPolicy{}, &db.P11ArenaFightPolicy{}} {
		if err := database.Model(table).Where("id=1").Update("enabled", true).Error; err != nil {
			t.Fatal(err)
		}
	}
	for id, rank := range map[int64]int64{f.id: 1000, first: 100, second: 101} {
		updated := database.Model(&db.P11ArenaMember{}).Where("player_id=? AND active=?", id, true).Update("position", rank)
		if updated.Error != nil || updated.RowsAffected != 1 {
			t.Fatalf("set rank fixture: rows=%d err=%v", updated.RowsAffected, updated.Error)
		}
	}
	chooseNPC := func(offer *pb.SCPvpFindMatch, used map[int64]bool) int64 {
		for _, candidate := range offer.GetRankList() {
			rank := candidate.GetPvpRank()
			if rank > 0 && rank < offer.GetMyrank().GetPvpRank() && candidate.GetInfo().GetPlayerId() == constUtil.P11ArenaNPCIDBase+rank && !used[rank] {
				return rank
			}
		}
		t.Fatal("no distinct higher NPC rank in the actual match offer")
		return 0
	}
	assertAuthoritative := func(id, stamp int64) {
		t.Helper()
		var settlement db.P11ArenaFightSettlement
		if err := database.First(&settlement, "player_id=? AND stamp=?", id, stamp).Error; err != nil {
			t.Fatalf("missing real settlement for %d: %v", id, err)
		}
		var proof db.P6BattleRecord
		if err := database.First(&proof, "id=?", settlement.P6RecordID).Error; err != nil || !proof.Authoritative || settlement.ScopeProof == "" {
			t.Fatalf("settlement for %d lacks original authoritative combat: %v", id, err)
		}
	}

	firstOffer := p11ArenaRound2Offer(t, f, first, 62201, now)
	initialTarget := chooseNPC(firstOffer, map[int64]bool{})
	beforeInitial := p11ArenaRound2Profile(t, first)
	initialRequest := &pb.CSPvpFightResult{PlayerId: proto.Int64(constUtil.P11ArenaNPCIDBase + initialTarget), ServerId: proto.Int32(1)}
	initialReply, err := f.s.p11ArenaScopedFightAt(f.ctx, first, initialRequest, 62202, now)
	if err != nil || len(initialReply) == 0 {
		t.Fatalf("first actual NPC settlement failed: %v", err)
	}
	initialResponse, ok := initialReply[0].(*pb.SCPvpFightResult)
	if !ok || initialResponse.GetWinPlayerId() != first || initialResponse.GetMyrank().GetPvpRank() != initialTarget {
		t.Fatal("first actual NPC fight did not win and occupy its offered rank")
	}
	if after := p11ArenaRound2Profile(t, first); after.energy != beforeInitial.energy-constUtil.P11ArenaEnergy {
		t.Fatal("first actual NPC settlement charged the wrong energy")
	}
	assertAuthoritative(first, 62202)
	for _, id := range []int64{first, second} {
		var count int64
		if err := database.Model(&db.P11ArenaFightSettlement{}).Where("player_id=?", id).Count(&count).Error; err != nil {
			t.Fatal(err)
		}
		want := int64(0)
		if id == first {
			want = 1
		}
		if count != want {
			t.Fatalf("unexpected actual settlement history for %d: got %d want %d", id, count, want)
		}
	}

	firstOffer = p11ArenaRound2Offer(t, f, first, 62211, now)
	secondOffer := p11ArenaRound2Offer(t, f, second, 62212, now)
	firstTarget := chooseNPC(firstOffer, map[int64]bool{})
	secondTarget := chooseNPC(secondOffer, map[int64]bool{firstTarget: true})
	targets := map[int64]int64{first: firstTarget, second: secondTarget}
	stamps := map[int64]int64{first: 62221, second: 62222}
	before := map[int64]p11ArenaRound2ProfileState{first: p11ArenaRound2Profile(t, first), second: p11ArenaRound2Profile(t, second)}
	results := p11ArenaRound2FightBoth(t, f, now, targets, stamps, first)
	for _, result := range results {
		if result.err != nil || len(result.reply) == 0 {
			t.Fatalf("independent NPC fight after first receipt failed: id=%d err=%v stage=%s", result.id, result.err, result.stage)
		}
		response, ok := result.reply[0].(*pb.SCPvpFightResult)
		if !ok || response.GetWinPlayerId() != result.id || response.GetMyrank().GetPvpRank() != targets[result.id] {
			t.Fatalf("independent NPC fight after first receipt did not occupy its offered rank: %d stage=%s", result.id, result.stage)
		}
		if after := p11ArenaRound2Profile(t, result.id); after.energy != before[result.id].energy-constUtil.P11ArenaEnergy {
			t.Fatalf("independent NPC fight after first receipt has wrong energy cost: %d", result.id)
		}
		var member db.P11ArenaMember
		if err := database.First(&member, "player_id=?", result.id).Error; err != nil || member.Rank != targets[result.id] {
			t.Fatalf("independent NPC fight after first receipt persisted the wrong rank: id=%d rank=%d err=%v", result.id, member.Rank, err)
		}
		assertAuthoritative(result.id, result.stamp)
	}
	for _, id := range []int64{first, second} {
		var count int64
		if err := database.Model(&db.P11ArenaFightSettlement{}).Where("player_id=?", id).Count(&count).Error; err != nil {
			t.Fatal(err)
		}
		want := int64(1)
		if id == first {
			want = 2
		}
		if count != want {
			t.Fatalf("unexpected settlement count after independent fights for %d: got %d want %d", id, count, want)
		}
	}
}
