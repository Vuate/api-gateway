package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"sync"
	"time"

	"github.com/Vuate/api-gateway/config"
)

type serviceStatus struct {
	Status  string `json:"status"`
	Latency string `json:"latency,omitempty"`
	Error   string `json:"error,omitempty"`
}

type healthResponse struct {
	Status   string                    `json:"status"`
	Services map[string]*serviceStatus `json:"services"`
}

const healthCheckTimeout = 3 * time.Second

func Health(cfg *config.Config) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		upstreamServices := map[string]string{
			"market-data": cfg.MarketDataURL + "/health",
			"exchange":    cfg.ExchangeURL + "/health",
			"auth":        cfg.AuthURL + "/health",
		}

		ctx, cancel := context.WithTimeout(r.Context(), healthCheckTimeout)
		defer cancel()

		resp := &healthResponse{
			Status:   "ok",
			Services: make(map[string]*serviceStatus),
		}

		var wg sync.WaitGroup
		var mu sync.Mutex

		for name, url := range upstreamServices {
			wg.Add(1)
			go func(name, url string) {
				defer wg.Done()
				svc := checkService(ctx, url)
				mu.Lock()
				resp.Services[name] = svc
				if svc.Status == "down" {
					resp.Status = "degraded"
				}
				mu.Unlock()
			}(name, url)
		}

		wg.Wait()

		w.Header().Set("Content-Type", "application/json")
		if resp.Status == "ok" {
			w.WriteHeader(http.StatusOK)
		} else {
			w.WriteHeader(http.StatusServiceUnavailable)
		}
		json.NewEncoder(w).Encode(resp)
	}
}

func checkService(ctx context.Context, url string) *serviceStatus {
	start := time.Now()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return &serviceStatus{Status: "down", Error: err.Error()}
	}
	req.Header.Set("ngrok-skip-browser-warning", "true")

	client := &http.Client{}
	res, err := client.Do(req)
	if err != nil {
		return &serviceStatus{Status: "down", Error: err.Error()}
	}
	defer res.Body.Close()

	latency := time.Since(start).Round(time.Millisecond).String()
	if res.StatusCode == http.StatusOK {
		return &serviceStatus{Status: "ok", Latency: latency}
	}
	return &serviceStatus{Status: "down", Latency: latency, Error: http.StatusText(res.StatusCode)}
}
