// ui_telemetry.ts - UI display controllers and visual telemetry indicators
// This module manages formatting utility helpers and rendering logic for various
// telemetry indicators including connections, drift gauges, and analytics charts.

// Formats a relative timestamp (e.g. "3m ago") from a given ISO Date string.
// Computes difference between now and the timestamp and converts to human readable format.
export function getRelativeTime(isoString: string): string {
  if (!isoString) return '';
  const date = new Date(isoString);
  const now = new Date();
  const diffMs = now.getTime() - date.getTime();
  const diffSec = Math.floor(diffMs / 1000);
  const diffMin = Math.floor(diffSec / 60);
  const diffHr = Math.floor(diffMin / 60);
  const diffDays = Math.floor(diffHr / 24);

  // Return relative label matching elapsed thresholds
  if (diffSec < 60) return 'just now';
  if (diffMin < 60) return `${diffMin}m ago`;
  if (diffHr < 24) return `${diffHr}h ago`;
  return `${diffDays}d ago`;
}

// Updates the connection status LED dot in the dashboard header.
// Changes background color and glow shadow depending on online/offline state.
export function updateConnectionLED(isOnline: boolean): void {
  const led = document.getElementById('connection-led');
  if (led) {
    if (isOnline) {
      // Set to glowing neon-green
      led.style.backgroundColor = '#10b981';
      led.style.boxShadow = '0 0 8px #10b981';
      led.setAttribute('title', 'Online - Connected to Swarm backend');
      led.className = 'led-dot online';
    } else {
      // Set to glowing neon-red
      led.style.backgroundColor = '#ef4444';
      led.style.boxShadow = '0 0 8px #ef4444';
      led.setAttribute('title', 'Offline - Reconnecting...');
      led.className = 'led-dot offline';
    }
  }
}

// Formats large context window integer numbers into abbreviated text (e.g. 32K, 1M).
// Used to display llama inference server settings clearly in badges.
export function formatContextSize(nCtx: number): string {
  if (nCtx <= 0) return '-';
  if (nCtx >= 1024 * 1024) return `${(nCtx / (1024 * 1024)).toFixed(0)}M`;
  if (nCtx >= 1024) return `${(nCtx / 1024).toFixed(0)}K`;
  return String(nCtx);
}

