package webhook

import (
	"context"
	"github.com/expediagroup/kubernetes-sidecar-injector/pkg/admission"
	"github.com/ghodss/yaml"
	"github.com/samber/lo"
	log "github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"strings"
)

type SideCar struct {
	Name             string                        `yaml:"name"`
	InitContainers   []corev1.Container            `yaml:"initContainers"`
	Containers       []corev1.Container            `yaml:"containers"`
	Volumes          []corev1.Volume               `yaml:"volumes"`
	ImagePullSecrets []corev1.LocalObjectReference `yaml:"imagePullSecrets"`
	Annotations      map[string]string             `yaml:"annotations"`
	Labels           map[string]string             `yaml:"labels"`
}

type SidecarInjectorPatcher struct {
	K8sClient                kubernetes.Interface
	InjectPrefix             string
	InjectName               string
	SidecarDataKey           string
	AllowAnnotationOverrides bool
	AllowLabelOverrides      bool
}

func (patcher *SidecarInjectorPatcher) sideCarInjectionAnnotation() string {
	return patcher.InjectPrefix + "/" + patcher.InjectName
}

func (patcher *SidecarInjectorPatcher) configmapSidecarNames(namespace string, pod corev1.Pod) []string {
	podName := pod.GetName()
	if podName == "" {
		podName = pod.GetGenerateName()
	}
	annotations := map[string]string{}
	if pod.GetAnnotations() != nil {
		annotations = pod.GetAnnotations()
	}
	if sidecars, ok := annotations[patcher.sideCarInjectionAnnotation()]; ok {
		parts := lo.Map[string, string](strings.Split(sidecars, ","), func(part string, _ int) string {
			return strings.TrimSpace(part)
		})

		if len(parts) > 0 {
			log.Infof("sideCar injection for %v/%v: sidecars: %v", namespace, podName, sidecars)
			return parts
		}
	}
	log.Infof("Skipping mutation for [%v]. No action required", pod.GetName())
	return nil
}

func createArrayPatches[T any](newCollection []T, existingCollection []T, path string) []admission.PatchOperation {
	var patches []admission.PatchOperation
	for index, item := range newCollection {
		var value interface{}
		first := index == 0 && len(existingCollection) == 0
		if !first {
			path = path + "/-"
			value = item
		} else {
			value = []T{item}
		}
		patches = append(patches, admission.PatchOperation{
			Op:    "add",
			Path:  path,
			Value: value,
		})
	}
	return patches
}

func createObjectPatches(newMap map[string]string, existingMap map[string]string, path string, override bool) []admission.PatchOperation {
	var patches []admission.PatchOperation
	if existingMap == nil {
		patches = append(patches, admission.PatchOperation{
			Op:    "add",
			Path:  path,
			Value: newMap,
		})
	} else {
		for key, value := range newMap {
			if _, ok := existingMap[key]; !ok || (ok && override) {
				op := "add"
				if ok {
					op = "replace"
				}
				patches = append(patches, admission.PatchOperation{
					Op:    op,
					Path:  path + "/" + key,
					Value: value,
				})
			}
		}
	}
	return patches
}

func (patcher *SidecarInjectorPatcher) PatchPodCreate(ctx context.Context, namespace string, pod corev1.Pod) ([]admission.PatchOperation, error) {
	podName := pod.GetName()
	if podName == "" {
		podName = pod.GetGenerateName()
	}
	var patches []admission.PatchOperation
	if configmapSidecarNames := patcher.configmapSidecarNames(namespace, pod); configmapSidecarNames != nil {
		for _, configmapSidecarName := range configmapSidecarNames {
			configmapSidecar, err := patcher.K8sClient.CoreV1().ConfigMaps(namespace).Get(ctx, configmapSidecarName, metav1.GetOptions{})
			if k8serrors.IsNotFound(err) {
				log.Warnf("sidecar configmap %s/%s was not found", namespace, configmapSidecarName)
			} else if err != nil {
				log.Errorf("error fetching sidecar configmap %s/%s - %v", namespace, configmapSidecarName, err)
			} else if sidecarsStr, ok := configmapSidecar.Data[patcher.SidecarDataKey]; ok {
				var sidecars []SideCar
				if err := yaml.Unmarshal([]byte(sidecarsStr), &sidecars); err != nil {
					log.Errorf("error unmarshalling %s from configmap %s/%s", patcher.SidecarDataKey, pod.GetNamespace(), configmapSidecarName)
				}
				if sidecars != nil {
					for _, sidecar := range sidecars {
						patches = append(patches, createArrayPatches(sidecar.InitContainers, pod.Spec.InitContainers, "/spec/initContainers")...)
						patches = append(patches, createArrayPatches(sidecar.Containers, pod.Spec.Containers, "/spec/containers")...)
						patches = append(patches, createArrayPatches(sidecar.Volumes, pod.Spec.Volumes, "/spec/volumes")...)
						patches = append(patches, createArrayPatches(sidecar.ImagePullSecrets, pod.Spec.ImagePullSecrets, "/spec/imagePullSecrets")...)
						patches = append(patches, createObjectPatches(sidecar.Annotations, pod.Annotations, "/metadata/annotations", patcher.AllowAnnotationOverrides)...)
						patches = append(patches, createObjectPatches(sidecar.Labels, pod.Labels, "/metadata/labels", patcher.AllowLabelOverrides)...)
					}
					log.Debugf("sidecar patches being applied for %v/%v: patches: %v", namespace, podName, patches)
				}
			}
		}
	}
	return patches, nil
}

/*PatchPodUpdate not supported, only support create */
func (patcher *SidecarInjectorPatcher) PatchPodUpdate(_ context.Context, _ string, _ corev1.Pod, _ corev1.Pod) ([]admission.PatchOperation, error) {
	return nil, nil
}

/*PatchPodDelete not supported, only support create */
func (patcher *SidecarInjectorPatcher) PatchPodDelete(_ context.Context, _ string, _ corev1.Pod) ([]admission.PatchOperation, error) {
	return nil, nil
}
