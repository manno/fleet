# Migration Plan: Agentmanagement Controllers to Controller-Runtime

## Overview

This document outlines the migration strategy for moving Fleet's agentmanagement controllers from wrangler to controller-runtime. The agentmanagement subsystem is the last remaining component using wrangler, while all other Fleet controllers have been successfully migrated to controller-runtime.

## Background

Fleet's controller migration from wrangler to controller-runtime was a major undertaking. However, the agentmanagement controllers were deemed too complex to migrate initially due to their heavy reliance on wrangler-specific patterns:

- Complex event propagation via `relatedresource.Watch`
- Generating handlers with `apply.Apply` for declarative resource management
- Custom status handlers and cross-resource dependencies
- Label-based ownership tracking and automatic pruning

## Current Agentmanagement Controllers

The agentmanagement subsystem consists of 6 main controllers:

1. **bootstrap** - Handles cluster bootstrap resources
2. **cluster** - Manages cluster lifecycle and namespace cleanup  
3. **clusterregistration** - Agent-initiated and manager-initiated registration
4. **clusterregistrationtoken** - Registration token management
5. **config** - Global configuration management
6. **manageagent** - Deploys Fleet Agent as Bundle

## Key Migration Challenges

### 1. Behavioral Differences (Core Issue)

The primary challenge is not interface compatibility but fundamental behavioral differences:

**Apply's Declarative Set Management**
- **Wrangler**: Manages entire object sets with automatic pruning
- **Controller-Runtime**: Individual resource operations, manual lifecycle management

**Error Handling & Retries**
- **Wrangler**: Built-in retry logic with exponential backoff per object set
- **Controller-Runtime**: Relies on controller work queue retries

**Conflict Resolution**
- **Wrangler**: Three-way merge with label-based ownership tracking  
- **Controller-Runtime**: Server-side apply with field management

**Resource Discovery & Caching**
- **Wrangler**: Dynamic GVK→GVR mapping with custom informer integration
- **Controller-Runtime**: Manager-based caching with structured clients

### 2. Complex Event Propagation

Heavy use of `relatedresource.Watch` for cross-controller event propagation:
```go
relatedresource.WatchClusterScoped(ctx, "manage-agent-resolver", h.resolveNS, namespaces, clusters)
```

### 3. Apply.Apply Usage

Extensive use of wrangler's declarative apply mechanism:
```go
apply.WithSetID("fleet-bootstrap-data").
      WithNoDeleteGVK(fleetns.GVK()).
      ApplyObjects(resources...)
```

## Migration Strategy

### Phase 1: Behavioral Analysis & Strategy

#### Replace Apply Behavior with Server-Side Apply

**Current (Wrangler Apply):**
```go
apply := h.apply.WithSetID("fleet-manage-agent").WithCacheTypes(bundle)
return apply.ApplyObjects(resources...)
```

**Migrated (Controller-Runtime):**
```go
func (r *Reconciler) applyResourceSet(ctx context.Context, owner runtime.Object, resources []runtime.Object) error {
    // Apply all resources individually
    for _, resource := range resources {
        if err := r.applyResource(ctx, resource, owner); err != nil {
            return err
        }
    }
    
    // Manual pruning of orphaned resources
    return r.pruneOrphans(ctx, owner, resources)
}

func (r *Reconciler) applyResource(ctx context.Context, resource runtime.Object, owner runtime.Object) error {
    // Set controller owner reference
    if err := controllerutil.SetControllerReference(owner, resource, r.Scheme); err != nil {
        return err
    }
    
    // Use server-side apply
    return r.Patch(ctx, resource, client.Apply, client.ForceOwnership, client.FieldOwner("fleet-controller"))
}
```

#### Replace Event Propagation with Watches

**Current (Wrangler):**
```go
relatedresource.WatchClusterScoped(ctx, "name", resolver, target, source)
```

**Migrated (Controller-Runtime):**
```go
func (r *Reconciler) SetupWithManager(mgr ctrl.Manager) error {
    return ctrl.NewControllerManagedBy(mgr).
        For(&fleet.Cluster{}).
        Watches(&fleet.BundleDeployment{}, 
            handler.EnqueueRequestsFromMapFunc(r.mapBundleDeploymentToCluster)).
        Complete(r)
}

func (r *Reconciler) mapBundleDeploymentToCluster(ctx context.Context, obj client.Object) []ctrl.Request {
    // Convert wrangler resolver logic to return []ctrl.Request
    bd := obj.(*fleet.BundleDeployment)
    // Implementation depends on specific resolver logic
    return []ctrl.Request{{NamespacedName: types.NamespacedName{...}}}
}
```

### Phase 2: Controller-by-Controller Migration

#### Migration Order (Simplest to Most Complex)

1. **config** - Single resource type, direct processing
2. **manageagent** - Simple apply usage
3. **bootstrap** - Resource creation only
4. **cluster** - Status handling + watches  
5. **clusterregistrationtoken** - Token lifecycle management
6. **clusterregistration** - Most complex (generating handlers + complex apply)

