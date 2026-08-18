import { showToast } from '../toast.js';
import { renderMemoryClusterCanvas } from '../nebula.js';

export let cachedLessons: any[] = [];
export let activeMemoryCategory = 'All';
export let activeMemoryTag = 'All';
export let activeSearchQuery = '';
export let selectedLessonHash: string | null = null;
let searchDebounceTimer: any = null;

export let showSwarmTraces = false;

(window as any).toggleSwarmTracesCallback = () => {
    showSwarmTraces = !showSwarmTraces;
    if ((window as any).refreshData) {
        (window as any).refreshData();
    }
};

export const presetColors = [
  'var(--neon-green)',
  'var(--neon-blue)',
  'var(--neon-pink)',
  'var(--neon-yellow)',
  'var(--neon-purple)',
  'var(--neon-indigo)',
  '#06b6d4',
  '#f97316'
];

export function setCachedLessons(val: any[]) {
  cachedLessons = val;
}

export function setSelectedLessonHash(val: string | null) {
  selectedLessonHash = val;
}

export function setActiveTags(val: string[]) {
  activeTags = val;
}

export let activeTags: string[] = [];

export function updateTimelineUI(lessons: any[]): void {
  let filtered = Array.isArray(lessons) ? lessons : [];
  if (!showSwarmTraces) {
    filtered = filtered.filter(l => !l.isAgent);
  }
  cachedLessons = filtered;

  // Extract unique categories from the database lessons list, starting with standard categories
  const categories = new Set<string>(['metadata', 'submodules', 'xss-security', 'tdd-gates', 'general', 'walkthrough', 'explainer', 'quiz']);
  cachedLessons.forEach(l => {
    if (l.category && l.category.trim()) {
      categories.add(l.category.trim());
    }
  });

  // Extract unique tags
  const uniqueTags = new Set<string>();
  lessons.forEach(l => {
    if (Array.isArray(l.tags)) {
      l.tags.forEach((t: string) => {
        if (t && t.trim()) uniqueTags.add(t.trim());
      });
    }
  });

  // Re-render tags container programmatically to match database categories and sub-tags!
  const tagsContainer = document.getElementById('memory-tags-container');
  if (tagsContainer) {
    tagsContainer.replaceChildren();
    tagsContainer.style.display = 'flex';
    tagsContainer.style.flexDirection = 'column';
    tagsContainer.style.gap = '8px';

    // 1. Categories Section Label
    const catLabel = document.createElement('div');
    catLabel.style.fontSize = '0.65rem';
    catLabel.style.color = 'var(--text-muted)';
    catLabel.style.textTransform = 'uppercase';
    catLabel.style.fontWeight = 'bold';
    catLabel.style.letterSpacing = '0.05em';
    catLabel.textContent = 'Categories';
    tagsContainer.appendChild(catLabel);

    const catRow = document.createElement('div');
    catRow.style.display = 'flex';
    catRow.style.flexWrap = 'wrap';
    catRow.style.gap = '6px';
    catRow.style.marginBottom = '4px';

    // All Category tag
    const allCatBadge = document.createElement('span');
    allCatBadge.className = activeMemoryCategory === 'All' ? 'tag-badge active' : 'tag-badge';
    allCatBadge.setAttribute('data-category', 'All');
    const totalCount = lessons.length;
    allCatBadge.textContent = `All (${totalCount})`;
    allCatBadge.style.cursor = 'pointer';
    allCatBadge.style.fontSize = '0.7rem';
    allCatBadge.style.fontWeight = '700';
    allCatBadge.style.padding = '2px 8px';
    allCatBadge.style.borderRadius = '12px';
    if (activeMemoryCategory === 'All') {
      allCatBadge.style.background = 'var(--neon-purple)';
      allCatBadge.style.color = 'var(--text-main)';
    } else {
      allCatBadge.style.background = 'rgba(255,255,255,0.05)';
      allCatBadge.style.color = 'var(--text-muted)';
    }
    allCatBadge.onclick = () => filterMemoryByCategory('All');
    catRow.appendChild(allCatBadge);

    // Add each dynamic category (only show categories with > 0 items)
    Array.from(categories).sort().forEach(cat => {
      const catCount = lessons.filter(l => (l.category || 'general').trim() === cat).length;
      if (catCount === 0) return;

      const tag = document.createElement('span');
      tag.className = activeMemoryCategory === cat ? 'tag-badge active' : 'tag-badge';
      tag.setAttribute('data-category', cat);
      tag.textContent = `${cat} (${catCount})`;
      tag.style.cursor = 'pointer';
      tag.style.fontSize = '0.7rem';
      tag.style.fontWeight = '700';
      tag.style.padding = '2px 8px';
      tag.style.borderRadius = '12px';
      if (activeMemoryCategory === cat) {
        tag.style.background = 'var(--neon-purple)';
        tag.style.color = 'var(--text-main)';
      } else {
        tag.style.background = 'rgba(255,255,255,0.05)';
        tag.style.color = 'var(--text-muted)';
      }
      tag.onclick = () => filterMemoryByCategory(cat);
      catRow.appendChild(tag);
    });
    tagsContainer.appendChild(catRow);

    // 2. Sub-Tags Section Label
    const tagLabel = document.createElement('div');
    tagLabel.style.fontSize = '0.65rem';
    tagLabel.style.color = 'var(--text-muted)';
    tagLabel.style.textTransform = 'uppercase';
    tagLabel.style.fontWeight = 'bold';
    tagLabel.style.letterSpacing = '0.05em';
    tagLabel.textContent = 'Sub-Tags';
    tagsContainer.appendChild(tagLabel);

    const tagRow = document.createElement('div');
    tagRow.style.display = 'flex';
    tagRow.style.flexWrap = 'wrap';
    tagRow.style.gap = '6px';

    // All Tag badge
    const allTagBadge = document.createElement('span');
    allTagBadge.className = activeMemoryTag === 'All' ? 'tag-badge active' : 'tag-badge';
    allTagBadge.setAttribute('data-tag', 'All');
    allTagBadge.textContent = `All (${totalCount})`;
    allTagBadge.style.cursor = 'pointer';
    allTagBadge.style.fontSize = '0.7rem';
    allTagBadge.style.fontWeight = '700';
    allTagBadge.style.padding = '2px 8px';
    allTagBadge.style.borderRadius = '12px';
    if (activeMemoryTag === 'All') {
      allTagBadge.style.background = 'var(--neon-blue)';
      allTagBadge.style.color = 'var(--text-main)';
    } else {
      allTagBadge.style.background = 'rgba(255,255,255,0.05)';
      allTagBadge.style.color = 'var(--text-muted)';
    }
    allTagBadge.onclick = () => filterMemoryByTag('All');
    tagRow.appendChild(allTagBadge);

    // Add each dynamic tag
    Array.from(uniqueTags).sort().forEach(t => {
      const tag = document.createElement('span');
      tag.className = activeMemoryTag === t ? 'tag-badge active' : 'tag-badge';
      tag.setAttribute('data-tag', t);
      const tagCount = lessons.filter(l => l.tags && l.tags.includes(t)).length;
      tag.textContent = `${t} (${tagCount})`;
      tag.style.cursor = 'pointer';
      tag.style.fontSize = '0.7rem';
      tag.style.fontWeight = '700';
      tag.style.padding = '2px 8px';
      tag.style.borderRadius = '12px';
      if (activeMemoryTag === t) {
        tag.style.background = 'var(--neon-blue)';
        tag.style.color = 'var(--text-main)';
      } else {
        tag.style.background = 'rgba(255,255,255,0.05)';
        tag.style.color = 'var(--text-muted)';
      }
      tag.onclick = () => filterMemoryByTag(t);
      tagRow.appendChild(tag);
    });
    tagsContainer.appendChild(tagRow);
  }

  renderLessonsList();
  renderMemoryClusterCanvas(cachedLessons, selectedLessonHash, selectMemoryNodeCallback);
}

