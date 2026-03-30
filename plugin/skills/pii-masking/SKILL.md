---
name: pii-masking
description: "This skill should be used when the user asks about PII masking in axio — PIIMasker, PIIHook, MaskString, MaskStringWithCounts, DefaultPIIConfig, PIIPattern, CustomPII, PatternCPF, PatternCNPJ, PatternCreditCard, PatternEmail, PatternPhone, sensitive fields, DefaultSensitiveFields, WithPII option, field redaction, or LGPD/GDPR compliance logging. Trigger phrases include \"PII\", \"masking\", \"PIIMasker\", \"PIIHook\", \"MaskString\", \"DefaultPIIConfig\", \"PatternCPF\", \"PatternCNPJ\", \"PatternEmail\", \"PatternPhone\", \"PatternCreditCard\", \"CustomPII\", \"sensitive fields\", \"REDACTED\", \"WithPII\", \"LGPD\", \"mask personal data\", \"credit card masking\", \"CPF masking\"."
---

# PII Masking

Axio automatically detects and masks sensitive personal data in log messages and structured fields.

## Built-in Patterns
- CPF: 123.456.789-01 → ***.***.***-**
- CNPJ: 12.345.678/0001-90 → **.***.***/**01-**
- Credit Card: 1234-5678-9012-3456 → ****-****-****-****
- Email: user@example.com → ***@***.***
- Phone: (11) 99999-9999 → (**) *****-****

## Custom Patterns
CustomPII{Name, Pattern (regex), Mask} for domain-specific patterns.

## Sensitive Fields
DefaultSensitiveFields: password, token, api_key, secret, credential, etc.
Fields matched case-insensitively with partial matching.

## Hook Execution Order
PIIHook runs FIRST — before AuditHook — so sensitive data never enters the audit chain.

## Usage
Use `/axio` command for detailed PII masking guidance.
