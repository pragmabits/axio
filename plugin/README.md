# axio Plugin

Claude Code plugin providing deep expertise on the **axio** Go structured logging library.

## Features

- **Full API coverage** — guidance on every axio feature: logger creation, configuration, outputs, PII masking, audit chains, OpenTelemetry tracing/metrics, wide events, annotations, hooks, and custom extensions
- **Autonomous migration** — scans codebases for logging usage (stdlib log, slog, logrus, zerolog, zap, apex/log), produces migration plans, and executes guided replacements
- **Runtime source reading** — reads axio source code directly for always-current API knowledge, with bundled docs as fallback

## Commands

| Command | Description |
|---------|-------------|
| `/axio` | General entry point for any axio question or task |
| `/axio-migrate` | Scan a codebase and plan migration to axio |

## Agents

| Agent | Model | Purpose |
|-------|-------|---------|
| `axio` | sonnet | Expert on all axio features — API usage, config, extensions, debugging |
| `migrate` | sonnet | Autonomous codebase scanner and migration executor |

## Skills

| Skill | Triggers |
|-------|----------|
| `core-api` | Logger interface, axio.New, levels, environments, formats |
| `configuration` | LoadConfig, YAML/JSON/TOML, functional options, config precedence |
| `outputs` | Console, Stdout, File, RotatingFile, rotation, agent mode |
| `pii-masking` | PIIMasker, PIIHook, CPF/CNPJ/email/phone patterns, CustomPII |
| `audit` | Hash chain, AuditHook, FileStore, ChainStore, compliance |
| `tracing-metrics` | OpenTelemetry, trace_id/span_id, Metrics interface |
| `migration-patterns` | Migrating from log, slog, logrus, zerolog, zap, apex/log |
| `custom-extensions` | Implementing Output, Hook, ChainStore, Tracer, Metrics, Annotable |

## Structure

```
plugin/
├── .claude-plugin/plugin.json
├── agents/
│   ├── axio.md
│   └── migrate.md
├── commands/
│   ├── axio.md
│   └── axio-migrate.md
├── skills/
│   ├── core-api/SKILL.md
│   ├── configuration/SKILL.md
│   ├── outputs/SKILL.md
│   ├── pii-masking/SKILL.md
│   ├── audit/SKILL.md
│   ├── tracing-metrics/SKILL.md
│   ├── migration-patterns/SKILL.md
│   └── custom-extensions/SKILL.md
├── docs/
│   └── api-reference.md
└── README.md
```

## Installation

```bash
claude --plugin-dir /path/to/axio/plugin
```

Or add to your project's `.claude-plugin/` configuration.
