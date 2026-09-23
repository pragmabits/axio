---
name: audit
description: "This skill should be used when the user asks about audit logging, hash chain, tamper-proof logs, HashChain, FileStore, ChainStore, NewHashChain, WithAudit, WithAuditChain, AuditConfig, SHA256 chain, chain verification, Verify, regulatory compliance (LGPD, SOX, PCI-DSS), or implementing custom chain stores. Trigger phrases include \"audit\", \"hash chain\", \"tamper-proof\", \"HashChain\", \"FileStore\", \"ChainStore\", \"WithAudit\", \"AuditConfig\", \"SHA256\", \"chain verification\", \"Verify\", \"LGPD\", \"SOX\", \"PCI-DSS\", \"compliance\", \"integrity\", \"tamper detection\"."
---

# Audit Hash Chain

Axio provides tamper-evident logging through SHA256 cryptographic hash chains.

## How It Works
Each audited JSON line ends with `previous_hash` and `hash`, always last. The hash is SHA-256 of the previous line's hash followed by every byte before the trailer, so it covers exactly what was written — message, error, stacktrace, service metadata, annotations and what custom hooks changed. Encoding, hashing and writing happen under one lock: the file's order is the chain's order. Only JSON outputs can be verified; text outputs show the first 6 characters of the hash.

## Components
- `HashChain` — chain state, hash computation, and `Verify(reader, previousHash)` over log lines
- `FileStore` — persists chain state to a JSON file
- `ChainStore` interface — implement for custom backends (database, etc.)

## Configuration
- `WithAudit(storePath)` — chain state in a local file; every Logger and Event with the same path in a process extends one chain
- `WithAuditChain(chain)` — a chain over any `ChainStore`; share the same chain across Loggers and Events
- AuditConfig in YAML: `audit: { enabled: true, storePath: /path }`

## Verifying
`chain.Verify(file, "")` returns `ErrHashMismatch` (a line changed), `ErrChainBroken` (a line removed, moved, inserted, or without trailer) or `ErrChainIncomplete` (the log ends before the chain). For a rotated file, pass the last hash of the file before it instead of `""`; `VerifyLines(reader, previousHash)` checks one file and returns its last hash. From the terminal: `axio verify --store chain.json app.log.1 app.log` (oldest first), exit 0 when the log verifies, 1 otherwise, naming the file and line.

## Hook Order
Auditing is not a hook: it happens at write time, after PIIHook and every custom hook, so sensitive data is masked before hashing and the hash covers what the hooks changed.

## Usage
Use `/axio` command for detailed audit configuration guidance.
