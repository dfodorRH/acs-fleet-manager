// Package reconciler provides update, delete and create logic for managing Central instances.
package reconciler

import (
	"bytes"
	"context"
	"fmt"
	"strconv"
	"sync/atomic"

	"github.com/stackrox/acs-fleet-manager/fleetshard/pkg/central/charts"
	"helm.sh/helm/v3/pkg/chart"
	"helm.sh/helm/v3/pkg/chartutil"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/golang/glog"
	openshiftRouteV1 "github.com/openshift/api/route/v1"
	"github.com/pkg/errors"
	"github.com/stackrox/acs-fleet-manager/fleetshard/pkg/k8s"
	"github.com/stackrox/acs-fleet-manager/fleetshard/pkg/util"
	centralConstants "github.com/stackrox/acs-fleet-manager/internal/dinosaur/constants"
	"github.com/stackrox/acs-fleet-manager/internal/dinosaur/pkg/api/private"
	"github.com/stackrox/acs-fleet-manager/internal/dinosaur/pkg/converters"
	"github.com/stackrox/acs-fleet-manager/internal/dinosaur/pkg/defaults"
	"github.com/stackrox/rox/operator/apis/platform/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	apiErrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/pointer"
	ctrlClient "sigs.k8s.io/controller-runtime/pkg/client"
)

// FreeStatus ...
const (
	FreeStatus int32 = iota
	BlockedStatus

	revisionAnnotationKey = "rhacs.redhat.com/revision"

	helmReleaseName = "tenant-resources"

	managedServicesAnnotation = "platform.stackrox.io/managed-services"

	vpaConfigurationCentralName = "vpa-config-central"
)

var (
	vpaConfigurations = []string{vpaConfigurationCentralName}
)

// CentralReconcilerOptions are the static options for creating a reconciler.
type CentralReconcilerOptions struct {
	UseRoutes         bool
	WantsAuthProvider bool
	EgressProxyImage  string
}

// CentralReconciler is a reconciler tied to a one Central instance. It installs, updates and deletes Central instances
// in its Reconcile function.
type CentralReconciler struct {
	client            ctrlClient.Client
	central           private.ManagedCentral
	status            *int32
	lastCentralHash   [16]byte
	useRoutes         bool
	wantsAuthProvider bool
	hasAuthProvider   bool
	routeService      *k8s.RouteService
	egressProxyImage  string

	resourcesChart *chart.Chart
}

