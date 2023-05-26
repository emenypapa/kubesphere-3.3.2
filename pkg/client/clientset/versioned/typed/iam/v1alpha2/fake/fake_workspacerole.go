/*
Copyright 2020 The KubeSphere Authors.

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

// Code generated by client-gen. DO NOT EDIT.

package fake

import (
	"context"

	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	labels "k8s.io/apimachinery/pkg/labels"
	schema "k8s.io/apimachinery/pkg/runtime/schema"
	types "k8s.io/apimachinery/pkg/types"
	watch "k8s.io/apimachinery/pkg/watch"
	testing "k8s.io/client-go/testing"
	v1alpha2 "kubesphere.io/api/iam/v1alpha2"
)

// FakeWorkspaceRoles implements WorkspaceRoleInterface
type FakeWorkspaceRoles struct {
	Fake *FakeIamV1alpha2
}

var workspacerolesResource = schema.GroupVersionResource{Group: "iam.kubesphere.io", Version: "v1alpha2", Resource: "workspaceroles"}

var workspacerolesKind = schema.GroupVersionKind{Group: "iam.kubesphere.io", Version: "v1alpha2", Kind: "WorkspaceRole"}

// Get takes name of the workspaceRole, and returns the corresponding workspaceRole object, and an error if there is any.
func (c *FakeWorkspaceRoles) Get(ctx context.Context, name string, options v1.GetOptions) (result *v1alpha2.WorkspaceRole, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewRootGetAction(workspacerolesResource, name), &v1alpha2.WorkspaceRole{})
	if obj == nil {
		return nil, err
	}
	return obj.(*v1alpha2.WorkspaceRole), err
}

// List takes label and field selectors, and returns the list of WorkspaceRoles that match those selectors.
func (c *FakeWorkspaceRoles) List(ctx context.Context, opts v1.ListOptions) (result *v1alpha2.WorkspaceRoleList, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewRootListAction(workspacerolesResource, workspacerolesKind, opts), &v1alpha2.WorkspaceRoleList{})
	if obj == nil {
		return nil, err
	}

	label, _, _ := testing.ExtractFromListOptions(opts)
	if label == nil {
		label = labels.Everything()
	}
	list := &v1alpha2.WorkspaceRoleList{ListMeta: obj.(*v1alpha2.WorkspaceRoleList).ListMeta}
	for _, item := range obj.(*v1alpha2.WorkspaceRoleList).Items {
		if label.Matches(labels.Set(item.Labels)) {
			list.Items = append(list.Items, item)
		}
	}
	return list, err
}

// Watch returns a watch.Interface that watches the requested workspaceRoles.
func (c *FakeWorkspaceRoles) Watch(ctx context.Context, opts v1.ListOptions) (watch.Interface, error) {
	return c.Fake.
		InvokesWatch(testing.NewRootWatchAction(workspacerolesResource, opts))
}

// Create takes the representation of a workspaceRole and creates it.  Returns the server's representation of the workspaceRole, and an error, if there is any.
func (c *FakeWorkspaceRoles) Create(ctx context.Context, workspaceRole *v1alpha2.WorkspaceRole, opts v1.CreateOptions) (result *v1alpha2.WorkspaceRole, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewRootCreateAction(workspacerolesResource, workspaceRole), &v1alpha2.WorkspaceRole{})
	if obj == nil {
		return nil, err
	}
	return obj.(*v1alpha2.WorkspaceRole), err
}

// Update takes the representation of a workspaceRole and updates it. Returns the server's representation of the workspaceRole, and an error, if there is any.
func (c *FakeWorkspaceRoles) Update(ctx context.Context, workspaceRole *v1alpha2.WorkspaceRole, opts v1.UpdateOptions) (result *v1alpha2.WorkspaceRole, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewRootUpdateAction(workspacerolesResource, workspaceRole), &v1alpha2.WorkspaceRole{})
	if obj == nil {
		return nil, err
	}
	return obj.(*v1alpha2.WorkspaceRole), err
}

// Delete takes name of the workspaceRole and deletes it. Returns an error if one occurs.
func (c *FakeWorkspaceRoles) Delete(ctx context.Context, name string, opts v1.DeleteOptions) error {
	_, err := c.Fake.
		Invokes(testing.NewRootDeleteAction(workspacerolesResource, name), &v1alpha2.WorkspaceRole{})
	return err
}

// DeleteCollection deletes a collection of objects.
func (c *FakeWorkspaceRoles) DeleteCollection(ctx context.Context, opts v1.DeleteOptions, listOpts v1.ListOptions) error {
	action := testing.NewRootDeleteCollectionAction(workspacerolesResource, listOpts)

	_, err := c.Fake.Invokes(action, &v1alpha2.WorkspaceRoleList{})
	return err
}

// Patch applies the patch and returns the patched workspaceRole.
func (c *FakeWorkspaceRoles) Patch(ctx context.Context, name string, pt types.PatchType, data []byte, opts v1.PatchOptions, subresources ...string) (result *v1alpha2.WorkspaceRole, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewRootPatchSubresourceAction(workspacerolesResource, name, pt, data, subresources...), &v1alpha2.WorkspaceRole{})
	if obj == nil {
		return nil, err
	}
	return obj.(*v1alpha2.WorkspaceRole), err
}
