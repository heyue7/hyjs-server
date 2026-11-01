package service

import (
	"bytes"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

func p10FinalSnapshotFixture(t testing.TB, roster int) *db.Profile {
	t.Helper()
	player := &pb.Player{PlayerId: proto.Int64(1), Name: proto.String("snapshot"), Level: proto.Int32(2), WarriorBagSize: proto.Int32(10), EquipBagSize: proto.Int32(10), PetBagSize: proto.Int32(10)}
	playerRaw, err := json.Marshal(player)
	if err != nil {
		t.Fatal(err)
	}
	game := &model.GameState{Position: &pb.Position{PlayerId: proto.Int64(1), Scene: proto.String("town_main")}}
	for i := 0; i < roster; i++ {
		game.Warriors = append(game.Warriors, &pb.Warrior{Id: proto.String("hero_001"), Guid: proto.String(fmt.Sprintf("snapshot-%04d", i)), Level: proto.Int32(40), Exp: proto.Int64(100)})
	}
	gameRaw, err := model.MarshalProfileGame86(game)
	if err != nil {
		t.Fatal(err)
	}
	playerJSON, gameJSON := string(playerRaw), string(gameRaw)
	created, chips := int64(123), int64(8)
	return &db.Profile{Id: 1, Uid: "snapshot", ServerID: 1, Session: []byte("0123456789abcdef"), AccountID: 12, AccountAuthRevision: 2, Player: &playerJSON, Game: &gameJSON, CreatedAt: &created, GGOCredit: 4, GGOCreditMigrated: true, GGOChipTotal: &chips, PasswordSalt: []byte("salt"), PasswordHash: []byte("hash")}
}

func p10FinalSnapshotRoute(t testing.TB, profile *db.Profile, decodes *int) *p10SceneRouteSnapshot {
	t.Helper()
	route := &p10SceneRouteSnapshot{pureRoute: true, decode: func(row *db.Profile) (*pb.Player, *model.GameState, error) {
		(*decodes)++
		return decodeProfile(row)
	}}
	for i := 0; i < 2; i++ {
		if _, err := route.routeGame(p10SceneProfileSnapshot(profile)); err != nil {
			t.Fatal(err)
		}
	}
	return route
}

func p10AssertSameDecoded(t *testing.T, gotPlayer *pb.Player, gotGame *model.GameState, wantProfile *db.Profile) {
	t.Helper()
	wantPlayer, wantGame, err := decodeProfile(wantProfile)
	if err != nil {
		t.Fatal(err)
	}
	gotJSON, gotErr := json.Marshal(gotGame)
	wantJSON, wantErr := json.Marshal(wantGame)
	if gotErr != nil || wantErr != nil || !proto.Equal(gotPlayer, wantPlayer) || !bytes.Equal(gotJSON, wantJSON) || gotGame.GGOCredit != wantGame.GGOCredit || gotGame.GGOCreditLoaded != wantGame.GGOCreditLoaded {
		t.Fatal("decoded player or game differs from a fresh decode")
	}
}

func TestP10SceneFinalSnapshotReusesIdenticalAuthorizedRow(t *testing.T) {
	for _, roster := range []int{4, 400} {
		t.Run(fmt.Sprintf("roster-%d", roster), func(t *testing.T) {
			profile := p10FinalSnapshotFixture(t, roster)
			if roster == 400 && !strings.Contains(*profile.Game, constUtil.ProfileWarriorsCodec86) {
				t.Fatal("large fixture must exercise the compressed roster")
			}
			decodes := 0
			route := p10FinalSnapshotRoute(t, profile, &decodes)
			player, game, err := route.authorizedProfile(p10SceneProfileSnapshot(profile))
			if err != nil || decodes != 1 || player != route.player || game != route.game {
				t.Fatalf("matching row should reuse one decode: decodes=%d err=%v", decodes, err)
			}
			p10AssertSameDecoded(t, player, game, profile)
			if _, _, err = route.authorizedProfile(profile); err != nil || decodes != 2 {
				t.Fatalf("the decoded objects must not be handed out twice: decodes=%d err=%v", decodes, err)
			}
		})
	}
}

func TestP10SceneFinalSnapshotInvalidatesEveryProfileColumn(t *testing.T) {
	changes := map[string]func(*db.Profile){
		"GGOCredit":           func(p *db.Profile) { p.GGOCredit++ },
		"GGOCreditMigrated":   func(p *db.Profile) { p.GGOCreditMigrated = false },
		"GGOChipTotal":        func(p *db.Profile) { *p.GGOChipTotal++ },
		"AccountID":           func(p *db.Profile) { p.AccountID++ },
		"AccountAuthRevision": func(p *db.Profile) { p.AccountAuthRevision++ },
		"ServerID":            func(p *db.Profile) { p.ServerID++ },
		"Id":                  func(p *db.Profile) { p.Id++ },
		"Uid":                 func(p *db.Profile) { p.Uid += "-other" },
		"Session":             func(p *db.Profile) { p.Session[0] ^= 1 },
		"Player": func(p *db.Profile) {
			*p.Player = `{"playerId":1,"level":7,"warriorBagSize":10,"equipBagSize":10,"petBagSize":10}`
		},
		"Game":                  func(p *db.Profile) { *p.Game = `{"position":{"scene":"field_001"},"warriors":[]}` },
		"CreatedAt":             func(p *db.Profile) { *p.CreatedAt++ },
		"LastSproutLoginAt":     func(p *db.Profile) { p.LastSproutLoginAt++ },
		"ReturnSproutStartedAt": func(p *db.Profile) { p.ReturnSproutStartedAt++ },
		"PasswordSalt":          func(p *db.Profile) { p.PasswordSalt[0] ^= 1 },
		"PasswordHash":          func(p *db.Profile) { p.PasswordHash[0] ^= 1 },
	}
	profileType := reflect.TypeOf(db.Profile{})
	if len(changes) != profileType.NumField() {
		t.Fatal("add an invalidation case for every Profile field")
	}
	for i := 0; i < profileType.NumField(); i++ {
		name := profileType.Field(i).Name
		change, ok := changes[name]
		if !ok {
			t.Fatalf("missing Profile field %s", name)
		}
		t.Run(name, func(t *testing.T) {
			profile := p10FinalSnapshotFixture(t, 4)
			decodes := 0
			route := p10FinalSnapshotRoute(t, profile, &decodes)
			change(profile)
			player, game, err := route.authorizedProfile(profile)
			if err != nil || decodes != 2 {
				t.Fatalf("changed row must be decoded again: decodes=%d err=%v", decodes, err)
			}
			p10AssertSameDecoded(t, player, game, profile)
		})
	}
}

func TestP10SceneProfileSnapshotDetachesReferenceFields(t *testing.T) {
	profile := p10FinalSnapshotFixture(t, 4)
	snapshot := p10SceneProfileSnapshot(profile)
	profile.Session[0] ^= 1
	profile.PasswordSalt[0] ^= 1
	profile.PasswordHash[0] ^= 1
	*profile.Player = "changed player"
	*profile.Game = "changed game"
	*profile.CreatedAt++
	*profile.GGOChipTotal++
	if reflect.DeepEqual(snapshot, profile) || string(snapshot.Session) != "0123456789abcdef" || string(snapshot.PasswordSalt) != "salt" || string(snapshot.PasswordHash) != "hash" || *snapshot.Player == *profile.Player || *snapshot.Game == *profile.Game || *snapshot.CreatedAt == *profile.CreatedAt || *snapshot.GGOChipTotal == *profile.GGOChipTotal {
		t.Fatal("snapshot aliased a reference field")
	}
	// Empty non-nil and nil slices remain distinguishable to DeepEqual.
	profile.Session = []byte{}
	profile.PasswordSalt = nil
	if !reflect.DeepEqual(p10SceneProfileSnapshot(profile), profile) {
		t.Fatal("snapshot changed nil or empty field semantics")
	}
	known := map[string]bool{"Session": true, "PasswordSalt": true, "PasswordHash": true, "Player": true, "Game": true, "CreatedAt": true, "GGOChipTotal": true}
	profileType := reflect.TypeOf(db.Profile{})
	for i := 0; i < profileType.NumField(); i++ {
		field := profileType.Field(i)
		switch field.Type.Kind() {
		case reflect.Pointer, reflect.Slice, reflect.Map, reflect.Interface:
			if !known[field.Name] {
				t.Fatalf("snapshot must detach new reference field %s", field.Name)
			}
		}
	}
}

func TestP10SceneFinalSnapshotRequiresBothUnchangedRoutes(t *testing.T) {
	profile := p10FinalSnapshotFixture(t, 4)
	decodes := 0
	route := &p10SceneRouteSnapshot{pureRoute: true, decode: func(row *db.Profile) (*pb.Player, *model.GameState, error) {
		decodes++
		return decodeProfile(row)
	}}
	if _, err := route.routeGame(profile); err != nil {
		t.Fatal(err)
	}
	changed := p10SceneProfileSnapshot(profile)
	changed.GGOCredit++
	if _, err := route.routeGame(changed); err != nil {
		t.Fatal(err)
	}
	player, game, err := route.authorizedProfile(changed)
	if err != nil || decodes != 3 {
		t.Fatalf("changed preflight must force final decode: decodes=%d err=%v", decodes, err)
	}
	p10AssertSameDecoded(t, player, game, changed)

	decodes = 0
	missingRoute := &p10SceneRouteSnapshot{pureRoute: true, decode: route.decode}
	if _, err = missingRoute.routeGame(profile); err != nil {
		t.Fatal(err)
	}
	if _, _, err = missingRoute.authorizedProfile(profile); err != nil || decodes != 2 {
		t.Fatalf("one preflight must force final decode: decodes=%d err=%v", decodes, err)
	}
}

func TestP10SceneFinalSnapshotRejectsCorruptOrMissingSave(t *testing.T) {
	for name, raw := range map[string]*string{
		"malformed": func() *string { v := `{"warriors":[}`; return &v }(),
		"bad-compressed": func() *string {
			v := `{"warriors":{"codec":"hyjs-warriors-zlib-v1","bytes":3,"sha256":"0000000000000000000000000000000000000000000000000000000000000000","data":"AQ=="}}`
			return &v
		}(),
		"missing": nil,
	} {
		t.Run(name, func(t *testing.T) {
			profile := p10FinalSnapshotFixture(t, 4)
			decodes := 0
			route := p10FinalSnapshotRoute(t, profile, &decodes)
			bad := p10SceneProfileSnapshot(profile)
			bad.Game = raw
			if _, _, err := route.authorizedProfile(bad); err == nil || decodes != 2 {
				t.Fatalf("bad final save must be decoded and rejected: decodes=%d err=%v", decodes, err)
			}
			if _, err := route.routeGame(bad); err == nil || route.profile != nil || route.player != nil || route.game != nil {
				t.Fatal("failed preflight retained a usable decoded snapshot")
			}
		})
	}
}

func TestP10SceneFinalSnapshotOldGuildExitDisablesReuse(t *testing.T) {
	ctx, service, profile := p10RouteFixture(t)
	old := db.P10ScenePresence{PlayerID: profile.Id, Scene: constUtil.P13GuildRoomScopePrefix + "old", Line: 1, SessionHash: p6Digest(profile.Session), Frame: []byte{}}
	if err := db.GetIns().Create(&old).Error; err != nil {
		t.Fatal(err)
	}
	key := p10ClientKey{service.p10ClientOwner(), profile.Id}
	sink := &p10ClientSink{session: append([]byte(nil), profile.Session...)}
	p10ClientSinks.Store(key, sink)
	t.Cleanup(func() { p10ClientSinks.CompareAndDelete(key, sink) })
	decodes := 0
	route := p10RouteCountingSnapshot(&decodes)
	route.pureRoute = true
	if handled, err := service.syncP13UnionFightField(ctx, profile.Id, false, route); handled || err != nil {
		t.Fatalf("union route: handled=%v err=%v", handled, err)
	}
	if handled, err := service.syncP13GuildRoom(ctx, profile.Id, false, route); handled || err != nil {
		t.Fatalf("guild exit: handled=%v err=%v", handled, err)
	}
	var after db.P10ScenePresence
	if err := db.GetIns().First(&after, "player_id=?", profile.Id).Error; err != nil || after.SessionHash != "" || route.pureRoute {
		t.Fatalf("guild exit cleanup or reuse guard failed: err=%v", err)
	}
	authorized, err := AuthorizeAccount(ctx, profile.Id, sink.session)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err = route.authorizedProfile(authorized); err != nil || decodes != 2 {
		t.Fatalf("guild exit must force final decode: decodes=%d err=%v", decodes, err)
	}
}

func TestP10SceneFinalSnapshotPreservesMovePlayerFrame(t *testing.T) {
	t.Setenv("HYJS_P10_MYSQL_DATABASE", "")
	f, _, _ := p10SceneSetup(t)
	var profile db.Profile
	if err := db.GetIns().First(&profile, f.id).Error; err != nil {
		t.Fatal(err)
	}
	decodes := 0
	route := p10FinalSnapshotRoute(t, &profile, &decodes)
	cachedPlayer, cachedGame, err := route.authorizedProfile(p10SceneProfileSnapshot(&profile))
	if err != nil || decodes != 1 {
		t.Fatalf("ordinary scene did not reuse the snapshot: decodes=%d err=%v", decodes, err)
	}
	freshPlayer, freshGame, err := decodeProfile(&profile)
	if err != nil {
		t.Fatal(err)
	}
	cached, err := f.s.p10SceneMovePlayer(cachedPlayer, cachedGame)
	if err != nil {
		t.Fatal(err)
	}
	fresh, err := f.s.p10SceneMovePlayer(freshPlayer, freshGame)
	if err != nil {
		t.Fatal(err)
	}
	encoder := proto.MarshalOptions{Deterministic: true}
	cachedWire, cachedErr := encoder.Marshal(cached)
	freshWire, freshErr := encoder.Marshal(fresh)
	if cachedErr != nil || freshErr != nil || !proto.Equal(cached, fresh) || !bytes.Equal(cachedWire, freshWire) {
		t.Fatal("scene appearance frame differs from a fresh decode")
	}
}

func TestP10SceneFinalSnapshotKeepsLiveAuthorization(t *testing.T) {
	for _, test := range []struct {
		name   string
		setup  func(*testing.T, *GameService, *db.Profile, *p10ClientSink)
		failed bool
	}{
		{name: "valid"},
		{name: "rotated-session", failed: true, setup: func(t *testing.T, _ *GameService, _ *db.Profile, sink *p10ClientSink) {
			sink.session = []byte("fedcba9876543210")
		}},
		{name: "disabled-account", failed: true, setup: func(t *testing.T, _ *GameService, profile *db.Profile, _ *p10ClientSink) {
			if err := db.GetIns().AutoMigrate(&db.GameAccount{}); err != nil {
				t.Fatal(err)
			}
			account := db.GameAccount{ID: 2, Username: "snapshot-account", Nickname: "snapshot", PasswordSalt: []byte("salt"), PasswordHash: []byte("hash"), Enabled: false, AuthRevision: 3}
			if err := db.GetIns().Create(&account).Error; err != nil {
				t.Fatal(err)
			}
			if err := db.GetIns().Model(&db.Profile{}).Where("id=?", profile.Id).Updates(map[string]any{"account_id": account.ID, "account_auth_revision": account.AuthRevision}).Error; err != nil {
				t.Fatal(err)
			}
		}},
		{name: "maintenance", failed: true, setup: func(t *testing.T, service *GameService, _ *db.Profile, _ *p10ClientSink) {
			d := db.GetIns()
			if err := service.InitializeClientUpdates(t.Context()); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { clientUpdateReady.Delete(clientUpdateDBKey(d)) })
			if err := d.Model(&db.ClientUpdatePolicy{}).Where("id=1").Update("maintenance", true).Error; err != nil {
				t.Fatal(err)
			}
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			ctx, service, profile := p10RouteFixture(t)
			if err := service.InitializeP10Scene(ctx); err != nil {
				t.Fatal(err)
			}
			if err := db.GetIns().Model(&db.P10ScenePolicy{}).Where("id=?", constUtil.P10ScenePolicyID).Update("enabled", true).Error; err != nil {
				t.Fatal(err)
			}
			// An unlisted scene exits immediately after final authorization/decode.
			if err := db.GetIns().Model(&db.Profile{}).Where("id=?", profile.Id).Update("game", `{"position":{"scene":"unlisted"},"warriors":[]}`).Error; err != nil {
				t.Fatal(err)
			}
			sink := &p10ClientSink{session: append([]byte(nil), profile.Session...)}
			if test.setup != nil {
				test.setup(t, service, profile, sink)
			}
			key := p10ClientKey{service.p10ClientOwner(), profile.Id}
			p10ClientSinks.Store(key, sink)
			t.Cleanup(func() { p10ClientSinks.CompareAndDelete(key, sink) })
			profileReads := 0
			callback := db.GetIns().Callback().Query()
			if err := callback.After("gorm:query").Register("p10_final_authorization_reads", func(tx *gorm.DB) {
				if tx.Statement.Table == "profiles" {
					profileReads++
				}
			}); err != nil {
				t.Fatal(err)
			}
			defer callback.Remove("p10_final_authorization_reads")
			err := service.SyncP10Scene(ctx, profile.Id, false)
			if (err != nil) != test.failed || profileReads != 3 {
				t.Fatalf("final auth or profile reads changed: failed=%v reads=%d err=%v", test.failed, profileReads, err)
			}
			var count int64
			if err := db.GetIns().Model(&db.P10ScenePresence{}).Count(&count).Error; err != nil || count != 0 {
				t.Fatalf("failed or unlisted scene changed presence: count=%d err=%v", count, err)
			}
		})
	}
}

