// modal.ts - Decoupled task details drawer and overlay popup managers


import { formatMarkdown, sanitizeHTMLString } from './artifacts.js';

let latestStatusGetter: () => any = () => null;

export function registerLatestStatusGetter(getter: () => any): void {
  latestStatusGetter = getter;
}

export function toggleModal(open: boolean): void {
  const overlay = document.getElementById('dispatch-overlay');
  if (overlay) overlay.classList.toggle('active', open);
  
  const latestStatus = latestStatusGetter();
  if (open && latestStatus && latestStatus.phaseState) {
    const taskInput = document.getElementById('form-task-id') as HTMLInputElement | null;
    if (taskInput && latestStatus.phaseState.task_id) {
      taskInput.value = latestStatus.phaseState.task_id;
    }
    const branchInput = document.getElementById('form-branch') as HTMLInputElement | null;
    if (branchInput && latestStatus.phaseState.active_branch) {
      branchInput.value = latestStatus.phaseState.active_branch;
    }
  }
}

export async function openDispatchModalWithTask(taskId: string): Promise<void> {
  const overlay = document.getElementById('dispatch-overlay');
  if (overlay) overlay.classList.add('active');
  
  const taskInput = document.getElementById('form-task-id') as HTMLInputElement | null;
  if (taskInput) {
    taskInput.value = taskId;
  }
  
  const branchInput = document.getElementById('form-branch') as HTMLInputElement | null;
  const instructionInput = document.getElementById('form-instruction') as HTMLTextAreaElement | null;
  
  if (branchInput) {
    branchInput.value = 'Loading...';
  }
  if (instructionInput) {
    instructionInput.value = 'Loading...';
  }
  
  try {
    const res = await fetch(`/api/backlog/${taskId}`);
    if (res.ok) {
      const task = await res.json();
      if (task) {
        if (branchInput) {
          const cleanId = taskId.replace('#', '');
          const kebabTitle = (task.title || '')
            .toLowerCase()
            .replace(/[^a-z0-9]+/g, '-')
            .replace(/^-|-$/g, '')
            .substring(0, 50)
            .replace(/-$/, '');
          branchInput.value = `task/${cleanId}-${kebabTitle}`;
        }
        if (instructionInput) {
          instructionInput.value = task.title || '';
        }
      }
    } else {
      if (branchInput) branchInput.value = '';
      if (instructionInput) instructionInput.value = '';
    }
  } catch {
    if (branchInput) branchInput.value = '';
    if (instructionInput) instructionInput.value = '';
  }
}

(window as any).openDispatchModalWithTask = openDispatchModalWithTask;


let activeViewingTaskId: number | null = null;
let pollingIntervalId: any = null;
let isFirstLoad = true;

function setSafeInnerHTML(element: HTMLElement, htmlString: string): void {
  const parser = new DOMParser();
  const doc = parser.parseFromString(htmlString, 'text/html');
  element.replaceChildren();
  while (doc.body.firstChild) {
    element.appendChild(doc.body.firstChild);
  }
}

