package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	"k8s.io/apimachinery/pkg/api/errors"
	metainternalversion "k8s.io/apimachinery/pkg/apis/meta/internalversion"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/apiserver/pkg/registry/rest"

	storagev1alpha1 "github.com/rancher/fleet/pkg/apis/storage.fleet.cattle.io/v1alpha1"
)

// BundleDeploymentStorage implements rest.StandardStorage for BundleDeployment
type BundleDeploymentStorage struct {
	db *Database
	watcher *Watcher
}

// NewBundleDeploymentStorage creates a new BundleDeployment storage
func NewBundleDeploymentStorage(db *Database) (*BundleDeploymentStorage, error) {
	watcher := NewWatcher(db)
	go watcher.Start()

	return &BundleDeploymentStorage{
		db:      db,
		watcher: watcher,
	}, nil
}

// Ensure BundleDeploymentStorage implements required interfaces
var _ rest.StandardStorage = &BundleDeploymentStorage{}
var _ rest.Scoper = &BundleDeploymentStorage{}
var _ rest.Storage = &BundleDeploymentStorage{}
var _ rest.SingularNameProvider = &BundleDeploymentStorage{}
var _ rest.GroupVersionKindProvider = &BundleDeploymentStorage{}

// New returns a new BundleDeployment
func (s *BundleDeploymentStorage) New() runtime.Object {
	return &storagev1alpha1.BundleDeployment{}
}

// Destroy cleans up resources on deletion
func (s *BundleDeploymentStorage) Destroy() {
	s.watcher.Stop()
}

// NewList returns a new BundleDeploymentList
func (s *BundleDeploymentStorage) NewList() runtime.Object {
	return &storagev1alpha1.BundleDeploymentList{}
}

// GetSingularName returns the singular name for BundleDeployment
func (s *BundleDeploymentStorage) GetSingularName() string {
	return "bundledeployment"
}

// GroupVersionKind returns the GVK for BundleDeployment
func (s *BundleDeploymentStorage) GroupVersionKind(containingGV schema.GroupVersion) schema.GroupVersionKind {
	return storagev1alpha1.SchemeGroupVersion.WithKind("BundleDeployment")
}

// NamespaceScoped returns true if the resource is namespaced
func (s *BundleDeploymentStorage) NamespaceScoped() bool {
	return true
}

// Get retrieves a BundleDeployment by namespace and name
func (s *BundleDeploymentStorage) Get(ctx context.Context, name string, options *metav1.GetOptions) (runtime.Object, error) {
	namespace := ""
	if ns, ok := ctx.Value("namespace").(string); ok {
		namespace = ns
	}

	query := `SELECT namespace, name, resource_version, uid, creation_timestamp, deletion_timestamp, 
	          generation, labels, annotations, finalizers, owner_references, spec, status 
	          FROM bundledeployments WHERE namespace = ? AND name = ?`

	row := s.db.DB().QueryRowContext(ctx, query, namespace, name)

	bd := &storagev1alpha1.BundleDeployment{}
	var labelsJSON, annotationsJSON, finalizersJSON, ownerRefsJSON, specJSON, statusJSON sql.NullString
	var deletionTimestamp sql.NullInt64
	var creationTimestamp int64
	var rvStr string
	var uidStr string

	err := row.Scan(
		&bd.Namespace,
		&bd.Name,
		&rvStr,
		&uidStr,
		&creationTimestamp,
		&deletionTimestamp,
		&bd.Generation,
		&labelsJSON,
		&annotationsJSON,
		&finalizersJSON,
		&ownerRefsJSON,
		&specJSON,
		&statusJSON,
	)

	if err == sql.ErrNoRows {
		return nil, errors.NewNotFound(storagev1alpha1.SchemeGroupVersion.WithResource("bundledeployments").GroupResource(), name)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get bundledeployment: %w", err)
	}

	// Set metadata
	bd.ResourceVersion = rvStr
	bd.UID = types.UID(uidStr)
	bd.CreationTimestamp = metav1.NewTime(time.Unix(creationTimestamp, 0))

	// Unmarshal JSON fields
	if labelsJSON.Valid {
		if err := json.Unmarshal([]byte(labelsJSON.String), &bd.Labels); err != nil {
			return nil, err
		}
	}
	if annotationsJSON.Valid {
		if err := json.Unmarshal([]byte(annotationsJSON.String), &bd.Annotations); err != nil {
			return nil, err
		}
	}
	if finalizersJSON.Valid {
		if err := json.Unmarshal([]byte(finalizersJSON.String), &bd.Finalizers); err != nil {
			return nil, err
		}
	}
	if ownerRefsJSON.Valid {
		if err := json.Unmarshal([]byte(ownerRefsJSON.String), &bd.OwnerReferences); err != nil {
			return nil, err
		}
	}
	if specJSON.Valid {
		if err := json.Unmarshal([]byte(specJSON.String), &bd.Spec); err != nil {
			return nil, err
		}
	}
	if statusJSON.Valid {
		if err := json.Unmarshal([]byte(statusJSON.String), &bd.Status); err != nil {
			return nil, err
		}
	}

	if deletionTimestamp.Valid {
		t := metav1.NewTime(time.Unix(deletionTimestamp.Int64, 0))
		bd.DeletionTimestamp = &t
	}

	bd.TypeMeta = metav1.TypeMeta{
		Kind:       "BundleDeployment",
		APIVersion: storagev1alpha1.SchemeGroupVersion.String(),
	}

	return bd, nil
}

