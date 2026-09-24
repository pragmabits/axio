# Stashed

Components of the axio plugin set aside, outside what Claude Code loads: the
`axio` and `migrate` agents, the `/axio` and `/axio-migrate` commands that only
dispatch to them, and the `api-reference.md` the agents read. The plugin ships
skills only for now.

Before any of it goes back:

- The pragmarketplace convention holds that a plugin shipping agents fetches
  live docs at runtime instead of carrying pre-baked snippets. The agents and
  `api-reference.md` are pre-baked. For axio the live source is `go doc` on the
  version the project's `go.mod` requires, and pkg.go.dev before axio is a
  dependency.
- Lines already stale when stashed, left as they were: `agents/axio.md` says
  the PIIHook comes "from `WithPII`", while it runs by default, and its options
  list lacks `WithOmitCaller`, `WithPIIDisabled`, `WithPIIMaxDepth` and
  `WithPIIOmitErrorVerbose`; `agents/migrate.md` does not warn that masking is
  on by default, that axio needs Go 1.27, or that `os.Exit` skips a deferred
  `Close`. The skills carry the current text.
- To restore, `git mv` each directory back to the plugin root and list the
  agents and commands again in `.claude-plugin/plugin.json`.
