package service

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"gorm.io/gorm"
	"hyjs-server/config"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

// The snapshot contains game facts only. Account credentials and certificate
// keys are always produced by this isolated fixture and its caller.
var trialBossSnapshotColumns = map[string]string{
	"p9_pet_instances":              "created_at data guid pet_id player_id source",
	"p6_battle_records":             "authoritative client_result client_version configuration_hash created_at engine_version expires_at finished_at id input input_hash operates outcome outcome_hash p13_match_id player_id replay_hash seed server_result source source_fight status",
	"p7_re_rush_policies":           "enabled generation_json id published_at source version",
	"p7_re_rush_power_proofs":       "floor player_id run_id snapshot snapshot_hash",
	"p7_re_rush_sections":           "acceptance_hash claimed options_json player_id run_id section selected spent_stars",
	"p7_re_rush_sweeps":             "floor player_id run_id source_fight source_hash star",
	"p7_rush_addition_policies":     "enabled end_at id nine_option published_at six_option source start_at three_option version",
	"p7_rush_continuation_policies": "enabled first_floor id last_floor source version",
	"p7_rush_fights":                "day diagnostic end_request id kind player_id replay_id result rule rule_hash run_id start start_hash status task_id",
	"p7_rush_manual_calculations":   "accepted_at created_at derived_calculation_source_sha256 driver_policy end_request_hash fight_id p6_record_id player_id preview preview_hash scope_proof series series_hash start_hash status terminal terminal_hash",
	"p7_rush_manual_policies":       "enabled id source",
	"p7_rush_policies":              "elite_buy_cost enabled id source trial_reset_cost",
	"p7_rush_progress":              "first_fight kind player_id star task_id",
	"p7_rush_scoped_calculations":   "created_at fight_id p6_record_id player_id scope_proof series series_hash start_hash terminal terminal_hash",
	"p7_rush_scoped_policies":       "enabled id source",
	"p7_rush_stage_rules":           "condition_task_id enabled enemies first_reward floor id kind max_round min_level recommended_power reward scene source star task_id win_key win_value",
	"p7_rush_states":                "active_fight addition_offer_section day elite_bought elite_used floor free_reset_used high_floor paid_reset_used player_id record_stars run_id total_stars",
	"p7_section_reward_policies":    "enabled end_at id nine_reward published_at section_from section_to six_reward source start_at three_reward version",
}

var trialBossSnapshotGlobal = []string{
	"p7_rush_policies", "p7_rush_scoped_policies", "p7_rush_manual_policies",
	"p7_re_rush_policies", "p7_rush_addition_policies", "p7_rush_continuation_policies",
	"p7_section_reward_policies", "p7_rush_stage_rules",
}

var trialBossSnapshotOwned = []string{
	"p9_pet_instances",
	"p7_rush_states", "p7_rush_progress", "p7_rush_fights", "p7_re_rush_sweeps",
	"p7_re_rush_power_proofs", "p7_re_rush_sections", "p6_battle_records",
	"p7_rush_scoped_calculations", "p7_rush_manual_calculations",
}

func trialBossSnapshotValue(table, column string, value any) (any, error) {
	switch v := value.(type) {
	case nil, string, bool:
		return v, nil
	case json.Number:
		if n, err := v.Int64(); err == nil {
			return n, nil
		}
		if table == "p7_rush_stage_rules" && column == "recommended_power" {
			f, err := v.Float64()
			if err == nil && !math.IsNaN(f) && !math.IsInf(f, 0) {
				return f, nil
			}
		}
	case map[string]any:
		if len(v) == 1 {
			h, ok := v["hex"]
			if ok && h == nil {
				return nil, nil
			}
			if text, ok := h.(string); ok {
				return hex.DecodeString(text)
			}
		}
	}
	return nil, errors.New("invalid snapshot value")
}

func trialBossSnapshotRows(raw []byte) (map[string][]map[string]any, error) {
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	var input map[string][]map[string]any
	if err := dec.Decode(&input); err != nil {
		return nil, err
	}
	for table, rows := range input {
		columns, ok := trialBossSnapshotColumns[table]
		if !ok {
			return nil, fmt.Errorf("unapproved snapshot table: %s", table)
		}
		allowed := map[string]bool{}
		for _, name := range strings.Fields(columns) {
			allowed[name] = true
		}
		for _, row := range rows {
			if len(row) != len(allowed) {
				return nil, fmt.Errorf("snapshot column count differs: %s", table)
			}
			for column, value := range row {
				if !allowed[column] {
					return nil, fmt.Errorf("unapproved snapshot column: %s.%s", table, column)
				}
				parsed, err := trialBossSnapshotValue(table, column, value)
				if err != nil {
					return nil, fmt.Errorf("invalid snapshot column: %s.%s", table, column)
				}
				row[column] = parsed
			}
			if allowed["player_id"] && row["player_id"] != int64(62) {
				return nil, fmt.Errorf("snapshot owner mismatch: %s", table)
			}
		}
	}
	for _, table := range trialBossSnapshotGlobal {
		if len(input[table]) == 0 {
			return nil, fmt.Errorf("required snapshot table empty: %s", table)
		}
	}
	if len(input["p7_rush_states"]) != 1 {
		return nil, errors.New("snapshot must have one trial state")
	}
	return input, nil
}

