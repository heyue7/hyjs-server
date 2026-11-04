package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	"gorm.io/gorm/clause"
	"gorm.io/gorm/logger"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

type popularityReadSQLTrace struct {
	logger.Interface
	mu         sync.Mutex
	statements []string
}

func (trace *popularityReadSQLTrace) Trace(_ context.Context, _ time.Time, sql func() (string, int64), _ error) {
	query, _ := sql()
	trace.mu.Lock()
	trace.statements = append(trace.statements, query)
	trace.mu.Unlock()
}

func (trace *popularityReadSQLTrace) take() []string {
	trace.mu.Lock()
	defer trace.mu.Unlock()
	result := trace.statements
	trace.statements = nil
	return result
}

// The same role and catalogues exercise the full protocol entrance, including
// clock, ordinary items, two rank offers, quota and previously spent proof.
func TestP4PopularityReadMySQLSnapshotAndCost(t *testing.T) {
	if os.Getenv("HYJS_P4_MYSQL_DATABASE") == "" {
		t.Skip("requires an isolated hyjs_test_p4_ MySQL database")
	}
	f := p4Setup(t)
	p4Fund(t, f)
	if err := updateProfile(f.ctx, f.id, func(p *pb.Player, _ *model.GameState) error {
		p.NoviceGuideSchedule = proto.Int32(constUtil.P4PopularityGuideCompleted)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if err := db.GetIns().AutoMigrate(&db.P11ArenaMember{}, &db.P11ArenaFightSettlement{}, &db.LocalPlayActivityOperation{}); err != nil {
		t.Fatal(err)
	}
	if err := f.s.InitializeServerGameClock(f.ctx); err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	key := p4PopularityPublish(t, f, now)
	var rankOffer db.P4CommerceRule
	if err := db.GetIns().First(&rankOffer, "id=?", key).Error; err != nil {
		t.Fatal(err)
	}
	var rankRules p4CommerceRules
	if err := json.Unmarshal(rankOffer.Rules, &rankRules); err != nil {
		t.Fatal(err)
	}
	rankRules.Products[0].ID = "p4PopularityRank_second_1"
	second := rankOffer
	second.ID = constUtil.P4PopularityRankRulePrefix + "test-second"
	second.Rules, _ = json.Marshal(rankRules)
	if err := db.GetIns().Create(&second).Error; err != nil {
		t.Fatal(err)
	}
	ctx := serverClockBindTest(t, f, f.id)
	query := &pb.CSShopProductList{ShopType: proto.Int32(4)}
	// This first request exercises the real freeze and clock recovery fallback.
	if _, err := f.s.p4Commerce(ctx, f.id, query, 0, now); err != nil {
		t.Fatal(err)
	}
	if err := updateProfile(f.ctx, f.id, func(p *pb.Player, _ *model.GameState) error {
		p.NoviceGuideSchedule = proto.Int32(constUtil.P4PopularityGuideCompleted - 1)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	waitingGuide, err := f.s.p4PopularityListRead(ctx, f.id, now)
	if err != nil || len(waitingGuide.GetShopItemList()) < 3 {
		t.Fatalf("configured unspent guide should remain a read: %v", err)
	}
	if err := updateProfile(f.ctx, f.id, func(p *pb.Player, _ *model.GameState) error {
		p.NoviceGuideSchedule = proto.Int32(constUtil.P4PopularityGuideCompleted)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	var stateRow db.P4CommerceState
	if err := db.GetIns().First(&stateRow, "player_id=? AND rule_id=?", f.id, key).Error; err != nil {
		t.Fatal(err)
	}
	var progress p4CommerceProgress
	if err := json.Unmarshal(stateRow.Payload, &progress); err != nil {
		t.Fatal(err)
	}
	var firstRules p4CommerceRules
	if err := json.Unmarshal(rankOffer.Rules, &firstRules); err != nil {
		t.Fatal(err)
	}
	product := firstRules.Products[0].ID
	progress.Total[product], progress.Bought[product] = 1, 1
	if err := saveP4Commerce(db.GetIns(), &stateRow, &progress); err != nil {
		t.Fatal(err)
	}
	claim := db.P4PopularityRankClaim{PlayerID: f.id, RuleID: key, ProductID: product, Rank: 500, RankProof: strings.Repeat("a", 64), OfferHash: strings.Repeat("b", 64), Source: rankOffer.Source, Stamp: 1, ClaimedAt: now.UnixMilli()}
	if err := db.GetIns().Create(&claim).Error; err != nil {
		t.Fatal(err)
	}
	if err := updateProfile(f.ctx, f.id, func(p *pb.Player, _ *model.GameState) error {
		p.NoviceGuideSchedule = proto.Int32(constUtil.P4PopularityGuideCompleted - 1)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	missingProof, err := f.s.p4PopularityListRead(ctx, f.id, now)
	if err != nil || p4PopularityFind(t, missingProof.GetShopItemList(), product).GetCanBuy() != 2 {
		t.Fatalf("missing historical proof should stay closed without a write: %v", err)
	}
	if err := updateProfile(f.ctx, f.id, func(p *pb.Player, _ *model.GameState) error {
		p.NoviceGuideSchedule = proto.Int32(constUtil.P4PopularityGuideCompleted)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if err := f.s.InitializeGameSchema64(ctx); err != nil {
		t.Fatal(err)
	}
	trace := &popularityReadSQLTrace{Interface: logger.Discard}
	priorLogger := db.GetIns().Logger
	db.GetIns().Logger = trace
	defer func() { db.GetIns().Logger = priorLogger }()

	started := time.Now()
	old, guide, err := f.s.p4CommerceAttempt(ctx, f.id, query, 0, now, false, errors.New("retry freeze"))
	oldElapsed := time.Since(started)
	if err != nil || guide != nil {
		t.Fatalf("existing list: %v", err)
	}
	oldSQL := trace.take()
	started = time.Now()
	fast, err := f.s.p4Commerce(ctx, f.id, query, 0, now)
	fastElapsed := time.Since(started)
	if err != nil || len(fast) != 1 || !proto.Equal(old, fast[0]) {
		t.Fatalf("same-input list differs: %v", err)
	}
	fastSQL := trace.take()
	if len(fastSQL) >= len(oldSQL) {
		t.Fatalf("SQL cost did not decrease: old=%d read=%d", len(oldSQL), len(fastSQL))
	}
	var rankReads int
	for _, statement := range fastSQL {
		upper := strings.ToUpper(strings.TrimSpace(statement))
		if strings.HasPrefix(upper, "UPDATE ") || strings.HasPrefix(upper, "INSERT ") || strings.HasPrefix(upper, "DELETE ") || strings.HasPrefix(upper, "REPLACE ") || strings.Contains(upper, "FOR UPDATE") || strings.Contains(upper, "FOR SHARE") || strings.Contains(upper, "LOCK IN SHARE MODE") {
			t.Fatalf("read path emitted a write or locking read")
		}
		if strings.Contains(upper, "P4_POPULARITY_RANK_CLAIMS") || strings.Contains(upper, "P4_POPULARITY_RANK_OFFER_HISTORY") {
			t.Fatal("completed guide revalidated an old claim")
		}
		if strings.Contains(upper, "FROM `P11_ARENA_MEMBERS`") {
			rankReads++
		}
	}
	if rankReads != 1 {
		t.Fatalf("two reward catalogues performed %d rank reads", rankReads)
	}
	t.Logf("same role and two rank offers: old SQL=%d elapsed=%s; MVCC SQL=%d elapsed=%s", len(oldSQL), oldElapsed, len(fastSQL), fastElapsed)
	stale := context.WithValue(ctx, constUtil.ServerGameClockRevisionKey, int64(-1))
	if _, err := f.s.p4Commerce(stale, f.id, query, 0, now); !errors.Is(err, constUtil.ErrAdminConfigConflict) {
		t.Fatalf("stale clock was accepted: %v", err)
	}

	var normal, unrelated db.P4CommerceRule
	if err := db.GetIns().Where("kind=? AND enabled=?", "popularity", true).First(&normal).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.GetIns().Where("kind=?", "point").First(&unrelated).Error; err != nil {
		t.Fatal(err)
	}
	var normalState db.P4CommerceState
	if err := db.GetIns().First(&normalState, "player_id=? AND rule_id=?", f.id, normal.ID).Error; err != nil {
		t.Fatal(err)
	}
	storedState := append([]byte(nil), normalState.Payload...)
	target := now.AddDate(0, 0, -1)
	if target.In(time.FixedZone("UTC+8", 8*3600)).Month() != now.In(time.FixedZone("UTC+8", 8*3600)).Month() {
		target = now.AddDate(0, 0, 1)
	}
	targetQuota := &p4CommerceProgress{Bought: map[string]int32{"p4Popularity_1": 7}}
	if err := localPlayCommerceDailySave(db.GetIns(), &normalState, p1Day(target), targetQuota); err != nil {
		t.Fatal(err)
	}
	var operationCount int64
	if err := db.GetIns().Model(&db.LocalPlayActivityOperation{}).Where("player_id=?", f.id).Count(&operationCount).Error; err != nil {
		t.Fatal(err)
	}
	targetList, err := f.s.p4Commerce(ctx, f.id, query, 0, target)
	if err != nil {
		t.Fatal(err)
	}
	item := p4PopularityFind(t, targetList[0].(*pb.SCShopProductList).GetShopItemList(), "p4Popularity_1")
	if item.GetDayLeftTimes() != 13 {
		t.Fatalf("historical day quota was not projected: %d", item.GetDayLeftTimes())
	}
	if err := db.GetIns().First(&normalState, "player_id=? AND rule_id=?", f.id, normal.ID).Error; err != nil || !bytes.Equal(storedState, normalState.Payload) {
		t.Fatalf("historical list changed current day state: %v", err)
	}
	var operationsAfter int64
	if err := db.GetIns().Model(&db.LocalPlayActivityOperation{}).Where("player_id=?", f.id).Count(&operationsAfter).Error; err != nil || operationCount != operationsAfter {
		t.Fatalf("historical list archived a day: %v", err)
	}
	holder := db.GetIns().Begin()
	if holder.Error != nil {
		t.Fatal(holder.Error)
	}
	defer holder.Rollback()
	var profile db.Profile
	if err := holder.Clauses(clause.Locking{Strength: "UPDATE"}).First(&profile, f.id).Error; err != nil {
		t.Fatal(err)
	}
	var locked []db.P4CommerceRule
	if err := holder.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id IN ?", []string{normal.ID, key, second.ID, unrelated.ID}).Find(&locked).Error; err != nil || len(locked) != 4 {
		t.Fatalf("lock fixture: %v", err)
	}
	readCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	lockedList, err := f.s.p4Commerce(readCtx, f.id, query, 0, now)
	if err != nil || len(lockedList) != 1 || !proto.Equal(fast[0], lockedList[0]) {
		t.Fatalf("MVCC list waited for a profile/catalogue writer or changed: %v", err)
	}
}
