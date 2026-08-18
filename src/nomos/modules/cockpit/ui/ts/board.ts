/* eslint-disable */
// agent_swimlanes.ts - Swarm Cockpit Agent Swimlanes & Swarm Matrix Module

import { parseAnsiLine } from './ansi.js';
import { addWSListener, removeWSListener } from './ws.js';

export interface TaskCard {
  key: string;
  title: string;
  labels: string[];
  status: string;
  html_url: string;
  body?: string;
  description?: string;
  assignee?: { login: string; avatar_url: string };
  dorStatus?: string;
  dodStatus?: string;
  parent_key?: string;
  type?: string;
  points?: number | string;
  blocked_by?: string[];
  estimated_duration?: string;
  project?: string;
}

let lastRenderedStateStr = '';

let kanbanFiltersInitialized = false;
let activeStatusFilter = 'OPEN';
let activeLabelFilters = new Set<string>();

export let activeKanbanTags: string[] = [];
export let activeKanbanOperator: 'AND' | 'OR' = 'OR';
let recentKanbanSearches: string[] = [];

function initKanbanFilters() {
  if (kanbanFiltersInitialized) return;
  
  const btnOpen = document.getElementById('status-toggle-open');
  const btnClosed = document.getElementById('status-toggle-closed');
  
  if (btnOpen && btnClosed) {
    kanbanFiltersInitialized = true;
    btnOpen.addEventListener('click', () => {
      activeStatusFilter = 'OPEN';
      btnOpen.classList.add('active');
      btnOpen.style.background = 'rgba(16, 185, 129, 0.2)';
      btnOpen.style.color = 'var(--neon-green)';
      btnClosed.classList.remove('active');
      btnClosed.style.background = 'transparent';
      btnClosed.style.color = 'var(--text-muted)';
      lastRenderedStateStr = ''; 
      if ((window as any).refreshData) (window as any).refreshData();
    });
    btnClosed.addEventListener('click', () => {
      activeStatusFilter = 'CLOSED';
      btnClosed.classList.add('active');
      btnClosed.style.background = 'rgba(239, 68, 68, 0.2)';
      btnClosed.style.color = 'var(--neon-red)';
      btnOpen.classList.remove('active');
      btnOpen.style.background = 'transparent';
      btnOpen.style.color = 'var(--text-muted)';
      lastRenderedStateStr = '';
      if ((window as any).refreshData) (window as any).refreshData();
    });
  }

  const labelBtn = document.getElementById('label-filter-btn');
  const labelDropdown = document.getElementById('label-filter-dropdown');
  if (labelBtn && labelDropdown) {
    labelBtn.addEventListener('click', (e) => {
      e.stopPropagation();
      labelDropdown.style.display = labelDropdown.style.display === 'none' ? 'block' : 'none';
    });
    document.addEventListener('click', (e) => {
      if (!labelDropdown.contains(e.target as Node) && e.target !== labelBtn) {
        labelDropdown.style.display = 'none';
      }
    });
  }
}

export function initKanbanSearch(): void {
  const container = document.getElementById('kanban-search-container');
  const input = document.getElementById('kanban-search') as HTMLInputElement | null;
  const datalist = document.getElementById('recent-searches') as HTMLDataListElement | null;
  const tagContainer = document.getElementById('kanban-tag-container');
  const clearBtn = document.getElementById('kanban-search-clear');
  const operatorToggle = document.getElementById('kanban-operator-toggle');

  if (!input || !tagContainer || !clearBtn || !operatorToggle || !datalist || !container) return;

  function updateDatalist() {
    datalist!.innerHTML = '';
    const unique = Array.from(new Set(recentKanbanSearches)).slice(0, 5);
    unique.forEach(term => {
      const option = document.createElement('option');
      option.value = term;
      datalist!.appendChild(option);
    });
  }

  function renderTags() {
    tagContainer!.innerHTML = '';
    activeKanbanTags.forEach((tag, idx) => {
      const el = document.createElement('div');
      el.className = 'kanban-tag';
      el.innerHTML = `<span>${tag}</span><div class="kanban-tag-close">×</div>`;
      el.querySelector('.kanban-tag-close')!.addEventListener('click', (e) => {
        e.stopPropagation();
        activeKanbanTags.splice(idx, 1);
        renderTags();
        resetKanbanCache();
        if ((window as any).refreshData) (window as any).refreshData();
      });
      tagContainer!.appendChild(el);
    });

    if (activeKanbanTags.length > 0 || input!.value.trim().length > 0) {
      clearBtn!.style.display = 'flex';
      operatorToggle!.style.display = 'flex';
    } else {
      clearBtn!.style.display = 'none';
      operatorToggle!.style.display = 'none';
    }
  }

  clearBtn.addEventListener('click', (e) => {
    e.stopPropagation();
    activeKanbanTags = [];
    input.value = '';
    renderTags();
    resetKanbanCache();
    if ((window as any).refreshData) (window as any).refreshData();
  });

  operatorToggle.addEventListener('click', (e) => {
    e.stopPropagation();
    activeKanbanOperator = activeKanbanOperator === 'AND' ? 'OR' : 'AND';
    operatorToggle.textContent = activeKanbanOperator;
    operatorToggle.className = 'kanban-operator-toggle mode-' + activeKanbanOperator.toLowerCase();
    resetKanbanCache();
    if ((window as any).refreshData) (window as any).refreshData();
  });

  input.addEventListener('keydown', (e) => {
    if (e.key === 'Enter') {
      const val = input.value.trim();
      if (val) {
        activeKanbanTags.push(val);
        recentKanbanSearches.unshift(val);
        updateDatalist();
        input.value = '';
        renderTags();
        resetKanbanCache();
        if ((window as any).refreshData) (window as any).refreshData();
      }
    }
  });

  input.addEventListener('input', () => {
    renderTags();
    resetKanbanCache();
    if ((window as any).refreshData) (window as any).refreshData();
  });

  container.addEventListener('click', (e) => {
    if (e.target !== operatorToggle && e.target !== clearBtn) {
      input.focus();
    }
  });

  renderTags();
  operatorToggle.textContent = activeKanbanOperator;
  operatorToggle.className = 'kanban-operator-toggle mode-' + activeKanbanOperator.toLowerCase();
}

let draggedCardSourceColumn = '';

/**
 * Resets the Kanban board render cache, forcing a redraw on the next update.
 */
export function resetKanbanCache(): void {
  lastRenderedStateStr = '';
}

/**
 * Checks if a specific task ID is included in the active task ID string.
 * @param taskNum The numeric ID of the task to check
 * @param activeTaskId The comma-separated string of active task IDs
 * @returns true if the task is currently active
 */
function isTaskIdActive(taskKey: string | number, activeTaskId: string): boolean {
  if (!activeTaskId) return false;
  const targetStr = String(taskKey).trim().toLowerCase();
  const parts = activeTaskId.split(',').map(s => s.trim().toLowerCase());

  return parts.some(p => {
    if (p === targetStr) return true;
    const cleanP = p.startsWith('#') ? p.slice(1) : p;
    const cleanTarget = targetStr.startsWith('#') ? targetStr.slice(1) : targetStr;
    if (cleanP === cleanTarget) return true;

    const pHasPrefix = cleanP.includes('-');
    const targetHasPrefix = cleanTarget.includes('-');
    if (!pHasPrefix && !targetHasPrefix) {
      return cleanP === cleanTarget;
    }
    return false;
  });
}

function getSafeHueForString(str: string): number {
  const safeHues = [200, 260, 280, 220, 240, 300, 320, 30, 40, 210, 270, 310, 250];
  let hash = 0;
  for (let i = 0; i < str.length; i++) {
    hash = str.charCodeAt(i) + ((hash << 5) - hash);
  }
  return safeHues[Math.abs(hash) % safeHues.length];
}

/**
 * Determines the appropriate Kanban column for a given task card based on phase state.
 * @param task The task card object
 * @param currentPhase The global workspace phase
 * @param activeTaskId The globally active task ID
 * @param isTaskRunning True if the task has an active background swarm process
 * @param matchingWorktree The isolated worktree object for the task, if one exists
 * @returns The column name string
 */