func TestTrialBossSnapshotPhoneFixture(t *testing.T) {
	if os.Getenv("HYJS_TRIAL_BOSS_SNAPSHOT") != "1" {
		t.Skip("isolated trial snapshot fixture not requested")
	}
	name, source, output := os.Getenv("HYJS_P7_MYSQL_DATABASE"), os.Getenv("HYJS_TRIAL_BOSS_SOURCE"), os.Getenv("HYJS_TRIAL82_PHONE_FIXTURE_FILE")
	prefix := "hyjs_test_p1_activity77_phone_82_"
	if !strings.HasPrefix(name, prefix) || len(name) <= len(prefix) || len(name) > 64 || !regexp.MustCompile(`^[a-z0-9_]+$`).MatchString(name) || !filepath.IsAbs(source) || !filepath.IsAbs(output) {
		t.Fatal("fixture requires a dedicated isolated MySQL schema and absolute private paths")
	}
	if _, err := os.Lstat(output); !os.IsNotExist(err) {
		t.Fatal("private fixture output must not already exist")
	}
	key := os.Getenv("HYJS_RUSH_HISTORY_CERTIFICATE_KEY")
	if decoded, err := hex.DecodeString(key); err != nil || len(decoded) != 32 {
		t.Fatal("fixture requires a 64-hex isolated history certificate key")
	}
	if config.AppConfigIns != nil && config.AppConfigIns.Game.AccountsDatabase != "" || os.Getenv("HYJS_GAME_ACCOUNTS_DATABASE") != "" {
		t.Fatal("shared accounts schema prohibited")
	}
	var sourceRows []map[string]json.RawMessage
	raw, err := os.ReadFile(source)
	if err != nil || json.Unmarshal(raw, &sourceRows) != nil {
		t.Fatal("cannot read source snapshot")
	}
	var playerRaw, gameRaw []byte
	for _, row := range sourceRows {
		if row["playerHex"] == nil {
			continue
		}
		var id int64
		var playerHex, gameHex string
		if playerRaw != nil || json.Unmarshal(row["id"], &id) != nil || id != 62 || json.Unmarshal(row["playerHex"], &playerHex) != nil || json.Unmarshal(row["gameHex"], &gameHex) != nil {
			t.Fatal("invalid source profile")
		}
		playerRaw, err = hex.DecodeString(playerHex)
		if err != nil {
			t.Fatal("invalid source player encoding")
		}
		gameRaw, err = hex.DecodeString(gameHex)
		if err != nil {
			t.Fatal("invalid source game encoding")
		}
	}
	var player pb.Player
	var game model.GameState
	if json.Unmarshal(playerRaw, &player) != nil || json.Unmarshal(gameRaw, &game) != nil || player.GetPlayerId() != 62 || game.Position == nil || game.Position.GetPlayerId() != 62 {
		t.Fatal("snapshot player identity is inconsistent")
	}
	var resource struct {
		Profile struct {
			GGOCredit         int64  `json:"ggoCredit"`
			GGOCreditMigrated int64  `json:"ggoCreditMigrated"`
			GGOChipTotal      *int64 `json:"ggoChipTotal"`
		} `json:"profile"`
		Wallet struct {
			ServerID, BalanceCents, Revision int64
		} `json:"wallet"`
	}
	resourceRaw, err := os.ReadFile(filepath.Join(filepath.Dir(source), "resource-source.json"))
	if err != nil || json.Unmarshal(resourceRaw, &resource) != nil || resource.Profile.GGOCredit < 0 || resource.Profile.GGOCreditMigrated < 0 || resource.Profile.GGOCreditMigrated > 1 || resource.Wallet.ServerID != 1 || resource.Wallet.BalanceCents < 0 || resource.Wallet.Revision < 0 {
		t.Fatal("invalid source resource snapshot")
	}
	historyRaw, err := os.ReadFile(filepath.Join(filepath.Dir(source), "history.json"))
	if err != nil {
		t.Fatal("cannot read trial history snapshot")
	}
	history, err := trialBossSnapshotRows(historyRaw)
	if err != nil {
		t.Fatal(err)
	}

	oldConfig := config.AppConfigIns
	var localConfig config.AppConfig
	if oldConfig != nil {
		localConfig = *oldConfig
	}
	localConfig.Game.AccountsDatabase = ""
	localConfig.Game.ServerID = 1
	localConfig.Security.RushHistoryCertificateKey = key
	config.AppConfigIns = &localConfig
	t.Cleanup(func() { config.AppConfigIns = oldConfig })
	for _, suffix := range []string{"HOST", "USER", "PASSWORD", "DATABASE"} {
		t.Setenv("HYJS_P1_MYSQL_"+suffix, os.Getenv("HYJS_P7_MYSQL_"+suffix))
	}
	private := filepath.Join(t.TempDir(), "trial82-private.json")
	t.Setenv("HYJS_TRIAL82_PHONE_FIXTURE_FILE", private)
	TestTrial82PhoneFixture(t)
	var login struct {
		Username  string `json:"username"`
		Password  string `json:"password"`
		PlayerID  int64  `json:"playerId"`
		AccountID int64  `json:"accountId"`
		ServerID  int64  `json:"serverId"`
		Schema    string `json:"schema"`
		Fixture   string `json:"fixture"`
	}
	privateRaw, err := os.ReadFile(private)
	if err != nil || json.Unmarshal(privateRaw, &login) != nil || login.PlayerID != 2 || login.AccountID != 2 || login.ServerID != 1 || login.Schema != name || login.Username == "" || login.Password == "" {
		t.Fatal("isolated account fixture is invalid")
	}
	ctx := context.Background()
	if err = initializeAccountWalletSchema(ctx); err != nil {
		t.Fatal("isolated account wallet schema", err)
	}
	if err = initializeRoleWalletSchema(ctx); err != nil {
		t.Fatal("isolated role wallet schema", err)
	}
	var currentDB string
	if err = db.GetIns().Raw("SELECT DATABASE()").Scan(&currentDB).Error; err != nil || currentDB != name || db.AccountsTableName("game_accounts") != "game_accounts" {
		t.Fatal("isolated database identity mismatch")
	}
	err = db.GetIns().Transaction(func(tx *gorm.DB) error {
		var occupied int64
		if e := tx.Model(&db.Profile{}).Where("id=?", 62).Count(&occupied).Error; e != nil || occupied != 0 {
			return errors.New("target profile is occupied")
		}
		var profile db.Profile
		if e := tx.First(&profile, 2).Error; e != nil || profile.AccountID != login.AccountID || profile.Uid != login.Username || profile.ServerID != 1 {
			return errors.New("fixture profile ownership mismatch")
		}
		changes := map[string]any{"id": int64(62), "player": string(playerRaw), "game": string(gameRaw), "ggo_credit": resource.Profile.GGOCredit, "ggo_credit_migrated": resource.Profile.GGOCreditMigrated != 0, "ggo_chip_total": resource.Profile.GGOChipTotal}
		updated := tx.Model(&db.Profile{}).Where("id=? AND account_id=?", 2, login.AccountID).Updates(changes)
		if updated.Error != nil || updated.RowsAffected != 1 {
			return errors.New("cannot move isolated profile")
		}
		updated = db.AccountsTable(tx, "game_account_roles").Where("account_id=? AND server_id=? AND player_id=?", login.AccountID, 1, 2).Updates(map[string]any{"player_id": int64(62), "name": player.GetName(), "level": player.GetLevel()})
		if updated.Error != nil || updated.RowsAffected != 1 {
			return errors.New("cannot move isolated role index")
		}
		updated = tx.Model(&db.PlayerOpeningCalendar{}).Where("player_id=?", 2).Update("player_id", 62)
		if updated.Error != nil || updated.RowsAffected != 1 {
			return errors.New("cannot move isolated opening calendar")
		}
		if e := db.AccountsTable(tx, "role_wallet_balances").Where("server_id=? AND player_id=?", 1, 62).Count(&occupied).Error; e != nil || occupied != 0 {
			return errors.New("target role wallet is occupied")
		}
		wallet := db.RoleWalletBalance{ServerID: 1, PlayerID: 62, AccountID: login.AccountID, BalanceCents: resource.Wallet.BalanceCents, Revision: resource.Wallet.Revision}
		if e := db.AccountsTable(tx, "role_wallet_balances").Create(&wallet).Error; e != nil {
			return errors.New("cannot create source-balance role wallet")
		}
		for _, table := range trialBossSnapshotGlobal {
			if e := tx.Exec("DELETE FROM `" + table + "`").Error; e != nil {
				return fmt.Errorf("cannot replace isolated table %s", table)
			}
		}
		for _, table := range trialBossSnapshotOwned {
			if e := tx.Table(table).Where("player_id=?", 62).Count(&occupied).Error; e != nil || occupied != 0 {
				return fmt.Errorf("target trial table occupied: %s", table)
			}
		}
		for _, table := range append(append([]string{}, trialBossSnapshotGlobal...), trialBossSnapshotOwned...) {
			for _, row := range history[table] {
				if e := tx.Table(table).Create(row).Error; e != nil {
					return fmt.Errorf("cannot import isolated table %s: %w", table, e)
				}
			}
		}
		var moved db.Profile
		if e := tx.First(&moved, 62).Error; e != nil {
			return e
		}
		return syncGlobalAccountRole(tx, &moved)
	})
	if err != nil {
		t.Fatal("isolated snapshot transaction", err)
	}
	p7InitializeDurableHistoryTest(t, ctx)
	s := &GameService{ServerID: 1}
	for _, load := range []func(string) error{s.LoadTables, s.LoadP5Tables, s.LoadP7Tables, s.LoadP8Tables, s.LoadP9Tables, s.LoadP13GuildTables} {
		if err = load("../../analysis/tables"); err != nil {
			t.Fatal(err)
		}
	}
	s.InstallGrowth74Resources()
	if err = s.InstallGGOItems(); err != nil {
		t.Fatal(err)
	}
	if err = s.InitializeServerGameClock(ctx); err != nil {
		t.Fatal(err)
	}
	ctx = context.WithValue(ctx, constUtil.GameServiceContextKey, s)
	profile, err := LoginAccount(ctx, login.Username, login.Password)
	if err != nil || profile.Id != 62 || profile.AccountID != login.AccountID {
		t.Fatal("isolated login did not resolve snapshot role", err)
	}
	version := "hyjs/2026110401/00000000000000000000000000000001/1.1.10"
	if err = s.CheckClientLogin(ctx, version); err != nil {
		t.Fatal("isolated client version gate", err)
	}
	if err = s.BindClientLogin(ctx, profile, version); err != nil {
		t.Fatal("isolated client session binding", err)
	}
	for _, selected := range []string{"", "11", "11"} {
		start := time.Now()
		page, e := s.P7ElitePage(ctx, 62, profile.Session, nil, 5, selected, "")
		if e != nil {
			t.Fatal("isolated elite page query", e)
		}
		packet := p7DecodeElitePage(t, page)
		if selected == "11" && (len(packet.BossInfoList) != 1 || packet.BossInfoList[0].GetIsOpen() != 1) {
			if e := db.GetIns().Transaction(func(tx *gorm.DB) error {
				var state db.P7RushState
				var stage db.P7RushStageRule
				var current db.Profile
				for _, query := range []*gorm.DB{tx.First(&state, "player_id=?", 62), tx.First(&stage, "kind=? AND task_id=?", 2, "11"), tx.First(&current, 62)} {
					if query.Error != nil {
						return query.Error
					}
				}
				var p pb.Player
				var g model.GameState
				if e := json.Unmarshal([]byte(*current.Player), &p); e != nil {
					return e
				}
				if e := json.Unmarshal([]byte(*current.Game), &g); e != nil {
					return e
				}
				q, e := p7NewEliteQueryProjection(tx, &state)
				if e != nil {
					return e
				}
				q.serverID = 1
				passed, prerequisiteErr := q.prerequisite(s, stage)
				_, enemyErr := s.validateP7Stage(stage)
				allowed, scopeErr := q.scopeAvailable(s, ctx, &p, &g, stage, time.Now())
				t.Logf("closed-source diagnostic: level=%d required=%d enabled=%t prerequisite=%t prerequisiteErr=%v enemyErr=%v scope=%t scopeErr=%v baseErr=%v reErr=%v", p.GetLevel(), stage.MinLevel, stage.Enabled, passed, prerequisiteErr, enemyErr, allowed, scopeErr, q.baseErr, q.reErr)
				return nil
			}); e != nil {
				t.Log("closed-source diagnostic error", e)
			}
			t.Fatal("source boss 11 did not pass its original prerequisites and scope")
		}
		t.Logf("isolated elite page selected=%q rows=%d pending=%d elapsed=%s", selected, len(packet.BossInfoList), len(page.PendingIDs), time.Since(start))
	}
	login.PlayerID = 62
	login.Fixture = "isolated owner62 trial snapshot; original P7 rules and historical sources"
	encoded, err := json.Marshal(login)
	if err != nil {
		t.Fatal("cannot encode private fixture")
	}
	file, err := os.OpenFile(output, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		t.Fatal("cannot create private fixture")
	}
	if _, err = file.Write(encoded); err != nil {
		_ = file.Close()
		_ = os.Remove(output)
		t.Fatal("cannot write private fixture")
	}
	if err = file.Close(); err != nil {
		_ = os.Remove(output)
		t.Fatal("cannot finish private fixture")
	}
}
