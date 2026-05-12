# Release Readiness — Axio v0.1.0

Status of the items needed to publish axio as a public Go module on
`pkg.go.dev`. Generated from a four-agent audit (public API, docs, code
quality, module/distribution) plus a follow-up doc pass.

## Status legend

- [x] Done in the current working tree.
- [ ] Open — needs your decision or a focused code change.
- ⚠ Requires user input (license choice, version target, infra setup).

---

## Doc pass — completed in the current working tree

Verified clean with `go build ./...`, `go vet ./...`, `go test ./...`,
`go test -race ./...`.

- [x] **`annotation.go`** — `Annotate` godoc no longer mentions
      `[zap.Any]`.
- [x] **`axio.go`** — Package doc rewritten; no longer links
      `[go.uber.org/zap]`. `Logger.Close` interface doc now documents
      idempotency and the `ErrLoggerClosed` / `ErrLoggerNotRoot`
      sentinels.
- [x] **`hook.go`** — Removed `BREAKING CHANGE v2.0:` marker; added
      pool-ownership constraint to `Hook.Process` (the `*Entry` is
      borrowed and must not be retained after the call).
- [x] **`options.go`** — Removed `BREAKING CHANGE v2.0:` marker from
      `WithMetrics`.
- [x] **`audit.go`** — `ChainStore.Save` now specifies the atomicity
      and durability contract for custom backends.
- [x] **`README.md`** — Error table grew by 3 rows (`ErrNilTracer`,
      `ErrLoggerClosed`, `ErrLoggerNotRoot`).
- [x] **`README.pt-BR.md`** — Replaced the non-existent `Marshaler` /
      `Annotator` section with `Annotable` / `Append`; added
      `RotationConfig` table and rotation block to the YAML example;
      added the same 3 new error rows; ported the full Wide Events
      section from the EN README.

### Side effect to review

