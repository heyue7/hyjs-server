package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

func TestP7RushQuery63SkipsOnlyPublicPeriodPrelock(t *testing.T) {
	f := p7Setup(t)
	if err := f.s.PublishP7ReRush(f.ctx); err != nil {
		t.Fatal(err)
	}
	f.s.P1Ready = true
	ctx := context.WithValue(f.ctx, constUtil.GameServiceContextKey, f.s)
	ctx = context.WithValue(ctx, constUtil.GameRequestContextKey, &pb.CSRushCurrentInfo{})
	periods, profiles := 0, 0
	name := "p7-query-period63"
	if err := db.GetIns().Callback().Query().Before("gorm:query").Register(name, func(tx *gorm.DB) {
		if tx.Statement.Table == "p1_period_instances" {
			periods++
		}
		if tx.Statement.Table == "profiles" {
			if lock, ok := tx.Statement.Clauses["FOR"].Expression.(clause.Locking); ok && lock.Strength == "UPDATE" {
				profiles++
			}
		}
	}); err != nil {
		t.Fatal(err)
	}
	defer db.GetIns().Callback().Query().Remove(name)
	reply, err := f.s.P7RushQuery(ctx, f.id, &pb.CSRushCurrentInfo{}, time.Now())
	if err != nil || len(reply[0].(*pb.SCRushCurrentInfo).TaskInfoList) != 3 || periods != 0 || profiles != 1 {
		t.Fatal("query lock scope", periods, profiles, err)
	}
	if err = updateProfileWithTx(ctx, f.id, func(*gorm.DB, *pb.Player, *model.GameState) error { return nil }); err != nil {
		t.Fatal(err)
	}
	if periods != 1 || profiles != 2 {
		t.Fatal("mutation lost original locks", periods, profiles)
	}
	// Recovery-compatible query transactions still save changes and roll back validation failures.
	if err = updateProfileForP7QueryWithTx(ctx, f.id, func(_ *gorm.DB, p *pb.Player, _ *model.GameState) error {
		p.Name = proto.String("查询事务保存检查")
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	player, _ := f.profile(t)
	if player.GetName() != "查询事务保存检查" {
		t.Fatal("query transaction dropped save")
	}
	rejected := errors.New("finite validation rejection")
	if err = updateProfileForP7QueryWithTx(ctx, f.id, func(_ *gorm.DB, p *pb.Player, _ *model.GameState) error {
		p.Name = proto.String("应回滚")
		return rejected
	}); !errors.Is(err, rejected) {
		t.Fatal(err)
	}
	player, _ = f.profile(t)
	if player.GetName() != "查询事务保存检查" {
		t.Fatal("query validation failed to roll back")
	}
	if periods != 1 {
		t.Fatal("query save acquired public period lock", periods)
	}
}