async function fetchAndRenderTaskDetails(taskId: number): Promise<void> {
  const container = document.getElementById('task-details-drawer');
  if (!container) return;

  const contentEl = document.getElementById('drawer-main-content');
  const titleEl = document.getElementById('drawer-task-title');
  const headerId = document.getElementById('drawer-task-id');
  const assigneeEl = document.getElementById('drawer-assignee');
  const stateEl = document.getElementById('drawer-state');
  const phaseEl = document.getElementById('drawer-phase');
  const tierEl = document.getElementById('drawer-tier');

  if (activeViewingTaskId !== taskId) {
    if (contentEl) contentEl.innerHTML = '<div style="text-align:center; padding: 2rem; color: var(--text-muted);">Loading context...</div>';
    if (titleEl) titleEl.textContent = 'Loading...';
    if (headerId) headerId.textContent = `#${taskId}`;
    isFirstLoad = true;
    activeViewingTaskId = taskId;
  }

  try {
    const res = await fetch(`/api/tasks/${taskId}`);
    const data = await res.json();
    const task = data.task || data;
    if (!task || !task.key) {
      if (activeViewingTaskId === taskId) {
        if (contentEl) contentEl.innerHTML = '<div style="color:var(--neon-yellow); text-align:center; padding:2rem;">Task artifact lost in subspace (Not Found).</div>';
      }
      return;
    }

    if (activeViewingTaskId !== taskId) return;

    if (titleEl) titleEl.textContent = task.title || 'Untitled Substrate Task';
    
    if (assigneeEl) {
      assigneeEl.innerHTML = task.assignee && task.assignee.login 
        ? `<span style="color:var(--text-muted);">@</span>${task.assignee.login}` 
        : '<span style="color:var(--text-muted);">Unassigned</span>';
    }

    if (stateEl) {
      const st = (task.status || 'open').toLowerCase();
      stateEl.textContent = st.toUpperCase();
      if (st === 'closed' || st === 'done') stateEl.className = 'status-badge closed';
      else if (st === 'in progress') stateEl.className = 'status-badge progress';
      else stateEl.className = 'status-badge open';
    }

    if (tierEl) {
      const tierLabel = (task.labels || []).find((l: string) => l.startsWith('tier:'));
      tierEl.textContent = tierLabel ? tierLabel.toUpperCase() : 'TIER:?';
    }

    populateTaskDetailsDOM(task);

    const isAtBottom = container.scrollHeight - container.scrollTop <= container.clientHeight + 20;

    const descHtml = formatMarkdown(task.description || task.body || '*No description provided.*');
    const commentsHtml = buildModalCommentsHtml(task);

    const descContainer = document.getElementById('drawer-description-container');
    if (descContainer) {
      setSafeInnerHTML(descContainer, sanitizeHTMLString(`
        <div class="drawer-meta-label" style="margin-bottom: 0.5rem; text-transform: uppercase; font-size: 0.7rem; color: var(--text-muted); letter-spacing: 0.5px;">Description</div>
        ${descHtml}
      `));
    }

    if (contentEl) {
      setSafeInnerHTML(contentEl, sanitizeHTMLString(`
        ${commentsHtml}
      `));
    }

    if (isFirstLoad) {
      container.scrollTop = 0;
      isFirstLoad = false;
    } else if (isAtBottom) {
      container.scrollTop = container.scrollHeight;
    }

  } catch (err: any) {
    if (activeViewingTaskId === taskId) {
      if (contentEl) setSafeInnerHTML(contentEl, sanitizeHTMLString(`<div style="color:var(--neon-yellow); text-align:center; padding:2rem;">Error: ${err.message}</div>`));
    }
  }
}

