.PHONY: asset deps install test

asset:
	rm -rf config/asset.go
	esc -o config/asset.go -pkg config config/

install: asset
	go install ./...

test: asset
	go test ./config
	go test ./gce
	go test ./gcm
	go test ./gds
	go test ./pubsub
	go test ./storage
	go test ./utility

deps:
	go get github.com/cihub/seelog
	go get github.com/facebookgo/inject
	go get github.com/mjibson/esc
	go get golang.org/x/net/context
	go get golang.org/x/oauth2
	go get golang.org/x/oauth2/google
	go get golang.org/x/oauth2/jwt
	go get google.golang.org/api/cloudmonitoring/v2beta2
	go get google.golang.org/api/monitoring/v3
	go get google.golang.org/api/compute/v1
	go get google.golang.org/api/pubsub/v1
	go get google.golang.org/api/replicapoolupdater/v1beta1
	go get google.golang.org/api/sqladmin/v1beta4
	go get google.golang.org/api/storage/v1
	go get google.golang.org/cloud
	go get google.golang.org/cloud/datastore
	# below is for test
	go get github.com/stretchr/testify/suite
	go get github.com/patrickmn/go-cache
	go get github.com/satori/go.uuid
