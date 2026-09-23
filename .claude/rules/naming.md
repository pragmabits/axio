# Naming

**A name is a word.** Shortening a name means removing a redundant word. It never
means mutilating a word.

```
signedUserSessionInTheSystem   too many words
userSession                    one redundant word, if no other session is near
session                        right
sess                           not a word
```

The two extremes are the same mistake pointed in opposite directions: one makes
the reader sweep past useless words, the other makes the reader translate. Both
charge the reader for the writer's convenience.

## The test

Does expanding it cost a beat of thought?

- `err`, `ctx`, `id` — no. You read them and move on.
- `sess`, `mu`, `conn`, `cfg`, `buf`, `req`, `res` — yes. And that cost is paid on
  every read, by everyone, forever, so the author could save four keystrokes once.

Scope carries meaning. `session` needs no `user` prefix when no other session is
in scope and nothing imported collides with it. Say what the scope does not
already say, and nothing more.

## The short names that stay

Three kinds of name are short on purpose, and nothing else is:

- **Receivers** are one letter, the first of the type: `l` for `*logger`, `m`
  for `*PIIMasker`, `c` for `*Config` (rule 7 in [rules.md](rules.md)). A method
  body is short and the receiver is on every line of it; the letter is read as
  "this".
- **The idioms every Go reader expands for free**: `err`, `ctx`, `ok`, and `t`,
  `b` for `*testing.T` and `*testing.B`.
- **Nothing else.** A loop variable is `index`, not `i`; a value is `value`, not
  `v`.

## Subtests are sentences in snake_case

A `t.Run` name says the behaviour the case checks, in lowercase words joined by
underscores: `sequence_increments`, `no_staging_leftover_on_success`,
`error_present_when_set`. `go test -run` takes it without quoting, and the
failure line reads as the claim that broke.

## A top-level test names its subject

A top-level test is `Test` followed by the identifier it tests, its first letter
raised when unexported (`TestBuildOutputs` for `buildOutputs`), and optionally
by what it checks, in CamelCase parts joined by underscores: `TestLogger_Close`,
`TestLogger_ServiceMetadataOnlyInJSON`, `TestHookChain_Process_ErrorStopsChain`.
Snake_case belongs to subtests, where the behaviour is a sentence. A test in an
external `_test` package names the exported identifier it goes through:
`TestNewRootCommand_Render`.

A subject that is not an identifier — `Audit`, `PII`, `ReviewFollowup` — names a
theme or an occasion, and nobody finds the test by the name of what it tests.

The test lives in the `_test.go` of the file that declares its subject —
`TestEnvironment_Validate` in `axio_test.go`, `TestBuildOutputs` in
`output_test.go` — so it is found where the code is. A test in an external
package lives beside the file of the command it runs: `render_test.go` for
`axio render`. A test that needs a build constraint of its own sits beside the
constrained file it tests, as `storelock_test.go` does. Benchmarks and examples,
which `go test` lists by kind, keep `benchmark_test.go` and `example_test.go`.

## An enum's values carry the type's word

`LevelInfo`, `FormatJSON`, `OutputStdout`, `PatternCPF`, `EnvironmentProduction`:
every value of an enumeration starts with the word that names its type — the
type's name less its redundant words, `Output` for `OutputType` and `Pattern`
for `PIIPattern`. The value reads as what it is wherever it appears, and
completion lists the set together.

## The name must be true, and must name a thing

Two failures the abbreviation rule does not catch.

**A name that asserts a behaviour has to hold.** A field named `DisableSample`
on a logger that never samples reads as a switch for sampling and switches
nothing. The name passes every linter and still tells the reader something
false.

**A name has to name a subject, not the occasion it arrived in.**
A file named `review_followup_test.go`, after the review that produced it, and
holding tests for `With`, `Named`, `Close`, PII and `WithOutputs`, is opened by
nobody looking for the `Close` tests. A file, a type or a test is named after what it
is about; when no candidate names a subject, look for the unnamed concept before
the next word.

A candidate is also out for colliding inside this project: `logger` is the
unexported type behind `Logger`, so a variable holding one is `loggerUnderTest`
in the benchmarks rather than a `logger` that shadows the type.

## The one exception

A name that comes from an API is not a name you chose. `flag.Args()` stays,
because the standard library decides it. The rule governs names the author picks;
it has no claim on names the author merely reaches.

`forbidigo` cannot tell the two apart — it sees the identifier, not who chose it
— so reaching one of these produces a finding. When that happens the exception
goes into `.golangci.yml`, scoped to the path where the library is reached and
matched on the exact spelling, so a name written by hand is still caught there.
The one in the config today is `cobra.Command`'s `Args` field, silenced only
for `Args:` as a key in `cmd/axio/internal/cli`.

## Where this is enforced

`forbidigo`, `revive` and `varnamelen` in `.golangci.yml`. The `forbidigo`
patterns are anchored (`^...$`) so they match a bare identifier and never a
component of one — that anchor is what keeps `configFile` from being reported as
`cfg`.

**Known blind spots.** Nothing verifies that a name is a word: no linter carries
a dictionary, and a denylist is reactive by nature — `conn` only gets listed
after somebody writes `conn`. And `forbidigo` reads usage sites only, so a
forbidden abbreviation in a declaration is missed there, and a use of it as the
receiver of a call (`enc.AppendString`) is missed too.