function populateTaskDetailsDOM(task: any) {
  const drawerRepo = document.getElementById('drawer-repo');
  const drawerType = document.getElementById('drawer-type');
  const drawerParent = document.getElementById('drawer-parent');
  const drawerCycle = document.getElementById('drawer-cycle');
  const drawerSpike = document.getElementById('drawer-spike');
  const drawerDuration = document.getElementById('drawer-duration');
  const drawerCreated = document.getElementById('drawer-created');
  const drawerUpdated = document.getElementById('drawer-updated');
  const drawerBlocked = document.getElementById('drawer-blocked');
  const drawerTags = document.getElementById('drawer-tags');
  const drawerDor = document.getElementById('drawer-dor');
  const drawerDod = document.getElementById('drawer-dod');

  if (drawerRepo) drawerRepo.textContent = task.project || 'Unknown';
  if (drawerType) drawerType.textContent = task.type || 'Task';
  if (drawerParent) drawerParent.innerHTML = task.parent_key ? `<a href="#" onclick="openTaskDetailsDrawer('${task.parent_key}')">#${task.parent_key}</a>` : '--';
  if (drawerCycle) drawerCycle.textContent = task.cycle !== undefined ? String(task.cycle) : '--';
  if (drawerSpike) drawerSpike.textContent = task.is_spike ? 'Yes' : 'No';
  if (drawerDuration) drawerDuration.textContent = task.estimated_duration || '--';
  
  if (drawerCreated) drawerCreated.textContent = task.created_at ? new Date(task.created_at).toLocaleString() : '--';
  if (drawerUpdated) drawerUpdated.textContent = task.updated_at ? new Date(task.updated_at).toLocaleString() : '--';
  
  if (drawerBlocked) {
    if (task.blocked_by && task.blocked_by.length > 0) {
      drawerBlocked.innerHTML = task.blocked_by.map((b: string) => `<a href="#" onclick="(window as any).openTaskDetailsDrawer('${b}')" class="tag">#${b}</a>`).join(' ');
    } else {
      drawerBlocked.textContent = 'None';
    }
  }
  
  if (drawerTags) {
    drawerTags.innerHTML = '';
    if (task.labels && task.labels.length > 0) {
      task.labels.forEach((lbl: string) => {
        const t = document.createElement('span');
        t.className = 'tag';
        t.style.background = 'rgba(var(--glass-rgb), 0.1)';
        t.style.border = '1px solid var(--border-indigo)';
        t.style.color = 'var(--text-main)';
        t.textContent = lbl;
        drawerTags.appendChild(t);
      });
    } else {
      drawerTags.textContent = 'No tags';
    }
  }

  if (drawerDor) {
    const dorVal = task.dorStatus || 'Drafting';
    drawerDor.textContent = dorVal;
    drawerDor.className = 'tag';
    if (dorVal === 'Ready') {
      drawerDor.classList.add('success');
      drawerDor.style.background = 'rgba(16, 185, 129, 0.15)';
      drawerDor.style.color = 'var(--neon-green)';
      drawerDor.style.borderColor = 'rgba(16, 185, 129, 0.25)';
    } else if (dorVal === 'Pending Review') {
      drawerDor.style.background = 'rgba(245, 158, 11, 0.15)';
      drawerDor.style.color = 'var(--neon-yellow)';
      drawerDor.style.borderColor = 'rgba(245, 158, 11, 0.25)';
    } else {
      drawerDor.style.background = 'rgba(156, 163, 175, 0.15)';
      drawerDor.style.color = '#9ca3af';
      drawerDor.style.borderColor = 'rgba(156, 163, 175, 0.25)';
    }
  }

  if (drawerDod) {
    const dodVal = task.dodStatus || 'Pending';
    drawerDod.textContent = dodVal;
    drawerDod.className = 'tag';
    if (dodVal.includes('100%') || dodVal.includes('All Green')) {
      drawerDod.classList.add('success');
      drawerDod.style.background = 'rgba(16, 185, 129, 0.15)';
      drawerDod.style.color = 'var(--neon-green)';
      drawerDod.style.borderColor = 'rgba(16, 185, 129, 0.25)';
    } else if (dodVal.includes('Blocked')) {
      drawerDod.style.background = 'rgba(239, 68, 68, 0.15)';
      drawerDod.style.color = 'var(--neon-red)';
      drawerDod.style.borderColor = 'rgba(239, 68, 68, 0.25)';
    } else {
      drawerDod.classList.add('primary');
      drawerDod.style.background = 'rgba(59, 130, 246, 0.15)';
      drawerDod.style.color = 'var(--neon-blue)';
      drawerDod.style.borderColor = 'rgba(59, 130, 246, 0.25)';
    }
  }
}

