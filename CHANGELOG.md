# Changelog

## [0.4.4](https://github.com/thereisnotime/warpgate-operator/compare/v0.4.3...v0.4.4) (2026-04-13)


### Bug Fixes

* **release:** add setup-oras step, pin Helm to v3.20.2 in release job ([23552a5](https://github.com/thereisnotime/warpgate-operator/commit/23552a5a6055e1d86683562384fe4b755c7a90e2))
* **release:** auto-sync Chart.yaml version after release-please cuts a release ([bd5e528](https://github.com/thereisnotime/warpgate-operator/commit/bd5e528b3f6ea0fa13a726ce81941e228d730507))
* **release:** use x-release-please-version annotations in Chart.yaml ([ec065ce](https://github.com/thereisnotime/warpgate-operator/commit/ec065ce0457b2bee0f7e8919b2e8a22ebce6b7bc))

## [0.4.3](https://github.com/thereisnotime/warpgate-operator/compare/v0.4.2...v0.4.3) (2026-04-13)


### Bug Fixes

* bump opentelemetry-go to 1.43.0 to resolve CVE-2026-39883 ([643b8e7](https://github.com/thereisnotime/warpgate-operator/commit/643b8e755b67f695e89184d1d770b0879d77be00))
* **ci:** pin Helm to v3.20.2 to avoid Helm 4 breaking changes ([c7fd8e2](https://github.com/thereisnotime/warpgate-operator/commit/c7fd8e28d8975ca08b91784d3401d05afd14402f))
* **ci:** pin kubeconform with checksum, install cert-manager for kind tests ([089d94d](https://github.com/thereisnotime/warpgate-operator/commit/089d94da35813594ac79634dc0e27047a4c7cbc4))
* **ci:** replace kubectl dry-run with kubeconform for manifest validation ([e460d51](https://github.com/thereisnotime/warpgate-operator/commit/e460d51c8e9a5c4d40b8b79841ce3aef476010a2))
* **ci:** scope security-events permission to SARIF jobs, fix coverage gate ([5691d8f](https://github.com/thereisnotime/warpgate-operator/commit/5691d8f12424f386262239b5256081194ecbe865))
* **ci:** use go install for gosec instead of Docker action ([261204a](https://github.com/thereisnotime/warpgate-operator/commit/261204a37d7c3c3de02f3f72134990f8c6b75e33))
* **release:** properly bump Chart.yaml on release, add Artifact Hub metadata ([738dfc4](https://github.com/thereisnotime/warpgate-operator/commit/738dfc4094fc2f63d466ae91c98c3c731ff1ea12))
* **release:** push container image with version tag (no v prefix), add ct-values ([442b053](https://github.com/thereisnotime/warpgate-operator/commit/442b0536cc2b83f8269c000d9e541228f920dd6d))
* resolve three pipeline failures from the best-practices commit ([d05ddf1](https://github.com/thereisnotime/warpgate-operator/commit/d05ddf1d9505e9ed4ed0002f59faf1c2877ffdd7))
* restore govulncheck and coverage gate with smarter logic ([e4f134b](https://github.com/thereisnotime/warpgate-operator/commit/e4f134b9f57398e7ad032734cd80315afa8274a9))
* upgrade to go1.26.2 and fix scorecard permissions ([6a90fde](https://github.com/thereisnotime/warpgate-operator/commit/6a90fde9e899bf5aeb3c9be6be68698958487aa0))

## [0.4.2](https://github.com/thereisnotime/warpgate-operator/compare/v0.4.1...v0.4.2) (2026-04-04)


### Bug Fixes

* use X-Warpgate-Token header instead of Authorization Bearer ([40bcd7f](https://github.com/thereisnotime/warpgate-operator/commit/40bcd7f926f56c97d451a7c6b45b53aa81e7c059))

## [0.4.1](https://github.com/thereisnotime/warpgate-operator/compare/v0.4.0...v0.4.1) (2026-04-04)


### Bug Fixes

* add missing RBAC for WarpgateInstance, Deployments, ConfigMaps, PVCs, cert-manager ([8adca00](https://github.com/thereisnotime/warpgate-operator/commit/8adca00ffd01f587c8de336b60a9c5f4fdb2386d))

## [0.4.0](https://github.com/thereisnotime/warpgate-operator/compare/v0.3.0...v0.4.0) (2026-04-04)


### Features

* align WarpgateInstance with official Warpgate Helm chart ([a3a01ba](https://github.com/thereisnotime/warpgate-operator/commit/a3a01ba4ed8beef51e15396ee45aebf1e3e01a34))


### Bug Fixes

* improve WarpgateInstance lifecycle management ([e31ef7e](https://github.com/thereisnotime/warpgate-operator/commit/e31ef7e52948a5c38cd5a931ab467e2cad35ee0e))

## [0.3.0](https://github.com/thereisnotime/warpgate-operator/compare/v0.2.1...v0.3.0) (2026-04-04)


### Features

* add WarpgateInstance CRD to deploy and manage Warpgate servers ([e9a8266](https://github.com/thereisnotime/warpgate-operator/commit/e9a8266061fec37b949414fd1a0721eba1c81cd3))
* support bearer token auth for OTP-enabled Warpgate instances ([915e1c6](https://github.com/thereisnotime/warpgate-operator/commit/915e1c6897c59a2ea85e29c0c6674de04555c62c))


### Bug Fixes

* add --validate=false to helm-test dry-run (no cluster available) ([591d76a](https://github.com/thereisnotime/warpgate-operator/commit/591d76ae0beb51501b80e67d25217b25409dd018))
* create Kind cluster before installing cert-manager in E2E workflow ([694db65](https://github.com/thereisnotime/warpgate-operator/commit/694db6585a1b3fe58944be6150276e2fdaad6ab9))
* properly fix E2E tests for Kind + cert-manager + webhooks ([0457159](https://github.com/thereisnotime/warpgate-operator/commit/0457159273ddcdbf396e88ffb26d8976a2b97750))
* rewrite just test-e2e as proper local smoke tests ([255fc9f](https://github.com/thereisnotime/warpgate-operator/commit/255fc9f5b555d883413df34fe526c1a1c096aa0a))

## [0.2.1](https://github.com/thereisnotime/warpgate-operator/compare/v0.2.0...v0.2.1) (2026-04-03)


### Bug Fixes

* auto-sync generated CRDs to Helm chart templates ([80baffa](https://github.com/thereisnotime/warpgate-operator/commit/80baffa0de7874b599e1ceae3a096cfa21f36838))
* pass IMG as env var to just build-installer in release workflow ([5a42424](https://github.com/thereisnotime/warpgate-operator/commit/5a424247960ce26a3e7668ce24a83b65f6c52001))

## [0.2.0](https://github.com/thereisnotime/warpgate-operator/compare/v0.1.0...v0.2.0) (2026-04-03)


### Features

* add configurable secret keys to WarpgateConnection ([f8fe760](https://github.com/thereisnotime/warpgate-operator/commit/f8fe7603faaf8e246faab4510b1041a15ed9c6f4))
