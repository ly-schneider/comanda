import "@testing-library/jest-dom"

// jsdom lacks the browser APIs Radix overlays (Popover, DropdownMenu, …) touch.
class ResizeObserverStub {
  observe() {}
  unobserve() {}
  disconnect() {}
}
globalThis.ResizeObserver ??= ResizeObserverStub as unknown as typeof ResizeObserver

if (typeof Element !== "undefined") {
  Element.prototype.hasPointerCapture ??= () => false
  Element.prototype.setPointerCapture ??= () => {}
  Element.prototype.releasePointerCapture ??= () => {}
  Element.prototype.scrollIntoView ??= () => {}
}

// jsdom here ships no Web Storage, and Node's global `localStorage` throws unless
// launched with --localstorage-file — so install a plain in-memory Storage.
class MemoryStorage implements Storage {
  private store = new Map<string, string>()
  get length() {
    return this.store.size
  }
  clear() {
    this.store.clear()
  }
  getItem(key: string) {
    return this.store.has(key) ? (this.store.get(key) as string) : null
  }
  key(index: number) {
    return Array.from(this.store.keys())[index] ?? null
  }
  removeItem(key: string) {
    this.store.delete(key)
  }
  setItem(key: string, value: string) {
    this.store.set(key, String(value))
  }
}
for (const name of ["localStorage", "sessionStorage"] as const) {
  const storage = new MemoryStorage()
  Object.defineProperty(globalThis, name, { value: storage, configurable: true, writable: true })
  if (typeof window !== "undefined") {
    Object.defineProperty(window, name, { value: storage, configurable: true, writable: true })
  }
}
