package service

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	pb "hyjs-server/model/protocol"
)

// Run only with an explicitly allocated hyjs_test_p1_* MySQL database. The
// READ COMMITTED transactions match updateProfileWithTx's production contract.
func TestRechargeDirectoryIndexMySQLBlockedCommitAndRollback(t *testing.T) {
	if os.Getenv("HYJS_P1_MYSQL_DATABASE") == "" {
		t.Skip("requires explicitly allocated isolated MySQL")
	}
	f := p1Setup(t)
	if db.GetIns().Dialector.Name() != "mysql" {
		t.Fatal("MySQL fixture required")
	}
	pool, err := db.GetIns().DB()
	if err != nil {
		t.Fatal(err)
	}
	pool.SetMaxOpenConns(4)
	var columns []struct {
		ColumnName string
		DataType   string
		ByteLength int64
	}
	if err := db.GetIns().Raw(`SELECT COLUMN_NAME, DATA_TYPE, CHARACTER_OCTET_LENGTH AS byte_length
		FROM information_schema.COLUMNS WHERE TABLE_SCHEMA=DATABASE() AND TABLE_NAME=? AND COLUMN_NAME IN ? ORDER BY COLUMN_NAME`,
		"recharge_directory_members", []string{"domain", "instance_id"}).Scan(&columns).Error; err != nil {
		t.Fatal(err)
	}
	if len(columns) != 2 || columns[0].ColumnName != "domain" || columns[0].DataType != "varbinary" || columns[0].ByteLength != 8 || columns[1].ColumnName != "instance_id" || columns[1].DataType != "varbinary" || columns[1].ByteLength != 512 {
		t.Fatal("unexpected binary index column sizes", columns)
	}
	var indexes []struct {
		ColumnName string
		NonUnique  int64
		SubPart    sql.NullInt64
	}
	if err := db.GetIns().Raw(`SELECT COLUMN_NAME, NON_UNIQUE, SUB_PART FROM information_schema.STATISTICS
		WHERE TABLE_SCHEMA=DATABASE() AND TABLE_NAME=? AND INDEX_NAME=? ORDER BY SEQ_IN_INDEX`,
		"recharge_directory_members", "idx_recharge_directory_binding").Scan(&indexes).Error; err != nil {
		t.Fatal(err)
	}
	if len(indexes) != 3 || indexes[0].ColumnName != "domain" || indexes[1].ColumnName != "instance_id" || indexes[2].ColumnName != "child_index" {
		t.Fatal("unexpected directory unique index", indexes)
	}
	for _, index := range indexes {
		if index.NonUnique != 0 || index.SubPart.Valid {
			t.Fatal("directory index is not fully unique", indexes)
		}
	}
	for _, commit := range []bool{true, false} {
		tail := "0"
		if commit {
			tail = "1"
		}
		suffix := strconv.FormatInt(time.Now().UnixNano(), 36) + tail
		prefix := "rechargeTotal_"
		candidate := rechargeDirectoryCandidate{domain: rechargeDirectoryP3, id: prefix + strings.Repeat("🐟", 128-len(prefix)-len(suffix)) + suffix}
		first := db.GetIns().WithContext(f.ctx).Begin(&sql.TxOptions{Isolation: sql.LevelReadCommitted})
		if first.Error != nil {
			t.Fatal(first.Error)
		}
		firstIndex, err := rechargeDirectoryIndex(first, candidate, 1)
		if err != nil {
			first.Rollback()
			t.Fatal(err)
		}
		type result struct {
			index int32
			err   error
		}
		started := make(chan struct{})
		finished := make(chan result, 1)
		go func() {
			var index int32
			close(started)
			err := db.GetIns().WithContext(f.ctx).Transaction(func(tx *gorm.DB) error {
				var err error
				index, err = rechargeDirectoryIndex(tx, candidate, 1)
				return err
			}, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
			finished <- result{index: index, err: err}
		}()
		<-started
		select {
		case got := <-finished:
			first.Rollback()
			t.Fatalf("concurrent binding did not wait for uncommitted insert: %+v", got)
		case <-time.After(200 * time.Millisecond):
		}
		if commit {
			err = first.Commit().Error
		} else {
			err = first.Rollback().Error
		}
		if err != nil {
			t.Fatal(err)
		}
		select {
		case got := <-finished:
			if got.err != nil || got.index <= 0 || (commit && got.index != firstIndex) {
				t.Fatalf("commit=%t first=%d second=%+v", commit, firstIndex, got)
			}
		case <-time.After(5 * time.Second):
			t.Fatal("concurrent index did not finish after transaction ended")
		}
		if count := p1Count(t, &db.RechargeDirectoryMember{}, "domain=? AND instance_id=? AND child_index=?", candidate.domain, candidate.id, 1); count != 1 {
			t.Fatalf("duplicate or missing mapping: %d", count)
		}
	}
}

// Exercise the full profile/receipt transaction with two independently paid
// roles and concurrent replay, once for each P1 and P3 claim order.
func TestRechargeDirectoryMySQLConcurrentClaimsAndReplay(t *testing.T) {
	if os.Getenv("HYJS_P4_MYSQL_DATABASE") == "" {
		t.Skip("requires explicitly allocated isolated MySQL")
	}
	f := rechargeDirectoryFixture(t)
	if db.GetIns().Dialector.Name() != "mysql" {
		t.Fatal("MySQL fixture required")
	}
	pool, err := db.GetIns().DB()
	if err != nil {
		t.Fatal(err)
	}
	// Four simultaneous requests plus the transaction holding both profile rows.
	pool.SetMaxOpenConns(6)
	now := time.Now()
	peer, err := LoginAccount(f.ctx, fmt.Sprintf("directory-concurrent-%d", now.UnixNano()), "fixture-password")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := CreatePlayer(f.ctx, peer.Id, "并发目录角色", "hero_043"); err != nil {
		t.Fatal(err)
	}
	peerSession := rechargeDirectoryBindClient(t, f, peer.Id, constUtil.RechargeTotalDirectoryMinBuild, constUtil.RechargeTotalDirectoryMinVersion)
	peerCtx := context.WithValue(f.ctx, ggoNativeSessionKey{}, peerSession)

	p1 := db.PeriodInstance{ID: "rechargeTotal_p4_concurrent", Title: "并发公共累充", Enabled: true, Kind: "payment_amount", Mode: "cumulative", Goal: 600, Rewards: "coin,11", StartAt: now.Add(-time.Hour).UnixMilli(), EndAt: now.Add(time.Hour).UnixMilli(), RewardEndAt: now.Add(2 * time.Hour).UnixMilli(), Source: "isolated fixture"}
	p3 := db.PaymentActivityRule{ID: "rechargeTotal_concurrent_local_p3", Kind: "total", Enabled: true, Threshold: 600, Reward: "coin,13", Name: "并发旧累充", Source: "isolated fixture", StartAt: p1.StartAt, EndAt: p1.EndAt, RewardEndAt: p1.RewardEndAt}
	for _, row := range []any{&p1, &p3} {
		if err := db.GetIns().Create(row).Error; err != nil {
			t.Fatal(err)
		}
	}
	peerFixture := *f
	peerFixture.id = peer.Id
	peerFixture.ctx = peerCtx
	recharge75Order(t, f, "directory-concurrent-a", constUtil.P3PaymentConfirmed, now.Add(-time.Minute), 600)
	recharge75Order(t, &peerFixture, "directory-concurrent-b", constUtil.P3PaymentConfirmed, now.Add(-time.Minute+time.Millisecond), 600)

	type actor struct {
		id     int64
		ctx    context.Context
		before int64
		p1, p3 *pb.ActivityItem
	}
	actors := []actor{{id: f.id, ctx: f.ctx}, {id: peer.Id, ctx: peerCtx}}
	for i := range actors {
		var row db.Profile
		if err := db.GetIns().First(&row, actors[i].id).Error; err != nil {
			t.Fatal(err)
		}
		player, _, err := decodeProfile(&row)
		if err != nil {
			t.Fatal(err)
		}
		actors[i].before = player.GetCoin()
		messages, err := f.s.P1Activity(actors[i].ctx, actors[i].id, &pb.CSActivityInfo{ActivityId: proto.String(constUtil.RechargeTotalDirectoryID)})
		if err != nil {
			t.Fatal(err)
		}
		info := messages[0].(*pb.SCActivityInfo)
		actors[i].p1 = rechargeDirectoryItem(t, info, rechargeDirectoryP1, p1.ID, 1)
		actors[i].p3 = rechargeDirectoryItem(t, info, rechargeDirectoryP3, p3.ID, 1)
		if len(info.ItemList) != 2 || actors[i].p1.GetStatus() != 1 || actors[i].p3.GetStatus() != 1 {
			t.Fatalf("role %d missing independently earned rewards: %v", actors[i].id, info)
		}
	}
	if actors[0].p1.GetIndex() != actors[1].p1.GetIndex() || actors[0].p3.GetIndex() != actors[1].p3.GetIndex() {
		t.Fatal("the same activity mapped to different indices across roles")
	}

	type result struct {
		actor int
		reply *pb.SCActivityReward
		err   error
	}
	for phase := 0; phase < 2; phase++ {
		// Queue four real service transactions behind both locked profile rows,
		// so each actor submits a claim and its replay before either can commit.
		holder := db.GetIns().WithContext(f.ctx).Begin(&sql.TxOptions{Isolation: sql.LevelReadCommitted})
		if holder.Error != nil {
			t.Fatal(holder.Error)
		}
		var locked []db.Profile
		if err := holder.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id IN ?", []int64{actors[0].id, actors[1].id}).Order("id").Find(&locked).Error; err != nil || len(locked) != 2 {
			holder.Rollback()
			t.Fatalf("could not lock both profiles: rows=%d err=%v", len(locked), err)
		}
		ctx, cancel := context.WithTimeout(f.ctx, 30*time.Second)
		started := make(chan struct{}, 4)
		results := make(chan result, 4)
		for i, a := range actors {
			item := a.p1
			if (phase+i)%2 == 1 {
				item = a.p3
			}
			for copyIndex := 0; copyIndex < 2; copyIndex++ {
				request := &pb.CSActivityReward{ActivityId: proto.String(constUtil.RechargeTotalDirectoryID), Index: proto.Int32(item.GetIndex()), GetResources: proto.Clone(item).(*pb.ActivityItem).RewardList}
				go func(index int, role actor, request *pb.CSActivityReward) {
					started <- struct{}{}
					requestCtx := context.WithValue(ctx, ggoNativeSessionKey{}, role.ctx.Value(ggoNativeSessionKey{}))
					messages, err := f.s.P1Activity(requestCtx, role.id, request)
					var reply *pb.SCActivityReward
					if err == nil {
						var ok bool
						if len(messages) == 1 {
							reply, ok = messages[0].(*pb.SCActivityReward)
						}
						if !ok {
							err = fmt.Errorf("unexpected reward response: %v", messages)
						}
					}
					results <- result{actor: index, reply: reply, err: err}
				}(i, a, request)
			}
		}
		for i := 0; i < 4; i++ {
			<-started
		}
		select {
		case got := <-results:
			holder.Rollback()
			cancel()
			t.Fatalf("claim completed while its profile was locked: %+v", got)
		case <-time.After(200 * time.Millisecond):
		}
		if err := holder.Commit().Error; err != nil {
			cancel()
			t.Fatal(err)
		}
		var grants [2]int
		var replays [2]int
		for i := 0; i < 4; i++ {
			select {
			case got := <-results:
				if got.err != nil {
					cancel()
					t.Fatalf("phase %d role %d: %v", phase, got.actor, got.err)
				}
				if len(got.reply.GetItemList()) != 2 {
					cancel()
					t.Fatalf("phase %d role %d lost directory items", phase, got.actor)
				}
				gain := got.reply.GetGet().GetSimps()
				if len(gain) == 0 {
					replays[got.actor]++
					continue
				}
				want := int32(11)
				if (phase+got.actor)%2 == 1 {
					want = 13
				}
				if len(gain) != 1 || gain[0].GetId() != "coin" || gain[0].GetNum() != want {
					cancel()
					t.Fatalf("phase %d role %d unexpected gain: %v", phase, got.actor, gain)
				}
				grants[got.actor]++
			case <-ctx.Done():
				cancel()
				t.Fatal("timed out waiting for concurrent claims")
			}
		}
		cancel()
		if grants != [2]int{1, 1} || replays != [2]int{1, 1} {
			t.Fatalf("phase %d: grants=%v replays=%v", phase, grants, replays)
		}
	}
	for _, a := range actors {
		var row db.Profile
		if err := db.GetIns().First(&row, a.id).Error; err != nil {
			t.Fatal(err)
		}
		player, _, err := decodeProfile(&row)
		if err != nil {
			t.Fatal(err)
		}
		if player.GetCoin() != a.before+24 {
			t.Fatalf("role %d: coin before=%d after=%d", a.id, a.before, player.GetCoin())
		}
		if count := p1Count(t, &db.RewardDelivery{}, "player_id=? AND source_key=? AND claimed_at>0", a.id, "activity:"+p1.ID); count != 1 {
			t.Fatalf("role %d: P1 claimed receipt count=%d", a.id, count)
		}
		if count := p1Count(t, &db.PaymentActivityState{}, "player_id=? AND activity_id=? AND claimed=?", a.id, p3.ID, true); count != 1 {
			t.Fatalf("role %d: P3 claimed state count=%d", a.id, count)
		}
		messages, err := f.s.P1Activity(a.ctx, a.id, &pb.CSActivityInfo{ActivityId: proto.String(constUtil.RechargeTotalDirectoryID)})
		if err != nil {
			t.Fatal(err)
		}
		info := messages[0].(*pb.SCActivityInfo)
		if len(info.ItemList) != 2 || rechargeDirectoryItem(t, info, rechargeDirectoryP1, p1.ID, 1).GetStatus() != 2 || rechargeDirectoryItem(t, info, rechargeDirectoryP3, p3.ID, 1).GetStatus() != 2 {
			t.Fatalf("role %d: claimed directory state not persisted: %v", a.id, info)
		}
	}
}

