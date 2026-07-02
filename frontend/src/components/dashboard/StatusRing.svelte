<script lang="ts" module>
  import type { StatusDistribution } from '$lib/types';

  export interface ArcSegment {
    state: 'up' | 'down' | 'unknown';
    count: number;
    angle: number;
  }

  /**
   * Compute arc angles for each state proportional to its count.
   * Returns an array of segments with their computed angle in degrees.
   * All angles sum to exactly 360° when total > 0.
   *
   * Exported for property-based testing (task 5.5).
   */
  export function computeArcAngles(distribution: StatusDistribution): ArcSegment[] {
    const { up, down, unknown, total } = distribution;

    if (total === 0) {
      return [
        { state: 'up', count: 0, angle: 0 },
        { state: 'down', count: 0, angle: 0 },
        { state: 'unknown', count: 0, angle: 0 },
      ];
    }

    const states: Array<{ state: 'up' | 'down' | 'unknown'; count: number }> = [
      { state: 'up', count: up },
      { state: 'down', count: down },
      { state: 'unknown', count: unknown },
    ];

    return states.map((s) => ({
      ...s,
      angle: (s.count / total) * 360,
    }));
  }

  /**
   * Build ARIA label describing each non-zero state and its count.
   * Exported for property-based testing (task 5.5).
   */
  export function buildAriaDescription(distribution: StatusDistribution): string {
    const parts: string[] = [];
    if (distribution.up > 0) parts.push(`${distribution.up} up`);
    if (distribution.down > 0) parts.push(`${distribution.down} down`);
    if (distribution.unknown > 0) parts.push(`${distribution.unknown} unknown`);
    return parts.join(', ');
  }
</script>

<script lang="ts">
  import { t } from '$lib/i18n';
  import WidgetShell from './WidgetShell.svelte';

  interface Props {
    data: StatusDistribution | null;
    loading: boolean;
    error: string | null;
    onRetry: (() => void) | null;
  }

  let { data, loading, error, onRetry }: Props = $props();

  // SVG geometry constants
  const SIZE = 120;
  const CENTER = SIZE / 2;
  const RADIUS = 44;
  const STROKE_WIDTH = 12;
  const CIRCUMFERENCE = 2 * Math.PI * RADIUS;

  // Map state to CSS custom property color
  const STATE_COLORS: Record<'up' | 'down' | 'unknown', string> = {
    up: 'var(--color-success)',
    down: 'var(--color-error)',
    unknown: 'var(--color-secondary)',
  };

  let segments = $derived.by(() => {
    if (!data) return [];
    return computeArcAngles(data);
  });

  let ariaLabel = $derived.by(() => {
    if (!data || data.total === 0) {
      return t('dashboard.statusRing.title');
    }
    return buildAriaDescription(data);
  });

  let totalDisplay = $derived(data?.total ?? 0);

  /**
   * Compute stroke-dasharray and stroke-dashoffset for each visible segment.
   * Uses the standard SVG circle donut technique:
   *  - dasharray = [segment_length, circumference - segment_length]
   *  - dashoffset = -(cumulative offset from the top of the circle)
   */
  let circleSegments = $derived.by(() => {
    if (!data || data.total === 0) return [];

    const result: Array<{
      state: 'up' | 'down' | 'unknown';
      color: string;
      dashArray: string;
      dashOffset: number;
    }> = [];

    let cumulativeAngle = 0;

    for (const seg of segments) {
      if (seg.count === 0) continue;

      const segmentLength = (seg.angle / 360) * CIRCUMFERENCE;
      const offset = (cumulativeAngle / 360) * CIRCUMFERENCE;

      result.push({
        state: seg.state,
        color: STATE_COLORS[seg.state],
        dashArray: `${segmentLength} ${CIRCUMFERENCE - segmentLength}`,
        dashOffset: -offset,
      });

      cumulativeAngle += seg.angle;
    }

    return result;
  });
</script>

<WidgetShell {loading} {error} {onRetry}>
  <div class="flex flex-col items-center gap-2 p-4" data-testid="status-ring">
    <h3 class="text-sm font-medium" style="color: var(--color-text-secondary)">
      {t('dashboard.statusRing.title')}
    </h3>

    <svg
      width={SIZE}
      height={SIZE}
      viewBox="0 0 {SIZE} {SIZE}"
      role="img"
      aria-label={ariaLabel}
      data-testid="status-ring-svg"
    >
      {#if !data || data.total === 0}
        <!-- Empty state: full ring with secondary color -->
        <circle
          cx={CENTER}
          cy={CENTER}
          r={RADIUS}
          fill="none"
          stroke="var(--color-secondary)"
          stroke-width={STROKE_WIDTH}
        />
      {:else}
        <!-- Render each non-zero segment -->
        {#each circleSegments as seg (seg.state)}
          <circle
            cx={CENTER}
            cy={CENTER}
            r={RADIUS}
            fill="none"
            stroke={seg.color}
            stroke-width={STROKE_WIDTH}
            stroke-dasharray={seg.dashArray}
            stroke-dashoffset={seg.dashOffset}
            stroke-linecap="butt"
            transform="rotate(-90 {CENTER} {CENTER})"
            data-testid="status-ring-segment-{seg.state}"
          />
        {/each}
      {/if}

      <!-- Center total count -->
      <text
        x={CENTER}
        y={CENTER}
        text-anchor="middle"
        dominant-baseline="central"
        class="text-lg font-semibold"
        style="fill: var(--color-text-primary)"
        data-testid="status-ring-total"
      >
        {totalDisplay}
      </text>
    </svg>
  </div>
</WidgetShell>
