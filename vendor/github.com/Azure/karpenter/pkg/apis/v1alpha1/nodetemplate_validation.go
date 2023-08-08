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

package v1alpha1

import (
	"context"
	"fmt"
	"regexp"

	"knative.dev/pkg/apis"
)

var (
	SubscriptionShape          = regexp.MustCompile(`[a-fA-F0-9]{8}-[a-fA-F0-9]{4}-[a-fA-F0-9]{4}-[a-fA-F0-9]{4}-[a-fA-F0-9]{12}`)
	SIGImageVersionRegex       = regexp.MustCompile(`(?i)/subscriptions/` + SubscriptionShape.String() + `/resourceGroups/[\w-]+/providers/Microsoft\.Compute/galleries/[\w-]+/images/[\w-]+/versions/[\d.]+`)
	CommunityImageVersionRegex = regexp.MustCompile(`(?i)/CommunityGalleries/[\w-]+/images/[\w-]+/versions/[\d.]+`)
)

func (a *NodeTemplate) Validate(ctx context.Context) (errs *apis.FieldError) {
	return errs.Also(
		apis.ValidateObjectMetadata(a).ViaField("metadata"),
		a.Spec.validate(ctx).ViaField("spec"),
	)
}

func (a *NodeTemplateSpec) validate(_ context.Context) (errs *apis.FieldError) {
	return errs.Also(
		a.Azure.Validate(),
		a.validateImageID(),
		// further top-level validators go here
	)
}

func (a *NodeTemplateSpec) validateImageID() (errs *apis.FieldError) {
	if a.IsEmptyImageID() || SIGImageVersionRegex.MatchString(*a.ImageID) || CommunityImageVersionRegex.MatchString(*a.ImageID) {
		return nil
	}
	return apis.ErrInvalidValue(fmt.Sprintf(
		"the provided image ID: '%s' is invalid because it doesn't match the expected format", *a.ImageID), "ImageID")
}

func (a *NodeTemplateSpec) IsEmptyImageID() bool {
	return a.ImageID == nil || *a.ImageID == ""
}
