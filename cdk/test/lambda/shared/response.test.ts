import {
  success,
  error,
  validationError,
  internalError,
} from "../../../lambda/shared/response";

describe("response helpers", () => {
  test("success sets statusCode and JSON body", () => {
    const result = success(202, { uid: "abc123" });
    expect(result.statusCode).toBe(202);
    expect(JSON.parse(result.body)).toEqual({ uid: "abc123" });
    expect(result.headers?.["Content-Type"]).toBe("application/json");
  });

  test("error sets statusCode and JSON body", () => {
    const result = error(404, { message: "not found" });
    expect(result.statusCode).toBe(404);
    expect(JSON.parse(result.body)).toEqual({ message: "not found" });
  });

  test("validationError returns 400 with errors object", () => {
    const result = validationError({ title: "required" });
    expect(result.statusCode).toBe(400);
    expect(JSON.parse(result.body)).toEqual({ errors: { title: "required" } });
  });

  test("internalError returns 500 with generic message", () => {
    const result = internalError();
    expect(result.statusCode).toBe(500);
    const body = JSON.parse(result.body);
    expect(body.message).toBeDefined();
  });
});
