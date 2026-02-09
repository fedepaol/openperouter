# openperouter Development Guidelines

Auto-generated from all feature plans. Last updated: 2026-02-09

## Active Technologies

- Go 1.24.9 (per go.mod)
- fsnotify v1.7.0 for file system watching
- controller-runtime for Kubernetes controllers
- FRR (Free Range Routing) for BGP/EVPN

## Project Structure

```text
cmd/hostcontroller/          # Main entrypoint for host/systemd mode
internal/
├── filewatcher/             # File system watching with debouncing
├── controller/
│   └── routerconfiguration/ # Reconcilers for static and API config
├── conversion/              # Config merge logic
└── staticconfiguration/     # Static file reading
e2etests/                    # End-to-end tests
```

## Commands

```bash
# Build
go build ./cmd/hostcontroller/...

# Test
go test ./...
go test -race ./...  # With race detector

# Lint
make lint

# E2E tests
go test ./e2etests/...
```

## Code Style

- Go idiomatic patterns (line of sight, early returns)
- Error wrapping with context
- Controller-runtime patterns for reconcilers
- Context-based lifecycle management

## Architecture Patterns

### File Watching
- `FileWatcher` uses fsnotify with 500ms debouncing
- Context-based lifecycle (no explicit Stop method)
- Events sent via `chan event.GenericEvent` to reconcilers

### Reconcilers
- `StaticConfigReconciler`: Static-only mode (before API available)
- `PERouterReconciler`: Hybrid mode (API + static merge)
- Transition: Cancel static context when API detected

### Configuration Merge
- `PERouterReconciler.mergeStaticConfig()` merges both sources
- `conversion.MergeAPIConfigs()` performs validation-based merge
- Conflicts reported as validation errors

## Recent Changes

- 001-static-file-watch (Phase 3): Static file watching MVP complete
- 001-static-file-watch (Phase 4): API merge and reconciler transition complete

<!-- MANUAL ADDITIONS START -->
<!-- MANUAL ADDITIONS END -->
