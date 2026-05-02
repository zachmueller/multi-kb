#!/usr/bin/env node
import * as cdk from "aws-cdk-lib";
import { MultiKbStack, resolveProps } from "../lib/multi-kb-stack";

const app = new cdk.App();
const props = resolveProps(app);

new MultiKbStack(app, "MultiKbStack", props);
