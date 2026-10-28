package main

import (
	"encoding/json"
	"hyjs-server/service"
	"os"
)

func main() {
	if len(os.Args) < 3 {
		panic("series output-directory [record]")
	}
	raw, e := os.ReadFile(os.Args[1])
	if e != nil {
		panic(e)
	}
	var r any
	if len(os.Args) > 3 {
		rec, e := os.ReadFile(os.Args[3])
		if e != nil {
			panic(e)
		}
		r, e = service.Trial87Compare(raw, rec)
	} else {
		r, e = service.Trial87Prepare(raw, os.Args[2])
	}
	if e != nil {
		panic(e)
	}
	b, _ := json.Marshal(r)
	os.Stdout.Write(b)
}
