// logs.ts - Decoupled real-time WebSocket log tailing connection, routing, and dynamic filtering module

import { parseAnsiLine } from './ansi.js';
import { subscribeLogs, addWSListener } from './ws.js';
import { processSwarmTelemetryEvent } from './ui_telemetry.js';

let activeLogSource = 'all';
let isWSLogsListenerAdded = false;
const disabledSources = new Set<string>();

export function clearTerminalLogs(): void {
  const termBody = document.getElementById('terminal-body-div');
  if (termBody) {
    termBody.replaceChildren();
  }
}

function getSourceBadgeColor(src: string): string {
  switch (src) {
    case 'telemetry':
    case 'os':
      return 'var(--neon-blue)';
    case 'substrate':
      return 'var(--neon-pink)';
    case 'swarm-telemetry':
      return 'var(--neon-green)';
    case 'llama-embed':
    case 'llama-coder':
    case 'llama-server':
      return 'var(--neon-yellow)';
    case 'control-plane':
      return 'var(--neon-purple)';
    case 'aider':
      return 'var(--neon-blue)';
    default:
      return 'var(--text-muted)';
  }
}

function formatLogTimestamp(rawTime: any): string {
  if (!rawTime) return '';
  if (typeof rawTime === 'number') {
    const d = new Date(rawTime * 1000);
    return d.toISOString().replace('T', ' ').substring(0, 19);
  }
  const str = String(rawTime);
  const tIdx = str.indexOf('T');
  if (tIdx !== -1) {
    return str.substring(0, 10) + ' ' + str.substring(tIdx + 1, tIdx + 9);
  }
  return str;
}

function finalizeLogLine(lineEl: HTMLElement, termBody: HTMLElement) {
	const isMatch = checkLineMatch(lineEl);
	lineEl.style.display = isMatch ? 'block' : 'none';

	const isAtBottom = termBody.scrollHeight - termBody.scrollTop - termBody.clientHeight < 50;

	termBody.appendChild(lineEl);
	while (termBody.children.length > 2000) {
		termBody.removeChild(termBody.firstChild!);
	}
	
	const ultLogs = document.getElementById('ultimate-live-logs');
	let ultIsAtBottom = false;
	if (ultLogs) {
		ultIsAtBottom = ultLogs.scrollHeight - ultLogs.scrollTop - ultLogs.clientHeight < 50;
		ultLogs.appendChild(lineEl.cloneNode(true));
		while (ultLogs.children.length > 2000) {
			ultLogs.removeChild(ultLogs.firstChild!);
		}
	}

	requestAnimationFrame(() => {
		if (isAtBottom) {
			termBody.scrollTop = termBody.scrollHeight;
		}
		if (ultLogs && ultIsAtBottom) {
			ultLogs.scrollTop = ultLogs.scrollHeight;
		}
	});
}

export function normalizeSourceTS(raw: string): string {
  const s = (raw || '').toLowerCase().trim();
  switch (s) {
    case 'telemetry':
    case 'os':
    case 'orchestrator':
    case 'kernel':
      return 'kernel';
    case 'substrate':
    case 'firewall':
    case 'syscall':
      return 'substrate';
    case 'swarm':
    case 'swarm-telemetry':
    case 'worker':
      return 'swarm';
    case 'aider':
    case 'aider-agent':
      return 'aider';
    case 'server':
    case 'control-plane':
    case 'nomosd':
    case 'daemon':
      return 'server';
    case 'llama':
    case 'llama-vm-logs':
    case 'gpu-telemetry':
      return 'llama';
    case 'all':
    case '':
      return 'all';
    default:
      if (s.startsWith('swarm')) return 'swarm';
      if (s.startsWith('aider')) return 'aider';
      return s;
  }
}

