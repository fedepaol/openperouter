# Research: Static File Configuration Watching with API Merge

**Date**: 2026-02-06
**Feature**: 001-static-file-watch

## Overview

This document consolidates research findings for implementing file watching and configuration merging in OpenPERouter's systemd mode.

## 1. File Watching Mechanism

### Decision: fsnotify/fsnotify

**Rationale**:
- Industry-standard Go library for file system notifications (50k+ stars on GitHub)
- Cross-platform support using native OS mechanisms (inotify on Linux, FSEvents on macOS)
- Well-maintained with active community
- Already used in similar Kubernetes projects (kubectl, kubelet use similar patterns)
- Low overhead - uses OS kernel notifications instead of polling

**Alternatives Considered**:

| Alternative | Pros | Cons | Why Rejected |
|-------------|------|------|--------------|
| Manual polling | Simple, no dependencies | High CPU usage, slower detection | Violates performance requirements (5s detection) |
| Custom inotify wrapper | Fine-grained control | Platform-specific, maintenance burden | fsnotify provides same capabilities with cross-platform support |
| Third-party commercial | Support contracts | Cost, vendor lock-in | Unnecessary for this use case |

**Implementation Pattern**:

```go
watcher, err := fsnotify.NewWatcher()
defer watcher.Close()

// Watch directory
err = watcher.Add(configDir)

// Event loop
for {
    select {
    case event := <-watcher.Events:
        // Handle Write, Create, Remove, Rename events
        if event.Op&fsnotify.Write == fsnotify.Write {
            // Trigger reconciliation
        }
    case err := <-watcher.Errors:
        // Log and continue
    }
}
```

**References**:
- https://github.com/fsnotify/fsnotify
- https://pkg.go.dev/github.com/fsnotify/fsnotify

---

## 2. Event Debouncing Strategy

### Decision: Time-based Debouncing with Timer Reset

**Rationale**:
- Prevents configuration thrashing from rapid successive file changes (e.g., editor save patterns)
- Common pattern in file-watching systems (e.g., webpack, nodemon use similar approach)
- Balances responsiveness with stability

**Implementation Pattern**:

```go
const debounceInterval = 500 * time.Millisecond

var debounceTimer *time.Timer

for {
    select {
    case event := <-watcher.Events:
        if debounceTimer != nil {
            debounceTimer.Stop()
        }
        debounceTimer = time.AfterFunc(debounceInterval, func() {
            triggerReconciliation()
        })
    }
}
```

**Timing Analysis**:
- 500ms debounce window allows for multi-file saves to coalesce
- Still meets 5-second detection requirement (500ms << 5s)
- Typical editor save sequences complete within 200-300ms

**Alternatives Considered**:

| Alternative | Pros | Cons | Why Rejected |
|-------------|------|------|--------------|
| No debouncing | Immediate response | Excessive reconciliation | Can trigger 10+ reconciliations for single logical change |
| File hash comparison | Skip identical content | I/O overhead on every event | Adds latency and complexity |
| Longer debounce (2s+) | Fewer reconciliations | Slower user feedback | Approaches the 5s limit |

**References**:
- Common pattern in file watcher implementations
- Similar to Kubernetes ConfigMap/Secret volume update debouncing

---

## 3. Channel-Based Architecture for Static/API Merge

### Decision: Unified Event Channel with Source-Tagged Events

**Rationale**:
- OpenPERouter already uses channel-based reconciliation (StaticConfigReconciler.TriggerChan)
- Controller-runtime source.Channel pattern is established
- Allows seamless transition from static-only to hybrid mode
- Follows existing architecture patterns in the codebase

**Architecture**:

```go
// Existing pattern in StaticConfigReconciler
type StaticConfigReconciler struct {
    TriggerChan chan event.GenericEvent
    // ... other fields
}

// Extended pattern for hybrid mode
type HybridConfigReconciler struct {
    StaticTriggerChan chan event.GenericEvent  // From file watcher
    APITriggerChan    chan event.GenericEvent  // From API watches
    // ... other fields
}

// In Reconcile():
// 1. Read static configs
// 2. If API available, read API configs
// 3. Merge using existing conversion.MergeAPIConfigs
// 4. Apply merged configuration
```

**Transition Flow**:

1. **Bootstrap (API unavailable)**:
   - File watcher → StaticTriggerChan → Reconcile with static-only config

2. **API Detection**:
   - Kubernetes client health check detects API availability
   - One-time trigger to reconcile with both sources

3. **Hybrid Mode (API available)**:
   - File watcher → StaticTriggerChan → Reconcile with merged config
   - API watch → APITriggerChan → Reconcile with merged config
   - Both channels feed same reconciliation logic

**Alternatives Considered**:

| Alternative | Pros | Cons | Why Rejected |
|-------------|------|------|--------------|
| Two separate controllers | Clear separation | Duplicate reconciliation logic, state sync complexity | Violates YAGNI, harder to maintain |
| Single merged channel | Simplest code | Loses source attribution | Need to track config source for debugging (SC-005) |
| Event aggregator goroutine | Flexible routing | Additional complexity | Current channel pattern sufficient |

