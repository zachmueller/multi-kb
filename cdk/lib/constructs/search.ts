import * as cdk from "aws-cdk-lib";
import * as ec2 from "aws-cdk-lib/aws-ec2";
import * as iam from "aws-cdk-lib/aws-iam";
import * as lambda from "aws-cdk-lib/aws-lambda";
import * as opensearchserverless from "aws-cdk-lib/aws-opensearchserverless";
import * as cr from "aws-cdk-lib/custom-resources";
import { NodejsFunction } from "aws-cdk-lib/aws-lambda-nodejs";
import { Construct } from "constructs";
import * as path from "path";

export interface SearchProps {
  readonly collectionName: string;
  readonly aossVpcEndpointId: string;
  readonly vpc: ec2.IVpc;
  readonly subnet: ec2.ISubnet;
  readonly endpointSecurityGroup: ec2.SecurityGroup;
  readonly ec2SecurityGroup: ec2.SecurityGroup;
}

export class Search extends Construct {
  readonly encryptionPolicy: opensearchserverless.CfnSecurityPolicy;
  readonly collection: opensearchserverless.CfnCollection;
  readonly collectionArn: string;
  readonly collectionEndpoint: string;
  readonly collectionId: string;
  readonly networkPolicy: opensearchserverless.CfnSecurityPolicy;
  readonly indexCreationLambdaRole: iam.Role;
  readonly indexName: string;

  constructor(scope: Construct, id: string, props: SearchProps) {
    super(scope, id);

    this.indexName = "bedrock-kb-index";

    // SRC-002: Encryption policy — must exist before collection (SRC-001)
    // Policy is a single object (NOT an array — unlike network and data access policies)
    this.encryptionPolicy = new opensearchserverless.CfnSecurityPolicy(
      this,
      "EncryptionPolicy",
      {
        name: `${props.collectionName}-enc`,
        type: "encryption",
        policy: JSON.stringify({
          AWSOwnedKey: true,
          Rules: [
            {
              ResourceType: "collection",
              Resource: [`collection/${props.collectionName}`],
            },
          ],
        }),
      },
    );

    // SRC-003: Network policy — dual access (EC2 via VPC endpoint + Bedrock via service networking)
    // AllowFromPublic: false is CRITICAL — setting true silently overrides SourceVPCEs and SourceServices
    this.networkPolicy = new opensearchserverless.CfnSecurityPolicy(
      this,
      "NetworkPolicy",
      {
        name: `${props.collectionName}-net`,
        type: "network",
        // Network policy is an ARRAY (unlike encryption policy which is a single object)
        policy: JSON.stringify([
          {
            AllowFromPublic: false,
            SourceVPCEs: [props.aossVpcEndpointId],
            SourceServices: ["bedrock.amazonaws.com"],
            Rules: [
              {
                ResourceType: "collection",
                Resource: [`collection/${props.collectionName}`],
              },
              {
                ResourceType: "dashboard",
                Resource: [`collection/${props.collectionName}`],
              },
            ],
          },
        ]),
      },
    );

    // SRC-001: OpenSearch Serverless collection — type VECTORSEARCH
    // standbyReplicas: DISABLED for MVP cost savings
    // MUST depend on encryption policy — collection creation FAILS without it
    this.collection = new opensearchserverless.CfnCollection(
      this,
      "Collection",
      {
        name: props.collectionName,
        type: "VECTORSEARCH",
        standbyReplicas: "DISABLED",
      },
    );
    this.collection.addDependency(this.encryptionPolicy);
    this.collection.addDependency(this.networkPolicy);

    this.collectionArn = this.collection.attrArn;
    this.collectionEndpoint = this.collection.attrCollectionEndpoint;
    this.collectionId = this.collection.attrId;

    // SRC-005: Stack output for collection endpoint
    new cdk.CfnOutput(this, "CollectionEndpoint", {
      value: this.collectionEndpoint,
      description: "OpenSearch Serverless collection endpoint",
    });

    // SRC-006: Create Lambda role for index creation BEFORE data access policy (SRC-004)
    // so the role ARN can be included in SRC-004 principals.
    this.indexCreationLambdaRole = new iam.Role(
      this,
      "IndexCreationLambdaRole",
      {
        assumedBy: new iam.ServicePrincipal("lambda.amazonaws.com"),
        managedPolicies: [
          iam.ManagedPolicy.fromAwsManagedPolicyName(
            "service-role/AWSLambdaVPCAccessExecutionRole",
          ),
        ],
        inlinePolicies: {
          AOSSAccess: new iam.PolicyDocument({
            statements: [
              new iam.PolicyStatement({
                actions: ["aoss:APIAccessAll"],
                resources: [this.collectionArn],
              }),
            ],
          }),
        },
      },
    );

    // SRC-006: Custom resource Lambda that pre-creates the OpenSearch vector index.
    // Bedrock KB does NOT auto-create the index via CloudFormation — this is required.
    const indexCreationFunction = new NodejsFunction(
      this,
      "IndexCreationFunction",
      {
        runtime: lambda.Runtime.NODEJS_22_X,
        architecture: lambda.Architecture.ARM_64,
        entry: path.join(
          __dirname,
          "../../lambda/custom-resource/create-index.ts",
        ),
        handler: "handler",
        memorySize: 256,
        timeout: cdk.Duration.minutes(5),
        role: this.indexCreationLambdaRole,
        vpc: props.vpc,
        vpcSubnets: { subnets: [props.subnet] },
        securityGroups: [props.ec2SecurityGroup],
        environment: {
          COLLECTION_ENDPOINT: this.collectionEndpoint,
          INDEX_NAME: this.indexName,
          COLLECTION_NAME: props.collectionName,
        },
        bundling: {
          minify: true,
          sourceMap: false,
          externalModules: [],
        },
      },
    );

    const indexProvider = new cr.Provider(this, "IndexProvider", {
      onEventHandler: indexCreationFunction,
    });

    const indexCustomResource = new cdk.CustomResource(
      this,
      "IndexCustomResource",
      {
        serviceToken: indexProvider.serviceToken,
        properties: {
          CollectionEndpoint: this.collectionEndpoint,
          IndexName: this.indexName,
        },
      },
    );

    // Custom resource execution depends on the collection being ready and data access policy
    // being applied. The data access policy is added via addIndexCreationPrincipal after SRC-004
    // is created. For now, depend on the collection.
    indexCustomResource.node.addDependency(this.collection);
  }