function buildModalCommentsHtml(task: any): string {
  let commentsHtml = '';
  if (task.comments && task.comments.length > 0) {
    commentsHtml += `
      <div class="drawer-comments-section">
        <div class="drawer-comments-title-row">
          <span class="drawer-comments-title">Timeline & Comments</span>
          <span class="drawer-comments-count">${task.comments.length}</span>
        </div>
        <div class="drawer-timeline-container">
    `;

    task.comments.forEach((comment: any) => {
      let author = 'system';
      let formattedDate = '';
      let commentBodyText = '';

      if (typeof comment === 'string') {
        const match = comment.match(/^([^\s]+)\s+\([^)]+\):\s*([\s\S]*)$/);
        if (match) {
          author = match[1];
          formattedDate = match[2];
          commentBodyText = match[3];
        } else {
          commentBodyText = comment;
        }
      } else if (comment && typeof comment === 'object') {
        author = (typeof comment.author === 'string') ? comment.author : (comment.author?.login || 'system');
        const tDate = comment.created_at || comment.createdAt;
        formattedDate = tDate ? new Date(tDate).toLocaleString() : '';
        commentBodyText = comment.body || '';
      }

      // Strip ANSI escape codes
      commentBodyText = commentBodyText.replace(/\x1b\[[0-9;]*m/g, '');

      let commentBodyHtml = '';
      const markedObj = (window as any).marked;
      if (markedObj && typeof markedObj.parse === 'function') {
        try {
          commentBodyHtml = markedObj.parse(commentBodyText);
        } catch {
          commentBodyHtml = formatMarkdown(commentBodyText);
        }
      } else {
        commentBodyHtml = formatMarkdown(commentBodyText);
      }

      const isAgentStatus = commentBodyText && (
        commentBodyText.includes('Definition of Ready') || 
        commentBodyText.includes('Definition of Done') || 
        commentBodyText.includes('Agent Progress:') || 
        commentBodyText.includes('has successfully entered') ||
        commentBodyText.includes('Telemetry')
      );
      const commentClass = isAgentStatus ? 'drawer-comment-card agent-status' : 'drawer-comment-card';

      commentsHtml += `
        <div class="drawer-timeline-node">
          <div class="drawer-timeline-dot ${isAgentStatus ? 'agent' : ''}"></div>
          <div class="${commentClass}">
            <div class="drawer-comment-header">
              <span class="drawer-comment-author">@${author}</span>
              <span class="drawer-comment-time">${formattedDate}</span>
            </div>
            <div class="drawer-comment-body">
              ${commentBodyHtml}
            </div>
          </div>
        </div>
      `;
    });

    commentsHtml += `
        </div>
      </div>
    `;
  } else {
    commentsHtml += `
      <div class="drawer-comments-section">
        <div class="drawer-comments-title-row">
          <span class="drawer-comments-title">Timeline & Comments</span>
        </div>
        <div style="color: var(--text-muted); font-size: 0.8rem; padding: 1rem 0; text-align: center;">
          No comments or status updates yet.
        </div>
      </div>
    `;
  }
  return commentsHtml;
}