// List retrieves a list of BundleDeployments
func (s *BundleDeploymentStorage) List(ctx context.Context, options *metainternalversion.ListOptions) (runtime.Object, error) {
	namespace := ""
	if ns, ok := ctx.Value("namespace").(string); ok {
		namespace = ns
	}

	query := `SELECT namespace, name, resource_version, uid, creation_timestamp, deletion_timestamp, 
	          generation, labels, annotations, finalizers, owner_references, spec, status 
	          FROM bundledeployments`

	var args []interface{}
	if namespace != "" {
		query += " WHERE namespace = ?"
		args = append(args, namespace)
	}

	// Apply label selector if present
	if options != nil && options.LabelSelector != nil {
		// Simplified label filtering - in production, implement proper label matching
		// For now, we'll filter in memory
	}

	rows, err := s.db.DB().QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to list bundledeployments: %w", err)
	}
	defer rows.Close()

	list := &storagev1alpha1.BundleDeploymentList{
		TypeMeta: metav1.TypeMeta{
			Kind:       "BundleDeploymentList",
			APIVersion: storagev1alpha1.SchemeGroupVersion.String(),
		},
		Items: []storagev1alpha1.BundleDeployment{},
	}

	for rows.Next() {
		bd := storagev1alpha1.BundleDeployment{}
		var labelsJSON, annotationsJSON, finalizersJSON, ownerRefsJSON, specJSON, statusJSON sql.NullString
		var deletionTimestamp sql.NullInt64
		var creationTimestamp int64
		var rvStr string
		var uidStr string

		err := rows.Scan(
			&bd.Namespace,
			&bd.Name,
			&rvStr,
			&uidStr,
			&creationTimestamp,
			&deletionTimestamp,
			&bd.Generation,
			&labelsJSON,
			&annotationsJSON,
			&finalizersJSON,
			&ownerRefsJSON,
			&specJSON,
			&statusJSON,
		)
		if err != nil {
			return nil, err
		}

		// Set metadata
		bd.ResourceVersion = rvStr
		bd.UID = types.UID(uidStr)
		bd.CreationTimestamp = metav1.NewTime(time.Unix(creationTimestamp, 0))

		// Unmarshal JSON fields
		if labelsJSON.Valid {
			json.Unmarshal([]byte(labelsJSON.String), &bd.Labels)
		}
		if annotationsJSON.Valid {
			json.Unmarshal([]byte(annotationsJSON.String), &bd.Annotations)
		}
		if finalizersJSON.Valid {
			json.Unmarshal([]byte(finalizersJSON.String), &bd.Finalizers)
		}
		if ownerRefsJSON.Valid {
			json.Unmarshal([]byte(ownerRefsJSON.String), &bd.OwnerReferences)
		}
		if specJSON.Valid {
			json.Unmarshal([]byte(specJSON.String), &bd.Spec)
		}
		if statusJSON.Valid {
			json.Unmarshal([]byte(statusJSON.String), &bd.Status)
		}

		if deletionTimestamp.Valid {
			t := metav1.NewTime(time.Unix(deletionTimestamp.Int64, 0))
			bd.DeletionTimestamp = &t
		}

		bd.TypeMeta = metav1.TypeMeta{
			Kind:       "BundleDeployment",
			APIVersion: storagev1alpha1.SchemeGroupVersion.String(),
		}

		// Apply label selector filtering
		if options != nil && options.LabelSelector != nil {
			if !options.LabelSelector.Matches(labels.Set(bd.Labels)) {
				continue
			}
		}

		list.Items = append(list.Items, bd)
	}

	// Set resource version to current
	list.ResourceVersion = strconv.FormatInt(s.db.CurrentResourceVersion(), 10)

	return list, nil
}

