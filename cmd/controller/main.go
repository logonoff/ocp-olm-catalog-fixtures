package main

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"
)

type kubeClient struct {
	client                               *http.Client
	base, token, namespace, group, image string
	allNamespaces                        bool
}

func main() {
	http.HandleFunc("/health", func(w http.ResponseWriter, _ *http.Request) { fmt.Fprintln(w, "ready") })
	if len(os.Args) > 1 && os.Args[1] == "operand" {
		http.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) { fmt.Fprintln(w, "migration fixture operand") })
		log.Fatal(http.ListenAndServe(":8000", nil))
	}
	c, err := newKubeClient()
	if err != nil {
		log.Fatal(err)
	}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer stop()
	go func() { log.Fatal(http.ListenAndServe(":8000", nil)) }()
	for {
		if err := c.reconcile(ctx); err != nil {
			log.Printf("reconcile: %v", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(time.Second):
		}
	}
}

func newKubeClient() (*kubeClient, error) {
	const serviceAccount = "/var/run/secrets/kubernetes.io/serviceaccount/"
	token, err := os.ReadFile(serviceAccount + "token")
	if err != nil {
		return nil, err
	}
	ca, err := os.ReadFile(serviceAccount + "ca.crt")
	if err != nil {
		return nil, err
	}
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(ca) {
		return nil, fmt.Errorf("invalid service account CA")
	}
	return &kubeClient{
		client:        &http.Client{Timeout: 10 * time.Second, Transport: &http.Transport{TLSClientConfig: &tls.Config{RootCAs: pool, MinVersion: tls.VersionTLS12}}},
		base:          "https://" + os.Getenv("KUBERNETES_SERVICE_HOST") + ":" + os.Getenv("KUBERNETES_SERVICE_PORT_HTTPS"),
		allNamespaces: os.Getenv("WATCH_ALL_NAMESPACES") == "true",
		token:         strings.TrimSpace(string(token)), namespace: os.Getenv("POD_NAMESPACE"), group: os.Getenv("WORKLOAD_GROUP"), image: os.Getenv("OPERAND_IMAGE"),
	}, nil
}

func (c *kubeClient) request(ctx context.Context, method, path string, body any) ([]byte, int, error) {
	var data []byte
	var err error
	if body != nil {
		data, err = json.Marshal(body)
		if err != nil {
			return nil, 0, err
		}
	}
	req, err := http.NewRequestWithContext(ctx, method, c.base+path, bytes.NewReader(data))
	if err != nil {
		return nil, 0, err
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Content-Type", "application/json")
	res, err := c.client.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer res.Body.Close()
	data, err = io.ReadAll(res.Body)
	return data, res.StatusCode, err
}

func (c *kubeClient) reconcile(ctx context.Context) error {
	listPath := "/apis/" + c.group + "/v1alpha1"
	if !c.allNamespaces {
		listPath += "/namespaces/" + c.namespace
	}
	data, status, err := c.request(ctx, http.MethodGet, listPath+"/workloads", nil)
	if err != nil {
		return err
	}
	if status != http.StatusOK {
		return fmt.Errorf("list workloads: HTTP %d: %s", status, data)
	}
	var list struct {
		Items []struct {
			Metadata struct{ Name, UID, Namespace string }
			Spec     struct{ Enabled *bool }
		}
	}
	if err := json.Unmarshal(data, &list); err != nil {
		return err
	}
	for _, item := range list.Items {
		namespace := item.Metadata.Namespace
		if namespace == "" {
			namespace = c.namespace
		}
		path := "/apis/apps/v1/namespaces/" + namespace + "/deployments"
		_, status, err := c.request(ctx, http.MethodGet, path+"/"+item.Metadata.Name, nil)
		if err != nil {
			return err
		}
		if status == http.StatusOK {
			continue
		}
		if status != http.StatusNotFound {
			return fmt.Errorf("get operand: HTTP %d", status)
		}
		body := deployment(namespace, item.Metadata.Name, item.Metadata.UID, c.group, c.image)
		// Disabled operands still prove reconciliation without starting probe pods.
		if item.Spec.Enabled != nil && !*item.Spec.Enabled {
			body["spec"].(map[string]any)["replicas"] = 0
		}
		data, status, err := c.request(ctx, http.MethodPost, path, body)
		if err != nil {
			return err
		}
		if status != http.StatusCreated && status != http.StatusConflict {
			return fmt.Errorf("create operand: HTTP %d: %s", status, data)
		}
	}
	return nil
}

func deployment(namespace, name, uid, group, image string) map[string]any {
	labels := map[string]string{"app": name, "migration-fixture": "operand"}
	return map[string]any{
		"apiVersion": "apps/v1", "kind": "Deployment",
		"metadata": map[string]any{"name": name, "namespace": namespace, "ownerReferences": []any{map[string]any{"apiVersion": group + "/v1alpha1", "kind": "Workload", "name": name, "uid": uid, "controller": true}}},
		"spec": map[string]any{"replicas": 1, "selector": map[string]any{"matchLabels": labels}, "template": map[string]any{
			"metadata": map[string]any{"labels": labels},
			"spec": map[string]any{"securityContext": map[string]any{"runAsNonRoot": true, "seccompProfile": map[string]string{"type": "RuntimeDefault"}}, "containers": []any{map[string]any{
				"name": "operand", "image": image, "args": []string{"operand"},
				"securityContext": map[string]any{"allowPrivilegeEscalation": false, "capabilities": map[string]any{"drop": []string{"ALL"}}},
				"readinessProbe":  map[string]any{"httpGet": map[string]any{"path": "/health", "port": 8000}},
			}}},
		}},
	}
}
