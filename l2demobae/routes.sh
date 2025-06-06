#!/bin/bash
# add local gw

# remove default route via eth0
kubectl get pods -o name | xargs -I {} kubectl exec {} -- ip route del default dev eth0
