---
name: migration
description: "Migrating Go code from another logging library to axio: stdlib log, log/slog, logrus, zerolog, zap and apex/log. Covers what to scan and decide before touching code, the call mapping for each library, level mapping, and the rules for doing it package by package. Trigger phrases include \"migrate to axio\", \"switch to axio\", \"replace logrus\", \"replace zap\", \"replace zerolog\", \"replace slog\", \"replace log.Printf\", \"from slog to axio\", \"adopt axio\", \"logging migration\", \"refactor logging to axio\"."
---

# Migrating to axio

The `axio` skill covers the API. This one covers moving existing code onto it.

## Before changing any code

1. **Scan.** Find the imports (`"log"`, `"log/slog"`, `github.com/sirupsen/logrus`, `github.com/rs/zerolog`, `go.uber.org/zap`, `github.com/apex/log`), count the call sites per package, and find where loggers are created. Note whether they are globals or injected, which outputs, formats and levels each environment uses, and where a `context.Context` is already in reach.
2. **Settle these with the user, since each changes what the logs contain:**
   - **PII masking is on by default in axio.** Logs that carry CPF, CNPJ, card numbers, or fields such as `password`, `token` or `authorization` today will carry them masked after the migration. Keep it, which is the secure default, or opt out with `WithPIIDisabled()`. Email and phone patterns are masked only when listed through `WithPII`.
   - **Go 1.27.** axio needs it, and `go get github.com/pragmabits/axio` raises the module's `go` directive.
   - **Format per environment.** With no output configured, development writes colored text to stderr, and staging and production write JSON to stdout. Match what the log collectors expect, or set `WithOutputs` explicitly.
3. **Plan by package,** and change code only after the user approves the plan.

## Level mapping

| Source level | axio |
|---|---|
| Trace | `Debug` (axio has no trace level) |
| Debug | `Debug(ctx, msg, ...)` |
| Info | `Info(ctx, msg, ...)` |
| Warn | `Warn(ctx, err, msg, ...)`, with `nil` when there is no error |
| Error | `Error(ctx, err, msg, ...)` |
| Fatal | `Error(...)`, then `logger.Close()`, then `os.Exit(1)` |
| Panic | `Error(...)`, then `panic(...)` |

`os.Exit` skips deferred calls, so a deferred `Close` never runs and a file output can lose buffered lines: close explicitly before exiting. A panic still runs the deferred `Close`.

## Call mapping

A formatted message becomes a constant message plus annotations, because axio never formats the message.

**stdlib log**

| Before | After |
|---|---|
| `log.Println("server started")` | `logger.Info(ctx, "server started")` |
| `log.Printf("user %s logged in", name)` | `logger.Info(ctx, "user logged in", axio.Field("user", name))` |
| `log.Fatalf("load config: %v", err)` | `logger.Error(ctx, err, "load config"); logger.Close(); os.Exit(1)` |
| `log.SetOutput(w)` | `axio.WithOutputs(...)` |
| `log.SetFlags(...)` | drop it: timestamp and caller are written already; `WithOmitCaller()` removes the caller |

**log/slog**

| Before | After |
|---|---|
| `slog.Info("msg", "key", value)` | `logger.Info(ctx, "msg", axio.Field("key", value))` |
| `slog.InfoContext(ctx, "msg", "key", value)` | `logger.Info(ctx, "msg", axio.Field("key", value))` |
| `slog.Error("msg", "err", err)` | `logger.Error(ctx, err, "msg")` |
| `slog.With("key", value)` | `logger.With(axio.Field("key", value))` |
| `slog.Group("request", "method", m, "path", p)` | `axio.Field("request", map[string]any{"method": m, "path": p})` |
| `slog.Default()` / `slog.SetDefault(...)` | inject the `axio.Logger` instead |

**logrus**

| Before | After |
|---|---|
| `logrus.WithField("k", v).Info("msg")` | `logger.Info(ctx, "msg", axio.Field("k", v))` |
| `logrus.WithFields(logrus.Fields{"a": a, "b": b})` | `logger.With(axio.Field("a", a), axio.Field("b", b))` |
| `logrus.WithError(err).Error("msg")` | `logger.Error(ctx, err, "msg")` |
| `logrus.SetFormatter(&logrus.JSONFormatter{})` | `axio.WithOutputs(axio.Stdout(axio.FormatJSON))` |
| `logrus.SetLevel(logrus.DebugLevel)` | `config.Level = axio.LevelDebug` |

**zerolog**

| Before | After |
|---|---|
| `log.Info().Str("k", v).Int("n", n).Msg("msg")` | `logger.Info(ctx, "msg", axio.Field("k", v), axio.Field("n", n))` |
| `log.Error().Err(err).Msg("msg")` | `logger.Error(ctx, err, "msg")` |
| `zerolog.New(os.Stdout).With().Timestamp().Logger()` | `axio.New(config, axio.WithOutputs(axio.Stdout(axio.FormatJSON)))`; the timestamp is always written |
| `log.Ctx(ctx)` | pass `ctx` to the level method |

**zap**

| Before | After |
|---|---|
| `logger.Info("msg", zap.String("k", v), zap.Int("n", n))` | `logger.Info(ctx, "msg", axio.Field("k", v), axio.Field("n", n))` |
| `logger.Error("msg", zap.Error(err))` | `logger.Error(ctx, err, "msg")` |
| `sugar.Infow("msg", "k", v)` | `logger.Info(ctx, "msg", axio.Field("k", v))` |
| `sugar.Infof("user %s", name)` | `logger.Info(ctx, "user", axio.Field("user", name))` |
| `zap.NewProduction()` / `zap.NewDevelopment()` | `axio.New(config)` with `EnvironmentProduction` / `EnvironmentDevelopment` |
| `logger.Named("db")`, `logger.With(zap.String(...))` | `logger.Named("db")`, `logger.With(axio.Field(...))` |
| `defer logger.Sync()` | `defer logger.Close()` |

**apex/log**

| Before | After |
|---|---|
| `log.WithField("k", v).Info("msg")` | `logger.Info(ctx, "msg", axio.Field("k", v))` |
| `log.WithFields(log.Fields{"a": a})` | `logger.With(axio.Field("a", a))` |
| `log.WithError(err).Error("msg")` | `logger.Error(ctx, err, "msg")` |
| `log.Infof("user %s", name)` | `logger.Info(ctx, "user", axio.Field("user", name))` |

## While migrating

- One package at a time: run `go build ./...` after each file and `go test ./... -count=1` after each package.
- Create the logger once in `main`, check the error from `axio.New`, `defer logger.Close()`, and pass the logger down. Do not recreate a global.
- Thread the context that exists. Where none does yet, use `context.TODO()` with a `// TODO` comment, so the gap stays visible.
- Never import `go.uber.org/zap` or `zapcore` to bridge the two, even when migrating from zap.
- At the end, run `go mod tidy` to drop the old library from `go.mod`.