// Reconcile takes a private.ManagedCentral and tries to install it into the cluster managed by the fleet-shard.
// It tries to create a namespace for the Central and applies necessary updates to the resource.
// TODO(sbaumer): Check correct Central gets reconciled
// TODO(sbaumer): Should an initial ManagedCentral be added on reconciler creation?
func (r *CentralReconciler) Reconcile(ctx context.Context, remoteCentral private.ManagedCentral) (*private.DataPlaneCentralStatus, error) {
	// Only allow to start reconcile function once
	if !atomic.CompareAndSwapInt32(r.status, FreeStatus, BlockedStatus) {
		return nil, ErrBusy
	}
	defer atomic.StoreInt32(r.status, FreeStatus)

	changed, err := r.centralChanged(remoteCentral)
	if err != nil {
		return nil, errors.Wrapf(err, "checking if central changed")
	}

	remoteCentralName := remoteCentral.Metadata.Name
	remoteCentralNamespace := remoteCentral.Metadata.Namespace
	if !changed && r.wantsAuthProvider == r.hasAuthProvider && isRemoteCentralReady(remoteCentral) {
		return nil, ErrCentralNotChanged
	}

	monitoringExposeEndpointEnabled := v1alpha1.ExposeEndpointEnabled

	centralResources := defaults.CentralResources
	if err = patchResourceList(&centralResources.Requests, remoteCentral.Spec.Central.Resources.Requests); err != nil {
		return nil, errors.Wrap(err, "updating Central resource requests")
	}
	if err = patchResourceList(&centralResources.Limits, remoteCentral.Spec.Central.Resources.Limits); err != nil {
		return nil, errors.Wrap(err, "updating Central resource limits")
	}

	scannerAnalyzerResources, err := converters.ConvertPrivateResourceRequirementsToCoreV1(&remoteCentral.Spec.Scanner.Analyzer.Resources)
	if err != nil {
		return nil, errors.Wrap(err, "converting Scanner Analyzer resources")
	}
	scannerAnalyzerScaling := converters.ConvertPrivateScalingToV1(&remoteCentral.Spec.Scanner.Analyzer.Scaling)
	scannerDbResources, err := converters.ConvertPrivateResourceRequirementsToCoreV1(&remoteCentral.Spec.Scanner.Db.Resources)
	if err != nil {
		return nil, errors.Wrap(err, "converting Scanner DB resources")
	}

	// Set proxy configuration
	envVars := getProxyEnvVars(remoteCentralNamespace)

	central := &v1alpha1.Central{
		ObjectMeta: metav1.ObjectMeta{
			Name:        remoteCentralName,
			Namespace:   remoteCentralNamespace,
			Labels:      map[string]string{k8s.ManagedByLabelKey: k8s.ManagedByFleetshardValue},
			Annotations: map[string]string{managedServicesAnnotation: "true"},
		},
		Spec: v1alpha1.CentralSpec{
			Central: &v1alpha1.CentralComponentSpec{
				Exposure: &v1alpha1.Exposure{
					Route: &v1alpha1.ExposureRoute{
						Enabled: pointer.BoolPtr(r.useRoutes),
					},
				},
				Monitoring: &v1alpha1.Monitoring{
					ExposeEndpoint: &monitoringExposeEndpointEnabled,
				},
				DeploymentSpec: v1alpha1.DeploymentSpec{
					Resources: &centralResources,
				},
			},
			Scanner: &v1alpha1.ScannerComponentSpec{
				Analyzer: &v1alpha1.ScannerAnalyzerComponent{
					DeploymentSpec: v1alpha1.DeploymentSpec{
						Resources: &scannerAnalyzerResources,
					},
					Scaling: &scannerAnalyzerScaling,
				},
				DB: &v1alpha1.DeploymentSpec{
					Resources: &scannerDbResources,
				},
				Monitoring: &v1alpha1.Monitoring{
					ExposeEndpoint: &monitoringExposeEndpointEnabled,
				},
			},
			Customize: &v1alpha1.CustomizeSpec{
				EnvVars: envVars,
			},
		},
	}

	// Check whether auth provider is actually created and this reconciler just is not aware of that.
	if r.wantsAuthProvider && !r.hasAuthProvider {
		exists, err := existsRHSSOAuthProvider(ctx, remoteCentral, r.client)
		if err != nil {
			return nil, err
		}
		// If sso.redhat.com auth provider exists, there is no need for admin/password login.
		// We also store whether auth provider exists within reconciler instance to avoid polluting network.
		if exists {
			glog.Infof("Auth provider for %s/%s already exists", remoteCentralNamespace, remoteCentralName)
			r.hasAuthProvider = true
		}
	}

	if r.hasAuthProvider {
		central.Spec.Central.AdminPasswordGenerationDisabled = pointer.BoolPtr(true)
	}

	if remoteCentral.Metadata.DeletionTimestamp != "" {
		deleted, err := r.ensureCentralDeleted(ctx, remoteCentral, central)
		if err != nil {
			return nil, errors.Wrapf(err, "delete central %s/%s", remoteCentralNamespace, remoteCentralName)
		}
		if deleted {
			return deletedStatus(), nil
		}
		return nil, ErrDeletionInProgress
	}

	if err := r.ensureNamespaceExists(remoteCentralNamespace); err != nil {
		return nil, errors.Wrapf(err, "unable to ensure that namespace %s exists", remoteCentralNamespace)
	}

	if err := r.ensureChartResourcesExist(ctx, remoteCentral); err != nil {
		return nil, errors.Wrapf(err, "unable to install chart resource for central %s/%s", central.GetNamespace(), central.GetName())
	}

	centralExists := true
	existingCentral := v1alpha1.Central{}
	err = r.client.Get(ctx, ctrlClient.ObjectKey{Namespace: remoteCentralNamespace, Name: remoteCentralName}, &existingCentral)
	if err != nil {
		if !apiErrors.IsNotFound(err) {
			return nil, errors.Wrapf(err, "unable to check the existence of central %s/%s", central.GetNamespace(), central.GetName())
		}
		centralExists = false
	}

	if !centralExists {
		if central.GetAnnotations() == nil {
			central.Annotations = map[string]string{}
		}
		central.GetAnnotations()[revisionAnnotationKey] = "1"

		glog.Infof("Creating central %s/%s", central.GetNamespace(), central.GetName())
		if err := r.client.Create(ctx, central); err != nil {
			return nil, errors.Wrapf(err, "creating new central %s/%s", remoteCentralNamespace, remoteCentralName)
		}
		glog.Infof("Central %s/%s created", central.GetNamespace(), central.GetName())

		if len(remoteCentral.Spec.Central.Resources.Limits) == 0 && len(remoteCentral.Spec.Central.Resources.Requests) == 0 {
			// Provided Central resources are empty. Create a VPA configuration in the tenant's namespace.
			if err := r.createVPAConfig(ctx, vpaConfigurationCentralName, "central", remoteCentral); err != nil {
				return nil, err
			}
		}
	} else {
		glog.Infof("Update central %s/%s", central.GetNamespace(), central.GetName())
		existingCentral.Spec = central.Spec

		err = r.incrementCentralRevision(&existingCentral)
		if err != nil {
			return nil, err
		}
		existingCentral.Spec = *central.Spec.DeepCopy()

		if err := r.client.Update(ctx, &existingCentral); err != nil {
			return nil, errors.Wrapf(err, "updating central %s/%s", central.GetNamespace(), central.GetName())
		}

		if len(remoteCentral.Spec.Central.Resources.Limits) == 0 && len(remoteCentral.Spec.Central.Resources.Requests) == 0 {
			// Provided Central resources are empty. Create a VPA configuration in the tenant's namespace.
			if err := r.ensureVPAConfigExists(ctx, vpaConfigurationCentralName, "central", remoteCentral); err != nil {
				return nil, err
			}
		} else {
			if _, err := r.ensureVPAConfigDeleted(ctx, vpaConfigurationCentralName, remoteCentral); err != nil {
				return nil, err
			}
		}
	}

	centralTLSSecretFound := true // pragma: allowlist secret
	if r.useRoutes {
		if err := r.ensureRoutesExist(ctx, remoteCentral); err != nil {
			if errors.Is(err, k8s.ErrCentralTLSSecretNotFound) {
				centralTLSSecretFound = false // pragma: allowlist secret
			} else {
				return nil, errors.Wrapf(err, "updating routes")
			}
		}
	}

	// Check whether deployment is ready.
	centralDeploymentReady, err := isCentralDeploymentReady(ctx, r.client, remoteCentral)
	if err != nil {
		return nil, err
	}
	if !centralDeploymentReady || !centralTLSSecretFound {
		if isRemoteCentralProvisioning(remoteCentral) && !changed { // no changes detected, wait until central become ready
			return nil, ErrCentralNotChanged
		}
		return installingStatus(), nil
	}

	// Skip auth provider initialisation if:
	// 1. Auth provider is already created
	// 2. OR reconciler creator specified auth provider not to be created
	// 3. OR Central request is in status "Ready" - meaning auth provider should've been initialised earlier
	if r.wantsAuthProvider && !r.hasAuthProvider && !isRemoteCentralReady(remoteCentral) {
		err = createRHSSOAuthProvider(ctx, remoteCentral, r.client)
		if err != nil {
			return nil, err
		}
		r.hasAuthProvider = true
	}

	status := readyStatus()
	// Do not report routes statuses if:
	// 1. Routes are not used on the cluster
	// 2. Central request is in status "Ready" - assuming that routes are already reported and saved
	if r.useRoutes && !isRemoteCentralReady(remoteCentral) {
		status.Routes, err = r.getRoutesStatuses(ctx, remoteCentralNamespace)
		if err != nil {
			return nil, err
		}
	}

	// Setting the last central hash must always be executed as the last step.
	// defer can't be used for this call because it is also executed after the reconcile failed.
	if err := r.setLastCentralHash(remoteCentral); err != nil {
		return nil, errors.Wrapf(err, "setting central reconcilation cache")
	}

	return status, nil
}

