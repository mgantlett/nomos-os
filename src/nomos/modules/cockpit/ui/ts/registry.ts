// registry.ts - Open-Core Extension Registry

export type HookCallback = (...args: any[]) => any;

export class ExtensionRegistry {
  private hooks = new Map<string, HookCallback[]>();

  registerHook(name: string, fn: HookCallback) {
    if (!this.hooks.has(name)) {
      this.hooks.set(name, []);
    }
    this.hooks.get(name)!.push(fn);
  }

  executeHook(name: string, ...args: any[]): any[] {
    if (!this.hooks.has(name)) return [];
    return this.hooks.get(name)!.map(fn => fn(...args));
  }
}

// Attach to window so Sovereign plugins can access it
(window as any).ExtensionRegistry = new ExtensionRegistry();
