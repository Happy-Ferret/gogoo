// Package gds provides the basic APIs to communicate with Google datastore
package gds

import (
	"reflect"
	"sync"

	log "github.com/cihub/seelog"
	"github.com/pkg/errors"
	"golang.org/x/net/context"
	"golang.org/x/oauth2/google"
	"golang.org/x/oauth2/jwt"
	"google.golang.org/cloud"
	"google.golang.org/cloud/datastore"
)

type Cloneable interface {
	Clone() interface{}
}

// Manager is for low level communication with Google datastore
type Manager struct {
	SuffixOfKind string

	Client *datastore.Client `inject:""`
}

// Setup sets the suffix of kind
func (m *Manager) Setup(suffixOfKind string) {
	m.SuffixOfKind = suffixOfKind
}

// BuildKey builds the datastore entity key of specific name
func (m *Manager) BuildKey(kind, keyName string) *datastore.Key {
	return datastore.NewKey(context.Background(), kind, keyName, 0, nil)
}

// Put inserts/updates the entity
func (m *Manager) Put(key *datastore.Key, entity interface{}) (*datastore.Key, error) {
	var resultKey *datastore.Key
	key, err := m.Client.Put(context.Background(), key, entity)
	if err != nil {
		return nil, err
	}
	resultKey = key

	// Use reflection to setup key of entity
	f := reflect.ValueOf(entity).Elem().FieldByName("Key")
	if f.IsValid() && f.CanSet() {
		f.Set(reflect.ValueOf(key))
	}

	return resultKey, nil
}

// PutUnique inserts/updates entity with unique key (if the same key existed, issue error)
func (m *Manager) PutUnique(key *datastore.Key, entity interface{}) error {
	log.Tracef("PutUnique entity: key[%s]", key.Name())

	tx := m.GetTx()

	if err := tx.Get(key, entity); err == nil {
		return errors.New("entity existed, unique condition violation!!")
	}

	_, err := tx.Put(key, entity)
	if err != nil {
		return errors.Wrap(err, "put fails")
	}

	if _, err := tx.Commit(); err != nil {
		return errors.Wrap(err, "commit fails")
	}

	return nil
}

// Get gets the entity by key
func (m *Manager) Get(key *datastore.Key, entity interface{}) error {
	log.Tracef("Get entity: key[%s]", key.Name())

	err := m.Client.Get(context.Background(), key, entity)
	if err != nil {
		return errors.Wrapf(err, "kind[%s], key[%s]", key.Kind(), key.Name())
	}

	// Use reflection to setup key of entity
	f := reflect.ValueOf(entity).Elem().FieldByName("Key")
	if f.IsValid() && f.CanSet() {
		f.Set(reflect.ValueOf(key))
	}

	return nil
}

// GetMulti gets the entities by keys
func (m *Manager) GetMulti(keys []*datastore.Key, dst interface{}) error {
	err := m.Client.GetMulti(context.Background(), keys, dst)
	if err != nil {
		return err
	}

	return nil
}

// GetKeysOnly gets only keys bu query
func (m *Manager) GetKeysOnly(query *datastore.Query) ([]*datastore.Key, error) {
	query = query.KeysOnly()

	type Any struct{}
	result := &[]Any{}
	keys, err := m.GetAll(query, result)
	if err != nil {
		return nil, err
	}

	return keys, nil
}

