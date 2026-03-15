import { useEffect, useRef, useCallback, useState } from 'react';
import { useQueryClient } from '@tanstack/react-query';
import { useUpdateNodeHealth, queryKeys } from '../api';
import type { WebSocketMessage, HealthUpdatePayload } from '../types';

type ConnectionStatus = 'connecting' | 'connected' | 'disconnected' | 'error';

const WS_URL = `${window.location.protocol === 'https:' ? 'wss:' : 'ws:'}//${window.location.host}/websocket`;
const RECONNECT_DELAYS = [1000, 2000, 4000, 8000, 16000]; // Exponential backoff
const MAX_RECONNECT_ATTEMPTS = 5;

export function useWebSocket() {
  const wsRef = useRef<WebSocket | null>(null);
  const reconnectAttemptRef = useRef(0);
  const reconnectTimeoutRef = useRef<ReturnType<typeof setTimeout> | undefined>(undefined);
  const isDisconnectingRef = useRef(false);
  const [status, setStatus] = useState<ConnectionStatus>('connecting');

  const updateNodeHealth = useUpdateNodeHealth();
  const queryClient = useQueryClient();

  const handleMessage = useCallback(
    (event: MessageEvent) => {
      try {
        const message: WebSocketMessage = JSON.parse(event.data);

        switch (message.type) {
          case 'health_update': {
            const payload = message.payload as HealthUpdatePayload;
            updateNodeHealth(payload.nodeId, payload.health);
            break;
          }
          case 'node_update':
          case 'graph_update':
            queryClient.invalidateQueries({ queryKey: queryKeys.graph });
            break;
        }
      } catch (err) {
        console.error('Failed to parse WebSocket message:', err);
      }
    },
    [updateNodeHealth, queryClient]
  );

  const connect = useCallback(() => {
    if (wsRef.current?.readyState === WebSocket.OPEN) return;

    isDisconnectingRef.current = false;
    setStatus('connecting');

    try {
      const ws = new WebSocket(WS_URL);

      ws.onopen = () => {
        setStatus('connected');
        reconnectAttemptRef.current = 0;
      };

      ws.onmessage = handleMessage;

      ws.onclose = () => {
        wsRef.current = null;

        // If disconnect() was called intentionally, don't reconnect
        if (isDisconnectingRef.current) {
          setStatus('disconnected');
          return;
        }

        setStatus('disconnected');

        // Stop reconnecting after max attempts
        if (reconnectAttemptRef.current >= MAX_RECONNECT_ATTEMPTS) return;

        // Schedule reconnect with exponential backoff
        const delay =
          RECONNECT_DELAYS[Math.min(reconnectAttemptRef.current, RECONNECT_DELAYS.length - 1)];
        reconnectAttemptRef.current++;

        reconnectTimeoutRef.current = setTimeout(connect, delay);
      };

      ws.onerror = () => {
        setStatus('error');
      };

      wsRef.current = ws;
    } catch (err) {
      setStatus('error');
    }
  }, [handleMessage]);

  const disconnect = useCallback(() => {
    isDisconnectingRef.current = true;
    if (reconnectTimeoutRef.current) {
      clearTimeout(reconnectTimeoutRef.current);
    }
    if (wsRef.current) {
      wsRef.current.close();
      wsRef.current = null;
    }
    setStatus('disconnected');
  }, []);

  useEffect(() => {
    connect();
    return disconnect;
  }, [connect, disconnect]);

  return {
    status,
    reconnect: connect,
    disconnect,
  };
}
