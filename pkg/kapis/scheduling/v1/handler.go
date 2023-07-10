/*
Copyright 2020 KubeSphere Authors

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

package v1

import (
	"context"
	apiv1 "k8s.io/api/core/v1"
	v1 "k8s.io/api/scheduling/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	kserr "kubesphere.io/kubesphere/pkg/server/errors"
	"net/http"
	"strings"

	"github.com/emicklei/go-restful"
	"kubesphere.io/kubesphere/pkg/api"
)

type Handler struct {
	k kubernetes.Interface
}

func New(k kubernetes.Interface) *Handler {
	return &Handler{
		k: k,
	}
}

func (h *Handler) CreatePriorityClass(request *restful.Request, response *restful.Response) {
	var pc api.PriorityClass
	err := request.ReadEntity(&pc)
	if err != nil {
		response.WriteHeaderAndEntity(http.StatusInternalServerError, kserr.Wrap(err))
		return
	}

	p := apiv1.PreemptLowerPriority
	priorityClass := &v1.PriorityClass{
		ObjectMeta: metav1.ObjectMeta{
			Name: pc.Name,
		},
		Value:            pc.Value,
		GlobalDefault:    false,
		Description:      "Eicas Scenes PriorityClass",
		PreemptionPolicy: &p,
	}

	ksPriorityClass, err := h.k.SchedulingV1().PriorityClasses().Create(context.TODO(), priorityClass, metav1.CreateOptions{})

	if err != nil {
		response.WriteHeaderAndEntity(http.StatusInternalServerError, kserr.Wrap(err))
		return
	}

	response.WriteHeaderAndJson(http.StatusOK, ksPriorityClass, restful.MIME_JSON)
}

func (h *Handler) DeletePriorityClass(request *restful.Request, response *restful.Response) {
	pcName := request.PathParameter("name")

	err := h.k.SchedulingV1().PriorityClasses().Delete(context.TODO(), pcName, metav1.DeleteOptions{})

	if err != nil {
		response.WriteHeaderAndEntity(http.StatusInternalServerError, kserr.Wrap(err))
		return
	}

	response.WriteHeaderAndJson(http.StatusOK, nil, restful.MIME_JSON)
}

func (h *Handler) UpdatePriorityClass(request *restful.Request, response *restful.Response) {
	var pc api.PriorityClass
	err := request.ReadEntity(&pc)
	if err != nil {
		response.WriteHeaderAndEntity(http.StatusInternalServerError, kserr.Wrap(err))
		return
	}

	ksPriorityClass, err := h.k.SchedulingV1().PriorityClasses().Get(context.TODO(), pc.Name, metav1.GetOptions{})
	if err != nil {
		response.WriteHeaderAndEntity(http.StatusInternalServerError, kserr.Wrap(err))
		return
	}

	err = h.k.SchedulingV1().PriorityClasses().Delete(context.TODO(), ksPriorityClass.Name, metav1.DeleteOptions{})
	if err != nil {
		response.WriteHeaderAndEntity(http.StatusInternalServerError, kserr.Wrap(err))
		return
	}

	p := apiv1.PreemptLowerPriority
	priorityClass := &v1.PriorityClass{
		ObjectMeta: metav1.ObjectMeta{
			Name: ksPriorityClass.Name,
		},
		Value:            pc.Value,
		GlobalDefault:    false,
		Description:      "Eicas Scenes PriorityClass",
		PreemptionPolicy: &p,
	}

	res, err := h.k.SchedulingV1().PriorityClasses().Create(context.TODO(), priorityClass, metav1.CreateOptions{})
	if err != nil {
		response.WriteHeaderAndEntity(http.StatusInternalServerError, kserr.Wrap(err))
		return
	}

	response.WriteHeaderAndJson(http.StatusOK, res, restful.MIME_JSON)
}

func canonicalizeRegistryError(request *restful.Request, response *restful.Response, err error) {
	if strings.Contains(err.Error(), "Unauthorized") {
		api.HandleUnauthorized(response, request, err)
	} else if strings.Contains(err.Error(), "MANIFEST_UNKNOWN") {
		api.HandleNotFound(response, request, err)
	} else {
		api.HandleBadRequest(response, request, err)
	}
}
