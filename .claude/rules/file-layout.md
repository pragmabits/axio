# Layout of a file

A file is read from the top. What is written first is what has to be known first.

## The order

```
package doc                             only in axio.go
imports

const ( ... )                           file-level constants

interfaces the subject calls through    the contract it is written against
types and enums the subject consumes    you cannot read the subject without them
type Subject                            what the file is about; an interface when
                                        the file is about a contract
func NewSubject                         the constructor
func MustSubject                        its panicking twin, right under it
func (s Subject) Public                 methods, public first
func (s Subject) private
func (s Subject) UnmarshalText          what a stdlib interface asks of it
func (s Subject) String                 the rendering closes that group

optional capabilities of the subject    right after it when it is an interface
type Secondary + its methods
```

**The package doc lives in the file named after the package**, and in no other
file: `axio.go`. There is no `doc.go`. The file a reader opens first is the one
carrying the package's name, and a `doc.go` is a file with no subject of its
own. Decided by the developer.

**Sentinel errors are not in this order: they live in `errors.go`**, in `var`
blocks grouped by concern — validation, building, audit, metrics, lifecycle (see
[patterns.md](patterns.md)). A caller reaches for a sentinel with `errors.Is`,
and in a single-package library the whole vocabulary of failure is one list read
once; spread over the files that return them, it has to be collected before it
can be read. `errors.go` is the one file whose subject is that vocabulary.

## A panicking twin sits under its original

A constructor that returns an error may have a `Must` twin that panics instead,
for initialisation where failure is fatal. The twin is a convenience over the
original, never a different behaviour: it calls the original and panics on its
error. It sits directly under the original — `MustFile` under `File`,
`MustRotatingFile` under `RotatingFile`, `MustPIIMasker` under `NewPIIMasker` —
so a reader who finds one finds both.

## The subject comes first, except when it cannot

`type Subject` goes before the other types — **unless the subject names one of
them in a field or a signature.** Then that one goes above, because
`Annotations []Annotation` cannot be read by someone who has not met
`Annotation`.

So "the subject first" means first among peers, not first in the file.

## Where an interface goes: three cases

Whether the file calls through an interface does not decide its place. In a
library the most important interfaces are the ones the file does *not* call:
`output.go` never calls `Output`, `tracing.go` never calls `Tracer`,
`metrics.go` never calls `Metrics` — the logger does. They are what those files
are about. So the question is what the interface is to the file, and there are
three answers, taken in this order:

1. **It is the subject.** It goes where the subject goes. `Tracer` opens
   `tracing.go` and `Metrics` opens `metrics.go`, with their implementations
   after them.
2. **It is an optional capability of another interface.** It goes right after
   the interface it extends, because it only means something to a reader who
   has just met that one: `RotationErrorReporter` right after `Output`,
   `MetricsAware` right after `Hook`. The capability is detected by type
   assertion and never widens the base interface (see [patterns.md](patterns.md)).
3. **Anything else.** It goes above the first type in the file that calls
   through it or implements it, so nobody meets a use before the contract.
   `hookChain.process` calls every `Hook`, so `Hook` is above `hookChain`;
   `HashChain` calls `Save` and `Load`, so `ChainStore` is above `HashChain`;
   `HTTP` implements `Annotable`, so `Annotable` is above `HTTP`.

**The position is fixed, not judged per file.** Once the case is known the place
follows, even when some other type is needed a few lines sooner, and the reason
is that this whole document is convention carried by review with no tool behind
it: a fixed place survives a reader in a hurry, and "whichever is needed
soonest" does not.

## An enum's values stay with the enum

The constants rule governs file-level constants — a threshold, a limit, a
published policy value. The values of an enumeration are part of the type, and
they sit with it wherever the order above put it:

```go
type Level string

const (
	LevelDebug Level = "debug"
	LevelInfo  Level = "info"
	LevelWarn  Level = "warn"
	LevelError Level = "error"
)
```

Hoisting those to the top of the file separates a type from what it means.

## What the standard library asks for goes last

A method that exists because an interface outside this project requires it is
not what the type is about. It is reached *through* the interface — by `%v`, by
`errors.Is`, by `yaml.Unmarshal` — and hardly ever looked up by name, so it is
the one method nobody scrolls to find.

