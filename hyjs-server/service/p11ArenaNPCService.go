package service

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"time"

	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

func p11ArenaNPCID(id int64) bool {
	return id > constUtil.P11ArenaNPCIDBase && id <= constUtil.P11ArenaNPCIDBase+constUtil.P11ArenaNPCMaxRank
}

// p11ArenaReserveVirtualSlots preserves witnessed member positions and migrates
// only untouched legacy empty-server seats, without creating fake accounts.
func p11ArenaReserveVirtualSlots(tx *gorm.DB, cfg *db.P11ArenaPolicy) error {
	var count, highest, conflicting int64
	if err := tx.Model(&db.P11ArenaMember{}).Count(&count).Error; err != nil {
		return err
	}
	if err := tx.Model(&db.P11ArenaMember{}).Select("COALESCE(MAX(position),0)").Scan(&highest).Error; err != nil {
		return err
	}
	if err := tx.Model(&db.Profile{}).Where("id>=?", constUtil.P11ArenaNPCIDBase).Count(&conflicting).Error; err != nil {
		return err
	}
	if conflicting != 0 {
		return errors.New("竞技虚拟身份域与实际账号冲突")
	}
	// Pre-NPC servers could assign an unplayed tutorial account rank one.
	// Only untouched legacy seats (zero migration marker and no participation
	// or award facts) are moved. Every witnessed real ranking stays intact.
	var untouched []db.P11ArenaMember
	if err := tx.Where("position>0 AND position<=? AND best_won_rank=0 AND rank_reward_best=0", constUtil.P11ArenaVirtualSlots).Order("position,player_id").Find(&untouched).Error; err != nil {
		return err
	}
	cursor := max(cfg.NextRank, constUtil.P11ArenaVirtualSlots+1, highest+1)
	for _, member := range untouched {
		participated, err := p11ArenaHasHistory(tx, member.PlayerID)
		if err != nil {
			return err
		}
		if participated {
			continue
		}
		if cursor > constUtil.P11ArenaNPCMaxRank {
			return errors.New("竞技虚拟名次域已满")
		}
		if err = tx.Model(&db.P11ArenaMember{}).Where("player_id=? AND rank_reward_best=0", member.PlayerID).Updates(map[string]any{"position": cursor, "rank_reward_best": constUtil.P11ArenaVirtualSlots + 1}).Error; err != nil {
			return err
		}
		highest = cursor
		cursor++
	}
	cfg.NextRank = max(cfg.NextRank, cursor)
	cfg.NextRank = max(cfg.NextRank, constUtil.P11ArenaVirtualSlots+count+1, highest+1)
	if cfg.NextRank > constUtil.P11ArenaNPCMaxRank {
		return errors.New("竞技虚拟名次域已满")
	}
	return nil
}

// Existing duel, daily admission and award records disqualify a seat from the
// empty-server migration, including a player who only defended or lost.
func p11ArenaHasHistory(tx *gorm.DB, id int64) (bool, error) {
	checks := []struct {
		model any
		where string
	}{
		{&db.P11ArenaFightSettlement{}, "player_id=? OR opponent_id=?"},
		{&db.P11ArenaDailyChallenge{}, "player_id=?"},
		{&db.P11ArenaDailyAward{}, "player_id=?"},
		{&db.P11ArenaPrizeAward{}, "player_id=?"},
	}
	for _, check := range checks {
		if !tx.Migrator().HasTable(check.model) {
			continue
		}
		args := []any{id}
		if check.where == "player_id=? OR opponent_id=?" {
			args = append(args, id)
		}
		var count int64
		if err := tx.Model(check.model).Where(check.where, args...).Count(&count).Error; err != nil {
			return false, err
		}
		if count > 0 {
			return true, nil
		}
	}
	return false, nil
}

