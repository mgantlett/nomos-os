import { showToast } from './toast.js';
import { refreshData, fetchSafeJson } from './app.js';
import { SelectedDiffFile } from './types.js';

let selectedDiffFile: SelectedDiffFile | null = null;

export async function pruneWorkspaces(wtPath?: string): Promise<void> {
  try {
    const payload = wtPath ? { path: wtPath } : {};
    const response = await fetch('/api/worktrees/prune', {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json'
      },
      body: JSON.stringify(payload)
    });
    if (!response.ok) {
      const errData = await response.json();
      throw new Error(errData.error || `HTTP error ${response.status}`);
    }
    const data = await response.json();
    if (data.pruned && data.pruned.length > 0) {
      showToast(`Successfully pruned ${data.pruned.length} workspace(s)`, 'success');
    } else {
      showToast(data.message || 'No workspaces were pruned');
    }
  } catch (err) {
    showToast(`Failed to prune workspace: ${(err as Error).message}`, 'error');
  } finally {
    refreshData();
  }
}

export async function auditBranches(): Promise<void> {
  const container = document.getElementById('merged-branches-list-container');
  if (container) {
    container.replaceChildren();
    const loader = document.createElement('div');
    loader.style.fontSize = '0.65rem';
    loader.style.fontStyle = 'italic';
    loader.style.color = 'rgba(255,255,255,0.4)';
    loader.style.textAlign = 'center';
    loader.style.padding = '0.25rem';
    loader.textContent = 'Auditing branches...';
    container.appendChild(loader);
  }
  try {
    const response = await fetch('/api/branches/audit');
    if (!response.ok) {
      throw new Error(`HTTP error ${response.status}`);
    }
    const data = await response.json();
    renderMergedBranches(data.branches || [], data.baseBranch || 'develop');
  } catch (err) {
    showToast(`Failed to audit branches: ${(err as Error).message}`, 'error');
    if (container) {
      container.replaceChildren();
      const failLabel = document.createElement('div');
      failLabel.style.fontSize = '0.65rem';
      failLabel.style.fontStyle = 'italic';
      failLabel.style.color = 'var(--neon-red)';
      failLabel.style.textAlign = 'center';
      failLabel.style.padding = '0.25rem';
      failLabel.textContent = 'Failed to audit';
      container.appendChild(failLabel);
    }
  }
}

function renderMergedBranches(branches: string[], baseBranch: string): void {
  (window as any).mergedBranchesList = branches || [];
  const container = document.getElementById('merged-branches-list-container');
  if (!container) return;
  container.replaceChildren();

  const pruneAllBtn = document.getElementById('btn-prune-all-branches');
  if (pruneAllBtn) {
    pruneAllBtn.style.display = branches.length > 0 ? 'inline-block' : 'none';
  }

  if (branches.length === 0) {
    const emptyLabel = document.createElement('div');
    emptyLabel.style.fontSize = '0.65rem';
    emptyLabel.style.fontStyle = 'italic';
    emptyLabel.style.color = 'rgba(255,255,255,0.4)';
    emptyLabel.style.textAlign = 'center';
    emptyLabel.style.padding = '0.25rem';
    emptyLabel.textContent = `No merged branches (base: ${baseBranch})`;
    container.appendChild(emptyLabel);
    return;
  }

  branches.forEach((branch) => {
    const row = document.createElement('div');
    row.style.display = 'flex';
    row.style.alignItems = 'center';
    row.style.justifyContent = 'space-between';
    row.style.padding = '6px 8px';
    row.style.background = 'rgba(255,255,255,0.03)';
    row.style.borderRadius = '4px';
    row.style.border = '1px solid rgba(255,255,255,0.05)';
    row.style.margin = '2px 0';

    const left = document.createElement('div');
    left.style.display = 'flex';
    left.style.alignItems = 'center';
    left.style.gap = '6px';

    const icon = document.createElement('span');
    icon.textContent = '🌱';
    icon.style.fontSize = '0.75rem';
    left.appendChild(icon);

    const name = document.createElement('span');
    name.textContent = branch;
    name.style.fontSize = '0.7rem';
    name.style.fontWeight = '600';
    name.style.fontFamily = 'monospace';
    name.style.color = 'rgba(255,255,255,0.75)';
    left.appendChild(name);

    row.appendChild(left);

    const deleteBtn = document.createElement('button');
    deleteBtn.textContent = '🗑️';
    deleteBtn.title = `Delete branch ${branch}`;
    deleteBtn.style.background = 'none';
    deleteBtn.style.border = 'none';
    deleteBtn.style.color = 'var(--neon-red)';
    deleteBtn.style.cursor = 'pointer';
    deleteBtn.style.fontSize = '0.8rem';
    deleteBtn.style.padding = '2px';
    deleteBtn.style.opacity = '0.6';
    deleteBtn.style.transition = 'all 0.2s';
    deleteBtn.addEventListener('mouseenter', () => {
      deleteBtn.style.opacity = '1';
      deleteBtn.style.transform = 'scale(1.1)';
    });
    deleteBtn.addEventListener('mouseleave', () => {
      deleteBtn.style.opacity = '0.6';
      deleteBtn.style.transform = 'scale(1)';
    });
    deleteBtn.addEventListener('click', async (e) => {
      e.stopPropagation();
      if (confirm(`Are you sure you want to delete local branch "${branch}"?`)) {
        await pruneBranch(branch);
      }
    });

    row.appendChild(deleteBtn);
    container.appendChild(row);
  });
}

