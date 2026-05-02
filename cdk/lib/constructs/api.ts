import * as cdk from "aws-cdk-lib";
import * as apigateway from "aws-cdk-lib/aws-apigateway";
import * as lambda from "aws-cdk-lib/aws-lambda";
import * as logs from "aws-cdk-lib/aws-logs";
import { Construct } from "constructs";

export interface ApiProps {
  readonly submitLambda: lambda.Function;
  readonly recallLambda: lambda.Function;
}

export class Api extends Construct {
  readonly api: apigateway.RestApi;
  readonly endpointUrl: string;

  constructor(scope: Construct, id: string, props: ApiProps) {
    super(scope, id);

    const accessLogGroup = new logs.LogGroup(this, "AccessLogs", {
      retention: logs.RetentionDays.ONE_MONTH,
    });

    // API-001: REST API (not HTTP API) — required for AWS_IAM auth
    this.api = new apigateway.RestApi(this, "RestApi", {
      restApiName: "multi-kb",
      description: "Multi-KB knowledge submission and recall API",
      deployOptions: {
        stageName: "prod",
        accessLogDestination: new apigateway.LogGroupLogDestination(
          accessLogGroup,
        ),
        accessLogFormat: apigateway.AccessLogFormat.jsonWithStandardFields(),
        loggingLevel: apigateway.MethodLoggingLevel.INFO,
      },
      // CORS not enabled — CLI is not a browser client
    });

    // API-002: POST /submitKnowledge with AWS_IAM auth + Lambda proxy integration
    const submitResource = this.api.root.addResource("submitKnowledge");
    submitResource.addMethod(
      "POST",
      new apigateway.LambdaIntegration(props.submitLambda, { proxy: true }),
      { authorizationType: apigateway.AuthorizationType.IAM },
    );

    // API-003: POST /recallKnowledge with AWS_IAM auth + Lambda proxy integration
    const recallResource = this.api.root.addResource("recallKnowledge");
    recallResource.addMethod(
      "POST",
      new apigateway.LambdaIntegration(props.recallLambda, { proxy: true }),
      { authorizationType: apigateway.AuthorizationType.IAM },
    );

    this.endpointUrl = this.api.url;

    // API-004: Stack outputs for API Gateway
    new cdk.CfnOutput(this, "ApiEndpoint", {
      value: this.api.url,
      description: "API Gateway endpoint URL",
    });

    new cdk.CfnOutput(this, "ApiId", {
      value: this.api.restApiId,
      description: "API Gateway REST API ID",
    });
  }
}