// p11ArenaBestTarget publishes the fastest permitted jump. Candidate filling
// never adds a higher target beyond this boundary merely to fill nine cards.
func p11ArenaBestTarget(rank int64) int64 {
	switch {
	case rank > 500:
		return max(int64(500), rank*3/4)
	case rank > 100:
		return max(int64(100), rank*9/10)
	case rank > 10:
		return max(int64(10), rank*95/100)
	case rank > 1:
		return rank - 1
	default:
		return 1
	}
}

// p11ArenaNPCState is a nonpersistent projection from original hero rows and
// the same starter builder used by actual players; no profile/account is saved.
func (s *GameService) p11ArenaNPCState(rank int64) (*pb.Player, *model.GameState, error) {
	if rank < 1 || rank > constUtil.P11ArenaNPCMaxRank {
		return nil, nil, errors.New(constUtil.P11ArenaFightRefreshRequired)
	}
	id := constUtil.P11ArenaNPCIDBase + rank
	player := &pb.Player{PlayerId: proto.Int64(id), Name: proto.String(fmt.Sprintf("训练剑士·%d", rank)), Level: proto.Int32(1), HeroWarId: proto.String(constUtil.P11ArenaNPCHeroGUID)}
	hero := &pb.Warrior{Id: proto.String(constUtil.P11ArenaNPCHeroID), Guid: proto.String(constUtil.P11ArenaNPCHeroGUID), Level: proto.Int32(1), Breached: &pb.Breached{Level: proto.Int32(1)}}
	game := &model.GameState{Warriors: []*pb.Warrior{hero}, Formation: []*pb.FormationItem{{FormatPos: proto.Int32(1), BattlePos: proto.Int32(1), Things: []*pb.FormationThing{{Pos: proto.Int32(0), Id: hero.Id, Guid: hero.Guid}}}}}
	return player, game, nil
}

func (s *GameService) p11ArenaNPCMember(rank int64) (*db.P11ArenaMember, error) {
	player, game, err := s.p11ArenaNPCState(rank)
	if err != nil {
		return nil, err
	}
	battle, err := s.battlePlayer(player, game)
	if err != nil {
		return nil, err
	}
	power, err := s.formationFightPower(player, game)
	if err != nil {
		return nil, err
	}
	info := &pb.SimplePlayerInfo{PlayerId: player.PlayerId, ServerId: proto.Int32(int32(s.OriginalServerID())), PlayerName: player.Name, Level: player.Level, HeroWarId: proto.String(constUtil.P11ArenaNPCHeroID), FightPower: proto.Int32(power)}
	rawInfo, err := proto.Marshal(info)
	if err != nil {
		return nil, err
	}
	rawBattle, err := proto.Marshal(battle)
	if err != nil {
		return nil, err
	}
	return &db.P11ArenaMember{PlayerID: player.GetPlayerId(), Rank: rank, Active: true, Info: rawInfo, Battle: rawBattle, SnapshotHash: p6Digest(append(append([]byte(nil), rawInfo...), rawBattle...))}, nil
}

// p11ArenaNPCSlot checks the current rank owner under the arena-policy lock.
// Inactive legacy members still reserve their historical position.
func p11ArenaNPCSlot(tx *gorm.DB, cfg *db.P11ArenaPolicy, id int64) (int64, error) {
	rank := id - constUtil.P11ArenaNPCIDBase
	if !p11ArenaNPCID(id) || rank >= cfg.NextRank {
		return 0, errors.New(constUtil.P11ArenaFightRefreshRequired)
	}
	var count int64
	if err := tx.Model(&db.P11ArenaMember{}).Where("position=?", rank).Count(&count).Error; err != nil {
		return 0, err
	}
	if count != 0 {
		return 0, errors.New(constUtil.P11ArenaFightRefreshRequired)
	}
	return rank, nil
}

