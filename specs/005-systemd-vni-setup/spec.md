# Feature Specification: Systemd-Based VNI Setup Script

**Feature Branch**: `005-systemd-vni-setup`  
**Created**: 2026-04-01  
**Status**: Draft  
**Input**: User description: "in @systemdmode/common.sh we already have a way to wait for ensuring the frr cotnaienr running in systemd mode is running, and to run commands inside and outside the network namespace. What I want is a scritp (that I will run as systemd unit) that looks for frr to be ready and to setup a l2vpn and l3vpn (basically what the @config/samples/l2vni.yaml adn @config/crd/bases/openpe.openperouter.github.io_l3vnis.yaml  do now). YOu can look at https://github.com/fedepaol/dev-scripts/blob/001-sno-agent-bridge-config-firstboot-multinode/deploy/extras/config/openpe_config.yaml for the vni values. the veth corresponding to the l2vni must be enslaved to br0 as per that configuration. Also need raw frr configuration for the perouter with full l2+l3 evpn setup. The system must also handle underlay setup: moving a host NIC into the network namespace and setting up the BGP session with the TOR switch. VTEP IP should be 10.0.0.X/24 where X is the last octet from br0's IP."

## Clarifications

### Session 2026-04-01

- Q: How should the script identify which host NIC to move into the FRR namespace? → A: Environment variable with hardcoded fallback default
- Q: How should BGP session parameters (AS number, neighbor IP, authentication) for the TOR switch be provided to the script? → A: Environment variables with defaults for TOR IP and AS number, no authentication
- Q: If the script fails partway through configuration (e.g., after moving the NIC but before establishing BGP), what should happen to the partial configuration? → A: Exit with error on failure, no rollback (not production-ready)
- Q: What should be the default timeout (in seconds) when waiting for the FRR container to become ready? → A: 60 seconds
- Q: After moving the host NIC into the FRR namespace, how should the NIC be configured with an IP address for underlay connectivity? → A: NIC already has IP from host, preserved when moved to namespace

## User Scenarios & Testing

### User Story 1 - Automated VPN Setup on System Boot (Priority: P1)

A system administrator deploying OpenPERouter in systemd mode needs the complete network stack (underlay and overlay) to be automatically configured when the system boots, without manual intervention. This includes moving the host NIC into the FRR namespace, establishing BGP peering with the TOR switch, and configuring both L2 and L3 VPN networks. The setup must wait for the FRR routing daemon container to be fully operational before attempting network configuration, ensuring all dependencies are met.

**Why this priority**: This is the core functionality that enables hands-off deployment. Without automatic setup, administrators would need to manually configure underlay connectivity, BGP sessions, and VPNs on every boot or deployment, defeating the purpose of systemd integration.

**Independent Test**: Can be fully tested by enabling the systemd unit, rebooting the system, and verifying that the host NIC is in FRR namespace, BGP session with TOR is established, and both L2VPN (VNI 210) and L3VPN (VNI 100) are operational without any manual commands.

**Acceptance Scenarios**:

1. **Given** host NIC is identified for underlay, **When** VPN setup runs, **Then** the NIC is moved from host namespace into FRR network namespace
2. **Given** host NIC has been moved to FRR namespace, **When** setup configures underlay, **Then** BGP session with TOR switch (kindswitch) is established and shows "Established" state
3. **Given** system is freshly booted with FRR container configured, **When** systemd starts the VPN setup service, **Then** L3VPN with VNI 100 and VRF "red" is created and operational
4. **Given** system is freshly booted with FRR container configured, **When** systemd starts the VPN setup service, **Then** L2VPN with VNI 210 and VRF "red" is created and operational
5. **Given** FRR container is not yet running, **When** VPN setup service starts, **Then** script waits for FRR to become ready before proceeding with configuration
6. **Given** L2VPN veth interface has been created, **When** setup completes, **Then** the veth interface is enslaved to the br0 bridge
7. **Given** br0 has IP address 192.168.1.5/24, **When** VPN setup runs, **Then** VTEP IP is configured as 10.0.0.5/24
8. **Given** L2VPN is configured and operational, **When** checking EVPN routes, **Then** EVPN type 2 routes (MAC-IP) are present and exchanged with kindswitch
9. **Given** L3VPN is configured and operational, **When** checking EVPN routes, **Then** EVPN type 5 routes (IP Prefix) are present and exchanged with kindswitch
10. **Given** rawfrrconfigs entry is provided in static YAML config, **When** perouter controller processes the configuration, **Then** EVPN BGP configuration for both L2 and L3 VPNs is rendered and applied to FRR, matching CRD-based configuration output

---

### User Story 2 - Diagnostic Logging for Troubleshooting (Priority: P2)

A system administrator troubleshooting deployment issues needs clear, actionable log messages that indicate what the script is doing at each step and provide specific error details when configuration fails. Logs should be accessible through standard systemd journal commands.

**Why this priority**: While not critical for basic functionality, good logging significantly reduces time-to-resolution for deployment issues and is essential for production environments.

