// ast.ts - Structured Package-Level Coupling Observer
// Replaces the force-directed graph with a structured package coupling table/tree

import { ASTNode } from './types.js';

export interface ASTLink {
  source: string;
  target: string;
}

export interface ASTData {
  nodes: ASTNode[];
  links: ASTLink[];
  cycles?: string[][];
}

interface PackageCouplingStats {
  package: string;
  afferent: number;
  efferent: number;
  imports: string[] | null;
  importedBy: string[] | null;
}

interface CouplingReport {
  packages: PackageCouplingStats[];
  cycle: string[] | null;
}

// Keep diagnostic interface on window object for compatibility
(window as any).astDiag = {
  nodesCount: 0,
  linksCount: 0,
  canvasWidth: 0,
  canvasHeight: 0,
  panX: 0,
  panY: 0,
  zoom: 1.0,
  orbitAngle: 0,
  sampleNodes: [],
  clickedNodeId: null,
  getNodeById: () => null,
  lastError: null
};

let cachedCouplingData: CouplingReport | null = null;
let currentSelectedPackage: string | null = null;
let currentInspectedFile: string | null = null;


// Dynamically inject styles for the package observer table
function injectStyles(): void {
  if (document.getElementById('ast-coupling-styles')) return;
  const style = document.createElement('style');
  style.id = 'ast-coupling-styles';
  style.textContent = `
    .coupling-container {
      position: absolute;
      top: 0;
      left: 0;
      right: 0;
      bottom: 0;
      display: flex;
      flex-direction: column;
      padding: 1rem;
      box-sizing: border-box;
      gap: 1rem;
    }
    .coupling-search {
      width: 100%;
      background: rgba(255, 255, 255, 0.03);
      border: 1px solid var(--border-indigo);
      border-radius: 6px;
      padding: 0.6rem 1rem;
      color: var(--text-main);
      font-family: 'JetBrains Mono', monospace;
      font-size: 0.8rem;
      outline: none;
      box-sizing: border-box;
      transition: border-color 0.2s;
    }
    .coupling-search:focus {
      border-color: var(--neon-blue);
      box-shadow: 0 0 8px rgba(14, 165, 233, 0.2);
    }
    .coupling-table-wrapper {
      flex: 1;
      overflow-y: auto;
      border: 1px solid rgba(255, 255, 255, 0.05);
      border-radius: 6px;
      background: var(--bg-glass);
    }
    .coupling-table {
      width: 100%;
      border-collapse: collapse;
      text-align: left;
      font-size: 0.75rem;
      font-family: 'Inter', sans-serif;
    }
    .coupling-table th {
      position: sticky;
      top: 0;
      background: #0f0b1a;
      color: var(--text-muted);
      font-weight: 600;
      text-transform: uppercase;
      font-size: 0.65rem;
      letter-spacing: 0.05em;
      padding: 0.75rem 1rem;
      border-bottom: 1px solid var(--border-indigo);
      z-index: 1;
    }
    .coupling-table td {
      padding: 0.75rem 1rem;
      border-bottom: 1px solid rgba(255, 255, 255, 0.03);
      color: rgba(255, 255, 255, 0.85);
      font-family: 'JetBrains Mono', monospace;
      cursor: pointer;
      transition: background 0.15s;
    }
    .coupling-table tr:hover td {
      background: rgba(255, 255, 255, 0.02);
    }
    .coupling-table tr.selected td {
      background: rgba(99, 102, 241, 0.1) !important;
      border-left: 3px solid var(--neon-indigo);
      color: var(--text-main);
    }
    .coupling-metric-val {
      font-weight: bold;
      text-align: center;
    }
    .coupling-tag {
      padding: 2px 6px;
      border-radius: 4px;
      font-size: 0.65rem;
      font-weight: bold;
      text-transform: uppercase;
    }
    .tag-ok {
      background: rgba(16, 185, 129, 0.1);
      color: var(--neon-green);
      border: 1px solid rgba(16, 185, 129, 0.2);
    }
    .tag-cyclic {
      background: rgba(239, 68, 68, 0.1);
      color: var(--neon-red);
      border: 1px solid rgba(239, 68, 68, 0.2);
      box-shadow: 0 0 6px rgba(239, 68, 68, 0.1);
    }
    .coupling-table-wrapper::-webkit-scrollbar {
      width: 6px;
    }
    .coupling-table-wrapper::-webkit-scrollbar-track {
      background: transparent;
    }
    .coupling-table-wrapper::-webkit-scrollbar-thumb {
      background: rgba(255, 255, 255, 0.1);
      border-radius: 3px;
    }
    .coupling-table-wrapper::-webkit-scrollbar-thumb:hover {
      background: rgba(255, 255, 255, 0.2);
    }
  `;
  document.head.appendChild(style);
}

