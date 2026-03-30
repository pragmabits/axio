---
name: migration-patterns
description: "This skill should be used when the user asks about migrating from other Go logging libraries to axio — stdlib log, slog, logrus, zerolog, zap (direct), apex/log. Covers pattern mapping, migration strategies, common pitfalls, and step-by-step migration guides. Trigger phrases include \"migrate\", \"migration\", \"replace logging\", \"switch to axio\", \"from log to axio\", \"from slog\", \"from logrus\", \"from zerolog\", \"from zap\", \"refactor logging\", \"logging refactor\", \"adopt axio\", \"replace log.Println\", \"replace slog.Info\", \"logging audit\"."
---

# Migration Patterns

Patterns for migrating from other Go logging libraries to axio.

## Supported Source Libraries
- stdlib `log` — log.Println, log.Printf, log.Fatal
- `log/slog` — slog.Info, slog.With, slog.Error
- logrus — WithField, WithFields, SetFormatter
- zerolog — log.Info().Str().Msg(), zerolog.New
- zap (direct) — zap.L().Info, sugar.Infow
- apex/log — log.WithField, log.Info

## Key Migration Concerns
1. context.Context — axio always requires it; add context.TODO() where unavailable
2. Error parameter — Warn/Error take error as second param
3. Structured fields → Annotate[T] generic
4. Logger initialization → axio.New(config) with proper error handling
5. Global loggers → dependency injection pattern

## Usage
Use `/axio-migrate` command to trigger autonomous codebase scanning and guided migration.
