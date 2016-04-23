// Package cloudsql provides the basic APIs to communicate with Google Cloud SQL
package cloudsql

import (
	"fmt"

	log "github.com/cihub/seelog"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	"golang.org/x/oauth2/jwt"
	sql "google.golang.org/api/sqladmin/v1beta4"
)

// BuildCloudSQLService builds the singlton service for CloudSQL
func BuildCloudSQLService(serviceEmail string, key []byte) (*sql.Service, error) {
	conf := &jwt.Config{
		Email:      serviceEmail,
		PrivateKey: key,
		Scopes: []string{
			sql.SqlserviceAdminScope,
		},
		TokenURL: google.JWTTokenURL,
	}
	service, err := sql.New(conf.Client(oauth2.NoContext))
	if err != nil {
		return nil, err
	}

	return service, nil
}

// Manager https://godoc.org/google.golang.org/api/sqladmin/v1beta4
type Manager struct {
	Service *sql.Service `inject:""`
}

// GetDatabase gets the database instance
func (m *Manager) GetDatabase(projectID, dbName string) (*sql.DatabaseInstance, error) {
	log.Tracef("GetDatabase: projectID[%s], db[%s]", projectID, dbName)

	dbInstanceService := sql.NewInstancesService(m.Service)
	if dbInstanceService == nil {
		return nil, fmt.Errorf("Fail NewInstancesService")
	}

	dbInstance, err := dbInstanceService.Get(projectID, dbName).Do()
	if err != nil {
		return nil, err
	}

	for _, an := range dbInstance.Settings.IpConfiguration.AuthorizedNetworks {
		log.Debugf("dbInstance: %+v", an)
	}
	return dbInstance, nil
}

// PatchAclEntriesOfDatabase updates the aclEntries settings of the database instance
func (m *Manager) PatchAclEntriesOfDatabase(projectID, dbName string, entries []*sql.AclEntry) (*sql.Operation, error) {
	log.Debugf("PatchAclEntriesOfDatabase: projectID[%s], db[%s]", projectID, dbName)

	dbInstance, err := m.GetDatabase(projectID, dbName)
	if err != nil {
		return nil, err
	}

	dbInstance.Settings.IpConfiguration.AuthorizedNetworks = entries

	dbInstanceService := sql.NewInstancesService(m.Service)
	if dbInstanceService == nil {
		return nil, fmt.Errorf("Fail NewInstancesService")
	}

	return dbInstanceService.Patch(projectID, dbName, dbInstance).Do()
}

// GetFilteredAclEntriesOfDatabase gets aclEntries which satisfies entry name filter
func (m *Manager) GetFilteredAclEntriesOfDatabase(
	projectID, dbName string, notContain func(string) bool) ([]*sql.AclEntry, error) {

	log.Debugf("GetFilteredAclEntriesOfDatabase: projectID[%s], db[%s]", projectID, dbName)

	dbInstance, err := m.GetDatabase(projectID, dbName)
	if err != nil {
		return nil, err
	}

	aclEntries := []*sql.AclEntry{}
	entries := dbInstance.Settings.IpConfiguration.AuthorizedNetworks
	for _, entry := range entries {
		if notContain(entry.Name) {
			aclEntries = append(aclEntries, entry)
		}
	}

	return aclEntries, nil
}
