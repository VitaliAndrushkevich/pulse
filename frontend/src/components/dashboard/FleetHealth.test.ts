import { afterEach, describe, expect, it } from 'vitest';
import { cleanup, render, screen } from '@testing-library/svelte';
import FleetHealth from './FleetHealth.svelte';
import { setLocale } from '$lib/i18n';

afterEach(cleanup);

describe('Fleet Health presentation', () => {
 for (const locale of ['en', 'ar', 'be']) {
  it(`does not claim all monitors are up when some are unknown (${locale})`, async () => {
   await setLocale(locale);
   render(FleetHealth, { healthData: {uptime_percent:98.3,active_monitor_count:3,sampled_monitor_count:2,partial_data:true},distributionData:{up:2,down:0,unknown:1,total:3},loading:false,error:null,onRetry:null });
   expect(screen.queryByTestId('fleet-health-all-up')).toBeNull();
   expect(screen.getByTestId('fleet-health-unknown')).toBeTruthy();
   expect(screen.getByTestId('health-score-value').textContent?.trim()).toBe('98.30%');
  });
 }
 it('shows no historical value when active monitors have no retained checks', () => {
  render(FleetHealth, {healthData:{uptime_percent:0,active_monitor_count:3,sampled_monitor_count:0,partial_data:true},distributionData:{up:0,down:0,unknown:3,total:3},loading:false,error:null,onRetry:null});
  expect(screen.getByTestId('health-score-value').textContent?.trim()).toBe('—');
 });
});
