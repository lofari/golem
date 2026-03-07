const BASE_URL = "http://localhost:8314";

export interface ProjectInfo {
  id: string;
  path: string;
  name: string;
  phase: string;
}

export interface ProcessInfo {
  id: string;
  command: string;
  status: string;
  startedAt: string;
  pid?: number;
}

export interface Task {
  name: string;
  status: string;
  notes?: string;
  depends_on?: string[];
  blocked_reason?: string;
}

export interface State {
  project: { name: string; summary: string; stack: string; docs_path: string };
  status: { current_focus: string; phase: string; last_session: string };
  decisions: { what: string; why: string; when: string }[];
  locked: { path: string; note: string }[];
  tasks: Task[];
  pitfalls: { what: string; fix: string }[];
}

export interface Session {
  iteration: number;
  timestamp: string;
  task: string;
  outcome: string;
  summary: string;
  files_changed?: string[];
}

export interface LaunchConfig {
  command: string;
  config: {
    maxIterations?: number;
    maxToolCalls?: number;
    model?: string;
    task?: string;
    sandbox?: boolean;
    mcp?: boolean;
    parallel?: number;
  };
}

export interface GolemConfig {
  "max-iterations": number;
  "max-tool-calls": number;
  verbose: boolean;
  sandbox: boolean;
  "sandbox-tools"?: string[];
  "sandbox-timeout"?: string;
  "sandbox-memory"?: string;
  mcp: boolean;
  parallel: number;
  "plugin-dir"?: string[];
  model: string;
}

async function fetchJSON<T>(path: string, init?: RequestInit): Promise<T> {
  const resp = await fetch(`${BASE_URL}${path}`, {
    headers: { "Content-Type": "application/json" },
    ...init,
  });
  if (!resp.ok) {
    const err = await resp.json().catch(() => ({ error: resp.statusText }));
    throw new Error(err.error || resp.statusText);
  }
  return resp.json();
}

export const api = {
  health: () => fetchJSON<{ status: string }>("/api/health"),

  listProjects: () => fetchJSON<ProjectInfo[]>("/api/projects"),
  registerProject: (path: string) =>
    fetchJSON<{ id: string }>("/api/projects", {
      method: "POST",
      body: JSON.stringify({ path }),
    }),
  getState: (projectId: string) =>
    fetchJSON<State>(`/api/projects/${projectId}/state`),
  getLog: (projectId: string) =>
    fetchJSON<{ sessions: Session[] }>(`/api/projects/${projectId}/log`),
  getProjectConfig: (projectId: string) =>
    fetchJSON<GolemConfig>(`/api/projects/${projectId}/config`),
  updateProjectConfig: (projectId: string, config: Partial<GolemConfig>) =>
    fetchJSON<{ status: string }>(`/api/projects/${projectId}/config`, {
      method: "PUT",
      body: JSON.stringify(config),
    }),

  listProcesses: (projectId: string) =>
    fetchJSON<ProcessInfo[]>(`/api/projects/${projectId}/processes`),
  launchProcess: (projectId: string, config: LaunchConfig) =>
    fetchJSON<{ id: string }>(`/api/projects/${projectId}/processes`, {
      method: "POST",
      body: JSON.stringify(config),
    }),
  stopProcess: (projectId: string, processId: string) =>
    fetchJSON<{ status: string }>(
      `/api/projects/${projectId}/processes/${processId}`,
      { method: "DELETE" }
    ),

  getGlobalConfig: () => fetchJSON<GolemConfig>("/api/config"),
  updateGlobalConfig: (config: Partial<GolemConfig>) =>
    fetchJSON<{ status: string }>("/api/config", {
      method: "PUT",
      body: JSON.stringify(config),
    }),
};

export function processStreamURL(projectId: string, processId: string): string {
  return `ws://localhost:8314/api/projects/${projectId}/processes/${processId}/stream`;
}

export function stateWatchURL(projectId: string): string {
  return `ws://localhost:8314/api/projects/${projectId}/watch`;
}
