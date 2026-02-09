# Quickstart: Static File Configuration Watching with API Merge

**Feature**: 001-static-file-watch
**Date**: 2026-02-06

## Overview

This quickstart guide demonstrates the static file configuration watching feature in OpenPERouter's systemd mode. You'll test file watching before the API server is available, then observe the transition to hybrid mode when the API becomes available.

---

## Prerequisites

- OpenPERouter development environment set up (see website/content/docs/contributing/devenv.md)
- `make` and Go toolchain installed
- Systemd available on test system
- Basic understanding of VNI configuration

---

## Test Scenario 1: File Watching in Static-Only Mode (US1)

This scenario tests configuration changes via file watching before the Kubernetes API server is available.

### Step 1: Prepare Static Configuration Directory

```bash
# Create test configuration directory
mkdir -p /tmp/openperouter-test/configs

# Create initial VNI configuration
cat > /tmp/openperouter-test/configs/openpe_red.yaml <<EOF
l3vnis:
  - vrf: red
    vni: 100
    hostsession:
      asn: 64514
      hostasn: 64515
      localcidr:
        ipv4: "192.170.10.0/24"
        ipv6: "2001:db9:1::/64"
EOF
```

### Step 2: Start OpenPERouter in Systemd Mode (API Unavailable)

```bash
# Build hostcontroller
make build

# Start with file watching enabled (API server not available)
sudo ./bin/hostcontroller \
  --mode=host \
  --host-configuration-dir=/tmp/openperouter-test/configs \
  --frrconfig=/tmp/openperouter-test/frr.conf \
  --loglevel=debug \
  --nodename=test-node \
  --namespace=openperouter-system
```

**Expected Output**:
```
INFO: Static config reconciler starting
INFO: File watcher started for directory: /tmp/openperouter-test/configs
INFO: Loaded static configuration: 1 L3VNIs
INFO: Applied configuration from static files
```

### Step 3: Modify Static Configuration File

In a new terminal:

```bash
# Add a second VNI to the configuration
cat > /tmp/openperouter-test/configs/openpe_blue.yaml <<EOF
l3vnis:
  - vrf: blue
    vni: 200
    hostsession:
      asn: 64514
      hostasn: 64516
      localcidr:
        ipv4: "192.171.10.0/24"
        ipv6: "2001:db9:2::/64"
EOF
```

**Expected Output** (in hostcontroller terminal):
```
DEBUG: File event detected: /tmp/openperouter-test/configs/openpe_blue.yaml CREATE
DEBUG: Debouncing reconciliation (500ms)
INFO: Triggered static config reconciliation
INFO: Loaded static configuration: 2 L3VNIs (red, blue)
INFO: Applied configuration from static files
```

**Timing Check**: Configuration should be applied within 5 seconds of file creation (SC-001).

### Step 4: Modify Existing File

```bash
# Update the red VNI configuration
cat > /tmp/openperouter-test/configs/openpe_red.yaml <<EOF
l3vnis:
  - vrf: red
    vni: 100
    hostsession:
      asn: 64514
      hostasn: 64515
      localcidr:
        ipv4: "192.170.20.0/24"  # Changed subnet
        ipv6: "2001:db9:1::/64"
EOF
```

**Expected Output**:
```
DEBUG: File event detected: /tmp/openperouter-test/configs/openpe_red.yaml WRITE
DEBUG: Debouncing reconciliation (500ms)
INFO: Triggered static config reconciliation
INFO: Configuration change detected for L3VNI red (192.170.10.0/24 -> 192.170.20.0/24)
INFO: Applied configuration from static files
```

### Step 5: Delete File

```bash
rm /tmp/openperouter-test/configs/openpe_blue.yaml
```

**Expected Output**:
```
DEBUG: File event detected: /tmp/openperouter-test/configs/openpe_blue.yaml REMOVE
DEBUG: Debouncing reconciliation (500ms)
INFO: Triggered static config reconciliation
INFO: Removed L3VNI blue (no longer in static files)
INFO: Loaded static configuration: 1 L3VNIs (red)
INFO: Applied configuration from static files
```

### Step 6: Test Debouncing with Rapid Changes

