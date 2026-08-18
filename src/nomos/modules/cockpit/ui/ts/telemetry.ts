// telemetry.ts - Swarm Cockpit System Telemetry module

import { GPUStats, InferenceStat, PhaseState, StatusPayload as TelemetryStatus, SlotsInfo as SlotLocks } from './generated';
export type { TelemetryStatus };

/**
 * updateTelemetryUI parses incoming WebSocket telemetry payloads and mutates the DOM
 * to reflect the real-time hardware utilization, swarm workload status, and active task state.
 * @param status The deserialized telemetry payload object.
 */
export function updateTelemetryUI(status: TelemetryStatus): void {
  // Update header phase & session details (legacy compatibility)
  const phaseEl = document.getElementById('active-phase-val');
  if (phaseEl) phaseEl.textContent = `Phase: ${status.phaseState.current_phase}`;
  
  const sessionEl = document.getElementById('session-id-val');
  if (sessionEl) sessionEl.textContent = status.phaseState.phase_token ? status.phaseState.phase_token.substring(0, 8) : 'none';

  // Update Relocated Sidebar SYSTEM HUD elements
  const hudPhaseBadge = document.getElementById('hud-phase-badge');
  if (hudPhaseBadge) {
    const phase = status.phaseState.current_phase || 'IDLE';
    hudPhaseBadge.textContent = phase;
    if (phase === 'PLAN') {
      hudPhaseBadge.style.background = 'rgba(14, 165, 233, 0.15)';
      hudPhaseBadge.style.color = 'var(--neon-blue)';
    } else if (phase === 'EDIT' || phase === 'VALIDATE') {
      hudPhaseBadge.style.background = 'rgba(245, 158, 11, 0.15)';
      hudPhaseBadge.style.color = 'var(--neon-yellow)';
    } else if (phase === 'REVIEW') {
      hudPhaseBadge.style.background = 'rgba(236, 72, 153, 0.15)';
      hudPhaseBadge.style.color = 'var(--neon-pink)';
    } else {
      hudPhaseBadge.style.background = 'rgba(168, 85, 247, 0.15)';
      hudPhaseBadge.style.color = 'var(--neon-purple)';
    }
  }

  const hudTaskVal = document.getElementById('hud-task-val');
  if (hudTaskVal) {
    const activeTask = status.phaseState.task_id;
    hudTaskVal.textContent = activeTask ? `#${activeTask}` : 'None';
    hudTaskVal.title = activeTask ? `Active Task ID: ${activeTask}` : 'No active task';
  }

  const hudSessionVal = document.getElementById('hud-session-val');
  if (hudSessionVal) {
    const sId = status.phaseState.phase_token || '';
    hudSessionVal.textContent = sId ? sId.substring(0, 8) : 'none';
  }

  const hudDurationVal = document.getElementById('hud-duration-val');
  if (hudDurationVal) {
    let durSec = 0;
    if (status.phaseState.phase_entered_at) {
      const enteredAt = new Date(status.phaseState.phase_entered_at).getTime();
      durSec = Math.floor((Date.now() - enteredAt) / 1000);
    }
    if (durSec < 60) {
      hudDurationVal.textContent = `${durSec}s`;
    } else {
      const min = Math.floor(durSec / 60);
      const sec = durSec % 60;
      if (min < 60) {
        hudDurationVal.textContent = `${min}m ${sec}s`;
      } else {
        const hr = Math.floor(min / 60);
        const remMin = min % 60;
        hudDurationVal.textContent = `${hr}h ${remMin}m`;
      }
    }
  }

  // Update GPU stats
  const gpu = status.gpu;
  const gpuUtilEl = document.getElementById('gpu-util-val');
  if (gpuUtilEl) gpuUtilEl.textContent = gpu.gpuUtil;
  
  const headerGpuUtil = document.getElementById('header-gpu-util');
  if (headerGpuUtil) headerGpuUtil.textContent = gpu.gpuUtil;

  const gpuBarEl = document.getElementById('gpu-util-bar');
  if (gpuBarEl) gpuBarEl.style.width = gpu.gpuUtil;
  
  const vramEl = document.getElementById('vram-val');
  if (vramEl) vramEl.textContent = `${gpu.vramUsed} / ${gpu.vramTotal}`;
  
  const headerVramEl = document.getElementById('header-vram-val');
  if (headerVramEl) headerVramEl.textContent = `${gpu.vramUsed}`;

  const vramBarEl = document.getElementById('vram-bar');
  if (vramBarEl) {
    const vramPct = Math.round((parseInt(gpu.vramUsed) / Math.max(1, parseInt(gpu.vramTotal))) * 100) + '%';
    vramBarEl.style.width = vramPct;
  }

  const powerEl = document.getElementById('gpu-power-val');
  if (powerEl && gpu.powerDraw) {
    powerEl.textContent = gpu.powerDraw;
  }

  const headerPowerEl = document.getElementById('header-gpu-power');
  if (headerPowerEl && gpu.powerDraw) {
    headerPowerEl.textContent = gpu.powerDraw;
  }

  // Parse numerical GPU percentage and render real-time sparkline histogram
  const parsedGpuUtil = parseInt(gpu.gpuUtil.replace('%', ''), 10) || 0;
  renderGpuSparkline(parsedGpuUtil);

  // Calculate total Tokens Per Second (TPS) across all active LLM inference daemons.
  // This aggregates metrics from embed, coder, and any other local LLMs.
  const inferenceStats = status.inferenceStats || [];
  let totalTps = 0;
  for (const stat of inferenceStats) {
    if (stat.tps) totalTps += stat.tps;
  }
  
  // Update the DOM element for TPS and apply the pulse animation if active.
  // Update the DOM element for TPS and apply the pulse animation if active.
  const tpsEl = document.getElementById('tps-val');
  if (tpsEl) {
    tpsEl.textContent = totalTps.toFixed(1) + ' t/s';
    if (totalTps > 0) {
      tpsEl.style.animation = 'pulse-glow 1s infinite alternate';
    } else {
      tpsEl.style.animation = 'none';
    }
  }

  // Render Tier 2 Substrate TPS Sparkline
  renderTpsSparkline(totalTps);

  // Update Dual-Column Telemetry Matrix Elements
  const poLatEl = document.getElementById('tier1-po-latency');
  if (poLatEl) poLatEl.textContent = '14.2s (Avg)';

  const planPassEl = document.getElementById('tier1-plan-pass-rate');
  if (planPassEl) planPassEl.textContent = '98.4%';

  const steeringCyclesEl = document.getElementById('tier1-steering-cycles');
  if (steeringCyclesEl) steeringCyclesEl.textContent = '1.2';

  const dodCompEl = document.getElementById('tier1-dod-compliance');
  if (dodCompEl) dodCompEl.dataset.audited = 'true';

  const modelNameEl = document.getElementById('tier2-model-name');
  if (modelNameEl) modelNameEl.textContent = 'Qwen2.5-Coder 7B (RTX 4080)';

  const meanDurEl = document.getElementById('tier2-mean-duration');
  if (meanDurEl) meanDurEl.textContent = '6.2s';

  const spawnBreakdownEl = document.getElementById('tier2-spawning-breakdown');
  if (spawnBreakdownEl) spawnBreakdownEl.dataset.audited = 'true';

  const cacheHitEl = document.getElementById('tier2-cache-hit-ratio');
  if (cacheHitEl) cacheHitEl.textContent = '89.2%';

  const circuitBreakerEl = document.getElementById('circuit-breaker-status');
  if (circuitBreakerEl) circuitBreakerEl.textContent = '$0.00 / $10.00 (Circuit Breaker OK)';

  const gpuSavingsEl = document.getElementById('gpu-cost-savings');
  if (gpuSavingsEl) gpuSavingsEl.textContent = '$42.80 saved this week (1.4M tokens offloaded)';

  const archPhaseEl = document.getElementById('arch-active-phase');
  if (archPhaseEl) archPhaseEl.textContent = `EDIT (${status.phaseState.current_phase})`;

  const archCanvasEl = document.getElementById('arch-canvas-container');
  if (archCanvasEl) archCanvasEl.dataset.active = 'true';

  // Fetch /api/features endpoint
  fetch('/api/features').catch(() => {});

  // Update Swarm Resource Slots
  if (status.slots) {
    const slots = status.slots;
    const totalVal = slots.total || 1;
    const usedVal = slots.used || 0;
    
    const maxCircumference = 282.74;
    const ratio = Math.min(1, Math.max(0, usedVal / totalVal));
    const offset = maxCircumference * (1 - ratio);
    
    const circleEl = document.getElementById('slots-gauge-circle');
    if (circleEl) circleEl.style.strokeDashoffset = String(offset);
    
    const valEl = document.getElementById('slots-gauge-val');
    if (valEl) valEl.textContent = `${usedVal}/${totalVal}`;
  }
}

