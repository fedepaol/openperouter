# Devenv: Declarative Topology Configuration

The `devenv` tool generates FRR configurations, IP allocations, setup scripts, and
topology state from a declarative environment configuration file. It replaces
manual IP assignment and FRR config authoring with automatic, deterministic
resource allocation.

Both the CLI and e2e tests use the same Go library (`pkg/devenv`), ensuring
consistent behavior.

## Building

```bash
make devenv
```

This produces `bin/devenv`.

## Environment Configuration File

An environment configuration file (e.g., `env.yaml`) declares the logical
behavior of a containerlab topology: which nodes are edge vs transit routers,
their ASNs, BGP settings, VRFs, and the IP ranges to allocate from.

Every node in the containerlab topology must be matched by exactly one
`node_groups` pattern. If a node is unmatched or matches multiple patterns,
the tool errors out.

### Format

```yaml
name: singlecluster                          # unique name for this configuration
topology_file: kind.clab.yml                 # path to the containerlab .clab.yml

allocation:
  link_subnet_base_v4: "192.168.1.0/24"     # base range for point-to-point /31 subnets
  link_subnet_base_v6: "fd00:1::/48"         # base range for point-to-point /127 subnets
  broadcast_subnet_base_v4: "192.168.11.0/24" # base range for broadcast /24 subnets
  broadcast_subnet_base_v6: "fd00:11::/48"   # base range for broadcast /64 subnets
  vtep_base: "100.64.0.0/24"                # base range for VTEP IPs (edge nodes)
  router_id_base: "10.200.0.0/24"           # base range for BGP router IDs

node_groups:
  - pattern: "leaf[AB]"        # regex matched against containerlab node names
    role: edge                 # "edge" (tunnel endpoint with VRFs) or "transit" (passthrough)
    asn: 64520                 # BGP AS number (0 = no BGP for this group)
    bgp:
      ipv4: true               # enable IPv4 address family
      ipv6: false              # enable IPv6 address family
      evpn: true               # enable L2VPN EVPN address family
      bfd: false               # enable BFD for BGP peers
    vrfs:                      # VRFs (edge nodes only)
      - name: red
        vni: 100
      - name: blue
        vni: 200

  - pattern: "spine"
    role: transit
    asn: 64612
    bgp:
      ipv4: true
      evpn: true

  # Nodes without BGP (hosts, bridges, external containers)
  # still need a matching group with asn: 0
  - pattern: "host.*"
    role: edge
    asn: 0
    bgp:
      ipv4: false
```

### Fields

| Field | Required | Description |
|-------|----------|-------------|
| `name` | yes | Unique name, used to derive the state file path |
| `topology_file` | yes | Path to the containerlab `.clab.yml` file |
| `allocation` | yes | IP range configuration for automatic allocation |
| `node_groups` | yes | List of groups; each group matches nodes by regex pattern |
| `node_groups[].pattern` | yes | Go regex pattern (anchored with `^...$` automatically) |
| `node_groups[].role` | yes | `edge` or `transit` |
| `node_groups[].asn` | yes | BGP AS number; set to `0` for non-BGP nodes |
| `node_groups[].bgp` | no | BGP address family flags |
| `node_groups[].vrfs` | no | VRF list (edge nodes only) |

### Validation Rules

- Every node in the containerlab topology must match exactly one pattern
- A pattern that matches zero nodes is an error
- A node matching multiple patterns is an error
- ASN is required when any BGP address family is enabled

## CLI Usage

### Generate and save configuration

```bash
# Generate configs from an environment file
bin/devenv apply --env clab/singlecluster/env.yaml
```

This will:
1. Parse the containerlab topology and environment config
2. Validate all nodes are covered by patterns
3. Allocate IPs, VTEPs, router IDs, and MACs
4. Generate FRR configs and setup scripts
5. Print a summary and save state to `.devenv-state-<name>.yaml`

### Re-apply from existing state

```bash
# Re-apply a previously generated configuration
bin/devenv apply --state .devenv-state-singlecluster.yaml
```

This loads the state and runs `clab deploy --reconfigure` to apply it.

### View configuration summary

```bash
# Human-readable summary
bin/devenv summary --state .devenv-state-singlecluster.yaml

# Machine-readable YAML output
bin/devenv summary --state .devenv-state-singlecluster.yaml --format yaml
```

### Query topology state

```bash
# Look up a node by name
bin/devenv query --state .devenv-state-singlecluster.yaml --node leafA

# Reverse lookup: find which node owns an IP
bin/devenv query --state .devenv-state-singlecluster.yaml --ip 192.168.1.1

# Find nodes by regex pattern
bin/devenv query --state .devenv-state-singlecluster.yaml --pattern "leaf.*"
```

## Using from Go (e2e tests)

The `pkg/devenv` package exposes the same functionality as the CLI. E2e tests
import it directly to avoid hardcoding topology parameters.

```go
import "github.com/openperouter/openperouter/pkg/devenv"

// Load from an existing state file
cfg, err := devenv.LoadFromState(".devenv-state-singlecluster.yaml")

// Or generate fresh from env config
cfg, err := devenv.LoadFromEnvConfig("clab/singlecluster/kind.clab.yml", "clab/singlecluster/env.yaml")

// Query - no hardcoded IPs
leafA, _ := cfg.NodeByName("leafA")
fmt.Println(leafA.VTEPIP)

ipv4A, ipv4B, _, _, _ := cfg.LinkIPsBetween("leafA", "spine")

nodeName, ifaceName, _ := cfg.NodeByIP("192.168.1.1")

// Edit at runtime
cfg.AddVRF("leafA", devenv.VRF{Name: "green", VNI: 300})

// Apply changes to running topology
cfg.Reconfigure()
```

A convenience helper is available in `pkg/devenv/devenvtest`:

```go
import "github.com/openperouter/openperouter/pkg/devenv/devenvtest"

// Loads from state if it exists, otherwise generates from env config
cfg, err := devenvtest.LoadOrCreate(
    "clab/singlecluster/kind.clab.yml",
    "clab/singlecluster/env.yaml",
    ".devenv-state-singlecluster.yaml",
)
```

## Multiple Topology Variations

You can create multiple environment configs for the same containerlab topology.
Each produces a separate state file named after the `name` field:

```bash
bin/devenv apply --env evpn-env.yaml     # -> .devenv-state-evpn.yaml
bin/devenv apply --env srv6-env.yaml     # -> .devenv-state-srv6.yaml
```

## How It Works

1. **Parse** the containerlab `.clab.yml` to discover nodes and links
2. **Match** each node to a `node_groups` pattern using Go regex
3. **Allocate** IPs (/31 for p2p links, /24 for broadcast), VTEPs, router IDs, and MACs sequentially from the configured base ranges. Allocations are deterministic (sorted by node/link name) so re-running produces identical results
4. **Generate** FRR configuration from templates (edge nodes get BGP + EVPN + VRF config; transit nodes get BGP only)
5. **Generate** setup scripts for edge nodes (VTEP IP, VRF creation, VXLAN bridge setup)
6. **Persist** the full state to a YAML file
7. **Reconfigure** applies changes by writing configs to disk and running `clab deploy --reconfigure`
