package service

import (
	"bytes"
	"context"

	"hyjs-server/db"
	"hyjs-server/model"
)

// A scope is confined to one synchronous request chain. Keeping only its last
// verified input bounds retention during arena daily processing and sweeps.
type p6InputReuseScopeKey struct{}

type p6InputReuseScope struct {
	last *p6InputReuseEntry
}

type p6InputReuseEntry struct {
	id                string
	playerID          int64
	seed              int64
	inputHash         string
	configurationHash string
	engineVersion     string
	clientVersion     string
	storedInput       []byte
	input             *model.FrozenBattleInput
}

func withP6InputReuseScope(ctx context.Context) context.Context {
	if ctx == nil {
		return nil
	}
	if scope, ok := ctx.Value(p6InputReuseScopeKey{}).(*p6InputReuseScope); ok && scope != nil {
		return ctx
	}
	return context.WithValue(ctx, p6InputReuseScopeKey{}, &p6InputReuseScope{})
}

// Only reviewed read-only P11 consumers use this helper. The public decoder
// continues to return an independent value for all other consumers.
func decodeP6RecordScoped(ctx context.Context, record *db.P6BattleRecord) (*model.FrozenBattleInput, error) {
	if ctx == nil || record == nil {
		return decodeP6Record(record)
	}
	scope, _ := ctx.Value(p6InputReuseScopeKey{}).(*p6InputReuseScope)
	if scope == nil {
		return decodeP6Record(record)
	}
	last := scope.last
	if last != nil && last.id == record.ID && last.playerID == record.PlayerID && last.seed == record.Seed && last.inputHash == record.InputHash && last.configurationHash == record.ConfigurationHash && last.engineVersion == record.EngineVersion && last.clientVersion == record.ClientVersion && bytes.Equal(last.storedInput, record.Input) {
		return last.input, nil
	}
	input, err := decodeP6Record(record)
	if err != nil {
		return nil, err
	}
	scope.last = &p6InputReuseEntry{
		id:                record.ID,
		playerID:          record.PlayerID,
		seed:              record.Seed,
		inputHash:         record.InputHash,
		configurationHash: record.ConfigurationHash,
		engineVersion:     record.EngineVersion,
		clientVersion:     record.ClientVersion,
		storedInput:       append([]byte(nil), record.Input...),
		input:             input,
	}
	return input, nil
}
