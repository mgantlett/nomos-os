// setup-test-env.ts - Mocks browser global objects for Node environment
(global as any).window = {
  addEventListener: () => {}
};
(global as any).document = {
  getElementById: () => null,
  querySelectorAll: () => [],
  createElement: () => ({
    appendChild: () => {},
    replaceChildren: () => {},
    style: {},
    classList: { add: () => {} }
  })
};
