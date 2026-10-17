<script lang="ts">
  import { untrack } from 'svelte';
  import SelectField from '../../SelectField.svelte';
  let { disabled = false, numeric = false, initialNull = false, lastLabel = 'TCP', onChange = (_value: string | number | null) => {} } = $props();
  let value = $state<string | number | null>(untrack(() => initialNull ? null : numeric ? 1 : 'http'));
</script>

<label for="protocol">Protocol</label>
<SelectField id="protocol" bind:value {disabled} onchange={() => onChange(value)}>
  <option value={initialNull ? null : numeric ? 1 : 'http'}>HTTP</option>
  <option value={numeric ? 2 : 'dns'} disabled>DNS</option>
  <option value={numeric ? 3 : 'tcp'}>{lastLabel}</option>
</SelectField>
<button type="button">Next field</button>
<output data-testid="value">{typeof value}:{value}</output>
