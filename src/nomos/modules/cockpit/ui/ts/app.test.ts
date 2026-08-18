// app.test.ts - Test suite for Cockpit UI Modular Components
import './setup-test-env.js';
import { describe, it } from 'node:test';
import * as assert from 'node:assert';
import { parseAnsiLine } from './ansi.js';
import { getCurrentActiveArtifactType } from './artifacts.js';
import { getBasename } from './git.js';

describe('Cockpit UI Modular Components', () => {
  it('should get the basename of a file path', () => {
    assert.strictEqual(getBasename('src/control-plane-ui/ts/app.ts'), 'app.ts');
    assert.strictEqual(getBasename('app.ts'), 'app.ts');
  });

  it('should parse ANSI sequences correctly', () => {
    const parts = parseAnsiLine('\u001b[31mRed Text\u001b[0m');
    assert.ok(parts.length > 0);
  });

  it('should retrieve active artifact type', () => {
    assert.strictEqual(getCurrentActiveArtifactType(), 'implementation_plan');
  });

  it('should resolve column mapping correctly based on phase and worker status', () => {
    const resolveColumn = (taskNum: number, activeTaskId: string, currentPhase: string, swarms: any[]) => {
      const isTaskRunning = swarms.some((sw: any) => {
        if (!sw.id) return false;
        const parts = sw.id.split('-');
        return String(parts[0]) === String(taskNum) && sw.status === 'running';
      });

      if (currentPhase === 'SHIP' && String(taskNum) === String(activeTaskId)) {
        return 'SHIP';
      } else if (isTaskRunning) {
        return 'EDIT';
      } else if (String(taskNum) === String(activeTaskId)) {
        if (currentPhase === 'PLAN') return 'PLAN';
        if (currentPhase === 'EDIT' || currentPhase === 'VALIDATE') return 'EDIT';
        if (currentPhase === 'REVIEW') return 'REVIEW';
      }
      return 'BACKLOG';
    };

    // 1. Worker is running -> should be mapped to EDIT regardless of phase
    const activeSwarms = [{ id: '314-aider', status: 'running' }];
    assert.strictEqual(resolveColumn(314, '314', 'PLAN', activeSwarms), 'EDIT');

    // 2. Current phase is SHIP and task is the active task -> should be SHIP
    assert.strictEqual(resolveColumn(314, '314', 'SHIP', []), 'SHIP');

    // 3. Active task is in PLAN phase -> should be PLAN
    assert.strictEqual(resolveColumn(314, '314', 'PLAN', []), 'PLAN');

    // 4. Inactive task -> should default to BACKLOG
    assert.strictEqual(resolveColumn(315, '314', 'PLAN', []), 'BACKLOG');
  });
});
