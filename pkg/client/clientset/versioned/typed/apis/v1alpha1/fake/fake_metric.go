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

package fake

import (
	"context"

	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	labels "k8s.io/apimachinery/pkg/labels"
	types "k8s.io/apimachinery/pkg/types"
	watch "k8s.io/apimachinery/pkg/watch"
	testing "k8s.io/client-go/testing"
	v1alpha1 "sigs.k8s.io/kwok/pkg/apis/v1alpha1"
)

// FakeMetrics implements MetricInterface
type FakeMetrics struct {
	Fake *FakeKwokV1alpha1
}

var metricsResource = v1alpha1.SchemeGroupVersion.WithResource("metrics")

var metricsKind = v1alpha1.SchemeGroupVersion.WithKind("Metric")

// Get takes name of the metric, and returns the corresponding metric object, and an error if there is any.
func (c *FakeMetrics) Get(ctx context.Context, name string, options v1.GetOptions) (result *v1alpha1.Metric, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewRootGetAction(metricsResource, name), &v1alpha1.Metric{})
	if obj == nil {
		return nil, err
	}
	return obj.(*v1alpha1.Metric), err
}

// List takes label and field selectors, and returns the list of Metrics that match those selectors.
func (c *FakeMetrics) List(ctx context.Context, opts v1.ListOptions) (result *v1alpha1.MetricList, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewRootListAction(metricsResource, metricsKind, opts), &v1alpha1.MetricList{})
	if obj == nil {
		return nil, err
	}

	label, _, _ := testing.ExtractFromListOptions(opts)
	if label == nil {
		label = labels.Everything()
	}
	list := &v1alpha1.MetricList{ListMeta: obj.(*v1alpha1.MetricList).ListMeta}
	for _, item := range obj.(*v1alpha1.MetricList).Items {
		if label.Matches(labels.Set(item.Labels)) {
			list.Items = append(list.Items, item)
		}
	}
	return list, err
}

// Watch returns a watch.Interface that watches the requested metrics.
func (c *FakeMetrics) Watch(ctx context.Context, opts v1.ListOptions) (watch.Interface, error) {
	return c.Fake.
		InvokesWatch(testing.NewRootWatchAction(metricsResource, opts))
}

// Create takes the representation of a metric and creates it.  Returns the server's representation of the metric, and an error, if there is any.
func (c *FakeMetrics) Create(ctx context.Context, metric *v1alpha1.Metric, opts v1.CreateOptions) (result *v1alpha1.Metric, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewRootCreateAction(metricsResource, metric), &v1alpha1.Metric{})
	if obj == nil {
		return nil, err
	}
	return obj.(*v1alpha1.Metric), err
}

// Update takes the representation of a metric and updates it. Returns the server's representation of the metric, and an error, if there is any.
func (c *FakeMetrics) Update(ctx context.Context, metric *v1alpha1.Metric, opts v1.UpdateOptions) (result *v1alpha1.Metric, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewRootUpdateAction(metricsResource, metric), &v1alpha1.Metric{})
	if obj == nil {
		return nil, err
	}
	return obj.(*v1alpha1.Metric), err
}

// UpdateStatus was generated because the type contains a Status member.
// Add a +genclient:noStatus comment above the type to avoid generating UpdateStatus().
func (c *FakeMetrics) UpdateStatus(ctx context.Context, metric *v1alpha1.Metric, opts v1.UpdateOptions) (*v1alpha1.Metric, error) {
	obj, err := c.Fake.
		Invokes(testing.NewRootUpdateSubresourceAction(metricsResource, "status", metric), &v1alpha1.Metric{})
	if obj == nil {
		return nil, err
	}
	return obj.(*v1alpha1.Metric), err
}

// Delete takes name of the metric and deletes it. Returns an error if one occurs.
func (c *FakeMetrics) Delete(ctx context.Context, name string, opts v1.DeleteOptions) error {
	_, err := c.Fake.
		Invokes(testing.NewRootDeleteActionWithOptions(metricsResource, name, opts), &v1alpha1.Metric{})
	return err
}

// DeleteCollection deletes a collection of objects.
func (c *FakeMetrics) DeleteCollection(ctx context.Context, opts v1.DeleteOptions, listOpts v1.ListOptions) error {
	action := testing.NewRootDeleteCollectionAction(metricsResource, listOpts)

	_, err := c.Fake.Invokes(action, &v1alpha1.MetricList{})
	return err
}

// Patch applies the patch and returns the patched metric.
func (c *FakeMetrics) Patch(ctx context.Context, name string, pt types.PatchType, data []byte, opts v1.PatchOptions, subresources ...string) (result *v1alpha1.Metric, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewRootPatchSubresourceAction(metricsResource, name, pt, data, subresources...), &v1alpha1.Metric{})
	if obj == nil {
		return nil, err
	}
	return obj.(*v1alpha1.Metric), err
}
