import { useAppStore } from "../stores/appStore";
import type { Task } from "../lib/api";

const statusIcons: Record<string, { icon: string; color: string }> = {
  done: { icon: "\u2713", color: "text-[var(--green)]" },
  "in-progress": { icon: "\u25D0", color: "text-[var(--yellow)]" },
  todo: { icon: "\u25CB", color: "text-[var(--text-secondary)]" },
  blocked: { icon: "\u2717", color: "text-[var(--red)]" },
};

function TaskItem({ task }: { task: Task }) {
  const { icon, color } = statusIcons[task.status] || statusIcons.todo;

  return (
    <div className="flex items-start gap-2 py-1">
      <span className={`${color} font-mono text-xs mt-0.5`}>{icon}</span>
      <div className="min-w-0">
        <div className="text-sm truncate">{task.name}</div>
        {task.notes && (
          <div className="text-xs text-[var(--text-secondary)] truncate">{task.notes}</div>
        )}
        {task.blocked_reason && (
          <div className="text-xs text-[var(--red)] truncate">{task.blocked_reason}</div>
        )}
      </div>
    </div>
  );
}

export function TaskPanel() {
  const { projectState, sessions } = useAppStore();

  if (!projectState) return null;

  const tasks = projectState.tasks || [];
  const doneTasks = tasks.filter((t) => t.status === "done").length;
  const totalTasks = tasks.length;
  const lastSession = sessions.length > 0 ? sessions[sessions.length - 1] : null;

  return (
    <div className="w-56 min-w-56 border-l border-[var(--border)] bg-[var(--bg-surface)] flex flex-col">
      <div className="px-3 py-2 border-b border-[var(--border)] flex items-center justify-between">
        <span className="text-xs font-semibold uppercase tracking-wider text-[var(--text-secondary)]">
          Tasks
        </span>
        <span className="text-xs text-[var(--text-secondary)]">
          {doneTasks}/{totalTasks}
        </span>
      </div>
      <div className="flex-1 overflow-y-auto px-3 py-2 space-y-0.5">
        {tasks.map((t) => (
          <TaskItem key={t.name} task={t} />
        ))}
      </div>

      <div className="border-t border-[var(--border)] px-3 py-2 space-y-1 text-xs text-[var(--text-secondary)]">
        <div className="flex justify-between">
          <span>Phase</span>
          <span className="text-[var(--text-primary)]">{projectState.status.phase || "\u2014"}</span>
        </div>
        <div className="flex justify-between">
          <span>Focus</span>
          <span className="text-[var(--text-primary)] truncate ml-2">
            {projectState.status.current_focus || "\u2014"}
          </span>
        </div>
        {lastSession && (
          <>
            <div className="flex justify-between">
              <span>Last iter</span>
              <span className="text-[var(--text-primary)]">#{lastSession.iteration}</span>
            </div>
            <div className="flex justify-between">
              <span>Outcome</span>
              <span
                className={
                  lastSession.outcome === "done"
                    ? "text-[var(--green)]"
                    : lastSession.outcome === "blocked" || lastSession.outcome === "unproductive"
                    ? "text-[var(--red)]"
                    : "text-[var(--yellow)]"
                }
              >
                {lastSession.outcome}
              </span>
            </div>
          </>
        )}
        <div className="flex justify-between">
          <span>Decisions</span>
          <span className="text-[var(--text-primary)]">{projectState.decisions?.length || 0}</span>
        </div>
        <div className="flex justify-between">
          <span>Pitfalls</span>
          <span className="text-[var(--text-primary)]">{projectState.pitfalls?.length || 0}</span>
        </div>
      </div>
    </div>
  );
}
