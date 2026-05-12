# Patterns

Two kinds of patterns live here: **invariants** (non-obvious facts about runtime
behavior — the things that bite you if you don't know them) and **recurring code
patterns** (the shapes the codebase consistently uses — copy them, don't invent
new ones).

## Invariants

- `WithOutputs` stashes live `Output`s in `Config.resolvedOutputs` and `BuildOutputs` short-circuits on them. It also mirrors `Type`/`Format`/`Path` into `Config.Outputs` for validation via type assertions — when adding a new `Output` implementation, update both the resolved-output path and the metadata mirror in `options.go`.
- Only the root `Logger` (the one from `New`) owns the engine and outputs. `Named` and `With` return forks that mark `isFork = true`; calling `Close` on a fork returns `ErrLoggerNotRoot` and leaves resources untouched. Do not add resource ownership semantics to forks.
- `cloneAnnotations` is called by `Named`, `log()`, and `Event.Emit` to break shared backing-array aliasing. Any new code path that forks a logger or hands an annotation slice to hooks must clone first — otherwise hook mutations (e.g. PII masking) leak across siblings.
- `Logger.Close` is idempotent on the root: a second call returns `ErrLoggerClosed`, and `log()` becomes a silent no-op after close (no panic). The `closed *atomic.Bool` is shared with forks so the no-op covers them too.

## Recurring code patterns

- **Functional options.** New configuration entry points are `func WithX(...) Option` closures that mutate `*Config`. See `options.go:42` (`WithOutputs`), `:104` (`WithHooks`), `:124` (`WithPII`), `:147` (`WithAudit`), `:171` (`WithMetrics`), `:202` (`WithTracer`). Don't add public setters, build-time flags, or package-level state — extend `Option` instead.

- **Sentinel errors grouped by concern.** `errors.go` defines `var (...)` blocks per concern (validation, building, audit, metrics, lifecycle). New error cases declare a sentinel and wrap at the call site with `fmt.Errorf("%w: %w", ErrX, err)` so callers can `errors.Is`. Don't inline `errors.New(...)` at the call site.

- **Optional capabilities via type assertion.** Hook features that not every hook needs are expressed as marker interfaces (e.g. `MetricsAware` at `hook.go:77`) and detected via `aware, ok := hook.(MetricsAware)` at `hook.go:121` and `:138`. Don't widen the base `Hook` interface to absorb optional behavior — add a new marker interface and assert.

- **Fixed hook chain ordering.** `HookChain` runs hooks in a deliberate order — PII → Audit → custom — documented at `hook.go:91-93` and enforced by the append order in `BuildHooks`. The ordering matters: PII masking must finish before `AuditHook` hashes the entry. Any new hook must slot in at the right position, not be appended unconditionally.

- **`Validate()` chain.** Enum types each carry a `Validate() error` (`axio.go:150` `Environment`, `:195` `Level`, `:229` `Format`), and `Config.Validate` (`config.go:212`) composes them, wrapping with `ErrValidateConfig`. New enum-like types or config fields follow the same shape rather than scattering ad-hoc checks at construction.
