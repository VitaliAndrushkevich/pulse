import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { cleanup, fireEvent, render, screen } from '@testing-library/svelte';
import { setLocale, t } from '$lib/i18n';
import ResponseTimes from './ResponseSparklines.svelte';
import SSLWarnings from './SSLWarnings.svelte';
import type { SSLExpiryEntry } from '$lib/types';

afterEach(cleanup);
beforeEach(async () => { await setLocale('en'); });
const ready = { loading: false, error: null, onRetry: null };
const certificate = (id: string, days: number): SSLExpiryEntry => ({monitor_id:id, monitor_name:`Certificate ${id}`,days_remaining:days,expires_at:'2026-11-01T00:00:00Z'});

describe('Response time panel', () => {
 it('ranks recorded averages, links monitors, and labels the selected period', () => {
  const {container}=render(ResponseTimes,{...ready,period:'7d',monitors:[
   {monitor_id:'fast',monitor_name:'Fast API',avg_latency_ms:25},
   {monitor_id:'slow',monitor_name:'Slow API',avg_latency_ms:100}
  ]});
  expect(screen.getByText('Five slowest monitors · Last 7 days')).toBeTruthy();
  const links=screen.getAllByRole('link');
  expect(links.map(link=>link.getAttribute('href'))).toEqual(['/monitors/slow','/monitors/fast']);
  expect(links[0].textContent).toContain('100 ms');
  expect(links[1].textContent).toContain('25 ms');
  expect(container.querySelector('svg')).toBeNull();
  expect([...container.querySelectorAll<HTMLElement>('.latency-track span')].map(el=>el.style.inlineSize)).toEqual(['100%','25%']);
 });
 it('handles zero measurements without inventing a bar or displaying NaN', () => {
  const {container}=render(ResponseTimes,{...ready,monitors:[{monitor_id:'zero',monitor_name:'Zero API',avg_latency_ms:0}]});
  expect(screen.getByRole('link').textContent).toContain('0 ms');
  expect(container.querySelector<HTMLElement>('.latency-track span')?.style.inlineSize).toBe('0%');
 });
 it('distinguishes missing measurements from a request failure',async()=>{
  const retry=vi.fn();
  const {rerender}=render(ResponseTimes,{...ready,monitors:[],onRetry:retry});
  expect(screen.getByTestId('response-times-empty')).toBeTruthy();
  await rerender({monitors:[],loading:false,error:'Request failed',onRetry:retry});
  expect(screen.queryByTestId('response-times-empty')).toBeNull();
  expect(screen.getByRole('alert').textContent).toContain('Request failed');
  await fireEvent.click(screen.getByRole('button',{name:'Retry'}));
  expect(retry).toHaveBeenCalledOnce();
 });
});

describe('Certificate panel',()=>{
 for(const locale of ['en','ar','be']){
  it(`distinguishes no observations from no expiring certificates (${locale})`,async()=>{
   await setLocale(locale);
   const {rerender}=render(SSLWarnings,{...ready,entries:[],checkedMonitorCount:0});
   expect(screen.getByText(t('dashboard.certificates.noData'))).toBeTruthy();
   expect(screen.queryByTestId('ssl-empty')).toBeNull();
   await rerender({...ready,entries:[],checkedMonitorCount:5});
   expect(screen.queryByTestId('ssl-no-data')).toBeNull();
   expect(screen.getByText(t('dashboard.certificates.allClear'))).toBeTruthy();
  });
 }
 it('shows urgent certificates first, their dates, and monitor links',()=>{
  render(SSLWarnings,{...ready,checkedMonitorCount:3,entries:[certificate('later',20),certificate('soon',3),certificate('expired',-2)]});
  const links=screen.getAllByRole('link');
  expect(links.map(link=>link.getAttribute('href'))).toEqual(['/monitors/expired','/monitors/soon','/monitors/later']);
  expect(links[0].textContent).toContain('Expired');
  expect(links[1].textContent).toContain('3');
  expect(links.every(link=>link.textContent?.includes('2026'))).toBe(true);
 });
 it('renders a bounded number of rows for a fleet of 500 certificates',()=>{
  render(SSLWarnings,{...ready,checkedMonitorCount:500,entries:Array.from({length:500},(_,index)=>certificate(String(index),2))});
  const links=screen.getAllByRole('link');
  expect(links.length).toBeLessThanOrEqual(60);
  expect(links.length).toBeGreaterThan(0);
 });
 it('does not replace a failed certificate query with an all-clear',()=>{
  render(SSLWarnings,{...ready,error:'Certificate query failed',entries:[],checkedMonitorCount:0});
  expect(screen.getByRole('alert').textContent).toContain('Certificate query failed');
  expect(screen.queryByTestId('ssl-no-data')).toBeNull();
  expect(screen.queryByTestId('ssl-empty')).toBeNull();
 });
});
