import { useEffect, useRef, useState, useCallback } from 'react';
import type { Trace } from '@/lib/types';

interface UseWebSocketOptions {
  /** Maximum number of traces to keep in the buffer. Defaults to 200. */
  maxBuffer?: number;
  /** Whether the connection should be active. Defaults to true. */
  enabled?: boolean;
}

interface UseWebSocketReturn {
  traces: Trace[];
  connected: boolean;
  error: string | null;
  clear: () => void;
}

function getWsUrl(): string {
  const proto = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
  return `${proto}//${window.location.host}/api/ws/traces`;
}

export function useWebSocket(options: UseWebSocketOptions = {}): UseWebSocketReturn {
  const { maxBuffer = 200, enabled = true } = options;
  const [traces, setTraces] = useState<Trace[]>([]);
  const [connected, setConnected] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const wsRef = useRef<WebSocket | null>(null);
  const reconnectTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  const reconnectDelayRef = useRef(1000);

  const connect = useCallback(() => {
    if (!enabled) return;

    try {
      const ws = new WebSocket(getWsUrl());
      wsRef.current = ws;

      ws.onopen = () => {
        setConnected(true);
        setError(null);
        reconnectDelayRef.current = 1000;
      };

      ws.onmessage = (event) => {
        try {
          const trace: Trace = JSON.parse(event.data);
          setTraces((prev) => {
            const next = [trace, ...prev];
            return next.length > maxBuffer ? next.slice(0, maxBuffer) : next;
          });
        } catch {
          // Ignore unparseable messages
        }
      };

      ws.onclose = () => {
        setConnected(false);
        wsRef.current = null;
        // Exponential backoff reconnect
        reconnectTimerRef.current = setTimeout(() => {
          reconnectDelayRef.current = Math.min(reconnectDelayRef.current * 2, 30000);
          connect();
        }, reconnectDelayRef.current);
      };

      ws.onerror = () => {
        setError('WebSocket connection error');
        ws.close();
      };
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to connect');
    }
  }, [enabled, maxBuffer]);

  useEffect(() => {
    connect();
    return () => {
      if (reconnectTimerRef.current) clearTimeout(reconnectTimerRef.current);
      if (wsRef.current) {
        wsRef.current.onclose = null; // Prevent reconnect on intentional close
        wsRef.current.close();
      }
    };
  }, [connect]);

  const clear = useCallback(() => {
    setTraces([]);
  }, []);

  return { traces, connected, error, clear };
}
