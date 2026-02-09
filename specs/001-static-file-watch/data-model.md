# Data Model: Static File Configuration Watching with API Merge

**Date**: 2026-02-06
**Feature**: 001-static-file-watch

## Overview

This document defines the data structures and state management for file watching and configuration merging in OpenPERouter's systemd mode.

---

## 1. Core Data Types

### 1.1 FileWatcher

Manages file system monitoring and event dispatching.

**Location**: `internal/filewatcher/filewatcher.go`

```go
// FileWatcher monitors a directory for configuration file changes
// and triggers reconciliation through a channel.
type FileWatcher struct {
    // Configuration
    watchDir         string
    logger           *slog.Logger
    debounceDuration time.Duration

    // Runtime state
    watcher     *fsnotify.Watcher
    triggerChan chan<- event.GenericEvent

    // Debouncing
    debounceTimer *time.Timer
}

// New creates a new FileWatcher for the specified directory.
// triggerChan is where reconciliation events are sent.
func New(watchDir string, triggerChan chan<- event.GenericEvent, logger *slog.Logger) (*FileWatcher, error)

// Start begins watching the directory for changes.
// Returns immediately; watching happens in background goroutine.
// Cleanup is handled automatically when context is cancelled.
func (fw *FileWatcher) Start(ctx context.Context) error
```

**State Transitions**:
1. Created → Started (via Start())
2. Started → Stopped (via context cancellation)

**Invariants**:
- watcher must be non-nil after Start() succeeds
- triggerChan must not be closed while watcher is running
- watcher cleanup handled in deferred function when context cancelled

---

### 1.2 Configuration Source Tracking

Tracks the origin of configuration elements for debugging and conflict resolution.

**Location**: `internal/conversion/api.go` (extended)

```go
// ConfigSource indicates where a configuration element originated
type ConfigSource int

const (
    SourceStatic ConfigSource = iota  // From static file
    SourceAPI                          // From Kubernetes API
    SourceMerged                       // Merged from both sources
)

// ConfigMetadata tracks metadata about configuration elements
type ConfigMetadata struct {
    Source       ConfigSource
    FilePath     string           // For SourceStatic
    ResourceName string           // For SourceAPI
    Namespace    string           // For SourceAPI
    LoadedAt     time.Time
}

// ApiConfigData extended with source tracking
type ApiConfigData struct {
    Underlays     []v1alpha1.Underlay
    L3VNIs        []v1alpha1.L3VNI
    L2VNIs        []v1alpha1.L2VNI
    L3Passthrough []v1alpha1.L3Passthrough

    // NEW: Metadata for each element (indexed by resource name)
    Metadata      map[string]ConfigMetadata
}
```

**Rationale**:
- Enables SC-005: Operators can trace configuration source through logs
- Supports debugging of merge conflicts
- Minimal overhead (metadata only stored, not used in hot path)

---

### 1.3 Integration with Existing StaticConfigReconciler

**Key Insight**: StaticConfigReconciler already has the necessary infrastructure - no struct changes needed.

**Existing Structure** (no changes):

```go
// StaticConfigReconciler - NO CHANGES to struct
type StaticConfigReconciler struct {
    Scheme          *runtime.Scheme
    Logger          *slog.Logger
    NodeIndex       int
    LogLevel        string
    FRRConfigPath   string
    FRRReloadSocket string
    RouterProvider  RouterProvider
    ConfigDir       string

    TriggerChan chan event.GenericEvent  // Already exists - used by both sources
}

// TriggerReconcile() - Already exists - called by both sources
func (r *StaticConfigReconciler) TriggerReconcile() {
    select {
    case r.TriggerChan <- event.GenericEvent{...}:
        r.Logger.Info("triggered reconciliation")
    default:
        r.Logger.Debug("reconciliation already queued")
    }
}
```

**Integration Points**:

1. **FileWatcher → StaticConfigReconciler**: FileWatcher calls `TriggerReconcile()` on file events
2. **API Controller → StaticConfigReconciler**: Underlay/VNI controller calls `TriggerReconcile()` on API events
3. **Reconcile() Method**: Modified to read both sources and merge when API available

**State Machine** (simplified):