function appendAndFormatLogLine(line: string, sourceName?: string) {
  const termBody = document.getElementById('terminal-body-div');
  if (!termBody) return;

  const lineEl = document.createElement('div');
  lineEl.className = 'log-line';
  let effectiveSource = sourceName || activeLogSource;
  lineEl.setAttribute('data-source', effectiveSource);

  // Parse and colour-code structured JSON telemetry logs
  let isJSON = false;
  let logObj: any = null;
  if (line.trim().startsWith('{') && line.trim().endsWith('}')) {
    try {
      logObj = JSON.parse(line);
      isJSON = true;
      if (logObj.agent === 'substrate' || logObj.source === 'substrate' || logObj.op) {
        effectiveSource = 'substrate';
        lineEl.setAttribute('data-source', 'substrate');
      } else if (logObj.source) {
        effectiveSource = normalizeSourceTS(logObj.source);
        lineEl.setAttribute('data-source', effectiveSource);
      }
    } catch (e) {
      isJSON = false;
    }
  }

  if (isJSON && logObj) {
    if (logObj.source === 'swarm_telemetry' || effectiveSource === 'swarm' || logObj.agent === 'swarm_telemetry') {
      processSwarmTelemetryEvent(logObj);
    }
    
    const levelVal = (logObj.level || logObj.event_type || 'INFO').toUpperCase();
    let agentVal = logObj.agent || logObj.agent_name || logObj.source || '';
    const cmdVal = logObj.cmd || '';
    const msgVal = logObj.msg || logObj.detail || '';

    if (agentVal.startsWith('orchestrator:')) {
      agentVal = agentVal.replace('orchestrator:', 'kernel:');
    } else if (agentVal === 'telemetry' || agentVal === 'os') {
      agentVal = 'kernel';
    } else if (agentVal === 'substrate') {
      agentVal = 'firewall';
    }

    lineEl.setAttribute('data-level', levelVal.toLowerCase());
    lineEl.setAttribute('data-agent', agentVal);
    lineEl.setAttribute('data-cmd', cmdVal);
    lineEl.setAttribute('data-msg', msgVal);
    
    // 1. Timestamp formatting [YYYY-MM-DD HH:MM:SS] with fallback to current time
    const rawTime = logObj.ts || logObj.timestamp || logObj.time || new Date().toISOString();
    const tsSpan = document.createElement('span');
    tsSpan.style.color = 'var(--text-muted)';
    tsSpan.style.marginRight = '6px';
    tsSpan.style.fontFamily = "'JetBrains Mono', monospace";
    tsSpan.textContent = `[${formatLogTimestamp(rawTime)}]`;
    lineEl.appendChild(tsSpan);
      
      // 2. Level formatting
      const lvlSpan = document.createElement('span');
      lvlSpan.style.marginRight = '6px';
      lvlSpan.style.fontWeight = 'bold';
      lvlSpan.style.fontFamily = "'JetBrains Mono', monospace";
      lvlSpan.textContent = levelVal;
      if (levelVal === 'INFO' || levelVal === 'PHASE_TRANSITION' || levelVal === 'COMMIT' || levelVal === 'DOD_RESULT') {
        lvlSpan.style.color = 'var(--neon-green)';
      } else if (levelVal === 'ERROR' || levelVal === 'FATAL') {
        lvlSpan.style.color = 'var(--neon-red)';
      } else if (levelVal === 'WARN' || levelVal === 'WARNING') {
        lvlSpan.style.color = 'var(--neon-yellow)';
      } else {
        lvlSpan.style.color = 'var(--neon-blue)';
      }
      lineEl.appendChild(lvlSpan);
      
      // 2.5 Project formatting (if injected)
      if (logObj.project) {
        lineEl.setAttribute('data-project', logObj.project);
        const projSpan = document.createElement('span');
        projSpan.style.color = 'var(--neon-purple)';
        projSpan.style.fontWeight = 'bold';
        projSpan.style.marginRight = '6px';
        projSpan.textContent = `[${logObj.project}]`;
        lineEl.appendChild(projSpan);
      }
      
      // 3. Agent formatting
      if (agentVal) {
        const agSpan = document.createElement('span');
        agSpan.style.color = agentVal.startsWith('telemetry:') ? 'var(--neon-blue)' : 'var(--neon-pink)';
        agSpan.style.fontWeight = '600';
        agSpan.style.marginRight = '6px';
        agSpan.textContent = `[${agentVal}]`;
        lineEl.appendChild(agSpan);
      }
      
      // 4. Command context
      if (cmdVal) {
        const cmdSpan = document.createElement('span');
        cmdSpan.style.color = 'var(--neon-blue)';
        cmdSpan.style.marginRight = '6px';
        cmdSpan.textContent = `(${cmdVal})`;
        lineEl.appendChild(cmdSpan);
      }
      
      // 5. Column Divider
      const divSpan = document.createElement('span');
      divSpan.style.color = 'var(--border-indigo)';
      divSpan.style.marginRight = '6px';
      divSpan.textContent = '│';
      lineEl.appendChild(divSpan);
      
      // 6. Message body
      if (msgVal) {
        const msgSpan = document.createElement('span');
        msgSpan.style.color = 'var(--text-normal)';
        msgSpan.textContent = msgVal;
        lineEl.appendChild(msgSpan);
      }
      
      // 7. Version identifier
      if (logObj.ver && logObj.ver !== 'unknown') {
        const verSpan = document.createElement('span');
        verSpan.style.color = 'var(--text-muted)';
        verSpan.style.fontSize = '0.75rem';
        verSpan.style.marginLeft = '4px';
        verSpan.textContent = `(v${logObj.ver})`;
        lineEl.appendChild(verSpan);
      }
      
      // Apply filter constraints immediately
      finalizeLogLine(lineEl, termBody);
      return;
  }

  // Fallback ANSI formatting for standard process streams
  if (line.includes('HTTP Request:') || line.includes('nomosd')) {
    lineEl.setAttribute('data-source', 'server');
  }
  const parts = parseAnsiLine(line);
  parts.forEach(p => lineEl.appendChild(p));
  lineEl.setAttribute('data-raw', line);

  finalizeLogLine(lineEl, termBody);
}

