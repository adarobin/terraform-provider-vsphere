package contentlibrary

import (
	"context"
	"fmt"
	"github.com/hashicorp/terraform-plugin-sdk/helper/schema"
	"github.com/hashicorp/terraform-provider-vsphere/vsphere/internal/helper/datastore"
	"github.com/hashicorp/terraform-provider-vsphere/vsphere/internal/helper/provider"
	"github.com/vmware/govmomi"
	"github.com/vmware/govmomi/vapi/library"
	"github.com/vmware/govmomi/vapi/rest"
	"github.com/vmware/govmomi/vapi/vcenter"
	"log"
	"path/filepath"
	"time"
)

// FromName accepts a Content Library name and returns a Library object.
func FromName(c *rest.Client, name string) (*library.Library, error) {
	log.Printf("[DEBUG] contentlibrary.FromName: Retrieving content library %s by name", name)
	clm := library.NewManager(c)
	ctx := context.TODO()
	lib, err := clm.GetLibraryByName(ctx, name)
	if err != nil {
		return nil, provider.ProviderError(name, "FromName", err)
	}
	if lib == nil {
		return nil, provider.ProviderError(name, "FromName", fmt.Errorf("Unable to find content library (%s)", name))
	}
	log.Printf("[DEBUG] contentlibrary.FromName: Successfully retrieved content library %s", name)
	return lib, nil
}

// FromID accepts a Content Library ID and returns a Library object.
func FromID(c *rest.Client, id string) (*library.Library, error) {
	log.Printf("[DEBUG] contentlibrary.FromID: Retrieving content library %s by ID", id)
	clm := library.NewManager(c)
	ctx := context.TODO()
	lib, err := clm.GetLibraryByID(ctx, id)
	if err != nil {
		return nil, provider.ProviderError(id, "FromID", err)
	}
	if lib == nil {
		return nil, fmt.Errorf("Unable to find content library (%s)", id)
	}
	log.Printf("[DEBUG] contentlibrary.FromID: Successfully retrieved content library %s", id)
	return lib, nil
}

// CreateLibrary creates a Content Library.
func CreateLibrary(c *rest.Client, name string, description string, backings []library.StorageBackings) (string, error) {
	log.Printf("[DEBUG] contentlibrary.CreateLibrary: Creating content library %s", name)
	clm := library.NewManager(c)
	ctx := context.TODO()
	lib := library.Library{
		Description: description,
		Name:        name,
		Storage:     backings,
		Type:        "LOCAL", // govmomi only supports LOCAL library creation
	}
	id, err := clm.CreateLibrary(ctx, lib)
	if err != nil {
		return "", provider.ProviderError(name, "CreateLibrary", err)
	}
	log.Printf("[DEBUG] contentlibrary.CreateLibrary: Content library %s successfully created", name)
	return id, nil
}

func UpdateLibrary(c *rest.Client, ol *library.Library, name string, description string, backings []library.StorageBackings) error {
	// Not currently supported in govmomi
	return nil
}

// DeleteLibrary deletes a Content Library.
func DeleteLibrary(c *rest.Client, lib *library.Library) error {
	log.Printf("[DEBUG] contentlibrary.DeleteLibrary: Deleting library %s", lib.Name)
	clm := library.NewManager(c)
	ctx := context.TODO()
	err := clm.DeleteLibrary(ctx, lib)
	if err != nil {
		return provider.ProviderError(lib.ID, "DeleteLibrary", err)
	}
	log.Printf("[DEBUG] contentlibrary.DeleteLibrary: Deleting library %s", lib.Name)
	return nil
}

// ItemFromName accepts a Content Library item name along with a Content Library and will return the item object.
func ItemFromName(c *rest.Client, l *library.Library, name string) (*library.Item, error) {
	log.Printf("[DEBUG] contentlibrary.ItemFromName: Retrieving library item %s.", name)
	clm := library.NewManager(c)
	ctx := context.TODO()
	fi := library.FindItem{
		LibraryID: l.ID,
		Name:      name,
	}
	items, err := clm.FindLibraryItems(ctx, fi)
	if err != nil {
		return nil, nil
	}
	if len(items) < 1 {
		return nil, fmt.Errorf("Unable to find content library item (%s)", name)
	}
	item, err := clm.GetLibraryItem(ctx, items[0])
	if err != nil {
		return nil, err
	}
	log.Printf("[DEBUG] contentlibrary.ItemFromName: Library item %s retrieved successfully", name)
	return item, nil
}