function resolveTaskColumn(
  task: TaskCard,
  currentPhase: string,
  activeTaskId: string,
  isTaskRunning: boolean,
  matchingWorktree: any
): string {
  const statusLower = (task.status || '').toLowerCase().trim();
  const isClosed = statusLower === 'closed' || statusLower === 'done' || statusLower === 'completed' || statusLower === 'cancelled' || statusLower === 'canceled';
  if (isClosed) return 'DONE';
  if (statusLower === 'triage') return 'TRIAGE';
  if (statusLower === 'plan' || statusLower === 'edit' || statusLower === 'review' || statusLower === 'in_progress' || statusLower === 'in-progress' || statusLower === 'in_review') {
    return 'IN_PROGRESS';
  }
  
  if (isTaskRunning) return 'IN_PROGRESS';
  if (matchingWorktree && matchingWorktree.phase && matchingWorktree.phase !== 'IDLE' && matchingWorktree.phase !== 'DONE') {
    return 'IN_PROGRESS';
  }
  if (isTaskIdActive(task.key, activeTaskId) && currentPhase !== 'DONE' && currentPhase !== 'IDLE') {
    return 'IN_PROGRESS';
  }

  if (statusLower === 'backlog') return 'BACKLOG';

  return 'BACKLOG';
}

/**
 * Handles clicks on a task card to select or reset the active sovereign context.
 * @param e The click event object
 * @param task The task card that was clicked
 */
function handleCardClick(e: MouseEvent, task: TaskCard): void {
  if ((e.target as HTMLElement).closest('button') || (e.target as HTMLElement).closest('a')) {
    return;
  }
  if ((window as any).openAgentDetailsDrawer) {
    (window as any).openAgentDetailsDrawer(task);
  } else if ((window as any).openTaskDetailsDrawer) {
    (window as any).openTaskDetailsDrawer(task.key);
  }
}



