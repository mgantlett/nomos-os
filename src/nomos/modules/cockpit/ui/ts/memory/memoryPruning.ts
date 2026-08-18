// memoryPruning.ts - Facade delegating to decomposed pruning sub-widgets

import { renderPruningAdvisorDashboard } from './pruningAdvisor.js';
import { selectMemoryNode } from './pruningInspector.js';
import { saveMemoryEdit, pruneMemoryNode } from './pruningActions.js';

export {
  renderPruningAdvisorDashboard,
  selectMemoryNode,
  saveMemoryEdit,
  pruneMemoryNode
};
