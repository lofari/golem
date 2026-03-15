# Troubleshooting

## Common Errors

### MalformedOutputError — `session-output.json not found`

**Message:** `step "X" did not write session-output.json`
Also: `invalid JSON in session-output.json: <details>`

**Cause:** An agentic step completed without writing a valid `session-output.json` to the project root. This file is how Claude Code hands structured state back to the engine. It is expected to be a JSON object containing at least the keys listed in the step's `writes` field.

**Fix:**
- Check the step's prompt — it must explicitly instruct Claude to write `session-output.json`.
- Review the run log for the raw Claude output: `.ctx/runs/<run-id>/log.json`.
- If the step is consistently failing, the engine will retry up to 2 times by default (`re-run` action). You can increase this in the blueprint:
  ```yaml
  errors:
    malformed-output:
      action: re-run
      max: 3
      hint: "Always write session-output.json as the final action."
  ```

---

### TransientError — timeouts and non-zero exits

**Messages:**
- `agentic step X: <underlying error>`
- `shell step "X" failed: <underlying error>`
- `reading session-output.json: <underlying error>`

**Cause:** A step timed out, Claude returned a non-zero exit, or a transient I/O failure occurred. By default the engine retries transient errors up to 3 times.

**Fix:**
- If the step regularly times out, increase its timeout in the blueprint:
  ```yaml
  - implement:
      timeout: 45m
      max-turns: 250
  ```
- Default timeouts per step type: `plan` 20 min / 50 turns, `implement` 30 min / 200 turns, `review` 20 min / 50 turns. All others: 20 min / 75 turns.
- For shell steps, set `errors.non-zero: halt` to stop the pipeline immediately on failure rather than retrying.

---

### UnrecoverableError — immediate halt

**Message:** `shell step "X" failed: <details>` (when `non-zero: halt` is set)

**Cause:** A shell step exited non-zero and the step is configured with `errors: non-zero: halt`. The engine stops immediately without retrying.

**Fix:** Inspect the step's output in the state snapshot (`.ctx/runs/<run-id>/state-*.json`; the `output` key is stored in state when the step halts). Fix the underlying command or change `non-zero` to `transient` if partial failure is acceptable.

---

### Contract Violations

**Messages:**
- `contract violation: step "X" reads "Y" which is not produced by any prior step or initial-state`
- `contract violation: step "X" reads "Y" which is only conditionally written; use optional-reads instead`

**Cause:** The blueprint parser validates data-flow contracts at load time. If a step lists a key in `reads` that no earlier step writes (or that is not in `initial-state`), parsing fails. If the key is written only inside a `when`/`while`/`if` block it is "conditionally written" and must be declared as `optional-reads`.

**Fix:**
- Move the key from `reads` to `optional-reads` if it may not always be present.
- Add the key to `initial-state` if it is seeded externally.
- Reorder steps so the producing step runs before the consuming step.

---

### Unknown Field Errors

**Messages:**
- `blueprint: unknown field "X" (did you mean "Y"?)`
- `blueprint: step "Z": unknown field "X" (did you mean "Y"?)`

**Cause:** A blueprint YAML contains an unrecognised field. The parser knows common typos:

| Typo | Correct |
|------|---------|
| `step` | `steps` |
| `error` | `errors` |
| `configs` | `config` |
| `desc` | `description` |
| `tool` | `tools` |
| `write` | `writes` |
| `read` | `reads` |
| `optional-read` | `optional-reads` |

**Fix:** Correct the field name. Run `golem agents` to list agents and confirm the file loads without errors.

---

### Unresolved Template Tokens

**Message:** `template error: unresolved tokens ${X}, ${Y} (typo in template?)`

**Cause:** A prompt template contains `${key}` placeholders that were not replaced. This happens when a token refers to a key that is neither in `reads`, `optional-reads`, config, nor the built-in `${agent.name}` / `${run.id}` tokens.

**Fix:** Add the missing key to the step's `reads` (if it must exist) or `optional-reads` (if it may be absent), or fix the typo in the template.

---

