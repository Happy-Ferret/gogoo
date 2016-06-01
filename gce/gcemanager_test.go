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

	"github.com/facebookgo/inject"
	"github.com/iKala/gogoo/config"
	"github.com/iKala/gogoo/utility"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
	"google.golang.org/api/compute/v1"
)

var testedGceManager Manager
var testedProjectID string
var testedZone string

func TestGceManagerTestSuite(t *testing.T) {
	suite.Run(t, new(GceManagerTestSuite))
}

type GceManagerTestSuite struct {
	suite.Suite
}

func (suite *GceManagerTestSuite) SetupSuite() {
	gcloudConfig := config.LoadGcloudConfig(config.LoadAsset("/config/config.json"))
	key, _ := ioutil.ReadAll(config.LoadAsset("/config/key.pem"))

	// Construct dependency graph
	computeService, _ := BuildGceService(gcloudConfig.ServiceAccount, key)

	var g inject.Graph
	err := g.Provide(
		&inject.Object{Value: computeService},
		&inject.Object{Value: &testedGceManager},
	)
	if err != nil {
		os.Exit(1)
	}
	if err := g.Populate(); err != nil {
		os.Exit(1)
	}
	// :~)

	testedProjectID = gcloudConfig.ProjectID
	testedZone = "asia-east1-b"

	log.Println("======== SetupSuite  ========")
}

func (suite *GceManagerTestSuite) Test01_NewVM() {
	template, _ := ioutil.ReadAll(config.LoadAsset("/config/instance_template.json"))
	vm, _ := testedGceManager.InitVMFromTemplate(template, "asia-east1-b")

	testedGceManager.NewVM(testedProjectID, testedZone, vm)
}

func (suite *GceManagerTestSuite) Test02_GetVM() {
	newVM, _ := testedGceManager.GetVM(testedProjectID, testedZone, "instance-test")

	result, _ := json.MarshalIndent(newVM, "", "  ")
	log.Println(string(result))
}

func (suite *GceManagerTestSuite) Test03_GetNatIP() {
	instance, _ := testedGceManager.GetVM(testedProjectID, testedZone, "instance-test")
	natIP := testedGceManager.GetNatIP(instance)

	log.Printf("NatIP: %s", natIP)
}

func (suite *GceManagerTestSuite) Test04_AttachTags() {
	testedGceManager.AttachTags(testedProjectID, testedZone, "instance-test", []string{"rtc-8000"})

	time.Sleep(3 * time.Second)
	vm, _ := testedGceManager.GetVM(testedProjectID, testedZone, "instance-test")
	assert.True(suite.T(), utility.InStringSlice(vm.Tags.Items, "rtc-8000"))
}

func (suite *GceManagerTestSuite) Test05_ListVMs() {
	instanceList, _ := testedGceManager.ListVMs(testedProjectID, testedZone)

	// List all instances
	for _, v := range instanceList.Items {
		result, _ := json.MarshalIndent(v, "", "  ")
		log.Println(string(result))
	}

	assert.NotNil(suite.T(), instanceList)
}

func (suite *GceManagerTestSuite) Test051_ListVMsWithFilter() {
	instanceList, err := testedGceManager.ListVMsWithFilter(testedProjectID, testedZone, "name eq instance-.*")
	if err != nil {
		log.Println("err:%s", err)
	}

	// List all instances
	for _, v := range instanceList.Items {
		result, _ := json.MarshalIndent(v, "", "  ")
		log.Println(string(result))
	}

	assert.NotNil(suite.T(), instanceList)
}

func (suite *GceManagerTestSuite) Test06_ListImages() {
	imageList, _ := testedGceManager.ListImages(testedProjectID)

	for _, v := range imageList.Items {
		result, _ := json.MarshalIndent(v, "", "  ")
		log.Println(string(result))
	}

	assert.NotNil(suite.T(), imageList)
}

func (suite *GceManagerTestSuite) Test07_NewDisk() {
	testedGceManager.NewDisk(testedProjectID, testedZone, "disk-test", "global/snapshots/snapshot-test", 20)
}

