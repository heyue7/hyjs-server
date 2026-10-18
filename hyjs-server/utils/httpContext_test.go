package utils

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// TestPostWithContextCancellation 验证跨服发送取消后收口，不等默认网络超时。
func TestPostWithContextCancellation(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(started)
		select {
		case <-r.Context().Done():
		case <-release:
		}
	}))
	defer server.Close()
	defer close(release)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() {
		_, _, err := NewHttp().PostWithContext(ctx, server.URL, []byte("signed-body"))
		done <- err
	}()
	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("request did not arrive")
	}
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatal("caller cancellation not propagated", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("cancelled sender kept waiting")
	}
}

// TestPostWithContextRawBody 保存签名原字节和显式头部，不被JSON编码更改。
func TestPostWithContextRawBody(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		if r.Method != http.MethodPost || r.Header.Get("Content-Type") != "application/octet-stream" || r.Header.Get("X-Peer-Proof") != "fixture-proof" || string(body) != "\x00\xffsigned-body" {
			t.Error("signed request body or headers changed")
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		w.WriteHeader(http.StatusAccepted)
		_, _ = w.Write([]byte("ack"))
	}))
	defer server.Close()
	client := NewHttp().AddHeader("Content-Type", "application/octet-stream").AddHeader("X-Peer-Proof", "fixture-proof")
	code, raw, err := client.PostWithContext(context.Background(), server.URL, []byte("\x00\xffsigned-body"))
	if err != nil || code != http.StatusAccepted || string(raw) != "ack" {
		t.Fatal(code, string(raw), err)
	}
}
