/**
 * Shared input validation helpers.
 * Implementation in API-001.
 */
export function validateRequired(
  value: unknown,
  fieldName: string,
): asserts value is string {
  if (typeof value !== "string" || value.trim().length === 0) {
    throw new Error(`${fieldName} is required`);
  }
}
