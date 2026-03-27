package handler

import (
	"encoding/json"
	"net/http"
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

func Health(cfg *config.Config) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		upstreamServices := map[string]string{
			"market-data": cfg.MarketDataURL + "/health",
			"exchange":    cfg.ExchangeURL + "/health",
		}

		resp := &healthResponse{
			Status:   "ok",
			Services: make(map[string]*serviceStatus),
		}

		client := &http.Client{Timeout: 3 * time.Second}

		for name, url := range upstreamServices {
			svc := checkService(client, url)
			resp.Services[name] = svc
			if svc.Status == "down" {
				resp.Status = "degraded"
			}
		}

		w.Header().Set("Content-Type", "application/json")
		if resp.Status == "ok" {
			w.WriteHeader(http.StatusOK)
		} else {
			w.WriteHeader(http.StatusServiceUnavailable)
		}
		json.NewEncoder(w).Encode(resp)
	}
}

func checkService(client *http.Client, url string) *serviceStatus {
	start := time.Now()
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return &serviceStatus{Status: "down", Error: err.Error()}
	}
	req.Header.Set("ngrok-skip-browser-warning", "true")

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
