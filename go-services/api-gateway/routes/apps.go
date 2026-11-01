package routes

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/aegis/go-services/api-gateway/storage"
	"github.com/gorilla/mux"
	"gopkg.in/yaml.v3"
)

type appRegistrationRequest struct {
	Name         string                       `json:"name"`
	Services     []storage.ApplicationService `json:"services"`
	TopologyYAML string                       `json:"topology_yaml,omitempty"`
}

func appID(name string) string {
	sum := sha256.Sum256([]byte(strings.ToLower(strings.TrimSpace(name)) + fmt.Sprint(time.Now().UnixNano())))
	return "app-" + hex.EncodeToString(sum[:])[:16]
}

func newAppKey() (string, string, string) {
	raw := make([]byte, 32)
	_, _ = rand.Read(raw)
	key := "aegis_app_" + hex.EncodeToString(raw)
	sum := sha256.Sum256([]byte(key))
	return key, hex.EncodeToString(sum[:]), key[:20]
}

func normalizeAppService(service storage.ApplicationService) storage.ApplicationService {
	if service.Mode == "" {
		service.Mode = "recommend"
	}
	if service.AllowedActions == nil {
		service.AllowedActions = []string{}
	}
	return service
}

func CreateApplicationHandler(w http.ResponseWriter, r *http.Request) {
	var req appRegistrationRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid application payload"}`, http.StatusBadRequest)
		return
	}
	if len(req.Services) == 0 && strings.TrimSpace(req.TopologyYAML) != "" {
		var topology struct {
			SystemName string `yaml:"system_name"`
			Nodes      []struct {
				ID             string   `yaml:"id"`
				Name           string   `yaml:"name"`
				BaseURL        string   `yaml:"base_url"`
				Host           string   `yaml:"host"`
				Port           int      `yaml:"port"`
				Environment    string   `yaml:"environment"`
				DependsOn      []string `yaml:"depends_on"`
				Dependencies   []string `yaml:"dependencies"`
				HealthURL      string   `yaml:"health_url"`
				HealthEndpoint string   `yaml:"health_endpoint"`
				Region         string   `yaml:"region"`
				Provider       string   `yaml:"provider"`
				Mode           string   `yaml:"mode"`
				Allowed        []string `yaml:"allowed_actions"`
			} `yaml:"nodes"`
			Services []storage.ApplicationService `yaml:"services"`
		}
		if err := yaml.Unmarshal([]byte(req.TopologyYAML), &topology); err != nil {
			http.Error(w, `{"error":"invalid topology_yaml"}`, http.StatusBadRequest)
			return
		}
		if strings.TrimSpace(req.Name) == "" {
			req.Name = topology.SystemName
		}
		req.Services = topology.Services
		if len(req.Services) == 0 {
			for _, node := range topology.Nodes {
				baseURL := strings.TrimSpace(node.BaseURL)
				if baseURL == "" && node.Host != "" {
					baseURL = fmt.Sprintf("http://%s", node.Host)
					if node.Port > 0 {
						baseURL = fmt.Sprintf("%s:%d", baseURL, node.Port)
					}
				}
				if strings.TrimSpace(req.Name) == "" {
					http.Error(w, `{"error":"name is required (or provide system_name in topology_yaml)"}`, http.StatusBadRequest)
					return
				}
				healthURL := strings.TrimSpace(node.HealthURL)
				if healthURL == "" {
					healthURL = strings.TrimSpace(node.HealthEndpoint)
				}
				dependsOn := node.DependsOn
				if len(dependsOn) == 0 {
					dependsOn = node.Dependencies
				}
				mode := node.Mode
				if mode == "" {
					mode = "recommend"
				}
				req.Services = append(req.Services, storage.ApplicationService{
					ID: node.ID, Name: node.Name, BaseURL: baseURL, Host: node.Host,
					Environment: node.Environment, DependsOn: dependsOn, HealthURL: healthURL,
					Region: node.Region, Provider: node.Provider, Mode: mode,
					AllowedActions: node.Allowed,
				})
			}
		}
	}
	if len(req.Services) == 0 {
		http.Error(w, `{"error":"at least one service is required"}`, http.StatusBadRequest)
		return
	}
	seen := map[string]bool{}
	for i := range req.Services {
		req.Services[i] = normalizeAppService(req.Services[i])
		s := &req.Services[i]
		if s.ID == "" || s.Name == "" || seen[s.ID] {
			http.Error(w, `{"error":"each service needs a unique id and name"}`, http.StatusBadRequest)
			return
		}
		if s.Mode != "observe" && s.Mode != "recommend" && s.Mode != "execute" && s.Mode != "auto_low" {
			http.Error(w, `{"error":"invalid service mode"}`, http.StatusBadRequest)
			return
		}
		seen[s.ID] = true
	}
	caller := GetCaller(r)
	createdBy := "system"
	if caller != nil {
		createdBy = caller.ActorID
	}
	app := &storage.Application{ID: appID(req.Name), Name: strings.TrimSpace(req.Name), Services: req.Services, CreatedBy: createdBy, CreatedAt: time.Now().UTC()}
	if err := GetAuthStore().CreateApplication(r.Context(), app); err != nil {
		http.Error(w, `{"error":"failed to create application"}`, http.StatusInternalServerError)
		return
	}
	rawKey, hash, prefix := newAppKey()
	key := &storage.APIKey{ID: "key-" + app.ID, Name: app.Name, KeyPrefix: prefix, KeyHash: hash, Scopes: "agent:poll,agent:result,ingest", Role: storage.RoleOperator, CreatedBy: createdBy, AppID: app.ID}
	if err := GetAuthStore().CreateAPIKey(r.Context(), key); err != nil {
		http.Error(w, `{"error":"failed to create application key"}`, http.StatusInternalServerError)
		return
	}
	LogAudit(r, "application.create", app.ID, "LOW", "", "services="+fmt.Sprint(len(app.Services)), "SUCCESS", "")
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(map[string]interface{}{"application": app, "api_key": rawKey, "warning": "Store this key now. It will not be shown again."})
}