export function onMemorySearchChange(val: string): void {
  activeSearchQuery = val;
  clearTimeout(searchDebounceTimer);
  searchDebounceTimer = setTimeout(() => {
    fetchSearchLessons(activeSearchQuery, activeMemoryCategory, activeMemoryTag);
  }, 250);
}

export function filterMemoryByCategory(category: string): void {
  activeMemoryCategory = category;
  
  const tags = document.querySelectorAll('#memory-tags-container .tag-badge[data-category]');
  tags.forEach(tag => {
    const cat = tag.getAttribute('data-category');
    if (cat === category) {
      tag.className = 'tag-badge active';
      (tag as HTMLElement).style.background = 'var(--neon-purple)';
      (tag as HTMLElement).style.color = 'var(--text-main)';
    } else {
      tag.className = 'tag-badge';
      (tag as HTMLElement).style.background = 'rgba(255,255,255,0.05)';
      (tag as HTMLElement).style.color = 'var(--text-muted)';
    }
  });

  fetchSearchLessons(activeSearchQuery, activeMemoryCategory, activeMemoryTag);
}

export function filterMemoryByTag(tag: string): void {
  activeMemoryTag = tag;
  
  const tags = document.querySelectorAll('#memory-tags-container .tag-badge[data-tag]');
  tags.forEach(tagElement => {
    const t = tagElement.getAttribute('data-tag');
    if (t === tag) {
      tagElement.className = 'tag-badge active';
      (tagElement as HTMLElement).style.background = 'var(--neon-blue)';
      (tagElement as HTMLElement).style.color = 'var(--text-main)';
    } else {
      tagElement.className = 'tag-badge';
      (tagElement as HTMLElement).style.background = 'rgba(255,255,255,0.05)';
      (tagElement as HTMLElement).style.color = 'var(--text-muted)';
    }
  });

  fetchSearchLessons(activeSearchQuery, activeMemoryCategory, activeMemoryTag);
}

