# Tasks: Static File Configuration Watching with API Merge

**Input**: Design documents from `/specs/001-static-file-watch/`
**Prerequisites**: plan.md (required), spec.md (required for user stories), research.md, data-model.md, quickstart.md

**Tests**: Tests are included in this implementation as the feature requires validation of file watching, API merge, and edge case handling.

**Organization**: Tasks are grouped by user story to enable independent implementation and testing of each story.

## Format: `[ID] [P?] [Story] Description`

- **[P]**: Can run in parallel (different files, no dependencies)
- **[Story]**: Which user story this task belongs to (e.g., US1, US2, US3)
- Include exact file paths in descriptions

## Path Conventions

- **Single project**: `internal/`, `cmd/`, `e2etests/` at repository root
- Paths shown below follow OpenPERouter monorepo structure from plan.md

---

## Phase 1: Setup (Shared Infrastructure)

**Purpose**: Project initialization and dependency setup

- [x] T001 Add fsnotify/fsnotify v1.7.0 dependency to go.mod
- [x] T002 [P] Run go mod tidy to update dependencies
- [x] T003 [P] Verify linter passes with make lint (baseline check)

**Checkpoint**: Dependencies ready, baseline tests pass

---

## Phase 2: Foundational (Blocking Prerequisites)

**Purpose**: Core infrastructure that MUST be complete before ANY user story can be implemented

**⚠️ CRITICAL**: No user story work can begin until this phase is complete

- [x] T004 Create internal/filewatcher package directory structure
- [x] T005 Implement FileWatcher type in internal/filewatcher/filewatcher.go with New() and Start() methods (context-based lifecycle)
- [x] T006 Add fsnotify integration and debouncing logic (500ms) in internal/filewatcher/filewatcher.go
- [ ] T007 ~~Add ConfigSource constants and ConfigMetadata type to internal/conversion/api.go~~ (not needed - using existing merge logic)
- [ ] T008 ~~Extend ApiConfigData with Metadata map~~ (not needed - using existing merge logic)
- [ ] T009 ~~Add readAPIConfigs() function~~ (not needed - existing controller handles API config)

**Checkpoint**: Foundation ready - user story implementation can now begin in parallel

---

## Phase 3: User Story 1 - Early Boot Configuration Updates (Priority: P1) 🎯 MVP

**Goal**: Enable automatic detection and application of static file configuration changes before API server is available, without requiring process restart.

**Independent Test**: Start OpenPERouter in systemd mode with API unavailable, modify static file, verify change detected and applied within 5 seconds without restart.

### Tests for User Story 1

> **NOTE: Write these tests FIRST, ensure they FAIL before implementation**

- [x] T010 [P] [US1] Unit test for FileWatcher event detection in internal/filewatcher/filewatcher_test.go
- [x] T011 [P] [US1] Unit test for FileWatcher debouncing behavior in internal/filewatcher/filewatcher_test.go
- [x] T012 [P] [US1] Unit test for FileWatcher Start/Stop lifecycle in internal/filewatcher/filewatcher_test.go
- [x] T013 [P] [US1] E2E test for file modification detection in e2etests/systemd_static_suite/systemd_static_files_test.go
- [x] T014 [P] [US1] E2E test for file creation detection in e2etests/systemd_static_suite/systemd_static_files_test.go
- [x] T015 [P] [US1] E2E test for file deletion detection in e2etests/systemd_static_suite/systemd_static_files_test.go

### Implementation for User Story 1

- [x] T016 [US1] Wire FileWatcher in cmd/hostcontroller/main.go for systemd mode startup
- [x] T017 [US1] Connect FileWatcher to StaticConfigReconciler.TriggerChan in cmd/hostcontroller/main.go
- [x] T018 [US1] Context-based lifecycle (automatic cleanup on context cancellation)
- [ ] T019 [US1] ~~Populate ConfigMetadata with static file source~~ (deferred - not needed for MVP)
- [x] T020 [US1] Add logging for file watch events with source attribution in internal/filewatcher/filewatcher.go

**Checkpoint**: At this point, file watching in static-only mode should be fully functional and testable independently. Run e2etests/systemd_static_suite to validate.

---

## Phase 4: User Story 2 - Post-API Configuration Merge (Priority: P2)

**Goal**: Seamlessly merge static file configuration with API-sourced configuration when API server becomes available, without service interruption.

**Independent Test**: Start with static config, bring up API with additional resources, verify both sources merged correctly and conflicts detected.

### Tests for User Story 2

- [ ] T021 [P] [US2] Unit test for API config reading in internal/controller/routerconfiguration/static_configuration_reader.go (test readAPIConfigs function)
- [ ] T022 [P] [US2] Unit test for merge conflict detection in internal/conversion/api.go (test MergeAPIConfigs with conflicts)
- [ ] T023 [P] [US2] Unit test for ConfigMetadata population from API source in internal/controller/routerconfiguration/static_configuration_reader.go
- [ ] T024 [P] [US2] E2E test for non-conflicting merge in e2etests/tests/hybrid_config_test.go
- [ ] T025 [P] [US2] E2E test for conflict detection and error reporting in e2etests/tests/hybrid_config_test.go
- [ ] T026 [P] [US2] E2E test for API unavailability after initial connection in e2etests/tests/hybrid_config_test.go