export async function pruneBranch(branch: string): Promise<void> {
  try {
    const response = await fetch('/api/branches/prune', {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json'
      },
      body: JSON.stringify({ branch })
    });
    if (!response.ok) {
      const errData = await response.json();
      throw new Error(errData.error || `HTTP error ${response.status}`);
    }
    await response.json();
    showToast(`Successfully pruned branch ${branch}`, 'success');
  } catch (err) {
    showToast(`Failed to prune branch: ${(err as Error).message}`, 'error');
  } finally {
    auditBranches();
  }
}

export async function pruneAllBranches(): Promise<void> {
  try {
    if (!confirm('Are you sure you want to delete ALL audited merged local branches? This cannot be undone.')) {
      return;
    }
    const response = await fetch('/api/branches/prune', {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json'
      },
      body: JSON.stringify({ pruneAll: true })
    });
    if (!response.ok) {
      const errData = await response.json();
      throw new Error(errData.error || `HTTP error ${response.status}`);
    }
    const data = await response.json();
    if (data.pruned && data.pruned.length > 0) {
      showToast(`Successfully pruned ${data.pruned.length} branch(es)`, 'success');
    } else {
      showToast(data.message || 'No branches were pruned');
    }
  } catch (err) {
    showToast(`Failed to prune all branches: ${(err as Error).message}`, 'error');
  } finally {
    auditBranches();
  }
}

export async function fetchGitStatus(): Promise<void> {
  try {
    const pathParam = '';
    const data = await fetchSafeJson<any>(`/api/git/status${pathParam}`, { success: false });
    if (data && data.success) {
      renderGitStatus(data.staged || [], data.unstaged || [], data.untracked || []);
    }
  } catch (err) {
    console.error('Failed to fetch Git status:', err);
  }
}

export async function fetchGitDiff(file: string, staged: boolean): Promise<void> {
  const diffViewport = document.getElementById('git-diff-viewport');
  const fileTitle = document.getElementById('git-diff-file-title');
  const typeBadge = document.getElementById('git-diff-type-badge');

  if (!diffViewport || !fileTitle || !typeBadge) return;

  fileTitle.textContent = file;
  typeBadge.textContent = staged ? 'staged' : 'unstaged';
  typeBadge.style.display = 'inline-block';

  try {
    const pathParam = '';
    const response = await fetch(`/api/git/diff?file=${encodeURIComponent(file)}&staged=${staged}${pathParam}`);
    if (response.ok) {
      const data = await response.json();
      if (data.success) {
        renderVisualDiff(data.diff || '');
      } else {
        diffViewport.textContent = `Error: ${data.error || 'Failed to fetch diff'}`;
        diffViewport.style.color = 'var(--neon-red)';
      }
    } else {
      const data = await response.json();
      diffViewport.textContent = `Error: ${data.error || 'Failed to fetch diff'}`;
      diffViewport.style.color = 'var(--neon-red)';
    }
  } catch (err) {
    diffViewport.textContent = `Network Error: ${(err as Error).message}`;
    diffViewport.style.color = 'var(--neon-red)';
  }
}

