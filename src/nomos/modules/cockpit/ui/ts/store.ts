export interface CockpitState {
  activeProjectFilter: string;
  lastBacklog: any[];
  lastFleet: any;
  status: any;
  ideActiveTaskId: string | null;
  idePhase: string | null;
  activeDrift: any;
  ast: any;
  lastLessons: any[];
  lastGitbrain: any;
  activeSwarms: any[];
  mergedBranchesList: any[];
}

export type Listener = (state: CockpitState) => void;

class Store {
  private state: CockpitState = {
    activeProjectFilter: 'ALL',
    lastBacklog: [],
    lastFleet: null,
    status: null,
    ideActiveTaskId: null,
    idePhase: null,
    activeDrift: null,
    ast: null,
    lastLessons: [],
    lastGitbrain: null,
    activeSwarms: [],
    mergedBranchesList: []
  };
  private listeners: Set<Listener> = new Set();

  getState(): CockpitState {
    return this.state;
  }

  setState(newState: Partial<CockpitState>): void {
    this.state = { ...this.state, ...newState };
    this.notify();
  }

  subscribe(listener: Listener): () => void {
    this.listeners.add(listener);
    listener(this.state);
    return () => this.listeners.delete(listener);
  }

  private notify(): void {
    for (const listener of this.listeners) {
      listener(this.state);
    }
  }
}

export const CockpitStore = new Store();
