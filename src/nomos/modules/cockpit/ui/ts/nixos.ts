import { showToast } from './toast.js';

export interface NixosGeneration {
  id: number;
  active: boolean;
  path: string;
  createdAt: string;
}

export interface NixosDrift {
  driftPercent: number;
  uncommitted: string[];
  syncStatus: string;
}

export interface NixosConfig {
  plasmaEnabled: boolean;
  gpuCapEnabled: boolean;
  daemonEnabled: boolean;
}

export async function initNixosUI(): Promise<void> {
  const panel = document.getElementById('tab-host-os');
  if (!panel) return;

  // Render HTML structure if empty
  if (panel.innerHTML.trim() === '') {
    panel.innerHTML = `
      <div class="nixos-dashboard">
        <div class="nixos-header-row">
          <h2><i class="codicon codicon-server-environment"></i> NixOS Host Management Panel</h2>
          <button class="nixos-refresh-btn" id="nixos-refresh-btn">
            <i class="codicon codicon-refresh"></i> Refresh State
          </button>
        </div>

        <div class="nixos-grid">
          <!-- Left Column: Generations & Rollover -->
          <div class="nixos-card">
            <h3><i class="codicon codicon-history"></i> System Generations</h3>
            <p class="nixos-card-desc">Active NixOS boot profiles. Select any profile generation to perform a safe rollback switch.</p>
            <div class="nixos-generations-list" id="nixos-gens-list">
              <div class="nixos-loading">Loading generations...</div>
            </div>
          </div>

          <!-- Right Column: Drift & Toggles -->
          <div class="nixos-side-column">
            <!-- Drift Meter Card -->
            <div class="nixos-card">
              <h3><i class="codicon codicon-git-compare"></i> Configuration Drift Meter</h3>
              <div class="nixos-drift-container">
                <div class="nixos-drift-gauge">
                  <div class="nixos-drift-gauge-fill" id="nixos-drift-gauge-fill" style="width: 0%;"></div>
                </div>
                <div class="nixos-drift-meta">
                  <span class="nixos-drift-val" id="nixos-drift-val">0% Drift</span>
                  <span class="nixos-drift-status" id="nixos-drift-status">Synced</span>
                </div>
              </div>
              <div class="nixos-drift-details" id="nixos-drift-details">
                No local config modifications detected.
              </div>
            </div>

            <!-- Telemetry Sync Toggles Card -->
            <div class="nixos-card">
              <h3><i class="codicon codicon-settings-gear"></i> Telemetry & System Toggles</h3>
              <p class="nixos-card-desc">Apply declarative resource caps or parameter overrides dynamically.</p>
              <div class="nixos-toggles-list">
                <div class="nixos-toggle-item">
                  <div class="nixos-toggle-label">
                    <strong>Plasma Desktop Optimization</strong>
                    <span>Tune display manager parameters</span>
                  </div>
                  <label class="nixos-switch">
                    <input type="checkbox" id="toggle-plasma">
                    <span class="nixos-slider"></span>
                  </label>
                </div>
                <div class="nixos-toggle-item">
                  <div class="nixos-toggle-label">
                    <strong>GPU Power Cap</strong>
                    <span>Apply dynamic GPU resource limiters</span>
                  </div>
                  <label class="nixos-switch">
                    <input type="checkbox" id="toggle-gpucap">
                    <span class="nixos-slider"></span>
                  </label>
                </div>
                <div class="nixos-toggle-item">
                  <div class="nixos-toggle-label">
                    <strong>Developer Daemon Control</strong>
                    <span>Run background analytics watcher service</span>
                  </div>
                  <label class="nixos-switch">
                    <input type="checkbox" id="toggle-daemon">
                    <span class="nixos-slider"></span>
                  </label>
                </div>
              </div>
            </div>
          </div>
        </div>
      </div>
    `;

    // Add event listeners
    document.getElementById('nixos-refresh-btn')?.addEventListener('click', () => refreshNixosState());
    
    // Toggle change events
    document.getElementById('toggle-plasma')?.addEventListener('change', (e) => handleToggleChange('plasma', (e.target as HTMLInputElement).checked));
    document.getElementById('toggle-gpucap')?.addEventListener('change', (e) => handleToggleChange('gpucap', (e.target as HTMLInputElement).checked));
    document.getElementById('toggle-daemon')?.addEventListener('change', (e) => handleToggleChange('daemon', (e.target as HTMLInputElement).checked));
  }

  await refreshNixosState();
}

export async function refreshNixosState(): Promise<void> {
  await Promise.all([
    fetchGenerations(),
    fetchDrift(),
    fetchToggles()
  ]);
}

