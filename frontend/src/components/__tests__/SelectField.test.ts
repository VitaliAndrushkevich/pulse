import { afterEach, describe, expect, it, vi } from 'vitest';
import { cleanup, fireEvent, render, waitFor } from '@testing-library/svelte';
import SelectFieldHarness from './fixtures/SelectFieldHarness.svelte';

afterEach(cleanup);

describe('SelectField interaction', () => {
  it('has an associated field label and commits a pointer selection through the form binding', async () => {
    const onChange = vi.fn();
    const view = render(SelectFieldHarness, { props: { onChange } });
    const trigger = view.getByRole('combobox', { name: 'Protocol' });
    expect(trigger.textContent).toContain('HTTP');
    await fireEvent.click(trigger);
    expect(trigger.getAttribute('aria-expanded')).toBe('true');
    await fireEvent.click(view.getByRole('option', { name: 'TCP' }));
    expect(view.getByTestId('value').textContent).toBe('string:tcp');
    expect(onChange).toHaveBeenCalledWith('tcp');
    expect(view.queryByRole('listbox')).toBeNull();
    expect(document.activeElement).toBe(trigger);
  });

  it('skips disabled options with arrows and retains numeric option types', async () => {
    const onChange = vi.fn();
    const view = render(SelectFieldHarness, { props: { numeric: true, onChange } });
    const trigger = view.getByRole('combobox');
    await fireEvent.keyDown(trigger, { key: 'ArrowDown' });
    await fireEvent.keyDown(trigger, { key: 'ArrowDown' });
    await fireEvent.keyDown(trigger, { key: 'Enter' });
    expect(view.getByTestId('value').textContent).toBe('number:3');
    expect(onChange).toHaveBeenCalledWith(3);
  });

  it('preserves nullable option values used by notification reminder bindings', async () => {
    const onChange = vi.fn();
    const view = render(SelectFieldHarness, { props: { initialNull: true, onChange } });
    const trigger = view.getByRole('combobox');
    expect(trigger.textContent).toContain('HTTP');
    await fireEvent.click(trigger);
    await fireEvent.click(view.getByRole('option', { name: 'TCP' }));
    await fireEvent.click(trigger);
    await fireEvent.click(view.getByRole('option', { name: 'HTTP' }));
    expect(view.getByTestId('value').textContent).toBe('object:');
    expect(onChange).toHaveBeenLastCalledWith(null);
  });

  it('supports typeahead and Escape without changing the selected value', async () => {
    const view = render(SelectFieldHarness);
    const trigger = view.getByRole('combobox');
    await fireEvent.keyDown(trigger, { key: 't' });
    const option = view.getByRole('option', { name: 'TCP' });
    expect(trigger.getAttribute('aria-activedescendant')).toBe(option.id);
    await fireEvent.keyDown(trigger, { key: 'Escape' });
    expect(view.queryByRole('listbox')).toBeNull();
    expect(view.getByTestId('value').textContent).toBe('string:http');
  });

  it('closes on outside pointer events and Tab without trapping focus', async () => {
    const view = render(SelectFieldHarness);
    const trigger = view.getByRole('combobox');
    await fireEvent.click(trigger);
    await fireEvent.pointerDown(view.getByRole('button', { name: 'Next field' }));
    expect(view.queryByRole('listbox')).toBeNull();
    await fireEvent.click(trigger);
    await fireEvent.keyDown(trigger, { key: 'Tab' });
    expect(view.queryByRole('listbox')).toBeNull();
  });

  it('updates translated option labels while mounted', async () => {
    const view = render(SelectFieldHarness);
    await view.rerender({ lastLabel: 'Transmission Control Protocol' });
    await fireEvent.click(view.getByRole('combobox'));
    await waitFor(() => expect(view.getByRole('option', { name: 'Transmission Control Protocol' })).toBeTruthy());
  });

  it('exposes disabled state without opening a menu', async () => {
    const view = render(SelectFieldHarness, { props: { disabled: true } });
    const trigger = view.getByRole('combobox') as HTMLButtonElement;
    expect(trigger.disabled).toBe(true);
    await fireEvent.keyDown(trigger, { key: 'ArrowDown' });
    expect(view.queryByRole('listbox')).toBeNull();
  });
});