func (suite *GceManagerTestSuite) Test08_GetDisk() {
	disk, _ := testedGceManager.GetDisk(testedProjectID, testedZone, "disk-test")

	result, _ := json.MarshalIndent(disk, "", "  ")
	log.Println(string(result))

	assert.NotNil(suite.T(), disk)

	assert.Equal(suite.T(), "snapshot-test", testedGceManager.GetSnapshotOfDisk(disk))
}

func (suite *GceManagerTestSuite) Test09_GetSnapshots() {
	snapshots, _ := testedGceManager.GetSnapshots(testedProjectID)

	assert.Equal(suite.T(), 1, len(snapshots))
	assert.Equal(suite.T(), "snapshot-test", snapshots[0].Name)
}

func (suite *GceManagerTestSuite) Test10_GetLatestSnapshot() {
	testedSnapshots := []*compute.Snapshot{
		&compute.Snapshot{
			Name: "zebra-rtc-alpha-snapshot-201503032021"},
		&compute.Snapshot{
			Name: "zebra-rtc-alpha-snapshot-201502161516"},
		&compute.Snapshot{
			Name: "zebra-rtc-alpha-snapshot-201502131103"},
	}

	latestSnapshot, _ := testedGceManager.GetLatestSnapshot("alpha", testedSnapshots)

	assert.Equal(suite.T(), "zebra-rtc-alpha-snapshot-201503032021", latestSnapshot.Name)
}

func (suite *GceManagerTestSuite) Test11_StopVM() {
	var stoppedChecker = func(projectID, zone, instanceName string) (bool, error) {
		instanceStoppedObserver := make(chan bool)
		go testedGceManager.ProbeVMStopped(projectID, zone, instanceName, instanceStoppedObserver)

		done := <-instanceStoppedObserver
		if !done {
			return false, fmt.Errorf("VM not stopped: instance[%s]", instanceName)
		}
		return true, nil
	}

	testedGceManager.StopVM(testedProjectID, testedZone, "instance-test", stoppedChecker)

	// SetMachineType
	testedGceManager.SetMachineType(testedProjectID, testedZone, "instance-test", "f1-micro")
}

func (suite *GceManagerTestSuite) Test12_StartVM() {

	var preparedChecker = func(projectID, zone, instanceName string) (bool, error) {
		instanceRunningObserver := make(chan bool)
		go testedGceManager.ProbeVMRunning(projectID, zone, instanceName, instanceRunningObserver)

		if done := <-instanceRunningObserver; !done {
			return false, fmt.Errorf("VM not running")
		}
		return true, nil
	}

	testedGceManager.StartVM(testedProjectID, testedZone, "instance-test", preparedChecker)

	vm, _ := testedGceManager.GetVM(testedProjectID, testedZone, "instance-test")
	assert.True(suite.T(), strings.Contains(vm.MachineType, "f1-micro"))
}

func (suite *GceManagerTestSuite) Test13_GetSnapshot() {
	snapshot, _ := testedGceManager.GetSnapshot(testedProjectID, "snapshot-test")

	log.Printf("snapshot: %+v", snapshot)
}

func (suite *GceManagerTestSuite) Test14_ListDisks() {
	disks, _ := testedGceManager.ListDisks(testedProjectID, testedZone)

	log.Printf("disk: %+v", disks.Items[0])
}

func (suite *GceManagerTestSuite) Test15_DetachTags() {
	testedGceManager.DetachTags(testedProjectID, testedZone, "instance-test", []string{"rtc-8000"})

	time.Sleep(3 * time.Second)
	vm, _ := testedGceManager.GetVM(testedProjectID, testedZone, "instance-test")
	assert.False(suite.T(), utility.InStringSlice(vm.Tags.Items, "rtc-8000"))
}

func (suite *GceManagerTestSuite) TearDownSuite() {
	log.Println("======== TearDown  ========")

	testedGceManager.DeleteDisk(testedProjectID, testedZone, "disk-test")
}
