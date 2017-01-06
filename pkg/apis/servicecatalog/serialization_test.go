/*
Copyright 2014 The Kubernetes Authors.

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

package servicecatalog_test

import (
	"encoding/hex"
	"fmt"
	"math/rand"
	"reflect"
	"strings"
	"testing"

	"github.com/davecgh/go-spew/spew"
	proto "github.com/golang/protobuf/proto"
	flag "github.com/spf13/pflag"

	"k8s.io/kubernetes/pkg/api"
	"k8s.io/kubernetes/pkg/api/meta"
	"k8s.io/kubernetes/pkg/api/testapi"
	apitesting "k8s.io/kubernetes/pkg/api/testing"
	"k8s.io/kubernetes/pkg/apimachinery/registered"
	"k8s.io/kubernetes/pkg/runtime"
	"k8s.io/kubernetes/pkg/runtime/schema"
	"k8s.io/kubernetes/pkg/util/diff"
	"k8s.io/kubernetes/pkg/util/sets"

	"github.com/kubernetes-incubator/service-catalog/pkg/apis/servicecatalog"

	_ "github.com/kubernetes-incubator/service-catalog/pkg/apis/servicecatalog/install"
)

// BABYNETES: ripped from pkg/api/serialization_test.go

var fuzzIters = flag.Int("fuzz-iters", 20, "How many fuzzing iterations to do.")

var codecsToTest = []func(version schema.GroupVersion, item runtime.Object) (runtime.Codec, bool, error){
	func(version schema.GroupVersion, item runtime.Object) (runtime.Codec, bool, error) {
		c, err := testapi.GetCodecForObject(item)
		return c, true, err
	},
}

func fuzzInternalObject(t *testing.T, forVersion schema.GroupVersion, item runtime.Object, seed int64) runtime.Object {
	apitesting.FuzzerFor(t, forVersion, rand.NewSource(seed)).Fuzz(item)

	j, err := meta.TypeAccessor(item)
	if err != nil {
		t.Fatalf("Unexpected error %v for %#v", err, item)
	}
	j.SetKind("")
	j.SetAPIVersion("")

	return item
}

func dataAsString(data []byte) string {
	dataString := string(data)
	if !strings.HasPrefix(dataString, "{") {
		dataString = "\n" + hex.Dump(data)
		proto.NewBuffer(make([]byte, 0, 1024)).DebugPrint("decoded object", data)
	}
	return dataString
}

func roundTrip(t *testing.T, codec runtime.Codec, item runtime.Object) {
	printer := spew.ConfigState{DisableMethods: true}

	original := item
	copied, err := api.Scheme.DeepCopy(item)
	if err != nil {
		panic(fmt.Sprintf("unable to copy: %v", err))
	}
	item = copied.(runtime.Object)

	name := reflect.TypeOf(item).Elem().Name()
	data, err := runtime.Encode(codec, item)
	if err != nil {
		if runtime.IsNotRegisteredError(err) {
			t.Logf("%v: not registered: %v (%s)", name, err, printer.Sprintf("%#v", item))
		} else {
			t.Errorf("%v: %v (%s)", name, err, printer.Sprintf("%#v", item))
		}
		return
	}

	if !api.Semantic.DeepEqual(original, item) {
		t.Errorf("0: %v: encode altered the object, diff: %v", name, diff.ObjectReflectDiff(original, item))
		return
	}

	obj2, err := runtime.Decode(codec, data)
	if err != nil {
		t.Errorf("0: %v: %v\nCodec: %#v\nData: %s\nSource: %#v", name, err, codec, dataAsString(data), printer.Sprintf("%#v", item))
		panic("failed")
	}
	if !api.Semantic.DeepEqual(original, obj2) {
		t.Errorf("\n1: %v: diff: %v\nCodec: %#v\nSource:\n\n%#v\n\nEncoded:\n\n%s\n\nFinal:\n\n%#v", name, diff.ObjectReflectDiff(item, obj2), codec, printer.Sprintf("%#v", item), dataAsString(data), printer.Sprintf("%#v", obj2))
		return
	}

	obj3 := reflect.New(reflect.TypeOf(item).Elem()).Interface().(runtime.Object)
	if err := runtime.DecodeInto(codec, data, obj3); err != nil {
		t.Errorf("2: %v: %v", name, err)
		return
	}
	if !api.Semantic.DeepEqual(item, obj3) {
		t.Errorf("3: %v: diff: %v\nCodec: %#v", name, diff.ObjectReflectDiff(item, obj3), codec)
		return
	}
}

// roundTripSame verifies the same source object is tested in all API versions.
func roundTripSame(t *testing.T, group TestGroup, item runtime.Object, except ...string) {
	set := sets.NewString(except...)
	seed := rand.Int63()
	fuzzInternalObject(t, group.InternalGroupVersion(), item, seed)

	version := *group.GroupVersion()
	codecs := []runtime.Codec{}
	for _, fn := range codecsToTest {
		codec, ok, err := fn(version, item)
		if err != nil {
			t.Errorf("unable to get codec: %v", err)
			return
		}
		if !ok {
			continue
		}
		codecs = append(codecs, codec)
	}

	if !set.Has(version.String()) {
		fuzzInternalObject(t, version, item, seed)
		for _, codec := range codecs {
			roundTrip(t, codec, item)
		}
	}
}

func serviceCatalogAPIGroup() TestGroup {
	groupVersion, err := schema.ParseGroupVersion("servicecatalog/v1alpha1")
	if err != nil {
		panic(fmt.Sprintf("Error parsing groupversion: %v", err))
	}

	externalGroupVersion := schema.GroupVersion{Group: servicecatalog.GroupName,
		Version: registered.GroupOrDie(servicecatalog.GroupName).GroupVersion.Version}

	return TestGroup{
		externalGroupVersion: groupVersion,
		internalGroupVersion: servicecatalog.SchemeGroupVersion,
		internalTypes:        api.Scheme.KnownTypes(servicecatalog.SchemeGroupVersion),
		externalTypes:        api.Scheme.KnownTypes(externalGroupVersion),
	}
}

// For debugging problems
func TestSpecificKind(t *testing.T) {
	group := serviceCatalogAPIGroup()

	for _, kind := range group.InternalTypes() {
		fmt.Println(kind)
	}

	kind := "Broker"
	for i := 0; i < *fuzzIters; i++ {
		doRoundTripTest(serviceCatalogAPIGroup(), kind, t)
		if t.Failed() {
			break
		}
	}
}

// func TestList(t *testing.T) {
// 	kind := "List"
// 	item, err := api.Scheme.New(api.SchemeGroupVersion.WithKind(kind))
// 	if err != nil {
// 		t.Errorf("Couldn't make a %v? %v", kind, err)
// 		return
// 	}
// 	roundTripSame(t, testapi.Default, item)
// }

var nonRoundTrippableTypes = sets.NewString(
	"ExportOptions",
	"GetOptions",
	// WatchEvent does not include kind and version and can only be deserialized
	// implicitly (if the caller expects the specific object). The watch call defines
	// the schema by content type, rather than via kind/version included in each
	// object.
	"WatchEvent",
)
var nonInternalRoundTrippableTypes = sets.NewString("List", "ListOptions", "ExportOptions")
var nonRoundTrippableTypesByVersion = map[string][]string{}

// func TestRoundTripTypes(t *testing.T) {
// 	for groupKey, group := range testapi.Groups {
// 		for kind := range group.InternalTypes() {
// 			t.Logf("working on %v in %v", kind, groupKey)
// 			if nonRoundTrippableTypes.Has(kind) {
// 				continue
// 			}
// 			// Try a few times, since runTest uses random values.
// 			for i := 0; i < *fuzzIters; i++ {
// 				doRoundTripTest(group, kind, t)
// 				if t.Failed() {
// 					break
// 				}
// 			}
// 		}
// 	}
// }

func doRoundTripTest(group TestGroup, kind string, t *testing.T) {
	item, err := api.Scheme.New(group.InternalGroupVersion().WithKind(kind))
	if err != nil {
		t.Fatalf("Couldn't make a %v? %v", kind, err)
	}
	if _, err := meta.TypeAccessor(item); err != nil {
		t.Fatalf("%q is not a TypeMeta and cannot be tested - add it to nonRoundTrippableTypes: %v", kind, err)
	}
	if api.Scheme.Recognizes(group.GroupVersion().WithKind(kind)) {
		roundTripSame(t, group, item, nonRoundTrippableTypesByVersion[kind]...)
	}
	if !nonInternalRoundTrippableTypes.Has(kind) && api.Scheme.Recognizes(group.GroupVersion().WithKind(kind)) {
		roundTrip(t, group.Codec(), fuzzInternalObject(t, group.InternalGroupVersion(), item, rand.Int63()))
	}
}

// func TestEncode_Ptr(t *testing.T) {
// 	grace := int64(30)
// 	pod := &api.Pod{
// 		ObjectMeta: api.ObjectMeta{
// 			Labels: map[string]string{"name": "foo"},
// 		},
// 		Spec: api.PodSpec{
// 			RestartPolicy: api.RestartPolicyAlways,
// 			DNSPolicy:     api.DNSClusterFirst,

// 			TerminationGracePeriodSeconds: &grace,

// 			SecurityContext: &api.PodSecurityContext{},
// 			Affinity:        &api.Affinity{},
// 		},
// 	}
// 	obj := runtime.Object(pod)
// 	data, err := runtime.Encode(testapi.Default.Codec(), obj)
// 	obj2, err2 := runtime.Decode(testapi.Default.Codec(), data)
// 	if err != nil || err2 != nil {
// 		t.Fatalf("Failure: '%v' '%v'", err, err2)
// 	}
// 	if _, ok := obj2.(*api.Pod); !ok {
// 		t.Fatalf("Got wrong type")
// 	}
// 	if !api.Semantic.DeepEqual(obj2, pod) {
// 		t.Errorf("\nExpected:\n\n %#v,\n\nGot:\n\n %#vDiff: %v\n\n", pod, obj2, diff.ObjectDiff(obj2, pod))

// 	}
// }

func TestBadJSONRejection(t *testing.T) {
	badJSONMissingKind := []byte(`{ }`)
	if _, err := runtime.Decode(testapi.Default.Codec(), badJSONMissingKind); err == nil {
		t.Errorf("Did not reject despite lack of kind field: %s", badJSONMissingKind)
	}
	badJSONUnknownType := []byte(`{"kind": "bar"}`)
	if _, err1 := runtime.Decode(testapi.Default.Codec(), badJSONUnknownType); err1 == nil {
		t.Errorf("Did not reject despite use of unknown type: %s", badJSONUnknownType)
	}
	/*badJSONKindMismatch := []byte(`{"kind": "Pod"}`)
	if err2 := DecodeInto(badJSONKindMismatch, &Node{}); err2 == nil {
		t.Errorf("Kind is set but doesn't match the object type: %s", badJSONKindMismatch)
	}*/
}

