// Package gce communicates with compute engine
package gce

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"text/template"
	"time"

	"github.com/iKala/gogoo/utility"

	log "github.com/cihub/seelog"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	"golang.org/x/oauth2/jwt"
	compute "google.golang.org/api/compute/v1"
)

func gceError(errMessage string) error {
	return fmt.Errorf("GCE operation fails: %s", errMessage)
}

const (
	// VMRunningTimeout is timeout of VM creation to running
	VMRunningTimeout = 180 * time.Second
	// VMStoppingTimeout is timeout of VM running to stopped
	VMStoppingTimeout = 180 * time.Second
	// DiskCreationTimeout is timeout of disk creation
	DiskCreationTimeout = 180 * time.Second
)

// VMConditionChecker return a bool value by checking condition inside VM
type VMConditionChecker func(projectID, zone, instanceName string) (bool, error)

// BuildGceService builds the singlton service for Manager
func BuildGceService(serviceEmail string, key []byte) (*compute.Service, error) {
	conf := &jwt.Config{
		Email:      serviceEmail,
		PrivateKey: key,
		Scopes: []string{
			compute.ComputeScope,
		},
		TokenURL: google.JWTTokenURL,
	}

	service, err := compute.New(conf.Client(oauth2.NoContext))
	if err != nil {
		return nil, err
	}

	return service, nil
}

// BySnapshotName is used to sort all gce snapshot by name
type BySnapshotName []*compute.Snapshot

func (a BySnapshotName) Len() int           { return len(a) }
func (a BySnapshotName) Swap(i, j int)      { a[i], a[j] = a[j], a[i] }
func (a BySnapshotName) Less(i, j int) bool { return a[i].Name < a[j].Name }

// Manager is for low level communication with Google Compute Engine.
type Manager struct {
	Service *compute.Service `inject:""`
}

// NewVM creates a new VM.
// This method block till the status of created VM is RUNNING
// or will be timeout if it takes over `VMRunningTimeout`.
// https://godoc.org/google.golang.org/api/compute/v1#InstancesService.Insert
func (m *Manager) NewVM(projectID, zone string, vm *compute.Instance) error {
	log.Tracef("New VM: project[%s], zone[%s]", projectID, zone)

	if _, err := m.Service.Instances.Insert(projectID, zone, vm).Do(); err != nil {
		return gceError(err.Error())
	}

	// Pooling the status of the created vm
	vmRunningObserver := make(chan bool)
	go m.ProbeVMRunning(projectID, zone, vm.Name, vmRunningObserver)

	done := <-vmRunningObserver
	if !done {
		return gceError(fmt.Sprintf("NewVM timeout: VM[%s]", vm.Name))
	}

	return nil
}

// GetVM gets a VM. If VM not existed, return nil.
// https://godoc.org/google.golang.org/api/compute/v1#InstancesService.Get
func (m *Manager) GetVM(projectID, zone, vmName string) (*compute.Instance, error) {
	log.Tracef("Get VM: project[%s], zone[%s], vmName[%s]", projectID, zone, vmName)

	vm, err := m.Service.Instances.Get(projectID, zone, vmName).Do()
	if err != nil {
		return nil, gceError(err.Error())
	}

	return vm, nil
}

// DeleteVM deletes a VM.
// https://godoc.org/google.golang.org/api/compute/v1#InstancesService.Delete
func (m *Manager) DeleteVM(projectID, zone, vmName string) error {
	log.Tracef("Delete VM: project[%s], zone[%s], vmName[%s]", projectID, zone, vmName)

	if _, err := m.Service.Instances.Delete(projectID, zone, vmName).Do(); err != nil {
		return gceError(err.Error())
	}

	return nil
}

// StartVM starts a VM.
// parameter `vcc` is the checker function to check if the VM is successfully started.
// https://godoc.org/google.golang.org/api/compute/v1#InstancesService.Start
func (m *Manager) StartVM(projectID, zone, vmName string, vcc VMConditionChecker) (*compute.Operation, error) {
	log.Tracef("Start instance: project[%s], zone[%s], vmName[%s]", projectID, zone, vmName)

	op, err := m.Service.Instances.Start(projectID, zone, vmName).Do()
	if err != nil {
		return nil, gceError(err.Error())
	}

	pass, err := vcc(projectID, zone, vmName)
	if pass {
		return op, nil
	}

	return nil, err
}

// StopVM stops a VM.
// parameter `vcc` is the checker function to check if the VM is successfully stopped.
// https://godoc.org/google.golang.org/api/compute/v1#InstancesService.Stop
func (m *Manager) StopVM(projectID, zone, vmName string, vcc VMConditionChecker) (*compute.Operation, error) {
	log.Tracef("Stop instance: project[%s], zone[%s], vmName[%s]", projectID, zone, vmName)

	op, err := m.Service.Instances.Stop(projectID, zone, vmName).Do()
	if err != nil {
		return nil, gceError(err.Error())
	}

	pass, err := vcc(projectID, zone, vmName)
	if pass {
		return op, nil
	}

	return nil, err
}