**Independent Test**: Can be tested by intentionally causing failures (e.g., FRR not running, missing dependencies) and verifying that journalctl shows clear error messages indicating the specific problem.

**Acceptance Scenarios**:

1. **Given** FRR container fails to start, **When** VPN setup runs, **Then** logs clearly indicate waiting for FRR and eventually timeout after 60 seconds with actionable error message
2. **Given** br0 bridge does not exist, **When** L2VPN setup attempts to enslave veth, **Then** logs indicate the missing bridge and provide guidance
3. **Given** VPN setup completes successfully, **When** administrator checks logs, **Then** logs show each configuration step completed (L3VPN created, L2VPN created, veth enslaved)

---

### Edge Cases

- What happens when the designated host NIC does not exist? Script should fail gracefully with clear error message indicating the missing NIC and its expected name or identifier.
- What happens when the designated host NIC does not have an IP address configured? Script should fail gracefully with clear error message indicating the NIC must be configured with an IP before running.
- What happens when the host NIC is already in use by another namespace or service? Script should detect the conflict and either reclaim it or fail with a clear error message.
- What happens when TOR switch is not reachable for BGP peering? Script should timeout after configured period and fail with clear error message indicating TOR connectivity issue.
- What happens when BGP session with TOR fails to establish? Script should retry with backoff and eventually fail with diagnostic information about the BGP session state.
- What happens when the FRR container crashes during VPN setup? Script should detect the failure and exit with an error code, leaving partial configuration intact for debugging.
- What happens when br0 bridge does not exist when attempting to enslave L2VPN veth? Script should fail gracefully with clear error message indicating missing bridge dependency.
- What happens when br0 bridge does not have an IP address assigned? Script should fail gracefully with clear error message indicating br0 must have an IP address configured.
- What happens when br0 has multiple IP addresses? Script should use the primary IP address (first one listed) to extract the last octet.
- What happens when the network namespace does not exist? Script should wait or create it as needed, depending on FRR container initialization.
- What happens when VNI values conflict with existing configuration? Script should detect conflicts and either reconfigure or log warnings.
- What happens when the script is run before the FRR container is scheduled to start? Script should wait with timeout and fail gracefully if FRR doesn't become available.

## Requirements

### Functional Requirements

- **FR-001**: Script MUST identify the host NIC via environment variable (with hardcoded fallback default) and move it into the FRR network namespace for underlay connectivity, preserving its existing IP address configuration
- **FR-002**: Script MUST configure the BGP underlay session with the TOR (Top of Rack) switch using TOR IP and AS number from environment variables (with hardcoded defaults), without authentication
- **FR-003**: Script MUST wait for FRR container to be in running state before attempting network configuration
- **FR-004**: Script MUST verify FRR is operational and responsive before proceeding (not just container running)
- **FR-005**: Script MUST create L3VPN with VNI 100, VRF name "red", and VXLAN port 4789
- **FR-006**: Script MUST create L2VPN with VNI 210, VRF name "red", and VXLAN port 4789
- **FR-007**: Script MUST configure L2VPN gateway IP as 192.168.110.1/24
- **FR-008**: Script MUST enslave the veth interface created for L2VPN to the br0 linux bridge
- **FR-009**: Script MUST extract the last octet from br0's IP address and use it for the VTEP IP configuration
- **FR-010**: Script MUST configure VTEP IP as 10.0.0.X/24 where X is the last octet from br0's IP address
- **FR-011**: Script MUST use utility functions from systemdmode/common.sh for namespace operations
- **FR-012**: Script MUST log informational messages for each major configuration step
- **FR-013**: Script MUST log error messages with specific diagnostic information when failures occur
- **FR-014**: Script MUST exit with non-zero status code on any configuration failure without attempting rollback (development/testing environment)
- **FR-015**: Script MUST exit with zero status code when configuration succeeds
- **FR-016**: Script MUST have a configurable timeout for waiting on FRR readiness (default: 60 seconds)
- **FR-017**: Utility functions in common.sh MUST provide ability to execute commands inside FRR network namespace
- **FR-018**: Utility functions in common.sh MUST provide ability to execute commands outside FRR network namespace (host namespace)
- **FR-019**: System MUST provide a rawfrrconfigs entry in the static YAML configuration containing the complete EVPN configuration for both L2 and L3 VPNs
- **FR-020**: Raw FRR configuration in rawfrrconfigs MUST be equivalent to the configuration generated by the CRD-based L2VNI and L3VNI controllers

### Key Entities

