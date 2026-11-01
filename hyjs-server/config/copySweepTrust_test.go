package config

import "testing"

func TestCopySweepHistoryCompleteRequiresMatchingRealm(t *testing.T) {
	for _, tc := range []struct {
		value   string
		server  int64
		enabled bool
		bad     bool
	}{
		{"", 1, false, false},
		{"0", 1, false, false},
		{"1", 1, true, false},
		{"2", 1, false, true},
		{"-1", 1, false, true},
		{"01", 1, false, true},
		{"true", 1, false, true},
	} {
		t.Setenv("HYJS_COPY_SWEEP_P9_COMPLETE_SERVER", tc.value)
		enabled, err := CopySweepHistoryComplete(tc.server)
		if enabled != tc.enabled || (err != nil) != tc.bad {
			t.Fatalf("value=%q server=%d: enabled=%v error=%v", tc.value, tc.server, enabled, err)
		}
	}
}
