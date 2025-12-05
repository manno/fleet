# Full API Aggregation Integration Tests

**Status**: 🚧 Work in Progress - OpenAPI Configuration Required

This directory contains an implementation for full API aggregation integration testing, which would test the complete flow from k8s client through envtest apiserver to our aggregated apiserver.

## What Was Implemented

✅ **Test Suite Setup** (`suite_test.go`):
- Start envtest environment
- Find available port for aggregated apiserver
- Generate TLS certificates (self-signed for testing)
- Start aggregated apiserver as HTTP server
- Create APIService, Service, and Endpoints resources
- Wait for APIService to become Available

✅ **Test Cases** (`aggregation_test.go`):
- Create BundleDeployment through k8s client
- Get BundleDeployment through k8s client
- Update BundleDeployment through k8s client
- Delete BundleDeployment through k8s client
- List BundleDeployments through k8s client
- List with label selector through k8s client
- Update status subresource through k8s client

## Current Blocker

❌ **OpenAPI Configuration**: The aggregated apiserver requires OpenAPI definitions for `storage.fleet.cattle.io/v1alpha1`, which haven't been generated yet.

Error: `cannot find model definition for storage.fleet.cattle.io/v1alpha1.BundleDeployment`

## To Complete This Test

### Option 1: Generate OpenAPI Definitions (Recommended)

1. Add OpenAPI generation markers to `pkg/apis/storage.fleet.cattle.io/v1alpha1/`:
   ```go
   // +k8s:openapi-gen=true
   package v1alpha1
   ```

2. Run code generation:
   ```bash
   go generate ./...
   ```

3. Update `pkg/generated/openapi/` to include storage.fleet.cattle.io types

4. Use the generated OpenAPI in the test

### Option 2: Simplify Test (Quick Fix)

Modify the server config to not require OpenAPI (may lose some functionality):
```go
serverConfig.OpenAPIConfig = &genericapiserver.OpenAPIConfig{
    Info: spec.Info{...},
    DefaultResponse: &spec.Response{...},
    GetDefinitions: func(ref common.ReferenceCallback) map[string]common.OpenAPIDefinition {
        return map[string]common.OpenAPIDefinition{}
    },
}
```

## What This Test Would Validate

Once working, this test would verify:

1. ✅ **API Aggregation Routing**: Requests to `storage.fleet.cattle.io` route to our server
2. ✅ **Authentication/Authorization**: Auth flow through main apiserver to aggregated server
3. ✅ **API Discovery**: The API group is discoverable via `/apis`
4. ✅ **kubectl Compatibility**: Works like any other Kubernetes API
5. ✅ **Full CRUD**: All operations work through the aggregated path
6. ✅ **Status Subresource**: Status updates work correctly

## Architecture

```
Test → k8sClient 
     ↓
envtest kube-apiserver 
     ↓  (sees APIService resource)
APIService Routing
     ↓
Our Aggregated APIServer (HTTP on localhost:random_port)
     ↓
Storage Layer (BundleDeploymentStorage)
     ↓
SQLite Database
```

## Comparison with Storage Tests

| Aspect | Storage Tests | Full Integration Tests |
|--------|---------------|------------------------|
| **What's Tested** | Storage layer only | Complete API aggregation |
| **Speed** | Fast (~4s) | Slower (~20s+ with server startup) |
| **Setup** | Simple | Complex (HTTP server, TLS, APIService) |
| **Tests** | Storage operations | End-to-end API flow |
| **Use Case** | Development/TDD | Pre-deployment validation |

## Running Tests (When Fixed)

```bash
# From repository root
ginkgo run ./integrationtests/apiserver-aggregation/

# With verbose output
ginkgo run -v ./integrationtests/apiserver-aggregation/
```

## Files

- `suite_test.go`: Test suite setup with HTTP server lifecycle
- `aggregation_test.go`: Test cases for CRUD operations
- `README.md`: This file

## Next Steps

1. Generate OpenAPI definitions for `storage.fleet.cattle.io/v1alpha1`
2. Update test to use generated OpenAPI
3. Run tests to validate full API aggregation
4. Add additional test cases (watch, patch, etc.)
5. Add performance benchmarks

## Alternative: Use Storage Tests

For now, use the storage layer tests in `../apiserver/` which work perfectly and test the same functionality minus the HTTP/aggregation layer:

```bash
ginkgo run ./integrationtests/apiserver/
```

These tests are sufficient for validating storage logic and are much faster for development.

##License

See main Fleet LICENSE file.
