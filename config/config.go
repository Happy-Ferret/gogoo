package config

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

// GcloudConfig is the binding object of config json file
type GcloudConfig struct {
	ServiceAccount string `json:"service_account"`
	ProjectID      string `json:"project_id"`
}

// LoadGcloudConfig loads config.json to GcloudConfig
func LoadGcloudConfig(file io.Reader) GcloudConfig {
	decoder := json.NewDecoder(file)
	configuration := GcloudConfig{}
	err := decoder.Decode(&configuration)
	if err != nil {
		fmt.Println("error:", err)
	}

	return configuration
}

// LoadAsset is wrapper function to read file from asset created by
// http://godoc.org/github.com/mjibson/esc
func LoadAsset(path string) http.File {
	asset := FS(false)
	file, _ := asset.Open(path)

	return file
}
