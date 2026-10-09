# OLM migration test fixtures

Small operators and file-based catalogs for OpenShift Console migration E2E tests.
The tests use real OLMv0 installations and OLMv1 migrations, without replacing API responses.

## Publishing

Every push to `main` updates the `main` image tag. Publishing a release such as
`v0.1.0` updates that release tag and `latest`. Publishing uses `GITHUB_TOKEN`
with `packages: write`. Make the GHCR packages public so cluster image pulls can access them.

The target catalog is `ghcr.io/logonoff/ocp-olm-catalog-fixtures:main`; the source
catalog uses `:source-main`. Releases publish `:v0.1.0`, `:latest`, `:source-v0.1.0`,
and `:source-latest`. Bundles and controllers also receive `main`, release, and
commit-specific tags. Catalogs reference the commit-specific bundle and controller
images so later pushes cannot change an existing catalog's content.

Images under `ghcr.io/logonoff/ocp-olm-catalog-fixtures`:

| Image | Purpose |
| --- | --- |
| `controller` | Reconciles Workload custom resources into operand Deployments; also serves the operand HTTP endpoint |
| `console-migration-a-bundle`, `console-migration-b-bundle` | Eligible, dependency-free AllNamespaces fixtures |
| `console-migration-own-namespace-bundle` | Installs successfully in its own namespace, but cannot migrate to AllNamespaces |
| `console-migration-no-target-bundle` | Present only in the source catalog; exercises missing target catalog content |
| `:source-main` | All four packages, served to OLMv0 through a CatalogSource |
| `:main` | Three packages, served to OLMv1 through a ClusterCatalog |

Each package owns a distinct CRD and has only namespaced RBAC. The controller
watches its installation namespace. Operand Deployments are owned by their Workload
custom resources, so removing the CSV does not delete or restart them. After
migration, creating a second Workload verifies that the OLMv1-managed controller
still reconciles resources.

## Local validation

```sh
go test ./...
go run ./cmd/generate --version 0.1.0
```

Generated bundle manifests and catalog package/channel entries are in `dist/`.
The workflow renders bundle entries with `opm` and validates both complete catalogs
before publishing. Bundle and catalog JSON files are valid YAML as well.

## Console E2E configuration

The console tests use the hardcoded `main` catalog tags by default. To use a
release or mirrored catalogs, override these in `frontend/e2e/.env`:

```sh
OLM_MIGRATION_SOURCE_CATALOG_IMAGE=ghcr.io/logonoff/ocp-olm-catalog-fixtures:source-v0.1.0
OLM_MIGRATION_TARGET_CATALOG_IMAGE=ghcr.io/logonoff/ocp-olm-catalog-fixtures:v0.1.0
```

Use a disposable Tech Preview cluster with both OLM versions and the experimental
ClusterObjectSet API enabled. The suite creates its own CatalogSources, ClusterCatalogs,
namespaces, and admission policies, and tracks them for cleanup. Policies match only
one test fixture, never customer operators. Run the console's `olm` Playwright project.

For disconnected runs, mirror all bundle, controller, and catalog images and configure
image digest mirror rules. Set the two catalog image variables to the mirrored images.
The console suite exercises the same workflow in either environment; it does not
verify network isolation itself.

## License

[WTFPL version 2](LICENSE).
