// app.ts - Swarm Cockpit Dashboard Application Entry Point
// Orchestrates all telemetry polling, WebSocket real-time updates,
// UI rendering pipelines, event bindings, and global window method exposure.
// This is the single top-level module that wires all dashboard subsystems together.
//
// ============================================================================
// ARCHITECTURAL DOCUMENTATION & DESIGN OVERVIEW
// ============================================================================
// The Nomos Command Center frontend is structured around a centralized state
// synchronization loop. All telemetry data is queried periodically via polling or
// pushed asynchronously through WebSockets. The main application is divided into
// several key modules:
//
// 1. Telemetry Subsystem (telemetry.ts, ui_telemetry.ts):
//    Handles system status, LEDs, memory usages, CPU load, and workspace drift.
// 2. Task Boards:
//    Visualizes open backlog issues, plans, active edit tasks, and review states.
// 3. AST & Dependency Analysis (ast.js):
//    Visualizes packages coupling metrics and detects cyclic dependencies.
// 4. Memory & Log Streamers (memory.ts, logs.js):
//    Handles logs, vector database memories, search queries, and historical entries.
// 5. Sovereignty HUD (hud.ts):
//    Exposes active contexts, handles workspace resetting, and visualizes slot execution states.
//
// Key Design Patterns:
// - Single Source of Truth: app.ts maintains latestStatus.
// - Progressive Enhancement: falling back to HTTP long-polling if WS disconnects.
// - Clean Architecture: clear boundaries between telemetry processing and visual components.
// ============================================================================
import { renderArchitectureTopology, triggerArchitecturePulse } from './architecture.js';
import { CreativeWizard } from './components/CreativeWizard.js';
async function handlePlanApprove(btn) {
    btn.disabled = true;
    btn.textContent = 'Approving...';
    try {
        const res = await fetch(`/api/phase?cmd=review`, { method: 'POST' });
        const data = await res.json();
        if (data.success) {
            showToast('Plan approved! Swarm worker entered EDIT phase.', 'success');
            refreshData();
        }
        else {
            showToast(`Approval failed: ${data.error}`, 'error');
        }
    }
    catch (err) {
        showToast(`Network error: ${err.message}`, 'error');
    }
    finally {
        btn.disabled = false;
        btn.textContent = '✅ Approve Plan & Enter EDIT';
    }
}
async function handleTaskSignoff(btn) {
    btn.disabled = true;
    btn.textContent = 'Signing off...';
    try {
        const res = await fetch(`/api/phase?cmd=approve`, { method: 'POST' });
        const data = await res.json();
        if (data.success) {
            showToast('Task approved! Committing and pushing changes.', 'success');
            refreshData();
        }
        else {
            showToast(`Sign-off failed: ${data.error}`, 'error');
        }
    }
    catch (err) {
        showToast(`Network error: ${err.message}`, 'error');
    }
    finally {
        btn.disabled = false;
        btn.textContent = '🚀 Approve & Commit/Push';
    }
}
async function handleGcpToggle(gcpToggleBtn) {
    const isOnline = gcpToggleBtn.textContent === 'Stop';
    const action = isOnline ? 'stop' : 'start';
    gcpToggleBtn.disabled = true;
    gcpToggleBtn.textContent = isOnline ? 'Stopping...' : 'Starting...';
    const errEl = document.getElementById('gcp-spot-error');
    if (errEl)
        errEl.style.display = 'none';
    try {
        const res = await fetch(`/api/provider/toggle?action=${action}`, { method: 'POST' });
        const data = await res.json();
        if (data.success) {
            showToast(`Cloud Substrate VM ${action} command sent successfully.`, 'success');
        }
        else {
            showToast(data.error || `Failed to ${action} Cloud Substrate VM`, 'error');
            if (errEl) {
                errEl.textContent = data.error || `Failed to ${action} VM`;
                errEl.style.display = 'block';
            }
        }
    }
    catch (err) {
        showToast(`Network error toggling Cloud Substrate: ${err.message}`, 'error');
        if (errEl) {
            errEl.textContent = err.message;
            errEl.style.display = 'block';
        }
    }
    finally {
        gcpToggleBtn.disabled = false;
    }
}
// --- Module Imports ---
// Each imported module encapsulates a specific dashboard panel or subsystem.
// The architecture follows a hub-and-spoke pattern: app.ts is the hub that
// coordinates data flow between the API layer and each rendering module.
import { CockpitStore } from './store.js';
import { initHeader } from './components/Header.js';
import { TaskBoardComponent } from './components/TaskBoard.js';
import { updateTelemetryUI, renderSteeringButtons } from './telemetry.js';
import { renderASTGraph } from './ast.js';
import { showToast } from './toast.js';
import { toggleModal, openTaskDetailsDrawer, openAgentDetailsDrawer, closeTaskDetailsDrawer, registerLatestStatusGetter } from './modal.js';
import { connectLogs, changeLogSource, updateLogDropdownWithWorkers } from './logs.js';
import { fetchActiveArtifact, saveActiveArtifact, getCurrentActiveArtifactType } from './artifacts.js';
import { updateTimelineUI, selectedLessonHash, activeSearchQuery, activeMemoryCategory, onMemorySearchChange, filterMemoryByCategory, filterMemoryByTag, renderPruningAdvisorDashboard as renderPruningAdvisorDashboardInMemory } from './memory.js';
import { updateHeaderContextIndicator } from './hud.js';
import { openSwarmChatDrawer, closeSwarmChatDrawer, sendUserChatMessage, renderChatMessage } from './chat.js';
import { pruneWorkspaces, auditBranches, pruneBranch, pruneAllBranches, fetchGitStatus } from './git.js';
import { initWorkspaceContextSwitcher, updateWorkspaceContextDropdownSelection } from './workspace.js';
import { initNixosUI } from './nixos.js';
import { initControlPlaneWS, addWSListener, wsConnected } from './ws.js';
import { initSplitters } from './splitters.js';
import { updateConnectionLED, updateDriftUI, updateAnalyticsUI, formatContextSize, updateFleetUI, updateGitBrainUI, updateHeaderTelemetryUI } from './ui_telemetry.js';
import { openSwarmConsoleDrawer, closeSwarmConsoleDrawer } from './swarm_drawer.js';
// --- Application State ---
// These module-level variables hold the latest telemetry snapshot and UI state.
// They are updated on each polling cycle or WebSocket frame and read by render functions.
let latestStatus = null; // Most recent /api/status response
let activeTab = 'tab-logs'; // Currently active bottom-panel tab
let isWSFrame = false; // True when data was pushed via WebSocket (skip HTTP fetch)
let refreshTimeoutId = null; // Active setTimeout reference for the refresh loop
let activeDrift = {}; // Current spec parity drift payload from /api/drift
let lastFleet = null; // Latest fleet neighbors from /api/fleet
let lastGitbrainData = null; // Latest gitbrain metrics
// Register latestStatus getter for modal actions
registerLatestStatusGetter(() => latestStatus);
// Re-export aliased pruning advisor renderer for cleaner call sites
const renderPruningAdvisorDashboard = renderPruningAdvisorDashboardInMemory;
// Dispatch handler - submits a new swarm worker dispatch request to the backend
// Reads form inputs (branch, taskId, instruction), POSTs to /api/dispatch,
// and shows a toast notification on success or failure.
async function handleDispatchSubmit(e) {
    e.preventDefault();
    const branch = document.getElementById('form-branch').value;
    const taskId = document.getElementById('form-task-id').value;
    const instruction = document.getElementById('form-instruction').value;
    try {
        const res = await fetch(`/api/dispatch?branch=${encodeURIComponent(branch)}&task_id=${encodeURIComponent(taskId)}&instruction=${encodeURIComponent(instruction)}`);
        const data = await res.json();
        if (!data.success)
            return showToast(`Dispatch rejected: ${data.error || 'unknown'}`, 'error');
        showToast(`Swarm Worker dispatched successfully (PID: ${data.pid})`, 'success');
        toggleModal(false);
        document.getElementById('dispatch-form').reset();
        refreshData();
    }
    catch (err) {
        showToast('Connection failed during agent dispatch', 'error');
    }
}
// Signal handler - sends a process signal (kill/stop) to a running swarm worker PID
async function handleProcessSignal(pid, action) {
    try {
        const res = await fetch(`/api/${action}?pid=${pid}`);
        const data = await res.json();
        if (!data.success)
            return showToast(`Signal delivery failed: ${data.error}`, 'error');
        showToast(`Process ${pid} successfully sent signal: ${action.toUpperCase()}`, 'success');
        refreshData();
    }
    catch (e) {
        showToast('Signal conduit error', 'error');
    }
}
// Helper to retrieve JSON data with a type-safe fallback
export async function fetchSafeJson(url, defaultValue) {
    try {
        const res = await fetch(url);
        if (res.ok) {
            if (url === '/api/status')
                updateConnectionLED(true);
            return await res.json();
        }
        if (url === '/api/status')
            updateConnectionLED(false);
    }
    catch (err) {
        console.error(`[Telemetry Poll] Safe fetch failed for ${url}:`, err.message || err);
        if (url === '/api/status')
            updateConnectionLED(false);
    }
    return defaultValue;
}
// Module level cache for low-frequency dashboard components.
// These are only re-fetched every Nth poll cycle to reduce server load.
// High-frequency data (status, drift) is fetched every cycle; low-frequency
// data (backlog, AST graph, lessons, analytics) uses adaptive intervals.
let lastBacklog = []; // Cached backlog task list
let lastAST = { nodes: [], links: [] }; // Cached AST dependency graph
let lastLessons = []; // Cached memory timeline lessons
let lastAnalytics = null; // Cached analytics dashboard data
let pollCounter = 0; // Monotonic poll cycle counter
let activeSwarmsList = []; // Currently running swarm workers
let lastActiveSwarmsList = []; // Previous cycle's swarm list (for diff detection)
// Refresh Data Controller Orchestrator
// Central polling loop that fetches all API data, updates caches,
// and dispatches to each UI rendering subsystem.
// Supports two modes: HTTP polling and WebSocket push (isWSFrame flag).
export async function refreshData(forceHttp = false) {
    if (refreshTimeoutId) {
        clearTimeout(refreshTimeoutId);
        refreshTimeoutId = null;
    }
    if (!forceHttp && wsConnected && !isWSFrame) {
        refreshTimeoutId = setTimeout(() => refreshData(false), 1500);
        return;
    }
    // Increment the monotonic poll counter to track execution cycles
    pollCounter++;
    const isInitial = (pollCounter === 1);
    // Adaptive polling interval setup: If workspace is IDLE, query low frequency components
    // every 10 cycles (15 seconds); if active, query every 5 cycles (7.5 seconds) to reduce load.
    const isIdle = (latestStatus && latestStatus.phaseState && latestStatus.phaseState.current_phase === 'IDLE');
    const lowFreqInterval = isIdle ? 10 : 5;
    // Decide whether to fetch low-frequency elements (backlog tasks, AST graphs, historical lessons)
    const shouldFetchLowFreq = isInitial
        || (pollCounter % lowFreqInterval === 0)
        || (lastBacklog.length === 0)
        || (!lastAST || !lastAST.nodes || lastAST.nodes.length === 0);
    try {
        let status, backlog, swarm, activeSwarms, ast, drift, lessons, analytics, gitbrainData;
        if (isWSFrame) {
            status = latestStatus;
            backlog = lastBacklog;
            swarm = window.latestSwarmData || { nodes: [], links: [] };
            activeSwarms = activeSwarmsList || [];
            ast = lastAST;
            drift = activeDrift;
            lessons = lastLessons;
            analytics = lastAnalytics;
            gitbrainData = lastGitbrainData;
            isWSFrame = false; // Reset
        }
        else {
            const fetched = await Promise.all([
                fetchSafeJson('/api/status', { phaseState: { current_phase: 'IDLE', plan_approved: 'false', task_id: '' } }),
                shouldFetchLowFreq ? fetchSafeJson(`/api/backlog?t=${Date.now()}`, []) : Promise.resolve(lastBacklog),
                fetchSafeJson('/api/swarm', { nodes: [], links: [] }),
                fetchSafeJson('/api/swarm/active-list', []),
                shouldFetchLowFreq ? fetchSafeJson('/api/graph', { nodes: [], links: [] }) : Promise.resolve(lastAST),
                fetchSafeJson(`/api/drift?t=${Date.now()}`, {}),
                shouldFetchLowFreq ? fetchSafeJson(`/api/search?project=${CockpitStore.getState().activeProjectFilter}&q=&category=&tag=All&t=${Date.now()}`, []) : Promise.resolve(lastLessons),
                shouldFetchLowFreq ? fetchSafeJson(`/api/analytics?t=${Date.now()}`, null) : Promise.resolve(lastAnalytics),
                fetchSafeJson(`/api/gitbrain?project=${CockpitStore.getState().activeProjectFilter}&t=${Date.now()}`, null),
                fetchSafeJson('/api/fleet', null)
            ]);
            status = fetched[0];
            backlog = fetched[1];
            swarm = fetched[2];
            activeSwarms = fetched[3];
            ast = fetched[4];
            drift = fetched[5];
            lessons = fetched[6];
            analytics = fetched[7];
            gitbrainData = fetched[8];
            if (fetched.length > 9 && fetched[9]) {
                updateFleetUI(fetched[9]);
                lastFleet = fetched[9];
            }
        }
        if (CockpitStore.getState().activeProjectFilter !== 'ALL' && !(status && status.edition === 'community')) {
            activeSwarms = (activeSwarms || []).filter((sw) => sw.path?.toLowerCase().includes(CockpitStore.getState().activeProjectFilter.toLowerCase()));
        }
        const safeBacklog = backlog || [];
        lastBacklog = safeBacklog;
        lastAST = ast;
        lastLessons = lessons;
        lastAnalytics = analytics;
        lastGitbrainData = gitbrainData;
        activeSwarmsList = activeSwarms;
        window.activeSwarmsList = activeSwarms;
        latestStatus = status;
        window.latestStatus = latestStatus;
        window.lastBacklog = lastBacklog;
        window.latestSwarmData = swarm;
        if (status && status.repoRoot) {
            updateWorkspaceContextDropdownSelection(status.repoRoot);
        }
        if (status && status.version) {
            const vEl = document.getElementById('nomos-version-display');
            if (vEl)
                vEl.textContent = status.version;
        }
        detectStartedTaskWorkers(activeSwarms);
        lastActiveSwarmsList = activeSwarms;
        activeDrift = drift;
        updateTelemetryUI(status);
        updateGitBrainUI(gitbrainData);
        updateLogDropdownWithWorkers(swarm, activeSwarmsList);
        updateWorktreesUI(activeSwarmsList);
        updateHITLBanner(status);
        updateSlotsUI(status.slots);
        window.swarmSlotsData = status.slots;
        updateHeaderTelemetryUI(status);
        // Update LLM LEDs in Header
        (status.inferenceStats || []).forEach((daemon) => {
            const isEmbed = daemon.name && daemon.name.includes('embed');
            const isCoder = daemon.name && daemon.name.includes('coder');
            const ledEl = document.getElementById(isEmbed ? 'led-embed' : (isCoder ? 'led-coder' : ''));
            if (ledEl) {
                const isOnline = daemon.status === 'online';
                const isActive = isOnline && ((daemon.tps || 0) > 0 || (daemon.promptTps || 0) > 0);
                ledEl.className = `led-dot ${daemon.status}`;
                ledEl.style.backgroundColor = isOnline ? 'var(--neon-green)' : '#ef4444';
                ledEl.style.boxShadow = `0 0 8px ${isOnline ? 'var(--neon-green)' : '#ef4444'}`;
                ledEl.style.animation = isActive ? 'pulse-glow 0.8s ease-in-out infinite alternate' : '';
                ledEl.title = `${daemon.name} (${daemon.status}) | Click to ${isOnline ? 'stop' : 'start'}`;
                ledEl.dataset.isOnline = isOnline ? 'true' : 'false';
            }
        });
        // Update GPU Metrics in Header
        const gpuUtilEl = document.getElementById('header-gpu-util');
        const vramValEl = document.getElementById('header-vram-val');
        const gpuPowerEl = document.getElementById('header-gpu-power');
        const tpsValEl = document.getElementById('header-tps-val');
        const workspaceBadge = document.getElementById('workspace-badge');
        if (workspaceBadge) {
            if (status.workspaceName) {
                workspaceBadge.textContent = status.workspaceName;
                workspaceBadge.style.display = 'inline-block';
                if (status.workspaceName === 'PRIVATE') {
                    workspaceBadge.style.background = 'rgba(239, 68, 68, 0.15)'; // Red tint
                    workspaceBadge.style.color = '#ef4444';
                    workspaceBadge.style.border = '1px solid rgba(239, 68, 68, 0.3)';
                }
                else if (status.workspaceName === 'OPEN') {
                    workspaceBadge.style.background = 'rgba(16, 185, 129, 0.15)'; // Green tint
                    workspaceBadge.style.color = 'var(--neon-green)';
                    workspaceBadge.style.border = '1px solid rgba(16, 185, 129, 0.3)';
                }
                else {
                    workspaceBadge.style.background = 'rgba(147, 51, 234, 0.15)';
                    workspaceBadge.style.color = 'var(--neon-purple)';
                    workspaceBadge.style.border = '1px solid rgba(147, 51, 234, 0.3)';
                }
            }
            else {
                workspaceBadge.style.display = 'none';
            }
        }
        if (status.gpu) {
            if (gpuUtilEl)
                gpuUtilEl.textContent = status.gpu.gpuUtil;
            if (vramValEl)
                vramValEl.textContent = status.gpu.vramUsed;
            const powerEl = document.getElementById('header-gpu-power');
            if (powerEl && status.gpu.powerDraw) {
                powerEl.textContent = status.gpu.powerDraw;
            }
        }
        const inferenceStats = status.inferenceStats || [];
        let totalTps = 0;
        for (const stat of inferenceStats) {
            if (stat.tps)
                totalTps += stat.tps;
        }
        const win = window;
        if (!win.maxTotalTps || totalTps > win.maxTotalTps) {
            win.maxTotalTps = totalTps;
        }
        if (tpsValEl)
            tpsValEl.textContent = `${(win.maxTotalTps || 0).toFixed(1)}`;
        const activeTaskId = status.phaseState?.task_id;
        const ideActiveTaskId = status.idePhaseState?.task_id || '';
        window.ideActiveTaskId = ideActiveTaskId;
        updateHeaderContextIndicator();
        const idePhase = status.idePhaseState?.current_phase || status.phaseState?.current_phase || 'IDLE';
        const gbCtxEl = document.getElementById('gitbrain-project-context');
        if (gbCtxEl) {
            const projectName = CockpitStore.getState().activeProjectFilter === 'ALL' ? 'ALL PROJECTS' : CockpitStore.getState().activeProjectFilter;
            gbCtxEl.textContent = projectName;
        }
        const hudStateEl = document.getElementById('hud-state');
        if (hudStateEl) {
            hudStateEl.textContent = idePhase;
            hudStateEl.className = `stat-value stat-${idePhase.toLowerCase()}`;
        }
        const hudSessionIdEl = document.getElementById('hud-session-id');
        if (hudSessionIdEl) {
            const sid = status.phaseState?.task_id;
            hudSessionIdEl.textContent = sid ? sid : 'None';
        }
        const planApproved = status.phaseState?.plan_approved === 'true' || status.phaseState?.plan_approved === true;
        const selectEl = document.getElementById('artifact-type-select');
        if (selectEl) {
            const walkthroughOpt = selectEl.querySelector('option[value="walkthrough"]');
            if (walkthroughOpt) {
                walkthroughOpt.style.display = planApproved ? 'block' : 'none';
            }
            if (!planApproved && getCurrentActiveArtifactType() === 'walkthrough') {
                fetchActiveArtifact('implementation_plan');
            }
        }
        CockpitStore.setState({
            status,
            lastBacklog,
            ideActiveTaskId,
            idePhase,
            lastFleet
        });
        // Render the force-directed AST dependency graph only when OVERVIEW tab is active
        if (activeTab === 'tab-overview' || activeTab === 'tab-ast') {
            renderASTGraph(ast, activeDrift?.modifiedFiles || []);
        }
        // Update the spec parity gauge, alert text, and SVG ring stroke
        updateDriftUI(activeDrift, status.phaseState?.current_phase);
        if (!activeSearchQuery.trim() && activeMemoryCategory === 'All') {
            updateTimelineUI(lessons);
        }
        if (analytics) {
            updateAnalyticsUI(analytics);
        }
        if (activeTab === 'tab-git') {
            fetchGitStatus();
        }
        renderSteeringButtons(status.phaseState.current_phase, status.phaseState.plan_approved);
        if (!selectedLessonHash) {
            const inspector = document.getElementById('memory-node-inspector');
            if (inspector) {
                renderPruningAdvisorDashboard(inspector);
            }
        }
    }
    catch (e) {
        // Fallback error logging
    }
    finally {
        if (!refreshTimeoutId) {
            refreshTimeoutId = setTimeout(refreshData, 1500);
        }
    }
}
// Window load initializer - bootstraps all event listeners, WebSocket connection,
// log streaming, tab switching, drawer resizers, and the initial data poll.
window.addEventListener('load', () => {
    const savedTheme = localStorage.getItem('nomos-theme') || 'nomos-dark';
    document.documentElement.className = `theme-${savedTheme}`;
    const themeSwitcher = document.getElementById('ui-theme-switcher');
    if (themeSwitcher) {
        themeSwitcher.value = savedTheme;
        themeSwitcher.addEventListener('change', () => {
            const selected = themeSwitcher.value;
            document.documentElement.className = `theme-${selected}`;
            localStorage.setItem('nomos-theme', selected);
        });
    }
    // Initialize WebSocket connection and subscribe to real-time push frames.
    // The WS connection provides instant status, backlog, drift, and graph updates
    // without waiting for the next HTTP poll cycle.
    initHeader();
    window.taskBoard = new TaskBoardComponent('backlog-ledger-container');
    initControlPlaneWS((connected) => {
        updateConnectionLED(connected);
    });
    addWSListener((frame) => {
        if (frame.type === 'status') {
            latestStatus = frame.payload;
            window.latestSwarmData = latestStatus.swarm || { nodes: [] };
            activeSwarmsList = latestStatus.worktrees || [];
            if (CockpitStore.getState().activeProjectFilter !== 'ALL' && !(latestStatus && latestStatus.edition === 'community')) {
                activeSwarmsList = activeSwarmsList.filter((sw) => sw.path?.toLowerCase().includes(CockpitStore.getState().activeProjectFilter.toLowerCase()));
            }
            window.activeSwarmsList = activeSwarmsList;
            isWSFrame = true;
            refreshData();
        }
        else if (frame.type === 'backlog') {
            lastBacklog = frame.payload || [];
            isWSFrame = true;
            refreshData();
        }
        else if (frame.type === 'drift') {
            activeDrift = frame.payload;
            isWSFrame = true;
            refreshData();
        }
        else if (frame.type === 'logs') {
            if (frame.log_source === 'gitbrain' || frame.log_source === 'system' || frame.log_source === 'nomos') {
                try {
                    const logObj = JSON.parse(frame.log_text);
                    if (logObj.event_type === 'phase_transition') {
                        if (latestStatus && latestStatus.phaseState) {
                            latestStatus.phaseState.current_phase = logObj.detail || logObj.msg;
                            if (logObj.task_id) {
                                latestStatus.phaseState.task_id = logObj.task_id;
                            }
                            isWSFrame = true;
                            refreshData();
                        }
                    }
                    renderChatMessage('system', logObj.msg || frame.log_text);
                }
                catch (e) {
                    renderChatMessage('system', frame.log_text);
                }
            }
        }
        else if (frame.type === 'graph') {
            lastAST = frame.payload;
            isWSFrame = true;
            refreshData();
        }
    });
    connectLogs('all');
    refreshData(true);
    auditBranches();
    initWorkspaceContextSwitcher();
    document.querySelectorAll('.tab-btn').forEach(btn => {
        btn.addEventListener('click', () => {
            if (btn.classList.contains('disabled-sovereign-tab')) {
                return;
            }
            document.querySelectorAll('.tab-btn').forEach(b => b.classList.remove('active'));
            btn.classList.add('active');
            const targetTab = btn.getAttribute('data-tab');
            if (!targetTab)
                return;
            document.querySelectorAll('.tab-panel').forEach(p => p.classList.remove('active-panel'));
            const activePanel = document.getElementById(targetTab);
            if (activePanel) {
                activePanel.classList.add('active-panel');
            }
            activeTab = targetTab;
            if (targetTab === 'tab-spec-editor') {
                fetchActiveArtifact(getCurrentActiveArtifactType());
            }
            else if (targetTab === 'tab-git') {
                fetchGitStatus();
            }
            else if (targetTab === 'tab-host-os') {
                initNixosUI();
            }
            else if (targetTab === 'tab-architecture') {
                renderArchitectureTopology();
            }
        });
    });
    const btnRefreshArch = document.getElementById('btn-refresh-arch');
    if (btnRefreshArch) {
        btnRefreshArch.addEventListener('click', () => {
            triggerArchitecturePulse();
        });
    }
    // Initialize resizable panel splitters for left/right panel layout
    initSplitters();
    // Register drawer resizer handles for task details and swarm console drawers.
    // Each drawer is independently resizable via mouse drag, with width persisted
    // to localStorage so the user's preferred layout survives page reloads.
    const registerDrawerResizer = (splitterId, drawerId, storageKey, defaultWidth) => {
        const splitter = document.getElementById(splitterId);
        const drawer = document.getElementById(drawerId);
        if (!splitter || !drawer)
            return;
        const savedWidth = localStorage.getItem(storageKey);
        if (savedWidth) {
            drawer.style.width = savedWidth;
            document.documentElement.style.setProperty(`--${storageKey}`, savedWidth);
        }
        else {
            drawer.style.width = defaultWidth;
            document.documentElement.style.setProperty(`--${storageKey}`, defaultWidth);
        }
        splitter.addEventListener('mousedown', (e) => {
            e.preventDefault();
            splitter.classList.add('dragging');
            document.body.classList.add('dragging-active');
            document.body.style.cursor = 'ew-resize';
            const startWidth = drawer.offsetWidth;
            const startX = e.clientX;
            function onMouseMove(moveEvent) {
                const deltaX = moveEvent.clientX - startX;
                let newWidth = startWidth - deltaX;
                if (newWidth < 300)
                    newWidth = 300;
                if (newWidth > window.innerWidth * 0.9)
                    newWidth = window.innerWidth * 0.9;
                drawer.style.width = newWidth + 'px';
                document.documentElement.style.setProperty(`--${storageKey}`, newWidth + 'px');
            }
            function onMouseUp() {
                if (splitter)
                    splitter.classList.remove('dragging');
                document.body.classList.remove('dragging-active');
                document.body.style.cursor = '';
                localStorage.setItem(storageKey, drawer.style.width);
                window.removeEventListener('mousemove', onMouseMove);
                window.removeEventListener('mouseup', onMouseUp);
            }
            window.addEventListener('mousemove', onMouseMove);
            window.addEventListener('mouseup', onMouseUp);
        });
    };
    registerDrawerResizer('task-drawer-splitter', 'task-details-drawer', 'nomos-task-drawer-width', '420px');
    registerDrawerResizer('swarm-drawer-splitter', 'swarm-console-drawer', 'nomos-swarm-drawer-width', '60%');
    const mermaidObj = window.mermaid || window.mermaid;
    if (mermaidObj) {
        mermaidObj.initialize({
            startOnLoad: false,
            theme: 'dark',
            securityLevel: 'loose'
        });
    }
    const textarea = document.getElementById('spec-markdown-textarea');
    const preview = document.getElementById('spec-preview-body-div');
    const syncCheckbox = document.getElementById('sync-scroll-chk');
    let isScrollingTextarea = false;
    let isScrollingPreview = false;
    if (textarea && preview && syncCheckbox) {
        textarea.addEventListener('scroll', () => {
            if (!syncCheckbox.checked)
                return;
            if (isScrollingPreview)
                return;
            isScrollingTextarea = true;
            const maxTextareaScroll = textarea.scrollHeight - textarea.clientHeight;
            if (maxTextareaScroll > 0) {
                const scrollPct = textarea.scrollTop / maxTextareaScroll;
                preview.scrollTop = scrollPct * (preview.scrollHeight - preview.clientHeight);
            }
            setTimeout(() => { isScrollingTextarea = false; }, 50);
        });
        preview.addEventListener('scroll', () => {
            if (!syncCheckbox.checked)
                return;
            if (isScrollingTextarea)
                return;
            isScrollingPreview = true;
            const maxPreviewScroll = preview.scrollHeight - preview.clientHeight;
            if (maxPreviewScroll > 0) {
                const scrollPct = preview.scrollTop / maxPreviewScroll;
                textarea.scrollTop = scrollPct * (textarea.scrollHeight - textarea.clientHeight);
            }
            setTimeout(() => { isScrollingPreview = false; }, 50);
        });
    }
    const kanbanStateToggle = document.getElementById('kanban-state-toggle');
    if (kanbanStateToggle) {
        kanbanStateToggle.addEventListener('change', () => {
        if (window.taskBoard) {
            // TaskBoardComponent updates via store subscription automatically
        }
            refreshData();
        });
    }
    // No kanban search required for community board
    const matrixSort = document.getElementById('matrix-sort');
    if (matrixSort) {
        matrixSort.addEventListener('change', () => {
        if (window.taskBoard) {
            // TaskBoardComponent updates via store subscription automatically
        }
            refreshData();
        });
    }
    const dispatchForm = document.getElementById('dispatch-form');
    if (dispatchForm) {
        dispatchForm.addEventListener('submit', handleDispatchSubmit);
    }
    const showModalBtn = document.getElementById('show-dispatch-modal-btn');
    if (showModalBtn) {
        showModalBtn.addEventListener('click', () => toggleModal(true));
    }
    const closeModalBtn = document.getElementById('close-dispatch-modal-btn');
    if (closeModalBtn) {
        closeModalBtn.addEventListener('click', () => toggleModal(false));
    }
    const saveSpecBtn = document.getElementById('save-spec-btn');
    if (saveSpecBtn) {
        saveSpecBtn.addEventListener('click', saveActiveArtifact);
    }
    const selectArtifact = document.getElementById('artifact-type-select');
    if (selectArtifact) {
        selectArtifact.addEventListener('change', (e) => {
            const val = e.target.value;
            fetchActiveArtifact(val);
        });
    }
    const closeDrawerBtn = document.getElementById('close-drawer-btn');
    if (closeDrawerBtn) {
        closeDrawerBtn.addEventListener('click', closeTaskDetailsDrawer);
    }
    const pinDrawerBtn = document.getElementById('pin-drawer-btn');
    if (pinDrawerBtn) {
        pinDrawerBtn.addEventListener('click', () => {
            const drawer = document.getElementById('task-details-drawer');
            const cockpit = document.querySelector('.cockpit-container');
            const overlay = document.getElementById('drawer-overlay');
            if (drawer && cockpit) {
                drawer.classList.toggle('pinned');
                cockpit.classList.toggle('drawer-pinned');
                if (drawer.classList.contains('pinned')) {
                    if (overlay)
                        overlay.classList.remove('open');
                    pinDrawerBtn.style.opacity = '0.5';
                }
                else {
                    if (overlay)
                        overlay.classList.add('open');
                    pinDrawerBtn.style.opacity = '1';
                }
            }
        });
    }
    const closeSwarmBtn = document.getElementById('close-swarm-drawer-btn');
    if (closeSwarmBtn) {
        closeSwarmBtn.addEventListener('click', closeSwarmConsoleDrawer);
    }
    const chatToggleBtn = document.getElementById('btn-chat-toggle');
    if (chatToggleBtn) {
        chatToggleBtn.addEventListener('click', openSwarmChatDrawer);
    }
    const chatCloseBtn = document.getElementById('btn-chat-close');
    if (chatCloseBtn) {
        chatCloseBtn.addEventListener('click', closeSwarmChatDrawer);
    }
    const drawerOverlay = document.getElementById('drawer-overlay');
    if (drawerOverlay) {
        drawerOverlay.addEventListener('click', () => {
            closeTaskDetailsDrawer();
            closeSwarmConsoleDrawer();
            closeSwarmChatDrawer();
        });
    }
    window.addEventListener('keydown', (e) => {
        if (e.key === 'Escape') {
            closeTaskDetailsDrawer();
            closeSwarmConsoleDrawer();
            closeSwarmChatDrawer();
        }
    });
    const chatInputText = document.getElementById('chat-input-text');
    const chatSendBtn = document.getElementById('btn-chat-send');
    if (chatInputText && chatSendBtn) {
        chatInputText.addEventListener('input', () => {
            chatSendBtn.disabled = !chatInputText.value.trim();
            // Auto-resize textarea height
            chatInputText.style.height = 'auto';
            chatInputText.style.height = (chatInputText.scrollHeight - 16) + 'px';
        });
        chatInputText.addEventListener('keydown', (e) => {
            if (e.key === 'Enter' && !e.shiftKey) {
                e.preventDefault();
                sendUserChatMessage();
            }
        });
    }
    if (chatSendBtn) {
        chatSendBtn.addEventListener('click', sendUserChatMessage);
    }
    const chatDrawer = document.getElementById('swarm-chat-drawer');
    if (chatDrawer) {
        chatDrawer.addEventListener('click', (e) => {
            e.stopPropagation();
        });
    }
    const resizeHandle = document.getElementById('chat-resize-handle');
    if (chatDrawer && resizeHandle) {
        let isResizing = false;
        let startX = 0;
        let startWidth = 0;
        resizeHandle.addEventListener('mousedown', (e) => {
            isResizing = true;
            startX = e.clientX;
            startWidth = chatDrawer.offsetWidth;
            document.body.style.cursor = 'ew-resize';
            document.body.style.userSelect = 'none';
            e.preventDefault();
        });
        window.addEventListener('mousemove', (e) => {
            if (!isResizing)
                return;
            const deltaX = startX - e.clientX;
            const newWidth = Math.max(300, Math.min(window.innerWidth * 0.9, startWidth + deltaX));
            chatDrawer.style.width = `${newWidth}px`;
        });
        window.addEventListener('mouseup', () => {
            if (!isResizing)
                return;
            isResizing = false;
            document.body.style.cursor = '';
            document.body.style.userSelect = '';
        });
    }
    const chatResetBtn = document.getElementById('btn-chat-reset');
    if (chatResetBtn) {
        chatResetBtn.addEventListener('click', () => {
            const container = document.getElementById('chat-messages-container');
            if (container) {
                container.replaceChildren();
                const row = document.createElement('div');
                row.className = 'chat-msg-row system';
                const bubble = document.createElement('div');
                bubble.className = 'chat-bubble';
                bubble.textContent = 'Conduit online. Prompt the active session.';
                row.appendChild(bubble);
                container.appendChild(row);
            }
            clearChatHistoryFromStorage();
        });
    }
    const pruneWorktreeBtn = document.getElementById('btn-prune-worktree');
    if (pruneWorktreeBtn) {
        pruneWorktreeBtn.addEventListener('click', () => {
            pruneWorkspaces();
        });
    }
    const gcpToggleBtn = document.getElementById('gcp-spot-toggle-btn');
    if (gcpToggleBtn) {
        gcpToggleBtn.addEventListener('click', async () => { await handleGcpToggle(gcpToggleBtn); });
    }
    fetchActiveArtifact(getCurrentActiveArtifactType());
});
/**
 * Triggers a click on the corresponding tab button to switch active panel views.
 * Exposed on window for standard HTML onclick handlers.
 *
 * @param targetTab The ID string of the target tab to switch to.
 */
