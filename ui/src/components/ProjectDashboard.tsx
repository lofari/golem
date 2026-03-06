import { useAppStore } from "../stores/appStore";

export function ProjectDashboard() {
  const { projectState, sessions } = useAppStore();

  if (!projectState) return null;

  const tasks = projectState.tasks || [];
  const done = tasks.filter((t) => t.status === "done").length;
  const recentSessions = sessions.slice(-5).reverse();

  return (
    <div className="flex-1 overflow-y-auto p-6 max-w-3xl mx-auto space-y-6">
      <div>
        <h1 className="text-xl font-semibold">{projectState.project.name}</h1>
        <p className="text-sm text-[var(--text-secondary)] mt-1">{projectState.project.summary}</p>
        <div className="flex gap-4 mt-2 text-xs text-[var(--text-secondary)]">
          <span>Stack: {projectState.project.stack}</span>
          <span>Phase: {projectState.status.phase}</span>
        </div>
      </div>

      <div>
        <h2 className="text-sm font-semibold mb-2">Tasks ({done}/{tasks.length})</h2>
        <div className="w-full bg-[var(--bg-elevated)] rounded-full h-2 mb-3">
          <div
            className="bg-[var(--green)] h-2 rounded-full transition-all"
            style={{ width: tasks.length ? `${(done / tasks.length) * 100}%` : "0%" }}
          />
        </div>
        <div className="space-y-1">
          {tasks.map((t) => (
            <div key={t.name} className="flex items-center gap-2 text-sm">
              <span className={t.status === "done" ? "text-[var(--green)]" : t.status === "blocked" ? "text-[var(--red)]" : "text-[var(--text-secondary)]"}>
                {t.status === "done" ? "\u2713" : t.status === "in-progress" ? "\u25D0" : t.status === "blocked" ? "\u2717" : "\u25CB"}
              </span>
              <span className={t.status === "done" ? "text-[var(--text-secondary)] line-through" : ""}>{t.name}</span>
            </div>
          ))}
        </div>
      </div>

      {recentSessions.length > 0 && (
        <div>
          <h2 className="text-sm font-semibold mb-2">Recent Sessions</h2>
          <div className="space-y-2">
            {recentSessions.map((s, i) => (
              <div key={i} className="bg-[var(--bg-surface)] border border-[var(--border)] rounded p-3 text-sm">
                <div className="flex justify-between">
                  <span>#{s.iteration} — {s.task}</span>
                  <span className={s.outcome === "done" ? "text-[var(--green)]" : s.outcome === "partial" ? "text-[var(--yellow)]" : "text-[var(--red)]"}>
                    {s.outcome}
                  </span>
                </div>
                {s.summary && <div className="text-xs text-[var(--text-secondary)] mt-1">{s.summary}</div>}
              </div>
            ))}
          </div>
        </div>
      )}

      {projectState.decisions?.length > 0 && (
        <div>
          <h2 className="text-sm font-semibold mb-2">Decisions ({projectState.decisions.length})</h2>
          <div className="space-y-1">
            {projectState.decisions.map((d, i) => (
              <div key={i} className="text-sm">
                <span className="text-[var(--text-secondary)]">{d.when}</span>{" "}
                <span>{d.what}</span>{" "}
                <span className="text-[var(--text-secondary)]">— {d.why}</span>
              </div>
            ))}
          </div>
        </div>
      )}

      {projectState.pitfalls?.length > 0 && (
        <div>
          <h2 className="text-sm font-semibold mb-2">Pitfalls ({projectState.pitfalls.length})</h2>
          <div className="space-y-1">
            {projectState.pitfalls.map((p, i) => (
              <div key={i} className="text-sm">
                <span>{p.what}</span>
                {p.fix && <span className="text-[var(--text-secondary)]"> — Fix: {p.fix}</span>}
              </div>
            ))}
          </div>
        </div>
      )}
    </div>
  );
}