let initialPlaceholderCleared = false;

function setPillVisualState(el: Element, active: boolean) {
  const htmlEl = el as HTMLElement;
  if (active) {
    htmlEl.classList.add('active');
    htmlEl.style.opacity = '1';
    htmlEl.style.filter = 'none';
  } else {
    htmlEl.classList.remove('active');
    htmlEl.style.opacity = '0.35';
    htmlEl.style.filter = 'grayscale(80%)';
  }
}

export function toggleSourcePill(sourceName: string): void {
  const pillBtn = document.querySelector(`.source-pill[data-source="${sourceName}"]`);
  if (sourceName === 'all') {
    disabledSources.clear();
    const allPills = document.querySelectorAll('.source-pill');
    allPills.forEach(p => {
      setPillVisualState(p, true);
    });
  } else {
    if (disabledSources.has(sourceName)) {
      disabledSources.delete(sourceName);
      if (pillBtn) setPillVisualState(pillBtn, true);
    } else {
      disabledSources.add(sourceName);
      if (pillBtn) setPillVisualState(pillBtn, false);
    }
  }
  applyLogFilters();
}
(window as any).toggleSourcePill = toggleSourcePill;

export function connectLogs(sourceId: string): void {
  const isSourceChanging = activeLogSource !== sourceId;
  activeLogSource = sourceId;

  const termBody = document.getElementById('terminal-body-div');
  if (!termBody) return;
  
  if (isSourceChanging) {
    termBody.replaceChildren();
    const logLine = document.createElement('div');
    logLine.className = 'log-line';
    logLine.style.color = 'var(--text-muted)';
    if (sourceId === 'all') {
      logLine.textContent = 'Re-establishing WebSocket conduit to Unified Real-Time Log stream...';
    } else if (sourceId === 'os') {
      logLine.textContent = 'Re-establishing WebSocket conduit to Nomos telemetry stream...';
    } else if (sourceId === 'gpu-telemetry') {
      logLine.textContent = 'Subscribing to remote GPU hardware telemetry via SSH...';
    } else {
      logLine.textContent = `Subscribing to WebSocket telemetry log conduit for ${sourceId}...`;
    }
    termBody.appendChild(logLine);
    initialPlaceholderCleared = true;
  } else {
    initialPlaceholderCleared = true;
  }

function isLogSourceMatch(frameSource: string, activeSource: string): boolean {
  if (activeSource === 'all' || activeSource === '' || !activeSource) return true;
  if (!frameSource) return true;
  if (frameSource === activeSource) return true;
  const f = frameSource.toLowerCase();
  const a = activeSource.toLowerCase();
  if ((a === 'telemetry' || a === 'os' || a === 'kernel') && (f === 'telemetry' || f === 'os' || f === 'kernel' || f === 'orchestrator')) return true;
  if ((a === 'substrate' || a === 'firewall') && (f === 'substrate' || f === 'firewall')) return true;
  if ((a === 'swarm' || a === 'swarm-telemetry') && (f === 'swarm' || f === 'swarm-telemetry')) return true;
  if ((a === 'server' || a === 'control-plane' || a === 'nomosd') && (f === 'server' || f === 'control-plane' || f === 'nomosd')) return true;
  return false;
}

  // Set up global WS listener once
  if (!isWSLogsListenerAdded) {
    isWSLogsListenerAdded = true;
    addWSListener((frame: any) => {
      if (frame.type === 'logs' && isLogSourceMatch(frame.log_source, activeLogSource)) {
        if (!initialPlaceholderCleared) {
          const tb = document.getElementById('terminal-body-div');
          if (tb) tb.replaceChildren();
          initialPlaceholderCleared = true;
        }
        appendAndFormatLogLine(frame.log_text, frame.log_source);
      }
    });
  }

  subscribeLogs(sourceId);

  const labelEl = document.getElementById('active-log-source');
  if (labelEl) {
    if (sourceId === 'all') {
      labelEl.textContent = 'Source: Unified Real-Time Logs (All 8 Streams)';
    } else if (sourceId === 'os') {
      labelEl.textContent = 'Source: WebSocket (telemetry.jsonl)';
    } else if (sourceId === 'gpu-telemetry') {
      labelEl.textContent = 'Source: SSH (nvidia-smi)';
    } else {
      labelEl.textContent = `Source: WebSocket (worker_${sourceId}.log)`;
    }
  }
}

