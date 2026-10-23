package service

import (
	"context"
	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
	"hyjs-server/db"
	pb "hyjs-server/model/protocol"
	"reflect"
)

// redPointSchemaSnapshot belongs to one initialized database and base service.
type redPointSchemaSnapshot struct {
	database *gorm.DB
	tables   map[reflect.Type]bool
}

// InitializeRedPointSchemaReady records startup schema availability without changing it.
// Uninitialized services and alternate test databases retain dynamic compatibility checks.
func (s *GameService) InitializeRedPointSchemaReady() {
	database := db.GetIns()
	snapshot := &redPointSchemaSnapshot{database: database, tables: map[reflect.Type]bool{}}
	for _, table := range []any{&db.RewardDelivery{}, &db.MailMessage{}, &db.PaymentOrder{}, &db.MembershipClaim{}, &db.P5Achievement{}, &db.PeriodInstance{}, &db.P4WelfareRule{}, &db.PaymentActivityRule{}, &db.ActivityRules{}} {
		snapshot.tables[reflect.TypeOf(table)] = database.Migrator().HasTable(table)
	}
	s.p10ClientOwner().redPointSchema.Store(snapshot)
}

func (s *GameService) redPointHasTable(tx *gorm.DB, table any) bool {
	if snapshot := s.p10ClientOwner().redPointSchema.Load(); snapshot != nil && snapshot.database == db.GetIns() {
		if present, ok := snapshot.tables[reflect.TypeOf(table)]; ok {
			return present
		}
	}
	return tx.Migrator().HasTable(table)
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
