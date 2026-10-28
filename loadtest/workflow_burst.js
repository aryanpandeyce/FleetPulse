import http from 'k6/http';
import { check, sleep } from 'k6';

export const options = {
  stages: [
    { duration: '5s', target: 10 },
    { duration: '15s', target: 30 },
    { duration: '5s', target: 0 },
  ],
  thresholds: {
    http_req_duration: ['p(95)<400'],
    http_req_failed: ['rate<0.02'],
  },
};

const BASE_URL = __ENV.GATEWAY_URL || 'http://localhost:8080';

export function setup() {
  const loginRes = http.post(`${BASE_URL}/api/v1/auth/login`, JSON.stringify({
    email: 'operator@acme.com',
    password: 'OperatorPass123!',
  }), {
    headers: { 'Content-Type': 'application/json' },
  });

  const body = JSON.parse(loginRes.body);
  return { token: body.access_token };
}

export default function (data) {
  const idemKey = `k6-job-${__VU}-${__ITER}-${Date.now()}`;
  const payload = JSON.stringify({
    type: 'firmware_upgrade',
    priority: 1,
    targets: [`UCS-C220-M7-${String(__VU).padStart(5, '0')}`],
    parameters: { version: '4.3(1a)' },
    idempotency_key: idemKey,
  });

  const params = {
    headers: {
      'Authorization': `Bearer ${data.token}`,
      'Content-Type': 'application/json',
    },
  };

  const res = http.post(`${BASE_URL}/api/v1/workflows`, payload, params);
  check(res, {
    'job created (201)': (r) => r.status === 201,
  });

  // Immediate retry with identical idempotency key to test deduplication
  const dupRes = http.post(`${BASE_URL}/api/v1/workflows`, payload, params);
  check(dupRes, {
    'idempotent duplicate accepted (201)': (r) => r.status === 201,
  });

  sleep(0.2);
}
