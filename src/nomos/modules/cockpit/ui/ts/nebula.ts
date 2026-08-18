// nebula.ts - Decoupled pseudo-3D rotating clustered vector memory nebula visualizer

// 3D Nebula Global State
let angleX = 0.25;
let angleY = 0.5;
let isDraggingNebula = false;
let isHoveringNebula = false;
let mouseXInSvg = -9999;
let mouseYInSvg = -9999;
let lastMouseX = 0;
let lastMouseY = 0;
let zoomFactor = 1.0;
let nebulaAnimFrameId: any = null;

let activeCachedLessons: any[] = [];
let activeSelectedLessonHash: string | null = null;
let activeSelectMemoryNodeCallback: (hash: string) => void = () => {};

function getDeterministic3DOffset(hash: string): { x: number; y: number; z: number } {
  let h = 0;
  for (let i = 0; i < hash.length; i++) {
    h = (h << 5) - h + hash.charCodeAt(i);
    h |= 0;
  }
  const theta = (Math.abs(h % 1000) / 1000) * 2 * Math.PI;
  const phi = (Math.abs((h >> 10) % 1000) / 1000) * Math.PI;
  const r = 35 + (Math.abs((h >> 20) % 1000) / 1000) * 20; // distance 35-55
  return {
    x: r * Math.sin(phi) * Math.cos(theta),
    y: r * Math.sin(phi) * Math.sin(theta),
    z: r * Math.cos(phi)
  };
}


// Caching maps for DOM elements to avoid DOM thrashing
const categoryDOMMap = new Map<string, any>();
const nodeDOMMap = new Map<string, any>();
let lastLessonsRef: any[] | null = null;
let lastSelectedHashRef: string | null = null;

