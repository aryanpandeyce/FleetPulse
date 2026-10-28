/**
 * FleetPulse Dashboard - Standalone Enterprise SPA
 * Multi-Tenant Server Fleet Management inspired by Cisco Intersight
 */

// Initial deterministic state
const TENANTS = {
  acme: {
    id: "c827b56a-1234-4b55-a22b-8a89201f92e1",
    name: "Acme Corporation",
    devices: [
      { id: "dev-001", serial: "UCS-C220-M7-00001", model: "UCS-C220-M7", firmware: "4.2(3a)", power: "on", health: "ok", inlet: 22.4, cpu: 48.2, fan: [4520, 4510, 4490, 4530], psu: [162, 158], errors: 0, rack: "R01", profile: "Production-C220-Golden", compliant: true },
      { id: "dev-002", serial: "UCS-C220-M7-00002", model: "UCS-C220-M7", firmware: "4.2(3a)", power: "on", health: "ok", inlet: 21.8, cpu: 47.9, fan: [4480, 4500, 4510, 4490], psu: [159, 161], errors: 0, rack: "R01", profile: "Production-C220-Golden", compliant: true },
      { id: "dev-003", serial: "UCS-C240-M7-00003", model: "UCS-C240-M7", firmware: "4.1(2b)", power: "on", health: "warning", inlet: 25.1, cpu: 74.5, fan: [5200, 5180, 5210, 5190], psu: [184, 180], errors: 12, rack: "R02", profile: "Production-C220-Golden", compliant: false, drift: "Firmware 4.1(2b) != Target 4.2(3a)" },
      { id: "dev-004", serial: "UCS-X210c-00004", model: "UCS-X210c", firmware: "4.2(3a)", power: "on", health: "critical", inlet: 38.6, cpu: 89.2, fan: [0, 6800, 6900, 6850], psu: [210, 205], errors: 64, rack: "CH-1", profile: "Production-C220-Golden", compliant: true, fault: "fan_failure" },
      { id: "dev-005", serial: "UCS-C220-M7-00005", model: "UCS-C220-M7", firmware: "4.2(3a)", power: "on", health: "ok", inlet: 22.0, cpu: 49.1, fan: [4500, 4520, 4500, 4510], psu: [160, 160], errors: 1, rack: "R03", profile: "Production-C220-Golden", compliant: true }
    ],
    alarms: [
      { id: "alm-81273a", serial: "UCS-X210c-00004", rule: "FanFailure", severity: "critical", status: "raised", metric: "fan_rpm", value: "0 RPM", time: "2 mins ago" },
      { id: "alm-90214b", serial: "UCS-X210c-00004", rule: "CriticalCPUTemp", severity: "critical", status: "raised", metric: "cpu_temp", value: "89.2 °C", time: "1 min ago" },
      { id: "alm-10492c", serial: "UCS-C240-M7-00003", rule: "HighInletTemp", severity: "warning", status: "acknowledged", metric: "inlet_temp", value: "25.1 °C", time: "14 mins ago" }
    ],
    workflows: [
      { id: "job-fa8120", type: "firmware_upgrade", priority: 1, state: "succeeded", targets: ["UCS-C220-M7-00001"], created: "10 mins ago", steps: ["PrerequisiteValidation (success)", "TriggerFirmwareFlash (success)", "VerifyUpgradeStatus (success)"] },
      { id: "job-bc9012", type: "drift_remediation", priority: 2, state: "running", targets: ["UCS-C240-M7-00003"], created: "3 mins ago", steps: ["PrerequisiteValidation (success)", "ApplyServerProfile (running)"] }
    ]
  },
  globex: {
    id: "d938c67b-5678-4c66-b33c-9b90312fa3f2",
    name: "Globex Industries",
    devices: [
      { id: "dev-101", serial: "UCS-C240-M7-00006", model: "UCS-C240-M7", firmware: "4.2(3a)", power: "on", health: "ok", inlet: 21.5, cpu: 46.8, fan: [4450, 4420, 4460, 4450], psu: [155, 154], errors: 0, rack: "R10", profile: "Compute-C240-Standard", compliant: true },
      { id: "dev-102", serial: "UCS-C240-M7-00007", model: "UCS-C240-M7", firmware: "4.2(3a)", power: "on", health: "ok", inlet: 22.1, cpu: 47.3, fan: [4470, 4450, 4480, 4460], psu: [158, 157], errors: 0, rack: "R10", profile: "Compute-C240-Standard", compliant: true },
      { id: "dev-103", serial: "UCS-X210c-00008", model: "UCS-X210c", firmware: "4.0(1a)", power: "on", health: "warning", inlet: 23.4, cpu: 71.0, fan: [4950, 4920, 4960, 4940], psu: [175, 172], errors: 5, rack: "CH-A", profile: "Compute-C240-Standard", compliant: false, drift: "Firmware 4.0(1a) != Target 4.2(3a)" }
    ],
    alarms: [
      { id: "alm-44120a", serial: "UCS-X210c-00008", rule: "MemoryErrorRate", severity: "warning", status: "raised", metric: "memory_errors", value: "5 errors/hr", time: "8 mins ago" }
    ],
    workflows: []
  }
};

