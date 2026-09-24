---
name: axio-migrate
description: Scan a codebase and plan migration from other Go logging libraries to axio
argument-hint: "[target directory or library] - e.g. \"./cmd/api\" or \"migrate from logrus\" or \"scan for logging usage\""
---

# Axio Migration

Triggers the migrate agent to scan a codebase, identify logging usage, and plan a migration to axio.

## User Context

$ARGUMENTS

## Execution

### Resolve plugin root

Before invoking the agent, determine the absolute path to this plugin's root directory:

```
Plugin root: ${CLAUDE_PLUGIN_ROOT}
```

### Argument handling

If `$ARGUMENTS` is exactly `--resolve-root`:
- Output the plugin root path: `${CLAUDE_PLUGIN_ROOT}`
- Do not invoke the agent
- Stop here

### Invoke agent

Otherwise, invoke the migrate agent with the resolved plugin root.

Use the Agent tool with:
- **subagent_type**: `axio:migrate`
- **description**: "Axio migration planning"
- **prompt**: Include the plugin root and user context.

Prompt template:
```
Scan and plan a logging migration to axio.

Plugin root: ${CLAUDE_PLUGIN_ROOT}
User context: $ARGUMENTS

Follow your scanning protocol to identify all logging usage in the codebase. Present findings via AskUserQuestion, then create a migration plan. Only modify code after explicit user approval.
```

This command does not perform migrations directly. The migrate agent owns the full scan → plan → execute workflow.