// ItemFromID accepts a Content Library item ID and will return the item object.
func ItemFromID(c *rest.Client, id string) (*library.Item, error) {
	log.Printf("[DEBUG] contentlibrary.ItemFromID: Retrieving library item %s", id)
	clm := library.NewManager(c)
	ctx := context.TODO()
	item, err := clm.GetLibraryItem(ctx, id)
	if err != nil {
		return nil, provider.ProviderError(id, "ItemFromID", err)
	}
	log.Printf("[DEBUG] contentlibrary.ItemFromID: Library item %s retrieved successfully", id)
	return item, nil
}

// IsContentLibraryItem accepts an ID and determines if that ID is associated with an item in a Content Library.
func IsContentLibraryItem(c *rest.Client, id string) bool {
	log.Printf("[DEBUG] contentlibrary.IsContentLibrary: Checking if %s is a content library source", id)
	item, _ := ItemFromID(c, id)
	if item != nil {
		return true
	}
	return false
}

// CreateLibraryItem creates an item in a Content Library.
func CreateLibraryItem(c *rest.Client, l *library.Library, name string, desc string, t string, file string, moid string) (*string, error) {
	log.Printf("[DEBUG] contentlibrary.CreateLibraryItem: Creating content library item %s.", name)
	clm := library.NewManager(c)
	ctx := context.TODO()
	item := library.Item{
		Description: desc,
		LibraryID:   l.ID,
		Name:        name,
		Type:        t,
	}
	uploadSession := libraryUploadSession{
		ContentLibraryManager: clm,
		RestClient:            c,
		LibraryID:             l.ID,
	}
	if moid != "" {
		return uploadSession.cloneTemplate(moid, name, t)
	}

	id, err := clm.CreateLibraryItem(ctx, item)
	if err != nil {
		return nil, provider.ProviderError(name, "CreateLibraryItem", err)
	}
	session, err := clm.CreateLibraryItemUpdateSession(ctx, library.Session{LibraryItemID: id})
	if err != nil {
		return nil, provider.ProviderError(name, "CreateLibraryItem", err)
	}
	defer clm.CompleteLibraryItemUpdateSession(ctx, session)
	uploadSession.UploadSession = session

	isOva := false
	isLocal := true

	if strings.HasPrefix(file, "http") {
		isLocal = false
	}
	if strings.HasSuffix(file, ".ova") {
		isOva = true
	}

	ovfDescriptor, err := ovfdeploy.GetOvfDescriptor(file, isOva, isLocal, true)
	if err != nil {
		return nil, provider.ProviderError(name, "CreateLibraryItem", err)
	}
	e, err := ReadEnvelope(ovfDescriptor)
	if err != nil {
		return nil, fmt.Errorf("failed to parse ovf: %s", err)
	}

	if isLocal {
		if isOva {
			name := strings.TrimSuffix(filepath.Base(file), "ova")
			if err := uploadSession.uploadString(ovfDescriptor, name+"ovf"); err != nil {
				return nil, err
			}
			if err = uploadSession.uploadOvaDisksFromLocal(file, e); err != nil {
				return nil, err
			}
		} else {
			if err := uploadSession.uploadLocalFile(file); err != nil {
				return nil, err
			}
			dir := filepath.Dir(file)
			for i := range e.References {
				if err := uploadSession.uploadLocalFile(dir + "/" + e.References[i].Href); err != nil {
					return nil, err
				}
			}
		}
	} else {
		if !isOva {
			clm.AddLibraryItemFileFromURI(ctx, session, filepath.Base(file), file)
			clm.WaitOnLibraryItemUpdateSession(ctx, session, time.Second*10, func() { log.Printf("Waiting...") })
		} else {
			name := strings.TrimSuffix(filepath.Base(file), "ova")
			if err := uploadSession.uploadString(ovfDescriptor, name+"ovf"); err != nil {
				return nil, err
			}
			for _, disk := range e.References {
				uploadSession.uploadOvaDisksFromUrl(file, disk.Href, int64(disk.Size))
			}
		}
	}

	err = clm.CompleteLibraryItemUpdateSession(ctx, session)
	if err != nil {
		return nil, err
	}
	log.Printf("[DEBUG] contentlibrary.CreateLibraryItem: Successfully created content library item %s.", name)
	return &id, nil
}

type libraryUploadSession struct {
	ContentLibraryManager *library.Manager
	RestClient            *rest.Client
	UploadSession         string
	LibraryID             string
}