export function renderASTGraph(data: ASTData, modifiedFiles: string[] = []): void {
  const svg = document.getElementById('ast-svg-canvas');
  if (svg) svg.style.display = 'none';

  const oldCanvas = document.getElementById('ast-canvas');
  if (oldCanvas) oldCanvas.remove();

  const viewport = document.getElementById('graph-viewport-div');
  if (!viewport) return;

  injectStyles();

  // If coupling-container already exists, just update in background and return
  const existingContainer = viewport.querySelector('.coupling-container');
  const searchInput = viewport.querySelector('.coupling-search') as HTMLInputElement;
  if (existingContainer && searchInput) {
    updateCouplingReport(data, searchInput.value.trim().toLowerCase());
    return;
  }

  viewport.replaceChildren();

  // Create Container
  const container = document.createElement('div');
  container.className = 'coupling-container';

  // Search Input
  const newSearchInput = document.createElement('input');
  newSearchInput.type = 'text';
  newSearchInput.className = 'coupling-search';
  newSearchInput.placeholder = 'Search packages by path or import name...';
  container.appendChild(newSearchInput);

  // Table wrapper
  const tableWrapper = document.createElement('div');
  tableWrapper.className = 'coupling-table-wrapper';

  const table = document.createElement('table');
  table.className = 'coupling-table';
  table.innerHTML = `
    <thead>
      <tr>
        <th style="width: 50%;">Package Path</th>
        <th style="text-align: center; width: 12%;">Afferent (Ca)</th>
        <th style="text-align: center; width: 12%;">Efferent (Ce)</th>
        <th style="text-align: center; width: 14%;">Instability (I)</th>
        <th style="text-align: center; width: 12%;">Cycle</th>
      </tr>
    </thead>
    <tbody id="coupling-table-body">
      <tr>
        <td colspan="5" style="text-align: center; color: var(--text-muted); padding: 2rem;">
          Querying package coupling stats...
        </td>
      </tr>
    </tbody>
  `;
  tableWrapper.appendChild(table);
  container.appendChild(tableWrapper);
  viewport.appendChild(container);

  // Trigger Async Fetch
  fetchCouplingReport(data, newSearchInput);
}

async function fetchCouplingReport(data: ASTData, searchInput: HTMLInputElement): Promise<void> {
  const tableBody = document.getElementById('coupling-table-body');
  if (!tableBody) return;

  try {
    const res = await fetch('/api/coupling');
    const report: CouplingReport = await res.json();
    cachedCouplingData = report;

    renderTableRows(report.packages, data, '');

    // Setup real-time search filtering
    searchInput.addEventListener('input', () => {
      renderTableRows(report.packages, data, searchInput.value.trim().toLowerCase());
    });

    // Populate cycle sidebar panel
    renderCycleSidebar(report);

  } catch (err: any) {
    tableBody.innerHTML = `
      <tr>
        <td colspan="5" style="text-align: center; color: var(--neon-red); padding: 2rem;">
          Failed to fetch coupling telemetry: ${err.message || err}
        </td>
      </tr>
    `;
  }
}

let lastCouplingFetchTime = 0;
let isCouplingFetching = false;

async function updateCouplingReport(data: ASTData, filterText: string): Promise<void> {
  const tableBody = document.getElementById('coupling-table-body');
  if (!tableBody) return;

  const now = Date.now();
  if (isCouplingFetching || (now - lastCouplingFetchTime < 30000)) {
    if (cachedCouplingData) {
      renderTableRows(cachedCouplingData.packages, data, filterText);
      renderCycleSidebar(cachedCouplingData);
    }
    return;
  }

  isCouplingFetching = true;
  lastCouplingFetchTime = now;

  try {
    const res = await fetch('/api/coupling');
    const report: CouplingReport = await res.json();
    cachedCouplingData = report;

    renderTableRows(report.packages, data, filterText);
    renderCycleSidebar(report);
  } catch (err: any) {
    console.error('Failed to update coupling report in background:', err);
  } finally {
    isCouplingFetching = false;
  }
}