### Implementation for User Story 2

- [ ] T027 [US2] Implement readAPIConfigs() to read L3VNI, L2VNI, Underlay from API in internal/controller/routerconfiguration/static_configuration_reader.go
- [ ] T028 [US2] Populate ConfigMetadata with API resource source (namespace/name) in readAPIConfigs()
- [ ] T029 [US2] Modify Reconcile() in internal/controller/routerconfiguration/reconcile.go to detect API availability
- [ ] T030 [US2] Add API config reading to Reconcile() when API available in internal/controller/routerconfiguration/reconcile.go
- [ ] T031 [US2] Call existing conversion.MergeAPIConfigs() with both sources in internal/controller/routerconfiguration/reconcile.go
- [ ] T032 [US2] Handle merge conflicts by logging error with source attribution and preserving last valid config in internal/controller/routerconfiguration/reconcile.go
- [ ] T033 [US2] Enhance MergeAPIConfigs error messages with ConfigMetadata details in internal/conversion/api.go
- [ ] T034 [US2] Add logging for configuration source trace (SC-005) in internal/controller/routerconfiguration/reconcile.go

**Checkpoint**: At this point, API detection and merge should work. Static-only and hybrid modes both functional independently.

---

## Phase 5: User Story 3 - Continuous Runtime Configuration Updates (Priority: P3)

**Goal**: Enable both static file changes and API resource changes to be detected and applied continuously throughout runtime in hybrid mode.

**Independent Test**: Run with both sources available, make changes to each at different times, verify all changes detected and applied with proper merge.

### Tests for User Story 3

- [ ] T035 [P] [US3] E2E test for static file change while API available in e2etests/tests/hybrid_config_test.go
- [ ] T036 [P] [US3] E2E test for API resource change while files being watched in e2etests/tests/hybrid_config_test.go
- [ ] T037 [P] [US3] E2E test for simultaneous changes from both sources in e2etests/tests/hybrid_config_test.go
- [ ] T038 [P] [US3] Performance test for 100 changes/minute throughput in e2etests/tests/hybrid_config_test.go

### Implementation for User Story 3

- [ ] T039 [P] [US3] Add StaticConfigReconciler reference to underlay controller in internal/controller/routerconfiguration/underlay_vni_controller.go
- [ ] T040 [P] [US3] Add StaticConfigReconciler reference to VNI controllers in internal/controller/routerconfiguration/underlay_vni_controller.go
- [ ] T041 [US3] Call staticReconciler.TriggerReconcile() on API resource events in internal/controller/routerconfiguration/underlay_vni_controller.go
- [ ] T042 [US3] Pass StaticConfigReconciler reference to API controllers in cmd/hostcontroller/main.go
- [ ] T043 [US3] Add logging for reconciliation trigger source (file vs API) in internal/controller/routerconfiguration/reconcile.go

**Checkpoint**: All user stories should now be independently functional. Both file and API changes trigger merge and application.

---

## Phase 6: Edge Case Handling & Polish

**Purpose**: Handle edge cases documented in spec.md and add robustness

- [ ] T044 [P] Add retry logic with exponential backoff for partial file writes in internal/filewatcher/filewatcher.go
- [ ] T045 [P] Add malformed file error handling with config preservation in internal/controller/routerconfiguration/reconcile.go
- [ ] T046 [P] Add permission error handling and logging in internal/filewatcher/filewatcher.go
- [ ] T047 [P] Add large file size check (>1MB warning) in internal/controller/routerconfiguration/static_configuration_reader.go
- [ ] T048 [P] Add watched directory deletion detection and recovery in internal/filewatcher/filewatcher.go
- [ ] T049 [P] Unit test for malformed file handling in internal/filewatcher/filewatcher_test.go
- [ ] T050 [P] Unit test for permission error handling in internal/filewatcher/filewatcher_test.go
- [ ] T051 [P] E2E test for malformed file preservation of last valid config in e2etests/systemd_static_suite/systemd_static_files_test.go
- [ ] T052 Run quickstart.md validation scenarios (all three test scenarios)
- [ ] T053 Update website/content/docs/architecture.md with file watching subsystem description
- [ ] T054 Run make lint and address any new warnings
- [ ] T055 Run make test to verify all unit tests pass
- [ ] T056 Run make e2etest to verify all e2e tests pass (with active deployment)

**Checkpoint**: Feature complete with edge case handling, documentation updated, all tests passing

---

## Dependencies & Execution Order

### Phase Dependencies

- **Setup (Phase 1)**: No dependencies - can start immediately
- **Foundational (Phase 2)**: Depends on Setup completion - BLOCKS all user stories
- **User Stories (Phase 3+)**: All depend on Foundational phase completion
  - User stories can then proceed in parallel (if staffed)
  - Or sequentially in priority order (P1 → P2 → P3)
