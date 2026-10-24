import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/svelte';
import { getDashboardSummary } from '$lib/api';
import { dashboardStore } from '$lib/stores/dashboard.svelte';
import { setLocale } from '$lib/i18n';
import Dashboard from './+page.svelte';
vi.mock('$lib/api',()=>({getDashboardSummary:vi.fn()}));

afterEach(cleanup);
beforeEach(async () => {
 await setLocale('en');
 localStorage.clear();
 dashboardStore.reset();
 vi.mocked(getDashboardSummary).mockReset();
 vi.mocked(getDashboardSummary).mockImplementation(async (period='24h')=>({
  health_score:{uptime_percent:98,active_monitor_count:1,partial_data:false,period},
  status_distribution:{up:1,down:0,unknown:0,total:1},active_incidents:[],
  incidents:[{id:'i',monitor_id:'m',monitor_name:'Recovered API',started_at:'2026-10-01T10:00:00Z',resolved_at:'2026-10-01T11:00:00Z',cause:null}],
  incident_count:1,period_from:'2026-10-01T00:00:00Z',period_to:'2026-10-02T00:00:00Z',
  top_latency_monitors:[],ssl_expiry:[],heatmap:[],recent_events:[],generated_at:new Date().toISOString()
 }));
});

describe('Dashboard period control',()=>{
 it('places the period selector in the page header and applies it to incident history',async()=>{
  render(Dashboard);
  const control=await screen.findByRole('combobox',{name:'Period'});
  expect(control.closest('.page-header')).not.toBeNull();
  expect(screen.getByTestId('fleet-health').contains(control)).toBe(false);
  await screen.findByText('Recovered API');
  await fireEvent.click(control);
  await fireEvent.keyDown(control,{key:'ArrowDown'});
  await fireEvent.keyDown(control,{key:'Enter'});
  await waitFor(()=>expect(getDashboardSummary).toHaveBeenLastCalledWith('7d'));
  expect(localStorage.getItem('pulse-fleet-period')).toBe('7d');
  expect(screen.getByText('Incidents in period')).toBeTruthy();
 });
});

it('shows response time and certificates together without an events tab',async()=>{
 const base=await getDashboardSummary();
 vi.mocked(getDashboardSummary).mockResolvedValue({...base,top_latency_monitors:[{monitor_id:'slow',monitor_name:'Slow API',avg_latency_ms:120}],ssl_checked_monitor_count:2,ssl_expiry:[],widget_errors:{'ssl-expiry':'DASHBOARD_QUERY_FAILED'},partial_data:true});
 render(Dashboard);
 await screen.findByRole('link',{name:/Slow API/});
 expect(screen.getByRole('heading',{name:'Response time'})).toBeTruthy();
 expect(screen.getByRole('heading',{name:'Certificates'})).toBeTruthy();
 expect(screen.queryByRole('tab')).toBeNull();
 expect(screen.queryByText('Recent Events')).toBeNull();
 expect(screen.getByRole('alert')).toBeTruthy();
 expect(screen.getByRole('link',{name:/Slow API/})).toBeTruthy();
});