function renderTableRows(packages: PackageCouplingStats[], data: ASTData, query: string): void {
  const tableBody = document.getElementById('coupling-table-body');
  if (!tableBody) return;
  tableBody.replaceChildren();

  const filtered = packages.filter(pkg => pkg.package.toLowerCase().includes(query));

  if (filtered.length === 0) {
    const row = document.createElement('tr');
    row.innerHTML = `
      <td colspan="5" style="text-align: center; color: var(--text-muted); padding: 2rem;">
        No packages match search filter.
      </td>
    `;
    tableBody.appendChild(row);
    return;
  }

  filtered.forEach(pkg => {
    const totalCoupling = pkg.afferent + pkg.efferent;
    const instability = totalCoupling > 0 ? (pkg.efferent / totalCoupling).toFixed(2) : '0.00';
    const isCyclic = cachedCouplingData?.cycle && cachedCouplingData.cycle.includes(pkg.package);

    const row = document.createElement('tr');
    if (currentSelectedPackage === pkg.package) {
      row.className = 'selected';
      if (currentInspectedFile) {
        inspectFile(currentInspectedFile, pkg, data);
      } else {
        populateInspector(pkg, data);
      }
    }

    row.innerHTML = `
      <td style="color: var(--neon-blue); font-weight: 500;">${pkg.package}</td>
      <td class="coupling-metric-val" style="color: #60a5fa;">${pkg.afferent}</td>
      <td class="coupling-metric-val" style="color: #f472b6;">${pkg.efferent}</td>
      <td class="coupling-metric-val" style="color: #fb7185; font-weight: bold;">${instability}</td>
      <td style="text-align: center;">
        <span class="coupling-tag ${isCyclic ? 'tag-cyclic' : 'tag-ok'}">
          ${isCyclic ? 'cyclic' : 'ok'}
        </span>
      </td>
    `;

    row.addEventListener('click', () => {
      document.querySelectorAll('.coupling-table tr').forEach(r => r.classList.remove('selected'));
      row.classList.add('selected');
      currentSelectedPackage = pkg.package;
      populateInspector(pkg, data);
    });

    tableBody.appendChild(row);
  });
}

