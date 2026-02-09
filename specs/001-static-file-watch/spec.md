# Feature Specification: Static File Configuration Watching with API Merge

**Feature Branch**: `001-static-file-watch`
**Created**: 2026-02-06
**Status**: Draft
**Input**: User description: "when we run in systemd mode we are able to consume files from a static folder. Those files are read only when the process starts but we want to be able to react to the changes of those files both before the api server becomes available and after it's available. When the apiserver is available, the configuration read via the static files must be merged with what comes from the k8s api"

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Early Boot Configuration Updates (Priority: P1)

When OpenPERouter runs in systemd mode before the Kubernetes API server is available, operators need the system to automatically detect and apply configuration changes from static files without requiring a process restart.

**Why this priority**: This is the foundational capability that enables configuration updates during the critical early boot phase when the API server is not yet available. Without this, operators must restart the entire process to apply configuration changes, causing service disruption.

**Independent Test**: Can be fully tested by starting OpenPERouter in systemd mode with API server unavailable, modifying a static configuration file, and verifying the configuration change is detected and applied without process restart.

**Acceptance Scenarios**:

1. **Given** OpenPERouter is running in systemd mode with API server unavailable, **When** an operator modifies a static configuration file, **Then** the system detects the file change within 5 seconds and applies the new configuration
2. **Given** OpenPERouter is running in systemd mode with API server unavailable, **When** multiple static configuration files are modified simultaneously, **Then** the system processes all changes and applies the complete updated configuration
3. **Given** OpenPERouter is running in systemd mode with API server unavailable, **When** a static configuration file is deleted, **Then** the system removes the corresponding configuration entries and logs the deletion
4. **Given** OpenPERouter is running in systemd mode with API server unavailable, **When** a new static configuration file is added to the watched folder, **Then** the system detects and applies the new configuration

---

### User Story 2 - Post-API Configuration Merge (Priority: P2)

When the Kubernetes API server becomes available after OpenPERouter has been running with static file configuration, the system must seamlessly merge static file configuration with API-sourced configuration without service interruption.

**Why this priority**: This enables the transition from static-file-only mode to hybrid mode where both static files and API resources provide configuration. This is critical for systems that start before Kubernetes is fully operational.

**Independent Test**: Can be tested by starting OpenPERouter with static configuration files, then bringing up the API server with additional configuration resources, and verifying both sources are merged correctly.

**Acceptance Scenarios**:

1. **Given** OpenPERouter is running with static file configuration and API server becomes available, **When** API server provides additional configuration resources, **Then** both static file and API configurations are merged and applied together
2. **Given** OpenPERouter has both static file and API configuration active, **When** static file configuration conflicts with API configuration for the same resource, **Then** the system detects the conflict, reports it as a validation error, and does not apply the conflicting configuration
3. **Given** OpenPERouter is running in merged configuration mode, **When** API server becomes unavailable, **Then** the system continues operating with the last known merged configuration and continues watching static files for changes
4. **Given** OpenPERouter is running in merged configuration mode, **When** a configuration exists in both static files and API, and the static file is modified, **Then** the merge is recalculated and applied

---

### User Story 3 - Continuous Runtime Configuration Updates (Priority: P3)

When OpenPERouter is running in systemd mode with API server available, operators need both static file changes and API resource changes to be detected and applied continuously throughout the system's runtime.

**Why this priority**: This provides ongoing operational flexibility by allowing configuration updates through either static files or API resources after the initial boot and merge phases are complete.

**Independent Test**: Can be tested by running OpenPERouter with both static files and API server available, making changes to both sources at different times, and verifying all changes are detected and applied.

**Acceptance Scenarios**:

1. **Given** OpenPERouter is running with API server available and watching both static files and API resources, **When** an operator updates a static configuration file, **Then** the change is detected, merged with API configuration, and applied
2. **Given** OpenPERouter is running with API server available and watching both static files and API resources, **When** an API resource is created, updated, or deleted, **Then** the change is detected, merged with static file configuration, and applied
3. **Given** OpenPERouter is running in hybrid mode, **When** configuration changes occur in both static files and API resources within the same time window, **Then** all changes are detected and the final merged configuration reflects both updates

---

### Edge Cases

- What happens when a static configuration file is partially written (file being written by another process)?
- How does the system handle malformed or invalid configuration in static files?
- What happens when static file permissions prevent reading?
- How does the system handle very large configuration files (>1MB)?
- What happens when the watched folder itself is deleted or moved?
- How does the system handle rapid successive changes to the same file (debouncing)?
- What happens when static file configuration references resources that don't exist yet?
- How does the system handle configuration that was valid in static-only mode but becomes invalid after API merge?

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: System MUST monitor static configuration files for changes continuously while running in systemd mode
- **FR-002**: System MUST detect file modifications, additions, and deletions in the configured static folder within 5 seconds
- **FR-003**: System MUST apply static file configuration changes without requiring process restart
- **FR-004**: System MUST continue processing static file configuration before API server is available
- **FR-005**: System MUST detect when API server becomes available and initiate configuration merge
- **FR-006**: System MUST merge static file configuration with API-sourced configuration when both are available
- **FR-007**: System MUST detect conflicts between static file and API configuration using existing validation logic and report them as errors
- **FR-008**: System MUST continue watching static files for changes after API server becomes available
- **FR-009**: System MUST continue monitoring API resources for changes after initial merge
- **FR-010**: System MUST validate static file configuration before applying changes
- **FR-011**: System MUST log all configuration source changes (static file or API) with timestamps
- **FR-012**: System MUST handle malformed static files by logging errors and preserving previous valid configuration
- **FR-013**: System MUST support graceful degradation when API server becomes unavailable after initial connection
- **FR-014**: System MUST prevent configuration thrashing by debouncing rapid successive changes to the same file
- **FR-015**: System MUST preserve configuration state across static file and API source transitions

### Key Entities

- **Static Configuration File**: Represents configuration data stored in files on the local filesystem; monitored for changes; contains routing, VPN, or network configuration
- **API Configuration Resource**: Represents configuration data sourced from Kubernetes Custom Resources via the API server; includes CRDs like VNI, Underlay, etc.
- **Merged Configuration**: Represents the unified configuration state combining both static file and API sources; applied to the running system; maintains precedence rules for conflict resolution
- **Configuration Source**: Identifies whether a specific configuration element originated from static files or API; tracks source for conflict resolution and debugging

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: Configuration changes in static files are detected and applied within 5 seconds without process restart
- **SC-002**: System successfully transitions from static-file-only mode to merged mode when API server becomes available without service interruption
- **SC-003**: System handles 100 configuration changes per minute across both static files and API resources without degradation
- **SC-004**: Invalid or malformed static configuration files do not cause system failure or loss of existing valid configuration
- **SC-005**: Operators can trace the source (static file or API) of any active configuration element through logs
- **SC-006**: System continues operating with last known configuration when either configuration source becomes temporarily unavailable

## Assumptions

- Static configuration files use the same format/schema as used in current systemd mode implementation
- The static folder path is provided as a startup parameter and does not change during runtime
- File system notification mechanisms (inotify or similar) are available on the target platform
- API server availability can be detected through existing Kubernetes client health checks
- Configuration merge operations complete within milliseconds for typical configuration sizes
- Static files and API resources use compatible schema versions for merging
