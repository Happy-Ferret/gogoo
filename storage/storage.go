// Package storage communicates with google storage
package storage

import (
	"encoding/json"
	"log"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	"golang.org/x/oauth2/jwt"
	storage "google.golang.org/api/storage/v1"
)

// BuildStorageService builds the singlton service for Storage
func BuildStorageService(serviceEmail string, key []byte) (*storage.Service, error) {
	conf := &jwt.Config{
		Email:      serviceEmail,
		PrivateKey: key,
		Scopes: []string{
			storage.DevstorageFullControlScope,
			storage.DevstorageReadWriteScope,
		},
		TokenURL: google.JWTTokenURL,
	}

	service, err := storage.New(conf.Client(oauth2.NoContext))
	if err != nil {
		return nil, err
	}

	return service, nil
}

// Manager manages google storage service
type Manager struct {
	*storage.Service `inject:""`
	bucketsService   *storage.BucketsService
	objectsService   *storage.ObjectsService
}

// Setup acts as init function
func (s *Manager) Setup() {
	s.bucketsService = storage.NewBucketsService(s.Service)
	s.objectsService = storage.NewObjectsService(s.Service)
}

// GetObject gets the google storage object
func (s *Manager) GetObject(bucketName, objectName string) {
	log.Printf("GetObject: bucket[%s], object[%s]", bucketName, objectName)
	obj, err := s.objectsService.Get(bucketName, objectName).Do()

	if err != nil {
		log.Printf("err: %+v", err)
		return
	}

	b, _ := json.Marshal(obj)
	log.Println(string(b))
}

// ListObjectsUnderPath lists all objects under some path
func (s *Manager) ListObjectsUnderPath(bucketName, path string) []*storage.Object {
	log.Printf("ListObjectsUnderPath: bucket[%s], path[%s]", bucketName, path)

	result, err := s.objectsService.List(bucketName).
		Prefix(path).
		Do()
	if err != nil {
		log.Printf("err: %+v", err)
		return []*storage.Object{}
	}

	return result.Items
}

// ListFilesUnderPath lists all files under some path
func (s *Manager) ListFilesUnderPath(bucketName, path string) []*storage.Object {
	log.Printf("ListFilesUnderPath: bucket[%s], path[%s]", bucketName, path)

	files := []*storage.Object{}
	objects := s.ListObjectsUnderPath(bucketName, path)
	for _, obj := range objects {
		if obj.Size > 0 {
			files = append(files, obj)
		}
	}
	return files
}

// lListBuckets lists all buckets under some bucket
func (s *Manager) ListBuckets(projectID string) {
	buckets, err := s.bucketsService.List(projectID).Do()
	if err != nil {
		log.Printf("err: %+v", err)
		return
	}

	log.Printf("buckets: %+v", buckets.Items[0].Name)
}