function buildTaskDetailsPaneHtml(task: any): string {
  const descHtml = formatMarkdown(task.description || task.body || '*No description provided.*');
  
  let commentsHtml = '';
  if (task.comments && task.comments.length > 0) {
    commentsHtml += `
      <div class="drawer-comments-section" style="margin-top: 1.5rem;">
        <div class="drawer-comments-title-row" style="margin-bottom: 0.75rem;">
          <span class="drawer-comments-title" style="font-weight: 700; color: var(--neon-cyan); text-transform: uppercase; font-size: 0.8rem;">Timeline & Discussion (${task.comments.length})</span>
        </div>
        <div class="drawer-timeline-container">
    `;
    task.comments.forEach((comment: any) => {
      let author = 'system';
      let formattedDate = '';
      let commentBodyText = '';
      if (typeof comment === 'string') {
        const match = comment.match(/^([^\s]+)\s+\(([^)]+)\):\s*([\s\S]*)$/);
        if (match) {
          author = match[1];
          formattedDate = match[2];
          commentBodyText = match[3];
        } else {
          commentBodyText = comment;
        }
      } else if (comment && typeof comment === 'object') {
        author = (typeof comment.author === 'string') ? comment.author : (comment.author?.login || 'system');
        const tDate = comment.created_at || comment.createdAt;
        formattedDate = tDate ? new Date(tDate).toLocaleString() : '';
        commentBodyText = comment.body || '';
      }
      commentBodyText = commentBodyText.replace(/\x1b\[[0-9;]*m/g, '');
      let commentBodyHtml = formatMarkdown(commentBodyText);
      const isAgentStatus = commentBodyText && (
        commentBodyText.includes('Definition of Ready') || 
        commentBodyText.includes('Definition of Done') || 
        commentBodyText.includes('Agent Progress:')
      );
      commentsHtml += `
        <div class="drawer-timeline-node" style="margin-bottom: 0.75rem;">
          <div class="drawer-comment-card ${isAgentStatus ? 'agent-status' : ''}" style="background: rgba(0,0,0,0.3); border: 1px solid var(--border-indigo); border-radius: 6px; padding: 0.75rem;">
            <div class="drawer-comment-header" style="display: flex; justify-content: space-between; font-size: 0.75rem; color: var(--text-muted); margin-bottom: 0.4rem;">
              <span class="drawer-comment-author" style="color: var(--neon-purple); font-weight: 600;">@${author}</span>
              <span class="drawer-comment-time">${formattedDate}</span>
            </div>
            <div class="drawer-comment-body" style="font-size: 0.85rem;">
              ${commentBodyHtml}
            </div>
          </div>
        </div>
      `;
    });
    commentsHtml += `</div></div>`;
  } else {
    commentsHtml = `
      <div style="margin-top: 1.5rem; color: var(--text-muted); font-size: 0.8rem; text-align: center; padding: 1rem; border: 1px dashed var(--border-indigo); border-radius: 6px;">
        No comments or status updates yet.
      </div>
    `;
  }

  const tagsHtml = (task.labels && task.labels.length > 0)
    ? task.labels.map((lbl: string) => `<span class="tag" style="background: rgba(var(--glass-rgb), 0.1); border: 1px solid var(--border-indigo); color: var(--text-main); font-size: 0.7rem; padding: 2px 6px; border-radius: 4px; margin-right: 4px;">${lbl}</span>`).join('')
    : '<span style="color: var(--text-muted); font-size: 0.8rem;">No tags</span>';

  const blastRadiusBadge = task.labels && task.labels.some((l: string) => l.includes('blast:high'))
    ? '<span style="background: rgba(239, 68, 68, 0.2); color: #fca5a5; border: 1px solid rgba(239, 68, 68, 0.4); font-size: 0.7rem; padding: 2px 6px; border-radius: 4px; font-weight: 700;">BLAST: HIGH</span>'
    : '<span style="background: rgba(34, 197, 94, 0.15); color: #86efac; border: 1px solid rgba(34, 197, 94, 0.3); font-size: 0.7rem; padding: 2px 6px; border-radius: 4px; font-weight: 700;">BLAST: LOW</span>';

  const parentLinkHtml = task.parent_key
    ? `<a href="#" onclick="openTaskDetailsDrawer('${task.parent_key}'); return false;" style="color: var(--neon-cyan); font-weight: 600; text-decoration: underline;">#${task.parent_key}</a>`
    : '<span style="color: var(--text-muted);">None (Root Task)</span>';

  return `
    <div style="color: var(--text-primary);">
      <!-- Task Header -->
      <div style="background: rgba(0, 240, 255, 0.05); border: 1px solid rgba(0, 240, 255, 0.2); padding: 1rem; border-radius: 8px; margin-bottom: 1.25rem;">
        <div style="display: flex; justify-content: space-between; align-items: center; margin-bottom: 0.5rem;">
          <span style="font-family: monospace; font-size: 0.9rem; font-weight: 700; color: var(--neon-cyan);">#${task.key || task.id}</span>
          <div style="display: flex; gap: 0.4rem; align-items: center;">
            ${blastRadiusBadge}
            <span style="background: rgba(16, 185, 129, 0.2); color: var(--neon-green); font-size: 0.75rem; padding: 0.2rem 0.5rem; border-radius: 4px; font-weight: 700;">${task.status || 'OPEN'}</span>
          </div>
        </div>
        <h3 style="margin: 0 0 0.5rem 0; font-size: 1.1rem; color: var(--text-main); line-height: 1.3;">${task.title || ''}</h3>
        <div style="display: flex; gap: 0.5rem; flex-wrap: wrap; align-items: center; font-size: 0.8rem; color: var(--text-muted);">
          <span><strong>Project:</strong> ${task.project || 'nomos'}</span>
          <span>•</span>
          <span><strong>Parent Epic:</strong> ${parentLinkHtml}</span>
          <span>•</span>
          <span><strong>Assignee:</strong> ${task.assignee ? `@${task.assignee}` : '@unassigned'}</span>
          <span>•</span>
          <span><strong>Burden:</strong> ${task.context_burden || 0}/5</span>
          <span>•</span>
          <span><strong>Depth:</strong> ${task.logic_depth || 0}/5</span>
        </div>
        <div style="margin-top: 0.6rem;">${tagsHtml}</div>
      </div>

      <!-- Scrollable Description Section (400px max height) -->
      <div style="background: rgba(0, 0, 0, 0.3); border: 1px solid var(--border-indigo); border-radius: 8px; padding: 1rem; margin-bottom: 1.25rem;">
        <div style="font-size: 0.7rem; font-weight: 700; text-transform: uppercase; color: var(--text-muted); letter-spacing: 0.5px; margin-bottom: 0.5rem; border-bottom: 1px dashed rgba(255,255,255,0.1); padding-bottom: 4px;">
          Task Description & Deliverables
        </div>
        <div style="max-height: 400px; overflow-y: auto; font-size: 0.88rem; line-height: 1.5;" class="scrollable-indigo">
          ${descHtml}
        </div>
      </div>

      <!-- Linked Commits & Code Impact -->
      <div style="background: rgba(0, 0, 0, 0.25); border: 1px solid var(--border-indigo); border-radius: 8px; padding: 1rem; margin-bottom: 1.25rem;">
        <div style="font-size: 0.7rem; font-weight: 700; text-transform: uppercase; color: var(--neon-cyan); letter-spacing: 0.5px; margin-bottom: 0.4rem;">
          🔀 Linked Commits & Code Impact
        </div>
        <div style="font-size: 0.82rem; color: var(--text-muted);">
          Task commits bound to branch <code style="color: var(--neon-purple); font-family: monospace;">task/${task.key || task.id}</code>
        </div>
      </div>

      <!-- Timeline & Comments -->
      ${commentsHtml}
    </div>
  `;
}

