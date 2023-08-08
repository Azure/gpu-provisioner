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

package scheduling

import (
	"context"
	"fmt"
	"sort"

	"github.com/samber/lo"
	csitranslation "k8s.io/csi-translation-lib"
	"knative.dev/pkg/logging"

	v1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/sets"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/aws/karpenter-core/pkg/utils/pretty"
)

const (
	IsDefaultStorageClassAnnotation = "storageclass.kubernetes.io/is-default-class"
)

// translator is a CSI Translator that translates in-tree plugin names to their out-of-tree CSI driver names
var translator = csitranslation.New()

// changeMonitor is a change monitor for global volumeUsage logging
var changeMonitor = pretty.NewChangeMonitor()

// VolumeUsage tracks volume limits on a per node basis.  The number of volumes that can be mounted varies by instance
// type. We need to be aware and track the mounted volume usage to inform our awareness of which pods can schedule to
// which nodes.
type VolumeUsage struct {
	volumes    volumes
	podVolumes map[types.NamespacedName]volumes
}

func NewVolumeUsage() *VolumeUsage {
	return &VolumeUsage{
		volumes:    volumes{},
		podVolumes: map[types.NamespacedName]volumes{},
	}
}

type volumes map[string]sets.String

func (u volumes) Add(provisioner string, pvcID string) {
	existing, ok := u[provisioner]
	if !ok {
		existing = sets.NewString()
		u[provisioner] = existing
	}
	existing.Insert(pvcID)
}

func (u volumes) union(vol volumes) volumes {
	cp := volumes{}
	for k, v := range u {
		cp[k] = sets.NewString(v.List()...)
	}
	for k, v := range vol {
		existing, ok := cp[k]
		if !ok {
			existing = sets.NewString()
			cp[k] = existing
		}
		existing.Insert(v.List()...)
	}
	return cp
}

func (u volumes) insert(volumes volumes) {
	for k, v := range volumes {
		existing, ok := u[k]
		if !ok {
			existing = sets.NewString()
			u[k] = existing
		}
		existing.Insert(v.List()...)
	}
}

func (u volumes) copy() volumes {
	cp := volumes{}
	for k, v := range u {
		cp[k] = sets.NewString(v.List()...)
	}
	return cp
}

func (v *VolumeUsage) Add(ctx context.Context, kubeClient client.Client, pod *v1.Pod) {
	podVolumes, err := v.validate(ctx, kubeClient, pod)
	if err != nil {
		logging.FromContext(ctx).Errorf("inconsistent state error adding volume, %s, please file an issue", err)
	}
	v.podVolumes[client.ObjectKeyFromObject(pod)] = podVolumes
	v.volumes = v.volumes.union(podVolumes)
}

// VolumeCount stores a mapping between the driver name that provides volumes
// and the number of volumes associated with that driver
type VolumeCount map[string]int

// Exceeds returns true if the volume count exceeds the limits provided.  If there is no value for a storage provider, it
// is treated as unlimited.
func (c VolumeCount) Exceeds(limits VolumeCount) bool {
	for k, v := range c {
		limit, hasLimit := limits[k]
		if !hasLimit {
			continue
		}
		if v > limit {
			return true
		}
	}
	return false
}

// Fits returns true if the rhs 'fits' within the volume count.
func (c VolumeCount) Fits(rhs VolumeCount) bool {
	for k, v := range rhs {
		limit, hasLimit := c[k]
		if !hasLimit {
			continue
		}
		if v > limit {
			return false
		}
	}
	return true
}

func (v *VolumeUsage) Validate(ctx context.Context, kubeClient client.Client, pod *v1.Pod) (VolumeCount, error) {
	podVolumes, err := v.validate(ctx, kubeClient, pod)
	if err != nil {
		return nil, err
	}
	result := VolumeCount{}
	for k, v := range v.volumes.union(podVolumes) {
		result[k] += len(v)
	}
	return result, nil
}

//nolint:gocyclo
func (v *VolumeUsage) validate(ctx context.Context, kubeClient client.Client, pod *v1.Pod) (volumes, error) {
	ctx = logging.WithLogger(ctx, logging.FromContext(ctx).With("pod", pod.Name))
	podPVCs := volumes{}
	defaultStorageClassName, err := v.discoverDefaultStorageClassName(ctx, kubeClient)
	if err != nil {
		return nil, fmt.Errorf("discovering default storage class, %w", err)
	}
	for _, volume := range pod.Spec.Volumes {
		ctx = logging.WithLogger(ctx, logging.FromContext(ctx).With("volume", volume.Name))
		var pvcID string
		var storageClassName *string
		var volumeName string
		var pvc v1.PersistentVolumeClaim
		if volume.PersistentVolumeClaim != nil {
			if err := kubeClient.Get(ctx, client.ObjectKey{Namespace: pod.Namespace, Name: volume.PersistentVolumeClaim.ClaimName}, &pvc); err != nil {
				return nil, err
			}
			pvcID = fmt.Sprintf("%s/%s", pod.Namespace, volume.PersistentVolumeClaim.ClaimName)
			storageClassName = pvc.Spec.StorageClassName
			if storageClassName == nil || *storageClassName == "" {
				storageClassName = defaultStorageClassName
			}
			volumeName = pvc.Spec.VolumeName
		} else if volume.Ephemeral != nil {
			// generated name per https://kubernetes.io/docs/concepts/storage/ephemeral-volumes/#persistentvolumeclaim-naming
			pvcID = fmt.Sprintf("%s/%s-%s", pod.Namespace, pod.Name, volume.Name)
			storageClassName = volume.Ephemeral.VolumeClaimTemplate.Spec.StorageClassName
			if storageClassName == nil || *storageClassName == "" {
				storageClassName = defaultStorageClassName
			}
			volumeName = volume.Ephemeral.VolumeClaimTemplate.Spec.VolumeName
		} else {
			continue
		}
		driverName, err := v.resolveDriver(ctx, kubeClient, volumeName, storageClassName)
		if err != nil {
			return nil, err
		}
		// might be a non-CSI driver, something we don't currently handle
		if driverName != "" {
			podPVCs.Add(driverName, pvcID)
		}
	}
	return podPVCs, nil
}