let currentTenantKey = "acme";
let currentView = "overview";
let selectedDeviceSerial = "UCS-C220-M7-00001";
let searchFilter = "";
let healthFilter = "all";

// Periodic telemetry random walk simulator
setInterval(() => {
  const tenant = TENANTS[currentTenantKey];
  tenant.devices.forEach(d => {
    if (d.fault === "thermal_spike") {
      d.cpu = Math.min(96.0, d.cpu + 0.8);
      d.inlet = Math.min(42.0, d.inlet + 0.3);
      d.health = "critical";
    } else if (d.fault === "fan_failure") {
      d.fan[0] = 0;
      d.cpu = Math.min(92.0, d.cpu + 0.5);
      d.health = "critical";
    } else {
      // Normal walk
      d.inlet = +(d.inlet + (Math.random() - 0.5) * 0.2).toFixed(1);
      d.cpu = +(d.cpu + (Math.random() - 0.5) * 0.4).toFixed(1);
      d.fan = d.fan.map(f => Math.round(f + (Math.random() - 0.5) * 20));
      d.psu = d.psu.map(p => Math.round(p + (Math.random() - 0.5) * 4));
    }
  });
  if (currentView === "device-detail") {
    renderDeviceDetail();
  }
}, 3000);

function initApp() {
  document.getElementById("tenant-selector").addEventListener("change", (e) => {
    currentTenantKey = e.target.value;
    render();
  });

  document.querySelectorAll(".nav-item").forEach(item => {
    item.addEventListener("click", () => {
      document.querySelectorAll(".nav-item").forEach(n => n.classList.remove("active"));
      item.classList.add("active");
      currentView = item.dataset.view;
      render();
    });
  });

  render();
}

function render() {
  const tenant = TENANTS[currentTenantKey];
  const container = document.getElementById("main-content");

  switch (currentView) {
    case "overview":
      renderOverview(container, tenant);
      break;
    case "devices":
      renderDevices(container, tenant);
      break;
    case "device-detail":
      renderDeviceDetail(container, tenant);
      break;
    case "alarms":
      renderAlarms(container, tenant);
      break;
    case "workflows":
      renderWorkflows(container, tenant);
      break;
  }
}

