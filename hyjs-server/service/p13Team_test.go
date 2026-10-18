package service

import (
	"fmt"
	"google.golang.org/protobuf/proto"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
	"sync"
	"testing"
	"time"
)

func p13TeamSetup(t *testing.T) (*p1Fixture, []int64, []*p10SceneRecorder) {
	t.Helper()
	f, ids := p13GuildSetup(t)
	if err := f.s.InitializeP10Scene(f.ctx); err != nil {
		t.Fatal(err)
	}
	if err := f.s.InitializeP13Team(f.ctx); err != nil {
		t.Fatal(err)
	}
	for _, table := range []any{&db.P13TeamReceipt{}, &db.P13TeamInvite{}, &db.P13TeamMember{}, &db.P13Team{}, &db.P13TeamPolicy{}} {
		if err := db.GetIns().Where("1=1").Delete(table).Error; err != nil {
			t.Fatal(err)
		}
	}
	if err := f.s.InitializeP13Team(f.ctx); err != nil {
		t.Fatal(err)
	}
	recorders := make([]*p10SceneRecorder, len(ids))
	for i, id := range ids {
		recorders[i] = &p10SceneRecorder{}
		var row db.Profile
		if err := db.GetIns().First(&row, id).Error; err != nil {
			t.Fatal(err)
		}
		detach := f.s.RegisterP10Client(f.ctx, id, row.Session, recorders[i].send)
		t.Cleanup(detach)
	}
	return f, ids, recorders
}
func p13TeamOpen(t *testing.T) {
	t.Helper()
	if err := db.GetIns().Model(&db.P13TeamPolicy{}).Where("id=1").Update("enabled", true).Error; err != nil {
		t.Fatal(err)
	}
}
func p13TeamCall(t *testing.T, f *p1Fixture, id int64, request proto.Message, stamp int64, now time.Time) proto.Message {
	t.Helper()
	var messages []proto.Message
	var err error
	switch request.(type) {
	case *pb.CSQueryTeam, *pb.CSCheckTeam, *pb.CSQueryTeamTick:
		messages, err = f.s.p13TeamQuery(f.ctx, id, request)
	default:
		messages, err = f.s.p13TeamAt(f.ctx, id, request, stamp, now)
	}
	if err != nil {
		t.Fatal(err)
	}
	blob, err := proto.Marshal(messages[0])
	if err != nil {
		t.Fatal(err)
	}
	clone := messages[0].ProtoReflect().New().Interface()
	if err = proto.Unmarshal(blob, clone); err != nil || !proto.Equal(messages[0], clone) {
		t.Fatal("native original team binary", err)
	}
	return messages[0]
}
func p13TeamInvites(messages []proto.Message) []*pb.SCTeamCreateAndInvite {
	var result []*pb.SCTeamCreateAndInvite
	for _, msg := range messages {
		if invitation, ok := msg.(*pb.SCTeamCreateAndInvite); ok {
			result = append(result, invitation)
		}
	}
	return result
}
func TestP13TeamOriginalManagementInvitationsLeaderPermissionsExitAndRecovery(t *testing.T) {
	f, ids, recs := p13TeamSetup(t)
	now := time.Now()
	if _, err := f.s.p13TeamAt(f.ctx, ids[0], &pb.CSCreateTeam{}, 1, now); err == nil {
		t.Fatal("closed team created")
	}
	p13TeamOpen(t)
	request := &pb.CSTeamCreateAndInvite{InvitePlayerIdList: []int64{ids[1], ids[2], ids[3]}, InviteServerIdList: []int32{1, 1, 1}}
	created := p13TeamCall(t, f, ids[0], request, 1, now).(*pb.SCTeamCreateAndInvite)
	if created.GetInvitePlayerId() != ids[0] || created.GetTeam().GetMaxSize() != 5 || created.GetTeam().GetMemberList()[0].GetPlayerId() != ids[0] {
		t.Fatal("original inviter/first leader/maxSize", created)
	}
	team := created.GetTeam().GetTeamId()
	for i, rec := range recs {
		invites := p13TeamInvites(rec.drain())
		expected := 0
		if i > 0 && i < 4 {
			expected = 1
		}
		if len(invites) != expected {
			t.Fatal("native invite target", i, len(invites), expected)
		}
	}
	p13TeamCall(t, f, ids[0], request, 1, now)
	p13TeamCall(t, f, ids[0], request, 0, now)
	for _, rec := range recs {
		if len(p13TeamInvites(rec.drain())) != 0 {
			t.Fatal("retry duplicate invitation")
		}
	}
	// 第二个被邀请者仍可在第一次加入后接受，邀请绑定队长和该目标成员代数。
	for i, id := range ids[1:3] {
		joined := p13TeamCall(t, f, id, &pb.CSJoinTeam{TeamId: proto.String(team)}, int64(10+i), now).(*pb.SCJoinTeam)
		if joined.GetToPlayerId() != id {
			t.Fatal("original join target", joined)
		}
	}
	query := p13TeamCall(t, f, ids[1], &pb.CSQueryTeam{TeamId: proto.String(team)}, 0, now).(*pb.SCQueryTeam)
	if len(query.GetTeam().GetMemberList()) != 3 || query.GetTeam().GetMemberList()[0].GetPlayerId() != ids[0] {
		t.Fatal("real ordered team", query)
	}
	if _, err := f.s.p13TeamQuery(f.ctx, ids[4], &pb.CSQueryTeam{TeamId: proto.String(team)}); err == nil {
		t.Fatal("foreign team read")
	}
	plain := p13TeamCall(t, f, ids[4], &pb.CSCreateTeam{}, 7, now).(*pb.SCCreateTeam)
	if plain.GetTeam().GetMemberList()[0].GetPlayerId() != ids[4] || plain.GetTeam().GetMaxSize() != 5 {
		t.Fatal("plain original team create", plain)
	}
	ownQuery := p13TeamCall(t, f, ids[4], &pb.CSQueryTeam{}, 0, now).(*pb.SCQueryTeam)
	if ownQuery.GetTeam().GetTeamId() != plain.GetTeam().GetTeamId() {
		t.Fatal("own default team query", ownQuery)
	}
	busy := p13TeamCall(t, f, ids[0], &pb.CSCheckTeam{ToPlayerId: proto.Int64(ids[4]), ToServerid: proto.Int32(1)}, 0, now).(*pb.SCCheckTeam)
	free := p13TeamCall(t, f, ids[0], &pb.CSCheckTeam{ToPlayerId: proto.Int64(ids[3]), ToServerid: proto.Int32(1)}, 0, now).(*pb.SCCheckTeam)
	if busy.GetCheckResult() != 0 || free.GetCheckResult() != 1 {
		t.Fatal("explicit local availability projection", busy, free)
	}

	if _, err := f.s.p13TeamAt(f.ctx, ids[1], &pb.CSTeamKick{TreamId: proto.String(team), ToPlayerId: proto.Int64(ids[2])}, 20, now); err == nil {
		t.Fatal("member kicked peer")
	}
	p13TeamCall(t, f, ids[0], &pb.CSChangeTeamLeader{TeamId: proto.String(team), ToPlayerId: proto.Int64(ids[1])}, 21, now)
	query = p13TeamCall(t, f, ids[0], &pb.CSQueryTeam{TeamId: proto.String(team)}, 0, now).(*pb.SCQueryTeam)
	if query.GetTeam().GetMemberList()[0].GetPlayerId() != ids[1] {
		t.Fatal("old leader still first", query)
	}
	if _, err := f.s.p13TeamAt(f.ctx, ids[3], &pb.CSJoinTeam{TeamId: proto.String(team)}, 22, now); err == nil {
		t.Fatal("old leader invitation survived transfer")
	}
	if _, err := f.s.p13TeamAt(f.ctx, ids[0], &pb.CSDisbandTeam{TeamId: proto.String(team)}, 23, now); err == nil {
		t.Fatal("previous leader retained permission")
	}
	p13TeamCall(t, f, ids[1], &pb.CSTeamKick{TreamId: proto.String(team), ToPlayerId: proto.Int64(ids[2])}, 24, now)
	p, _ := p10LuckyProfile(t, ids[2])
	if p.GetTeamId() != "" {
		t.Fatal("kicked profile retained team id")
	}
	p13TeamCall(t, f, ids[0], &pb.CSExitTeam{TeamId: proto.String(team)}, 25, now)
	sql, _ := db.GetIns().DB()
	sql.Close()
	f.open(t)
	if err := f.s.InitializeP13Team(f.ctx); err != nil {
		t.Fatal(err)
	}
	tick := p13TeamCall(t, f, ids[1], &pb.CSQueryTeamTick{}, 0, now).(*pb.SCQueryTeamTick)
	if tick.GetSeconds() != 3 || tick.GetTeam().GetTeamId() != team || tick.GetTeam().GetMemberList()[0].GetPlayerId() != ids[1] {
		t.Fatal("restart real team recovery", tick)
	}
	empty := p13TeamCall(t, f, ids[0], &pb.CSQueryTeamTick{}, 0, now).(*pb.SCQueryTeamTick)
	if empty.Team == nil || empty.GetTeam().GetTeamId() != "" {
		t.Fatal("empty native tick must retain nonnil Team", empty)
	}
	disband := &pb.CSDisbandTeam{TeamId: proto.String(team)}
	old := p13TeamCall(t, f, ids[1], disband, 26, now)
	again := p13TeamCall(t, f, ids[1], disband, 26, now)
	if !proto.Equal(old, again) {
		t.Fatal("deleted team receipt lost")
	}
	if _, err := f.s.p13TeamAt(f.ctx, ids[3], &pb.CSJoinTeam{TeamId: proto.String(team)}, 27, now); err == nil {
		t.Fatal("disbanded invitation resurrected team")
	}
}
func TestP13TeamConcurrentAcceptUniqueMembershipExactReceiptAndAtomicInviteFailure(t *testing.T) {
	f, ids, recs := p13TeamSetup(t)
	now := time.Now()
	p13TeamOpen(t)
	teams := make([]string, 2)
	for i, leader := range ids[:2] {
		r := p13TeamCall(t, f, leader, &pb.CSTeamCreateAndInvite{InvitePlayerIdList: []int64{ids[2]}, InviteServerIdList: []int32{1}}, 1, now).(*pb.SCTeamCreateAndInvite)
		teams[i] = r.GetTeam().GetTeamId()
	}
	for _, rec := range recs {
		rec.drain()
	}
	var wg sync.WaitGroup
	errs := make(chan error, 2)
	for i, team := range teams {
		wg.Add(1)
		go func(i int, team string) {
			defer wg.Done()
			_, err := f.s.p13TeamAt(f.ctx, ids[2], &pb.CSJoinTeam{TeamId: proto.String(team)}, int64(10+i), now)
			errs <- err
		}(i, team)
	}
	wg.Wait()
	close(errs)
	success := 0
	for err := range errs {
		if err == nil {
			success++
		}
	}
	if success != 1 {
		t.Fatal("double team membership", success)
	}
	member, err := p13TeamMember(db.GetIns(), ids[2])
	if err != nil || member.TeamID == "" {
		t.Fatal("actual joined member missing", err)
	}
	leader := ids[0]
	if member.TeamID == teams[1] {
		leader = ids[1]
	}
	transfer := &pb.CSChangeTeamLeader{TeamId: proto.String(member.TeamID), ToPlayerId: proto.Int64(ids[2])}
	errs = make(chan error, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); _, err := f.s.p13TeamAt(f.ctx, leader, transfer, 20, now); errs <- err }()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	var invitations int64
	db.GetIns().Model(&db.P13TeamInvite{}).Where("target_id=?", ids[2]).Count(&invitations)
	if invitations != 0 {
		t.Fatal("joined player retained old invites")
	}
	var beforeTeams int64
	db.GetIns().Model(&db.P13Team{}).Count(&beforeTeams)
	// 新建与邀请是同一事务，已有队伍目标令整笔创建回滚。
	if _, err = f.s.p13TeamAt(f.ctx, ids[3], &pb.CSTeamCreateAndInvite{InvitePlayerIdList: []int64{ids[2]}, InviteServerIdList: []int32{1}}, 30, now); err == nil {
		t.Fatal("atomic invite accepted occupied target")
	}
	var afterTeams int64
	db.GetIns().Model(&db.P13Team{}).Count(&afterTeams)
	p, _ := p10LuckyProfile(t, ids[3])
	if afterTeams != beforeTeams || p.GetTeamId() != "" {
		t.Fatal("failed invite left created team or profile", afterTeams, beforeTeams, p)
	}
	for _, req := range []proto.Message{&pb.CSTeamCreateAndInvite{InvitePlayerIdList: []int64{ids[4], ids[4]}, InviteServerIdList: []int32{1, 1}}, &pb.CSTeamCreateAndInvite{InvitePlayerIdList: []int64{ids[4]}, InviteServerIdList: []int32{2}}, &pb.CSTeamCreateAndInvite{InvitePlayerIdList: []int64{ids[4]}, InviteServerIdList: nil}} {
		if _, err = f.s.p13TeamAt(f.ctx, ids[3], req, 31, now); err == nil {
			t.Fatal("invalid original paired invitation", req)
		}
	}
}

