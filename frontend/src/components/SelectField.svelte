<script lang="ts" generics="Value extends string | number | null">
  import { onMount, tick, type Snippet } from 'svelte';
  import type { HTMLSelectAttributes } from 'svelte/elements';

  type Props = Omit<HTMLSelectAttributes, 'value' | 'children' | 'class' | 'onblur'> & {
    value?: Value;
    children: Snippet;
    class?: string;
    onblur?: (event: FocusEvent) => void;
  };

  let { value = $bindable(), children, class: className = '', id, disabled = false,
    onblur, ...attributes }: Props = $props();
  const uid = $props.id();
  const fieldId = $derived(id ?? `${uid}-field`);
  const menuId = `${uid}-options`;
  let native = $state<HTMLSelectElement>();
  let trigger = $state<HTMLButtonElement>();
  let menu: HTMLDivElement | undefined = $state();
  let ready = $state(false);
  let open = $state(false);
  let options = $state.raw<{ label: string; disabled: boolean }[]>([]);
  let activeIndex = $state(-1);
  let position = $state({ top: 0, left: 0, width: 0, maxHeight: 288 });
  let anchor = { top: 0, left: 0 };
  let search = '';
  let searchTimer: ReturnType<typeof setTimeout> | undefined;

  let selectedIndex = $state(-1);

  $effect(() => {
    void value;
    // Read after Svelte applies its typed option binding to the native control.
    tick().then(() => { selectedIndex = native?.selectedIndex ?? -1; });
  });
  const selectedLabel = $derived(options[selectedIndex]?.label ?? '');

  onMount(() => {
    function syncOptions() {
      options = Array.from(native!.options, (option) => ({
        label: option.label, disabled: option.disabled
      }));
    }
    syncOptions();
    selectedIndex = native?.selectedIndex ?? -1;
    ready = true;
    const observer = new MutationObserver(syncOptions);
    observer.observe(native!, { childList: true, subtree: true, characterData: true, attributes: true });
    function handleScroll(event: Event) {
      // Nested panels may scroll independently; scrolling the option menu is safe.
      if (!open || !trigger || (event.target instanceof Node && menu?.contains(event.target))) return;
      const rect = trigger.getBoundingClientRect();
      if (Math.abs(rect.top - anchor.top) > 1 || Math.abs(rect.left - anchor.left) > 1) close();
    }
    document.addEventListener('scroll', handleScroll, true);
    return () => {
      observer.disconnect();
      document.removeEventListener('scroll', handleScroll, true);
      clearTimeout(searchTimer);
    };
  });

  function close() { open = false; }

  $effect(() => {
    if (disabled) close();
  });

  async function reveal(index = selectedIndex) {
    if (disabled || options.every((option) => option.disabled)) return;
    activeIndex = index >= 0 && !options[index]?.disabled ? index : options.findIndex((option) => !option.disabled);
    if (!trigger) return;
    const rect = trigger.getBoundingClientRect();
    anchor = { top: rect.top, left: rect.left };
    const desiredHeight = Math.min(288, options.length * 36 + 8);
    const below = window.innerHeight - rect.bottom - 12;
    const above = rect.top - 12;
    const placeAbove = below < desiredHeight && above > below;
    const maxHeight = Math.max(36, Math.min(desiredHeight, placeAbove ? above : below));
    const width = Math.min(Math.max(rect.width, 160), window.innerWidth - 16);
    const rtl = getComputedStyle(trigger).direction === 'rtl';
    position = {
      top: placeAbove ? Math.max(8, rect.top - maxHeight - 6) : rect.bottom + 6,
      left: Math.max(8, Math.min(rtl ? rect.right - width : rect.left, window.innerWidth - width - 8)),
      width, maxHeight
    };
    open = true;
    await tick();
    // The top layer keeps menus clear of scrollable tables, panels and dialogs.
    if (menu?.showPopover) menu.showPopover();
    else menu?.removeAttribute('popover');
    scrollActive();
  }

  function scrollActive() {
    const option = menu?.querySelector<HTMLElement>(`[data-index="${activeIndex}"]`);
    if (!menu || !option) return;
    // Scroll only the menu; scrollIntoView can also move the entire workspace.
    if (option.offsetTop < menu.scrollTop) menu.scrollTop = option.offsetTop;
    else if (option.offsetTop + option.offsetHeight > menu.scrollTop + menu.clientHeight) {
      menu.scrollTop = option.offsetTop + option.offsetHeight - menu.clientHeight;
    }
  }

  function choose(index: number) {
    if (options[index]?.disabled || !options[index]) return;
    if (!native) return;
    native.selectedIndex = index;
    selectedIndex = index;
    // Let Svelte's native binding retain typed option values and existing callbacks.
    native.dispatchEvent(new Event('change', { bubbles: true }));
    close();
    trigger?.focus();
  }

  async function move(direction: number) {
    if (!open) { await reveal(); return; }
    let next = activeIndex;
    for (let i = 0; i < options.length; i++) {
      next = (next + direction + options.length) % options.length;
      if (!options[next].disabled) { activeIndex = next; break; }
    }
    await tick();
    scrollActive();
  }

  async function handleKeydown(event: KeyboardEvent) {
    if (['ArrowDown', 'ArrowUp', 'Home', 'End', 'Enter', ' ', 'Escape'].includes(event.key)) {
      if (event.key === 'Escape' && !open) return;
      event.preventDefault();
      if (event.key === 'Escape') { event.stopPropagation(); close(); }
      else if (event.key === 'ArrowDown') await move(1);
      else if (event.key === 'ArrowUp') await move(-1);
      else if (event.key === 'Home' || event.key === 'End') {
        const enabled = options.map((option, index) => option.disabled ? -1 : index).filter((index) => index >= 0);
        await reveal(event.key === 'Home' ? enabled[0] : enabled.at(-1));
      } else if (open) choose(activeIndex);
      else await reveal();
    } else if (event.key === 'Tab') close();
    else if (event.key.length === 1 && !event.ctrlKey && !event.metaKey && !event.altKey) {
      event.preventDefault();
      clearTimeout(searchTimer);
      search += event.key.toLocaleLowerCase();
      searchTimer = setTimeout(() => { search = ''; }, 600);
      const start = search.length === 1 ? activeIndex + 1 : Math.max(0, activeIndex);
      for (let i = 0; i < options.length; i++) {
        const index = (start + i) % options.length;
        if (!options[index].disabled && options[index].label.toLocaleLowerCase().startsWith(search)) {
          await reveal(index);
          break;
        }
      }
    }
  }

  function handleOutside(event: Event) {
    const target = event.target;
    if (target instanceof Node && !trigger?.contains(target) && !menu?.contains(target)) close();
  }