export function switchBottomTab(targetTab) {
    const btn = document.querySelector(`.tab-btn[data-tab="${targetTab}"]`);
    if (btn) {
        btn.click();
    }
}
/**
 * Clears chat history keys from both localStorage and sessionStorage.
 * This is used to reset conversation contexts when pruning workspaces.
 */
function clearChatHistoryFromStorage() {
    [localStorage, sessionStorage].forEach(s => {
        for (let i = s.length - 1; i >= 0; i--) {
            const k = s.key(i);
            if (k && (k.includes('chat') || k.includes('history')))
                s.removeItem(k);
        }
    });
    showToast('Chat history cleared', 'success');
}
// Renders the list of active git worktrees/task workspaces in the sidebar.
function updateWorktreesUI(worktrees) {
    const container = document.getElementById('worktrees-list-container');
    if (!container)
        return;
    container.replaceChildren();
    if (!worktrees || worktrees.length === 0) {
        const empty = document.createElement('div');
        empty.style.fontSize = '0.65rem';
        empty.style.fontStyle = 'italic';
        empty.style.color = 'rgba(255,255,255,0.4)';
        empty.style.textAlign = 'center';
        empty.textContent = 'No active task workspaces.';
        container.appendChild(empty);
        return;
    }
    worktrees.forEach((wt) => {
        if (wt.name === 'nomos-commons' || !wt.name.startsWith('task-')) {
            return;
        }
        const item = document.createElement('div');
        item.style.display = 'flex';
        item.style.flexDirection = 'column';
        item.style.background = 'rgba(255,255,255,0.02)';
        item.style.borderRadius = '4px';
        item.style.padding = '6px 8px';
        item.style.borderLeft = '2px solid var(--neon-blue)';
        item.style.gap = '4px';
        const header = document.createElement('div');
        header.style.display = 'flex';
        header.style.justifyContent = 'space-between';
        header.style.alignItems = 'center';
        const titleSpan = document.createElement('span');
        titleSpan.style.fontFamily = 'monospace';
        titleSpan.style.fontWeight = 'bold';
        titleSpan.style.color = 'var(--text-main)';
        titleSpan.textContent = wt.name.toUpperCase();
        const phaseSpan = document.createElement('span');
        phaseSpan.style.fontSize = '0.55rem';
        phaseSpan.style.fontWeight = '700';
        phaseSpan.style.padding = '1px 4px';
        phaseSpan.style.borderRadius = '2px';
        phaseSpan.style.textTransform = 'uppercase';
        if (wt.phase === 'EDIT') {
            phaseSpan.style.background = 'rgba(16, 185, 129, 0.15)';
            phaseSpan.style.color = 'var(--neon-green)';
        }
        else if (wt.phase === 'REVIEW') {
            phaseSpan.style.background = 'rgba(245, 158, 11, 0.15)';
            phaseSpan.style.color = 'var(--neon-yellow)';
        }
        else if (wt.phase === 'PLAN') {
            phaseSpan.style.background = 'rgba(59, 130, 246, 0.15)';
            phaseSpan.style.color = 'var(--neon-blue)';
        }
        else {
            phaseSpan.style.background = 'rgba(255,255,255,0.05)';
            phaseSpan.style.color = 'var(--text-muted)';
        }
        phaseSpan.textContent = wt.phase || 'IDLE';
        header.appendChild(titleSpan);
        header.appendChild(phaseSpan);
        const branchRow = document.createElement('div');
        branchRow.style.display = 'flex';
        branchRow.style.justifyContent = 'space-between';
        branchRow.style.alignItems = 'center';
        branchRow.style.fontSize = '0.6rem';
        branchRow.style.color = 'var(--text-muted)';
        const branchSpan = document.createElement('span');
        branchSpan.textContent = `Branch: ${wt.branch || 'unknown'}`;
        const pruneBtn = document.createElement('button');
        pruneBtn.textContent = 'Prune';
        pruneBtn.style.background = 'rgba(239, 68, 68, 0.15)';
        pruneBtn.style.color = 'var(--neon-red)';
        pruneBtn.style.border = '1px solid rgba(239, 68, 68, 0.3)';
        pruneBtn.style.padding = '1px 6px';
        pruneBtn.style.borderRadius = '2px';
        pruneBtn.style.cursor = 'pointer';
        pruneBtn.style.fontSize = '0.55rem';
        pruneBtn.addEventListener('click', (e) => {
            e.stopPropagation();
            pruneWorkspaces(wt.path);
        });
        branchRow.appendChild(branchSpan);
        branchRow.appendChild(pruneBtn);
        item.appendChild(header);
        item.appendChild(branchRow);
        container.appendChild(item);
    });
}
/**
 * Helper to build standard action approval buttons with unified styling.
 *
 * @param background The CSS linear gradient value for button background.
 * @param label The text content label on the button.
 * @param onClick The click event callback function.
 * @returns The styled HTMLButtonElement.
 */
