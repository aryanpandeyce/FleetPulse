import http from 'k6/http';
import { check, sleep } from 'k6';

export const options = {
  stages: [
    { duration: '10s', target: 20 },  // Ramp-up
    { duration: '20s', target: 50 },  // Sustained load
    { duration: '5s', target: 0 },    // Ramp-down
  ],
  thresholds: {
    http_req_duration: ['p(95)<300', 'p(99)<500'], // 95% of requests under 300ms
    http_req_failed: ['rate<0.01'],                 // Error rate < 1%
  },
};

const BASE_URL = __ENV.GATEWAY_URL || 'http://localhost:8080';

export function setup() {
  // Login to acquire operator JWT
  const loginRes = http.post(`${BASE_URL}/api/v1/auth/login`, JSON.stringify({
    email: 'operator@acme.com',
    password: 'OperatorPass123!',
  }), {
    headers: { 'Content-Type': 'application/json' },
  });

  check(loginRes, {
    'login succeeded (200)': (r) => r.status === 200,
  });

  const body = JSON.parse(loginRes.body);
  return { token: body.access_token };
}

export default function (data) {
  const params = {
    headers: {
      'Authorization': `Bearer ${data.token}`,
      'Accept': 'application/json',
    },
  };

  // 1. List devices (keyset paginated)
  const devRes = http.get(`${BASE_URL}/api/v1/devices?limit=25`, params);
  check(devRes, {
    'list devices 200': (r) => r.status === 200,
  });

  // 2. Get fleet summary
  const sumRes = http.get(`${BASE_URL}/api/v1/fleet/summary`, params);
  check(sumRes, {
    'fleet summary 200': (r) => r.status === 200,
  });

  sleep(0.1);
}
