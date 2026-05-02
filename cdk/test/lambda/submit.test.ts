import type { APIGatewayProxyEvent } from "aws-lambda";

// Mock SQS client before importing the handler
const mockSend = jest.fn();
jest.mock("@aws-sdk/client-sqs", () => ({
  SQSClient: jest.fn(() => ({ send: mockSend })),
  SendMessageCommand: jest.fn((params: unknown) => params),
}));

// Mock generateUid
jest.mock("../../lambda/shared/uid", () => ({
  generateUid: jest.fn(() => "TESTUID123456789"),
}));

import { handler } from "../../lambda/submit/index";

function makeEvent(body: unknown): APIGatewayProxyEvent {
  return {
    body: typeof body === "string" ? body : JSON.stringify(body),
    headers: {},
    multiValueHeaders: {},
    httpMethod: "POST",
    isBase64Encoded: false,
    path: "/submitKnowledge",
    pathParameters: null,
    queryStringParameters: null,
    multiValueQueryStringParameters: null,
    stageVariables: null,
    requestContext: {
      requestId: "test-request-id-123",
      accountId: "123456789012",
      apiId: "testapi",
      authorizer: null,
      protocol: "HTTP/1.1",
      httpMethod: "POST",
      identity: {
        accessKey: null,
        accountId: null,
        apiKey: null,
        apiKeyId: null,
        caller: null,
        clientCert: null,
        cognitoAuthenticationProvider: null,
        cognitoAuthenticationType: null,
        cognitoIdentityId: null,
        cognitoIdentityPoolId: null,
        principalOrgId: null,
        sourceIp: "127.0.0.1",
        user: null,
        userAgent: null,
        userArn: null,
      },
      path: "/submitKnowledge",
      stage: "prod",
      requestTimeEpoch: Date.now(),
      resourceId: "test",
      resourcePath: "/submitKnowledge",
    },
    resource: "/submitKnowledge",
  };
}

describe("submitKnowledge handler", () => {
  beforeEach(() => {
    jest.clearAllMocks();
    process.env.SQS_QUEUE_URL =
      "https://sqs.us-east-1.amazonaws.com/123456789012/test-queue";
    mockSend.mockResolvedValue({});
  });

  afterEach(() => {
    delete process.env.SQS_QUEUE_URL;
  });

  test("valid input returns 202 with uid and request_id", async () => {
    const event = makeEvent({
      title: "Test Note",
      content: "This is the note body",
      author: "alice",
    });

    const result = await handler(event);

    expect(result.statusCode).toBe(202);
    const body = JSON.parse(result.body);
    expect(body.uid).toBe("TESTUID123456789");
    expect(body.request_id).toBe("test-request-id-123");
  });

  test("valid input sends SQS message with correct fields", async () => {
    const event = makeEvent({
      title: "Test Note",
      content: "This is the note body",
      author: "alice",
    });

    await handler(event);

    expect(mockSend).toHaveBeenCalledTimes(1);
    const sentCommand = mockSend.mock.calls[0][0];
    expect(sentCommand.QueueUrl).toBe(process.env.SQS_QUEUE_URL);
    const messageBody = JSON.parse(sentCommand.MessageBody);
    expect(messageBody.uid).toBe("TESTUID123456789");
    expect(messageBody.title).toBe("Test Note");
    expect(messageBody.content).toBe("This is the note body");
    expect(messageBody.author).toBe("alice");
    expect(messageBody.submitted_at).toBeDefined();
  });

  test("missing title returns 400 with validation error", async () => {
    const event = makeEvent({
      content: "body",
      author: "alice",
    });

    const result = await handler(event);

    expect(result.statusCode).toBe(400);
    const body = JSON.parse(result.body);
    expect(body.errors.title).toBeDefined();
    expect(mockSend).not.toHaveBeenCalled();
  });

  test("missing content returns 400", async () => {
    const event = makeEvent({
      title: "Test",
      author: "alice",
    });

    const result = await handler(event);

    expect(result.statusCode).toBe(400);
    const body = JSON.parse(result.body);
    expect(body.errors.content).toBeDefined();
  });

  test("missing author returns 400", async () => {
    const event = makeEvent({
      title: "Test",
      content: "body",
    });

    const result = await handler(event);

    expect(result.statusCode).toBe(400);
    const body = JSON.parse(result.body);
    expect(body.errors.author).toBeDefined();
  });

  test("empty body object returns 400 with all field errors", async () => {
    const event = makeEvent({});

    const result = await handler(event);

    expect(result.statusCode).toBe(400);
    const body = JSON.parse(result.body);
    expect(body.errors.title).toBeDefined();
    expect(body.errors.content).toBeDefined();
    expect(body.errors.author).toBeDefined();
  });

  test("invalid JSON body returns 400", async () => {
    const event = makeEvent("not-valid-json{{{");

    const result = await handler(event);

    expect(result.statusCode).toBe(400);
    const body = JSON.parse(result.body);
    expect(body.errors.body).toBeDefined();
  });

  test("null body parses as empty object and returns 400", async () => {
    const event = makeEvent({});
    event.body = null;

    const result = await handler(event);

    expect(result.statusCode).toBe(400);
  });

  test("SQS failure returns 500", async () => {
    mockSend.mockRejectedValue(new Error("SQS send failed"));

    const event = makeEvent({
      title: "Test Note",
      content: "body content",
      author: "alice",
    });

    const result = await handler(event);

    expect(result.statusCode).toBe(500);
    const body = JSON.parse(result.body);
    expect(body.message).toBeDefined();
  });

  test("response has Content-Type application/json header", async () => {
    const event = makeEvent({
      title: "Test",
      content: "body",
      author: "alice",
    });

    const result = await handler(event);

    expect(result.headers?.["Content-Type"]).toBe("application/json");
  });
});
