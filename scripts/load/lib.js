// Helpers shared by the load scripts. They run inside the grafana/k6 container
// of deploy/compose.prod.yml (see docs/load-test.md), so BASE is the Caddy
// service and INSTANCES are the server containers on the stack's network.
import http from 'k6/http';
import { Trend } from 'k6/metrics';

export const BASE = __ENV.BASE || 'http://web:80';
export const API = `${BASE}/api/v1`;
export const WS_URL = `${BASE.replace(/^http/, 'ws')}/api/v1/ws`;

// The server instances, as IP addresses, in the order run.sh lists them. The
// probe reads /metrics from each one, and the direct socket test connects to
// the first one.
export const INSTANCES = (__ENV.INSTANCES || '').split(',').filter(Boolean);

export const JSON_HEADERS = { 'Content-Type': 'application/json' };

// Thresholds from SRS NFR-01 and NFR-02. The env overrides exist to prove that a
// missed threshold fails the run; the defaults are the targets.
export const REST_P95_MS = Number(__ENV.REST_P95_MS || 200);
export const WS_P95_MS = Number(__ENV.WS_P95_MS || 300);

export const probeWsConnections = new Trend('server_ws_connections', false);
export const probeWsMessagesSent = new Trend('server_ws_messages_sent', false);
export const probeHttpRequests = new Trend('server_http_requests', false);

export function fixture() {
  return JSON.parse(open(__ENV.FIXTURE || '/out/fixture.json'));
}

// refresh trades a single-use refresh token for an access token (15 min).
export function refresh(refreshToken) {
  const res = http.post(
    `${API}/auth/refresh`,
    JSON.stringify({ refresh_token: refreshToken }),
    { headers: JSON_HEADERS, tags: { kind: 'auth' } },
  );
  if (res.status !== 200) {
    throw new Error(`refresh failed: ${res.status} ${res.body}`);
  }
  return res.json('access_token');
}

export function bearer(accessToken, workspaceId) {
  const headers = { Authorization: `Bearer ${accessToken}`, ...JSON_HEADERS };
  if (workspaceId) headers['X-Workspace-ID'] = workspaceId;
  return { headers };
}

// A message body names its run, phase, sender and send time, so a socket can
// tell its own traffic apart and measure send-to-receive latency. Both ends run
// in the same k6 process, so one clock serves.
export function encodeBody(runId, phase, sender, seq, sentAt) {
  return `ktest|${runId}|${phase}|${sender}|${seq}|${sentAt}`;
}

export function decodeBody(body) {
  const p = String(body).split('|');
  if (p.length !== 6 || p[0] !== 'ktest') return null;
  return { runId: p[1], phase: p[2], sender: p[3], seq: Number(p[4]), sentAt: Number(p[5]) };
}

export function uuid() {
  return 'xxxxxxxx-xxxx-4xxx-yxxx-xxxxxxxxxxxx'.replace(/[xy]/g, (c) => {
    const r = (Math.random() * 16) | 0;
    return (c === 'x' ? r : (r & 0x3) | 0x8).toString(16);
  });
}

// probeInstances reads /metrics on each server instance, which only the stack's
// network reaches (Caddy never forwards it). It records the gauge of open
// WebSockets and the counters, one series per instance.
export function probeInstances() {
  INSTANCES.forEach((ip, i) => {
    const res = http.get(`http://${ip}:8080/metrics`, { tags: { kind: 'probe' } });
    if (res.status !== 200) return;
    const tags = { instance: String.fromCharCode(97 + i) };
    const gauge = (name) => {
      const m = res.body.match(new RegExp(`^${name} (\\S+)$`, 'm'));
      return m ? Number(m[1]) : NaN;
    };
    probeWsConnections.add(gauge('ws_connections'), tags);
    probeWsMessagesSent.add(gauge('ws_messages_sent_total'), tags);
    // Sum over every route and status: the request count of this instance.
    let requests = 0;
    for (const m of res.body.matchAll(/^http_request_duration_seconds_count\{[^}]*\} (\S+)$/gm)) {
      requests += Number(m[1]);
    }
    probeHttpRequests.add(requests, tags);
  });
}
