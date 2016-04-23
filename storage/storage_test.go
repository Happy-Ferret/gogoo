package storage

import (
	"io/ioutil"
	"log"
	"os"
	"testing"

	"github.com/facebookgo/inject"
	"github.com/iKala/gogoo/config"
	"github.com/stretchr/testify/suite"
)

var tested Manager
var testedProjectID string
var testedZone string

func TestStorageManagerTestSuite(t *testing.T) {
	suite.Run(t, new(StorageManagerTestSuite))
}

type StorageManagerTestSuite struct {
	suite.Suite
}

func (suite *StorageManagerTestSuite) SetupSuite() {
	gcloudConfig := config.LoadGcloudConfig(config.LoadAsset("/config/config.json"))
	key, _ := ioutil.ReadAll(config.LoadAsset("/config/key.pem"))

	// Construct dependency graph
	storageService, _ := BuildStorageService(gcloudConfig.ServiceAccount, key)

	var g inject.Graph
	err := g.Provide(
		&inject.Object{Value: storageService},
		&inject.Object{Value: &tested},
	)
	if err != nil {
		os.Exit(1)
	}
	if err := g.Populate(); err != nil {
		os.Exit(1)
	}
	// :~)

	tested.Setup()

	testedProjectID = gcloudConfig.ProjectID
	testedZone = "asia-east1-b"

	log.Println("======== SetupSuite  ========")
}

func (suite *StorageManagerTestSuite) Test01_ListBuckets() {
	tested.ListBuckets("livehouse-test")
}

func (suite *StorageManagerTestSuite) Test02_GetObject() {
	tested.GetObject("livehouse-test", "ts/vod_id/video.zip")
}

func (suite *StorageManagerTestSuite) Test03_ListObjectsUnderPath() {
	objects := tested.ListObjectsUnderPath("livehouse-test", "ts/vod_id")
	for _, obj := range objects {
		log.Printf("obj: name[%s], size[%d]", obj.Name, obj.Size)
	}
}

func (suite *StorageManagerTestSuite) Test04_ListFilesUnderPath() {
	files := tested.ListFilesUnderPath("livehouse-test", "ts/vod_id")
	for _, file := range files {
		log.Printf("obj: name[%s], size[%d]", file.Name, file.Size)
	}
}

func (suite *StorageManagerTestSuite) TearDownSuite() {
	log.Println("======== TearDown  ========")
}