func TestP10SceneFinalSnapshotRechecksChangesBeforeFinalAuthorization(t *testing.T) {
	for _, test := range []struct {
		name    string
		updates map[string]any
	}{
		{name: "session", updates: map[string]any{"session": []byte("fedcba9876543210")}},
		{name: "ggo-credit", updates: map[string]any{"ggo_credit": int64(-1)}},
		{name: "corrupt-game", updates: map[string]any{"game": `{"warriors":[}`}},
	} {
		t.Run(test.name, func(t *testing.T) {
			ctx, service, profile := p10RouteFixture(t)
			if err := service.InitializeP10Scene(ctx); err != nil {
				t.Fatal(err)
			}
			if err := db.GetIns().Model(&db.P10ScenePolicy{}).Where("id=?", constUtil.P10ScenePolicyID).Update("enabled", true).Error; err != nil {
				t.Fatal(err)
			}
			sink := &p10ClientSink{session: append([]byte(nil), profile.Session...)}
			key := p10ClientKey{service.p10ClientOwner(), profile.Id}
			p10ClientSinks.Store(key, sink)
			t.Cleanup(func() { p10ClientSinks.CompareAndDelete(key, sink) })
			var hookErr error
			profileReads := 0
			callback := db.GetIns().Callback().Query()
			if err := callback.After("gorm:query").Register("p10_final_change_after_routes", func(tx *gorm.DB) {
				if tx.Statement.Table == "profiles" {
					profileReads++
					if profileReads == 2 {
						hookErr = tx.Session(&gorm.Session{NewDB: true}).Model(&db.Profile{}).Where("id=?", profile.Id).Updates(test.updates).Error
					}
				}
			}); err != nil {
				t.Fatal(err)
			}
			defer callback.Remove("p10_final_change_after_routes")
			err := service.SyncP10Scene(ctx, profile.Id, false)
			if hookErr != nil || err == nil || profileReads != 3 {
				t.Fatalf("changed final row was not rejected: reads=%d err=%v hook=%v", profileReads, err, hookErr)
			}
			var count int64
			if err := db.GetIns().Model(&db.P10ScenePresence{}).Count(&count).Error; err != nil || count != 0 {
				t.Fatalf("invalid changed row affected scene presence: count=%d err=%v", count, err)
			}
		})
	}
}

