package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestReconcileRetainsExistingOperand(t *testing.T) {
	for _, exists := range []bool{true, false} {
		t.Run(map[bool]string{true: "existing", false: "missing"}[exists], func(t *testing.T) {
			creates := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch {
				case r.URL.Path == "/apis/test/v1alpha1/namespaces/ns/workloads":
					w.Write([]byte(`{"items":[{"metadata":{"name":"operand","uid":"abc"}}]}`))
				case r.Method == http.MethodGet:
					if !exists {
						w.WriteHeader(http.StatusNotFound)
					}
				case r.Method == http.MethodPost:
					creates++
					w.WriteHeader(http.StatusCreated)
				default:
					t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
				}
			}))
			defer server.Close()
			c := kubeClient{client: server.Client(), base: server.URL, namespace: "ns", group: "test", image: "image"}
			if err := c.reconcile(context.Background()); err != nil {
				t.Fatal(err)
			}
			if exists && creates != 0 || !exists && creates != 1 {
				t.Fatalf("creates = %d", creates)
			}
		})
	}
}

func TestReconcileAcrossNamespaces(t *testing.T) {
	for _, tt := range []struct {
		name     string
		enabled  bool
		replicas int
	}{
		{name: "running operand", enabled: true, replicas: 1},
		{name: "reconciliation probe", enabled: false, replicas: 0},
	} {
		t.Run(tt.name, func(t *testing.T) {
			creates := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch {
				case r.Method == http.MethodGet && r.URL.Path == "/apis/test/v1alpha1/workloads":
					json.NewEncoder(w).Encode(map[string]any{"items": []any{map[string]any{"metadata": map[string]string{"name": "operand", "namespace": "other", "uid": "abc"}, "spec": map[string]bool{"enabled": tt.enabled}}}})
				case r.Method == http.MethodGet && r.URL.Path == "/apis/apps/v1/namespaces/other/deployments/operand":
					w.WriteHeader(http.StatusNotFound)
				case r.Method == http.MethodPost && r.URL.Path == "/apis/apps/v1/namespaces/other/deployments":
					creates++
					var body struct {
						Metadata struct{ Namespace string }
						Spec     struct{ Replicas int }
					}
					if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
						t.Error(err)
					}
					if body.Metadata.Namespace != "other" || body.Spec.Replicas != tt.replicas {
						t.Errorf("unexpected deployment: %+v", body)
					}
					w.WriteHeader(http.StatusCreated)
				default:
					t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
					w.WriteHeader(http.StatusBadRequest)
				}
			}))
			defer server.Close()
			c := kubeClient{client: server.Client(), base: server.URL, namespace: "operators", group: "test", image: "image", allNamespaces: true}
			if err := c.reconcile(context.Background()); err != nil {
				t.Fatal(err)
			}
			if creates != 1 {
				t.Fatalf("creates = %d", creates)
			}
		})
	}
}