function renderVisualDiff(diffText: string): void {
  const diffViewport = document.getElementById('git-diff-viewport');
  if (!diffViewport) return;

  diffViewport.replaceChildren();
  diffViewport.style.color = '';

  if (!diffText.trim()) {
    const emptyMsg = document.createElement('div');
    emptyMsg.className = 'git-diff-placeholder';
    emptyMsg.textContent = 'No differences detected in this file.';
    diffViewport.appendChild(emptyMsg);
    return;
  }

  const lines = diffText.split('\n');
  lines.forEach(line => {
    const lineEl = document.createElement('span');
    lineEl.className = 'git-diff-line';
    lineEl.textContent = line;

    if (line.startsWith('+') && !line.startsWith('+++')) {
      lineEl.classList.add('addition');
    } else if (line.startsWith('-') && !line.startsWith('---')) {
      lineEl.classList.add('deletion');
    } else if (line.startsWith('diff ') || line.startsWith('index ') || line.startsWith('---') || line.startsWith('+++')) {
      lineEl.classList.add('header');
    } else if (line.startsWith('@@ ')) {
      lineEl.classList.add('meta');
    }

    diffViewport.appendChild(lineEl);
  });
}

export function clearDiffView(): void {
  selectedDiffFile = null;
  const diffViewport = document.getElementById('git-diff-viewport');
  const fileTitle = document.getElementById('git-diff-file-title');
  const typeBadge = document.getElementById('git-diff-type-badge');

  if (fileTitle) fileTitle.textContent = 'select a file to view diff';
  if (typeBadge) typeBadge.style.display = 'none';
  if (diffViewport) {
    diffViewport.replaceChildren();
    const emptyMsg = document.createElement('div');
    emptyMsg.className = 'git-diff-placeholder';
    emptyMsg.textContent = 'No file selected. Click on any file in the sidebar to inspect its modifications.';
    diffViewport.appendChild(emptyMsg);
  }
}

export function renderGitStatus(staged: any[], unstaged: any[], untracked: any[]): void {
  const stagedCount = document.getElementById('git-staged-count');
  if (stagedCount) stagedCount.textContent = String(staged.length);
  const unstagedCount = document.getElementById('git-unstaged-count');
  if (unstagedCount) unstagedCount.textContent = String(unstaged.length + untracked.length);

  const stagedList = document.getElementById('git-staged-list');
  if (stagedList) {
    stagedList.replaceChildren();
    if (staged.length === 0) {
      const empty = document.createElement('div');
      empty.style.color = 'var(--text-muted)';
      empty.style.fontSize = '0.75rem';
      empty.style.fontStyle = 'italic';
      empty.style.textAlign = 'center';
      empty.style.padding = '2rem';
      empty.textContent = 'No staged changes';
      stagedList.appendChild(empty);
    } else {
      staged.forEach(change => {
        stagedList.appendChild(createGitFileItem(change.file, change.status, false));
      });
    }
  }

  const unstagedList = document.getElementById('git-unstaged-list');
  if (unstagedList) {
    unstagedList.replaceChildren();
    if (unstaged.length === 0 && untracked.length === 0) {
      const empty = document.createElement('div');
      empty.style.color = 'var(--text-muted)';
      empty.style.fontSize = '0.75rem';
      empty.style.fontStyle = 'italic';
      empty.style.textAlign = 'center';
      empty.style.padding = '2rem';
      empty.textContent = 'No unstaged changes';
      unstagedList.appendChild(empty);
    } else {
      unstaged.forEach(change => {
        unstagedList.appendChild(createGitFileItem(change.file, change.status, true));
      });
      untracked.forEach(change => {
        unstagedList.appendChild(createGitFileItem(change.file, 'untracked', true));
      });
    }
  }

  if (selectedDiffFile) {
    const isStaged = staged.some(c => c.file === selectedDiffFile?.file);
    const isUnstaged = unstaged.some(c => c.file === selectedDiffFile?.file) || untracked.some(c => c.file === selectedDiffFile?.file);

    if (isStaged || isUnstaged) {
      fetchGitDiff(selectedDiffFile.file, selectedDiffFile.staged);
    } else {
      clearDiffView();
    }
  }
}

