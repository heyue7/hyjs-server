package service

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	mysqlDriver "github.com/go-sql-driver/mysql"
	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
	"hyjs-server/config"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

// This fixture participates in an external, managed backup orchestration. It
// never creates/drops a database, imports a dump, starts a server, or exercises
// HTTP/TCP. The caller owns both database allocations and the private files.
//
// Source: HYJS_GGO_RESTORE_DRILL=1, HYJS_P1_MYSQL_{HOST,USER,PASSWORD,DATABASE},
// HYJS_GGO_RESTORE_DIR, and optionally HYJS_GGO_RESTORE_WAIT_SECONDS (1..600).
// The private directory must be a new, owned 0700 directory named with the
// prefix hyjs-ggo-restore86-. Source writes session.bin then source.json, both
// 0600, and waits for 0600 backup.done containing status=authenticated,
// sourceSHA256=<source.json SHA-256>, backupSHA256=<ciphertext SHA-256>.
// Acknowledgement only reports what the external runner says; this test does
// not authenticate the backup itself. Run Verify against a different managed
// database/account after import and before a game process changes the target.

type ggoRestoreDrillProfile86 struct {
	ID              int64  `json:"id"`
	ServerID        int64  `json:"serverId"`
	AccountID       int64  `json:"accountId"`
	AccountRevision int64  `json:"accountRevision"`
	Credit          int64  `json:"credit"`
	CreditMigrated  bool   `json:"creditMigrated"`
	PlayerSHA256    string `json:"playerSha256"`
	GameSHA256      string `json:"gameSha256"`
	SessionSHA256   string `json:"sessionSha256"`
}

type ggoRestoreDrillState86 struct {
	Profiles            []ggoRestoreDrillProfile86 `json:"profiles"`
	BossRowsSHA256      string                     `json:"bossRowsSha256"`
	LineRowsSHA256      string                     `json:"lineRowsSha256"`
	ReceiptsSHA256      string                     `json:"receiptsSha256"`
	ContributionsSHA256 string                     `json:"contributionsSha256"`
	UpdateBindingsSHA   string                     `json:"updateBindingsSha256"`
	WalletBalanceSHA    string                     `json:"walletBalanceSha256"`
	WalletEntriesSHA    string                     `json:"walletEntriesSha256"`
	BossRows            int                        `json:"bossRows"`
	LineRows            int                        `json:"lineRows"`
	Receipts            int                        `json:"receipts"`
	Contributions       int                        `json:"contributions"`
	WalletEntries       int                        `json:"walletEntries"`
}

type ggoRestoreDrillFight86 struct {
	FightID     string `json:"fightId"`
	InputSHA256 string `json:"inputSha256"`
	NativeSHA   string `json:"nativeSha256"`
	EventID     string `json:"eventId"`
}

type ggoRestoreDrillSummary86 struct {
	Schema           string                 `json:"schema"`
	SourceDatabase   string                 `json:"sourceDatabase"`
	SourceUserSHA256 string                 `json:"sourceUserSha256"`
	CreatedAtMS      int64                  `json:"createdAtMs"`
	ActorID          int64                  `json:"actorId"`
	OtherLineActorID int64                  `json:"otherLineActorId"`
	CompressedID     int64                  `json:"compressedId"`
	LegacyID         int64                  `json:"legacyId"`
	LineOneEvent     string                 `json:"lineOneEvent"`
	LineTwoEvent     string                 `json:"lineTwoEvent"`
	Active           ggoRestoreDrillFight86 `json:"active"`
	Settled          ggoRestoreDrillFight86 `json:"settled"`
	State            ggoRestoreDrillState86 `json:"state"`
}

type ggoRestoreDrillAck86 struct {
	Status       string `json:"status"`
	SourceSHA256 string `json:"sourceSHA256"`
	BackupSHA256 string `json:"backupSHA256"`
}

