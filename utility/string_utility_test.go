package utility

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGetLastSplit(t *testing.T) {
	src := "https://www.googleapis.com/compute/v1/projects/ikalacomputeenginetest/zones/asia-east1-c/machineTypes/n1-highcpu-4"

	assert.Equal(t, "n1-highcpu-4", GetLastSplit(src, "/"))
}
