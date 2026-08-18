import { showToast } from '../toast.js';
import { renderMemoryClusterCanvas } from '../nebula.js';
import {
  cachedLessons,
  selectedLessonHash,
  renderLessonsList,
  setCachedLessons
} from './memoryCore.js';
import { renderTechnicalDebtRegistry } from './memoryDashboard.js';
import { selectMemoryNode } from './memoryPruning.js';

let lowContextCollapsed = false;
let redundantCollapsed = false;
let lastAdvisorLessons: any[] | null = null;
let lastSelectedLessonHash: string | null = null;
const stopwords = new Set(['the', 'is', 'a', 'and', 'or', 'in', 'on', 'to', 'for', 'of', 'with', 'by', 'as', 'at', 'this', 'that', 'commit', 'architectural', 'spec', 'deep', 'review', 'chose', 'because', 'alternatives', 'considered', 'falsifiable', 'if']);

function tokenize(text: string): Set<string> {
  let cleanText = text.replace(/^(Commit: \w+|Architectural Spec:|Deep review:)/i, '');
  const tokens = cleanText.toLowerCase()
    .replace(/[.,\/#!$%\^&\*;:{}=\-_`~()?"']/g, ' ')
    .split(/\s+/)
    .map(t => t.trim())
    .filter(t => t.length > 3 && !stopwords.has(t));
  return new Set(tokens);
}

function calculateJaccardPreTokenized(idxA: number, idxB: number, tokenizedSets: Set<string>[]): number {
  const setA = tokenizedSets[idxA];
  const setB = tokenizedSets[idxB];
  if (setA.size === 0 || setB.size === 0) return 0;

  let intersectionSize = 0;
  setA.forEach(token => {
    if (setB.has(token)) intersectionSize++;
  });

  const unionSize = setA.size + setB.size - intersectionSize;
  return unionSize > 0 ? (intersectionSize / unionSize) : 0;
}

function createMetricCard(title: string, value: string | number, color: string) {
  const card = document.createElement('div');
  card.style.background = 'rgba(255, 255, 255, 0.01)';
  card.style.border = '1px solid rgba(255, 255, 255, 0.03)';
  card.style.borderRadius = '6px';
  card.style.padding = '8px';
  card.style.display = 'flex';
  card.style.flexDirection = 'column';
  card.style.alignItems = 'center';
  card.style.justifyContent = 'center';

  const t = document.createElement('div');
  t.style.fontSize = '0.6rem';
  t.style.color = 'var(--text-muted)';
  t.style.textTransform = 'uppercase';
  t.style.textAlign = 'center';
  t.style.marginBottom = '2px';
  t.style.fontWeight = 'bold';
  t.textContent = title;

  const v = document.createElement('div');
  v.style.fontSize = '1.1rem';
  v.style.fontWeight = '800';
  v.style.color = color;
  v.style.fontFamily = 'monospace';
  v.textContent = String(value);

  card.appendChild(t);
  card.appendChild(v);
  return card;
}

function createCollapsibleSection(
  titleText: string,
  count: number,
  renderContent: (body: HTMLDivElement) => void,
  pruneAllFn?: () => void
) {
  const section = document.createElement('div');
  section.style.display = 'flex';
  section.style.flexDirection = 'column';
  section.style.border = '1px solid rgba(255,255,255,0.03)';
  section.style.borderRadius = '6px';
  section.style.background = 'rgba(255,255,255,0.01)';
  section.style.overflow = 'hidden';

  const header = document.createElement('div');
  header.style.padding = '8px 12px';
  header.style.background = 'rgba(255, 255, 255, 0.02)';
  header.style.display = 'flex';
  header.style.justifyContent = 'space-between';
  header.style.alignItems = 'center';
  header.style.cursor = 'pointer';
  header.style.userSelect = 'none';

  const titleGroup = document.createElement('div');
  titleGroup.style.display = 'flex';
  titleGroup.style.alignItems = 'center';
  titleGroup.style.gap = '6px';
  
  const chevron = document.createElement('span');
  chevron.textContent = '▼';
  chevron.style.fontSize = '0.6rem';
  chevron.style.color = 'var(--text-muted)';
  chevron.style.transition = 'transform 0.2s';

  const label = document.createElement('span');
  label.style.fontSize = '0.75rem';
  label.style.fontWeight = 'bold';
  label.style.color = 'rgba(255,255,255,0.85)';
  label.textContent = titleText;

  titleGroup.appendChild(chevron);
  titleGroup.appendChild(label);
  header.appendChild(titleGroup);

  const rightGroup = document.createElement('div');
  rightGroup.style.display = 'flex';
  rightGroup.style.alignItems = 'center';
  rightGroup.style.gap = '8px';

  if (pruneAllFn && count > 0) {
    const pruneAllBtn = document.createElement('button');
    pruneAllBtn.textContent = '✂️ Prune All';
    pruneAllBtn.style.fontSize = '0.65rem';
    pruneAllBtn.style.padding = '2px 8px';
    pruneAllBtn.style.borderRadius = '4px';
    pruneAllBtn.style.background = 'transparent';
    pruneAllBtn.style.color = 'var(--neon-pink)';
    pruneAllBtn.style.border = '1px solid var(--neon-pink)';
    pruneAllBtn.style.cursor = 'pointer';
    pruneAllBtn.style.transition = 'all 0.2s';
    pruneAllBtn.style.fontWeight = 'bold';
    pruneAllBtn.onmouseover = () => {
      pruneAllBtn.style.background = 'rgba(236, 72, 153, 0.1)';
    };
    pruneAllBtn.onmouseout = () => {
      pruneAllBtn.style.background = 'transparent';
    };
    pruneAllBtn.onclick = (e) => {
      e.stopPropagation();
      pruneAllFn();
    };
    rightGroup.appendChild(pruneAllBtn);
  }

  const badge = document.createElement('span');
  badge.style.fontSize = '0.65rem';
  badge.style.background = count > 0 ? 'rgba(255, 255, 255, 0.08)' : 'rgba(255, 255, 255, 0.03)';
  badge.style.color = count > 0 ? 'var(--text-main)' : 'var(--text-muted)';
  badge.style.padding = '1px 6px';
  badge.style.borderRadius = '10px';
  badge.style.fontWeight = 'bold';
  badge.textContent = String(count);

  rightGroup.appendChild(badge);
  header.appendChild(rightGroup);

  const body = document.createElement('div');
  body.style.padding = '8px';
  body.style.display = 'flex';
  body.style.flexDirection = 'column';
  body.style.gap = '6px';
  body.style.transition = 'all 0.2s ease-in-out';

  let isCollapsed = titleText === 'Low-Context Candidates' ? lowContextCollapsed : redundantCollapsed;
  if (isCollapsed) {
    body.style.display = 'none';
    chevron.style.transform = 'rotate(-90deg)';
  } else {
    body.style.display = 'flex';
    chevron.style.transform = 'rotate(0deg)';
  }

  header.onclick = () => {
    isCollapsed = !isCollapsed;
    if (titleText === 'Low-Context Candidates') {
      lowContextCollapsed = isCollapsed;
    } else {
      redundantCollapsed = isCollapsed;
    }
    if (isCollapsed) {
      body.style.display = 'none';
      chevron.style.transform = 'rotate(-90deg)';
    } else {
      body.style.display = 'flex';
      chevron.style.transform = 'rotate(0deg)';
    }
  };

  renderContent(body);
  section.appendChild(header);
  section.appendChild(body);
  return section;
}

export function renderPruningAdvisorDashboard(container: HTMLElement): void {
  const hasWrapper = container.querySelector('.scrollable-indigo') !== null;
  if (hasWrapper && cachedLessons === lastAdvisorLessons && selectedLessonHash === lastSelectedLessonHash) {
    return;
  }
  lastAdvisorLessons = cachedLessons;
  lastSelectedLessonHash = selectedLessonHash;

  let savedScrollTop = 0;
  const existingWrapper = container.firstElementChild as HTMLElement | null;
  if (existingWrapper && existingWrapper.classList.contains('scrollable-indigo')) {
    savedScrollTop = existingWrapper.scrollTop;
  }

  container.replaceChildren();
  container.style.textAlign = 'left';
  container.style.justifyContent = 'flex-start';
  container.style.height = '100%';
  container.style.overflow = 'hidden';
  container.style.display = 'flex';
  container.style.flexDirection = 'column';
  
  const wrapper = document.createElement('div');
  wrapper.style.display = 'flex';
  wrapper.style.flexDirection = 'column';
  wrapper.style.height = '100%';
  wrapper.style.width = '100%';
  wrapper.style.overflowY = 'auto';
  wrapper.className = 'scrollable-indigo';
  wrapper.style.gap = '16px';
  wrapper.style.paddingRight = '4px';

  const headerDiv = document.createElement('div');
  headerDiv.style.display = 'flex';
  headerDiv.style.flexDirection = 'column';
  headerDiv.style.gap = '4px';

  const title = document.createElement('div');
  title.style.fontSize = '1rem';
  title.style.fontWeight = 'bold';
  title.style.color = 'var(--text-main)';
  title.style.display = 'flex';
  title.style.alignItems = 'center';
  title.style.gap = '8px';
  
  const iconSpan = document.createElement('span');
  iconSpan.textContent = '🧠';
  title.appendChild(iconSpan);
  
  const textSpan = document.createElement('span');
  textSpan.textContent = 'Somatic Pruning Advisor';
  title.appendChild(textSpan);

  const subtitle = document.createElement('div');
  subtitle.style.fontSize = '0.7rem';
  subtitle.style.color = 'var(--text-muted)';
  subtitle.textContent = 'Optimize somatic index size and RAG vector relevance.';

  headerDiv.appendChild(title);
  headerDiv.appendChild(subtitle);
  wrapper.appendChild(headerDiv);

  const tokenizedSets = cachedLessons.map(l => tokenize(l.insight || ''));

  const lowContextCandidates: any[] = [];
  const redundantPairs: { nodeA: any; nodeB: any; similarity: number }[] = [];
  
  cachedLessons.forEach(l => {
    if (!l.insight || l.insight.length < 60) {
      lowContextCandidates.push(l);
    }
  });

  for (let i = 0; i < cachedLessons.length; i++) {
    for (let j = i + 1; j < cachedLessons.length; j++) {
      const sim = calculateJaccardPreTokenized(i, j, tokenizedSets);
      if (sim > 0.65) {
        redundantPairs.push({ nodeA: cachedLessons[i], nodeB: cachedLessons[j], similarity: sim });
      }
    }
  }

  let score = 100;
  score -= lowContextCandidates.length * 2.5;
  score -= redundantPairs.length * 5.0;
  score = Math.max(0, Math.min(100, score));

  let grade = 'F';
  let statusColor = 'var(--neon-pink)';
  if (score >= 90) {
    grade = 'A';
    statusColor = 'var(--neon-green)';
  } else if (score >= 80) {
    grade = 'B';
    statusColor = 'var(--neon-yellow)';
  } else if (score >= 70) {
    grade = 'C';
    statusColor = '#f97316';
  } else if (score >= 60) {
    grade = 'D';
    statusColor = 'var(--neon-pink)';
  }

  const totalDensity = cachedLessons.reduce((acc, l) => acc + (l.insight ? l.insight.length : 0), 0);
  const avgDensity = cachedLessons.length > 0 ? Math.round(totalDensity / cachedLessons.length) : 0;

  const healthCard = document.createElement('div');
  healthCard.style.background = 'rgba(255, 255, 255, 0.02)';
  healthCard.style.border = '1px solid rgba(255, 255, 255, 0.05)';
  healthCard.style.borderRadius = '8px';
  healthCard.style.padding = '12px';
  healthCard.style.display = 'flex';
  healthCard.style.alignItems = 'center';
  healthCard.style.justifyContent = 'space-between';
  healthCard.style.boxShadow = 'inset 0 1px 1px rgba(255, 255, 255, 0.05)';

  const healthLeft = document.createElement('div');
  healthLeft.style.display = 'flex';
  healthLeft.style.flexDirection = 'column';
  healthLeft.style.gap = '2px';

  const healthTitle = document.createElement('div');
  healthTitle.style.fontSize = '0.75rem';
  healthTitle.style.color = 'var(--text-muted)';
  healthTitle.style.fontWeight = 'bold';
  healthTitle.textContent = 'SYSTEM SOMATIC HEALTH';

  const healthDetail = document.createElement('div');
  healthDetail.style.fontSize = '0.65rem';
  healthDetail.style.color = 'rgba(255,255,255,0.5)';
  healthDetail.textContent = score >= 90 ? 'Somatic memory is highly optimized!' : 'Prune low-context or redundant nodes to restore health.';

  healthLeft.appendChild(healthTitle);
  healthLeft.appendChild(healthDetail);

  const healthRight = document.createElement('div');
  healthRight.style.display = 'flex';
  healthRight.style.alignItems = 'center';
  healthRight.style.gap = '8px';

  const scoreText = document.createElement('div');
  scoreText.style.fontSize = '1.8rem';
  scoreText.style.fontWeight = '800';
  scoreText.style.color = statusColor;
  scoreText.style.fontFamily = 'monospace';
  scoreText.textContent = `${score}%`;

  const gradeBadge = document.createElement('div');
  gradeBadge.style.width = '28px';
  gradeBadge.style.height = '28px';
  gradeBadge.style.borderRadius = '50%';
  gradeBadge.style.background = statusColor;
  gradeBadge.style.color = '#0c0714';
  gradeBadge.style.display = 'flex';
  gradeBadge.style.alignItems = 'center';
  gradeBadge.style.justifyContent = 'center';
  gradeBadge.style.fontWeight = '900';
  gradeBadge.style.fontSize = '0.95rem';
  gradeBadge.textContent = grade;

  healthRight.appendChild(scoreText);
  healthRight.appendChild(gradeBadge);

  healthCard.appendChild(healthLeft);
  healthCard.appendChild(healthRight);
  wrapper.appendChild(healthCard);

  const grid = document.createElement('div');
  grid.style.display = 'grid';
  grid.style.gridTemplateColumns = 'repeat(2, 1fr)';
  grid.style.gap = '8px';

  const metric1 = createMetricCard('Total Somatic Nodes', cachedLessons.length, 'var(--text-main)');
  const metric2 = createMetricCard('Avg Context Density', `${avgDensity} ch`, 'var(--neon-blue)');
  const metric3 = createMetricCard('Low-Context Candidates', lowContextCandidates.length, lowContextCandidates.length > 0 ? 'var(--neon-yellow)' : 'var(--neon-green)');
  const metric4 = createMetricCard('Redundant Clusters', redundantPairs.length, redundantPairs.length > 0 ? 'var(--neon-pink)' : 'var(--neon-green)');

  grid.appendChild(metric1);
  grid.appendChild(metric2);
  grid.appendChild(metric3);
  grid.appendChild(metric4);
  wrapper.appendChild(grid);

  const debtCard = document.createElement('div');
  debtCard.style.background = 'rgba(255, 255, 255, 0.02)';
  debtCard.style.border = '1px solid rgba(255, 255, 255, 0.05)';
  debtCard.style.borderRadius = '8px';
  debtCard.style.padding = '12px';
  debtCard.style.display = 'flex';
  debtCard.style.flexDirection = 'column';
  debtCard.style.gap = '8px';
  debtCard.style.boxShadow = 'inset 0 1px 1px rgba(255, 255, 255, 0.05)';

  const debtHeader = document.createElement('div');
  debtHeader.style.display = 'flex';
  debtHeader.style.justifyContent = 'space-between';
  debtHeader.style.alignItems = 'center';

  const debtTitle = document.createElement('div');
  debtTitle.style.fontSize = '0.75rem';
  debtTitle.style.color = 'var(--text-muted)';
  debtTitle.style.fontWeight = 'bold';
  debtTitle.textContent = '⚖️ ACTIVE QUALITY DEBT REGISTRY';

  debtHeader.appendChild(debtTitle);
  debtCard.appendChild(debtHeader);

  const debtListContainer = document.createElement('div');
  debtListContainer.style.display = 'flex';
  debtListContainer.style.flexDirection = 'column';
  debtListContainer.style.gap = '6px';
  debtCard.appendChild(debtListContainer);

  renderTechnicalDebtRegistry(debtListContainer);
  wrapper.appendChild(debtCard);

  const listsContainer = document.createElement('div');
  listsContainer.style.display = 'flex';
  listsContainer.style.flexDirection = 'column';
  listsContainer.style.gap = '12px';

  const lowContextSection = createCollapsibleSection('Low-Context Candidates', lowContextCandidates.length, (body) => {
    if (lowContextCandidates.length === 0) {
      const empty = document.createElement('div');
      empty.style.fontSize = '0.65rem';
      empty.style.color = 'var(--neon-green)';
      empty.style.fontStyle = 'italic';
      empty.style.textAlign = 'center';
      empty.style.padding = '12px 0';
      empty.textContent = '✨ Great job! All memory nodes have rich context density!';
      body.appendChild(empty);
      return;
    }

    lowContextCandidates.forEach(node => {
      const card = document.createElement('div');
      card.style.background = 'rgba(255, 255, 255, 0.01)';
      card.style.border = '1px solid rgba(255, 255, 255, 0.03)';
      card.style.borderRadius = '4px';
      card.style.padding = '6px 8px';
      card.style.cursor = 'pointer';
      card.style.transition = 'all 0.15s';
      card.style.display = 'flex';
      card.style.flexDirection = 'column';
      card.style.gap = '2px';

      card.onmouseover = () => {
        card.style.border = '1px solid var(--neon-yellow)';
        card.style.background = 'rgba(255, 255, 255, 0.03)';
      };
      card.onmouseout = () => {
        card.style.border = '1px solid rgba(255, 255, 255, 0.03)';
        card.style.background = 'rgba(255, 255, 255, 0.01)';
      };

      card.onclick = () => selectMemoryNode(node.commitHash);

      const topRow = document.createElement('div');
      topRow.style.display = 'flex';
      topRow.style.justifyContent = 'space-between';
      topRow.style.fontSize = '0.65rem';

      const hashSpan = document.createElement('span');
      hashSpan.style.fontFamily = 'monospace';
      hashSpan.style.fontWeight = 'bold';
      hashSpan.style.color = 'var(--neon-yellow)';
      hashSpan.textContent = node.commitHash.substring(0, 7);

      const sizeSpan = document.createElement('span');
      sizeSpan.style.color = 'var(--text-muted)';
      sizeSpan.textContent = `${node.insight ? node.insight.length : 0} chars`;

      topRow.appendChild(hashSpan);
      topRow.appendChild(sizeSpan);

      const textPreview = document.createElement('div');
      textPreview.style.fontSize = '0.65rem';
      textPreview.style.color = 'rgba(255,255,255,0.7)';
      textPreview.style.whiteSpace = 'nowrap';
      textPreview.style.overflow = 'hidden';
      textPreview.style.textOverflow = 'ellipsis';
      textPreview.textContent = node.insight || '(empty)';

      card.appendChild(topRow);
      card.appendChild(textPreview);
      body.appendChild(card);
    });
  }, () => {
    const consent = confirm(`Are you sure you want to permanently prune ALL ${lowContextCandidates.length} low-context nodes? This cannot be undone.`);
    if (consent) {
      showToast(`Pruning ${lowContextCandidates.length} nodes...`);
      Promise.all(
        lowContextCandidates.map(node =>
          fetch('/api/memory/prune', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ commitHash: node.commitHash })
          })
        )
      )
      .then(() => {
        showToast('Successfully pruned all low-context nodes!', 'success');
        const candidateHashes = new Set(lowContextCandidates.map(n => n.commitHash));
        setCachedLessons(cachedLessons.filter(l => !candidateHashes.has(l.commitHash)));
        renderLessonsList();
        renderMemoryClusterCanvas(cachedLessons, selectedLessonHash, selectMemoryNode);
        renderPruningAdvisorDashboard(container);
      })
      .catch(err => {
        showToast(`Error pruning nodes: ${err.message}`, 'error');
      });
    }
  });

  const redundantSection = createCollapsibleSection('Redundant Memory Clusters', redundantPairs.length, (body) => {
    if (redundantPairs.length === 0) {
      const empty = document.createElement('div');
      empty.style.fontSize = '0.65rem';
      empty.style.color = 'var(--neon-green)';
      empty.style.fontStyle = 'italic';
      empty.style.textAlign = 'center';
      empty.style.padding = '12px 0';
      empty.textContent = '✨ Excellent! No redundant concept overlaps detected.';
      body.appendChild(empty);
      return;
    }

    redundantPairs.forEach(pair => {
      const card = document.createElement('div');
      card.style.background = 'rgba(255, 255, 255, 0.01)';
      card.style.border = '1px solid rgba(255, 255, 255, 0.03)';
      card.style.borderRadius = '4px';
      card.style.padding = '6px 8px';
      card.style.cursor = 'pointer';
      card.style.transition = 'all 0.15s';
      card.style.display = 'flex';
      card.style.flexDirection = 'column';
      card.style.gap = '2px';

      card.onmouseover = () => {
        card.style.border = '1px solid var(--neon-pink)';
        card.style.background = 'rgba(255, 255, 255, 0.03)';
      };
      card.onmouseout = () => {
        card.style.border = '1px solid rgba(255, 255, 255, 0.03)';
        card.style.background = 'rgba(255, 255, 255, 0.01)';
      };

      card.onclick = () => selectMemoryNode(pair.nodeA.commitHash);

      const topRow = document.createElement('div');
      topRow.style.display = 'flex';
      topRow.style.justifyContent = 'space-between';
      topRow.style.fontSize = '0.65rem';
      topRow.style.alignItems = 'center';

      const nodesSpan = document.createElement('span');
      nodesSpan.style.fontFamily = 'monospace';
      nodesSpan.style.color = 'var(--text-main)';
      
      const nodeAHash = document.createElement('span');
      nodeAHash.style.color = 'var(--neon-pink)';
      nodeAHash.style.fontWeight = 'bold';
      nodeAHash.textContent = pair.nodeA.commitHash.substring(0, 7);
      
      const arrowText = document.createTextNode(' ↔ ');
      
      const nodeBHash = document.createElement('span');
      nodeBHash.style.color = 'var(--neon-pink)';
      nodeBHash.style.fontWeight = 'bold';
      nodeBHash.textContent = pair.nodeB.commitHash.substring(0, 7);

      nodesSpan.appendChild(nodeAHash);
      nodesSpan.appendChild(arrowText);
      nodesSpan.appendChild(nodeBHash);

      const simSpan = document.createElement('span');
      simSpan.style.color = 'var(--neon-pink)';
      simSpan.style.fontWeight = 'bold';
      simSpan.textContent = `${Math.round(pair.similarity * 100)}% Match`;

      topRow.appendChild(nodesSpan);
      topRow.appendChild(simSpan);

      const previewA = document.createElement('div');
      previewA.style.fontSize = '0.6rem';
      previewA.style.color = 'rgba(255,255,255,0.5)';
      previewA.style.whiteSpace = 'nowrap';
      previewA.style.overflow = 'hidden';
      previewA.style.textOverflow = 'ellipsis';
      previewA.textContent = `A: ${pair.nodeA.insight || ''}`;

      const previewB = document.createElement('div');
      previewB.style.fontSize = '0.6rem';
      previewB.style.color = 'rgba(255,255,255,0.5)';
      previewB.style.whiteSpace = 'nowrap';
      previewB.style.overflow = 'hidden';
      previewB.style.textOverflow = 'ellipsis';
      previewB.textContent = `B: ${pair.nodeB.insight || ''}`;

      card.appendChild(topRow);
      card.appendChild(previewA);
      card.appendChild(previewB);
      body.appendChild(card);
    });
  }, () => {
    const hashesToPrune = Array.from(new Set(redundantPairs.map(p => p.nodeB.commitHash)));
    const consent = confirm(`Are you sure you want to permanently prune the redundant node in each of the ${redundantPairs.length} clusters (total of ${hashesToPrune.length} unique nodes)? This cannot be undone.`);
    if (consent) {
      showToast(`Pruning ${hashesToPrune.length} redundant nodes...`);
      Promise.all(
        hashesToPrune.map(hash =>
          fetch('/api/memory/prune', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ commitHash: hash })
          })
        )
      )
      .then(() => {
        showToast('Successfully pruned redundant nodes!', 'success');
        const pruneSet = new Set(hashesToPrune);
        setCachedLessons(cachedLessons.filter(l => !pruneSet.has(l.commitHash)));
        renderLessonsList();
        renderMemoryClusterCanvas(cachedLessons, selectedLessonHash, selectMemoryNode);
        renderPruningAdvisorDashboard(container);
      })
      .catch(err => {
        showToast(`Error pruning redundant nodes: ${err.message}`, 'error');
      });
    }
  });

  listsContainer.appendChild(lowContextSection);
  listsContainer.appendChild(redundantSection);
  wrapper.appendChild(listsContainer);

  container.appendChild(wrapper);

  if (savedScrollTop > 0) {
    wrapper.scrollTop = savedScrollTop;
  }
}
