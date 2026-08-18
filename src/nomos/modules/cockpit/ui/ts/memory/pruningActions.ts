import { showToast } from '../toast.js';
import { renderMemoryClusterCanvas } from '../nebula.js';
import {
  cachedLessons,
  activeTags,
  selectedLessonHash,
  setCachedLessons,
  setSelectedLessonHash,
  renderLessonsList
} from './memoryCore.js';
import { selectMemoryNode, renderPruningAdvisorDashboard } from './memoryPruning.js';

export function saveMemoryEdit(hash: string): void {
  const editCat = document.getElementById('edit-memory-category') as HTMLSelectElement | null;
  const editInsight = document.getElementById('edit-memory-insight') as HTMLTextAreaElement | null;
  if (!editCat || !editInsight) return;

  const category = editCat.value;
  const insight = editInsight.value;

  fetch('/api/memory/update', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ commitHash: hash, category, insight, tags: activeTags })
  })
  .then(res => {
    if (!res.ok) throw new Error('Update failed');
    return res.json();
  })
  .then(() => {
    showToast('Memory insight updated successfully!', 'success');
    const idx = cachedLessons.findIndex(l => l.commitHash === hash);
    if (idx !== -1) {
      cachedLessons[idx].category = category;
      cachedLessons[idx].insight = insight;
      cachedLessons[idx].tags = [...activeTags];
    }
    selectMemoryNode(hash);
  })
  .catch(err => {
    showToast(`Error updating memory: ${err.message}`, 'error');
  });
}

export function pruneMemoryNode(hash: string): void {
  const consent = confirm(`Are you sure you want to permanently prune memory insight ${hash.substring(0, 7)} from your SQLite database and git history? This cannot be undone.`);
  if (!consent) return;

  fetch('/api/memory/prune', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ commitHash: hash })
  })
  .then(res => {
    if (!res.ok) throw new Error('Prune failed');
    return res.json();
  })
  .then(() => {
    showToast('Memory node pruned successfully!', 'success');
    setCachedLessons(cachedLessons.filter(l => l.commitHash !== hash));
    setSelectedLessonHash(null);
    renderLessonsList();
    renderMemoryClusterCanvas(cachedLessons, selectedLessonHash, selectMemoryNode);
    const inspector = document.getElementById('memory-node-inspector');
    if (inspector) {
      renderPruningAdvisorDashboard(inspector);
    }
  })
  .catch(err => {
    showToast(`Error pruning memory node: ${err.message}`, 'error');
  });
}
