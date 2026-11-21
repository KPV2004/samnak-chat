// REST mix for NFR-01 (p95 of API calls under 200 ms). One VU per user of the
// fixture, each pacing its own requests so that the total is REST_RPS, through
// Caddy so both server instances take traffic. Every API call is tagged
// kind:api; the thresholds read only those.
import http from 'k6/http';
import { check, sleep } from 'k6';
import exec from 'k6/execution';
import { API, bearer, refresh, fixture, uuid, probeInstances, REST_P95_MS } from './lib.js';

const fx = fixture();
const RATE = Number(__ENV.REST_RPS || 100);
const DURATION_MS = Number(__ENV.REST_DURATION_MS || 90000);
const USERS = fx.users.rest;
const CHANNEL = fx.channels.rest;
const WORKSPACE = fx.workspaceId;
const PERIOD_MS = (USERS.length * 1000) / RATE;

export const options = {
  scenarios: {
    mix: {
      executor: 'per-vu-iterations',
      vus: USERS.length,
      iterations: 1,
      maxDuration: `${Math.ceil(DURATION_MS / 1000) + 60}s`,
      exec: 'mix',
    },
    probe: {
      executor: 'constant-vus',
      vus: 1,
      duration: `${Math.ceil(DURATION_MS / 1000)}s`,
      exec: 'probe',
    },
  },
  thresholds: {
    'http_req_duration{kind:api}': [`p(95)<${REST_P95_MS}`],
    'http_req_failed{kind:api}': ['rate<0.01'],
  },
};

function api(token, method, path, body, workspace, endpoint, status) {
  const params = { ...bearer(token, workspace), tags: { kind: 'api', endpoint } };
  const url = `${API}${path}`;
  const res = method === 'GET' ? http.get(url, params) : http.post(url, JSON.stringify(body), params);
  check(res, { [`${endpoint}: ${status}`]: (r) => r.status === status });
}

// The mix: reads most of the time, plus one send in seven. Weights sum to 100.
const MIX = [
  { weight: 15, run: (t) => api(t, 'GET', '/auth/me', null, null, 'me', 200) },
  { weight: 25, run: (t) => api(t, 'GET', '/chat/channels', null, WORKSPACE, 'channels', 200) },
  { weight: 25, run: (t) => api(t, 'GET', `/chat/channels/${CHANNEL}/messages?after_seq=0`, null, WORKSPACE, 'catch_up', 200) },
  { weight: 15, run: (t) => api(t, 'GET', '/workspace/members', null, WORKSPACE, 'members', 200) },
  { weight: 5, run: (t) => api(t, 'GET', '/notify/unread-count', null, WORKSPACE, 'unread', 200) },
  {
    weight: 15,
    run: (t) => api(t, 'POST', `/chat/channels/${CHANNEL}/messages`, { client_msg_id: uuid(), body: `rest mix ${uuid()}` }, WORKSPACE, 'send', 201),
  },
];

function pickStep() {
  let pick = Math.random() * 100;
  for (const step of MIX) {
    pick -= step.weight;
    if (pick < 0) return step;
  }
  return MIX[MIX.length - 1];
}

export function mix() {
  const user = USERS[exec.scenario.iterationInInstance];
  const token = refresh(user.refresh);
  const start = Date.now();
  for (let k = 0; k * PERIOD_MS < DURATION_MS; k++) {
    const wait = start + k * PERIOD_MS - Date.now();
    if (wait > 0) sleep(wait / 1000);
    pickStep().run(token);
  }
}

export function probe() {
  probeInstances();
  sleep(5);
}