#### Example: Config Controller Migration

**Before (Wrangler):**
```go
func Register(ctx context.Context, systemNamespace string, configMaps corecontrollers.ConfigMapController) error {
    configMaps.OnChange(ctx, "config", func(key string, obj *corev1.ConfigMap) (*corev1.ConfigMap, error) {
        return obj, config.Set(obj)
    })
}
```

**After (Controller-Runtime):**
```go
type ConfigReconciler struct {
    client.Client
    Log logr.Logger
}

func (r *ConfigReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
    var configMap corev1.ConfigMap
    if err := r.Get(ctx, req.NamespacedName, &configMap); err != nil {
        return ctrl.Result{}, client.IgnoreNotFound(err)
    }
    
    if err := config.Set(&configMap); err != nil {
        return ctrl.Result{}, err
    }
    
    return ctrl.Result{}, nil
}

func (r *ConfigReconciler) SetupWithManager(mgr ctrl.Manager) error {
    return ctrl.NewControllerManagedBy(mgr).
        For(&corev1.ConfigMap{}).
        WithOptions(controller.Options{MaxConcurrentReconciles: 1}).
        Complete(r)
}
```

#### Example: ClusterRegistration Controller Migration

**Before (Wrangler):**
```go
fleetcontrollers.RegisterClusterRegistrationGeneratingHandler(ctx,
    clusterRegistration,
    apply.WithCacheTypes(serviceAccount, secret, role, roleBinding),
    "",
    "cluster-registration",
    h.OnChange,
    &generic.GeneratingHandlerOptions{AllowClusterScoped: true})
```

**After (Controller-Runtime):**
```go
type ClusterRegistrationReconciler struct {
    client.Client
    Log logr.Logger
    SystemNamespace string
    SystemRegistrationNamespace string
}

func (r *ClusterRegistrationReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
    var clusterReg fleet.ClusterRegistration
    if err := r.Get(ctx, req.NamespacedName, &clusterReg); err != nil {
        return ctrl.Result{}, client.IgnoreNotFound(err)
    }
    
    // Replace generating handler with direct resource creation
    resources, newStatus, err := r.generateResources(ctx, &clusterReg)
    if err != nil {
        return ctrl.Result{}, err
    }
    
    // Apply resources individually with proper error handling
    if err := r.applyResourceSet(ctx, &clusterReg, resources); err != nil {
        if isTransientError(err) {
            return ctrl.Result{RequeueAfter: time.Second * 30}, nil
        }
        return ctrl.Result{}, err
    }
    
    // Update status
    if !reflect.DeepEqual(clusterReg.Status, newStatus) {
        clusterReg.Status = newStatus
        if err := r.Status().Update(ctx, &clusterReg); err != nil {
            return ctrl.Result{}, err
        }
    }
    
    return ctrl.Result{}, nil
}

func (r *ClusterRegistrationReconciler) SetupWithManager(mgr ctrl.Manager) error {
    // Add field indexers for efficient lookups
    if err := mgr.GetFieldIndexer().IndexField(ctx, &fleet.Cluster{}, "spec.clientID", 
        func(obj client.Object) []string {
            return []string{obj.(*fleet.Cluster).Spec.ClientID}
        }); err != nil {
        return err
    }
    
    return ctrl.NewControllerManagedBy(mgr).
        For(&fleet.ClusterRegistration{}).
        Owns(&corev1.Secret{}).
        Owns(&rbacv1.Role{}).
        Owns(&rbacv1.RoleBinding{}).
        Complete(r)
}
```

### Phase 3: Handle Apply Behavior Differences

#### Resource Set Management

```go
type ResourceTracker struct {
    client.Client
    ownerKey string
}

func (rt *ResourceTracker) ApplyResourceSet(ctx context.Context, owner runtime.Object, resources []runtime.Object) error {
    // Apply all resources with owner references
    for _, resource := range resources {
        if err := rt.applyResource(ctx, resource, owner); err != nil {
            return err
        }
    }
    
    // Prune orphaned resources manually
    return rt.pruneOrphans(ctx, owner, resources)
}

func (rt *ResourceTracker) applyResource(ctx context.Context, resource runtime.Object, owner runtime.Object) error {
    // Set controller owner reference for automatic cleanup
    if err := controllerutil.SetControllerReference(owner, resource, rt.Scheme); err != nil {
        return err
    }
    
    // Add tracking labels
    labels := map[string]string{
        "fleet.cattle.io/managed-by": rt.ownerKey,
    }
    resource.(metav1.Object).SetLabels(mergeMaps(resource.(metav1.Object).GetLabels(), labels))
    
    return rt.Patch(ctx, resource, client.Apply, client.ForceOwnership, client.FieldOwner("fleet-controller"))
}

func (rt *ResourceTracker) pruneOrphans(ctx context.Context, owner runtime.Object, currentResources []runtime.Object) error {
    // Find resources owned by this controller that aren't in current set
    selector := labels.SelectorFromSet(map[string]string{
        "fleet.cattle.io/managed-by": rt.ownerKey,
    })
    
    // Implementation depends on which GVKs each controller manages
    // List each managed resource type and delete orphans
    return nil
}
```

