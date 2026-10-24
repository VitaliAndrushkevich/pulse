import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { cleanup, render, screen } from '@testing-library/svelte';
import { setLocale, t } from '$lib/i18n';
import IncidentsPanel, { formatDuration } from './IncidentsPanel.svelte';

afterEach(() => { cleanup(); vi.useRealTimers(); });
beforeEach(async () => { await setLocale('en'); });

describe('Incident history', () => {
 it('stops duration at recovery rather than continuing until now', () => {
  expect(formatDuration('2026-10-01T10:00:00Z','2026-10-01T11:00:00Z',Date.parse('2026-10-24T12:00:00Z'))).toBe('1h');
 });
 for (const locale of ['en','ar','be']) {
  it(`shows original start/end and ongoing status (${locale})`, async () => {
   await setLocale(locale);
   render(IncidentsPanel, {historical:true,totalCount:12,loading:false,error:null,onRetry:null,
    periodFrom:'2026-10-01T10:30:00Z',periodTo:'2026-10-02T10:30:00Z',
    incidents:[
     {id:'resolved',monitor_id:'1',monitor_name:'Recovered API',started_at:'2026-10-01T10:00:00Z',resolved_at:'2026-10-01T11:00:00Z',cause:'Timeout'},
     {id:'open',monitor_id:'1',monitor_name:'Ongoing API',started_at:'2026-10-01T12:00:00Z',resolved_at:null,cause:null}
    ]});
   const times = [...document.querySelectorAll('time')].map(el=>el.getAttribute('datetime'));
   expect(times).toEqual(['2026-10-01T12:00:00Z','2026-10-01T10:00:00Z','2026-10-01T11:00:00Z']);
   expect(screen.getByText(t('dashboard.incidents.resolved'))).toBeTruthy();
   expect(screen.getAllByText(t('dashboard.incidents.ongoing'))).toHaveLength(2);
   expect(screen.getByTestId('incidents-overflow').textContent).toContain('2');
   expect(screen.getAllByTestId('incident-duration')[1].textContent?.trim()).toBe(t('dashboard.incidents.durationHours',{hours:1,minutes:0}));
  });
 }
 it('does not describe a period without incidents as fully operational', () => {
  render(IncidentsPanel,{historical:true,incidents:[],loading:false,error:null,onRetry:null});
  expect(screen.getByTestId('incidents-empty').textContent).toContain('No incidents in the selected period');
 });
});

it('prioritizes ongoing incidents even when a recovered incident started later',()=>{
 render(IncidentsPanel,{...{loading:false,error:null,onRetry:null},historical:true,incidents:[
 {id:'recovered',monitor_id:'r',monitor_name:'Recovered API',started_at:'2026-10-02T12:00:00Z',resolved_at:'2026-10-02T13:00:00Z',cause:null},
 {id:'ongoing',monitor_id:'o',monitor_name:'Ongoing API',started_at:'2026-10-01T10:00:00Z',resolved_at:null,cause:null}
 ]});
 expect(screen.getAllByRole('link').map(link=>link.getAttribute('href'))).toEqual(['/monitors/o','/monitors/r']);
});