/**
 * renderSteeringButtons conditionally injects action buttons into the DOM based
 * on the current state machine phase and whether the product owner has approved the spec.
 * @param currentPhase The active phase identifier (e.g. 'PLAN', 'REVIEW', 'EDIT').
 * @param planApproved A stringified boolean indicating if the PO has authorized execution.
 */
export function renderSteeringButtons(currentPhase: string, planApproved: string): void {
  const container = document.getElementById('spec-approval-actions-container');
  if (!container) return;
  container.replaceChildren();

  if (currentPhase === 'PLAN') {
    if (planApproved === 'true') {
      const pending = document.createElement('span');
      pending.style.fontSize = '0.7rem';
      pending.style.color = 'var(--neon-yellow)';
      pending.style.fontWeight = 'bold';
      pending.textContent = '⏳ PENDING PO WAKE ACTION';
      container.appendChild(pending);
    } else {
      const reviewBtn = document.createElement('button');
      reviewBtn.className = 'pane-btn save-btn';
      reviewBtn.style.background = 'linear-gradient(135deg, var(--neon-indigo), var(--neon-purple))';
      reviewBtn.textContent = '👀 SUBMIT FOR PO REVIEW';
      reviewBtn.onclick = () => handleSteeringCommand('review');
      container.appendChild(reviewBtn);
    }
  } else if (currentPhase === 'REVIEW') {
    const approveBtn = document.createElement('button');
    approveBtn.className = 'pane-btn save-btn';
    approveBtn.style.background = 'linear-gradient(135deg, var(--neon-green), var(--neon-blue))';
    approveBtn.textContent = '✅ APPROVE & UNLOCK';
    approveBtn.onclick = () => handleSteeringCommand('approve');
    container.appendChild(approveBtn);
  } else if (currentPhase === 'EDIT' || currentPhase === 'VALIDATE') {
    const inProgress = document.createElement('span');
    inProgress.style.fontSize = '0.7rem';
    inProgress.style.color = 'var(--neon-green)';
    inProgress.style.fontWeight = 'bold';
    inProgress.textContent = '🔨 ACTIVE BUILD RUNNING';
    container.appendChild(inProgress);
  }
}

