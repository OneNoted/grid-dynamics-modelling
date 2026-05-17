const app = document.querySelector('#app');
const fmt = (value, digits = 2) => Number.isFinite(value) ? value.toFixed(digits) : '—';
async function getJSON(path) {
  const response = await fetch(path);
  if (!response.ok) throw new Error(`${path}: ${response.status}`);
  return response.json();
}
function card(label, value, suffix = '') {
  return `<article class="card"><b>${label}</b><span>${value}${suffix}</span></article>`;
}
function pointsPath(points, key, width, height) {
  const numeric = points.map((p, i) => ({ x: i, y: Number(p[key]) })).filter(p => Number.isFinite(p.y));
  if (!numeric.length) return '';
  const max = Math.max(...numeric.map(p => p.y));
  const min = Math.min(...numeric.map(p => p.y));
  const span = Math.max(1e-9, max - min);
  const last = Math.max(1, numeric[numeric.length - 1].x);
  return numeric.map((p, i) => `${i ? 'L' : 'M'} ${(p.x / last) * width} ${height - ((p.y - min) / span) * (height - 24) - 12}`).join(' ');
}
function eventRects(points, width, height) {
  const last = Math.max(1, points.length - 1);
  return points.map((p, i) => p.event_active ? `<rect class="event" x="${(i / last) * width}" y="0" width="${Math.max(1, width / last)}" height="${height}" />` : '').join('');
}
function chart(title, baseline, controlled, key) {
  const width = 1080, height = 260;
  return `<section class="panel"><h2>${title}</h2><svg class="chart" viewBox="0 0 ${width} ${height}" role="img" aria-label="${title}">${eventRects(controlled, width, height)}<line class="axis" x1="0" y1="${height - 12}" x2="${width}" y2="${height - 12}"/><path class="baseline" d="${pointsPath(baseline, key, width, height)}"/><path class="controlled" d="${pointsPath(controlled, key, width, height)}"/></svg><p class="legend"><span><i style="background:#ffb86b"></i>Baseline</span><span><i style="background:#6ee7ff"></i>Controlled</span><span><i style="background:#ef4444"></i>PMU event</span></p></section>`;
}
function queueChart(controlled) {
  const width = 1080, height = 220;
  return `<section class="panel"><h2>Deferred workload queue</h2><svg class="chart" viewBox="0 0 ${width} ${height}" role="img" aria-label="Deferred queue"><path class="queue" d="${pointsPath(controlled, 'deferred_queue_mwh', width, height)}"/></svg></section>`;
}
async function main() {
  const [manifest, metrics, baseline, controlled] = await Promise.all([
    getJSON('/api/runs/current/manifest'),
    getJSON('/api/runs/current/metrics'),
    getJSON('/api/runs/current/timeseries?mode=baseline'),
    getJSON('/api/runs/current/timeseries?mode=controlled'),
  ]);
  app.innerHTML = `<h1>${manifest.scenario_name}</h1><p class="muted">Baseline vs controlled AI datacenter grid-response run.</p><section class="grid">${card('Feasible', metrics.feasible ? 'yes' : 'no')}${card('Peak ramp reduction', fmt(metrics.peak_ramp_rate_reduction_mw_per_min), ' MW/min')}${card('Ramp violations', metrics.ramp_rate_violation_count, '')}${card('BESS discharged', fmt(metrics.bess_energy_discharged_mwh, 3), ' MWh')}${card('SoC range', `${fmt(metrics.bess_min_soc)}–${fmt(metrics.bess_max_soc)}`)}${card('Recovery', fmt(metrics.workload_recovery_time_seconds, 0), ' s')}</section>${chart('Net grid draw', baseline, controlled, 'net_grid_mw')}${chart('BESS dispatch', baseline, controlled, 'bess_power_mw')}${queueChart(controlled)}`;
}
main().catch(error => { app.innerHTML = `<h1>Unable to load run</h1><pre>${error.stack || error.message}</pre>`; });
