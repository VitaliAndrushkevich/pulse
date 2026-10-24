/** SvelteKit route resolution for components rendered outside the Kit runtime. */
export function resolve(route: string, params: Record<string, string | number> = {}): string {
  return route.replace(/\[([^\]]+)\]/g, (_, key: string) => encodeURIComponent(String(params[key])));
}
