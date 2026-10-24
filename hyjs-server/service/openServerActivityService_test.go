package service

import (
	"context"
	"encoding/csv"
	"fmt"
	"google.golang.org/protobuf/proto"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

// TestOpenServerActivityWindow 验证固定整服日历、成功登录事件、宽限冻结与回执重启。
func TestOpenServerActivityWindow(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "open.db")
	if e := db.InitGame(ctx, path); e != nil {
		t.Fatal(e)
	}
	sqlDB, _ := db.GetIns().DB()
	defer func() { sqlDB.Close() }()
	s := &GameService{}
	if e := s.LoadTables("../../analysis/tables"); e != nil {
		t.Fatal(e)
	}
	loadOpenServerTaskFixture(t, s)
	if e := s.InitializeOpenServerActivity(ctx); e != nil {
		t.Fatal(e)
	}
	row, e := LoginAccount(ctx, "isolated-open", "test-password")
	if e != nil {
		t.Fatal(e)
	}
	if _, e = CreatePlayer(ctx, row.Id, "开服测试", "hero_043"); e != nil {
		t.Fatal(e)
	}
	start := time.Date(2026, 10, 11, 0, 0, 0, 0, time.FixedZone("UTC+8", 8*3600))
	now := start.Add(time.Hour)
	for _, test := range []struct {
		at              time.Time
		seven, fourteen int32
	}{{start.Add(-time.Millisecond), 0, 0}, {start, 1, 0}, {start.AddDate(0, 0, 7).Add(-time.Millisecond), 1, 0}, {start.AddDate(0, 0, 7), 1, 1}, {start.AddDate(0, 0, 14), 1, 1}, {start.AddDate(0, 0, 15), 0, 0}} {
		states, e := s.openServerOpenStatesAt(ctx, row.Id, test.at)
		if e != nil {
			t.Fatal(e)
		}
		if len(states) < 2 || states[0].GetType() != constUtil.OpenServerActivitySevenDayType || states[1].GetType() != constUtil.OpenServerActivityFourteenDayType || states[0].GetIsOpen() != test.seven || states[1].GetIsOpen() != test.fourteen {
			t.Fatal("open states calendar", test, states)
		}
	}
	call := func(req proto.Message, stamp int64, at time.Time) ([]proto.Message, error) {
		return s.openServerActivityAt(ctx, row.Id, req, stamp, at)
	}
	tab := func(day, tap int32, at time.Time) *pb.SCNewOpenSevenDayByTitle {
		t.Helper()
		messages, e := call(&pb.CSNewOpenSevenDayByTitle{Day: proto.Int32(day), Tap: proto.Int32(tap)}, 0, at)
		if e != nil {
			t.Fatal(e)
		}
		return messages[0].(*pb.SCNewOpenSevenDayByTitle)
	}
	if got := tab(1, 1, now); got.ItemList[0].GetState() != 0 {
		t.Fatal("query fabricated login", got)
	}
	if e := recordOpenServerCounterAt(db.GetIns(), row.Id, constUtil.OpenServerActivityCounterLogin+"2026-10-11", 1, now); e != nil {
		t.Fatal(e)
	}
	if e := recordOpenServerCounterAt(db.GetIns(), row.Id, constUtil.OpenServerActivityCounterLogin+"2026-10-11", 1, now); e != nil {
		t.Fatal(e)
	}
	if got := tab(1, 1, now); got.ItemList[0].GetState() != 1 || got.ItemList[0].GetCurrentNum() != 1 {
		t.Fatal("login event", got)
	}
	req := &pb.CSGetOpenServerReward{ItemId: proto.Int32(1)}
	first, e := call(req, 1, now)
	if e != nil {
		t.Fatal(e)
	}
	replay, e := call(req, 1, now)
	if e != nil || !proto.Equal(first[0], replay[0]) {
		t.Fatal("receipt replay", e)
	}
	if _, e = call(req, 2, now); e == nil {
		t.Fatal("new timestamp duplicate claim")
	}
	if _, e = call(&pb.CSGetOpenServerReward{ItemId: proto.Int32(2)}, 1, now); e == nil {
		t.Fatal("stamp changed request")
	}
	if got := tab(1, 1, now); got.ItemList[0].GetState() != 2 || got.ItemList[1].GetState() != 0 {
		t.Fatal("claim/payment state", got)
	}
	if _, e = call(&pb.CSNewOpenSevenDayByTitle{Day: proto.Int32(2), Tap: proto.Int32(1)}, 0, now); e == nil {
		t.Fatal("future day query")
	}
	if e := db.GetIns().First(row, row.Id).Error; e != nil {
		t.Fatal(e)
	}
	p, g, e := decodeProfile(row)
	if e != nil {
		t.Fatal(e)
	}
	p.Level = proto.Int32(20)
	if e := s.refreshOpenServerProgressAt(db.GetIns(), p, g, now); e != nil {
		t.Fatal(e)
	}
	p.Level = proto.Int32(25)
	if e := saveProfile(db.GetIns(), row, p, g); e != nil {
		t.Fatal(e)
	}
	grace := start.AddDate(0, 0, 14).Add(time.Hour)
	messages, e := call(&pb.CSNewOpenSevenDayByTitle{Day: proto.Int32(1), Tap: proto.Int32(2)}, 0, grace)
	if e != nil {
		t.Fatal(e)
	}
	states := map[int32]int32{}
	for _, item := range messages[0].(*pb.SCNewOpenSevenDayByTitle).ItemList {
		states[item.GetItemId()] = item.GetState()
	}
	if states[7] != 1 || states[8] != 0 {
		t.Fatal("grace unlocked fresh level", states)
	}
	if _, e = call(&pb.CSGetOpenServerReward{ItemId: proto.Int32(7)}, 3, grace); e != nil {
		t.Fatal(e)
	}
	if _, e = call(&pb.CSGetOpenServerReward{ItemId: proto.Int32(20)}, 4, grace); e == nil {
		t.Fatal("purchase during grace")
	}
	if _, e = call(&pb.CSOpenServerActivity{DayType: proto.Int32(14)}, 0, start.AddDate(0, 0, 15)); e == nil {
		t.Fatal("cutoff accepted")
	}
	sqlDB.Close()
	if e := db.InitGame(ctx, path); e != nil {
		t.Fatal(e)
	}
	sqlDB, _ = db.GetIns().DB()
	loadOpenServerTaskFixture(t, s)
	if e := s.InitializeOpenServerActivity(ctx); e != nil {
		t.Fatal(e)
	}
	replay, e = call(req, 1, grace)
	if e != nil || !proto.Equal(first[0], replay[0]) {
		t.Fatal("restart receipt", e)
	}
}