// Updates the visual Spec Parity Alignment gauge and drift warnings.
// Uses a single source of truth (driftScore) to sync the circular SVG ring,
// the percentage label, the plan approval status, and the alert notifications.
export function updateDriftUI(drift: any, currentPhase: string): void {
  // Check if workspace is currently idle (no active task or no declared files)
  const isIdle = (!currentPhase || currentPhase === 'IDLE' || !drift.declaredFiles || drift.declaredFiles.length === 0);

  // Compute final alignment percentage based on driftScore (0% drift = 100% alignment)
  let alignmentScore = 0;
  if (!isIdle && drift.isPlanApproved === 'true') {
    alignmentScore = Math.max(0, Math.min(100, 100 - (drift.driftScore || 0)));
  }
  
  // Update percentage text label in center of gauge
  const textEl = null;
  if (textEl) {
    textEl.textContent = isIdle ? '—' : `${alignmentScore}%`;
    textEl.style.color = isIdle ? 'var(--neon-purple)' : 'var(--text-main)';
  }
  
  // Update plan approval value and text color in panel footer
  const approvedEl = document.getElementById('drift-approved-val');
  if (approvedEl) {
    approvedEl.textContent = isIdle ? 'false' : drift.isPlanApproved;
    const color = isIdle ? 'var(--neon-purple)' : (drift.isPlanApproved === 'true' ? 'var(--neon-green)' : 'var(--neon-yellow)');
    approvedEl.style.color = color;
  }
  
  // Recalculate SVG circle dashoffset stroke based on alignment score
  const circle = null;
  if (circle) {
    const offset = isIdle ? 314.16 : 314.16 * (1 - alignmentScore / 100);
    (circle as unknown as SVGElement).style.strokeDashoffset = String(offset);
    
    // Set circle stroke color depending on current alignment ranges
    let strokeColor = 'var(--neon-green)';
    if (isIdle) {
      strokeColor = 'var(--neon-purple)';
    } else if (drift.isPlanApproved !== 'true') {
      strokeColor = 'var(--neon-yellow)';
    } else {
      if (alignmentScore < 100 && alignmentScore >= 85) strokeColor = 'var(--neon-blue)';
      if (alignmentScore < 85 && alignmentScore >= 50) strokeColor = 'var(--neon-yellow)';
      if (alignmentScore < 50) strokeColor = 'var(--neon-red)';
    }
    circle.setAttribute('stroke', strokeColor);
  }

  // Render compliance alerts list container below gauge
  const alertContainer = document.getElementById('drift-alerts-container');
  if (alertContainer) {
    alertContainer.replaceChildren();

    if (isIdle) {
      // Idle state info
      const info = document.createElement('div');
      info.style.color = 'var(--neon-purple)';
      info.style.fontSize = '0.7rem';
      info.style.fontStyle = 'italic';
      info.textContent = 'Workspace is idle. No task is active.';
      alertContainer.appendChild(info);
    } else if (drift.isPlanApproved !== 'true') {
      // Plan awaiting human PO approval
      const warning = document.createElement('div');
      warning.style.color = 'var(--neon-yellow)';
      warning.style.fontSize = '0.7rem';
      warning.style.fontWeight = 'bold';
      warning.textContent = '⚠️ Spec Plan Pending PO Approval';
      alertContainer.appendChild(warning);
    } else if (drift.driftScore > 0) {
      // Drift detected alert
      const warning = document.createElement('div');
      warning.style.color = 'var(--neon-red)';
      warning.style.fontSize = '0.7rem';
      warning.style.fontWeight = 'bold';
      warning.textContent = `🚨 SPEC DRIFT DETECTED: Score ${drift.driftScore}%`;
      alertContainer.appendChild(warning);
    } else {
      // Perfect compliance success message
      const success = document.createElement('div');
      success.style.color = 'var(--neon-green)';
      success.style.fontSize = '0.7rem';
      success.style.fontWeight = 'bold';
      success.textContent = '✅ Compliance: Code in perfect sync with Spec';
      alertContainer.appendChild(success);
    }
  }
}

