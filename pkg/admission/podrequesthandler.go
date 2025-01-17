package admission

import (
	"context"
	"encoding/json"
	"github.com/pkg/errors"
	admissionv1 "k8s.io/api/admission/v1"
	corev1 "k8s.io/api/core/v1"
)

type PodAdmissionRequestHandler struct {
	PodHandler PodPatcher
}

func (handler *PodAdmissionRequestHandler) handleAdmissionCreate(ctx context.Context, request *admissionv1.AdmissionRequest) ([]PatchOperation, error) {
	if pod, err := unmarshalPod(request.Object.Raw); err != nil {
		return nil, err
	} else {
		return handler.PodHandler.PatchPodCreate(ctx, request.Namespace, pod)
	}
}

func (handler *PodAdmissionRequestHandler) handleAdmissionUpdate(ctx context.Context, request *admissionv1.AdmissionRequest) ([]PatchOperation, error) {
	oldPod, err := unmarshalPod(request.OldObject.Raw)
	if err != nil {
		return nil, err
	}
	if newPod, err := unmarshalPod(request.Object.Raw); err != nil {
		return nil, err
	} else {
		return handler.PodHandler.PatchPodUpdate(ctx, request.Namespace, oldPod, newPod)
	}
}

func (handler *PodAdmissionRequestHandler) handleAdmissionDelete(ctx context.Context, request *admissionv1.AdmissionRequest) ([]PatchOperation, error) {
	if pod, err := unmarshalPod(request.OldObject.Raw); err != nil {
		return nil, err
	} else {
		return handler.PodHandler.PatchPodDelete(ctx, request.Namespace, pod)
	}
}

func unmarshalPod(rawObject []byte) (corev1.Pod, error) {
	var pod corev1.Pod
	err := json.Unmarshal(rawObject, &pod)
	return pod, errors.Wrapf(err, "error unmarshalling object")
}
