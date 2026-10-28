package main

import "testing"

func TestGGOIntegrationRefusesNonIsolatedEnvironments(t *testing.T) {
	valid := []struct{ env, bind, driver, host, database, accounts string }{
		{"test", "127.0.0.1", "mysql", "127.0.0.1:3306", "hyjs_test_desert_1", ""},
		{"dev", "::1", "mysql", "[::1]:3306", "hyjs_test_desert_2", "hyjs_test_desert_2"},
	}
	for _, c := range valid {
		if err := validateGGOIntegration(c.env, c.bind, c.driver, c.host, c.database, c.accounts); err != nil {
			t.Fatal(c, err)
		}
	}
	invalid := []struct{ env, bind, driver, host, database, accounts string }{
		{"prd", "127.0.0.1", "mysql", "127.0.0.1:3306", "hyjs_test_desert_1", ""},
		{"test", "0.0.0.0", "mysql", "127.0.0.1:3306", "hyjs_test_desert_1", ""},
		{"test", "localhost", "mysql", "127.0.0.1:3306", "hyjs_test_desert_1", ""},
		{"test", "127.0.0.1", "sqlite", "127.0.0.1:3306", "hyjs_test_desert_1", ""},
		{"test", "127.0.0.1", "mysql", "10.0.0.4:3306", "hyjs_test_desert_1", ""},
		{"test", "127.0.0.1", "mysql", "127.0.0.1:3306", "hyjs", ""},
		{"test", "127.0.0.1", "mysql", "127.0.0.1:3306", "hyjs_test_", ""},
		{"test", "127.0.0.1", "mysql", "127.0.0.1:3306", "hyjs_test_desert_1", "hyjs"},
	}
	for _, c := range invalid {
		if err := validateGGOIntegration(c.env, c.bind, c.driver, c.host, c.database, c.accounts); err == nil {
			t.Fatal("unsafe environment accepted", c)
		}
	}
}
