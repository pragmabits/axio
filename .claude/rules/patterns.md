# Patterns

Two kinds of patterns live here: **invariants** (non-obvious facts about runtime
behavior — the things that bite you if you don't know them) and **recurring code
patterns** (the shapes the codebase consistently uses — copy them, don't invent
new ones).

## Invariants

- `WithOutputs` stashes live `Output`s in `Config.resolvedOutputs` and `buildOutputs` short-circuits on them. It also mirrors `Type`/`Format`/`Path` into `Config.Outputs` for validation via type assertions — when adding a new `Output` implementation, update both the resolved-output path and the metadata mirror in `options.go`.
- Only the root `Logger` (the one from `New`) owns the engine and outputs. `Named` and `With` return forks that mark `isFork = true`; calling `Close` on a fork returns `ErrLoggerNotRoot` and leaves resources untouched. Do not add resource ownership semantics to forks.
- `cloneAnnotations` is called by `Named`, `log()`, and `Event.Emit` to break shared backing-array aliasing. Any new code path that forks a logger or hands an annotation slice to hooks must clone first — otherwise hook mutations (e.g. PII masking) leak across siblings.
- `Logger.Close` is idempotent on the root: a second call returns `ErrLoggerClosed`, and `log()` becomes a silent no-op after close (no panic). The `closed *atomic.Bool` is shared with forks so the no-op covers them too.
- An audited Logger or Event writes through `auditCore` (`audit.go:338`): it encodes the entry once as JSON, and `HashChain.appendAndWrite` (`audit.go:190`) hashes those bytes, persists the chain and writes every output while holding the chain's mutex. Keep the writes inside that call: the file's order is the chain's order only because nothing is written after the lock is released.
- `WithAudit(path)` resolves to one `HashChain` per absolute path through `chainsByPath` (`audit.go:476`), so a Logger and any number of Events on the same path extend one chain. It is the one deliberate piece of package-level state: two chains loaded from the same store would each start from its last hash and fork it.
- The text format has two writers that must agree: the Console, and `logline.Renderer` rendering JSON back into text for `axio render`. Service metadata reaches only JSON (attached to JSON cores and to the audited core's canonical encoder, never to text encoders) and the text hash is `logline.ShortHash`; the renderer drops and shortens the same keys. `TestRenderer_MatchesConsole` compares the two byte for byte — a change to either side that the test does not cover is a divergence waiting to happen.

## Recurring code patterns

- **Functional options.** New configuration entry points are `func WithX(...) Option` closures that mutate `*Config`. See `options.go:45` (`WithOutputs`), `:118` (`WithHooks`), `:138` (`WithPII`), `:167` (`WithAudit`), `:188` (`WithAuditChain`), `:214` (`WithMetrics`), `:245` (`WithTracer`). Don't add public setters, build-time flags, or package-level state — extend `Option` instead.

- **Sentinel errors grouped by concern.** `errors.go` defines `var (...)` blocks per concern (validation, building, audit, metrics, lifecycle). New error cases declare a sentinel and wrap at the call site with `fmt.Errorf("%w: %w", ErrX, err)` so callers can `errors.Is`. Don't inline `errors.New(...)` at the call site.

- **Optional capabilities via type assertion.** Hook features that not every hook needs are expressed as marker interfaces (e.g. `MetricsAware` at `hook.go:76`) and detected via `aware, ok := hook.(MetricsAware)` at `hook.go:117` and `:134`. Don't widen the base `Hook` interface to absorb optional behavior — add a new marker interface and assert.

- **Fixed hook chain ordering.** The internal `hookChain` runs hooks in a deliberate order — PII → custom — documented at `hook.go:81-95` and enforced by the append order in `buildHooks` (`hook.go:193`). Auditing is not a hook: it hashes the line at write time, after the whole chain, so PII is masked before hashing and the hash covers what custom hooks changed. Any new hook must slot in at the right position, not be appended unconditionally.

- **`Validate()` chain.** Enum types each carry a `Validate() error` (`axio.go:206` `Environment`, `:251` `Level`, `:285` `Format`), and `Config.Validate` (`config.go:268`) composes them; `New` and `NewEvent` wrap its error with `ErrValidateConfig`. New enum-like types or config fields follow the same shape rather than scattering ad-hoc checks at construction.
