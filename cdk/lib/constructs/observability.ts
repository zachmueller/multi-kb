import * as cdk from "aws-cdk-lib";
import * as cloudwatch from "aws-cdk-lib/aws-cloudwatch";
import * as logs from "aws-cdk-lib/aws-logs";
import * as sqs from "aws-cdk-lib/aws-sqs";
import { Construct } from "constructs";

export interface ObservabilityProps {
  readonly dlq: sqs.Queue;
  readonly submitLambdaFunctionName: string;
  readonly recallLambdaFunctionName: string;
  readonly asgName?: string;
}

export class Observability extends Construct {
  readonly ec2LogGroup: logs.LogGroup;
  readonly ec2LogGroupName: string;
  readonly submitLambdaLogGroup: logs.LogGroup;
  readonly recallLambdaLogGroup: logs.LogGroup;

  constructor(scope: Construct, id: string, props: ObservabilityProps) {
    super(scope, id);

    // --- OBS-001: CloudWatch Log Groups ---

    // EC2 CLI process log group (CloudWatch agent ships logs here)
    this.ec2LogGroup = new logs.LogGroup(this, "Ec2CliLogGroup", {
      logGroupName: "/multi-kb/ec2/server",
      retention: logs.RetentionDays.ONE_MONTH,
      removalPolicy: cdk.RemovalPolicy.DESTROY,
    });
    this.ec2LogGroupName = this.ec2LogGroup.logGroupName;

    // Lambda log groups — Lambda auto-creates these, but we set retention
    this.submitLambdaLogGroup = new logs.LogGroup(
      this,
      "SubmitLambdaLogGroup",
      {
        logGroupName: `/aws/lambda/${props.submitLambdaFunctionName}`,
        retention: logs.RetentionDays.ONE_MONTH,
        removalPolicy: cdk.RemovalPolicy.DESTROY,
      },
    );

    this.recallLambdaLogGroup = new logs.LogGroup(
      this,
      "RecallLambdaLogGroup",
      {
        logGroupName: `/aws/lambda/${props.recallLambdaFunctionName}`,
        retention: logs.RetentionDays.ONE_MONTH,
        removalPolicy: cdk.RemovalPolicy.DESTROY,
      },
    );

    // --- OBS-002: CloudWatch Alarms ---

    // Alarm: DLQ messages visible > 0
    new cloudwatch.Alarm(this, "DlqAlarm", {
      alarmName: "multi-kb-dlq-messages",
      alarmDescription:
        "Dead-letter queue has messages — indicates processing failures",
      metric: props.dlq.metricApproximateNumberOfMessagesVisible({
        period: cdk.Duration.minutes(5),
        statistic: "Maximum",
      }),
      threshold: 0,
      comparisonOperator:
        cloudwatch.ComparisonOperator.GREATER_THAN_THRESHOLD,
      evaluationPeriods: 1,
      treatMissingData: cloudwatch.TreatMissingData.NOT_BREACHING,
      // No alarm actions — metrics only for MVP
    });

    // Alarm: ASG instances < 1 (EC2 unhealthy)
    if (props.asgName) {
      new cloudwatch.Alarm(this, "AsgUnhealthyAlarm", {
        alarmName: "multi-kb-ec2-unhealthy",
        alarmDescription:
          "No EC2 instances in service — server mode is down",
        metric: new cloudwatch.Metric({
          namespace: "AWS/AutoScaling",
          metricName: "GroupInServiceInstances",
          dimensionsMap: { AutoScalingGroupName: props.asgName },
          period: cdk.Duration.minutes(5),
          statistic: "Minimum",
        }),
        threshold: 1,
        comparisonOperator:
          cloudwatch.ComparisonOperator.LESS_THAN_THRESHOLD,
        evaluationPeriods: 1,
        treatMissingData: cloudwatch.TreatMissingData.BREACHING,
      });
    }

    // Alarm: Dream cycle lock held > 60 minutes (metric filter on EC2 log group)
    const lockHeldMetricFilter = this.ec2LogGroup.addMetricFilter(
      "DreamCycleLockHeld",
      {
        filterPattern: logs.FilterPattern.literal('"dream cycle lock"'),
        metricNamespace: "MultiKB",
        metricName: "DreamCycleLockHeldMinutes",
        metricValue: "1",
        defaultValue: 0,
      },
    );

    new cloudwatch.Alarm(this, "DreamCycleLockAlarm", {
      alarmName: "multi-kb-dream-cycle-lock",
      alarmDescription:
        "Dream cycle lock held for over 60 minutes — possible deadlock",
      metric: lockHeldMetricFilter.metric({
        period: cdk.Duration.minutes(60),
        statistic: "Sum",
      }),
      threshold: 60,
      comparisonOperator:
        cloudwatch.ComparisonOperator.GREATER_THAN_THRESHOLD,
      evaluationPeriods: 1,
      treatMissingData: cloudwatch.TreatMissingData.NOT_BREACHING,
    });
  }
}