func ListApplicationsHandler(w http.ResponseWriter, r *http.Request) {
	caller := GetCaller(r)
	owner := ""
	if caller != nil && caller.Role == storage.RoleViewer {
		owner = caller.ActorID
	}
	apps, err := GetAuthStore().ListApplications(r.Context(), owner)
	if err != nil {
		http.Error(w, `{"error":"failed to list applications"}`, http.StatusInternalServerError)
		return
	}
	_ = json.NewEncoder(w).Encode(apps)
}

func GetApplicationHandler(w http.ResponseWriter, r *http.Request) {
	app, err := GetAuthStore().GetApplication(r.Context(), mux.Vars(r)["id"])
	if err != nil {
		http.Error(w, `{"error":"application not found"}`, http.StatusNotFound)
		return
	}

	_ = json.NewEncoder(w).Encode(app)
}

func UpdateApplicationPermissionsHandler(w http.ResponseWriter, r *http.Request) {
	app, err := GetAuthStore().GetApplication(r.Context(), mux.Vars(r)["id"])
	if err != nil {
		http.Error(w, `{"error":"application not found"}`, http.StatusNotFound)
		return
	}
	var payload struct {
		Mode           string   `json:"mode"`
		AllowedActions []string `json:"allowed_actions"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		http.Error(w, `{"error":"invalid permissions payload"}`, http.StatusBadRequest)
		return
	}
	if payload.Mode != "observe" && payload.Mode != "recommend" && payload.Mode != "execute" && payload.Mode != "auto_low" {
		http.Error(w, `{"error":"invalid mode"}`, http.StatusBadRequest)
		return
	}
	for i := range app.Services {
		if app.Services[i].ID == mux.Vars(r)["service"] {
			app.Services[i].Mode = payload.Mode
			app.Services[i].AllowedActions = payload.AllowedActions
			if err := GetAuthStore().UpdateApplication(r.Context(), app); err != nil {
				http.Error(w, `{"error":"failed to update permissions"}`, http.StatusInternalServerError)
				return
			}
			LogAudit(r, "application.permissions.update", app.Services[i].ID, "LOW", "", payload.Mode, "SUCCESS", "")
			_ = json.NewEncoder(w).Encode(app.Services[i])
			return
		}
	}
	http.Error(w, `{"error":"service not found"}`, http.StatusNotFound)
}

func riskForAppAction(action string) string {
	switch strings.ToUpper(action) {
	case "FLUSH_CACHE":
		return "LOW"
	case "INCREASE_POOL", "SCALE_UP":
		return "MEDIUM"
	case "RESTART", "REROUTE_TRAFFIC", "ISOLATE":
		return "HIGH"
	default:
		return "HIGH"
	}
}

func actionSignature(action *storage.QueuedAction) string {
	secret := os.Getenv("AEGIS_ACTION_SIGNING_SECRET")
	if secret == "" {
		secret = "development-only-action-secret"
	}
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(action.ID + "|" + action.AppID + "|" + action.ServiceID + "|" + action.Action + "|" + action.ExpiresAt.UTC().Format(time.RFC3339Nano)))
	return hex.EncodeToString(mac.Sum(nil))
}

type proposeActionRequest struct {
	ServiceID      string                 `json:"service_id"`
	Action         string                 `json:"action"`
	Evidence       map[string]interface{} `json:"evidence"`
	ExpectedEffect string                 `json:"expected_effect"`
	Parameters     map[string]string      `json:"parameters"`
	TTLSeconds     int                    `json:"ttl_seconds"`
}

func ProposeApplicationActionHandler(w http.ResponseWriter, r *http.Request) {
	appIDValue := mux.Vars(r)["id"]
	app, err := GetAuthStore().GetApplication(r.Context(), appIDValue)
	if err != nil {
		http.Error(w, `{"error":"application not found"}`, http.StatusNotFound)
		return
	}
	var req proposeActionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid action payload"}`, http.StatusBadRequest)
		return
	}
	var service *storage.ApplicationService
	for i := range app.Services {
		if app.Services[i].ID == req.ServiceID {
			service = &app.Services[i]
			break
		}
	}
	if service == nil {
		http.Error(w, `{"error":"unknown service_id"}`, http.StatusBadRequest)
		return
	}
	action := strings.ToUpper(strings.TrimSpace(req.Action))
	allowed := false
	for _, candidate := range service.AllowedActions {
		if strings.EqualFold(candidate, action) {
			allowed = true
		}
	}
	if !allowed {
		http.Error(w, `{"error":"action is not allowed for this service"}`, http.StatusForbidden)
		LogAudit(r, "application.action.blocked", req.ServiceID, riskForAppAction(action), "", action, "BLOCKED", "action not allowed")
		return
	}
	ttl := time.Duration(req.TTLSeconds) * time.Second
	if ttl <= 0 || ttl > time.Hour {
		ttl = 10 * time.Minute
	}
	queued := &storage.QueuedAction{ID: "act-" + fmt.Sprint(time.Now().UnixNano()), AppID: appIDValue, ServiceID: req.ServiceID, Action: action, RiskLevel: riskForAppAction(action), Evidence: req.Evidence, ExpectedEffect: req.ExpectedEffect, State: "PROPOSED", ExpiresAt: time.Now().UTC().Add(ttl), Parameters: req.Parameters, CreatedAt: time.Now().UTC()}
	queued.ETASeconds = 30
	if service.Mode == "auto_low" && queued.RiskLevel == "LOW" {
		queued.State = "APPROVED"
	}
	queued.SignedToken = actionSignature(queued)
	if err := GetAuthStore().CreateQueuedAction(r.Context(), queued); err != nil {
		http.Error(w, `{"error":"failed to queue action"}`, http.StatusInternalServerError)
		return
	}
	_ = json.NewEncoder(w).Encode(queued)
}

