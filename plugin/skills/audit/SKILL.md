---
name: audit
description: "This skill should be used when the user asks about audit logging, hash chain, tamper-proof logs, AuditHook, HashChain, FileStore, ChainStore, NewAuditHook, MustAuditHook, NewHashChain, WithAudit, AuditConfig, SHA256 chain, chain verification, Verify, regulatory compliance (LGPD, SOX, PCI-DSS), or implementing custom chain stores. Trigger phrases include \"audit\", \"hash chain\", \"tamper-proof\", \"AuditHook\", \"HashChain\", \"FileStore\", \"ChainStore\", \"WithAudit\", \"AuditConfig\", \"SHA256\", \"chain verification\", \"Verify\", \"LGPD\", \"SOX\", \"PCI-DSS\", \"compliance\", \"integrity\", \"tamper detection\"."
---

# Audit Hash Chain

Axio provides tamper-evident logging through SHA256 cryptographic hash chains.

## How It Works
Each log entry receives a SHA256 hash that includes the previous entry's hash, forming an immutable chain. Any modification invalidates all subsequent hashes.

## Components
- `AuditHook` — hook that adds Hash and PreviousHash to entries
- `HashChain` — manages the chain state and hash computation
- `FileStore` — persists chain state to a JSON file
- `ChainStore` interface — implement for custom backends (database, etc.)

## Configuration
- `WithAudit(storePath)` option — simplest setup
- Or manually: `NewAuditHook(NewFileStore(path))`
- AuditConfig in YAML: `audit: { enabled: true, storePath: /path }`

## Hook Order
AuditHook runs AFTER PIIHook — sensitive data is already masked before hash calculation.

## Usage
Use `/axio` command for detailed audit configuration guidance.