func isRemoteCentralProvisioning(remoteCentral private.ManagedCentral) bool {
	return remoteCentral.RequestStatus == centralConstants.CentralRequestStatusProvisioning.String()
}

func isRemoteCentralReady(remoteCentral private.ManagedCentral) bool {
	return remoteCentral.RequestStatus == centralConstants.CentralRequestStatusReady.String()
}

func (r *CentralReconciler) getRoutesStatuses(ctx context.Context, namespace string) ([]private.DataPlaneCentralStatusRoutes, error) {
	reencryptIngress, err := r.routeService.FindReencryptIngress(ctx, namespace)
	if err != nil {
		return nil, fmt.Errorf("obtaining ingress for reencrypt route: %w", err)
	}
	passthroughIngress, err := r.routeService.FindPassthroughIngress(ctx, namespace)
	if err != nil {
		return nil, fmt.Errorf("obtaining ingress for passthrough route: %w", err)
	}
	return []private.DataPlaneCentralStatusRoutes{
		getRouteStatus(reencryptIngress),
		getRouteStatus(passthroughIngress),
	}, nil
}

func getRouteStatus(ingress *openshiftRouteV1.RouteIngress) private.DataPlaneCentralStatusRoutes {
	return private.DataPlaneCentralStatusRoutes{
		Domain: ingress.Host,
		Router: ingress.RouterCanonicalHostname,
	}
}

