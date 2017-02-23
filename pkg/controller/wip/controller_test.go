/*
Copyright 2017 The Kubernetes Authors.

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

package wip

import (
	"testing"
	"time"

	"github.com/golang/glog"
	"github.com/kubernetes-incubator/service-catalog/pkg/apis/servicecatalog/v1alpha1"
	"github.com/kubernetes-incubator/service-catalog/pkg/brokerapi"
	fakebrokerapi "github.com/kubernetes-incubator/service-catalog/pkg/brokerapi/fake"
	servicecatalogclientset "github.com/kubernetes-incubator/service-catalog/pkg/client/clientset_generated/clientset/fake"
	servicecataloginformers "github.com/kubernetes-incubator/service-catalog/pkg/client/informers"

	"k8s.io/client-go/1.5/kubernetes/fake"
	"k8s.io/kubernetes/pkg/api/v1"
)

func TestReconcileBroker(t *testing.T) {
	// create a fake kube client
	fakeClient := &fake.Clientset{}
	// create a fake sc client
	fakeCatalogClient := &servicecatalogclientset.Clientset{}
	// create a fake broker client
	//fakeBrokerClient := fakebrokerapi.Client{}

	catalogCl := &fakebrokerapi.CatalogClient{
		RetCatalog: &brokerapi.Catalog{
			Services: []*brokerapi.Service{{
				Name:        "test-service",
				ID:          "12345",
				Description: "a test service",
				Plans: []brokerapi.ServicePlan{{
					Name:        "test-plan",
					Free:        true,
					ID:          "34567",
					Description: "a test plan",
				}},
			}},
		},
	}
	instanceCl := fakebrokerapi.NewInstanceClient()
	bindingCl := fakebrokerapi.NewBindingClient()
	brokerClFunc := fakebrokerapi.NewClientFunc(catalogCl, instanceCl, bindingCl)

	resyncDuration, err := time.ParseDuration("1m")
	if err != nil {
		glog.Fatal(err)
	}

	// create informers
	informerFactory := servicecataloginformers.NewSharedInformerFactory(nil, fakeCatalogClient, resyncDuration)
	serviceCatalogSharedInformers := informerFactory.Servicecatalog().V1alpha1()

	// create a test controller
	testController, err := NewController(
		fakeClient,
		fakeCatalogClient.ServicecatalogV1alpha1(),
		serviceCatalogSharedInformers.Brokers(),
		serviceCatalogSharedInformers.ServiceClasses(),
		serviceCatalogSharedInformers.Instances(),
		serviceCatalogSharedInformers.Bindings(),
		brokerClFunc,
	)
	if err != nil {
		glog.Fatal(err)
	}

	broker := &v1alpha1.Broker{
		ObjectMeta: v1.ObjectMeta{Name: "name"},
		Spec: v1alpha1.BrokerSpec{
			URL:     "https://example.com",
			OSBGUID: "OSBGUID field",
		},
	}
	stopCh := make(chan struct{})

	// inject a broker resource into broker informer
	serviceCatalogSharedInformers.Brokers().Informer().GetStore().Add(broker)

	t.Logf("%+v\n", fakeCatalogClient.Actions())

	go testController.Run(stopCh)
	// verify broker's catalog method is called
	// verify sc client has service classes created
	// verify no kube resources created
	stopCh <- struct{}{}
}