func ggoRestoreDrillDigest86(data []byte) string {
	return fmt.Sprintf("%x", sha256.Sum256(data))
}

func ggoRestoreDrillStartDigest86(t *testing.T, data []byte) string {
	t.Helper()
	var start pb.SCSidePveBossFightStart
	if err := proto.Unmarshal(data, &start); err != nil {
		t.Fatal("cannot decode fixture native start")
	}
	canonical, err := (proto.MarshalOptions{Deterministic: true}).Marshal(&start)
	if err != nil {
		t.Fatal("cannot hash fixture native start")
	}
	return ggoRestoreDrillDigest86(canonical)
}

func ggoRestoreDrillJSONDigest86(t *testing.T, value any) string {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatal("cannot hash isolated fixture state")
	}
	return ggoRestoreDrillDigest86(raw)
}

func ggoRestoreDrillPrivateFile86(t *testing.T, path string) []byte {
	t.Helper()
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm() != 0600 || info.Size() > 1024*1024 {
		t.Fatal("private fixture file must be an existing 0600 regular file")
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal("cannot read private fixture file")
	}
	return raw
}

func ggoRestoreDrillWrite86(t *testing.T, path string, value []byte) {
	t.Helper()
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		t.Fatal("cannot create fresh private fixture file")
	}
	if _, err = file.Write(value); err == nil {
		err = file.Sync()
	}
	if closeErr := file.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		t.Fatal("cannot write private fixture file")
	}
}

func ggoRestoreDrillDir86(t *testing.T) string {
	t.Helper()
	dir := os.Getenv("HYJS_GGO_RESTORE_DIR")
	if !filepath.IsAbs(dir) || filepath.Clean(dir) != dir || !strings.HasPrefix(filepath.Base(dir), "hyjs-ggo-restore86-") {
		t.Fatal("an absolute owned hyjs-ggo-restore86- private directory is required")
	}
	info, err := os.Lstat(dir)
	resolved, resolveErr := filepath.EvalSymlinks(dir)
	if err != nil || resolveErr != nil || !info.IsDir() || info.Mode().Perm() != 0700 || resolved != dir {
		t.Fatal("fixture directory must be a real 0700 directory without symlink components")
	}
	return dir
}

