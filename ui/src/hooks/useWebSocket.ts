import { useEffect, useRef, useCallback } from "react";

interface UseWebSocketOptions {
  url: string | null;
  onMessage: (data: unknown) => void;
  onOpen?: () => void;
  onClose?: () => void;
}

export function useWebSocket({ url, onMessage, onOpen, onClose }: UseWebSocketOptions) {
  const wsRef = useRef<WebSocket | null>(null);
  const reconnectTimeout = useRef<ReturnType<typeof setTimeout> | undefined>(undefined);

  const connect = useCallback(() => {
    if (!url) return;

    const ws = new WebSocket(url);
    wsRef.current = ws;

    ws.onopen = () => {
      onOpen?.();
    };

    ws.onmessage = (event) => {
      try {
        const data = JSON.parse(event.data);
        onMessage(data);
      } catch {
        // ignore non-JSON messages
      }
    };

    ws.onclose = () => {
      onClose?.();
      reconnectTimeout.current = setTimeout(connect, 2000);
    };

    ws.onerror = () => {
      ws.close();
    };
  }, [url, onMessage, onOpen, onClose]);

  useEffect(() => {
    connect();
    return () => {
      clearTimeout(reconnectTimeout.current);
      wsRef.current?.close();
    };
  }, [connect]);

  return wsRef;
}
