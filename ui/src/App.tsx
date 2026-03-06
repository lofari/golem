import { useEffect } from "react";
import { Sidebar } from "./components/Sidebar";
import { ConnectionStatus } from "./components/ConnectionStatus";
import { ProjectView } from "./components/ProjectView";
import { useAppStore } from "./stores/appStore";
import { api } from "./lib/api";
import "./App.css";

function App() {
  const { setConnected, setProjects, selectedProjectId } = useAppStore();

  useEffect(() => {
    let mounted = true;
    let interval: ReturnType<typeof setInterval>;

    async function poll() {
      try {
        await api.health();
        if (!mounted) return;
        setConnected(true);
        const projects = await api.listProjects();
        if (mounted) setProjects(projects);
      } catch {
        if (mounted) setConnected(false);
      }
    }

    poll();
    interval = setInterval(poll, 5000);

    return () => {
      mounted = false;
      clearInterval(interval);
    };
  }, [setConnected, setProjects]);

  return (
    <div className="h-screen flex flex-col bg-[var(--bg-primary)]">
      <div className="flex-1 flex overflow-hidden">
        <Sidebar />
        <main className="flex-1 flex overflow-hidden">
          {selectedProjectId ? (
            <ProjectView />
          ) : (
            <div className="flex-1 flex items-center justify-center text-center text-[var(--text-secondary)]">
              <div>
                <div className="text-lg mb-2">Select a project</div>
                <div className="text-sm">or register one via golem serve</div>
              </div>
            </div>
          )}
        </main>
      </div>
      <ConnectionStatus />
    </div>
  );
}

export default App;