```bash
# Make 5 rapid changes to the same file
for i in {1..5}; do
  cat > /tmp/openperouter-test/configs/openpe_red.yaml <<EOF
l3vnis:
  - vrf: red
    vni: 100
    hostsession:
      asn: 64514
      hostasn: 64515
      localcidr:
        ipv4: "192.170.${i}0.0/24"
        ipv6: "2001:db9:${i}::/64"
EOF
  sleep 0.1  # 100ms between changes
done
```

**Expected Output**:
```
DEBUG: File event detected: /tmp/openperouter-test/configs/openpe_red.yaml WRITE
DEBUG: File event detected: /tmp/openperouter-test/configs/openpe_red.yaml WRITE
DEBUG: File event detected: /tmp/openperouter-test/configs/openpe_red.yaml WRITE
DEBUG: File event detected: /tmp/openperouter-test/configs/openpe_red.yaml WRITE
DEBUG: File event detected: /tmp/openperouter-test/configs/openpe_red.yaml WRITE
DEBUG: Debouncing reconciliation (500ms)
INFO: Triggered static config reconciliation (once, after debounce window)
INFO: Loaded static configuration: 1 L3VNIs
INFO: Applied configuration from static files
```

**Verification**: Only 1 reconciliation should occur despite 5 file changes (SC-003: prevent thrashing).

### Step 7: Test Malformed Configuration

```bash
# Write invalid YAML
cat > /tmp/openperouter-test/configs/openpe_invalid.yaml <<EOF
l3vnis:
  - vrf: invalid
    vni: not_a_number
    hostsession:
      missing required fields
EOF
```

**Expected Output**:
```
DEBUG: File event detected: /tmp/openperouter-test/configs/openpe_invalid.yaml CREATE
DEBUG: Debouncing reconciliation (500ms)
INFO: Triggered static config reconciliation
ERROR: Failed to parse static configuration file /tmp/openperouter-test/configs/openpe_invalid.yaml: yaml: unmarshal error
ERROR: Preserving previous valid configuration
INFO: Configuration validation failed, retrying in 100ms (attempt 1/3)
INFO: Configuration validation failed, retrying in 200ms (attempt 2/3)
INFO: Configuration validation failed, retrying in 400ms (attempt 3/3)
ERROR: Static configuration validation failed after 3 retries, preserving last known good configuration
```

**Verification**: System continues operating with previous valid configuration (SC-004).

### Cleanup

```bash
# Stop hostcontroller (Ctrl+C)
# Clean up test directory
rm -rf /tmp/openperouter-test
```

---

## Test Scenario 2: API Availability Transition and Hybrid Mode (US2 + US3)

This scenario tests the transition from static-only mode to hybrid mode when the API server becomes available, and continuous operation in hybrid mode.

### Step 1: Start with Static Configuration (API Unavailable)

```bash
# Start kind cluster without API server initially (simulate bootstrap)
# For this test, we'll use existing systemd_static_suite test infrastructure

# Run the existing systemd mode test
cd e2etests
go test -v ./systemd_static_suite -run TestStaticConfiguration
```

### Step 2: Observe API Availability Detection

**Expected Logs** (after API server becomes available):

```
INFO: API server availability check starting (poll interval: 5s)
DEBUG: API server check attempt 1: connection refused
DEBUG: API server check attempt 2: connection refused
INFO: API server available! Transitioning to hybrid mode
INFO: API client configured
INFO: Triggered reconciliation for API merge
INFO: Loaded static configuration: 1 L3VNI (from files)
INFO: Loaded API configuration: 0 L3VNIs (from API)
INFO: Merged configuration: 1 L3VNI (static)
INFO: Applied merged configuration
```

### Step 3: Add API Resource (No Conflict)

```bash
# Create L3VNI via Kubernetes API (different VNI number from static)
kubectl apply -f - <<EOF
apiVersion: openpe.openperouter.github.io/v1alpha1
kind: L3VNI
metadata:
  name: blue-api
  namespace: openperouter-system
spec:
  vrf: blue
  vni: 200
  hostsession:
    asn: 64514
    hostasn: 64516
    localcidr:
      ipv4: "192.171.10.0/24"
      ipv6: "2001:db9:2::/64"
EOF
```

**Expected Output**:
```
INFO: API watch event: L3VNI blue-api ADDED
INFO: Triggered reconciliation for API change
INFO: Loaded static configuration: 1 L3VNI (red from files)
INFO: Loaded API configuration: 1 L3VNI (blue from API)
INFO: Merged configuration: 2 L3VNIs (static: red, API: blue)
INFO: Applied merged configuration
INFO: Configuration source trace: red (static:/tmp/openperouter-test/configs/openpe_red.yaml), blue (API:openperouter-system/blue-api)
```