function processKanbanTask(
  task: any,
  currentPhase: string,
  activeTaskId: string,
  activeKanbanTags: string[],
  globalSearchQuery: string,
  activeKanbanOperator: string,
  activeLabelFilters: Set<string>,
  activeStatusFilter: string,
  tier1Container: HTMLElement | null,
  tier2Container: HTMLElement | null,
  tbody: HTMLTableSectionElement
): { rendered: boolean, isClosed: boolean } {
    const statusLower = (task.status || '').toLowerCase();
    const isClosed = statusLower === 'closed' || statusLower === 'done' || statusLower === 'completed' || statusLower === 'cancelled' || statusLower === 'canceled';

    // Setup active swarm checks
    const latestSwarmData = (window as any).latestSwarmData || { nodes: [] };
    const isTaskRunning = Array.isArray(latestSwarmData.nodes) && latestSwarmData.nodes.some((n: any) => {
      return n.type === 'worker' && String(n.taskID) === String(task.key);
    });

    // Map column first
    const activeSwarms = (window as any).activeSwarmsList || [];
    const matchingWorktree = activeSwarms.find((sw: any) => {
      return sw.name === `task-${task.key}` || sw.name === String(task.key);
    });

    const column = resolveTaskColumn(task, currentPhase, activeTaskId, isTaskRunning, matchingWorktree);

    const matchText = `#${task.key} ${task.key} ${task.title} ${task.body || ''} ${task.description || ''} ${task.labels ? task.labels.join(' ') : ''}`.toLowerCase();
    
    let allSearchTerms = [...activeKanbanTags];
    if (globalSearchQuery) {
      allSearchTerms.push(globalSearchQuery);
    }
    
    if (allSearchTerms.length > 0) {
      if (activeKanbanOperator === 'AND') {
        const hasAll = allSearchTerms.every(term => matchText.includes(term.toLowerCase()));
        if (!hasAll) return { rendered: false, isClosed };
      } else {
        const hasAny = allSearchTerms.some(term => matchText.includes(term.toLowerCase()));
        if (!hasAny) return { rendered: false, isClosed };
      }
    }

    const kanbanProjectSelector = document.getElementById('project-selector') as HTMLSelectElement | null;
    const isCommunityMode = (window as any).latestStatus && (window as any).latestStatus.edition === 'community';
    if (kanbanProjectSelector && kanbanProjectSelector.value !== 'ALL' && !isCommunityMode) {
      if (task.project && task.project.toLowerCase() !== kanbanProjectSelector.value.toLowerCase()) {
        return { rendered: false, isClosed };
      }
    }

    // Counting handled by caller

    const card = document.createElement('div');
    card.className = 'task-card';
    card.setAttribute('data-id', String(task.key));
    card.style.cursor = 'pointer';
    
    const pKey = task.parent_key || (task.type?.toLowerCase() === 'epic' ? task.key : 'unbundled');
    let bundleHue = 0;
    if (pKey !== 'unbundled') {
      bundleHue = getSafeHueForString(pKey);
      card.style.borderLeft = `3px solid hsl(${bundleHue}, 70%, 50%)`;
    }

    if (isTaskIdActive(task.key, activeTaskId)) {
      card.classList.add('active-selection');
    }
    
    let activeAgentSlot = null;
    const swarmSlotsData = (window as any).swarmSlotsData;
    if (swarmSlotsData && swarmSlotsData.slotStates) {
      activeAgentSlot = swarmSlotsData.slotStates.find((s: any) => s.is_locked && String(s.lock_task_id) === String(task.key));
    }

    const isDodFailed = task.dodStatus && (task.dodStatus.includes('Blocked') || task.dodStatus.includes('Failed') || task.dodStatus.includes('Violation'));

    if (isDodFailed) {
      card.style.border = '1px solid var(--neon-red, #ef4444)';
      card.style.boxShadow = '0 0 15px rgba(239, 68, 68, 0.4)';
    } else if (activeAgentSlot || isTaskRunning) {
      card.style.border = '1px solid var(--neon-green)';
      card.style.boxShadow = '0 0 15px rgba(16, 185, 129, 0.2)';
    }

    card.addEventListener('click', (e) => handleCardClick(e, task));

    const header = document.createElement('div');
    header.className = 'card-header';

    const leftHeader = document.createElement('div');
    leftHeader.style.display = 'flex';
    leftHeader.style.gap = '8px';
    leftHeader.style.alignItems = 'center';

    const cardId = document.createElement('a');
    cardId.className = 'card-id task-id-link';
    cardId.href = '#';
    cardId.textContent = `#${task.key}`;
    cardId.style.cursor = 'pointer';
    cardId.style.textDecoration = 'underline';
    cardId.style.color = 'var(--neon-green)';
    cardId.addEventListener('click', (e) => {
      e.stopPropagation();
      e.preventDefault();
      if ((window as any).openTaskDetailsDrawer) {
        (window as any).openTaskDetailsDrawer(task.key);
      }
    });
    leftHeader.appendChild(cardId);

    const statusUpper = (task.status || '').toUpperCase();
    if (statusUpper === 'CANCELLED' || statusUpper === 'CANCELED') {
      const cardState = document.createElement('span');
      cardState.style.fontSize = '0.65rem';
      cardState.style.textTransform = 'uppercase';
      cardState.style.fontWeight = 'bold';
      cardState.style.color = 'var(--text-danger, #ef4444)';
      cardState.textContent = 'CANCELLED';
      leftHeader.appendChild(cardState);
    }
    
    if (pKey !== 'unbundled') {
      const bundleBadge = document.createElement('span');
      bundleBadge.style.fontSize = '0.65rem';
      bundleBadge.style.fontWeight = 'bold';
      bundleBadge.style.color = `hsl(${bundleHue}, 80%, 70%)`;
      bundleBadge.style.background = `hsla(${bundleHue}, 70%, 50%, 0.1)`;
      bundleBadge.style.padding = '1px 4px';
      bundleBadge.style.borderRadius = '3px';
      bundleBadge.textContent = `Epic #${pKey}`;
      leftHeader.appendChild(bundleBadge);
    }
    
    const projectSelector = document.getElementById('project-selector') as HTMLSelectElement | null;
    if (projectSelector && projectSelector.value === 'ALL' && task.project) {
      const projectBadge = document.createElement('span');
      projectBadge.style.fontSize = '0.65rem';
      projectBadge.style.fontWeight = 'bold';
      projectBadge.style.color = 'var(--neon-blue)';
      projectBadge.style.background = 'rgba(59, 130, 246, 0.1)';
      projectBadge.style.padding = '1px 4px';
      projectBadge.style.borderRadius = '3px';
      projectBadge.textContent = task.project;
      leftHeader.appendChild(projectBadge);
    }
    
    if (activeAgentSlot) {
      const avatarOrb = document.createElement('div');
      avatarOrb.style.width = '18px';
      avatarOrb.style.height = '18px';
      avatarOrb.style.borderRadius = '50%';
      avatarOrb.className = 'pulse-glow';
      avatarOrb.style.background = 'var(--neon-green)';
      avatarOrb.style.boxShadow = '0 0 8px var(--neon-green)';
      avatarOrb.style.border = '1px solid rgba(255,255,255,0.8)';
      avatarOrb.style.display = 'flex';
      avatarOrb.style.alignItems = 'center';
      avatarOrb.style.justifyContent = 'center';
      avatarOrb.style.fontSize = '0.5rem';
      avatarOrb.style.color = 'var(--text-main)';
      avatarOrb.style.fontWeight = 'bold';
      avatarOrb.title = `Agent: ${activeAgentSlot.lock_owner || `A${activeAgentSlot.index}`}`;
      avatarOrb.textContent = `A${activeAgentSlot.index}`;
      leftHeader.appendChild(avatarOrb);
    }

    const tAny = task as any;
    const rawTime = tAny.updated_at || tAny.updated || tAny.created_at || tAny.created;
    if (rawTime) {
      const updatedBadge = document.createElement('span');
      updatedBadge.style.fontSize = '0.6rem';
      updatedBadge.style.fontFamily = 'monospace';
      updatedBadge.style.color = 'var(--text-muted)';
      updatedBadge.style.marginLeft = 'auto';
      updatedBadge.style.opacity = '0.8';
      updatedBadge.textContent = `⏱️ ${formatRelativeTime(rawTime)}`;
      updatedBadge.title = `Last updated: ${new Date(rawTime).toLocaleString()}`;
      leftHeader.appendChild(updatedBadge);
    }

    header.appendChild(leftHeader);

    const cardActions = document.createElement('div');
    if (matchingWorktree) {
      // Worktree logic
    }
    card.appendChild(header);

    const title = document.createElement('div');
    title.className = 'card-title';
    title.textContent = task.title;
    title.setAttribute('title', task.title);
    card.appendChild(title);

    const rootWorktree = activeSwarms.find((sw: any) => sw.path === (window as any).rootWorkspacePath);
    let branchName = '';
    let isSwarmWT = false;
    let isMerged = false;

    if (matchingWorktree && matchingWorktree.branch) {
      branchName = matchingWorktree.branch;
      isSwarmWT = true;
    } else if (isTaskIdActive(task.key, activeTaskId) && rootWorktree && rootWorktree.branch) {
      branchName = rootWorktree.branch;
    }

    const mergedList = (window as any).mergedBranchesList || [];
    if (branchName && mergedList.includes(branchName)) {
      isMerged = true;
    }

    if (branchName) {
      const branchContainer = document.createElement('div');
      branchContainer.className = 'card-branch-container';
      branchContainer.style.display = 'flex';
      branchContainer.style.alignItems = 'center';
      branchContainer.style.justifyContent = 'space-between';
      branchContainer.style.marginTop = '6px';
      branchContainer.style.background = 'rgba(255, 255, 255, 0.02)';
      branchContainer.style.padding = '3px 8px';
      branchContainer.style.borderRadius = '4px';
      branchContainer.style.border = '1px solid rgba(255, 255, 255, 0.05)';

      const branchInfo = document.createElement('span');
      branchInfo.className = 'card-branch-info';
      branchInfo.style.fontSize = '0.65rem';
      branchInfo.style.color = isMerged ? 'var(--neon-green)' : 'var(--text-muted)';
      branchInfo.style.whiteSpace = 'nowrap';
      branchInfo.style.overflow = 'hidden';
      branchInfo.style.textOverflow = 'ellipsis';
      branchInfo.style.maxWidth = '150px';
      branchInfo.textContent = `🌿 ${branchName}`;
      branchInfo.setAttribute('title', branchName);
      branchContainer.appendChild(branchInfo);

      const envBadge = document.createElement('span');
      envBadge.style.fontSize = '0.6rem';
      envBadge.style.fontFamily = 'monospace';
      envBadge.style.padding = '1px 4px';
      envBadge.style.borderRadius = '3px';
      envBadge.style.fontWeight = 'bold';
      envBadge.style.marginLeft = '4px';

      if (isSwarmWT) {
        envBadge.textContent = '🌳 WORKTREE';
        envBadge.style.background = 'rgba(168, 85, 247, 0.15)';
        envBadge.style.color = '#c084fc';
        envBadge.style.border = '1px solid rgba(168, 85, 247, 0.4)';
        const wtPath = matchingWorktree.path || `~/.nomos/data/${task.project || 'project'}/worktrees/task-${task.key}`;
        envBadge.title = `Isolated Task Worktree: ${wtPath}`;
      } else {
        envBadge.textContent = '📂 ROOT';
        envBadge.style.background = 'rgba(59, 130, 246, 0.15)';
        envBadge.style.color = '#60a5fa';
        envBadge.style.border = '1px solid rgba(59, 130, 246, 0.4)';
        envBadge.title = `Primary Repo Root Workspace: ~/Projects/...`;
      }
      branchContainer.appendChild(envBadge);

      const branchActions = document.createElement('div');
      branchActions.style.display = 'flex';
      branchActions.style.gap = '4px';
      branchActions.style.alignItems = 'center';

      if (isMerged) {
        const mergedBadge = document.createElement('span');
        mergedBadge.textContent = 'MERGED';
        mergedBadge.style.fontSize = '0.55rem';
        mergedBadge.style.background = 'rgba(16, 185, 129, 0.15)';
        mergedBadge.style.color = 'var(--neon-green)';
        mergedBadge.style.padding = '1px 4px';
        mergedBadge.style.borderRadius = '2px';
        mergedBadge.style.fontWeight = 'bold';
        branchActions.appendChild(mergedBadge);

        const pruneBranchBtn = document.createElement('button');
        pruneBranchBtn.textContent = '🗑️';
        pruneBranchBtn.title = `Delete merged local branch "${branchName}"`;
        pruneBranchBtn.style.background = 'none';
        pruneBranchBtn.style.border = 'none';
        pruneBranchBtn.style.color = 'var(--neon-red)';
        pruneBranchBtn.style.cursor = 'pointer';
        pruneBranchBtn.style.fontSize = '0.75rem';
        pruneBranchBtn.style.padding = '0';
        pruneBranchBtn.addEventListener('click', async (e) => {
          e.stopPropagation();
          if (confirm(`Are you sure you want to delete merged local branch "${branchName}"?`)) {
            if ((window as any).pruneBranch) {
              await (window as any).pruneBranch(branchName);
            }
          }
        });
        branchActions.appendChild(pruneBranchBtn);
      }

      branchContainer.appendChild(branchActions);
      card.appendChild(branchContainer);
    }

    if (task.labels) {
      const tags = document.createElement('div');
      tags.className = 'card-tags';
      task.labels.forEach((lbl) => {
        const isCli = lbl.startsWith('cli:');
        const isProfile =
          lbl.startsWith('blast_radius:') || lbl.startsWith('tier:') || lbl.startsWith('context_depth:');
        if (column === 'BACKLOG' || isCli || isProfile) {
          const tag = document.createElement('span');
          tag.className = 'tag';
          if (lbl.startsWith('Size:') || lbl.startsWith('agent:')) {
            tag.classList.add('primary');
          } else if (isCli) {
            const level = lbl.split(':')[1];
            tag.classList.add(`cli-${level}`);
          } else if (isProfile) {
            tag.classList.add('profile-tag');
          }
          tag.textContent = lbl;
          tags.appendChild(tag);
        }
      });
      if (tags.children.length > 0) {
        card.appendChild(tags);
      }
    }

    // DoR status, DoD progress, and worker/IDE active metrics
    const metrics = document.createElement('div');
    metrics.className = 'card-metrics';

    // Show WorkspacePhase Badge for IN_PROGRESS tasks
    if (column === 'IN_PROGRESS') {
      let actualPhase = 'EDIT'; // Default fallback
      if (matchingWorktree && matchingWorktree.phase && matchingWorktree.phase !== 'IDLE') {
        actualPhase = matchingWorktree.phase;
      } else if (isTaskIdActive(task.key, activeTaskId) && currentPhase && currentPhase !== 'IDLE') {
        actualPhase = currentPhase;
      }

      const phaseBadge = document.createElement('span');
      phaseBadge.className = 'card-badge';
      phaseBadge.style.fontWeight = 'bold';
      phaseBadge.style.fontFamily = 'monospace';
      phaseBadge.style.fontSize = '0.65rem';
      phaseBadge.style.padding = '2px 6px';
      phaseBadge.style.borderRadius = '4px';

      if (actualPhase === 'PLAN') {
        phaseBadge.textContent = `📋 Phase: PLAN`;
        phaseBadge.style.background = 'rgba(168, 85, 247, 0.2)';
        phaseBadge.style.color = '#c084fc';
        phaseBadge.style.border = '1px solid rgba(168, 85, 247, 0.4)';
      } else if (actualPhase === 'EDIT') {
        phaseBadge.textContent = `🔨 Phase: EDIT`;
        phaseBadge.style.background = 'rgba(59, 130, 246, 0.2)';
        phaseBadge.style.color = '#60a5fa';
        phaseBadge.style.border = '1px solid rgba(59, 130, 246, 0.4)';
      } else if (actualPhase === 'VALIDATE') {
        phaseBadge.textContent = `🛡️ Phase: VALIDATE`;
        phaseBadge.style.background = 'rgba(6, 182, 212, 0.2)';
        phaseBadge.style.color = '#22d3ee';
        phaseBadge.style.border = '1px solid rgba(6, 182, 212, 0.4)';
      } else if (actualPhase === 'REVIEW') {
        phaseBadge.textContent = `⏸️ Phase: REVIEW (HITL)`;
        phaseBadge.style.background = 'rgba(234, 179, 8, 0.25)';
        phaseBadge.style.color = '#facc15';
        phaseBadge.style.border = '1px solid rgba(234, 179, 8, 0.5)';
      } else if (actualPhase === 'DONE' || actualPhase === 'CLOSED') {
        phaseBadge.textContent = `✅ Phase: DONE`;
        phaseBadge.style.background = 'rgba(34, 197, 94, 0.2)';
        phaseBadge.style.color = '#4ade80';
        phaseBadge.style.border = '1px solid rgba(34, 197, 94, 0.4)';
      } else {
        phaseBadge.textContent = `Phase: ${actualPhase}`;
        phaseBadge.style.background = 'rgba(156, 163, 175, 0.2)';
        phaseBadge.style.color = '#cbd5e1';
      }
      metrics.appendChild(phaseBadge);

      const assigneeTask = (task as any).assignee || '';
      const isAssignedToSwarmTask = assigneeTask.startsWith('swarm:') || assigneeTask === 'opencode' || assigneeTask === 'swarm-opencode' || (task as any).tier === 'T2' || (task as any).tier === '2';
      const isTier2Card = isTaskRunning || activeAgentSlot || isAssignedToSwarmTask;

      if (isTier2Card) {
        const opencodeBadge = document.createElement('span');
        opencodeBadge.className = 'card-badge';
        opencodeBadge.style.background = 'rgba(239, 68, 68, 0.15)';
        opencodeBadge.style.color = '#f87171';
        opencodeBadge.style.border = '1px solid rgba(239, 68, 68, 0.4)';
        opencodeBadge.style.display = 'inline-flex';
        opencodeBadge.style.alignItems = 'center';
        opencodeBadge.style.gap = '4px';
        opencodeBadge.style.fontWeight = 'bold';
        opencodeBadge.style.fontSize = '0.65rem';
        opencodeBadge.style.padding = '2px 6px';
        opencodeBadge.style.borderRadius = '4px';
        opencodeBadge.innerHTML = `<img src="/public/assets/opencode_favicon.ico" style="width: 12px; height: 12px; object-fit: contain;"> OpenCode Swarm`;
        metrics.appendChild(opencodeBadge);
      }

      const dodBadge = document.createElement('span');
      dodBadge.className = 'card-badge';
      const dodVal = task.dodStatus || 'Pending';
      dodBadge.textContent = dodVal;
      if (dodVal.includes('100%') || dodVal.includes('All Green') || dodVal.includes('Green')) {
        dodBadge.classList.add('dod-complete');
      } else if (dodVal.includes('Blocked') || dodVal.includes('Failed')) {
        dodBadge.classList.add('dod-blocked');
      } else {
        dodBadge.classList.add('dod-progress');
      }
      metrics.appendChild(dodBadge);
    }

    const isMainWorkspaceActive =
      isTaskIdActive(task.key, activeTaskId) && currentPhase && currentPhase !== 'IDLE';

    if (isTaskRunning || activeAgentSlot) {
      const runningBadge = document.createElement('span');
      runningBadge.className = 'card-badge worker-running';
      runningBadge.textContent = '⚡ RUNNING';
      metrics.appendChild(runningBadge);
    }

    if (metrics.children.length > 0) {
      card.appendChild(metrics);
    }

    if ((task as any).assignee?.startsWith('swarm:') || isTaskRunning || activeAgentSlot || matchingWorktree) {
      const mwBtn = document.createElement('button');
      mwBtn.className = 'mw-toggle-btn';
      mwBtn.textContent = '🌌 Expand Micro World';
      mwBtn.style.marginTop = '8px';
      mwBtn.style.width = '100%';
      mwBtn.style.background = 'rgba(139, 92, 246, 0.15)';
      mwBtn.style.border = '1px solid var(--border-purple)';
      mwBtn.style.color = 'var(--neon-purple)';
      mwBtn.style.padding = '4px';
      mwBtn.style.borderRadius = '4px';
      mwBtn.style.cursor = 'pointer';
      mwBtn.style.fontSize = '0.7rem';
      
      const mwTemplate = document.getElementById('micro-world-template') as HTMLTemplateElement;
      if (mwTemplate) {
        const mwContent = mwTemplate.content.cloneNode(true) as DocumentFragment;
        const mwContainer = mwContent.querySelector('.micro-world-container') as HTMLElement;
        const diffBtn = mwContainer.querySelector('.mw-fetch-diff-btn') as HTMLButtonElement;
        const diffContent = mwContainer.querySelector('.mw-diff-content') as HTMLElement;
        const terminal = mwContainer.querySelector('.mw-terminal-content') as HTMLElement;
        


        const pauseBtn = mwContainer.querySelector('.mw-pause-btn') as HTMLButtonElement | null;
        const resumeBtn = mwContainer.querySelector('.mw-resume-btn') as HTMLButtonElement | null;
        const killBtn = mwContainer.querySelector('.mw-kill-btn') as HTMLButtonElement | null;

        if (pauseBtn) {
          pauseBtn.addEventListener('click', async (e) => {
            e.stopPropagation();
            try {
              await fetch('/api/swarm/pause/' + task.key, { method: 'POST' });
              if ((window as any).showToast) (window as any).showToast(`Paused worker daemon for task #${task.key}`, 'info');
            } catch(err) {
              if ((window as any).showToast) (window as any).showToast(`Failed to pause worker daemon for task #${task.key}`, 'error');
            }
          });
        }

        if (resumeBtn) {
          resumeBtn.addEventListener('click', async (e) => {
            e.stopPropagation();
            try {
              await fetch('/api/swarm/resume/' + task.key, { method: 'POST' });
              if ((window as any).showToast) (window as any).showToast(`Resumed worker daemon for task #${task.key}`, 'success');
            } catch(err) {
              if ((window as any).showToast) (window as any).showToast(`Failed to resume worker daemon for task #${task.key}`, 'error');
            }
          });
        }

        if (killBtn) {
          killBtn.addEventListener('click', async (e) => {
            e.stopPropagation();
            if (!confirm(`Are you sure you want to terminate worker daemon for task #${task.key}?`)) return;
            try {
              await fetch('/api/swarm/kill/' + task.key, { method: 'POST' });
              if ((window as any).showToast) (window as any).showToast(`Terminated worker daemon for task #${task.key}`, 'warning');
            } catch(err) {
              if ((window as any).showToast) (window as any).showToast(`Failed to terminate worker daemon for task #${task.key}`, 'error');
            }
          });
        }

        if (isTaskRunning || activeAgentSlot) {
            terminal.innerHTML = `<span style="color: var(--neon-green)">[Swarm Backend]</span> Agent active and connected to worktree...`;
        }

        diffBtn.addEventListener('click', async (e) => {
          e.stopPropagation();
          diffBtn.textContent = 'Fetching from Backend...';
          try {
             const res = await fetch('/api/swarm/diff/' + task.key);
             const data = await res.json();
             diffContent.textContent = (data.status || '') + '\n\n' + (data.diff || '');
             diffContent.style.display = 'block';
             diffBtn.style.display = 'none';
          } catch(err) {
             diffContent.textContent = 'Failed to load diff.';
             diffContent.style.display = 'block';
          }
        });

        let isExpanded = false;
        let wsLogHandler: ((frame: any) => void) | null = null;

        mwBtn.addEventListener('click', (e) => {
          e.stopPropagation();
          isExpanded = !isExpanded;
          mwContainer.style.display = isExpanded ? 'flex' : 'none';
          mwBtn.textContent = isExpanded ? '🌌 Collapse Micro World' : '🌌 Expand Micro World';

          if (isExpanded) {
            if (!wsLogHandler) {
              wsLogHandler = (frame: any) => {
                if (!isExpanded || !frame) return;
                const isLogFrame = frame.type === 'log' || frame.log || frame.line || frame.msg;
                if (!isLogFrame) return;

                const frameTask = frame.task_id || frame.taskId || (frame.log && frame.log.task_id);
                if (frameTask && String(frameTask) !== String(task.key)) return;

                const logText = frame.line || frame.msg || (typeof frame.log === 'string' ? frame.log : (frame.log ? frame.log.msg : ''));
                if (!logText) return;

                const lineDiv = document.createElement('div');
                lineDiv.className = 'mw-log-line';
                lineDiv.style.fontFamily = "'JetBrains Mono', monospace";
                lineDiv.style.fontSize = '0.65rem';
                lineDiv.style.lineHeight = '1.3';
                lineDiv.style.whiteSpace = 'pre-wrap';
                const spans = parseAnsiLine(logText);
                spans.forEach(sp => lineDiv.appendChild(sp));
                terminal.appendChild(lineDiv);

                while (terminal.children.length > 50) {
                  terminal.removeChild(terminal.firstChild!);
                }
                terminal.scrollTop = terminal.scrollHeight;
              };
              addWSListener(wsLogHandler);
            }
          } else {
            if (wsLogHandler) {
              removeWSListener(wsLogHandler);
              wsLogHandler = null;
            }
          }
        });

        card.appendChild(mwBtn);
        card.appendChild(mwContainer);
      }
    }
    const isClosedTask = ['closed', 'done', 'completed', 'cancelled', 'canceled', 'parked', 'rejected'].includes(statusLower);
    const isActiveStatus = ['plan', 'edit', 'review', 'in_progress', 'in-progress', 'in progress', 'in_review', 'validate'].includes(statusLower);
    const assignee = (task as any).assignee || '';
    const isAssignedToSwarm = assignee.startsWith('swarm:') || assignee === 'opencode' || assignee === 'swarm-opencode' || (task as any).tier === 'T2' || (task as any).tier === '2';
    const isAssignedToIDE = !isAssignedToSwarm;

    const isTier2 = !isClosedTask && !isCommunityMode && (isAssignedToSwarm || isTaskRunning || activeAgentSlot);
    const isTier1 = !isClosedTask && !isTier2 && (isMainWorkspaceActive || isTaskIdActive(task.key, (window as any).ideActiveTaskId || activeTaskId) || (isAssignedToIDE && isActiveStatus));
    if (isTier1 || isTier2) {
      if (isTier1) {
        if (tier1Container) tier1Container.appendChild(card);
      } else {
        if (tier2Container) tier2Container.appendChild(card);
      }
    } else {
      const isClosedTask = statusLower === 'closed' || statusLower === 'done' || statusLower === 'completed' || statusLower === 'cancelled' || statusLower === 'canceled';
      if (activeStatusFilter === 'OPEN' && isClosedTask) return { rendered: false, isClosed };
      if (activeStatusFilter === 'CLOSED' && !isClosedTask) return { rendered: false, isClosed };
      
      if (activeLabelFilters.size > 0) {
        if (!task.labels) return { rendered: false, isClosed };
        const tLabels = task.labels.map((l: string) => l.toLowerCase());
        let hasAll = true;
        for (const req of activeLabelFilters) {
          if (!tLabels.includes(req)) {
            hasAll = false; break;
          }
        }
        if (!hasAll) return { rendered: false, isClosed };
      }

      const tr = document.createElement('tr');
      tr.style.borderBottom = '1px solid rgba(255, 255, 255, 0.05)';
      tr.style.cursor = 'pointer';
      tr.addEventListener('mouseenter', () => tr.style.background = 'rgba(255, 255, 255, 0.02)');
      tr.addEventListener('mouseleave', () => tr.style.background = 'transparent');
      tr.addEventListener('click', (e) => handleCardClick(e, task));

      const sizeStr = task.points ? String(task.points) : '-';
      
      let labelsHtml = '';
      if (task.labels && task.labels.length > 0) {
        labelsHtml = task.labels.map((lbl: string) => {
          let bg = 'rgba(255, 255, 255, 0.1)';
          let color = '#ccc';
          if (lbl.startsWith('tier:1') || lbl.includes('antigravity')) {
            bg = 'rgba(0, 240, 255, 0.15)'; color = 'var(--neon-cyan)';
          } else if (lbl.startsWith('tier:2') || lbl.includes('aider')) {
            bg = 'rgba(239, 68, 68, 0.15)'; color = 'var(--neon-red)';
          } else if (lbl.startsWith('priority:high') || lbl.startsWith('priority:critical')) {
            bg = 'rgba(255, 107, 107, 0.2)'; color = '#ff6b6b';
          } else if (lbl.startsWith('priority:medium')) {
            bg = 'rgba(252, 211, 77, 0.2)'; color = '#fcd34d';
          }
          return `<span style="font-size: 0.6rem; padding: 2px 4px; border-radius: 3px; background: ${bg}; color: ${color}; font-family: monospace; margin-right: 4px; white-space: nowrap; display: inline-block; margin-bottom: 2px;">${lbl.toUpperCase()}</span>`;
        }).join('');
      }
      
      const tAny = task as any;
      const rawTime = tAny.updated_at || tAny.updated || tAny.created_at || tAny.created;
      const updatedDisplay = formatRelativeTime(rawTime);

      tr.innerHTML = `
        <td style="padding: 10px 8px; color: var(--neon-purple); font-family: monospace;">#${task.key}</td>
        <td style="padding: 10px 8px;">${task.title}</td>
        <td style="padding: 10px 8px;">${labelsHtml}</td>
        <td style="padding: 10px 8px;">
          <span style="font-size: 0.65rem; background: rgba(59, 130, 246, 0.1); color: var(--neon-blue); padding: 2px 6px; border-radius: 4px;">${task.project || 'unassigned'}</span>
        </td>
        <td style="padding: 10px 8px;">${sizeStr}</td>
        <td style="padding: 10px 8px;">
          <span style="font-size: 0.65rem; background: rgba(255, 255, 255, 0.1); padding: 2px 6px; border-radius: 4px;">${task.status || 'BACKLOG'}</span>
        </td>
        <td style="padding: 10px 8px; font-size: 0.7rem; color: var(--text-muted); font-family: monospace; white-space: nowrap;">${updatedDisplay}</td>
      `;
      tbody.appendChild(tr);
    }
    return { rendered: true, isClosed };
}