// SetMachineType changes the machine type for a stopped instance to the machine type specified in the request.
// https://godoc.org/google.golang.org/api/compute/v1#InstancesService.SetMachineType
func (m *Manager) SetMachineType(projectID, zone, vmName, machineType string) (*compute.Operation, error) {
	log.Debugf("SetMachineType: project[%s], zone[%s], vmName[%s], type[%s]",
		projectID, zone, vmName, machineType)

	instanceService := compute.NewInstancesService(m.Service)
	machineTypeURI := fmt.Sprintf("zones/%s/machineTypes/%s", zone, machineType)
	request := compute.InstancesSetMachineTypeRequest{MachineType: machineTypeURI}

	return instanceService.SetMachineType(projectID, zone, vmName, &request).Do()
}

// ResetInstance resets a instance.
// https://godoc.org/google.golang.org/api/compute/v1#InstancesService.Reset
func (m *Manager) ResetInstance(projectID, zone, vmName string) (*compute.Operation, error) {
	log.Debugf("Reset instance: project[%s], zone[%s], vmName[%s]", projectID, zone, vmName)

	return m.Service.Instances.Reset(projectID, zone, vmName).Do()
}

// ListVMs lists all VMs.
// https://godoc.org/google.golang.org/api/compute/v1#InstancesService.List
func (m *Manager) ListVMs(projectID, zone string) (*compute.InstanceList, error) {
	log.Tracef("List VMs: project[%s], zone[%s]", projectID, zone)

	res, err := m.Service.Instances.List(projectID, zone).Do()
	if err != nil {
		return nil, gceError(err.Error())
	}

	return res, nil
}

// ListImages lists all images.
// https://godoc.org/google.golang.org/api/compute/v1#ImagesService.List
func (m *Manager) ListImages(projectID string) (*compute.ImageList, error) {
	log.Tracef("List images: project[%s]", projectID)

	res, err := m.Service.Images.List(projectID).Do()
	if err != nil {
		return nil, gceError(err.Error())
	}

	return res, nil
}

// ListDisks lists all disks.
// https://godoc.org/google.golang.org/api/compute/v1#DisksService.List
func (m *Manager) ListDisks(projectID, zone string) (*compute.DiskList, error) {
	log.Tracef("List disks: project[%s], zone[%s]", projectID, zone)

	diskService := compute.NewDisksService(m.Service)

	res, err := diskService.List(projectID, zone).Do()
	if err != nil {
		return nil, gceError(err.Error())
	}

	return res, nil
}

// NewDisk creates a new disk by specified snapshot.
// https://godoc.org/google.golang.org/api/compute/v1#DisksService.Insert
func (m *Manager) NewDisk(projectID, zone, name, sourceSnapshot string, sizeGb int64) error {
	log.Tracef("New disk: project[%s], zone[%s], name[%s], sourceSnapshot[%s]",
		projectID, zone, name, sourceSnapshot)

	diskService := compute.NewDisksService(m.Service)

	disk := &compute.Disk{
		Name:           name,
		SizeGb:         sizeGb,
		SourceSnapshot: sourceSnapshot}

	if _, err := diskService.Insert(projectID, zone, disk).Do(); err != nil {
		return gceError(err.Error())
	}

	diskCreationObserver := make(chan bool)
	go m.ProbeDiskCreation(projectID, zone, name, diskCreationObserver)

	done := <-diskCreationObserver
	if !done {
		return fmt.Errorf("NewDisk timeout: disk[%s]", name)
	}

	return nil
}

// GetDisk gets disk.
// https://godoc.org/google.golang.org/api/compute/v1#DisksService.Get
func (m *Manager) GetDisk(projectID, zone, diskName string) (*compute.Disk, error) {
	log.Tracef("Get disk: project[%s], zone[%s], diskName[%s]", projectID, zone, diskName)

	diskService := compute.NewDisksService(m.Service)

	disk, err := diskService.Get(projectID, zone, diskName).Do()
	if err != nil {
		return nil, gceError(err.Error())
	}

	return disk, nil
}

// DeleteDisk deletes disk.
// https://godoc.org/google.golang.org/api/compute/v1#DisksService.Delete
func (m *Manager) DeleteDisk(projectID, zone, diskName string) error {
	log.Tracef("Delete disk: project[%s], zone[%s], diskName[%s]", projectID, zone, diskName)

	diskService := compute.NewDisksService(m.Service)

	if _, err := diskService.Delete(projectID, zone, diskName).Do(); err != nil {
		return gceError(err.Error())
	}

	return nil
}