**References**:
- Existing `internal/controller/routerconfiguration/static_configuration_controller.go`
- controller-runtime source.Channel pattern

---

## 4. API Server Availability Detection

### Decision: Kubernetes Client DiscoveryClient Polling

**Rationale**:
- Uses existing client-go infrastructure
- Reliable detection mechanism
- Non-blocking background check

**Implementation Pattern**:

```go
func watchAPIAvailability(ctx context.Context, config *rest.Config, onAvailable func()) {
    ticker := time.NewTicker(5 * time.Second)
    defer ticker.Stop()

    for {
        select {
        case <-ticker.C:
            client, err := discovery.NewDiscoveryClientForConfig(config)
            if err != nil {
                continue
            }
            _, err = client.ServerVersion()
            if err == nil {
                // API is available
                onAvailable()
                return
            }
        case <-ctx.Done():
            return
        }
    }
}
```

**Alternatives Considered**:

| Alternative | Pros | Cons | Why Rejected |
|-------------|------|------|--------------|
| TCP socket check | Lightweight | Doesn't verify API functionality | Port open doesn't mean API ready |
| HTTP health endpoint | Standard pattern | May not exist in all deployments | Discovery client more reliable |
| Wait for first successful API call | No polling | Delayed detection | Want proactive detection |

**References**:
- k8s.io/client-go/discovery
- Similar pattern in kubelet API server wait logic

---

## 5. Configuration Merge and Conflict Detection

### Decision: Use Existing conversion.MergeAPIConfigs with Validation

**Rationale**:
- Already implemented and tested in `internal/conversion/api.go`
- Handles conflict detection through validation
- Consistent with current architecture
- User confirmed existing conflict handling should be preserved

**Current Merge Logic**:

```go
// From internal/conversion/api.go
func MergeAPIConfigs(configs ...ApiConfigData) (ApiConfigData, error) {
    // Merges underlays, L3VNIs, L2VNIs, L3Passthrough
    // Detects conflicts and returns errors
}
```

**Integration Pattern**:

```go
// In Reconcile():
staticConfig := readStaticConfigs(configDir)
apiConfig := readAPIConfigs(client) // When API available

merged, err := conversion.MergeAPIConfigs(staticConfig, apiConfig)
if err != nil {
    // Log conflict error, preserve last known good config
    return ctrl.Result{}, fmt.Errorf("config merge conflict: %w", err)
}

// Apply merged configuration
```

**Conflict Handling**:
- Same resource ID in both sources → validation error
- Error logged with source attribution (static file path vs. API resource name/namespace)
- Last known good configuration preserved
- Operator must resolve conflict manually

**Alternatives Considered**:

| Alternative | Pros | Cons | Why Rejected |
|-------------|------|------|--------------|
| Last-write-wins | No conflicts | Data loss risk, unexpected behavior | User explicitly requested validation errors |
| API-always-wins precedence | Clear priority | Ignores static config changes | Violates user requirement for conflict errors |
| Per-field merge | Granular control | Complex logic, hard to reason about | Existing validation approach is simpler |

**References**:
- Existing `internal/conversion/api.go`
- User requirement: "Conflicts are already handled today, I don't expect the behavior to change"

---

## 6. Edge Case Handling

### Partial File Writes

**Decision**: Retry-with-backoff on Parse Errors

**Implementation**:
- On file event, attempt to parse configuration
- If parse fails, log warning and schedule retry (exponential backoff: 100ms, 200ms, 400ms, max 3 retries)
- If still failing, preserve previous valid configuration and log error
- Next file change event resets retry counter

**Rationale**: Most partial writes complete within 100-200ms; retry handles editor save patterns

---

### Malformed Configuration

**Decision**: Preserve Last Known Good Config + Error Logging

**Implementation**:
- Validation errors logged with file path and line number
- Previous valid configuration remains active
- Status/health endpoint reports configuration validation error
- Operator alerted through logs

**Rationale**: Fail-safe behavior prevents service disruption from typos

---

### Permission Errors

**Decision**: Log Error + Continue with Last Config

**Implementation**:
- File read permission errors logged with actionable message
- Continue operating with last successfully loaded configuration
- Periodic retry (next file event or 60s timer)

**Rationale**: Temporary permission issues shouldn't crash the service

---

### Large Files (>1MB)

**Decision**: Size Check + Streaming Parse

**Implementation**:
- Check file size before loading
- Warn if >1MB (unusually large for config)
- Use streaming YAML parser to avoid full load into memory
- Timeout mechanism (5s) for parsing

**Rationale**: Config files typically 10-100KB; >1MB suggests misconfiguration

---

### Watched Directory Deletion

**Decision**: Recreate Watch + Error Logging