export function getBasename(file: string): string {
  const parts = file.split('/');
  return parts[parts.length - 1];
}

function createSvgElement(isStageAction: boolean): SVGElement {
  const svg = document.createElementNS("http://www.w3.org/2000/svg", "svg");
  svg.setAttribute("width", "14");
  svg.setAttribute("height", "14");
  svg.setAttribute("viewBox", "0 0 24 24");
  svg.setAttribute("fill", "none");
  svg.setAttribute("stroke", "currentColor");
  svg.setAttribute("stroke-width", "2.5");
  svg.setAttribute("stroke-linecap", "round");
  svg.setAttribute("stroke-linejoin", "round");

  const rect = document.createElementNS("http://www.w3.org/2000/svg", "rect");
  rect.setAttribute("x", "3");
  rect.setAttribute("y", "3");
  rect.setAttribute("width", "18");
  rect.setAttribute("height", "18");
  rect.setAttribute("rx", "2");
  rect.setAttribute("ry", "2");
  svg.appendChild(rect);

  if (isStageAction) {
    const line1 = document.createElementNS("http://www.w3.org/2000/svg", "line");
    line1.setAttribute("x1", "12");
    line1.setAttribute("y1", "8");
    line1.setAttribute("x2", "12");
    line1.setAttribute("y2", "16");
    svg.appendChild(line1);

    const line2 = document.createElementNS("http://www.w3.org/2000/svg", "line");
    line2.setAttribute("x1", "8");
    line2.setAttribute("y1", "12");
    line2.setAttribute("x2", "16");
    line2.setAttribute("y2", "12");
    svg.appendChild(line2);
  } else {
    const line = document.createElementNS("http://www.w3.org/2000/svg", "line");
    line.setAttribute("x1", "8");
    line.setAttribute("y1", "12");
    line.setAttribute("x2", "16");
    line.setAttribute("y2", "12");
    svg.appendChild(line);
  }

  return svg;
}

export function createGitFileItem(file: string, status: string, isStageAction: boolean): HTMLElement {
  const item = document.createElement('div');
  item.className = 'git-file-item';

  if (selectedDiffFile && selectedDiffFile.file === file && selectedDiffFile.staged === !isStageAction) {
    item.classList.add('selected');
  }

  const info = document.createElement('div');
  info.className = 'git-file-info';

  const badge = document.createElement('span');
  badge.className = `git-file-status ${status}`;
  badge.textContent = status === 'untracked' ? 'UNT' : status.substring(0, 3);
  info.appendChild(badge);

  const name = document.createElement('span');
  name.className = 'git-file-name';
  name.textContent = file;
  name.title = file;
  info.appendChild(name);

  item.appendChild(info);

  const actionBtn = document.createElement('button');
  actionBtn.className = 'git-file-action';
  actionBtn.title = isStageAction ? 'Stage Changes' : 'Unstage Changes';
  actionBtn.appendChild(createSvgElement(isStageAction));

  actionBtn.addEventListener('click', async (e) => {
    e.stopPropagation();
    actionBtn.disabled = true;
    try {
      const payload: any = { file, stage: isStageAction };
      
      const response = await fetch('/api/git/stage', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(payload)
      });
      if (response.ok) {
        showToast(isStageAction ? `Staged ${getBasename(file)}` : `Unstaged ${getBasename(file)}`, 'success');
        selectedDiffFile = { file, staged: isStageAction };
        fetchGitStatus();
      } else {
        const data = await response.json();
        showToast(data.error || 'Failed to update stage state', 'error');
      }
    } catch (err) {
      showToast(`Error: ${(err as Error).message}`, 'error');
    }
  });

  item.appendChild(actionBtn);

  item.addEventListener('click', () => {
    document.querySelectorAll('.git-file-item').forEach(el => el.classList.remove('selected'));
    item.classList.add('selected');
    selectedDiffFile = { file, staged: !isStageAction };
    fetchGitDiff(file, !isStageAction);
  });

  return item;
}
