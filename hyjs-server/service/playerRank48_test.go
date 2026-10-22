package service

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	"gorm.io/gorm/logger"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

func TestPlayerRank48SameZoneReadOnly(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "rank48.db")
	fixture := &p1Fixture{ctx: ctx, path: path}
	if name := os.Getenv("HYJS_P1_MYSQL_DATABASE"); name != "" {
		if !strings.HasPrefix(name, "hyjs_test_p1_") {
			t.Fatal("isolated P1 database required")
		}
		fixture.mysql = db.MySQLConfig{Host: os.Getenv("HYJS_P1_MYSQL_HOST"), User: os.Getenv("HYJS_P1_MYSQL_USER"), Password: os.Getenv("HYJS_P1_MYSQL_PASSWORD"), Database: name}
	}
	fixture.open(t)
	defer func() { d, _ := db.GetIns().DB(); d.Close() }()
	s := &GameService{}
	if err := db.GetIns().AutoMigrate(&db.P13Guild{}, &db.P13GuildMember{}, &db.P7RushState{}, &db.P12WantedFight{}, &db.P12WantedInstance{}); err != nil {
		t.Fatal(err)
	}
	for id := int64(1); id <= 5; id++ {
		guid := fmt.Sprintf("%d-hero", id)
		p := &pb.Player{PlayerId: proto.Int64(id), Name: proto.String(guid), HeroWarId: proto.String(guid), Level: proto.Int32(int32(id + 10)), FightPower: proto.Int32(int32(id * 1000)), WarriorBagSize: proto.Int32(100), EquipBagSize: proto.Int32(100), PetBagSize: proto.Int32(100)}
		w, err := model.GameMessage("Warrior", map[string]any{"id": "hero_043", "guid": guid, "level": id + 1, "breached": map[string]any{"level": 1}, "training": map[string]any{}, "tempTraining": map[string]any{}, "awake": map[string]any{}})
		if err != nil {
			t.Fatal(err)
		}
		f, err := model.GameMessage("FormationItem", map[string]any{"formatPos": 1, "battlePos": 1, "things": []any{map[string]any{"pos": 0, "guid": guid, "id": "hero_043"}}})
		if err != nil {
			t.Fatal(err)
		}
		star := int32(id)
		game := &model.GameState{Warriors: []*pb.Warrior{w.(*pb.Warrior)}, Formation: []*pb.FormationItem{f.(*pb.FormationItem)}, Copy: &pb.SCCopyPassedInfo{PassedCopyInfo: []*pb.CopySimpleInfo{{ChapterId: proto.String("1"), MissionId: proto.String("1"), Star: proto.Int32(star)}, {ChapterId: proto.String("1"), MissionId: proto.String("1"), Star: proto.Int32(star)}}}}
		pr, _ := json.Marshal(p)
		gr, _ := json.Marshal(game)
		server := int64(1)
		if id == 5 {
			server = 2
		}
		row := &db.Profile{Id: id, ServerID: server, Uid: guid, Session: []byte{}, Player: proto.String(string(pr)), Game: proto.String(string(gr))}
		if err := db.GetIns().Create(row).Error; err != nil {
			t.Fatal(err)
		}
		if err := db.GetIns().Create(&db.P7RushState{PlayerID: id, RecordStars: int32(10 - id), TotalStars: 0}).Error; err != nil {
			t.Fatal(err)
		}
	}
	if err := db.GetIns().Create(&db.P13Guild{ID: "guild", Name: "真实公会"}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.GetIns().Create(&db.P13GuildMember{PlayerID: 1, GuildID: "guild"}).Error; err != nil {
		t.Fatal(err)
	}
	var before []db.Profile
	db.GetIns().Order("id").Find(&before)
	now := time.Now()
	read := func(kind int32) *pb.RankInfo {
		t.Helper()
		out, handled, err := s.DispatchP10Social(ctx, 1, &pb.CSRank{Type: proto.Int32(kind)}, 1)
		if err != nil || !handled {
			t.Fatalf("type%d: %v handled=%v", kind, err, handled)
		}
		return out[0].(*pb.SCRank).GetRankIno()
	}
	for _, kind := range []int32{1, 2, 3, 6} {
		rank := read(kind)
		if len(rank.List) != 4 {
			t.Fatalf("type%d list=%v", kind, rank)
		}
		found := false
		for i, row := range rank.List {
			wantLen := 1
			if kind == 1 || kind == 6 {
				wantLen = 2
			}
			if len(row.Values) != wantLen || row.GetRank() != int32(i+1) || row.GetPid() == 5 {
				t.Fatalf("bad row: %v", row)
			}
			if row.GetPid() == 1 {
				found = true
				if row.GetUnionName() != "真实公会" {
					t.Fatal(row)
				}
			}
			if i > 0 && rank.List[i-1].Values[0] < row.Values[0] {
				t.Fatalf("unsorted: %v", rank)
			}
		}
		if !found {
			t.Fatal("self missing")
		}
		if kind == 1 && rank.List[0].Values[0] != 4 {
			t.Fatal("duplicate stars counted")
		}
		if kind == 3 && rank.List[0].GetPid() != 4 {
			t.Fatal("level order")
		}
		if kind == 6 && (rank.List[0].GetPid() != 1 || rank.List[0].Values[0] != 9) {
			t.Fatal("historical stars")
		}
	}
	for _, kind := range []int32{4, 5} {
		if len(read(kind).List) != 0 {
			t.Fatal("invented wanted standings")
		}
	}

	// Raw committed fixtures isolate the existing P12 daily projection; no combat rewards run.
	for _, instance := range []db.P12WantedInstance{{ID: "same", ServerID: 1}, {ID: "other", ServerID: 2}} {
		if err := db.GetIns().Create(&instance).Error; err != nil {
			t.Fatal(err)
		}
	}
	for _, fight := range []db.P12WantedFight{
		{ID: "a", PlayerID: 1, InstanceID: "same", Committed: true, CommittedAt: now.UnixMilli(), Performance: 10, AppliedDamage: 20.5},
		{ID: "b", PlayerID: 1, InstanceID: "same", Committed: true, CommittedAt: now.UnixMilli(), Performance: 5, AppliedDamage: 30.5},
		{ID: "c", PlayerID: 2, InstanceID: "same", Committed: true, CommittedAt: now.UnixMilli(), Performance: 15, AppliedDamage: 30.9},
		{ID: "old", PlayerID: 3, InstanceID: "same", Committed: true, CommittedAt: now.AddDate(0, 0, -1).UnixMilli(), Performance: 999, AppliedDamage: 999},
		{ID: "pending", PlayerID: 3, InstanceID: "same", Committed: false, CommittedAt: now.UnixMilli(), Performance: 999, AppliedDamage: 999},
		{ID: "foreign", PlayerID: 4, InstanceID: "other", Committed: true, CommittedAt: now.UnixMilli(), Performance: 999, AppliedDamage: 999},
	} {
		fight.StartProto = []byte{}
		fight.EndProto = []byte{}
		if err := db.GetIns().Create(&fight).Error; err != nil {
			t.Fatal(err)
		}
	}
	damage := read(4)
	if len(damage.List) != 2 || damage.List[0].GetPid() != 2 || damage.List[0].Values[0] != 30 || damage.List[1].Values[0] != 30 {
		t.Fatalf("daily damage precision/order: %v", damage)
	}
	performance := read(5)
	if len(performance.List) != 2 || performance.List[0].GetPid() != 1 || performance.List[0].Values[0] != 15 || performance.List[1].Values[0] != 15 {
		t.Fatalf("daily sum/tie: %v", performance)
	}
	if _, err := s.playerRank48(ctx, 1, 0, now); err == nil {
		t.Fatal("accepted invalid type")
	}
	d, _ := db.GetIns().DB()
	d.Close()
	fixture.open(t)
	if len(read(3).List) != 4 {
		t.Fatal("cold read")
	}
	var after []db.Profile
	db.GetIns().Order("id").Find(&after)
	if !reflect.DeepEqual(before, after) {
		t.Fatal("rank query mutated profiles")
	}
	var count int64
	db.GetIns().Model(&db.P7RushState{}).Count(&count)
	if count != 5 {
		t.Fatal("rank query created rush state")
	}
	db.GetIns().Model(&db.P13GuildMember{}).Count(&count)
	if count != 1 {
		t.Fatal("rank query created member")
	}
	if err := db.GetIns().Model(&db.Profile{}).Where("id=1").Update("server_id", 3).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.GetIns().Model(&db.Profile{}).Where("id=1").Updates(map[string]any{"player": nil, "game": nil}).Error; err != nil {
		t.Fatal(err)
	}
	if len(read(3).List) != 0 {
		t.Fatal("empty zone invented role")
	}
}