func (s libraryUploadSession) cloneTemplate(moid string, name string, templateType string) (*string, error) {
	ctx := context.TODO()
	if templateType == "ovf" {
		ovf := vcenter.OVF{
			Spec: vcenter.CreateSpec{
				Name: name,
			},
			Source: vcenter.ResourceID{
				Value: moid,
			},
			Target: vcenter.LibraryTarget{
				LibraryID: s.LibraryID,
			},
		}
		id, err := vcenter.NewManager(s.RestClient).CreateOVF(ctx, ovf)
		if err != nil {
			return nil, err
		}
		return &id, nil
	}
	return nil, fmt.Errorf("Unsupported template type. Only ovf can be used when cloning from vCenter")
}

func (s libraryUploadSession) uploadString(data string, name string) error {
	stringReader := strings.NewReader(data)
	openFile := io.Reader(stringReader)
	size := int64(len([]byte(data)))
	if err := s.upload(name, &openFile, size); err != nil {
		return err
	}
	return nil
}

func (s libraryUploadSession) uploadLocalFile(file string) error {
	openFile, size, err := openLocalFile(file)
	if err != nil {
		return err
	}
	if err = s.upload(filepath.Base(file), openFile, *size); err != nil {
		return err
	}
	return nil
}

func openLocalFile(file string) (*io.Reader, *int64, error) {
	openFile, err := os.Open(file)
	if err != nil {
		return nil, nil, err
	}
	statFile, err := openFile.Stat()
	if err != nil {
		return nil, nil, err
	}
	openFileReader := io.Reader(openFile)
	size := statFile.Size()
	return &openFileReader, &size, nil
}

func (s libraryUploadSession) uploadOvaDisksFromLocal(ovaFilePath string, envelope *ovf.Envelope) error {
	ovaFile, _, err := openLocalFile(ovaFilePath)
	if err != nil {
		return err
	}

	for _, disk := range envelope.References {
		size := disk.Size
		fileName := disk.Href
		if err = s.findAndUploadDiskFromOva(*ovaFile, fileName, int64(size)); err != nil {
			return err
		}
	}
	return err
}

func (s libraryUploadSession) uploadOvaDisksFromUrl(ovfFilePath string, diskName string, size int64) error {
	resp, err := http.Get(ovfFilePath)
	if err != nil {
		return err
	}
	if resp.StatusCode == http.StatusOK {
		err = s.findAndUploadDiskFromOva(resp.Body, diskName, size)
		if err != nil {
			return err
		}
	} else {
		return fmt.Errorf("got status %d while getting the file from remote url %s ", resp.StatusCode, ovfFilePath)
	}
	return nil
}

func (s libraryUploadSession) findAndUploadDiskFromOva(ovaFile io.Reader, diskName string, size int64) error {
	log.Printf("[DEBUG] findAndUploadDiskFromOva: Finding %s", diskName)
	ovaReader := tar.NewReader(ovaFile)
	for {
		fileHdr, err := ovaReader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		if fileHdr.Name == diskName {
			log.Printf("[DEBUG] findAndUploadDiskFromOva: %s found", diskName)
			ioOvaReader := io.Reader(ovaReader)
			err = s.upload(diskName, &ioOvaReader, size)
			if err != nil {
				return fmt.Errorf("error while uploading the file %s %s", diskName, err)
			}
			return nil
		}
	}
	return fmt.Errorf("disk %s not found inside ova", diskName)
}

func ReadEnvelope(data string) (*ovf.Envelope, error) {
	e, err := ovf.Unmarshal(strings.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("failed to parse ovf: %s", err)
	}

	return e, nil
}

func (s libraryUploadSession) uploadOvfFromLocal(name string) error {
	base := filepath.Base(name)
	fileOpen, err := os.Open(name)
	if err != nil {
		return err
	}
	stat, err := fileOpen.Stat()
	if err != nil {
		return err
	}
	size := stat.Size()
	fileReader := io.Reader(fileOpen)
	return s.upload(base, &fileReader, size)
}

func (s libraryUploadSession) upload(name string, file *io.Reader, size int64) error {
	ctx := context.TODO()

	info := library.UpdateFile{
		Name:       name,
		SourceType: "PUSH",
		Size:       size,
	}

	update, err := s.ContentLibraryManager.AddLibraryItemFile(ctx, s.UploadSession, info)
	if err != nil {
		return err
	}

	p := soap.DefaultUpload
	p.ContentLength = size
	u, err := url.Parse(update.UploadEndpoint.URI)
	if err != nil {
		return err
	}
	return s.RestClient.Upload(ctx, *file, u, &p)
}

