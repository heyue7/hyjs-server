package service

import (
	"encoding/json"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"testing"
)

func TestP6NativeGoldenCacheMemoryFootprint(t *testing.T) {
	path := os.Getenv("HYJS_P6_NATIVE_MEMORY_REPORT")
	if path == "" {
		t.Skip("explicit full native-corpus memory measurement disabled")
	}
	expectedCases := 0
	for _, fixture := range p6NativeGoldenClockFixtures() {
		count := 14
		if fixture[0] == "native-damage-factor-tick-golden-v51.json" {
			count = 4
		}
		expectedCases += count
		t.Run(fixture[0], func(t *testing.T) { p6NativeAutomaticPVPOriginalTickGolden(t, fixture[0], fixture[1], count) })
	}
	runtime.GC()
	var stats runtime.MemStats
	runtime.ReadMemStats(&stats)
	var usage syscall.Rusage
	err := syscall.Getrusage(syscall.RUSAGE_SELF, &usage)
	if err != nil {
		t.Fatal(err)
	}
	maxRSS := usage.Maxrss
	if runtime.GOOS != "darwin" {
		maxRSS *= 1024
	}
	liveRSS := int64(0)
	if raw, err := exec.Command("rtk", "proxy", "ps", "-o", "rss=", "-p", strconv.Itoa(os.Getpid())).Output(); err == nil {
		liveRSS, _ = strconv.ParseInt(strings.TrimSpace(string(raw)), 10, 64)
		liveRSS *= 1024
	}
	p6NativeGoldenCache.Lock()
	cases, ticks, actors, buffs, events, calcs, hits := 0, 0, 0, 0, 0, 0, 0
	for _, row := range p6NativeGoldenCache.rows {
		cases++
		ticks += len(row.ticks)
		events += len(row.outcome.Events)
		calcs += row.calculations
		hits += row.hits
		for _, tick := range row.ticks {
			for _, player := range tick.players {
				for _, actor := range player.actors {
					actors++
					buffs += len(actor.buffs) + len(actor.outgoingReduction)
				}
			}
		}
	}
	p6NativeGoldenCache.Unlock()
	report := map[string]any{"scope": "complete original core/RNG/HP/AP/operations/winner assertions, one actual observed simulation per PB; retained immutable native-clock snapshots after forcedGC", "nativeCases": cases, "actualCalculations": calcs, "cacheHits": hits, "originalClockSnapshots": ticks, "actorSnapshots": actors, "buffSnapshots": buffs, "auditEvents": events, "liveHeapAfterGCBytes": stats.HeapAlloc, "heapInUseBytes": stats.HeapInuse, "heapSysBytes": stats.HeapSys, "runtimeSysBytes": stats.Sys, "maxRSSBytes": maxRSS, "liveRSSBytes": liveRSS, "numGC": stats.NumGC, "goMemoryLimit": "2GiB", "goVersion": runtime.Version(), "authoritative": false, "nativeCompared": false}
	raw, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(path, append(raw, '\n'), 0600); err != nil {
		t.Fatal(err)
	}
	if cases != expectedCases || calcs != expectedCases {
		t.Fatal("memory corpus requires every SHA-pinned current original case", expectedCases, cases, calcs)
	}
	t.Log(string(raw))
}