// Create creates a new BundleDeployment
func (s *BundleDeploymentStorage) Create(ctx context.Context, obj runtime.Object, createValidation rest.ValidateObjectFunc, options *metav1.CreateOptions) (runtime.Object, error) {
	bd, ok := obj.(*storagev1alpha1.BundleDeployment)
	if !ok {
		return nil, fmt.Errorf("invalid object type")
	}

	namespace := bd.Namespace
	if namespace == "" {
		if ns, ok := ctx.Value("namespace").(string); ok {
			namespace = ns
			bd.Namespace = namespace
		}
	}

	// Generate UID if not present
	if bd.UID == "" {
		bd.UID = types.UID(fmt.Sprintf("%s-%d", bd.Name, time.Now().UnixNano()))
	}

	// Set creation timestamp
	if bd.CreationTimestamp.IsZero() {
		bd.CreationTimestamp = metav1.NewTime(time.Now())
	}

	// Set generation
	bd.Generation = 1

	// Get next resource version
	rv, err := s.db.NextResourceVersion()
	if err != nil {
		return nil, err
	}
	bd.ResourceVersion = strconv.FormatInt(rv, 10)

	// Marshal JSON fields
	labelsJSON, _ := json.Marshal(bd.Labels)
	annotationsJSON, _ := json.Marshal(bd.Annotations)
	finalizersJSON, _ := json.Marshal(bd.Finalizers)
	ownerRefsJSON, _ := json.Marshal(bd.OwnerReferences)
	specJSON, _ := json.Marshal(bd.Spec)
	statusJSON, _ := json.Marshal(bd.Status)

	query := `INSERT INTO bundledeployments 
	          (namespace, name, resource_version, uid, creation_timestamp, generation, 
	           labels, annotations, finalizers, owner_references, spec, status) 
	          VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`

	_, err = s.db.DB().ExecContext(ctx, query,
		bd.Namespace,
		bd.Name,
		rv,
		string(bd.UID),
		bd.CreationTimestamp.Unix(),
		bd.Generation,
		string(labelsJSON),
		string(annotationsJSON),
		string(finalizersJSON),
		string(ownerRefsJSON),
		string(specJSON),
		string(statusJSON),
	)

	if err != nil {
		return nil, fmt.Errorf("failed to create bundledeployment: %w", err)
	}

	// Record watch event
	s.watcher.RecordEvent(rv, watch.Added, namespace, bd.Name)

	bd.TypeMeta = metav1.TypeMeta{
		Kind:       "BundleDeployment",
		APIVersion: storagev1alpha1.SchemeGroupVersion.String(),
	}

	return bd, nil
}

// Update updates an existing BundleDeployment
func (s *BundleDeploymentStorage) Update(ctx context.Context, name string, objInfo rest.UpdatedObjectInfo, createValidation rest.ValidateObjectFunc, updateValidation rest.ValidateObjectUpdateFunc, forceAllowCreate bool, options *metav1.UpdateOptions) (runtime.Object, bool, error) {
	// Get existing object
	existing, err := s.Get(ctx, name, &metav1.GetOptions{})
	if err != nil {
		if !errors.IsNotFound(err) || !forceAllowCreate {
			return nil, false, err
		}
		// Create if not found and force create is allowed
		newObj, err := objInfo.UpdatedObject(ctx, nil)
		if err != nil {
			return nil, false, err
		}
		created, err := s.Create(ctx, newObj, createValidation, &metav1.CreateOptions{})
		return created, true, err
	}

	existingBD := existing.(*storagev1alpha1.BundleDeployment)

	// Get updated object
	updatedObj, err := objInfo.UpdatedObject(ctx, existingBD)
	if err != nil {
		return nil, false, err
	}

	bd, ok := updatedObj.(*storagev1alpha1.BundleDeployment)
	if !ok {
		return nil, false, fmt.Errorf("invalid object type")
	}

	// Preserve immutable fields
	bd.UID = existingBD.UID
	bd.CreationTimestamp = existingBD.CreationTimestamp
	bd.Generation = existingBD.Generation + 1

	// Get next resource version
	rv, err := s.db.NextResourceVersion()
	if err != nil {
		return nil, false, err
	}
	bd.ResourceVersion = strconv.FormatInt(rv, 10)

	// Marshal JSON fields
	labelsJSON, _ := json.Marshal(bd.Labels)
	annotationsJSON, _ := json.Marshal(bd.Annotations)
	finalizersJSON, _ := json.Marshal(bd.Finalizers)
	ownerRefsJSON, _ := json.Marshal(bd.OwnerReferences)
	specJSON, _ := json.Marshal(bd.Spec)
	statusJSON, _ := json.Marshal(bd.Status)

	var deletionTimestampVal interface{}
	if bd.DeletionTimestamp != nil {
		deletionTimestampVal = bd.DeletionTimestamp.Unix()
	}

	query := `UPDATE bundledeployments SET 
	          resource_version = ?, generation = ?, deletion_timestamp = ?,
	          labels = ?, annotations = ?, finalizers = ?, owner_references = ?, 
	          spec = ?, status = ? 
	          WHERE namespace = ? AND name = ?`

	_, err = s.db.DB().ExecContext(ctx, query,
		rv,
		bd.Generation,
		deletionTimestampVal,
		string(labelsJSON),
		string(annotationsJSON),
		string(finalizersJSON),
		string(ownerRefsJSON),
		string(specJSON),
		string(statusJSON),
		bd.Namespace,
		bd.Name,
	)

	if err != nil {
		return nil, false, fmt.Errorf("failed to update bundledeployment: %w", err)
	}

	// Record watch event
	s.watcher.RecordEvent(rv, watch.Modified, bd.Namespace, bd.Name)

	bd.TypeMeta = metav1.TypeMeta{
		Kind:       "BundleDeployment",
		APIVersion: storagev1alpha1.SchemeGroupVersion.String(),
	}

	return bd, false, nil
}

