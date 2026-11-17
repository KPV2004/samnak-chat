import "@testing-library/jest-dom/vitest";

// jsdom lacks a few browser APIs that Radix (tooltips, menus, popovers, select, command) calls.
class ResizeObserverStub {
  observe() {}
  unobserve() {}
  disconnect() {}
}
globalThis.ResizeObserver ??= ResizeObserverStub;
Element.prototype.scrollIntoView ??= () => {};
Element.prototype.hasPointerCapture ??= () => false;
Element.prototype.releasePointerCapture ??= () => {};
