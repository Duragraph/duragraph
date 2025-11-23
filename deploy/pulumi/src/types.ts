import * as pulumi from "@pulumi/pulumi";

/**
 * Common interface for DuraGraph deployment outputs
 */
export interface DuraGraphOutputs {
  apiUrl: pulumi.Output<string>;
  dashboardUrl: pulumi.Output<string>;
  databaseHost: pulumi.Output<string>;
  databasePort: pulumi.Output<number>;
  databaseName: pulumi.Output<string>;
  natsUrl: pulumi.Output<string>;
}

/**
 * Common interface for database configuration
 */
export interface DatabaseConfig {
  instanceSize: "small" | "medium" | "large";
  storageGB: number;
  backupEnabled: boolean;
  version: string;
}

/**
 * Common interface for container/compute configuration
 */
export interface ComputeConfig {
  cpu: number;
  memoryMB: number;
  minInstances: number;
  maxInstances: number;
}

/**
 * Provider-specific deployment interface
 */
export interface CloudProvider {
  deploy(): Promise<DuraGraphOutputs>;
}
