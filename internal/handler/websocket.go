package handler

import (
	"log"
	"net/http"
	"net/url"
	"strings"

	"golang.org/x/net/websocket"
)

// NewWebSocketProxy WebSocket bağlantılarını upstream'e tüneller
func NewWebSocketProxy(target string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		targetURL, err := url.Parse(target)
		if err != nil {
			http.Error(w, "bad gateway", http.StatusBadGateway)
			return
		}

		// ws:// veya wss:// scheme'e çevir
		switch targetURL.Scheme {
		case "http":
			targetURL.Scheme = "ws"
		case "https":
			targetURL.Scheme = "wss"
		}

		// Path'i koru
		targetURL.Path = r.URL.Path
		targetURL.RawQuery = r.URL.RawQuery

		// Upstream'e bağlan
		upstreamConfig, err := websocket.NewConfig(targetURL.String(), r.Header.Get("Origin"))
		if err != nil {
			http.Error(w, "bad gateway", http.StatusBadGateway)
			return
		}

		// Client header'larını ilet (Authorization, X-User-ID vb.)
		for k, vals := range r.Header {
			if strings.HasPrefix(k, "X-") || k == "Authorization" {
				for _, v := range vals {
					upstreamConfig.Header.Add(k, v)
				}
			}
		}

		upstream, err := websocket.DialConfig(upstreamConfig)
		if err != nil {
			log.Printf("websocket upstream dial error: %v", err)
			http.Error(w, "bad gateway", http.StatusBadGateway)
			return
		}
		defer upstream.Close()

		// Client WebSocket handshake
		websocket.Handler(func(client *websocket.Conn) {
			defer client.Close()

			done := make(chan struct{})

			// upstream → client
			go func() {
				defer close(done)
				buf := make([]byte, 4096)
				for {
					n, err := upstream.Read(buf)
					if err != nil {
						return
					}
					if _, err := client.Write(buf[:n]); err != nil {
						return
					}
				}
			}()

			// client → upstream
			buf := make([]byte, 4096)
			for {
				n, err := client.Read(buf)
				if err != nil {
					return
				}
				if _, err := upstream.Write(buf[:n]); err != nil {
					return
				}
			}
		}).ServeHTTP(w, r)
	})
}