// Iterate iterates one by one to the query result
func (m *Manager) Iterate(
	query *datastore.Query,
	cursorStr string,
	dst Cloneable,
	op func(key *datastore.Key, dst interface{})) (string, error) {

	if cursorStr != "" {
		cursor, err := datastore.DecodeCursor(cursorStr)
		if err != nil {
			return "", errors.Errorf("Bad cursor %q: %v", cursorStr, err)
		}
		query = query.Start(cursor)
	}

	type Entity struct {
		Key *datastore.Key
		Dst interface{}
	}

	entityArr := []Entity{}

	it := m.Client.Run(context.Background(), query)
	key, err := it.Next(dst)
	for err == nil {
		entityArr = append(entityArr, Entity{key, dst.Clone()})
		key, err = it.Next(dst)
	}
	if err != datastore.Done {
		return "", errors.Errorf("Failed fetching results: %v", err)
	}

	var wg sync.WaitGroup
	for _, en := range entityArr {
		wg.Add(1)
		go func(key *datastore.Key, dst interface{}, wg *sync.WaitGroup) {
			if wg != nil {
				defer wg.Done()
			}

			op(key, dst)
		}(en.Key, en.Dst, &wg)
	}
	wg.Wait()

	nextCursor, err := it.Cursor()
	if err != nil {
		return "", errors.Errorf("Failed fetching cursor: %v", err)
	}

	return nextCursor.String(), nil
}

// Batch iterates the query result with customized step
func (m *Manager) BatchIterate(
	query *datastore.Query,
	batchSize int,
	dst Cloneable,
	op func(key *datastore.Key, dst interface{})) error {

	query = query.Limit(batchSize)
	cursor := ""
	count := 0
	for {
		nxt, err := m.Iterate(query, cursor, dst, op)
		if err != nil {
			return err
		}
		if cursor == nxt {
			break
		}
		count += batchSize
		log.Debugf("processed: count[%d]", count)
		cursor = nxt
	}

	return nil
}

// Delete deletes the entity by key (if the entity is not existed, there is no error)
func (m *Manager) Delete(key *datastore.Key) error {
	if key == nil {
		return errors.New("Key cann't be null")
	}

	err := m.Client.Delete(context.Background(), key)
	if err != nil {
		return err
	}

	return nil
}

// GetAll fetchs all entities by the query. The parameter `result` should be type of `*[]*<Entity>`
func (m *Manager) GetAll(query *datastore.Query, result interface{}) ([]*datastore.Key, error) {
	log.Trace("Get all by query")

	keys, err := m.Client.GetAll(context.Background(), query, result)
	if err != nil {
		return nil, err
	}

	// Use reflection to setup keys of entities
	s := reflect.ValueOf(result).Elem()
	for i := 0; i < s.Len(); i++ {
		f := s.Index(i).Elem().FieldByName("Key")
		if f.IsValid() && f.CanSet() {
			f.Set(reflect.ValueOf(keys[i]))
		}
	}

	return keys, nil
}

// GetCount return count of result
func (m *Manager) GetCount(query *datastore.Query) (int, error) {
	log.Trace("Get count by query")

	count, err := m.Client.Count(context.Background(), query)
	if err != nil {
		return 0, err
	}

	return count, nil
}

// DeleteAll deletes all entities under some Kind
func (m *Manager) DeleteAll(kindName string) error {
	log.Trace("Delete all")

	query := datastore.NewQuery(kindName).KeysOnly()

	type Any struct{}
	result := &[]Any{}

	keys, err := m.GetAll(query, result)
	if err != nil {
		return err
	}

	for _, key := range keys {
		err = m.Delete(key)
		if err != nil {
			log.Warnf("Delete fails: key[%s]", key.Name())
		}
	}

	return nil
}

// GetTx gets the datastore transaction
func (m *Manager) GetTx() *datastore.Transaction {
	tx, _ := m.Client.NewTransaction(context.Background())
	return tx
}

// BuildGdsContext builds the singlton context for Manager
func BuildGdsContext(serviceEmail string, key []byte, projectID string) (context.Context, *datastore.Client, error) {
	conf := &jwt.Config{
		Email:      serviceEmail,
		PrivateKey: key,
		Scopes: []string{
			datastore.ScopeDatastore,
		},
		TokenURL: google.JWTTokenURL,
	}

	ctx := context.Background()
	client, err := datastore.NewClient(ctx, projectID, cloud.WithTokenSource(conf.TokenSource(ctx)))
	if err != nil {
		return ctx, nil, err
	}

	return ctx, client, nil
}
