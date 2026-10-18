package config

import (
	"os"
	"path/filepath"
	"testing"
)

func isolateConfiguration(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	t.Chdir(dir)
	for _, name := range environmentBindings {
		t.Setenv(name, "")
		// Unset models an absent override; AllowEmptyEnv is tested separately.
		if err := os.Unsetenv(name); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("HYJS_CONFIG_FILE", "")
	var (
		previous    = AppConfigIns
		previousEnv = Env
	)
	t.Cleanup(func() { AppConfigIns, Env = previous, previousEnv })
	return dir
}

func writeConfiguration(t *testing.T, name, content string) {
	t.Helper()
	if err := os.WriteFile(name, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
}

func TestInitEnvironmentOverridesAndExplicitFile(t *testing.T) {
	dir := isolateConfiguration(t)
	privateFile := filepath.Join(dir, "private.yaml")
	writeConfiguration(t, privateFile, "game:\n  serverId: 1\n  advertiseHost: yaml.invalid\n  httpPort: 18088\nmysql:\n  pass: yaml-placeholder\n")
	t.Setenv("HYJS_CONFIG_FILE", privateFile)
	t.Setenv("HYJS_GAME_ADVERTISE_HOST", "env.invalid")
	t.Setenv("HYJS_GAME_HTTP_PORT", "28080")
	t.Setenv("HYJS_GAME_TCP_PORT", "28081")
	t.Setenv("HYJS_GAME_ENABLE_LOCAL_WALLET", "true")
	t.Setenv("HYJS_MYSQL_PASSWORD", "")
	t.Setenv("HYJS_JWT_KEY", "unit-test-placeholder")
	if err := Init(" TEST "); err != nil {
		t.Fatal(err)
	}
	cfg := AppConfigIns
	if Env != "test" || cfg.Game.AdvertiseHost != "env.invalid" || cfg.Game.HTTPPort != 28080 || cfg.Game.TCPPort != 28081 || !cfg.Game.EnableLocalWallet || cfg.Game.Driver != "mysql" || cfg.MysqlConfig.Pass != "" || cfg.JwtKey != "unit-test-placeholder" {
		t.Fatal("private config and environment precedence did not apply")
	}
}

func TestInitEnvironmentFileFallbackAndIsolation(t *testing.T) {
	isolateConfiguration(t)
	writeConfiguration(t, "config.yaml", "game:\n  serverId: 2\nmysql:\n  user: fallback\n")
	writeConfiguration(t, "config.dev.yaml", "game:\n  serverId: 3\nmysql:\n  pass: dev-placeholder\n")
	if err := Init(""); err != nil {
		t.Fatal(err)
	}
	if AppConfigIns.Game.ServerID != 3 || AppConfigIns.MysqlConfig.Pass == "" {
		t.Fatal("environment config was not selected")
	}
	if err := Init("test"); err != nil {
		t.Fatal(err)
	}
	if AppConfigIns.Game.ServerID != 2 || AppConfigIns.MysqlConfig.User != "fallback" || AppConfigIns.MysqlConfig.Pass != "" {
		t.Fatal("fallback inherited state from the previous environment")
	}
	t.Setenv("HYJS_CONFIG_FILE", "missing-private.yaml")
	if err := Init("test"); err == nil {
		t.Fatal("an explicit missing private file must fail without fallback")
	}
}

func TestInitRejectsInvalidEnvironmentTypes(t *testing.T) {
	isolateConfiguration(t)
	writeConfiguration(t, "config.yaml", "game:\n  serverId: 1\n")
	t.Setenv("HYJS_GAME_HTTP_PORT", "invalid")
	if err := Init("test"); err == nil {
		t.Fatal("invalid port must fail before starting the server")
	}
}
