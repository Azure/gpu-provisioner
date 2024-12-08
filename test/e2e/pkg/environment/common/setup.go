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
	"fmt"
	"sync"
	"time"

	. "github.com/onsi/ginkgo/v2" //nolint:revive,stylecheck
	. "github.com/onsi/gomega"    //nolint:revive,stylecheck
	"github.com/samber/lo"
	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/util/workqueue"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/apiutil"
	karpenterv1 "sigs.k8s.io/karpenter/pkg/apis/v1"
	"sigs.k8s.io/karpenter/pkg/test"
)

var (
	CleanableObjects = []client.Object{
		&v1.Pod{},
		&appsv1.Deployment{},
		&karpenterv1.NodeClaim{},
		&v1.Node{},
	}
)

// nolint:gocyclo
func (env *Environment) BeforeEach() {
	env.StartingNodeCount = env.Monitor.NodeCount()
}

func (env *Environment) Cleanup() {
	env.CleanupObjects(CleanableObjects...)
}

func (env *Environment) AfterEach() {
	env.printControllerLogs(&v1.PodLogOptions{Container: "controller"})
}

func (env *Environment) CleanupObjects(cleanableObjects ...client.Object) {
	wg := sync.WaitGroup{}
	for _, obj := range cleanableObjects {
		wg.Add(1)
		go func(obj client.Object) {
			defer wg.Done()
			defer GinkgoRecover()

			gvk := lo.Must(apiutil.GVKForObject(obj, env.Client.Scheme()))
			// This only gets the metadata for the objects since we don't need all the details of the objects
			metaList := &metav1.PartialObjectMetadataList{}
			metaList.SetGroupVersionKind(gvk)
			Expect(env.Client.List(env, metaList, client.HasLabels([]string{test.DiscoveryLabel}))).To(Succeed())
			// Limit the concurrency of these calls to 50 workers per object so that we try to limit how aggressively we
			// are deleting so that we avoid getting client-side throttled
			workqueue.ParallelizeUntil(env, 50, len(metaList.Items), func(i int) {
				defer GinkgoRecover()
				Eventually(func(g Gomega) {
					g.Expect(client.IgnoreNotFound(env.Client.Delete(env, &metaList.Items[i], client.PropagationPolicy(metav1.DeletePropagationForeground)))).To(Succeed())
				}).WithPolling(time.Second).Should(Succeed())
			})
			Eventually(func(g Gomega) {
				metaList = &metav1.PartialObjectMetadataList{}
				metaList.SetGroupVersionKind(gvk)
				err := env.Client.List(env, metaList, client.HasLabels([]string{test.DiscoveryLabel}))
				g.Expect(err).To(Succeed())
				g.Expect(len(metaList.Items)).To(BeZero(), fmt.Sprintf("Not all objects(%s) are deleted", gvk.String()))
			}).WithPolling(time.Second * 10).Should(Succeed())
		}(obj)
	}
	wg.Wait()
}
