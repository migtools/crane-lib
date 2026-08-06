package indirect

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
)

func TestBuildRcloneCommand(t *testing.T) {
	tests := []struct {
		name       string
		subcommand string
		src        string
		dst        string
		wantLen    int
		wantFirst  string
	}{
		{
			name:       "sync upload",
			subcommand: "sync",
			src:        "/data",
			dst:        "remote:bucket/ns/pvc",
			wantLen:    9,
			wantFirst:  "rclone",
		},
		{
			name:       "sync download",
			subcommand: "sync",
			src:        "remote:bucket/ns/pvc",
			dst:        "/data",
			wantLen:    9,
			wantFirst:  "rclone",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := buildRcloneCommand(tt.subcommand, tt.src, tt.dst)
			if len(got) != tt.wantLen {
				t.Errorf("length = %d, want %d, got %v", len(got), tt.wantLen, got)
			}
			if got[0] != tt.wantFirst {
				t.Errorf("first arg = %q, want %q", got[0], tt.wantFirst)
			}
			if got[1] != tt.subcommand {
				t.Errorf("subcommand = %q, want %q", got[1], tt.subcommand)
			}
			if got[2] != tt.src {
				t.Errorf("src = %q, want %q", got[2], tt.src)
			}
			if got[3] != tt.dst {
				t.Errorf("dst = %q, want %q", got[3], tt.dst)
			}
		})
	}
}

func TestBuildPod(t *testing.T) {
	transfer := New(nil, nil, Options{
		Image:        "test-image:latest",
		ConfigSecret: "my-rclone-secret",
		CloudStorage: "remote:my-bucket",
		Labels: map[string]string{
			"app": "test",
		},
	})

	pvcName := "test-pvc"
	command := []string{"rclone", "sync", "/data", "remote:bucket/ns/pvc"}
	pod := transfer.buildPod("test-upload", "test-ns", pvcName, command, corev1.PodSecurityContext{})

	if pod.Name != "test-upload" {
		t.Errorf("pod name = %q, want %q", pod.Name, "test-upload")
	}
	if pod.Namespace != "test-ns" {
		t.Errorf("pod namespace = %q, want %q", pod.Namespace, "test-ns")
	}
	if len(pod.Spec.Containers) != 1 {
		t.Fatalf("containers = %d, want 1", len(pod.Spec.Containers))
	}
	if pod.Spec.Containers[0].Image != "test-image:latest" {
		t.Errorf("image = %q, want %q", pod.Spec.Containers[0].Image, "test-image:latest")
	}
	if len(pod.Spec.Volumes) != 2 {
		t.Fatalf("volumes = %d, want 2", len(pod.Spec.Volumes))
	}
	if pod.Spec.Volumes[0].PersistentVolumeClaim.ClaimName != pvcName {
		t.Errorf("pvc claim = %q, want %q", pod.Spec.Volumes[0].PersistentVolumeClaim.ClaimName, pvcName)
	}
	if pod.Spec.Volumes[1].Secret.SecretName != "my-rclone-secret" {
		t.Errorf("secret name = %q, want %q", pod.Spec.Volumes[1].Secret.SecretName, "my-rclone-secret")
	}
}

func TestBuildPodLabelsAreCopied(t *testing.T) {
	labels := map[string]string{"app": "test"}
	transfer := New(nil, nil, Options{Labels: labels})

	pod1 := transfer.buildPod("pod1", "ns", "pvc", []string{"echo"}, corev1.PodSecurityContext{})
	pod2 := transfer.buildPod("pod2", "ns", "pvc", []string{"echo"}, corev1.PodSecurityContext{})

	pod1.Labels["extra"] = "modified"
	if _, found := pod2.Labels["extra"]; found {
		t.Error("modifying pod1 labels should not affect pod2 labels")
	}
}

func TestTruncatePodName(t *testing.T) {
	short := "rclone-upload-mydata"
	if got := truncatePodName(short); got != short {
		t.Errorf("short name should not be truncated, got %q", got)
	}

	long := "rclone-upload-a-very-long-pvc-name-that-exceeds-sixty-three-characters-limit"
	got := truncatePodName(long)
	if len(got) > maxPodNameLen {
		t.Errorf("long name should be truncated to %d, got %d", maxPodNameLen, len(got))
	}

	endsWithHyphen := "rclone-upload-a-very-long-pvc-name-that-exceeds-sixty-three-cha"
	if len(endsWithHyphen) <= maxPodNameLen {
		endsWithHyphen = endsWithHyphen + "racters-and-ends-with-hyphen-"
	}
	got = truncatePodName(endsWithHyphen)
	lastChar := got[len(got)-1]
	if lastChar == '-' || lastChar == '.' {
		t.Errorf("truncated name should not end with hyphen or dot, got %q", got)
	}
}

func TestDefaultImage(t *testing.T) {
	transfer := New(nil, nil, Options{})
	if transfer.options.Image != defaultImage {
		t.Errorf("default image = %q, want %q", transfer.options.Image, defaultImage)
	}
}

func TestDefaultLabels(t *testing.T) {
	transfer := New(nil, nil, Options{})
	if transfer.options.Labels["app.kubernetes.io/name"] != "crane" {
		t.Errorf("missing default label app.kubernetes.io/name")
	}
	if transfer.options.Labels["app.kubernetes.io/component"] != "indirect-transfer" {
		t.Errorf("missing default label app.kubernetes.io/component")
	}
}

func TestOptionsValidation(t *testing.T) {
	tests := []struct {
		name    string
		opts    Options
		wantErr bool
	}{
		{
			name:    "valid options",
			opts:    Options{CloudStorage: "remote:bucket", ConfigSecret: "my-secret"},
			wantErr: false,
		},
		{
			name:    "missing cloud storage",
			opts:    Options{ConfigSecret: "my-secret"},
			wantErr: true,
		},
		{
			name:    "missing config secret",
			opts:    Options{CloudStorage: "remote:bucket"},
			wantErr: true,
		},
		{
			name:    "both missing",
			opts:    Options{},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.opts.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