function populateInspector(pkg: PackageCouplingStats, data: ASTData): void {
  currentInspectedFile = null;
  const inspectorBody = document.getElementById('ast-inspector-body');
  if (!inspectorBody) return;
  inspectorBody.replaceChildren();
  inspectorBody.style.justifyContent = 'flex-start';
  inspectorBody.style.alignItems = 'stretch';
  inspectorBody.style.textAlign = 'left';

  // Find Go files belonging to this package
  const pkgFiles = data.nodes.filter(n => {
    if (!n.id.endsWith('.go')) return false;
    const dir = n.id.slice(0, n.id.lastIndexOf('/'));
    return dir === pkg.package;
  });

  const header = document.createElement('div');
  header.style.borderBottom = '1px solid var(--border-indigo)';
  header.style.paddingBottom = '0.5rem';
  header.style.marginBottom = '0.75rem';

  const title = document.createElement('div');
  title.style.fontSize = '0.95rem';
  title.style.fontWeight = 'bold';
  title.style.color = 'var(--neon-blue)';
  title.textContent = pkg.package;
  header.appendChild(title);

  const stats = document.createElement('div');
  stats.style.fontSize = '0.7rem';
  stats.style.color = 'var(--text-muted)';
  stats.style.marginTop = '4px';
  stats.textContent = `Contains ${pkgFiles.length} file(s) • Instability: ${(pkg.efferent / (pkg.afferent + pkg.efferent || 1)).toFixed(2)}`;
  header.appendChild(stats);
  inspectorBody.appendChild(header);

  // Files list
  const filesTitle = document.createElement('div');
  filesTitle.style.fontSize = '0.75rem';
  filesTitle.style.fontWeight = 'bold';
  filesTitle.style.color = 'var(--neon-indigo)';
  filesTitle.style.marginBottom = '0.25rem';
  filesTitle.textContent = `Source Files (${pkgFiles.length})`;
  inspectorBody.appendChild(filesTitle);

  const filesList = document.createElement('div');
  filesList.style.maxHeight = '90px';
  filesList.style.overflowY = 'auto';
  filesList.style.background = 'rgba(255, 255, 255, 0.01)';
  filesList.style.border = '1px solid rgba(255, 255, 255, 0.05)';
  filesList.style.borderRadius = '4px';
  filesList.style.padding = '0.5rem';
  filesList.style.fontSize = '0.7rem';
  filesList.style.fontFamily = "'JetBrains Mono', monospace";
  filesList.style.display = 'flex';
  filesList.style.flexDirection = 'column';
  filesList.style.gap = '4px';

  if (pkgFiles.length === 0) {
    filesList.textContent = 'No Go source files detected.';
  } else {
    pkgFiles.forEach(file => {
      const item = document.createElement('div');
      item.textContent = file.label;
      item.style.color = 'var(--neon-blue)';
      item.style.cursor = 'pointer';
      item.style.textDecoration = 'underline';
      item.addEventListener('mouseenter', () => {
        item.style.color = 'var(--neon-purple)';
      });
      item.addEventListener('mouseleave', () => {
        item.style.color = 'var(--neon-blue)';
      });
      item.addEventListener('click', () => {
        inspectFile(file.id, pkg, data);
      });
      filesList.appendChild(item);
    });
  }
  inspectorBody.appendChild(filesList);

  // Efferent Imports list
  const ceTitle = document.createElement('div');
  ceTitle.style.fontSize = '0.75rem';
  ceTitle.style.fontWeight = 'bold';
  ceTitle.style.color = 'var(--neon-pink)';
  ceTitle.style.marginTop = '0.75rem';
  ceTitle.style.marginBottom = '0.25rem';
  ceTitle.textContent = `Efferent Dependencies - Ce (${pkg.efferent})`;
  inspectorBody.appendChild(ceTitle);

  const ceList = document.createElement('div');
  ceList.style.maxHeight = '95px';
  ceList.style.overflowY = 'auto';
  ceList.style.background = 'rgba(255, 255, 255, 0.01)';
  ceList.style.border = '1px solid rgba(255, 255, 255, 0.05)';
  ceList.style.borderRadius = '4px';
  ceList.style.padding = '0.5rem';
  ceList.style.fontSize = '0.7rem';
  ceList.style.fontFamily = "'JetBrains Mono', monospace";
  ceList.style.display = 'flex';
  ceList.style.flexDirection = 'column';
  ceList.style.gap = '4px';

  if (!pkg.imports || pkg.imports.length === 0) {
    ceList.textContent = 'Imports no internal packages.';
  } else {
    pkg.imports.forEach(imp => {
      const item = document.createElement('div');
      item.style.color = 'var(--neon-pink)';
      item.textContent = imp;
      ceList.appendChild(item);
    });
  }
  inspectorBody.appendChild(ceList);

  // Afferent Dependents list
  const caTitle = document.createElement('div');
  caTitle.style.fontSize = '0.75rem';
  caTitle.style.fontWeight = 'bold';
  caTitle.style.color = 'var(--neon-green)';
  caTitle.style.marginTop = '0.75rem';
  caTitle.style.marginBottom = '0.25rem';
  caTitle.textContent = `Afferent Dependents - Ca (${pkg.afferent})`;
  inspectorBody.appendChild(caTitle);

  const caList = document.createElement('div');
  caList.style.maxHeight = '95px';
  caList.style.overflowY = 'auto';
  caList.style.background = 'rgba(255, 255, 255, 0.01)';
  caList.style.border = '1px solid rgba(255, 255, 255, 0.05)';
  caList.style.borderRadius = '4px';
  caList.style.padding = '0.5rem';
  caList.style.fontSize = '0.7rem';
  caList.style.fontFamily = "'JetBrains Mono', monospace";
  caList.style.display = 'flex';
  caList.style.flexDirection = 'column';
  caList.style.gap = '4px';

  if (!pkg.importedBy || pkg.importedBy.length === 0) {
    caList.textContent = 'Not imported by any internal packages.';
  } else {
    pkg.importedBy.forEach(imp => {
      const item = document.createElement('div');
      item.style.color = 'var(--neon-green)';
      item.textContent = imp;
      caList.appendChild(item);
    });
  }
  inspectorBody.appendChild(caList);
}

