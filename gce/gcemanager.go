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

	"github.com/iKala/gosak"

	log "github.com/cihub/seelog"
	"github.com/pkg/errors"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	"golang.org/x/oauth2/jwt"
	"google.golang.org/api/compute/v1"
)

const (
	// VMCreationTimeout is timeout of VM creation to running
	VMCreationTimeout = 180 * time.Second
	// VMStoppingTimeout is timeout of VM running to stopped
	VMStoppingTimeout = 180 * time.Second
	// DiskCreationTimeout is timeout of disk creation
	DiskCreationTimeout = 180 * time.Second
	// VMSetMachineTypeTimeout is timeout of set machine type
	VMSetMachineTypeTimeout = 180 * time.Second
	// InstanceTemplateCreationTimeout is timeout of creating instance template
	InstanceTemplateCreationTimeout = 180 * time.Second

	// VMStatusTerminated ...
	VMStatusTerminated = "TERMINATED"
	// VMStatusRunning ...
	VMStatusRunning = "RUNNING"
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
// This method blocks till the status of created VM to be RUNNING
// or will be timeout if it takes over `VMCreationTimeout`.
// https://godoc.org/google.golang.org/api/compute/v1#InstancesService.Insert
func (m *Manager) NewVM(projectID, zone string, vm *compute.Instance) error {
	log.Tracef("New VM: project[%s], zone[%s]", projectID, zone)

	if _, err := m.Service.Instances.Insert(projectID, zone, vm).Do(); err != nil {
		return err
	}

	// Pooling the status of the created vm
	vmRunningObserver := make(chan bool)
	go m.ProbeVMRunning(projectID, zone, vm.Name, vmRunningObserver)

	done := <-vmRunningObserver
	if !done {
		return errors.Errorf("NewVM timeout: VM[%s]", vm.Name)
	}

	return nil
}

// GetVM gets a VM. If VM not existed, return nil.
// https://godoc.org/google.golang.org/api/compute/v1#InstancesService.Get
func (m *Manager) GetVM(projectID, zone, vmName string) (*compute.Instance, error) {
	log.Tracef("Get VM: project[%s], zone[%s], vmName[%s]", projectID, zone, vmName)

	vm, err := m.Service.Instances.Get(projectID, zone, vmName).Do()
	if err != nil {
		return nil, err
	}

	return vm, nil
}

// DeleteVM deletes a VM.
// https://godoc.org/google.golang.org/api/compute/v1#InstancesService.Delete
func (m *Manager) DeleteVM(projectID, zone, vmName string) error {
	log.Debugf("Delete VM: project[%s], zone[%s], vmName[%s]", projectID, zone, vmName)

	if _, err := m.Service.Instances.Delete(projectID, zone, vmName).Do(); err != nil {
		return err
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
		return nil, err
	}

	if _, err := vcc(projectID, zone, vmName); err != nil {
		return nil, errors.Wrap(err, "VM condition checking fails")
	}

	return op, nil
}

// StopVM stops a VM.
// parameter `vcc` is the checker function to check if the VM is successfully stopped.
// https://godoc.org/google.golang.org/api/compute/v1#InstancesService.Stop
func (m *Manager) StopVM(projectID, zone, vmName string, vcc VMConditionChecker) (*compute.Operation, error) {
	log.Tracef("Stop instance: project[%s], zone[%s], vmName[%s]", projectID, zone, vmName)

	op, err := m.Service.Instances.Stop(projectID, zone, vmName).Do()
	if err != nil {
		return nil, err
	}

	if _, err := vcc(projectID, zone, vmName); err != nil {
		return nil, errors.Wrap(err, "VM condition checking fails")
	}

	return op, nil
}

// SetMachineType changes the machine type for a stopped instance to the machine type specified in the request.
// https://godoc.org/google.golang.org/api/compute/v1#InstancesService.SetMachineType
func (m *Manager) SetMachineType(projectID, zone, vmName, machineType string) error {
	log.Debugf("SetMachineType: project[%s], zone[%s], vmName[%s], type[%s]",
		projectID, zone, vmName, machineType)

	instanceService := compute.NewInstancesService(m.Service)
	machineTypeURI := fmt.Sprintf("zones/%s/machineTypes/%s", zone, machineType)
	request := compute.InstancesSetMachineTypeRequest{MachineType: machineTypeURI}

	if _, err := instanceService.SetMachineType(projectID, zone, vmName, &request).Do(); err != nil {
		return err
	}

	vmMachineTypeChangingObserver := make(chan bool)
	go m.ProbeVMMachineTypeChanged(projectID, zone, vmName, machineType, vmMachineTypeChangingObserver)

	done := <-vmMachineTypeChangingObserver
	if !done {
		return errors.Errorf("SetMachineType timeout: VM[%s]", vmName)
	}

	return nil
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
		return nil, err
	}

	return res, nil
}