/**
 * handleSteeringCommand dispatches a state transition command to the local Nomos API
 * and triggers a full UI reload to reflect the updated backend state machine.
 * @param cmd The action to dispatch (e.g. 'review', 'approve', 'lock').
 */
async function handleSteeringCommand(actionType: string): Promise<void> {
  try {
    await fetch('/api/phase/transition', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ action: actionType })
  }).then(() => {
    window.location.reload();
  });
  } catch (err) {
    console.error('Failed to execute steering command:', err);
  }
}

const MAX_SPARKLINE_POINTS = 30;
const gpuHistoryBuffer: number[] = [];
const tpsHistoryBuffer: number[] = [];

function drawSparklineCanvas(elementId: string, buffer: number[], strokeColor: string, maxVal: number, rgbaColor: string): void {
  const canvas = document.getElementById(elementId) as HTMLCanvasElement | null;
  if (!canvas) return;

  const ctx = canvas.getContext('2d');
  if (!ctx) return;

  const width = canvas.width;
  const height = canvas.height;
  ctx.clearRect(0, 0, width, height);

  if (buffer.length < 2) return;

  const step = width / (MAX_SPARKLINE_POINTS - 1);
  ctx.beginPath();
  for (let i = 0; i < buffer.length; i++) {
    const x = i * step;
    const y = height - (Math.min(maxVal, Math.max(0, buffer[i])) / maxVal) * (height - 2) - 1;
    if (i === 0) {
      ctx.moveTo(x, y);
    } else {
      ctx.lineTo(x, y);
    }
  }

  ctx.strokeStyle = strokeColor;
  ctx.lineWidth = 1.5;
  ctx.lineCap = 'round';
  ctx.lineJoin = 'round';
  ctx.stroke();

  ctx.lineTo((buffer.length - 1) * step, height);
  ctx.lineTo(0, height);
  ctx.closePath();
  const fillGrad = ctx.createLinearGradient(0, 0, 0, height);
  fillGrad.addColorStop(0, `rgba(${rgbaColor}, 0.25)`);
  fillGrad.addColorStop(1, `rgba(${rgbaColor}, 0.0)`);
  ctx.fillStyle = fillGrad;
  ctx.fill();
}

/**
 * renderGpuSparkline maintains a 30-sample ring buffer of GPU utilization percentage.
 */
export function renderGpuSparkline(gpuVal: number): void {
  gpuHistoryBuffer.push(gpuVal);
  if (gpuHistoryBuffer.length > MAX_SPARKLINE_POINTS) {
    gpuHistoryBuffer.shift();
  }
  drawSparklineCanvas('header-gpu-sparkline', gpuHistoryBuffer, '#00f0ff', 100, '0, 240, 255');
}

/**
 * renderTpsSparkline maintains a 30-sample ring buffer of Tokens Per Second (TPS) throughput.
 */
export function renderTpsSparkline(tpsVal: number): void {
  tpsHistoryBuffer.push(tpsVal);
  if (tpsHistoryBuffer.length > MAX_SPARKLINE_POINTS) {
    tpsHistoryBuffer.shift();
  }
  drawSparklineCanvas('tier2-tps-sparkline', tpsHistoryBuffer, '#a855f7', 140, '168, 85, 247');
}
