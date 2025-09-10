package grpc

import "fmt"

type Keyer interface {
	Key() string
}

func (l *L2VNI) Key() string {
	return fmt.Sprintf("l2vni-%d", l.Vni)
}

func (l *L3VNI) Key() string {
	return fmt.Sprintf("l3vni-%d", l.Vni)
}

func (l *L3Passthrough) Key() string {
	return "l3passthrough"
}

func (u *Underlay) Key() string {
	nic := "empty"
	if len(u.Nics) > 1 {
		nic = u.Nics[0]
	}
	return fmt.Sprintf("underlay-%s", nic)
}
