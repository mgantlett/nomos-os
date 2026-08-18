// hud.ts - Swarm Sovereign Context UI module
import { showToast } from './toast.js';

export function updateHeaderContextIndicator(): void {
  const headerIndicator = document.getElementById('header-context-indicator');
  const headerLabel = document.getElementById('header-context-label');
  const resetBtn = document.getElementById('btn-header-reset-context');
  if (!headerIndicator || !headerLabel) return;

  headerIndicator.style.display = 'flex';
  const ideTaskId = (window as any).ideActiveTaskId || '';
  headerLabel.textContent = ideTaskId ? `Root (Task ${ideTaskId})` : 'Root';
  if (resetBtn) resetBtn.style.display = 'none';
}