export function renderKanbanBoard(tasks: TaskCard[], activeTaskId: string, currentPhase: string): void {
  const searchInput = document.getElementById('kanban-search') as HTMLInputElement | null;
  const globalSearchQuery = searchInput ? searchInput.value.toLowerCase() : '';

  const activeSwarms = (window as any).activeSwarmsList || [];
  const mergedList = (window as any).mergedBranchesList || [];
  const currentStateStr = JSON.stringify({ tasks, activeTaskId, currentPhase, globalSearchQuery, activeSwarms, mergedList });
  if (currentStateStr === lastRenderedStateStr) {
    return;
  }
  lastRenderedStateStr = currentStateStr;

  const ledgerContainer = document.getElementById('backlog-ledger-container');
  const tier1Container = document.getElementById('tier1-agent-container');
  const tier2Container = document.getElementById('tier2-agent-container');
  if (ledgerContainer) ledgerContainer.replaceChildren();
  if (tier1Container) tier1Container.replaceChildren();
  if (tier2Container) tier2Container.replaceChildren();

  const ledgerTable = document.createElement('table');
  ledgerTable.style.width = '100%';
  ledgerTable.style.borderCollapse = 'collapse';
  ledgerTable.style.fontSize = '0.85rem';
  ledgerTable.style.color = 'var(--text-main)';

  const thHtml = `
    <thead>
      <tr style="border-bottom: 2px solid var(--border-indigo); text-align: left; color: var(--text-muted); font-size: 0.7rem; text-transform: uppercase;">
        <th style="padding: 8px;">ID</th>
        <th style="padding: 8px;">Description</th>
        <th style="padding: 8px;">Labels</th>
        <th style="padding: 8px;">Repo</th>
        <th style="padding: 8px;">Size</th>
        <th style="padding: 8px;">Status</th>
        <th style="padding: 8px;">Updated</th>
      </tr>
    </thead>
  `;
  ledgerTable.innerHTML = thHtml;
  const tbody = document.createElement('tbody');

  initKanbanFilters();

  let openCount = 0;
  let closedCount = 0;
  let allLabels = new Set<string>();

  tasks.forEach((task) => {
    if (task.labels) task.labels.forEach(l => allLabels.add(l.toLowerCase()));
  });

  const sortedTasks = [...tasks].sort((a: any, b: any) => {
    const timeA = new Date(a.updated_at || a.updated || a.created_at || a.created || 0).getTime() || (parseInt(String(a.key).replace(/\D/g, ''), 10) || 0);
    const timeB = new Date(b.updated_at || b.updated || b.created_at || b.created || 0).getTime() || (parseInt(String(b.key).replace(/\D/g, ''), 10) || 0);
    return timeB - timeA;
  });

  sortedTasks.forEach((task) => {
    // Count ALL tasks that match the project filter (ignore status filter for metrics pills)
    let matchesProject = true;
    const kanbanProjectSelector = document.getElementById('project-selector') as HTMLSelectElement | null;
    const isCommunityMode = (window as any).latestStatus && (window as any).latestStatus.edition === 'community';
    if (kanbanProjectSelector && kanbanProjectSelector.value !== 'ALL' && !isCommunityMode) {
      if (task.project && task.project.toLowerCase() !== kanbanProjectSelector.value.toLowerCase()) {
         matchesProject = false;
      }
    }
    
    if (matchesProject) {
        const statusLower = (task.status || '').toLowerCase();
        const isClosedTask = statusLower === 'closed' || statusLower === 'done' || statusLower === 'completed' || statusLower === 'cancelled' || statusLower === 'canceled';
        if (isClosedTask) closedCount++; else openCount++;
    }

    const res = processKanbanTask(
      task, currentPhase, activeTaskId, activeKanbanTags, globalSearchQuery,
      activeKanbanOperator, activeLabelFilters, activeStatusFilter,
      tier1Container, tier2Container, tbody
    );
  });

  const countOpenEl = document.getElementById('count-open');
  if (countOpenEl) countOpenEl.textContent = String(openCount);
  const countClosedEl = document.getElementById('count-closed');
  if (countClosedEl) countClosedEl.textContent = String(closedCount);

  if (ledgerContainer) {
    ledgerTable.appendChild(tbody);
    ledgerContainer.appendChild(ledgerTable);
  }

  // Set card count numbers omitted for HUD layout

  // Render Ultimate Task Matrix
  renderTaskMatrix(tasks, activeTaskId, currentPhase, '', globalSearchQuery, activeSwarms);
  
  // Render Epic Gantt Chart
  renderGanttChart(tasks, '');
}

