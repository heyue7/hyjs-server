// Offline source-only export. No database initialization or business mutation.
package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"google.golang.org/protobuf/proto"
	"hyjs-server/constUtil"
	"hyjs-server/service"
)

func main() {
	tables := flag.String("tables", "", "original table directory")
	out := flag.String("out", "", "new source export JSON; refuses overwrite")
	flag.Parse()
	if *tables == "" || *out == "" {
		panic("explicit --tables and --out required")
	}
	s := &service.GameService{ServerID: 1}
	if err := s.LoadTables(*tables); err != nil {
		panic(err)
	}
	if err := s.LoadP13GuildTables(*tables); err != nil {
		panic(err)
	}
	guards := map[string]any{}
	for _, prefix := range []struct {
		name  string
		count int
	}{{"tower_guard_", 6}, {"factory_guard_", 2}, {"baseCamp_guard_", 2}} {
		for i := 1; i <= prefix.count; i++ {
			id := fmt.Sprintf("%s%03d", prefix.name, i)
			monster, err := s.P13UnionFightMirrorGuardTemplate(id)
			if err != nil {
				panic(err)
			}
			raw, err := (proto.MarshalOptions{Deterministic: true}).Marshal(monster)
			if err != nil {
				panic(err)
			}
			h := sha256.Sum256(raw)
			guards[id] = map[string]any{"hex": hex.EncodeToString(raw), "sha256": fmt.Sprintf("%x", h), "monster": monster}
		}
	}
	pins := map[string]string{}
	files, err := filepath.Glob(filepath.Join(*tables, "*.tsv"))
	if err != nil {
		panic(err)
	}
	for _, p := range files {
		raw, e := os.ReadFile(p)
		if e != nil {
			panic(e)
		}
		h := sha256.Sum256(raw)
		pins[filepath.Base(p)] = fmt.Sprintf("%x", h)
	}
	payload := map[string]any{"schema": "war-prestart-original-source-v1", "serverID": 1, "guards": guards, "tablePins": pins, "guardRule": constUtil.P13UFGuardMirrorRule, "guardProof": constUtil.P6UFGuardMirrorNativeProof, "playerProof": constUtil.P6UFPartyPlayerNativeProof, "authorityGranted": false}
	raw, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		panic(err)
	}
	f, err := os.OpenFile(*out, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		panic(err)
	}
	defer f.Close()
	if _, err = f.Write(append(raw, '\n')); err != nil {
		panic(err)
	}
}
