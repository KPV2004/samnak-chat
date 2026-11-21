// Seed step of make load. Creates an owner, a workspace, three public channels
// and the users each scenario uses, and writes their refresh tokens to
// /out/fixture.json. Refresh tokens are single use: every run seeds a new set,
// and each scenario uses each user once.
//
// Groups: rest (REST mix), senders_direct and direct (1 channel, sockets on one
// instance), senders_fanout and fanout (sockets through Caddy on both instances).
import http from 'k6/http';
import { fail } from 'k6';
import { API, JSON_HEADERS, bearer } from './lib.js';

const RUN = Date.now().toString(36);
const PASSWORD = `load-${RUN}-pass`;
const SIZE = {
  rest: Number(__ENV.REST_USERS || 40),
  senders_direct: Number(__ENV.SENDERS || 10),
  direct: Number(__ENV.DIRECT_SOCKETS || 1000),
  senders_fanout: Number(__ENV.SENDERS || 10),
  fanout: Number(__ENV.FANOUT_SOCKETS || 200),
};

export const options = {
  setupTimeout: '20m',
  // Concurrency of http.batch: the seed is bound by argon2 on the server, not by
  // the number of round trips.
  batch: 50,
  batchPerHost: 50,
  vus: 1,
  iterations: 1,
};

function expect(res, status, what) {
  if (res.status !== status) {
    fail(`seed: ${what}: HTTP ${res.status} ${res.body}`);
  }
  return res;
}

function expectAll(responses, status, what) {
  responses.forEach((res) => expect(res, status, what));
  return responses;
}

export function setup() {
  const started = Date.now();
  const ownerEmail = `load-${RUN}-owner@example.com`;
  expect(
    http.post(`${API}/auth/register`, JSON.stringify({ email: ownerEmail, password: PASSWORD, display_name: 'Load owner' }), { headers: JSON_HEADERS }),
    201,
    'owner register',
  );
  const ownerLogin = expect(
    http.post(`${API}/auth/login`, JSON.stringify({ email: ownerEmail, password: PASSWORD, device_name: 'load seed', platform: 'web' }), { headers: JSON_HEADERS }),
    200,
    'owner login',
  );
  const owner = ownerLogin.json('access_token');

  const workspaceId = expect(
    http.post(`${API}/workspace`, JSON.stringify({ name: `Load ${RUN}`, slug: `load-${RUN}` }), bearer(owner)),
    201,
    'workspace',
  ).json('id');
  const ownerAuth = bearer(owner, workspaceId);

  const channels = {};
  for (const name of ['direct', 'fanout', 'rest']) {
    channels[name] = expect(
      http.post(`${API}/chat/channels`, JSON.stringify({ type: 'public', name: `${name}-${RUN}` }), ownerAuth),
      201,
      `channel ${name}`,
    ).json('id');
  }

  const users = {};
  const all = [];
  for (const [group, n] of Object.entries(SIZE)) {
    users[group] = [];
    for (let i = 0; i < n; i++) {
      const u = { group, email: `load-${RUN}-${group}-${i}@example.com`, displayName: `${group} ${i}` };
      users[group].push(u);
      all.push(u);
    }
  }

  expectAll(
    http.batch(all.map((u) => ['POST', `${API}/auth/register`, JSON.stringify({ email: u.email, password: PASSWORD, display_name: u.displayName }), { headers: JSON_HEADERS }])),
    201,
    'register',
  ).forEach((res, i) => {
    all[i].id = res.json('id');
  });

  expectAll(
    http.batch(all.map((u) => ['POST', `${API}/auth/login`, JSON.stringify({ email: u.email, password: PASSWORD, device_name: `load ${u.group}`, platform: 'web' }), { headers: JSON_HEADERS }])),
    200,
    'login',
  ).forEach((res, i) => {
    all[i].access = res.json('access_token');
    all[i].refresh = res.json('refresh_token');
  });

  // One link invite per user; each is used once, by that user's accept call.
  expectAll(
    http.batch(all.map(() => ['POST', `${API}/workspace/invites`, JSON.stringify({ role: 'member' }), bearer(owner, workspaceId)])),
    201,
    'invite',
  ).forEach((res, i) => {
    all[i].invite = res.json('token');
  });
  expectAll(
    http.batch(all.map((u) => ['POST', `${API}/workspace/invites/accept`, JSON.stringify({ token: u.invite }), bearer(u.access)])),
    200,
    'accept invite',
  );

  const addMembers = (channelId, list) => {
    for (let i = 0; i < list.length; i += 100) {
      const ids = list.slice(i, i + 100).map((u) => u.id);
      expect(
        http.post(`${API}/chat/channels/${channelId}/members`, JSON.stringify({ user_ids: ids }), ownerAuth),
        204,
        'add members',
      );
    }
  };
  addMembers(channels.direct, [...users.senders_direct, ...users.direct]);
  addMembers(channels.fanout, [...users.senders_fanout, ...users.fanout]);
  addMembers(channels.rest, users.rest);

  const pick = (list) => list.map((u) => ({ id: u.id, refresh: u.refresh }));
  const fixture = {
    runId: RUN,
    workspaceId,
    channels,
    users: Object.fromEntries(Object.entries(users).map(([group, list]) => [group, pick(list)])),
  };
  console.log(`seed: ${all.length} users, workspace ${workspaceId}, ${((Date.now() - started) / 1000).toFixed(1)} s`);
  return fixture;
}

export default function () {}

export function handleSummary(data) {
  return { '/out/fixture.json': JSON.stringify(data.setup_data) };
}
