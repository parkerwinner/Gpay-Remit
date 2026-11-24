import http from 'k6/http';
import { check, sleep } from 'k6';

export const options = {
  stages: [
    { duration: '30s', target: 50 },
    { duration: '1m', target: 50 },
    { duration: '30s', target: 0 },
  ],
  thresholds: {
    'http_req_duration{endpoint:read}': ['p(99)<500'],
    checks: ['rate>0.95'],
  },
};

const BASE_URL = (__ENV.BASE_URL || 'http://localhost:8080').replace(/\/$/, '');
const API_BASE_URL = `${BASE_URL}/api/v1`;

export function setup() {
  const loginRes = http.post(`${API_BASE_URL}/auth/login`, JSON.stringify({
    email: __ENV.TEST_EMAIL || 'integration@example.com',
    password: __ENV.TEST_PASSWORD || 'Integration-FullFlow-2026!9',
  }), { headers: { 'Content-Type': 'application/json' } });
  const accessToken = loginRes.json('access_token');
  if (loginRes.status !== 200 || !accessToken) {
    throw new Error(`Staging load-test login failed with status ${loginRes.status}`);
  }
  return { accessToken };
}

export default function (data) {
  const healthRes = http.get(`${BASE_URL}/health/ready`, { tags: { endpoint: 'read' } });
  check(healthRes, {
    'readiness check status is 200': (r) => r.status === 200,
  });

  const remittancesRes = http.get(`${API_BASE_URL}/remittances?page_size=20`, {
    headers: { Authorization: `Bearer ${data.accessToken}` },
    tags: { endpoint: 'read' },
  });
  check(remittancesRes, {
    'remittance read status is 200': (r) => r.status === 200,
  });

  sleep(1);
}
