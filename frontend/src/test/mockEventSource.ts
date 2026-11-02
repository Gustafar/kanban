export class MockEventSource {
  static instances: MockEventSource[] = []
  url: string
  onopen: (() => void) | null = null
  onerror: (() => void) | null = null
  onmessage: ((e: { data: string }) => void) | null = null
  closed = false

  constructor(url: string) {
    this.url = url
    MockEventSource.instances.push(this)
  }

  emit(type: string, data: unknown) {
    this.onmessage?.({ data: JSON.stringify({ type, data }) })
  }

  close() {
    this.closed = true
  }
}

export function installMockEventSource() {
  MockEventSource.instances = []
  ;(globalThis as any).EventSource = MockEventSource
  return MockEventSource
}