```
┌─────────────────┐
│  Static-Only    │  File events → TriggerReconcile()
│  Mode           │  (API not available)
│                 │
│ - File watching │
│ - Static config │
└────────┬────────┘
         │
         │ API becomes available
         ▼
┌─────────────────┐
│  Hybrid Mode    │  File events → TriggerReconcile()
│                 │  API events → TriggerReconcile()
│ - File watching │  (same trigger channel for both!)
│ - API watching  │
│ - Merged config │
└─────────────────┘
```

---

## 2. Data Flows

### 2.1 Static-Only Mode (Before API Available)

```
┌──────────────┐
│ Static Files │
│ (YAML)       │
└──────┬───────┘
       │
       │ fsnotify event
       ▼
┌──────────────┐
│ FileWatcher  │
│ - Debounce   │
└──────┬───────┘
       │
       │ trigger event
       ▼
┌──────────────────────┐
│ TriggerChan          │
└──────┬───────────────┘
       │
       │ controller-runtime
       ▼
┌──────────────────────┐
│ StaticConfigReconciler│
│ Reconcile()          │
└──────┬───────────────┘
       │
       │ readStaticConfigs()
       ▼
┌──────────────────────┐
│ conversion.          │
│ ApiConfigData        │
│ (static only)        │
└──────┬───────────────┘
       │
       │ apply
       ▼
┌──────────────────────┐
│ FRR Configuration    │
│ (via reloader)       │
└──────────────────────┘
```

---

### 2.2 API Detection and Transition

```
┌──────────────────────┐
│ Background Goroutine │
│ (API Availability    │
│  Polling)            │
└──────┬───────────────┘
       │
       │ poll every 5s
       ▼
┌──────────────────────┐
│ DiscoveryClient.     │
│ ServerVersion()      │
└──────┬───────────────┘
       │
       │ success!
       ▼
┌──────────────────────┐
│ SetAPIClient()       │
│ apiAvailable = true  │
└──────┬───────────────┘
       │
       │ trigger event
       ▼
┌──────────────────────┐
│ Reconcile()          │
│ (first hybrid run)   │
└──────────────────────┘
```

---

### 2.3 Hybrid Mode (API Available)

```
┌──────────────┐                    ┌──────────────┐
│ Static Files │                    │ API Resources│
│ (YAML)       │                    │ (CRDs)       │
└──────┬───────┘                    └──────┬───────┘
       │                                   │
       │ fsnotify                          │ API watch
       ▼                                   ▼
┌──────────────┐                    ┌──────────────────┐
│ FileWatcher  │                    │ Underlay/VNI     │
│              │                    │ Controller       │
└──────┬───────┘                    └──────┬───────────┘
       │                                   │
       │ staticReconciler.                 │ staticReconciler.
       │ TriggerReconcile()                │ TriggerReconcile()
       │                                   │
       └─────────────┬─────────────────────┘
                     │
                     │ Both use same trigger method!
                     ▼
       ┌─────────────────────────────────┐
       │  StaticConfigReconciler         │
       │  .TriggerChan                   │
       └─────────────┬───────────────────┘
                     │
                     │ controller-runtime
                     ▼
       ┌─────────────────────────────────┐
       │  Reconcile() method             │
       │  (in reconcile.go)              │
       └─────────────┬───────────────────┘
                     │
                     ├─> readStaticConfigs() → staticConfig
                     │
                     ├─> readAPIConfigs() → apiConfig (when available)
                     │
                     ▼
       ┌─────────────────────────────────┐
       │  conversion.MergeAPIConfigs(    │
       │    staticConfig, apiConfig)     │
       │  (existing merge logic!)        │
       │                                 │
       │  - Detect conflicts             │
       │  - Return error if conflicts    │
       │  - Merge if no conflicts        │
       └─────────────┬───────────────────┘
                     │
                     │ if error → log, preserve previous
                     │ if success → apply merged
                     ▼
       ┌─────────────────────────────────┐
       │  FRR Configuration              │
       │  (via reloader)                 │
       └─────────────────────────────────┘
```

**Key Points**:
- Both file watcher and API controller call the **same** `TriggerReconcile()` method
- StaticConfigReconciler struct needs **no changes**
- Merge logic already exists in `conversion.MergeAPIConfigs()`

