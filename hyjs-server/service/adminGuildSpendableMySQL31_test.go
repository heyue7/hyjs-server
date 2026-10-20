package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"gorm.io/gorm"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
)

// TestAdminGuildSpendableMySQL31RepeatableRead uses the registered isolated database only.
// The barrier pauses after the ownership snapshot and before any profile row lock.
func TestAdminGuildSpendableMySQL31RepeatableRead(t *testing.T) {
	name := os.Getenv("HYJS_P13_MYSQL_DATABASE")
	if name == "" {
		t.Skip("requires registered HYJS_P13_MYSQL_DATABASE")
	}
	if name != "hyjs_test_p13_b31_20261020" {
		t.Fatal("requires the registered batch31 isolated database")
	}
	f, ids := p13GuildSetup(t)
	actor := adminGuildTestActor(t, constUtil.AdminRoleOperator)
	if err := f.s.InitializeAdminGuildSettings(f.ctx); err != nil {
		t.Fatal(err)
	}
	guildID := adminGuildTestCreate(t, f, ids[0], "并发甲")
	baseCtx := context.WithValue(f.ctx, constUtil.AdminServerScopeContextKey, int64(1))
	external, err := db.OpenGameMySQL(context.Background(), f.mysql, false)
	if err != nil {
		t.Fatal(err)
	}
	connection, err := external.DB()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { connection.Close() })

	for _, tc := range []struct {
		name              string
		concurrentBalance int32
		conflict          bool
	}{
		{"balanceChangedMustConflict", 19, true},
		{"walletOnlyChangeMustSurvive", 20, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// Establish a known wallet while retaining the actual role and unrelated profile fields.
			var profile db.Profile
			if err := external.First(&profile, ids[0]).Error; err != nil {
				t.Fatal(err)
			}
			var original map[string]json.RawMessage
			if err := json.Unmarshal([]byte(*profile.Player), &original); err != nil {
				t.Fatal(err)
			}
			original["contribution"] = json.RawMessage("20")
			original["coin"] = json.RawMessage("1000")
			original["futureGuildWallet"] = json.RawMessage(`{"marker":"preserve"}`)
			raw, err := json.Marshal(original)
			if err != nil {
				t.Fatal(err)
			}
			if err = external.Model(&profile).Update("player", string(raw)).Error; err != nil {
				t.Fatal(err)
			}
			beforeGame := *profile.Game
			detail, err := f.s.AdminGuildDetail(baseCtx, guildID)
			if err != nil {
				t.Fatal(err)
			}
			member := detail.Members[0]
			expected, newValue := int32(20), int32(7)
			req := model.AdminGuildMemberUpdateRequest{Version: member.Version, Reason: "RR并发可用贡献验证", Contribution: member.Contribution + 1, TodayCoin: member.TodayCoin, TodayBadge: member.TodayBadge, Role: member.Role, SpendableContribution: &newValue, ExpectedSpendableContribution: &expected}

			type barrierKey struct{}
			ctx, cancel := context.WithTimeout(context.WithValue(baseCtx, barrierKey{}, true), 20*time.Second)
			defer cancel()
			arrived := make(chan struct{})
			release := make(chan struct{})
			var releaseOnce sync.Once
			unblock := func() { releaseOnce.Do(func() { close(release) }) }
			defer unblock()
			var hit atomic.Bool
			callback := "admin-guild-spendable31-" + tc.name
			if err = db.GetIns().Callback().Query().After("gorm:query").Register(callback, func(q *gorm.DB) {
				if q.Error != nil || q.Statement.Context.Value(barrierKey{}) != true || q.Statement.Table != "profiles" {
					return
				}
				if strings.Join(q.Statement.Selects, ",") != "id,server_id,player" {
					return
				}
				if _, locked := q.Statement.Clauses["FOR"]; locked {
					return
				}
				if _, ownedProfiles := q.Statement.Dest.(*[]db.Profile); !ownedProfiles || !hit.CompareAndSwap(false, true) {
					return
				}
				// Read the isolation on the same transaction connection that owns the stale snapshot.
				var isolation string
				if err := q.Statement.ConnPool.QueryRowContext(q.Statement.Context, "SELECT @@transaction_isolation").Scan(&isolation); err != nil {
					q.AddError(err)
					close(arrived)
					return
				}
				if strings.ToUpper(isolation) != "REPEATABLE-READ" {
					q.AddError(fmt.Errorf("requires REPEATABLE-READ, got %s", isolation))
					close(arrived)
					return
				}
				close(arrived)
				select {
				case <-release:
				case <-q.Statement.Context.Done():
					q.AddError(q.Statement.Context.Err())
				}
			}); err != nil {
				t.Fatal(err)
			}
			result := make(chan error, 1)
			finished := make(chan struct{})
			defer func() {
				unblock()
				cancel()
				select {
				case <-finished:
				case <-time.After(5 * time.Second):
					t.Error("admin request goroutine did not stop during cleanup")
				}
				db.GetIns().Callback().Query().Remove(callback)
			}()
			go func() {
				defer close(finished)
				_, saveErr := f.s.AdminUpdateGuildMember(ctx, actor, guildID, ids[0], req)
				result <- saveErr
			}()
			select {
			case <-arrived:
			case <-ctx.Done():
				t.Fatal("admin request did not reach unlocked ownership snapshot", ctx.Err())
			}
			// A distinct connection commits after the admin's RR snapshot and before its lock.
			concurrent := map[string]json.RawMessage{}
			for key, value := range original {
				concurrent[key] = value
			}
			concurrent["contribution"] = json.RawMessage(fmt.Sprint(tc.concurrentBalance))
			concurrent["coin"] = json.RawMessage("777")
			committed, err := json.Marshal(concurrent)
			if err != nil {
				t.Fatal(err)
			}
			if err = external.WithContext(ctx).Model(&db.Profile{}).Where("id=?", ids[0]).Update("player", string(committed)).Error; err != nil {
				t.Fatal(err)
			}
			unblock()
			var saveErr error
			select {
			case saveErr = <-result:
			case <-ctx.Done():
				t.Fatal("admin request did not finish", ctx.Err())
			}
			if tc.conflict {
				if !errors.Is(saveErr, constUtil.ErrAdminConfigConflict) {
					t.Fatalf("stale balance accepted: %v", saveErr)
				}
			} else if saveErr != nil {
				t.Fatal(saveErr)
			}
			if err = external.First(&profile, ids[0]).Error; err != nil {
				t.Fatal(err)
			}
			var actual map[string]json.RawMessage
			if err = json.Unmarshal([]byte(*profile.Player), &actual); err != nil {
				t.Fatal(err)
			}
			wanted := tc.concurrentBalance
			if !tc.conflict {
				wanted = newValue
			}
			if string(actual["contribution"]) != fmt.Sprint(wanted) || string(actual["coin"]) != "777" || *profile.Game != beforeGame {
				t.Fatalf("latest balance/coin/game lost: contribution=%s coin=%s", actual["contribution"], actual["coin"])
			}
			if len(actual) != len(original) {
				t.Fatal("unrelated JSON field count changed")
			}
			for key, value := range original {
				if key != "contribution" && key != "coin" && string(actual[key]) != string(value) {
					t.Fatalf("unrelated field changed: %s", key)
				}
			}
			var savedMember db.P13GuildMember
			if err = external.First(&savedMember, "player_id=?", ids[0]).Error; err != nil {
				t.Fatal(err)
			}
			wantedContribution := member.Contribution
			if !tc.conflict {
				wantedContribution++
			}
			if savedMember.Contribution != wantedContribution {
				t.Fatal("conflict changed cumulative contribution", savedMember.Contribution)
			}
		})
	}
}
