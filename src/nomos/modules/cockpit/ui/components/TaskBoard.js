import { CockpitStore } from '../store.js';

export class TaskBoardComponent {
    constructor(containerId) {
        this.container = document.getElementById(containerId);
        if (!this.container) return;
        
        // Bind methods
        this.updateBoard = this.updateBoard.bind(this);
        
        // Subscribe to store
        this.unsubscribe = CockpitStore.subscribe(this.updateBoard);
    }

    destroy() {
        if (this.unsubscribe) {
            this.unsubscribe();
        }
        if (this.container) {
            this.container.innerHTML = '';
        }
    }

    updateBoard(state) {
        if (!this.container) return;
        const { status, activeProjectFilter, lastBacklog, ideActiveTaskId, idePhase } = state;
        
        const isCommunity = status && status.edition === 'community';
        const filteredBacklog = (activeProjectFilter === 'ALL' || isCommunity)
            ? lastBacklog || []
            : (lastBacklog || []).filter((t) => t.project?.toLowerCase() === activeProjectFilter.toLowerCase());

        this.render(filteredBacklog, ideActiveTaskId, idePhase);
    }

    render(tasks, activeTaskId, activePhase) {
        if (!tasks || tasks.length === 0) {
            this.container.innerHTML = '<div style="padding: 20px; color: var(--text-muted); text-align: center;">No active tasks found in backlog.</div>';
            return;
        }

        let html = '<div style="display: flex; gap: 10px; overflow-x: auto; padding: 10px;">';
        
        // Simple Community Edition Board Columns
        const columns = [
            { id: 'todo', title: 'To Do', filter: t => !t.status || t.status === 'open' },
            { id: 'in_progress', title: 'In Progress', filter: t => t.status === 'in_progress' || t.id === activeTaskId },
            { id: 'done', title: 'Done', filter: t => t.status === 'closed' || t.status === 'done' }
        ];

        for (const col of columns) {
            const colTasks = tasks.filter(col.filter);
            html += `<div style="flex: 1; min-width: 300px; background: rgba(255,255,255,0.02); border: 1px solid #30363d; border-radius: 6px; padding: 10px;">`;
            html += `<h3 style="margin-top: 0; color: #fff; font-size: 0.9rem; border-bottom: 1px solid #30363d; padding-bottom: 8px;">${col.title} <span style="color: var(--text-muted); font-size: 0.8em;">(${colTasks.length})</span></h3>`;
            html += `<div style="display: flex; flex-direction: column; gap: 8px;">`;
            
            for (const task of colTasks) {
                const isActive = task.id === activeTaskId;
                const border = isActive ? '1px solid #00f0ff' : '1px solid #444c56';
                html += `
                    <div style="background: #161b22; border: ${border}; padding: 10px; border-radius: 4px;">
                        <div style="font-size: 0.75rem; color: var(--text-muted); margin-bottom: 4px;">${task.id}</div>
                        <div style="color: #c9d1d9; font-size: 0.85rem;">${task.title || 'Untitled Task'}</div>
                        ${isActive ? `<div style="margin-top: 6px; display: inline-block; background: rgba(0,240,255,0.1); color: #00f0ff; padding: 2px 6px; border-radius: 3px; font-size: 0.65rem;">Active: ${activePhase || 'UNKNOWN'}</div>` : ''}
                    </div>
                `;
            }
            html += `</div></div>`;
        }

        html += '</div>';
        this.container.innerHTML = html;
    }
}
