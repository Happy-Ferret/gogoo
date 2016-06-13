package gce

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/iKala/gogoo/config"

	"github.com/facebookgo/inject"
	"github.com/iKala/gosak"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	"google.golang.org/api/compute/v1"
)

var tested Manager
var projID string
var zone = "asia-east1-b"
var region = "asia-east1"
var vmName = "instance-test"
var diskName = "disk-test"

/*
 * Prepare below resources before running test
 */
var snapshotName = "snapshot-test"
var instanceGroupName = "instance-group-test"
var targetPoolName = "target-pool-test"
var instanceTemplateName = "instance-template-test"
var instanceGroupManagerName = "instance-group-manager-test"

func TestGceManagerTestSuite(t *testing.T) {
	suite.Run(t, new(GceManagerTestSuite))
}

type GceManagerTestSuite struct {
	suite.Suite
}

func (suite *GceManagerTestSuite) SetupSuite() {
	gcloudConfig := config.LoadGcloudConfig(config.LoadAsset("/config/config.json"))
	key, _ := ioutil.ReadAll(config.LoadAsset("/config/key.pem"))

	projID = gcloudConfig.ProjectID

	// Construct dependency graph
	computeService, _ := BuildGceService(gcloudConfig.ServiceAccount, key)

	var g inject.Graph
	err := g.Provide(
		&inject.Object{Value: computeService},
		&inject.Object{Value: &tested},
	)
	if err != nil {
		os.Exit(1)
	}
	if err := g.Populate(); err != nil {
		os.Exit(1)
	}
	// :~)

	// Prepare testVM/testDisk
	if !testing.Short() {
		createTestVM()
		_, err := getTestVM()
		require.Nil(suite.T(), err)

		createTestDisk()
		_, err = getTestDisk()
		require.Nil(suite.T(), err)
	}

	log.Println("======== SetupSuite  ========")
}

func (suite *GceManagerTestSuite) Test_GetVM() {
	newVM, _ := tested.GetVM(projID, zone, vmName)

	if testing.Verbose() {
		result, _ := json.MarshalIndent(newVM, "", "  ")
		log.Println(string(result))
	}
}

func (suite *GceManagerTestSuite) Test_StopVMThenStartVM() {
	// Stop VM
	var stoppedChecker = func(projectID, zone, instanceName string) (bool, error) {
		instanceStoppedObserver := make(chan bool)
		go tested.ProbeVMStopped(projectID, zone, instanceName, instanceStoppedObserver)

		done := <-instanceStoppedObserver
		if !done {
			return false, fmt.Errorf("VM not stopped: instance[%s]", instanceName)
		}
		return true, nil
	}
	tested.StopVM(projID, zone, vmName, stoppedChecker)

	// Assert VM stopped
	vm, _ := getTestVM()
	assert.Equal(suite.T(), VMStatusTerminated, vm.Status)

	// SetMachineType
	tested.SetMachineType(projID, zone, vmName, "f1-micro")

	// Start VM
	var preparedChecker = func(projectID, zone, instanceName string) (bool, error) {
		instanceRunningObserver := make(chan bool)
		go tested.ProbeVMRunning(projectID, zone, instanceName, instanceRunningObserver)

		if done := <-instanceRunningObserver; !done {
			return false, fmt.Errorf("VM not running")
		}
		return true, nil
	}
	tested.StartVM(projID, zone, vmName, preparedChecker)

	// Assert VM started and machine type changed
	vm, _ = getTestVM()
	assert.Equal(suite.T(), VMStatusRunning, vm.Status)
	assert.True(suite.T(), strings.Contains(vm.MachineType, "f1-micro"))
}

func (suite *GceManagerTestSuite) Test_ListVMs() {
	instanceList, _ := tested.ListVMs(projID, zone)

	assert.Equal(suite.T(), 1, len(instanceList.Items))

	if testing.Verbose() {
		// List all instances
		for _, v := range instanceList.Items {
			result, _ := json.MarshalIndent(v, "", "  ")
			log.Println(string(result))
		}
	}
}

func (suite *GceManagerTestSuite) Test_ListVMsWithFilter() {
	instanceList, err := tested.ListVMsWithFilter(projID, zone, "name eq instance-.*")

	assert.Nil(suite.T(), err)
	assert.Equal(suite.T(), 1, len(instanceList.Items))

	if testing.Verbose() {
		// List all instances
		for _, v := range instanceList.Items {
			result, _ := json.MarshalIndent(v, "", "  ")
			log.Println(string(result))
		}
	}
}

func (suite *GceManagerTestSuite) Test_ListImages() {
	imageList, _ := tested.ListImages(projID)

	assert.Equal(suite.T(), 1, len(imageList.Items))

	if testing.Verbose() {
		for _, v := range imageList.Items {
			result, _ := json.MarshalIndent(v, "", "  ")
			log.Println(string(result))
		}
	}
}

func (suite *GceManagerTestSuite) Test_ListDisks() {
	disks, _ := tested.ListDisks(projID, zone)

	assert.NotZero(suite.T(), len(disks.Items))

	if testing.Verbose() {
		log.Printf("disks: num[%d]", len(disks.Items))
	}
}

