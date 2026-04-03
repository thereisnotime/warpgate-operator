# Changelog

## 1.0.0 (2026-04-03)


### Features

* add Helm chart for operator deployment ([d3724ee](https://github.com/thereisnotime/warpgate-operator/commit/d3724ee4a7f815a6199be66896d425b20ef94b72))
* implement all 9 CRDs, controllers, and reconciliation logic ([54ffb20](https://github.com/thereisnotime/warpgate-operator/commit/54ffb20e8ea3ee5c3b8b1f5ae95ddd28d88893b9))


### Bug Fixes

* add CODECOV_TOKEN to coverage upload action ([b1972cc](https://github.com/thereisnotime/warpgate-operator/commit/b1972ccf5756fecbc2ecfd16e52f58ac6abc1210))
* add pull-requests write permission for release-please ([adc9bd0](https://github.com/thereisnotime/warpgate-operator/commit/adc9bd07ae470c547e7617de01fb3bf9f2bf2db1))
* **deps:** patch CVE in otel SDK and grpc transitive dependencies ([8703c9b](https://github.com/thereisnotime/warpgate-operator/commit/8703c9b623c3ca8538f1ddd12d358ebad2c16299))
* fix container build for podman in CI ([20fab79](https://github.com/thereisnotime/warpgate-operator/commit/20fab79427c21f9c098b647a1bec9f3e435f24fe))
* resolve all lint warnings in config YAML and fix justfile tool caching ([a60bf4c](https://github.com/thereisnotime/warpgate-operator/commit/a60bf4c18f34c2800acffeb7bec97968de341231))
* resolve CI lint failures, add CRD docs, and improve README ([3193554](https://github.com/thereisnotime/warpgate-operator/commit/3193554f155de1ae4555b2fadcc3e7f525749fb1))
* suppress gosec false positives with #nosec annotations ([85777cb](https://github.com/thereisnotime/warpgate-operator/commit/85777cb237be1960b43ac55835d6f4d8c4b5803a))
* use docker for E2E builds so kind can load the image ([835e34b](https://github.com/thereisnotime/warpgate-operator/commit/835e34b1b2146a78665478be81ca0596a5b0fc93))
* use relative path in Containerfile go build for Go 1.26 compat ([2d7febc](https://github.com/thereisnotime/warpgate-operator/commit/2d7febcc68bcb8d90253d3aea42d95961f909501))
