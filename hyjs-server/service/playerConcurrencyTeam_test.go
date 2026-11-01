package service

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	"gorm.io/gorm/clause"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

func TestPlayerConcurrencyMySQLTeamIsolation(t *testing.T) {
	name := os.Getenv("HYJS_P13_MYSQL_DATABASE")
	if name == "" {
		t.Skip("requires a coordinated isolated hyjs_test_p13_ MySQL database")
	}
	if !strings.HasPrefix(name, "hyjs_test_p13_") || os.Getenv("HYJS_GAME_ACCOUNTS_DATABASE") != "" {
		t.Fatal("requires an isolated team and accounts fixture")
	}

	t.Run("different-teams", func(t *testing.T) {
		f, ids, _ := p13TeamSetup(t)
		p13TeamOpen(t)
		now := time.Now()
		teamA := p13TeamCall(t, f, ids[0], &pb.CSCreateTeam{}, 1, now).(*pb.SCCreateTeam).GetTeam().GetTeamId()
		teamB := p13TeamCall(t, f, ids[1], &pb.CSCreateTeam{}, 2, now).(*pb.SCCreateTeam).GetTeam().GetTeamId()
		if teamA == "" || teamB == "" || teamA == teamB {
			t.Fatal("fixture did not create two distinct teams")
		}

		ctx, cancel := context.WithTimeout(f.ctx, 15*time.Second)
		hold := db.GetIns().WithContext(ctx).Begin(&sql.TxOptions{Isolation: sql.LevelReadCommitted})
		if hold.Error != nil {
			cancel()
			t.Fatal(hold.Error)
		}
		var workers sync.WaitGroup
		defer func() {
			_ = hold.Rollback().Error
			cancel()
			workers.Wait()
		}()
		var policy db.P13TeamPolicy
		if err := hold.Clauses(clause.Locking{Strength: "SHARE"}).First(&policy, 1).Error; err != nil {
			t.Fatal(err)
		}
		var team db.P13Team
		if err := hold.Clauses(clause.Locking{Strength: "UPDATE"}).First(&team, "id=?", teamA).Error; err != nil {
			t.Fatal(err)
		}
		other := make(chan error, 1)
		workers.Add(1)
		go func() {
			defer workers.Done()
			_, err := f.s.p13TeamAt(ctx, ids[1], &pb.CSTeamInvite{TeamId: proto.String(teamB), InvitePlayerIdList: []int64{ids[2]}, InviteServerIdList: []int32{1}}, 3, now)
			other <- err
		}()
		select {
		case err := <-other:
			if err != nil {
				t.Fatalf("other team invitation failed: %v", err)
			}
		case <-time.After(2 * time.Second):
			t.Fatal("other team invitation waited for the held team")
		case <-ctx.Done():
			t.Fatalf("other team invitation timed out: %v", ctx.Err())
		}
		var invite db.P13TeamInvite
		if err := db.GetIns().First(&invite, "team_id=? AND target_id=?", teamB, ids[2]).Error; err != nil {
			t.Fatalf("other team invitation was not committed: %v", err)
		}

		started, same := make(chan struct{}), make(chan error, 1)
		workers.Add(1)
		go func() {
			defer workers.Done()
			close(started)
			_, err := f.s.p13TeamAt(ctx, ids[0], &pb.CSTeamInvite{TeamId: proto.String(teamA), InvitePlayerIdList: []int64{ids[3]}, InviteServerIdList: []int32{1}}, 4, now)
			same <- err
		}()
		select {
		case <-started:
		case <-ctx.Done():
			t.Fatalf("same team invitation did not start: %v", ctx.Err())
		}
		select {
		case err := <-same:
			t.Fatalf("same team invitation passed the held lock: %v", err)
		case <-time.After(100 * time.Millisecond):
		case <-ctx.Done():
			t.Fatalf("same team lock check timed out: %v", ctx.Err())
		}
		if err := hold.Commit().Error; err != nil {
			t.Fatal(err)
		}
		select {
		case err := <-same:
			if err != nil {
				t.Fatalf("same team invitation after release: %v", err)
			}
		case <-ctx.Done():
			t.Fatalf("same team invitation did not finish: %v", ctx.Err())
		}
		invite = db.P13TeamInvite{}
		if err := db.GetIns().First(&invite, "team_id=? AND target_id=?", teamA, ids[3]).Error; err != nil {
			t.Fatalf("same team invitation was not committed: %v", err)
		}
	})

	t.Run("last-slot", func(t *testing.T) {
		f, ids, _ := p13TeamSetup(t)
		p13TeamOpen(t)
		now := time.Now()
		login, err := LoginAccount(f.ctx, fmt.Sprintf("team-isolation-extra-%d", now.UnixNano()), "isolated-local-pass")
		if err != nil {
			t.Fatal(err)
		}
		if _, err := CreatePlayer(f.ctx, login.Id, "队伍隔离候选", "hero_047"); err != nil {
			t.Fatal(err)
		}
		if err := updateProfile(f.ctx, login.Id, func(p *pb.Player, _ *model.GameState) error {
			p.Level = proto.Int32(30)
			return nil
		}); err != nil {
			t.Fatal(err)
		}
		var row db.Profile
		if err := db.GetIns().First(&row, login.Id).Error; err != nil {
			t.Fatal(err)
		}
		recorder := &p10SceneRecorder{}
		detach := f.s.RegisterP10Client(f.ctx, login.Id, row.Session, recorder.send)
		t.Cleanup(detach)
		ids = append(ids, login.Id)
		created := p13TeamCall(t, f, ids[0], &pb.CSTeamCreateAndInvite{InvitePlayerIdList: ids[1:5], InviteServerIdList: []int32{1, 1, 1, 1}}, 10, now).(*pb.SCTeamCreateAndInvite)
		teamID := created.GetTeam().GetTeamId()
		for i, id := range ids[1:4] {
			p13TeamCall(t, f, id, &pb.CSJoinTeam{TeamId: proto.String(teamID)}, int64(11+i), now)
		}
		p13TeamCall(t, f, ids[0], &pb.CSTeamInvite{TeamId: proto.String(teamID), InvitePlayerIdList: []int64{ids[5]}, InviteServerIdList: []int32{1}}, 14, now)

		ctx, cancel := context.WithTimeout(f.ctx, 15*time.Second)
		var workers sync.WaitGroup
		defer func() { cancel(); workers.Wait() }()
		type result struct {
			id  int64
			err error
		}
		ready := make(chan struct{}, 2)
		start := make(chan struct{})
		results := make(chan result, 2)
		for i, id := range ids[4:6] {
			workers.Add(1)
			go func(id, stamp int64) {
				defer workers.Done()
				ready <- struct{}{}
				select {
				case <-start:
				case <-ctx.Done():
					return
				}
				_, err := f.s.p13TeamAt(ctx, id, &pb.CSJoinTeam{TeamId: proto.String(teamID)}, stamp, now)
				results <- result{id: id, err: err}
			}(id, int64(15+i))
		}
		for i := 0; i < 2; i++ {
			select {
			case <-ready:
			case <-ctx.Done():
				t.Fatalf("final-slot workers did not start: %v", ctx.Err())
			}
		}
		close(start)
		var winner, loser int64
		for i := 0; i < 2; i++ {
			select {
			case got := <-results:
				if got.err == nil {
					if winner != 0 {
						t.Fatal("both candidates joined the final slot")
					}
					winner = got.id
				} else {
					if got.err.Error() != "队伍已满" {
						t.Fatalf("final-slot join failed unexpectedly: %v", got.err)
					}
					loser = got.id
				}
			case <-ctx.Done():
				t.Fatalf("final-slot join timed out: %v", ctx.Err())
			}
		}
		if winner == 0 || loser == 0 {
			t.Fatal("final slot did not produce exactly one successful join")
		}
		members, err := p13TeamMembers(db.GetIns(), teamID)
		if err != nil || len(members) != 5 {
			t.Fatalf("final team capacity: members=%d err=%v", len(members), err)
		}
		for _, candidate := range []struct {
			id   int64
			want string
		}{{winner, teamID}, {loser, ""}} {
			member, err := p13TeamMember(db.GetIns(), candidate.id)
			if err != nil || member.TeamID != candidate.want {
				t.Fatalf("candidate membership = %q, want %q: %v", member.TeamID, candidate.want, err)
			}
			player, _ := p10LuckyProfile(t, candidate.id)
			if player.GetTeamId() != candidate.want {
				t.Fatalf("candidate profile team = %q, want %q", player.GetTeamId(), candidate.want)
			}
		}
	})

	t.Run("same-stamp-replay", func(t *testing.T) {
		f, ids, recorders := p13TeamSetup(t)
		p13TeamOpen(t)
		now := time.Now()
		for _, recorder := range recorders {
			recorder.drain()
		}
		type result struct {
			messages []proto.Message
			err      error
		}
		race := func(request proto.Message, stamp int64, teamID string) []proto.Message {
			t.Helper()
			ctx, cancel := context.WithTimeout(f.ctx, 15*time.Second)
			hold := db.GetIns().WithContext(ctx).Begin(&sql.TxOptions{Isolation: sql.LevelReadCommitted})
			if hold.Error != nil {
				cancel()
				t.Fatal(hold.Error)
			}
			var workers sync.WaitGroup
			defer func() {
				_ = hold.Rollback().Error
				cancel()
				workers.Wait()
			}()
			var policy db.P13TeamPolicy
			if err := hold.Clauses(clause.Locking{Strength: "SHARE"}).First(&policy, 1).Error; err != nil {
				t.Fatal(err)
			}
			if teamID == "" {
				var profile db.Profile
				if err := hold.Clauses(clause.Locking{Strength: "UPDATE"}).First(&profile, ids[0]).Error; err != nil {
					t.Fatal(err)
				}
			} else {
				var team db.P13Team
				if err := hold.Clauses(clause.Locking{Strength: "UPDATE"}).First(&team, "id=?", teamID).Error; err != nil {
					t.Fatal(err)
				}
			}
			ready := make(chan struct{}, 2)
			start := make(chan struct{})
			results := make(chan result, 2)
			for i := 0; i < 2; i++ {
				workers.Add(1)
				go func() {
					defer workers.Done()
					ready <- struct{}{}
					select {
					case <-start:
					case <-ctx.Done():
						return
					}
					messages, err := f.s.p13TeamAt(ctx, ids[0], request, stamp, now)
					results <- result{messages: messages, err: err}
				}()
			}
			for i := 0; i < 2; i++ {
				select {
				case <-ready:
				case <-ctx.Done():
					t.Fatalf("same-stamp workers did not start: %v", ctx.Err())
				}
			}
			close(start)
			select {
			case got := <-results:
				t.Fatalf("same-stamp request passed held lock: %v", got.err)
			case <-time.After(100 * time.Millisecond):
			case <-ctx.Done():
				t.Fatalf("same-stamp lock check timed out: %v", ctx.Err())
			}
			if err := hold.Commit().Error; err != nil {
				t.Fatal(err)
			}
			var first []proto.Message
			for i := 0; i < 2; i++ {
				select {
				case got := <-results:
					if got.err != nil || len(got.messages) != 2 {
						t.Fatalf("same-stamp request failed: %v", got.err)
					}
					if i == 0 {
						first = got.messages
					} else if !proto.Equal(first[0], got.messages[0]) {
						t.Fatal("same-stamp response changed")
					}
					player, ok := got.messages[1].(*pb.SCPlayer)
					if !ok || player.GetPlayer().GetPlayerId() != ids[0] {
						t.Fatal("same-stamp replay lost the current actor")
					}
				case <-ctx.Done():
					t.Fatalf("same-stamp requests did not finish: %v", ctx.Err())
				}
			}
			var count int64
			op := string(request.ProtoReflect().Descriptor().Name())
			if err := db.GetIns().Model(&db.P13TeamReceipt{}).Where("player_id=? AND operation=? AND stamp=?", ids[0], op, stamp).Count(&count).Error; err != nil || count != 1 {
				t.Fatalf("same-stamp receipt count = %d: %v", count, err)
			}
			return first
		}
		created := race(&pb.CSCreateTeam{}, 20, "")[0].(*pb.SCCreateTeam)
		teamID := created.GetTeam().GetTeamId()
		if teamID == "" {
			t.Fatal("same-stamp create returned an empty team")
		}
		var teamCount int64
		if err := db.GetIns().Model(&db.P13Team{}).Count(&teamCount).Error; err != nil || teamCount != 1 {
			t.Fatalf("same-stamp create team count = %d: %v", teamCount, err)
		}
		invite := &pb.CSTeamInvite{TeamId: proto.String(teamID), InvitePlayerIdList: []int64{ids[1]}, InviteServerIdList: []int32{1}}
		race(invite, 21, teamID)
		if got := len(p13TeamInvites(recorders[1].drain())); got != 1 {
			t.Fatalf("same-stamp invitation pushes = %d, want 1", got)
		}
		conflict := &pb.CSTeamInvite{TeamId: proto.String(teamID), InvitePlayerIdList: []int64{ids[2]}, InviteServerIdList: []int32{1}}
		if _, err := f.s.p13TeamAt(f.ctx, ids[0], conflict, 21, now); err == nil {
			t.Fatal("same-stamp different invitation was accepted")
		}
		p13TeamCall(t, f, ids[1], &pb.CSJoinTeam{TeamId: proto.String(teamID)}, 22, now)
		for _, recorder := range recorders {
			recorder.drain()
		}
		race(&pb.CSDisbandTeam{TeamId: proto.String(teamID)}, 23, teamID)
		if err := db.GetIns().Model(&db.P13Team{}).Where("id=?", teamID).Count(&teamCount).Error; err != nil || teamCount != 0 {
			t.Fatalf("same-stamp disband team count = %d: %v", teamCount, err)
		}
		for _, id := range ids[:2] {
			member, err := p13TeamMember(db.GetIns(), id)
			if err != nil || member.TeamID != "" {
				t.Fatalf("same-stamp disband retained membership: %v", err)
			}
			player, _ := p10LuckyProfile(t, id)
			if player.GetTeamId() != "" {
				t.Fatal("same-stamp disband retained profile team")
			}
		}
		var disbands int
		for _, message := range recorders[1].drain() {
			if _, ok := message.(*pb.SCDisbandTeam); ok {
				disbands++
			}
		}
		if disbands != 1 {
			t.Fatalf("same-stamp disband pushes = %d, want 1", disbands)
		}
	})
}
