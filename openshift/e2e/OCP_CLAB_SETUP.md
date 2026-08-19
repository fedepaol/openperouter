# OCP Clab Setup — Step-by-Step Justification

Every step in `setup-clab.sh` mapped against its upstream kind equivalent, with an explanation of why OCP needs it or why OCP does it differently.

## Upstream Kind Pipeline

The kind CI runs `clab/setup.sh` which orchestrates scripts `00` through `10`:

| Script | What | OCP Equivalent |
|--------|------|----------------|
| `00-environment.sh` | Creates bridges, checks for kind binary | **Not needed**: dev-scripts creates libvirt bridges via `EXTRA_NETWORK_NAMES` |
| `01-registry.sh` | Starts local Docker registry for kind | **Not needed**: OCP uses real registries |
| `02-leaf-configs.sh` | Generates FRR configs for leafA, leafB, leafkind1, leafkind2 | **Step 2**: Same — generates identical FRR configs |
| `03-kind-configs.sh` | Generates kind cluster YAML | **Not needed**: OCP provisioned by dev-scripts |
| `04-containerlab-deploy.sh` | Deploys clab topology | **Step 4**: Same — deploys `ocp.clab.yml` instead of `kind.clab.yml` |
| `05-load-images.sh` | Loads images into kind nodes | **Not needed**: OCP pulls from registries |
| `06-kubeconfig-setup.sh` | Extracts kubeconfig from kind | **Not needed**: kubeconfig from dev-scripts |
| `07-frr-k8s-setup.sh` | Deploys frr-k8s + Multus + copies CNI binaries | **Not needed**: OCP enables frr-k8s via CNO, Multus built-in |
| `08-ip-assignment.sh` | Assigns IPs to clab containers + kind nodes | **Steps 5+8**: Assigns IPs to clab containers AND OCP node NICs separately |
| `09-container-setup.sh` | Runs `/setup.sh` in clab containers | **Step 6**: Same |
| `10-veth-monitoring.sh` | Starts `check_veths` daemon | **Not needed**: OCP uses real NICs, no veths to monitor |

## setup-clab.sh Steps

### Step 1: Disable DHCP on extra networks
```bash
virsh net-update "${net}" delete ip-dhcp-range "${DHCP_RANGE}" --live --config
```
**Kind equivalent:** None — kind bridges don't run DHCP.

**Why OCP needs this:** dev-scripts enables dnsmasq DHCP on each libvirt network by default. DHCP IPs compete with our static assignments and expire after 60 minutes, silently breaking routing mid-test. We use `virsh net-update` to remove the DHCP range from the live network (`--live`) and persist the change across libvirt restarts (`--config`).

### Step 2: Generate FRR configs
```bash
./generate_leaf_config/generate_leafkind -leaf singlecluster/leafkind1 -asn 64512 ...
```
**Kind equivalent:** `02-leaf-configs.sh` — identical commands, identical parameters.

**Why same:** The fabric topology (leafA, leafB, spine, leafkind1, leafkind2) is the same on both platforms. LeafA/leafB configs are gitignored and must be generated. The leafkind FRR configs use `interface toswitch1` for IS-IS and `bgp listen range 192.168.11.0/24` for dynamic peers — both work identically because the clab endpoint naming matches (`leafkind1:toswitch1`).

### Step 3: Enable podman socket
```bash
systemctl enable --now podman.socket
```
**Kind equivalent:** None — kind uses Docker, socket is always running.

**Why OCP needs this:** containerlab with `--runtime podman` requires the podman socket. RHEL/RHCOS doesn't enable it by default.

### Step 4: Deploy clab topology
```bash
containerlab deploy --runtime podman --topo ocp.clab.yml --reconfigure
```
**Kind equivalent:** `04-containerlab-deploy.sh` — same command, different topology file and runtime.

**Why OCP is different:** Uses `ocp.clab.yml` (libvirt bridge references, no kind nodes) instead of `kind.clab.yml`. Uses `--runtime podman` instead of Docker.

### Step 5: Assign IPs to clab containers + MTU fix
```bash
go run tools/assign_ips/assign_ips.go -file ip_map_ocp.txt -engine "sudo podman"
ip link set dev toswitch1 mtu 1500   # on leafkind1/2
```
**Kind equivalent:** `08-ip-assignment.sh` — same `assign_ips` tool, different ip_map file (`ip_map.txt` vs `ip_map_ocp.txt`).

**Why OCP is different:**
- **ip_map_ocp.txt** — same as kind's `ip_map.txt` for spine/leaf/host IPs and leafkind toswitch IPs, but excludes kind node IPs (those are assigned in step 8 via nsenter, not via clab exec).
- **MTU fix** — clab creates veths at MTU 9500 by default. The libvirt bridge is MTU 1500. If the leafkind toswitch1 veth doesn't auto-negotiate to the bridge MTU, IS-IS PDUs get dropped. The `mtu 1500` ensures they match.

### Step 6: Run container setup scripts
```bash
for c in leafA leafB leafSRV6 hostA_red ...; do
    podman exec clab-kind-${c} /setup.sh
done
```
**Kind equivalent:** `09-container-setup.sh` — identical logic, iterates over same containers.

**Why same:** The clab containers (leafA, leafB, hosts, leafSRV6) are the same on both platforms. Their setup scripts configure loopback IPs, VRFs, routes, SRV6 locators — all fabric-internal, unrelated to the K8s platform.

### Step 7: Rename NICs + udev rules
```bash
virsh domiflist "${vm_name}" | grep "toswitch${i}" | awk '{print $5}'  # get MAC
echo 'SUBSYSTEM=="net", ATTR{address}=="$MAC", NAME="toswitch${i}"' > /etc/udev/rules.d/70-toswitch${i}.rules
ip link set $IFACE name toswitch${i}
```
**Kind equivalent:** None — kind nodes are containers. Clab names their veths directly in the topology YAML (`pe-kind-worker:toswitch1`).

**Why OCP needs this:** Libvirt assigns virtio NICs with kernel-generated names (enp3s0, enp4s0). The names are unpredictable and change across reboots. Tests and the underlay CR reference `toswitch1`/`toswitch2` by name. Udev rules persist the rename across reboots. `nmcli device set managed no` prevents NetworkManager from reconfiguring the NICs.

### Step 8: Assign static IPs + generate nodelink.json
```bash
ip addr replace ${TS1_V4}/24 dev toswitch1   # on OCP nodes
ip -6 addr add ${TS1_V6}/64 dev toswitch1
```
**Kind equivalent:** `08-ip-assignment.sh` assigns IPs to kind nodes via `docker exec pe-kind-worker ip addr add ...`. The IP map file contains the kind node entries.

**Why OCP is different:** OCP nodes are VMs, not containers. We can't `docker exec` into them. We use `nsenter` via controller pods to run commands in the node's host netns. The nodelink.json is also generated here (kind's `nodelink-default.json` is static/checked-in because kind always has the same nodes with the same IPs).

## Files

| File | Purpose |
|------|---------|
| `setup-clab.sh` | Main setup script — all steps above |
| `ocp.clab.yml` | Clab topology — same fabric as kind but with libvirt bridge references |
| `ip_map_ocp.txt` | IP assignments for clab containers (spine, leafA/B, leafkind, hosts, leafSRV6) |
| `README.md` | Quick-start for running tests |
| `OCP_CLAB_SETUP.md` | This file |
