import { validateSubmitKnowledge } from "../../../lambda/shared/validation";

describe("validateSubmitKnowledge", () => {
  const valid = { title: "Test", content: "Body content", author: "alice" };

  test("valid input returns success", () => {
    const result = validateSubmitKnowledge(valid);
    expect(result.valid).toBe(true);
  });

  test("missing title returns error", () => {
    const r = validateSubmitKnowledge({ ...valid, title: "" });
    expect(r.valid).toBe(false);
    if (!r.valid) expect(r.errors.title).toBeDefined();
  });

  test("whitespace-only title returns error", () => {
    const r = validateSubmitKnowledge({ ...valid, title: "   " });
    expect(r.valid).toBe(false);
    if (!r.valid) expect(r.errors.title).toBeDefined();
  });

  test("title >255 chars returns error", () => {
    const r = validateSubmitKnowledge({ ...valid, title: "a".repeat(256) });
    expect(r.valid).toBe(false);
    if (!r.valid) expect(r.errors.title).toBeDefined();
  });

  test("missing content returns error", () => {
    const r = validateSubmitKnowledge({ ...valid, content: "" });
    expect(r.valid).toBe(false);
    if (!r.valid) expect(r.errors.content).toBeDefined();
  });

  test("content >100K chars returns error", () => {
    const r = validateSubmitKnowledge({ ...valid, content: "a".repeat(100_001) });
    expect(r.valid).toBe(false);
    if (!r.valid) expect(r.errors.content).toBeDefined();
  });

  test("missing author returns error", () => {
    const r = validateSubmitKnowledge({ ...valid, author: "" });
    expect(r.valid).toBe(false);
    if (!r.valid) expect(r.errors.author).toBeDefined();
  });

  test("author >100 chars returns error", () => {
    const r = validateSubmitKnowledge({ ...valid, author: "a".repeat(101) });
    expect(r.valid).toBe(false);
    if (!r.valid) expect(r.errors.author).toBeDefined();
  });

  test("multiple errors reported together", () => {
    const r = validateSubmitKnowledge({ title: "", content: "", author: "" });
    expect(r.valid).toBe(false);
    if (!r.valid) {
      expect(Object.keys(r.errors).length).toBe(3);
    }
  });
});
