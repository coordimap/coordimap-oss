package kubernetes

import (
	"testing"
)

// TestGetAppVersionFromLabels tests the GetAppVersionFromLabels function.
func TestGetAppVersionFromLabels(t *testing.T) {
	testCases := []struct {
		name            string
		labels          map[string]string
		expectedVersion string
		expectedFound   bool
	}{
		{
			name: "Version label exists",
			labels: map[string]string{
				"app.kubernetes.io/name":    "my-app",
				"app.kubernetes.io/version": "1.2.3",
				"app.kubernetes.io/part-of": "my-system",
			},
			expectedVersion: "1.2.3",
			expectedFound:   true,
		},
		{
			name: "Version label does not exist",
			labels: map[string]string{
				"app.kubernetes.io/name":    "another-app",
				"app.kubernetes.io/part-of": "another-system",
			},
			expectedVersion: "",
			expectedFound:   false,
		},
		{
			name:            "Empty labels map",
			labels:          map[string]string{},
			expectedVersion: "",
			expectedFound:   false,
		},
		{
			name:            "Nil labels map",
			labels:          nil,
			expectedVersion: "",
			expectedFound:   false,
		},
		{
			name: "Version label exists but is empty",
			labels: map[string]string{
				"app.kubernetes.io/version": "",
			},
			expectedVersion: "",
			expectedFound:   true, // The label exists, even if the value is empty
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			version, found := GetAppVersionFromLabels(tc.labels)

			if found != tc.expectedFound {
				t.Errorf("Expected found to be %v, but got %v", tc.expectedFound, found)
				return
			}

			if version != tc.expectedVersion {
				t.Errorf("Expected version to be %q, but got %q", tc.expectedVersion, version)
			}
		})
	}
}
