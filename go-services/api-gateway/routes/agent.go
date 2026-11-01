package routes

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/aegis/go-services/api-gateway/storage"
)

func appKeyCaller(r *http.Request, scope string) (*storage.CallerContext, error) {
	caller := GetCaller(r)
	if caller == nil || !caller.IsAPIKey || caller.APIKeyObj == nil || caller.APIKeyObj.AppID == "" || !caller.APIKeyObj.HasScope(scope) {
		return nil, storage.ErrNotFound
	}
	return caller, nil
}

func AgentPollHandler(w http.ResponseWriter, r *http.Request) {
	caller, err := appKeyCaller(r, "agent:poll")
	if err != nil {
		http.Error(w, `{"error":"application API key required"}`, http.StatusUnauthorized)
		return
	}
	actions, err := GetAuthStore().ListQueuedActions(r.Context(), caller.APIKeyObj.AppID, []string{"APPROVED", "DISPATCHED"})
	if err != nil {
		http.Error(w, `{"error":"failed to poll actions"}`, http.StatusInternalServerError)
		return
	}
	for _, action := range actions {
		if action.State == "APPROVED" {
			action.State = "DISPATCHED"
			_ = GetAuthStore().UpdateQueuedAction(r.Context(), action)
		}
	}
	_ = json.NewEncoder(w).Encode(actions)
}

type agentResult struct {
	ActionID    string `json:"action_id"`
	Status      string `json:"status"`
	Logs        string `json:"logs"`
	SignedToken string `json:"signed_token"`
}

func AgentResultHandler(w http.ResponseWriter, r *http.Request) {
	caller, err := appKeyCaller(r, "agent:result")
	if err != nil {
		http.Error(w, `{"error":"application API key required"}`, http.StatusUnauthorized)
		return
	}
	var result agentResult
	if err := json.NewDecoder(r.Body).Decode(&result); err != nil {
		http.Error(w, `{"error":"invalid result payload"}`, http.StatusBadRequest)
		return
	}
	actions, err := GetAuthStore().ListQueuedActions(r.Context(), caller.APIKeyObj.AppID, []string{})
	if err != nil {
		http.Error(w, `{"error":"failed to load action"}`, http.StatusInternalServerError)
		return
	}
	var action *storage.QueuedAction
	for _, candidate := range actions {
		if candidate.ID == result.ActionID {
			action = candidate
			break
		}
	}
	if action == nil || action.ExpiresAt.Before(time.Now()) {
		http.Error(w, `{"error":"unknown or expired action"}`, http.StatusNotFound)
		return
	}
	if action.State != "DISPATCHED" {
		http.Error(w, `{"error":"action has not been dispatched"}`, http.StatusConflict)
		return
	}
	if result.SignedToken == "" || result.SignedToken != action.SignedToken {
		http.Error(w, `{"error":"invalid action signature"}`, http.StatusForbidden)
		return
	}
	status := strings.ToUpper(result.Status)
	if status != "SUCCEEDED" && status != "FAILED" {
		http.Error(w, `{"error":"status must be SUCCEEDED or FAILED"}`, http.StatusBadRequest)
		return
	}
	action.State = status
	action.ResultLogs = result.Logs
	if err := GetAuthStore().UpdateQueuedAction(r.Context(), action); err != nil {
		http.Error(w, `{"error":"failed to record result"}`, http.StatusInternalServerError)
		return
	}
	actor := caller.Email
	LogAudit(r, "agent.action.result", action.ServiceID, action.RiskLevel, action.ApprovedBy, "action="+action.Action, status, "reported_by="+actor)
	_ = json.NewEncoder(w).Encode(map[string]interface{}{"accepted": true, "state": status})
}
