package handler

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

var testUpgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

func wsURL(httpURL string) string {
	return strings.Replace(httpURL, "http://", "ws://", 1)
}

// TestWebSocketProxy_UpstreamDown_Returns502 → upstream yoksa HTTP 502 dönmeli
func TestWebSocketProxy_UpstreamDown_Returns502(t *testing.T) {
	proxy := NewWebSocketProxy("http://localhost:1")

	req := httptest.NewRequest(http.MethodGet, "/ws", nil)
	rec := httptest.NewRecorder()

	proxy.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadGateway {
		t.Errorf("beklenen 502, gelen %d", rec.Code)
	}
}

// TestWebSocketProxy_ForwardsMessages → client mesajı proxy üzerinden upstream echo sunucusuna ulaşmalı
func TestWebSocketProxy_ForwardsMessages(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := testUpgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()
		for {
			mt, msg, err := conn.ReadMessage()
			if err != nil {
				return
			}
			if err := conn.WriteMessage(mt, msg); err != nil {
				return
			}
		}
	}))
	defer upstream.Close()

	proxyServer := httptest.NewServer(NewWebSocketProxy(upstream.URL))
	defer proxyServer.Close()

	conn, _, err := websocket.DefaultDialer.Dial(wsURL(proxyServer.URL)+"/ws", nil)
	if err != nil {
		t.Fatalf("proxy'ye bağlanılamadı: %v", err)
	}
	defer conn.Close()

	if err := conn.WriteMessage(websocket.TextMessage, []byte("merhaba")); err != nil {
		t.Fatalf("mesaj gönderilemedi: %v", err)
	}

	conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	_, got, err := conn.ReadMessage()
	if err != nil {
		t.Fatalf("mesaj alınamadı: %v", err)
	}

	if string(got) != "merhaba" {
		t.Errorf("beklenen 'merhaba', gelen %q", string(got))
	}
}

// TestWebSocketProxy_XHeadersForwarded → X- prefix'li header'lar upstream'e iletilmeli
func TestWebSocketProxy_XHeadersForwarded(t *testing.T) {
	var gotHeader string

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotHeader = r.Header.Get("X-Custom-Test")
		conn, err := testUpgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		conn.Close()
	}))
	defer upstream.Close()

	proxyServer := httptest.NewServer(NewWebSocketProxy(upstream.URL))
	defer proxyServer.Close()

	header := http.Header{}
	header.Set("X-Custom-Test", "hello")
	conn, _, _ := websocket.DefaultDialer.Dial(wsURL(proxyServer.URL)+"/ws", header)
	if conn != nil {
		conn.Close()
	}

	if gotHeader != "hello" {
		t.Errorf("beklenen X-Custom-Test=hello, gelen %q", gotHeader)
	}
}

// TestWebSocketProxy_TokenFromQueryParam → ?token=xxx upstream'e Authorization: Bearer xxx olarak iletilmeli
func TestWebSocketProxy_TokenFromQueryParam(t *testing.T) {
	var gotAuth string

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		conn, err := testUpgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		conn.Close()
	}))
	defer upstream.Close()

	proxyServer := httptest.NewServer(NewWebSocketProxy(upstream.URL))
	defer proxyServer.Close()

	conn, _, _ := websocket.DefaultDialer.Dial(wsURL(proxyServer.URL)+"/ws?token=mytoken", nil)
	if conn != nil {
		conn.Close()
	}

	if gotAuth != "Bearer mytoken" {
		t.Errorf("beklenen 'Bearer mytoken', gelen %q", gotAuth)
	}
}