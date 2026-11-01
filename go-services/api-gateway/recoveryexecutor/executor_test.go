package recoveryexecutor

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestWebhookExecutorShopKartActions(t *testing.T) {
	var gotToken string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotToken = r.Header.Get("X-Admin-Token")
		if r.URL.Path != "/admin/scale" || r.URL.Query().Get("replicas") != "3" {
			t.Fatalf("unexpected request: %s?%s", r.URL.Path, r.URL.RawQuery)
		}
		w.WriteHeader(http.StatusAccepted)
	}))
	defer server.Close()

	executor := NewWebhookExecutor(nil)
	result, err := executor.Execute(context.Background(), Action{Type: "SCALE_UP"}, Node{
		ID: "shopkart", BaseURL: server.URL, AuthToken: "server-only-token", Replicas: 2,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.StatusCode != http.StatusAccepted || gotToken != "server-only-token" {
		t.Fatalf("unexpected result: %#v token=%q", result, gotToken)
	}
	if strings.Contains(result.Request, "server-only-token") {
		t.Fatal("request audit data must not contain the auth token")
	}
}

func TestWebhookExecutorDoesNotRetryUnauthorized(t *testing.T) {
	attempts := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer server.Close()

	_, err := NewWebhookExecutor(nil).Execute(context.Background(), Action{Type: "RESTART"}, Node{ID: "shopkart", BaseURL: server.URL})
	if err == nil || attempts != 1 {
		t.Fatalf("expected one unauthorized attempt, attempts=%d err=%v", attempts, err)
	}
}