function inspectFile(filePath: string, pkg: PackageCouplingStats, data: ASTData): void {
  currentInspectedFile = filePath;
  (window as any).lastInspectedFile = filePath;
  const inspectorBody = document.getElementById('ast-inspector-body');
  if (!inspectorBody) return;
  inspectorBody.replaceChildren();

  // Show loading state
  const loading = document.createElement('div');
  loading.style.color = 'var(--text-muted)';
  loading.style.padding = '1.5rem';
  loading.style.fontSize = '0.75rem';
  loading.style.textAlign = 'center';
  loading.textContent = 'Querying file AST symbols...';
  inspectorBody.appendChild(loading);

  fetch(`/api/inspect?file=${encodeURIComponent(filePath)}`)
    .then(res => res.json())
    .then(fileData => {
      if (fileData.error) {
        throw new Error(fileData.error);
      }
      inspectorBody.replaceChildren();

      // Back Button
      const backBtn = document.createElement('button');
      backBtn.style.padding = '4px 10px';
      backBtn.style.fontSize = '0.65rem';
      backBtn.style.fontFamily = "'Outfit', sans-serif";
      backBtn.style.border = '1px solid var(--border-indigo)';
      backBtn.style.background = 'rgba(255, 255, 255, 0.03)';
      backBtn.style.color = 'var(--text-main)';
      backBtn.style.borderRadius = '4px';
      backBtn.style.cursor = 'pointer';
      backBtn.style.marginBottom = '0.75rem';
      backBtn.style.transition = 'background 0.2s';
      backBtn.textContent = '← Back to Package';
      backBtn.addEventListener('mouseenter', () => {
        backBtn.style.background = 'rgba(255, 255, 255, 0.08)';
      });
      backBtn.addEventListener('mouseleave', () => {
        backBtn.style.background = 'rgba(255, 255, 255, 0.03)';
      });
      backBtn.addEventListener('click', () => {
        populateInspector(pkg, data);
      });
      inspectorBody.appendChild(backBtn);

      const header = document.createElement('div');
      header.style.borderBottom = '1px solid var(--border-indigo)';
      header.style.paddingBottom = '0.5rem';
      header.style.marginBottom = '0.75rem';

      const title = document.createElement('div');
      title.style.fontSize = '0.9rem';
      title.style.fontWeight = 'bold';
      title.style.color = 'var(--neon-blue)';
      title.style.wordBreak = 'break-all';
      title.textContent = fileData.filePath || filePath;
      header.appendChild(title);

      const stats = document.createElement('div');
      stats.style.fontSize = '0.65rem';
      stats.style.color = 'var(--text-muted)';
      stats.style.marginTop = '4px';
      const sizeKB = (fileData.size / 1024).toFixed(2);
      stats.textContent = `Size: ${sizeKB} KB (${fileData.size} bytes) • Lines: ${fileData.linesCount}`;
      header.appendChild(stats);
      inspectorBody.appendChild(header);

      // Imports List
      const impTitle = document.createElement('div');
      impTitle.style.fontSize = '0.75rem';
      impTitle.style.fontWeight = 'bold';
      impTitle.style.color = 'var(--neon-pink)';
      impTitle.style.marginBottom = '0.25rem';
      impTitle.textContent = `File Imports (${fileData.imports ? fileData.imports.length : 0})`;
      inspectorBody.appendChild(impTitle);

      const impList = document.createElement('div');
      impList.style.maxHeight = '90px';
      impList.style.overflowY = 'auto';
      impList.style.background = 'rgba(255, 255, 255, 0.01)';
      impList.style.border = '1px solid rgba(255, 255, 255, 0.05)';
      impList.style.borderRadius = '4px';
      impList.style.padding = '0.5rem';
      impList.style.fontSize = '0.65rem';
      impList.style.fontFamily = "'JetBrains Mono', monospace";
      impList.style.display = 'flex';
      impList.style.flexDirection = 'column';
      impList.style.gap = '4px';

      if (!fileData.imports || fileData.imports.length === 0) {
        impList.textContent = 'No internal imports.';
      } else {
        fileData.imports.forEach((imp: string) => {
          const item = document.createElement('div');
          item.style.color = 'var(--neon-pink)';
          item.textContent = imp;
          impList.appendChild(item);
        });
      }
      inspectorBody.appendChild(impList);

      // Symbols List
      const symTitle = document.createElement('div');
      symTitle.style.fontSize = '0.75rem';
      symTitle.style.fontWeight = 'bold';
      symTitle.style.color = 'var(--neon-indigo)';
      symTitle.style.marginTop = '0.75rem';
      symTitle.style.marginBottom = '0.25rem';
      symTitle.textContent = `Declared Symbols (${fileData.symbols ? fileData.symbols.length : 0})`;
      inspectorBody.appendChild(symTitle);

      const symList = document.createElement('div');
      symList.style.maxHeight = '180px';
      symList.style.overflowY = 'auto';
      symList.style.background = 'rgba(255, 255, 255, 0.01)';
      symList.style.border = '1px solid rgba(255, 255, 255, 0.05)';
      symList.style.borderRadius = '4px';
      symList.style.padding = '0.5rem';
      symList.style.fontSize = '0.65rem';
      symList.style.fontFamily = "'JetBrains Mono', monospace";
      symList.style.display = 'flex';
      symList.style.flexDirection = 'column';
      symList.style.gap = '4px';

      if (!fileData.symbols || fileData.symbols.length === 0) {
        symList.textContent = 'No symbols exported.';
      } else {
        fileData.symbols.forEach((sym: string) => {
          const item = document.createElement('div');
          item.style.color = '#e2e8f0';
          item.textContent = sym;
          symList.appendChild(item);
        });
      }
      inspectorBody.appendChild(symList);
    })
    .catch(err => {
      inspectorBody.replaceChildren();
      const errorDiv = document.createElement('div');
      errorDiv.style.color = 'var(--neon-red)';
      errorDiv.style.padding = '1rem';
      errorDiv.style.fontSize = '0.75rem';
      errorDiv.textContent = 'Failed to inspect file symbols: ' + err.message;
      inspectorBody.appendChild(errorDiv);
    });
}