func (s *GameService) p11ArenaCandidates(tx *gorm.DB, cfg *db.P11ArenaPolicy, viewer int64, member db.P11ArenaMember, match bool) ([]db.P11ArenaMember, error) {
	var all []db.P11ArenaMember
	if err := tx.Order("position").Find(&all).Error; err != nil {
		return nil, err
	}
	occupied := map[int64]db.P11ArenaMember{}
	for _, row := range all {
		occupied[row.Rank] = row
	}
	var result []db.P11ArenaMember
	used := map[int64]bool{}
	add := func(rank int64) error {
		if rank < 1 || rank >= cfg.NextRank || used[rank] || match && rank == member.Rank {
			return nil
		}
		used[rank] = true
		if row, found := occupied[rank]; found {
			if !row.Active || match && row.PlayerID == viewer {
				return nil
			}
			if match {
				blocked, err := p10Blocked(tx, viewer, row.PlayerID)
				if err != nil {
					return err
				}
				if blocked {
					return nil
				}
			}
			result = append(result, row)
			return nil
		}
		npc, err := s.p11ArenaNPCMember(rank)
		if err != nil {
			return err
		}
		result = append(result, *npc)
		return nil
	}
	if !match {
		for rank := int64(1); rank < cfg.NextRank && len(result) < int(cfg.RankShow); rank++ {
			if err := add(rank); err != nil {
				return nil, err
			}
		}
		return result, nil
	}
	best := p11ArenaBestTarget(member.Rank)
	better := int(cfg.MatchCount) - 2
	if better < 1 {
		better = 1
	}
	span := member.Rank - best
	for i := 0; i < better && span > 0; i++ {
		distance := int64(1)
		if better > 1 {
			distance += (span - 1) * int64(i) / int64(better-1)
		}
		if err := add(member.Rank - distance); err != nil {
			return nil, err
		}
	}
	for rank := member.Rank + 1; rank < cfg.NextRank && rank <= member.Rank+2 && len(result) < int(cfg.MatchCount); rank++ {
		if err := add(rank); err != nil {
			return nil, err
		}
	}
	for distance := int64(1); len(result) < int(cfg.MatchCount) && (member.Rank-distance >= best || member.Rank+distance < cfg.NextRank); distance++ {
		if member.Rank-distance >= best {
			if err := add(member.Rank - distance); err != nil {
				return nil, err
			}
		}
		if len(result) < int(cfg.MatchCount) {
			if err := add(member.Rank + distance); err != nil {
				return nil, err
			}
		}
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Rank < result[j].Rank })
	return result, nil
}

func (s *GameService) p11ArenaNPCPreview(ctx context.Context, id, npcID int64, now time.Time) (*pb.FormationSimpleInfo, []*pb.FormationItem, error) {
	var info *pb.FormationSimpleInfo
	var slots []*pb.FormationItem
	err := db.GetIns().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := ServerGameClockGuardTransaction(tx); err != nil {
			return err
		}
		if _, err := p10LockProfiles(tx, id, id); err != nil {
			return err
		}
		cfg, err := p11ArenaConfig(tx)
		if err != nil {
			return err
		}
		if err = s.p11SyncArenaMembers(tx, cfg, now); err != nil {
			return err
		}
		rank, err := p11ArenaNPCSlot(tx, cfg, npcID)
		if err != nil {
			return err
		}
		player, game, err := s.p11ArenaNPCState(rank)
		if err != nil {
			return err
		}
		battle, err := s.battlePlayer(player, game)
		if err != nil {
			return err
		}
		power, err := s.formationFightPower(player, game)
		if err != nil {
			return err
		}
		hero := battle.GetFormation()[0]
		info = &pb.FormationSimpleInfo{Name: player.Name, FightPower: proto.Int32(power), FormationItemInfoList: []*pb.FormationItemSimpleInfo{{FormatPos: proto.Int32(1), WarInfo: &pb.WarriorSimpleInfo{Pos: proto.Int32(0), HeroId: hero.Id, Level: hero.Level, BreachedLevel: hero.BreachedLevel, BaseProp: hero.Prop}}}}
		slots = game.Formation
		return nil
	})
	return info, slots, err
}
