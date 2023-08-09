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

package imagefamily_test

import (
	"context"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/gpu-vmprovisioner/pkg/apis/v1alpha1"
	"github.com/gpu-vmprovisioner/pkg/providers/imagefamily"
	"github.com/aws/karpenter-core/pkg/cloudprovider"
	"github.com/samber/lo"
)

var imageProvider *imagefamily.Provider

func TestAzure(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Providers/ImageProvider/Azure")
}

var _ = BeforeSuite(func() {
	imageProvider = imagefamily.NewProvider(nil, nil)
})

var _ = Describe("ImageProvider", func() {
	DescribeTable("Get Image ID", func(nodeTemplate *v1alpha1.NodeTemplate, instanceType *cloudprovider.InstanceType, imageFamily imagefamily.ImageFamily, expectedImageID string, expectedError error) {
		imageID, err := imageProvider.Get(context.Background(), nodeTemplate, instanceType, imageFamily)
		Expect(imageID).To(Equal(expectedImageID))
		if expectedError != nil {
			Expect(err).To(Not(BeNil()))
			Expect(err.Error()).To(Equal(expectedError.Error()))
		} else {
			Expect(err).To(BeNil())
		}
	},
		Entry("should return default image ID when node template image ID is not set",
			&v1alpha1.NodeTemplate{
				ObjectMeta: v1.ObjectMeta{
					Name: "default",
				},
			}, &cloudprovider.InstanceType{}, imagefamily.Ubuntu{}, imagefamily.DefaultImageID, nil),
		Entry("should return node template image ID when node template image ID is set",
			&v1alpha1.NodeTemplate{
				ObjectMeta: v1.ObjectMeta{
					Name: "default",
				},
				Spec: v1alpha1.NodeTemplateSpec{
					Azure: v1alpha1.Azure{
						ImageID: lo.ToPtr("/CommunityGalleries/previewaks-1a06572d-8508-419c-a0d1-baffcbcb2f3b/Images/2204Gen2/Versions/1.1685741267.25933"),
					},
				},
			}, &cloudprovider.InstanceType{}, imagefamily.Ubuntu{}, "/CommunityGalleries/previewaks-1a06572d-8508-419c-a0d1-baffcbcb2f3b/Images/2204Gen2/Versions/1.1685741267.25933", nil),
	)
})
