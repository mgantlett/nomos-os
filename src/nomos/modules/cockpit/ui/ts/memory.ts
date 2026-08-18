// memory.ts - Refactored entrypoint delegating to modular UI widgets

import {
  updateTimelineUI,
  onMemorySearchChange,
  filterMemoryByCategory,
  filterMemoryByTag,
  renderLessonsList,
  registerSelectNodeCallback,
  activeMemoryCategory,
  activeMemoryTag,
  activeSearchQuery,
  selectedLessonHash
} from './memory/memoryCore.js';
import {
  renderPruningAdvisorDashboard,
  selectMemoryNode,
  saveMemoryEdit,
  pruneMemoryNode
} from './memory/memoryPruning.js';

// Wire up callback route to break circular dependencies
registerSelectNodeCallback(selectMemoryNode);

// Re-export expected public members to maintain 100% backward compatibility
export {
  updateTimelineUI,
  onMemorySearchChange,
  filterMemoryByCategory,
  filterMemoryByTag,
  renderLessonsList,
  renderPruningAdvisorDashboard,
  selectMemoryNode,
  saveMemoryEdit,
  pruneMemoryNode,
  activeMemoryCategory,
  activeMemoryTag,
  activeSearchQuery,
  selectedLessonHash
};
