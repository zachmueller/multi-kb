import type { CloudFormationCustomResourceEvent, CloudFormationCustomResourceResponse } from "aws-lambda";

export const handler = async (
  event: CloudFormationCustomResourceEvent,
): Promise<CloudFormationCustomResourceResponse> => {
  console.log("Create-index custom resource invoked", JSON.stringify(event));
  // Implementation in SRC-001.
  return {
    Status: "SUCCESS",
    PhysicalResourceId: event.LogicalResourceId,
    StackId: event.StackId,
    RequestId: event.RequestId,
    LogicalResourceId: event.LogicalResourceId,
  };
};
