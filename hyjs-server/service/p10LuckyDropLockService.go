package service

import (
	"database/sql"
	"sync"
)

type p10LuckyDropKey struct {
	database *sql.DB
	scene    string
	line     int32
}

type p10LuckyDropEntry struct {
	mutex sync.Mutex
	refs  int
}

var p10LuckyDropRegistry = struct {
	sync.Mutex
	entries map[p10LuckyDropKey]*p10LuckyDropEntry
}{entries: make(map[p10LuckyDropKey]*p10LuckyDropEntry)}

// p10LuckyDropLock serializes only concurrent throws in the same database and line.
func p10LuckyDropLock(key p10LuckyDropKey) func() {
	p10LuckyDropRegistry.Lock()
	entry := p10LuckyDropRegistry.entries[key]
	if entry == nil {
		entry = &p10LuckyDropEntry{}
		p10LuckyDropRegistry.entries[key] = entry
	}
	entry.refs++
	p10LuckyDropRegistry.Unlock()
	entry.mutex.Lock()
	return func() {
		entry.mutex.Unlock()
		p10LuckyDropRegistry.Lock()
		entry.refs--
		if entry.refs == 0 {
			delete(p10LuckyDropRegistry.entries, key)
		}
		p10LuckyDropRegistry.Unlock()
	}
}
