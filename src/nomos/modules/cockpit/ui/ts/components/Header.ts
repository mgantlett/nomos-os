import { CockpitStore, CockpitState } from '../store.js';

export function initHeader(): void {
  CockpitStore.subscribe(renderHeader);
}

function renderHeader(state: CockpitState): void {
  const { status, lastFleet, lastBacklog, activeProjectFilter } = state;
  const latestStatus = status;

  // In Community Edition, hide empty Swarm HUD to give full screen height to Kanban board
  const hudPanel = document.getElementById('active-swarm-hud');
  const splitterBar = document.getElementById('cockpit-splitter');
  if (hudPanel) {
    if (status && status.edition === 'community') {
      hudPanel.style.display = 'none';
      if (splitterBar) splitterBar.style.display = 'none';
    } else {
      hudPanel.style.display = 'flex';
      if (splitterBar) splitterBar.style.display = 'block';
    }
  }

  // Populate Project Selector if available
  const projectSelector = document.getElementById('project-selector') as HTMLSelectElement | null;
  if (projectSelector) {
    if (status && status.edition === 'community') {
      const repoName = status.repoRoot ? status.repoRoot.split('/').filter(Boolean).pop() || 'nomos' : 'nomos';
      projectSelector.innerHTML = `<option value="${repoName}">${repoName.toUpperCase()}</option>`;
      projectSelector.disabled = true;
      projectSelector.style.opacity = '0.85';
      projectSelector.style.cursor = 'default';
    } else {
      projectSelector.disabled = false;
      projectSelector.style.opacity = '1';
      projectSelector.style.cursor = 'pointer';
      const projects = new Set<string>();
      
      if (latestStatus && latestStatus.project) {
        projects.add(latestStatus.project);
      }
      
      if (lastFleet && lastFleet.neighbors) {
        lastFleet.neighbors.forEach((n: any) => {
          if (n.name && n.name.trim() !== '' && !n.name.startsWith('nomos_dod_test')) {
            projects.add(n.name);
          }
        });
      }

      if (lastBacklog) {
        lastBacklog.forEach((t: any) => {
          if (t.project && t.project.trim() !== '' && !t.project.startsWith('nomos_dod_test')) {
            projects.add(t.project);
          }
        });
      }
      
      if (activeProjectFilter !== 'ALL') {
        projects.add(activeProjectFilter);
      }
      
      if (projectSelector.options.length !== projects.size + 1) {
        projectSelector.innerHTML = '<option value="ALL">All Projects</option>';
        Array.from(projects).sort().forEach(p => {
          const opt = document.createElement('option');
          opt.value = p;
          opt.textContent = p;
          projectSelector.appendChild(opt);
        });
      }
      
      projectSelector.value = activeProjectFilter;

      projectSelector.onchange = (e) => {
        const val = (e.target as HTMLSelectElement).value;
        (window as any).activeProjectFilter = val;
        CockpitStore.setState({ activeProjectFilter: val });
        import('../logs.js').then(m => m.applyLogFilters());
        if ((window as any).refreshData) {
          (window as any).refreshData(true);
        }
      };
    }
  }

  const isCommunity = status && status.edition === 'community';
  const gpuMetrics = document.getElementById('header-gpu-metrics');
  const telemetryContainer = document.getElementById('header-telemetry-container');
  const tier2Container = document.getElementById('tier2-agent-container');
  let tier2Header: HTMLElement | null = null;
  document.querySelectorAll('.substrate-hud-panel div').forEach((div) => {
    if (div.textContent && div.textContent.includes('Tier 2: Swarm')) {
      tier2Header = div as HTMLElement;
    }
  });

  if (isCommunity) {
    if (gpuMetrics) gpuMetrics.style.display = 'none';
    if (telemetryContainer) telemetryContainer.style.display = 'none';

    if (tier2Header && tier2Container) {
      tier2Header.style.opacity = '0.35';
      tier2Container.style.opacity = '0.35';
      tier2Container.style.pointerEvents = 'none';
      if (!tier2Header.querySelector('.lock-badge')) {
        const badge = document.createElement('span');
        badge.className = 'lock-badge';
        badge.style.fontSize = '0.65rem';
        badge.style.marginLeft = '8px';
        badge.style.color = 'var(--neon-yellow)';
        badge.style.border = '1px solid var(--neon-yellow)';
        badge.style.padding = '1px 4px';
        badge.style.borderRadius = '3px';
        badge.style.textTransform = 'none';
        badge.textContent = '🔒 Sovereign Only';
        tier2Header.appendChild(badge);
      }
    }

    document.querySelectorAll('.tab-btn').forEach((el) => {
      const btn = el as HTMLElement;
      const tabId = btn.getAttribute('data-tab');
      if (tabId && tabId !== 'tab-backlog' && tabId !== 'tab-logs') {
        btn.classList.add('disabled-sovereign-tab');
        btn.style.opacity = '0.4';
        btn.style.cursor = 'not-allowed';
        if (!btn.querySelector('.tab-lock-icon')) {
          const lockIcon = document.createElement('span');
          lockIcon.className = 'tab-lock-icon';
          lockIcon.style.marginLeft = 'auto';
          lockIcon.style.fontSize = '0.75rem';
          lockIcon.textContent = '🔒';
          btn.appendChild(lockIcon);
        }
      }
    });
  } else {
    if (gpuMetrics) gpuMetrics.style.display = 'flex';
    if (telemetryContainer) telemetryContainer.style.display = 'flex';

    if (tier2Header && tier2Container) {
      tier2Header.style.opacity = '1';
      tier2Container.style.opacity = '1';
      tier2Container.style.pointerEvents = 'auto';
      const badge = tier2Header.querySelector('.lock-badge');
      if (badge) badge.remove();
    }

    document.querySelectorAll('.tab-btn').forEach((el) => {
      const btn = el as HTMLElement;
      btn.classList.remove('disabled-sovereign-tab');
      btn.style.opacity = '1';
      btn.style.cursor = 'pointer';
      const lockIcon = btn.querySelector('.tab-lock-icon');
      if (lockIcon) lockIcon.remove();
    });
  }
}
