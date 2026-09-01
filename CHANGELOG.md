# Changelog

## [0.4.15](https://github.com/thereisnotime/warpgate-operator/compare/v0.4.14...v0.4.15) (2026-09-01)


### Features

* add configurable reconcile interval ([62017e0](https://github.com/thereisnotime/warpgate-operator/commit/62017e02307f9b1ea073f662f7a49cd7adc80308))
* configurable reconcile interval ([d7877e6](https://github.com/thereisnotime/warpgate-operator/commit/d7877e633489f9be9e4247417a18256ae78031ee))
* **helm:** add optional NetworkPolicy for the operator pod ([6503a60](https://github.com/thereisnotime/warpgate-operator/commit/6503a60870c92e4a28b168fa2d78364d179c0120))
* **helm:** add optional NetworkPolicy for the operator pod ([25e659b](https://github.com/thereisnotime/warpgate-operator/commit/25e659b7ebaea34aa84bf6691bc134e741751fd1))
* **helm:** add ServiceMonitor template for Prometheus Operator ([de43551](https://github.com/thereisnotime/warpgate-operator/commit/de435519a2165e72f44d97cad590c581aab6c490))
* **helm:** add ServiceMonitor template for Prometheus Operator ([006d9cb](https://github.com/thereisnotime/warpgate-operator/commit/006d9cb0cee415d0990912aa09a8f6bb35f6d9f8))


### Bug Fixes

* **ci:** add missing operator.reconcileInterval in values.yaml, gofmt ticket test ([09b8074](https://github.com/thereisnotime/warpgate-operator/commit/09b8074281c762e2a87d90d86f3003904b902d4f))
* **helm:** make reconcile-interval arg conditional, skip in CI install test ([d85a004](https://github.com/thereisnotime/warpgate-operator/commit/d85a00404a1e733174d588e52528639256e85a0a))
* **prometheus:** disable insecureSkipVerify in ServiceMonitor ([11386a7](https://github.com/thereisnotime/warpgate-operator/commit/11386a726e7e7626a271f4083a6603d227f2efe3))
* **prometheus:** disable insecureSkipVerify in ServiceMonitor ([6b9258a](https://github.com/thereisnotime/warpgate-operator/commit/6b9258a74400a13623ef04c68c4ca1e373e8ab76))

## [0.4.14](https://github.com/thereisnotime/warpgate-operator/compare/v0.4.13...v0.4.14) (2026-09-01)


### Features

* add CRD TargetGroup ([#82](https://github.com/thereisnotime/warpgate-operator/issues/82)) ([7b34e53](https://github.com/thereisnotime/warpgate-operator/commit/7b34e53b00b287500817e036be1756f3f0926400))

## [0.4.13](https://github.com/thereisnotime/warpgate-operator/compare/v0.4.12...v0.4.13) (2026-08-31)


### Features

* **controller:** HTTPRoute auto-discovery for WarpgateTarget ([#91](https://github.com/thereisnotime/warpgate-operator/issues/91)) ([f3817a3](https://github.com/thereisnotime/warpgate-operator/commit/f3817a3c6c677b2fff050852e19d8fdf104f9a6f))


### Bug Fixes

* treat 409 as success in CreateTargetRole and CreateUserRole ([#89](https://github.com/thereisnotime/warpgate-operator/issues/89)) ([ba5dfbd](https://github.com/thereisnotime/warpgate-operator/commit/ba5dfbda6a20f91ea83126dc4c7aebf1253a088a)), closes [#85](https://github.com/thereisnotime/warpgate-operator/issues/85)


### Dependencies

* **deps:** bump actions/checkout from 6.0.3 to 7.0.0 ([#72](https://github.com/thereisnotime/warpgate-operator/issues/72)) ([37579ea](https://github.com/thereisnotime/warpgate-operator/commit/37579ea7bb6f8410c321259b2d78c190d466c4fe))
* **deps:** bump actions/setup-go from 6.4.0 to 6.5.0 ([#79](https://github.com/thereisnotime/warpgate-operator/issues/79)) ([cae0866](https://github.com/thereisnotime/warpgate-operator/commit/cae0866282787438ae06cdd55a5d2cfba079f879))
* **deps:** bump azure/setup-helm from 5.0.0 to 5.0.1 ([#78](https://github.com/thereisnotime/warpgate-operator/issues/78)) ([4408a90](https://github.com/thereisnotime/warpgate-operator/commit/4408a90d11911c783ab7c8e655f2f3c3fd584148))
* **deps:** bump github.com/onsi/ginkgo/v2 from 2.28.3 to 2.31.0 ([#74](https://github.com/thereisnotime/warpgate-operator/issues/74)) ([af7e554](https://github.com/thereisnotime/warpgate-operator/commit/af7e554adcf1dd264c588d360a336f684e0e4810))
* **deps:** bump github.com/onsi/gomega from 1.41.0 to 1.42.0 ([#88](https://github.com/thereisnotime/warpgate-operator/issues/88)) ([e3d9553](https://github.com/thereisnotime/warpgate-operator/commit/e3d95539a10de8948ed040d008d2e82207d308c1))
* **deps:** bump github/codeql-action/upload-sarif ([#80](https://github.com/thereisnotime/warpgate-operator/issues/80)) ([5c5e5a9](https://github.com/thereisnotime/warpgate-operator/commit/5c5e5a96808bcbf7deb4278c35b78ee0961fabc0))
* **deps:** bump gitleaks/gitleaks-action from 2.3.9 to 3.0.0 ([#70](https://github.com/thereisnotime/warpgate-operator/issues/70)) ([bacc0b0](https://github.com/thereisnotime/warpgate-operator/commit/bacc0b015be8cc6dd8b446d2d18fc4ea9bd8cb95))
* **deps:** bump k8s.io/client-go from 0.36.1 to 0.36.2 ([#73](https://github.com/thereisnotime/warpgate-operator/issues/73)) ([b10d4c9](https://github.com/thereisnotime/warpgate-operator/commit/b10d4c933c433bba8e2abcda70dd6a9c4cd97df2))
* **deps:** bump library/golang from 1.26.6 to 1.27.0 ([#86](https://github.com/thereisnotime/warpgate-operator/issues/86)) ([da07686](https://github.com/thereisnotime/warpgate-operator/commit/da0768627375204f0018ef1e405606b52d77a4af))

## [0.4.12](https://github.com/thereisnotime/warpgate-operator/compare/v0.4.11...v0.4.12) (2026-08-19)


### Bug Fixes

* **deps:** bump grpc to v1.82.1 and x/text to v0.39.0 ([#83](https://github.com/thereisnotime/warpgate-operator/issues/83)) ([4d3ef94](https://github.com/thereisnotime/warpgate-operator/commit/4d3ef94ed2e33730a7d1ead4347480fae6fc0260))

## [0.4.11](https://github.com/thereisnotime/warpgate-operator/compare/v0.4.10...v0.4.11) (2026-06-11)


### Features

* **target:** add postgres protocol version ([2a898be](https://github.com/thereisnotime/warpgate-operator/commit/2a898be2d7172f77d418946eff92004577189e24))
* **target:** add postgres protocol version ([aab4ea3](https://github.com/thereisnotime/warpgate-operator/commit/aab4ea35c20a9df9f779494513dd7c754374f16c))

## [0.4.10](https://github.com/thereisnotime/warpgate-operator/compare/v0.4.9...v0.4.10) (2026-06-11)


### Features

* add rate limit and ticket configuration fields to targets ([1752f1f](https://github.com/thereisnotime/warpgate-operator/commit/1752f1f707b6c2995ae4b3be600059a36f8c9ad5))
* add rate limit and ticket configuration fields to targets ([ef3abc7](https://github.com/thereisnotime/warpgate-operator/commit/ef3abc702d89eb358366a8d1fdf1d0eee0f6f5b4))
* add SSH jump host support ([0714c51](https://github.com/thereisnotime/warpgate-operator/commit/0714c51e0592cfcfc3b6defd89427a2f95c599dc))
* add SSH jump host support ([b7ebd52](https://github.com/thereisnotime/warpgate-operator/commit/b7ebd52327c09fd500cc9de91a097780fa8eef1b))

## [0.4.9](https://github.com/thereisnotime/warpgate-operator/compare/v0.4.8...v0.4.9) (2026-06-11)


### Bug Fixes

* **api:** replace deprecated scheme.Builder with local implementation ([a9bb493](https://github.com/thereisnotime/warpgate-operator/commit/a9bb493bb99294dc50b7645e9eace682da159387))
* **deps:** upgrade golang.org/x/net to v0.55.0 ([66d9e1a](https://github.com/thereisnotime/warpgate-operator/commit/66d9e1abb58d1c12cbbef2dcc50cd3dd261f1dcb))


### Dependencies

* **deps:** bump aquasecurity/trivy-action from 0.35.0 to 0.36.0 ([d3d2aeb](https://github.com/thereisnotime/warpgate-operator/commit/d3d2aebca45ea17bfddc3c4f323cbb48019c33c5))
* **deps:** bump DavidAnson/markdownlint-cli2-action from 23.0.0 to 23.2.0 ([1f7bb45](https://github.com/thereisnotime/warpgate-operator/commit/1f7bb45462d1b4966d81bdbb7b1862823924f554))
* **deps:** bump github.com/onsi/ginkgo/v2 from 2.28.1 to 2.28.3 ([798905b](https://github.com/thereisnotime/warpgate-operator/commit/798905b8ef68f85819839d3ea8e7fdd3e7e2fc43))
* **deps:** bump github.com/onsi/gomega from 1.39.1 to 1.40.0 ([#45](https://github.com/thereisnotime/warpgate-operator/issues/45)) ([4518485](https://github.com/thereisnotime/warpgate-operator/commit/45184852a7bc874d8b139cb7248ee255a8405d3f))
* **deps:** bump github/codeql-action from 4.35.2 to 4.35.4 ([12b949d](https://github.com/thereisnotime/warpgate-operator/commit/12b949db37049af84173c278439911dfb2f1a4d9))
* **deps:** bump Go modules and GitHub Actions to latest ([2717f42](https://github.com/thereisnotime/warpgate-operator/commit/2717f42ffea4fd98421162ebb50f73e2f33d9b4c))
* **deps:** bump golang.org/x/net to v0.54.0 and related deps ([0f7ff49](https://github.com/thereisnotime/warpgate-operator/commit/0f7ff49703365fda7d83f06dd3a4ea2f0ddeafb1))
* **deps:** bump golang.org/x/net to v0.54.0 and related deps ([864b631](https://github.com/thereisnotime/warpgate-operator/commit/864b6315c330ed22d3d28a9ba0548f302aa96f8d))
* **deps:** bump googleapis/release-please-action from 4.4.1 to 5.0.0 ([b7b3609](https://github.com/thereisnotime/warpgate-operator/commit/b7b36096ea09a32e9ddb2d72ba2271f1f62b52fe))
* **deps:** bump k8s.io packages to 0.36.0 and controller-runtime to 0.24.0 ([3a71f5f](https://github.com/thereisnotime/warpgate-operator/commit/3a71f5f95baa6db96aeb493aed61e9b40585448d))
* **deps:** bump k8s.io/{api,apimachinery,client-go} to 0.36.0 and controller-runtime to 0.24.0 ([cf42a9a](https://github.com/thereisnotime/warpgate-operator/commit/cf42a9a309d2f1f4ce08e04b936734ba876f6d6c))
* **deps:** bump sigs.k8s.io/controller-runtime from 0.23.3 to 0.24.0 ([#47](https://github.com/thereisnotime/warpgate-operator/issues/47)) ([817acc9](https://github.com/thereisnotime/warpgate-operator/commit/817acc9ba06f1e8c163aef2ee4e91b486fdf3e8c))
* **deps:** bump sigstore/cosign-installer from 4.1.1 to 4.1.2 ([c0342b0](https://github.com/thereisnotime/warpgate-operator/commit/c0342b0e6759cd0ba6addfa8efc3fb888e278843))
* **deps:** tidy go.sum after dependency updates ([116abf6](https://github.com/thereisnotime/warpgate-operator/commit/116abf6cb3d5daf8aac7b7fb3e36078d12b196c9))
* **deps:** tidy go.sum after dependency updates ([380bc34](https://github.com/thereisnotime/warpgate-operator/commit/380bc34ee0db6d20347f3ede0c81593e8d280522))

## [0.4.8](https://github.com/thereisnotime/warpgate-operator/compare/v0.4.7...v0.4.8) (2026-04-22)


### Bug Fixes

* **ci:** include dependency updates in release changelogs ([280b52c](https://github.com/thereisnotime/warpgate-operator/commit/280b52cbbde3870f5c10b7d6ef4f8a07dd994415))


### Dependencies

* **deps:** bump actions/attest-build-provenance from 2.4.0 to 4.1.0 ([#32](https://github.com/thereisnotime/warpgate-operator/issues/32)) ([58d8bb5](https://github.com/thereisnotime/warpgate-operator/commit/58d8bb5634c7fa6d61d61fe65b93bf2220605ea1))
* **deps:** bump github/codeql-action from 3.35.1 to 4.35.2 ([#34](https://github.com/thereisnotime/warpgate-operator/issues/34)) ([449cb00](https://github.com/thereisnotime/warpgate-operator/commit/449cb004d217a74f8bce88cef97873f82a2e2313))
* **deps:** bump googleapis/release-please-action from 4.4.0 to 4.4.1 ([#35](https://github.com/thereisnotime/warpgate-operator/issues/35)) ([d5ac440](https://github.com/thereisnotime/warpgate-operator/commit/d5ac44071850d1299d134d23b9ba186cb300f400))
* **deps:** bump k8s.io/client-go from 0.35.3 to 0.35.4 ([#33](https://github.com/thereisnotime/warpgate-operator/issues/33)) ([6c35028](https://github.com/thereisnotime/warpgate-operator/commit/6c35028b09c753ee04d74e9f5a92cd8836261533))
* **deps:** bump sigstore/cosign-installer from 3.8.2 to 4.1.1 ([#31](https://github.com/thereisnotime/warpgate-operator/issues/31)) ([2439c2d](https://github.com/thereisnotime/warpgate-operator/commit/2439c2d9a88522bf699a59074b3acadf3d0ea583))

## [0.4.7](https://github.com/thereisnotime/warpgate-operator/compare/v0.4.6...v0.4.7) (2026-04-13)


### Bug Fixes

* **chart:** remove double blank line from CHANGELOG, use latest image tag in ArtifactHub annotation ([a51635e](https://github.com/thereisnotime/warpgate-operator/commit/a51635e339282f473e4990eb058d404a0460796d))
* **ci:** never cancel SAST runs so every commit gets coverage ([fbdde2b](https://github.com/thereisnotime/warpgate-operator/commit/fbdde2ba6d81fcec12fb3d966550de7fc4f2a482))

## [0.4.6](https://github.com/thereisnotime/warpgate-operator/compare/v0.4.5...v0.4.6) (2026-04-13)


### Features

* **security:** sign install.yaml with cosign keyless signing for Signed-Releases scorecard check ([21b1dc1](https://github.com/thereisnotime/warpgate-operator/commit/21b1dc16776783a5c6aef7ec8e201fd4b2f1ad99))


### Bug Fixes

* **chart:** add README and artifacthub images annotation ([aefbb00](https://github.com/thereisnotime/warpgate-operator/commit/aefbb00b900464d62cf3aa5342a4df39db1dee75))
* **ci:** use operator_version tag (no v prefix) for install manifest image ([acc410c](https://github.com/thereisnotime/warpgate-operator/commit/acc410c017526d228756e9f6ed1c87cae8b9d5c8))
* markdown lint errors in CONTRIBUTORS.md and chart CHANGELOG ([aaff9e3](https://github.com/thereisnotime/warpgate-operator/commit/aaff9e345543c49f547347ebcb065131475faf8a))

## [0.4.5](https://github.com/thereisnotime/warpgate-operator/compare/v0.4.4...v0.4.5) (2026-04-13)


### Features

* **release:** independent versioning for operator and helm chart ([4359f22](https://github.com/thereisnotime/warpgate-operator/commit/4359f22760efbd9d714f7e8ce352f1f09a17ab0f))
* **release:** jsonpath extra-files for appVersion, manifest-read for chart/operator versions, document release flow ([2fef1ac](https://github.com/thereisnotime/warpgate-operator/commit/2fef1ac78cc299d866f13d1368a80cc488a8d0ae))


### Bug Fixes

* **release:** changelog-path in chart package is relative to package root ([7d97330](https://github.com/thereisnotime/warpgate-operator/commit/7d973309697764532a99d533eeaedbb927336dfa))

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
