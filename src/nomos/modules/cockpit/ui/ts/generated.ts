/* Do not change, this code is generated from Golang structs */


export interface NixShellProcess {
    pid: number;
    ppid: number;
    command: string;
    args: string[];
}
export interface WorktreeInfo {
    path: string;
    name: string;
    branch: string;
    phase: string;
}
export interface GPUStats {
    gpuUtil: string;
    vramUsed: string;
    vramTotal: string;
    powerDraw?: string;
}
export interface InferenceStat {
    name: string;
    port: number;
    status: string;
    model?: string;
    tps?: number;
    promptTps?: number;
    queueLength?: number;
}
export interface SlotState {
    slot: number;
    status: string;
    taskID?: string;
    branch?: string;
    folderName?: string;
}
export interface SlotsInfo {
    type: string;
    used: number;
    total: number;
    slotStates: SlotState[];
}
export interface PhaseState {
    agent: string;
    agent_tier: string;
    agent_type: string;
    commit_approved: string;
    current_phase: string;
    dod_failure_count: number;
    phase_entered_at: string;
    plan_approved: string;
    prev_phase: string;
    session_commits: number;
    session_started_at: string;
    task_id: string;
    waiting_on_human: string;
    compact_context: boolean;
    phase_token: string;
    active_sprint: number;
}
export interface StatusPayload {
    repoRoot: string;
    phaseState: PhaseState;
    idePhaseState: PhaseState;
    slots: SlotsInfo;
    inferenceStats: InferenceStat[];
    gpu: GPUStats;
    worktrees: WorktreeInfo[];
    nixShells: NixShellProcess[];
    gcpSpotStatus: string;
    gcpSpotSeconds: number;
    gcpSpotCost: number;
    version: string;
    workspaceName: string;
}


export interface TaskTransition {
    timestamp: Time;
    old_status: string;
    new_status: string;
    author: string;
}
export interface TaskComment {
    author: string;
    created_at: Time;
    body: string;
}
export interface Time {

}
export interface Task {
    key: string;
    project?: string;
    parent_key?: string;
    type?: string;
    title: string;
    status: string;
    assignee?: string;
    sequence: number;
    context_burden: number;
    logic_depth: number;
    labels: string[];
    blocked_by: string[];
    created_at: Time;
    updated_at: Time;
    closed_at?: Time;
    agent_cycles?: number;
    rework_frequency?: number;
    description: string;
    comments?: TaskComment[];
    activity_log: TaskTransition[];
    is_spike: boolean;
    cycle: number;
}
