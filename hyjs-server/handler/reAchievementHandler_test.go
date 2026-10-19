package handler

import (
	"bytes"
	"encoding/json"
	"github.com/gin-gonic/gin"
	"google.golang.org/protobuf/proto"
	"hyjs-server/constUtil"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestReAchievementRequirementsHTTP(t *testing.T) {
	for _, update := range []bool{false, true} {
		for _, req := range []model.AdminAchievementDTO{
			{Kind: constUtil.ReAchievementGrowthKind, Target: 3},
			{Kind: constUtil.ReAchievementGrowthKind, Target: 3, Requirements: &model.AdminAchievementRequirements{LoginDays: 7, Level: 0, Stages: 30}},
			{Kind: "level", Target: 20, Requirements: &model.AdminAchievementRequirements{LoginDays: 7, Level: 40, Stages: 30}},
		} {
			req.Name = "测试成就"
			req.Source = "operator"
			req.Version = strings.Repeat("a", 64)
			raw, _ := json.Marshal(req)
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Request = httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(raw))
			c.Request.Header.Set("Content-Type", "application/json")
			c.Params = gin.Params{{Key: "id", Value: "os310_hero_077"}}
			if update {
				AdminAchievementUpdateHandler(c)
			} else {
				AdminAchievementCreateHandler(c)
			}
			if w.Code != http.StatusBadRequest {
				t.Fatalf("invalid OS requirements reached service: update=%v status=%d", update, w.Code)
			}
		}
	}
}
func TestReAchievementDisplayIDRequest(t *testing.T) {
	valid := &model.GamePacket{Timestamp: 1, Body: &pb.CSGetAchievementReward{Id: proto.String("os310_hero_077~7~40~30")}}
	if e := ValidateGameRequest(valid); e != nil {
		t.Fatal(e)
	}
	for _, id := range []string{"", strings.Repeat("a", 129), " test", "test\n"} {
		valid.Body = &pb.CSGetAchievementReward{Id: proto.String(id)}
		if ValidateGameRequest(valid) == nil {
			t.Fatal("invalid claim id accepted")
		}
	}
}
