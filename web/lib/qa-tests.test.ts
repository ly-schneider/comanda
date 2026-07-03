import { describe, expect, it } from "vitest"
import { QA_SECTIONS, QA_TEST_COUNT, qaTestKeys } from "./qa-tests"

const KEY_PATTERN = /^[a-z0-9]+(?:[.-][a-z0-9]+)*$/

describe("qa-tests definitions", () => {
  it("has globally unique test keys", () => {
    const keys = qaTestKeys()
    expect(new Set(keys).size).toBe(keys.length)
  })

  it("uses backend-compatible keys namespaced by section", () => {
    for (const section of QA_SECTIONS) {
      expect(section.key).toMatch(KEY_PATTERN)
      for (const test of section.tests) {
        expect(test.key).toMatch(KEY_PATTERN)
        expect(test.key.length).toBeLessThanOrEqual(120)
        expect(test.key.startsWith(`${section.key}.`)).toBe(true)
      }
    }
  })

  it("has non-empty content on every test", () => {
    for (const section of QA_SECTIONS) {
      expect(section.title).toBeTruthy()
      expect(section.haveReady.length).toBeGreaterThan(0)
      for (const test of section.tests) {
        expect(test.title).toBeTruthy()
        expect(test.action).toBeTruthy()
        expect(test.expected).toBeTruthy()
      }
    }
  })

  it("covers the full checklist", () => {
    expect(QA_TEST_COUNT).toBe(130)
  })
})
