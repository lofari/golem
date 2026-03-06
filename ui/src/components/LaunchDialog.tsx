import { useState, useEffect } from "react";
import { useAppStore } from "../stores/appStore";
import { api } from "../lib/api";
import type { GolemConfig } from "../lib/api";

interface LaunchDialogProps {
  open: boolean;
  onClose: () => void;
}

export function LaunchDialog({ open, onClose }: LaunchDialogProps) {
  const { selectedProjectId, setProcesses } = useAppStore();
  const [command, setCommand] = useState("code");
  const [model, setModel] = useState("");
  const [maxIterations, setMaxIterations] = useState(20);
  const [maxToolCalls, setMaxToolCalls] = useState(200);
  const [sandbox, setSandbox] = useState(false);
  const [mcp, setMcp] = useState(true);
  const [parallel, setParallel] = useState(1);
  const [task, setTask] = useState("");
  const [launching, setLaunching] = useState(false);

  useEffect(() => {
    if (!selectedProjectId || !open) return;
    api.getProjectConfig(selectedProjectId).then((cfg: GolemConfig) => {
      setMaxIterations(cfg["max-iterations"]);
      setMaxToolCalls(cfg["max-tool-calls"]);
      setSandbox(cfg.sandbox);
      setMcp(cfg.mcp);
      setParallel(cfg.parallel);
      if (cfg.model) setModel(cfg.model);
    }).catch(() => {});
  }, [selectedProjectId, open]);

  if (!open) return null;

  async function handleLaunch() {
    if (!selectedProjectId) return;
    setLaunching(true);
    try {
      await api.launchProcess(selectedProjectId, {
        command,
        config: {
          maxIterations,
          maxToolCalls,
          model: model || undefined,
          task: task || undefined,
          sandbox,
          mcp,
          parallel,
        },
      });
      const procs = await api.listProcesses(selectedProjectId);
      setProcesses(procs);
      onClose();
    } catch (err) {
      alert(`Launch failed: ${err}`);
    } finally {
      setLaunching(false);
    }
  }

  return (
    <div className="fixed inset-0 bg-black/50 flex items-center justify-center z-50" onClick={onClose}>
      <div
        className="bg-[var(--bg-surface)] border border-[var(--border)] rounded-lg w-[420px] p-6 shadow-xl"
        onClick={(e) => e.stopPropagation()}
      >
        <h2 className="text-lg font-semibold mb-4">Launch Process</h2>

        <div className="space-y-4">
          <label className="block">
            <span className="text-xs text-[var(--text-secondary)] uppercase tracking-wider">Command</span>
            <select
              value={command}
              onChange={(e) => setCommand(e.target.value)}
              className="mt-1 block w-full bg-[var(--bg-primary)] border border-[var(--border)] rounded px-3 py-2 text-sm"
            >
              <option value="code">code</option>
              <option value="review">review</option>
              <option value="qa">qa</option>
              <option value="plan">plan</option>
            </select>
          </label>

          <label className="block">
            <span className="text-xs text-[var(--text-secondary)] uppercase tracking-wider">Model</span>
            <select
              value={model}
              onChange={(e) => setModel(e.target.value)}
              className="mt-1 block w-full bg-[var(--bg-primary)] border border-[var(--border)] rounded px-3 py-2 text-sm"
            >
              <option value="">default</option>
              <option value="sonnet">sonnet</option>
              <option value="opus">opus</option>
              <option value="haiku">haiku</option>
            </select>
          </label>

          <div className="grid grid-cols-2 gap-3">
            <label className="block">
              <span className="text-xs text-[var(--text-secondary)] uppercase tracking-wider">Max Iterations</span>
              <input
                type="number"
                value={maxIterations}
                onChange={(e) => setMaxIterations(parseInt(e.target.value) || 20)}
                className="mt-1 block w-full bg-[var(--bg-primary)] border border-[var(--border)] rounded px-3 py-2 text-sm"
              />
            </label>
            <label className="block">
              <span className="text-xs text-[var(--text-secondary)] uppercase tracking-wider">Max Tool Calls</span>
              <input
                type="number"
                value={maxToolCalls}
                onChange={(e) => setMaxToolCalls(parseInt(e.target.value) || 200)}
                className="mt-1 block w-full bg-[var(--bg-primary)] border border-[var(--border)] rounded px-3 py-2 text-sm"
              />
            </label>
          </div>

          <div className="flex gap-6">
            <label className="flex items-center gap-2 text-sm">
              <input
                type="checkbox"
                checked={sandbox}
                onChange={(e) => setSandbox(e.target.checked)}
                className="rounded"
              />
              Sandbox
            </label>
            <label className="flex items-center gap-2 text-sm">
              <input
                type="checkbox"
                checked={mcp}
                onChange={(e) => setMcp(e.target.checked)}
                className="rounded"
              />
              MCP
            </label>
          </div>

          <label className="block">
            <span className="text-xs text-[var(--text-secondary)] uppercase tracking-wider">Parallel</span>
            <input
              type="number"
              value={parallel}
              onChange={(e) => setParallel(parseInt(e.target.value) || 1)}
              min={1}
              className="mt-1 block w-20 bg-[var(--bg-primary)] border border-[var(--border)] rounded px-3 py-2 text-sm"
            />
          </label>

          <label className="block">
            <span className="text-xs text-[var(--text-secondary)] uppercase tracking-wider">Task Override</span>
            <input
              type="text"
              value={task}
              onChange={(e) => setTask(e.target.value)}
              placeholder="(optional)"
              className="mt-1 block w-full bg-[var(--bg-primary)] border border-[var(--border)] rounded px-3 py-2 text-sm placeholder:text-[var(--text-secondary)]"
            />
          </label>
        </div>

        <div className="flex justify-end gap-3 mt-6">
          <button
            onClick={onClose}
            className="px-4 py-2 text-sm text-[var(--text-secondary)] hover:text-[var(--text-primary)] transition-colors"
          >
            Cancel
          </button>
          <button
            onClick={handleLaunch}
            disabled={launching}
            className="px-4 py-2 text-sm bg-[var(--accent)] text-white rounded hover:opacity-90 transition-opacity disabled:opacity-50"
          >
            {launching ? "Launching..." : "Launch"}
          </button>
        </div>
      </div>
    </div>
  );
}
