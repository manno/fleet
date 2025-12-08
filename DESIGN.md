# Fleet - Design and Architecture

## Overview

**Fleet** is a GitOps and HelmOps solution built for managing Kubernetes deployments at scale. It excels at managing deployments across multiple clusters, handling hundreds of clusters, thousands of deployments, and multiple teams within a single organization.

Fleet watches Git repositories and automatically deploys applications to target clusters using a unified approach where all resources (raw Kubernetes YAML, Helm charts, or Kustomize configurations) are dynamically converted into Helm charts for consistent deployment, management, and lifecycle control.

## Project Architecture

![Fleet Architecture](./docs/arch.png)

### Core Components

Fleet operates through three primary components that work together to provide GitOps at scale:

#### 1. **Fleet Controller** (`fleetcontroller`)
The central management component running in the Fleet management cluster.

**Responsibilities:**
- Watches GitRepo custom resources to monitor Git repositories
- Scans Git repositories and generates Bundles from discovered resources
- Manages the targeting and distribution of Bundles to downstream clusters
- Reconciles the desired state across all managed clusters
- Coordinates cluster registration and authentication
- Handles image scanning and automated updates
- Manages scheduled deployments via cron expressions

**Key Reconcilers:**
- `GitRepo` controller: Scans repos, creates/updates Bundles
- `Bundle` controller: Determines target clusters and creates BundleDeployments
- `Cluster` controller: Manages cluster lifecycle and health
- `ClusterGroup` controller: Groups clusters for targeted deployments
- `ImageScan` controller: Watches container registries for new images
- `Schedule` controller: Handles time-based deployment triggers

#### 2. **Fleet Agent** (`fleetagent`)
Lightweight agent deployed in every downstream cluster (including the local cluster).

**Responsibilities:**
- Deploys and manages BundleDeployments in the local cluster
- Reports status back to the Fleet controller
- Uses Helm as the deployment engine for all workloads
- Monitors deployed resources and updates status conditions
- Handles drift detection and correction
- Manages cleanup on bundle deletion

**Key Controllers:**
- `BundleDeployment` controller: Deploys resources using Helm
- Status reporter: Updates deployment health and readiness

#### 3. **Fleet CLI** (`fleetcli`)
Command-line tool for developers and operators.

**Capabilities:**
- Apply GitRepo resources and manifests
- Test bundle targeting and matching locally
- Validate fleet.yaml configurations
- Debug deployments and target matching
- Clone and inspect git repositories

### Custom Resource Definitions (CRDs)

Fleet extends Kubernetes with several custom resources that form the foundation of its GitOps model:

#### **GitRepo**
Represents a Git repository to watch and deploy.
- Specifies repo URL, branch/revision, paths, and authentication
- Supports filtering via paths and glob patterns
- Handles Helm repository credentials
- Defines target clusters via selectors and labels
- Configures deployment options (namespace, Helm values, service account)

#### **Bundle**
A collection of resources to deploy, generated from a GitRepo.
- Represents one deployable unit (app/chart/manifest set)
- Contains rendered Helm chart and metadata
- Defines rollout strategy (max unavailable, auto-partitions)
- Includes diff configuration for drift detection
- Can be stored in OCI registries for large deployments

#### **BundleDeployment**
Represents a Bundle deployed to a specific cluster.
- One per Bundle per target cluster
- Contains actual deployment status and conditions
- Reports resource readiness and errors
- Tracks Helm release information
- Monitors for modifications/drift

#### **Cluster**
Represents a downstream Kubernetes cluster managed by Fleet.
- Registers cluster with labels for targeting
- Reports agent health and version
- Defines resource constraints and limits
- Contains kubeconfig for cluster access
- Tracks bundle deployment statistics

#### **ClusterGroup**
Logical grouping of clusters for simplified targeting.
- Uses label selectors to include clusters
- Enables deploying to groups rather than individual clusters
- Supports dynamic membership based on labels

#### **ClusterRegistration & ClusterRegistrationToken**
Handles secure cluster registration flow.
- Generates registration tokens for new clusters
- Manages authentication credentials
- Creates cluster-specific namespaces

#### **ImageScan**
Enables automated image updates from container registries.
- Watches OCI registries for new image tags
- Filters tags using policy expressions and SemVer
- Triggers GitRepo updates when new images are available
- Supports private registries with authentication

### Data Flow

