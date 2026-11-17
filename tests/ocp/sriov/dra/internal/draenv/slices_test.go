package draenv_test

import (
	"testing"

	"github.com/rh-ecosystem-edge/eco-gotests/tests/ocp/sriov/dra/internal/draenv"
	resourcev1 "k8s.io/api/resource/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func sample(name string, generation, total int64, devices int) resourcev1.ResourceSlice {
	node, attribute := "worker-a", "openshift.io/test"

	slice := resourcev1.ResourceSlice{
		ObjectMeta: metav1.ObjectMeta{Name: name},
		Spec: resourcev1.ResourceSliceSpec{
			Driver: draenv.Driver, NodeName: &node,
			Pool: resourcev1.ResourcePool{Name: "pool-a", Generation: generation, ResourceSliceCount: total},
		},
	}
	for i := 0; i < devices; i++ {
		slice.Spec.Devices = append(slice.Spec.Devices, resourcev1.Device{
			Attributes: map[resourcev1.QualifiedName]resourcev1.DeviceAttribute{
				resourcev1.QualifiedName(draenv.ResourceAttribute): {StringValue: &attribute},
			},
		})
	}

	return slice
}

func TestCountDevices(t *testing.T) {
	cases := []struct {
		name    string
		slices  []resourcev1.ResourceSlice
		want    int
		wantErr bool
	}{
		{"empty", nil, 0, false},
		{"single pool", []resourcev1.ResourceSlice{sample("a", 1, 1, 2)}, 2, false},
		{"overlapping generations", []resourcev1.ResourceSlice{sample("old", 1, 1, 2), sample("new", 2, 1, 4)}, 4, false},
		{"complete split pool", []resourcev1.ResourceSlice{sample("a", 2, 2, 2), sample("b", 2, 2, 2)}, 4, false},
		{"incomplete newer generation", []resourcev1.ResourceSlice{sample("old", 1, 1, 4), sample("a", 2, 2, 2)}, 0, true},
		{"conflicting pool count", []resourcev1.ResourceSlice{sample("a", 2, 2, 2), sample("b", 2, 3, 2)}, 0, true},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			got, err := draenv.CountDevices(testCase.slices, "worker-a", "openshift.io/test")
			if (err != nil) != testCase.wantErr {
				t.Fatalf("error=%v, want error=%v", err, testCase.wantErr)
			}

			if !testCase.wantErr && got != testCase.want {
				t.Fatalf("count=%d want %d", got, testCase.want)
			}
		})
	}
}

func TestCountDevicesFiltersUnrelatedResources(t *testing.T) {
	valid := sample("valid", 1, 1, 2)
	unrelated := sample("other", 1, 1, 4)
	unrelated.Spec.Driver = "other.example"
	node := "worker-b"
	otherNode := sample("node", 1, 1, 4)
	otherNode.Spec.NodeName = &node
	otherResource := sample("resource", 1, 1, 4)
	otherResource.Spec.Pool.Name = "pool-b"

	value := "openshift.io/other"

	key := resourcev1.QualifiedName(draenv.ResourceAttribute)
	for i := range otherResource.Spec.Devices {
		otherResource.Spec.Devices[i].Attributes[key] = resourcev1.DeviceAttribute{
			StringValue: &value,
		}
	}

	count, err := draenv.CountDevices(
		[]resourcev1.ResourceSlice{valid, unrelated, otherNode, otherResource}, "worker-a", "openshift.io/test")
	if err != nil || count != 2 {
		t.Fatalf("count=%d err=%v, want 2", count, err)
	}
}