function formatRelativeTime(tsRaw: any): string {
  if (!tsRaw) return '-';
  const ts = new Date(tsRaw).getTime();
  if (isNaN(ts) || ts <= 0) return '-';
  const diffMs = Date.now() - ts;
  const mins = Math.floor(diffMs / 60000);
  if (mins < 1) return 'just now';
  if (mins < 60) return `${mins}m ago`;
  const hours = Math.floor(mins / 60);
  if (hours < 24) return `${hours}h ago`;
  const days = Math.floor(hours / 24);
  if (days < 30) return `${days}d ago`;
  return new Date(ts).toLocaleDateString();
}


function processMatrixTask(
  task: any,
  currentPhase: string,
  activeTaskId: string,
  activeFilter: string,
  globalSearchQuery: string,
  activeSwarms: any[],
  lastParentKey: string,
  matrixList: HTMLElement
): { rendered: boolean, newLastParentKey: string } {

    const statusLower = (task.status || '').toLowerCase().trim();
    const isClosed = statusLower === 'closed' || statusLower === 'done' || statusLower === 'completed' || statusLower === 'cancelled' || statusLower === 'canceled';
    if (activeFilter === 'OPEN' && isClosed) return { rendered: false, newLastParentKey: lastParentKey };
    if (activeFilter === 'DONE' && !isClosed) return { rendered: false, newLastParentKey: lastParentKey };

    if (globalSearchQuery) {
      const matchText = `#${task.key} ${task.key} ${task.title} ${task.body || ''} ${task.description || ''} ${task.labels ? task.labels.join(' ') : ''}`.toLowerCase();
      if (!matchText.includes(globalSearchQuery)) return { rendered: false, newLastParentKey: lastParentKey };
    }



    const latestSwarmData = (window as any).latestSwarmData || { nodes: [] };
    const isTaskRunning = Array.isArray(latestSwarmData.nodes) && latestSwarmData.nodes.some((n: any) => n.type === 'worker' && String(n.taskID) === String(task.key));
    const matchingWorktree = activeSwarms.find((sw: any) => sw.name === `task-${task.key}` || sw.name === String(task.key));

    const column = resolveTaskColumn(task, currentPhase, activeTaskId, isTaskRunning, matchingWorktree);

    const pKey = task.parent_key || (task.type?.toLowerCase() === 'epic' ? task.key : 'unbundled');
    
    if (pKey !== lastParentKey) {
       const header = document.createElement('div');
       header.style.padding = '8px 12px';
       header.style.background = 'rgba(139, 92, 246, 0.1)';
       header.style.borderLeft = '3px solid var(--neon-purple)';
       header.style.color = 'var(--neon-purple)';
       header.style.fontSize = '0.75rem';
       header.style.fontWeight = 'bold';
       header.style.textTransform = 'uppercase';
       header.style.letterSpacing = '0.05em';
       header.style.marginTop = matrixList.children.length > 0 ? '12px' : '0';
       header.style.display = 'flex';
       header.style.alignItems = 'center';
       header.style.gap = '8px';
       
       const icon = document.createElement('span');
       icon.textContent = pKey === 'unbundled' ? '📦' : '🔗';
       header.appendChild(icon);
       
       const title = document.createElement('span');
       title.textContent = pKey === 'unbundled' ? 'Unbundled Tasks' : `Bundle / Epic #${pKey}`;
       header.appendChild(title);
       
       matrixList.appendChild(header);
       lastParentKey = pKey;
    }

    const row = document.createElement('div');
    row.style.display = 'flex';
    row.style.alignItems = 'center';
    row.style.justifyContent = 'space-between';
    row.style.background = 'rgba(255, 255, 255, 0.03)';
    row.style.border = '1px solid rgba(255, 255, 255, 0.05)';
    row.style.borderRadius = '6px';
    row.style.padding = '8px 12px';
    row.style.cursor = 'pointer';

    if (isTaskIdActive(task.key, activeTaskId)) {
      row.style.border = '1px solid var(--neon-cyan)';
      row.style.boxShadow = '0 0 10px rgba(0, 240, 255, 0.2)';
    }

    row.addEventListener('click', (e) => handleCardClick(e, task));

    const left = document.createElement('div');
    left.style.display = 'flex';
    left.style.alignItems = 'center';
    left.style.gap = '12px';

    const colBadge = document.createElement('span');
    colBadge.style.fontSize = '0.65rem';
    colBadge.style.fontWeight = 'bold';
    colBadge.style.padding = '2px 6px';
    colBadge.style.borderRadius = '4px';
    colBadge.style.background = 'rgba(255,255,255,0.1)';
    colBadge.textContent = column;
    left.appendChild(colBadge);

    const idBadge = document.createElement('span');
    idBadge.style.color = 'var(--neon-purple)';
    idBadge.style.fontFamily = 'monospace';
    idBadge.style.fontSize = '0.75rem';
    idBadge.textContent = `#${task.key}`;
    left.appendChild(idBadge);
    
    let activeAgentSlot = null;
    const swarmSlotsData = (window as any).swarmSlotsData;
    if (swarmSlotsData && swarmSlotsData.slotStates) {
      activeAgentSlot = swarmSlotsData.slotStates.find((s: any) => s.is_locked && String(s.lock_task_id) === String(task.key));
    }
    
    if (activeAgentSlot) {
      const avatarOrb = document.createElement('div');
      avatarOrb.style.width = '16px';
      avatarOrb.style.height = '16px';
      avatarOrb.style.borderRadius = '50%';
      avatarOrb.className = 'pulse-glow';
      avatarOrb.style.background = 'var(--neon-green)';
      avatarOrb.style.boxShadow = '0 0 8px var(--neon-green)';
      avatarOrb.style.border = '1px solid rgba(255,255,255,0.8)';
      avatarOrb.style.display = 'flex';
      avatarOrb.style.alignItems = 'center';
      avatarOrb.style.justifyContent = 'center';
      avatarOrb.style.fontSize = '0.45rem';
      avatarOrb.style.color = 'var(--text-main)';
      avatarOrb.style.fontWeight = 'bold';
      avatarOrb.title = `Agent: ${activeAgentSlot.lock_owner || `A${activeAgentSlot.index}`}`;
      avatarOrb.textContent = `A${activeAgentSlot.index}`;
      left.appendChild(avatarOrb);
    }

    const titleSpan = document.createElement('span');
    titleSpan.style.fontSize = '0.85rem';
    titleSpan.style.whiteSpace = 'nowrap';
    titleSpan.style.overflow = 'hidden';
    titleSpan.style.textOverflow = 'ellipsis';
    titleSpan.style.maxWidth = '250px';
    titleSpan.textContent = task.title;
    left.appendChild(titleSpan);

    const right = document.createElement('div');
    right.style.display = 'flex';
    right.style.alignItems = 'center';
    right.style.gap = '8px';

    if (task.labels) {
      task.labels.forEach(lbl => {
        if (lbl.startsWith('tier:') || lbl.startsWith('agent:') || lbl.startsWith('cli:') || lbl.startsWith('priority:')) {
          const badge = document.createElement('span');
          badge.style.fontSize = '0.6rem';
          badge.style.padding = '2px 4px';
          badge.style.borderRadius = '3px';
          
          if (lbl.startsWith('tier:1') || lbl.includes('antigravity')) {
            badge.style.background = 'rgba(0, 240, 255, 0.15)';
            badge.style.color = 'var(--neon-cyan)';
          } else if (lbl.startsWith('tier:2') || lbl.includes('aider')) {
            badge.style.background = 'rgba(239, 68, 68, 0.15)';
            badge.style.color = 'var(--neon-red)';
          } else if (lbl.startsWith('priority:high') || lbl.startsWith('priority:critical')) {
            badge.style.background = 'rgba(255, 107, 107, 0.2)';
            badge.style.color = '#ff6b6b';
          } else if (lbl.startsWith('priority:medium')) {
            badge.style.background = 'rgba(252, 211, 77, 0.2)';
            badge.style.color = '#fcd34d';
          } else {
            badge.style.background = 'rgba(255, 255, 255, 0.1)';
            badge.style.color = '#ccc';
          }
          
          badge.style.fontFamily = 'monospace';
          badge.textContent = lbl.toUpperCase();
          right.appendChild(badge);
        }
      });
    }

    if (task.assignee && task.assignee.login) {
      const assigneeBadge = document.createElement('span');
      assigneeBadge.style.fontSize = '0.6rem';
      assigneeBadge.style.color = 'var(--text-muted)';
      assigneeBadge.style.fontFamily = 'monospace';
      assigneeBadge.style.background = 'rgba(255, 255, 255, 0.05)';
      assigneeBadge.style.padding = '2px 4px';
      assigneeBadge.style.borderRadius = '3px';
      assigneeBadge.textContent = `@${task.assignee.login}`;
      right.appendChild(assigneeBadge);
    }

    if (isTaskRunning) {
      const runBadge = document.createElement('span');
      runBadge.style.fontSize = '0.6rem';
      runBadge.style.color = 'var(--neon-yellow)';
      runBadge.textContent = '⚡ RUNNING';
      right.appendChild(runBadge);
    }

    const detailsBtn = document.createElement('button');
    detailsBtn.textContent = '📋';
    detailsBtn.style.background = 'none';
    detailsBtn.style.border = 'none';
    detailsBtn.style.cursor = 'pointer';
    detailsBtn.addEventListener('click', (e) => {
      e.stopPropagation();
      if ((window as any).openTaskDetailsDrawer) {
        (window as any).openTaskDetailsDrawer(task.key);
      }
    });
    right.appendChild(detailsBtn);

    row.appendChild(left);
    row.appendChild(right);
    matrixList.appendChild(row);
    return { rendered: true, newLastParentKey: lastParentKey };
}

