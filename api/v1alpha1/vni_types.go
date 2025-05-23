/*
Copyright 2024.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// VNISpec defines the desired state of VNI.
type VNISpec struct {
	// ASN is the local AS number to use to establish a BGP session with
	// the default namespace.
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=4294967295
	// +required
	ASN uint32 `json:"asn,omitempty"`

	// VRF is the name of the linux VRF to be used inside the PERouter namespace.
	// The field is optional, if not set it the name of the VNI instance will be used.
	// +kubebuilder:validation:Pattern=`^[a-zA-Z][a-zA-Z0-9_-]*$`
	// +kubebuilder:validation:MaxLength=15
	// +optional
	VRF *string `json:"vrf,omitempty"`

	// ASN is the expected AS number for a BGP speaking component running in
	// the default network namespace. If not set, the ASN field is going to be used.
	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:validation:Maximum=4294967295
	// +optional
	HostASN *uint32 `json:"hostasn,omitempty"`

	// VNI is the VXLan VNI to be used
	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:validation:Maximum=4294967295
	// +optional
	VNI uint32 `json:"vni,omitempty"`

	// LocalCIDR is the CIDR to be used for the veth pair
	// to connect with the default namespace. The interface under
	// the PERouter side is going to use the first IP of the cidr on all the nodes.
	LocalCIDR string `json:"localcidr,omitempty"`

	// VXLanPort is the port to be used for VXLan encapsulation.
	// +kubebuilder:default:=4789
	VXLanPort uint32 `json:"vxlanport,omitempty"`
}

// VNIStatus defines the observed state of VNI.
type VNIStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status

// VNI represents a VXLan VNI to receive EVPN type 5 routes
// from.
type VNI struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   VNISpec   `json:"spec,omitempty"`
	Status VNIStatus `json:"status,omitempty"`
}

// VRFName returns the name to be used for the
// vrf corresponding to the object.
func (v VNI) VRFName() string {
	if v.Spec.VRF != nil && *v.Spec.VRF != "" {
		return *v.Spec.VRF
	}
	return v.Name
}

// +kubebuilder:object:root=true

// VNIList contains a list of VNI.
type VNIList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []VNI `json:"items"`
}

func init() {
	SchemeBuilder.Register(&VNI{}, &VNIList{})
}
