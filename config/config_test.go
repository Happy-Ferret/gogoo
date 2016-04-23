package config

import (
	"log"
	"testing"
)

func TestLoadGcloudConfig(t *testing.T) {
	config := LoadGcloudConfig(LoadAsset("/config/config.json"))

	log.Printf("config: %+v", config)
}