func (r CentralReconciler) ensureCentralDeleted(ctx context.Context, remoteCentral private.ManagedCentral, central *v1alpha1.Central) (bool, error) {
	globalDeleted := true
	if r.useRoutes {
		reencryptRouteDeleted, err := r.ensureReencryptRouteDeleted(ctx, central.GetNamespace())
		if err != nil {
			return false, err
		}
		passthroughRouteDeleted, err := r.ensurePassthroughRouteDeleted(ctx, central.GetNamespace())
		if err != nil {
			return false, err
		}

		globalDeleted = globalDeleted && reencryptRouteDeleted && passthroughRouteDeleted
	}

	centralDeleted, err := r.ensureCentralCRDeleted(ctx, central)
	if err != nil {
		return false, err
	}
	globalDeleted = globalDeleted && centralDeleted

	vpaDeleted, err := r.ensureVPAConfigDeleted(ctx, vpaConfigurationCentralName, remoteCentral)
	if err != nil {
		return false, err
	}
	globalDeleted = globalDeleted && vpaDeleted

	chartResourcesDeleted, err := r.ensureChartResourcesDeleted(ctx, remoteCentral)
	if err != nil {
		return false, err
	}
	globalDeleted = globalDeleted && chartResourcesDeleted

	nsDeleted, err := r.ensureNamespaceDeleted(ctx, central.GetNamespace())
	if err != nil {
		return false, err
	}
	globalDeleted = globalDeleted && nsDeleted
	return globalDeleted, nil
}

// centralChanged compares the given central to the last central reconciled using a hash
func (r *CentralReconciler) centralChanged(central private.ManagedCentral) (bool, error) {
	currentHash, err := util.MD5SumFromJSONStruct(&central)
	if err != nil {
		return true, errors.Wrap(err, "hashing central")
	}

	return !bytes.Equal(r.lastCentralHash[:], currentHash[:]), nil
}

func (r *CentralReconciler) setLastCentralHash(central private.ManagedCentral) error {
	hash, err := util.MD5SumFromJSONStruct(&central)
	if err != nil {
		return fmt.Errorf("calculating MD5 from JSON: %w", err)
	}

	r.lastCentralHash = hash
	return nil
}

func (r *CentralReconciler) incrementCentralRevision(central *v1alpha1.Central) error {
	revision, err := strconv.Atoi(central.Annotations[revisionAnnotationKey])
	if err != nil {
		return errors.Wrapf(err, "failed to increment central revision %s", central.GetName())
	}
	revision++
	central.Annotations[revisionAnnotationKey] = fmt.Sprintf("%d", revision)
	return nil
}

func (r *CentralReconciler) getNamespace(name string) (*corev1.Namespace, error) {
	namespace := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
	}
	err := r.client.Get(context.Background(), ctrlClient.ObjectKey{Name: name}, namespace)
	if err != nil {
		// Propagate corev1.Namespace to the caller so that the namespace can be easily created
		return namespace, fmt.Errorf("retrieving resource for namespace %q from Kubernetes: %w", name, err)
	}
	return namespace, nil
}

func (r *CentralReconciler) ensureNamespaceExists(name string) error {
	namespace, err := r.getNamespace(name)
	if err != nil {
		if apiErrors.IsNotFound(err) {
			err = r.client.Create(context.Background(), namespace)
			if err != nil {
				return fmt.Errorf("creating namespace %q: %w", name, err)
			}
			return nil
		}
		return fmt.Errorf("getting namespace %s: %w", name, err)
	}
	return nil
}