function buildAgentDetailsPaneHtml(task: any): string {
  // A task has an active agent if:
  // 1. Assignee is an actual agent (not a human PO like markg or unassigned)
  // 2. Engine is present and valid.
  const isAgentAssignee = task && typeof task.assignee === 'string' && 
    ['antigravity', 'swarm', 'opencode', 'agent'].some(name => task.assignee.toLowerCase().includes(name));
  const validEngine = task && typeof task.engine === 'string' && task.engine.trim() !== '' && task.engine !== 'none';

  // We no longer trigger hasAgent just because a task has a 'tier' label, 
  // nor do we trigger it for human assignees.
  const hasAgent = task && (isAgentAssignee || validEngine);

  if (!hasAgent) {
    return `
      <div style="display: flex; flex-direction: column; justify-content: center; align-items: center; height: 100%; min-height: 300px; color: var(--text-muted); padding: 2rem; text-align: center;">
        <span style="font-size: 2.5rem; margin-bottom: 0.75rem; opacity: 0.5;">🤖</span>
        <span style="font-weight: 600; color: var(--text-muted); font-size: 1rem;">No active agent bound</span>
        <span style="font-size: 0.82rem; opacity: 0.7; margin-top: 4px;">Task is not currently assigned to an agent</span>
      </div>
    `;
  }

  const isSwarm = (task.assignee && (task.assignee.includes('swarm') || task.assignee.includes('opencode'))) || task.tier === 'T2';
  const agentTitle = isSwarm ? '🤖 Swarm Tier 2 Agent (OpenCode Daemon)' : '⚡ Tier 1 IDE Agent (Antigravity)';
  const modelName = task.engine || (isSwarm ? 'Local Default Model' : 'Cloud Session Model');
  const executionMode = isSwarm ? 'Autonomous Swarm Pool (Tier 2 Sub-Agent)' : 'Direct Human-in-the-Loop Pair (Tier 1 Orchestrator)';
  
  const worktreePath = isSwarm
    ? `~/.nomos/data/${task.project || 'nomos'}/worktrees/task-${task.key || task.id}`
    : `~/Projects/sophialabs/open/${task.project || 'nomos'}`;
  
  const estimatedCost = task.cost || '--';
  
  let statusBadge = `<span style="background: rgba(34, 197, 94, 0.2); color: #4ade80; font-size: 0.75rem; padding: 0.2rem 0.5rem; border-radius: 4px; font-weight: 600;">ACTIVE</span>`;
  if (task.status === 'DONE' || task.status === 'CLOSED') {
    statusBadge = `<span style="background: rgba(168, 85, 247, 0.2); color: #c084fc; font-size: 0.75rem; padding: 0.2rem 0.5rem; border-radius: 4px; font-weight: 600;">COMPLETED</span>`;
  } else if (task.status === 'BACKLOG' || task.status === 'OPEN') {
    statusBadge = `<span style="background: rgba(255, 255, 255, 0.1); color: var(--text-muted); font-size: 0.75rem; padding: 0.2rem 0.5rem; border-radius: 4px; font-weight: 600;">INACTIVE</span>`;
  }

  return `
    <div style="color: var(--text-primary);">
      <!-- Agent Identity & Model Runtime -->
      <div style="background: rgba(168, 85, 247, 0.08); border: 1px solid rgba(168, 85, 247, 0.25); padding: 1.1rem; border-radius: 8px; margin-bottom: 1.25rem;">
        <div style="display: flex; justify-content: space-between; align-items: center; margin-bottom: 0.5rem;">
          <h4 style="margin: 0; color: #c084fc; font-size: 1.05rem;">${agentTitle}</h4>
          ${statusBadge}
        </div>
        <p style="margin: 0.25rem 0; font-size: 0.85rem;"><strong>Inference Engine:</strong> ${modelName}</p>
        <p style="margin: 0.25rem 0; font-size: 0.85rem;"><strong>Operating Mode:</strong> ${executionMode}</p>
        <p style="margin: 0.25rem 0; font-size: 0.85rem;"><strong>Est. Session Cost:</strong> <span style="color: var(--neon-green); font-weight: 600;">${estimatedCost}</span></p>
        <p style="margin: 0.25rem 0 0 0; font-size: 0.85rem;"><strong>Execution Tier:</strong> Tier ${task.tier || (isSwarm ? '2' : '1')} | <strong>Cognitive Burden:</strong> ${task.context_burden || '--'}/5 | <strong>Logic Depth:</strong> ${task.logic_depth || '--'}/5</p>
      </div>

      <!-- Live Infrastructure & Hardware Footprint -->
      <div style="background: rgba(0, 0, 0, 0.25); border: 1px solid var(--border-indigo); padding: 1.1rem; border-radius: 8px; margin-bottom: 1.25rem;">
        <h5 style="margin: 0 0 0.6rem 0; color: var(--neon-cyan); font-size: 0.9rem; text-transform: uppercase; letter-spacing: 0.5px;">⚡ Real-Time Hardware & Telemetry Footprint</h5>
        <div style="display: grid; grid-template-columns: 1fr 1fr; gap: 0.75rem; font-size: 0.82rem;">
          <div style="background: rgba(255,255,255,0.03); padding: 0.6rem; border-radius: 6px;">
            <span style="color: var(--text-muted); display: block; font-size: 0.75rem;">GPU VRAM Allocation</span>
            <strong id="agent-vram-allocation" style="color: var(--neon-green); font-size: 0.95rem;">--</strong>
          </div>
          <div style="background: rgba(255,255,255,0.03); padding: 0.6rem; border-radius: 6px;">
            <span style="color: var(--text-muted); display: block; font-size: 0.75rem;">Token Throughput</span>
            <strong id="agent-token-tps" style="color: var(--neon-yellow); font-size: 0.95rem;">-- TPS</strong>
          </div>
          <div style="background: rgba(255,255,255,0.03); padding: 0.6rem; border-radius: 6px;">
            <span style="color: var(--text-muted); display: block; font-size: 0.75rem;">Executed Tool Calls</span>
            <strong id="agent-tool-calls" style="color: var(--neon-pink); font-size: 0.95rem;">${task.tool_calls_count || '--'} Tool Calls</strong>
          </div>
          <div style="background: rgba(255,255,255,0.03); padding: 0.6rem; border-radius: 6px;">
            <span style="color: var(--text-muted); display: block; font-size: 0.75rem;">Substrate Lock</span>
            <strong id="agent-substrate-lock" style="color: #3b82f6; font-size: 0.95rem;">--</strong>
          </div>
        </div>
      </div>

      <!-- Execution Isolation & Worktree -->
      <div style="background: rgba(0, 0, 0, 0.25); border: 1px solid var(--border-indigo); padding: 1.1rem; border-radius: 8px; margin-bottom: 1.25rem;">
        <div style="display: flex; justify-content: space-between; align-items: center; margin-bottom: 0.6rem;">
          <h5 style="margin: 0; color: #a855f7; font-size: 0.9rem; text-transform: uppercase; letter-spacing: 0.5px;">🌳 Execution Isolation & Worktree</h5>
          <button onclick="navigator.clipboard.writeText('${worktreePath}'); alert('Worktree path copied to clipboard!');" style="background: rgba(168, 85, 247, 0.2); border: 1px solid #a855f7; color: #c084fc; font-size: 0.7rem; padding: 2px 8px; border-radius: 4px; cursor: pointer;">📋 Copy Path</button>
        </div>
        <p style="margin: 0; font-size: 0.82rem; font-family: monospace; word-break: break-all; background: rgba(0,0,0,0.3); padding: 0.5rem; border-radius: 4px;">
          ${worktreePath}
        </p>
        <p style="margin: 0.5rem 0 0 0; font-size: 0.82rem; color: var(--text-muted);">
          ${isSwarm ? '🔒 Isolation: Task-isolated git worktree linked via go.work' : '📂 Isolation: Shared root clone (Tier 1 Interactive Session)'}
        </p>
      </div>
    </div>
  `;
}

