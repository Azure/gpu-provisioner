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

package common

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/onsi/gomega"
	"github.com/samber/lo"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	loggingtesting "knative.dev/pkg/logging/testing"
	"knative.dev/pkg/system"
	controllerruntime "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	_ "sigs.k8s.io/karpenter/pkg/apis/v1"
	"sigs.k8s.io/karpenter/pkg/test/v1alpha1"
)

type ContextKey string

const (
	GitRefContextKey = ContextKey("gitRef")
)

type Environment struct {
	context.Context

	Client     client.Client
	Config     *rest.Config
	KubeClient kubernetes.Interface
	Monitor    *Monitor

	StartingNodeCount int
}

func NewEnvironment(t *testing.T) *Environment {
	ctx := loggingtesting.TestContextWithLogger(t)
	config := NewConfig()
	client := lo.Must(NewClient(config))

	lo.Must0(os.Setenv(system.NamespaceEnvKey, "gpu-provisioner"))
	kubernetesInterface := kubernetes.NewForConfigOrDie(config)
	if val, ok := os.LookupEnv("GIT_REF"); ok {
		ctx = context.WithValue(ctx, GitRefContextKey, val)
	}

	gomega.SetDefaultEventuallyTimeout(10 * time.Minute)
	gomega.SetDefaultEventuallyPollingInterval(1 * time.Second)
	return &Environment{
		Context:    ctx,
		Config:     config,
		Client:     client,
		KubeClient: kubernetesInterface,
		Monitor:    NewMonitor(ctx, client),
	}
}

func NewConfig() *rest.Config {
	config := controllerruntime.GetConfigOrDie()
	config.UserAgent = strings.ReplaceAll(schema.GroupVersion{Group: v1alpha1.Group, Version: "v1alpha1"}.String(), "/", "-")
	config.QPS = 1e6
	config.Burst = 1e6
	return config
}

func NewClient(config *rest.Config) (client.Client, error) {
	return client.New(config, client.Options{Scheme: scheme.Scheme})
}