function renderOverview(container, tenant) {
  const okCount = tenant.devices.filter(d => d.health === "ok").length;
  const warnCount = tenant.devices.filter(d => d.health === "warning").length;
  const critCount = tenant.devices.filter(d => d.health === "critical").length;
  const activeAlarms = tenant.alarms.filter(a => a.status === "raised").length;

  container.innerHTML = `
    <div class="grid-stats">
      <div class="card">
        <div class="card-title">Fleet Servers</div>
        <div class="card-val" style="color: var(--accent-cyan);">${tenant.devices.length}</div>
        <div class="card-subtext">Across UCS C220, C240 & X-Series</div>
      </div>
      <div class="card">
        <div class="card-title">Healthy (OK)</div>
        <div class="card-val" style="color: var(--health-ok);">${okCount}</div>
        <div class="card-subtext">${Math.round((okCount/tenant.devices.length)*100)}% compliance</div>
      </div>
      <div class="card">
        <div class="card-title">Warning / Drift</div>
        <div class="card-val" style="color: var(--health-warning);">${warnCount}</div>
        <div class="card-subtext">Config or firmware drift</div>
      </div>
      <div class="card">
        <div class="card-title">Active Alarms</div>
        <div class="card-val" style="color: var(--health-critical);">${activeAlarms}</div>
        <div class="card-subtext">${critCount} critical servers</div>
      </div>
    </div>

    <div style="display: grid; grid-template-columns: 2fr 1fr; gap: 20px; margin-bottom: 24px;">
      <div class="card">
        <div class="card-title">Fleet Telemetry Distribution (CPU Temperatures °C)</div>
        <div class="chart-container">
          ${tenant.devices.map(d => {
            const height = Math.min(100, Math.max(20, (d.cpu / 100) * 180));
            const color = d.health === 'critical' ? 'var(--health-critical)' : d.health === 'warning' ? 'var(--health-warning)' : 'var(--accent-cyan)';
            return `<div class="chart-bar" style="height: ${height}px; background: ${color};" data-tooltip="${d.serial}: ${d.cpu}°C"></div>`;
          }).join('')}
        </div>
        <div style="display: flex; justify-content: space-between; font-size: 11px; color: var(--text-muted); margin-top: 8px;">
          <span>Target baseline: 48°C</span>
          <span>Critical threshold: 85°C</span>
        </div>
      </div>

      <div class="card">
        <div class="card-title">Anomaly Detection Engine (EWMA + Z-Score)</div>
        <div style="display: flex; flex-direction: column; gap: 12px; margin-top: 10px;">
          ${tenant.devices.filter(d => d.health === 'critical' || d.health === 'warning').map(d => `
            <div style="background: rgba(255,255,255,0.03); padding: 10px; border-radius: 6px; border-left: 3px solid ${d.health === 'critical' ? 'var(--health-critical)' : 'var(--health-warning)'};">
              <div style="display: flex; justify-content: space-between; font-size: 12px; font-weight: 600;">
                <span class="font-mono">${d.serial}</span>
                <span style="color: var(--health-critical);">Z-Score: +${(d.cpu > 80 ? 4.8 : 2.6).toFixed(1)}σ</span>
              </div>
              <div style="font-size: 11px; color: var(--text-secondary); margin-top: 4px;">
                Observed CPU ${d.cpu}°C deviates from EWMA baseline 48.0°C
              </div>
            </div>
          `).join('') || '<div style="color: var(--text-muted); font-size: 13px;">No statistical anomalies detected in fleet.</div>'}
        </div>
      </div>
    </div>
  `;
}

