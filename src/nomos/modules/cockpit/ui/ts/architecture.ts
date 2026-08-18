// architecture.ts - Visual Architecture Cockpit topology renderer & Gamified XP engine for Nomos OS

export interface ArchNode {
  id: string;
  label: string;
  category: 'kernel' | 'firewall' | 'swarm' | 'gitbrain' | 'daemon';
  x: number;
  y: number;
  color: string;
  description: string;
  metrics: string;
}

export interface ArchEdge {
  from: string;
  to: string;
  label?: string;
}

const archNodes: ArchNode[] = [
  {
    id: 'kernel',
    label: 'Kernel Engine',
    category: 'kernel',
    x: 400,
    y: 240,
    color: '#00f0ff',
    description: 'Core Nomos Go engine managing task scheduling, verification gates, and GlobalBus telemetry pub/sub.',
    metrics: 'Active Task: #326 │ Subsystems: 18'
  },
  {
    id: 'firewall',
    label: 'Substrate Firewall',
    category: 'firewall',
    x: 120,
    y: 70,
    color: '#ff0055',
    description: 'Cryptographic LD_PRELOAD C wrapper (nomos_lock.so) protecting disk mutations outside EDIT phase.',
    metrics: 'Security Mode: Enforced (0440) │ IPC: Unix Domain Socket'
  },
  {
    id: 'daemon',
    label: 'nomosd Server',
    category: 'daemon',
    x: 680,
    y: 70,
    color: '#bd00ff',
    description: 'REST & WebSocket daemon server on port 8089 delivering real-time status and logs to the Control Plane Web UI.',
    metrics: 'Port: 8089 │ WS Clients: Active'
  },
  {
    id: 'swarm',
    label: 'Swarm Fleet',
    category: 'swarm',
    x: 120,
    y: 410,
    color: '#00ff9f',
    description: 'Autonomous Tier 2 Aider & Tier 1 Antigravity worker daemons executing offloaded code edits.',
    metrics: 'Active Daemons: Aider + Antigravity'
  },
  {
    id: 'gitbrain',
    label: 'GitBrain Memory Engine',
    category: 'gitbrain',
    x: 680,
    y: 410,
    color: '#ffb800',
    description: 'SQLite vector store holding architectural explainers, retro timelines, and interactive walkthrough quizzes.',
    metrics: 'Vector DB: active │ Quiz Engine: Ready'
  }
];

const archEdges: ArchEdge[] = [
  { from: 'firewall', to: 'kernel', label: 'IPC Mutex' },
  { from: 'kernel', to: 'daemon', label: 'GlobalBus Pub/Sub' },
  { from: 'swarm', to: 'kernel', label: 'Telemetry Stream' },
  { from: 'kernel', to: 'gitbrain', label: 'Vector Sync' },
  { from: 'daemon', to: 'gitbrain', label: 'API Queries' }
];

let selectedNodeId: string | null = null;

export function renderArchitectureTopology(): void {
  const container = document.getElementById('arch-svg-graph');
  if (!container) return;

  const svgNS = 'http://www.w3.org/2000/svg';
  container.innerHTML = '';
  container.setAttribute('viewBox', '0 0 800 480');

  // Draw edges
  archEdges.forEach(edge => {
    const fromNode = archNodes.find(n => n.id === edge.from);
    const toNode = archNodes.find(n => n.id === edge.to);
    if (!fromNode || !toNode) return;

    const line = document.createElementNS(svgNS, 'line');
    line.setAttribute('x1', String(fromNode.x));
    line.setAttribute('y1', String(fromNode.y));
    line.setAttribute('x2', String(toNode.x));
    line.setAttribute('y2', String(toNode.y));
    line.setAttribute('stroke', 'rgba(139, 92, 246, 0.5)');
    line.setAttribute('stroke-width', '2');
    line.setAttribute('stroke-dasharray', '5 5');
    line.setAttribute('id', `edge-${edge.from}-${edge.to}`);
    container.appendChild(line);

    if (edge.label) {
      const midX = (fromNode.x + toNode.x) / 2;
      const midY = (fromNode.y + toNode.y) / 2;

      // Label background badge
      const rect = document.createElementNS(svgNS, 'rect');
      const textLen = edge.label.length * 7;
      rect.setAttribute('x', String(midX - textLen / 2 - 6));
      rect.setAttribute('y', String(midY - 12));
      rect.setAttribute('width', String(textLen + 12));
      rect.setAttribute('height', '18');
      rect.setAttribute('rx', '4');
      rect.setAttribute('fill', 'rgba(var(--bg-panel-rgb), 0.95)');
      rect.setAttribute('stroke', 'rgba(139, 92, 246, 0.4)');
      rect.setAttribute('stroke-width', '1');
      container.appendChild(rect);

      const text = document.createElementNS(svgNS, 'text');
      text.setAttribute('x', String(midX));
      text.setAttribute('y', String(midY + 1));
      text.setAttribute('fill', '#c084fc');
      text.setAttribute('font-size', '10');
      text.setAttribute('font-weight', 'bold');
      text.setAttribute('text-anchor', 'middle');
      text.setAttribute('font-family', 'JetBrains Mono, monospace');
      text.textContent = edge.label;
      container.appendChild(text);
    }
  });

  // Draw nodes
  archNodes.forEach(node => {
    const g = document.createElementNS(svgNS, 'g');
    g.setAttribute('style', 'cursor: pointer;');
    g.setAttribute('data-node-id', node.id);

    const circle = document.createElementNS(svgNS, 'circle');
    circle.setAttribute('cx', String(node.x));
    circle.setAttribute('cy', String(node.y));
    circle.setAttribute('r', '28');
    circle.setAttribute('fill', 'rgba(var(--bg-panel-rgb), 0.95)');
    circle.setAttribute('stroke', node.color);
    circle.setAttribute('stroke-width', node.id === selectedNodeId ? '4' : '2.5');
    circle.setAttribute('style', `filter: drop-shadow(0 0 10px ${node.color}); transition: all 0.3s ease;`);

    // Title background pill to prevent line overlaps
    const titleLen = node.label.length * 7.5;
    const titleBg = document.createElementNS(svgNS, 'rect');
    titleBg.setAttribute('x', String(node.x - titleLen / 2 - 8));
    titleBg.setAttribute('y', String(node.y + 40));
    titleBg.setAttribute('width', String(titleLen + 16));
    titleBg.setAttribute('height', '20');
    titleBg.setAttribute('rx', '4');
    titleBg.setAttribute('fill', 'rgba(var(--bg-dark-rgb), 0.9)');
    titleBg.setAttribute('stroke', 'rgba(255, 255, 255, 0.1)');

    const text = document.createElementNS(svgNS, 'text');
    text.setAttribute('x', String(node.x));
    text.setAttribute('y', String(node.y + 54));
    text.setAttribute('fill', 'var(--text-main)');
    text.setAttribute('font-size', '12');
    text.setAttribute('font-weight', 'bold');
    text.setAttribute('text-anchor', 'middle');
    text.setAttribute('font-family', 'Outfit, sans-serif');
    text.textContent = node.label;

    g.appendChild(circle);
    g.appendChild(titleBg);
    g.appendChild(text);

    g.addEventListener('click', () => {
      selectedNodeId = node.id;
      inspectArchNode(node);
      renderArchitectureTopology();
    });

    container.appendChild(g);
  });

  // Render initial node details if none selected
  if (!selectedNodeId) {
    inspectArchNode(archNodes[0]);
  }
}

