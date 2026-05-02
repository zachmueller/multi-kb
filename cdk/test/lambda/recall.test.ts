import type { APIGatewayProxyEvent } from "aws-lambda";

// Mock AWS SDK clients before importing the handler
const mockBedrockAgentSend = jest.fn();
const mockBedrockRuntimeSend = jest.fn();
const mockS3Send = jest.fn();

jest.mock("@aws-sdk/client-bedrock-agent-runtime", () => ({
  BedrockAgentRuntimeClient: jest.fn(() => ({
    send: mockBedrockAgentSend,
  })),
  RetrieveCommand: jest.fn((params: unknown) => params),
}));

jest.mock("@aws-sdk/client-bedrock-runtime", () => ({
  BedrockRuntimeClient: jest.fn(() => ({
    send: mockBedrockRuntimeSend,
  })),
  InvokeModelCommand: jest.fn((params: unknown) => params),
}));

jest.mock("@aws-sdk/client-s3", () => ({
  S3Client: jest.fn(() => ({ send: mockS3Send })),
  PutObjectCommand: jest.fn((params: unknown) => params),
}));

import { handler } from "../../lambda/recall/index";

function makeEvent(body: unknown): APIGatewayProxyEvent {
  return {
    body: typeof body === "string" ? body : JSON.stringify(body),
    headers: {},
    multiValueHeaders: {},
    httpMethod: "POST",
    isBase64Encoded: false,
    path: "/recallKnowledge",
    pathParameters: null,
    queryStringParameters: null,
    multiValueQueryStringParameters: null,
    stageVariables: null,
    requestContext: {
      requestId: "recall-request-123",
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
      path: "/recallKnowledge",
      stage: "prod",
      requestTimeEpoch: Date.now(),
      resourceId: "test",
      resourcePath: "/recallKnowledge",
    },
    resource: "/recallKnowledge",
  };
}

function makeRetrieveResponse(
  results: Array<{
    uid: string;
    title: string;
    content: string;
    score: number;
  }>,
) {
  return {
    retrievalResults: results.map((r) => ({
      content: { text: r.content },
      metadata: { uid: r.uid, title: r.title },
      score: r.score,
    })),
  };
}

