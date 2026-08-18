// ws.ts - Unified WebSocket control plane socket client for Nomos

export let wsConnected = false;
export let mainWS: WebSocket | null = null;

const listeners: ((frame: any) => void)[] = [];
let reconnectTimer: any = null;
let currentSubscribedLogSource = '';

export function addWSListener(cb: (frame: any) => void): void {
  listeners.push(cb);
}

export function removeWSListener(cb: (frame: any) => void): void {
  const idx = listeners.indexOf(cb);
  if (idx !== -1) {
    listeners.splice(idx, 1);
  }
}

export function sendWSFrame(type: string, payload?: any, extra?: any): void {
  if (mainWS && mainWS.readyState === WebSocket.OPEN) {
    mainWS.send(JSON.stringify({
      type,
      payload,
      ...extra
    }));
  }
}

export function subscribeLogs(sourceId: string): void {
  if (currentSubscribedLogSource === sourceId) return;

  if (currentSubscribedLogSource) {
    sendWSFrame('unsubscribe_logs', null, { log_source: currentSubscribedLogSource });
  }

  currentSubscribedLogSource = sourceId;
  sendWSFrame('subscribe_logs', null, { log_source: sourceId });
}

export function initControlPlaneWS(onConnectChange?: (connected: boolean) => void): void {
  if (mainWS) return;
  connectSocket(onConnectChange);
}

function connectSocket(onConnectChange?: (connected: boolean) => void): void {
  if (mainWS) return;

  const loc = window.location;
  const protocol = loc.protocol === 'https:' ? 'wss:' : 'ws:';
  const wsUrl = `${protocol}//${loc.host}/api/ws`;

  console.log('[WebSocket] Connecting to Nomos control plane at:', wsUrl);
  mainWS = new WebSocket(wsUrl);

  mainWS.onopen = () => {
    console.log('[WebSocket] Connected to Nomos control plane.');
    wsConnected = true;
    if (onConnectChange) onConnectChange(true);

    const sub = currentSubscribedLogSource || 'all';
    currentSubscribedLogSource = sub;
    sendWSFrame('subscribe_logs', null, { log_source: sub });
  };

  mainWS.onclose = () => {
    wsConnected = false;
    mainWS = null;
    if (onConnectChange) onConnectChange(false);

    if (reconnectTimer) clearTimeout(reconnectTimer);
    reconnectTimer = setTimeout(() => initControlPlaneWS(onConnectChange), 5000);
  };

  mainWS.onerror = (err) => {
    console.warn('[WebSocket] Control plane WebSocket unavailable on active server edition.');
  };

  mainWS.onmessage = (event) => {
    try {
      const frame = JSON.parse(event.data);
      listeners.forEach(cb => cb(frame));
    } catch (e) {
      console.error('[WebSocket] Failed to parse message frame:', e);
    }
  };
}
