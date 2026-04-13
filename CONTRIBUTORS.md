# Contributing

## Conventional Commits

All commits must follow [Conventional Commits](https://www.conventionalcommits.org/). This drives
automated versioning — no conventional commit, no release.

| Prefix | Effect |
|--------|--------|
| `feat:` | minor bump |
| `fix:` | patch bump |
| `feat!:` / `fix!:` / `BREAKING CHANGE:` | major bump |
| `chore:`, `docs:`, `test:`, `refactor:` | no release triggered |

Examples:

```text
feat: add MySQL target reconciler
fix: handle nil pointer when Warpgate is unreachable
feat!: rename WarpgateConnection.spec.token to spec.tokenRef
```

## Release Tracks

This repo maintains two independent release tracks via
[release-please](https://github.com/googleapis/release-please):

| Track | Package root | Version file | Released when |
|-------|-------------|--------------|---------------|
| Operator | `.` | `CHANGELOG.md` | `feat:`/`fix:` commits touch Go source files |
| Helm chart | `charts/warpgate-operator` | `charts/warpgate-operator/CHANGELOG.md` | `feat:`/`fix:` commits touch chart files |

Versions are stored in `.release-please-manifest.json`. Merging a release-please PR cuts the
release and tags it.

---

## Operator Release Flow

Triggered by any `feat:` or `fix:` commit that touches Go source, `go.mod`, `Makefile`, CRD
types, etc.

```text
commit merged to main
        │
        ▼
release-please opens PR:
  - bumps "." in .release-please-manifest.json  (e.g. 0.4.5 → 0.4.6)
  - updates CHANGELOG.md
  - updates charts/warpgate-operator/Chart.yaml  appVersion: 0.4.6
        │
        ▼  (PR merged)
  ┌─────────────────┐  ┌──────────────────────┐  ┌─────────────────────────────────┐
  │  container job  │  │   installer job      │  │         helm job                │
  │                 │  │                      │  │                                 │
  │ build + push    │  │ generate dist/       │  │ helm package                    │
  │ ghcr.io/…:v0.4.6│  │ install.yaml         │  │   --version 0.4.5  (unchanged)  │
  │ ghcr.io/…:0.4.6 │  │ upload to release    │  │   --app-version 0.4.6           │
  │ ghcr.io/…:latest│  │                      │  │ push to oci://ghcr.io/…/charts  │
  └─────────────────┘  └──────────────────────┘  └─────────────────────────────────┘
```

Key points:

- The container image is tagged with both `v0.4.6` (with `v`, for the GitHub release tag) and
  `0.4.6` (without `v`, matching `appVersion` in Chart.yaml).
- The Helm chart `version` does **not** bump on an operator-only release. The chart `version`
  stays at whatever it was last bumped to by a chart release. The existing OCI tag for that chart
  version is overwritten with the new `appVersion`.
- `Chart.yaml` `appVersion` is updated in the release-please PR itself via the `extra-files`
  jsonpath updater, so the source tree always reflects the current operator version.

---

## Helm Chart Release Flow

Triggered by any `feat:` or `fix:` commit that touches files under `charts/warpgate-operator/`
(templates, values, RBAC, helpers, etc.).

```text
commit merged to main
        │
        ▼
release-please opens PR:
  - bumps "charts/warpgate-operator" in .release-please-manifest.json  (e.g. 0.4.5 → 0.4.6)
  - updates charts/warpgate-operator/CHANGELOG.md
  (operator version is unchanged)
        │
        ▼  (PR merged)
  ┌──────────────────────────────────────────────────────┐
  │                      helm job                        │
  │                                                      │
  │  chart_version = 0.4.6  (from release-please output) │
  │  operator_version = 0.4.6  (read from manifest ".")  │
  │                                                      │
  │  helm package                                        │
  │    --version 0.4.6                                   │
  │    --app-version 0.4.6                               │
  │  push to oci://ghcr.io/thereisnotime/charts          │
  └──────────────────────────────────────────────────────┘
```

Key points:

- No new container image or `install.yaml` is published.
- `appVersion` in the packaged chart is read from `.release-please-manifest.json` (the `"."` key),
  which always holds the current operator version.

---

## Both Release Simultaneously

When a single commit (or a batch of commits) touches both Go source and chart files, release-please
creates **one PR** that bumps both packages. Both release workflows run, each using their own
release-please outputs.

---

## Summary

| Change | Operator version | Chart version | New container | install.yaml | Helm OCI |
|--------|-----------------|---------------|---------------|-------------|----------|
| Go source change | bumps | unchanged | yes | yes | yes (same chart version, appVersion updated) |
| Chart template change | unchanged | bumps | no | no | yes (new chart version) |
| Both | both bump | both bump | yes | yes | yes |

---

## Version Scheme

Both tracks use the same semver rules pre-1.0 (configured in `release-please-config.json`):

- `feat:` → patch bump (not minor, because `bump-patch-for-minor-pre-major: true`)
- `fix:` → patch bump
- Breaking change → minor bump (not major, because `bump-minor-pre-major: true`)

Once the project hits 1.0 these flags should be removed so `feat:` bumps minor and breaking
changes bump major.