// TestOpenServerActivityStock 验证跨角色有限库存及购买失败的资源、状态、回执整笔回滚。
func TestOpenServerActivityStock(t *testing.T) {
	ctx := context.Background()
	if e := db.InitGame(ctx, filepath.Join(t.TempDir(), "stock.db")); e != nil {
		t.Fatal(e)
	}
	sqlDB, _ := db.GetIns().DB()
	defer sqlDB.Close()
	s := &GameService{}
	if e := s.LoadTables("../../analysis/tables"); e != nil {
		t.Fatal(e)
	}
	loadOpenServerTaskFixture(t, s)
	if e := s.InitializeOpenServerActivity(ctx); e != nil {
		t.Fatal(e)
	}
	now := time.Date(2026, 10, 12, 12, 0, 0, 0, time.FixedZone("UTC+8", 8*3600))
	var ids []int64
	for _, uid := range []string{"stock-a", "stock-b"} {
		row, e := LoginAccount(ctx, uid, "test-password")
		if e != nil {
			t.Fatal(e)
		}
		if _, e = CreatePlayer(ctx, row.Id, uid, "hero_043"); e != nil {
			t.Fatal(e)
		}
		if e := updateProfile(ctx, row.Id, func(p *pb.Player, g *model.GameState) error { p.Diamond = proto.Int64(1000); return nil }); e != nil {
			t.Fatal(e)
		}
		ids = append(ids, row.Id)
	}
	if e := db.GetIns().Model(&db.OpenServerActivityTask{}).Where("id = ?", 20).Updates(map[string]any{"target": 1, "new_price": 400, "reward": "diamond,1"}).Error; e != nil {
		t.Fatal(e)
	}
	s.table("ActivityNewOpen7Day", "id", "20")["reward"] = "diamond,1"
	var wg sync.WaitGroup
	errs := make(chan error, 2)
	for _, id := range ids {
		wg.Add(1)
		go func(id int64) {
			defer wg.Done()
			_, e := s.openServerActivityAt(ctx, id, &pb.CSGetOpenServerReward{ItemId: proto.Int32(20)}, 11, now)
			errs <- e
		}(id)
	}
	wg.Wait()
	close(errs)
	wins := 0
	for e := range errs {
		if e == nil {
			wins++
		}
	}
	if wins != 1 {
		t.Fatal("stock oversold", wins)
	}
	var task db.OpenServerActivityTask
	if e := db.GetIns().First(&task, 20).Error; e != nil || task.UsedStock != 1 {
		t.Fatal("stock mutation", task, e)
	}
	// 奖励第二段金币溢出时，先扣钻和先增的道具均不能落库。
	if e := db.GetIns().Model(&db.OpenServerActivityTask{}).Where("id = ?", 37).Updates(map[string]any{"reward": "item_001,1;coin,1"}).Error; e != nil {
		t.Fatal(e)
	}
	s.table("ActivityNewOpen7Day", "id", "37")["reward"] = "item_001,1;coin,1"
	if e := updateProfile(ctx, ids[0], func(p *pb.Player, g *model.GameState) error { p.Coin = proto.Int64(9223372036854775807); return nil }); e != nil {
		t.Fatal(e)
	}
	var before, after db.Profile
	db.GetIns().First(&before, ids[0])
	if _, e := s.openServerActivityAt(ctx, ids[0], &pb.CSGetOpenServerReward{ItemId: proto.Int32(37)}, 12, now); e == nil {
		t.Fatal("overflow accepted")
	}
	db.GetIns().First(&after, ids[0])
	if *before.Player != *after.Player || *before.Game != *after.Game {
		t.Fatal("partial resource mutation")
	}
	var count int64
	db.GetIns().Model(&db.OpenServerActivityReceipt{}).Where("player_id = ? AND timestamp = ?", ids[0], 12).Count(&count)
	if count != 0 {
		t.Fatal("rollback receipt exists")
	}
}

