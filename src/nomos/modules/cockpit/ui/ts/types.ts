// types.ts - Unified interface contracts and schemas for the Cockpit UI
export * from './generated';



export enum WorkspacePhase {
  PLAN = 'PLAN',
  EDIT = 'EDIT',
  REVIEW = 'REVIEW',
  IDLE = 'IDLE'
}

export enum TaskStatus {
  BACKLOG = 'BACKLOG',
  IN_PROGRESS = 'IN_PROGRESS',
  DONE = 'DONE',
  TRIAGE = 'TRIAGE',
  CANCELLED = 'CANCELLED'
}

export interface TelemetryStatus {
  cpu: number;
  mem: number;
  gpus?: any[];
  slots?: any;
  phaseState: import('./generated').PhaseState;
  inferenceStats?: any[];
  swarmConfig?: any;
}

export interface TaskCard {
  key: string;
  title: string;
  body: string;
  status: string;
  labels?: string[];
  context_burden?: number;
  logic_depth?: number;
  assignee?: string;
  parent_key?: string;
  type?: string;
  project?: string;
}

export interface ASTNode {
  id: string;
  label: string;
  group?: string;
  type?: string;
}

export interface ASTLink {
  source: string;
  target: string;
}

export interface ASTData {
  nodes: ASTNode[];
  links: ASTLink[];
}

export interface RetrospectiveLesson {
  commitHash: string;
  insight: string;
  category: string;
  score?: number;
}

/**
 * Represents a single message in the Swarm Chat drawer.
 */
export interface ChatMessage {
  sender: 'user' | 'agent' | 'system';
  text: string;
}

/**
 * Represents the active git worktree/task context selected in the HUD.
 */
export interface SovereignTaskContext {
  path: string;
  name: string;
}

/**
 * Represents a selected file in the Git diff view.
 */
export interface SelectedDiffFile {
  file: string;
  staged: boolean;
}

/**
 * Represents the repository discovery list returned by the server.
 */
export interface WorkspaceDiscoveryResponse {
  discovered: string[];
  current: string;
}

/**
 * Represents a somatic memory node candidate in the Pruning Advisor.
 */
export interface SomaticNode {
  commitHash: string;
  insight?: string;
  category?: string;
  tags?: string[];
  timestamp: number | string;
}