async function fetchGenerations(): Promise<void> {
  const container = document.getElementById('nixos-gens-list');
  if (!container) return;

  try {
    const res = await fetch('/api/nixos/generations');
    if (!res.ok) throw new Error('Failed to load generations');
    const gens: NixosGeneration[] = await res.json();

    if (gens.length === 0) {
      container.innerHTML = '<div class="nixos-empty">No system profiles discovered.</div>';
      return;
    }

    container.innerHTML = gens.map(g => {
      const activeClass = g.active ? 'active' : '';
      const dateStr = new Date(g.createdAt).toLocaleString();
      return `
        <div class="nixos-gen-item ${activeClass}" data-id="${g.id}">
          <div class="nixos-gen-info">
            <span class="nixos-gen-id">Generation #${g.id}</span>
            <span class="nixos-gen-date">${dateStr}</span>
            <span class="nixos-gen-path" title="${g.path}">${g.path}</span>
          </div>
          ${g.active ? '<span class="nixos-active-badge">Active</span>' : `<button class="nixos-rollback-btn" data-id="${g.id}">Rollback</button>`}
        </div>
      `;
    }).join('');

    // Attach rollback event listeners
    container.querySelectorAll('.nixos-rollback-btn').forEach(btn => {
      btn.addEventListener('click', async (e) => {
        e.stopPropagation();
        const id = parseInt((e.target as HTMLElement).getAttribute('data-id') || '0', 10);
        if (id > 0) {
          await triggerRollback(id);
        }
      });
    });

  } catch (err) {
    container.innerHTML = `<div class="nixos-error">Error loading generations: ${(err as Error).message}</div>`;
  }
}

async function fetchDrift(): Promise<void> {
  const fill = document.getElementById('nixos-drift-gauge-fill');
  const val = document.getElementById('nixos-drift-val');
  const status = document.getElementById('nixos-drift-status');
  const details = document.getElementById('nixos-drift-details');

  try {
    const res = await fetch('/api/nixos/drift');
    if (!res.ok) throw new Error();
    const drift: NixosDrift = await res.json();

    if (fill) fill.style.width = `${drift.driftPercent}%`;
    if (val) val.textContent = `${drift.driftPercent}% Drift`;
    if (status) {
      status.textContent = drift.syncStatus;
      status.className = `nixos-drift-status status-${drift.syncStatus.toLowerCase()}`;
    }

    if (details) {
      if (drift.uncommitted.length === 0) {
        details.innerHTML = '<span class="nixos-text-clean"><i class="codicon codicon-check"></i> System configuration matches Flake repository baseline.</span>';
      } else {
        details.innerHTML = `
          <div class="nixos-drift-header">Uncommitted modifications:</div>
          <ul class="nixos-drift-files-list">
            ${drift.uncommitted.map(f => `<li><i class="codicon codicon-file-diff"></i> ${f}</li>`).join('')}
          </ul>
        `;
      }
    }
  } catch (e) {
    if (val) val.textContent = 'Error';
    if (status) status.textContent = 'Offline';
  }
}

async function fetchToggles(): Promise<void> {
  try {
    const res = await fetch('/api/nixos/toggle');
    if (!res.ok) throw new Error();
    const config: NixosConfig = await res.json();

    const plasma = document.getElementById('toggle-plasma') as HTMLInputElement;
    const gpucap = document.getElementById('toggle-gpucap') as HTMLInputElement;
    const daemon = document.getElementById('toggle-daemon') as HTMLInputElement;

    if (plasma) plasma.checked = config.plasmaEnabled;
    if (gpucap) gpucap.checked = config.gpuCapEnabled;
    if (daemon) daemon.checked = config.daemonEnabled;
  } catch (e) {
    // Fail silently or log
  }
}

async function triggerRollback(generationId: number): Promise<void> {
  const confirmSwitch = confirm(`Are you sure you want to rollback to Generation #${generationId}?`);
  if (!confirmSwitch) return;

  try {
    const res = await fetch('/api/nixos/rollback', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ generation: generationId })
    });
    const data = await res.json();
    if (data.success) {
      showToast(`Successfully rolled back system profile to Generation #${generationId}`, 'success');
      await refreshNixosState();
    } else {
      showToast(`Rollback failed: ${data.error}`, 'error');
    }
  } catch (err) {
    showToast('Failed to switch boot profile', 'error');
  }
}

async function handleToggleChange(feature: string, value: boolean): Promise<void> {
  try {
    const res = await fetch('/api/nixos/toggle', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ feature, value })
    });
    const data = await res.json();
    if (data.success) {
      showToast(`State updated successfully: ${feature.toUpperCase()} set to ${value ? 'ON' : 'OFF'}`, 'success');
    } else {
      showToast(`Failed to update toggle: ${data.error}`, 'error');
    }
  } catch (err) {
    showToast('State synchronization failed', 'error');
  }
}