---

## 3. State Management

### 3.1 File Watcher State

**Stored In**: FileWatcher struct (in-memory)

**State Elements**:
- Current watcher instance (fsnotify.Watcher)
- Debounce timer state
- Background goroutine lifecycle (managed by context)

**Lifecycle**: Started with Start() method, stopped via context cancellation

**Persistence**: None - ephemeral state, recreated on restart

---

### 3.2 Configuration State

**Stored In**:
- Files on disk (static configuration)
- Kubernetes API server (API resources)
- In-memory during reconciliation (merged ApiConfigData)

**State Elements**:
- Last successfully applied configuration (implicit in FRR state)
- Configuration source metadata (in ApiConfigData.Metadata)

**Lifecycle**:
- Static files: Persistent on disk
- API resources: Persistent in etcd
- Merged state: Recomputed on every reconciliation

**Persistence**:
- FRR configuration persisted to /etc/perouter/frr/frr.conf
- No additional state persistence required

---

### 3.3 API Availability State

**Stored In**: StaticConfigReconciler.apiAvailable (bool)

**State Elements**:
- API availability flag
- API client instance (when available)

**State Transitions**:
- false → true: When DiscoveryClient successfully connects
- true → false: Not currently implemented (continue with last config on API loss per FR-013)

**Persistence**: None - detected on startup

---

## 4. Validation Rules

### 4.1 Static Configuration Validation

**Applied At**: File load time (readStaticConfigs)

**Rules** (existing):
- YAML syntax must be valid
- Required fields must be present (VNI, VRF name, etc.)
- IP addresses must be valid CIDR notation
- ASN values must be in valid range (1-4294967295)

**Error Handling**: Log error, preserve previous valid configuration

---

### 4.2 API Configuration Validation

**Applied At**: API watch event handling (existing controller logic)

**Rules** (existing):
- CRD schema validation (enforced by API server)
- Same validation rules as static configuration
- Additional Kubernetes resource validation (namespace, name, etc.)

**Error Handling**: Standard controller error handling, requeue

---

### 4.3 Merge Validation

**Applied At**: After MergeAPIConfigs() call

**Rules**:
- No duplicate resource identifiers across sources
  - Same VNI number in both static and API → conflict
  - Same VRF name in both static and API → conflict
  - Same underlay interface in both static and API → conflict
- Merged configuration must pass all validation rules
- Resource references must be resolvable (existing behavior)

**Error Handling**:
- Return error from Reconcile()
- Log detailed conflict information (source files/resources)
- Preserve previous valid configuration
- Operator must resolve manually

**Conflict Error Format**:
```go
type MergeConflictError struct {
    ResourceType string  // "L3VNI", "Underlay", etc.
    Identifier   string  // VNI number, VRF name, etc.
    StaticSource string  // File path
    APISource    string  // Resource name/namespace
}
```

---

## 5. Relationships

### 5.1 FileWatcher ↔ StaticConfigReconciler

- **Relationship**: One-to-one
- **Connection**: FileWatcher sends events to StaticConfigReconciler.TriggerChan
- **Lifecycle**: FileWatcher created and started by main.go with StaticConfigReconciler's context
- **Cleanup**: Automatic cleanup via context cancellation when reconciler shuts down

---

### 5.2 Static Configuration ↔ API Configuration

- **Relationship**: Many-to-many (merged via MergeAPIConfigs)
- **Connection**: Both converted to ApiConfigData format, then merged
- **Conflict Resolution**: Validation error if same resource ID in both sources
- **Independence**: Either source can provide configuration; both optional

---

### 5.3 ApiConfigData ↔ FRR Configuration

- **Relationship**: One-to-one transformation
- **Connection**: ApiConfigData → FRR config generation → FRR reload
- **Existing Flow**: No changes to existing reconciliation logic
- **Validation**: FRR config validation before reload (existing)

---

## 6. Indexing and Lookups

### 6.1 Configuration Metadata Index

**Index**: Map[string]ConfigMetadata keyed by resource identifier

**Purpose**:
- Fast lookup of configuration source for logging
- SC-005: Trace source of any active configuration element

