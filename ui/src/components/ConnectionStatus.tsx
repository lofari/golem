import { useAppStore } from "../stores/appStore";

export function ConnectionStatus() {
  const { connected, projects, processes } = useAppStore();

  return (
    <div className="h-7 border-t border-[var(--border)] bg-[var(--bg-surface)] flex items-center px-3 text-xs text-[var(--text-secondary)]">
      <span
        className={`inline-block w-2 h-2 rounded-full mr-2 ${
          connected ? "bg-[var(--green)]" : "bg-[var(--red)]"
        }`}
      />
      {connected
        ? `golem serve · ${projects.length} project${projects.length !== 1 ? "s" : ""} · ${processes.length} process${processes.length !== 1 ? "es" : ""}`
        : "Disconnected — start golem serve"}
    </div>
  );
}
