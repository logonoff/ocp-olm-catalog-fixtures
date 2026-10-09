package main

import (
	"context"
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
