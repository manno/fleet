# API Server Integration Tests

This directory contains integration tests for the Fleet API aggregation server.

## Test Types

### 1. Storage Layer Tests (`apiserver_test.go`)
**What it tests**: Direct storage layer operations
- ✅ Fast execution (~4 seconds)
- ✅ Simple setup
- ✅ Good for TDD and rapid iteration
- ⚠️  Does NOT test API aggregation

**Flow**: `Test → Storage Layer (Direct)`

### 2. Full Integration Tests (`full_integration_test.go`)
**What it WOULD test**: Complete API aggregation flow
- ⚠️  Currently SKIPPED (marked with `Skip()`)
- 🔨 Complex setup required
- 🐌 Slower execution
- ✅ Tests real API aggregation

**Flow**: `Test → k8sClient → envtest kube-apiserver → APIService → Our Aggregated APIServer (HTTP) → Storage`

## Why Two Test Types?

### Current Tests (Storage Layer Only)
```go
// Direct storage access - bypasses API aggregation
store.Create(ctx, bundleDeployment, ...)
```

**Pros**:
- Fast feedback for storage logic
- No TLS/port management needed
- Simple to debug

**Cons**:
- Doesn't test API aggregation routing
- Doesn't test authentication/authorization flow  
- Doesn't validate kubectl compatibility

### Full Integration Tests (Not Yet Implemented)
```go
// Goes through full stack
k8sClient.Create(ctx, bundleDeployment)
```

**Pros**:
- Tests real API aggregation
- Validates auth/authz flow
- Tests API discovery (`/apis` endpoint)
- Ensures kubectl compatibility

**Cons**:
- Complex setup (TLS certs, ports, APIService, Service/Endpoints)
- Slower execution
- More failure points

## Implementing Full Integration Tests

To implement full API aggregation testing, you need to:

1. **Start HTTP Server**: Launch apiserver on real port with TLS
2. **Create APIService**: Register with envtest kube-apiserver
3. **Create Service/Endpoints**: Point to localhost apiserver
4. **Wait for Available**: APIService status must be Available
5. **Use k8sClient**: Now requests route through aggregation

See `full_integration_test.go` for detailed comments and skeleton code.

### Key Challenges:
- **TLS Certificates**: Generate certs envtest will trust
- **Port Management**: Find free ports, avoid conflicts
- **Timing**: Wait for server startup and registration
- **Cleanup**: Properly shutdown HTTP server

## Test Coverage

### BundleDeployment CRUD Operations
- ✅ Create a BundleDeployment
- ✅ Get a BundleDeployment  
- ✅ Update a BundleDeployment
- ✅ Delete a BundleDeployment
- ✅ List BundleDeployments
- ✅ List BundleDeployments with label selector

### Status Operations
- ✅ Update status subresource

### API Group Verification
- ✅ Verify `storage.fleet.cattle.io` API group
- ✅ Verify correct GroupVersionKind

## Running the Tests

From the repository root:
```bash
ginkgo run ./integrationtests/apiserver/
```

Or with verbose output:
```bash
ginkgo run -v ./integrationtests/apiserver/
```

**Note**: Full integration tests are currently skipped. To see them:
```bash
ginkgo run -v ./integrationtests/apiserver/ --focus="Full API Aggregation"
```

## Architecture

The tests use:
- **envtest**: Provides a real Kubernetes API server for testing
- **SQLite**: In-memory database for BundleDeployment storage
- **Ginkgo/Gomega**: BDD-style testing framework

Current tests validate the storage layer without starting the full apiserver HTTP server, focusing on database and storage backend functionality.

## Next Steps

To add full API aggregation testing:

1. Extract apiserver `run()` function to be testable
2. Implement TLS certificate generation for tests
3. Implement port management and server lifecycle
4. Create APIService, Service, and Endpoints resources
5. Un-skip full integration tests
6. Add test cases for authentication/authorization

This would provide end-to-end validation that API aggregation works correctly.

