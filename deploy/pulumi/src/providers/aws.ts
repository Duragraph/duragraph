import * as pulumi from "@pulumi/pulumi";
import * as aws from "@pulumi/aws";
import * as awsx from "@pulumi/awsx";
import { DuraGraphConfig } from "../config";
import { CloudProvider, DuraGraphOutputs, DatabaseConfig, ComputeConfig } from "../types";

export class AWSProvider implements CloudProvider {
  private config: DuraGraphConfig;
  private vpc: awsx.ec2.Vpc;
  private dbSubnetGroup: aws.rds.SubnetGroup;

  constructor(config: DuraGraphConfig) {
    this.config = config;
  }

  async deploy(): Promise<DuraGraphOutputs> {
    // Create VPC
    this.vpc = new awsx.ec2.Vpc("duragraph-vpc", {
      numberOfAvailabilityZones: 2,
      enableDnsHostnames: true,
      enableDnsSupport: true,
      tags: {
        Name: `duragraph-${this.config.environment}`,
        Environment: this.config.environment,
      },
    });

    // Create DB Subnet Group
    this.dbSubnetGroup = new aws.rds.SubnetGroup("duragraph-db-subnet", {
      subnetIds: this.vpc.privateSubnetIds,
      tags: {
        Name: `duragraph-db-${this.config.environment}`,
      },
    });

    // Deploy Database
    const database = this.deployDatabase();

    // Deploy NATS
    const nats = this.deployNATS();

    // Deploy API
    const api = this.deployAPI(database, nats);

    // Deploy Dashboard
    const dashboard = this.deployDashboard(api);

    return {
      apiUrl: api.url,
      dashboardUrl: dashboard.url,
      databaseHost: database.endpoint.apply(e => e.split(":")[0]),
      databasePort: pulumi.output(5432),
      databaseName: database.dbName,
      natsUrl: nats.url,
    };
  }

  private deployDatabase() {
    const dbConfig = this.getDBConfig();

    // Security Group for RDS
    const dbSecurityGroup = new aws.ec2.SecurityGroup("duragraph-db-sg", {
      vpcId: this.vpc.vpcId,
      ingress: [
        {
          protocol: "tcp",
          fromPort: 5432,
          toPort: 5432,
          cidrBlocks: [this.vpc.vpc.cidrBlock],
        },
      ],
      egress: [
        {
          protocol: "-1",
          fromPort: 0,
          toPort: 0,
          cidrBlocks: ["0.0.0.0/0"],
        },
      ],
      tags: {
        Name: `duragraph-db-sg-${this.config.environment}`,
      },
    });

    // RDS PostgreSQL Instance
    const db = new aws.rds.Instance("duragraph-db", {
      engine: "postgres",
      engineVersion: dbConfig.version,
      instanceClass: this.getDBInstanceClass(dbConfig.instanceSize),
      allocatedStorage: dbConfig.storageGB,
      dbName: "duragraph",
      username: "duragraph_admin",
      password: pulumi.secret(this.generatePassword()),
      dbSubnetGroupName: this.dbSubnetGroup.name,
      vpcSecurityGroupIds: [dbSecurityGroup.id],
      skipFinalSnapshot: this.config.environment === "dev",
      backupRetentionPeriod: dbConfig.backupEnabled ? 7 : 0,
      multiAz: this.config.environment === "production",
      storageEncrypted: true,
      publiclyAccessible: false,
      tags: {
        Name: `duragraph-db-${this.config.environment}`,
        Environment: this.config.environment,
      },
    });

    return db;
  }

  private deployNATS() {
    // ECS Cluster
    const cluster = new aws.ecs.Cluster("duragraph-cluster", {
      tags: {
        Name: `duragraph-${this.config.environment}`,
      },
    });

    // Security Group for NATS
    const natsSecurityGroup = new aws.ec2.SecurityGroup("duragraph-nats-sg", {
      vpcId: this.vpc.vpcId,
      ingress: [
        {
          protocol: "tcp",
          fromPort: 4222,
          toPort: 4222,
          cidrBlocks: [this.vpc.vpc.cidrBlock],
        },
        {
          protocol: "tcp",
          fromPort: 8222,
          toPort: 8222,
          cidrBlocks: [this.vpc.vpc.cidrBlock],
        },
      ],
      egress: [
        {
          protocol: "-1",
          fromPort: 0,
          toPort: 0,
          cidrBlocks: ["0.0.0.0/0"],
        },
      ],
      tags: {
        Name: `duragraph-nats-sg-${this.config.environment}`,
      },
    });

    // NATS Fargate Service
    const natsService = new awsx.ecs.FargateService("duragraph-nats", {
      cluster: cluster.arn,
      networkConfiguration: {
        subnets: this.vpc.privateSubnetIds,
        securityGroups: [natsSecurityGroup.id],
      },
      taskDefinitionArgs: {
        container: {
          image: "nats:2.10-alpine",
          cpu: 512,
          memory: 1024,
          essential: true,
          portMappings: [
            { containerPort: 4222, protocol: "tcp" },
            { containerPort: 8222, protocol: "tcp" },
          ],
          command: ["-js", "-sd", "/data", "-m", "8222"],
        },
      },
      desiredCount: 1,
      tags: {
        Name: `duragraph-nats-${this.config.environment}`,
      },
    });

    return {
      service: natsService,
      url: pulumi.interpolate`nats://${natsService.taskDefinition.family}:4222`,
    };
  }

