import { SQSClient, SendMessageCommand } from "@aws-sdk/client-sqs";
import type {
  APIGatewayProxyEvent,
  APIGatewayProxyResult,
} from "aws-lambda";
import { generateUid } from "../shared/uid";
import {
  internalError,
  success,
  validationError,
} from "../shared/response";
import { validateSubmitKnowledge } from "../shared/validation";

const sqs = new SQSClient({});

export const handler = async (
  event: APIGatewayProxyEvent,
): Promise<APIGatewayProxyResult> => {
  try {
    let body: unknown;
    try {
      body = JSON.parse(event.body ?? "{}");
    } catch {
      return validationError({ body: "invalid JSON" });
    }

    const validation = validateSubmitKnowledge(body);
    if (!validation.valid) {
      return validationError(validation.errors);
    }

    const { title, content, author } = validation.data;
    const uid = generateUid();
    const submittedAt = new Date().toISOString();

    const message = { uid, title, content, author, submitted_at: submittedAt };

    await sqs.send(
      new SendMessageCommand({
        QueueUrl: process.env.SQS_QUEUE_URL,
        MessageBody: JSON.stringify(message),
      }),
    );

    return success(202, {
      uid,
      request_id: event.requestContext.requestId,
    });
  } catch (err) {
    console.error("submitKnowledge error:", err);
    return internalError();
  }
};