func (r *CentralReconciler) ensureNamespaceDeleted(ctx context.Context, name string) (bool, error) {
	namespace, err := r.getNamespace(name)
	if err != nil {
		if apiErrors.IsNotFound(err) {
			return true, nil
		}
		return false, errors.Wrapf(err, "delete central namespace %s", name)
	}
	if namespace.Status.Phase == corev1.NamespaceTerminating {
		return false, nil // Deletion is already in progress, skipping deletion request
	}
	if err = r.client.Delete(ctx, namespace); err != nil {
		return false, errors.Wrapf(err, "delete central namespace %s", name)
	}
	glog.Infof("Central namespace %s is marked for deletion", name)
	return false, nil
}

func (r *CentralReconciler) ensureCentralCRDeleted(ctx context.Context, central *v1alpha1.Central) (bool, error) {
	err := r.client.Get(ctx, ctrlClient.ObjectKey{Namespace: central.GetNamespace(), Name: central.GetName()}, central)
	if err != nil {
		if apiErrors.IsNotFound(err) {
			return true, nil
		}

		return false, errors.Wrapf(err, "delete central CR %s/%s", central.GetNamespace(), central.GetName())
	}
	if err := r.client.Delete(ctx, central); err != nil {
		return false, errors.Wrapf(err, "delete central CR %s/%s", central.GetNamespace(), central.GetName())
	}
	glog.Infof("Central CR %s/%s is marked for deletion", central.GetNamespace(), central.GetName())
	return false, nil
}

func (r *CentralReconciler) createVPAConfig(ctx context.Context, name string, deploymentName string, remoteCentral private.ManagedCentral) error {
	glog.Infof("creating VPA configuration for Central %s/%s", remoteCentral.Metadata.Namespace, remoteCentral.Metadata.Name)

	vpaConfig := unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "autoscaling.k8s.io/v1",
			"kind":       "VerticalPodAutoscaler",
			"metadata": map[string]interface{}{
				"name":      name,
				"namespace": remoteCentral.Metadata.Namespace,
			},
			"spec": map[string]interface{}{
				"targetRef": map[string]interface{}{
					"apiVersion": "apps/v1",
					"kind":       "Deployment",
					"name":       deploymentName,
				},
				"updatePolicy": map[string]interface{}{
					"updateMode": "Auto",
				},
			},
		},
	}

	err := r.client.Create(ctx, &vpaConfig)
	if err != nil {
		return fmt.Errorf("creating VPA configuration for Central %s/%s: %w", remoteCentral.Metadata.Namespace, remoteCentral.Metadata.Name, err)
	}

	glog.Infof("VPA configuration for %s/%s created", remoteCentral.Metadata.Namespace, remoteCentral.Metadata.Name)

	return nil
}

func (r *CentralReconciler) ensureVPAConfigExists(ctx context.Context, name string, deploymentName string, remoteCentral private.ManagedCentral) error {
	vpaConfig := unstructured.Unstructured{}
	namespace := remoteCentral.Metadata.Namespace

	vpaConfig.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   "autoscaling.k8s.io",
		Version: "v1",
		Kind:    "VerticalPodAutoscaler",
	})
	err := r.client.Get(ctx, ctrlClient.ObjectKey{Namespace: remoteCentral.Metadata.Namespace, Name: name}, &vpaConfig)
	if err != nil && !apiErrors.IsNotFound(err) {
		return fmt.Errorf("retrieving VPA configuration for namespace %q: %w", namespace, err)
	}

	if apiErrors.IsNotFound(err) {
		if err := r.createVPAConfig(ctx, name, deploymentName, remoteCentral); err != nil {
			return err
		}
	}

	return nil
}

func (r *CentralReconciler) ensureVPAConfigDeleted(ctx context.Context, name string, central private.ManagedCentral) (bool, error) {
	vpaConfig := unstructured.Unstructured{}
	vpaConfig.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   "autoscaling.k8s.io",
		Version: "v1",
		Kind:    "VerticalPodAutoscaler",
	})

	err := r.client.Get(ctx, ctrlClient.ObjectKey{Namespace: central.Metadata.Namespace, Name: name}, &vpaConfig)
	if err != nil {
		if apiErrors.IsNotFound(err) {
			return true, nil
		}
		return false, errors.Wrapf(err, "deleting VPA configuration for Central %s/%s", central.Metadata.Namespace, vpaConfigurationCentralName)
	}
	if err := r.client.Delete(ctx, &vpaConfig); err != nil {
		return false, errors.Wrapf(err, "deleting VPA configuration for Central %s/%s", central.Metadata.Namespace, vpaConfigurationCentralName)
	}
	glog.Infof("VPA configuration %s/%s deleted", central.Metadata.Namespace, vpaConfigurationCentralName)
	return false, nil
}