</script>

<svelte:document onpointerdown={handleOutside} onfocusin={handleOutside} />
<svelte:window onresize={close} />

<div class={['select-field', className]}>
  <select
    {...attributes}
    id={ready ? `${fieldId}-native` : fieldId}
    bind:this={native}
    bind:value
    {disabled}
    hidden={ready}
    aria-hidden={ready ? 'true' : undefined}
    tabindex={ready ? -1 : undefined}
    class="native-fallback"
  >
    {@render children()}
  </select>
  {#if ready}
    <button
      bind:this={trigger}
      id={fieldId}
      type="button"
      role="combobox"
      aria-haspopup="listbox"
      aria-expanded={open}
      aria-controls={open ? menuId : undefined}
      aria-activedescendant={open && activeIndex >= 0 ? `${menuId}-${activeIndex}` : undefined}
      aria-label={attributes['aria-label']}
      aria-labelledby={attributes['aria-labelledby']}
      aria-describedby={attributes['aria-describedby']}
      aria-invalid={attributes['aria-invalid']}
      aria-required={attributes.required}
      {disabled}
      class="select-trigger"
      onclick={() => open ? close() : reveal()}
      onkeydown={handleKeydown}
      {onblur}
    >
      <span class="select-value">{selectedLabel}</span>
      <svg class:expanded={open} viewBox="0 0 20 20" fill="none" stroke="currentColor" stroke-width="1.5" aria-hidden="true"><path d="m6 8 4 4 4-4" /></svg>
    </button>
  {/if}
</div>

{#if open}
  <div
    bind:this={menu}
    id={menuId}
    role="listbox"
    aria-labelledby={fieldId}
    tabindex="-1"
    popover="manual"
    class="select-menu"
    style:top="{position.top}px"
    style:left="{position.left}px"
    style:width="{position.width}px"
    style:max-height="{position.maxHeight}px"
    dir={typeof document !== 'undefined' && document.documentElement.dir === 'rtl' ? 'rtl' : 'ltr'}
  >
    {#each options as option, index (index)}
      <button
        type="button"
        role="option"
        id="{menuId}-{index}"
        data-index={index}
        aria-selected={selectedIndex === index}
        disabled={option.disabled}
        tabindex="-1"
        class={['select-option', { active: activeIndex === index }]}
        onpointermove={() => { if (!option.disabled) activeIndex = index; }}
        onpointerdown={(event) => event.preventDefault()}
        onclick={() => choose(index)}
      >
        <span>{option.label}</span>
        {#if selectedIndex === index}
          <svg viewBox="0 0 20 20" fill="none" stroke="currentColor" stroke-width="1.5" aria-hidden="true"><path d="m4 10 4 4 8-8" /></svg>
        {/if}
      </button>
    {/each}
  </div>
{/if}

<style>
  .select-field { min-width: 0; }
  .select-trigger, .native-fallback {
    display: flex; align-items: center; justify-content: space-between; gap: 0.75rem;
    width: 100%; min-height: 2.5rem; padding: 0.5rem 0.75rem; border: 1px solid var(--color-border);
    border-radius: 0.25rem; background: var(--color-bg-surface); color: var(--color-text-primary);
    font-size: 0.875rem; line-height: 1.25rem; text-align: start;
  }
  .native-fallback[hidden] { display: none; }
  .select-trigger:hover:not(:disabled) { border-color: var(--color-brand-primary); }
  .select-trigger:focus-visible { outline: 2px solid var(--color-brand-primary); outline-offset: 2px; }
  .select-trigger:disabled { opacity: 0.5; cursor: not-allowed; }
  .select-value { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .select-trigger svg, .select-option svg { width: 1.1rem; height: 1.1rem; flex-shrink: 0; }
  .select-trigger svg { color: var(--color-text-secondary); transition: transform 120ms; }
  .select-trigger svg.expanded { transform: rotate(180deg); }
  .select-menu {
    position: fixed; inset: auto; margin: 0; padding: 0.25rem; z-index: 100;
    border: 1px solid var(--color-border); border-radius: 0.375rem;
    background: var(--color-bg-surface); color: var(--color-text-primary);
    box-shadow: 0 8px 24px color-mix(in srgb, var(--color-text-primary) 12%, transparent);
    overflow-y: auto; overscroll-behavior: contain;
  }
  .select-option { display: flex; align-items: center; justify-content: space-between; gap: 0.75rem; width: 100%; min-height: 2.25rem; padding: 0.5rem 0.625rem; border-radius: 0.2rem; font-size: 0.875rem; line-height: 1.25rem; text-align: start; }
  .select-option.active { background: var(--color-bg-surface-hover); }
  .select-option[aria-selected="true"] { color: var(--color-brand-primary); font-weight: 600; }
  .select-option:disabled { opacity: 0.45; cursor: not-allowed; }
</style>