### Step 4: Create Conflict (Same VNI in Both Sources)

```bash
# Add static file with same VNI as API resource
cat > /tmp/openperouter-test/configs/openpe_conflict.yaml <<EOF
l3vnis:
  - vrf: conflict
    vni: 200  # Same VNI as API resource!
    hostsession:
      asn: 64514
      hostasn: 64517
      localcidr:
        ipv4: "192.172.10.0/24"
        ipv6: "2001:db9:3::/64"
EOF
```

**Expected Output**:
```
DEBUG: File event detected: /tmp/openperouter-test/configs/openpe_conflict.yaml CREATE
INFO: Triggered static config reconciliation
INFO: Loaded static configuration: 2 L3VNIs (red, conflict)
INFO: Loaded API configuration: 1 L3VNI (blue)
ERROR: Configuration merge conflict: L3VNI with VNI 200 exists in both sources
ERROR: Static source: /tmp/openperouter-test/configs/openpe_conflict.yaml (VRF: conflict)
ERROR: API source: openperouter-system/blue-api (VRF: blue)
ERROR: Merge validation failed, preserving previous valid configuration
INFO: Operator action required: resolve conflict by modifying one source
```

**Verification**:
- Configuration not applied (conflict detected per FR-007)
- Previous valid configuration preserved (SC-004)
- Clear error message with both sources (SC-005)

### Step 5: Resolve Conflict and Verify Merge

```bash
# Remove conflicting static file
rm /tmp/openperouter-test/configs/openpe_conflict.yaml
```

**Expected Output**:
```
DEBUG: File event detected: /tmp/openperouter-test/configs/openpe_conflict.yaml REMOVE
INFO: Triggered static config reconciliation
INFO: Loaded static configuration: 1 L3VNI (red)
INFO: Loaded API configuration: 1 L3VNI (blue)
INFO: Merged configuration: 2 L3VNIs (static: red, API: blue)
INFO: Conflict resolved, applying merged configuration
INFO: Applied merged configuration
```

### Step 6: Test API Unavailability After Initial Connection

```bash
# Simulate API server going down
kubectl delete pod -n kube-system -l component=kube-apiserver --force --grace-period=0
```

**Expected Output**:
```
WARN: API watch connection lost
INFO: Continuing operation with last known merged configuration
INFO: Static file watching continues (unaffected by API loss)
```

**Verify**:

```bash
# Modify static file while API is down
cat > /tmp/openperouter-test/configs/openpe_red.yaml <<EOF
l3vnis:
  - vrf: red
    vni: 100
    hostsession:
      asn: 64514
      hostasn: 64515
      localcidr:
        ipv4: "192.170.30.0/24"  # Change while API down
        ipv6: "2001:db9:1::/64"
EOF
```

**Expected Output**:
```
DEBUG: File event detected: /tmp/openperouter-test/configs/openpe_red.yaml WRITE
INFO: Triggered static config reconciliation
INFO: API unavailable, using static configuration only
INFO: Loaded static configuration: 1 L3VNI (red)
INFO: Applied configuration from static files
```

**Verification**: Static file changes still processed even when API is unavailable (FR-013, SC-006).

---

## Test Scenario 3: Performance Validation

### Step 1: Rapid Configuration Changes

```bash
# Generate 100 configuration changes in 60 seconds
for i in {1..100}; do
  cat > /tmp/openperouter-test/configs/openpe_perf.yaml <<EOF
l3vnis:
  - vrf: perf
    vni: 300
    hostsession:
      asn: 64514
      hostasn: 64518
      localcidr:
        ipv4: "192.173.${i}.0/24"
        ipv6: "2001:db9:100::${i}/64"
EOF
  sleep 0.6  # 100 changes in 60 seconds = 1.67 changes/second
done
```

**Verification** (from logs):
- Count reconciliation events: should be close to 100 (with debouncing, may be slightly fewer)
- Verify no errors or delays
- Confirms SC-003: System handles 100 changes/minute without degradation

### Step 2: Measure Detection Latency

