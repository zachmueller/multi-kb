import type { APIGatewayProxyResult } from "aws-lambda";

const JSON_HEADERS = { "Content-Type": "application/json" };

export function success(
  statusCode: number,
  body: unknown,
): APIGatewayProxyResult {
  return {
    statusCode,
    headers: JSON_HEADERS,
    body: JSON.stringify(body),
  };
}

export function error(
  statusCode: number,
  body: unknown,
): APIGatewayProxyResult {
  return {
    statusCode,
    headers: JSON_HEADERS,
    body: JSON.stringify(body),
  };
}

export function validationError(
  errors: Record<string, string>,
): APIGatewayProxyResult {
  return error(400, { errors });
}

export function internalError(): APIGatewayProxyResult {
  return error(500, { message: "Internal server error" });
}