func (r *CentralReconciler) ensureChartResourcesExist(ctx context.Context, remoteCentral private.ManagedCentral) error {
	vals, err := r.chartValues(remoteCentral)
	if err != nil {
		return fmt.Errorf("obtaining values for resources chart: %w", err)
	}

	objs, err := charts.RenderToObjects(helmReleaseName, remoteCentral.Metadata.Namespace, r.resourcesChart, vals)
	if err != nil {
		return fmt.Errorf("rendering resources chart: %w", err)
	}
	for _, obj := range objs {
		if obj.GetNamespace() == "" {
			obj.SetNamespace(remoteCentral.Metadata.Namespace)
		}
		key := ctrlClient.ObjectKey{Namespace: obj.GetNamespace(), Name: obj.GetName()}
		var out unstructured.Unstructured
		out.SetGroupVersionKind(obj.GroupVersionKind())
		err := r.client.Get(ctx, key, &out)
		if err == nil {
			continue
		}
		if !apiErrors.IsNotFound(err) {
			return fmt.Errorf("failed to retrieve object %s/%s of type %v: %w", key.Namespace, key.Name, obj.GroupVersionKind(), err)
		}
		err = r.client.Create(ctx, obj)
		if err != nil && !apiErrors.IsAlreadyExists(err) {
			return fmt.Errorf("failed to create object %s/%s of type %v: %w", key.Namespace, key.Name, obj.GroupVersionKind(), err)
		}
	}
	return nil
}

func (r *CentralReconciler) ensureChartResourcesDeleted(ctx context.Context, remoteCentral private.ManagedCentral) (bool, error) {
	vals, err := r.chartValues(remoteCentral)
	if err != nil {
		return false, fmt.Errorf("obtaining values for resources chart: %w", err)
	}

	objs, err := charts.RenderToObjects(helmReleaseName, remoteCentral.Metadata.Namespace, r.resourcesChart, vals)
	if err != nil {
		return false, fmt.Errorf("rendering resources chart: %w", err)
	}

	waitForDelete := false
	for _, obj := range objs {
		key := ctrlClient.ObjectKey{Namespace: obj.GetNamespace(), Name: obj.GetName()}
		if key.Namespace == "" {
			key.Namespace = remoteCentral.Metadata.Namespace
		}
		var out unstructured.Unstructured
		out.SetGroupVersionKind(obj.GroupVersionKind())
		err := r.client.Get(ctx, key, &out)
		if err != nil {
			if apiErrors.IsNotFound(err) {
				continue
			}
			return false, fmt.Errorf("retrieving object %s/%s of type %v: %w", key.Namespace, key.Name, obj.GroupVersionKind(), err)
		}
		if out.GetDeletionTimestamp() != nil {
			waitForDelete = true
			continue
		}
		err = r.client.Delete(ctx, &out)
		if err != nil && !apiErrors.IsNotFound(err) {
			return false, fmt.Errorf("retrieving object %s/%s of type %v: %w", key.Namespace, key.Name, obj.GroupVersionKind(), err)
		}
	}
	return !waitForDelete, nil
}

func (r *CentralReconciler) ensureRoutesExist(ctx context.Context, remoteCentral private.ManagedCentral) error {
	err := r.ensureReencryptRouteExists(ctx, remoteCentral)
	if err != nil {
		return err
	}
	return r.ensurePassthroughRouteExists(ctx, remoteCentral)
}

// TODO(ROX-9310): Move re-encrypt route reconciliation to the StackRox operator
func (r *CentralReconciler) ensureReencryptRouteExists(ctx context.Context, remoteCentral private.ManagedCentral) error {
	namespace := remoteCentral.Metadata.Namespace
	_, err := r.routeService.FindReencryptRoute(ctx, namespace)
	if err != nil && !apiErrors.IsNotFound(err) {
		return fmt.Errorf("retrieving reencrypt route for namespace %q: %w", namespace, err)
	}

	if apiErrors.IsNotFound(err) {
		err = r.routeService.CreateReencryptRoute(ctx, remoteCentral)
		if err != nil {
			return fmt.Errorf("creating reencrypt route for central %s: %w", remoteCentral.Id, err)
		}
	}

	return nil
}

