# UI Improvements & Knowledge Graph Integration — Design

**Date:** 2026-03-08
**Goal:** Transform the Flutter desktop UI from a functional MVP into a polished daily-driver interface for agentic coding orchestration. Integrate knowledge graph features and cumulative diffs into the UI.

**Approach:** Context-aware single view (Approach C). Keep the two-mode layout (dashboard vs process view), enrich both with graph and diff data, add a graph explorer overlay, and polish all visuals. No file tree — the graph explorer replaces it with richer structural awareness.

---

## 1. Shell Layout & Navigation

**Top bar (48px, refined):**
- **Left:** Project switcher — dropdown showing current project name + phase badge. Click to switch or register a new project. Shows project name + path + phase for each registered project. "Add project..." option at the bottom. Active project highlighted.
- **Center:** Process tabs — pill-shaped, subtle background on active, status dot (pulsing green for running, static red for failed, gray for completed).
- **Right:** Graph explorer icon (small), Launch button, Settings gear, connection indicator (dot only, no text).

**Content area (two modes):**
- **Dashboard mode** — when no process tab is selected.
- **Process mode** — when a process tab is selected.

**Status bar (28px, slimmed):**
- Left: iteration counter when a process is running ("Iteration 3/20 · 4 tasks remaining").
- Right: "golem serve · 2 processes" or connection status.

---

## 2. Dashboard View

Scrollable column with a project header and card-based sections.

### Project header
Single row: name, phase badge, stack, summary. Tighter than current layout.

### Three-card grid (responsive, side-by-side)

**Tasks card (left, wider):**
- Progress bar + task list with status icons (same icons as now).
- Click a task to expand inline notes/blocked reason.
- Filter chips: All / Active / Done / Blocked.

**Graph summary card (center):**
- Node/edge counts: "342 nodes · 891 edges".
- Embedding status: "384-dim · 298 nodes embedded" or "No embeddings — run `golem graph embed`".
- Last indexed timestamp.
- "Explore graph" button → opens graph explorer overlay.
- Empty state: single call-to-action with the CLI command to run.

**Recent activity card (right):**
- Last 5 sessions, compact format.
- Each session shows files changed count as a small badge.

### Cumulative diff card (full-width, below grid)
- Git diff since the last `golem code` run started (or since a user-chosen ref).
- File list on the left: changed files with +/- line counts, color-coded.
- Click a file to see its diff on the right: syntax-highlighted, unified diff format.
- Header: "12 files changed, +340 −89" summary.
- Empty state: "No changes since last run".

### Decisions & Pitfalls row (two smaller cards, below diff)
Same as current, no changes.

---

## 3. Process View

**Terminal pane (left, expanded):**
- Same xterm widget, better padding and border treatment.
- Thin header bar above terminal: process command + elapsed time ("golem code · 4m 23s").

**Right sidebar (280px default, resizable):**
- Draggable to resize (min 220px, max 450px).
- Three toggle tabs at top: **Tasks** | **Context** | **Diff**.

**Tasks tab (default):**
- Task progress bar + task list.
- Compact stats footer: phase, focus, iteration count.
- Same as current but with refined styling.

**Context tab (new):**
- Shows the context map injected into the current iteration's prompt.
- Lists symbols with file:line, type (function/method/type), and score.
- Updates each iteration via WebSocket.
- Empty state: "No knowledge graph — build with `golem graph build`".

**Diff tab (new):**
- Cumulative diff for this process's run.
- File list with +/- counts.
- Click a file to expand diff below (vertical layout since sidebar is narrower).
- Same styling as dashboard diff viewer.

---

## 4. Graph Explorer Overlay

Full-screen overlay (90% of window). Accessible from:
- "Explore graph" button on dashboard graph card.
- Graph icon in the top bar (always available).

### Left pane (300px)
- **Search bar** — natural language semantic search (`POST /graph/search`).
- **Results list** — matching symbols with type icon, name, file:line, relevance score.
- **Type filter chips** below search: All / Functions / Types / Files / Docs.
- Click a result to select it → populates right pane.

### Right pane (expanded)
- **Symbol detail header:** name, type, file path, line number.
- **Relationships section:**
  - "Calls" — outbound CALLS edges.
  - "Called by" — inbound CALLS edges.
  - "Depends on" — IMPORTS/REFERENCES edges.
  - "Depended on by" — reverse.
  - Each item is clickable → navigates to that symbol.
