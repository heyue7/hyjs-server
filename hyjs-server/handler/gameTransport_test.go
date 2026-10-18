package handler

import (
	"context"
	"encoding/binary"
	"errors"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
	"hyjs-server/service"
	"io"
	"net"
	"path/filepath"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
)

type observedListener struct {
	net.Listener
	accepted chan struct{}
}

func (l *observedListener) Accept() (net.Conn, error) {
	conn, err := l.Listener.Accept()
	if err == nil {
		select {
		case l.accepted <- struct{}{}:
		default:
		}
	}
	return conn, err
}

// startTransportTest 使用独立临时存档和随机本机端口验证真实 TCP 读写。
func startTransportTest(t *testing.T) (net.Listener, context.CancelFunc, <-chan error) {
	t.Helper()
	if err := db.InitGame(context.Background(), filepath.Join(t.TempDir(), "profiles.sqlite3")); err != nil {
		t.Fatal(err)
	}
	sqlDB, err := db.GetIns().DB()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { sqlDB.Close() })
	rawListener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	listener := &observedListener{Listener: rawListener, accepted: make(chan struct{}, 1)}
	var (
		ctx, cancel = context.WithCancel(context.Background())
		done        = make(chan error, 1)
	)
	go func() {
		done <- ServeGameTCP(ctx, listener, &service.GameService{})
		close(done)
	}()
	t.Cleanup(func() {
		cancel()
		select {
		case <-done:
		case <-time.After(2 * time.Second):
			t.Error("TCP server did not release its connections")
		}
	})
	return listener, cancel, done
}

func tcpRequest(t *testing.T, id int64, name string, fields any) []byte {
	t.Helper()
	msg, err := model.GameMessage(name, fields)
	if err != nil {
		t.Fatal(err)
	}
	body, err := proto.Marshal(msg)
	if err != nil {
		t.Fatal(err)
	}
	raw := make([]byte, 28, 28+len(body))
	binary.BigEndian.PutUint32(raw, uint32(24+len(body)))
	binary.BigEndian.PutUint32(raw[4:], 1)
	for code, value := range model.OpcodeNames {
		if value == name {
			binary.BigEndian.PutUint32(raw[8:], uint32(code))
			break
		}
	}
	binary.BigEndian.PutUint32(raw[12:], 1)
	binary.BigEndian.PutUint64(raw[16:], uint64(id))
	binary.BigEndian.PutUint32(raw[24:], uint32(len(body)))
	return append(raw, body...)
}

func tcpResponse(t *testing.T, conn net.Conn) proto.Message {
	t.Helper()
	header := make([]byte, 8)
	if _, err := io.ReadFull(conn, header); err != nil {
		t.Fatal(err)
	}
	size := binary.BigEndian.Uint32(header)
	if size > constUtil.MaxPacket {
		t.Fatalf("invalid response length %d", size)
	}
	body := make([]byte, size)
	if _, err := io.ReadFull(conn, body); err != nil {
		t.Fatal(err)
	}
	msg, err := model.GameMessage(model.OpcodeNames[int32(binary.BigEndian.Uint32(header[4:]))], nil)
	if err != nil {
		t.Fatal(err)
	}
	if err = proto.Unmarshal(body, msg); err != nil {
		t.Fatal(err)
	}
	return msg
}

func TestTCPFragmentationCoalescingAndSessionRotation(t *testing.T) {
	listener, _, _ := startTransportTest(t)
	row, err := service.LoginAccount(context.Background(), "tcp-segments", "local-password")
	if err != nil {
		t.Fatal(err)
	}
	conn, err := net.Dial("tcp", listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	conn.SetDeadline(time.Now().Add(3 * time.Second))
	login := tcpRequest(t, row.Id, "CSSocketLogin", map[string]any{"sessionKey": row.Session})
	// 将长度前缀、剩余包头与消息体分别写入，确认读取不会假定一次 Read 得到整包。
	for _, part := range [][]byte{login[:1], login[1:4], login[4:17], login[17:28], login[28:]} {
		if _, err = conn.Write(part); err != nil {
			t.Fatal(err)
		}
	}
	if reply, ok := tcpResponse(t, conn).(*pb.SCSocketLogin); !ok || reply.GetResult() != 100 {
		t.Fatal("socket login failed")
	}
	// 连续两帧只调用一次 Write，确认包边界按协议长度恢复。
	first := tcpRequest(t, row.Id, "CSPingTcp", map[string]any{"clientTime": 11})
	second := tcpRequest(t, row.Id, "CSPingTcp", map[string]any{"clientTime": 22})
	if _, err = conn.Write(append(first, second...)); err != nil {
		t.Fatal(err)
	}
	for _, want := range []int64{11, 22} {
		if reply, ok := tcpResponse(t, conn).(*pb.SCPingTcp); !ok || reply.GetClientTime() != want {
			t.Fatalf("coalesced response did not preserve clientTime %d", want)
		}
	}
	other, err := service.LoginAccount(context.Background(), "other-player", "local-password")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = conn.Write(tcpRequest(t, other.Id, "CSPingTcp", map[string]any{"clientTime": 33})); err != nil {
		t.Fatal(err)
	}
	if reply, ok := tcpResponse(t, conn).(*pb.ErrorMessage); !ok || reply.GetCode() != constUtil.ErrGameSession {
		t.Fatal("socket session crossed player boundary")
	}
	if _, err = service.LoginAccount(context.Background(), "tcp-segments", "local-password"); err != nil {
		t.Fatal(err)
	}
	if _, err = conn.Write(first); err != nil {
		t.Fatal(err)
	}
	if reply, ok := tcpResponse(t, conn).(*pb.ErrorMessage); !ok || reply.GetCode() != constUtil.ErrGameSession {
		t.Fatal("rotated session remained usable")
	}
}

func TestTCPShutdownAndListenerFailureReleasePartialFrames(t *testing.T) {
	for _, failure := range []bool{false, true} {
		t.Run(map[bool]string{false: "shutdown", true: "accept-failure"}[failure], func(t *testing.T) {
			listener, cancel, done := startTransportTest(t)
			conn, err := net.Dial("tcp", listener.Addr().String())
			if err != nil {
				t.Fatal(err)
			}
			defer conn.Close()
			// 只发送一个字节，让连接协程停在原始帧长度的 ReadFull。
			if _, err = conn.Write([]byte{0}); err != nil {
				t.Fatal(err)
			}
			select {
			case <-listener.(*observedListener).accepted:
			case <-time.After(2 * time.Second):
				t.Fatal("connection was not accepted")
			}
			if failure {
				listener.Close()
			} else {
				cancel()
			}
			select {
			case err = <-done:
				if failure && !errors.Is(err, net.ErrClosed) {
					t.Fatalf("listener failure was lost: %v", err)
				}
				if !failure && err != nil {
					t.Fatal(err)
				}
				cancel()
			case <-time.After(2 * time.Second):
				t.Fatal("partial frame kept server alive after listener stopped")
			}
		})
	}
}