func (suite *GceManagerTestSuite) Test_GetNatIP() {
	vm, _ := tested.GetVM(projID, zone, vmName)

	natIP := tested.GetNatIP(vm)
	networkIP := tested.GetNetworkIP(vm)

	assert.True(suite.T(), gosak.IsIP(natIP))
	assert.True(suite.T(), gosak.IsIP(networkIP))
}

func (suite *GceManagerTestSuite) Test_ListSnapshots() {
	snapshots, err := tested.ListSnapshots(projID)

	assert.Nil(suite.T(), err)
	assert.Equal(suite.T(), 1, len(snapshots))
	assert.Equal(suite.T(), snapshotName, snapshots[0].Name)
}

func (suite *GceManagerTestSuite) Test_GetSnapshot() {
	snapshot, _ := tested.GetSnapshot(projID, snapshotName)

	assert.Equal(suite.T(), snapshotName, snapshot.Name)
}

func (suite *GceManagerTestSuite) Test_GetSnapshotOfDisk() {
	disk, _ := getTestDisk()

	assert.Equal(suite.T(), snapshotName, tested.GetSnapshotOfDisk(disk))
}

func (suite *GceManagerTestSuite) Test_GetLatestSnapshot() {
	testedSnapshots := []*compute.Snapshot{
		&compute.Snapshot{
			Name: "zebra-rtc-alpha-snapshot-201503032021"},
		&compute.Snapshot{
			Name: "zebra-rtc-alpha-snapshot-201502161516"},
		&compute.Snapshot{
			Name: "zebra-rtc-alpha-snapshot-201502131103"},
	}

	latestSnapshot, _ := tested.GetLatestSnapshot("alpha", testedSnapshots)

	assert.Equal(suite.T(), "zebra-rtc-alpha-snapshot-201503032021", latestSnapshot.Name)
}

func (suite *GceManagerTestSuite) Test_AttachTagsThenDetachTags() {
	vm, _ := tested.GetVM(projID, zone, vmName)
	assert.Equal(suite.T(), 0, len(vm.Tags.Items))

	// Attach tags
	tested.AttachTags(projID, zone, vmName, []string{"tag1", "tag2"})
	time.Sleep(5 * time.Second)

	vm, _ = tested.GetVM(projID, zone, vmName)
	assert.Equal(suite.T(), 2, len(vm.Tags.Items))
	assert.True(suite.T(), gosak.InStringSlice(vm.Tags.Items, "tag1"))
	assert.True(suite.T(), gosak.InStringSlice(vm.Tags.Items, "tag2"))

	// Detach tags
	tested.DetachTags(projID, zone, vmName, []string{"tag2"})
	time.Sleep(5 * time.Second)

	vm, _ = tested.GetVM(projID, zone, vmName)
	assert.Equal(suite.T(), 1, len(vm.Tags.Items))
	assert.Equal(suite.T(), "tag1", vm.Tags.Items[0])
}

func (suite *GceManagerTestSuite) Test_PatchInstanceMachineType() {
	result := tested.PatchInstanceMachineType(
		"https://www.googleapis.com/compute/v1/projects/livehouse-test/zones/asia-east1-b/machineTypes/g1-small",
		"f1-micro")

	assert.Equal(suite.T(),
		"https://www.googleapis.com/compute/v1/projects/livehouse-test/zones/asia-east1-b/machineTypes/f1-micro",
		result)
}
func (suite *GceManagerTestSuite) Test_ListInstanceGroupsByZone() {
	result := tested.ListInstanceGroupsByZone(projID, zone, func(string) bool { return true })
	assert.Equal(suite.T(), 2, len(result))
}

func (suite *GceManagerTestSuite) Test_InstanceGroupOperation() {
	// No instance in instance group
	ig, _ := tested.GetInstanceGroup(projID, zone, instanceGroupName)
	assert.Equal(suite.T(), int64(0), ig.Size)

	instances := []string{"zones/asia-east1-b/instances/instance-test"}
	_, err := tested.AddInstancesIntoInstanceGroup(projID, zone, instanceGroupName, instances)
	assert.Nil(suite.T(), err)
	time.Sleep(3 * time.Second)

	// One instance in instance group
	ig, _ = tested.GetInstanceGroup(projID, zone, instanceGroupName)
	assert.Equal(suite.T(), int64(1), ig.Size)
	result, _ := tested.ListInstancesInInstanceGroup(projID, zone, instanceGroupName)
	assert.Equal(suite.T(), vmName, result[0])

	_, err = tested.RemoveInstancesIntoInstanceGroup(projID, zone, instanceGroupName, instances)
	assert.Nil(suite.T(), err)
	time.Sleep(3 * time.Second)

	// No instance in instance group
	ig, _ = tested.GetInstanceGroup(projID, zone, instanceGroupName)
	assert.Equal(suite.T(), int64(0), ig.Size)
}

