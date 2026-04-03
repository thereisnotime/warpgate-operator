# Security Policy

## Reporting Vulnerabilities

**Do not** open public GitHub issues for security vulnerabilities.

Instead, please report them via [GitHub Security Advisories](https://github.com/thereisnotime/warpgate-operator/security/advisories/new).

## Supported Versions

| Version | Supported |
|---------|-----------|
| 0.x     | Yes       |

## Security Considerations

- Sensitive fields (credentials, passwords, SSH keys) are stored in Kubernetes Secrets, never inlined in CRD specs
- The operator container runs as non-root (UID 65532) with a read-only root filesystem
- RBAC permissions follow least-privilege principles
- TLS verification is enabled by default for Warpgate API connections
