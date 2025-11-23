import * as pulumi from "@pulumi/pulumi";
import * as digitalocean from "@pulumi/digitalocean";
import { DuraGraphConfig } from "../config";
import { CloudProvider, DuraGraphOutputs } from "../types";

export class DigitalOceanProvider implements CloudProvider {
  private config: DuraGraphConfig;

  constructor(config: DuraGraphConfig) {
    this.config = config;
  }

  async deploy(): Promise<DuraGraphOutputs> {
    // Create PostgreSQL Database Cluster
    const database = new digitalocean.DatabaseCluster("duragraph-db", {
      engine: "pg",
      version: "15",
      size: this.getDBSize(this.config.dbInstanceSize),
      region: this.config.region,
      nodeCount: this.config.environment === "production" ? 2 : 1,
      tags: [`duragraph-${this.config.environment}`],
    });

    // Create App Platform spec
    const app = new digitalocean.App("duragraph", {
      spec: {
        name: `duragraph-${this.config.environment}`,
        region: this.config.region,

        databases: [{
          name: "db",
          engine: "PG",
          version: "15",
          production: this.config.environment === "production",
        }],

        services: [
          // API Service
          {
            name: "api",
            instanceCount: this.config.apiInstanceCount,
            instanceSizeSlug: "basic-xxs",
            httpPort: 8080,
            dockerfilePath: "deploy/docker/Dockerfile.server",
            github: {
              repo: "your-org/duragraph",
              branch: this.config.environment === "production" ? "main" : "develop",
              deployOnPush: true,
            },
            healthCheck: {
              httpPath: "/health",
              initialDelaySeconds: 10,
              periodSeconds: 30,
              timeoutSeconds: 5,
            },
            envs: [
              { key: "PORT", value: "8080" },
              { key: "HOST", value: "0.0.0.0" },
              { key: "DB_HOST", value: "${db.HOSTNAME}" },
              { key: "DB_PORT", value: "${db.PORT}" },
              { key: "DB_USER", value: "${db.USERNAME}" },
              { key: "DB_PASSWORD", value: "${db.PASSWORD}" },
              { key: "DB_NAME", value: "${db.DATABASE}" },
              { key: "DB_SSLMODE", value: "require" },
              { key: "NATS_URL", value: "nats://nats:4222" },
              { key: "AUTH_ENABLED", value: this.config.authEnabled.toString() },
              { key: "OPENAI_API_KEY", value: this.config.openaiApiKey || "", type: "SECRET" },
              { key: "ANTHROPIC_API_KEY", value: this.config.anthropicApiKey || "", type: "SECRET" },
              { key: "JWT_SECRET", value: this.config.jwtSecret || "", type: "SECRET" },
            ],
          },

          // NATS Service
          {
            name: "nats",
            instanceCount: 1,
            instanceSizeSlug: "basic-xxs",
            image: {
              registryType: "DOCKER_HUB",
              registry: "nats",
              repository: "nats",
              tag: "2.10-alpine",
            },
            internalPorts: [4222, 8222],
            runCommand: "-js -sd /data -m 8222",
            healthCheck: {
              httpPath: "/healthz",
              port: 8222,
            },
          },

          // Dashboard Service
          {
            name: "dashboard",
            instanceCount: 1,
            instanceSizeSlug: "basic-xxs",
            httpPort: 80,
            dockerfilePath: "deploy/docker/Dockerfile.dashboard",
            github: {
              repo: "your-org/duragraph",
              branch: this.config.environment === "production" ? "main" : "develop",
              deployOnPush: true,
            },
            routes: [{ path: "/" }],
          },
        ],
      },
    });

    return {
      apiUrl: app.liveUrl.apply(url => `https://${url}`),
      dashboardUrl: app.liveUrl.apply(url => `https://${url}/dashboard`),
      databaseHost: database.host,
      databasePort: database.port,
      databaseName: database.database,
      natsUrl: pulumi.output("nats://nats:4222"),
    };
  }

  private getDBSize(size: string): string {
    const sizes = {
      small: "db-s-1vcpu-1gb",
      medium: "db-s-2vcpu-4gb",
      large: "db-s-4vcpu-8gb",
    };
    return sizes[size] || sizes.small;
  }
}