func (suite *GceManagerTestSuite) Test_TargetPoolOperation() {
	// No instance in target pool
	pool, _ := tested.GetTargetPool(projID, region, targetPoolName)
	assert.Equal(suite.T(), 0, len(pool.Instances))

	instances := []string{"zones/asia-east1-b/instances/instance-test"}
	_, err := tested.AddInstancesIntoTargetPool(projID, region, targetPoolName, instances)
	assert.Nil(suite.T(), err)
	time.Sleep(3 * time.Second)

	// One instance in target pool
	pool, _ = tested.GetTargetPool(projID, region, targetPoolName)
	assert.Equal(suite.T(), 1, len(pool.Instances))

	_, err = tested.RemoveInstancesFromTargetPool(projID, region, targetPoolName, instances)
	assert.Nil(suite.T(), err)
	time.Sleep(5 * time.Second)

	// No instance in target pool
	pool, _ = tested.GetTargetPool(projID, region, targetPoolName)
	assert.Equal(suite.T(), 0, len(pool.Instances))
}

func (suite *GceManagerTestSuite) Test_InstanceTemplateOperation() {
	tpl, err := tested.GetInstanceTemplate(projID, instanceTemplateName)
	assert.Nil(suite.T(), err)
	assert.Equal(suite.T(), instanceTemplateName, tpl.Name)

	tpl.Name = "new-instance-template-test"
	err = tested.NewInstanceTemplate(projID, tpl)
	assert.Nil(suite.T(), err)
	time.Sleep(3 * time.Second)

	{
		tpls, err := tested.ListInstanceTemplates(projID, "template")
		assert.Nil(suite.T(), err)
		assert.Equal(suite.T(), 2, len(tpls))
	}

	_, err = tested.DeleteInstanceTemplate(projID, "new-instance-template-test")
	assert.Nil(suite.T(), err)
	time.Sleep(3 * time.Second)

	{
		tpls, err := tested.ListInstanceTemplates(projID, "template")
		assert.Nil(suite.T(), err)
		assert.Equal(suite.T(), 1, len(tpls))
	}
}

func (suite *GceManagerTestSuite) Test_InstanceGroupManagerOperation() {
	{
		gm, err := tested.GetInstanceGroupManager(projID, zone, instanceGroupManagerName)
		assert.Nil(suite.T(), err)
		// Assert the original instance template is `instance-template-test`
		assert.True(suite.T(), strings.Contains(gm.InstanceTemplate, instanceTemplateName))
	}

	gms, err := tested.ListInstanceGroupManagers(projID, zone)
	assert.Nil(suite.T(), err)
	assert.Equal(suite.T(), 1, len(gms.Items))
	assert.Equal(suite.T(), instanceGroupManagerName, gms.Items[0].Name)
	if testing.Verbose() {
		result, _ := json.MarshalIndent(gms, "", "  ")
		log.Println(string(result))
	}

	// Create another instance template
	tpl, err := tested.GetInstanceTemplate(projID, instanceTemplateName)
	assert.Nil(suite.T(), err)
	assert.Equal(suite.T(), instanceTemplateName, tpl.Name)

	tpl.Name = "new-instance-template-test"
	err = tested.NewInstanceTemplate(projID, tpl)
	assert.Nil(suite.T(), err)
	time.Sleep(3 * time.Second)
	tpl, err = tested.GetInstanceTemplate(projID, "new-instance-template-test")

	// Set instance template to the instance group manager
	err = tested.SetInstanceTemplate(projID, zone, instanceGroupManagerName, tpl.SelfLink)
	assert.Nil(suite.T(), err)
	{
		gm, err := tested.GetInstanceGroupManager(projID, zone, instanceGroupManagerName)
		assert.Nil(suite.T(), err)
		// Assert the instance template has been changed to `new-instance-template-test`
		assert.True(suite.T(), strings.Contains(gm.InstanceTemplate, "new-instance-template-test"))
	}

	// Clean
	tpl, _ = tested.GetInstanceTemplate(projID, instanceTemplateName)
	err = tested.SetInstanceTemplate(projID, zone, instanceGroupManagerName, tpl.SelfLink)
	assert.Nil(suite.T(), err)

	_, err = tested.DeleteInstanceTemplate(projID, "new-instance-template-test")
	assert.Nil(suite.T(), err)
}

func (suite *GceManagerTestSuite) TearDownSuite() {
	log.Println("======== TearDown  ========")

	deleteTestVM()
	tested.DeleteDisk(projID, zone, diskName)
	tested.DeleteDisk(projID, zone, vmName)
}

func createTestVM() error {
	template, _ := ioutil.ReadAll(config.LoadAsset("/config/instance_template.json"))
	vm, _ := tested.InitVMFromTemplate(template, "asia-east1-b")
	return tested.NewVM(projID, zone, vm)
}

func createTestDisk() error {
	return tested.NewDisk(projID, zone, diskName, "global/snapshots/snapshot-test", 10)
}

func getTestVM() (*compute.Instance, error) {
	return tested.GetVM(projID, zone, vmName)
}

func getTestDisk() (*compute.Disk, error) {
	return tested.GetDisk(projID, zone, diskName)
}

func deleteTestVM() error {
	return tested.DeleteVM(projID, zone, vmName)
}