func TestP13TeamFrozenFiveMemberCapacityConcurrentLastSlotAndLeaveRevokesOldInvite(t *testing.T) {
	f, ids, _ := p13TeamSetup(t)
	now := time.Now()
	p13TeamOpen(t)
	login, err := LoginAccount(f.ctx, fmt.Sprintf("p13-team-extra-%d", time.Now().UnixNano()), "isolated-local-pass")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = CreatePlayer(f.ctx, login.Id, "队伍最后候选", "hero_047"); err != nil {
		t.Fatal(err)
	}
	if err = updateProfile(f.ctx, login.Id, func(p *pb.Player, g *model.GameState) error { p.Level = proto.Int32(30); return nil }); err != nil {
		t.Fatal(err)
	}
	var row db.Profile
	if err = db.GetIns().First(&row, login.Id).Error; err != nil {
		t.Fatal(err)
	}
	rec := &p10SceneRecorder{}
	detach := f.s.RegisterP10Client(f.ctx, login.Id, row.Session, rec.send)
	t.Cleanup(detach)
	ids = append(ids, login.Id)
	created := p13TeamCall(t, f, ids[0], &pb.CSTeamCreateAndInvite{InvitePlayerIdList: ids[1:5], InviteServerIdList: []int32{1, 1, 1, 1}}, 1, now).(*pb.SCTeamCreateAndInvite)
	team := created.GetTeam().GetTeamId()
	for _, id := range ids[1:4] {
		p13TeamCall(t, f, id, &pb.CSJoinTeam{TeamId: proto.String(team)}, 2, now)
	}
	p13TeamCall(t, f, ids[0], &pb.CSTeamInvite{TeamId: proto.String(team), InvitePlayerIdList: []int64{ids[5]}, InviteServerIdList: []int32{1}}, 3, now)
	var wg sync.WaitGroup
	errs := make(chan error, 2)
	for _, id := range ids[4:6] {
		wg.Add(1)
		go func(id int64) {
			defer wg.Done()
			_, err := f.s.p13TeamAt(f.ctx, id, &pb.CSJoinTeam{TeamId: proto.String(team)}, 4, now)
			errs <- err
		}(id)
	}
	wg.Wait()
	close(errs)
	success := 0
	for err := range errs {
		if err == nil {
			success++
		}
	}
	if success != 1 {
		t.Fatal("last slot overfilled", success)
	}
	members, err := p13TeamMembers(db.GetIns(), team)
	if err != nil || len(members) != 5 {
		t.Fatal("frozen maxsize breached", len(members), err)
	}
	// 被邀请者加入、退出其他队伍使旧成员代数邀请失效，不依赖臆造时钟期限。
	quitter := ids[1]
	p13TeamCall(t, f, quitter, &pb.CSExitTeam{TeamId: proto.String(team)}, 5, now)
	if _, err = f.s.p13TeamAt(f.ctx, quitter, &pb.CSJoinTeam{TeamId: proto.String(team)}, 6, now); err == nil {
		t.Fatal("old consumed invitation resurrected after exit")
	}
	fresh := &pb.CSTeamInvite{TeamId: proto.String(team), InvitePlayerIdList: []int64{quitter}, InviteServerIdList: []int32{1}}
	p13TeamCall(t, f, ids[0], fresh, 7, now)
	p13TeamCall(t, f, quitter, &pb.CSJoinTeam{TeamId: proto.String(team)}, 8, now)
}
