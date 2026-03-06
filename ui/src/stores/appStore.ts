import { create } from "zustand";
import type { ProjectInfo, ProcessInfo, State, Session } from "../lib/api";

interface AppState {
  // Connection
  connected: boolean;
  setConnected: (connected: boolean) => void;

  // Projects
  projects: ProjectInfo[];
  setProjects: (projects: ProjectInfo[]) => void;
  selectedProjectId: string | null;
  selectProject: (id: string) => void;

  // Processes
  processes: ProcessInfo[];
  setProcesses: (processes: ProcessInfo[]) => void;
  selectedProcessId: string | null;
  selectProcess: (id: string | null) => void;

  // State
  projectState: State | null;
  setProjectState: (state: State) => void;

  // Log
  sessions: Session[];
  setSessions: (sessions: Session[]) => void;
  addSession: (session: Session) => void;

  // Output
  outputLines: Map<string, string[]>;
  appendOutput: (processId: string, line: string) => void;
  clearOutput: (processId: string) => void;
}

export const useAppStore = create<AppState>((set) => ({
  connected: false,
  setConnected: (connected) => set({ connected }),

  projects: [],
  setProjects: (projects) => set({ projects }),
  selectedProjectId: null,
  selectProject: (id) => set({ selectedProjectId: id, selectedProcessId: null }),

  processes: [],
  setProcesses: (processes) => set({ processes }),
  selectedProcessId: null,
  selectProcess: (id) => set({ selectedProcessId: id }),

  projectState: null,
  setProjectState: (projectState) => set({ projectState }),

  sessions: [],
  setSessions: (sessions) => set({ sessions }),
  addSession: (session) =>
    set((s) => ({ sessions: [...s.sessions, session] })),

  outputLines: new Map(),
  appendOutput: (processId, line) =>
    set((s) => {
      const lines = new Map(s.outputLines);
      const existing = lines.get(processId) || [];
      const updated = [...existing, line].slice(-5000);
      lines.set(processId, updated);
      return { outputLines: lines };
    }),
  clearOutput: (processId) =>
    set((s) => {
      const lines = new Map(s.outputLines);
      lines.delete(processId);
      return { outputLines: lines };
    }),
}));