function renderCycleSidebar(report: CouplingReport): void {
  const container = document.getElementById('ast-cycles-list-container');
  if (!container) return;
  container.replaceChildren();

  if (!report.cycle || report.cycle.length === 0) {
    const box = document.createElement('div');
    box.style.display = 'flex';
    box.style.flexDirection = 'column';
    box.style.alignItems = 'center';
    box.style.gap = '8px';
    box.style.padding = '1rem';
    box.style.border = '1px dashed var(--border-indigo)';
    box.style.borderRadius = '6px';
    box.style.background = 'rgba(16, 185, 129, 0.02)';
    box.style.color = 'var(--neon-green)';
    box.style.textAlign = 'center';

    const check = document.createElement('div');
    check.style.fontSize = '1.5rem';
    check.textContent = '✅';
    box.appendChild(check);

    const txt = document.createElement('div');
    txt.textContent = 'No package coupling cycles detected. Deterministic architecture!';
    box.appendChild(txt);

    container.appendChild(box);
  } else {
    const box = document.createElement('div');
    box.style.display = 'flex';
    box.style.flexDirection = 'column';
    box.style.gap = '8px';
    box.style.padding = '0.75rem';
    box.style.border = '1px solid rgba(239, 68, 68, 0.3)';
    box.style.borderRadius = '6px';
    box.style.background = 'rgba(239, 68, 68, 0.03)';
    box.style.color = 'var(--neon-red)';

    const head = document.createElement('div');
    head.style.fontWeight = 'bold';
    head.style.fontSize = '0.75rem';
    head.textContent = '🚨 CIRCULAR PACKAGE COUPLING DETECTED';
    box.appendChild(head);

    const path = document.createElement('div');
    path.style.fontFamily = "'JetBrains Mono', monospace";
    path.style.fontSize = '0.7rem';
    path.style.lineHeight = '1.4';
    path.textContent = report.cycle.join(' ➔ ');
    box.appendChild(path);

    container.appendChild(box);
  }
}

(window as any).inspectFile = inspectFile;