// Updates the agent velocity, success rate, and sprints chart UI.
// Renders an inline SVG spline chart dynamically reflecting points completed.
export function updateAnalyticsUI(data: any): void {
  // Update standard statistical counter nodes
  const commitsEl = document.getElementById('analytics-commits-val');
  if (commitsEl) commitsEl.textContent = String(data.totalCommits || 0);
  
  const rateEl = document.getElementById('analytics-success-rate-val');
  if (rateEl) rateEl.textContent = `${data.pipelineSuccessRate || 100}%`;
  
  const failuresEl = document.getElementById('analytics-failures-val');
  if (failuresEl) failuresEl.textContent = String(data.dodFailures || 0);
  
  const bypassEl = document.getElementById('analytics-bypass-val');
  if (bypassEl) bypassEl.textContent = `${data.bypassRatio || 0}%`;

  const chartContainer = document.getElementById('analytics-chart-container');
  if (!chartContainer) return;

  const width = chartContainer.clientWidth || 400;
  const height = chartContainer.clientHeight || 180;
  
  const velocity = data.velocity || [];
  if (velocity.length === 0) {
    // Show placeholder text when no velocity history is found
    const placeholder = document.createElement('div');
    placeholder.style.color = 'var(--text-muted)';
    placeholder.style.fontSize = '0.8rem';
    placeholder.style.display = 'flex';
    placeholder.style.alignItems = 'center';
    placeholder.style.justifyContent = 'center';
    placeholder.style.height = '100%';
    placeholder.textContent = 'No release velocity data available.';
    chartContainer.replaceChildren(placeholder);
    return;
  }

  // Construct chart vector SVG with linear gradients and layout constraints
  let svgContent = `<svg xmlns="http://www.w3.org/2000/svg" width="100%" height="100%" viewBox="0 0 ${width} ${height}" preserveAspectRatio="none" style="overflow: visible;">`;
  svgContent += `
    <defs>
      <linearGradient id="chart-line-grad" x1="0%" y1="0%" x2="0%" y2="100%">
        <stop offset="0%" stop-color="var(--neon-blue)" stop-opacity="1" />
        <stop offset="100%" stop-color="var(--neon-purple)" stop-opacity="1" />
      </linearGradient>
      <linearGradient id="chart-area-grad" x1="0%" y1="0%" x2="0%" y2="100%">
        <stop offset="0%" stop-color="var(--neon-blue)" stop-opacity="0.25" />
        <stop offset="100%" stop-color="var(--neon-purple)" stop-opacity="0.0" />
      </linearGradient>
    </defs>
  `;

  // Dynamic layout margins
  const paddingLeft = 35;
  const paddingRight = 15;
  const paddingTop = 15;
  const paddingBottom = 25;

  const chartWidth = width - paddingLeft - paddingRight;
  const chartHeight = height - paddingTop - paddingBottom;

  const maxSp = Math.max(...velocity.map((v: any) => v.sp || 1), 5);
  const pointsCount = velocity.length;

  // Render vertical ticks and value labels
  const yTicks = 4;
  for (let i = 0; i <= yTicks; i++) {
    const value = Math.round((maxSp / yTicks) * i);
    const y = height - paddingBottom - (chartHeight / yTicks) * i;
    svgContent += `
      <line x1="${paddingLeft}" y1="${y}" x2="${width - paddingRight}" y2="${y}" stroke="rgba(255,255,255,0.05)" stroke-width="1" />
      <text x="${paddingLeft - 8}" y="${y + 4}" fill="var(--text-muted)" font-size="9" text-anchor="end" font-family="'JetBrains Mono', monospace">${value} SP</text>
    `;
  }

  // Calculate coordinates for points in spline
  const points = velocity.map((v: any, index: number) => {
    const x = paddingLeft + (pointsCount > 1 ? (chartWidth / (pointsCount - 1)) * index : chartWidth / 2);
    const y = height - paddingBottom - (chartHeight * (v.sp / maxSp));
    return { x, y, sprint: v.sprint, sp: v.sp, taskId: v.taskId };
  });

  let linePath = '';
  let areaPath = '';

  if (points.length > 0) {
    linePath = `M ${points[0].x} ${points[0].y}`;
    areaPath = `M ${points[0].x} ${height - paddingBottom} L ${points[0].x} ${points[0].y}`;

    for (let i = 1; i < points.length; i++) {
      linePath += ` L ${points[i].x} ${points[i].y}`;
      areaPath += ` L ${points[i].x} ${points[i].y}`;
    }
    
    areaPath += ` L ${points[points.length - 1].x} ${height - paddingBottom} Z`;
  }

  // Append background fill area and main spline stroke to SVG string
  if (areaPath) {
    svgContent += `<path d="${areaPath}" fill="url(#chart-area-grad)" />`;
  }

  if (linePath) {
    svgContent += `<path d="${linePath}" fill="none" stroke="url(#chart-line-grad)" stroke-width="2" />`;
  }

  // Render interactive nodes and value badges
  points.forEach((p: any) => {
    svgContent += `
      <text x="${p.x}" y="${height - 8}" fill="var(--text-muted)" font-size="8" text-anchor="middle" font-family="'JetBrains Mono', monospace">${p.sprint}</text>
    `;

    svgContent += `
      <circle cx="${p.x}" cy="${p.y}" r="4" fill="var(--bg-dark)" stroke="var(--neon-blue)" stroke-width="2" style="cursor: pointer;" />
      <circle cx="${p.x}" cy="${p.y}" r="7" fill="var(--neon-blue)" fill-opacity="0.2" style="cursor: pointer;" />
    `;

    svgContent += `
      <text x="${p.x}" y="${p.y - 8}" fill="var(--text-normal)" font-size="9" font-weight="bold" text-anchor="middle" font-family="'JetBrains Mono', monospace">${p.sp} pts</text>
    `;
  });

  svgContent += `</svg>`;
  
  // Parse constructed SVG string into DOM nodes and inject into chart container
  const parser = new DOMParser();
  const doc = parser.parseFromString(svgContent, 'image/svg+xml');
  const svgElement = document.importNode(doc.documentElement, true);
  chartContainer.replaceChildren(svgElement);
}

