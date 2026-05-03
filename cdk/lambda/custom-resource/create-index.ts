import type {
  CloudFormationCustomResourceEvent,
  CloudFormationCustomResourceResponse,
} from "aws-lambda";
import { SignatureV4 } from "@smithy/signature-v4";
import { Sha256 } from "@aws-crypto/sha256-js";
import { defaultProvider } from "@aws-sdk/credential-provider-node";
import * as https from "https";
import * as http from "http";

const INDEX_SCHEMA = {
  settings: {
    index: {
      knn: true,
    },
  },
  mappings: {
    properties: {
      "bedrock-knowledge-base-default-vector": {
        type: "knn_vector",
        dimension: 1024,
        method: {
          name: "hnsw",
          engine: "faiss",
          space_type: "l2",
        },
      },
      AMAZON_BEDROCK_TEXT_CHUNK: {
        type: "text",
        index: true,
      },
      AMAZON_BEDROCK_METADATA: {
        type: "text",
        index: false,
      },
    },
  },
};

async function signedRequest(
  method: string,
  url: string,
  body: string,
): Promise<{ statusCode: number; body: string }> {
  const parsed = new URL(url);
  const region = process.env.AWS_REGION ?? "us-east-1";

  const signer = new SignatureV4({
    credentials: defaultProvider(),
    region,
    service: "aoss",
    sha256: Sha256,
  });

  const request = {
    method,
    hostname: parsed.hostname,
    path: parsed.pathname,
    protocol: parsed.protocol,
    headers: {
      host: parsed.hostname,
      "content-type": "application/json",
      "content-length": Buffer.byteLength(body).toString(),
    },
    body,
  };

  const signed = await signer.sign(request);

  return new Promise((resolve, reject) => {
    const mod = parsed.protocol === "https:" ? https : http;
    const req = mod.request(
      {
        hostname: parsed.hostname,
        path: parsed.pathname,
        method: signed.method,
        headers: signed.headers as Record<string, string>,
      },
      (res) => {
        let responseBody = "";
        res.on("data", (chunk: Buffer) => {
          responseBody += chunk.toString();
        });
        res.on("end", () => {
          resolve({
            statusCode: res.statusCode ?? 0,
            body: responseBody,
          });
        });
      },
    );
    req.on("error", reject);
    req.write(body);
    req.end();
  });
}

export const handler = async (
  event: CloudFormationCustomResourceEvent,
): Promise<CloudFormationCustomResourceResponse> => {
  console.log("Create-index custom resource invoked", JSON.stringify(event));

  const collectionEndpoint =
    event.ResourceProperties["CollectionEndpoint"] ??
    process.env.COLLECTION_ENDPOINT;
  const indexName =
    event.ResourceProperties["IndexName"] ?? process.env.INDEX_NAME;

  const physicalResourceId = `${collectionEndpoint}/${indexName}`;

  const base: Omit<CloudFormationCustomResourceResponse, "Status"> = {
    PhysicalResourceId: physicalResourceId,
    StackId: event.StackId,
    RequestId: event.RequestId,
    LogicalResourceId: event.LogicalResourceId,
  };

  if (event.RequestType === "Delete") {
    // Best-effort deletion of the index
    try {
      const url = `${collectionEndpoint}/${indexName}`;
      await signedRequest("DELETE", url, "");
    } catch (err) {
      console.warn("Index deletion failed (ignoring):", err);
    }
    return { ...base, Status: "SUCCESS" };
  }

  if (event.RequestType === "Update") {
    // Verify index still exists; recreate if missing (e.g. after collection rebuild)
    try {
      const url = `${collectionEndpoint}/${indexName}`;
      const check = await signedRequest("GET", url, "");
      if (check.statusCode === 200) {
        console.log("Index exists on Update — no-op");
        return { ...base, Status: "SUCCESS" };
      }
      console.log(`Index missing on Update (HTTP ${check.statusCode}), recreating`);
    } catch (err) {
      console.log("Index check failed on Update, attempting recreation:", err);
    }
    // Fall through to Create logic below
  }

  // Create
  try {
    const url = `${collectionEndpoint}/${indexName}`;
    const body = JSON.stringify(INDEX_SCHEMA);

    const response = await signedRequest("PUT", url, body);
    console.log("Create index response:", response.statusCode, response.body);

    const created =
      response.statusCode === 200 ||
      response.statusCode === 201 ||
      response.body.includes("already_exists") ||
      response.body.includes("resource_already_exists_exception");

    if (!created) {
      throw new Error(
        `Unexpected status ${response.statusCode}: ${response.body}`,
      );
    }

    // AOSS has eventual consistency: poll until the index is visible via GET
    const maxAttempts = 30;
    const intervalMs = 5000;
    for (let i = 1; i <= maxAttempts; i++) {
      const check = await signedRequest("GET", url, "");
      console.log(`Index readiness check ${i}/${maxAttempts}: HTTP ${check.statusCode}`);
      if (check.statusCode === 200) {
        console.log("Index confirmed ready via VPC path, waiting 60s for AOSS service networking propagation");
        await new Promise((r) => setTimeout(r, 60000));
        console.log("Stabilization wait complete");
        return { ...base, Status: "SUCCESS" };
      }
      if (i < maxAttempts) {
        await new Promise((r) => setTimeout(r, intervalMs));
      }
    }

    throw new Error(
      `Index created but not queryable after ${maxAttempts * intervalMs / 1000}s`,
    );
  } catch (err) {
    console.error("Failed to create index:", err);
    return {
      ...base,
      Status: "FAILED",
      Reason: err instanceof Error ? err.message : String(err),
    };
  }
};