// UpdateLibraryItem updates an item in a Content Library.
func UpdateLibraryItem(c *rest.Client, l *library.Library, oi *library.Item, name string, desc string) (string, error) {
	log.Printf("[DEBUG] contentlibrary.UpdateLibraryItem: Updating content library item %s.", name)
	clm := library.NewManager(c)
	ctx := context.TODO()
	item := library.Item{
		Description: desc,
		LibraryID:   l.ID,
		ID:          oi.ID,
		Name:        name,
	}
	id, err := clm.CreateLibraryItem(ctx, item)
	if err != nil {
		return "", err
	}
	log.Printf("[DEBUG] contentlibrary.UpdateLibraryItem: Updating content library item %s.", name)
	return id, nil
}

// DeleteLibraryItem deletes an item from a Content Library.
func DeleteLibraryItem(c *rest.Client, item *library.Item) error {
	log.Printf("[DEBUG] contentlibrary.DeleteLibraryItem: Deleting content library item %s.", item.Name)
	clm := library.NewManager(c)
	ctx := context.TODO()
	err := clm.DeleteLibraryItem(ctx, item)
	if err != nil {
		return err
	}
	log.Printf("[DEBUG] contentlibrary.DeleteLibraryItem: Successfully deleted content library item %s.", item.Name)
	return nil
}

// ExpandStorageBackings takes ResourceData, and returns a list of StorageBackings.
func ExpandStorageBackings(c *govmomi.Client, d *schema.ResourceData) ([]library.StorageBackings, error) {
	log.Printf("[DEBUG] contentlibrary.ExpandStorageBackings: Expanding OVF storage backing.")
	sb := []library.StorageBackings{}
	for _, dsId := range d.Get("storage_backing").(*schema.Set).List() {
		ds, err := datastore.FromID(c, dsId.(string))
		if err != nil {
			return nil, provider.ProviderError(d.Id(), "ExpandStorageBackings", err)
		}
		sb = append(sb, library.StorageBackings{
			DatastoreID: ds.Reference().Value,
			Type:        "DATASTORE",
		})
	}
	log.Printf("[DEBUG] contentlibrary.ExpandStorageBackings: Successfully expanded OVF storage backing.")
	return sb, nil
}

// FlattenStorageBackings takes a list of StorageBackings, and returns a list of datastore IDs.
func FlattenStorageBackings(d *schema.ResourceData, sb []library.StorageBackings) error {
	log.Printf("[DEBUG] contentlibrary.FlattenStorageBackings: Flattening OVF storage backing.")
	sbl := []string{}
	for _, backing := range sb {
		if backing.Type == "DATASTORE" {
			sbl = append(sbl, backing.DatastoreID)
		}
	}
	return d.Set("storage_backing", sbl)
}

// MapStorageDevices maps disks defined in the OVF to datastores.
func MapStorageDevices(d *schema.ResourceData) []vcenter.StorageMapping {
	sm := []vcenter.StorageMapping{}
	disks := d.Get("disk").([]interface{})
	for _, di := range disks {
		dm := di.(map[string]interface{})["ovf_mapping"].(string)
		dd := di.(map[string]interface{})["datastore_id"].(string)
		if dd == "<computed>" || dd == "" {
			dd = d.Get("datastore_id").(string)
		}
		dp := di.(map[string]interface{})["storage_policy_id"].(string)
		if dp == "" {
			dp = d.Get("storage_policy_id").(string)
		}
		sm = append(sm, vcenter.StorageMapping{Key: dm, Value: vcenter.StorageGroupMapping{Type: "DATASTORE", DatastoreID: dd, StorageProfileID: dp}})
	}
	return sm
}

// MapNetworkDevices maps NICs defined in the OVF to networks..
func MapNetworkDevices(d *schema.ResourceData) []vcenter.NetworkMapping {
	nm := []vcenter.NetworkMapping{}
	nics := d.Get("network_interface").([]interface{})
	for _, di := range nics {
		dm := di.(map[string]interface{})["ovf_mapping"].(string)
		dd := di.(map[string]interface{})["network_id"].(string)
		dp := di.(map[string]interface{})["storage_policy_id"]
		if dp != nil && dp.(string) == "" {
			dp = d.Get("storage_policy_id").(string)
		}
		nm = append(nm, vcenter.NetworkMapping{Key: dm, Value: dd})
	}
	return nm
}