function renderDevices(container, tenant) {
  const filtered = tenant.devices.filter(d => {
    const matchesSearch = d.serial.toLowerCase().includes(searchFilter.toLowerCase()) || d.model.toLowerCase().includes(searchFilter.toLowerCase());
    const matchesHealth = healthFilter === "all" || d.health === healthFilter;
    return matchesSearch && matchesHealth;
  });

  container.innerHTML = `
    <div style="display: flex; justify-content: space-between; align-items: center; margin-bottom: 20px;">
      <div>
        <h2 style="font-size: 20px; font-weight: 700;">Server Inventory</h2>
        <div style="color: var(--text-secondary); font-size: 13px;">Keyset cursor pagination & real-time telemetry</div>
      </div>
      <div style="display: flex; gap: 12px;">
        <input type="text" placeholder="Search serial or model..." id="device-search" value="${searchFilter}"
          style="background: var(--bg-card); border: 1px solid var(--border-color); color: #fff; padding: 6px 14px; border-radius: 6px; font-size: 13px; outline: none;">
        <select id="health-filter" style="background: var(--bg-card); border: 1px solid var(--border-color); color: #fff; padding: 6px 12px; border-radius: 6px; font-size: 13px;">
          <option value="all" ${healthFilter === 'all' ? 'selected' : ''}>All Health States</option>
          <option value="ok" ${healthFilter === 'ok' ? 'selected' : ''}>OK</option>
          <option value="warning" ${healthFilter === 'warning' ? 'selected' : ''}>Warning</option>
          <option value="critical" ${healthFilter === 'critical' ? 'selected' : ''}>Critical</option>
        </select>
      </div>
    </div>

    <div class="table-wrapper">
      <table>
        <thead>
          <tr>
            <th>Serial</th>
            <th>Model</th>
            <th>Health</th>
            <th>Firmware</th>
            <th>CPU Temp</th>
            <th>Fans (RPM)</th>
            <th>Power</th>
            <th>Compliance</th>
            <th>Actions</th>
          </tr>
        </thead>
        <tbody>
          ${filtered.map(d => `
            <tr>
              <td class="font-mono" style="font-weight: 600; color: #38bdf8; cursor: pointer;" onclick="selectDevice('${d.serial}')">${d.serial}</td>
              <td>${d.model}</td>
              <td><span class="pill pill-${d.health}">${d.health.toUpperCase()}</span></td>
              <td class="font-mono">${d.firmware}</td>
              <td>${d.cpu} °C</td>
              <td class="font-mono">${d.fan[0]} RPM</td>
              <td>${d.psu[0] + d.psu[1]} W</td>
              <td>
                ${d.compliant 
                  ? '<span style="color: var(--health-ok); font-weight: 600;">✓ Compliant</span>' 
                  : '<span style="color: var(--health-warning); font-weight: 600;">⚠ Drifted</span>'}
              </td>
              <td>
                <button class="btn btn-secondary btn-sm" onclick="selectDevice('${d.serial}')">View Sensors</button>
              </td>
            </tr>
          `).join('')}
        </tbody>
      </table>
    </div>
  `;

  document.getElementById("device-search").addEventListener("input", (e) => {
    searchFilter = e.target.value;
    renderDevices(container, tenant);
  });
  document.getElementById("health-filter").addEventListener("change", (e) => {
    healthFilter = e.target.value;
    renderDevices(container, tenant);
  });
}

function selectDevice(serial) {
  selectedDeviceSerial = serial;
  currentView = "device-detail";
  document.querySelectorAll(".nav-item").forEach(n => n.classList.remove("active"));
  document.querySelector('[data-view="device-detail"]').classList.add("active");
  render();
}

