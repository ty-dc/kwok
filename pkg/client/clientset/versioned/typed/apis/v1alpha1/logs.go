/*
Copyright 2023 The Kubernetes Authors.

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

package v1alpha1

import (
	"context"
	"time"

	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	types "k8s.io/apimachinery/pkg/types"
	watch "k8s.io/apimachinery/pkg/watch"
	rest "k8s.io/client-go/rest"
	v1alpha1 "sigs.k8s.io/kwok/pkg/apis/v1alpha1"
	scheme "sigs.k8s.io/kwok/pkg/client/clientset/versioned/scheme"
)

// LogsGetter has a method to return a LogsInterface.
// A group's client should implement this interface.
type LogsGetter interface {
	Logs(namespace string) LogsInterface
}

// LogsInterface has methods to work with Logs resources.
type LogsInterface interface {
	Create(ctx context.Context, logs *v1alpha1.Logs, opts v1.CreateOptions) (*v1alpha1.Logs, error)
	Update(ctx context.Context, logs *v1alpha1.Logs, opts v1.UpdateOptions) (*v1alpha1.Logs, error)
	UpdateStatus(ctx context.Context, logs *v1alpha1.Logs, opts v1.UpdateOptions) (*v1alpha1.Logs, error)
	Delete(ctx context.Context, name string, opts v1.DeleteOptions) error
	DeleteCollection(ctx context.Context, opts v1.DeleteOptions, listOpts v1.ListOptions) error
	Get(ctx context.Context, name string, opts v1.GetOptions) (*v1alpha1.Logs, error)
	List(ctx context.Context, opts v1.ListOptions) (*v1alpha1.LogsList, error)
	Watch(ctx context.Context, opts v1.ListOptions) (watch.Interface, error)
	Patch(ctx context.Context, name string, pt types.PatchType, data []byte, opts v1.PatchOptions, subresources ...string) (result *v1alpha1.Logs, err error)
	LogsExpansion
}

// logs implements LogsInterface
type logs struct {
	client rest.Interface
	ns     string
}

// newLogs returns a Logs
func newLogs(c *KwokV1alpha1Client, namespace string) *logs {
	return &logs{
		client: c.RESTClient(),
		ns:     namespace,
	}
}

// Get takes name of the logs, and returns the corresponding logs object, and an error if there is any.
func (c *logs) Get(ctx context.Context, name string, options v1.GetOptions) (result *v1alpha1.Logs, err error) {
	result = &v1alpha1.Logs{}
	err = c.client.Get().
		Namespace(c.ns).
		Resource("logs").
		Name(name).
		VersionedParams(&options, scheme.ParameterCodec).
		Do(ctx).
		Into(result)
	return
}

// List takes label and field selectors, and returns the list of Logs that match those selectors.
func (c *logs) List(ctx context.Context, opts v1.ListOptions) (result *v1alpha1.LogsList, err error) {
	var timeout time.Duration
	if opts.TimeoutSeconds != nil {
		timeout = time.Duration(*opts.TimeoutSeconds) * time.Second
	}
	result = &v1alpha1.LogsList{}
	err = c.client.Get().
		Namespace(c.ns).
		Resource("logs").
		VersionedParams(&opts, scheme.ParameterCodec).
		Timeout(timeout).
		Do(ctx).
		Into(result)
	return
}

// Watch returns a watch.Interface that watches the requested logs.
func (c *logs) Watch(ctx context.Context, opts v1.ListOptions) (watch.Interface, error) {
	var timeout time.Duration
	if opts.TimeoutSeconds != nil {
		timeout = time.Duration(*opts.TimeoutSeconds) * time.Second
	}
	opts.Watch = true
	return c.client.Get().
		Namespace(c.ns).
		Resource("logs").
		VersionedParams(&opts, scheme.ParameterCodec).
		Timeout(timeout).
		Watch(ctx)
}

// Create takes the representation of a logs and creates it.  Returns the server's representation of the logs, and an error, if there is any.
func (c *logs) Create(ctx context.Context, logs *v1alpha1.Logs, opts v1.CreateOptions) (result *v1alpha1.Logs, err error) {
	result = &v1alpha1.Logs{}
	err = c.client.Post().
		Namespace(c.ns).
		Resource("logs").
		VersionedParams(&opts, scheme.ParameterCodec).
		Body(logs).
		Do(ctx).
		Into(result)
	return
}

// Update takes the representation of a logs and updates it. Returns the server's representation of the logs, and an error, if there is any.
func (c *logs) Update(ctx context.Context, logs *v1alpha1.Logs, opts v1.UpdateOptions) (result *v1alpha1.Logs, err error) {
	result = &v1alpha1.Logs{}
	err = c.client.Put().
		Namespace(c.ns).
		Resource("logs").
		Name(logs.Name).
		VersionedParams(&opts, scheme.ParameterCodec).
		Body(logs).
		Do(ctx).
		Into(result)
	return
}

// UpdateStatus was generated because the type contains a Status member.
// Add a +genclient:noStatus comment above the type to avoid generating UpdateStatus().
func (c *logs) UpdateStatus(ctx context.Context, logs *v1alpha1.Logs, opts v1.UpdateOptions) (result *v1alpha1.Logs, err error) {
	result = &v1alpha1.Logs{}
	err = c.client.Put().
		Namespace(c.ns).
		Resource("logs").
		Name(logs.Name).
		SubResource("status").
		VersionedParams(&opts, scheme.ParameterCodec).
		Body(logs).
		Do(ctx).
		Into(result)
	return
}

// Delete takes name of the logs and deletes it. Returns an error if one occurs.
func (c *logs) Delete(ctx context.Context, name string, opts v1.DeleteOptions) error {
	return c.client.Delete().
		Namespace(c.ns).
		Resource("logs").
		Name(name).
		Body(&opts).
		Do(ctx).
		Error()
}

// DeleteCollection deletes a collection of objects.
func (c *logs) DeleteCollection(ctx context.Context, opts v1.DeleteOptions, listOpts v1.ListOptions) error {
	var timeout time.Duration
	if listOpts.TimeoutSeconds != nil {
		timeout = time.Duration(*listOpts.TimeoutSeconds) * time.Second
	}
	return c.client.Delete().
		Namespace(c.ns).
		Resource("logs").
		VersionedParams(&listOpts, scheme.ParameterCodec).
		Timeout(timeout).
		Body(&opts).
		Do(ctx).
		Error()
}

// Patch applies the patch and returns the patched logs.
func (c *logs) Patch(ctx context.Context, name string, pt types.PatchType, data []byte, opts v1.PatchOptions, subresources ...string) (result *v1alpha1.Logs, err error) {
	result = &v1alpha1.Logs{}
	err = c.client.Patch(pt).
		Namespace(c.ns).
		Resource("logs").
		Name(name).
		SubResource(subresources...).
		VersionedParams(&opts, scheme.ParameterCodec).
		Body(data).
		Do(ctx).
		Into(result)
	return
}
