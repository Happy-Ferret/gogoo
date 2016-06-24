package gds

import (
	"io/ioutil"
	"log"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/facebookgo/inject"
	"github.com/iKala/gogoo/config"
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

func (a *Article) Clone() interface{} {
	return *a
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

	testedProjectID = gcloudConfig.ProjectID
	testedZone = "asia-east1-b"

	// construct dependency graph
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
		log.Printf("err: %s", err)
		os.Exit(1)
	}
	if err := g.Populate(); err != nil {
		os.Exit(1)
	}
	// :~)

	// test fixture
	newKey := datastore.NewKey(context.Background(), TestKind, "instance-1", 0, nil)
	newEntity := &Article{
		Title:       "title-1",
		Number:      10,
		PublishedAt: time.Now(),
	}
	tested.Put(newKey, newEntity)

	newKey = datastore.NewKey(context.Background(), TestKind, "instance-2", 0, nil)
	newEntity = &Article{
		Title:       "title-2",
		Number:      10,
		PublishedAt: time.Now(),
	}
	tested.Put(newKey, newEntity)
	// :~)

	log.Println("======== SetupSuite  ========")
}

func (suite *GdsManagerTestSuite) Test_Get() {
	// existed
	newKey := datastore.NewKey(context.Background(), TestKind, "instance-1", 0, nil)
	entity := &Article{}
	tested.Get(newKey, entity)

	assert.NotNil(suite.T(), entity)

	// not existed
	newKey = datastore.NewKey(context.Background(), TestKind, "not-existed", 0, nil)
	entity = &Article{}
	err := tested.Get(newKey, entity)

	assert.NotNil(suite.T(), err)
	assert.Equal(suite.T(), Article{}, *entity)
}

func (suite *GdsManagerTestSuite) Test_GetMulti() {
	key1 := datastore.NewKey(context.Background(), TestKind, "instance-1", 0, nil)
	key2 := datastore.NewKey(context.Background(), TestKind, "instance-2", 0, nil)
	keys := []*datastore.Key{key1, key2}
	result := make([]Article, len(keys))

	err := tested.GetMulti(keys, result)

	// Assert existed
	assert.Nil(suite.T(), err)
	assert.Equal(suite.T(), "title-1", result[0].Title)
	assert.Equal(suite.T(), "title-2", result[1].Title)
}

func (suite *GdsManagerTestSuite) Test_GetKeysOnly() {
	query := datastore.NewQuery(TestKind).Filter("number =", 10)

	keys, _ := tested.GetKeysOnly(query)

	// Assert existed
	assert.Equal(suite.T(), 2, len(keys))
	assert.Equal(suite.T(), "instance-1", keys[0].Name())
	assert.Equal(suite.T(), "instance-2", keys[1].Name())
}

func (suite *GdsManagerTestSuite) Test_Put() {
	newKey := datastore.NewKey(context.Background(), TestKind, "instance-2", 0, nil)
	newEntity := &Article{
		Title:       "title-2",
		Number:      7,
		PublishedAt: time.Now(),
	}

	tested.Put(newKey, newEntity)

	// failure
	_, err := tested.Put(nil, newEntity)
	log.Printf("err: %+v", err)
	assert.NotNil(suite.T(), err)
}

func (suite *GdsManagerTestSuite) Test_PutUnique() {
	// unique fails
	newKey := datastore.NewKey(context.Background(), TestKind, "instance-1", 0, nil)
	newEntity := &Article{
		Title:       "title-1",
		Number:      999,
		PublishedAt: time.Now(),
	}

	err := tested.PutUnique(newKey, newEntity)
	if err != nil {
		log.Println(err)
	}
	assert.NotNil(suite.T(), err)

	// transaction fails
	newKey = datastore.NewKey(context.Background(), TestKind, "tx-fails", 0, nil)

	hasError := false
	var wg sync.WaitGroup
	pu := func() {
		defer wg.Done()
		err := tested.PutUnique(newKey, newEntity)
		if err != nil {
			hasError = true
		}
	}

	wg.Add(2)
	go pu()
	go pu()

	wg.Wait()
	assert.True(suite.T(), hasError)
}

func (suite *GdsManagerTestSuite) Test_GetAll() {
	query := datastore.NewQuery(TestKind).Filter("number >", 5)
	result := &[]*Article{}

	tested.GetAll(query, result)

	articles := *result
	assert.Equal(suite.T(), 2, len(articles))
}

func (suite *GdsManagerTestSuite) Test_GetCount() {
	query := datastore.NewQuery(TestKind).Filter("number >", 5)

	count, _ := tested.GetCount(query)
	assert.Equal(suite.T(), 2, count)
}

func (suite *GdsManagerTestSuite) Test_Iterate() {
	// change number to 20
	op := func(key *datastore.Key, dst interface{}) {
		a := dst.(Article)
		a.Number = 20
		if _, err := tested.Put(key, &a); err != nil {
			log.Printf("err:%s", err)
		}
	}

	query := datastore.NewQuery(TestKind)
	tested.Iterate(query, "", &Article{}, op)

	// asset all articles have been changed
	result := &[]*Article{}
	tested.GetAll(query, result)
	articles := *result
	assert.Equal(suite.T(), 2, len(articles))
	assert.Equal(suite.T(), 20, articles[0].Number)
	assert.Equal(suite.T(), 20, articles[1].Number)
}

func (suite *GdsManagerTestSuite) Test_BatchIterate() {
	op := func(key *datastore.Key, dst interface{}) {
		a := dst.(Article)
		log.Printf("a: %+v", a)
	}

	query := datastore.NewQuery(TestKind)
	tested.BatchIterate(query, 1, &Article{}, op)
}

func (suite *GdsManagerTestSuite) Test_Delete() {
	// prepare
	newKey := datastore.NewKey(context.Background(), TestKind, "instance-z", 0, nil)
	newEntity := &Article{
		Title:       "title-1",
		Number:      10,
		PublishedAt: time.Now(),
	}
	tested.Put(newKey, newEntity)

	entity := &Article{}
	err := tested.Get(newKey, entity)

	assert.Nil(suite.T(), err)

	// delete entity
	tested.Delete(newKey)

	err = tested.Get(newKey, entity)

	assert.NotNil(suite.T(), err)

	// clean
	tested.Delete(newKey)
}

func (suite *GdsManagerTestSuite) TearDownSuite() {
	log.Println("======== TearDown  ========")

	tested.DeleteAll(TestKind)
}
