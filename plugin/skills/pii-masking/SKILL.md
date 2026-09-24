---
name: pii-masking
description: "This skill should be used when the user asks about PII masking in axio — PIIMasker, PIIHook, MaskString, MaskStringWithCounts, DefaultPIIConfig, PIIPattern, CustomPII, PatternCPF, PatternCNPJ, PatternCreditCard, PatternEmail, PatternPhone, sensitive fields, DefaultSensitiveFields, WithPII option, field redaction, or LGPD/GDPR compliance logging. Trigger phrases include \"PII\", \"masking\", \"PIIMasker\", \"PIIHook\", \"MaskString\", \"DefaultPIIConfig\", \"PatternCPF\", \"PatternCNPJ\", \"PatternEmail\", \"PatternPhone\", \"PatternCreditCard\", \"CustomPII\", \"sensitive fields\", \"REDACTED\", \"WithPII\", \"LGPD\", \"mask personal data\", \"credit card masking\", \"CPF masking\"."
---

# PII Masking

With masking on (`WithPII` or `piiEnabled: true`), axio detects and masks sensitive personal data in log messages and structured fields.

## Built-in Patterns
- CPF: 123.456.789-01 → ***.***.***-**
- CNPJ: 12.345.678/0001-90 → **.***.***/****-**
- Credit Card: 1234-5678-9012-3456 → ****-****-****-****
- Email: user@example.com → ***@***.***
- Phone: (11) 99999-9999 → (**) *****-****

Default when none are given: CPF, CNPJ and credit card. Email (`PatternEmail`) and phone (`PatternPhone`, `PatternPhoneNoDDD`) must be listed explicitly.

## Custom Patterns
CustomPII{Name, Pattern (regex), Mask} for domain-specific patterns.

## Sensitive Fields
DefaultSensitiveFields: password, token, api_key, secret, credential, etc.
Fields matched case-insensitively with partial matching.

## Coverage
Every value the caller hands a line (the logger name, service metadata, caller and stack trace are written as they are): the message; the entry's error, by its message and its verbose form; the error of a value that fails to encode; strings, errors and fmt.Stringer annotations, by their text; []byte, byte arrays and named byte-slice types, by the text they hold (bytes that are not UTF-8 text become [REDACTED], inside a structured value too; a byte array or named byte-slice type is looked for only in a value whose type may hold one or an interface, at one more allocation per field or element); any string with the shape of base64 (standard or URL alphabet, padded or not) — message, error, annotation, nested value, and bytes of text inside a structured value — is also decoded and masked when its text carries PII (one that decodes to binary data becomes [REDACTED] when a pattern matches inside it, and passes otherwise); a JWT or JWE anywhere in a text becomes [REDACTED] whole, and so does a JWS or JWE in JSON serialization (payload with signature or signatures, or ciphertext with iv) inside a structured value; and structured values — maps, slices, structs, pointers, http.Header — walked as the JSON encoding the log writes for them, keys checked against the sensitive fields and strings against the patterns at every level. Annotable values such as HTTP are expanded into their fields before any hook runs. A structured value that needed masking is written as its masked JSON tree (object keys in alphabetical order). A container nested deeper than the depth limit (default 32) becomes [REDACTED] whole; set it with WithPIIMaxDepth(n), piiMaxDepth in the config file, or PIIConfig.MaxDepth. The verbose form of an error that formats itself (errorVerbose, often a stack) is masked and kept, which costs about 250 µs for a 2 KB stack with the default patterns; WithPIIOmitErrorVerbose(), piiOmitErrorVerbose: true, or PIIConfig.OmitErrorVerbose omit it instead, never reading it.

## Hook Execution Order
The PIIHook that `WithPII` (or `piiEnabled`) turns on runs FIRST, and auditing hashes the line only when it is written, after every hook — so sensitive data never enters the audit chain.

## Usage
Use `/axio` command for detailed PII masking guidance.
