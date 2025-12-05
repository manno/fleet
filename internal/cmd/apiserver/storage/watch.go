package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	metainternalversion "k8s.io/apimachinery/pkg/apis/meta/internalversion"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/watch"

	storagev1alpha1 "github.com/rancher/fleet/pkg/apis/storage.fleet.cattle.io/v1alpha1"
)

// Watcher manages watch connections for BundleDeployments
type Watcher struct {
	db       *Database
	mu       sync.RWMutex
	watchers map[int]*watcherInstance
	nextID   int
	stopCh   chan struct{}
}

type watcherInstance struct {
	id             int
	namespace      string
	resourceVersion int64
	ch             chan watch.Event
	ctx            context.Context
	labelSelector  labels.Selector
	fieldSelector  fields.Selector
}

type watchEvent struct {
	ID              int64
	ResourceVersion int64
	EventType       string
	Namespace       string
	Name            string
	Timestamp       int64
}

// NewWatcher creates a new watcher
func NewWatcher(db *Database) *Watcher {
	return &Watcher{
		db:       db,
		watchers: make(map[int]*watcherInstance),
		stopCh:   make(chan struct{}),
	}
}

// Start starts the watcher polling loop
func (w *Watcher) Start() {
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-w.stopCh:
			return
		case <-ticker.C:
			w.poll()
		}
	}
}

// Stop stops the watcher
func (w *Watcher) Stop() {
	close(w.stopCh)
	w.mu.Lock()
	defer w.mu.Unlock()

	for _, wi := range w.watchers {
		close(wi.ch)
	}
	w.watchers = make(map[int]*watcherInstance)
}

// Watch creates a new watch for BundleDeployments
func (w *Watcher) Watch(ctx context.Context, namespace string, resourceVersion int64, options *metainternalversion.ListOptions) (watch.Interface, error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	wi := &watcherInstance{
		id:              w.nextID,
		namespace:       namespace,
		resourceVersion: resourceVersion,
		ch:              make(chan watch.Event, 100),
		ctx:             ctx,
	}

	if options != nil {
		wi.labelSelector = options.LabelSelector
		wi.fieldSelector = options.FieldSelector
	}

	w.nextID++
	w.watchers[wi.id] = wi

	// Start goroutine to monitor context cancellation
	go func() {
		<-ctx.Done()
		w.removeWatcher(wi.id)
	}()

	// Send initial events for existing resources if starting from beginning
	if resourceVersion == 0 {
		go w.sendInitialEvents(wi)
	}

	return &watchAdapter{ch: wi.ch}, nil
}

// sendInitialEvents sends initial ADDED events for all existing resources
func (w *Watcher) sendInitialEvents(wi *watcherInstance) {
	query := `SELECT namespace, name, resource_version, uid, creation_timestamp, deletion_timestamp, 
	          generation, labels, annotations, finalizers, owner_references, spec, status 
	          FROM bundledeployments`

	var args []interface{}
	if wi.namespace != "" {
		query += " WHERE namespace = ?"
		args = append(args, wi.namespace)
	}

	rows, err := w.db.DB().Query(query, args...)
	if err != nil {
		return
	}
	defer rows.Close()

	for rows.Next() {
		bd := &storagev1alpha1.BundleDeployment{}
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
			continue
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

		// Apply label selector
		if wi.labelSelector != nil && !wi.labelSelector.Matches(labels.Set(bd.Labels)) {
			continue
		}

		select {
		case wi.ch <- watch.Event{Type: watch.Added, Object: bd}:
		case <-wi.ctx.Done():
			return
		default:
			// Channel full, skip
		}
	}
}

// poll checks for new watch events and dispatches them
func (w *Watcher) poll() {
	w.mu.RLock()
	watchers := make([]*watcherInstance, 0, len(w.watchers))
	for _, wi := range w.watchers {
		watchers = append(watchers, wi)
	}
	w.mu.RUnlock()

	for _, wi := range watchers {
		w.pollWatcher(wi)
	}
}

