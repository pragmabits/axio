# Rules

Code-writing rules for this project. Numbering continues from
[constraints.md](constraints.md) so "rule 6" remains "no zap in public API"
across history.

6. **No zap/zapcore in public API.** All public types, interfaces, and function signatures use axio's own types. Zap is an internal implementation detail.

7. **Naming conventions are non-negotiable:**
   - Receivers: single lowercase letter matching the type's first letter (`l` for `*logger`/`Level`, `m` for `*PIIMasker`, `h` for `HTTP`, `e` for `*Event`, `c` for `*HookChain`, etc.). Don't invent multi-letter or descriptive receivers.
   - Everything else (params, variables, struct fields, loop vars): descriptive names, no abbreviations (`value` not `v`, `index` not `i`, `fieldName` not `fn`)

8. **Do not rename or remove methods that already work.** When refactoring a type, reimplement existing methods on the new type with the same signatures.

## Testing

- Pure `testing` package — no testify, no external frameworks
- Subtests with `t.Run()` for organization
- Test helpers in `testutil_test.go` (tempDir, tempFile, assertEqual, etc.)
- Table-driven tests where appropriate