function renderDeviceDetail() {
  const container = document.getElementById("main-content");
  const tenant = TENANTS[currentTenantKey];
  const d = tenant.devices.find(dev => dev.serial === selectedDeviceSerial) || tenant.devices[0];

  container.innerHTML = `
    <div style="display: flex; justify-content: space-between; align-items: center; margin-bottom: 20px;">
      <div>
        <div style="display: flex; align-items: center; gap: 12px;">
          <h2 class="font-mono" style="font-size: 22px; font-weight: 700;">${d.serial}</h2>
          <span class="pill pill-${d.health}">${d.health.toUpperCase()}</span>
          ${d.fault && d.fault !== 'none' ? `<span style="background: rgba(239,68,68,0.2); border: 1px solid var(--health-critical); color: #fca5a5; padding: 2px 8px; border-radius: 4px; font-size: 11px;">FAULT: ${d.fault}</span>` : ''}
        </div>
        <div style="color: var(--text-secondary); font-size: 13px;">${d.model} &bull; Rack: ${d.rack} &bull; Desired Profile: ${d.profile}</div>
      </div>
      <div style="display: flex; gap: 10px;">
        <button class="btn btn-secondary btn-sm" onclick="injectFault('${d.serial}', 'thermal_spike')">🔥 Thermal Spike</button>
        <button class="btn btn-secondary btn-sm" onclick="injectFault('${d.serial}', 'fan_failure')">❌ Fan Failure</button>
        <button class="btn btn-secondary btn-sm" onclick="injectFault('${d.serial}', 'none')">🧹 Clear Fault</button>
        <button class="btn btn-sm" onclick="openUpgradeModal('${d.serial}')">🚀 Upgrade Firmware</button>
      </div>
    </div>

    <div class="grid-stats">
      <div class="card">
        <div class="card-title">CPU Temperature</div>
        <div class="card-val" style="color: ${d.cpu > 80 ? 'var(--health-critical)' : d.cpu > 70 ? 'var(--health-warning)' : '#38bdf8'};">${d.cpu} °C</div>
        <div class="card-subtext">EWMA Expected: 48.0 ± 4.2 °C</div>
      </div>
      <div class="card">
        <div class="card-title">Inlet Temperature</div>
        <div class="card-val">${d.inlet} °C</div>
        <div class="card-subtext">Ambient normal (threshold 35°C)</div>
      </div>
      <div class="card">
        <div class="card-title">Fan Tachometer (Fan 1)</div>
        <div class="card-val" style="color: ${d.fan[0] === 0 ? 'var(--health-critical)' : '#38bdf8'};">${d.fan[0]} RPM</div>
        <div class="card-subtext">Fans 2-4: ${d.fan[1]} / ${d.fan[2]} / ${d.fan[3]} RPM</div>
      </div>
      <div class="card">
        <div class="card-title">Power Consumption</div>
        <div class="card-val">${d.psu[0] + d.psu[1]} W</div>
        <div class="card-subtext">PSU 1: ${d.psu[0]}W &bull; PSU 2: ${d.psu[1]}W</div>
      </div>
    </div>

    <div class="card" style="margin-top: 20px;">
      <div class="card-title">Golden Server Profile Compliance & Drift</div>
      <div style="display: flex; justify-content: space-between; align-items: center; padding: 14px 0; border-bottom: 1px solid var(--border-color);">
        <div>
          <div style="font-weight: 600;">Firmware Version</div>
          <div style="font-size: 12px; color: var(--text-muted);">Current: ${d.firmware} &bull; Target: 4.2(3a)</div>
        </div>
        <div>
          ${d.compliant 
            ? '<span class="pill pill-ok">Compliant</span>' 
            : '<span class="pill pill-warning">Drift Detected</span>'}
        </div>
      </div>
      <div style="display: flex; justify-content: space-between; align-items: center; padding: 14px 0;">
        <div>
          <div style="font-weight: 600;">Boot Order & Power Policy</div>
          <div style="font-size: 12px; color: var(--text-muted);">NVMe, PXE &bull; Performance Profile</div>
        </div>
        <div><span class="pill pill-ok">Compliant</span></div>
      </div>
    </div>
  `;
}

function injectFault(serial, fault) {
  const tenant = TENANTS[currentTenantKey];
  const d = tenant.devices.find(dev => dev.serial === serial);
  if (!d) return;

  d.fault = fault;
  if (fault === "thermal_spike") {
    d.cpu = 88.5;
    d.inlet = 38.0;
    d.health = "critical";
    tenant.alarms.unshift({
      id: "alm-" + Math.random().toString(36).substring(2, 8),
      serial: d.serial,
      rule: "HighInletTemp",
      severity: "warning",
      status: "raised",
      metric: "inlet_temp",
      value: "38.0 °C",
      time: "Just now"
    });
  } else if (fault === "fan_failure") {
    d.fan[0] = 0;
    d.cpu = 89.2;
    d.health = "critical";
    tenant.alarms.unshift({
      id: "alm-" + Math.random().toString(36).substring(2, 8),
      serial: d.serial,
      rule: "FanFailure",
      severity: "critical",
      status: "raised",
      metric: "fan_rpm",
      value: "0 RPM",
      time: "Just now"
    });
  } else {
    d.cpu = 48.0;
    d.inlet = 22.0;
    d.fan = [4500, 4510, 4500, 4520];
    d.health = "ok";
  }
  render();
}