// TestOpenServerFormationConditions 所有装备条件需要完整槽位，最高等级条件读取实际穿戴实例。
func TestOpenServerFormationConditions(t *testing.T) {
	s := &GameService{}
	if e := s.LoadTables("../../analysis/tables"); e != nil {
		t.Fatal(e)
	}
	g := &model.GameState{}
	for i := int32(1); i <= 5; i++ {
		guid := "hero-" + strconv.Itoa(int(i))
		g.Warriors = append(g.Warriors, &pb.Warrior{Id: proto.String("hero_043"), Guid: proto.String(guid), Breached: &pb.Breached{Level: proto.Int32(4)}})
		slot := &pb.FormationItem{FormatPos: proto.Int32(i), Things: []*pb.FormationThing{{Pos: proto.Int32(0), Id: proto.String("hero_043"), Guid: proto.String(guid)}}}
		for pos := int32(1); pos <= 4; pos++ {
			equipID := []string{"equip_015", "equip_016", "equip_017", "equip_018"}[pos-1]
			eqGuid := guid + "-equip-" + strconv.Itoa(int(pos))
			g.Equipment = append(g.Equipment, &pb.Equipment{EquipId: proto.String(equipID), Guid: proto.String(eqGuid), Level: proto.Int32(30), RefineLv: proto.Int32(2)})
			slot.Things = append(slot.Things, &pb.FormationThing{Pos: proto.Int32(pos), Id: proto.String(equipID), Guid: proto.String(eqGuid)})
		}
		for pos := int32(5); pos <= 6; pos++ {
			trGuid := guid + "-trea-" + strconv.Itoa(int(pos))
			g.Treasures = append(g.Treasures, &pb.Treasure{TreasureId: proto.String("trea_011"), Guid: proto.String(trGuid), RefineLevel: proto.Int32(3)})
			slot.Things = append(slot.Things, &pb.FormationThing{Pos: proto.Int32(pos), Id: proto.String("trea_011"), Guid: proto.String(trGuid)})
		}
		g.Formation = append(g.Formation, slot)
	}
	for kind, want := range map[int32]int32{6: 30, 14: 2, 15: 2, 16: 4, 17: 4, 25: 3, 26: 3} {
		got, e := s.openServerFormationCurrent(g, kind)
		if e != nil || got != want {
			t.Fatalf("kind%d=%d want%d err%v", kind, got, want, e)
		}
	}
	g.Formation[0].Things = g.Formation[0].Things[:4]
	if got, e := s.openServerFormationCurrent(g, 6); e != nil || got != 0 {
		t.Fatal("missing equipment met all-equipment goal", got, e)
	}
	if got, e := s.openServerFormationCurrent(g, 25); e != nil || got != 0 {
		t.Fatal("missing treasure met all-treasure goal", got, e)
	}
}

// loadOpenServerTaskFixture 为领域单元测试直接加载原任务表，进程黑盒另行检查启动白名单接线。
func loadOpenServerTaskFixture(t *testing.T, s *GameService) {
	t.Helper()
	if err := db.GetIns().AutoMigrate(&db.P7RushState{}); err != nil {
		t.Fatal(err)
	}
	if !s.P1Ready {
		if err := s.InitializeP1(context.Background()); err != nil {
			t.Fatal(err)
		}
	}
	file, e := os.Open("../../analysis/tables/TableActivityNewOpen7Day.tsv")
	if e != nil {
		t.Fatal(e)
	}
	defer file.Close()
	reader := csv.NewReader(file)
	reader.Comma = '\t'
	reader.FieldsPerRecord = -1
	rows, e := reader.ReadAll()
	if e != nil {
		t.Fatal(e)
	}
	s.Tables["ActivityNewOpen7Day"] = nil
	for _, row := range rows[5:] {
		item := map[string]string{}
		for i, value := range row {
			if i < len(rows[1]) {
				item[rows[1][i]] = value
			}
		}
		s.Tables["ActivityNewOpen7Day"] = append(s.Tables["ActivityNewOpen7Day"], item)
	}
}