type routeSupplierFunc func() (*openshiftRouteV1.Route, error)

// TODO(ROX-9310): Move re-encrypt route reconciliation to the StackRox operator
// TODO(ROX-11918): Make hostname configurable on the StackRox operator
func (r *CentralReconciler) ensureReencryptRouteDeleted(ctx context.Context, namespace string) (bool, error) {
	return r.ensureRouteDeleted(ctx, func() (*openshiftRouteV1.Route, error) {
		return r.routeService.FindReencryptRoute(ctx, namespace) //nolint:wrapcheck
	})
}

// TODO(ROX-11918): Make hostname configurable on the StackRox operator
func (r *CentralReconciler) ensurePassthroughRouteExists(ctx context.Context, remoteCentral private.ManagedCentral) error {
	namespace := remoteCentral.Metadata.Namespace
	_, err := r.routeService.FindPassthroughRoute(ctx, namespace)
	if err != nil && !apiErrors.IsNotFound(err) {
		return fmt.Errorf("retrieving passthrough route for namespace %q: %w", namespace, err)
	}

	if apiErrors.IsNotFound(err) {
		err = r.routeService.CreatePassthroughRoute(ctx, remoteCentral)
		if err != nil {
			return fmt.Errorf("creating passthrough route for central %s: %w", remoteCentral.Id, err)
		}
	}

	return nil
}

// TODO(ROX-11918): Make hostname configurable on the StackRox operator
func (r *CentralReconciler) ensurePassthroughRouteDeleted(ctx context.Context, namespace string) (bool, error) {
	return r.ensureRouteDeleted(ctx, func() (*openshiftRouteV1.Route, error) {
		return r.routeService.FindPassthroughRoute(ctx, namespace) //nolint:wrapcheck
	})
}

func (r *CentralReconciler) ensureRouteDeleted(ctx context.Context, routeSupplier routeSupplierFunc) (bool, error) {
	route, err := routeSupplier()
	if err != nil {
		if apiErrors.IsNotFound(err) {
			return true, nil
		}
		return false, errors.Wrapf(err, "get central route %s/%s", route.GetNamespace(), route.GetName())
	}
	if err := r.client.Delete(ctx, route); err != nil {
		return false, errors.Wrapf(err, "delete central route %s/%s", route.GetNamespace(), route.GetName())
	}
	return false, nil
}

func (r *CentralReconciler) chartValues(remoteCentral private.ManagedCentral) (chartutil.Values, error) {
	vals := chartutil.Values{
		"labels": map[string]interface{}{
			k8s.ManagedByLabelKey: k8s.ManagedByFleetshardValue,
		},
	}
	if r.egressProxyImage != "" {
		override := chartutil.Values{
			"egressProxy": chartutil.Values{
				"image": r.egressProxyImage,
			},
		}
		vals = chartutil.CoalesceTables(vals, override)
	}

	return vals, nil
}

var (
	resourcesChart = charts.MustGetChart("tenant-resources")
)

// NewCentralReconciler ...
func NewCentralReconciler(k8sClient ctrlClient.Client, central private.ManagedCentral, opts CentralReconcilerOptions) *CentralReconciler {
	return &CentralReconciler{
		client:            k8sClient,
		central:           central,
		status:            pointer.Int32(FreeStatus),
		useRoutes:         opts.UseRoutes,
		wantsAuthProvider: opts.WantsAuthProvider,
		routeService:      k8s.NewRouteService(k8sClient),
		egressProxyImage:  opts.EgressProxyImage,

		resourcesChart: resourcesChart,
	}
}

func patchResourceList(resources *corev1.ResourceList, updates map[string]string) error {
	knownResources := []string{corev1.ResourceCPU.String(), corev1.ResourceMemory.String()}
	resourcesMap := (map[corev1.ResourceName]resource.Quantity)(*resources)

nextUpdate:
	for k, v := range updates {
		for _, res := range knownResources {
			if k == res {
				qty, err := resource.ParseQuantity(v)
				if err != nil {
					return fmt.Errorf("parsing quantity: %w", err)
				}
				resourcesMap[corev1.ResourceName(k)] = qty
				continue nextUpdate
			}
		}
	}

	return nil
}