// GetSnapshots gets all snapshots of the project.
// https://godoc.org/google.golang.org/api/compute/v1#SnapshotsService.List
func (m *Manager) GetSnapshots(projectID string) ([]*compute.Snapshot, error) {
	log.Tracef("Get snapshots: project[%s]", projectID)

	snapshotService := compute.NewSnapshotsService(m.Service)
	result, err := snapshotService.List(projectID).Do()
	if err != nil {
		return nil, gceError(err.Error())
	}

	snapshots := result.Items
	for _, snapshot := range snapshots {
		log.Tracef("snapshot: id[%d], name[%s]", snapshot.Id, snapshot.Name)
	}

	return snapshots, nil
}

// GetSnapshot gets the specific snapshot
// https://godoc.org/google.golang.org/api/compute/v1#SnapshotsService.Get
func (m *Manager) GetSnapshot(projectID, snapshot string) (*compute.Snapshot, error) {
	log.Tracef("Get snapshot: project[%s], snapshot[%s]", projectID, snapshot)

	snapshotService := compute.NewSnapshotsService(m.Service)

	result, err := snapshotService.Get(projectID, snapshot).Do()
	if err != nil {
		return nil, gceError(err.Error())
	}

	return result, nil
}

// adjustTags adjusts tags of VM.
// https://godoc.org/google.golang.org/api/compute/v1#InstancesService.SetTags
func (m *Manager) adjustTags(
	projectID, zone, vmName string, tags []string, newTagsGenerator func([]string, []string) []string) (
	*compute.Operation, error) {

	vm, err := m.GetVM(projectID, zone, vmName)
	if err != nil {
		return nil, err
	}

	vm.Tags.Items = newTagsGenerator(vm.Tags.Items, tags)

	op, err := m.Service.Instances.SetTags(projectID, zone, vmName, vm.Tags).Do()
	if err != nil {
		return nil, gceError(err.Error())
	}

	return op, nil
}

// AttachTags attaches tags onto VM.
func (m *Manager) AttachTags(projectID, zone, vmName string, addedTags []string) (*compute.Operation, error) {
	log.Tracef("AttachTags: vm[%s], addedTags[%s]", vmName, addedTags)

	attacher := func(src, new []string) []string {
		for _, n := range new {
			src = append(src, n)
		}
		return src
	}

	return m.adjustTags(projectID, zone, vmName, addedTags, attacher)
}

// DetachTags detaches tags from VM.
func (m *Manager) DetachTags(projectID, zone, vmName string, removedTages []string) (*compute.Operation, error) {
	log.Tracef("DetachTags: vm[%s], removedTages[%s]", vmName, removedTages)

	detacher := func(src, remove []string) []string {
		result := []string{}
		for _, s := range src {
			if utility.InStringSlice(remove, s) {
				continue
			}
			result = append(result, s)
		}
		return result
	}

	return m.adjustTags(projectID, zone, vmName, removedTages, detacher)
}

// AddInstancesIntoInstanceGroup adds instances into some instance group
// https://godoc.org/google.golang.org/api/compute/v1#InstanceGroupsService.AddInstances
func (m *Manager) AddInstancesIntoInstanceGroup(
	projectID, zone, instanceGroupName string, instances []string) (
	*compute.Operation, error) {

	log.Tracef(
		"AddInstancesIntoInstanceGroup: project[%s], region[%s], instanceGroupName[%s], instances[%s]",
		projectID, zone, instanceGroupName, instances)

	instanceGroupService := compute.NewInstanceGroupsService(m.Service)

	instanceReferences := []*compute.InstanceReference{}
	for _, instance := range instances {
		instanceRef := compute.InstanceReference{Instance: instance}
		instanceReferences = append(instanceReferences, &instanceRef)
	}
	request := compute.InstanceGroupsAddInstancesRequest{Instances: instanceReferences}

	op, err := instanceGroupService.AddInstances(projectID, zone, instanceGroupName, &request).Do()
	if err != nil {
		return nil, gceError(err.Error())
	}

	return op, nil
}

// GetLatestSnapshot gets latest snapshot with specified prefix in its name
func (m *Manager) GetLatestSnapshot(prefix string, snapshots []*compute.Snapshot) (*compute.Snapshot, error) {
	filteredSnapshots := []*compute.Snapshot{}
	for _, snapshot := range snapshots {
		if strings.Contains(snapshot.Name, prefix) {
			filteredSnapshots = append(filteredSnapshots, snapshot)
		}
	}

	sort.Sort(BySnapshotName(filteredSnapshots))
	if len(filteredSnapshots) < 1 {
		log.Warn("No snapshot found")
		return nil, fmt.Errorf("No snapshot found")
	}

	result := filteredSnapshots[len(filteredSnapshots)-1]
	log.Tracef("Latest snapshot found: name[%s]", result.Name)

	return result, nil
}

