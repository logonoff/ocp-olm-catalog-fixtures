package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
)

type object = map[string]any

func main() {
	registry := flag.String("registry", "ghcr.io/logonoff/ocp-olm-catalog-fixtures", "image prefix")
	version := flag.String("version", "0.1.0", "bundle and controller version")
	imageTag := flag.String("image-tag", "", "controller image tag (defaults to version)")
	flag.Parse()
	if *registry == "" {
		log.Fatal("--registry is required")
	}
	if *imageTag == "" {
		*imageTag = *version
	}
	for _, name := range []string{"console-migration-a", "console-migration-b", "console-migration-own-namespace", "console-migration-no-target"} {
		generate(name, *registry, *version, *imageTag)
	}
}

func writeJSON(path string, value any) {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		log.Fatal(err)
	}
	write(path, string(data)+"\n")
}

func write(path, value string) {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		log.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(value), 0644); err != nil {
		log.Fatal(err)
	}
}

func generate(name, registry, version, imageTag string) {
	dir := filepath.Join("dist", "bundles", name)
	group := name + ".migration-fixtures.console.openshift.io"
	image := registry + "/controller:" + imageTag
	crd := "workloads." + group
	labels := object{"app": name}
	security := object{"allowPrivilegeEscalation": false, "capabilities": object{"drop": []string{"ALL"}}}
	container := object{
		"name": "controller", "image": image,
		"env":             []any{object{"name": "POD_NAMESPACE", "valueFrom": object{"fieldRef": object{"fieldPath": "metadata.namespace"}}}, object{"name": "WORKLOAD_GROUP", "value": group}, object{"name": "OPERAND_IMAGE", "value": image}},
		"securityContext": security,
		"readinessProbe":  object{"httpGet": object{"path": "/health", "port": 8000}},
		"resources":       object{"requests": object{"cpu": "10m", "memory": "16Mi"}, "limits": object{"memory": "64Mi"}},
	}
	mode := name != "console-migration-own-namespace"
	writeJSON(filepath.Join(dir, "manifests", "csv.yaml"), object{
		"apiVersion": "operators.coreos.com/v1alpha1", "kind": "ClusterServiceVersion",
		"metadata": object{"name": name + ".v" + version, "annotations": object{"description": "Operator migration E2E fixture", "capabilities": "Basic Install"}},
		"spec": object{
			"displayName": name, "description": "Creates a stable operand Deployment for console migration tests.", "version": version,
			"provider": object{"name": "Console migration fixtures"}, "maturity": "alpha",
			"installModes":              []any{object{"type": "OwnNamespace", "supported": true}, object{"type": "SingleNamespace", "supported": true}, object{"type": "MultiNamespace", "supported": false}, object{"type": "AllNamespaces", "supported": mode}},
			"customresourcedefinitions": object{"owned": []any{object{"name": crd, "version": "v1alpha1", "kind": "Workload", "displayName": "Migration workload", "description": "A stable migration test operand."}}},
			"install": object{"strategy": "deployment", "spec": object{
				"permissions": []any{object{"serviceAccountName": name, "rules": []any{
					object{"apiGroups": []string{group}, "resources": []string{"workloads"}, "verbs": []string{"get", "list"}},
					object{"apiGroups": []string{"apps"}, "resources": []string{"deployments"}, "verbs": []string{"get", "create"}},
				}}},
				"deployments": []any{object{"name": name, "spec": object{"replicas": 1, "selector": object{"matchLabels": labels}, "template": object{
					"metadata": object{"labels": labels}, "spec": object{"serviceAccountName": name, "securityContext": object{"runAsNonRoot": true, "seccompProfile": object{"type": "RuntimeDefault"}}, "containers": []any{container}},
				}}}},
			}},
		},
	})
	writeJSON(filepath.Join(dir, "manifests", "crd.yaml"), object{
		"apiVersion": "apiextensions.k8s.io/v1", "kind": "CustomResourceDefinition", "metadata": object{"name": crd},
		"spec": object{"group": group, "scope": "Namespaced", "names": object{"kind": "Workload", "plural": "workloads", "singular": "workload"}, "versions": []any{object{
			"name": "v1alpha1", "served": true, "storage": true,
			"schema": object{"openAPIV3Schema": object{"type": "object", "properties": object{"spec": object{"type": "object", "properties": object{"enabled": object{"type": "boolean"}}}}}},
		}}},
	})
	annotations := object{"annotations": object{
		"operators.operatorframework.io.bundle.mediatype.v1":       "registry+v1",
		"operators.operatorframework.io.bundle.manifests.v1":       "manifests/",
		"operators.operatorframework.io.bundle.metadata.v1":        "metadata/",
		"operators.operatorframework.io.bundle.package.v1":         name,
		"operators.operatorframework.io.bundle.channels.v1":        "stable",
		"operators.operatorframework.io.bundle.channel.default.v1": "stable",
	}}
	writeJSON(filepath.Join(dir, "metadata", "annotations.yaml"), annotations)
	write(filepath.Join(dir, "Dockerfile"), fmt.Sprintf(`FROM scratch
LABEL operators.operatorframework.io.bundle.mediatype.v1=registry+v1 \
      operators.operatorframework.io.bundle.manifests.v1=manifests/ \
      operators.operatorframework.io.bundle.metadata.v1=metadata/ \
      operators.operatorframework.io.bundle.package.v1=%s \
      operators.operatorframework.io.bundle.channels.v1=stable \
      operators.operatorframework.io.bundle.channel.default.v1=stable
COPY manifests/ /manifests/
COPY metadata/ /metadata/
`, name))
	for _, catalog := range []string{"source", "target"} {
		if catalog == "target" && name == "console-migration-no-target" {
			continue
		}
		writeJSON(filepath.Join("dist", "catalogs", catalog, name+"-package.json"), object{"schema": "olm.package", "name": name, "defaultChannel": "stable"})
		writeJSON(filepath.Join("dist", "catalogs", catalog, name+"-channel.json"), object{"schema": "olm.channel", "package": name, "name": "stable", "entries": []any{object{"name": name + ".v" + version}}})
	}
}
