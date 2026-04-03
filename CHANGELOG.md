# Changelog

## 1.0.0 (2026-04-03)


### Features

* add Helm chart for operator deployment ([d3724ee](https://github.com/thereisnotime/warpgate-operator/commit/d3724ee4a7f815a6199be66896d425b20ef94b72))
* add validation and defaulting webhooks for all 9 CRDs ([8bffed2](https://github.com/thereisnotime/warpgate-operator/commit/8bffed2fb6058764613424fd9f915164fe6a78b0))
* complete webhook validation with cert-manager, tests, and docs ([6ae468a](https://github.com/thereisnotime/warpgate-operator/commit/6ae468aac36efd1ae8033fc4613f7475ff2419cc))
* implement all 9 CRDs, controllers, and reconciliation logic ([54ffb20](https://github.com/thereisnotime/warpgate-operator/commit/54ffb20e8ea3ee5c3b8b1f5ae95ddd28d88893b9))


### Bug Fixes

* add CODECOV_TOKEN to coverage upload action ([b1972cc](https://github.com/thereisnotime/warpgate-operator/commit/b1972ccf5756fecbc2ecfd16e52f58ac6abc1210))
* add pull-requests write permission for release-please ([adc9bd0](https://github.com/thereisnotime/warpgate-operator/commit/adc9bd07ae470c547e7617de01fb3bf9f2bf2db1))
* **deps:** patch CVE in otel SDK and grpc transitive dependencies ([8703c9b](https://github.com/thereisnotime/warpgate-operator/commit/8703c9b623c3ca8538f1ddd12d358ebad2c16299))
* fix container build for podman in CI ([20fab79](https://github.com/thereisnotime/warpgate-operator/commit/20fab79427c21f9c098b647a1bec9f3e435f24fe))
* include all 9 CRDs in kustomize config (was only WarpgateConnection) ([e98b455](https://github.com/thereisnotime/warpgate-operator/commit/e98b455333c99ddd9e5ff22d4199a0dc48e3491e))
* make minikube-deploy fully automated and deterministic ([fa0cf4b](https://github.com/thereisnotime/warpgate-operator/commit/fa0cf4b2252f6749522e289d43c5d8a9eac44ea7))
* remove all stale token references from docs, charts, and CRD descriptions ([d3a20d0](https://github.com/thereisnotime/warpgate-operator/commit/d3a20d0538888cdd6a0c5917f02314de23159fea))
* resolve all lint warnings in config YAML and fix justfile tool caching ([a60bf4c](https://github.com/thereisnotime/warpgate-operator/commit/a60bf4c18f34c2800acffeb7bec97968de341231))
* resolve CI lint failures, add CRD docs, and improve README ([3193554](https://github.com/thereisnotime/warpgate-operator/commit/3193554f155de1ae4555b2fadcc3e7f525749fb1))
* suppress gosec false positives with #nosec annotations ([85777cb](https://github.com/thereisnotime/warpgate-operator/commit/85777cb237be1960b43ac55835d6f4d8c4b5803a))
* switch from token auth to session-based cookie auth ([7b1dbb3](https://github.com/thereisnotime/warpgate-operator/commit/7b1dbb35714335a71e72db2c9423c45154b658b3))
* use docker for E2E builds so kind can load the image ([835e34b](https://github.com/thereisnotime/warpgate-operator/commit/835e34b1b2146a78665478be81ca0596a5b0fc93))
* use relative path in Containerfile go build for Go 1.26 compat ([2d7febc](https://github.com/thereisnotime/warpgate-operator/commit/2d7febcc68bcb8d90253d3aea42d95961f909501))