export function renderMemoryClusterCanvas(
  cachedLessons: any[],
  selectedLessonHash: string | null,
  selectMemoryNodeCallback: (hash: string) => void
): void {
  // Update active state for the animation loop
  activeCachedLessons = cachedLessons;
  activeSelectedLessonHash = selectedLessonHash;
  activeSelectMemoryNodeCallback = selectMemoryNodeCallback;

  const svg = document.getElementById('memory-cluster-canvas') as any;
  if (!svg) return;

  const nodesCountEl = document.getElementById('nebula-nodes-count');
  if (nodesCountEl) {
    nodesCountEl.textContent = `${cachedLessons ? cachedLessons.length : 0} node${cachedLessons && cachedLessons.length === 1 ? '' : 's'}`;
  }

  const w = svg.clientWidth || 300;
  const h = svg.clientHeight || 300;

  // Bind mouse drag and hover listeners for 3D orbit controls
  if (!svg.hasNebulaListeners) {
    svg.hasNebulaListeners = true;
    svg.style.cursor = 'grab';

    // Event Delegation: single listener on SVG for node clicks
    svg.addEventListener('click', (e: MouseEvent) => {
      let target = e.target as HTMLElement;
      while (target && target !== svg) {
        if (target.classList && target.classList.contains('nebula-node')) {
          const hash = target.getAttribute('data-hash');
          if (hash && !(window as any).wasNebulaDragging) {
            e.stopPropagation();
            selectMemoryNodeCallback(hash);
          }
          break;
        }
        target = target.parentNode as HTMLElement;
      }
    });

    svg.addEventListener('mousemove', (e: MouseEvent) => {
      const rect = svg.getBoundingClientRect();
      mouseXInSvg = e.clientX - (rect.left + rect.width / 2);
      mouseYInSvg = e.clientY - (rect.top + rect.height / 2);
    });

    svg.addEventListener('mouseenter', () => {
      isHoveringNebula = true;
    });

    svg.addEventListener('mouseleave', () => {
      isHoveringNebula = false;
      mouseXInSvg = -9999;
      mouseYInSvg = -9999;
      if (isDraggingNebula) {
        isDraggingNebula = false;
      }
      svg.style.cursor = 'grab';
    });

    svg.addEventListener('mousedown', (e: MouseEvent) => {
      isDraggingNebula = true;
      svg.style.cursor = 'grabbing';
      lastMouseX = e.clientX;
      lastMouseY = e.clientY;
      (window as any).wasNebulaDragging = false;
      svg.mouseDownX = e.clientX;
      svg.mouseDownY = e.clientY;
    });

    window.addEventListener('mousemove', (e: MouseEvent) => {
      if (!isDraggingNebula) return;
      const deltaX = e.clientX - lastMouseX;
      const deltaY = e.clientY - lastMouseY;
      angleY += deltaX * 0.005;
      angleX += deltaY * 0.005;
      lastMouseX = e.clientX;
      lastMouseY = e.clientY;

      const totalDist = Math.hypot(e.clientX - svg.mouseDownX, e.clientY - svg.mouseDownY);
      if (totalDist > 5) {
        (window as any).wasNebulaDragging = true;
      }
    });

    window.addEventListener('mouseup', () => {
      if (isDraggingNebula) {
        isDraggingNebula = false;
        svg.style.cursor = 'grab';
        // Keep wasNebulaDragging true for a split second to allow onclick to ignore it
        setTimeout(() => {
          (window as any).wasNebulaDragging = false;
        }, 80);
      }
    });

    svg.addEventListener('wheel', (e: WheelEvent) => {
      e.preventDefault();
      const zoomDelta = -e.deltaY * 0.0015;
      zoomFactor = Math.max(0.4, Math.min(3.0, zoomFactor + zoomDelta));
      renderMemoryClusterCanvas(cachedLessons, selectedLessonHash, selectMemoryNodeCallback);
    }, { passive: false });
  }

  // Get all unique categories in cachedLessons
  const uniqueCategories = new Set<string>();
  cachedLessons.forEach(l => {
    if (l.category) uniqueCategories.add(l.category);
  });
  ['general', 'walkthrough', 'explainer', 'quiz'].forEach(cat => uniqueCategories.add(cat));
  const catList = Array.from(uniqueCategories).sort();

  // 3D Category Core Coordinates in space
  const categoryCenters3D: { [key: string]: { x: number; y: number; z: number } } = {};
  const dRadius = Math.min(w, h) * 0.22;

  // Filter out 'general' for outer radial distribution
  const outerCats = catList.filter(c => c !== 'general');
  const numOuter = outerCats.length;

  outerCats.forEach((cat, index) => {
    const angle = (index / numOuter) * 2 * Math.PI;
    categoryCenters3D[cat] = {
      x: dRadius * Math.cos(angle),
      y: dRadius * Math.sin(angle),
      z: (index % 2 === 0 ? 30 : -30) // Alternate depth offset
    };
  });

  categoryCenters3D['general'] = { x: 0, y: 0, z: 0 };

  // Group nodes by category
  const nodesByCategory: { [key: string]: any[] } = {};
  cachedLessons.forEach(l => {
    const cat = l.category || 'general';
    const mappedCat = categoryCenters3D[cat] ? cat : 'general';
    if (!nodesByCategory[mappedCat]) nodesByCategory[mappedCat] = [];
    nodesByCategory[mappedCat].push(l);
  });

  // Calculate 3D rotated & projected coordinates for category centers
  const cosX = Math.cos(angleX);
  const sinX = Math.sin(angleX);
  const cosY = Math.cos(angleY);
  const sinY = Math.sin(angleY);

  const presetColors = [
    'var(--neon-green)',
    'var(--neon-blue)',
    'var(--neon-pink)',
    'var(--neon-yellow)',
    'var(--neon-purple)',
    'var(--neon-indigo)',
    '#06b6d4',
    '#f97316'
  ];

  const projCenters: { [key: string]: { x: number; y: number; z: number; color: string } } = {};
  Object.keys(categoryCenters3D).forEach(cat => {
    const core = categoryCenters3D[cat];
    // Rotate Y
    const x1 = core.x * cosY - core.z * sinY;
    const z1 = core.x * sinY + core.z * cosY;
    const y1 = core.y;
    // Rotate X
    const x2 = x1;
    const y2 = y1 * cosX - z1 * sinX;
    const z2 = y1 * sinX + z1 * cosX;

    // Perspective Projection
    const dist = 400;
    const perspective = dist / (dist + z2);
    const screenX = w / 2 + x2 * perspective * zoomFactor;
    const screenY = h / 2 + y2 * perspective * zoomFactor;

    let color = 'var(--neon-purple)';
    if (cat !== 'general') {
      const idx = outerCats.indexOf(cat);
      if (idx !== -1) {
        color = presetColors[idx % presetColors.length];
      }
    }

    projCenters[cat] = { x: screenX, y: screenY, z: z2, color };
  });

  const projNodes: any[] = [];
  Object.keys(nodesByCategory).forEach(cat => {
    const list = nodesByCategory[cat];
    const core3D = categoryCenters3D[cat];
    const projCore = projCenters[cat];

    list.forEach(node => {
      const offset = getDeterministic3DOffset(node.commitHash);
      const nx = core3D.x + offset.x;
      const ny = core3D.y + offset.y;
      const nz = core3D.z + offset.z;

      // Rotate Y
      const x1 = nx * cosY - nz * sinY;
      const z1 = nx * sinY + nz * cosY;
      const y1 = ny;
      // Rotate X
      const x2 = x1;
      const y2 = y1 * cosX - z1 * sinX;
      const z2 = y1 * sinX + z1 * cosX;

      // Perspective Projection
      const dist = 400;
      const perspective = dist / (dist + z2);
      const screenX = w / 2 + x2 * perspective * zoomFactor;
      const screenY = h / 2 + y2 * perspective * zoomFactor;

      projNodes.push({
        commitHash: node.commitHash,
        category: cat,
        x: screenX,
        y: screenY,
        z: z2,
        centerX: projCore.x,
        centerY: projCore.y,
        color: projCore.color,
        isSelected: selectedLessonHash === node.commitHash
      });
    });
  });

  // Rebuild the DOM entirely if the lesson data or selection changes
  const needsRebuild = cachedLessons !== lastLessonsRef || selectedLessonHash !== lastSelectedHashRef;
  
  if (needsRebuild) {
    svg.innerHTML = '';
    categoryDOMMap.clear();
    nodeDOMMap.clear();
    lastLessonsRef = cachedLessons;
    lastSelectedHashRef = selectedLessonHash;

    const elements: { z: number, el: SVGElement }[] = [];

    const createSVGElement = (tag: string) => document.createElementNS('http://www.w3.org/2000/svg', tag);

    Object.keys(projCenters).forEach(cat => {
      const core = projCenters[cat];
      const g = createSVGElement('g');
      
      const c1 = createSVGElement('circle');
      c1.setAttribute('r', '26');
      c1.setAttribute('fill', core.color);
      c1.setAttribute('fill-opacity', '0.04');
      c1.setAttribute('stroke', core.color);
      c1.setAttribute('stroke-opacity', '0.08');
      c1.setAttribute('stroke-width', '1');
      
      const c2 = createSVGElement('circle');
      c2.setAttribute('r', '8');
      c2.setAttribute('fill', 'none');
      c2.setAttribute('stroke', core.color);
      c2.setAttribute('stroke-width', '1.5');
      c2.setAttribute('stroke-dasharray', '3 3');
      c2.setAttribute('stroke-opacity', '0.3');
      
      const anim = createSVGElement('animate');
      anim.setAttribute('attributeName', 'stroke-dashoffset');
      anim.setAttribute('values', '0;24');
      anim.setAttribute('dur', '8s');
      anim.setAttribute('repeatCount', 'indefinite');
      c2.appendChild(anim);

      const text = createSVGElement('text');
      text.setAttribute('fill', core.color);
      text.setAttribute('fill-opacity', '0.7');
      text.setAttribute('font-size', '7.5');
      text.setAttribute('font-weight', 'bold');
      text.setAttribute('font-family', "'JetBrains Mono', monospace");
      text.setAttribute('text-anchor', 'middle');
      text.style.userSelect = 'none';
      text.textContent = cat.toUpperCase();

      g.appendChild(c1);
      g.appendChild(c2);
      g.appendChild(text);

      elements.push({ z: core.z, el: g });
      categoryDOMMap.set(cat, { g, c1, c2, text, z: core.z });
    });

    projNodes.forEach(node => {
      const line = createSVGElement('line');
      line.setAttribute('stroke', node.color);
      line.setAttribute('stroke-width', '0.75');
      line.setAttribute('stroke-dasharray', '2 2');

      const g = createSVGElement('g');
      g.classList.add('nebula-node');
      g.setAttribute('data-hash', node.commitHash);
      g.style.cursor = 'pointer';

      const c1 = createSVGElement('circle');
      const c2 = createSVGElement('circle');
      
      if (node.isSelected) {
        c1.setAttribute('fill', 'none');
        c1.setAttribute('stroke', node.color);
        c1.setAttribute('stroke-width', '1.5');
        
        const animR = createSVGElement('animate');
        animR.setAttribute('attributeName', 'r');
        animR.setAttribute('dur', '2s');
        animR.setAttribute('repeatCount', 'indefinite');
        animR.classList.add('anim-r');
        c1.appendChild(animR);

        const animOp = createSVGElement('animate');
        animOp.setAttribute('attributeName', 'stroke-opacity');
        animOp.setAttribute('dur', '2s');
        animOp.setAttribute('repeatCount', 'indefinite');
        animOp.classList.add('anim-op');
        c1.appendChild(animOp);

        c2.setAttribute('fill', 'var(--text-main)');
        c2.setAttribute('stroke', node.color);
        c2.setAttribute('stroke-width', '2');
      } else {
        c1.setAttribute('fill', 'var(--bg-dark)');
        c1.setAttribute('stroke', node.color);
        c1.setAttribute('stroke-width', '1.5');
        
        c2.setAttribute('fill', node.color);
        c2.setAttribute('stroke', 'none');
      }

      g.appendChild(c1);
      g.appendChild(c2);

      elements.push({ z: node.z + 10, el: line });
      elements.push({ z: node.z, el: g });
      nodeDOMMap.set(node.commitHash, { line, g, c1, c2, isSelected: node.isSelected, z: node.z });
    });

    elements.sort((a, b) => b.z - a.z);
    elements.forEach(e => svg.appendChild(e.el));
  }

  // Update existing DOM element positions
  Object.keys(projCenters).forEach(cat => {
    const core = projCenters[cat];
    const dom = categoryDOMMap.get(cat);
    if (dom) {
      dom.c1.setAttribute('cx', core.x.toString());
      dom.c1.setAttribute('cy', core.y.toString());
      dom.c2.setAttribute('cx', core.x.toString());
      dom.c2.setAttribute('cy', core.y.toString());
      dom.text.setAttribute('x', core.x.toString());
      dom.text.setAttribute('y', (core.y + 4).toString());
      dom.z = core.z; // update z for sorting if needed
    }
  });

  projNodes.forEach(node => {
    const dom = nodeDOMMap.get(node.commitHash);
    if (dom) {
      const depthScale = 400 / (400 + node.z);
      const nodeOpacity = Math.max(0.15, Math.min(1.0, 0.2 + 0.8 * depthScale));
      const size = Math.max(1.5, Math.min(8.0, 3.5 * depthScale));

      dom.line.setAttribute('x1', node.centerX.toString());
      dom.line.setAttribute('y1', node.centerY.toString());
      dom.line.setAttribute('x2', node.x.toString());
      dom.line.setAttribute('y2', node.y.toString());
      dom.line.setAttribute('stroke-opacity', (nodeOpacity * 0.4).toString());

      dom.c1.setAttribute('cx', node.x.toString());
      dom.c1.setAttribute('cy', node.y.toString());
      dom.c2.setAttribute('cx', node.x.toString());
      dom.c2.setAttribute('cy', node.y.toString());

      if (dom.isSelected) {
        const animR = dom.c1.querySelector('.anim-r');
        if (animR) animR.setAttribute('values', `${size + 2};${size + 7};${size + 2}`);
        const animOp = dom.c1.querySelector('.anim-op');
        if (animOp) animOp.setAttribute('values', `${nodeOpacity};${nodeOpacity * 0.3};${nodeOpacity}`);
        dom.c2.setAttribute('r', size.toString());
      } else {
        dom.c1.setAttribute('r', size.toString());
        dom.c1.setAttribute('stroke-opacity', nodeOpacity.toString());
        dom.c2.setAttribute('r', (size + 2).toString());
        dom.c2.setAttribute('fill-opacity', (nodeOpacity * 0.15).toString());
      }
      dom.z = node.z;
    }
  });

  // Re-sort the DOM children dynamically based on depth
  // Only doing this every few frames or on drag could be an optimization, but browsers handle appendChild fast.
  const sortedNodes = Array.from(nodeDOMMap.values()).sort((a, b) => b.z - a.z);
  const sortedCores = Array.from(categoryDOMMap.values()).sort((a, b) => b.z - a.z);
  
  // Actually, we want lines in the back, then cores, then nodes.
  // We can just append lines, then cores, then nodes.
  Array.from(nodeDOMMap.values()).forEach(n => svg.appendChild(n.line));
  sortedCores.forEach(c => svg.appendChild(c.g));
  sortedNodes.forEach(n => svg.appendChild(n.g));

  // Initialize loop if not already running
  if (!nebulaAnimFrameId) {
    const tick = () => {
      let speedMult = 1.0;
      if (isHoveringNebula && mouseXInSvg !== -9999) {
        const dist = Math.hypot(mouseXInSvg, mouseYInSvg);
        const maxDist = Math.max(w, h) * 0.45;
        speedMult = Math.min(1.0, Math.max(0.0, (dist - 45) / (maxDist - 45)));
      }

      if (!isDraggingNebula) {
        angleY += 0.002 * speedMult;
        angleX += 0.0005 * speedMult;
      }
      renderMemoryClusterCanvas(activeCachedLessons, activeSelectedLessonHash, activeSelectMemoryNodeCallback);
      nebulaAnimFrameId = requestAnimationFrame(tick);
    };
    nebulaAnimFrameId = requestAnimationFrame(tick);
  }
}
