# Webhook Validation

## Summary
Add validating and defaulting webhooks for all CRDs to catch configuration errors at admission time.

## Motivation
Currently, invalid CRD specs (e.g., no target type set, missing required fields, conflicting options) are only caught during reconciliation. Webhooks would reject invalid resources immediately at apply time, giving users instant feedback.

## What Changes
- ValidatingWebhookConfiguration for all 9 CRDs
- Defaulting webhooks (e.g., default port for SSH targets)
- cert-manager integration for webhook TLS
- Webhook tests
