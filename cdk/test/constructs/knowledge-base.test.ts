import * as cdk from "aws-cdk-lib";
import { Match, Template } from "aws-cdk-lib/assertions";
import { KnowledgeBase } from "../../lib/constructs/knowledge-base";

function createTemplate(
  overrides?: Partial<{
    collectionArn: string;
    collectionName: string;
    vectorIndexName: string;
    bucketArn: string;
    embeddingModelId: string;
  }>,
): Template {
  const app = new cdk.App();
  const stack = new cdk.Stack(app, "TestStack", {
    env: { account: "123456789012", region: "us-east-1" },
  });
  new KnowledgeBase(stack, "KB", {
    collectionArn:
      overrides?.collectionArn ??
      "arn:aws:aoss:us-east-1:123456789012:collection/abc123",
    collectionName: overrides?.collectionName ?? "multi-kb",
    vectorIndexName: overrides?.vectorIndexName ?? "bedrock-kb-index",
    bucketArn:
      overrides?.bucketArn ??
      "arn:aws:s3:::multi-kb-123456789012-us-east-1",
    embeddingModelId:
      overrides?.embeddingModelId ?? "amazon.titan-embed-text-v2:0",
  });
  return Template.fromStack(stack);
}

describe("KnowledgeBase Construct", () => {
  test("creates Bedrock service role with bedrock trust policy", () => {
    const template = createTemplate();
    template.hasResourceProperties("AWS::IAM::Role", {
      AssumeRolePolicyDocument: Match.objectLike({
        Statement: Match.arrayWith([
          Match.objectLike({
            Principal: { Service: "bedrock.amazonaws.com" },
            Condition: Match.objectLike({
              StringEquals: Match.objectLike({
                "aws:SourceAccount": "123456789012",
              }),
            }),
          }),
        ]),
      }),
    });
  });

  test("service role has S3 GetObject scoped to bucket ARN/*", () => {
    const template = createTemplate();
    template.hasResourceProperties("AWS::IAM::Role", {
      Policies: Match.arrayWith([
        Match.objectLike({
          PolicyName: "S3Access",
          PolicyDocument: Match.objectLike({
            Statement: Match.arrayWith([
              Match.objectLike({
                Action: "s3:GetObject",
                Resource:
                  "arn:aws:s3:::multi-kb-123456789012-us-east-1/*",
              }),
            ]),
          }),
        }),
      ]),
    });
  });

  test("service role has S3 ListBucket scoped to bucket ARN", () => {
    const template = createTemplate();
    template.hasResourceProperties("AWS::IAM::Role", {
      Policies: Match.arrayWith([
        Match.objectLike({
          PolicyName: "S3Access",
          PolicyDocument: Match.objectLike({
            Statement: Match.arrayWith([
              Match.objectLike({
                Action: "s3:ListBucket",
                Resource:
                  "arn:aws:s3:::multi-kb-123456789012-us-east-1",
              }),
            ]),
          }),
        }),
      ]),
    });
  });

  test("service role has AOSS access scoped to collection ARN", () => {
    const template = createTemplate();
    template.hasResourceProperties("AWS::IAM::Role", {
      Policies: Match.arrayWith([
        Match.objectLike({
          PolicyName: "AOSSAccess",
          PolicyDocument: Match.objectLike({
            Statement: Match.arrayWith([
              Match.objectLike({
                Action: "aoss:APIAccessAll",
                Resource:
                  "arn:aws:aoss:us-east-1:123456789012:collection/abc123",
              }),
            ]),
          }),
        }),
      ]),
    });
  });

  test("service role has Bedrock InvokeModel scoped to embedding model ARN", () => {
    const template = createTemplate();
    template.hasResourceProperties("AWS::IAM::Role", {
      Policies: Match.arrayWith([
        Match.objectLike({
          PolicyName: "BedrockEmbedding",
          PolicyDocument: Match.objectLike({
            Statement: Match.arrayWith([
              Match.objectLike({
                Action: "bedrock:InvokeModel",
                Resource:
                  "arn:aws:bedrock:us-east-1::foundation-model/amazon.titan-embed-text-v2:0",
              }),
            ]),
          }),
        }),
      ]),
    });
  });

  test("creates Bedrock KnowledgeBase with VECTOR type", () => {
    const template = createTemplate();
    template.hasResourceProperties("AWS::Bedrock::KnowledgeBase", {
      KnowledgeBaseConfiguration: {
        Type: "VECTOR",
        VectorKnowledgeBaseConfiguration: {
          EmbeddingModelArn:
            "arn:aws:bedrock:us-east-1::foundation-model/amazon.titan-embed-text-v2:0",
        },
      },
    });
  });

  test("KB storage config points to OpenSearch Serverless", () => {
    const template = createTemplate();
    template.hasResourceProperties("AWS::Bedrock::KnowledgeBase", {
      StorageConfiguration: Match.objectLike({
        Type: "OPENSEARCH_SERVERLESS",
        OpensearchServerlessConfiguration: Match.objectLike({
          CollectionArn:
            "arn:aws:aoss:us-east-1:123456789012:collection/abc123",
          VectorIndexName: "bedrock-kb-index",
          FieldMapping: {
            VectorField: "bedrock-knowledge-base-default-vector",
            TextField: "AMAZON_BEDROCK_TEXT_CHUNK",
            MetadataField: "AMAZON_BEDROCK_METADATA",
          },
        }),
      }),
    });
  });

  test("creates DataSource with S3 type and NONE chunking", () => {
    const template = createTemplate();
    template.hasResourceProperties("AWS::Bedrock::DataSource", {
      DataSourceConfiguration: {
        Type: "S3",
        S3Configuration: {
          BucketArn:
            "arn:aws:s3:::multi-kb-123456789012-us-east-1",
        },
      },
      VectorIngestionConfiguration: {
        ChunkingConfiguration: {
          ChunkingStrategy: "NONE",
        },
      },
    });
  });

  test("creates KnowledgeBaseId CfnOutput", () => {
    const template = createTemplate();
    const outputs = template.findOutputs("*");
    const hasOutput = Object.keys(outputs).some((k) =>
      k.includes("KnowledgeBaseIdOutput"),
    );
    expect(hasOutput).toBe(true);
  });

  test("creates DataSourceId CfnOutput", () => {
    const template = createTemplate();
    const outputs = template.findOutputs("*");
    const hasOutput = Object.keys(outputs).some((k) =>
      k.includes("DataSourceIdOutput"),
    );
    expect(hasOutput).toBe(true);
  });
});