- ⚠ **`go.mod`** auto-downgraded from `go 1.25` to `go 1.24.0` during
      `go build` because the local toolchain is 1.24. Not authored — a
      tool effect. Decide whether to:
  - keep at `go 1.24.0`,
  - downgrade further to `go 1.21` or `go 1.22` (typical floor for new
    public libs — see #4 below),
  - or set `toolchain go1.25.x` so the source can stay on 1.25.

---

## Hard blockers (must resolve before any tag)

- [ ] **#1 — Missing `LICENSE` file** ⚠
      README badge claims MIT but no `LICENSE` exists at the repo root.
      pkg.go.dev shows a "non-standard license" warning until present.
      *Needs:* license choice + copyright holder line.

- [ ] **#2 — No semver tags** ⚠
      `git tag --list` is empty. pkg.go.dev does not index a module
      until at least one tag (e.g. `v0.1.0`) is pushed to the canonical
      remote.
      *Needs:* you to tag the commit you consider releasable, then
      `git push origin v0.1.0`.

- [ ] **#3 — No CI** ⚠
      No `.github/workflows/`, no `Makefile`. There is no automated
      gate against regressions before a tag.
      *Needs:* CI scaffold running `build` / `vet` / `test -race` (and
      ideally `staticcheck` / `gosec`) on push and PR. Matrix and
      required-check policy is your call.

---

## High priority (resolve before v0.1.0)

- [ ] **#4 — `go.mod` `go` directive too high**
      Currently `go 1.24.0` (post-auto-downgrade). Standard for new
      public libs is `1.21` or `1.22`. Audit the codebase for
      1.22+/1.23+/1.24+ language or stdlib usage before lowering.
      *Location:* `go.mod:3`

- [x] **#5 — `Annotation` embeds `field zapcore.Field`** *(won't fix — deliberate exception)*
      The struct field is unexported and unreachable through any method
      or signature on the public API; godoc hides it. Wrapping it in an
      axio-native discriminated union would add a per-log-call
      type-switch (axio kind → `zapcore.Field`) with no user-visible
      reward. The deliberate exception is now documented in the
      `Annotation` godoc at `annotation.go`.

- [ ] **#6 — `HookChain` and `Add` exported, bypass ordering guarantee**
      `chain.Add(myHook)` from external code can be called after the
      logger is built, appending after the intended PII→Audit→Custom
      sequence and breaking the guarantee documented at
      `hook.go:87-99`. Unexport `HookChain`, `NewHookChain`, and
      `Add` — only `BuildHooks` needs them internally.
      *Location:* `hook.go:100`, `:110`, `:132`

- [ ] **#7 — `Build*` helpers are public but only `New` can call them**
      `BuildOutputs`, `BuildHooks`, `BuildMetrics`, `BuildTracer`
      operate on private `Config` fields (`resolvedOutputs`,
      `metrics`, `metricsProvider`, `hooks`, `tracer`) that no
      external caller can populate. Unexport all four.
      *Locations:* `output.go:130`, `hook.go:193`, `metrics.go:165`,
      `tracing.go:85`

- [ ] **#8 — `DefaultSensitiveFields` is mutable exported slice**
      Any caller can `axio.DefaultSensitiveFields = nil` or `append`
      into the shared backing array, silently rewiring every future
      `DefaultPIIConfig()` call. Convert to an unexported slice with
      a `DefaultSensitiveFields()` accessor that returns a fresh
      copy.
      *Location:* `pii.go:83`

---

## Medium priority (target by v0.2.0)

- [ ] **#9 — `Event.Close` fail-fast leaks subsequent outputs**
      First `out.Close()` error aborts the loop; remaining outputs
      are never closed. Mirror `logger.go`'s `errors.Join` pattern.
      *Location:* `event.go:278-285`

- [ ] **#10 — Time-rotation goroutine swallows `Rotate()` errors**
      `_ = f.lumberjack.Rotate()` discards rotation errors silently —
      writes continue to an oversized file with no signal. Log to
      `os.Stderr` at minimum.
      *Location:* `output.go:308`

- [ ] **#11 — `Annotation.Set(value any)` not generic**
      `Annotate[T any]` is generic but `Set` takes `any`. A caller
      that built via `Annotate("k", 42)` can call `.Set("hello")` and
      silently swap the type at runtime with no compile-time check.
      Drop `Set`, make it generic, or explicitly document the gap.
      *Location:* `annotation.go:96`

- [ ] **#12 — `HashChain.Verify` callback signature unstable**
      `func(int) []byte` as a parameter signature is the kind of API
      that gets revised after first real use. Lock it in before v0.1
      or accept a v0.x breaking change.
      *Location:* `audit.go:169`

- [ ] **#13 — `gopkg.in/natefinch/lumberjack.v2` is a personal fork**
      Last tagged Feb 2023. Canonical maintained path is
      `gopkg.in/lumberjack.v2`. Switch after verifying API
      compatibility, or document the deliberate choice.
      *Location:* `go.mod:11`

---

## Low priority / optional

- [ ] **#14 — `NoopTracer` exposed two ways**
      Both `NoopTracer{}` (struct, embeddable) and `NoopTracing()`
      (func) return the same value. Unexport the struct, keep the
      function.
      *Locations:* `tracing.go:38`, `:49`

- [ ] **#15 — Missing `CHANGELOG.md`, `CONTRIBUTING.md`, `SECURITY.md`**
      Not required for pkg.go.dev. `CHANGELOG.md` is the most
      conventional one to add ahead of v0.1.0.

- [ ] **#16 — Doc rot at `annotation.go:199`**
      Internal comment on the unexported `toField` mentions
      "`zap.Any` for best-effort serialization." Invisible on
      pkg.go.dev but still rot for future contributors. Minor.

---

## Suggested release paths

Pick one:

- **Lockdown-then-tag** — close #1, #2, #3, #4, #6, #7, #8, then tag
  `v0.1.0`. Medium items defer to v0.2.
- **Polish-first** — close everything through #14 before tagging.
  Slower, produces a more stable v0.1.0.
- **Pre-release** — tag `v0.0.1-alpha`, gather adopter signal while
  working through #4–#14.

---

## What was already clean

For context — none of these need action.

- `go build`, `go vet`, `go test ./... -count=1`, `go test -race` all
  pass.
- `go mod verify` clean; `go mod tidy -diff` produces no diff.
- All 10 example directories compile.
- No TODO / FIXME / XXX / HACK markers in non-test source.
- Module path matches the canonical `github.com/pragmabits/axio`.
- No platform-specific build tags, no `syscall` / `os/exec` /
  `/proc/` usage — cross-platform clean.
- No `replace` directives, no `vendor/`.
- The `Annotate("http", axio.HTTP{...})` pattern, CNPJ mask
  (`**.***.***/****-**`), `previous_hash` audit field, and the
  collapsed `Metrics.HookDuration` interface all match across code
  and (now) both READMEs.
