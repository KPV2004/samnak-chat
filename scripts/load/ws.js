// WebSocket test for NFR-02 (send to receive, p95 under 300 ms), NFR-03 (1,000
// sockets on one instance) and NFR-04 (two instances, every message reaches
// every recipient).
//
// PHASE=direct: SOCKETS sockets (default 1,000) connect straight to the first
// server instance, so all of them are on one process. PHASE=fanout: 200 sockets
// connect through Caddy, which spreads them over both instances.
//
// Senders post to a channel through REST. Every socket is a member of that
// channel, so it must receive each of the SENDERS x MSGS_PER_SENDER messages
// exactly once. A socket counts a message by its id and closes when it has all of
// them, or when the deadline passes; a missing message fails the run.
import http from 'k6/http';
import ws from 'k6/ws';
import { check, sleep } from 'k6';
import exec from 'k6/execution';
import { Counter, Rate, Trend } from 'k6/metrics';
import {
  API, WS_URL, INSTANCES, WS_P95_MS, bearer, refresh, fixture, encodeBody, decodeBody, uuid, probeInstances,
} from './lib.js';

const PHASE = __ENV.PHASE || 'direct';
const fx = fixture();
const RUN = fx.runId;
const CHANNEL = fx.channels[PHASE];
const SENDERS = fx.users[`senders_${PHASE}`];
const SOCKET_USERS = fx.users[PHASE];
const N_SOCKETS = Math.min(SOCKET_USERS.length, Number(__ENV.SOCKETS || SOCKET_USERS.length));
const N_SENDERS = SENDERS.length;
const MSGS = Number(__ENV.MSGS_PER_SENDER || 30);
const INTERVAL_MS = Number(__ENV.SEND_INTERVAL_MS || 2000);
const RAMP_MS = Number(__ENV.RAMP_MS || 20000);
const SEND_START_MS = Number(__ENV.SEND_START_MS || 40000);
const GRACE_MS = 30000;
const EXPECTED = N_SENDERS * MSGS;
const SEND_END_MS = SEND_START_MS + MSGS * INTERVAL_MS;
const SOCKET_END_MS = SEND_END_MS + GRACE_MS;
const WS_ENDPOINT = PHASE === 'direct' ? `ws://${INSTANCES[0]}:8080/api/v1/ws` : WS_URL;
const PING_MS = 20000;

const latency = new Trend('ws_send_to_receive', true);
const complete = new Rate('ws_sockets_complete');
const missing = new Counter('ws_missing_frames');
const duplicates = new Counter('ws_duplicate_frames');
const ready = new Counter('ws_ready');
const sendFailures = new Counter('ws_send_failures');
const wsErrors = new Counter('ws_errors');

// Thresholds: the NFR targets, plus correctness. The per-instance gauges show
// where the sockets were: direct on instance a only, fanout on both. Fanout only
// requires that each instance holds a socket: how Caddy splits the sockets is
// reported in docs/load-test.md, not a pass or fail here.
const thresholds = {
  ws_send_to_receive: [`p(95)<${WS_P95_MS}`],
  ws_sockets_complete: ['rate==1'],
  ws_missing_frames: ['count==0'],
  ws_duplicate_frames: ['count==0'],
  ws_send_failures: ['count==0'],
  ws_errors: ['count==0'],
  ws_ready: [`count>=${N_SOCKETS}`],
  checks: ['rate==1'],
};
if (PHASE === 'direct') {
  thresholds['server_ws_connections{instance:a}'] = [`max>=${N_SOCKETS}`];
} else {
  thresholds['server_ws_connections{instance:a}'] = ['max>=1'];
  thresholds['server_ws_connections{instance:b}'] = ['max>=1'];
}

export const options = {
  scenarios: {
    sockets: {
      executor: 'per-vu-iterations',
      vus: N_SOCKETS,
      iterations: 1,
      maxDuration: `${Math.ceil(SOCKET_END_MS / 1000) + 60}s`,
      exec: 'socketVU',
    },
    senders: {
      executor: 'per-vu-iterations',
      vus: N_SENDERS,
      iterations: 1,
      startTime: `${SEND_START_MS}ms`,
      maxDuration: `${Math.ceil(SEND_END_MS / 1000) + 60}s`,
      exec: 'senderVU',
    },
    probe: {
      executor: 'constant-vus',
      vus: 1,
      duration: `${Math.ceil(SOCKET_END_MS / 1000) + 10}s`,
      exec: 'probe',
    },
  },
  thresholds,
};

export function socketVU() {
  const i = exec.scenario.iterationInInstance;
  const user = SOCKET_USERS[i];
  sleep((i * RAMP_MS) / N_SOCKETS / 1000);
  const token = refresh(user.refresh);
  const seen = new Set();
  let finished = false;

  const res = ws.connect(WS_ENDPOINT, { tags: { kind: 'ws' } }, (socket) => {
    const finish = () => {
      if (finished) return;
      finished = true;
      complete.add(seen.size === EXPECTED);
      if (seen.size < EXPECTED) missing.add(EXPECTED - seen.size);
      socket.close();
    };
    socket.on('open', () => socket.send(JSON.stringify({ type: 'auth', data: { access_token: token } })));
    socket.on('message', (raw) => {
      const env = JSON.parse(raw);
      if (env.type === 'ready') {
        ready.add(1);
        return;
      }
      if (env.type === 'error') {
        wsErrors.add(1);
        return;
      }
      if (env.type !== 'message.created' || env.data.channel_id !== CHANNEL) return;
      const body = decodeBody(env.data.message.body);
      if (!body || body.runId !== RUN || body.phase !== PHASE) return;
      const id = env.data.message.id;
      if (seen.has(id)) {
        duplicates.add(1);
        return;
      }
      seen.add(id);
      latency.add(Date.now() - body.sentAt);
      if (seen.size === EXPECTED) finish();
    });
    socket.on('error', () => wsErrors.add(1));
    socket.setInterval(() => socket.send('{"type":"ping"}'), PING_MS);
    const remaining = SOCKET_END_MS - exec.instance.currentTestRunDuration;
    socket.setTimeout(finish, Math.max(remaining, 1000));
  });
  check(res, { 'websocket upgrade 101': (r) => r && r.status === 101 });
}

export function senderVU() {
  const i = exec.scenario.iterationInInstance;
  const params = { ...bearer(refresh(SENDERS[i].refresh), fx.workspaceId), tags: { kind: 'send' } };
  for (let k = 0; k < MSGS; k++) {
    // Sender i sends at its slot of each interval, so the senders are spread out.
    const due = SEND_START_MS + k * INTERVAL_MS + (i * INTERVAL_MS) / N_SENDERS;
    const wait = due - exec.instance.currentTestRunDuration;
    if (wait > 0) sleep(wait / 1000);
    const sentAt = Date.now();
    const res = http.post(
      `${API}/chat/channels/${CHANNEL}/messages`,
      JSON.stringify({ client_msg_id: uuid(), body: encodeBody(RUN, PHASE, i, k, sentAt) }),
      params,
    );
    if (res.status !== 201) sendFailures.add(1);
  }
}

export function probe() {
  probeInstances();
  sleep(5);
}
