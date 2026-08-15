import { CockpitStore } from '../store.js';
import { renderKanbanBoard as renderKanban } from '../agent_swimlanes.js';
export function initKanbanBoard() {
    CockpitStore.subscribe(updateKanbanBoard);
}
function updateKanbanBoard(state) {
    const { status, activeProjectFilter, lastBacklog, ideActiveTaskId, idePhase } = state;
    const isCommunity = status && status.edition === 'community';
    const filteredBacklog = (activeProjectFilter === 'ALL' || isCommunity)
        ? lastBacklog
        : lastBacklog.filter((t) => t.project?.toLowerCase() === activeProjectFilter.toLowerCase());
    // Render the Kanban sprint board with task cards filtered by active task
    renderKanban(filteredBacklog, ideActiveTaskId, idePhase);
}