describe("recallKnowledge handler", () => {
  beforeEach(() => {
    jest.clearAllMocks();
    process.env.KNOWLEDGE_BASE_ID = "KB-12345";
    process.env.BUCKET_NAME = "test-bucket";
    process.env.COVERAGE_MODEL_ID = "anthropic.claude-haiku-4-5-20251001";
    process.env.COVERAGE_SCORE_THRESHOLD = "0.3";
    process.env.EXCLUDE_PENDING = "true";
    process.env.AWS_REGION = "us-east-1";

    // Default: retrieval returns good results (above threshold)
    mockBedrockAgentSend.mockResolvedValue(
      makeRetrieveResponse([
        {
          uid: "UID001",
          title: "First result",
          content: "Some content",
          score: 0.8,
        },
        {
          uid: "UID002",
          title: "Second result",
          content: "More content",
          score: 0.6,
        },
      ]),
    );

    mockS3Send.mockResolvedValue({});
  });

  afterEach(() => {
    delete process.env.KNOWLEDGE_BASE_ID;
    delete process.env.BUCKET_NAME;
    delete process.env.COVERAGE_MODEL_ID;
    delete process.env.COVERAGE_SCORE_THRESHOLD;
    delete process.env.EXCLUDE_PENDING;
    delete process.env.AWS_REGION;
  });

  test("valid query returns 200 with results sorted by score", async () => {
    const event = makeEvent({ query: "test query" });

    const result = await handler(event);

    expect(result.statusCode).toBe(200);
    const body = JSON.parse(result.body);
    expect(body).toHaveLength(2);
    expect(body[0].uid).toBe("UID001");
    expect(body[0].score).toBe(0.8);
    expect(body[1].uid).toBe("UID002");
  });

  test("missing query returns 400", async () => {
    const event = makeEvent({});

    const result = await handler(event);

    expect(result.statusCode).toBe(400);
    const body = JSON.parse(result.body);
    expect(body.errors.query).toBeDefined();
  });

  test("empty query string returns 400", async () => {
    const event = makeEvent({ query: "   " });

    const result = await handler(event);

    expect(result.statusCode).toBe(400);
    const body = JSON.parse(result.body);
    expect(body.errors.query).toBeDefined();
  });

  test("non-string query returns 400", async () => {
    const event = makeEvent({ query: 42 });

    const result = await handler(event);

    expect(result.statusCode).toBe(400);
  });

  test("invalid JSON body returns 400", async () => {
    const event = makeEvent("{bad json");

    const result = await handler(event);

    expect(result.statusCode).toBe(400);
    const body = JSON.parse(result.body);
    expect(body.errors.body).toBeDefined();
  });

  test("limit out of range returns 400", async () => {
    const event = makeEvent({ query: "test", limit: 0 });

    const result = await handler(event);

    expect(result.statusCode).toBe(400);
    const body = JSON.parse(result.body);
    expect(body.errors.limit).toBeDefined();
  });

  test("limit > 100 returns 400", async () => {
    const event = makeEvent({ query: "test", limit: 101 });

    const result = await handler(event);

    expect(result.statusCode).toBe(400);
  });

  test("non-integer limit returns 400", async () => {
    const event = makeEvent({ query: "test", limit: "ten" });

    const result = await handler(event);

    expect(result.statusCode).toBe(400);
  });

  test("valid limit is accepted", async () => {
    const event = makeEvent({ query: "test", limit: 5 });

    const result = await handler(event);

    expect(result.statusCode).toBe(200);
  });

  test("default limit is 10 when not specified", async () => {
    const event = makeEvent({ query: "test" });

    await handler(event);

    const retrieveCall = mockBedrockAgentSend.mock.calls[0][0];
    expect(
      retrieveCall.retrievalConfiguration.vectorSearchConfiguration
        .numberOfResults,
    ).toBe(10);
  });

  test("calls retrieve with excludePending filter when EXCLUDE_PENDING=true", async () => {
    const event = makeEvent({ query: "test" });

    await handler(event);

    const retrieveCall = mockBedrockAgentSend.mock.calls[0][0];
    expect(
      retrieveCall.retrievalConfiguration.vectorSearchConfiguration.filter,
    ).toEqual({ equals: { key: "status", value: "active" } });
  });

  test("no filter when EXCLUDE_PENDING=false", async () => {
    process.env.EXCLUDE_PENDING = "false";
    const event = makeEvent({ query: "test" });

    await handler(event);

    const retrieveCall = mockBedrockAgentSend.mock.calls[0][0];
    expect(
      retrieveCall.retrievalConfiguration.vectorSearchConfiguration.filter,
    ).toBeUndefined();
  });

  test("coverage assessment triggered when top score below threshold", async () => {
    // Return results below threshold
    mockBedrockAgentSend.mockResolvedValue(
      makeRetrieveResponse([
        {
          uid: "LOW1",
          title: "Low score",
          content: "Content",
          score: 0.1,
        },
      ]),
    );

    // Mock coverage model response
    mockBedrockRuntimeSend.mockResolvedValue({
      body: Buffer.from(
        JSON.stringify({
          content: [
            {
              type: "text",
              text: JSON.stringify({
                gap_detected: true,
                refined_query: "refined search terms",
              }),
            },
          ],
        }),
      ),
    });

    // Second retrieval with refined query
    mockBedrockAgentSend
      .mockResolvedValueOnce(
        makeRetrieveResponse([
          { uid: "LOW1", title: "Low score", content: "Content", score: 0.1 },
        ]),
      )
      .mockResolvedValueOnce(
        makeRetrieveResponse([
          {
            uid: "NEW1",
            title: "New result",
            content: "Better content",
            score: 0.5,
          },
        ]),
      );

    const event = makeEvent({ query: "obscure query" });
    const result = await handler(event);

    expect(result.statusCode).toBe(200);
    const body = JSON.parse(result.body);
    // Should have merged results
    expect(body.length).toBeGreaterThanOrEqual(1);
  });

  test("coverage assessment failure falls back to original results", async () => {
    mockBedrockAgentSend.mockResolvedValue(
      makeRetrieveResponse([
        { uid: "LOW1", title: "Low", content: "Content", score: 0.1 },
      ]),
    );
    mockBedrockRuntimeSend.mockRejectedValue(new Error("Model error"));

    const event = makeEvent({ query: "test" });
    const result = await handler(event);

    // Should still return 200 with original results
    expect(result.statusCode).toBe(200);
    const body = JSON.parse(result.body);
    expect(body[0].uid).toBe("LOW1");
  });

  test("writes recall log to S3", async () => {
    const event = makeEvent({ query: "test" });

    await handler(event);

    expect(mockS3Send).toHaveBeenCalledTimes(1);
    const s3Call = mockS3Send.mock.calls[0][0];
    expect(s3Call.Bucket).toBe("test-bucket");
    expect(s3Call.Key).toMatch(/^recall-logs\//);
    expect(s3Call.ContentType).toBe("application/json");
  });

  test("S3 log failure does not affect response", async () => {
    mockS3Send.mockRejectedValue(new Error("S3 write failed"));

    const event = makeEvent({ query: "test" });
    const result = await handler(event);

    expect(result.statusCode).toBe(200);
  });

  test("Bedrock retrieval failure returns 500", async () => {
    mockBedrockAgentSend.mockRejectedValue(new Error("Bedrock unavailable"));

    const event = makeEvent({ query: "test" });
    const result = await handler(event);

    expect(result.statusCode).toBe(500);
    const body = JSON.parse(result.body);
    expect(body.message).toBeDefined();
  });

  test("response has Content-Type application/json header", async () => {
    const event = makeEvent({ query: "test" });
    const result = await handler(event);

    expect(result.headers?.["Content-Type"]).toBe("application/json");
  });
});
