import { useEffect, useRef } from "react";
import { useAppStore } from "../stores/appStore";
import { useWebSocket } from "../hooks/useWebSocket";
import { processStreamURL } from "../lib/api";

export function OutputPane() {
  const { selectedProjectId, selectedProcessId, outputLines, appendOutput } = useAppStore();
  const bottomRef = useRef<HTMLDivElement>(null);

  const wsUrl =
    selectedProjectId && selectedProcessId
      ? processStreamURL(selectedProjectId, selectedProcessId)
      : null;

  useWebSocket({
    url: wsUrl,
    onMessage: (data: any) => {
      if (data.type === "output" && selectedProcessId) {
        appendOutput(selectedProcessId, data.line);
      }
    },
  });

  const lines = selectedProcessId ? outputLines.get(selectedProcessId) || [] : [];

  useEffect(() => {
    bottomRef.current?.scrollIntoView({ behavior: "smooth" });
  }, [lines.length]);

  if (!selectedProcessId) {
    return (
      <div className="flex-1 flex items-center justify-center text-[var(--text-secondary)] text-sm">
        Select a process or launch a new one
      </div>
    );
  }

  return (
    <div className="flex-1 overflow-y-auto bg-[var(--bg-primary)] font-mono text-sm p-3">
      {lines.map((line, i) => (
        <div key={i} className="whitespace-pre-wrap break-all leading-relaxed text-[var(--text-primary)]">
          <span className="text-[var(--border)] mr-2 select-none">{"\u258E"}</span>
          {line}
        </div>
      ))}
      <div ref={bottomRef} />
    </div>
  );
}
