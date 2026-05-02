import { randomBytes } from "crypto";

const CROCKFORD_ALPHABET = "0123456789ABCDEFGHJKMNPQRSTVWXYZ";

/**
 * Encode a Buffer to Crockford base32 using a bit-buffer algorithm.
 * Reads 8 bits per byte, extracts 5-bit groups MSB-first via unsigned right shift.
 */
export function encodeCrockford(buf: Buffer): string {
  let bits = 0;
  let value = 0;
  let output = "";

  for (const byte of buf) {
    value = (value << 8) | byte;
    bits += 8;
    while (bits >= 5) {
      bits -= 5;
      output += CROCKFORD_ALPHABET[(value >>> bits) & 0x1f];
    }
  }

  if (bits > 0) {
    output += CROCKFORD_ALPHABET[(value << (5 - bits)) & 0x1f];
  }

  return output;
}

/**
 * Generate a 16-character Crockford base32 UID from 10 random bytes (80 bits entropy).
 */
export function generateUid(): string {
  return encodeCrockford(randomBytes(10));
}
