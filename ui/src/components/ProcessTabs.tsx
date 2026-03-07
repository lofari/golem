import { useAppStore } from "../stores/appStore";
import type { ProcessInfo } from "../lib/api";

const statusColors: Record<string, string> = {
  running: "bg-[var(--green)]",
  stopped: "bg-[var(--text-secondary)]",
  failed: "bg-[var(--red)]",
};

function ProcessTab({ process }: { process: ProcessInfo }) {
  const { selectedProcessId, selectProcess } = useAppStore();
  const isSelected = selectedProcessId === process.id;

  return (
    <button
      onClick={() => selectProcess(process.id)}
      className={`flex items-center gap-2 px-3 py-1.5 text-sm rounded-t border-b-2 transition-colors ${
        isSelected
          ? "bg-[var(--bg-surface)] text-[var(--text-primary)] border-[var(--accent)]"
          : "text-[var(--text-secondary)] border-transparent hover:text-[var(--text-primary)]"
      }`}
    >
      <span className={`w-2 h-2 rounded-full ${statusColors[process.status] || ""}`} />
      {process.command}
    </button>
  );
}

interface ProcessTabsProps {
  onLaunch: () => void;
}

export function ProcessTabs({ onLaunch }: ProcessTabsProps) {
  const { processes } = useAppStore();

  return (
    <div className="flex items-center gap-1 px-2 border-b border-[var(--border)]">
      {processes.map((p) => (
        <ProcessTab key={p.id} process={p} />
      ))}
      <button
        onClick={onLaunch}
        className="px-2 py-1.5 text-sm text-[var(--text-secondary)] hover:text-[var(--text-primary)] transition-colors"
        title="Launch new process"
      >
        +
      </button>
    </div>
  );
}