// ListVMsWithFilter lists VMs with filter.
// https://godoc.org/google.golang.org/api/compute/v1#InstancesService.List
func (m *Manager) ListVMsWithFilter(projectID, zone, filter string) (*compute.InstanceList, error) {
	log.Tracef("List VMs with filter: project[%s], zone[%s], filter[%s]", projectID, zone, filter)

	res, err := m.Service.Instances.List(projectID, zone).
		Filter(filter).
		Do()

	if err != nil {
		return nil, err
	}

	return res, nil
}

// ListImages lists all images.
// https://godoc.org/google.golang.org/api/compute/v1#ImagesService.List
func (m *Manager) ListImages(projectID string) (*compute.ImageList, error) {
	log.Tracef("List images: project[%s]", projectID)

	res, err := m.Service.Images.List(projectID).Do()
	if err != nil {
		return nil, err
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
		return nil, err
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
		return err
	}

	diskCreationObserver := make(chan bool)
	go m.ProbeDiskCreation(projectID, zone, name, diskCreationObserver)

	done := <-diskCreationObserver
	if !done {
		return errors.Errorf("NewDisk timeout: disk[%s]", name)
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
		return nil, err
	}

	return disk, nil
}

// DeleteDisk deletes disk.
// https://godoc.org/google.golang.org/api/compute/v1#DisksService.Delete
func (m *Manager) DeleteDisk(projectID, zone, diskName string) error {
	log.Tracef("Delete disk: project[%s], zone[%s], diskName[%s]", projectID, zone, diskName)

	diskService := compute.NewDisksService(m.Service)

	if _, err := diskService.Delete(projectID, zone, diskName).Do(); err != nil {
		return err
	}

	return nil
}

