import { useState, useEffect } from 'react';
import { supabase } from '@/lib/supabase';

export type MetricData = {
  timestamp: string;
  latency: number;
  savings: number;
};

export type FingerprintLog = {
  id: string;
  fingerprint: string;
  status: 'VALID' | 'BLOCKED' | 'QUARANTINE';
  time: string;
};

export type MCPLog = {
  id: string;
  device: string;
  action: string;
  status: 'APPROVED' | 'DENIED';
  time: string;
};

export function useMetrics() {
  const [metrics, setMetrics] = useState<MetricData[]>([]);
  const [fingerprints, setFingerprints] = useState<FingerprintLog[]>([]);
  const [mcpLogs, setMcpLogs] = useState<MCPLog[]>([]);

  useEffect(() => {
    // Fetch initial metrics from Supabase to prevent 0-flicker on load
    supabase.from('bifrost_metrics')
      .select('request_count, cache_hits, blocked_attacks, total_savings')
      .eq('id', 'global')
      .maybeSingle()
      .then(({ data }) => {
        if (data) {
          const initialMetric = {
            timestamp: new Date().toLocaleTimeString(),
            latency: 0,
            savings: data.total_savings,
            request_count: data.request_count,
            cache_hits: data.cache_hits,
            blocked_attacks: data.blocked_attacks
          };
          setMetrics([initialMetric]);
        }
      });

    // In production, this points to wss://bifrost-proxy/ws/metrics
    const httpUrl = process.env.NEXT_PUBLIC_PROXY_URL || 'http://localhost:8080';
    const wsUrl = httpUrl.replace(/^http/, 'ws') + '/ws/metrics';
    const ws = new WebSocket(wsUrl);

    ws.onmessage = (event) => {
      try {
        const data = JSON.parse(event.data);
        
        if (data.type === 'METRIC') {
          setMetrics(prev => [...prev.slice(-19), data.payload]);
        } else if (data.type === 'FINGERPRINT') {
          setFingerprints(prev => [data.payload, ...prev].slice(0, 5));
        } else if (data.type === 'MCP') {
          setMcpLogs(prev => [data.payload, ...prev].slice(0, 5));
        }
      } catch (e) {
        console.error("Failed to parse websocket message", e);
      }
    };

// Mock data removed for live WebSocket demonstration

    return () => {
      ws.close();
    };
  }, []);

  return { metrics, fingerprints, mcpLogs };
}
