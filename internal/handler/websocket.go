package handler

import (
	"crypto/tls"
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/gorilla/websocket"
)

// upgrader → gelen HTTP isteğini WebSocket'e yükseltir
// CheckOrigin her zaman true döner çünkü gateway kendisi origin kontrolü yapmaz,
// downstream servis zaten JWT ile korunuyor
var upgrader = websocket.Upgrader{
	CheckOrigin:     func(r *http.Request) bool { return true },
	ReadBufferSize:  32768, // 32 KB — orderbook gibi büyük mesajlar için
	WriteBufferSize: 32768,
}

// dialer → upstream'e (ngrok wss://) bağlanmak için kullanılır
// InsecureSkipVerify: ngrok sertifikasını doğrulama — prod'da false yapılabilir
var dialer = websocket.Dialer{
	TLSClientConfig:  &tls.Config{InsecureSkipVerify: true},
	HandshakeTimeout: 10 * time.Second,
}

// NewWebSocketProxy → gelen WS bağlantısını upstream'e tüneller
func NewWebSocketProxy(target string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {

		// Target URL'yi parse et ve ws/wss scheme'e çevir
		targetURL, err := url.Parse(target)
		if err != nil {
			http.Error(w, "bad gateway", http.StatusBadGateway)
			return
		}
		switch targetURL.Scheme {
		case "http":
			targetURL.Scheme = "ws"
		case "https":
			targetURL.Scheme = "wss"
		}
		targetURL.Path = r.URL.Path
		targetURL.RawQuery = r.URL.RawQuery

		//  Upstream'e iletilecek header'ları hazırla
		// Authorization ve X- prefix'li header'lar geçer (X-User-ID JWT'den gelir)
		upstreamHeader := http.Header{}
		for k, vals := range r.Header {
			if strings.HasPrefix(k, "X-") || k == "Authorization" {
				upstreamHeader[k] = vals
			}
		}
		// ngrok browser uyarı sayfasını atla
		upstreamHeader.Set("ngrok-skip-browser-warning", "true")

		//  ÖNCE upstream'e bağlan
		// Upstream yoksa client'a düzgün HTTP 502 dönebiliriz
		// (client henüz WS upgrade olmadı, HTTP error yazılabilir)
		upstream, _, err := dialer.Dial(targetURL.String(), upstreamHeader)
		if err != nil {
			log.Printf("[WS] upstream dial error: %v", err)
			http.Error(w, "bad gateway", http.StatusBadGateway)
			return
		}
		defer upstream.Close()

		//  Client'ı WebSocket'e yükselt (HTTP → WS handshake)
		// Bu noktadan sonra HTTP error yazamayız, bağlantı WS'e geçti
		client, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			log.Printf("[WS] client upgrade error: %v", err)
			// upstream defer ile kapanır
			return
		}
		defer client.Close()

		//  İki yönlü köprü: her yön kendi goroutine'inde
		// errc channel'ı: herhangi bir yön kapanınca diğerini de durdurur
		errc := make(chan error, 2)

		// upstream → client
		go func() {
			for {
				msgType, msg, err := upstream.ReadMessage()
				if err != nil {
					errc <- err
					return
				}
				if err := client.WriteMessage(msgType, msg); err != nil {
					errc <- err
					return
				}
			}
		}()

		// client → upstream
		go func() {
			for {
				msgType, msg, err := client.ReadMessage()
				if err != nil {
					errc <- err
					return
				}
				if err := upstream.WriteMessage(msgType, msg); err != nil {
					errc <- err
					return
				}
			}
		}()

		//  İlk hata gelince (bağlantı kopması, timeout vb.) her ikisi de kapanır
		// defer'lar upstream ve client'ı kapatır → goroutine'ler err alır → biter
		<-errc
	})
}
