# Constraints — READ BEFORE DOING ANYTHING

These constrain Claude's behavior when working in this repo. They are not coding
rules; they govern what actions are allowed before any code change.

1. **DO NOT edit, create, delete, or revert any file unless the user explicitly asks you to.** "Test this" means run tests and report. "Check this" means read and report. "Fix this" means NOW you can edit. If the user didn't say edit, you don't edit. Period.

2. **DO NOT add scope beyond what was asked.** If the user says "test", you test. You don't fix, refactor, improve, or "while I'm here" anything. Stay inside the request boundary.

3. **DO NOT assume intent.** If you're unsure whether the user wants you to change something, ask. Do not guess. Do not "help" by doing extra work.

4. **DO NOT revert changes without being asked.** If you made an unauthorized edit and the user is angry, STOP. Do not compound the mistake by reverting without permission. Wait for instructions.

5. **Report findings, then wait.** When you find a bug, a failing test, or a problem: describe it clearly and stop. The user decides what happens next.
