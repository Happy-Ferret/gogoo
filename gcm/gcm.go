// Package gcm provides the basic APIs to communicate with Google cloud monitoring
package gcm

import (
	"fmt"
	"time"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	"golang.org/x/oauth2/jwt"
	monitor "google.golang.org/api/monitoring/v3"
)

// BuildCloudMonitorService builds the singlton service for CloudMonitor
func BuildCloudMonitorService(serviceEmail string, key []byte) (*monitor.Service, error) {
	conf := &jwt.Config{
		Email:      serviceEmail,
		PrivateKey: key,
		Scopes: []string{
			monitor.MonitoringScope,
			monitor.CloudPlatformScope,
		},
		TokenURL: google.JWTTokenURL,
	}

	service, err := monitor.New(conf.Client(oauth2.NoContext))
	if err != nil {
		return nil, err
	}

	return service, nil
}

// Manager is for low level communication with Google CloudMonitor.
type Manager struct {
	*monitor.Service `inject:""`
}

// GetAvgCPUUtilization gets average CPU utilization of recent 3 minutes
func (m *Manager) GetAvgCPUUtilization(projectID, instanceName string) (float64, error) {
	name := fmt.Sprintf("projects/%s", projectID)

	holder := "metric.type = %s AND metric.label.instance_name = %s"

	filter := fmt.Sprintf(holder,
		"\"compute.googleapis.com/instance/cpu/utilization\"",
		fmt.Sprintf("\"%s\"", instanceName))

	response, err := m.Service.Projects.TimeSeries.List(name).
		Filter(filter).
		IntervalStartTime(time.Now().Add(-3 * time.Minute).In(time.UTC).Format(time.RFC3339Nano)).
		IntervalEndTime(time.Now().In(time.UTC).Format(time.RFC3339Nano)).
		AggregationAlignmentPeriod("180s").
		AggregationPerSeriesAligner("ALIGN_MEAN").Do()

	if err != nil {
		return 0.0, err
	}

	return response.TimeSeries[0].Points[0].Value.DoubleValue, nil
}