**Implementation**:
- fsnotify emits error event on directory deletion
- Log critical error
- Attempt to re-add watch every 10s
- Continue operating with last loaded configuration

**Rationale**: Directory deletion is operator error; graceful degradation preferred

---

### Rapid Successive Changes (Debouncing)

**Decision**: 500ms Debounce Window (covered in Section 2)

---

### References to Non-Existent Resources

**Decision**: Defer Resolution to Reconciliation Logic

**Implementation**:
- Configuration merge accepts forward references
- Reconciliation logic handles missing resources (existing behavior)
- Standard error logging and retry

**Rationale**: Existing reconciliation already handles resource dependencies

---

### Invalid Merged Configuration

**Decision**: Validation Error + Reject Merge

**Implementation**:
- After merge, validate combined configuration
- If validation fails, log error with details
- Preserve pre-merge configuration (split static/API configs both remain active independently)
- Operator must resolve by modifying one source

**Rationale**: Prevents applying broken configuration; matches existing conflict behavior

---

## 7. Testing Strategy

### Unit Tests

**Location**: `internal/filewatcher/filewatcher_test.go`

**Coverage**:
- File create, modify, delete events trigger reconciliation
- Debouncing prevents thrashing
- Error handling (parse errors, permission errors)
- Watcher lifecycle (start, stop, cleanup)

**Approach**: Mock fsnotify events, verify reconciliation triggers

---

### E2E Tests (Pre-API): systemd_static_suite

**Location**: `e2etests/systemd_static_suite/systemd_static_files_test.go`

**New Test Cases**:
1. Modify static VNI config → routes updated within 5s
2. Add new static VNI file → new VNI configured
3. Delete static VNI file → VNI removed
4. Multiple rapid changes → debounced into single reconciliation
5. Malformed file → previous config preserved, error logged

**Approach**: Use existing systemd mode deployment, modify files in watched directory, verify route changes

---

### E2E Tests (Post-API): e2etests/tests

**Location**: `e2etests/tests/hybrid_config_test.go` (new)

**New Test Cases**:
1. Start with static config → API becomes available → both configs active
2. Static config + API config (no conflicts) → merged successfully
3. Static config + API config (conflict) → validation error, neither applied
4. API config exists → modify static file → merge recalculated
5. API server goes down → continue with last merged config
6. Static file change while API down → static changes applied

**Approach**: Use existing e2e infrastructure, simulate API availability transitions

---

### Performance Tests

**Metrics to Validate**:
- SC-001: File change detection within 5 seconds
- SC-003: 100 config changes/minute throughput
- Configuration merge completes in <10ms (per assumption)

**Approach**: Benchmark tests with repeated file modifications

---

## 8. Dependencies

### New Dependencies

**fsnotify/fsnotify**: v1.7.0 (latest stable)
- License: BSD-3-Clause (compatible)
- Maturity: Stable, widely used
- Risk: Low - battle-tested in Kubernetes ecosystem

### Existing Dependencies (No Changes)

- controller-runtime: Existing
- k8s.io/client-go: Existing
- k8s.io/apimachinery: Existing

---

## 9. Migration and Rollout

### Backward Compatibility

**Impact**: None - feature is additive

- Existing static config (read-once at startup) continues to work
- File watching only activates when enabled via flag or config
- No changes to configuration file format
- No API/CRD changes

### Rollout Plan

1. **Phase 1**: File watching in static-only mode (US1)
2. **Phase 2**: API detection and merge (US2)
3. **Phase 3**: Continuous hybrid operation (US3)

Each phase independently testable and deployable.

---

## 10. Security Considerations

### File System Access

- File watcher requires read access to configuration directory
- No elevation of privileges required
- Uses standard Go file I/O (no direct syscalls)

### Validation

- All loaded configurations validated before application
- Existing FRR config validation prevents malformed routing config
- File path traversal prevented by watching single directory

### Audit Trail

- All configuration changes logged with source (file path or API resource)
- Timestamps recorded for compliance (SC-005)

---

## Summary

All research decisions support the requirements in spec.md:

| Requirement | Research Decision | Section |
|-------------|-------------------|---------|
| FR-001: Monitor files continuously | fsnotify watcher | 1 |
| FR-002: Detect within 5 seconds | fsnotify + 500ms debounce | 1, 2 |
| FR-003: No process restart | Channel-based reconciliation | 3 |
| FR-005: Detect API availability | DiscoveryClient polling | 4 |
| FR-006: Merge configs | Existing MergeAPIConfigs | 5 |
| FR-007: Detect conflicts | Existing validation logic | 5 |
| FR-012: Handle malformed files | Error handling + last good config | 6 |
| FR-014: Prevent thrashing | Debouncing | 2, 6 |
| SC-001: 5 second detection | fsnotify (instant) + debounce (500ms) | 1, 2 |
| SC-003: 100 changes/min | Channel architecture + efficient merge | 3, 5 |

No NEEDS CLARIFICATION items remain. All unknowns resolved through research.