func (v *VolumeUsage) discoverDefaultStorageClassName(ctx context.Context, kubeClient client.Client) (*string, error) {
	storageClassList := &storagev1.StorageClassList{}
	if err := kubeClient.List(ctx, storageClassList); err != nil {
		return nil, err
	}
	// Find all StorageClasses that have the default annotation
	defaults := lo.Filter(storageClassList.Items, func(sc storagev1.StorageClass, _ int) bool {
		return sc.Annotations[IsDefaultStorageClassAnnotation] == "true"
	})
	if len(defaults) == 0 {
		return nil, nil
	}
	// Sort the default StorageClasses by timestamp and take the newest one
	// https://github.com/kubernetes/kubernetes/pull/110559
	sort.Slice(defaults, func(i, j int) bool {
		return defaults[i].CreationTimestamp.After(defaults[j].CreationTimestamp.Time)
	})
	return lo.ToPtr(defaults[0].Name), nil
}

// resolveDriver resolves the storage driver name in the following order:
//  1. If the PV associated with the pod volume is using CSI.driver in its spec, then use that name
//  2. If the StorageClass associated with the PV has a Provisioner
func (v *VolumeUsage) resolveDriver(ctx context.Context, kubeClient client.Client, volumeName string, storageClassName *string) (string, error) {
	// We can track the volume usage by the CSI Driver name which is pulled from the storage class for dynamic
	// volumes, or if it's bound/static we can pull the volume name
	if volumeName != "" {
		driverName, err := v.driverFromVolume(ctx, kubeClient, volumeName)
		if err != nil {
			return "", err
		}
		if driverName != "" {
			return driverName, nil
		}
	}
	if storageClassName != nil && *storageClassName != "" {
		driverName, err := v.driverFromSC(ctx, kubeClient, *storageClassName)
		if err != nil {
			return "", err
		}
		if driverName != "" {
			return driverName, nil
		}
	}
	// Driver name wasn't able to resolve for this volume. In this case, we just ignore the
	// volume and move on to the other volumes that the pod has
	return "", nil
}

// driverFromSC resolves the storage driver name by getting the Provisioner name from the StorageClass
func (v *VolumeUsage) driverFromSC(ctx context.Context, kubeClient client.Client, storageClassName string) (string, error) {
	var sc storagev1.StorageClass
	if err := kubeClient.Get(ctx, client.ObjectKey{Name: storageClassName}, &sc); err != nil {
		return "", err
	}
	// Check if the provisioner name is an in-tree plugin name
	if csiName, err := translator.GetCSINameFromInTreeName(sc.Provisioner); err == nil {
		if changeMonitor.HasChanged(fmt.Sprintf("sc/%s", storageClassName), nil) {
			logging.FromContext(ctx).With("storage-class", sc.Name, "provisioner", sc.Provisioner).Errorf("StorageClass .spec.provisioner uses an in-tree storage plugin which is unsupported by Karpenter and is deprecated by Kubernetes. Scale-ups may fail because Karpenter will not discover driver limits. Create a new StorageClass with a .spec.provisioner referencing the CSI driver plugin name '%s'.", csiName)
		}
	}
	return sc.Provisioner, nil
}

// driverFromVolume resolves the storage driver name by getting the CSI spec from inside the PersistentVolume
func (v *VolumeUsage) driverFromVolume(ctx context.Context, kubeClient client.Client, volumeName string) (string, error) {
	var pv v1.PersistentVolume
	if err := kubeClient.Get(ctx, client.ObjectKey{Name: volumeName}, &pv); err != nil {
		return "", err
	}
	if pv.Spec.CSI != nil {
		return pv.Spec.CSI.Driver, nil
	} else if pv.Spec.AWSElasticBlockStore != nil {
		if changeMonitor.HasChanged(fmt.Sprintf("pv/%s", pv.Name), nil) {
			logging.FromContext(ctx).With("persistent-volume", pv.Name).Errorf("PersistentVolume source 'AWSElasticBlockStore' uses an in-tree storage plugin which is unsupported by Karpenter and is deprecated by Kubernetes. Scale-ups may fail because Karpenter will not discover driver limits. Use a PersistentVolume that references the 'CSI' volume source for Karpenter auto-scaling support.")
		}
	}
	return "", nil
}

func (v *VolumeUsage) DeletePod(key types.NamespacedName) {
	delete(v.podVolumes, key)
	// volume names could be duplicated, so we re-create our volumes
	v.volumes = volumes{}
	for _, c := range v.podVolumes {
		v.volumes.insert(c)
	}
}

func (v *VolumeUsage) DeepCopy() *VolumeUsage {
	if v == nil {
		return nil
	}
	out := &VolumeUsage{}
	v.DeepCopyInto(out)
	return out
}

func (v *VolumeUsage) DeepCopyInto(out *VolumeUsage) {
	out.volumes = v.volumes.copy()
	out.podVolumes = map[types.NamespacedName]volumes{}
	for k, v := range v.podVolumes {
		out.podVolumes[k] = v.copy()
	}
}
