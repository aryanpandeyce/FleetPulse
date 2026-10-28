import http from 'k6/http';
import { check, sleep } from 'k6';

export const options = {
  stages: [
    { duration: '10s', target: 50 },
    { duration: '20s', target: 100 },
    { duration: '5s', target: 0 },
  ],
  thresholds: {
    http_req_duration: ['p(95)<200'],
    http_req_failed: ['rate<0.01'],
  },
};

const BASE_URL = __ENV.SIMULATOR_URL || 'http://localhost:8081';

export default function () {
  const serial = `UCS-C220-M7-${String(__VU).padStart(5, '0')}`;

  // Read thermal sensor data from simulated Redfish endpoint
  const res = http.get(`${BASE_URL}/redfish/v1/Systems/${serial}/Thermal`);
  check(res, {
    'redfish thermal 200': (r) => r.status === 200 || r.status === 404, // 404 if beyond generated count
  });

  sleep(0.05);
}