- **Edge Case Handling (Phase 6)**: Depends on all desired user stories being complete

### User Story Dependencies

- **User Story 1 (P1)**: Can start after Foundational (Phase 2) - No dependencies on other stories
- **User Story 2 (P2)**: Can start after Foundational (Phase 2) - Builds on US1 but independently testable
- **User Story 3 (P3)**: Can start after Foundational (Phase 2) - Integrates US1 + US2 but independently testable

### Within Each User Story

- Tests MUST be written and FAIL before implementation
- Core FileWatcher implementation (T005-T006) before wiring (T016-T018)
- Read functions (T009, T027) before Reconcile modifications (T029-T032)
- Integration points (T039-T042) after core functionality

### Parallel Opportunities

- All Setup tasks marked [P] can run in parallel
- All Foundational tasks marked [P] can run in parallel (within Phase 2)
- Once Foundational phase completes, all user stories can start in parallel (if team capacity allows)
- All tests for a user story marked [P] can run in parallel
- Implementation tasks within a story marked [P] can run in parallel
- Different user stories can be worked on in parallel by different team members

---

## Parallel Example: User Story 1

```bash
# Launch all tests for User Story 1 together:
Task T010: "Unit test for FileWatcher event detection in internal/filewatcher/filewatcher_test.go"
Task T011: "Unit test for FileWatcher debouncing behavior in internal/filewatcher/filewatcher_test.go"
Task T012: "Unit test for FileWatcher Start/Stop lifecycle in internal/filewatcher/filewatcher_test.go"
Task T013: "E2E test for file modification detection in e2etests/systemd_static_suite/systemd_static_files_test.go"
Task T014: "E2E test for file creation detection in e2etests/systemd_static_suite/systemd_static_files_test.go"
Task T015: "E2E test for file deletion detection in e2etests/systemd_static_suite/systemd_static_files_test.go"

# After tests fail, launch implementation tasks:
Task T019: "Populate ConfigMetadata with static file source in internal/controller/routerconfiguration/static_configuration_reader.go"
Task T020: "Add logging for file watch events with source attribution in internal/filewatcher/filewatcher.go"
```

---

## Implementation Strategy

### MVP First (User Story 1 Only)

1. Complete Phase 1: Setup (T001-T003)
2. Complete Phase 2: Foundational (T004-T009) - CRITICAL - blocks all stories
3. Complete Phase 3: User Story 1 (T010-T020)
4. **STOP and VALIDATE**: Run e2etests/systemd_static_suite to test file watching independently
5. Deploy/demo if ready - this is a viable MVP!

### Incremental Delivery

1. Complete Setup + Foundational → Foundation ready
2. Add User Story 1 (T010-T020) → Test independently → Deploy/Demo (MVP!)
3. Add User Story 2 (T021-T034) → Test independently → Deploy/Demo (API merge capability)
4. Add User Story 3 (T035-T043) → Test independently → Deploy/Demo (continuous hybrid mode)
5. Add Edge Cases (T044-T056) → Full feature complete
6. Each story adds value without breaking previous stories

### Parallel Team Strategy

With multiple developers:

1. Team completes Setup + Foundational together (T001-T009)
2. Once Foundational is done:
   - Developer A: User Story 1 (T010-T020) - File watching
   - Developer B: User Story 2 (T021-T034) - API merge
   - Developer C: User Story 3 (T035-T043) - Continuous updates
3. Stories complete and integrate independently
4. Team collaborates on Edge Cases & Polish (T044-T056)

---

## Notes

- [P] tasks = different files, no dependencies
- [Story] label maps task to specific user story for traceability
- Each user story should be independently completable and testable
- Verify tests fail before implementing (TDD approach)
- Commit after each task or logical group
- Stop at any checkpoint to validate story independently
- **Key Insight**: StaticConfigReconciler.TriggerReconcile() already exists - both file watcher and API controllers call it
- **Key Insight**: conversion.MergeAPIConfigs() already exists - just needs to be called when both sources available
- Avoid: vague tasks, same file conflicts, cross-story dependencies that break independence

---

## Task Count Summary

- **Total Tasks**: 56
- **Setup (Phase 1)**: 3 tasks
- **Foundational (Phase 2)**: 6 tasks
- **User Story 1 (Phase 3)**: 11 tasks (6 tests + 5 implementation)
- **User Story 2 (Phase 4)**: 14 tasks (6 tests + 8 implementation)
- **User Story 3 (Phase 5)**: 10 tasks (4 tests + 6 implementation)
- **Edge Cases & Polish (Phase 6)**: 12 tasks (4 edge case tests + 8 polish/validation)

**Parallel Opportunities**: 38 tasks marked [P] can run in parallel with other tasks in same phase

**Independent Test Validation**:
- US1: File watching without API (e2etests/systemd_static_suite)
- US2: API merge and conflict detection (e2etests/tests/hybrid_config_test.go)
- US3: Continuous hybrid mode (e2etests/tests/hybrid_config_test.go)

**MVP Scope**: Phases 1-3 (Tasks T001-T020) = 20 tasks for basic file watching capability
