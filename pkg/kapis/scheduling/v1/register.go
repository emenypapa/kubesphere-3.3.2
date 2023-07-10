/*
Copyright 2019 The KubeSphere Authors.

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
	"github.com/emicklei/go-restful"
	restfulspec "github.com/emicklei/go-restful-openapi"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/kubernetes"
	"kubesphere.io/kubesphere/pkg/api"
	"kubesphere.io/kubesphere/pkg/apiserver/runtime"
	"kubesphere.io/kubesphere/pkg/server/errors"
	"net/http"
)

const (
	GroupName = "scheduling.k8s.io"

	tagClusteredResource  = "Clustered Resource"
	tagComponentStatus    = "Component Status"
	tagNamespacedResource = "Namespaced Resource"

	ok = "OK"
)

var GroupVersion = schema.GroupVersion{Group: GroupName, Version: "v1"}

func Resource(resource string) schema.GroupResource {
	return GroupVersion.WithResource(resource).GroupResource()
}

func AddToContainer(c *restful.Container, k8sClient kubernetes.Interface) error {
	webservice := runtime.NewWebService(GroupVersion)
	handler := New(k8sClient)
	webservice.Route(webservice.POST("/priorityclasses").
		To(handler.CreatePriorityClass).
		Metadata(restfulspec.KeyOpenAPITags, []string{tagClusteredResource}).
		Returns(http.StatusOK, api.StatusOK, errors.Error{}))
	webservice.Route(webservice.DELETE("/priorityclasses/{name}").
		To(handler.DeletePriorityClass).
		Param(webservice.PathParameter("name", "name of the priorityClass name.").Required(true)).
		Metadata(restfulspec.KeyOpenAPITags, []string{tagClusteredResource}).
		Returns(http.StatusOK, api.StatusOK, errors.Error{}))
	webservice.Route(webservice.PUT("/priorityclasses").
		To(handler.UpdatePriorityClass).
		Metadata(restfulspec.KeyOpenAPITags, []string{tagClusteredResource}).
		Returns(http.StatusOK, api.StatusOK, errors.Error{}))
	c.Add(webservice)

	return nil
}
