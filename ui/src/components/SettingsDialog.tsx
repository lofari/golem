import { useState, useEffect } from "react";
import { useAppStore } from "../stores/appStore";
import { api } from "../lib/api";
import type { GolemConfig } from "../lib/api";

interface SettingsDialogProps {
  open: boolean;
  onClose: () => void;
}

type Tab = "project" | "global";

function ConfigForm({
  config,
  onChange,
}: {
  config: GolemConfig;
  onChange: (config: GolemConfig) => void;
}) {
  function set<K extends keyof GolemConfig>(key: K, value: GolemConfig[K]) {
    onChange({ ...config, [key]: value });
  }

  return (
    <div className="space-y-3">
      <div className="grid grid-cols-2 gap-3">
        <label className="block">
          <span className="text-xs text-[var(--text-secondary)]">max-iterations</span>
          <input
            type="number"
            value={config["max-iterations"]}
            onChange={(e) => set("max-iterations", parseInt(e.target.value) || 20)}
            className="mt-1 block w-full bg-[var(--bg-primary)] border border-[var(--border)] rounded px-3 py-1.5 text-sm"
          />
        </label>
        <label className="block">
          <span className="text-xs text-[var(--text-secondary)]">max-tool-calls</span>
          <input
            type="number"
            value={config["max-tool-calls"]}
            onChange={(e) => set("max-tool-calls", parseInt(e.target.value) || 200)}
            className="mt-1 block w-full bg-[var(--bg-primary)] border border-[var(--border)] rounded px-3 py-1.5 text-sm"
          />
        </label>
      </div>
      <label className="block">
        <span className="text-xs text-[var(--text-secondary)]">model</span>
        <select
          value={config.model}
          onChange={(e) => set("model", e.target.value)}
          className="mt-1 block w-full bg-[var(--bg-primary)] border border-[var(--border)] rounded px-3 py-1.5 text-sm"
        >
          <option value="">default</option>
          <option value="sonnet">sonnet</option>
          <option value="opus">opus</option>
          <option value="haiku">haiku</option>
        </select>
      </label>
      <div className="flex gap-6">
        <label className="flex items-center gap-2 text-sm">
          <input type="checkbox" checked={config.verbose} onChange={(e) => set("verbose", e.target.checked)} />
          verbose
        </label>
        <label className="flex items-center gap-2 text-sm">
          <input type="checkbox" checked={config.sandbox} onChange={(e) => set("sandbox", e.target.checked)} />
          sandbox
        </label>
        <label className="flex items-center gap-2 text-sm">
          <input type="checkbox" checked={config.mcp} onChange={(e) => set("mcp", e.target.checked)} />
          mcp
        </label>
      </div>
      <label className="block">
        <span className="text-xs text-[var(--text-secondary)]">parallel</span>
        <input
          type="number"
          value={config.parallel}
          onChange={(e) => set("parallel", parseInt(e.target.value) || 1)}
          min={1}
          className="mt-1 block w-20 bg-[var(--bg-primary)] border border-[var(--border)] rounded px-3 py-1.5 text-sm"
        />
      </label>
    </div>
  );
}

export function SettingsDialog({ open, onClose }: SettingsDialogProps) {
  const { selectedProjectId } = useAppStore();
  const [tab, setTab] = useState<Tab>("project");
  const [config, setConfig] = useState<GolemConfig | null>(null);
  const [saving, setSaving] = useState(false);

  useEffect(() => {
    if (!open) return;
    if (tab === "project" && selectedProjectId) {
      api.getProjectConfig(selectedProjectId).then(setConfig).catch(() => setConfig(null));
    } else {
      api.getGlobalConfig().then(setConfig).catch(() => setConfig(null));
    }
  }, [open, tab, selectedProjectId]);

  if (!open) return null;

  async function handleSave() {
    if (!config) return;
    setSaving(true);
    try {
      if (tab === "project" && selectedProjectId) {
        await api.updateProjectConfig(selectedProjectId, config);
      } else {
        await api.updateGlobalConfig(config);
      }
      onClose();
    } catch (err) {
      alert(`Save failed: ${err}`);
    } finally {
      setSaving(false);
    }
  }

  return (
    <div className="fixed inset-0 bg-black/50 flex items-center justify-center z-50" onClick={onClose}>
      <div
        className="bg-[var(--bg-surface)] border border-[var(--border)] rounded-lg w-[460px] p-6 shadow-xl"
        onClick={(e) => e.stopPropagation()}
      >
        <h2 className="text-lg font-semibold mb-4">Settings</h2>

        <div className="flex gap-1 mb-4">
          <button
            onClick={() => setTab("project")}
            className={`px-3 py-1.5 text-sm rounded ${
              tab === "project"
                ? "bg-[var(--bg-elevated)] text-[var(--text-primary)]"
                : "text-[var(--text-secondary)]"
            }`}
          >
            Project
          </button>
          <button
            onClick={() => setTab("global")}
            className={`px-3 py-1.5 text-sm rounded ${
              tab === "global"
                ? "bg-[var(--bg-elevated)] text-[var(--text-primary)]"
                : "text-[var(--text-secondary)]"
            }`}
          >
            Global
          </button>
        </div>

        {config ? (
          <ConfigForm config={config} onChange={setConfig} />
        ) : (
          <div className="text-sm text-[var(--text-secondary)] py-8 text-center">Loading...</div>
        )}

        <div className="flex justify-end gap-3 mt-6">
          <button onClick={onClose} className="px-4 py-2 text-sm text-[var(--text-secondary)]">
            Cancel
          </button>
          <button
            onClick={handleSave}
            disabled={saving || !config}
            className="px-4 py-2 text-sm bg-[var(--accent)] text-white rounded disabled:opacity-50"
          >
            {saving ? "Saving..." : "Save"}
          </button>
        </div>
      </div>
    </div>
  );
}
