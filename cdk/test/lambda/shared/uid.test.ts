import { encodeCrockford, generateUid } from "../../../lambda/shared/uid";

const CROCKFORD_ALPHABET = new Set("0123456789ABCDEFGHJKMNPQRSTVWXYZ");

describe("encodeCrockford", () => {
  test("all zeros → 16 zeros", () => {
    expect(encodeCrockford(Buffer.alloc(10, 0))).toBe("0000000000000000");
  });

  test("all 0xFF → 16 Z's", () => {
    expect(encodeCrockford(Buffer.alloc(10, 0xff))).toBe("ZZZZZZZZZZZZZZZZ");
  });

  test("0x00..0x09 sequence", () => {
    expect(
      encodeCrockford(
        Buffer.from([0x00, 0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08, 0x09]),
      ),
    ).toBe("000G40R40M30E209");
  });

  test("DEADBEEF... sequence", () => {
    expect(
      encodeCrockford(
        Buffer.from([0xde, 0xad, 0xbe, 0xef, 0xca, 0xfe, 0xba, 0xbe, 0x00, 0x42]),
      ),
    ).toBe("VTPVXVYAZTXBW022");
  });

  test("HelloWorld bytes", () => {
    expect(encodeCrockford(Buffer.from("HelloWorld"))).toBe("91JPRV3FAXQQ4V34");
  });
});

describe("generateUid", () => {
  test("produces exactly 16 characters", () => {
    expect(generateUid()).toHaveLength(16);
  });

  test("uses only valid Crockford alphabet", () => {
    for (let i = 0; i < 100; i++) {
      const uid = generateUid();
      for (const ch of uid) {
        expect(CROCKFORD_ALPHABET.has(ch)).toBe(true);
      }
    }
  });

  test("no forbidden characters (I, L, O, U)", () => {
    for (let i = 0; i < 1000; i++) {
      const uid = generateUid();
      expect(uid).not.toMatch(/[ILOU]/);
    }
  });

  test("produces unique values", () => {
    const ids = new Set<string>();
    for (let i = 0; i < 1000; i++) {
      ids.add(generateUid());
    }
    expect(ids.size).toBe(1000);
  });
});
