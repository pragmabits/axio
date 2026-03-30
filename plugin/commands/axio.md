---
name: axio
description: Axio structured logging expert — API usage, configuration, outputs, PII masking, audit chains, tracing, hooks, wide events, and custom extensions
argument-hint: "[question or task] - e.g. \"set up PII masking with custom patterns\" or \"how do wide events work\""
---

# Axio Expert

Convenient entrypoint for the axio agent. All axio logging guidance is handled by the agent.

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

Otherwise, invoke the axio agent with the resolved plugin root.

Use the Agent tool with:
- **subagent_type**: `axio`
- **description**: "Axio logging expert guidance"
- **prompt**: Include the plugin root and user context, then instruct the agent to look up source and answer.

Prompt template:
```
Answer the user's axio question using current source code and documentation.

Plugin root: ${CLAUDE_PLUGIN_ROOT}
User context: $ARGUMENTS

Follow your Source Code Lookup Protocol. Read axio source files to verify API details before responding. If source is unavailable, use bundled docs at ${CLAUDE_PLUGIN_ROOT}/docs/.
```

This command does not answer questions directly. The axio agent owns all axio guidance — API usage, configuration, outputs, PII, audit, tracing, hooks, events, and extensions.
