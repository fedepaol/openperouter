```bash
                                                   64612                                    
                                               ┌───────────┐                                
                    ┌──────────────────────────┼           │                                
                    │                          │  Spine    ┼──────────────┐                 
                    │                   ┌──────┼           │              │                 
                    │                   │      └───────────┘              │                 
                    │                   │                                 │                 
                    │                   │                                 │                 
                    │                   │                                 │                 
                    │                   │                                 │                 
                    │                   │                                 │                 
             ┌──────┴────┐        ┌─────┴─────┐                     ┌─────┴─────┐           
             │           │        │           │                     │           │           
64520        │  Leaf A   │        │  Leaf B   │                     │  Leaf     │      64512
             │           │        │           │                     │  Kind     │           
             └──┬──────┬─┘        └─┬───────┬─┘                     └─┬───────┬─┘           
                │      │            │       │                         │       │             
           ┌────┴─┐  ┌─┴────┐  ┌────┴─┐  ┌──┴───┐                     │       │             
           │ Host │  │ Host │  │ Host │  │ Host │           ┌─────────┴─┐   ┌─┴─────────┐   
           │ Red  │  │ Blue │  │ Red  │  │ Blue │           │           │   │           │   
           └──────┘  └──────┘  └──────┘  └──────┘           │  Kind     │   │  Kind     │   
                                                            │  Worker   │   │  ControlP │   
                                                            └───────────┘   └───────────┘   

```

The interfaces are:

```
    - endpoints: ["leafA:eth1", "spine:eth1"]
    - endpoints: ["leafB:eth1", "spine:eth2"]
    - endpoints: ["leafkind:eth1", "spine:eth3"]
    - endpoints: ["leafA:ethred", "hostA_red:eth1"]
    - endpoints: ["leafA:ethblue", "hostA_blue:eth1"]
    - endpoints: ["leafB:ethred", "hostB_red:eth1"]
    - endpoints: ["leafB:ethblue", "hostB_blue:eth1"]
    - endpoints: ["leafkind:toswitch", "leafkind-switch:leaf2"]
    - endpoints: ["pe-kind-control-plane:toswitch", "leafkind-switch:kindctrlpl"]
    - endpoints: ["pe-kind-worker:toswitch", "leafkind-switch:kindworker"]
```

The ips are:

```
spine,eth1,192.168.1.0/31 
spine,eth2,192.168.1.2/31 
spine,eth3,192.168.1.4/31 
leafA,eth1,192.168.1.1/31
leafB,eth1,192.168.1.3/31
leafkind,eth1,192.168.1.5/31
leafkind,toswitch,192.168.11.2/24
pe-kind-control-plane,toswitch,192.168.11.3/24
pe-kind-worker,toswitch,192.168.11.4/24
leafA,ethred,192.168.20.1/24
leafB,ethred,192.168.21.1/24
hostA_red,eth1,192.168.21.1/24
hostB_red,eth1,192.168.21.1/24
leafA,ethblue,192.169.20.1/24
leafB,ethblue,192.169.21.1/24
hostA_blue,eth1,192.169.20.2/24
hostB_blue,eth1,192.169.21.2/24
```