function renderTaskMatrix(
  tasks: TaskCard[],
  activeTaskId: string,
  currentPhase: string,
  activeFilter: string,
  globalSearchQuery: string,
  activeSwarms: any[]
): void {
  const container = document.getElementById('ultimate-matrix-container');
  if (!container) return;
  container.replaceChildren();

  const matrixList = document.createElement('div');
  matrixList.style.display = 'flex';
  matrixList.style.flexDirection = 'column';
  matrixList.style.gap = '8px';
  matrixList.style.padding = '12px';

  const sortType = (document.getElementById('matrix-sort') as HTMLSelectElement)?.value || 'DEFAULT';

  const getPriorityValue = (t: TaskCard) => {
    const lbls = t.labels || [];
    if (lbls.includes('priority:critical')) return 4;
    if (lbls.includes('priority:high')) return 3;
    if (lbls.includes('priority:medium')) return 2;
    if (lbls.includes('priority:low')) return 1;
    return 0;
  };

  const sortedTasks = [...tasks].sort((a, b) => {
    if (sortType === 'PRIORITY') {
      const pA = getPriorityValue(a);
      const pB = getPriorityValue(b);
      return pB - pA || Number(a.key) - Number(b.key);
    }
    if (sortType === 'POINTS') {
      const ptA = typeof a.points !== 'undefined' ? Number(a.points) : 0;
      const ptB = typeof b.points !== 'undefined' ? Number(b.points) : 0;
      return ptB - ptA || Number(a.key) - Number(b.key);
    }
    const pA = a.parent_key || (a.type?.toLowerCase() === 'epic' ? a.key : 'zzz');
    const pB = b.parent_key || (b.type?.toLowerCase() === 'epic' ? b.key : 'zzz');
    return pA.localeCompare(pB) || Number(a.key) - Number(b.key);
  });

  let lastParentKey = '';

  sortedTasks.forEach((task) => {
    const res = processMatrixTask(
      task, currentPhase, activeTaskId, activeFilter, globalSearchQuery, activeSwarms, lastParentKey, matrixList
    );
    lastParentKey = res.newLastParentKey;
  });

  container.appendChild(matrixList);
}

