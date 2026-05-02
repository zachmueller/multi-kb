import * as cdk from "aws-cdk-lib";
import * as bedrock from "aws-cdk-lib/aws-bedrock";
import * as iam from "aws-cdk-lib/aws-iam";
import { Construct } from "constructs";

export interface KnowledgeBaseProps {
  readonly collectionArn: string;
  readonly collectionName: string;
  readonly vectorIndexName: string;
  readonly bucketArn: string;
  readonly embeddingModelId: string;
}

export class KnowledgeBase extends Construct {
  readonly serviceRole: iam.Role;
  readonly serviceRoleArn: string;
  readonly knowledgeBase: bedrock.CfnKnowledgeBase;
  readonly knowledgeBaseId: string;
  readonly knowledgeBaseArn: string;
  readonly dataSource: bedrock.CfnDataSource;
  readonly dataSourceId: string;

  constructor(scope: Construct, id: string, props: KnowledgeBaseProps) {
    super(scope, id);

    const stack = cdk.Stack.of(this);
    const embeddingModelArn = `arn:aws:bedrock:${stack.region}::foundation-model/${props.embeddingModelId}`;

    // KBS-002: Bedrock KB service role
    // Trust policy scoped to bedrock.amazonaws.com with aws:SourceAccount + ArnLike conditions
    this.serviceRole = new iam.Role(this, "ServiceRole", {
      assumedBy: new iam.ServicePrincipal("bedrock.amazonaws.com", {
        conditions: {
          StringEquals: {
            "aws:SourceAccount": stack.account,
          },
          ArnLike: {
            "aws:SourceArn": `arn:aws:bedrock:${stack.region}:${stack.account}:knowledge-base/*`,
          },
        },
      }),
      inlinePolicies: {
        S3Access: new iam.PolicyDocument({
          statements: [
            new iam.PolicyStatement({
              actions: ["s3:GetObject"],
              resources: [`${props.bucketArn}/*`],
              conditions: {
                StringEquals: {
                  "aws:ResourceAccount": stack.account,
                },
              },
            }),
            new iam.PolicyStatement({
              actions: ["s3:ListBucket"],
              resources: [props.bucketArn],
              conditions: {
                StringEquals: {
                  "aws:ResourceAccount": stack.account,
                },
              },
            }),
          ],
        }),
        AOSSAccess: new iam.PolicyDocument({
          statements: [
            new iam.PolicyStatement({
              actions: ["aoss:APIAccessAll"],
              resources: [props.collectionArn],
            }),
          ],
        }),
        BedrockEmbedding: new iam.PolicyDocument({
          statements: [
            new iam.PolicyStatement({
              actions: ["bedrock:InvokeModel"],
              // Foundation model ARN uses empty account ID (AWS-owned resource)
              resources: [embeddingModelArn],
            }),
          ],
        }),
      },
    });
    this.serviceRoleArn = this.serviceRole.roleArn;

    // KBS-001: Bedrock Knowledge Base
    // Vector index must exist before KB creation — handled via addDependency in the main stack
    this.knowledgeBase = new bedrock.CfnKnowledgeBase(this, "KnowledgeBase", {
      name: `${props.collectionName}-kb`,
      roleArn: this.serviceRoleArn,
      knowledgeBaseConfiguration: {
        type: "VECTOR",
        vectorKnowledgeBaseConfiguration: {
          embeddingModelArn,
        },
      },
      storageConfiguration: {
        type: "OPENSEARCH_SERVERLESS",
        opensearchServerlessConfiguration: {
          collectionArn: props.collectionArn,
          vectorIndexName: props.vectorIndexName,
          fieldMapping: {
            vectorField: "bedrock-knowledge-base-default-vector",
            textField: "AMAZON_BEDROCK_TEXT_CHUNK",
            metadataField: "AMAZON_BEDROCK_METADATA",
          },
        },
      },
    });
    this.knowledgeBaseId = this.knowledgeBase.attrKnowledgeBaseId;
    this.knowledgeBaseArn = this.knowledgeBase.attrKnowledgeBaseArn;

    // KBS-003: Bedrock KB Data Source — S3 bucket, chunking strategy NONE
    this.dataSource = new bedrock.CfnDataSource(this, "DataSource", {
      knowledgeBaseId: this.knowledgeBaseId,
      name: `${props.collectionName}-ds`,
      dataSourceConfiguration: {
        type: "S3",
        s3Configuration: {
          bucketArn: props.bucketArn,
        },
      },
      vectorIngestionConfiguration: {
        chunkingConfiguration: {
          chunkingStrategy: "NONE",
        },
      },
    });
    this.dataSourceId = this.dataSource.attrDataSourceId;

    // Stack outputs — search infrastructure (SRC-005)
    new cdk.CfnOutput(this, "KnowledgeBaseIdOutput", {
      exportName: "KnowledgeBaseId",
      value: this.knowledgeBaseId,
    });
    new cdk.CfnOutput(this, "DataSourceIdOutput", {
      exportName: "DataSourceId",
      value: this.dataSourceId,
    });
  }
}