export function inspectArchNode(node: ArchNode): void {
  const detailsEl = document.getElementById('arch-node-details');
  if (!detailsEl) return;

  detailsEl.innerHTML = `
    <div style="display: flex; align-items: center; gap: 8px; margin-bottom: 0.75rem;">
      <span style="width: 12px; height: 12px; border-radius: 50%; background: ${node.color}; box-shadow: 0 0 8px ${node.color};"></span>
      <h3 style="margin: 0; color: var(--text-main); font-size: 1rem; font-weight: 800;">${node.label}</h3>
    </div>
    <div style="background: rgba(255, 255, 255, 0.03); border: 1px solid var(--border-indigo); border-radius: 6px; padding: 0.75rem; margin-bottom: 0.75rem;">
      <div style="font-size: 0.7rem; color: var(--neon-purple); font-weight: bold; text-transform: uppercase; margin-bottom: 4px;">Role & Architecture</div>
      <div>${node.description}</div>
    </div>
    <div style="background: rgba(255, 255, 255, 0.03); border: 1px solid var(--border-indigo); border-radius: 6px; padding: 0.75rem;">
      <div style="font-size: 0.7rem; color: var(--neon-green); font-weight: bold; text-transform: uppercase; margin-bottom: 4px;">Subsystem Metrics</div>
      <div style="font-family: 'JetBrains Mono', monospace; font-size: 0.75rem; color: var(--text-main);">${node.metrics}</div>
    </div>
  `;
}

export function triggerArchitecturePulse(fromId?: string, toId?: string): void {
  const container = document.getElementById('arch-svg-graph');
  if (!container) return;

  const svgNS = 'http://www.w3.org/2000/svg';
  const edgesToAnimate = (fromId && toId)
    ? archEdges.filter(e => e.from === fromId && e.to === toId)
    : archEdges;

  edgesToAnimate.forEach((edge, idx) => {
    setTimeout(() => {
      const fromNode = archNodes.find(n => n.id === edge.from);
      const toNode = archNodes.find(n => n.id === edge.to);
      if (!fromNode || !toNode) return;

      const dot = document.createElementNS(svgNS, 'circle');
      dot.setAttribute('cx', String(fromNode.x));
      dot.setAttribute('cy', String(fromNode.y));
      dot.setAttribute('r', '7');
      dot.setAttribute('fill', fromNode.color);
      dot.setAttribute('style', `filter: drop-shadow(0 0 10px ${fromNode.color});`);
      container.appendChild(dot);

      const startTime = performance.now();
      const duration = 650;

      function step(now: number) {
        const elapsed = now - startTime;
        const progress = Math.min(elapsed / duration, 1);

        const currentX = fromNode.x + (toNode.x - fromNode.x) * progress;
        const currentY = fromNode.y + (toNode.y - fromNode.y) * progress;

        dot.setAttribute('cx', String(currentX));
        dot.setAttribute('cy', String(currentY));

        if (progress < 1) {
          requestAnimationFrame(step);
        } else {
          dot.remove();
          // Highlight destination node upon arrival
          const targetG = container.querySelector(`g[data-node-id="${toNode.id}"] circle`);
          if (targetG) {
            targetG.setAttribute('style', `filter: drop-shadow(0 0 22px ${toNode.color}); transition: all 0.3s ease;`);
            setTimeout(() => {
              targetG.setAttribute('style', `filter: drop-shadow(0 0 10px ${toNode.color}); transition: all 0.3s ease;`);
            }, 500);
          }
        }
      }

      requestAnimationFrame(step);
    }, idx * 180);
  });
}
