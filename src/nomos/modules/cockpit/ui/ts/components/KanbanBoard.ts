import { CockpitStore, CockpitState } from '../store.js';
import { renderKanbanBoard as renderKanban } from '../board.js';

export function initKanbanBoard(): void {
  CockpitStore.subscribe(updateKanbanBoard);
}

function updateKanbanBoard(state: CockpitState): void {
  const { status, activeProjectFilter, lastBacklog, ideActiveTaskId, idePhase } = state;
  const isCommunity = status && status.edition === 'community';
  
  const filteredBacklog = (activeProjectFilter === 'ALL' || isCommunity) 
    ? lastBacklog 
    : lastBacklog.filter((t: any) => t.project?.toLowerCase() === activeProjectFilter.toLowerCase());

  // Render the Kanban sprint board with task cards filtered by active task
  renderKanban(filteredBacklog, ideActiveTaskId, idePhase);
}
