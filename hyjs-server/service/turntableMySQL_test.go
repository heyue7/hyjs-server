package service

import (
	"os"
	"strings"
	"testing"
)

// TestMySQLTurntableTransactions validates the approved personal wheel using the same real consumers.
func TestMySQLTurntableTransactions(t *testing.T) {
	name := os.Getenv("HYJS_MYSQL_TEST_DATABASE")
	if name == "" {
		t.Skip("isolated MySQL test database required")
	}
	if !strings.HasPrefix(name, "hyjs_test_p1_") {
		t.Fatal("personal wheel representative requires managed hyjs_test_p1_ database")
	}
	t.Setenv("HYJS_P1_MYSQL_DATABASE", name)
	for _, key := range []string{"HOST", "USER", "PASSWORD"} {
		t.Setenv("HYJS_P1_MYSQL_"+key, os.Getenv("HYJS_MYSQL_"+key))
	}
	TestPersonalTurntableTwoOwnersInventoryJackpotAndColdReceipt(t)
}