export function changeLogSource(sourceId: string): void {
  connectLogs(sourceId);
}

/**
 * Checks if a log element matches the currently active level and text filter controls
 */
function checkLineMatch(el: HTMLElement): boolean {
  const levelSelect = (document.getElementById('log-filter-level') || document.getElementById('log-level-filter')) as HTMLSelectElement | null;
  const activeLevel = levelSelect?.value || 'ALL';
  const textInput = (document.getElementById('log-filter-text') || document.getElementById('log-text-filter')) as HTMLInputElement | null;
  const filterText = textInput?.value.toLowerCase() || '';

  const lineSource = el.getAttribute('data-source') || '';
  if (disabledSources.has(lineSource) || (lineSource.startsWith('llama') && disabledSources.has('llama'))) {
    return false;
  }

  // 1. Text filter evaluation
  const rawText = el.getAttribute('data-raw') || el.textContent || '';
  if (filterText && !rawText.toLowerCase().includes(filterText)) {
    return false;
  }

  // 1b. Project Filter evaluation
  const activeProjectFilter = (window as any).activeProjectFilter || 'ALL';
  const elProject = el.getAttribute('data-project') || 'ALL';
  if (activeProjectFilter !== 'ALL' && elProject !== 'ALL' && elProject !== activeProjectFilter) {
    return false;
  }

  // 2. Log level evaluation
  if (activeLevel === 'ALL') {
    return true;
  }

  const elLevel = (el.getAttribute('data-level') || 'INFO').toUpperCase();
  if (activeLevel === 'ERROR') {
    return elLevel === 'ERROR' || elLevel === 'FATAL';
  }
  if (activeLevel === 'WARN') {
    return elLevel === 'WARN' || elLevel === 'WARNING' || elLevel === 'ERROR' || elLevel === 'FATAL';
  }
  return elLevel === activeLevel;
}

export function applyLogFilters(): void {
  const termBody = document.getElementById('terminal-body-div');
  if (!termBody) return;

  const lines = termBody.querySelectorAll('.log-line');
  lines.forEach(line => {
    const isMatch = checkLineMatch(line as HTMLElement);
    (line as HTMLElement).style.display = isMatch ? 'block' : 'none';
  });
}

export function updateLogDropdownWithWorkers(swarmData: any, activeSwarms: any[] = []): void {
  const select = document.getElementById('log-source-select') as HTMLSelectElement | null;
  if (!select) return;

  const currentSelection = select.value;
  const nodes = swarmData.nodes || [];

  // Remove dynamically added worker options
  const toRemove: HTMLOptionElement[] = [];
  for (let i = 0; i < select.options.length; i++) {
    const opt = select.options[i];
    if (
      opt.value !== 'os' &&
      opt.value !== 'telemetry' &&
      opt.value !== 'substrate' &&
      opt.value !== 'llama-embed' &&
      opt.value !== 'llama-coder' &&
      opt.value !== 'llama-server' &&
      opt.value !== 'control-plane' &&
      opt.value !== 'aider' &&
      opt.value !== 'gpu-telemetry'
    ) {
      toRemove.push(opt);
    }
  }
  toRemove.forEach(opt => opt.remove());

  // Add active worker instances
  nodes.forEach((n: any) => {
    if (n.id && n.id !== 'orchestrator') {
      const opt = document.createElement('option');
      opt.value = n.id;
      opt.textContent = `Worker Task #${n.id}`;
      select.appendChild(opt);
    }
  });

  // Dynamically add any task IDs from active worktrees (even during VM boot startup phase)
  const swarmsList = Array.isArray(activeSwarms) ? activeSwarms : [];
  for (const wt of swarmsList) {
    if (!wt.name || !wt.name.startsWith('task-')) {
      continue;
    }
    const taskID = wt.name.replace('task-', '');
    // Check if option already exists
    let exists = false;
    for (let i = 0; i < select.options.length; i++) {
      if (select.options[i].value === taskID) {
        exists = true;
        break;
      }
    }
    if (exists) {
      continue;
    }
    const opt = document.createElement('option');
    opt.value = taskID;
    opt.textContent = `Worker Task #${taskID}`;
    select.appendChild(opt);
  }

  // Restore selection if still exists
  let optionExists = false;
  for (let i = 0; i < select.options.length; i++) {
    if (select.options[i].value === currentSelection) {
      optionExists = true;
      break;
    }
  }

  if (optionExists) {
    select.value = currentSelection;
  } else {
    select.value = 'os';
  }
}

