package service

import (
	"bytes"
	"context"
	"errors"
	"github.com/google/uuid"
	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
	"math"
	"sort"
	"time"
)

// InitializeP13Team 默认关闭野外组队管理；容量和邀请策略作为本地兼容配置冻结。
func (s *GameService) InitializeP13Team(ctx context.Context) error {
	tx := db.GetIns().WithContext(ctx)
	if err := tx.AutoMigrate(&db.P13TeamPolicy{}, &db.P13Team{}, &db.P13TeamMember{}, &db.P13TeamInvite{}, &db.P13TeamReceipt{}); err != nil {
		return err
	}
	return tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&db.P13TeamPolicy{ID: 1, MaxSize: constUtil.P13TeamMaxSize, Source: constUtil.P13TeamSource}).Error
}
func p13TeamMember(tx *gorm.DB, id int64) (*db.P13TeamMember, error) {
	var m db.P13TeamMember
	err := tx.First(&m, "player_id=?", id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return &db.P13TeamMember{PlayerID: id}, nil
	}
	return &m, err
}
func p13TeamMembers(tx *gorm.DB, teamID string) ([]db.P13TeamMember, error) {
	var rows []db.P13TeamMember
	err := tx.Where("team_id=?", teamID).Order("joined_at,player_id").Find(&rows).Error
	return rows, err
}
func p13TeamInviteValid(tx *gorm.DB, team db.P13Team, target int64) (bool, error) {
	var invite db.P13TeamInvite
	err := tx.First(&invite, "team_id=? AND target_id=?", team.ID, target).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	sender, err := p13TeamMember(tx, invite.InviterID)
	if err != nil {
		return false, err
	}
	receiver, err := p13TeamMember(tx, target)
	if err != nil {
		return false, err
	}
	return invite.AuthorityVersion == team.AuthorityVersion && invite.InviterID == team.LeaderID && sender.TeamID == team.ID && sender.Generation == invite.InviterGeneration && receiver.TeamID == "" && receiver.Generation == invite.TargetGeneration, nil
}
func (s *GameService) p13TeamPlayer(tx *gorm.DB, viewer, id int64) (*pb.TeamMember, error) {
	info, err := s.p10Player(tx, id)
	if err != nil {
		return nil, err
	}
	var row db.Profile
	if err = tx.First(&row, id).Error; err != nil {
		return nil, err
	}
	_, g, err := decodeProfile(&row)
	if err != nil {
		return nil, err
	}
	var viewerRow db.Profile
	if err = tx.First(&viewerRow, viewer).Error; err != nil {
		return nil, err
	}
	_, own, err := decodeProfile(&viewerRow)
	if err != nil {
		return nil, err
	}
	state := int32(0)
	// 原TableText明确state=1表示同一场景；离线或不同实际位置使用0。
	if s.p10ChannelOnline(tx, id) && s.p10ChannelOnline(tx, viewer) && p10SceneScope(g) != "" && p10SceneScope(g) == p10SceneScope(own) {
		state = 1
	}
	hp := float64(0)
	if g.Field != nil && g.Field.Side != nil {
		hp = g.Field.Side.GetCurHp()
		if math.IsNaN(hp) || math.IsInf(hp, 0) || hp < 0 {
			return nil, errors.New("队员实际血量无效")
		}
	}
	unionName := ""
	guild, _, err := P13GuildScope(tx, id)
	if err != nil {
		return nil, err
	}
	if guild != "" {
		var union db.P13Guild
		if err = tx.First(&union, "id=?", guild).Error; err != nil {
			return nil, err
		}
		unionName = union.Name
	}
	return &pb.TeamMember{PlayerId: info.PlayerId, ServerId: proto.Int32(int32(s.OriginalServerID())), HeroId: info.HeroWarId, Level: info.Level, NickName: info.PlayerName, UnionName: proto.String(unionName), FightPower: info.FightPower, State: proto.Int32(state), CurHp: proto.Float64(hp)}, nil
}
func (s *GameService) p13TeamProjection(tx *gorm.DB, viewer int64, team db.P13Team) (*pb.Team, error) {
	if team.ID == "" {
		return &pb.Team{TeamId: proto.String(""), MaxSize: proto.Int32(constUtil.P13TeamMaxSize)}, nil
	}
	members, err := p13TeamMembers(tx, team.ID)
	if err != nil {
		return nil, err
	}
	sort.SliceStable(members, func(i, j int) bool {
		if (members[i].PlayerID == team.LeaderID) != (members[j].PlayerID == team.LeaderID) {
			return members[i].PlayerID == team.LeaderID
		}
		return members[i].JoinedAt < members[j].JoinedAt || members[i].JoinedAt == members[j].JoinedAt && members[i].PlayerID < members[j].PlayerID
	})
	if len(members) == 0 || members[0].PlayerID != team.LeaderID || len(members) > int(team.MaxSize) {
		return nil, errors.New("真实队长或队伍人数事实无效")
	}
	result := &pb.Team{TeamId: proto.String(team.ID), MaxSize: proto.Int32(team.MaxSize)}
	for _, m := range members {
		info, err := s.p13TeamPlayer(tx, viewer, m.PlayerID)
		if err != nil {
			return nil, err
		}
		result.MemberList = append(result.MemberList, info)
	}
	return result, nil
}
func (s *GameService) p13TeamQuery(ctx context.Context, id int64, request proto.Message) ([]proto.Message, error) {
	var response proto.Message
	err := db.GetIns().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := ServerGameClockGuardTransaction(tx); err != nil {
			return err
		}

		own, err := p13TeamMember(tx, id)
		if err != nil {
			return err
		}
		teamID := own.TeamID
		switch b := request.(type) {
		case *pb.CSQueryTeam:
			if b.GetTeamId() != "" {
				teamID = b.GetTeamId()
			}
		case *pb.CSCheckTeam:
			if b.GetToPlayerId() < 1 || int64(b.GetToServerid()) != s.OriginalServerID() {
				return errors.New("仅恢复本地实际队友查询")
			}
			var target db.Profile
			if err = tx.First(&target, b.GetToPlayerId()).Error; err != nil {
				return err
			}
			other, err := p13TeamMember(tx, b.GetToPlayerId())
			if err != nil {
				return err
			}
			available := int32(0)
			if other.TeamID == "" && s.p10ChannelOnline(tx, b.GetToPlayerId()) {
				available = 1
			}
			response = &pb.SCCheckTeam{CheckResult: proto.Int32(available)}
			return nil
		}
		var team db.P13Team
		if teamID != "" {
			if err = tx.First(&team, "id=?", teamID).Error; err != nil {
				return err
			}
			if own.TeamID != team.ID {
				valid, err := p13TeamInviteValid(tx, team, id)
				if err != nil {
					return err
				}
				if !valid {
					return errors.New("当前角色无队伍或邀请查询权限")
				}
			}
		}
		view, err := s.p13TeamProjection(tx, id, team)
		if err != nil {
			return err
		}
		switch request.(type) {
		case *pb.CSQueryTeam:
			response = &pb.SCQueryTeam{Team: view}
		case *pb.CSQueryTeamTick:
			response = &pb.SCQueryTeamTick{Team: view, Seconds: proto.Int32(constUtil.P13TeamTickSeconds)}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return []proto.Message{response}, nil
}

type p13TeamProfile struct {
	row     db.Profile
	player  *pb.Player
	game    *model.GameState
	changed bool
}
type p13TeamPush struct {
	target     int64
	teamID     string
	generation int64
	invitation bool
	revision   int64
	message    proto.Message
}

func p13TeamRequestInfo(request proto.Message) (string, []int64, []int32) {
	switch b := request.(type) {
	case *pb.CSQueryTeam:
		return b.GetTeamId(), nil, nil
	case *pb.CSJoinTeam:
		return b.GetTeamId(), nil, nil
	case *pb.CSExitTeam:
		return b.GetTeamId(), nil, nil
	case *pb.CSDisbandTeam:
		return b.GetTeamId(), nil, nil
	case *pb.CSTeamInvite:
		return b.GetTeamId(), b.GetInvitePlayerIdList(), b.GetInviteServerIdList()
	case *pb.CSTeamCreateAndInvite:
		return "", b.GetInvitePlayerIdList(), b.GetInviteServerIdList()
	case *pb.CSChangeTeamLeader:
		return b.GetTeamId(), []int64{b.GetToPlayerId()}, nil
	case *pb.CSTeamKick:
		return b.GetTreamId(), []int64{b.GetToPlayerId()}, nil
	}
	return "", nil, nil
}
func p13TeamResponse(request proto.Message) proto.Message {
	switch request.(type) {
	case *pb.CSCreateTeam:
		return &pb.SCCreateTeam{}
	case *pb.CSJoinTeam:
		return &pb.SCJoinTeam{}
	case *pb.CSExitTeam:
		return &pb.SCExitTeam{}
	case *pb.CSDisbandTeam:
		return &pb.SCDisbandTeam{}
	case *pb.CSTeamInvite:
		return &pb.SCTeamInvite{}
	case *pb.CSChangeTeamLeader:
		return &pb.SCChangeTeamLeader{}
	case *pb.CSTeamKick:
		return &pb.SCTeamKick{}
	case *pb.CSTeamCreateAndInvite:
		return &pb.SCTeamCreateAndInvite{}
	}
	return nil
}
func p13TeamSetView(response proto.Message, team *pb.Team, actor int64, removed *pb.TeamMember) {
	switch r := response.(type) {
	case *pb.SCCreateTeam:
		r.Team = team
	case *pb.SCJoinTeam:
		r.Team = team
		r.ToPlayerId = proto.Int64(actor)
	case *pb.SCExitTeam:
		r.Team = team
		r.Member = removed
	case *pb.SCDisbandTeam:
		r.TeamId = team.TeamId
	case *pb.SCTeamInvite:
		r.Team = team
	case *pb.SCChangeTeamLeader:
		r.Team = team
	case *pb.SCTeamKick:
		r.Team = team
		r.Member = removed
	case *pb.SCTeamCreateAndInvite:
		r.Team = team
		r.InvitePlayerId = proto.Int64(actor)
	}
}

// p13TeamAt 管理事务锁住独立门禁，再按升序锁所有受影响真实角色；提交后按当前代数推送。
func (s *GameService) p13TeamAt(ctx context.Context, id int64, request proto.Message, stamp int64, now time.Time) ([]proto.Message, error) {
	response := p13TeamResponse(request)
	if response == nil {
		return nil, errors.New("原队伍变更入口未恢复")
	}
	if stamp < 0 {
		return nil, errors.New("队伍请求时间戳无效")
	}
	raw, err := proto.Marshal(request)
	if err != nil {
		return nil, err
	}
	op := string(request.ProtoReflect().Descriptor().Name())
	var player *pb.Player
	var pushes []p13TeamPush
	err = db.GetIns().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := ServerGameClockGuardTransaction(tx); err != nil {
			return err
		}

		var cfg db.P13TeamPolicy
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&cfg, 1).Error; err != nil {
			return err
		}
		// 回执先于已删除队伍读取，解散/退出后重试仍只回放原响应。
		if stamp > 0 {
			var receipt db.P13TeamReceipt
			err := tx.First(&receipt, "player_id=? AND operation=? AND stamp=?", id, op, stamp).Error
			if err == nil {
				if !bytes.Equal(raw, receipt.Request) {
					return errors.New("队伍请求时间戳冲突")
				}
				var row db.Profile
				if err = tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&row, id).Error; err != nil {
					return err
				}
				p, _, err := decodeProfileForCalendarUpdate(tx, &row)
				if err != nil {
					return err
				}
				player = p
				return proto.Unmarshal(receipt.Response, response)
			}
			if !errors.Is(err, gorm.ErrRecordNotFound) {
				return err
			}
		}
		own, err := p13TeamMember(tx, id)
		if err != nil {
			return err
		}
		supplied, targets, servers := p13TeamRequestInfo(request)
		teamID := supplied
		if teamID == "" {
			teamID = own.TeamID
		}
		var team db.P13Team
		var members []db.P13TeamMember
		if teamID != "" {
			if err = tx.First(&team, "id=?", teamID).Error; err != nil {
				return err
			}
			members, err = p13TeamMembers(tx, team.ID)
			if err != nil {
				return err
			}
		}
		lockIDs := map[int64]bool{id: true}
		for _, m := range members {
			lockIDs[m.PlayerID] = true
		}
		for _, target := range targets {
			if target < 1 || target == id {
				return errors.New("队伍对象无效")
			}
			lockIDs[target] = true
		}
		ids := make([]int64, 0, len(lockIDs))
		for target := range lockIDs {
			ids = append(ids, target)
		}
		sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
		profiles := map[int64]*p13TeamProfile{}
		for _, target := range ids {
			var row db.Profile
			if err = tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&row, target).Error; err != nil {
				return err
			}
			p, g, err := decodeProfileForCalendarUpdate(tx, &row)
			if err != nil {
				return err
			}
			profiles[target] = &p13TeamProfile{row: row, player: p, game: g}
		}
		actor := profiles[id]
		player = actor.player
		if !cfg.Enabled {
			return errors.New(constUtil.P13TeamClosed)
		}
		if cfg.Source == "" || cfg.MaxSize != constUtil.P13TeamMaxSize || cfg.InviteTTLMillis != 0 {
			return errors.New("本批队伍兼容配置不匹配冻结值")
		}
		if !s.p10ChannelOnline(tx, id) {
			return errors.New("队伍变更需要实际有效TCP登录")
		}
		if len(targets) > int(cfg.MaxSize)-1 {
			return errors.New("邀请数量超过冻结队伍容量")
		}
		if len(servers) > 0 && len(servers) != len(targets) {
			return errors.New("原邀请玩家和服务器列表长度不同")
		}
		seen := map[int64]bool{}
		for i, target := range targets {
			if seen[target] {
				return errors.New("队伍邀请目标重复")
			}
			seen[target] = true
			if len(servers) > 0 && int64(servers[i]) != s.OriginalServerID() {
				return errors.New("跨服组队尚未恢复")
			}
		}
		isCreate := false
		isInvite := false
		switch request.(type) {
		case *pb.CSCreateTeam:
			isCreate = true
		case *pb.CSTeamCreateAndInvite:
			isCreate = true
			isInvite = true
		case *pb.CSTeamInvite:
			isInvite = true
		}
		if isInvite && (len(targets) == 0 || len(servers) != len(targets)) {
			return errors.New("原邀请目标列表缺失或不配对")
		}
		if isCreate {
			if own.TeamID == "" {
				if teamID != "" {
					return errors.New("创建队伍不能提供其他队伍")
				}
				team = db.P13Team{ID: uuid.NewString(), LeaderID: id, MaxSize: cfg.MaxSize, AuthorityVersion: 1, Revision: 1, CreatedAt: now.UnixMilli()}
				if err = tx.Create(&team).Error; err != nil {
					return err
				}
				if own.Generation == math.MaxInt64 {
					return errors.New("成员代数溢出")
				}
				own.Generation++
				own.TeamID = team.ID
				own.JoinedAt = now.UnixMilli()
				if err = tx.Save(own).Error; err != nil {
					return err
				}
				actor.player.TeamId = proto.String(team.ID)
				actor.changed = true
			} else if team.LeaderID != id {
				return errors.New("当前成员不能创建第二队")
			}
		} else if _, join := request.(*pb.CSJoinTeam); !join && (team.ID == "" || own.TeamID != team.ID) {
			return errors.New("当前角色不属于请求队伍")
		}
		var removed *pb.TeamMember
		changed := false
		if isInvite {
			if team.LeaderID != id {
				return errors.New("仅队长可以邀请")
			}
			current, err := p13TeamMembers(tx, team.ID)
			if err != nil {
				return err
			}
			if len(current) >= int(team.MaxSize) {
				return errors.New("队伍已满")
			}
			for _, target := range targets {
				m, err := p13TeamMember(tx, target)
				if err != nil {
					return err
				}
				if m.TeamID != "" || !s.p10ChannelOnline(tx, target) {
					return errors.New("邀请对象已有队伍或未实际在线")
				}
				valid, err := p13TeamInviteValid(tx, team, target)
				if err != nil {
					return err
				}
				if valid {
					continue
				}
				invitation := db.P13TeamInvite{TeamID: team.ID, TargetID: target, InviterID: id, AuthorityVersion: team.AuthorityVersion, InviterGeneration: own.Generation, TargetGeneration: m.Generation, CreatedAt: now.UnixMilli()}
				if err = tx.Save(&invitation).Error; err != nil {
					return err
				}
				view, err := s.p13TeamProjection(tx, target, team)
				if err != nil {
					return err
				}
				pushes = append(pushes, p13TeamPush{target: target, teamID: team.ID, generation: m.Generation, invitation: true, message: &pb.SCTeamCreateAndInvite{Team: view, InvitePlayerId: proto.Int64(id)}})
			}
		} else {
			switch b := request.(type) {
			case *pb.CSCreateTeam:
			case *pb.CSJoinTeam:
				if team.ID == "" {
					return errors.New("加入队伍编号缺失")
				}
				if own.TeamID == team.ID {
					break
				}
				if own.TeamID != "" {
					return errors.New("角色已有其他队伍")
				}
				valid, err := p13TeamInviteValid(tx, team, id)
				if err != nil {
					return err
				}
				if !valid {
					return errors.New("原队伍邀请缺失或代数失效")
				}
				current, err := p13TeamMembers(tx, team.ID)
				if err != nil {
					return err
				}
				if len(current) >= int(team.MaxSize) {
					return errors.New("队伍已满")
				}
				if own.Generation == math.MaxInt64 {
					return errors.New("成员代数溢出")
				}
				own.Generation++
				own.TeamID = team.ID
				own.JoinedAt = now.UnixMilli()
				if err = tx.Save(own).Error; err != nil {
					return err
				}
				actor.player.TeamId = proto.String(team.ID)
				actor.changed = true
				changed = true
				if err = tx.Where("target_id=?", id).Delete(&db.P13TeamInvite{}).Error; err != nil {
					return err
				}
			case *pb.CSChangeTeamLeader:
				if team.LeaderID != id {
					return errors.New("仅队长可以转让")
				}
				target, err := p13TeamMember(tx, b.GetToPlayerId())
				if err != nil {
					return err
				}
				if target.TeamID != team.ID {
					return errors.New("新队长不属于队伍")
				}
				if team.AuthorityVersion == math.MaxInt64 {
					return errors.New("队长授权代数溢出")
				}
				team.LeaderID = target.PlayerID
				team.AuthorityVersion++
				changed = true
				if err = tx.Where("team_id=?", team.ID).Delete(&db.P13TeamInvite{}).Error; err != nil {
					return err
				}
			case *pb.CSTeamKick, *pb.CSExitTeam:
				targetID := id
				if kick, ok := request.(*pb.CSTeamKick); ok {
					if team.LeaderID != id {
						return errors.New("仅队长可以踢人")
					}
					targetID = kick.GetToPlayerId()
				}
				target, err := p13TeamMember(tx, targetID)
				if err != nil {
					return err
				}
				if target.TeamID != team.ID {
					return errors.New("目标不属于当前队伍")
				}
				if targetID == team.LeaderID {
					return errors.New("队长需先转让或解散")
				}
				removed, err = s.p13TeamPlayer(tx, id, targetID)
				if err != nil {
					return err
				}
				if target.Generation == math.MaxInt64 {
					return errors.New("成员代数溢出")
				}
				target.Generation++
				target.TeamID = ""
				if err = tx.Save(target).Error; err != nil {
					return err
				}
				profiles[targetID].player.TeamId = proto.String("")
				profiles[targetID].changed = true
				changed = true
				if err = tx.Where("target_id=? OR inviter_id=?", targetID, targetID).Delete(&db.P13TeamInvite{}).Error; err != nil {
					return err
				}
			case *pb.CSDisbandTeam:
				if team.LeaderID != id {
					return errors.New("仅队长可以解散")
				}
				for _, m := range members {
					if m.Generation == math.MaxInt64 {
						return errors.New("成员代数溢出")
					}
					m.Generation++
					m.TeamID = ""
					if err = tx.Save(&m).Error; err != nil {
						return err
					}
					profiles[m.PlayerID].player.TeamId = proto.String("")
					profiles[m.PlayerID].changed = true
				}
				if err = tx.Where("team_id=?", team.ID).Delete(&db.P13TeamInvite{}).Error; err != nil {
					return err
				}
				if err = tx.Delete(&team).Error; err != nil {
					return err
				}
				changed = true
			}
		}
		if changed {
			if team.Revision == math.MaxInt64 {
				return errors.New("队伍版本溢出")
			}
			team.Revision++
			if _, disband := request.(*pb.CSDisbandTeam); !disband {
				if err = tx.Save(&team).Error; err != nil {
					return err
				}
			}
		}
		for _, target := range ids {
			row := profiles[target]
			if row.changed {
				if err = saveProfile(tx, &row.row, row.player, row.game); err != nil {
					return err
				}
			}
		}
		view := &pb.Team{TeamId: proto.String(team.ID), MaxSize: proto.Int32(team.MaxSize)}
		if _, disband := request.(*pb.CSDisbandTeam); !disband {
			view, err = s.p13TeamProjection(tx, id, team)
			if err != nil {
				return err
			}
		}
		p13TeamSetView(response, view, id, removed)
		if changed {
			after, err := p13TeamMembers(tx, team.ID)
			if err != nil {
				return err
			}
			all := map[int64]bool{}
			for _, m := range members {
				all[m.PlayerID] = true
			}
			for _, m := range after {
				all[m.PlayerID] = true
			}
			delete(all, id)
			for target := range all {
				m, err := p13TeamMember(tx, target)
				if err != nil {
					return err
				}
				targetView := view
				if _, disband := request.(*pb.CSDisbandTeam); !disband {
					targetView, err = s.p13TeamProjection(tx, target, team)
					if err != nil {
						return err
					}
				}
				message := p13TeamResponse(request)
				p13TeamSetView(message, targetView, id, removed)
				pushes = append(pushes, p13TeamPush{target: target, teamID: m.TeamID, generation: m.Generation, revision: team.Revision, message: message})
			}
		}
		if stamp > 0 {
			encoded, err := proto.Marshal(response)
			if err != nil {
				return err
			}
			return tx.Create(&db.P13TeamReceipt{PlayerID: id, Operation: op, Stamp: stamp, Request: raw, Response: encoded, CreatedAt: now.UnixMilli()}).Error
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	// 提交后按当前会话和实际成员代数复核；换队/转让后的旧邀请和旧变更帧不推送。
	tx := db.GetIns().WithContext(ctx)
	for _, push := range pushes {
		m, err := p13TeamMember(tx, push.target)
		if err != nil || m.Generation != push.generation {
			continue
		}
		if push.invitation {
			var team db.P13Team
			if tx.First(&team, "id=?", push.teamID).Error != nil {
				continue
			}
			valid, err := p13TeamInviteValid(tx, team, push.target)
			if err != nil || !valid {
				continue
			}
		} else if m.TeamID != push.teamID {
			continue
		} else if push.teamID != "" {
			var team db.P13Team
			if tx.First(&team, "id=?", push.teamID).Error != nil || team.Revision != push.revision {
				continue
			}
		}
		s.publishP10Private(ctx, push.target, push.message)
	}
	return []proto.Message{response, &pb.SCPlayer{Player: player}}, nil
}
func (s *GameService) DispatchP13Team(ctx context.Context, id int64, request proto.Message, stamp int64) ([]proto.Message, bool, error) {
	switch request.(type) {
	case *pb.CSQueryTeam, *pb.CSQueryTeamTick, *pb.CSCheckTeam:
		messages, err := s.p13TeamQuery(ctx, id, request)
		return messages, true, err
	case *pb.CSCreateTeam, *pb.CSJoinTeam, *pb.CSExitTeam, *pb.CSDisbandTeam, *pb.CSTeamInvite, *pb.CSChangeTeamLeader, *pb.CSTeamKick, *pb.CSTeamCreateAndInvite:
		messages, err := s.p13TeamAt(ctx, id, request, stamp, time.Now())
		return messages, true, err
	}
	return nil, false, nil
}
