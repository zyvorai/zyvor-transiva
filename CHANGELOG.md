# Changelog

All notable changes to Transiva are recorded here. The format follows
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/).

## [Unreleased]

### Changed (dependency and toolchain upgrades)

- `go get -u ./...` plus `go mod tidy`; `go` directive raised to 1.27.4.
  Notable bumps: govmomi v0.56.2, aws-sdk-go-v2 v1.48.0 (and service modules),
  azcore v1.24.0, google.golang.org/api v0.316.0, oci-go-sdk v65.131.0,
  kubevirt.io/client-go and api v1.10.0, golang.org/x/crypto v0.59.0,
  kin-openapi v0.153.0, nutanix clustermgmt v4.4.1.
- OpenStack provider migrated from `github.com/gophercloud/gophercloud` v1 to
  `github.com/gophercloud/gophercloud/v2` (context-first API, `image/v2` package
  path, `ResponseCodeIs` for 404 checks).
- GitHub Actions: `actions/setup-go` v5/v6 -> v7, `actions/setup-node` v4 -> v7;
  Node pins 20/22 -> 24. No Python pins exist in this repo.
- Dockerfile runtime base `fedora-minimal:43` -> `44` (tag verified on the
  Fedora registry). The builder stays on the floating `golang:1.27-alpine`
  (1.27.4-alpine verified to exist).

### Held back (with reasons)

- `k8s.io/api`, `apimachinery`, `client-go`, `apiextensions-apiserver` stay on
  v0.36.4 and `sigs.k8s.io/controller-runtime` on v0.24.1: v0.37.x of client-go
  drops `kubernetes/typed/scheduling/v1alpha2`, which `kubevirt.io/client-go`
  v1.10.0 still imports, so the build fails. Revisit when kubevirt.io ships
  support for client-go 0.37.
- `github.com/aliyun/aliyun-oss-go-sdk` stays on v3.0.2+incompatible: a `/v3`
  module path exists on the Go proxy but was not migrated in this pass (not
  verified API-compatible, no OSS endpoint to test against).
- `go.opentelemetry.io/otel/exporters/jaeger` v1.17.0 stays: the module is
  deprecated upstream (use OTLP); replacing it is a behaviour change, not a bump.
- CI Go matrix stays on 1.27.x only: `go.mod` requires 1.27.x, so a 1.26.x leg
  cannot build this module.
- `docker-compose.yml` third-party images (`redis:7-alpine`,
  `prom/prometheus:v2.48.1`, `grafana/grafana:10.2.3`): newer majors exist
  (redis 8-alpine, prometheus v3.16.0, grafana 13.2.0, tags verified on Docker
  Hub) but config and dashboard compatibility could not be tested here without
  Docker, so they are left alone.
- `fedora:41` / `rockylinux:9` in the release RPM matrix: left, the `fc41` /
  `el9` dist labels are part of the artifact names.
- Unverified action pins left unchanged: `actions/cache@v6`,
  `actions/upload-artifact@v4`, `codecov/codecov-action@v7`,
  `golangci/golangci-lint-action@v9`, `github/codeql-action/upload-sarif@v4`,
  `aquasecurity/trivy-action@master`, `softprops/action-gh-release@v3`,
  `helm/kind-action@v1`, `azure/setup-kubectl@v5`,
  `DavidAnson/markdownlint-cli2-action@v25`, `peter-evans/create-pull-request@v5`,
  `medyagh/setup-minikube@latest`, `ludeeus/action-shellcheck@master`,
  `actions/upload-release-asset@v1`.
- Already at the verified majors: checkout v7, download-artifact v8,
  upload-pages-artifact v5, deploy-pages v5, build-push-action v7,
  setup-buildx v4, login-action v4, metadata-action v6, azure/setup-helm v5.