### Missing Prompt Template

**Message:** `no prompt template for step "X": inline prompt not set and templates/prompts/X.md not found`

**Cause:** An agentic step has no `prompt:` field and golem could not find a built-in template at `templates/prompts/<name>.md`.

**Fix:** Add an inline `prompt:` to the step definition, or use one of the built-in step names (`plan`, `implement`, `review`, `reflect`, `research`).

---

## Debugging Tips

### Run Logs

Every run writes JSON-lines events to `.ctx/runs/<run-id>/log.json`. Each line is an `EngineEvent` with a `type` field:

| Event type | Meaning |
|---|---|
| `pipeline-start` / `pipeline-end` | Overall run boundaries |
| `step-start` / `step-end` | Per-step execution |
| `error-retry` | Retry or re-run attempt |
| `error-occurred` | Unrecoverable error |
| `loop-enter` / `loop-exit` | While-loop iterations |
| `conditional-skip` | When/if branch not taken |

```bash
# Pretty-print the latest run log
cat .ctx/runs/$(ls -t .ctx/runs | head -1)/log.json | jq .
```

### State Snapshots

State is saved after every successful step as `.ctx/runs/<run-id>/state-001.json`, `state-002.json`, etc., plus a rolling `state.json`. Inspect these to see what data was in scope when a step ran or failed.

### Live Monitoring

```bash
golem status --watch   # live state view
golem log              # iteration history
golem runs list        # recent run IDs and outcomes
golem runs attach      # tail output of the active run
```

---

## Configuration Issues

### Precedence

Flags override all files. Resolution order (lowest to highest):

1. Built-in defaults
2. `~/.config/golem/config.yaml` (global)
3. `.ctx/config.yaml` (project)
4. CLI flags

### Inspect Effective Config

```bash
golem config list      # show all resolved values
golem config get <key> # show one key
```

### Common Config Keys

| Key | Default | Description |
|-----|---------|-------------|
| `max-iterations` | 20 | Max builder loop iterations |
| `max-tool-calls` | 200 | Max tool calls per Claude session |
| `model` | — | Claude model (`sonnet`, `opus`, `haiku`) |
| `timeout` | per step | Override per-step in blueprint `timeout:` |
| `mcp` | true | Enable golem MCP server |
| `lsp` | true | Enable LSP code intelligence |
| `context-map` | true | Inject context map into prompts |
| `context-map-limit` | 15 | Max symbols in context map |
| `engine` | `go` | Orchestration engine (`go` or `blueprint`) |
| `agent` | `build-feature` | Default agent |
| `verbose` | false | Extra output detail |
| `sandbox` | false | Run Claude in warden sandbox |

Set a key: `golem config set max-tool-calls 300`

---

## Graph Issues

The knowledge graph accelerates semantic search, call-graph queries, and context injection. A stale or missing graph degrades step quality but does not stop the engine.

```bash
golem graph build    # re-index source files (tree-sitter + LSP)
golem graph embed    # re-embed for semantic search (requires embeddings model)
golem graph status   # check index health and symbol counts
```

**After switching branches:** The graph may reflect the previous branch. Run `golem graph build` to re-index. This is especially important after large merges or rebases.

**Graph build fails:** Check that the project compiles (`go build ./...` for Go projects). LSP indexing requires a working language server; disable with `golem config set lsp false` if unavailable.

---

## Performance Tips

- **Slow steps:** Increase `max-turns` and `timeout` at the step level in the blueprint. The `implement` step defaults to 200 turns / 30 minutes; complex tasks may need more.
- **Large graphs slow down search:** Check symbol count with `golem graph status`. Limit indexed paths in `.ctx/config.yaml` if the graph is excessively large.
- **Context map token overhead:** Reduce `context-map-limit` (e.g., `golem config set context-map-limit 8`) or disable with `golem config set context-map false`.
- **Parallel sessions:** Set `parallel` > 1 only if your blueprint has independent steps and your API quota supports it.
- **Execution history:** Reduce `execution-history` (default 5) to limit disk usage in long-running projects.
