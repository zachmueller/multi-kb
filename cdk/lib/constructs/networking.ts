import * as cdk from "aws-cdk-lib";
import * as ec2 from "aws-cdk-lib/aws-ec2";
import * as opensearchserverless from "aws-cdk-lib/aws-opensearchserverless";
import { Construct } from "constructs";

export interface NetworkingProps {
  readonly vpcId?: string;
  readonly collectionName: string;
}

export class Networking extends Construct {
  readonly vpc: ec2.IVpc;
  readonly subnet: ec2.ISubnet;
  readonly availabilityZone: string;
  readonly ec2SecurityGroup: ec2.SecurityGroup;
  readonly endpointSecurityGroup: ec2.SecurityGroup;
  readonly aossVpcEndpointId: string;

  constructor(scope: Construct, id: string, props: NetworkingProps) {
    super(scope, id);

    // VPC — import existing or create new single-AZ private VPC
    if (props.vpcId) {
      this.vpc = ec2.Vpc.fromLookup(this, "Vpc", { vpcId: props.vpcId });
      this.availabilityZone = cdk.Stack.of(this).availabilityZones[0];
    } else {
      const vpc = new ec2.Vpc(this, "Vpc", {
        ipAddresses: ec2.IpAddresses.cidr("10.0.0.0/16"),
        maxAzs: 1,
        natGateways: 0,
        subnetConfiguration: [
          {
            name: "private",
            subnetType: ec2.SubnetType.PRIVATE_ISOLATED,
            cidrMask: 24,
          },
        ],
      });
      this.vpc = vpc;
      this.availabilityZone = vpc.availabilityZones[0];
    }

    // Pin to the first private isolated subnet in the single AZ
    this.subnet = this.vpc.isolatedSubnets[0];

    // Security groups — EC2 and VPC endpoints
    // endpoint SG created first so EC2 SG can reference it
    this.endpointSecurityGroup = new ec2.SecurityGroup(
      this,
      "EndpointSecurityGroup",
      {
        vpc: this.vpc,
        description: "VPC endpoint security group — HTTPS from EC2 SG only",
        allowAllOutbound: false,
      },
    );

    this.ec2SecurityGroup = new ec2.SecurityGroup(
      this,
      "Ec2SecurityGroup",
      {
        vpc: this.vpc,
        description: "EC2 instance security group — HTTPS to endpoints only",
        allowAllOutbound: false,
      },
    );

    // Use separate L1 resources to avoid cyclic inline SG references
    new ec2.CfnSecurityGroupEgress(this, "Ec2ToEndpointEgress", {
      groupId: this.ec2SecurityGroup.securityGroupId,
      ipProtocol: "tcp",
      fromPort: 443,
      toPort: 443,
      destinationSecurityGroupId: this.endpointSecurityGroup.securityGroupId,
      description: "HTTPS to VPC endpoints",
    });

    new ec2.CfnSecurityGroupIngress(this, "EndpointFromEc2Ingress", {
      groupId: this.endpointSecurityGroup.securityGroupId,
      ipProtocol: "tcp",
      fromPort: 443,
      toPort: 443,
      sourceSecurityGroupId: this.ec2SecurityGroup.securityGroupId,
      description: "HTTPS from EC2 instances",
    });

    // S3 gateway endpoint (free — no hourly cost)
    this.vpc.addGatewayEndpoint("S3GatewayEndpoint", {
      service: ec2.GatewayVpcEndpointAwsService.S3,
      subnets: [{ subnets: [this.subnet] }],
    });

    // 8 standard interface endpoints — `open: false` prevents CDK auto-adding 0.0.0.0/0
    const interfaceEndpointServices: Array<{
      id: string;
      service: ec2.InterfaceVpcEndpointService;
    }> = [
      {
        id: "SqsEndpoint",
        service: new ec2.InterfaceVpcEndpointService(
          `com.amazonaws.${cdk.Stack.of(this).region}.sqs`,
        ),
      },
      {
        id: "CodeCommitEndpoint",
        service: new ec2.InterfaceVpcEndpointService(
          `com.amazonaws.${cdk.Stack.of(this).region}.git-codecommit`,
        ),
      },
      {
        id: "BedrockRuntimeEndpoint",
        service: new ec2.InterfaceVpcEndpointService(
          `com.amazonaws.${cdk.Stack.of(this).region}.bedrock-runtime`,
        ),
      },
      {
        id: "BedrockAgentEndpoint",
        service: new ec2.InterfaceVpcEndpointService(
          `com.amazonaws.${cdk.Stack.of(this).region}.bedrock-agent`,
        ),
      },
      {
        id: "SsmEndpoint",
        service: new ec2.InterfaceVpcEndpointService(
          `com.amazonaws.${cdk.Stack.of(this).region}.ssm`,
        ),
      },
      {
        id: "SsmMessagesEndpoint",
        service: new ec2.InterfaceVpcEndpointService(
          `com.amazonaws.${cdk.Stack.of(this).region}.ssmmessages`,
        ),
      },
      {
        id: "Ec2MessagesEndpoint",
        service: new ec2.InterfaceVpcEndpointService(
          `com.amazonaws.${cdk.Stack.of(this).region}.ec2messages`,
        ),
      },
      {
        id: "CloudWatchLogsEndpoint",
        service: new ec2.InterfaceVpcEndpointService(
          `com.amazonaws.${cdk.Stack.of(this).region}.logs`,
        ),
      },
    ];

    for (const { id, service } of interfaceEndpointServices) {
      new ec2.InterfaceVpcEndpoint(this, id, {
        vpc: this.vpc,
        service,
        subnets: { subnets: [this.subnet] },
        securityGroups: [this.endpointSecurityGroup],
        open: false, // CRITICAL: prevents CDK adding permissive 0.0.0.0/0 ingress
        privateDnsEnabled: true,
      });
    }

    // AOSS VPC endpoint — must use CfnVpcEndpoint (L1), not InterfaceVpcEndpoint
    // Name must match ^[a-z][a-z0-9-]{2,31}$
    const aossEndpointName = `${props.collectionName.toLowerCase().replace(/[^a-z0-9-]/g, "-").substring(0, 28)}-ep`;
    const aossEndpoint = new opensearchserverless.CfnVpcEndpoint(
      this,
      "AossVpcEndpoint",
      {
        name: aossEndpointName,
        vpcId: this.vpc.vpcId,
        subnetIds: [this.subnet.subnetId],
        securityGroupIds: [this.endpointSecurityGroup.securityGroupId],
      },
    );

    // attrId is the VPC endpoint ID used in the network policy SourceVPCEs field
    this.aossVpcEndpointId = aossEndpoint.attrId;
  }
}
