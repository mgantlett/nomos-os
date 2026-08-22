<script lang="ts">
  import { onMount, onDestroy } from 'svelte';

  let status: any = null;
  let backlog: any[] = [];
  let error: string | null = null;
  let timer: any;

  async function fetchTelemetry() {
    try {
      const [statusRes, backlogRes] = await Promise.all([
        fetch('/api/status'),
        fetch('/api/backlog')
      ]);

      if (!statusRes.ok) throw new Error(`Status HTTP error: ${statusRes.status}`);
      if (!backlogRes.ok) throw new Error(`Backlog HTTP error: ${backlogRes.status}`);

      status = await statusRes.json();
      backlog = await backlogRes.json();
      error = null;
    } catch (e: any) {
      error = e.message;
    }
  }

  onMount(() => {
    fetchTelemetry();
    timer = setInterval(fetchTelemetry, 2000);
  });

  onDestroy(() => {
    clearInterval(timer);
  });
</script>

<main class="dashboard">
  <header class="header">
    <div class="brand">
      <div class="logo"></div>
      <h1>Nomos Cockpit</h1>
    </div>
    <div class="connection">
      {#if error}
        <span class="badge danger">Disconnected</span>
      {:else if status}
        <span class="badge success">Connected</span>
      {:else}
        <span class="badge warning">Connecting...</span>
      {/if}
    </div>
  </header>

  {#if error}
    <div class="alert danger">
      <strong>Connection Error:</strong> {error}
    </div>
  {/if}

  <div class="grid">
    <section class="card">
      <h2>System Status</h2>
      {#if status}
        <div class="stats">
          <div class="stat">
            <span class="label">Edition</span>
            <span class="value">{status.edition}</span>
          </div>
          <div class="stat">
            <span class="label">Version</span>
            <span class="value">{status.version}</span>
          </div>
          <div class="stat">
            <span class="label">Workspace</span>
            <span class="value">{status.workspaceName}</span>
          </div>
          <div class="stat">
            <span class="label">Project</span>
            <span class="value">{status.project}</span>
          </div>
        </div>
      {:else}
        <p class="loading">Loading status...</p>
      {/if}
    </section>

    <section class="card backlog-card">
      <h2>Active Backlog</h2>
      {#if backlog.length > 0}
        <ul class="task-list">
          {#each backlog as task}
            <li class="task-item">
              <div class="task-header">
                <span class="task-key">{task.key}</span>
                <span class="task-type {task.type?.toLowerCase()}">{task.type || 'Task'}</span>
                <span class="task-status {task.status?.toLowerCase().replace(' ', '-')}">{task.status}</span>
              </div>
              <h3 class="task-title">{task.title}</h3>
              <div class="task-meta">
                <span class="priority {task.priority}">{task.priority}</span>
              </div>
            </li>
          {/each}
        </ul>
      {:else if status}
        <p class="empty">No active tasks in backlog.</p>
      {:else}
        <p class="loading">Loading backlog...</p>
      {/if}
    </section>
  </div>
</main>
