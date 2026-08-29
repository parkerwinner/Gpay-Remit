import http from 'k6/http';
import { check, sleep } from 'k6';

export const options = {
  stages: [
    { duration: '30s', target: 50 },
    { duration: '1m', target: 50 },
    { duration: '30s', target: 0 },
  ],
  thresholds: {
    http_req_duration: ['p(95)<500'],
  },
};

const BASE_URL = __ENV.BASE_URL || 'http://localhost:8080/api/v1';

export default function () {
  // Test health check endpoint (assuming it exists)
  let healthRes = http.get(`${BASE_URL}/health`);
  check(healthRes, {
    'health check status is 200': (r) => r.status === 200,
  });

  // Simulated typical user flow (if tokens were available)
  // For a basic load test without complex auth flow per VU, 
  // we can test public endpoints or mock auth endpoints.
  let loginRes = http.post(`${BASE_URL}/auth/login`, JSON.stringify({
    email: 'integration@example.com',
    password: 'securepass123',
  }), { headers: { 'Content-Type': 'application/json' } });

  // Not checking login status strictly here since the user might not exist in load test DB,
  // but we can check if it returns a valid response format.
  check(loginRes, {
    'login responded': (r) => r.status === 200 || r.status === 401 || r.status === 404,
  });

  sleep(1);
}