export async function openTaskDetailsDrawer(taskId: number | string): Promise<void> {
  const tabBtn = document.querySelector('.tab-btn[data-tab="tab-backlog"]') as HTMLButtonElement | null;
  if (tabBtn) {
    tabBtn.click();
  }

  const leftPaneContent = document.getElementById('task-details-pane-content');
  const rightPaneContent = document.getElementById('agent-details-pane-content');

  if (leftPaneContent) {
    setSafeInnerHTML(leftPaneContent, sanitizeHTMLString('<div style="display:flex; justify-content:center; padding:2rem;"><span class="pulsing-text">Fetching task details...</span></div>'));
  }

  try {
    const res = await fetch(`/api/backlog/${taskId}`);
    if (!res.ok) throw new Error('Failed to retrieve task details');
    const task = await res.json();

    if (leftPaneContent) {
      setSafeInnerHTML(leftPaneContent, sanitizeHTMLString(buildTaskDetailsPaneHtml(task)));
    }
    if (rightPaneContent) {
      setSafeInnerHTML(rightPaneContent, sanitizeHTMLString(buildAgentDetailsPaneHtml(task)));
    }
  } catch (err: any) {
    if (leftPaneContent) {
      setSafeInnerHTML(leftPaneContent, sanitizeHTMLString(`<div style="color:var(--neon-yellow); text-align:center; padding:2rem;">Error: ${err.message}</div>`));
    }
  }
}

