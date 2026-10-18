package handler

import (
	"encoding/base64"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"google.golang.org/protobuf/proto"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
	"hyjs-server/service"
)

func TestP10VoiceOriginalFrameValidation(t *testing.T) {
	valid := &pb.CSChatTcp{ChatType: proto.Int32(3), PlayerId: proto.Int64(5), IsAudio: proto.Int32(2), DownLoadUrl: proto.String("hyjsvoice_0123456789abcdef0123456789abcdef.wav"), AudioTime: proto.Int32(1), FileSize: proto.Int64(32044)}
	packet := &model.GamePacket{PlayerId: 4, Body: valid}
	if err := ValidateGameRequest(packet); err != nil {
		t.Fatal(err)
	}
	guild := proto.Clone(valid).(*pb.CSChatTcp)
	guild.ChatType = proto.Int32(2)
	guild.PlayerId = proto.Int64(0)
	if err := ValidateGameRequest(&model.GamePacket{PlayerId: 4, Body: guild}); err != nil {
		t.Fatal("original guild audio has no private recipient", err)
	}
	for _, change := range []func(*pb.CSChatTcp){
		func(b *pb.CSChatTcp) { b.DownLoadUrl = proto.String("https://example.invalid/voice.wav") },
		func(b *pb.CSChatTcp) { b.DownLoadUrl = proto.String("x") },
		func(b *pb.CSChatTcp) { b.DownLoadUrl = proto.String("hyjsvoice_0123456789abcdef0123456789abcdeG.wav") },
		func(b *pb.CSChatTcp) { b.FileName = proto.String("another.wav") },
		func(b *pb.CSChatTcp) { b.IsAudio = proto.Int32(1) },
		func(b *pb.CSChatTcp) { b.ChatType = proto.Int32(4) },
		func(b *pb.CSChatTcp) { b.PlayerId = proto.Int64(4) },
		func(b *pb.CSChatTcp) { b.Content = proto.String(strings.Repeat("🙂", 21)) },
		func(b *pb.CSChatTcp) { b.Content = proto.String("bad\x00") },
		func(b *pb.CSChatTcp) { b.AudioTime = proto.Int32(31) },
		func(b *pb.CSChatTcp) { b.FileSize = proto.Int64(960045) },
	} {
		b := proto.Clone(valid).(*pb.CSChatTcp)
		change(b)
		if err := ValidateGameRequest(&model.GamePacket{PlayerId: 4, Body: b}); err == nil {
			t.Fatal("invalid original voice frame accepted")
		}
	}
}

func TestP10VoiceHTTPRejectsInvalidHeadersAndOversizedStream(t *testing.T) {
	previous := gameService
	gameService = &service.GameService{P10VoiceReady: true}
	defer func() { gameService = previous }()
	gin.SetMode(gin.TestMode)
	key := base64.StdEncoding.EncodeToString(make([]byte, 16))
	for _, path := range []string{"/voice/download", "/voice/delete"} {
		for _, filename := range []string{"x", "../other.wav", "hyjsvoice_0123456789abcdef0123456789abcdeG.wav"} {
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest("POST", path, nil)
			c.Request.Header.Set("X-Hyjs-Player-ID", "4")
			c.Request.Header.Set("X-Hyjs-Session-Key", key)
			c.Request.Header.Set("X-Hyjs-File-Name", filename)
			var err error
			if path == "/voice/download" {
				err = P10VoiceDownloadHandler(c)
			} else {
				err = P10VoiceDeleteHandler(c)
			}
			if err == nil {
				t.Fatal("invalid file header reached service")
			}
		}
	}
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("POST", "/voice/upload", strings.NewReader(strings.Repeat("x", 960045)))
	c.Request.Header.Set("X-Hyjs-Player-ID", "4")
	c.Request.Header.Set("X-Hyjs-Session-Key", key)
	c.Request.Header.Set("X-Hyjs-Chat-Type", "1")
	c.Request.Header.Set("X-Hyjs-Request-ID", "request-1")
	c.Request.Header.Set("X-Hyjs-Voice-Scope", "world:1")
	c.Request.Header.Set("Content-Type", "audio/wav")
	if err := P10VoiceUploadHandler(c); err == nil {
		t.Fatal("oversized stream reached storage")
	}
}