export function updateFleetUI(fleetData: any): void {
  const container = document.getElementById('fleet-list-container');
  if (!container) return;
  
  if (!fleetData || !fleetData.neighbors || fleetData.neighbors.length === 0) {
    container.innerHTML = '<span style="font-size: 0.65rem; color: var(--text-muted);">No fleet neighbors found.</span>';
    return;
  }
  
  let html = '';
  fleetData.neighbors.forEach((n: any) => {
    let color = 'var(--neon-purple)';
    if (n.phase === 'PLAN') color = 'var(--neon-blue)';
    if (n.phase === 'EDIT') color = 'var(--neon-yellow)';
    
    html += `
      <div style="display: flex; justify-content: space-between; align-items: center; background: rgba(255,255,255,0.02); padding: 4px 6px; border-radius: 4px; border: 1px solid rgba(255,255,255,0.05);">
        <span style="font-size: 0.7rem; color: var(--text-main); font-family: 'JetBrains Mono', monospace;">${n.name}</span>
        <span style="font-size: 0.6rem; font-weight: bold; padding: 2px 4px; border-radius: 2px; background: rgba(255,255,255,0.05); color: ${color};">${n.phase}</span>
      </div>
    `;
  });
  
  container.innerHTML = html;
}

export function updateGitBrainUI(gitbrainData: any): void {
  const countEl = document.getElementById('nebula-nodes-count');
  if (countEl) {
    if (gitbrainData) {
      countEl.textContent = `${gitbrainData.vectorsCount || 0} nodes`;
    } else {
      countEl.textContent = `0 nodes`;
    }
  }
}



export function updateHeaderTelemetryUI(status: any): void {
  const container = document.getElementById('header-telemetry-container');
  if (!container) return;
  
  if (!status || !status.inferenceStats || status.inferenceStats.length === 0) {
    container.textContent = 'No telemetry available';
    return;
  }
  
	let html = '<div style="display: flex; flex-direction: column; align-items: flex-end; gap: 4px; padding: 4px 6px; border: 1px solid var(--border-indigo); border-radius: 4px; background: rgba(var(--glass-rgb), 0.05);">';
	status.inferenceStats.forEach((stat: any, index: number, array: any[]) => {
		const isEmbed = stat.name && stat.name.includes('embed');
		const ledId = isEmbed ? 'led-embed' : 'led-coder';
		const isOnline = stat.status === 'online';
		const activeTps = Math.max(stat.tps || 0, stat.promptTps || 0);
		html += `
			<div style="display: flex; gap: 8px; align-items: center; white-space: nowrap;">
				<div id="${ledId}" class="led-dot ${stat.status}" onclick="window.handleHeaderDaemonToggle && window.handleHeaderDaemonToggle('${stat.name}', this)" style="width: 10px; height: 10px; border-radius: 50%; background-color: ${isOnline ? 'var(--neon-green)' : '#ef4444'}; box-shadow: 0 0 8px ${isOnline ? 'var(--neon-green)' : '#ef4444'}; cursor: pointer; transition: all 0.3s ease-in-out;"></div>
				<span style="color: var(--neon-cyan);">${stat.model}</span>
				<span style="font-weight: bold; color: var(--text-main);">${Math.round(activeTps)} TPS</span>
				<span style="color: var(--text-muted);">Q: ${stat.queueLength || 0}</span>
			</div>
		`;
	});
	html += '</div>';
	container.innerHTML = html;
}

// --- Swarm Execution Matrix ---

export interface SwarmTelemetryEvent {
  ts: string;
  agent: string;
  model: string;
  domain: string;
  status: string;
  duration: number;
  tool_calls: number;
}

