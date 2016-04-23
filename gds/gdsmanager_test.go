package gds

import (
	"io/ioutil"
	"log"
	"os"
	"testing"
	"time"

	"github.com/browny/gogoo/config"
	"github.com/facebookgo/inject"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
	"golang.org/x/net/context"
	"google.golang.org/cloud/datastore"
)

var tested Manager
var testedProjectID string
var testedZone string

const (
	TestKind = "TestKind"
)

type Article struct {
	Key         *datastore.Key `datastore:"-"`
	Title       string         `datastore:"title"`
	Number      int            `datastore:"number"`
	PublishedAt time.Time      `datastore:"publish_at"`
}

func TestGdsManagerTestSuite(t *testing.T) {
	suite.Run(t, new(GdsManagerTestSuite))
}

type GdsManagerTestSuite struct {
	suite.Suite
}

func (suite *GdsManagerTestSuite) SetupSuite() {
	gcloudConfig := config.LoadGcloudConfig(config.LoadAsset("/config/config.json"))
	key, _ := ioutil.ReadAll(config.LoadAsset("/config/key.pem"))

	// Construct dependency graph
	_, client, _ := BuildGdsContext(
		gcloudConfig.ServiceAccount,
		key,
		gcloudConfig.ProjectID)

	var g inject.Graph
	err := g.Provide(
		&inject.Object{Value: client},
		&inject.Object{Value: &tested},
	)
	if err != nil {
		log.Printf("err: %s", err.Error())
		os.Exit(1)
	}
	if err := g.Populate(); err != nil {
		os.Exit(1)
	}
	// :~)

	testedProjectID = gcloudConfig.ProjectID
	testedZone = "asia-east1-b"

	// Fill test fixture
	newKey := datastore.NewKey(context.Background(), TestKind, "instance-1", 0, nil)
	newEntity := &Article{
		Title:       "instance-1-title",
		Number:      10,
		PublishedAt: time.Now(),
	}
	tested.Put(newKey, newEntity)

	newKey = datastore.NewKey(context.Background(), TestKind, "instance-2", 0, nil)
	newEntity = &Article{
		Title:       "instance-2-title",
		Number:      10,
		PublishedAt: time.Now(),
	}
	tested.Put(newKey, newEntity)

	log.Println("======== SetupSuite  ========")
}

func (suite *GdsManagerTestSuite) Test01_Get() {
	newKey := datastore.NewKey(context.Background(), TestKind, "instance-1", 0, nil)
	entity := &Article{}
	tested.Get(newKey, entity)

	// Assert existed
	assert.NotNil(suite.T(), entity)

	newKey = datastore.NewKey(context.Background(), TestKind, "instance-not-existed", 0, nil)
	entity = &Article{}
	tested.Get(newKey, entity)

	// Assert not existed
	assert.Equal(suite.T(), Article{}, *entity)
}

func (suite *GdsManagerTestSuite) Test011_GetMulti() {
	key1 := datastore.NewKey(context.Background(), TestKind, "instance-1", 0, nil)
	key2 := datastore.NewKey(context.Background(), TestKind, "instance-2", 0, nil)
	keys := []*datastore.Key{key1, key2}

	result := make([]Article, len(keys))

	tested.GetMulti(keys, result)

	// Assert existed
	assert.Equal(suite.T(), "instance-1-title", result[0].Title)
	assert.Equal(suite.T(), "instance-2-title", result[1].Title)
}

func (suite *GdsManagerTestSuite) Test012_GetKeysOnly() {
	query := datastore.NewQuery(TestKind).Filter("number =", 10)

	keys, _ := tested.GetKeysOnly(query)

	// Assert existed
	assert.Equal(suite.T(), 2, len(keys))
	assert.Equal(suite.T(), "instance-1", keys[0].Name())
	assert.Equal(suite.T(), "instance-2", keys[1].Name())
}

func (suite *GdsManagerTestSuite) Test02_Put() {
	newKey := datastore.NewKey(context.Background(), TestKind, "instance-2", 0, nil)
	newEntity := &Article{
		Title:       "instance-2-title",
		Number:      7,
		PublishedAt: time.Now(),
	}

	tested.Put(newKey, newEntity)
}

func (suite *GdsManagerTestSuite) Test021_PutUnique() {
	newKey := datastore.NewKey(context.Background(), TestKind, "instance-1", 0, nil)
	newEntity := &Article{
		Title:       "instance-1-title",
		Number:      999,
		PublishedAt: time.Now(),
	}

	err := tested.PutUnique(newKey, newEntity)
	if err != nil {
		log.Println(err.Error())
	}
	assert.NotNil(suite.T(), err)
}

func (suite *GdsManagerTestSuite) Test03_GetAll() {
	query := datastore.NewQuery(TestKind).Filter("number >", 5)
	result := &[]*Article{}

	tested.GetAll(query, result)

	articles := *result
	assert.Equal(suite.T(), articles[0].Title, articles[0].Key.Name()+"-title")
	assert.Equal(suite.T(), 2, len(articles))
}

func (suite *GdsManagerTestSuite) Test05_GetCount() {
	query := datastore.NewQuery(TestKind).Filter("number >", 5)

	count, _ := tested.GetCount(query)
	assert.Equal(suite.T(), 2, count)
}

func (suite *GdsManagerTestSuite) Test06_Tx() {
	read := func() {
		tx := tested.GetTx()
		newKey := datastore.NewKey(context.Background(), TestKind, "instance-1", 0, nil)

		article := Article{}
		if err := tx.Get(newKey, &article); err != nil {
			log.Printf("Get Tx error: %s", err.Error())
		}
		log.Printf("read title: %s", article.Title)
	}

	readThenWrite := func() {
		tx := tested.GetTx()
		newKey := datastore.NewKey(context.Background(), TestKind, "instance-1", 0, nil)
		article := Article{
			Title: "hello",
		}
		if _, err := tx.Put(newKey, &article); err != nil {
			log.Printf("Fail Tx Put: %s", err.Error())
			return
		}

		if _, err := tx.Commit(); err != nil {
			log.Printf("Tx Commit error: %s", err.Error())

			return
		}

		entity := &Article{}
		tested.Get(newKey, entity)
		log.Printf("readThenWrite title: %s", entity.Title)
	}

	go read()
	go readThenWrite()
	go read()

	time.Sleep(5 * time.Second)
}

func (suite *GdsManagerTestSuite) Test07_Delete() {
	newKey := datastore.NewKey(context.Background(), TestKind, "instance-1", 0, nil)
	tested.Delete(newKey)

	entity := &Article{}
	err := tested.Get(newKey, entity)

	assert.NotNil(suite.T(), err)
}

func (suite *GdsManagerTestSuite) TearDownSuite() {
	log.Println("======== TearDown  ========")

	tested.DeleteAll(TestKind)
}