// Attach filter change listeners
window.addEventListener('load', () => {
  const lvlFilter = document.getElementById('log-level-filter');
  if (lvlFilter) {
    lvlFilter.addEventListener('change', applyLogFilters);
  }

  const txtFilter = document.getElementById('log-text-filter');
  if (txtFilter) {
    txtFilter.addEventListener('input', applyLogFilters);
  }

  const pillBtns = document.querySelectorAll('.source-pill');
  pillBtns.forEach(p => {
    p.addEventListener('click', () => {
      const srcName = p.getAttribute('data-source');
      if (srcName) {
        toggleSourcePill(srcName);
      }
    });
  });

  const srcSelect = document.getElementById('log-source-select');
  if (srcSelect) {
    srcSelect.addEventListener('change', (e) => {
      const target = e.target as HTMLSelectElement;
      changeLogSource(target.value);
    });
  }

  // Connect unified log stream by default
  connectLogs('all');

  const toggleBtn = document.getElementById('toggle-nix-shell-btn');
  const monitorPanel = document.querySelector('.nix-shell-monitor-panel') as HTMLElement | null;
  const termBody = document.getElementById('terminal-body-div');

  // Dynamically toggles visibility of the active nix-shell monitoring tree panel
  const updateNixShellVisibility = (visible: boolean) => {
    if (!monitorPanel) return;
    if (visible) {
      monitorPanel.style.display = 'flex';
      if (termBody) termBody.style.borderRight = '1px solid var(--border-indigo)';
      if (toggleBtn) toggleBtn.textContent = 'Hide Nomos Shells';
    } else {
      monitorPanel.style.display = 'none';
      if (termBody) termBody.style.borderRight = 'none';
      if (toggleBtn) toggleBtn.textContent = 'Show Nomos Shells';
    }
  };

  // Load and apply initial state preference from localStorage
  let showNixShells = localStorage.getItem('nomos-show-nix-shells') !== 'false';
  updateNixShellVisibility(showNixShells);

  if (toggleBtn) {
    toggleBtn.addEventListener('click', () => {
      showNixShells = !showNixShells;
      localStorage.setItem('nomos-show-nix-shells', showNixShells ? 'true' : 'false');
      updateNixShellVisibility(showNixShells);
    });
  }

  // Periodic polling to update active nix-shell tree telemetry without altering app.ts
  const pollNixShells = async () => {
    try {
      const res = await fetch('/api/status');
      if (res.ok) {
        const data = await res.json();
        updateNixShellsUI(data.nixShells || []);
      }
    } catch (err) {
      console.error('Error fetching nix-shell status:', err);
    }
  };
  pollNixShells();
  setInterval(pollNixShells, 5000);
});

/**
 * Searches the arguments array of a nix-shell process to extract the --run or -c command script.
 * Returns empty string if no run command flag was passed to the shell.
 */
function findRunCommand(args: string[]): string {
  if (!args) return '';
  for (let i = 0; i < args.length; i++) {
    const isRunFlag = args[i] === '--run' || args[i] === '-c';
    if (isRunFlag && i + 1 < args.length) {
      return args[i + 1];
    }
  }
  return '';
}

/**
 * Dynamically builds a single child process row element showing command name, PID, and arguments.
 */