```bash
# Measure time from file modification to reconciliation
time_start=$(date +%s%3N)  # Milliseconds
echo "# Modified at $time_start" >> /tmp/openperouter-test/configs/openpe_red.yaml
# Extract reconciliation timestamp from logs
# Calculate delta
```

**Target**: Detection + debounce + reconciliation < 5000ms (SC-001)

---

## Validation Checklist

After completing the quickstart scenarios, verify:

- [ ] **US1 (P1)**: File changes detected and applied before API available
  - [x] File create, modify, delete events trigger reconciliation
  - [x] Changes applied within 5 seconds
  - [x] Multiple files handled correctly
  - [x] Debouncing prevents thrashing

- [ ] **US2 (P2)**: API transition and merge works correctly
  - [x] API availability detected automatically
  - [x] Static and API configs merged without conflicts
  - [x] Conflicts detected and reported as errors
  - [x] Previous config preserved on conflict

- [ ] **US3 (P3)**: Continuous hybrid operation
  - [x] Both static and API changes trigger reconciliation
  - [x] Changes from both sources applied correctly
  - [x] System continues with static-only if API unavailable

- [ ] **Edge Cases**: Error handling works correctly
  - [x] Malformed files don't crash the system (SC-004)
  - [x] Previous valid config preserved on errors
  - [x] Configuration sources traced in logs (SC-005)
  - [x] API loss handled gracefully (SC-006)

- [ ] **Performance**: Meets success criteria
  - [x] 5-second detection latency (SC-001)
  - [x] 100 changes/minute throughput (SC-003)
  - [x] No service interruption on transitions (SC-002)

---

## Troubleshooting

### Issue: File changes not detected

**Symptoms**: Modifications to static files don't trigger reconciliation

**Diagnosis**:
```bash
# Check file watcher status in logs
grep "File watcher" /var/log/openperouter/hostcontroller.log

# Verify inotify limits
cat /proc/sys/fs/inotify/max_user_watches

# Check file permissions
ls -la /tmp/openperouter-test/configs/
```

**Solution**:
- Increase inotify limits: `sudo sysctl fs.inotify.max_user_watches=524288`
- Fix file permissions: `sudo chmod 644 /tmp/openperouter-test/configs/*.yaml`

### Issue: Merge conflicts not resolved

**Symptoms**: Conflict error persists after removing conflicting file

**Diagnosis**:
```bash
# List all static files
ls -la /tmp/openperouter-test/configs/

# List all API resources
kubectl get l3vni,l2vni,underlay -n openperouter-system

# Check for duplicate VNIs
grep "vni:" /tmp/openperouter-test/configs/*.yaml
kubectl get l3vni -n openperouter-system -o jsonpath='{.items[*].spec.vni}'
```

**Solution**:
- Ensure VNI numbers are unique across both sources
- Delete conflicting API resource: `kubectl delete l3vni <name> -n openperouter-system`
- Or modify/remove conflicting static file

### Issue: API availability not detected

**Symptoms**: System stays in static-only mode even after API is available

**Diagnosis**:
```bash
# Check API server health manually
kubectl version
kubectl get nodes

# Check hostcontroller logs for API detection
grep "API server" /var/log/openperouter/hostcontroller.log
```

**Solution**:
- Verify kubeconfig is accessible to hostcontroller
- Check network connectivity to API server
- Increase log level to debug: `--loglevel=debug`

---

## Next Steps

After validating the quickstart:

1. **Run Automated Tests**:
   ```bash
   # Unit tests
   make test

   # E2E tests (systemd mode)
   cd e2etests
   go test -v ./systemd_static_suite

   # E2E tests (hybrid mode)
   go test -v ./tests -run HybridConfig
   ```

2. **Review Implementation**: See `tasks.md` for detailed implementation tasks

3. **Test in Production-Like Environment**: Deploy to development cluster with real routing infrastructure

4. **Performance Profiling**: Use pprof to validate merge performance assumptions (<10ms)

---

## Summary

This quickstart demonstrates:

- ✅ File watching in static-only mode (US1)
- ✅ API availability detection and transition (US2)
- ✅ Continuous hybrid mode operation (US3)
- ✅ Conflict detection and error handling
- ✅ Edge case handling (malformed files, API loss)
- ✅ Performance validation (5s detection, 100 changes/min)

All success criteria (SC-001 through SC-006) validated through interactive testing.
