# Shape of the code

Functions are short and flat. The numbers in `.golangci.yml` are 50 lines,
cyclomatic complexity 10, and at most 3 levels of control nesting, with
early return preferred over an else branch.

Tests are exempt from all three, and the reason is specific: a table of cases
grows in the table, not in the logic. Splitting one up to fit a line limit hides
what it covers.

## A case is read by its braces

A case in a test table that does not fit on one line is a keyed literal with
its braces on lines of their own: one field per line, `name` first, and a
function field last.

```go
{
	name:     "cpf_with_punctuation",
	patterns: []PIIPattern{PatternCPF},
	input:    "CPF: 123.456.789-01",
	want:     "CPF: ***.***.***-**",
},
```

A positional case carrying a function or a nested literal opens the case's
brace and the function's on the same line and closes them together with the
expected value, and nobody can tell where the case ends. A multi-line raw string
inside a case does the same to the indentation.

A case of a few short scalars stays positional on one line, and needs no
`name`: the input is the name.

```go
{"24h", Duration(24 * time.Hour)},
```

Decided by the developer.

## Tests come first

Code is written test-first. For every behaviour, refusal or fix:

1. **Red.** Write the case and run it, and see it fail on an assertion. A
   compile error is not red: declare the name the test reaches — the sentinel,
   the type, the empty function — so the package builds and the case fails for
   the reason it exists.
2. **Green.** Write the least code that makes it pass, with the rest of the
   suite still passing.
3. **Refactor** with the suite green, or say there was nothing to refactor.

A fix starts with the case that reproduces the defect. A refusal starts with the
case that plants it: one rule, a case that passes and a case that fails.

The reason is what a test written afterwards cannot show. A case written against
code that already exists is shaped by that code and confirms what it does,
including what it does wrong. The red run is the only evidence that the case can
fail at all.

Checking a finished suite by disabling a check and watching a case fail is
still worth doing, and it is extra evidence, never a substitute for the red run.

**Known blind spot.** Nothing enforces the order. The history shows the test
and the code landing together, never which ran red first. It is carried by
review, and a report of the work quotes the red output — a claim of TDD without
it is indistinguishable from a test written after.

## Errors are first class

Every error is handled or explicitly dismissed with a stated reason. Go:
`errcheck`. Its one exclusion is by the shape of the line rather than by a list
of function names — a deferred call is the last thing a function does and has
nowhere left to return an error to, while a list of names is wrong the moment
somebody forgets to add to it. The same call written without `defer` is still
reported. Blind spot this accepts: a deferred `Close` on a *write* path can lose
buffered data, and that error is worth handling; nothing will flag it.

**A cleanup does not stop at the first failure.** Releasing several resources
attempts every one and returns the failures together with `errors.Join`, so one
file that fails to close does not leave the others open: `Logger.Close`,
`Event.Close`, and `WithAgentMode` closing the outputs it discards.

**The log path has no one to return an error to.** `Info`, `Error`, `Emit` and
the rest return nothing and must not panic in the caller's code. A failure there
is reported on stderr, one line prefixed `axio:`, and the call goes on: a hook
error, a panic while formatting the message (recovered, and the message replaced
by `[INVALID FORMAT]`), metrics enabled without a provider. Anything that can
fail *before* the first entry — options, validation, opening outputs — fails in
`New` with a sentinel instead, where the caller can still act on it.

Concurrency is verified, not trusted: `datarace`, `forbidden-call-in-wg-go`, and
`defer` restricted to loop, recover and immediate-recover in `.golangci.yml`,
and `go test -race ./...`, which needs cgo and therefore a C compiler.

## Type escapes

`any` is not a way out, and `interface{}` is the same escape spelled longer.

A logging library has one place where `any` is the honest type: the value a
caller attaches to an entry, which axio cannot know in advance — `Annotate`,
`Event.Add`, `Annotation.Data`. Even there it is narrowed as early as possible:
`Annotate` switches the primitive types into typed fields, and only what is left
travels as an interface.

Anywhere else, name the type. Where a value truly crosses out of what the types
know, name the type at that boundary in one line, exactly where the knowledge
ends. The author is then made to declare what they believe they received.
