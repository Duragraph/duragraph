import * as pulumi from "@pulumi/pulumi";

const config = new pulumi.Config("duragraph");

export interface DuraGraphConfig {
  provider: "aws" | "gcp" | "azure" | "digitalocean";
  environment: string;
  region: string;
  dbInstanceSize: "small" | "medium" | "large";
  apiInstanceCount: number;
  apiMaxInstances: number;
  openaiApiKey?: string;
  anthropicApiKey?: string;
  jwtSecret?: string;
  authEnabled: boolean;
}

export const duraGraphConfig: DuraGraphConfig = {
  provider: config.get("provider") as any || "aws",
  environment: config.get("environment") || "dev",
  region: config.get("region") || "us-east-1",
  dbInstanceSize: config.get("dbInstanceSize") as any || "small",
  apiInstanceCount: config.getNumber("apiInstanceCount") || 1,
  apiMaxInstances: config.getNumber("apiMaxInstances") || 3,
  openaiApiKey: config.getSecret("openaiApiKey"),
  anthropicApiKey: config.getSecret("anthropicApiKey"),
  jwtSecret: config.getSecret("jwtSecret"),
  authEnabled: config.get("authEnabled") === "true",
};

export const stackName = pulumi.getStack();
export const projectName = pulumi.getProject();
