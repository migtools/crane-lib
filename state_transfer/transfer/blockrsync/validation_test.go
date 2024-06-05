package blockrsync

import (
	"strings"
	"testing"

	"github.com/konveyor/crane-lib/state_transfer/transfer"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var (
	block      = corev1.PersistentVolumeBlock
	fileSystem = corev1.PersistentVolumeFilesystem
)

const (
	testPVCName = "test-pvc"
)

func TestIsBlockOrVM(t *testing.T) {
	if err := isBlockOrKubeVirtDisk(createPVC(testPVCName, testNamespace, &block)); err != nil {
		t.Errorf("isBlockOrKubeVirtDisk() should return nil, %v", err)
	}
	fsPvc := createPVC(testPVCName, testNamespace, &fileSystem)
	if err := isBlockOrKubeVirtDisk(fsPvc); err == nil {
		t.Errorf("isBlockOrKubeVirtDisk() should not return nil")
	}
	fsPvc.Annotations = map[string]string{
		kubeVirtAnnKey: kubevirtContentType,
	}
	if err := isBlockOrKubeVirtDisk(fsPvc); err != nil {
		t.Errorf("isBlockOrKubeVirtDisk() should return nil, %v", err)
	}
}

func TestValidatePVCName(t *testing.T) {
	pvcPair := transfer.NewPVCPair(createPVC(testPVCName, testNamespace, &block), createPVC(testPVCName, testNamespace, &block))
	if err := validatePVCName(pvcPair); err != nil {
		t.Errorf("validatePVCName() should return nil, %v", err)
	}
	pvcPair = transfer.NewPVCPair(createPVC(testPVCName, testNamespace, &fileSystem), createPVC("test-pvc-2", testNamespace, &fileSystem))
	if err := validatePVCName(pvcPair); err == nil {
		t.Errorf("validatePVCName() should not return nil")
	}

	pvcPair = &testPVCPair{
		source: &testPVC{
			label: strings.Repeat("a", 64),
			pvc:   createPVC(testPVCName, testNamespace, &block),
		},
		dest: &testPVC{
			label: strings.Repeat("a", 64),
			pvc:   createPVC("test-pvc-2", testNamespace, &block),
		},
	}
	if err := validatePVCName(pvcPair); err == nil {
		t.Errorf("validatePVCName() should not return nil")
	}
}

func TestValidatePVCList(t *testing.T) {
	pvcList := transfer.PVCPairList{
		&testPVCPair{
			source: &testPVC{
				label: testPVCName,
				pvc:   createPVC(testPVCName, testNamespace, &block),
			},
			dest: &testPVC{
				label: testPVCName,
				pvc:   createPVC(testPVCName, testNamespace, &block),
			},
		},
	}
	if err := validatePVCList(pvcList); err != nil {
		t.Errorf("validatePVCList() should return nil, %v", err)
	}

	pvcList = transfer.PVCPairList{
		&testPVCPair{
			source: &testPVC{
				label: testPVCName,
				pvc:   createPVC(testPVCName, testNamespace, &block),
			},
			dest: &testPVC{
				label: testPVCName,
				pvc:   createPVC(testPVCName, testNamespace, &block),
			},
		},
		&testPVCPair{
			source: &testPVC{
				label: testPVCName,
				pvc:   createPVC(testPVCName, "test-namespace2", &block),
			},
			dest: &testPVC{
				label: testPVCName,
				pvc:   createPVC(testPVCName, "test-namespace2", &block),
			},
		},
	}
	if err := validatePVCList(pvcList); err == nil {
		t.Errorf("validatePVCList() should not return nil")
	}

	pvcList = transfer.PVCPairList{
		&testPVCPair{
			source: &testPVC{
				label: testPVCName,
				pvc:   createPVC(testPVCName, testNamespace, &fileSystem),
			},
			dest: &testPVC{
				label: testPVCName,
				pvc:   createPVC(testPVCName, testNamespace, &block),
			},
		},
	}
	if err := validatePVCList(pvcList); err == nil {
		t.Errorf("validatePVCList() should not return nil")
	}

	pvcList = transfer.PVCPairList{}
	if err := validatePVCList(pvcList); err == nil {
		t.Errorf("validatePVCList() should not return nil")
	}

}

func createPVC(name, namespace string, volumeMode *corev1.PersistentVolumeMode) *corev1.PersistentVolumeClaim {
	return &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: corev1.PersistentVolumeClaimSpec{
			AccessModes: []corev1.PersistentVolumeAccessMode{
				corev1.ReadWriteOnce,
			},
			VolumeMode: volumeMode,
		},
	}
}

type testPVCPair struct {
	source *testPVC
	dest   *testPVC
}

func (p *testPVCPair) Source() transfer.PVC {
	return p.source
}

func (p *testPVCPair) Destination() transfer.PVC {
	return p.dest
}

type testPVC struct {
	label string
	pvc   *corev1.PersistentVolumeClaim
}

func (p *testPVC) LabelSafeName() string {
	return p.label
}

func (p *testPVC) Claim() *corev1.PersistentVolumeClaim {
	return p.pvc
}