type TestGroup struct {
	externalGroupVersion schema.GroupVersion
	internalGroupVersion schema.GroupVersion
	internalTypes        map[string]reflect.Type
	externalTypes        map[string]reflect.Type
}

func (g TestGroup) ContentConfig() (string, *schema.GroupVersion, runtime.Codec) {
	return "application/json", g.GroupVersion(), g.Codec()
}

func (g TestGroup) GroupVersion() *schema.GroupVersion {
	copyOfGroupVersion := g.externalGroupVersion
	return &copyOfGroupVersion
}

// InternalGroupVersion returns the group,version used to identify the internal
// types for this API
func (g TestGroup) InternalGroupVersion() schema.GroupVersion {
	return g.internalGroupVersion
}

// InternalTypes returns a map of internal API types' kind names to their Go types.
func (g TestGroup) InternalTypes() map[string]reflect.Type {
	return g.internalTypes
}

// ExternalTypes returns a map of external API types' kind names to their Go types.
func (g TestGroup) ExternalTypes() map[string]reflect.Type {
	return g.externalTypes
}

// Codec returns the codec for the API version to test against, as set by the
// KUBE_TEST_API_TYPE env var.
func (g TestGroup) Codec() runtime.Codec {
	return api.Codecs.LegacyCodec(g.externalGroupVersion)
}

// NegotiatedSerializer returns the negotiated serializer for the server.
func (g TestGroup) NegotiatedSerializer() runtime.NegotiatedSerializer {
	return api.Codecs
}