// InitVMFromTemplate builds the sample VM from template
func (m *Manager) InitVMFromTemplate(templateFile []byte, zone string) (*compute.Instance, error) {
	type TemplateParameter struct {
		Zone string
	}
	var tp = TemplateParameter{Zone: zone}

	tmpl, _ := template.New("test").Parse(string(templateFile[:]))
	var b bytes.Buffer
	tmpl.Execute(&b, tp)

	var vm compute.Instance
	err := json.Unmarshal(b.Bytes(), &vm)
	if err != nil {
		return nil, err
	}
	// :~)

	return &vm, nil
}

// ProbeVMRunning probes the VM status till its status is RUNNING or timeout
func (m *Manager) ProbeVMRunning(projectID, zone, vmName string, observer chan<- bool) {
	startTime := time.Now()

	for {
		if time.Now().Sub(startTime) > VMRunningTimeout {
			log.Warnf("VM creation Timeout: VM[%s]", vmName)
			observer <- false

			break
		}

		createdInstance, err := m.GetVM(projectID, zone, vmName)

		if err != nil {
			log.Tracef("VM not yet Existed: VM[%s]", vmName)
			time.Sleep(10 * time.Second)

			continue
		}

		if createdInstance.Status != "RUNNING" {
			log.Tracef("VM not yet Running: VM[%s]", vmName)
			time.Sleep(10 * time.Second)

			continue
		}

		log.Infof("VM Running!: VM[%s]", vmName)
		observer <- true

		break
	}
}

// ProbeVMStopped probes the instance status till its status is Stopping or timeout
func (m *Manager) ProbeVMStopped(projectID, zone, vmName string, observer chan<- bool) {
	startTime := time.Now()

	for {
		if time.Now().Sub(startTime) > VMStoppingTimeout {
			log.Warnf("VM stop Timeout: VM[%s]", vmName)
			observer <- false

			break
		}

		createdInstance, err := m.GetVM(projectID, zone, vmName)

		if err != nil {
			log.Tracef("VM not yet Existed: VM[%s]", vmName)
			time.Sleep(10 * time.Second)

			continue
		}

		if createdInstance.Status != "TERMINATED" {
			log.Tracef("VM not yet Stopped: VM[%s]", vmName)
			time.Sleep(10 * time.Second)

			continue
		}

		log.Infof("VM Stopped!: VM[%s]", vmName)
		observer <- true

		break
	}
}

// ProbeDiskCreation probes the disk status till its status is READY or timeout
func (m *Manager) ProbeDiskCreation(projectID, zone, diskName string, observer chan<- bool) {
	startTime := time.Now()

	for {
		if time.Now().Sub(startTime) > DiskCreationTimeout {
			log.Warnf("Disk creation Timeout: disk[%s]", diskName)
			observer <- false

			break
		}

		disk, err := m.GetDisk(projectID, zone, diskName)
		if err != nil {
			log.Tracef("Disk not yet Created: name[%s]", diskName)
			time.Sleep(10 * time.Second)

			continue
		}
		if disk.Status != "READY" {
			log.Tracef("Disk not yet Ready: name[%s]", disk.Name)
			time.Sleep(5 * time.Second)

			continue
		}

		log.Infof("Disk Created!: name[%s]", disk.Name)
		observer <- true

		break
	}
}

// GetNatIP gets NAT IP address from VM
func (m *Manager) GetNatIP(vm *compute.Instance) string {
	if vm == nil {
		return "missing"
	}
	natIP := vm.NetworkInterfaces[0].AccessConfigs[0].NatIP

	log.Tracef("Got NatIP: VM[%s], ip[%s]", vm.Name, natIP)

	return natIP
}

// GetNetworkIP gets internal IP address from VM
func (m *Manager) GetNetworkIP(vm *compute.Instance) string {
	if vm == nil {
		return "missing"
	}
	networkIP := vm.NetworkInterfaces[0].NetworkIP

	log.Tracef("Got NetworkIP: VM[%s], ip[%s]", vm.Name, networkIP)

	return networkIP
}

// GetSnapshotOfDisk gets the snapshot name of the disk
func (m *Manager) GetSnapshotOfDisk(disk *compute.Disk) string {
	log.Tracef("Snapshot of the disk: snapshot[%s]", disk.SourceSnapshot)

	arr := strings.Split(disk.SourceSnapshot, "/")

	return arr[len(arr)-1]
}

// PatchInstanceMachineType replaces the last part of machineTypeURI with targetType
func (m *Manager) PatchInstanceMachineType(machineTypeURI, targetType string) string {
	split := strings.Split(machineTypeURI, "/")
	split[len(split)-1] = targetType
	return strings.Join(split, "/")
}