function createApproveButton(background, label, onClick) {
    const btn = document.createElement('button');
    btn.className = 'pane-btn save-btn';
    btn.style.background = background;
    btn.style.color = 'var(--text-main)';
    btn.style.border = 'none';
    btn.style.fontSize = '0.7rem';
    btn.style.padding = '3px 12px';
    btn.style.borderRadius = '3px';
    btn.style.cursor = 'pointer';
    btn.style.fontWeight = 'bold';
    btn.textContent = label;
    btn.onclick = () => onClick(btn);
    return btn;
}
// Updates the top HITL plan and completion approval alert banner.
function updateHITLBanner(status) {
    const banner = document.getElementById('hitl-alert-banner');
    const msgEl = document.getElementById('hitl-alert-message');
    const actionsEl = document.getElementById('hitl-alert-actions');
    if (!banner || !msgEl || !actionsEl)
        return;
    const currentPhase = status.phaseState?.current_phase;
    const planApproved = status.phaseState?.plan_approved;
    const taskId = status.phaseState?.task_id || window.ideActiveTaskId || '';
    const isIDE = status.phaseState?.agent_type === 'ide' || status.phaseState?.agent === 'antigravity';
    if (currentPhase === 'PLAN' && planApproved !== 'true') {
        banner.style.display = 'flex';
        if (isIDE) {
            msgEl.textContent = `Waiting for PO Approval on Plan. Task #${taskId} (IDE-driven) is awaiting Plan approval. Please approve this plan directly in your IDE chat session.`;
        }
        else {
            msgEl.textContent = `Waiting for PO Approval on Plan. Task #${taskId} is awaiting Implementation Plan approval.`;
        }
        actionsEl.replaceChildren();
        const viewBtn = document.createElement('button');
        viewBtn.className = 'pane-btn';
        viewBtn.style.background = 'rgba(255,255,255,0.05)';
        viewBtn.style.color = 'var(--text-main)';
        viewBtn.style.border = '1px solid rgba(255,255,255,0.15)';
        viewBtn.style.fontSize = '0.7rem';
        viewBtn.style.padding = '3px 10px';
        viewBtn.style.borderRadius = '3px';
        viewBtn.style.cursor = 'pointer';
        viewBtn.textContent = '👀 View Plan';
        viewBtn.onclick = () => {
            const specTabBtn = document.getElementById('tab-spec') || document.querySelector('[onclick*="switchBottomTab(\'spec\'"]');
            if (specTabBtn)
                specTabBtn.click();
            const leftTabBtn = document.querySelector('[onclick*="switchTab(\'tab-spec-parity\'"]');
            if (leftTabBtn)
                leftTabBtn.click();
        };
        actionsEl.appendChild(viewBtn);
        if (!isIDE) {
            const approveBtn = createApproveButton('linear-gradient(135deg, var(--neon-purple), var(--neon-pink))', '✅ Approve Plan & Enter EDIT', async (btn) => { await handlePlanApprove(btn); });
            actionsEl.appendChild(approveBtn);
        }
    }
    else if (currentPhase === 'REVIEW') {
        banner.style.display = 'flex';
        if (isIDE) {
            msgEl.textContent = `Task Complete. Waiting for PO Verification. Task #${taskId} (IDE-driven) requires verification. Please approve this commit directly in your IDE chat session.`;
        }
        else {
            msgEl.textContent = `Task Complete. Waiting for PO Verification. Task #${taskId} is complete and awaiting review and sign-off.`;
        }
        actionsEl.replaceChildren();
        const viewLogsBtn = document.createElement('button');
        viewLogsBtn.className = 'pane-btn';
        viewLogsBtn.style.background = 'rgba(255,255,255,0.05)';
        viewLogsBtn.style.color = 'var(--text-main)';
        viewLogsBtn.style.border = '1px solid rgba(255,255,255,0.15)';
        viewLogsBtn.style.fontSize = '0.7rem';
        viewLogsBtn.style.padding = '3px 10px';
        viewLogsBtn.style.borderRadius = '3px';
        viewLogsBtn.style.cursor = 'pointer';
        viewLogsBtn.style.marginRight = '8px';
        viewLogsBtn.textContent = '📋 View Logs';
        viewLogsBtn.onclick = () => {
            if (taskId) {
                openSwarmConsoleDrawer(String(taskId), String(taskId));
            }
            else {
                showToast('No active worker task context linked.', 'error');
            }
        };
        actionsEl.appendChild(viewLogsBtn);
        const viewDiffBtn = document.createElement('button');
        viewDiffBtn.className = 'pane-btn';
        viewDiffBtn.style.background = 'rgba(255,255,255,0.05)';
        viewDiffBtn.style.color = 'var(--text-main)';
        viewDiffBtn.style.border = '1px solid rgba(255,255,255,0.15)';
        viewDiffBtn.style.fontSize = '0.7rem';
        viewDiffBtn.style.padding = '3px 10px';
        viewDiffBtn.style.borderRadius = '3px';
        viewDiffBtn.style.cursor = 'pointer';
        viewDiffBtn.textContent = '🔍 View Git Changes';
        viewDiffBtn.onclick = () => {
            switchBottomTab('tab-git');
        };
        actionsEl.appendChild(viewDiffBtn);
        if (!isIDE) {
            const approveBtn = createApproveButton('linear-gradient(135deg, var(--neon-green), var(--neon-blue))', '🚀 Approve & Commit/Push', async (btn) => { await handleTaskSignoff(btn); });
            actionsEl.appendChild(approveBtn);
        }
    }
    else {
        banner.style.display = 'none';
    }
}
// Renders a single inference slot state row showing lock status, task binding,
// and branch association for the concurrency slot management panel.
function renderSlotStateNode(slotState, listEl) {
    const container = document.createElement('div');
    container.style.display = 'flex';
    container.style.flexDirection = 'column';
    container.style.background = 'rgba(255,255,255,0.02)';
    container.style.borderRadius = '4px';
    container.style.margin = '4px 0';
    container.style.padding = '4px 8px';
    const item = document.createElement('div');
    item.style.display = 'flex';
    item.style.alignItems = 'center';
    item.style.justifyContent = 'space-between';
    item.style.width = '100%';
    const labelSpan = document.createElement('span');
    labelSpan.style.fontFamily = 'monospace';
    labelSpan.style.fontSize = '0.75rem';
    const statusSpan = document.createElement('span');
    statusSpan.style.fontSize = '0.65rem';
    statusSpan.style.fontWeight = '700';
    statusSpan.style.textTransform = 'uppercase';
    statusSpan.style.padding = '2px 6px';
    statusSpan.style.borderRadius = '3px';
    if (slotState.status === 'LOCKED') {
        container.style.borderLeft = '2px solid var(--neon-purple)';
        labelSpan.style.color = 'var(--text-main)';
        labelSpan.style.fontWeight = '600';
        labelSpan.textContent = `Slot ${slotState.slot}: LOCKED by Task #${slotState.taskID}`;
        statusSpan.style.background = 'rgba(16, 85, 247, 0.15)';
        statusSpan.style.color = 'var(--neon-purple)';
        statusSpan.textContent = 'LOCKED';
        item.appendChild(labelSpan);
        item.appendChild(statusSpan);
        container.appendChild(item);
        if (slotState.folderName) {
            const subItem = document.createElement('div');
            subItem.style.fontSize = '0.65rem';
            subItem.style.color = 'var(--text-muted)';
            subItem.style.paddingLeft = '0.75rem';
            subItem.style.marginTop = '2px';
            subItem.style.fontFamily = 'monospace';
            subItem.textContent = `└─ ${slotState.folderName} (Branch: ${slotState.branch || 'unknown'})`;
            container.appendChild(subItem);
        }
    }
    else {
        container.style.borderLeft = '2px solid var(--neon-green)';
        labelSpan.style.color = 'rgba(255,255,255,0.5)';
        labelSpan.textContent = `Slot ${slotState.slot}: FREE`;
        statusSpan.style.background = 'rgba(16, 185, 129, 0.15)';
        statusSpan.style.color = 'var(--neon-green)';
        statusSpan.textContent = 'FREE';
        item.appendChild(labelSpan);
        item.appendChild(statusSpan);
        container.appendChild(item);
    }
    listEl.appendChild(container);
}
// Handles daemon start/stop toggle button clicks by sending a signal to the backend
// and temporarily disabling the button to prevent double-clicks.
async function handleDaemonToggle(daemonName, isOnline, btn) {
    btn.disabled = true;
    const action = isOnline ? 'stop' : 'start';
    btn.textContent = isOnline ? 'Stopping...' : 'Starting...';
    const simpleName = daemonName.replace('llama-server-', '');
    try {
        const resp = await fetch(`/api/swarm/toggle?daemon=${simpleName}&action=${action}`);
        if (resp.ok) {
            showToast(`Daemon ${daemonName} ${action} command sent.`, 'success');
        }
        else {
            showToast(`Failed to toggle daemon ${daemonName}.`, 'error');
        }
    }
    catch (err) {
        showToast(`Error: ${err.message}`, 'error');
    }
    finally {
        setTimeout(refreshData, 1000);
    }
}
// Renders a single inference daemon badge card showing name, port, model,
// context window size, KV cache usage, VRAM/RAM stats, and a start/stop toggle.
function renderDaemonBadge(daemon, daemonContainer) {
    const dBadge = document.createElement('div');
    dBadge.className = 'daemon-badge';
    dBadge.style.display = 'flex';
    dBadge.style.flexDirection = 'column';
    dBadge.style.alignItems = 'stretch';
    dBadge.style.width = '100%';
    dBadge.style.padding = '8px';
    dBadge.style.background = 'rgba(255,255,255,0.03)';
    dBadge.style.borderRadius = '4px';
    dBadge.style.margin = '4px 0';
    dBadge.style.border = '1px solid rgba(255,255,255,0.05)';
    dBadge.style.gap = '6px';
    const leftDiv = document.createElement('div');
    leftDiv.style.display = 'flex';
    leftDiv.style.flexDirection = 'column';
    leftDiv.style.gap = '2px';
    const nameSpan = document.createElement('span');
    nameSpan.className = 'daemon-name';
    nameSpan.textContent = `${daemon.name}`;
    nameSpan.style.fontSize = '0.8rem';
    nameSpan.style.fontWeight = 'bold';
    const portSpan = document.createElement('span');
    portSpan.textContent = `Port: ${daemon.port}`;
    portSpan.style.fontSize = '0.65rem';
    portSpan.style.color = 'var(--text-muted)';
    leftDiv.appendChild(nameSpan);
    leftDiv.appendChild(portSpan);
    if (daemon.status === 'online') {
        if (daemon.model) {
            const modelSpan = document.createElement('span');
            modelSpan.style.fontSize = '0.65rem';
            modelSpan.style.color = 'var(--neon-blue)';
            modelSpan.style.wordBreak = 'break-all';
            modelSpan.style.marginTop = '2px';
            modelSpan.textContent = `Model: ${daemon.model}`;
            leftDiv.appendChild(modelSpan);
        }
        if (daemon.nCtx || daemon.kvCache !== undefined) {
            const ctxSpan = document.createElement('span');
            ctxSpan.style.fontSize = '0.65rem';
            ctxSpan.style.color = 'var(--text-muted)';
            const ctxStr = daemon.nCtx ? formatContextSize(daemon.nCtx) : '-';
            const kvStr = daemon.kvCache !== undefined ? `${daemon.kvCache.toFixed(1)}%` : '-';
            ctxSpan.textContent = `Ctx: ${ctxStr} | KV: ${kvStr}`;
            leftDiv.appendChild(ctxSpan);
        }
        if (daemon.vram || daemon.memory) {
            const memSpan = document.createElement('span');
            memSpan.style.fontSize = '0.65rem';
            memSpan.style.color = 'var(--text-muted)';
            const vramStr = daemon.vram || '-';
            memSpan.textContent = `VRAM: ${vramStr} | RAM: ${daemon.memory}`;
            leftDiv.appendChild(memSpan);
        }
    }
    const rightDiv = document.createElement('div');
    rightDiv.style.display = 'flex';
    rightDiv.style.alignItems = 'center';
    rightDiv.style.justifyContent = 'space-between';
    rightDiv.style.gap = '8px';
    const statusSpan = document.createElement('span');
    statusSpan.className = `daemon-status ${daemon.status}`;
    statusSpan.textContent = daemon.status.toUpperCase();
    statusSpan.style.fontSize = '0.7rem';
    statusSpan.style.padding = '2px 6px';
    statusSpan.style.borderRadius = '3px';
    const toggleBtn = document.createElement('button');
    const isOnline = daemon.status === 'online';
    toggleBtn.textContent = isOnline ? 'Stop' : 'Start';
    toggleBtn.style.background = isOnline ? 'rgba(239, 68, 68, 0.15)' : 'rgba(59, 130, 246, 0.15)';
    toggleBtn.style.color = isOnline ? 'var(--neon-red)' : 'var(--neon-blue)';
    toggleBtn.style.border = isOnline ? '1px solid rgba(239, 68, 68, 0.3)' : '1px solid rgba(59, 130, 246, 0.3)';
    toggleBtn.style.padding = '2px 8px';
    toggleBtn.style.borderRadius = '3px';
    toggleBtn.style.fontSize = '0.7rem';
    toggleBtn.style.cursor = 'pointer';
    toggleBtn.style.transition = 'all 0.2s';
    toggleBtn.addEventListener('click', (e) => {
        e.stopPropagation();
        handleDaemonToggle(daemon.name, isOnline, toggleBtn);
    });
    rightDiv.appendChild(statusSpan);
    rightDiv.appendChild(toggleBtn);
    dBadge.appendChild(leftDiv);
    dBadge.appendChild(rightDiv);
    daemonContainer.appendChild(dBadge);
}
// Detects newly started swarm task workers by diffing the current active list
// against the previous poll cycle's list. When a new worker is detected,
// auto-selects its sovereign context so the dashboard tracks it immediately.
function detectStartedTaskWorkers(activeSwarms) {
    activeSwarms.forEach((sw) => {
        if (!sw.id || sw.status !== 'running')
            return;
        const wasRunning = lastActiveSwarmsList.some((p) => p.id === sw.id && p.status === 'running');
        if (wasRunning)
            return;
        const taskId = parseInt(sw.id.split('-')[0], 10);
        if (!isNaN(taskId)) {
            console.log(`Auto-binding sovereign context for starting worker task: #${taskId}`);
        }
    });
}
/**
 * Fetches workspace-scoped inspect data (phase state + drift) for a sovereign context.
 * Used when the user has selected a specific worktree to view its isolated telemetry.
 *
 * @param path The absolute path to the target worktree workspace.
 * @returns A promise resolving to the inspect data object, or null on failure.
 */