function renderAlarms(container, tenant) {
  container.innerHTML = `
    <div style="display: flex; justify-content: space-between; align-items: center; margin-bottom: 20px;">
      <div>
        <h2 style="font-size: 20px; font-weight: 700;">Alarms Triage & Anomaly Alerts</h2>
        <div style="color: var(--text-secondary); font-size: 13px;">Deduplicated stateful alerts with operator acknowledgment</div>
      </div>
    </div>

    <div class="table-wrapper">
      <table>
        <thead>
          <tr>
            <th>Alarm ID</th>
            <th>Server</th>
            <th>Rule / Anomaly</th>
            <th>Severity</th>
            <th>Trigger Value</th>
            <th>Status</th>
            <th>Time</th>
            <th>Actions</th>
          </tr>
        </thead>
        <tbody>
          ${tenant.alarms.map(a => `
            <tr>
              <td class="font-mono">${a.id}</td>
              <td class="font-mono" style="font-weight: 600; color: #38bdf8;">${a.serial}</td>
              <td><strong>${a.rule}</strong></td>
              <td><span class="pill pill-${a.severity}">${a.severity.toUpperCase()}</span></td>
              <td class="font-mono">${a.value}</td>
              <td>
                <span style="font-weight: 600; color: ${a.status === 'raised' ? 'var(--health-critical)' : 'var(--health-ok)'};">
                  ${a.status.toUpperCase()}
                </span>
              </td>
              <td style="color: var(--text-muted);">${a.time}</td>
              <td>
                ${a.status === 'raised' 
                  ? `<button class="btn btn-secondary btn-sm" onclick="ackAlarm('${a.id}')">Acknowledge</button>` 
                  : '<span style="color: var(--text-muted); font-size: 12px;">Acknowledged</span>'}
              </td>
            </tr>
          `).join('')}
        </tbody>
      </table>
    </div>
  `;
}

function ackAlarm(id) {
  const tenant = TENANTS[currentTenantKey];
  const alarm = tenant.alarms.find(a => a.id === id);
  if (alarm) {
    alarm.status = "acknowledged";
    render();
  }
}

function renderWorkflows(container, tenant) {
  container.innerHTML = `
    <div style="display: flex; justify-content: space-between; align-items: center; margin-bottom: 20px;">
      <div>
        <h2 style="font-size: 20px; font-weight: 700;">Workflow Orchestrator</h2>
        <div style="color: var(--text-secondary); font-size: 13px;">Priority min-heap scheduler with idempotency and step telemetry</div>
      </div>
      <button class="btn" onclick="openUpgradeModal()">+ Create Firmware Upgrade</button>
    </div>

    <div class="table-wrapper">
      <table>
        <thead>
          <tr>
            <th>Job ID</th>
            <th>Type</th>
            <th>Priority (Min-Heap)</th>
            <th>Targets</th>
            <th>State</th>
            <th>Milestone Steps</th>
            <th>Created</th>
          </tr>
        </thead>
        <tbody>
          ${tenant.workflows.map(j => `
            <tr>
              <td class="font-mono" style="font-weight: 600; color: #38bdf8;">${j.id}</td>
              <td>${j.type}</td>
              <td>
                <span class="pill ${j.priority === 1 ? 'pill-critical' : 'pill-ok'}">
                  Priority ${j.priority} (${j.priority === 1 ? 'High' : 'Normal'})
                </span>
              </td>
              <td class="font-mono">${j.targets.join(', ')}</td>
              <td>
                <span class="pill ${j.state === 'succeeded' ? 'pill-ok' : j.state === 'running' ? 'pill-warning' : 'pill-critical'}">
                  ${j.state.toUpperCase()}
                </span>
              </td>
              <td style="font-size: 12px; color: var(--text-secondary);">
                ${j.steps.join(' &rarr; ')}
              </td>
              <td style="color: var(--text-muted);">${j.created}</td>
            </tr>
          `).join('')}
        </tbody>
      </table>
    </div>
  `;
}

