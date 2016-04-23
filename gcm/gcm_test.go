package gcm

import (
	"flag"
	"io/ioutil"
	"log"
	"os"
	"testing"

	"github.com/browny/gogoo/config"
	"github.com/facebookgo/inject"
	"github.com/stretchr/testify/suite"
)

var tested Manager
var testedProjectID string
var testedZone string
var vmName = flag.String("vm", "", "")

func TestCloudMonitorTestSuite(t *testing.T) {
	suite.Run(t, new(CloudMonitorTestSuite))
}

type CloudMonitorTestSuite struct {
	suite.Suite
}

func (suite *CloudMonitorTestSuite) SetupSuite() {
	gcloudConfig := config.LoadGcloudConfig(config.LoadAsset("/config/config.json"))
	key, _ := ioutil.ReadAll(config.LoadAsset("/config/key.pem"))

	// Construct dependency graph
	cloudmonitorService, _ := BuildCloudMonitorService(gcloudConfig.ServiceAccount, key)

	var g inject.Graph
	err := g.Provide(
		&inject.Object{Value: cloudmonitorService},
		&inject.Object{Value: &tested},
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

// go test --vm=instance-1
func (suite *CloudMonitorTestSuite) Test01_GetAvgCpuUtilization() {
	if *vmName == "" {
		log.Printf("--vm flag not set")
		return
	}

	value, _ := tested.GetAvgCPUUtilization(testedProjectID, *vmName)
	log.Printf("value: %f", value)
}

func (suite *CloudMonitorTestSuite) TearDownSuite() {
	log.Println("======== TearDown  ========")
}
