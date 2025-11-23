import * as pulumi from "@pulumi/pulumi";
import { duraGraphConfig } from "./src/config";
import { AWSProvider } from "./src/providers/aws";
import { DigitalOceanProvider } from "./src/providers/digitalocean";
import { CloudProvider, DuraGraphOutputs } from "./src/types";

/**
 * Main entry point for DuraGraph Pulumi deployment
 * Supports multi-cloud deployment based on configuration
 */

async function main() {
  let provider: CloudProvider;

  // Select provider based on configuration
  switch (duraGraphConfig.provider) {
    case "aws":
      pulumi.log.info("Deploying to AWS");
      provider = new AWSProvider(duraGraphConfig);
      break;

    case "digitalocean":
      pulumi.log.info("Deploying to DigitalOcean");
      provider = new DigitalOceanProvider(duraGraphConfig);
      break;

    case "gcp":
      throw new Error("GCP provider not yet implemented. Use aws or digitalocean.");

    case "azure":
      throw new Error("Azure provider not yet implemented. Use aws or digitalocean.");

    default:
      throw new Error(`Unknown provider: ${duraGraphConfig.provider}. Supported: aws, digitalocean`);
  }

  // Deploy infrastructure
  const outputs = await provider.deploy();

  // Export outputs
  return {
    provider: duraGraphConfig.provider,
    environment: duraGraphConfig.environment,
    region: duraGraphConfig.region,
    apiUrl: outputs.apiUrl,
    dashboardUrl: outputs.dashboardUrl,
    databaseHost: outputs.databaseHost,
    databasePort: outputs.databasePort,
    databaseName: outputs.databaseName,
    natsUrl: outputs.natsUrl,
  };
}

// Execute deployment
export = main();