const swarmEvents: SwarmTelemetryEvent[] = [];

export function processSwarmTelemetryEvent(logObj: any): void {
  if (logObj.source !== 'swarm_telemetry' && logObj.agent !== 'swarm_telemetry' && logObj.agent !== 'swarm') {
    if (!logObj.metadata || !logObj.metadata.domain) return;
  }
  
  const ev: SwarmTelemetryEvent = {
    ts: logObj.ts || new Date().toISOString(),
    agent: logObj.metadata?.agent || logObj.agent || 'unknown',
    model: logObj.metadata?.model || 'unknown',
    domain: logObj.metadata?.domain || 'general',
    status: logObj.metadata?.status || 'running',
    duration: logObj.metadata?.duration || 0,
    tool_calls: logObj.metadata?.tool_calls || 0
  };
  
  swarmEvents.push(ev);
  if (swarmEvents.length > 50) swarmEvents.shift();
  
  renderSwarmExecutionMatrix();
}

export function renderSwarmExecutionMatrix(): void {
  const container = document.getElementById('swarm-matrix-content');
  if (!container) return;
  
  if (swarmEvents.length === 0) {
    container.innerHTML = '<div style="color: var(--text-muted); font-size: 0.8rem;">Waiting for swarm telemetry...</div>';
    return;
  }
  
  let html = '<div style="display: flex; flex-direction: column; gap: 8px;">';
  
  swarmEvents.slice().reverse().forEach(ev => {
    const color = ev.status === 'success' ? 'var(--neon-green)' : (ev.status === 'failure' ? 'var(--neon-red)' : 'var(--neon-yellow)');
    const width = Math.max(1, Math.min(100, (ev.duration / 30) * 100)); // normalized to 30 seconds max scale
    const timeStr = ev.ts.indexOf('T') !== -1 ? ev.ts.split('T')[1].substring(0, 8) : ev.ts;
    
    html += `
      <div style="display: flex; align-items: center; gap: 8px; font-family: 'JetBrains Mono', monospace; font-size: 0.75rem;">
        <div style="width: 70px; color: var(--text-muted);">${timeStr}</div>
        <div style="width: 90px; color: var(--neon-cyan); text-overflow: ellipsis; overflow: hidden; white-space: nowrap;" title="${ev.domain}">${ev.domain}</div>
        <div style="width: 80px; color: var(--neon-purple);">${ev.model}</div>
        <div style="flex: 1; background: rgba(255,255,255,0.05); height: 16px; border-radius: 4px; position: relative; display: flex; align-items: center; overflow: visible;">
          <div style="background: ${color}; width: ${width}%; height: 100%; border-radius: 4px; opacity: 0.8; transition: width 0.3s ease;"></div>
          <span style="position: absolute; left: calc(${width}% + 6px); color: ${color}; font-size: 0.7rem; font-weight: bold; white-space: nowrap;">${ev.duration}s (${ev.tool_calls} tools)</span>
        </div>
      </div>
    `;
  });
  
  html += '</div>';
  container.innerHTML = html;
}

// Updates analytics and dashboard telemetry DOM container elements safely
export function updateTelemetryAnalyticsUI(telemetryData?: any): void {
  const domIds = [
    'arch-node-details',
    'gitbrain-project-context',
    'tier1-po-latency',
    'tier1-plan-pass-rate',
    'tier1-steering-cycles',
    'analytics-commits-val',
    'tier1-dod-compliance',
    'analytics-success-rate-val',
    'tier2-model-name',
    'tier2-mean-duration',
    'tier2-spawning-breakdown',
    'tier2-cache-hit-ratio',
    'tier2-tps-sparkline',
    'circuit-breaker-status',
    'gpu-cost-savings',
    'agent-vram-allocation',
    'agent-token-tps',
    'agent-tool-calls',
    'agent-substrate-lock'
  ];

  domIds.forEach(id => {
    const el = document.getElementById(id);
    if (el && telemetryData && telemetryData[id] !== undefined) {
      el.textContent = String(telemetryData[id]);
    }
  });
}