function openUpgradeModal(targetSerial) {
  const modal = document.createElement("div");
  modal.className = "modal-overlay";
  modal.id = "upgrade-modal";
  modal.innerHTML = `
    <div class="modal-content">
      <h3 style="font-size: 18px; font-weight: 700; margin-bottom: 12px;">Dispatch Firmware Upgrade</h3>
      <p style="font-size: 13px; color: var(--text-secondary); margin-bottom: 20px;">
        Asynchronous workflow dispatched to the min-heap priority scheduler.
      </p>
      <div style="margin-bottom: 16px;">
        <label style="display: block; font-size: 12px; margin-bottom: 6px; color: var(--text-muted);">Target Server Serial</label>
        <input type="text" id="modal-target" value="${targetSerial || 'UCS-C220-M7-00001'}"
          style="width: 100%; background: var(--bg-primary); border: 1px solid var(--border-color); color: #fff; padding: 8px 12px; border-radius: 6px; font-size: 13px;">
      </div>
      <div style="margin-bottom: 20px;">
        <label style="display: block; font-size: 12px; margin-bottom: 6px; color: var(--text-muted);">Target Firmware Bundle</label>
        <select id="modal-firmware" style="width: 100%; background: var(--bg-primary); border: 1px solid var(--border-color); color: #fff; padding: 8px 12px; border-radius: 6px; font-size: 13px;">
          <option value="4.3(1a)">Cisco-UCSC-M7-Bundle v4.3(1a) (Recommended)</option>
          <option value="4.2(3b)">Cisco-UCSC-M7-Bundle v4.2(3b)</option>
        </select>
      </div>
      <div style="display: flex; justify-content: flex-end; gap: 10px;">
        <button class="btn btn-secondary" onclick="document.getElementById('upgrade-modal').remove()">Cancel</button>
        <button class="btn" onclick="submitUpgrade()">Confirm & Dispatch</button>
      </div>
    </div>
  `;
  document.body.appendChild(modal);
}

function submitUpgrade() {
  const target = document.getElementById("modal-target").value;
  const ver = document.getElementById("modal-firmware").value;
  const tenant = TENANTS[currentTenantKey];

  const newJob = {
    id: "job-" + Math.random().toString(36).substring(2, 8),
    type: "firmware_upgrade",
    priority: 1,
    state: "running",
    targets: [target],
    created: "Just now",
    steps: ["PrerequisiteValidation (success)", "TriggerFirmwareFlash (running)"]
  };

  tenant.workflows.unshift(newJob);
  document.getElementById("upgrade-modal").remove();

  // Progress job in 2 seconds
  setTimeout(() => {
    newJob.state = "succeeded";
    newJob.steps.push(`VerifyUpgradeStatus (success: ${ver})`);
    const dev = tenant.devices.find(d => d.serial === target);
    if (dev) {
      dev.firmware = ver;
      dev.compliant = true;
    }
    render();
  }, 2500);

  currentView = "workflows";
  document.querySelectorAll(".nav-item").forEach(n => n.classList.remove("active"));
  document.querySelector('[data-view="workflows"]').classList.add("active");
  render();
}

// Window load init
window.addEventListener("DOMContentLoaded", initApp);
