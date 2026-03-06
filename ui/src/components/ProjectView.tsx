import { useEffect, useState } from "react";
import { ProcessTabs } from "./ProcessTabs";
import { OutputPane } from "./OutputPane";
import { TaskPanel } from "./TaskPanel";
import { LaunchDialog } from "./LaunchDialog";
import { useAppStore } from "../stores/appStore";
import { api, stateWatchURL } from "../lib/api";
import { useWebSocket } from "../hooks/useWebSocket";

export function ProjectView() {
  const {
    selectedProjectId,
    setProcesses,
    setProjectState,
    setSessions,
  } = useAppStore();
  const [showLaunchDialog, setShowLaunchDialog] = useState(false);

  useEffect(() => {
    if (!selectedProjectId) return;
    let mounted = true;

    async function fetchData() {
      try {
        const [procs, state, log] = await Promise.all([
          api.listProcesses(selectedProjectId!),
          api.getState(selectedProjectId!),
          api.getLog(selectedProjectId!),
        ]);
        if (mounted) {
          setProcesses(procs);
          setProjectState(state);
          setSessions(log.sessions);
        }
      } catch {
        // ignore
      }
    }

    fetchData();
    const interval = setInterval(fetchData, 5000);
    return () => {
      mounted = false;
      clearInterval(interval);
    };
  }, [selectedProjectId, setProcesses, setProjectState, setSessions]);

  useWebSocket({
    url: selectedProjectId ? stateWatchURL(selectedProjectId) : null,
    onMessage: (data: any) => {
      if (data.type === "state_changed") {
        setProjectState(data.state);
      }
      if (data.type === "log_appended") {
        useAppStore.getState().addSession(data.session);
      }
    },
  });

  if (!selectedProjectId) return null;

  return (
    <div className="flex-1 flex flex-col">
      <ProcessTabs onLaunch={() => setShowLaunchDialog(true)} />
      <div className="flex-1 flex overflow-hidden">
        <OutputPane />
        <TaskPanel />
      </div>
      <LaunchDialog open={showLaunchDialog} onClose={() => setShowLaunchDialog(false)} />
    </div>
  );
}
