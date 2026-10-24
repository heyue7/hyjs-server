package service

import (
	"context"
	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
	"hyjs-server/db"
	pb "hyjs-server/model/protocol"
)

// InitializeRedPointSchemaReady preserves the legacy fixture API.
// Initialization failures leave no snapshot and retain dynamic compatibility checks;
// production startup calls InitializeGameSchema64 directly to return its error.
func (s *GameService) InitializeRedPointSchemaReady() {
	_ = s.InitializeGameSchema64(context.Background())
}

func (s *GameService) redPointHasTable(tx *gorm.DB, table any) bool {
	return gameSchemaHasTable(tx.WithContext(bindFieldRuntime(tx.Statement.Context, s)), table)
}

// afterRedPointRequest retains the existing frame selection and synchronous post-commit order.
// A shared-read failure retries each original group independently so one domain cannot hide the other.
func (s *GameService) afterRedPointRequest(ctx context.Context, id int64, request proto.Message, responses []proto.Message) {
	inventory := inventoryMailRequestRefresh(request, responses)
	welfare := welfareAchievementRequestRefresh(request, responses)
	if !inventory && !welfare {
		return
	}
	points, err := s.requestRedPoints(ctx, id, inventory, welfare)
	if err != nil {
		if inventory {
			s.publishInventoryMailRedPoints(ctx, id)
		}
		if welfare {
			if point, e := s.WelfareAchievementRedPoints(ctx, id); e == nil {
				s.publishP10Private(ctx, id, point)
			}
		}
		return
	}
	s.publishP10PrivateBatch(ctx, id, points)
}

func (s *GameService) requestRedPoints(ctx context.Context, id int64, inventory, welfare bool) ([]proto.Message, error) {
	var points []proto.Message
	err := db.GetIns().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var row db.Profile
		if err := tx.First(&row, id).Error; err != nil {
			return err
		}
		player, game, err := decodeProfile(&row)
		if err != nil {
			return err
		}
		var point *pb.SCRedPoint
		if inventory {
			point, err = s.inventoryMailRedPoints(ctx, tx, id, player, game)
			if err != nil {
				return err
			}
			points = append(points, point)
		}
		if welfare {
			point, err = s.welfareAchievementRedPoints(ctx, tx, id, player, game)
			if err != nil {
				return err
			}
			points = append(points, point)
		}
		return nil
	})
	return points, err
}
