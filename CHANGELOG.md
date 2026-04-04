# Changelog

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