```
┌─────────────────────────────────────────────────────────────────────────┐
│                         Fleet Management Cluster                        │
│                                                                         │
│  ┌──────────┐     ┌──────────────┐     ┌────────┐                     │
│  │ GitRepo  │────▶│    Fleet     │────▶│ Bundle │                     │
│  │ Resource │     │ Controller   │     │        │                     │
│  └──────────┘     └──────┬───────┘     └────┬───┘                     │
│                           │                  │                          │
│                           │                  ▼                          │
│                           │        ┌──────────────────┐                │
│                           │        │ BundleDeployment │                │
│                           │        │  (per cluster)   │                │
│                           │        └────────┬─────────┘                │
│                           │                 │                           │
└───────────────────────────┼─────────────────┼───────────────────────────┘
                            │                 │
                            │                 │ (synced to cluster)
                            │                 │
┌───────────────────────────┼─────────────────┼───────────────────────────┐
│                  Downstream Cluster         │                           │
│                                             ▼                           │
│                           ┌──────────────────────────┐                 │
│                           │     Fleet Agent          │                 │
│                           │  (watches & deploys)     │                 │
│                           └──────────┬───────────────┘                 │
│                                      │                                  │
│                                      ▼                                  │
│                           ┌─────────────────────┐                      │
│                           │  Helm Release       │                      │
│                           │  (actual workload)  │                      │
│                           └─────────────────────┘                      │
│                                                                         │
└─────────────────────────────────────────────────────────────────────────┘
```

**Flow Explanation:**

1. **Git Repository Monitoring**: User creates a GitRepo resource pointing to a Git repository
2. **Bundle Generation**: Fleet Controller scans the repo and creates Bundle resources
3. **Target Matching**: Controller evaluates cluster selectors and creates BundleDeployment for each matching cluster
4. **Distribution**: BundleDeployments are synced to downstream clusters via Kubernetes API
5. **Deployment**: Fleet Agent watches for BundleDeployments and deploys them using Helm
6. **Status Reporting**: Agent reports status back through BundleDeployment status
7. **Aggregation**: Controller aggregates status from all clusters to Bundle and GitRepo

## Key Design Principles

### 1. **Everything is Helm**
All resources—whether raw YAML, Kustomize, or native Helm charts—are converted to Helm charts before deployment. This provides:
- Unified deployment mechanism across all resource types
- Consistent lifecycle management (install, upgrade, rollback, delete)
- Built-in versioning and release tracking
- Dependency management and hooks support

### 2. **Declarative GitOps**
The entire system state is declared in Git:
- GitRepo resources point to source repositories
- Fleet automatically synchronizes cluster state with Git
- No manual kubectl apply needed
- Full audit trail via Git history

### 3. **Scale-First Architecture**
Designed from the ground up for large-scale scenarios:
- Efficient resource storage via OCI registry support
- Sharding support for controller workload distribution
- Optimized status aggregation and reporting
- Minimal agent footprint per cluster

### 4. **Flexible Targeting**
Sophisticated cluster targeting system:
- Label-based selectors (cluster labels, group labels)
- Support for cluster groups
- Advanced matching expressions
- Dynamic target updates as clusters join/leave

### 5. **Multi-Tenancy**
Built-in support for organizational isolation:
- Namespace-based workspace separation
- RBAC integration for access control
- Per-cluster and per-workspace resource limits
- Isolated GitRepo scanning and deployment

## Feature Highlights

### GitOps Capabilities
- **Multi-Repository Support**: Watch multiple Git repos simultaneously
- **Path-Based Filtering**: Deploy specific directories or use glob patterns
- **Branch/Tag/Commit Targeting**: Pin to specific revisions or follow branches
- **Private Repository Support**: SSH keys and basic auth for private repos
- **Automatic Polling**: Configurable polling intervals or webhook-driven updates

### Deployment Features
- **Rollout Control**: Configurable max unavailable and auto-partitioning
- **Dependency Management**: Deploy bundles in specific order
- **Service Account Customization**: Per-bundle service accounts
- **Namespace Management**: Auto-creation of target namespaces
- **Helm Value Overrides**: Per-cluster or per-group customization
- **Kustomize Support**: Native Kustomize overlay processing

### Operational Excellence
- **Drift Detection**: Monitors for manual changes to deployed resources
- **Status Aggregation**: Real-time visibility into deployment state
- **Resource Limits**: Prevent runaway deployments with configurable limits
- **Garbage Collection**: Automatic cleanup of removed resources
- **Health Monitoring**: Built-in readiness and liveness checks