#### Error Handling Strategy

```go
func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
    // Standard error handling pattern
    var obj fleet.ClusterRegistration
    if err := r.Get(ctx, req.NamespacedName, &obj); err != nil {
        return ctrl.Result{}, client.IgnoreNotFound(err)
    }
    
    if err := r.processObject(ctx, &obj); err != nil {
        if isTransientError(err) {
            // Let controller work queue handle exponential backoff
            return ctrl.Result{RequeueAfter: time.Second * 30}, nil
        }
        return ctrl.Result{}, err
    }
    
    return ctrl.Result{}, nil
}

func isTransientError(err error) bool {
    return errors.IsServiceUnavailable(err) || 
           errors.IsTimeout(err) || 
           errors.IsServerTimeout(err) ||
           errors.IsConflict(err)
}
```

#### Performance Configuration

```go
func (r *Reconciler) SetupWithManager(mgr ctrl.Manager) error {
    return ctrl.NewControllerManagedBy(mgr).
        For(&fleet.ClusterRegistration{}).
        WithOptions(controller.Options{
            MaxConcurrentReconciles: 1, // Start conservative, tune based on testing
            RateLimiter: ratelimiter.NewItemExponentialFailureRateLimiter(
                time.Millisecond*100, // Base delay
                time.Second*30,       // Max delay
            ),
        }).
        Complete(r)
}
```

### Phase 4: Testing Strategy

#### Behavioral Compatibility Tests

```go
func TestClusterRegistrationBehavior(t *testing.T) {
    // Test that new controller creates same resources as old
    // Test error conditions and retry behavior  
    // Test resource cleanup and orphan pruning
    // Test performance under load
}

func TestApplyVsServerSideApply(t *testing.T) {
    // Ensure server-side apply produces equivalent results
    // Test conflict resolution
    // Test field ownership and management
}

func TestEventPropagation(t *testing.T) {
    // Test that controller-runtime watches produce same event flow
    // Test cross-controller dependencies
}
```

#### Integration Tests

```bash
# Existing tests should continue to pass
ginkgo integrationtests/controller/agentmanagement
ginkgo e2e/multi-cluster  # Agent registration flows
```

#### Performance Benchmarks

```go
func BenchmarkControllerThroughput(b *testing.B) {
    // Compare wrangler vs controller-runtime performance
    // Measure resource usage, latency, throughput
}
```

### Phase 5: Rollout Strategy

#### Feature Flag Support

```go
const ControllerRuntimeAgentManagement = "controller-runtime-agent-management"

func setupAgentManagement(mgr ctrl.Manager, cfg *config.Config) error {
    if cfg.FeatureGates.Enabled(ControllerRuntimeAgentManagement) {
        return setupControllerRuntimeAgentManagement(mgr, cfg)
    }
    return setupWranglerAgentManagement(mgr, cfg)
}
```

#### Gradual Migration

1. **Development Testing**: Feature flag disabled by default
2. **Alpha Testing**: Enable in dev/test environments
3. **Beta Testing**: Enable by default, allow opt-out
4. **GA**: Remove wrangler code

### Phase 6: Timeline & Effort

| Phase | Duration | Description |
|-------|----------|-------------|
| 1. Analysis & Setup | 1 week | Behavioral analysis, framework setup |
| 2. Simple Controllers | 2 weeks | config, manageagent, bootstrap |
| 3. Complex Controllers | 4 weeks | cluster, clusterregistrationtoken, clusterregistration |
| 4. Behavior Compatibility | 2 weeks | Resource tracking, error handling, performance |
| 5. Testing & Validation | 2 weeks | Integration tests, performance testing |
| 6. Documentation & Rollout | 1 week | Documentation, rollout planning |

**Total: 12 weeks**

## Benefits After Migration

1. **Consistency**: All Fleet controllers use the same framework
2. **Maintainability**: Standard Kubernetes controller patterns
3. **Performance**: Better resource usage and caching
4. **Testing**: Easier to unit test with standard mocking
5. **Community**: Aligns with broader Kubernetes ecosystem
6. **Debugging**: Better error messages and standard tooling
7. **Future-proofing**: No dependency on wrangler-specific patterns

## Risk Mitigation

1. **Feature Flag**: Allow rollback to wrangler implementation
2. **Gradual Rollout**: Test in dev environments first
3. **Monitoring**: Add metrics to compare behavior
4. **Testing**: Comprehensive integration and e2e tests
5. **Documentation**: Clear migration guide for contributors

## Success Criteria

- [ ] All agentmanagement controllers migrated to controller-runtime
- [ ] Existing integration tests pass without modification
- [ ] Performance benchmarks show comparable or better performance
- [ ] No behavioral regressions in multi-cluster agent management
- [ ] Wrangler dependency removed from agentmanagement package