async function fetchInspectData(path) {
    try {
        const data = await fetchSafeJson(`/api/swarm/inspect?path=${encodeURIComponent(path)}`, null);
        if (data && data.phaseState && data.drift)
            return data;
    }
    catch (err) {
        console.error('Failed to fetch inspect data:', err);
    }
    return null;
}
/**
 * Updates the concurrency slots UI panel with current slot lock states.
 * Supports both cloud-mode (simple counter) and local-mode (per-slot detail cards).
 *
 * @param slots The slots metadata object containing type, limits, and states.
 */
function updateSlotsUI(slots) {
    const listEl = document.getElementById('slots-list-container');
    if (!listEl || !slots)
        return;
    listEl.replaceChildren();
    if (slots.type === 'cloud') {
        const dText = document.createElement('div');
        dText.style.color = 'var(--neon-blue)';
        dText.style.fontWeight = '600';
        dText.style.marginTop = '0.25rem';
        dText.textContent = `Active Concurrencies: ${slots.used}`;
        listEl.appendChild(dText);
        return;
    }
    if (slots.slotStates && slots.slotStates.length > 0) {
        slots.slotStates.forEach((slotState) => renderSlotStateNode(slotState, listEl));
    }
    else {
        const emptyLabel = document.createElement('div');
        emptyLabel.style.fontSize = '0.7rem';
        emptyLabel.style.fontStyle = 'italic';
        emptyLabel.style.color = 'rgba(255,255,255,0.4)';
        emptyLabel.style.textAlign = 'center';
        emptyLabel.textContent = 'No active slots found.';
        listEl.appendChild(emptyLabel);
    }
}
// --- Global Window Method Exports ---
// Expose key functions on the window object so they can be called from
// inline HTML event handlers, cross-module callbacks, and the browser console.
// This is the canonical binding point for all interactive dashboard actions.
window.changeLogSource = changeLogSource;
window.openSwarmConsoleDrawer = openSwarmConsoleDrawer;
window.closeSwarmConsoleDrawer = closeSwarmConsoleDrawer;
// Expose methods globally for callback routes
window.CockpitStore = CockpitStore;
window.refreshData = refreshData;
window.handleHeaderDaemonToggle = async (daemonName, ledEl) => {
    if (ledEl.style.opacity === '0.5')
        return; // Debounce
    const isOnline = ledEl.dataset.isOnline === 'true';
    const action = isOnline ? 'stop' : 'start';
    ledEl.style.opacity = '0.5';
    const simpleName = daemonName.replace('llama-server-', '');
    try {
        const resp = await fetch(`/api/swarm/toggle?daemon=${simpleName}&action=${action}`);
        if (resp.ok) {
            showToast(`Daemon ${daemonName} ${action} command sent.`, 'success');
        }
        else {
            showToast(`Failed to toggle daemon ${daemonName}.`, 'error');
        }
    }
    catch (err) {
        showToast(`Error: ${err.message}`, 'error');
    }
    finally {
        ledEl.style.opacity = '1';
        setTimeout(refreshData, 1000);
    }
};
window.openTaskDetailsDrawer = openTaskDetailsDrawer;
window.openAgentDetailsDrawer = openAgentDetailsDrawer;
window.closeTaskDetailsDrawer = closeTaskDetailsDrawer;
window.toggleModal = toggleModal;
window.handleDispatchSubmit = handleDispatchSubmit;
window.fetchActiveArtifact = fetchActiveArtifact;
window.saveActiveArtifact = saveActiveArtifact;
window.pruneWorkspaces = pruneWorkspaces;
window.auditBranches = auditBranches;
window.pruneBranch = pruneBranch;
window.pruneAllBranches = pruneAllBranches;
window.switchBottomTab = switchBottomTab;
window.updateHeaderContextIndicator = updateHeaderContextIndicator;
window.openSwarmChatDrawer = openSwarmChatDrawer;
window.closeSwarmChatDrawer = closeSwarmChatDrawer;
window.sendUserChatMessage = sendUserChatMessage;
// GitBrain Memory Bindings
window.onMemorySearchChange = onMemorySearchChange;
window.filterMemoryByCategory = filterMemoryByCategory;
window.filterMemoryByTag = filterMemoryByTag;
window.lastBacklog = lastBacklog;
window.latestStatus = latestStatus;
// Page Visibility API optimization for Task 325 GPU Power Draw
document.addEventListener('visibilitychange', () => {
    if (document.hidden) {
        window.__isWebUiTabHidden = true;
    }
    else {
        window.__isWebUiTabHidden = false;
        refreshData();
    }
});
function showHITLAlertBanner(taskId, msg) {
    const banner = document.getElementById('hitl-alert-banner');
    const msgEl = document.getElementById('hitl-alert-msg');
    const approveBtn = document.getElementById('hitl-approve-btn');
    if (banner && msgEl && approveBtn) {
        msgEl.textContent = `[HITL Action Required] ${msg}`;
        approveBtn.textContent = `Approve & Release ${taskId}`;
        approveBtn.onclick = async () => {
            try {
                await fetch(`/api/phase/transition`, {
                    method: 'POST',
                    headers: { 'Content-Type': 'application/json' },
                    body: JSON.stringify({ phase: 'REVIEW' })
                });
                await fetch(`/api/task/close`, {
                    method: 'POST',
                    headers: { 'Content-Type': 'application/json' },
                    body: JSON.stringify({ task_id: taskId })
                });
                banner.style.display = 'none';
                refreshData(true);
            }
            catch (e) {
                console.error('Failed to approve & release task:', e);
            }
        };
        banner.style.display = 'flex';
    }
}
window.showHITLAlertBanner = showHITLAlertBanner;
// Real-time EventBus Listener for zero-latency backlog updates (<50ms) & Tier 2 HITL Alerts
addWSListener((frame) => {
    if (frame && (frame.type === 'backlog_invalidated' || frame.type === 'task_updated')) {
        lastBacklog = [];
        refreshData(true);
    }
    if (frame && frame.type === 'hitl_action_required') {
        try {
            const payload = typeof frame.payload === 'string' ? JSON.parse(frame.payload) : frame.payload;
            showHITLAlertBanner(payload.task_id || 'Task', payload.message || 'Worker task completed');
        }
        catch (e) {
            console.error('Failed to parse hitl_action_required payload:', e);
        }
    }
});
window.toggleSidebar = function () {
    const container = document.querySelector('.cockpit-container');
    if (container) {
        const isCollapsed = container.classList.toggle('sidebar-collapsed');
        localStorage.setItem('nomos-sidebar-collapsed', isCollapsed ? 'true' : 'false');
    }
};
// Initialize sidebar state from localStorage
const sidebarCollapsed = localStorage.getItem('nomos-sidebar-collapsed');
if (sidebarCollapsed === 'true') {
    const container = document.querySelector('.cockpit-container');
    if (container) {
        container.classList.add('sidebar-collapsed');
    }
}

new CreativeWizard();