function showDropZones(currentColumn: string): void {
  hideDropZones();
  if (currentColumn === 'BACKLOG' || currentColumn === 'TRIAGE') {
    const el = document.getElementById('action-dispatch');
    if (el) el.classList.add('active');
  } else if (currentColumn === 'IN_PROGRESS') {
    const resetEl = document.getElementById('action-reset');
    if (resetEl) resetEl.classList.add('active');
    
    ['action-approve', 'action-submit-review', 'action-refine', 'action-release'].forEach(id => {
       const el = document.getElementById(id);
       if (el) el.classList.add('active');
    });
  }
}

function hideDropZones(): void {
  const elements = document.querySelectorAll('.drop-zone-action');
  elements.forEach((el) => {
    el.classList.remove('active');
  });
}

function renderGanttChart(tasks: TaskCard[], activeFilter: string): void {
  const container = document.getElementById('roadmap-gantt-container');
  if (!container) return;
  container.replaceChildren();

  // Find all epics
  const epics = tasks.filter(t => t.type?.toLowerCase() === 'epic');
  if (epics.length === 0) {
    container.innerHTML = '<div style="color: var(--text-muted); font-family: monospace; display: flex; align-items: center; justify-content: center; height: 100%;">No Epics found to render on the Roadmap.</div>';
    return;
  }

  const timelineContainer = document.createElement('div');
  timelineContainer.style.display = 'flex';
  timelineContainer.style.flexDirection = 'column';
  timelineContainer.style.gap = '16px';
  timelineContainer.style.position = 'relative';
  timelineContainer.style.paddingTop = '30px'; // Space for timeline header

  // Simple hardcoded timeline scale for visual representation (in weeks)
  const timelineHeader = document.createElement('div');
  timelineHeader.style.position = 'absolute';
  timelineHeader.style.top = '0';
  timelineHeader.style.left = '200px'; // offset for labels
  timelineHeader.style.right = '0';
  timelineHeader.style.display = 'flex';
  timelineHeader.style.borderBottom = '1px solid rgba(255,255,255,0.1)';
  timelineHeader.style.paddingBottom = '4px';

  for (let i = 1; i <= 8; i++) {
    const tick = document.createElement('div');
    tick.style.flex = '1';
    tick.style.fontSize = '0.7rem';
    tick.style.color = 'var(--text-muted)';
    tick.textContent = `Sprint ${i}`;
    timelineHeader.appendChild(tick);
  }
  timelineContainer.appendChild(timelineHeader);

  // Layout Epics
  epics.forEach((epic, idx) => {
    // Status check
    const statusLower = (epic.status || '').toLowerCase().trim();
    const isClosed = statusLower === 'closed' || statusLower === 'done' || statusLower === 'completed' || statusLower === 'cancelled' || statusLower === 'canceled';
    if (activeFilter === 'OPEN' && isClosed) return;
    if (activeFilter === 'DONE' && !isClosed) return;

    const row = document.createElement('div');
    row.style.display = 'flex';
    row.style.alignItems = 'center';
    row.style.position = 'relative';

    // Label
    const label = document.createElement('div');
    label.style.width = '190px';
    label.style.paddingRight = '10px';
    label.style.fontSize = '0.8rem';
    label.style.fontWeight = 'bold';
    label.style.whiteSpace = 'nowrap';
    label.style.overflow = 'hidden';
    label.style.textOverflow = 'ellipsis';
    label.style.color = isClosed ? 'var(--text-muted)' : 'var(--text-main)';
    label.textContent = `#${epic.key} ${epic.title}`;
    row.appendChild(label);

    // Timeline Bar Area
    const barArea = document.createElement('div');
    barArea.style.flex = '1';
    barArea.style.display = 'flex';
    barArea.style.position = 'relative';
    barArea.style.height = '30px';
    barArea.style.background = 'rgba(255,255,255,0.02)';
    barArea.style.borderRadius = '4px';

    // Calculate a fake width / offset for the POC (in a real Gantt, we'd use estimated_duration)
    const durationStr = epic.estimated_duration || '1w'; // fallback
    const weeks = parseInt(durationStr) || 1;
    
    // Create actual Bar
    const bar = document.createElement('div');
    bar.style.position = 'absolute';
    // Dummy offsets for visual staging: cascade them
    const leftOffset = (idx * 10) % 60;
    const widthPct = Math.min((weeks / 8) * 100, 100 - leftOffset);
    
    bar.style.left = `${leftOffset}%`;
    bar.style.width = `${widthPct}%`;
    bar.style.height = '100%';
    bar.style.borderRadius = '4px';
    bar.style.background = isClosed ? 'rgba(16, 185, 129, 0.4)' : 'linear-gradient(90deg, rgba(139,92,246,0.6) 0%, rgba(0,240,255,0.6) 100%)';
    bar.style.border = isClosed ? '1px solid var(--neon-green)' : '1px solid var(--neon-purple)';
    bar.style.boxShadow = isClosed ? 'none' : '0 0 10px rgba(139, 92, 246, 0.2)';
    bar.style.display = 'flex';
    bar.style.alignItems = 'center';
    bar.style.padding = '0 8px';
    bar.style.fontSize = '0.7rem';
    bar.style.color = 'var(--text-main)';
    bar.style.textShadow = '0 1px 2px rgba(0,0,0,0.8)';
    
    const childTasks = tasks.filter(t => t.parent_key === epic.key);
    bar.textContent = `${childTasks.length} stories`;

    barArea.appendChild(bar);
    row.appendChild(barArea);

    timelineContainer.appendChild(row);
  });

  container.appendChild(timelineContainer);
}

