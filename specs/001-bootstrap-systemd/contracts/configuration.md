# Bootstrap Configuration Contract

Status: proposed version-one interface for implementation. This document does not imply that `/bootstrap` currently exists.

## Document shape

```yaml
version: 1
deployment:
  image: quay.io/openperouter/router:main
  datapath: kernel
  logLevel: info
  ovsSocketPath: unix:/var/run/openvswitch/db.sock
  ovsRunDir: /var/run/openvswitch
  healthProbePort: 9081
  bgpListenLimit: 65535
  startupTimeoutSeconds: 300
node:
  nodeIndex:
    index: 0
routing:
  directory: /etc/openperouter/routing
```

Root keys are exactly `version`, `deployment`, `node`, and `routing`, all required. Reject unknown and duplicate fields throughout the complete document, null for typed fields, unsupported versions, and more than one YAML document. Parse embedded routing through the existing static model with strict field recognition, including its embedded structs; do not use a decoder that ignores their YAML tags.

Input and referenced directories are host paths. The file is root-owned and not writable by non-administrators; configuration may contain static BGP authentication data. Do not echo full YAML in diagnostics. Absolute paths must be clean and must not contain control characters or systemd specifier injection. Escape unit values with unit syntax rather than shell quoting.

## Deployment fields

| Field | Type | Default and validation |
|-------|------|------------------------|
| `image` | string, required | Fully qualified locally available image reference. Resolve to local image ID; require packaged controller, FRR, reloader, CNI, and bootstrap executables. Grout additionally requires compatible Grout, grcli, and FRR plugin. One image serves all runtime components. |
| `datapath` | string | `kernel`; enum `kernel`, `grout`. |
| `logLevel` | string | `info`; enum `all`, `debug`, `info`, `warn`, `error`, `none`. Runtime supports all six before acceptance. |
| `ovsSocketPath` | string | `unix:/var/run/openvswitch/db.sock`; initially require a local absolute Unix socket address consistent with the mounted ovsRunDir when OVS is used. |
| `ovsRunDir` | absolute path | `/var/run/openvswitch`; validate access when routing uses OVS. |
| `healthProbePort` | integer | 9081; 1–65535. Reject collision with required host services. |
| `bgpListenLimit` | integer | 65535; 1–65535. |
| `controllerMaxReconcileBackoffSeconds` | optional integer | Omitted uses controller's default limiter; supplied value at least 1. Explicit zero and null fail. |
| `startupTimeoutSeconds` | integer | 300; 30–1800. Total apply deadline. |
| `grout` | optional object | Allowed only with datapath grout; fields below. |

The seven overlapping operator CRD fields preserve their public names and permitted scalar values. Path constraints are additional host-access validation, not a change to operator semantics. `image`, startup timeout, and detailed grout fields are host deployment controls, not CRD fields.

`nodeSelector`, `tolerations`, `affinity`, `priorityClassName`, and `resources` fail as unsupported fields. No Kubernetes object wrapper (`apiVersion`, `kind`, `metadata`, `spec`) is accepted. No arbitrary CLI argument, environment variable, shell command, or unit-directive passthrough is provided.

## Grout fields

| Field | Type | Default and validation |
|-------|------|------------------------|
| `tapUnderlay` | boolean | false; false selects PCI, true selects TAP. |
| `maxMTU` | integer | 0; nonnegative, zero delegates to installed Grout default. Explicit positive value must accommodate configured underlay MTUs and the image's supported limits. |
| `testMode` | boolean | false; optional development smoke mode using existing Grout test behavior. Does not count as production forwarding proof. |

A production Grout deployment must satisfy actual hugepage, memory, interface, driver, and device access requirements. TAP does not implicitly turn on testMode. PCI does not universally require rebinding to VFIO: bifurcated drivers keep their supported binding. Preflight inspects but does not alter those devices. Both mode selections propagate to controller and daemon configuration.

## Node fields

- `nodeName`: optional string; omit to preserve existing host-derived node identity behavior.
- `nodeIndex`: required object. Exactly one of `index` (nonnegative integer, including zero) or `interfaceName` (nonempty string) is supplied. Optional `cidr` is a valid CIDR and requires interfaceName.
- Interface-derived index uses existing IPv4-preferred/IPv6-fallback selection and CIDR filtering against host interfaces. Missing interface/address is a preflight error; retry after host preparation. The configured method is preserved for later boots.
- `node.logLevel` is not accepted. Generate existing node-config logLevel from deployment.logLevel to avoid contradictory sources.

## Routing fields

Exactly one field is present:

- `inline`: an existing static `PERouterConfig` object. Supported keys are `underlays`, `l2vnis`, `l3vnis`, `l3vpns`, `bgppassthrough`, and `rawfrrconfigs`. Validate all resources and cross-references through existing conversion/domain validators before runtime mutation. Render as generated `openpe_bootstrap.yaml`; changes require bootstrap reapplication.
- `directory`: a clean absolute administrator-owned host directory containing existing `openpe_*.yaml` files. Validate matching files before activation, mount the directory directly, and preserve existing runtime reload on file creation/replacement/removal. Do not snapshot or rewrite files. An empty/missing directory fails bootstrap; directory identity changes require reapplication.

Inline and directory together fail rather than apply precedence. Empty inline configuration is valid only if existing static semantics permit it; it does not satisfy routing connectivity acceptance by itself. Secrets retain the existing static configuration representation and protected host access.

## Grout example

```yaml
version: 1
deployment:
  image: quay.io/openperouter/router:main-grout
  datapath: grout
  logLevel: debug
  healthProbePort: 9181
  bgpListenLimit: 4096
  controllerMaxReconcileBackoffSeconds: 30
  grout:
    tapUnderlay: true
    maxMTU: 9500
    testMode: false
node:
  nodeIndex:
    interfaceName: eth0
    cidr: 192.168.111.0/24
routing:
  directory: /etc/openperouter/routing
```

For PCI, set tapUnderlay false and supply existing supported accelerated-interface routing definitions and prepared hardware. Example addresses/interface names are topology-specific, not portable defaults.