It goes below the type's own methods, private ones included. That is the single
place a public method sits under a private one, and the reason is that it is not
really the type's method: it is the outside's. `Level.Validate` is the type's
own; `Level.UnmarshalText` is what the decoders ask of it, and it comes after.

Last **among the methods**, not last in the file. A free helper written for one
of them keeps its place at the bottom, and the method naming it is a forward
reference, which this file already permits.

Within the group, one interface's methods stay together — `UnmarshalText`
beside `MarshalText` on `Duration` — and the group closes with `String`. A
`String` is a convenience the outside asked for, the furthest thing from what
the type *does*, so it sits furthest from the declaration of what the type *is*.

**`error` is the case that draws the line, and it falls on the other side.**
`Error() string` has the signature of a `String` and none of its standing: a
type that exists to be an error is *for* that rendering, so `Error` stays among
the type's own methods. `Unwrap` is what the standard library asks of it, and
`Unwrap` is what goes last.

## No jump backwards

The direction of the jump is what matters, and this is the checkable half of
"reads top to bottom".

A **forward** reference is not a jump: you keep reading and you meet it. A
**backward** reference makes the reader scroll up and lose the thread. So a
declaration may freely name something defined below it, and the file is wrong
when understanding line 40 requires having read line 90 and come back.

So a constructor sits directly under its type even when it names an error and a
helper defined further down. The constructor belongs with its type; those names
are ahead of the reader, not behind.

## One file, one subject

This is the rule that makes ordering mostly answer itself. `duration.go` holds
`Duration` and what it needs; `tracing.go` holds trace extraction.

**When the order does not resolve itself, the symptom is not missing order — it
is a file with more than one subject.** Split by subject before reordering.

A subject is not a type. `pii.go` holds `PIIPattern`, `CustomPII`, `PIIConfig`,
`PIIMasker` and `PIIHook`: one subject — PII masking, from the pattern to the
hook that applies it — seen at five points. Splitting that into five files would
be the failure in the other direction.

## Godoc says what holds. It is not a diary.

A doc comment states what the thing is and the invariant that holds for it.

It does **not** carry the argument that produced the decision, the history of
another codebase, or what changed and when. Those go where they can be read as a
whole — the commit message, or the README when a user needs it. A comment that
recounts why a decision was reasonable is not documenting the code, it is
defending the author, and it is charged to every future reader.

The test: **who is this sentence for?** There is no team here. A comment that
explains a choice to someone who was in the conversation where the choice was
made is paying rent for nothing.

**The one argument a doc comment does carry is an exception to a project rule.**
It is named where it is taken, with its reason, because a reader who knows the
rule would otherwise read the code as a mistake. `Annotation` holding a
`zapcore.Field` is the case: its doc says it is a deliberate exception to rule 6
and why. An exception without its reason is indistinguishable from the rule never
having been applied.

**What a public doc carries.** Every exported type and function, and every
method of an exported type, has a doc comment that opens with its name. A
method of an unexported type that exists to satisfy an interface — `logger.Info`,
`output.Format` — carries none: its contract is the interface's doc, and
repeating it would give two texts to keep in step. A doc links what it mentions
— `[Logger.Close]`, `[ErrLoggerClosed]`, `[WithOutputs]` — so godoc turns every
reference into a jump, and it names the sentinel a failure returns rather than
saying "an error". An entry point a user calls directly — a constructor, an
option, a `Load` — ends with an `Example:` block showing the call in context;
a `Must` twin and its original may share one.

## Comments inside a body

Only where the code cannot say it. A body comment records a decision the
surrounding lines do not reveal. Anything narrating what the next line does is
noise the reader has to skip.

## Known blind spot

**No tool checks any of this.** `gofmt` never reorders declarations, and
`decorder` — the golangci-lint linter for declaration order — enforces the
opposite: its `dec-order` requires every `type` before every `func`, which
forbids putting a type's methods under it as soon as a file holds two types:

```
a.go:15:1: type must not be placed after func (desired order: var,const,type,func)
```

Its `dec-num` check would also forbid a file carrying two `const` blocks, which
any file holding an enum and a threshold does.
So `decorder` stays out of the enabled set, and this whole file is convention
carried by review, the same standing as the naming blind spot in
[naming.md](naming.md).
