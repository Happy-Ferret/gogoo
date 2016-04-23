package utility

import (
	"bytes"
	log "github.com/cihub/seelog"
	"net"
)

// CheckIPInRange checks if `ip` is in the specified range
func CheckIPInRange(ip string, start net.IP, end net.IP) bool {
	//sanity check
	input := net.ParseIP(ip)
	if input.To4() == nil {
		log.Debugf("%v is not a valid IPv4 address", input)

		return false
	}

	if bytes.Compare(input, start) >= 0 && bytes.Compare(input, end) <= 0 {
		log.Tracef("%v is between %v and %v", input, start, end)
		return true
	}

	log.Tracef("%v is NOT between %v and %v", input, start, end)

	return false
}
