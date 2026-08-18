// swarm_drawer.ts - Swarm log stream and git diff visual drawer controller

let activeSwarmEventSource: EventSource | null = null;
let swarmDurationTimer: any = null;

function updateSwarmDuration(swarmState: any) {
  const durationSpan = document.getElementById('swarm-meta-duration');
  if (!durationSpan) return;

  if (swarmState.status === 'running') {
    const elapsed = Math.round((Date.now() - parseInt(swarmState.started_at) * 1000) / 1000);
    durationSpan.textContent = `${elapsed}s`;
  } else {
    durationSpan.textContent = swarmState.duration || '-';
  }
}

function updateSwarmStatusSpan(status: string) {
  const statusSpan = document.getElementById('swarm-meta-status');
  if (!statusSpan) return;

  statusSpan.textContent = status || 'unknown';
  statusSpan.className = `tag ${status === 'running' ? 'primary' : status === 'completed' ? 'success' : 'danger'}`;
  
  if (status === 'completed') {
    statusSpan.style.background = 'rgba(16, 185, 129, 0.15)';
    statusSpan.style.color = 'var(--neon-green)';
    statusSpan.style.border = '1px solid rgba(16, 185, 129, 0.25)';
  } else if (status === 'failed') {
    statusSpan.style.background = 'rgba(239, 68, 68, 0.15)';
    statusSpan.style.color = 'var(--neon-red)';
    statusSpan.style.border = '1px solid rgba(239, 68, 68, 0.25)';
  } else {
    statusSpan.style.background = 'rgba(59, 130, 246, 0.15)';
    statusSpan.style.color = 'var(--neon-blue)';
    statusSpan.style.border = '1px solid rgba(59, 130, 246, 0.25)';
  }
}

function updateSwarmMetaFields(swarmState: any) {
  const modelSpan = document.getElementById('swarm-meta-model');
  if (modelSpan) modelSpan.textContent = swarmState.model || '-';

  const pidSpan = document.getElementById('swarm-meta-pid');
  if (pidSpan) pidSpan.textContent = `${swarmState.slot || '-'} / ${swarmState.pid || '-'}`;

  updateSwarmStatusSpan(swarmState.status);
  updateSwarmDuration(swarmState);

  const promptDisplay = document.getElementById('swarm-prompt-display');
  if (promptDisplay) promptDisplay.textContent = swarmState.prompt || '-';
}

async function fetchAndRenderSwarmDiff(pidOrId: string) {
  const diffStatus = document.getElementById('swarm-diff-status');
  if (!diffStatus) return;

  try {
    const diffRes = await fetch(`/api/swarm/diff/${pidOrId}`);
    if (!diffRes.ok) {
      diffStatus.textContent = 'Worktree cleaned up after completion/verification.';
      return;
    }
    const diffData = await diffRes.json();
    if (diffData.status || diffData.diff) {
      let content = '';
      if (diffData.status) content += '### Modified Files:\n' + diffData.status + '\n';
      if (diffData.diff) content += '\n### Git Diff:\n' + diffData.diff;
      diffStatus.textContent = content;
    } else {
      diffStatus.textContent = 'No changes made yet.';
    }
  } catch (e) {
    console.error('Failed to update swarm diff:', e);
  }
}

export async function updateSwarmDiffAndMetadata(pidOrId: string) {
  try {
    const res = await fetch('/api/swarm/active-list');
    if (res.ok) {
      const list = await res.json();
      const swarmState = list.find((s: any) => s.pid === pidOrId || s.id === pidOrId);
      if (swarmState) {
        updateSwarmMetaFields(swarmState);
      }
    }
  } catch (e) {
    console.error('Failed to update swarm metadata:', e);
  }

  await fetchAndRenderSwarmDiff(pidOrId);
}

function onSwarmMessage(event: MessageEvent, consoleLog: HTMLElement | null) {
  if (consoleLog) {
    consoleLog.textContent += event.data + '\n';
    consoleLog.scrollTop = consoleLog.scrollHeight;
  }
}

function onSwarmError(consoleLog: HTMLElement | null) {
  if (consoleLog && !consoleLog.textContent.includes('Connection closed')) {
    consoleLog.textContent += '\n[System] Connection closed or inactive worker.\n';
  }
  if (activeSwarmEventSource) {
    activeSwarmEventSource.close();
    activeSwarmEventSource = null;
  }
}

export function openSwarmConsoleDrawer(pidOrId: string, label: string) {
  if ((window as any).switchBottomTab) {
    (window as any).switchBottomTab('tab-logs');
  }

  const select = document.getElementById('log-source-select') as HTMLSelectElement | null;
  if (select) {
    let found = false;
    for (let i = 0; i < select.options.length; i++) {
      if (select.options[i].value === pidOrId) {
        found = true;
        break;
      }
    }
    if (!found) {
      const opt = document.createElement('option');
      opt.value = pidOrId;
      opt.textContent = `Worker Task Logs: ${label}`;
      select.appendChild(opt);
    }
    select.value = pidOrId;
    
    if ((window as any).changeLogSource) {
      (window as any).changeLogSource(pidOrId);
    }
  }
}

export function closeSwarmConsoleDrawer() {}
