# Agentmanagement Controller Migration

This directory contains both wrangler and controller-runtime implementations of the agentmanagement controllers.

## Migration Status

The agentmanagement controllers are being migrated from wrangler to controller-runtime. Currently implemented:

✅ **config** - Configuration management controller
🚧 **manageagent** - Agent bundle management (TODO)
🚧 **bootstrap** - Bootstrap resources (TODO)  
🚧 **cluster** - Cluster lifecycle management (TODO)
🚧 **clusterregistrationtoken** - Registration token management (TODO)
🚧 **clusterregistration** - Cluster registration (TODO)

## Feature Flag

Use the environment variable `FLEET_USE_CONTROLLER_RUNTIME_AGENTMANAGEMENT` to control which implementation is used:

```bash
# Use wrangler implementation (default)
export FLEET_USE_CONTROLLER_RUNTIME_AGENTMANAGEMENT=false

# Use controller-runtime implementation
export FLEET_USE_CONTROLLER_RUNTIME_AGENTMANAGEMENT=true
```

## Directory Structure

```
controllers/
├── runtime/           # controller-runtime implementations
│   ├── config_controller.go
│   └── setup.go
├── bootstrap/         # wrangler implementations
├── cluster/
├── clusterregistration/
├── clusterregistrationtoken/
├── config/
├── manageagent/
└── controllers.go     # wrangler setup
```

## Testing

To test the controller-runtime implementation:

```bash
# Set the feature flag
export FLEET_USE_CONTROLLER_RUNTIME_AGENTMANAGEMENT=true

# Run the controller
go run ./cmd/fleetcontroller agentmanagement --namespace cattle-fleet-system
```

## Migration Plan

See [MIGRATION.md](../../../../../../MIGRATION.md) for the complete migration plan.