  /**
   * SRC-004: Creates the data access policy at the given scope. Called from the main stack
   * after Compute (EC2 role) and KnowledgeBase (service role) are created.
   * Policy is created OUTSIDE the Search construct to avoid cyclic dependencies.
   */
  createDataAccessPolicy(
    scope: Construct,
    ec2RoleArn: string,
    bedrockKbRoleArn: string,
  ): opensearchserverless.CfnAccessPolicy {
    const collectionName = this.collection.name;
    const policy = new opensearchserverless.CfnAccessPolicy(
      scope,
      "DataAccessPolicy",
      {
        name: `${collectionName}-dap`,
        type: "data",
        policy: cdk.Stack.of(scope).toJsonString([
          {
            Rules: [
              {
                ResourceType: "index",
                Resource: [`index/${collectionName}/*`],
                Permission: [
                  "aoss:ReadDocument",
                  "aoss:WriteDocument",
                  "aoss:CreateIndex",
                  "aoss:DeleteIndex",
                  "aoss:UpdateIndex",
                  "aoss:DescribeIndex",
                ],
              },
              {
                ResourceType: "collection",
                Resource: [`collection/${collectionName}`],
                Permission: [
                  "aoss:CreateCollectionItems",
                  "aoss:DeleteCollectionItems",
                  "aoss:UpdateCollectionItems",
                  "aoss:DescribeCollectionItems",
                ],
              },
            ],
            Principal: [
              ec2RoleArn,
              bedrockKbRoleArn,
              this.indexCreationLambdaRole.roleArn,
            ],
          },
        ]),
      },
    );
    return policy;
  }
}
