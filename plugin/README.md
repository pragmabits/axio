# axio plugin

Claude Code plugin that teaches how to use **axio**, the Go structured logging library.

## Skills

| Skill | Loaded when |
|-------|-------------|
| `axio` | Any work with axio. It is the entry point and covers the API end to end: creating a logger, the level methods, annotations, configuration, outputs and rotation, PII masking and its defaults, the audit chain, wide events, hooks and extensions, tracing and metrics, and the `axio` command. It tells the agent to check signatures with `go doc` against the version the project's `go.mod` requires. |
| `migration` | Moving code from stdlib `log`, `log/slog`, logrus, zerolog, zap or apex/log. It says what to settle with the user first (PII masking on by default, Go 1.27, the output format), then gives the call and level mapping and the package-by-package rules. |

## Installation

From the pragmatic marketplace:

```bash
claude plugin marketplace add pragmabits/pragmarketplace
claude plugin install axio@pragmatic
```

From a clone of axio, for one session:

```bash
claude --plugin-dir ./plugin
```

## Structure

```
plugin/
├── .claude-plugin/plugin.json
├── skills/
│   ├── axio/SKILL.md
│   └── migration/SKILL.md
├── .stash/
└── README.md
```

`.stash/` holds the agents, the `/axio` and `/axio-migrate` commands and the API reference the plugin shipped up to 0.3.0. Claude Code does not load them. `.stash/README.md` says what they need before they come back.