async function fetchSearchLessons(q: string, cat: string, tag: string = 'All'): Promise<void> {
  try {
    const res = await fetch(`/api/search?q=${encodeURIComponent(q)}&category=${encodeURIComponent(cat)}&tag=${encodeURIComponent(tag)}`);
    if (res.ok) {
      const lessons = await res.json() as any[];
      cachedLessons = lessons;
      renderLessonsList();
      renderMemoryClusterCanvas(cachedLessons, selectedLessonHash, selectMemoryNodeCallback);
    }
  } catch (e) {}
}

export function renderLessonsList(): void {
  const container = document.getElementById('memory-lessons-list');
  if (!container) return;
  container.replaceChildren();

  if (!cachedLessons || cachedLessons.length === 0) {
    const empty = document.createElement('div');
    empty.style.color = 'var(--text-muted)';
    empty.style.fontSize = '0.8rem';
    empty.style.textAlign = 'center';
    empty.style.padding = '2rem';
    empty.textContent = 'No matching memory records found.';
    container.appendChild(empty);
    return;
  }

  cachedLessons.forEach(lesson => {
    const card = document.createElement('div');
    card.className = `timeline-card ${selectedLessonHash === lesson.commitHash ? 'active-card' : ''}`;
    card.style.border = selectedLessonHash === lesson.commitHash ? '1px solid var(--neon-purple)' : '1px solid rgba(255, 255, 255, 0.05)';
    card.style.background = selectedLessonHash === lesson.commitHash ? 'rgba(139, 92, 246, 0.05)' : 'rgba(255, 255, 255, 0.01)';
    card.style.borderRadius = '6px';
    card.style.padding = '10px';
    card.style.cursor = 'pointer';
    card.style.transition = 'all 0.2s';
    
    card.onclick = () => selectMemoryNodeCallback(lesson.commitHash);

    const header = document.createElement('div');
    header.style.display = 'flex';
    header.style.justifyContent = 'space-between';
    header.style.alignItems = 'center';
    header.style.marginBottom = '6px';

    const commitSpan = document.createElement('span');
    commitSpan.className = 'timeline-commit';
    commitSpan.textContent = `Commit: ${lesson.commitHash.substring(0, 7)}`;

    const sizeSpan = document.createElement('span');
    sizeSpan.style.fontSize = '0.65rem';
    sizeSpan.style.color = 'var(--text-muted)';
    sizeSpan.style.marginLeft = '6px';
    const charLen = lesson.insight ? lesson.insight.length : 0;
    sizeSpan.textContent = `(${charLen} chars)`;
    commitSpan.appendChild(sizeSpan);

    const badgesGroup = document.createElement('div');
    badgesGroup.style.display = 'flex';
    badgesGroup.style.gap = '4px';
    badgesGroup.style.alignItems = 'center';

    const catSpan = document.createElement('span');
    catSpan.className = `timeline-category ${lesson.category || 'general'}`;
    catSpan.textContent = lesson.category || 'general';
    catSpan.style.fontSize = '0.65rem';
    catSpan.style.padding = '2px 6px';
    catSpan.style.borderRadius = '4px';
    catSpan.style.border = '1px solid rgba(255,255,255,0.1)';
    
    let color = 'var(--text-muted)';
    if (lesson.category && lesson.category !== 'general') {
      const uniqueCats = Array.from(new Set(cachedLessons.map(l => l.category).filter(Boolean))).sort();
      const idx = uniqueCats.indexOf(lesson.category);
      if (idx !== -1) {
        color = presetColors[idx % presetColors.length];
      }
    } else if (lesson.category === 'general') {
      color = 'var(--neon-purple)';
    }
    catSpan.style.borderColor = color;
    catSpan.style.color = color;
    badgesGroup.appendChild(catSpan);

    if (Array.isArray(lesson.tags)) {
      lesson.tags.forEach((tg: string) => {
        const tgSpan = document.createElement('span');
        tgSpan.style.fontSize = '0.55rem';
        tgSpan.style.padding = '1px 5px';
        tgSpan.style.borderRadius = '3px';
        tgSpan.style.background = 'rgba(255, 255, 255, 0.08)';
        tgSpan.style.color = 'var(--neon-blue)';
        tgSpan.style.border = '1px solid rgba(0, 191, 255, 0.2)';
        tgSpan.style.fontWeight = 'bold';
        tgSpan.textContent = tg;
        badgesGroup.appendChild(tgSpan);
      });
    }

    header.appendChild(commitSpan);
    header.appendChild(badgesGroup);

    const insight = document.createElement('div');
    insight.className = 'timeline-insight';
    const cleanInsight = lesson.insight || '';
    if (cleanInsight.length > 120) {
      insight.textContent = cleanInsight.substring(0, 117) + '...';
    } else {
      insight.textContent = cleanInsight;
    }
    insight.style.fontSize = '0.75rem';
    insight.style.color = 'var(--text-normal)';

    if (lesson.score !== undefined && lesson.score < 1.0 && activeSearchQuery.trim()) {
      const matchScore = document.createElement('div');
      matchScore.style.fontSize = '0.65rem';
      matchScore.style.color = 'var(--neon-indigo)';
      matchScore.style.marginTop = '6px';
      matchScore.style.textAlign = 'right';
      matchScore.textContent = `Semantic Match: ${(lesson.score * 100).toFixed(1)}%`;
      insight.appendChild(matchScore);
    }

    card.appendChild(header);
    card.appendChild(insight);
    container.appendChild(card);
  });
}

// Lazy register callback to break circular dependency
let selectMemoryNodeCallback: (hash: string) => void = () => {};
export function registerSelectNodeCallback(fn: (hash: string) => void) {
  selectMemoryNodeCallback = fn;
}