function createChildProcessRow(child: any): HTMLDivElement {
  const childEl = document.createElement('div');
  childEl.style.fontSize = '0.68rem';
  childEl.style.color = 'var(--neon-blue)';
  
  const cmdName = document.createElement('span');
  cmdName.style.fontWeight = 'bold';
  cmdName.textContent = `▶ ${child.command} (PID ${child.pid}) `;
  childEl.appendChild(cmdName);

  if (child.args && child.args.length > 1) {
    const childArgs = document.createElement('span');
    childArgs.style.color = 'var(--text-muted)';
    childArgs.textContent = child.args.slice(1).join(' ');
    childEl.appendChild(childArgs);
  }
  return childEl;
}

/**
 * Constructs a premium card for a parent nix-shell process.
 * Populates all running sub-processes that are children or descendants of this nix-shell.
 */
function createNixShellCard(parent: any, children: any[]): HTMLDivElement {
  const parentEl = document.createElement('div');
  parentEl.className = 'nix-shell-card';
  parentEl.style.background = 'rgba(255, 255, 255, 0.03)';
  parentEl.style.border = '1px solid var(--border-indigo)';
  parentEl.style.borderRadius = '6px';
  parentEl.style.padding = '8px 12px';
  parentEl.style.marginBottom = '8px';

  // Card header displaying shell indicator icon and process PID
  const headerEl = document.createElement('div');
  headerEl.style.display = 'flex';
  headerEl.style.justifyContent = 'space-between';
  headerEl.style.alignItems = 'center';
  headerEl.style.marginBottom = '6px';

  const titleEl = document.createElement('span');
  titleEl.style.fontWeight = 'bold';
  titleEl.style.color = 'var(--neon-green)';
  titleEl.textContent = `🐚 nomos shell (PID ${parent.pid})`;

  headerEl.appendChild(titleEl);
  parentEl.appendChild(headerEl);

  // Render the specific command executed in this environment if present
  if (parent.args && parent.args.length > 0) {
    const runCmd = findRunCommand(parent.args);
    const argsEl = document.createElement('div');
    argsEl.style.fontSize = '0.7rem';
    argsEl.style.color = 'var(--text-normal)';
    argsEl.style.background = 'var(--bg-glass)';
    argsEl.style.padding = '4px 8px';
    argsEl.style.borderRadius = '4px';
    argsEl.style.wordBreak = 'break-all';
    argsEl.style.marginTop = '4px';
    argsEl.textContent = runCmd ? `run: ${runCmd}` : `args: ${parent.args.join(' ')}`;
    parentEl.appendChild(argsEl);
  }

  // Filter child processes belonging to this specific parent nix-shell
  const myChildren = children.filter(c => c.ppid === parent.pid);
  if (myChildren.length > 0) {
    const childrenTitle = document.createElement('div');
    childrenTitle.style.fontSize = '0.65rem';
    childrenTitle.style.color = 'var(--text-muted)';
    childrenTitle.style.marginTop = '8px';
    childrenTitle.style.fontWeight = '600';
    childrenTitle.textContent = 'RUNNING PROCESSES:';
    parentEl.appendChild(childrenTitle);

    const childrenList = document.createElement('div');
    childrenList.style.display = 'flex';
    childrenList.style.flexDirection = 'column';
    childrenList.style.gap = '4px';
    childrenList.style.marginTop = '4px';
    childrenList.style.paddingLeft = '8px';
    childrenList.style.borderLeft = '1px dashed var(--border-indigo)';

    myChildren.forEach(child => {
      const childEl = createChildProcessRow(child);
      childrenList.appendChild(childEl);
    });

    parentEl.appendChild(childrenList);
  }

  return parentEl;
}

/**
 * Triggers UI updates for the active nix-shell tree panel.
 * Groups and maps processes by parent/child relationships and appends them to container.
 */
export function updateNixShellsUI(nixShells: any[]): void {
  const container = document.getElementById('nix-shell-tree-container');
  const countBadge = document.getElementById('nix-shell-count-badge');
  if (!container) return;

  // Render placeholder if there are no active shells detected
  if (!nixShells || nixShells.length === 0) {
    container.innerHTML = `<div style="color: var(--text-muted); font-style: italic;">No active nix-shell sessions detected.</div>`;
    if (countBadge) countBadge.textContent = '0';
    return;
  }

  // Segment processes into nix-shell environments and their sub-processes
  const parents = nixShells.filter(p => p.command === 'nix-shell');
  const children = nixShells.filter(p => p.command !== 'nix-shell');

  if (countBadge) {
    countBadge.textContent = parents.length.toString();
  }

  container.replaceChildren();

  // Draw a premium UI card for each active nix-shell session
  parents.forEach(parent => {
    const parentEl = createNixShellCard(parent, children);
    container.appendChild(parentEl);
  });
}
