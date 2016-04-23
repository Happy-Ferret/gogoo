// Package replicapoolupdater provides APIs to communicate with Google autoscaling service
package replicapoolupdater

import (
	"fmt"

	log "github.com/cihub/seelog"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	"golang.org/x/oauth2/jwt"

	rpu "google.golang.org/api/replicapoolupdater/v1beta1"
)

var errRollingUpdate = fmt.Errorf("RollingUpdate Error")

// BuildRpuService builds the singlton service for RpuService
func BuildRpuService(serviceEmail string, key []byte) (*rpu.Service, error) {
	conf := &jwt.Config{
		Email:      serviceEmail,
		PrivateKey: key,
		Scopes: []string{
			rpu.ReplicapoolScope,
		},
		TokenURL: google.JWTTokenURL,
	}

	service, err := rpu.New(conf.Client(oauth2.NoContext))
	if err != nil {
		return nil, err
	}
	return service, nil
}

// RpuManager https://godoc.org/google.golang.org/api/replicapoolupdater/v1beta1
type RpuManager struct {
	Service *rpu.Service `inject:""`
}

// Insert starts rolling update for instances in instance group
//
// https://godoc.org/google.golang.org/api/replicapoolupdater/v1beta1#RollingUpdatesService.Insert
func (manager *RpuManager) Insert(projectID, zone string, rollingUpdate *rpu.RollingUpdate) (*rpu.Operation, error) {
	log.Tracef("Insert: projectID[%s], zone[%s]", projectID, zone)

	rollingUpdateService := rpu.NewRollingUpdatesService(manager.Service)

	op, err := rollingUpdateService.Insert(projectID, zone, rollingUpdate).Do()
	if err != nil {
		log.Warnf("Error: %s", err.Error())

		return nil, errRollingUpdate
	}

	return op, nil
}

// List lists recent rolling updates
//
// https://godoc.org/google.golang.org/api/replicapoolupdater/v1beta1#RollingUpdatesService.List
func (manager *RpuManager) List(projectID, zone string) (*rpu.RollingUpdateList, error) {
	log.Tracef("List: projectID[%s], zone[%s]", projectID, zone)

	rollingUpdateService := rpu.NewRollingUpdatesService(manager.Service)

	list, err := rollingUpdateService.List(projectID, zone).Do()
	if err != nil {
		log.Warnf("Error: %s", err.Error())

		return nil, errRollingUpdate
	}

	return list, nil
}

// Rollback rollbacks specified rolling update
//
// https://godoc.org/google.golang.org/api/replicapoolupdater/v1beta1#RollingUpdatesService.Rollback
func (manager *RpuManager) Rollback(projectID, zone, rollingUpdateID string) (*rpu.Operation, error) {
	log.Tracef("Rollback: projectID[%s], zone[%s], rollingUpdateID[%s]", projectID, zone, rollingUpdateID)

	rollingUpdateService := rpu.NewRollingUpdatesService(manager.Service)

	op, err := rollingUpdateService.Rollback(projectID, zone, rollingUpdateID).Do()
	if err != nil {
		log.Warnf("Error: %s", err.Error())

		return nil, errRollingUpdate
	}

	return op, nil
}
