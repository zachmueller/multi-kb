/**
 * Shared API response helpers.
 * Implementation in API-001.
 */
export function ok(body: unknown): { statusCode: number; body: string } {
  return { statusCode: 200, body: JSON.stringify(body) };
}

export function error(
  statusCode: number,
  message: string,
): { statusCode: number; body: string } {
  return { statusCode, body: JSON.stringify({ error: message }) };
}
