package cloudsql

import (
	"io/ioutil"
	"log"
	"os"
	"strings"
	"testing"

	"github.com/browny/gogoo/config"
	"github.com/facebookgo/inject"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
	"google.golang.org/api/sqladmin/v1beta4"
)

var testedCloudSQLManager Manager
var testedProjectID string
var testedZone string

func TestCloudSQLManagerTestSuite(t *testing.T) {
	suite.Run(t, new(CloudSQLManagerTestSuite))
}

type CloudSQLManagerTestSuite struct {
	suite.Suite
}

func (suite *CloudSQLManagerTestSuite) SetupSuite() {
	gcloudConfig := config.LoadGcloudConfig(config.LoadAsset("/config/config.json"))
	key, _ := ioutil.ReadAll(config.LoadAsset("/config/key.pem"))

	// Construct dependency graph
	sqlService, _ := BuildCloudSQLService(
		gcloudConfig.ServiceAccount, key)

	var g inject.Graph
	err := g.Provide(
		&inject.Object{Value: sqlService},
		&inject.Object{Value: &testedCloudSQLManager},
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

func (suite *CloudSQLManagerTestSuite) Test01_GetDatabase() {
	dbInstance, _ := testedCloudSQLManager.GetDatabase(testedProjectID, "frontend-staging")
	assert.Equal(suite.T(), "frontend-staging", dbInstance.Name)
}

func (suite *CloudSQLManagerTestSuite) Test02_PatchAclEntriesOfDatabase() {
	entries := []*sqladmin.AclEntry{
		&sqladmin.AclEntry{
			Kind:  "sql#aclEntry",
			Name:  "test",
			Value: "1.1.1.2",
		}}

	if _, err := testedCloudSQLManager.PatchAclEntriesOfDatabase(testedProjectID, "test-database", entries); err != nil {
		log.Printf("err: %s", err.Error())
	}
}

func (suite *CloudSQLManagerTestSuite) Test03_GetFilteredAclEntriesOfDatabase() {
	isContain := func(checked string) bool {
		return strings.Contains(checked, "frontend")
	}

	aclEntries, _ := testedCloudSQLManager.GetFilteredAclEntriesOfDatabase(testedProjectID, "frontend", isContain)
	for _, entry := range aclEntries {
		log.Printf("entry: name[%s]", entry.Name)
	}
}
