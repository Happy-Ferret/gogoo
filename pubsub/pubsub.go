// Package pubsub communicates with pub/sub.
// https://godoc.org/google.golang.org/cloud/pubsub.
// https://godoc.org/google.golang.org/api/pubsub/v1.
package pubsub

import (
	"log"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	"golang.org/x/oauth2/jwt"
	pbsb "google.golang.org/api/pubsub/v1"
)

// BuildPbsbService builds the singlton service for Manager
func BuildPbsbService(serviceEmail string, key []byte) (*pbsb.Service, error) {
	conf := &jwt.Config{
		Email:      serviceEmail,
		PrivateKey: key,
		Scopes: []string{
			pbsb.CloudPlatformScope,
			pbsb.PubsubScope,
		},
		TokenURL: google.JWTTokenURL,
	}

	service, err := pbsb.New(conf.Client(oauth2.NoContext))
	if err != nil {
		return nil, err
	}
	return service, nil
}

// Manager communicates with google cloud pub/sub
type Manager struct {
	Service       *pbsb.Service `inject:""`
	topicsService *pbsb.ProjectsTopicsService
}

// Setup acts as init function
func (m *Manager) Setup() {
	m.topicsService = pbsb.NewProjectsTopicsService(m.Service)
}

// ListTopics lists all pub/sub topics under the project
func (m *Manager) ListTopics(projectID string) {
	response, err := m.topicsService.List(projectID).Do()
	if err != nil {
		log.Printf("err: %+v", err)
		return
	}

	log.Printf("topics: %+v", response.Topics[0].Name)
}
