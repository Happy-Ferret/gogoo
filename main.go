// Package gogoo encapsulates google cloud api for more specific operation logic
package gogoo

import (
	"os"

	"github.com/iKala/gogoo/cloudsql"
	"github.com/iKala/gogoo/gce"
	"github.com/iKala/gogoo/gcm"
	"github.com/iKala/gogoo/gds"
	"github.com/iKala/gogoo/pubsub"
	"github.com/iKala/gogoo/replicapoolupdater"
	"github.com/iKala/gogoo/storage"

	"github.com/facebookgo/inject"
)

var gogoo GoGoo
var gceManager gce.Manager
var gdsManager gds.Manager
var gcmManager gcm.Manager
var cloudSQLManager cloudsql.Manager
var rpuManager replicapoolupdater.RpuManager
var pbsbManager pubsub.Manager
var storageManager storage.Manager

// AppContext as parameter object to initialize GoGoo
type AppContext struct {
	ServiceAccount      string
	KeyOfServiceAccount []byte
	ProjectID           string
}

// GoGoo acts as the handler to access different subpackages
type GoGoo struct {
	Gce                            *gce.Manager      `inject:""`
	Gds                            *gds.Manager      `inject:""`
	Monitor                        *gcm.Manager      `inject:""`
	CloudSQL                       *cloudsql.Manager `inject:""`
	*replicapoolupdater.RpuManager `inject:""`
	PubSub                         *pubsub.Manager  `inject:""`
	Storage                        *storage.Manager `inject:""`
}

// New creates a new GoGoo object.
func New(ctx AppContext) GoGoo {
	buildDependencyGraph(ctx)

	return gogoo
}

// Construct dependency graph
func buildDependencyGraph(ctx AppContext) {
	computeService, _ := gce.BuildGceService(ctx.ServiceAccount, ctx.KeyOfServiceAccount)
	_, client, _ := gds.BuildGdsContext(
		ctx.ServiceAccount,
		ctx.KeyOfServiceAccount,
		ctx.ProjectID)
	cloudmonitorService, _ := gcm.BuildCloudMonitorService(ctx.ServiceAccount, ctx.KeyOfServiceAccount)
	sqlService, _ := cloudsql.BuildCloudSQLService(ctx.ServiceAccount, ctx.KeyOfServiceAccount)
	rpuService, _ := replicapoolupdater.BuildRpuService(ctx.ServiceAccount, ctx.KeyOfServiceAccount)
	pbsbService, _ := pubsub.BuildPbsbService(ctx.ServiceAccount, ctx.KeyOfServiceAccount)
	storageService, _ := storage.BuildStorageService(ctx.ServiceAccount, ctx.KeyOfServiceAccount)

	var g inject.Graph
	err := g.Provide(
		&inject.Object{Value: client},
		&inject.Object{Value: computeService},
		&inject.Object{Value: cloudmonitorService},
		&inject.Object{Value: sqlService},
		&inject.Object{Value: rpuService},
		&inject.Object{Value: pbsbService},
		&inject.Object{Value: storageService},
		&inject.Object{Value: &gdsManager},
		&inject.Object{Value: &gceManager},
		&inject.Object{Value: &gcmManager},
		&inject.Object{Value: &cloudSQLManager},
		&inject.Object{Value: &rpuManager},
		&inject.Object{Value: &pbsbManager},
		&inject.Object{Value: &storageManager},
		&inject.Object{Value: &gogoo},
	)
	if err != nil {
		os.Exit(1)
	}
	if err := g.Populate(); err != nil {
		os.Exit(1)
	}
}