// ListSnapshots gets all snapshots of the project.
// https://godoc.org/google.golang.org/api/compute/v1#SnapshotsService.List
func (m *Manager) ListSnapshots(projectID string) ([]*compute.Snapshot, error) {
	log.Tracef("Get snapshots: project[%s]", projectID)

	snapshotService := compute.NewSnapshotsService(m.Service)
	result, err := snapshotService.List(projectID).Do()
	if err != nil {
		return nil, err
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
		return nil, err
	}

	return result, nil
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

// GetSnapshotOfDisk gets the snapshot name of the disk
func (m *Manager) GetSnapshotOfDisk(disk *compute.Disk) string {
	log.Tracef("Snapshot of the disk: snapshot[%s]", disk.SourceSnapshot)

	arr := strings.Split(disk.SourceSnapshot, "/")

	return arr[len(arr)-1]
}

// setTags adjusts tags of VM.
// https://godoc.org/google.golang.org/api/compute/v1#InstancesService.SetTags
func (m *Manager) setTags(
	projectID, zone, vmName string, tags []string, newTagsGenerator func([]string, []string) []string) (
	*compute.Operation, error) {

	vm, err := m.GetVM(projectID, zone, vmName)
	if err != nil {
		return nil, err
	}

	vm.Tags.Items = newTagsGenerator(vm.Tags.Items, tags)

	op, err := m.Service.Instances.SetTags(projectID, zone, vmName, vm.Tags).Do()
	if err != nil {
		return nil, err
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

	return m.setTags(projectID, zone, vmName, addedTags, attacher)
}

// DetachTags detaches tags from VM.
func (m *Manager) DetachTags(projectID, zone, vmName string, removedTages []string) (*compute.Operation, error) {
	log.Tracef("DetachTags: vm[%s], removedTages[%s]", vmName, removedTages)

	detacher := func(src, remove []string) []string {
		result := []string{}
		for _, s := range src {
			if gosak.InStringSlice(remove, s) {
				continue
			}
			result = append(result, s)
		}
		return result
	}

	return m.setTags(projectID, zone, vmName, removedTages, detacher)
}

// GetInstanceGroup - https://godoc.org/google.golang.org/api/compute/v1#InstanceGroupsService.Get
func (m *Manager) GetInstanceGroup(projectID, zone, instanceGroupName string) (
	*compute.InstanceGroup, error) {

	log.Tracef(
		"GetInstanceGroup: project[%s], zone[%s], instanceGroupName[%s]",
		projectID, zone, instanceGroupName)

	srv := compute.NewInstanceGroupsService(m.Service)

	return srv.Get(projectID, zone, instanceGroupName).Do()
}

// ListInstancesInInstanceGroup lists all instances under some instance group
// https://godoc.org/google.golang.org/api/compute/v1#InstanceGroupsService.ListInstances
func (m *Manager) ListInstancesInInstanceGroup(projectID, zone, instanceGroupName string) ([]string, error) {
	log.Tracef(
		"ListInstancesInInstanceGroup: project[%s], zone[%s], instanceGroupName[%s]",
		projectID, zone, instanceGroupName)

	srv := compute.NewInstanceGroupsService(m.Service)
	result, err := srv.ListInstances(projectID, zone, instanceGroupName, nil).Do()
	if err != nil {
		return []string{}, err
	}

	instances := []string{}
	for _, item := range result.Items {
		arr := strings.Split(item.Instance, "/")
		instances = append(instances, arr[len(arr)-1])
	}

	return instances, nil
}

// AddInstancesIntoInstanceGroup adds instances into some instance group
// https://godoc.org/google.golang.org/api/compute/v1#InstanceGroupsService.AddInstances
func (m *Manager) AddInstancesIntoInstanceGroup(
	projectID, zone, instanceGroupName string, instances []string) (
	*compute.Operation, error) {

	log.Tracef(
		"AddInstancesIntoInstanceGroup: project[%s], zone[%s], instanceGroupName[%s], instances[%s]",
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
		return nil, err
	}

	return op, nil
}

// RemoveInstancesIntoInstanceGroup adds instances into some instance group
// https://godoc.org/google.golang.org/api/compute/v1#InstanceGroupsService.RemoveInstances
func (m *Manager) RemoveInstancesIntoInstanceGroup(
	projectID, zone, instanceGroupName string, instances []string) (
	*compute.Operation, error) {

	log.Tracef(
		"RemoveInstancesIntoInstanceGroup: project[%s], zone[%s], instanceGroupName[%s], instances[%s]",
		projectID, zone, instanceGroupName, instances)

	instanceGroupService := compute.NewInstanceGroupsService(m.Service)

	instanceReferences := []*compute.InstanceReference{}
	for _, instance := range instances {
		instanceRef := compute.InstanceReference{Instance: instance}
		instanceReferences = append(instanceReferences, &instanceRef)
	}
	request := compute.InstanceGroupsRemoveInstancesRequest{Instances: instanceReferences}

	op, err := instanceGroupService.RemoveInstances(projectID, zone, instanceGroupName, &request).Do()
	if err != nil {
		return nil, err
	}

	return op, nil
}

// ListInstanceGroupsByZone lists all instance groups in specified zone with filter condition
// https://godoc.org/google.golang.org/api/compute/v1#InstanceGroupsService.List
func (m *Manager) ListInstanceGroupsByZone(projectID, zone string, isPrefix func(string) bool) []string {
	log.Tracef(
		"ListInstanceGroupsByZone: project[%s], zone[%s]", projectID, zone)

	instanceGroupService := compute.NewInstanceGroupsService(m.Service)
	instanceGroupList, err := instanceGroupService.List(projectID, zone).Do()
	if err != nil {
		log.Warnf("err: %s", err)
		return []string{}
	}

	result := []string{}
	for _, g := range instanceGroupList.Items {
		if isPrefix(g.Name) {
			result = append(result, g.Name)
		}
	}

	return result
}

// GetTargetPool - https://godoc.org/google.golang.org/api/compute/v1#TargetPoolsService.Get
func (m *Manager) GetTargetPool(projectID, region, targetPool string) (*compute.TargetPool, error) {
	log.Tracef("GetTargetPool: project[%s], region[%s], targetPool[%s]",
		projectID, region, targetPool)

	srv := compute.NewTargetPoolsService(m.Service)
	return srv.Get(projectID, region, targetPool).Do()
}

// AddInstancesIntoTargetPool adds instances into the target pool of load balancer
// https://godoc.org/google.golang.org/api/compute/v1#TargetPoolsService.AddInstance
func (m *Manager) AddInstancesIntoTargetPool(
	projectID, region, targetPool string, instances []string) (*compute.Operation, error) {

	log.Tracef("AddInstancesIntoTargetPool: project[%s], region[%s], targetPool[%s]",
		projectID, region, targetPool)

	srv := compute.NewTargetPoolsService(m.Service)

	instanceReferences := []*compute.InstanceReference{}
	for _, instance := range instances {
		instanceRef := compute.InstanceReference{Instance: instance}
		instanceReferences = append(instanceReferences, &instanceRef)
	}
	request := compute.TargetPoolsAddInstanceRequest{Instances: instanceReferences}

	op, err := srv.AddInstance(projectID, region, targetPool, &request).Do()
	if err != nil {
		return nil, err
	}

	return op, nil
}

// RemoveInstancesFromTargetPool removes instances from the target pool of load balancer
// https://godoc.org/google.golang.org/api/compute/v1#TargetPoolsService.RemoveInstance
func (m *Manager) RemoveInstancesFromTargetPool(
	projectID, region, targetPool string, instances []string) (*compute.Operation, error) {

	log.Tracef("RemoveInstancesFromTargetPool: project[%s], region[%s], targetPool[%s]",
		projectID, region, targetPool)

	srv := compute.NewTargetPoolsService(m.Service)

	instanceReferences := []*compute.InstanceReference{}
	for _, instance := range instances {
		instanceRef := compute.InstanceReference{Instance: instance}
		instanceReferences = append(instanceReferences, &instanceRef)
	}
	request := compute.TargetPoolsRemoveInstanceRequest{Instances: instanceReferences}

	op, err := srv.RemoveInstance(projectID, region, targetPool, &request).Do()
	if err != nil {
		return nil, err
	}

	return op, nil
}

// GetInstanceTemplate ...
// https://godoc.org/google.golang.org/api/compute/v1#InstanceTemplatesService.Get
func (m *Manager) GetInstanceTemplate(projectID, templateName string) (*compute.InstanceTemplate, error) {
	instanceTemplateService := compute.NewInstanceTemplatesService(m.Service)

	return instanceTemplateService.Get(projectID, templateName).Do()
}

// NewInstanceTemplate ...
// https://godoc.org/google.golang.org/api/compute/v1#InstanceTemplatesService.Insert
func (m *Manager) NewInstanceTemplate(projectID string, template *compute.InstanceTemplate) error {
	instanceTemplateService := compute.NewInstanceTemplatesService(m.Service)

	if _, err := instanceTemplateService.Insert(projectID, template).Do(); err != nil {
		log.Warnf("Fail to unmarshal template: error[%s]", err.Error())
		return err
	}

	instanceTemplateCreationObserver := make(chan bool)
	go m.ProbeInstanceTemplateCreation(projectID, template.Name, instanceTemplateCreationObserver)

	done := <-instanceTemplateCreationObserver
	if !done {
		return fmt.Errorf("Timeout new instance template[%s]", template.Name)
	}

	return nil
}

// DeleteInstanceTemplate ...
// https://godoc.org/google.golang.org/api/compute/v1#InstanceTemplatesService.Delete
func (m *Manager) DeleteInstanceTemplate(projectID, templateName string) (*compute.Operation, error) {
	instanceTemplateService := compute.NewInstanceTemplatesService(m.Service)

	return instanceTemplateService.Delete(projectID, templateName).Do()
}

// ListInstanceTemplates lists all instance templates which satisfies filter condition
// https://godoc.org/google.golang.org/api/compute/v1#InstanceTemplatesService.List
func (m *Manager) ListInstanceTemplates(projectID, filter string) ([]*compute.InstanceTemplate, error) {
	log.Debugf("ListInstanceTemplates: filter[%s]", filter)

	instanceTemplateService := compute.NewInstanceTemplatesService(m.Service)

	isContain := func(checked string) bool {
		return strings.Contains(checked, filter)
	}

	tplList, err := instanceTemplateService.List(projectID).Do()
	if err != nil {
		return []*compute.InstanceTemplate{}, err
	}

	tpls := tplList.Items
	result := []*compute.InstanceTemplate{}
	for _, tpl := range tpls {
		if isContain(tpl.Name) {
			result = append(result, tpl)
		}
	}

	return result, nil
}

// GetInstanceGroupManager ...
// https://godoc.org/google.golang.org/api/compute/v1#InstanceGroupManagersService.Get
func (m *Manager) GetInstanceGroupManager(projectID, zone, instanceGroupManagerName string) (
	*compute.InstanceGroupManager, error) {

	instanceGroupManagerService := compute.NewInstanceGroupManagersService(m.Service)

	return instanceGroupManagerService.Get(projectID, zone, instanceGroupManagerName).Do()
}

// ListInstanceGroupManagers ...
// https://godoc.org/google.golang.org/api/compute/v1#InstanceGroupManagersService.List
func (m *Manager) ListInstanceGroupManagers(projectID, zone string) (
	*compute.InstanceGroupManagerList, error) {

	instanceGroupManagerService := compute.NewInstanceGroupManagersService(m.Service)

	return instanceGroupManagerService.List(projectID, zone).Do()
}

// SetInstanceTemplate ...
// https://godoc.org/google.golang.org/api/compute/v1#InstanceGroupManagersService.SetInstanceTemplate
func (m *Manager) SetInstanceTemplate(projectID, zone, instanceGroupManager, instanceTemplate string) error {
	instanceGroupManagerService := compute.NewInstanceGroupManagersService(m.Service)

	templateRequest := &compute.InstanceGroupManagersSetInstanceTemplateRequest{
		InstanceTemplate: instanceTemplate,
	}
	if _, err := instanceGroupManagerService.SetInstanceTemplate(
		projectID, zone, instanceGroupManager, templateRequest).Do(); err != nil {
		return err
	}

	return nil
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

	return &vm, nil
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

// PatchInstanceMachineType replaces the last part of machineTypeURI with targetType
func (m *Manager) PatchInstanceMachineType(machineTypeURI, targetType string) string {
	split := strings.Split(machineTypeURI, "/")
	split[len(split)-1] = targetType
	return strings.Join(split, "/")
}

// ProbeVMRunning probes the VM status till its status is RUNNING or timeout
func (m *Manager) ProbeVMRunning(projectID, zone, vmName string, observer chan<- bool) {
	startTime := time.Now()

	for {
		if time.Now().Sub(startTime) > VMCreationTimeout {
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

		if createdInstance.Status != VMStatusTerminated {
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

// ProbeVMMachineTypeChanged probes if the machineType of VM has been changed
func (m *Manager) ProbeVMMachineTypeChanged(
	projectID, zone, vmName, machineType string, observer chan<- bool) {

	startTime := time.Now()

	for {
		if time.Now().Sub(startTime) > VMSetMachineTypeTimeout {
			log.Warnf("VM setMachineType Timeout: VM[%s]", vmName)
			observer <- false

			break
		}

		changedVM, err := m.GetVM(projectID, zone, vmName)

		if err != nil {
			log.Tracef("VM not yet Existed: VM[%s]", vmName)
			time.Sleep(10 * time.Second)

			continue
		}

		vmMachineType := gosak.GetLastSplit(changedVM.MachineType, "/")
		if vmMachineType != machineType {
			log.Tracef("VM machineType not yet changed: current[%s], target[%s]", vmMachineType, machineType)
			time.Sleep(10 * time.Second)

			continue
		}

		log.Infof("VM machineType Changed!: VM[%s], machineType[%s]", vmName, vmMachineType)

		observer <- true

		break
	}
}

// ProbeInstanceTemplateCreation ...
func (m *Manager) ProbeInstanceTemplateCreation(projectID, templateName string, observer chan<- bool) {
	startTime := time.Now()

	for {
		if time.Now().Sub(startTime) > InstanceTemplateCreationTimeout {
			log.Warnf("InstanceTemplate creation Timeout: name[%s]", templateName)
			observer <- false

			break
		}

		template, err := m.GetInstanceTemplate(projectID, templateName)
		if err != nil {
			log.Tracef("InstanceTemplate not yet Created: name[%s]", templateName)
			time.Sleep(10 * time.Second)

			continue
		}

		log.Infof("InstanceTemplate Created!: name[%s]", template.Name)
		observer <- true

		break
	}
}
