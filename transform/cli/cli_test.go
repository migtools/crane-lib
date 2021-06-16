package cli

import (
	"fmt"
	"reflect"
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

type fakeReader struct {
	*unstructured.Unstructured
	err error
}

func (f *fakeReader) Read(p []byte) (int, error) {
	if f.err != nil {
		return 0, f.err
	}
	b, err := f.Unstructured.MarshalJSON()
	for i, _ := range p {
		if i >= len(b) {
			return len(b), err
		}
		p[i] = b[i]
	}

	return len(b), err
}

func TestUnstructured(t *testing.T) {
	tests := []struct {
		name    string
		reader  *fakeReader
		want    unstructured.Unstructured
		wantErr bool
	}{
		// TODO: Add test cases.
		{
			name: "ValidJsonPodObject",
			reader: &fakeReader{
				Unstructured: &unstructured.Unstructured{Object: map[string]interface{}{
					"apiVersion": "v1",
					"kind":       "pod",
					"metadata": map[string]interface{}{
						"name":      "foo",
						"namespace": "bar",
					},
					"spec":   map[string]interface{}{},
					"status": map[string]interface{}{},
				}},
				err: nil,
			},
			want: unstructured.Unstructured{Object: map[string]interface{}{
				"apiVersion": "v1",
				"kind":       "pod",
				"metadata": map[string]interface{}{
					"name":      "foo",
					"namespace": "bar",
				},
				"spec":   map[string]interface{}{},
				"status": map[string]interface{}{},
			}},
			wantErr: false,
		},
		{
			name: "InValidJsonPodObject",
			reader: &fakeReader{
				Unstructured: &unstructured.Unstructured{},
				err:          fmt.Errorf("error decoding json data"),
			},
			want:    unstructured.Unstructured{},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Unstructured(tt.reader)
			if (err != nil) != tt.wantErr {
				t.Errorf("Unstructured() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(*got, tt.want) {
				t.Errorf("Unstructured() got = %v, want %v", got, tt.want)
			}
		})
	}
}
