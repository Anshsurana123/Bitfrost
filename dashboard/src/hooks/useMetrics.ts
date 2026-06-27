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

export function useMetrics(companyId: string | undefined) {
  const [metrics, setMetrics] = useState<MetricData[]>([]);
  const [fingerprints, setFingerprints] = useState<FingerprintLog[]>([]);
  const [mcpLogs, setMcpLogs] = useState<MCPLog[]>([]);

  useEffect(() => {
    if (!companyId) {
      setMetrics([]);
      setFingerprints([]);
      setMcpLogs([]);
      return;
    }

    // Fetch initial metrics from Supabase for this specific company to prevent 0-flicker on load
    supabase.from('bifrost_metrics')
      .select('request_count, cache_hits, blocked_attacks, total_savings')
      .eq('id', companyId)
      .maybeSingle()
      .then(({ data }) => {
        if (data) {
          const initialMetric = {
            timestamp: new Date().toLocaleTimeString([], { hour: '2-digit', minute: '2-digit', second: '2-digit', hour12: false }),
            latency: 0,
            savings: data.total_savings,
            request_count: data.request_count,
            cache_hits: data.cache_hits,
            blocked_attacks: data.blocked_attacks
          };
          setMetrics([initialMetric]);
        } else {
          const initialMetric = {
            timestamp: new Date().toLocaleTimeString([], { hour: '2-digit', minute: '2-digit', second: '2-digit', hour12: false }),
            latency: 0,
            savings: 0,
            request_count: 0,
            cache_hits: 0,
            blocked_attacks: 0
          };
          setMetrics([initialMetric]);
        }
      });

    // In production, this points to wss://bifrost-proxy/ws/metrics
    const httpUrl = process.env.NEXT_PUBLIC_PROXY_URL || 'http://localhost:8080';
    const wsUrl = httpUrl.replace(/^http/, 'ws') + `/ws/metrics?company_id=${companyId}`;
    const ws = new WebSocket(wsUrl);

    ws.onmessage = (event) => {
      try {
        const data = JSON.parse(event.data);
        
        if (data.type === 'METRIC') {
          const timeStr = typeof data.payload.timestamp === 'number'
            ? new Date(data.payload.timestamp * 1000).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit', second: '2-digit', hour12: false })
            : data.payload.timestamp;
          const payload = { ...data.payload, timestamp: timeStr };
          setMetrics(prev => [...prev.slice(-19), payload]);
        } else if (data.type === 'FINGERPRINT') {
          setFingerprints(prev => [data.payload, ...prev].slice(0, 5));
        } else if (data.type === 'MCP') {
          setMcpLogs(prev => [data.payload, ...prev].slice(0, 5));
        }
      } catch (e) {
        console.error("Failed to parse websocket message", e);
      }
    };

    return () => {
      ws.close();
    };
  }, [companyId]);

  return { metrics, fingerprints, mcpLogs };
}