// Same compressed profile and the same three observations; excludes SQL and
// authentication so the measured difference is only the routing decode work.
func BenchmarkP10SceneFinalSnapshot(b *testing.B) {
	// Pick representative raw JSON sizes without using real player data.
	probe := &pb.Warrior{Id: proto.String("hero_001"), Guid: proto.String("snapshot-0000"), Level: proto.Int32(40), Exp: proto.Int64(100)}
	probeJSON, err := json.Marshal(probe)
	if err != nil {
		b.Fatal(err)
	}
	for _, fixture := range []struct {
		name      string
		targetLen int
	}{{"average_69k", 69259}, {"large_144k", 144395}} {
		b.Run(fixture.name, func(b *testing.B) {
			roster := (fixture.targetLen - 1) / (len(probeJSON) + 1)
			profile := p10FinalSnapshotFixture(b, roster)
			if !strings.Contains(*profile.Game, constUtil.ProfileWarriorsCodec86) {
				b.Fatal("benchmark fixture is not compressed")
			}
			expanded, err := model.ExpandProfileGame86([]byte(*profile.Game))
			if err != nil {
				b.Fatal(err)
			}
			var fields struct {
				Warriors json.RawMessage `json:"warriors"`
			}
			if err = json.Unmarshal(expanded, &fields); err != nil {
				b.Fatal(err)
			}
			rosterBytes := len(fields.Warriors)
			if rosterBytes < fixture.targetLen-256 || rosterBytes > fixture.targetLen+256 {
				b.Fatalf("unexpected fixture size: target=%d actual=%d", fixture.targetLen, rosterBytes)
			}
			first := p10SceneProfileSnapshot(profile)
			second := p10SceneProfileSnapshot(profile)
			third := p10SceneProfileSnapshot(profile)
			b.Run("previous", func(b *testing.B) {
				b.ReportAllocs()
				for i := 0; i < b.N; i++ {
					_, game, err := decodeProfile(first)
					if err != nil || game == nil || !reflect.DeepEqual(first, second) {
						b.Fatal(err)
					}
					if _, _, err = decodeProfile(third); err != nil {
						b.Fatal(err)
					}
				}
				b.ReportMetric(float64(rosterBytes), "roster-bytes")
			})
			b.Run("reuse", func(b *testing.B) {
				b.ReportAllocs()
				for i := 0; i < b.N; i++ {
					route := p10SceneRouteSnapshot{pureRoute: true}
					if _, err := route.routeGame(first); err != nil {
						b.Fatal(err)
					}
					if _, err := route.routeGame(second); err != nil {
						b.Fatal(err)
					}
					if _, _, err := route.authorizedProfile(third); err != nil {
						b.Fatal(err)
					}
				}
				b.ReportMetric(float64(rosterBytes), "roster-bytes")
			})
		})
	}
}
