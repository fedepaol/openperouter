// SPDX-License-Identifier:Apache-2.0

package webhooks

import (
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/metallb/frr-k8s/api/v1beta1"
	"github.com/openperouter/openperouter/api/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const TestNamespace = "test-namespace"

var existingConfig = v1beta1.FRRConfiguration{
	ObjectMeta: metav1.ObjectMeta{
		Name:      "test-config",
		Namespace: TestNamespace,
	},
}

func TestValidateVNI(t *testing.T) {
	toRestore := getVNIs

	defer func() {
		getVNIs = toRestore
	}()

	tests := []struct {
		desc         string
		before       *v1alpha1.VNI
		vni          *v1alpha1.VNI
		isNew        bool
		failValidate bool
		expected     *v1alpha1.VNIList
		warnings     []string
	}{}
	/*
		{
			desc:   "Second config",
			before: &existingConfig,
			config: &v1beta1.FRRConfiguration{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test",
					Namespace: TestNamespace,
				},
			},
			isNew: true,
			expected: &v1beta1.FRRConfigurationList{
				Items: []v1beta1.FRRConfiguration{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "test-config",
							Namespace: TestNamespace,
						},
					},
					{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "test",
							Namespace: TestNamespace,
						},
					},
				},
			},
		},
	*/
	for _, test := range tests {
		t.Run(test.desc, func(t *testing.T) {
			var err error
			getVNIs = func() (*v1alpha1.VNIList, error) {
				return &v1alpha1.VNIList{
					Items: []v1alpha1.VNI{
						*test.before,
					},
				}, nil
			}
			mock := &mockValidator{}
			ValidateVNIs = mock.ValidateVNIs
			mock.forceError = test.failValidate

			var warnings []string

			if test.isNew {
				warnings, err = validateVNICreate(test.vni)
			} else {
				warnings, err = validateVNIUpdate(test.vni)
			}
			if test.failValidate && err == nil {
				t.Fatalf("test %s failed, expecting error", test.desc)
			}

			if !cmp.Equal(test.warnings, warnings) {
				t.Fatalf("admission warning message don't match: expected '%#v', got '%#v'", test.warnings, warnings)
			}

			if !cmp.Equal(test.expected, mock.vnis) {
				t.Fatalf("test %s failed, %s", test.desc, cmp.Diff(test.expected, mock.vnis))
			}
		})
	}
}