// TestMySQLOpenServerActivityTransactions 仅在明确隔离库执行真实行锁竞争和二进制回执验证。
func TestMySQLOpenServerActivityTransactions(t *testing.T) {
	name := os.Getenv("HYJS_MYSQL_TEST_DATABASE")
	if !strings.HasPrefix(name, "hyjs_test_") {
		t.Skip("isolated MySQL test database required")
	}
	ctx := context.Background()
	if e := db.InitGameMySQL(ctx, db.MySQLConfig{Host: os.Getenv("HYJS_MYSQL_HOST"), User: os.Getenv("HYJS_MYSQL_USER"), Password: os.Getenv("HYJS_MYSQL_PASSWORD"), Database: name}); e != nil {
		t.Fatal(e)
	}
	sqlDB, _ := db.GetIns().DB()
	defer sqlDB.Close()
	s := &GameService{}
	if e := s.LoadTables("../../analysis/tables"); e != nil {
		t.Fatal(e)
	}
	loadOpenServerTaskFixture(t, s)
	if e := s.InitializeOpenServerActivity(ctx); e != nil {
		t.Fatal(e)
	}
	now := time.Date(2026, 10, 12, 12, 0, 0, 0, time.FixedZone("UTC+8", 8*3600))
	var ids []int64
	for i := 0; i < 4; i++ {
		uid := fmt.Sprintf("isolated-mysql-open-%d-%d", time.Now().UnixNano(), i)
		row, e := LoginAccount(ctx, uid, "test-password")
		if e != nil {
			t.Fatal(e)
		}
		if _, e := CreatePlayer(ctx, row.Id, "开服MySQL", "hero_043"); e != nil {
			t.Fatal(e)
		}
		if e := updateProfile(ctx, row.Id, func(p *pb.Player, g *model.GameState) error { p.Diamond = proto.Int64(1000); return nil }); e != nil {
			t.Fatal(e)
		}
		ids = append(ids, row.Id)
	}
	if e := db.GetIns().Model(&db.OpenServerActivityTask{}).Where("id = ?", 20).Updates(map[string]any{"target": 1, "used_stock": 0, "reward": "trea_005,1", "new_price": 400}).Error; e != nil {
		t.Fatal(e)
	}
	s.table("ActivityNewOpen7Day", "id", "20")["reward"] = "trea_005,1"
	var wg sync.WaitGroup
	wins := make(chan int64, 4)
	for _, id := range ids {
		wg.Add(1)
		go func(id int64) {
			defer wg.Done()
			if _, e := s.openServerActivityAt(ctx, id, &pb.CSGetOpenServerReward{ItemId: proto.Int32(20)}, 500, now); e == nil {
				wins <- id
			}
		}(id)
	}
	wg.Wait()
	close(wins)
	var winner int64
	count := 0
	for id := range wins {
		winner = id
		count++
	}
	if count != 1 {
		t.Fatal("MySQL shared stock oversold", count)
	}
	req := &pb.CSGetOpenServerReward{ItemId: proto.Int32(20)}
	for i := 0; i < 5; i++ {
		if _, e := s.openServerActivityAt(ctx, winner, req, 500, now); e != nil {
			t.Fatal(e)
		}
	}
	var row db.Profile
	if e := db.GetIns().First(&row, winner).Error; e != nil {
		t.Fatal(e)
	}
	p, g, e := decodeProfile(&row)
	if e != nil || p.GetDiamond() != 600 || len(g.Treasures) != 1 {
		t.Fatal("MySQL replay repeated awards", e)
	}
	var columns []struct {
		ColumnName string
		DataType   string
	}
	if e := db.GetIns().Raw("SELECT COLUMN_NAME AS column_name,DATA_TYPE AS data_type FROM information_schema.COLUMNS WHERE TABLE_SCHEMA = ? AND TABLE_NAME = ? AND COLUMN_NAME IN ('request','response')", name, "open_server_activity_receipts").Scan(&columns).Error; e != nil {
		t.Fatal(e)
	}
	if len(columns) != 2 {
		t.Fatal("missing receipt columns")
	}
	for _, column := range columns {
		if column.DataType != "longblob" {
			t.Fatal("binary receipt type", column)
		}
	}
}
