---
name: pii-masking
description: "This skill should be used when the user asks about PII masking in axio — PIIMasker, PIIHook, MaskString, MaskStringWithCounts, DefaultPIIConfig, PIIPattern, CustomPII, PatternCPF, PatternCNPJ, PatternCreditCard, PatternEmail, PatternPhone, sensitive fields, DefaultSensitiveFields, WithPII option, field redaction, or LGPD/GDPR compliance logging. Trigger phrases include \"PII\", \"masking\", \"PIIMasker\", \"PIIHook\", \"MaskString\", \"DefaultPIIConfig\", \"PatternCPF\", \"PatternCNPJ\", \"PatternEmail\", \"PatternPhone\", \"PatternCreditCard\", \"CustomPII\", \"sensitive fields\", \"REDACTED\", \"WithPII\", \"LGPD\", \"mask personal data\", \"credit card masking\", \"CPF masking\"."
---

# PII Masking

Axio automatically detects and masks sensitive personal data in log messages and structured fields.

## Built-in Patterns
- CPF: 123.456.789-01 → ***.***.***-**
- CNPJ: 12.345.678/0001-90 → **.***.***/****-**
- Credit Card: 1234-5678-9012-3456 → ****-****-****-****
- Email: user@example.com → ***@***.***
- Phone: (11) 99999-9999 → (**) *****-****

## Custom Patterns
CustomPII{Name, Pattern (regex), Mask} for domain-specific patterns.

## Sensitive Fields
DefaultSensitiveFields: password, token, api_key, secret, credential, etc.
Fields matched case-insensitively with partial matching.

## Coverage
Every value a line carries: the message; the entry's error, by its message; strings, errors and fmt.Stringer annotations, by their text; []byte, by the text it holds (bytes that are not UTF-8 text become [REDACTED]); any string with the shape of standard base64 — message, error, annotation, nested value, a struct's []byte field — is also decoded and masked when its text carries PII; and structured values — maps, slices, structs, pointers, http.Header — walked as their JSON encoding, keys checked against the sensitive fields and strings against the patterns at every level. Annotable values such as HTTP are expanded into their fields before any hook runs. A structured value that needed masking is written as its masked JSON tree (object keys in alphabetical order). A container nested deeper than the depth limit (default 32) becomes [REDACTED] whole; set it with WithPIIMaxDepth(n), piiMaxDepth in the config file, or PIIConfig.MaxDepth.

## Hook Execution Order
PIIHook runs FIRST, and auditing hashes the line only when it is written, after every hook — so sensitive data never enters the audit chain.

## Usage
Use `/axio` command for detailed PII masking guidance.
