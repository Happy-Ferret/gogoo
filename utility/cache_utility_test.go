package utility

import (
	"testing"
	"time"

	"github.com/patrickmn/go-cache"
	"github.com/stretchr/testify/assert"
)

func TestCache(t *testing.T) {
	c := cache.New(5*time.Second, 1*time.Second)

	var fnWrite = func(value string) {
		c.Set("test-key", value, cache.DefaultExpiration)
	}
	var fnRead = func() string {
		value, found := c.Get("test-key")
		if found {
			return value.(string)
		}
		return ""
	}

	assert.Equal(t, "", fnRead())

	fnWrite("test-value")
	assert.Equal(t, "test-value", fnRead())

	// case overwrite
	fnWrite("test-value-2")
	assert.Equal(t, "test-value-2", fnRead())

	// case expiration
	time.Sleep(5 * time.Second)
	assert.Equal(t, "", fnRead())
}