### Advanced Features
- **Image Scanning**: Automated container image updates from registries
- **Scheduled Deployments**: Cron-based deployment triggers
- **OCI Storage**: Store large bundles in OCI registries instead of etcd
- **Sharding**: Distribute controller load across multiple instances
- **Prometheus Metrics**: Comprehensive observability

## What Makes Fleet Special

### 1. **True Multi-Cluster Management**
Unlike single-cluster GitOps tools, Fleet is purpose-built for fleet-wide operations:
- Manage 100+ clusters from a single control plane
- Deploy the same app to thousands of clusters with one command
- Different configurations per environment/region/team

### 2. **Helm as a Universal Engine**
By converting everything to Helm, Fleet provides:
- Consistent behavior regardless of source format
- Battle-tested deployment mechanism
- Native rollback capabilities
- Comprehensive release history

### 3. **Cluster Registration Simplicity**
Easy onboarding of new clusters:
- Token-based registration
- Automatic namespace creation
- Self-contained agent deployment
- Minimal prerequisites

### 4. **Sophisticated Targeting**
Cluster selection goes beyond simple labels:
- Boolean expressions for complex logic
- Cluster groups for abstraction
- Dynamic targeting as fleet evolves
- Preview targeting before deployment

### 5. **Performance at Scale**
Optimized for real-world large deployments:
- Efficient resource storage (OCI support)
- Incremental status updates
- Controller sharding for horizontal scaling
- Minimal API server load

### 6. **Rancher Integration**
Seamless integration with Rancher for enhanced UX:
- Visual fleet management dashboard
- RBAC integration with Rancher auth
- Cluster provisioning integration
- Multi-tenant workspace mapping

### 7. **Developer-Friendly**
Designed for operational simplicity:
- Simple `fleet.yaml` for deployment customization
- CLI for local testing and debugging
- Clear status reporting and error messages
- Extensive documentation and examples

## Technology Stack

**Core Technologies:**
- **Language**: Go 1.24+
- **Framework**: Kubernetes controller-runtime, Wrangler
- **Deployment Engine**: Helm 3
- **Git Libraries**: go-git for repository operations
- **OCI Support**: go-containerregistry for OCI operations

**Key Dependencies:**
- Kubernetes API Machinery (v0.34+)
- Helm SDK (v3.19+)
- Kustomize API (v0.21+)
- Prometheus client for metrics
- Ginkgo/Gomega for testing

**Infrastructure:**
- Kubernetes 1.32+ clusters
- Optional OCI registry for large deployments
- Git repositories (GitHub, GitLab, Bitbucket, etc.)
- Optional image registries for ImageScan

## Development and Testing

Fleet maintains high code quality through:
- **Unit Tests**: Extensive coverage of core logic
- **Integration Tests**: Controller behavior testing with mocks
- **E2E Tests**: Full cluster deployment scenarios using testcontainers
- **Performance Benchmarks**: Scalability testing suite
- **CI/CD**: Automated testing via GitHub Actions
- **Linting**: golangci-lint for code quality

**Test Philosophy:**
- Lightweight tests preferred over heavy E2E
- Randomized namespaces to prevent conflicts
- Gomega's Eventually for async assertions
- Proper cleanup in BeforeEach/AfterEach

## Use Cases

Fleet excels in these scenarios:

1. **Multi-Region Deployments**: Deploy applications across geographic regions
2. **Edge Computing**: Manage hundreds of edge clusters from central location
3. **Multi-Tenant SaaS**: Isolated environments for different customers
4. **Development/Staging/Production**: Promote through environments with GitOps
5. **Hybrid Cloud**: Consistent deployment across cloud providers
6. **Disaster Recovery**: Rapid replication to backup clusters

## Summary

Fleet represents a mature, production-ready GitOps solution that scales from single-cluster scenarios to managing thousands of clusters. Its unique approach of converting all resources to Helm charts provides consistency and reliability, while its sophisticated targeting and status aggregation make it ideal for organizations with complex, multi-cluster Kubernetes deployments.

The combination of declarative GitOps, flexible targeting, and Helm-based deployment makes Fleet an essential tool for platform teams building and managing Kubernetes infrastructure at scale.

---

**For More Information:**
- [User Documentation](https://fleet.rancher.io/)
- [Developer Guide](./DEVELOPING.md)
- [Contributing Guide](./CONTRIBUTING.md)
- [API Reference](./pkg/apis/fleet.cattle.io/v1alpha1/)
