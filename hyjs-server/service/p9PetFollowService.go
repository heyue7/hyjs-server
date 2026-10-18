package service

import (
	"context"

	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

// P9FollowPetInfo projects the original main camp's guarded native pet. Other
// camp pets affect their own hero and do not create extra world avatars.
func (s *GameService) P9FollowPetInfo(g *model.GameState) (*pb.PetSimpleInfo, error) {
	for _, slot := range g.Formation {
		if slot.GetFormatPos() != 1 {
			continue
		}
		_, _, id, err := s.P9GuardProperties(g, slot)
		if err != nil {
			return nil, err
		}
		if id != "" {
			return &pb.PetSimpleInfo{Pos: proto.Int32(1), PetId: proto.String(id)}, nil
		}
	}
	return nil, nil
}

func (s *GameService) P9OwnMovePlayerInfo(ctx context.Context, id int64) ([]proto.Message, error) {
	out := &pb.SCMovePlayerInfo{}
	err := s.p3Profile(ctx, id, func(tx *gorm.DB, row *db.Profile, p *pb.Player, g *model.GameState) error {
		pet, err := s.P9FollowPetInfo(g)
		if err != nil {
			return err
		}
		heroID, err := P5WorldHeroID(p, g)
		if err != nil {
			return err
		}
		info := &pb.MovePlayerInfo{ServerId: proto.Int32(1), PlayerId: proto.Int64(id), PlayerName: p.Name, Level: p.Level, WarId: proto.String(heroID), ServerName: proto.String("本地测试"), PetId: proto.String(pet.GetPetId())}
		if g.Field != nil {
			info.PlayerSide = g.Field.Side
		}
		out.PlayerInfos = []*pb.MovePlayer{{Position: g.Position, Info: info}}
		return nil
	})
	return []proto.Message{out}, err
}

func (s *GameService) DispatchP9PetFollow(ctx context.Context, id int64, b proto.Message) ([]proto.Message, bool, error) {
	switch request := b.(type) {
	case *pb.CSRefreshMovePlayerInfo:
		out, err := s.P9OwnMovePlayerInfo(ctx, id)
		return out, true, err
	case *pb.CSMovePlayerInfo:
		if len(request.GetPlayerIds()) > 1 || len(request.GetPlayerIds()) == 1 && request.GetPlayerIds()[0] != id {
			return nil, false, nil
		}
		out, err := s.P9OwnMovePlayerInfo(ctx, id)
		return out, true, err
	}
	return nil, false, nil
}
