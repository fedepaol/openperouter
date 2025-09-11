package grpc

func (vni *L3VNI) VRFName() string {
	if vni.Vrf != "" {
		return vni.Vrf
	}
	return vni.Key()
}

func (vni *L2VNI) VRFName() string {
	if vni.Vrf != "" {
		return vni.Vrf
	}
	return vni.Key()
}

func (bfd *BFDSettings) IsEmpty() bool {
	if bfd == nil {
		return true
	}
	return bfd.ReceiveInterval == 0 &&
		bfd.TransmitInterval == 0 &&
		bfd.DetectMultiplier == 0 &&
		bfd.EchoInterval == 0 &&
		!bfd.EchoMode &&
		!bfd.PassiveMode &&
		bfd.MinimumTtl == 0
}
