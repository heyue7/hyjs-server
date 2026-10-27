package main

import (
	"flag"
	"io"
	"os"
	"testing"
)

// 冲突命令在加载配置、连接数据库或运行任何维护动作前拒绝。
func TestFeedback78RejectsOtherMaintenanceCommands(t *testing.T) {
	previousArgs, previousFlags := os.Args, flag.CommandLine
	t.Cleanup(func() { os.Args, flag.CommandLine = previousArgs, previousFlags })
	for _, other := range [][]string{
		{"--apply-growth74"},
		{"--apply-growth75"},
		{"--apply-activity77"},
		{"--restore-vip10-book"},
		{"--warm-rush-history-player", "1"},
		{"--preview-activity-calendar"},
		{"--restore-activity-calendar", "request"},
		{"--calendar-maintenance", "begin"},
	} {
		os.Args = append([]string{"hyjs-server", "--apply-feedback78"}, other...)
		flag.CommandLine = flag.NewFlagSet("feedback78-test", flag.ContinueOnError)
		flag.CommandLine.SetOutput(io.Discard)
		if err := run(); err == nil || err.Error() != "每次只执行一个维护命令" {
			t.Fatalf("maintenance command %v was not rejected: %v", other, err)
		}
	}
}
