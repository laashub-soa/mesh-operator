/*
Copyright 2020 The Symcn Authors.

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

package configuredservice

import (
	"context"
	"errors"

	"github.com/golang/mock/gomock"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	meshv1alpha1 "github.com/symcn/mesh-operator/api/v1alpha1"
	. "github.com/symcn/mesh-operator/test"
	v1beta1 "istio.io/api/networking/v1beta1"
	networkingv1beta1 "istio.io/client-go/pkg/apis/networking/v1beta1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
)

var _ = Describe("ServiceEntry", func() {
	var (
		mockCtrl                *gomock.Controller
		mockClient              *MockClient
		existedServiceEntry     *networkingv1beta1.ServiceEntry
		toBeDeletedServiceEntry *networkingv1beta1.ServiceEntry
		noInstanceCr            *meshv1alpha1.ConfiguredService
	)

	BeforeEach(func() {
		noInstanceCr = &meshv1alpha1.ConfiguredService{
			ObjectMeta: v1.ObjectMeta{
				Name:      "test.no.instance.cr",
				Namespace: "mesh-test",
			},
			Spec: meshv1alpha1.ConfiguredServiceSpec{
				OriginalName:         "Test.No.Instance.Cr",
				Instances:            nil,
				MeshConfigGeneration: 0,
			},
		}
		existedServiceEntry = &networkingv1beta1.ServiceEntry{
			ObjectMeta: v1.ObjectMeta{
				Name:      "test.no.instance.cr",
				Namespace: "mesh-test",
				Labels: map[string]string{
					testOpt.SelectLabel: "Test.No.Instance.Cr",
				},
			},
			Spec: v1beta1.ServiceEntry{
				Hosts: []string{"before.update.host"},
			},
		}
		toBeDeletedServiceEntry = &networkingv1beta1.ServiceEntry{
			ObjectMeta: v1.ObjectMeta{
				Name:      "test.to.be.delete",
				Namespace: "mesh-test",
				Labels: map[string]string{
					testOpt.SelectLabel: "Test.No.Instance.Cr",
				},
			},
			Spec: v1beta1.ServiceEntry{
				Hosts: []string{"before.update.host"},
			},
		}
	})

	Describe("test reconcile serviceentry use mock client Get function", func() {
		BeforeEach(func() {
			mockCtrl = gomock.NewController(GinkgoT())
			mockClient = NewMockClient(mockCtrl)
			mockClient.EXPECT().Create(gomock.Any(), gomock.Any(), gomock.Any()).Return(errors.New("create se error"))
			mockClient.EXPECT().List(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)
		}, 5)
		AfterEach(func() {
			mockCtrl.Finish()
		}, 5)

		Context("test crate serviceentry error", func() {
			It("return create error from mock client", func() {
				r := Reconciler{
					Client:     mockClient,
					Log:        nil,
					Scheme:     getFakeScheme(),
					Opt:        testOpt,
					MeshConfig: getTestMeshConfig(),
				}
				err := r.reconcileServiceEntry(context.Background(), noInstanceCr)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(Equal("create se error"))
			})
		})
	})

	Describe("test reconcile serviceentry use mock client List function", func() {
		BeforeEach(func() {
			mockCtrl = gomock.NewController(GinkgoT())
			mockClient = NewMockClient(mockCtrl)
			mockClient.EXPECT().List(gomock.Any(), gomock.Any(), gomock.Any()).Return(errors.New("no se exist"))
		}, 5)
		AfterEach(func() {
			mockCtrl.Finish()
		}, 5)

		Context("get serviceentries map error", func() {
			It("no serviceentry exist", func() {
				r := Reconciler{
					Client:     mockClient,
					Log:        nil,
					Scheme:     getFakeScheme(),
					Opt:        testOpt,
					MeshConfig: getTestMeshConfig(),
				}
				err := r.reconcileServiceEntry(context.Background(), noInstanceCr)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(Equal("no se exist"))
			}, 5)
		})

	})

	Describe("test reconcile serviceentry without mock client", func() {
		Context("test set controller reference error", func() {
			It("no kind is registered in scheme", func() {
				r := Reconciler{
					Client:     getFakeClient(),
					Log:        nil,
					Scheme:     runtime.NewScheme(),
					Opt:        testOpt,
					MeshConfig: getTestMeshConfig(),
				}
				err := r.reconcileServiceEntry(context.Background(), noInstanceCr)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(Equal(`no kind is registered for the type v1alpha1.ConfiguredService in scheme "pkg/runtime/scheme.go:101"`))
			}, 5)
		})

		Context("test update serviceentry success", func() {
			It("no error occurred", func() {
				r := Reconciler{
					Client:     getFakeClient(existedServiceEntry),
					Log:        nil,
					Scheme:     getFakeScheme(),
					Opt:        testOpt,
					MeshConfig: getTestMeshConfig(),
				}
				err := r.reconcileServiceEntry(context.Background(), noInstanceCr)
				Expect(err).NotTo(HaveOccurred())

				By("get updated serviceentry")
				found := &networkingv1beta1.ServiceEntry{}
				err = r.Get(context.Background(), types.NamespacedName{Namespace: "mesh-test", Name: "test.no.instance.cr"}, found)
				Expect(err).NotTo(HaveOccurred())
				Expect(found.Name).To(Equal("test.no.instance.cr"))
				Expect(found.Spec.Hosts).To(ContainElement("test.no.instance.cr"))
				Expect(found.Spec.Hosts).NotTo(ContainElement("before.update.host"))
			}, 5)

			It("delete unused serviceentry", func() {
				r := Reconciler{
					Client:     getFakeClient(existedServiceEntry, toBeDeletedServiceEntry),
					Log:        nil,
					Scheme:     getFakeScheme(),
					Opt:        testOpt,
					MeshConfig: getTestMeshConfig(),
				}
				err := r.reconcileServiceEntry(context.Background(), noInstanceCr)
				Expect(err).NotTo(HaveOccurred())

				By("get updated serviceentry")
				found := &networkingv1beta1.ServiceEntry{}
				err = r.Get(context.Background(), types.NamespacedName{Namespace: "mesh-test", Name: "test.to.be.delete"}, found)
				Expect(err).To(HaveOccurred())
				Expect(apierrors.IsNotFound(err)).To(Equal(true))
			}, 5)
		})
	})
})