  private deployAPI(database: aws.rds.Instance, nats: any) {
    const computeConfig = this.getComputeConfig();

    // Security Group for API
    const apiSecurityGroup = new aws.ec2.SecurityGroup("duragraph-api-sg", {
      vpcId: this.vpc.vpcId,
      ingress: [
        {
          protocol: "tcp",
          fromPort: 8080,
          toPort: 8080,
          cidrBlocks: ["0.0.0.0/0"],
        },
      ],
      egress: [
        {
          protocol: "-1",
          fromPort: 0,
          toPort: 0,
          cidrBlocks: ["0.0.0.0/0"],
        },
      ],
      tags: {
        Name: `duragraph-api-sg-${this.config.environment}`,
      },
    });

    // Application Load Balancer
    const alb = new awsx.lb.ApplicationLoadBalancer("duragraph-alb", {
      subnetIds: this.vpc.publicSubnetIds,
      securityGroups: [apiSecurityGroup.id],
      tags: {
        Name: `duragraph-alb-${this.config.environment}`,
      },
    });

    // ECS Cluster (reuse or create)
    const cluster = new aws.ecs.Cluster("duragraph-cluster", {
      tags: {
        Name: `duragraph-${this.config.environment}`,
      },
    });

    // API Fargate Service
    const apiService = new awsx.ecs.FargateService("duragraph-api", {
      cluster: cluster.arn,
      networkConfiguration: {
        subnets: this.vpc.privateSubnetIds,
        securityGroups: [apiSecurityGroup.id],
      },
      taskDefinitionArgs: {
        container: {
          image: "duragraph/api:latest", // Replace with your ECR image
          cpu: computeConfig.cpu,
          memory: computeConfig.memoryMB,
          essential: true,
          portMappings: [{ containerPort: 8080, targetGroup: alb.defaultTargetGroup }],
          environment: [
            { name: "PORT", value: "8080" },
            { name: "HOST", value: "0.0.0.0" },
            { name: "DB_HOST", value: database.endpoint.apply(e => e.split(":")[0]) },
            { name: "DB_PORT", value: "5432" },
            { name: "DB_USER", value: database.username },
            { name: "DB_NAME", value: database.dbName },
            { name: "DB_SSLMODE", value: "require" },
            { name: "NATS_URL", value: nats.url },
            { name: "AUTH_ENABLED", value: this.config.authEnabled.toString() },
          ],
          secrets: [
            { name: "DB_PASSWORD", valueFrom: database.password },
            { name: "OPENAI_API_KEY", valueFrom: this.config.openaiApiKey || "" },
            { name: "ANTHROPIC_API_KEY", valueFrom: this.config.anthropicApiKey || "" },
            { name: "JWT_SECRET", valueFrom: this.config.jwtSecret || "" },
          ],
        },
      },
      desiredCount: computeConfig.minInstances,
      tags: {
        Name: `duragraph-api-${this.config.environment}`,
      },
    });

    return {
      service: apiService,
      url: pulumi.interpolate`http://${alb.loadBalancer.dnsName}`,
    };
  }

  private deployDashboard(api: any) {
    // S3 bucket for static hosting
    const bucket = new aws.s3.Bucket("duragraph-dashboard", {
      website: {
        indexDocument: "index.html",
        errorDocument: "index.html",
      },
      tags: {
        Name: `duragraph-dashboard-${this.config.environment}`,
      },
    });

    // CloudFront distribution
    const cdn = new aws.cloudfront.Distribution("duragraph-dashboard-cdn", {
      enabled: true,
      origins: [
        {
          originId: bucket.arn,
          domainName: bucket.websiteEndpoint,
          customOriginConfig: {
            originProtocolPolicy: "http-only",
            httpPort: 80,
            httpsPort: 443,
            originSslProtocols: ["TLSv1.2"],
          },
        },
      ],
      defaultCacheBehavior: {
        targetOriginId: bucket.arn,
        viewerProtocolPolicy: "redirect-to-https",
        allowedMethods: ["GET", "HEAD", "OPTIONS"],
        cachedMethods: ["GET", "HEAD"],
        forwardedValues: {
          queryString: false,
          cookies: { forward: "none" },
        },
      },
      restrictions: {
        geoRestriction: {
          restrictionType: "none",
        },
      },
      viewerCertificate: {
        cloudfrontDefaultCertificate: true,
      },
      tags: {
        Name: `duragraph-dashboard-${this.config.environment}`,
      },
    });

    return {
      bucket,
      cdn,
      url: pulumi.interpolate`https://${cdn.domainName}`,
    };
  }

  private getDBConfig(): DatabaseConfig {
    const configs = {
      small: { storageGB: 20 },
      medium: { storageGB: 100 },
      large: { storageGB: 500 },
    };

    return {
      instanceSize: this.config.dbInstanceSize,
      storageGB: configs[this.config.dbInstanceSize].storageGB,
      backupEnabled: this.config.environment !== "dev",
      version: "15.4",
    };
  }

  private getDBInstanceClass(size: string): string {
    const classes = {
      small: "db.t3.micro",
      medium: "db.t3.small",
      large: "db.t3.medium",
    };
    return classes[size] || classes.small;
  }

  private getComputeConfig(): ComputeConfig {
    return {
      cpu: 512,
      memoryMB: 1024,
      minInstances: this.config.apiInstanceCount,
      maxInstances: this.config.apiMaxInstances,
    };
  }

  private generatePassword(): string {
    return Math.random().toString(36).slice(-16) + Math.random().toString(36).slice(-16);
  }
}