func TestRechargeDirectoryMySQLP3ObservationBlocksDisable(t *testing.T) {
	if os.Getenv("HYJS_P1_MYSQL_DATABASE") == "" {
		t.Skip("requires explicitly allocated isolated MySQL")
	}
	f := p1Setup(t)
	pool, err := db.GetIns().DB()
	if err != nil {
		t.Fatal(err)
	}
	pool.SetMaxOpenConns(4)
	if err := db.GetIns().AutoMigrate(&db.PaymentActivityRule{}, &db.PaymentActivityState{}); err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	key := "rechargeTotal_mysql_" + strconv.FormatInt(now.UnixNano(), 36) + "_local_p3"
	rule := db.PaymentActivityRule{ID: key, Kind: "total", Name: "隔离累充", Source: "fixture", Enabled: true, Threshold: 100, Reward: "coin,1", StartAt: now.Add(-time.Hour).UnixMilli(), EndAt: now.Add(time.Hour).UnixMilli(), RewardEndAt: now.Add(time.Hour).UnixMilli()}
	if err := db.GetIns().Create(&rule).Error; err != nil {
		t.Fatal(err)
	}
	first := db.GetIns().WithContext(f.ctx).Begin(&sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if first.Error != nil {
		t.Fatal(first.Error)
	}
	defer first.Rollback()
	rows, err := f.s.rechargeDirectoryCandidates(first, f.id, now.UnixMilli())
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, row := range rows {
		found = found || (row.domain == rechargeDirectoryP3 && row.id == key)
	}
	if !found {
		t.Fatal("published P3 instance absent from observation")
	}
	started := make(chan struct{})
	done := make(chan error, 1)
	go func() {
		close(started)
		done <- db.GetIns().Model(&db.PaymentActivityRule{}).Where("id=?", key).Update("enabled", false).Error
	}()
	<-started
	select {
	case err := <-done:
		t.Fatalf("disable passed a live observation before commit: %v", err)
	case <-time.After(200 * time.Millisecond):
	}
	if err := first.Commit().Error; err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("disable did not finish after observation committed")
	}
	rows, err = f.s.rechargeDirectoryCandidates(db.GetIns().WithContext(f.ctx), f.id, now.UnixMilli())
	if err != nil {
		t.Fatal(err)
	}
	for _, row := range rows {
		if row.domain == rechargeDirectoryP3 && row.id == key {
			t.Fatal("disabled P3 instance remained eligible")
		}
	}
}
