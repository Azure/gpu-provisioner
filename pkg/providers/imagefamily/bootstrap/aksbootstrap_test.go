/*
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

package bootstrap

import (
	"testing"
)

func TestKubeBinaryURL(t *testing.T) {
	cases := []struct {
		name     string
		version  string
		expected string
	}{
		{
			name:     "Test version 1.24.x",
			version:  "1.24.5",
			expected: "https://acs-mirror.azureedge.net/kubernetes/v1.24.10-hotfix.20230509/binaries/kubernetes-node-linux-amd64.tar.gz",
		},
		{
			name:     "Test version 1.25.x",
			version:  "1.25.2",
			expected: "https://acs-mirror.azureedge.net/kubernetes/v1.25.6-hotfix.20230509/binaries/kubernetes-node-linux-amd64.tar.gz",
		},
		{
			name:     "Test version 1.26.x",
			version:  "1.26.0",
			expected: "https://acs-mirror.azureedge.net/kubernetes/v1.26.3-hotfix.20230509/binaries/kubernetes-node-linux-amd64.tar.gz",
		},
		{
			name:     "Test version 1.27.x",
			version:  "1.27.1",
			expected: "https://acs-mirror.azureedge.net/kubernetes/v1.27.1/binaries/kubernetes-node-linux-amd64.tar.gz",
		},
		{
			name:     "Test unhandled version",
			version:  "1.28.0",
			expected: "https://acs-mirror.azureedge.net/kubernetes/v1.27.1/binaries/kubernetes-node-linux-amd64.tar.gz",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			actual := kubeBinaryURL(tc.version)
			if actual != tc.expected {
				t.Errorf("Expected %s but got %s", tc.expected, actual)
			}
		})
	}
}