function setupDragAndDrop(): void {
  const columns = document.querySelectorAll('.kanban-column');
  columns.forEach((col) => {
    col.addEventListener('dragover', (e: Event) => {
      const targetColumn = col.querySelector('.card-list')?.id.split('-')[1].toUpperCase();
      if (targetColumn && isTransitionValid(draggedCardSourceColumn, targetColumn)) {
        e.preventDefault();
        col.classList.add('drag-over');
      }
    });

    col.addEventListener('dragleave', () => {
      col.classList.remove('drag-over');
    });

    col.addEventListener('drop', async (e: Event) => {
      e.preventDefault();
      e.stopPropagation();
      col.classList.remove('drag-over');

      const dragEvent = e as DragEvent;
      const id = dragEvent.dataTransfer ? dragEvent.dataTransfer.getData('text/plain') : '';
      const targetColumn = col.querySelector('.card-list')?.id.split('-')[1].toUpperCase();

      if (id && targetColumn) {
        if (!isTransitionValid(draggedCardSourceColumn, targetColumn)) {
          return;
        }

        let action = '';
        if (targetColumn === 'BACKLOG') action = 'reset';
        else if (targetColumn === 'PLAN') action = 'dispatch';
        else if (targetColumn === 'EDIT') {
          const cardEl = document.querySelector(`.task-card[data-id="${id}"]`);
          const isFromReview = cardEl?.parentElement?.id === 'list-review';
          action = isFromReview ? 'refine' : 'approve';
        } else if (targetColumn === 'REVIEW') {
          action = 'review';
        }

        if (action) {
          handleActionDrop(id, action);
        }
      }
    });
  });

  const actionZones = document.querySelectorAll('.drop-zone-action');
  actionZones.forEach((zone) => {
    zone.addEventListener('dragover', (e: Event) => {
      e.preventDefault();
      e.stopPropagation();
      zone.classList.add('drag-over');
    });

    zone.addEventListener('dragleave', (e: Event) => {
      e.stopPropagation();
      zone.classList.remove('drag-over');
    });

    zone.addEventListener('drop', async (e: Event) => {
      e.preventDefault();
      e.stopPropagation();
      zone.classList.remove('drag-over');

      const dragEvent = e as DragEvent;
      const id = dragEvent.dataTransfer ? dragEvent.dataTransfer.getData('text/plain') : '';
      const action = zone.getAttribute('data-action');

      if (id && action) {
        handleActionDrop(id, action);
      }
      hideDropZones();
    });
  });
}

let actionInProgress = false;

async function handleActionDrop(id: string, action: string): Promise<void> {
  if (actionInProgress) return;
  actionInProgress = true;

  if (action === 'dispatch') {
    actionInProgress = false;
    if ((window as any).openDispatchModalWithTask) {
      (window as any).openDispatchModalWithTask(id);
    }
  } else if (action === 'approve') {
    try {
      const res = await fetch('/api/phase', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ phase: 'EDIT' }),
      });
      const data = await res.json();
      if (data.success) {
        (window as any).showToast('Implementation Plan approved. Workspace unlocked in EDIT phase.', 'success');
        (window as any).refreshData();
      } else {
        (window as any).showToast(data.error || 'Failed to approve spec', 'error');
      }
    } catch (err) {
      console.error(err);
    } finally {
      actionInProgress = false;
    }
  } else if (action === 'review') {
    try {
      const res = await fetch('/api/phase', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ phase: 'REVIEW' }),
      });
      const data = await res.json();
      if (data.success) {
        (window as any).showToast('Workspace phase transitioned to REVIEW. Telemetry paused.', 'success');
        (window as any).refreshData();
      } else {
        (window as any).showToast(data.error || 'Failed to submit review', 'error');
      }
    } catch (err) {
      console.error(err);
    } finally {
      actionInProgress = false;
    }
  } else if (action === 'refine') {
    try {
      const res = await fetch(`/api/task/transition?id=${id}&column=EDIT`, { method: 'POST' });
      const data = await res.json();
      if (data.success) {
        (window as any).showToast('Task review rejected. Reopened for refinement.', 'warning');
        (window as any).refreshData();
      } else {
        (window as any).showToast(data.error || 'Failed to reopen task', 'error');
      }
    } catch (err) {
      console.error(err);
    } finally {
      actionInProgress = false;
    }
  } else if (action === 'release') {
    try {
      const res = await fetch(`/api/task/transition?id=${id}&column=DONE`, { method: 'POST' });
      const data = await res.json();
      if (data.success) {
        (window as any).showToast('Task successfully committed and closed!', 'success');
        (window as any).refreshData();
      } else {
        (window as any).showToast(data.error || 'Failed to close task', 'error');
      }
    } catch (err) {
      console.error(err);
    } finally {
      actionInProgress = false;
    }
  } else if (action === 'reset') {
    const confirmReset = confirm(
      `Are you sure you want to reset task #${id}? This will revert all local changes, branches, stashes, and return the task to the backlog.`
    );
    if (confirmReset) {
      try {
        (window as any).showToast(`Resetting task #${id}...`, 'info');
        const res = await fetch(`/api/task/reset?id=${id}`, { method: 'POST' });
        const data = await res.json();
        if (data.success) {
          (window as any).showToast(`Task #${id} successfully reset to backlog.`, 'success');
          (window as any).refreshData();
        } else {
          (window as any).showToast(data.error || 'Failed to reset task', 'error');
        }
      } catch (err) {
        console.error(err);
        (window as any).showToast('Error resetting task', 'error');
      } finally {
        actionInProgress = false;
      }
    } else {
      actionInProgress = false;
    }
  }
}

function isTransitionValid(source: string, target: string): boolean {
  if (source === target) return true;
  if (target === 'BACKLOG') {
    return source === 'PLAN' || source === 'EDIT' || source === 'REVIEW';
  }
  if (source === 'BACKLOG') {
    return target === 'PLAN';
  }
  if (source === 'PLAN') {
    return target === 'EDIT' || target === 'BACKLOG';
  }
  if (source === 'EDIT') {
    return target === 'REVIEW' || target === 'BACKLOG';
  }
  if (source === 'REVIEW') {
    return target === 'EDIT' || target === 'BACKLOG';
  }
  return false;
}
