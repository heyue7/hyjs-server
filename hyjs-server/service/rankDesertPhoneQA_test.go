package service

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"hyjs-server/config"
	"hyjs-server/db"
	pb "hyjs-server/model/protocol"
	"hyjs-server/utils"
)

type rankDesertPhoneIdentity struct {
	PlayerID   int64  `json:"playerId"`
	SessionHex string `json:"sessionHex"`
	Username   string `json:"username"`
	Password   string `json:"password"`
}

// rankDesertPhonePrivateTemp stages a private file in the fixture directory so
// rename is atomic and the original survives any database transaction failure.
func rankDesertPhonePrivateTemp(path string, data []byte) (string, error) {
	f, err := os.CreateTemp(filepath.Dir(path), ".rank-desert-phone-*")
	if err != nil {
		return "", err
	}
	name := f.Name()
	ok := false
	defer func() {
		if !ok {
			_ = f.Close()
			_ = os.Remove(name)
		}
	}()
	if err = f.Chmod(0600); err != nil {
		return "", err
	}
	if _, err = f.Write(data); err != nil {
		return "", err
	}
	if err = f.Sync(); err != nil {
		return "", err
	}
	if err = f.Close(); err != nil {
		return "", err
	}
	ok = true
	return name, nil
}

// This entry point only enriches the two freshly created synthetic phone roles.
// It is explicitly gated and retains the existing fixture's private path and
// isolated-schema restrictions.
func TestRankDesertIsolatedPhoneFixture(t *testing.T) {
	if os.Getenv("HYJS_RANK_DESERT_PHONE_QA") != "1" {
		t.Skip("isolated rank/desert phone fixture not requested")
	}
	output := os.Getenv("HYJS_DESERT_QA_FIXTURE")
	if output == "" {
		t.Fatal("isolated private fixture path required")
	}
	TestGGOIsolatedPerformance117PhoneFixture(t)

	// The existing fixture closes its own pool when it returns. Reopen only the
	// same already-created schema after repeating the isolation and path guards.
	const prefix = "hyjs_test_p1_performance117_phone_"
	database := os.Getenv("HYJS_MYSQL_DATABASE")
	root, err := filepath.Abs("../../.local-env/performance117-ui")
	if err != nil || os.Getenv("HYJS_RANK_DESERT_PHONE_QA") != "1" || os.Getenv("HYJS_MYSQL_HOST") != "127.0.0.1:3306" || !strings.HasPrefix(database, prefix) || len(database) == len(prefix) || os.Getenv("HYJS_GAME_ACCOUNTS_DATABASE") != "" || !filepath.IsAbs(output) || filepath.Clean(output) != output {
		t.Fatal("isolated phone fixture guard failed")
	}
	rootInfo, err := os.Lstat(root)
	if err != nil || !rootInfo.IsDir() || rootInfo.Mode()&os.ModeSymlink != 0 {
		t.Fatal("private fixture directory invalid")
	}
	fileInfo, err := os.Lstat(output)
	if err != nil || !fileInfo.Mode().IsRegular() || fileInfo.Mode().Perm() != 0600 || fileInfo.Size() > 1<<20 {
		t.Fatal("private fixture file invalid")
	}
	resolvedRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		t.Fatal("private fixture root unavailable")
	}
	resolvedFile, err := filepath.EvalSymlinks(output)
	if err != nil {
		t.Fatal("private fixture unavailable")
	}
	relative, err := filepath.Rel(resolvedRoot, resolvedFile)
	if err != nil || filepath.IsAbs(relative) || relative == "." || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		t.Fatal("private fixture outside designated directory")
	}
	if config.AppConfigIns == nil {
		t.Fatal("isolated configuration unavailable")
	}
	mc := config.AppConfigIns.MysqlConfig
	if mc.Host != "127.0.0.1:3306" || mc.DbName != database || config.AppConfigIns.Game.AccountsDatabase != "" {
		t.Fatal("isolated database configuration mismatch")
	}
	ctx := context.Background()
	if err := db.InitExistingGameMySQL(ctx, db.MySQLConfig{Host: mc.Host, User: mc.User, Password: mc.Pass, Database: mc.DbName}); err != nil {
		t.Fatal("isolated database reopen failed")
	}
	pool, err := db.GetIns().DB()
	if err != nil {
		t.Fatal("isolated database pool unavailable")
	}
	pool.SetMaxOpenConns(3)
	defer pool.Close()
	var actual string
	if err := db.GetIns().Raw("SELECT DATABASE()").Scan(&actual).Error; err != nil || actual != database {
		t.Fatal("isolated database identity mismatch")
	}

	original, err := os.ReadFile(output)
	if err != nil {
		t.Fatal("private fixture read failed")
	}
	var document map[string]json.RawMessage
	var identities []rankDesertPhoneIdentity
	var fixtureDatabase string
	if json.Unmarshal(original, &document) != nil || json.Unmarshal(document["schema"], &fixtureDatabase) != nil || fixtureDatabase != database || json.Unmarshal(document["players"], &identities) != nil || len(identities) != 2 || identities[0].PlayerID != 2 || identities[1].PlayerID != 3 || identities[0].Username != "qa-desert-player-1" || identities[1].Username != "qa-desert-player-2" || len(identities[0].Password) < 16 {
		t.Fatal("synthetic fixture identities invalid")
	}
	if _, err := hex.DecodeString(identities[0].Password); err != nil {
		t.Fatal("synthetic fixture credential invalid")
	}
	var playerDocs []map[string]json.RawMessage
	if json.Unmarshal(document["players"], &playerDocs) != nil || len(playerDocs) != 2 {
		t.Fatal("synthetic fixture player data invalid")
	}
	shortPassword := identities[0].Password[:16]
	playerDocs[0]["password"], err = json.Marshal(shortPassword)
	if err != nil {
		t.Fatal("private fixture preparation failed")
	}
	document["players"], err = json.Marshal(playerDocs)
	if err != nil {
		t.Fatal("private fixture preparation failed")
	}
	updatedFixture, err := json.Marshal(document)
	if err != nil {
		t.Fatal("private fixture preparation failed")
	}

	var profiles [2]db.Profile
	var accounts [2]db.GameAccount
	for i := range identities {
		session, e := hex.DecodeString(identities[i].SessionHex)
		if e != nil || len(session) == 0 || db.GetIns().First(&profiles[i], identities[i].PlayerID).Error != nil || profiles[i].AccountID <= 0 || profiles[i].Uid != identities[i].Username || !bytes.Equal(profiles[i].Session, session) || db.GetIns().First(&accounts[i], profiles[i].AccountID).Error != nil || accounts[i].Username != identities[i].Username || accounts[i].Protected || !accounts[i].Enabled {
			t.Fatal("isolated synthetic role guard failed")
		}
	}
	if accounts[0].ID == accounts[1].ID || profiles[1].Player == nil || profiles[1].Game == nil {
		t.Fatal("isolated synthetic role relationship invalid")
	}
	var second pb.Player
	if json.Unmarshal([]byte(*profiles[1].Player), &second) != nil || second.GetLevel() != 60 {
		t.Fatal("isolated second role prerequisite invalid")
	}
	valid, _, err := utils.VerifyPasswordCredential(ctx, identities[0].Password, accounts[0].PasswordSalt, accounts[0].PasswordHash)
	if err != nil || !valid || !bytes.Equal(profiles[0].PasswordSalt, accounts[0].PasswordSalt) || !bytes.Equal(profiles[0].PasswordHash, accounts[0].PasswordHash) {
		t.Fatal("isolated credential binding invalid")
	}
	salt, hash, err := utils.NewPasswordCredential(ctx, shortPassword)
	if err != nil {
		t.Fatal("isolated credential preparation failed")
	}

	fixtureRoot := "../../analysis"
	if configured := os.Getenv("HYJS_TEST_FIXTURE_ROOT"); configured != "" {
		if !filepath.IsAbs(configured) {
			t.Fatal("isolated analysis path invalid")
		}
		fixtureRoot = filepath.Clean(configured)
	}
	tables := filepath.Join(fixtureRoot, "tables")
	s := &GameService{ServerID: 1}
	for _, load := range []func(string) error{s.LoadTables, s.LoadP5Tables, s.LoadP8Tables, s.LoadP9Tables, s.LoadP12WorldBossTables} {
		if err := load(tables); err != nil {
			t.Fatal("isolated original tables unavailable")
		}
	}
	s.InstallGrowth74Resources()
	if err := s.ApplyP15GreenHeroCompatibility(); err != nil {
		t.Fatal("isolated hero compatibility unavailable")
	}
	if err := s.InstallGGOItems(); err != nil {
		t.Fatal("isolated GGO items unavailable")
	}
	s.InitializeGameTableLookup()
	ids := []string{"hero_065", "hero_066", "hero_067", "hero_074", "hero_075"}
	_, formation, _ := desertBossCalibrationTeam(t, s, ids)
	if len(formation.Warriors) != len(ids) || formation.Warriors[0].GetGuid() == "" {
		t.Fatal("isolated formation invalid")
	}
	titles := map[string]string{}
	for _, id := range []string{"title_001", "title_005", "title_019", "title_026"} {
		row := s.table("Title", "titleId", id)
		if row == nil || row["effects"] == "" {
			t.Fatal("isolated title source unavailable")
		}
		raw, err := json.Marshal(p5TitleState{Effects: row["effects"], Source: "isolated synthetic phone QA", DeadTime: 0})
		if err != nil {
			t.Fatal("isolated title preparation failed")
		}
		titles[id] = string(raw)
	}

	staged, err := rankDesertPhonePrivateTemp(output, updatedFixture)
	if err != nil {
		t.Fatal("private fixture staging failed")
	}
	defer os.Remove(staged)
	backup, err := rankDesertPhonePrivateTemp(output, original)
	if err != nil {
		t.Fatal("private fixture backup staging failed")
	}
	removeBackup := true
	defer func() {
		if removeBackup {
			_ = os.Remove(backup)
		}
	}()
	var power int32
	renamed := false
	err = db.GetIns().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var profile db.Profile
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&profile, identities[0].PlayerID).Error; err != nil || profile.AccountID != accounts[0].ID || profile.Uid != identities[0].Username || !bytes.Equal(profile.Session, profiles[0].Session) || !bytes.Equal(profile.PasswordSalt, profiles[0].PasswordSalt) || !bytes.Equal(profile.PasswordHash, profiles[0].PasswordHash) {
			return errors.New("isolated role changed during preparation")
		}
		var account db.GameAccount
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&account, profile.AccountID).Error; err != nil || account.Username != identities[0].Username || account.Protected || !account.Enabled || !bytes.Equal(account.PasswordSalt, accounts[0].PasswordSalt) || !bytes.Equal(account.PasswordHash, accounts[0].PasswordHash) {
			return errors.New("isolated account changed during preparation")
		}
		player, game, err := decodeProfile(&profile)
		if err != nil || player.GetPlayerId() != identities[0].PlayerID || player.GetLevel() != 60 || game.GGOField == nil || game.GGOField.Chips != [6]int64{10} || game.GGOField.EconomyVersion == "" {
			return errors.New("isolated growth prerequisite invalid")
		}
		ggoBefore, err := json.Marshal(game.GGOField)
		if err != nil {
			return errors.New("isolated terminal snapshot failed")
		}
		game.Warriors, game.Equipment, game.Treasures, game.Formation = formation.Warriors, formation.Equipment, formation.Treasures, formation.Formation
		game.TacticalFormations, game.TacticalSkillId = formation.TacticalFormations, formation.TacticalSkillId
		for _, id := range []string{"title_001", "title_005", "title_019", "title_026"} {
			p5SetKV(game, "p5.title:"+id, titles[id])
		}
		p5SetKV(game, "p5.current-title", "title_001")
		game.Position = &pb.Position{PlayerId: proto.Int64(profile.Id), Scene: proto.String("town_main"), Current: &pb.PointNew{X: proto.Float32(0), Y: proto.Float32(0), Z: proto.Float32(0)}}
		game.PositionLoading, game.Field = false, nil
		player.Level, player.HeroWarId = proto.Int32(100), proto.String(formation.Warriors[0].GetGuid())
		power, err = s.formationFightPower(player, game)
		if err != nil || power <= 0 {
			return errors.New("isolated formation power unavailable")
		}
		player.FightPower = proto.Int32(power)
		ggoAfter, err := json.Marshal(game.GGOField)
		if err != nil || !bytes.Equal(ggoBefore, ggoAfter) {
			return errors.New("isolated terminal changed during preparation")
		}
		if err := saveProfile(tx, &profile, player, game); err != nil {
			return errors.New("isolated role save failed")
		}
		updated := tx.Model(&db.Profile{}).Where("id=? AND account_id=?", profile.Id, account.ID).Updates(map[string]any{"password_salt": salt, "password_hash": hash})
		if updated.Error != nil || updated.RowsAffected != 1 {
			return errors.New("isolated role credential update failed")
		}
		updated = db.AccountsTable(tx, "game_accounts").Where("id=? AND protected=? AND enabled=?", account.ID, false, true).Updates(map[string]any{"password_salt": salt, "password_hash": hash})
		if updated.Error != nil || updated.RowsAffected != 1 {
			return errors.New("isolated account credential update failed")
		}
		var saved db.Profile
		if err := tx.First(&saved, profile.Id).Error; err != nil || !bytes.Equal(saved.Session, profiles[0].Session) || !bytes.Equal(saved.PasswordSalt, salt) || !bytes.Equal(saved.PasswordHash, hash) {
			return errors.New("isolated role verification failed")
		}
		var savedAccount db.GameAccount
		if err := tx.First(&savedAccount, account.ID).Error; err != nil || !bytes.Equal(savedAccount.PasswordSalt, salt) || !bytes.Equal(savedAccount.PasswordHash, hash) || savedAccount.AuthRevision != accounts[0].AuthRevision {
			return errors.New("isolated account verification failed")
		}
		savedPlayer, savedGame, err := decodeProfile(&saved)
		if err != nil || savedPlayer.GetFightPower() != power || savedPlayer.GetLevel() != 100 || savedPlayer.GetHeroWarId() != formation.Warriors[0].GetGuid() || savedGame.Position.GetScene() != "town_main" || savedGame.PositionLoading || savedGame.Field != nil {
			return errors.New("isolated formation verification failed")
		}
		recomputed, err := s.formationFightPower(savedPlayer, savedGame)
		if err != nil || recomputed != power || len(savedGame.Warriors) != len(ids) || p5KV(savedGame, "p5.current-title") != "title_001" {
			return errors.New("isolated saved power verification failed")
		}
		for _, id := range []string{"title_001", "title_005", "title_019", "title_026"} {
			if p5KV(savedGame, "p5.title:"+id) != titles[id] {
				return errors.New("isolated title verification failed")
			}
		}
		ggoSaved, err := json.Marshal(savedGame.GGOField)
		if err != nil || !bytes.Equal(ggoBefore, ggoSaved) {
			return errors.New("isolated terminal verification failed")
		}
		var untouched db.Profile
		if err := tx.First(&untouched, profiles[1].Id).Error; err != nil || untouched.Player == nil || untouched.Game == nil || *untouched.Player != *profiles[1].Player || *untouched.Game != *profiles[1].Game || !bytes.Equal(untouched.Session, profiles[1].Session) {
			return errors.New("isolated second role changed")
		}
		if err := os.Rename(staged, output); err != nil {
			return errors.New("private fixture publication failed")
		}
		renamed = true
		return nil
	})
	if err != nil {
		if renamed {
			if restoreErr := os.Rename(backup, output); restoreErr != nil {
				removeBackup = false
				t.Fatal("isolated transaction failed; private fixture backup retained")
			}
		}
		t.Fatal("isolated transaction failed")
	}
	t.Logf("isolated=true playerId=%d formationPower=%d", identities[0].PlayerID, power)
}
