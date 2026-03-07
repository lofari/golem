import { useAppStore } from "../stores/appStore";
import type { ProjectInfo } from "../lib/api";

function ProjectItem({ project }: { project: ProjectInfo }) {
  const { selectedProjectId, selectProject } = useAppStore();
  const isSelected = selectedProjectId === project.id;

  return (
    <button
      onClick={() => selectProject(project.id)}
      className={`w-full text-left px-3 py-2 rounded text-sm transition-colors ${
        isSelected
          ? "bg-[var(--bg-elevated)] text-[var(--text-primary)]"
          : "text-[var(--text-secondary)] hover:bg-[var(--bg-elevated)] hover:text-[var(--text-primary)]"
      }`}
    >
      <div className="font-medium truncate">{project.name || project.path.split("/").pop()}</div>
      <div className="text-xs text-[var(--text-secondary)] truncate">{project.phase || "—"}</div>
    </button>
  );
}

export function Sidebar() {
  const { projects } = useAppStore();

  return (
    <div className="w-48 min-w-48 border-r border-[var(--border)] bg-[var(--bg-surface)] flex flex-col">
      <div className="px-3 py-3 text-xs font-semibold uppercase tracking-wider text-[var(--text-secondary)] border-b border-[var(--border)]">
        Projects
      </div>
      <div className="flex-1 overflow-y-auto p-2 space-y-1">
        {projects.length === 0 ? (
          <div className="text-xs text-[var(--text-secondary)] px-3 py-4 text-center">
            No projects registered
          </div>
        ) : (
          projects.map((p) => <ProjectItem key={p.id} project={p} />)
        )}
      </div>
    </div>
  );
}