- **Underlay Network**: Physical network infrastructure connecting nodes to the TOR switch, provides the transport layer for VXLAN overlay traffic. Requires moving host NIC into FRR namespace and establishing BGP session with TOR.
- **Host NIC**: Physical network interface that will be moved from the host namespace into the FRR network namespace to provide underlay connectivity for BGP peering and VXLAN encapsulation.
- **TOR Switch Session**: BGP peering session established between the FRR router and the Top of Rack switch, enables route advertisement for EVPN and underlay reachability.
- **L3VPN Configuration**: Represents a layer 3 VXLAN network for routing traffic. Key attributes: VNI (100), VRF name ("red"), VXLAN port (4789), associated with EVPN type 5 routes.
- **L2VPN Configuration**: Represents a layer 2 VXLAN network for bridging traffic. Key attributes: VNI (210), VRF name ("red"), VXLAN port (4789), gateway IP (192.168.110.1/24), bridge attachment (br0).
- **VTEP IP Configuration**: VXLAN Tunnel Endpoint IP address derived from br0's IP address. Uses 10.0.0.X/24 CIDR where X matches the last octet of br0's IP. For example, if br0 is 192.168.1.3/24, VTEP IP is 10.0.0.3/24.
- **FRR Container**: The Free Range Routing daemon container running in systemd mode, provides the routing control plane for VPN networks.
- **RawFRRConfigs Entry**: Raw FRR configuration embedded in the static YAML config, containing complete EVPN BGP setup for L2 and L3 VPNs, processed by the perouter controller and rendered to FRR configuration files, equivalent to CRD-generated configuration.
- **Network Namespace**: Isolated network stack where FRR container operates, requires special utilities to execute commands within it.
- **br0 Bridge**: Linux bridge to which the L2VPN veth interface must be attached for layer 2 connectivity. Its IP address determines the last octet for VTEP IP assignment.

## Success Criteria

### Measurable Outcomes

- **SC-001**: Underlay setup (host NIC move and TOR BGP session) completes successfully within 30 seconds from script start
- **SC-002**: VPN setup completes successfully within 60 seconds from the time FRR container becomes ready
- **SC-003**: Both L2VPN and L3VPN configurations persist and remain operational across system reboots without requiring manual intervention
- **SC-004**: BGP session with TOR switch (kindswitch in clab deployment) is established and shows "Established" state
- **SC-005**: EVPN routes of type 2 (L2VPN MAC-IP routes) are advertised and received successfully
- **SC-006**: EVPN routes of type 5 (L3VPN IP Prefix routes) are advertised and received successfully
- **SC-007**: Script executes successfully as a systemd unit with 100% success rate when FRR container is healthy and all prerequisites are met
- **SC-008**: When setup fails, administrators can identify the root cause from log messages within 2 minutes of reviewing systemd journal

### Validation Method

The feature will be validated using containerlab (clab) deployment:

**Test Setup**:
1. Deploy containerlab topology with multiple kind nodes and a kindswitch acting as TOR
2. Configure each kind node with a linux bridge br0 with unique IP addresses (e.g., 192.168.1.2/24, 192.168.1.3/24, etc.)
3. Run the setup script as a systemd unit on each kind node

**Validation Criteria**:
1. Script successfully configures FRR on each node
2. Script creates proper network interfaces (veth pairs, VXLAN interfaces)
3. Script moves designated host NIC into FRR network namespace
4. BGP session between each node's FRR and the kindswitch shows "Established" state
5. EVPN route type 2 (MAC-IP advertisement for L2VPN) routes are present in the routing table
6. EVPN route type 5 (IP Prefix advertisement for L3VPN) routes are present in the routing table
7. Routes are correctly exchanged bidirectionally between all nodes through the kindswitch

**Validation Commands**:
- Check BGP session status: `vtysh -c "show bgp summary"`
- Verify EVPN type 2 routes: `vtysh -c "show bgp l2vpn evpn route type 2"`
- Verify EVPN type 5 routes: `vtysh -c "show bgp l2vpn evpn route type 5"`
- Verify namespace and interfaces: `ip netns exec <frr-ns> ip link show`

## Assumptions

- FRR container is configured to run under systemd using quadlet/podman configuration
- A designated physical host NIC is available for underlay connectivity; NIC name can be specified via environment variable or falls back to hardcoded default
- The designated host NIC is already configured with an IP address on the host before the script runs; this IP configuration is preserved when moved to FRR namespace
- TOR switch (kindswitch in containerlab deployment) is network-reachable from the host NIC and configured to accept BGP peering without authentication
- BGP session parameters (AS number, neighbor IP) for TOR switch are provided via environment variables with hardcoded fallback defaults
- For validation testing, containerlab topology includes kind nodes with FRR containers and a kindswitch acting as TOR
- The br0 linux bridge already exists with an assigned IP address before VPN setup runs
- The last octet of br0's IP address uniquely identifies this node in the VXLAN overlay network
- Network namespace for FRR container is created and managed by the container runtime (podman)
- VNI values (100 for L3, 210 for L2) and VRF name ("red") are fixed configuration parameters, not dynamically configurable
- The script will be invoked as a systemd oneshot service that runs once after FRR container starts
- Standard Linux networking tools (ip, bridge, veth commands) are available on the host system
- The script has sufficient privileges to configure network interfaces and namespaces (runs as root or with appropriate capabilities)
- Existing Kubernetes-based L2VNI and L3VNI CRD controllers demonstrate the required network configuration commands, which can be adapted for direct shell scripting
- Raw FRR configuration file will be consumed by the perouter component and must match the BGP EVPN configuration that the CRD controllers would generate
- The perouter is capable of loading and applying raw FRR configuration files for EVPN setup
