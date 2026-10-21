package service

import (
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

// TestOpenServerPurchase40NativeRemaining 对照原 HalfPriceBg 的总量减已购算法，验证查询、交易及冷恢复。
func TestOpenServerPurchase40NativeRemaining(t *testing.T) {
	f := feedback40Database(t, "purchase40.db")
	ctx := f.ctx
	sqlDB, _ := db.GetIns().DB()
	t.Cleanup(func() { sqlDB.Close() })
	s := &GameService{}
	if err := s.LoadTables("../../analysis/tables"); err != nil {
		t.Fatal(err)
	}
	loadOpenServerTaskFixture(t, s)
	if err := s.InitializeOpenServerActivity(ctx); err != nil {
		t.Fatal(err)
	}
	row, err := LoginAccount(ctx, "purchase40-isolated", "test-password")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = CreatePlayer(ctx, row.Id, "购买次数回归", "hero_043"); err != nil {
		t.Fatal(err)
	}
	if err = updateProfile(ctx, row.Id, func(p *pb.Player, g *model.GameState) error {
		p.Diamond = proto.Int64(1000000)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 10, 24, 12, 0, 0, 0, time.FixedZone("UTC+8", 8*3600))
	stamp := int64(40000)
	for _, test := range []struct {
		name, selection, resource string
		id, day, total, quantity  int32
		price                     int64
	}{
		{"orange-choice", "frag_hero_019", "frag_hero_019", 183, 10, 40, 2, 60},
		{"red-choice", "frag_hero_001", "frag_hero_001", 179, 10, 20, 10, 600},
		{"non-choice", "", "MineItem_051", 249, 14, 5, 1, 500},
	} {
		t.Run(test.name, func(t *testing.T) {
			query := func(expected int32) {
				t.Helper()
				messages, err := s.openServerActivityAt(ctx, row.Id, &pb.CSNewOpenSevenDayByTitle{Day: proto.Int32(test.day), Tap: proto.Int32(3)}, 0, now)
				if err != nil {
					t.Fatal(err)
				}
				for _, item := range messages[0].(*pb.SCNewOpenSevenDayByTitle).GetItemList() {
					if item.GetItemId() == test.id {
						// 当前 APK InitHalfPriceBg 取 allNumber-currentNum；成功回调再减少一次。
						if item.GetCurrentNum() != expected || test.total-item.GetCurrentNum() != test.total-expected {
							t.Fatalf("native remaining: claims=%d item=%v", expected, item)
						}
						wantState := int32(1)
						if expected == test.total {
							wantState = 2
						}
						if item.GetState() != wantState {
							t.Fatalf("purchase state=%d want=%d", item.GetState(), wantState)
						}
						return
					}
				}
				t.Fatal("native query omitted product")
			}
			balances := func() (int64, int64, int32) {
				t.Helper()
				var saved db.Profile
				if err := db.GetIns().First(&saved, row.Id).Error; err != nil {
					t.Fatal(err)
				}
				p, g, err := decodeProfile(&saved)
				if err != nil {
					t.Fatal(err)
				}
				state, err := openServerReadStates(db.GetIns(), row.Id)
				if err != nil {
					t.Fatal(err)
				}
				claims := int32(0)
				for _, st := range state {
					if st.TaskID == test.id {
						claims = st.Claims
					}
				}
				owned := simpleOwned(p, g, test.resource)
				for _, gem := range g.Gems {
					if gem.GetId() == test.resource {
						owned += int64(gem.GetNum())
					}
				}
				return p.GetDiamond(), owned, claims
			}
			query(0)
			initialDiamond, initialResource, _ := balances()
			var first proto.Message
			request := &pb.CSGetOpenServerReward{ItemId: proto.Int32(test.id)}
			if test.selection != "" {
				request.HeroId = proto.String(test.selection)
			}
			firstStamp := stamp + 1
			for n := int32(1); n <= test.total; n++ {
				stamp++
				messages, err := s.openServerActivityAt(ctx, row.Id, request, stamp, now)
				if err != nil {
					t.Fatalf("purchase %d: %v", n, err)
				}
				if n == 1 {
					first = proto.Clone(messages[0])
				}
				query(n)
				diamond, resource, claims := balances()
				if diamond != initialDiamond-int64(n)*test.price || resource != initialResource+int64(n*test.quantity) || claims != n {
					t.Fatalf("purchase %d balances diamond=%d resource=%d claims=%d", n, diamond, resource, claims)
				}
				// 同 stamp 回执返回相同增量，服务端不重复扣费、发片或计数。
				replay, err := s.openServerActivityAt(ctx, row.Id, request, stamp, now)
				if err != nil || !proto.Equal(replay[0], messages[0]) {
					t.Fatal("receipt replay", err)
				}
				gotDiamond, gotResource, gotClaims := balances()
				if gotDiamond != diamond || gotResource != resource || gotClaims != claims {
					t.Fatal("receipt changed persisted balances")
				}
			}
			stamp++
			if _, err := s.openServerActivityAt(ctx, row.Id, request, stamp, now); err == nil {
				t.Fatal("exhausted product accepted purchase")
			}
			query(test.total)
			if err := sqlDB.Close(); err != nil {
				t.Fatal(err)
			}
			f.open(t)
			sqlDB, _ = db.GetIns().DB()
			s = &GameService{Tables: s.Tables}
			query(test.total)
			replay, err := s.openServerActivityAt(ctx, row.Id, request, firstStamp, now)
			if err != nil || !proto.Equal(replay[0], first) {
				t.Fatal("cold receipt replay", err)
			}
			diamond, resource, claims := balances()
			if diamond != initialDiamond-int64(test.total)*test.price || resource != initialResource+int64(test.total*test.quantity) || claims != test.total {
				t.Fatal("cold restore changed purchase state")
			}
		})
	}
}