- **Co-changed files** — files that frequently change alongside this symbol's file.
- **Recent commits** — last few commits touching this file.
- **Execution data** (if available) — test results, recent failures.

### Empty state
When no symbol is selected: graph stats overview (node/edge breakdown by type, embedding coverage, history depth).

---

## 5. Visual Polish

### Spacing & Layout
- 16px padding on cards, 12px gaps between them.
- Cards: subtle 1px border, slight elevation on hover.
- Sections use clear headings with secondary text color. Whitespace instead of divider lines.

### Animations
- Fade-in on view transitions (dashboard ↔ process view).
- Smooth height transitions on expanding task notes or diff files.
- Subtle pulse on process tab dot when running.

### Typography
- 11px for metadata/secondary text.
- Bolder section headers.
- Monospace for all code-related content: file paths, symbols, diffs, line counts.

### Diff viewer
- Green/red background tint on added/removed lines (GitHub-style).
- Line numbers in muted color.
- File headers with expand/collapse chevron.

### Status badges
- Phase badge: filled pill (planning=blue, building=green, fixing=yellow, polishing=purple).
- Task status: current icons + subtle background tint matching status color.
- Process status: pulsing dot for running, static for completed/failed.

### Empty states
- Friendly messaging with CLI command to fix (e.g., code block with `golem graph build && golem graph embed`).
- No broken-looking blank areas.

---

## 6. API Additions

### New endpoints

| Endpoint | Method | Purpose |
|----------|--------|---------|
| `/api/projects/{id}/diff` | GET | Cumulative git diff since a ref (query param `?ref=<base>`, defaults to process start commit). Returns file list with stats + unified diff per file. |
| `/api/projects/{id}/graph/stats` | GET | Graph statistics: node/edge counts by type, embedding count, last indexed, history depth. Lighter than full graph queries. |
| `/api/projects/{id}/context-map` | GET | Returns the rendered context map for the current/last iteration. Query param `?task=<text>` for on-demand generation. |

### WebSocket additions

New message type on the project watch WebSocket:
- `context_map_updated` — emitted at each iteration start, contains the context map symbols list for the process view Context tab.

### Existing endpoints (no changes needed)
- `GET /graph/related` — covers graph explorer relationships.
- `POST /graph/search` — covers graph explorer semantic search.
- `GET /graph/runtime-path` — covers execution data in graph explorer.
- Process streaming WebSocket — already works.
- Config endpoints — already complete.

---

## 7. New Flutter Components

| Component | Location | Purpose |
|-----------|----------|---------|
| `ProjectSwitcher` | `views/project_switcher.dart` | Dropdown for switching between registered projects |
| `GraphSummaryCard` | `views/dashboard.dart` (inline) | Graph stats + "Explore" button on dashboard |
| `DiffCard` / `DiffViewer` | `views/diff_viewer.dart` | Cumulative diff with file list + syntax-highlighted unified diff |
| `GraphExplorer` | `views/graph_explorer.dart` | Full overlay with search, results, symbol detail |
| `ContextPanel` | `views/process_view.dart` (inline) | Context map display in process sidebar |
| `ResizableSidebar` | `views/process_view.dart` (inline) | Draggable sidebar width |

### New providers

| Provider | Purpose |
|----------|---------|
| `graphStatsProvider` | Fetches graph stats from API |
| `diffProvider` | Fetches cumulative diff from API |
| `contextMapProvider` | Watches context map updates via WebSocket |
| `projectListProvider` | Lists all registered projects for switcher |

### New models

| Model | Fields |
|-------|--------|
| `GraphStats` | nodesByType, edgesByType, embeddingCount, lastIndexed, commitCount |
| `DiffSummary` | files (list of FileDiff), totalAdded, totalRemoved |
| `FileDiff` | path, additions, deletions, hunks (list of DiffHunk) |
| `ContextMapEntry` | name, type, path, line, score, relationships |

---

## Non-goals

- File tree component — the graph explorer replaces this with richer structural awareness.
- Multi-project sidebar — a compact dropdown switcher is sufficient.
- Code editing — this is an orchestration tool, not an IDE.
- Graph visualization (node-link diagrams) — relationship lists are more practical and simpler to implement.
