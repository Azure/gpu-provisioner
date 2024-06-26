/*
       Copyright (c) Microsoft Corporation.
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

package settings

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/go-playground/validator/v10"
	"go.uber.org/multierr"
	v1 "k8s.io/api/core/v1"
	"knative.dev/pkg/configmap"
)

type settingsKeyType struct{}

var ContextKey = settingsKeyType{}

var defaultSettings = Settings{
	ClusterName: "",
}

// +k8s:deepcopy-gen=true
type Settings struct {
	ClusterName string `validate:"required"`
}

func (*Settings) ConfigMap() string {
	return "gpu-provisioner-global-settings"
}

// Inject creates a Settings from the supplied ConfigMap
func (*Settings) Inject(ctx context.Context, cm *v1.ConfigMap) (context.Context, error) {
	s := defaultSettings.DeepCopy()

	if err := configmap.Parse(cm.Data,
		configmap.AsString("azure.clusterName", &s.ClusterName),
	); err != nil {
		return ctx, fmt.Errorf("parsing settings, %w", err)
	}
	if err := s.Validate(); err != nil {
		return ctx, fmt.Errorf("validating settings, %w", err)
	}

	return ToContext(ctx, s), nil
}

func (s Settings) Data() (map[string]string, error) {
	d := map[string]string{}

	raw, err := json.Marshal(s)
	if err != nil {
		return nil, fmt.Errorf("marshaling settings, %w", err)
	}
	if err = json.Unmarshal(raw, &d); err != nil {
		return d, fmt.Errorf("unmarshalling settings, %w", err)
	}
	return d, nil
}

func (s Settings) Validate() error {
	validate := validator.New()
	return multierr.Combine(
		validate.Struct(s),
	)
}

func ToContext(ctx context.Context, s *Settings) context.Context {
	return context.WithValue(ctx, ContextKey, s)
}

func FromContext(ctx context.Context) *Settings {
	data := ctx.Value(ContextKey)
	if data == nil {
		// This is developer error if this happens, so we should panic
		panic("settings doesn't exist in context")
	}
	return data.(*Settings)
}
