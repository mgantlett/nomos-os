import './setup-test-env.js';
import { describe, it } from 'node:test';
import * as assert from 'node:assert';
import { renderPruningAdvisorDashboard } from './memory/pruningAdvisor.js';

describe('Memory Pruning Advisor UI', () => {
  it('should render pruning advisor dashboard without throwing', () => {
    const container = {
      querySelector: () => null,
      firstElementChild: null,
      replaceChildren: () => {},
      appendChild: () => {},
      style: {}
    } as any;

    assert.doesNotThrow(() => {
      renderPruningAdvisorDashboard(container);
    });
  });
});
