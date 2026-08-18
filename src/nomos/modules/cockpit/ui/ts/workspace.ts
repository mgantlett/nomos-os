import { resetKanbanCache } from './board.js';
import { showToast } from './toast.js';
import { refreshData } from './app.js';
import { WorkspaceDiscoveryResponse } from './types.js';

export function updateWorkspaceContextDropdownSelection(currentRepoRoot: string): void {
  const switcher = document.getElementById('workspace-context-switcher') as HTMLSelectElement | null;
  if (!switcher) return;

  let optionFound = false;
  for (let i = 0; i < switcher.options.length; i++) {
    const opt = switcher.options[i];
    if (opt.value === currentRepoRoot) {
      optionFound = true;
      if (!opt.selected) {
        opt.selected = true;
      }
      break;
    }
  }

  if (!optionFound && switcher.options.length > 0 && switcher.options[0].value !== "") {
    const option = document.createElement('option');
    option.value = currentRepoRoot;
    const basename = currentRepoRoot.split('/').pop() || currentRepoRoot;
    option.textContent = basename;
    option.title = currentRepoRoot;
    option.selected = true;
    switcher.appendChild(option);
  }
}

export async function initWorkspaceContextSwitcher(): Promise<void> {
  const switcher = document.getElementById('workspace-context-switcher') as HTMLSelectElement | null;
  if (!switcher) return;

  try {
    const res = await fetch('/api/context/list');
    if (!res.ok) {
      throw new Error(`HTTP error ${res.status}`);
    }
    const data = await res.json() as WorkspaceDiscoveryResponse;
    
    switcher.replaceChildren();
    
    data.discovered.forEach(path => {
      const option = document.createElement('option');
      option.value = path;
      if (path === "ALL") {
        option.textContent = "🌐 All Projects (Multi-Repo)";
        option.title = "Aggregated workspace telemetry across all project repos";
      } else {
        const basename = path.split('/').pop() || path;
        option.textContent = basename;
        option.title = path;
      }
      if (path === data.current) {
        option.selected = true;
      }
      switcher.appendChild(option);
    });

    if (!(switcher as any)._hasChangeListener) {
      (switcher as any)._hasChangeListener = true;
      switcher.addEventListener('change', async () => {
        const selectedPath = switcher.value;
        if (!selectedPath) return;

        try {
          const response = await fetch('/api/context', {
            method: 'POST',
            headers: {
              'Content-Type': 'application/json'
            },
            body: JSON.stringify({ repoRoot: selectedPath })
          });
          if (!response.ok) {
            const errData = await response.json();
            throw new Error(errData.error || `HTTP error ${response.status}`);
          }
          const resData = await response.json();
          if (resData.success) {
            let contextName = selectedPath.split('/').pop() || selectedPath;
            if (selectedPath === 'ALL') {
              contextName = 'All Projects';
            }
            showToast(`Switched workspace context to ${contextName}`, 'success');
            resetKanbanCache();
            await refreshData(true);
            
            // Re-subscribe to logs to ensure backend tails from the new context
            await resubscribeLogs();
          } else {
            showToast(resData.error || 'Failed to switch context', 'error');
          }
        } catch (err: any) {
          showToast(`Failed to switch context: ${err.message}`, 'error');
          await initWorkspaceContextSwitcher();
        }
      });
    }
  } catch (err: any) {
    console.error('Failed to initialize workspace context switcher:', err);
    switcher.replaceChildren();
    const opt = document.createElement('option');
    opt.value = '';
    opt.disabled = true;
    opt.selected = true;
    opt.textContent = 'Error loading workspaces';
    switcher.appendChild(opt);
  }
}

export async function resubscribeLogs() {
  const currentSub = (window as any).currentSubscribedLogSource || 'all';
  const { sendWSFrame } = await import('./ws.js');
  const { clearChatMessages } = await import('./chat.js');
  const { clearTerminalLogs } = await import('./logs.js');
  clearChatMessages();
  clearTerminalLogs();
  sendWSFrame('unsubscribe_logs', null, { log_source: currentSub });
  setTimeout(() => {
    sendWSFrame('subscribe_logs', null, { log_source: currentSub });
  }, 500);
}
