/*
Copyright 2016 The Rook Authors. All rights reserved.

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
package rgw

import (
	"path"
	"testing"

	testceph "github.com/rook/rook/pkg/cephmgr/client/test"
	"github.com/rook/rook/pkg/clusterd/inventory"

	"github.com/rook/rook/pkg/clusterd"
	"github.com/rook/rook/pkg/util"

	"github.com/stretchr/testify/assert"
)

func TestRGWConfig(t *testing.T) {
	etcdClient := util.NewMockEtcdClient()
	nodes := map[string]*inventory.NodeConfig{
		"a": &inventory.NodeConfig{PublicIP: "1.2.3.4"},
		"b": &inventory.NodeConfig{PublicIP: "2.3.4.5"},
	}
	context := &clusterd.Context{EtcdClient: etcdClient, Inventory: &inventory.Config{Nodes: nodes}}
	factory := &testceph.MockConnectionFactory{Fsid: "f", SecretKey: "k"}
	leader := NewLeader()

	// Nothing happens when not in desired state
	err := leader.Configure(context, factory)
	assert.Nil(t, err)
	desired, err := getObjectStoreState(context, false)
	assert.Nil(t, err)
	assert.False(t, desired)
	applied, err := getObjectStoreState(context, true)
	assert.Nil(t, err)
	assert.False(t, applied)

	// Add the object store to desired state
	err = EnableObjectStore(context)
	assert.Nil(t, err)
	desired, _ = getObjectStoreState(context, false)
	assert.True(t, desired)

	etcdClient.WatcherResponses["/rook/_notify/a/rgw/status"] = "succeeded"
	etcdClient.WatcherResponses["/rook/_notify/b/rgw/status"] = "succeeded"

	// Configure the object store
	err = leader.Configure(context, factory)
	assert.Nil(t, err)
	verifyObjectConfigured(t, context, true)

	// Remove the object service
	err = RemoveObjectStore(context)
	assert.Nil(t, err)
	err = leader.Configure(context, factory)
	assert.Nil(t, err)
	verifyObjectConfigured(t, context, false)
}

func verifyObjectConfigured(t *testing.T, context *clusterd.Context, configured bool) {
	desired, err := getObjectStoreState(context, false)
	assert.Nil(t, err)
	assert.Equal(t, configured, desired)
	applied, err := getObjectStoreState(context, true)
	assert.Nil(t, err)
	assert.Equal(t, configured, applied)

	// Check that both nodes are in desired and applied state
	desired, err = getRGWState(context.EtcdClient, "a", false)
	assert.Nil(t, err)
	assert.Equal(t, configured, desired)
	desired, err = getRGWState(context.EtcdClient, "b", false)
	assert.Nil(t, err)
	assert.Equal(t, configured, desired)
	desired, err = getRGWState(context.EtcdClient, "a", true)
	assert.Nil(t, err)
	assert.Equal(t, configured, desired)
	desired, err = getRGWState(context.EtcdClient, "b", true)
	assert.Nil(t, err)
	assert.Equal(t, configured, desired)
}

func TestDefaultDesiredState(t *testing.T) {
	etcdClient := util.NewMockEtcdClient()
	context := &clusterd.Context{EtcdClient: etcdClient}

	err := EnableObjectStore(context)
	assert.Nil(t, err)
	assert.Equal(t, "1", etcdClient.GetValue("/rook/services/ceph/object/desired"))

	err = RemoveObjectStore(context)
	assert.Nil(t, err)
	assert.Equal(t, 0, etcdClient.GetChildDirs("/rook/services/ceph/object/desired").Count())
}

func TestMarkApplied(t *testing.T) {
	etcdClient := util.NewMockEtcdClient()
	context := &clusterd.Context{EtcdClient: etcdClient}

	err := markApplied(context)
	assert.Nil(t, err)

	assert.Equal(t, "1", etcdClient.GetValue("/rook/services/ceph/object/applied"))
}

func TestGetDesiredNodes(t *testing.T) {
	etcdClient := util.NewMockEtcdClient()
	nodes := map[string]*inventory.NodeConfig{}
	context := &clusterd.Context{EtcdClient: etcdClient, Inventory: &inventory.Config{Nodes: nodes}}
	leader := NewLeader()

	// no nodes to select
	desired, err := leader.getDesiredRGWNodes(context, 0)
	assert.Nil(t, err)
	assert.Equal(t, 0, len(desired))

	nodes["a"] = &inventory.NodeConfig{}
	nodes["b"] = &inventory.NodeConfig{}

	// no nodes desired
	desired, err = leader.getDesiredRGWNodes(context, 0)
	assert.Nil(t, err)
	assert.Equal(t, 0, len(desired))

	// select only one node that was already in desired state
	etcdClient.SetValue(path.Join(getRGWNodeKey("a", false), "state"), "1")
	desired, err = leader.getDesiredRGWNodes(context, 1)
	assert.Nil(t, err)
	assert.Equal(t, 1, len(desired))
	assert.Equal(t, "a", desired[0])

	// select both nodes
	desired, err = leader.getDesiredRGWNodes(context, 2)
	assert.Nil(t, err)
	assert.Equal(t, 2, len(desired))

	// fail to select three nodes
	desired, err = leader.getDesiredRGWNodes(context, 3)
	assert.NotNil(t, err)
}
