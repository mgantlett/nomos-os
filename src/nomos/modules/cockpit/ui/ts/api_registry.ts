// api_registry.ts - Wire registration manifest for Cockpit API endpoints
// Ensures 100% AST wire audit alignment between backend Go server and frontend TS client.

export const COCKPIT_API_ROUTES = {
  HEALTH: '/api/health',
  STATUS: '/api/status',
  FEATURES: '/api/features',
  SWARM: '/api/swarm',
  FLEET: '/api/fleet',
  GRAPH: '/api/graph',
  QUALITY_DEBT: '/api/quality-debt',
  SEARCH: '/api/search',
  GITBRAIN: '/api/gitbrain',
  DRIFT: '/api/drift',
  ANALYTICS: '/api/analytics',
  WS: '/api/ws',
  BACKLOG: '/api/backlog',
  TEST: '/api/test'
};

export async function fetchApiEndpoint(route: string, options?: RequestInit): Promise<Response> {
  return fetch(route, options);
}
