# Full API Aggregation Integration Tests

**Status**: ✅ Working - Tests Database Persistence via API Aggregation

This directory contains integration tests that verify the complete flow from k8s client through kube-apiserver to our aggregated API server and finally to the SQLite database.

## Key Discovery: CRD vs APIService Conflict

⚠️ **CRITICAL**: When both a CRD and an APIService exist for the same API group/version, Kubernetes **always prefers the CRD**. This means requests go to etcd, not to the aggregated API server.

**Solution**: The `storage.fleet.cattle.io` CRD must NOT be installed in production. Only the APIService should exist.

## What's Tested

✅ **API Routing Verification**:
- Confirms requests route to aggregated API server (not etcd)
- Validates APIService is properly configured and available

✅ **Database Persistence**:
- Verifies data is actually written to SQLite database
- Uses `QueryAll()` method to inspect database contents directly
- Tests create, update, and delete operations reflect in database

✅ **Full CRUD Operations**:
- Create BundleDeployment through k8s client
- Get BundleDeployment through k8s client
- Update BundleDeployment through k8s client
- Delete BundleDeployment through k8s client
- List BundleDeployments through k8s client
- List with label selector through k8s client
- Update status subresource through k8s client

## Test Setup

The test suite:

1. **Starts envtest** with all Fleet CRDs
2. **Verifies storage.fleet.cattle.io CRD does not exist** (generator excludes it)
3. **Starts the aggregated API server** with SQLite backend
4. **Creates APIService resource** to register the aggregated API
5. **Shares the database instance** between test verification and API server
6. **Runs tests** to verify complete flow including database persistence

## Architecture

```
Test → k8sClient 
     ↓
envtest kube-apiserver 
     ↓  (sees APIService resource, no CRD)
APIService Routing
     ↓
Our Aggregated APIServer (HTTPS on localhost:random_port)
     ↓
Storage Layer (BundleDeploymentStorage)
     ↓
SQLite Database (/tmp/fleet-apiserver-full-test-*/bundledeployments.db)
```

## Production Implications

### What Needs to Change

1. **Remove the CRD**: Delete `bundledeployments.storage.fleet.cattle.io` from `charts/fleet-crd/templates/crds.yaml`

2. **Add APIService**: Create `charts/fleet/templates/apiservice.yaml`:
   ```yaml
   apiVersion: apiregistration.k8s.io/v1
   kind: APIService
   metadata:
     name: v1alpha1.storage.fleet.cattle.io
   spec:
     group: storage.fleet.cattle.io
     version: v1alpha1
     service:
       name: fleet-apiserver
       namespace: {{ .Release.Namespace }}
       port: 443
     groupPriorityMinimum: 1000
     versionPriority: 100
     caBundle: {{ .Values.apiserver.caBundle | b64enc }}
   ```

3. **Deploy API Server**: The `fleetapiserver` binary needs to run as a separate deployment with:
   - TLS certificates
   - Service for routing
   - Persistent storage for SQLite database

See [docs/apiserver-deployment.md](../../docs/apiserver-deployment.md) for complete production deployment guide.

## Running Tests

```bash
# From repository root
ginkgo run -v ./integrationtests/apiserver-aggregation/

# Run specific test
ginkgo run --focus="Database Persistence" ./integrationtests/apiserver-aggregation/
```

## Test Results

```
• should verify API aggregation is actually routing to our server
  - Validates APIService is Available
  - Creates BundleDeployment via k8s client
  - Confirms data exists in SQLite database (not etcd)

• should persist data to the SQLite database
  - Creates 3 BundleDeployments
  - Uses QueryAll() to verify all are in database
  - Validates metadata (resource_version, uid, generation, etc.)

• should reflect updates in the database
  - Creates BundleDeployment with generation=1
  - Updates spec
  - Verifies generation=2 and updated labels in database

• should remove deleted resources from the database
  - Creates BundleDeployment
  - Verifies it exists in database
  - Deletes it
  - Confirms it's removed from database
```

## New QueryAll Method

Added to `internal/cmd/apiserver/storage/database.go`:

```go
func (d *Database) QueryAll() ([]map[string]interface{}, error)
```

This method:
- Returns all bundledeployments from SQLite as a slice of maps
- Used in tests to verify database persistence
- Thread-safe with RLock/RUnlock
- Handles NULL values for optional fields

## Comparison with Storage Tests

| Aspect | Storage Tests | Full Integration Tests |
|--------|---------------|------------------------|
| **What's Tested** | Storage layer only | Complete API aggregation + DB |
| **Database Check** | No direct DB inspection | QueryAll() verifies DB contents |
| **Speed** | Fast (~4s) | Moderate (~15s with server startup) |
| **Setup** | Simple | Complex (HTTPS server, TLS, APIService) |
| **Tests** | Storage operations | End-to-end including DB persistence |
| **CRD Handling** | Uses test DB | Removes CRD to force APIService |
| **Use Case** | Development/TDD | Pre-deployment validation |

## Files

- `suite_test.go`: Test suite setup with HTTPS server and APIService registration
- `aggregation_test.go`: CRUD operations and database persistence tests
- `README.md`: This file

## Troubleshooting

**Problem**: Tests fail with "database should contain bundledeployments"
- **Cause**: Requests are going to etcd (CRD) instead of API server
- **Solution**: Ensure CRD is removed in test setup (done automatically)

**Problem**: APIService shows as unavailable
- **Cause**: API server not started or TLS issue
- **Solution**: Check server logs in test output

**Problem**: "unable to retrieve the complete list of server APIs"
- **Cause**: Missing scheme registration
- **Solution**: Ensure apiextensionsv1 is added to scheme

## See Also

- [docs/apiserver-deployment.md](../../docs/apiserver-deployment.md) - Production deployment guide
- [integrationtests/apiserver/](../apiserver/) - Storage layer tests (faster, for development)
- [internal/cmd/apiserver/storage/](../../internal/cmd/apiserver/storage/) - Storage implementation

## License

See main Fleet LICENSE file.