// Delete deletes a BundleDeployment
func (s *BundleDeploymentStorage) Delete(ctx context.Context, name string, deleteValidation rest.ValidateObjectFunc, options *metav1.DeleteOptions) (runtime.Object, bool, error) {
	// Get the object first
	existing, err := s.Get(ctx, name, &metav1.GetOptions{})
	if err != nil {
		return nil, false, err
	}

	bd := existing.(*storagev1alpha1.BundleDeployment)

	// Get namespace from context
	namespace := bd.Namespace

	// Get next resource version
	rv, err := s.db.NextResourceVersion()
	if err != nil {
		return nil, false, err
	}

	// Delete from database
	query := `DELETE FROM bundledeployments WHERE namespace = ? AND name = ?`
	_, err = s.db.DB().ExecContext(ctx, query, namespace, name)
	if err != nil {
		return nil, false, fmt.Errorf("failed to delete bundledeployment: %w", err)
	}

	// Record watch event
	s.watcher.RecordEvent(rv, watch.Deleted, namespace, name)

	return bd, true, nil
}

// DeleteCollection deletes a collection of BundleDeployments
func (s *BundleDeploymentStorage) DeleteCollection(ctx context.Context, deleteValidation rest.ValidateObjectFunc, options *metav1.DeleteOptions, listOptions *metainternalversion.ListOptions) (runtime.Object, error) {
	list, err := s.List(ctx, listOptions)
	if err != nil {
		return nil, err
	}

	bdList := list.(*storagev1alpha1.BundleDeploymentList)
	for _, bd := range bdList.Items {
		_, _, err := s.Delete(ctx, bd.Name, deleteValidation, options)
		if err != nil {
			return nil, err
		}
	}

	return list, nil
}

// Watch watches for changes to BundleDeployments
func (s *BundleDeploymentStorage) Watch(ctx context.Context, options *metainternalversion.ListOptions) (watch.Interface, error) {
	namespace := ""
	if ns, ok := ctx.Value("namespace").(string); ok {
		namespace = ns
	}

	rv := int64(0)
	if options != nil && options.ResourceVersion != "" {
		var err error
		rv, err = strconv.ParseInt(options.ResourceVersion, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("invalid resource version: %w", err)
		}
	}

	return s.watcher.Watch(ctx, namespace, rv, options)
}

// ConvertToTable converts the object to a table for printing
func (s *BundleDeploymentStorage) ConvertToTable(ctx context.Context, obj runtime.Object, tableOptions runtime.Object) (*metav1.Table, error) {
	gr := storagev1alpha1.SchemeGroupVersion.WithResource("bundledeployments").GroupResource()
	return rest.NewDefaultTableConvertor(gr).ConvertToTable(ctx, obj, tableOptions)
}

// Status returns a status writer for BundleDeployment
func (s *BundleDeploymentStorage) Status() *BundleDeploymentStatusStorage {
	return &BundleDeploymentStatusStorage{storage: s}
}

// BundleDeploymentStatusStorage implements status subresource
type BundleDeploymentStatusStorage struct {
	storage *BundleDeploymentStorage
}

var _ rest.Storage = &BundleDeploymentStatusStorage{}
var _ rest.Patcher = &BundleDeploymentStatusStorage{}

func (s *BundleDeploymentStatusStorage) New() runtime.Object {
	return &storagev1alpha1.BundleDeployment{}
}

func (s *BundleDeploymentStatusStorage) Destroy() {}

func (s *BundleDeploymentStatusStorage) Get(ctx context.Context, name string, options *metav1.GetOptions) (runtime.Object, error) {
	return s.storage.Get(ctx, name, options)
}

func (s *BundleDeploymentStatusStorage) Update(ctx context.Context, name string, objInfo rest.UpdatedObjectInfo, createValidation rest.ValidateObjectFunc, updateValidation rest.ValidateObjectUpdateFunc, forceAllowCreate bool, options *metav1.UpdateOptions) (runtime.Object, bool, error) {
	return s.storage.Update(ctx, name, objInfo, createValidation, updateValidation, false, options)
}


