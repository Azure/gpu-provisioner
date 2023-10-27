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

package settings_test

import (
	"context"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	v1 "k8s.io/api/core/v1"
	. "knative.dev/pkg/logging/testing"

	"github.com/azure/gpu-provisioner/pkg/apis/settings"
)

var ctx context.Context

func TestAPIs(t *testing.T) {
	ctx = TestContextWithLogger(t)
	RegisterFailHandler(Fail)
	RunSpecs(t, "Settings")
}

var _ = Describe("Validation", func() {
	It("should succeed to set defaults", func() {
		cm := &v1.ConfigMap{
			Data: map[string]string{
				"azure.clusterName": "my-cluster",
			},
		}
		ctx, err := (&settings.Settings{}).Inject(ctx, cm)
		Expect(err).ToNot(HaveOccurred())
		s := settings.FromContext(ctx)
		Expect(s.ClusterName).To(Equal("my-cluster"))

	})

	It("should fail validation with panic when clusterName not included", func() {
		cm := &v1.ConfigMap{
			Data: map[string]string{},
		}
		_, err := (&settings.Settings{}).Inject(ctx, cm)
		Expect(err).To(HaveOccurred())
	})
})