type playerRank48Trace struct {
	logger.Interface
	sql  []string
	rows []int64
}

func (l *playerRank48Trace) Trace(ctx context.Context, begin time.Time, fc func() (string, int64), err error) {
	q, n := fc()
	l.sql = append(l.sql, q)
	l.rows = append(l.rows, n)
}

func TestPlayerRank48ReadBoundaries(t *testing.T) {
	ctx := context.Background()
	fixture := &p1Fixture{ctx: ctx, path: filepath.Join(t.TempDir(), "rank-pages.db")}
	if name := os.Getenv("HYJS_P1_MYSQL_DATABASE"); name != "" {
		if !strings.HasPrefix(name, "hyjs_test_p1_") {
			t.Fatal("isolated P1 database required")
		}
		fixture.mysql = db.MySQLConfig{Host: os.Getenv("HYJS_P1_MYSQL_HOST"), User: os.Getenv("HYJS_P1_MYSQL_USER"), Password: os.Getenv("HYJS_P1_MYSQL_PASSWORD"), Database: name}
	}
	fixture.open(t)
	defer func() { d, _ := db.GetIns().DB(); d.Close() }()
	tx := db.GetIns()
	if err := tx.AutoMigrate(&db.P13Guild{}, &db.P13GuildMember{}, &db.P7RushState{}, &db.P12WantedFight{}, &db.P12WantedInstance{}); err != nil {
		t.Fatal(err)
	}
	for ordinal := int64(1); ordinal <= 260; ordinal++ {
		id := 10000 + ordinal
		p := &pb.Player{PlayerId: proto.Int64(id), Name: proto.String(fmt.Sprintf("角色%d", id)), HeroWarId: proto.String(fmt.Sprintf("hero-%d", id)), Level: proto.Int32(10), FightPower: proto.Int32(int32(ordinal * 10))}
		game := &model.GameState{Warriors: []*pb.Warrior{{Id: proto.String("hero_043"), Guid: p.HeroWarId}}, Copy: &pb.SCCopyPassedInfo{PassedCopyInfo: []*pb.CopySimpleInfo{{ChapterId: proto.String("1"), MissionId: proto.String("1"), Star: proto.Int32(2)}}}}
		a, _ := json.Marshal(p)
		b, _ := json.Marshal(game)
		if err := tx.Create(&db.Profile{Id: id, Uid: fmt.Sprint(id), ServerID: 48, Session: []byte{}, Player: proto.String(string(a)), Game: proto.String(string(b))}).Error; err != nil {
			t.Fatal(err)
		}
	}
	if err := tx.Create(&db.P13Guild{ID: "rank-guild", Name: "实时公会"}).Error; err != nil {
		t.Fatal(err)
	}
	if err := tx.Create(&db.P13GuildMember{PlayerID: 10001, GuildID: "rank-guild"}).Error; err != nil {
		t.Fatal(err)
	}
	s := &GameService{}
	trace := &playerRank48Trace{Interface: tx.Config.Logger}
	oldLogger := tx.Config.Logger
	tx.Config.Logger = trace
	defer func() { tx.Config.Logger = oldLogger }()
	read := func(kind int32) *pb.RankInfo {
		t.Helper()
		trace.sql = nil
		trace.rows = nil
		out, err := s.playerRank48(ctx, 10001, kind, time.Now())
		if err != nil {
			t.Fatal(err)
		}
		expectedQueries := 7
		if kind == 6 {
			expectedQueries = 10
		}
		if kind == 4 || kind == 5 {
			expectedQueries = 4
		}
		if len(trace.sql) != expectedQueries {
			t.Fatalf("kind%d query bound: got %d want %d", kind, len(trace.sql), expectedQueries)
		}
		t.Logf("kind=%d queries=%d", kind, len(trace.sql))
		var pageRows []int64
		for index, q := range trace.sql {
			lower := strings.ToLower(q)
			if strings.Contains(lower, "sqlite_master") || strings.Contains(lower, "information_schema") || strings.Contains(lower, "prayer") || strings.Contains(lower, "tech_") {
				t.Fatalf("unexpected per-role lookup: %s", q)
			}
			if strings.Contains(lower, "from `profiles`") && strings.Contains(lower, "json_extract") {
				if !strings.Contains(lower, "limit 128") || !strings.Contains(lower, "id>") && !strings.Contains(lower, "id >") {
					t.Fatalf("unbounded page: %s", q)
				}
				if strings.Contains(lower, "select *") || strings.Contains(lower, "`player`,") || strings.Contains(lower, "`game`") {
					t.Fatalf("full profile blob: %s", q)
				}
				if (kind == 4 || kind == 5) && (strings.Contains(lower, "$.level") || strings.Contains(lower, "$.fightpower") || !strings.Contains(lower, "id in (10260)")) {
					t.Fatalf("wanted scan did not stay within participant identity: %s", q)
				}
				if kind != 1 && strings.Contains(lower, "json_extract(game") {
					t.Fatalf("unneeded game fragment: %s", q)
				}
				if trace.rows[index] > constUtil.PlayerRank48ReadPageSize {
					t.Fatal("oversized page")
				}
				pageRows = append(pageRows, trace.rows[index])
			}
		}
		expectedPages := []int64{128, 128, 4}
		if kind == 4 || kind == 5 {
			expectedPages = []int64{1}
		}
		if !reflect.DeepEqual(pageRows, expectedPages) {
			t.Fatalf("page rows=%v", pageRows)
		}
		t.Logf("kind=%d profile page rows=%v", kind, pageRows)
		return out[0].(*pb.SCRank).GetRankIno()
	}
	for _, kind := range []int32{1, 2, 3, 6} {
		result := read(kind)
		if len(result.List) != 260 {
			t.Fatalf("truncated kind%d: %d", kind, len(result.List))
		}
		if kind == 2 && (result.List[0].GetPid() != 10260 || result.List[0].Values[0] != 2600) {
			t.Fatalf("saved power ignored: %v", result.List[0])
		}
		if kind == 1 && result.List[0].Values[1] != 10 {
			t.Fatal("copy power is not saved snapshot")
		}
		if kind == 3 && result.List[0].GetPid() != 10001 {
			t.Fatal("stable tie order")
		}
	}

	if err := tx.Create(&db.P12WantedInstance{ID: "rank-wanted", ServerID: 48}).Error; err != nil {
		t.Fatal(err)
	}
	if err := tx.Create(&db.P12WantedFight{ID: "rank-fight", PlayerID: 10260, InstanceID: "rank-wanted", Committed: true, CommittedAt: time.Now().UnixMilli(), Performance: 17, AppliedDamage: 33.3, StartProto: []byte{}, EndProto: []byte{}}).Error; err != nil {
		t.Fatal(err)
	}
	for _, kind := range []int32{4, 5} {
		result := read(kind)
		if len(result.List) != 1 || result.List[0].GetPid() != 10260 {
			t.Fatal("wanted identity scope")
		}
	}
	if err := tx.Model(&db.P13GuildMember{}).Where("player_id=10001").Update("guild_id", "").Error; err != nil {
		t.Fatal(err)
	}
	for _, row := range read(3).List {
		if row.GetPid() == 10001 && row.GetUnionName() != "" {
			t.Fatal("revoked membership cached")
		}
	}
}