**Update Pattern**:
- Populated during readStaticConfigs() and readAPIConfigs()
- Merged during MergeAPIConfigs()
- Used in error messages and debug logs

---

### 6.2 File Path Tracking

**Index**: Implicit via staticconfiguration.ReadRouterConfigs()

**Purpose**:
- Correlate file events to configuration changes
- Error reporting with file paths

**Update Pattern**:
- Maintained by existing static configuration reader
- No changes required

---

## 7. Memory and Performance Considerations

### 7.1 Memory Footprint

**FileWatcher**: ~1KB (struct + channel buffers)
**ConfigMetadata**: ~100 bytes per resource × typical 10-50 resources = 1-5KB
**Debounce Timer**: ~200 bytes

**Total Additional Memory**: <10KB

**Impact**: Negligible for target platform (Linux servers with GB+ RAM)

---

### 7.2 Performance Characteristics

**File Event Processing**:
- fsnotify: O(1) event delivery (kernel notification)
- Debouncing: O(1) timer reset
- Trigger channel: O(1) send (buffered channel)

**Configuration Merge**:
- MergeAPIConfigs: O(n) where n = total resources from both sources
- Typical n < 100 resources
- Expected time: <10ms (per assumptions in spec.md)

**Reconciliation Frequency**:
- Debounced to ~500ms minimum interval
- Max 100 changes/minute = ~1.7 changes/second
- Well within controller-runtime capacity

---

## 8. Concurrency and Thread Safety

### 8.1 FileWatcher Goroutine

**Goroutine**: Single background goroutine for fsnotify event loop

**Synchronization**:
- Channel-based communication (inherently thread-safe)
- No shared mutable state
- Timer protected by select statement

**Lifecycle**: Controlled by context cancellation

---

### 8.2 Reconciler Concurrency

**Concurrency Model**: controller-runtime's standard model
- Single worker per controller (default)
- Events queued, processed serially
- No additional concurrency required

**Thread Safety**:
- TriggerChan is buffered, multiple senders safe
- apiAvailable flag read/write in same goroutine (controller worker)

---

## 9. Error Recovery

### 9.1 File Watcher Failures

**Error**: fsnotify.Watcher error (e.g., inotify limit exceeded)

**Recovery**:
- Log error
- Attempt to recreate watcher
- Exponential backoff (1s, 2s, 4s, max 30s)
- Continue with last valid configuration

---

### 9.2 Configuration Parse Failures

**Error**: YAML parse error, validation error

**Recovery**:
- Log detailed error with file path and line number
- Retry with exponential backoff (100ms, 200ms, 400ms, max 3 retries)
- If still failing, preserve previous valid configuration
- Operator alerted via logs

---

### 9.3 Merge Conflicts

**Error**: MergeConflictError from MergeAPIConfigs

**Recovery**:
- Log conflict details (resource type, ID, sources)
- Return error from Reconcile() (controller-runtime will requeue)
- Preserve previous valid configuration
- No automatic recovery - operator must resolve

---

### 9.4 API Unavailability After Initial Connection

**Error**: API client calls fail after apiAvailable = true

**Recovery**:
- Log warning
- Continue operating with last successfully merged configuration
- Static file changes still processed (reconcile with static-only config)
- Periodic API health check continues (may transition back to hybrid mode)

---

## Summary

This data model supports all functional requirements:

| Requirement | Data Structure | Section |
|-------------|----------------|---------|
| FR-001: Monitor files | FileWatcher | 1.1 |
| FR-006: Merge configs | ApiConfigData, MergeAPIConfigs | 1.2, 2.3 |
| FR-007: Detect conflicts | MergeConflictError, validation rules | 4.3 |
| FR-010: Validate configs | Validation rules | 4.1, 4.2 |
| FR-011: Log with source | ConfigMetadata | 1.2 |
| FR-013: Graceful degradation | API availability state | 3.3, 9.4 |
| SC-005: Trace source | ConfigMetadata, indexed by resource | 1.2, 6.1 |

All data structures follow Go idiomatic patterns (Section I of Constitution):
- Clear naming (FileWatcher, not ConfigFileSystemMonitor)
- Errors wrapped with context
- No named returns
- Public APIs at top, helpers at bottom
