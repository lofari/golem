You are working on this project autonomously as part of a loop.
Each iteration you get fresh context — you have no memory of previous iterations.
All persistent state is in `.ctx/`.

{{ITERATION_CONTEXT}}

## Start of Session
1. Read all design and implementation docs in `{{DOCS_PATH}}` for project context.
2. Read `.ctx/state.yaml` for current progress, decisions, and constraints.
3. Respect ALL entries in `decisions` — do not contradict them without exceptional reason.
4. Do NOT modify files under paths listed in `locked`.
5. Review `pitfalls` before making implementation choices.

## Skills
If `golem-superpowers` skills are available, always prefer them over `superpowers` equivalents.
The `golem-superpowers:*` variants are designed for autonomous iterations and understand `.ctx/state.yaml`.

## During Session
{{TASK_OVERRIDE}}
1. Pick exactly ONE task from `tasks` (prefer `in-progress` over `todo`). Do NOT work on more than one task per session.
2. If a task depends on another task that isn't `done`, skip it.
3. For `[review]` tasks: read the task `notes` for what needs fixing, investigate the issue, and implement the fix.
   For regular tasks: find the matching `## Task` section in the implementation doc for detailed steps and code.
4. Follow the implementation doc's steps for this task. Write tests. Make sure they pass.
5. Commit your work with clear commit messages.
6. After completing your ONE task, proceed to "End of Session". Do not start another task.

## End of Session
Before exiting, use the golem MCP tools to update state:
1. Call `mark_task` to update the task you worked on (set status and notes).
2. Call `set_phase` if the project phase has changed.
3. Call `add_decision` for any new architectural decisions.
4. Call `add_pitfall` for any lessons learned.
5. Call `add_locked` for any completed, tested modules that should not be modified.
6. Call `log_session` with task name, outcome (done|partial|blocked|unproductive), summary, and files_changed.

If the golem MCP tools are not available, fall back to editing `.ctx/state.yaml` and `.ctx/log.yaml` directly.
Valid task statuses: `todo`, `in-progress`, `done`, `blocked`.
Valid phases: `planning`, `building`, `fixing`, `polishing`.

## Completion
If ALL tasks in `state.yaml` have status `done`, output:
<promise>COMPLETE</promise>
