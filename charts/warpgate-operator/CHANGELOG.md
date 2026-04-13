# Changelog

## [0.4.6](https://github.com/thereisnotime/warpgate-operator/compare/warpgate-operator-v0.4.5...warpgate-operator-v0.4.6) (2026-04-13)

### Bug Fixes

* **chart:** add README and artifacthub images annotation ([aefbb00](https://github.com/thereisnotime/warpgate-operator/commit/aefbb00b900464d62cf3aa5342a4df39db1dee75))
* markdown lint errors in CONTRIBUTORS.md and chart CHANGELOG ([aaff9e3](https://github.com/thereisnotime/warpgate-operator/commit/aaff9e345543c49f547347ebcb065131475faf8a))

## [0.4.5](https://github.com/thereisnotime/warpgate-operator/compare/warpgate-operator-v0.4.4...warpgate-operator-v0.4.5) (2026-04-13)

### Features

* add Helm chart for operator deployment ([d3724ee](https://github.com/thereisnotime/warpgate-operator/commit/d3724ee4a7f815a6199be66896d425b20ef94b72))
* add validation and defaulting webhooks for all 9 CRDs ([8bffed2](https://github.com/thereisnotime/warpgate-operator/commit/8bffed2fb6058764613424fd9f915164fe6a78b0))
* add WarpgateInstance CRD to deploy and manage Warpgate servers ([e9a8266](https://github.com/thereisnotime/warpgate-operator/commit/e9a8266061fec37b949414fd1a0721eba1c81cd3))
* align WarpgateInstance with official Warpgate Helm chart ([a3a01ba](https://github.com/thereisnotime/warpgate-operator/commit/a3a01ba4ed8beef51e15396ee45aebf1e3e01a34))
* complete webhook validation with cert-manager, tests, and docs ([6ae468a](https://github.com/thereisnotime/warpgate-operator/commit/6ae468aac36efd1ae8033fc4613f7475ff2419cc))
* **release:** independent versioning for operator and helm chart ([4359f22](https://github.com/thereisnotime/warpgate-operator/commit/4359f22760efbd9d714f7e8ce352f1f09a17ab0f))
* support bearer token auth for OTP-enabled Warpgate instances ([915e1c6](https://github.com/thereisnotime/warpgate-operator/commit/915e1c6897c59a2ea85e29c0c6674de04555c62c))

### Bug Fixes

* add missing RBAC for WarpgateInstance, Deployments, ConfigMaps, PVCs, cert-manager ([8adca00](https://github.com/thereisnotime/warpgate-operator/commit/8adca00ffd01f587c8de336b60a9c5f4fdb2386d))
* auto-sync generated CRDs to Helm chart templates ([80baffa](https://github.com/thereisnotime/warpgate-operator/commit/80baffa0de7874b599e1ceae3a096cfa21f36838))
* **release:** properly bump Chart.yaml on release, add Artifact Hub metadata ([738dfc4](https://github.com/thereisnotime/warpgate-operator/commit/738dfc4094fc2f63d466ae91c98c3c731ff1ea12))
* **release:** push container image with version tag (no v prefix), add ct-values ([442b053](https://github.com/thereisnotime/warpgate-operator/commit/442b0536cc2b83f8269c000d9e541228f920dd6d))
* **release:** use x-release-please-version annotations in Chart.yaml ([ec065ce](https://github.com/thereisnotime/warpgate-operator/commit/ec065ce0457b2bee0f7e8919b2e8a22ebce6b7bc))
* remove all stale token references from docs, charts, and CRD descriptions ([d3a20d0](https://github.com/thereisnotime/warpgate-operator/commit/d3a20d0538888cdd6a0c5917f02314de23159fea))
* use X-Warpgate-Token header instead of Authorization Bearer ([40bcd7f](https://github.com/thereisnotime/warpgate-operator/commit/40bcd7f926f56c97d451a7c6b45b53aa81e7c059))

## [0.4.4](https://github.com/thereisnotime/warpgate-operator/compare/charts/warpgate-operator-v0.4.3...charts/warpgate-operator-v0.4.4) (2026-04-13)

### Bug Fixes

* migrate to independent chart versioning via release-please multi-package config