func ggoRestoreDrillManaged86(t *testing.T, fresh bool) db.MySQLConfig {
	t.Helper()
	if os.Getenv("HYJS_GGO_RESTORE_DRILL") != "1" {
		t.Skip("explicit managed GGO restore drill not requested")
	}
	cfg := db.MySQLConfig{Host: os.Getenv("HYJS_P1_MYSQL_HOST"), User: os.Getenv("HYJS_P1_MYSQL_USER"),
		Password: os.Getenv("HYJS_P1_MYSQL_PASSWORD"), Database: os.Getenv("HYJS_P1_MYSQL_DATABASE")}
	if cfg.Host != "127.0.0.1:3306" || !regexp.MustCompile(`^hyjs_test_p1_[a-z0-9_]{1,51}$`).MatchString(cfg.Database) ||
		!regexp.MustCompile(`^hyjs_t_[a-z0-9_]{1,25}$`).MatchString(cfg.User) || cfg.Password == "" ||
		os.Getenv("HYJS_GAME_ACCOUNTS_DATABASE") != "" || config.AppConfigIns != nil && config.AppConfigIns.Game.AccountsDatabase != "" {
		t.Fatal("restore drill requires a local managed P1 allocation with no external account schema")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatal("cannot locate managed allocation registry")
	}
	base := filepath.Join(home, ".codex", "local-services", "test-connections", cfg.Database)
	var record struct {
		Project  string `json:"project"`
		Database string `json:"database"`
		User     string `json:"user"`
	}
	if err = json.Unmarshal(ggoRestoreDrillPrivateFile86(t, base+".json"), &record); err != nil ||
		record.Project != "hyjs" || record.Database != cfg.Database || record.User != cfg.User {
		t.Fatal("managed allocation record differs")
	}
	values := map[string]string{}
	for _, line := range strings.Split(string(ggoRestoreDrillPrivateFile86(t, base+".env")), "\n") {
		if line == "" {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok || key == "" || values[key] != "" {
			t.Fatal("invalid managed private connection record")
		}
		values[key] = value
	}
	if values["MYSQL_HOST"] != "127.0.0.1" || values["MYSQL_PORT"] != "3306" || values["MYSQL_DATABASE"] != cfg.Database ||
		values["MYSQL_USER"] != cfg.User || subtle.ConstantTimeCompare([]byte(values["MYSQL_PASSWORD"]), []byte(cfg.Password)) != 1 {
		t.Fatal("managed connection does not match the private allocation")
	}
	if fresh {
		mysql := mysqlDriver.NewConfig()
		mysql.Net, mysql.Addr, mysql.User, mysql.Passwd, mysql.DBName = "tcp", cfg.Host, cfg.User, cfg.Password, cfg.Database
		mysql.Timeout, mysql.ReadTimeout, mysql.WriteTimeout = 5*time.Second, 5*time.Second, 5*time.Second
		connection, openErr := sql.Open("mysql", mysql.FormatDSN())
		if openErr != nil {
			t.Fatal("cannot open managed source database")
		}
		defer connection.Close()
		var selected string
		var objects int64
		query := `SELECT DATABASE(),
			(SELECT COUNT(*) FROM information_schema.tables WHERE table_schema=DATABASE()) +
			(SELECT COUNT(*) FROM information_schema.routines WHERE routine_schema=DATABASE()) +
			(SELECT COUNT(*) FROM information_schema.events WHERE event_schema=DATABASE())`
		ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
		defer cancel()
		if err = connection.QueryRowContext(ctx, query).Scan(&selected, &objects); err != nil || selected != cfg.Database || objects != 0 {
			t.Fatal("managed source must be the selected, completely empty database")
		}
	}
	return cfg
}

func ggoRestoreDrillStateRead86(t *testing.T, database *gorm.DB, source ggoRestoreDrillSummary86) ggoRestoreDrillState86 {
	t.Helper()
	var state ggoRestoreDrillState86
	ids := []int64{source.CompressedID, source.LegacyID, source.ActorID, source.OtherLineActorID}
	var profiles []db.Profile
	if err := database.Where("id IN ?", ids).Order("id").Find(&profiles).Error; err != nil || len(profiles) != len(ids) {
		t.Fatal("isolated fixture profiles missing")
	}
	for _, row := range profiles {
		if row.Player == nil || row.Game == nil {
			t.Fatal("fixture profile payload missing")
		}
		state.Profiles = append(state.Profiles, ggoRestoreDrillProfile86{ID: row.Id, ServerID: row.ServerID,
			AccountID: row.AccountID, AccountRevision: row.AccountAuthRevision, Credit: row.GGOCredit,
			CreditMigrated: row.GGOCreditMigrated, PlayerSHA256: ggoRestoreDrillDigest86([]byte(*row.Player)),
			GameSHA256: ggoRestoreDrillDigest86([]byte(*row.Game)), SessionSHA256: ggoRestoreDrillDigest86(row.Session)})
	}
	var bosses []db.SharedFieldBoss55
	if err := database.Where("server_id=? AND scene=? AND pos_id=?", 1, constUtil.GGOFieldScene, constUtil.GGOFieldBossPosition).Order("line").Find(&bosses).Error; err != nil || len(bosses) != 2 {
		t.Fatal("isolated two-line boss rows missing")
	}
	for index, row := range bosses {
		event, _, _, err := ggoBossDecode(&row)
		if err != nil || row.Line != int64(index+1) || (index == 0 && event.ID != source.LineOneEvent) || (index == 1 && event.ID != source.LineTwoEvent) {
			t.Fatal("boss event identity or line changed")
		}
	}
	state.BossRows, state.BossRowsSHA256 = len(bosses), ggoRestoreDrillJSONDigest86(t, bosses)
	var lines []db.GGOFieldLine
	if err := database.Where("server_id=?", 1).Order("line").Find(&lines).Error; err != nil || len(lines) != 2 {
		t.Fatal("isolated public resource lines missing")
	}
	for index, row := range lines {
		var line ggoLineState
		if row.Line != int64(index+1) || row.Revision < 1 || json.Unmarshal(row.Payload, &line) != nil ||
			line.DeploymentRevision != "restore86-fixture" || len(line.Resources) == 0 || len(line.Monsters) == 0 {
			t.Fatal("isolated public resource line state invalid")
		}
	}
	state.LineRows, state.LineRowsSHA256 = len(lines), ggoRestoreDrillJSONDigest86(t, lines)
	var receipts []db.GGOFieldReceipt
	if err := database.Where("player_id IN ? AND operation=?", []int64{source.ActorID, source.OtherLineActorID}, constUtil.GGOFieldBossFightOperation).Order("player_id, request_id").Find(&receipts).Error; err != nil || len(receipts) < 2 {
		t.Fatal("settled boss receipts missing")
	}
	state.Receipts, state.ReceiptsSHA256 = len(receipts), ggoRestoreDrillJSONDigest86(t, receipts)
	var contributions []db.FieldBossContribution73
	if err := database.Where("server_id=? AND spawn_uuid IN ?", 1, []string{source.LineOneEvent, source.LineTwoEvent}).Order("id").Find(&contributions).Error; err != nil || len(contributions) < 2 {
		t.Fatal("isolated line contributions missing")
	}
	// Rules and chips are excluded by the model's public JSON tags, so bind them
	// explicitly without putting their bytes in the summary.
	var bound []struct {
		Value db.FieldBossContribution73 `json:"value"`
		Rules []byte                     `json:"rules"`
		Chips []byte                     `json:"chips"`
	}
	for _, row := range contributions {
		bound = append(bound, struct {
			Value db.FieldBossContribution73 `json:"value"`
			Rules []byte                     `json:"rules"`
			Chips []byte                     `json:"chips"`
		}{row, row.Rules, row.GGOChips})
	}
	state.Contributions, state.ContributionsSHA256 = len(contributions), ggoRestoreDrillJSONDigest86(t, bound)
	var bindings []db.ClientUpdateSession
	if err := database.Where("player_id IN ?", []int64{source.ActorID, source.OtherLineActorID}).Order("player_id").Find(&bindings).Error; err != nil || len(bindings) != 2 {
		t.Fatal("fixture session build bindings missing")
	}
	state.UpdateBindingsSHA = ggoRestoreDrillJSONDigest86(t, bindings)
	var balance db.RoleWalletBalance
	if err := database.Where("server_id=? AND player_id=?", 1, source.ActorID).Take(&balance).Error; err != nil || balance.BalanceCents != 12345 || balance.Revision < 1 {
		t.Fatal("isolated wallet balance missing")
	}
	var entries []db.RoleWalletEntry
	if err := database.Where("server_id=? AND player_id=?", 1, source.ActorID).Order("revision, `key`").Find(&entries).Error; err != nil || len(entries) < 1 {
		t.Fatal("isolated wallet ledger missing")
	}
	state.WalletBalanceSHA = ggoRestoreDrillJSONDigest86(t, balance)
	state.WalletEntries, state.WalletEntriesSHA = len(entries), ggoRestoreDrillJSONDigest86(t, entries)
	return state
}

func ggoRestoreDrillResumeCheck86(t *testing.T, database *gorm.DB, source ggoRestoreDrillSummary86, session []byte) {
	t.Helper()
	var actor db.Profile
	if err := database.First(&actor, source.ActorID).Error; err != nil || len(session) != 16 || subtle.ConstantTimeCompare(actor.Session, session) != 1 {
		t.Fatal("fixture actor or supplied session does not match")
	}
	var binding db.ClientUpdateSession
	if err := database.First(&binding, "player_id=?", source.ActorID).Error; err != nil ||
		binding.SessionHash != clientUpdateSessionHash(session) || binding.BuildCode < 1 {
		t.Fatal("fixture session is not bound to its build")
	}
	_, game, err := decodeProfile(&actor)
	if err != nil || game.Field == nil || game.Field.Fight == nil || game.Field.Fight.GGOBoss == nil ||
		game.Field.Fight.GGOBoss.Line != 1 || game.Field.Fight.GGOBoss.EventID != source.LineOneEvent {
		t.Fatal("restored active battle or frozen line differs")
	}
	activeReq := model.GGOFieldBattleResumeRequest{BattleType: constUtil.GGOBattleResumeBoss,
		ExpectedFightID: source.Active.FightID, ExpectedInputHash: source.Active.InputSHA256}
	active, err := ggoResumeActive(game, source.ActorID, activeReq, time.Now().UnixMilli())
	if err != nil || active.Status != constUtil.GGOBattleResumeActive || active.EventID != source.Active.EventID ||
		ggoRestoreDrillStartDigest86(t, active.NativeStart) != source.Active.NativeSHA {
		t.Fatal("active frozen start is not recoverable from the fixture")
	}
	settledReq := model.GGOFieldBattleResumeRequest{BattleType: constUtil.GGOBattleResumeBoss,
		ExpectedFightID: source.Settled.FightID, ExpectedInputHash: source.Settled.InputSHA256}
	settled, found, err := ggoResumeReceipt(database, source.ActorID, settledReq)
	if err != nil || !found || settled.Status != constUtil.GGOBattleResumeSettled || settled.EventID != source.Settled.EventID ||
		ggoRestoreDrillDigest86(settled.NativeEnd) != source.Settled.NativeSHA {
		t.Fatal("settled original response is not recoverable from the fixture")
	}
}

func TestGGORestoreDrill86Source(t *testing.T) {
	cfg := ggoRestoreDrillManaged86(t, true)
	dir := ggoRestoreDrillDir86(t)
	for _, name := range []string{"source.json", "session.bin", "backup.done", "verify.json"} {
		if _, err := os.Lstat(filepath.Join(dir, name)); !errors.Is(err, os.ErrNotExist) {
			t.Fatal("fixture directory must contain no previous drill files")
		}
	}
	waitSeconds := int64(180)
	if value := os.Getenv("HYJS_GGO_RESTORE_WAIT_SECONDS"); value != "" {
		parsed, err := strconv.ParseInt(value, 10, 64)
		if err != nil || parsed < 1 || parsed > 600 {
			t.Fatal("fixture wait must be between 1 and 600 seconds")
		}
		waitSeconds = parsed
	}
	f, ctx, session, first := ggoBossIntegrationFixture(t)
	// Do not schedule background notices while the external snapshot is taken.
	f.s.ggoStopNoticeTimers()
	if err := db.GetIns().Transaction(func(tx *gorm.DB) error {
		for _, line := range []int64{1, 2} {
			if _, _, _, err := ggoLoadLine(tx, f.s.OriginalServerID(), line, f.s.GGOFieldConfig, "restore86-fixture", time.Now().UnixMilli()); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		t.Fatal("cannot create isolated public resource line states")
	}
	// This is a finite local-demo credit in the owned test database; replay must
	// preserve one balance/ledger change before any backup is taken.
	credit, err := f.s.LocalWalletAdd(f.ctx, f.id, "restore86-fixture", 12345)
	if err != nil || credit == nil || credit.BalanceCents != 12345 {
		t.Fatal("cannot create isolated wallet ledger fixture")
	}
	replayedCredit, err := f.s.LocalWalletAdd(f.ctx, f.id, "restore86-fixture", 12345)
	if err != nil || replayedCredit == nil || !replayedCredit.AlreadyApplied || replayedCredit.BalanceCents != 12345 {
		t.Fatal("isolated wallet credit replay changed the balance")
	}
	var second *ggoBossEvent
	if err := db.GetIns().Transaction(func(tx *gorm.DB) error {
		_, event, _, _, err := f.s.ggoBossCurrent(tx, f.s.GGOFieldConfig, first.StartsAtMS+1, false, 2)
		second = event
		return err
	}); err != nil || second == nil || first.ID == second.ID || first.Line != 1 || second.Line != 2 {
		t.Fatal("cannot create distinct same-window line events")
	}
	peerID, peerCtx := ggoBossLines86Peer(t, f, ctx, second, 2)
	peerEnd := ggoBossLines86EndRequest(t, f, peerCtx, peerID, second.ID, second.Boss.HP)
	if messages, err := f.s.fieldBossFightEnd(peerCtx, peerID, peerEnd); err != nil || len(messages) == 0 || messages[0].(*pb.SCSidePveBossFightEnd).GetCode() != 1 {
		t.Fatal("isolated line-two settlement failed")
	}
	base := &ggoResume86Base{f: f, ctx: ctx, session: session, event: first}
	settled := ggoResume86SetupFromBase(t, constUtil.GGOBattleResumeBoss, base)
	end := ggoResume86EndRequest(t, settled)
	settledMessages, err := ggoResume86End(settled, end)
	if err != nil || len(settledMessages) == 0 {
		t.Fatal("isolated line-one settlement failed")
	}
	settledReq := settled.req
	settledResult, found, err := ggoResumeReceipt(db.GetIns(), f.id, settledReq)
	if err != nil || !found || settledResult.EventID != first.ID {
		t.Fatal("isolated settled receipt missing")
	}
	active := ggoResume86SetupFromBase(t, constUtil.GGOBattleResumeBoss, base)
	_, game := f.profile(t)
	activeResult, err := ggoResumeActive(game, f.id, active.req, time.Now().UnixMilli())
	if err != nil || activeResult.EventID != first.ID || active.req.ExpectedFightID == settledReq.ExpectedFightID {
		t.Fatal("isolated active fight did not retain line-one identity")
	}
	legacy, compressed := profileRollback86MySQLInput(t)
	compressedID, legacyID := int64(-862001), int64(-862002)
	profileRollback86MySQLInsert(t, db.GetIns(), compressedID, &compressed)
	profileRollback86MySQLInsert(t, db.GetIns(), legacyID, &legacy)
	source := ggoRestoreDrillSummary86{Schema: "hyjs-ggo-restore86-v1", SourceDatabase: cfg.Database,
		SourceUserSHA256: ggoRestoreDrillDigest86([]byte(cfg.User)), CreatedAtMS: time.Now().UnixMilli(), ActorID: f.id,
		OtherLineActorID: peerID, CompressedID: compressedID, LegacyID: legacyID, LineOneEvent: first.ID, LineTwoEvent: second.ID,
		Active: ggoRestoreDrillFight86{FightID: active.req.ExpectedFightID, InputSHA256: active.req.ExpectedInputHash,
			NativeSHA: ggoRestoreDrillStartDigest86(t, activeResult.NativeStart), EventID: activeResult.EventID},
		Settled: ggoRestoreDrillFight86{FightID: settledReq.ExpectedFightID, InputSHA256: settledReq.ExpectedInputHash,
			NativeSHA: ggoRestoreDrillDigest86(settledResult.NativeEnd), EventID: settledResult.EventID}}
	source.State = ggoRestoreDrillStateRead86(t, db.GetIns(), source)
	ggoRestoreDrillResumeCheck86(t, db.GetIns(), source, session)
	file, err := json.Marshal(source)
	if err != nil {
		t.Fatal("cannot encode isolated source summary")
	}
	ggoRestoreDrillWrite86(t, filepath.Join(dir, "session.bin"), session)
	ggoRestoreDrillWrite86(t, filepath.Join(dir, "source.json"), file)
	t.Log("source fixture ready; waiting for externally authenticated backup acknowledgement")
	deadline := time.NewTimer(time.Duration(waitSeconds) * time.Second)
	defer deadline.Stop()
	ticker := time.NewTicker(200 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-t.Context().Done():
			t.Fatal("fixture cancelled while waiting for backup")
		case <-deadline.C:
			t.Fatal("fixture backup acknowledgement timed out")
		case <-ticker.C:
			path := filepath.Join(dir, "backup.done")
			if _, err := os.Lstat(path); errors.Is(err, os.ErrNotExist) {
				continue
			} else if err != nil {
				t.Fatal("cannot inspect backup acknowledgement")
			}
			var ack ggoRestoreDrillAck86
			if err := json.Unmarshal(ggoRestoreDrillPrivateFile86(t, path), &ack); err != nil || ack.Status != "authenticated" ||
				ack.SourceSHA256 != ggoRestoreDrillDigest86(file) || !ggoResumeHex(ack.BackupSHA256, 64) {
				t.Fatal("invalid backup acknowledgement")
			}
			if current := ggoRestoreDrillStateRead86(t, db.GetIns(), source); !reflect.DeepEqual(current, source.State) {
				t.Fatal("source fixture changed while backup was in progress")
			}
			return
		}
	}
}

func TestGGORestoreDrill86Verify(t *testing.T) {
	cfg := ggoRestoreDrillManaged86(t, false)
	dir := ggoRestoreDrillDir86(t)
	raw := ggoRestoreDrillPrivateFile86(t, filepath.Join(dir, "source.json"))
	var source ggoRestoreDrillSummary86
	if err := json.Unmarshal(raw, &source); err != nil || source.Schema != "hyjs-ggo-restore86-v1" ||
		source.SourceDatabase == cfg.Database || source.SourceUserSHA256 == ggoRestoreDrillDigest86([]byte(cfg.User)) ||
		source.ActorID < 1 || source.OtherLineActorID < 1 || source.ActorID == source.OtherLineActorID ||
		!ggoResumeHex(source.Active.FightID, 24) || !ggoResumeHex(source.Settled.FightID, 24) {
		t.Fatal("source summary or independent target identity is invalid")
	}
	session := ggoRestoreDrillPrivateFile86(t, filepath.Join(dir, "session.bin"))
	database, err := db.OpenGameMySQL(t.Context(), cfg, false)
	if err != nil {
		t.Fatal("cannot open restored target without migration")
	}
	connection, err := database.DB()
	if err != nil {
		t.Fatal("cannot obtain target connection")
	}
	defer connection.Close()
	var selected string
	if err = database.Raw("SELECT DATABASE()").Scan(&selected).Error; err != nil || selected != cfg.Database {
		t.Fatal("selected target differs from managed allocation")
	}
	if actual := ggoRestoreDrillStateRead86(t, database, source); !reflect.DeepEqual(actual, source.State) {
		t.Fatal("restored target state differs from the source summary")
	}
	ggoRestoreDrillResumeCheck86(t, database, source, session)
	result, err := json.Marshal(struct {
		Schema       string `json:"schema"`
		SourceSHA256 string `json:"sourceSHA256"`
		Target       string `json:"targetDatabase"`
		Scope        string `json:"scope"`
		Passed       bool   `json:"passed"`
	}{"hyjs-ggo-restore86-verify-v1", ggoRestoreDrillDigest86(raw), cfg.Database,
		"database bytes and in-process resume helpers; no game process or HTTP/TCP", true})
	if err != nil {
		t.Fatal("cannot encode target verification")
	}
	ggoRestoreDrillWrite86(t, filepath.Join(dir, "verify.json"), result)
}