export function closeTaskDetailsDrawer(): void {
  activeViewingTaskId = null;
  if (pollingIntervalId) {
    clearInterval(pollingIntervalId);
    pollingIntervalId = null;
  }
}

export async function openAgentDetailsDrawer(task: any): Promise<void> {
  const tabBtn = document.querySelector('.tab-btn[data-tab="tab-backlog"]') as HTMLButtonElement | null;
  if (tabBtn) {
    tabBtn.click();
  }

  const rightPaneContent = document.getElementById('agent-details-pane-content');
  if (rightPaneContent) {
    setSafeInnerHTML(rightPaneContent, sanitizeHTMLString(buildAgentDetailsPaneHtml(task)));
  }

  if (task && (task.key || task.id)) {
    const leftPaneContent = document.getElementById('task-details-pane-content');
    if (leftPaneContent) {
      try {
        const res = await fetch(`/api/backlog/${task.key || task.id}`);
        if (res.ok) {
          const fullTask = await res.json();
          setSafeInnerHTML(leftPaneContent, sanitizeHTMLString(buildTaskDetailsPaneHtml(fullTask)));
        } else {
          setSafeInnerHTML(leftPaneContent, sanitizeHTMLString(buildTaskDetailsPaneHtml(task)));
        }
      } catch {
        setSafeInnerHTML(leftPaneContent, sanitizeHTMLString(buildTaskDetailsPaneHtml(task)));
      }
    }
  }
}

