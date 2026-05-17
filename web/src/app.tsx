// TypeScript source mirror for the dependency-light dashboard in web/static/app.js.
// V1 serves the checked-in JavaScript directly; future builds can compile this file.
type MetricSummary = {
  feasible: boolean;
  peak_ramp_rate_reduction_mw_per_min: number;
  ramp_rate_violation_count: number;
  bess_energy_discharged_mwh: number;
  bess_min_soc: number;
  bess_max_soc: number;
  workload_recovery_time_seconds: number;
};

type TimeseriesPoint = {
  timestamp: string;
  net_grid_mw: number;
  bess_power_mw: number;
  deferred_queue_mwh: number;
  event_active: boolean;
  controller_action?: string;
  deferred_mw?: number;
  recovered_mw?: number;
};

export type DashboardData = {
  metrics: MetricSummary;
  baseline: TimeseriesPoint[];
  controlled: TimeseriesPoint[];
};
