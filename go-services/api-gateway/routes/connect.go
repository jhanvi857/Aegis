package routes

import (
	"encoding/json"
	"net/http"
	"os"
	"strings"
)

func ConnectConfigHandler(w http.ResponseWriter, r *http.Request) {
	host := requestHost(r)
	apiPort := GetConfig().ServerPort
	otlpPort := GetConfig().OTLPHTTPPort
	tokenConfigured := strings.TrimSpace(os.Getenv("AEGIS_INGEST_TOKEN")) != ""
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"otlpHttpEndpoint":      "http://" + host + ":" + otlpPort,
		"otlpGrpcEndpoint":      "http://" + host + ":4317",
		"ingestTokenConfigured": tokenConfigured,
		"apiEndpoint":           "http://" + host + ":" + apiPort,
		"scrapeTargetExample":   host + ":8080/metrics",
		"dockerComposeSnippet":  "networks:\n  aegis-net:\n    external: true\nservices:\n  app:\n    networks: [aegis-net]\n    labels:\n      aegis.scrape: \"true\"\n      aegis.port: \"8080\"",
		"kubernetesAnnotations": map[string]string{
			"prometheus.io/scrape": "true",
			"prometheus.io/port":   "8080",
			"prometheus.io/path":   "/metrics",
			"aegis.io/service":     "<service.name>",
		},
	})
}

func requestHost(r *http.Request) string {
	host := r.Host
	if i := strings.LastIndex(host, ":"); i > 0 {
		host = host[:i]
	}
	if host == "" {
		host = "aegis"
	}
	return host
}
