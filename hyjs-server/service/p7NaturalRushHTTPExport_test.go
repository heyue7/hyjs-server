package service

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"google.golang.org/protobuf/proto"
	"hyjs-server/constUtil"
	pb "hyjs-server/model/protocol"
)

// 独立测试客户端只从本次真实 HTTP Start 计算完整原 End；不读写 DB、不授予业务权威。
func TestP7NaturalRushExportOriginalHTTPStart(t *testing.T) {
	file := os.Getenv("HYJS_P7_NATURAL_HTTP_START")
	if file == "" {
		t.Skip("explicit Natural Trial HTTP Start not requested")
	}
	inputPath, err := filepath.Abs(file)
	if err != nil {
		t.Fatal(err)
	}
	outputPath, err := filepath.Abs(os.Getenv("HYJS_P7_NATURAL_HTTP_OUTPUT"))
	if err != nil {
		t.Fatal(err)
	}
	dir := filepath.Dir(inputPath)
	if filepath.Dir(outputPath) != dir || !strings.HasPrefix(filepath.Base(dir), "function-natural-rush-") ||
		filepath.Base(filepath.Dir(dir)) != "p7" || filepath.Base(filepath.Dir(filepath.Dir(dir))) != "analysis" ||
		!strings.HasSuffix(inputPath, ".SCRushFightStart.pb") || !strings.HasSuffix(outputPath, ".calculated.json") {
		t.Fatal("owned analysis/p7/function-natural-rush-* input/output required")
	}
	raw, err := readTestAnalysis75(inputPath)
	if err != nil || p6Digest(raw) != os.Getenv("HYJS_P7_NATURAL_HTTP_START_SHA256") {
		t.Fatal("exact original HTTP Start SHA", err)
	}
	start := &pb.SCRushFightStart{}
	if err = proto.Unmarshal(raw, start); err != nil {
		t.Fatal(err)
	}
	autoAt, err := strconv.ParseFloat(os.Getenv("HYJS_P7_NATURAL_HTTP_AUTO_AT"), 64)
	if err != nil {
		t.Fatal(err)
	}
	s := p6OriginalTables(t)
	series, err := s.FreezeP6RushSeriesInput(start, true, false, nil, constUtil.P6OriginalClientHash)
	if err != nil {
		t.Fatal(err)
	}
	input, err := p7NaturalRushInputAt(series, autoAt)
	if err != nil {
		t.Fatal(err)
	}
	out, err := s.ComputeOriginalP6ProtocolFinalHP(t.Context(), input, nil)
	if err != nil || out == nil || out.Outcome == nil || out.Outcome.Authoritative {
		t.Fatal("independent original computation", err)
	}
	if err = validateP7NaturalRushTerminal(input, out); err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(out)
	if err != nil {
		t.Fatal(err)
	}
	writer, err := os.OpenFile(outputPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = writer.Write(encoded); err != nil {
		writer.Close()
		t.Fatal(err)
	}
	if err = writer.Close(); err != nil {
		t.Fatal(err)
	}
}
