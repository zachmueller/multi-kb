import * as opensearchserverless from "aws-cdk-lib/aws-opensearchserverless";
import { Construct } from "constructs";

export interface SearchProps {
  readonly collectionName: string;
}

export class Search extends Construct {
  readonly encryptionPolicy: opensearchserverless.CfnSecurityPolicy;

  constructor(scope: Construct, id: string, props: SearchProps) {
    super(scope, id);

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
  }
}