func ApproveApplicationActionHandler(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["action"]
	actions, err := GetAuthStore().ListQueuedActions(r.Context(), mux.Vars(r)["id"], []string{"PROPOSED"})
	if err != nil {
		http.Error(w, `{"error":"failed to load action"}`, http.StatusInternalServerError)
		return
	}
	var found *storage.QueuedAction
	for _, action := range actions {
		if action.ID == id {
			found = action
			break
		}
	}
	if found == nil {
		http.Error(w, `{"error":"action is expired or not proposed"}`, http.StatusNotFound)
		return
	}
	app, appErr := GetAuthStore().GetApplication(r.Context(), mux.Vars(r)["id"])
	if appErr != nil {
		http.Error(w, `{"error":"application not found"}`, http.StatusNotFound)
		return
	}
	for _, service := range app.Services {
		if service.ID == found.ServiceID && (service.Mode == "observe" || service.Mode == "recommend") {
			LogAudit(r, "application.action.blocked", found.ServiceID, found.RiskLevel, "REJECTED", found.Action, "BLOCKED", "service mode does not permit execution")
			http.Error(w, `{"error":"service mode does not permit execution"}`, http.StatusForbidden)
			return
		}
	}
	caller := GetCaller(r)
	if caller == nil {
		http.Error(w, `{"error":"approval identity required"}`, http.StatusForbidden)
		return
	}
	now := time.Now().UTC()
	found.State = "APPROVED"
	found.ApprovedBy = caller.Email
	found.ApprovedAt = &now
	if err := GetAuthStore().UpdateQueuedAction(r.Context(), found); err != nil {
		http.Error(w, `{"error":"failed to approve action"}`, http.StatusInternalServerError)
		return
	}
	LogAudit(r, "application.action.approve", found.ServiceID, found.RiskLevel, "APPROVED", found.Action, "SUCCESS", "")
	_ = json.NewEncoder(w).Encode(found)
}

var ingestLimiter = struct {
	sync.Mutex
	counts map[string][]time.Time
}{counts: map[string][]time.Time{}}

func allowIngest(key string) bool {
	ingestLimiter.Lock()
	defer ingestLimiter.Unlock()
	now := time.Now()
	cutoff := now.Add(-time.Minute)
	recent := ingestLimiter.counts[key][:0]
	for _, stamp := range ingestLimiter.counts[key] {
		if stamp.After(cutoff) {
			recent = append(recent, stamp)
		}
	}
	if len(recent) >= 120 {
		ingestLimiter.counts[key] = recent
		return false
	}
	ingestLimiter.counts[key] = append(recent, now)
	return true
}