// pollWatcher polls for events for a specific watcher
func (w *Watcher) pollWatcher(wi *watcherInstance) {
	query := `SELECT id, resource_version, event_type, namespace, name, timestamp 
	          FROM watch_events WHERE resource_version > ? ORDER BY resource_version ASC LIMIT 100`

	args := []interface{}{wi.resourceVersion}
	if wi.namespace != "" {
		query = `SELECT id, resource_version, event_type, namespace, name, timestamp 
		         FROM watch_events WHERE resource_version > ? AND namespace = ? 
		         ORDER BY resource_version ASC LIMIT 100`
		args = append(args, wi.namespace)
	}

	rows, err := w.db.DB().Query(query, args...)
	if err != nil {
		return
	}
	defer rows.Close()

	for rows.Next() {
		var event watchEvent
		if err := rows.Scan(&event.ID, &event.ResourceVersion, &event.EventType, &event.Namespace, &event.Name, &event.Timestamp); err != nil {
			continue
		}

		// Update resource version
		if event.ResourceVersion > wi.resourceVersion {
			wi.resourceVersion = event.ResourceVersion
		}

		// Convert event type
		var eventType watch.EventType
		switch event.EventType {
		case "ADDED":
			eventType = watch.Added
		case "MODIFIED":
			eventType = watch.Modified
		case "DELETED":
			eventType = watch.Deleted
		default:
			continue
		}

		// For DELETED events, we don't have the full object
		if eventType == watch.Deleted {
			bd := &storagev1alpha1.BundleDeployment{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: event.Namespace,
					Name:      event.Name,
				},
				TypeMeta: metav1.TypeMeta{
					Kind:       "BundleDeployment",
					APIVersion: storagev1alpha1.SchemeGroupVersion.String(),
				},
			}
			select {
			case wi.ch <- watch.Event{Type: eventType, Object: bd}:
			case <-wi.ctx.Done():
				return
			default:
				// Channel full
			}
			continue
		}

		// Fetch the full object for ADDED and MODIFIED events
		bd, err := w.getBundleDeployment(event.Namespace, event.Name)
		if err != nil {
			continue
		}

		// Apply label selector
		if wi.labelSelector != nil && !wi.labelSelector.Matches(labels.Set(bd.Labels)) {
			continue
		}

		select {
		case wi.ch <- watch.Event{Type: eventType, Object: bd}:
		case <-wi.ctx.Done():
			return
		default:
			// Channel full
		}
	}
}

// getBundleDeployment retrieves a BundleDeployment from the database
func (w *Watcher) getBundleDeployment(namespace, name string) (*storagev1alpha1.BundleDeployment, error) {
	query := `SELECT namespace, name, resource_version, uid, creation_timestamp, deletion_timestamp, 
	          generation, labels, annotations, finalizers, owner_references, spec, status 
	          FROM bundledeployments WHERE namespace = ? AND name = ?`

	row := w.db.DB().QueryRow(query, namespace, name)

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

	return bd, nil
}

// RecordEvent records a watch event in the database
func (w *Watcher) RecordEvent(resourceVersion int64, eventType watch.EventType, namespace, name string) {
	eventTypeStr := ""
	switch eventType {
	case watch.Added:
		eventTypeStr = "ADDED"
	case watch.Modified:
		eventTypeStr = "MODIFIED"
	case watch.Deleted:
		eventTypeStr = "DELETED"
	default:
		return
	}

	query := `INSERT INTO watch_events (resource_version, event_type, namespace, name, timestamp) 
	          VALUES (?, ?, ?, ?, ?)`

	_, err := w.db.DB().Exec(query, resourceVersion, eventTypeStr, namespace, name, time.Now().Unix())
	if err != nil {
		// Log error but don't fail the operation
		fmt.Printf("Failed to record watch event: %v\n", err)
	}

	// Clean up old watch events (keep last 10000)
	go w.cleanupOldEvents()
}

// cleanupOldEvents removes old watch events to prevent unbounded growth
func (w *Watcher) cleanupOldEvents() {
	query := `DELETE FROM watch_events WHERE id IN (
		SELECT id FROM watch_events ORDER BY id DESC LIMIT -1 OFFSET 10000
	)`
	w.db.DB().Exec(query)
}

// removeWatcher removes a watcher instance
func (w *Watcher) removeWatcher(id int) {
	w.mu.Lock()
	defer w.mu.Unlock()

	if wi, ok := w.watchers[id]; ok {
		close(wi.ch)
		delete(w.watchers, id)
	}
}

// watchAdapter adapts the channel-based watch to the watch.Interface
type watchAdapter struct {
	ch <-chan watch.Event
}

func (w *watchAdapter) Stop() {
	// Channel is closed by the watcher when context is done
}

func (w *watchAdapter) ResultChan() <-chan watch.Event {
	return w.ch
}
