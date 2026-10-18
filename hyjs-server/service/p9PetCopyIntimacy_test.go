package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

func TestP9CopyIntimacy(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "p9-copy-intimacy.db")
	schema := os.Getenv("HYJS_P9_MYSQL_DATABASE")
	open := func() {
		var err error
		if schema == "" {
			err = db.InitGame(ctx, path)
		} else {
			if !strings.HasPrefix(schema, "hyjs_test_p9_") {
				t.Fatal("P9 isolation")
			}
			err = db.InitGameMySQL(ctx, db.MySQLConfig{Host: os.Getenv("HYJS_MYSQL_HOST"), User: os.Getenv("HYJS_MYSQL_USER"), Password: os.Getenv("HYJS_MYSQL_PASSWORD"), Database: schema})
		}
		if err != nil {
			t.Fatal(err)
		}
	}
	open()
	defer func() { pool, _ := db.GetIns().DB(); pool.Close() }()
	s := &GameService{}
	if err := s.LoadTables("../../analysis/tables"); err != nil {
		t.Fatal(err)
	}
	if err := s.LoadP9Tables("../../analysis/tables"); err != nil {
		t.Fatal(err)
	}
	for _, init := range []func(context.Context) error{s.InitializeP1, s.InitializeP9Pets, s.InitializeP9PetNext, s.InitializeP9CopyIntimacy} {
		if err := init(ctx); err != nil {
			t.Fatal(err)
		}
	}
	stamp := int64(1110000)
	fresh := func(name string, guard bool) (int64, *pb.Pet) {
		t.Helper()
		row, err := LoginAccount(ctx, "p9-copy-"+name, "isolated-password")
		if err != nil {
			t.Fatal(err)
		}
		if _, err = CreatePlayer(ctx, row.Id, "原攻略亲密隔离", "hero_043"); err != nil {
			t.Fatal(err)
		}
		if err = updateProfile(ctx, row.Id, func(p *pb.Player, g *model.GameState) error {
			p.Level = proto.Int32(34)
			_, err := s.GrantFixedReward(p, g, "item_129,31")
			return err
		}); err != nil {
			t.Fatal(err)
		}
		stamp++
		out, _, err := s.DispatchP9Pets(ctx, row.Id, &pb.CSPetFirstRecruit{}, stamp)
		if err != nil {
			t.Fatal(err)
		}
		pet := out[0].(*pb.SCPetFirstRecruit).GetGet().GetPets()[0]
		if guard {
			stamp++
			if _, _, err = s.DispatchP9PetNext(ctx, row.Id, &pb.CSFormationOn{Id: pet.Id, Guid: pet.Guid, FormatPos: proto.Int32(1)}, stamp); err != nil {
				t.Fatal(err)
			}
		}
		return row.Id, pet
	}
	apply := func(id int64, source string, fail bool) ([]*pb.PetAddIntimacy, error) {
		var out []*pb.PetAddIntimacy
		err := updateProfileWithTx(ctx, id, func(tx *gorm.DB, p *pb.Player, g *model.GameState) error {
			var err error
			out, err = s.ApplyP9CopyIntimacy(tx, id, p, g, source)
			if err != nil {
				return err
			}
			if fail {
				return errors.New("isolated settlement rollback")
			}
			return nil
		})
		if err != nil {
			return nil, err
		}
		return out, nil
	}
	owned := func(id int64) *pb.Pet {
		t.Helper()
		pets, err := P9PetList(db.GetIns(), id)
		if err != nil || len(pets) != 1 {
			t.Fatal("owned source", err)
		}
		return pets[0]
	}
	facts := func(id int64, source string) []P9CopyIntimacyFact {
		t.Helper()
		var row db.P9PetCopyGrant
		if err := db.GetIns().First(&row, "player_id=? AND source_key=?", id, source).Error; err != nil {
			t.Fatal(err)
		}
		if row.CompletedCount != 1 {
			t.Fatal("native single completion count was not frozen")
		}
		var result []P9CopyIntimacyFact
		if err := json.Unmarshal(row.Facts, &result); err != nil {
			t.Fatal(err)
		}
		return result
	}
	t.Run("native-earned-two-six-concurrent-original-receipt-and-reopen", func(t *testing.T) {
		id, pet := fresh("concurrent", true)
		var wg sync.WaitGroup
		results := make(chan []*pb.PetAddIntimacy, 6)
		errs := make(chan error, 6)
		for i := 0; i < 6; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				out, err := apply(id, "copy:complete-1", false)
				if err != nil {
					errs <- err
				} else {
					results <- out
				}
			}()
		}
		wg.Wait()
		close(results)
		close(errs)
		for err := range errs {
			t.Fatal(err)
		}
		for out := range results {
			if len(out) != 1 || out[0].GetGuid() != pet.GetGuid() || out[0].GetAddExp() != 2 || out[0].GetCurExp() != 2 || out[0].GetPet().GetEat().GetExp() != 2 {
				t.Fatalf("original native PetAddIntimacy %+v", out)
			}
		}
		if owned(id).GetEat().GetExp() != 2 {
			t.Fatal("duplicated earning")
		}
		f := facts(id, "copy:complete-1")
		if len(f) != 1 || f[0].Earned != 2 || f[0].Applied != 2 || f[0].Deferred != 0 {
			t.Fatal("earning provenance")
		}
		pool, _ := db.GetIns().DB()
		pool.Close()
		open()
		out, err := apply(id, "copy:complete-1", false)
		if err != nil || len(out) != 1 || out[0].GetCurExp() != 2 {
			t.Fatal("receipt reopen", err)
		}
		next, err := apply(id, "sweep:100:1/1", false)
		if err != nil || next[0].GetAddExp() != 2 || owned(id).GetEat().GetExp() != 4 {
			t.Fatal("original paid-sweep source", err)
		}
	})
	t.Run("native-ten-successful-single-sweeps-earn-twenty-with-independent-receipts", func(t *testing.T) {
		id, pet := fresh("ten-single-sweeps", true)
		for i := 0; i < 10; i++ {
			source := fmt.Sprintf("sweep:%d:1/1", 200+i)
			out, err := apply(id, source, false)
			if err != nil || len(out) != 1 || out[0].GetGuid() != pet.GetGuid() || out[0].GetAddExp() != 2 || out[0].GetCurExp() != int32((i+1)*2) {
				t.Fatalf("single completion %d: %+v %v", i, out, err)
			}
			f := facts(id, source)
			if len(f) != 1 || f[0].Earned != 2 || f[0].Applied != 2 {
				t.Fatal("single source provenance")
			}
		}
		if owned(id).GetEat().GetExp() != 20 {
			t.Fatal("ten successful original requests must total twenty")
		}
		for i := 0; i < 10; i++ {
			out, err := apply(id, fmt.Sprintf("sweep:%d:1/1", 200+i), false)
			if err != nil || len(out) != 1 || out[0].GetCurExp() != int32((i+1)*2) {
				t.Fatal("historical single receipt changed", err)
			}
		}
		if owned(id).GetEat().GetExp() != 20 {
			t.Fatal("ten replay requests duplicated growth")
		}
		var count int64
		db.GetIns().Model(&db.P9PetCopyGrant{}).Where("player_id=? AND completed_count=1", id).Count(&count)
		if count != 10 {
			t.Fatal("single operation counts were not frozen")
		}
	})
	t.Run("unverified-random-unlock-earned-growth-retained-without-fight-block", func(t *testing.T) {
		id, pet := fresh("boundary", true)
		stamp++
		if _, _, err := s.DispatchP9PetIntimacy(ctx, id, &pb.CSPetEat{Guid: pet.Guid, Cost: []*pb.BaseResource{{Id: proto.String("item_129"), Num: proto.Int32(31)}}}, stamp); err != nil {
			t.Fatal(err)
		}
		for i, source := range []string{"copy:boundary1", "copy:boundary2", "copy:boundary3"} {
			out, err := apply(id, source, false)
			if err != nil || len(out) != 1 {
				t.Fatal("completed fight blocked", err)
			}
			want := int32(622 + i*2)
			gain := int32(2)
			if i == 2 {
				want = 625
				gain = 1
			}
			if out[0].GetCurExp() != want || out[0].GetAddExp() != gain {
				t.Fatalf("applied actual points %+v", out)
			}
		}
		f := facts(id, "copy:boundary3")
		if f[0].Earned != 2 || f[0].Applied != 1 || f[0].Deferred != 1 {
			t.Fatal("boundary point lost")
		}
		out, err := apply(id, "copy:boundary4", false)
		if err != nil || len(out) != 0 || owned(id).GetEat().GetExp() != 625 {
			t.Fatal("unknown allocation changed pet or blocked fight", err)
		}
		f = facts(id, "copy:boundary4")
		if f[0].Earned != 2 || f[0].Applied != 0 || f[0].Deferred != 2 {
			t.Fatal("unallocated earning lost")
		}
		out, err = apply(id, "copy:boundary4", false)
		if err != nil || len(out) != 0 {
			t.Fatal("zero-applied replay", err)
		}
		if owned(id).GetEat().GetLevel() != 0 || len(owned(id).GetRandPropList()) != 0 {
			t.Fatal("unverified random attributes invented")
		}
	})
	t.Run("settlement-failure-rolls-back-instance-profile-and-earning-row", func(t *testing.T) {
		id, _ := fresh("rollback", true)
		if _, err := apply(id, "copy:rollback", true); err == nil {
			t.Fatal("injected source failure did not rollback")
		}
		if owned(id).GetEat().GetExp() != 0 {
			t.Fatal("pet escaped transaction")
		}
		var count int64
		db.GetIns().Model(&db.P9PetCopyGrant{}).Where("player_id=?", id).Count(&count)
		if count != 0 {
			t.Fatal("earning receipt escaped rollback")
		}
		out, err := apply(id, "copy:rollback", false)
		if err != nil || out[0].GetAddExp() != 2 {
			t.Fatal("retry completed source", err)
		}
	})
	t.Run("empty-formation-source-freezes-no-reward-before-later-guard", func(t *testing.T) {
		id, pet := fresh("empty", false)
		out, err := apply(id, "copy:no-pet", false)
		if err != nil || len(out) != 0 {
			t.Fatal(err)
		}
		stamp++
		if _, _, err = s.DispatchP9PetNext(ctx, id, &pb.CSFormationOn{Id: pet.Id, Guid: pet.Guid, FormatPos: proto.Int32(1)}, stamp); err != nil {
			t.Fatal(err)
		}
		out, err = apply(id, "copy:no-pet", false)
		if err != nil || len(out) != 0 || owned(id).GetEat().GetExp() != 0 {
			t.Fatal("late guard rewrote original zero-source receipt", err)
		}
	})
}
