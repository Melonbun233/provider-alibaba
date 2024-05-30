/*
Copyright 2021 The Crossplane Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package redis

import (
	"context"
	"fmt"
	"strconv"
	"time"

	xpv1 "github.com/crossplane/crossplane-runtime/apis/common/v1"
	"github.com/crossplane/crossplane-runtime/pkg/event"
	"github.com/crossplane/crossplane-runtime/pkg/logging"
	"github.com/crossplane/crossplane-runtime/pkg/meta"
	"github.com/crossplane/crossplane-runtime/pkg/reconciler/managed"
	"github.com/crossplane/crossplane-runtime/pkg/resource"
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/crossplane-contrib/provider-alibaba/apis/redis/v1alpha1"
	aliv1beta1 "github.com/crossplane-contrib/provider-alibaba/apis/v1beta1"
	"github.com/crossplane-contrib/provider-alibaba/pkg/clients/redis"
)

const (
	controllerName = "AliCloud Controller"
	// Fall to connection instance error description
	errCreateInstanceConnectionFailed = "cannot instance connection"

	errNotInstance            = "managed resource is not an instance custom resource"
	errNoProvider             = "no provider config or provider specified"
	errCreateClient           = "cannot create redis client"
	errGetProviderConfig      = "cannot get provider config"
	errTrackUsage             = "cannot track provider config usage"
	errNoConnectionSecret     = "no connection secret specified"
	errGetConnectionSecret    = "cannot get connection secret"
	errInstanceIdEmpty        = "instance id is empty, maybe it's not created"
	errInstanceAlreadyCreated = "instance id is not empty, maybe it's already been created"
	errStatusUpdate           = "failed to update CR status"

	errCreateFailed        = "cannot create redis instance"
	errCreateAccountFailed = "cannot create redis account"
	errDeleteFailed        = "cannot delete redis instance"
	errDescribeFailed      = "cannot describe redis instance"
	errAccountNameInvalid  = "instance name is invalid"

	errFmtUnsupportedCredSource = "credentials source %q is not currently supported"
	errDuplicateConnectionPort  = "InvalidConnectionStringOrPort.Duplicate"
	errAccountNameDuplicate     = "InvalidAccountName.Duplicate"
)

// SetupRedisInstance adds a controller that reconciles RedisInstances.
func SetupRedisInstance(mgr ctrl.Manager, l logging.Logger) error {
	name := managed.ControllerName(v1alpha1.RedisInstanceGroupKind)

	connector := &redisConnector{
		kubeClient:     mgr.GetClient(),
		usage:          resource.NewProviderConfigUsageTracker(mgr.GetClient(), &aliv1beta1.ProviderConfigUsage{}),
		newRedisClient: redis.NewClient,
	}

	reconcilerOpts := []managed.ReconcilerOption{
		managed.WithExternalConnecter(connector),
		managed.WithLogger(l.WithValues(controllerName, name)),
		managed.WithRecorder(event.NewAPIRecorder(mgr.GetEventRecorderFor(name))),
		// Use empty initializer to make sure the externalName is not initialized
		// externalName is going to be updated after a successful creation
		managed.WithInitializers(),
		managed.WithCreationGracePeriod(3 * time.Minute),
	}

	r := managed.NewReconciler(
		mgr,
		resource.ManagedKind(v1alpha1.RedisInstanceGroupVersionKind),
		reconcilerOpts...,
	)

	return ctrl.NewControllerManagedBy(mgr).
		Named(name).
		For(&v1alpha1.RedisInstance{}).
		Complete(r)
}

type redisConnector struct {
	kubeClient     client.Client
	usage          resource.Tracker
	newRedisClient func(ctx context.Context, accessKeyID, accessKeySecret, region string) (redis.Client, error)
}

func (c *redisConnector) Connect(ctx context.Context, mg resource.Managed) (managed.ExternalClient, error) { //nolint:gocyclo
	// account for the deprecated Provider type.
	cr, ok := mg.(*v1alpha1.RedisInstance)
	if !ok {
		return nil, errors.New(errNotInstance)
	}

	// provider has more than one kind of managed resource.
	var (
		sel    *xpv1.SecretKeySelector
		region string
	)
	switch {
	case cr.GetProviderConfigReference() != nil:
		if err := c.usage.Track(ctx, mg); err != nil {
			return nil, errors.Wrap(err, errTrackUsage)
		}

		pc := &aliv1beta1.ProviderConfig{}
		if err := c.kubeClient.Get(ctx, types.NamespacedName{Name: cr.Spec.ProviderConfigReference.Name}, pc); err != nil {
			return nil, errors.Wrap(err, errGetProviderConfig)
		}
		if s := pc.Spec.Credentials.Source; s != xpv1.CredentialsSourceSecret {
			return nil, errors.Errorf(errFmtUnsupportedCredSource, s)
		}
		sel = pc.Spec.Credentials.SecretRef
		region = pc.Spec.Region
	default:
		return nil, errors.New(errNoProvider)
	}

	if sel == nil {
		return nil, errors.New(errNoConnectionSecret)
	}

	s := &corev1.Secret{}
	nn := types.NamespacedName{Namespace: sel.Namespace, Name: sel.Name}
	if err := c.kubeClient.Get(ctx, nn, s); err != nil {
		return nil, errors.Wrap(err, errGetConnectionSecret)
	}

	redisClient, err := c.newRedisClient(ctx, string(s.Data["accessKeyId"]), string(s.Data["accessKeySecret"]), region)
	return &external{redisClient: redisClient, kubeClient: c.kubeClient}, errors.Wrap(err, errCreateClient)
}

type external struct {
	redisClient redis.Client
	kubeClient  client.Client
}

func (e *external) Observe(ctx context.Context, mg resource.Managed) (managed.ExternalObservation, error) {
	cr, ok := mg.(*v1alpha1.RedisInstance)
	if !ok {
		return managed.ExternalObservation{}, errors.New(errNotInstance)
	}

	instanceId := meta.GetExternalName(cr)
	// externalName will not be initialized until a successful creation
	if instanceId == "" {
		return managed.ExternalObservation{
			ResourceExists: false,
		}, nil
	}

	attr, err := e.redisClient.DescribeInstance(instanceId)
	if err != nil {
		fmt.Print(err.Error(), resource.Ignore(redis.IsErrorNotFound, err))
		return managed.ExternalObservation{}, errors.Wrap(resource.Ignore(redis.IsErrorNotFound, err), errDescribeFailed)
	}

	cr.Status.AtProvider = redis.GenerateObservation(attr)

	switch cr.Status.AtProvider.DBInstanceStatus {
	case v1alpha1.RedisInstanceStateRunning:
		cr.Status.SetConditions(xpv1.Available())
	case v1alpha1.RedisInstanceStateCreating:
		cr.Status.SetConditions(xpv1.Creating())
	case v1alpha1.RedisInstanceStateDeleting:
		cr.Status.SetConditions(xpv1.Deleting())
	default:
		cr.Status.SetConditions(xpv1.Unavailable())
	}

	return managed.ExternalObservation{
		ResourceExists:    true,
		ResourceUpToDate:  true,
		ConnectionDetails: getConnectionDetails("", "", &v1alpha1.Endpoint{Address: attr.ConnectionDomain, Port: strconv.FormatInt(attr.Port, 10)}),
	}, nil
}

// Henry: No need to support for public access
// We don't need to call this method as the port and address are automatically configured now
// func (e *external) createConnectionIfNeeded(cr *v1alpha1.RedisInstance) (string, string, error) {

// 	if cr.Spec.ForProvider.PubliclyAccessible {
// 		return e.createPublicConnectionIfNeeded(cr)
// 	}
// 	return e.createPrivateConnectionIfNeeded(cr)
// }

// func (e *external) createPrivateConnectionIfNeeded(cr *v1alpha1.RedisInstance) (string, string, error) {
// 	domain := cr.Status.AtProvider.DBInstanceID + ".redis.rds.aliyuncs.com"
// 	if cr.Spec.ForProvider.Port == 0 {
// 		return domain, defaultRedisPort, nil
// 	}
// 	port := strconv.Itoa(cr.Spec.ForProvider.Port)
// 	if cr.Status.AtProvider.ConnectionReady {
// 		return domain, port, nil
// 	}
// 	connectionDomain, err := e.client.ModifyDBInstanceConnectionString(cr.Status.AtProvider.DBInstanceID, cr.Spec.ForProvider.Port)
// 	if err != nil {
// 		// The previous request might fail due to timeout. That's fine we will eventually reconcile it.
// 		var sdkerr sdkerror.Error
// 		if errors.As(err, &sdkerr) {
// 			if sdkerr.ErrorCode() == errDuplicateConnectionPort {
// 				cr.Status.AtProvider.ConnectionReady = true
// 				return domain, port, nil
// 			}
// 		}
// 		return "", "", err
// 	}

// 	cr.Status.AtProvider.ConnectionReady = true
// 	return connectionDomain, port, nil
// }

// func (e *external) createPublicConnectionIfNeeded(cr *v1alpha1.RedisInstance) (string, string, error) {
// 	domain := cr.Status.AtProvider.DBInstanceID + redis.PubilConnectionDomain
// 	if cr.Status.AtProvider.ConnectionReady {
// 		return domain, "", nil
// 	}
// 	port := defaultRedisPort
// 	if cr.Spec.ForProvider.InstancePort != 0 {
// 		port = strconv.Itoa(cr.Spec.ForProvider.InstancePort)
// 	}
// 	_, err := e.client.AllocateInstancePublicConnection(cr.Status.AtProvider.DBInstanceID, cr.Spec.ForProvider.InstancePort)
// 	if err != nil {
// 		// The previous request might fail due to timeout. That's fine we will eventually reconcile it.
// 		var sdkerr sdkerror.Error
// 		if errors.As(err, &sdkerr) {
// 			if sdkerr.ErrorCode() == errDuplicateConnectionPort || sdkerr.ErrorCode() == "NetTypeExists" {
// 				cr.Status.AtProvider.ConnectionReady = true
// 				return domain, port, nil
// 			}
// 		}
// 		return "", "", err
// 	}

// 	cr.Status.AtProvider.ConnectionReady = true
// 	return domain, port, nil
// }

// func (e *external) createAccount(cr *v1alpha1.RedisInstance) (string, error) {
// 	if cr.Status.AtProvider.AccountReady {
// 		return "", nil
// 	}

// 	pw, err := password.Generate()
// 	if err != nil {
// 		return "", err
// 	}

// 	instanceId, err := getInstanceId(cr)
// 	if err != nil {
// 		return "", err
// 	}

// 	username := getUsername(cr)

// 	// Use the instance name as the default user
// 	err = e.client.CreateAccount(instanceId, username, pw)
// 	if err != nil {
// 		// The previous request might fail due to timeout. That's fine we will eventually reconcile it.
// 		var sdkerr sdkerror.Error
// 		if errors.As(err, &sdkerr) {
// 			if sdkerr.ErrorCode() == errAccountNameDuplicate {
// 				cr.Status.AtProvider.AccountReady = true
// 				return "", nil
// 			}
// 		}
// 		return "", err
// 	}

// 	cr.Status.AtProvider.AccountReady = true
// 	return pw, nil
// }

func (e *external) Create(ctx context.Context, mg resource.Managed) (managed.ExternalCreation, error) {
	cr, ok := mg.(*v1alpha1.RedisInstance)
	if !ok {
		return managed.ExternalCreation{}, errors.New(errNotInstance)
	}

	cr.Status.SetConditions(xpv1.Creating())
	if err := e.kubeClient.Status().Update(ctx, cr); err != nil {
		return managed.ExternalCreation{}, errors.Wrap(err, errStatusUpdate)
	}

	resp, err := e.redisClient.CreateInstance(cr.GetObjectMeta().GetName(), &cr.Spec.ForProvider)
	if err != nil {
		return managed.ExternalCreation{}, errors.Wrap(err, errCreateFailed)
	}

	meta.SetExternalName(cr, resp.Id)

	// Any connection details emitted in ExternalClient are cumulative.
	return managed.ExternalCreation{ConnectionDetails: getConnectionDetails(resp.Id, resp.Password, resp.Endpoint)}, nil
}

func (e *external) Update(ctx context.Context, mg resource.Managed) (managed.ExternalUpdate, error) {
	cr, ok := mg.(*v1alpha1.RedisInstance)
	if !ok {
		return managed.ExternalUpdate{}, errors.New(errNotInstance)
	}

	cr.Status.SetConditions(xpv1.Creating())

	description := cr.Spec.ForProvider
	modifyReq := &redis.ModifyRedisInstanceRequest{
		InstanceClass: description.InstanceClass,
	}
	err := e.redisClient.UpdateInstance(meta.GetExternalName(cr), modifyReq)

	return managed.ExternalUpdate{}, err
}

func (e *external) Delete(ctx context.Context, mg resource.Managed) error {
	cr, ok := mg.(*v1alpha1.RedisInstance)
	if !ok {
		return errors.New(errNotInstance)
	}

	cr.SetConditions(xpv1.Deleting())

	err := e.redisClient.DeleteInstance(meta.GetExternalName(cr))
	return errors.Wrap(resource.Ignore(redis.IsErrorNotFound, err), errDeleteFailed)
}

func getConnectionDetails(id string, password string, endpoint *v1alpha1.Endpoint) managed.ConnectionDetails {
	cd := managed.ConnectionDetails{}

	// By default, a master user will be created with the instanceId
	username := id

	if username != "" {
		cd[xpv1.ResourceCredentialsSecretUserKey] = []byte(username)
	}

	if password != "" {
		cd[xpv1.ResourceCredentialsSecretPasswordKey] = []byte(password)
	}

	if endpoint != nil {
		if endpoint.Address != "" {
			cd[xpv1.ResourceCredentialsSecretEndpointKey] = []byte(endpoint.Address)
		}
		if endpoint.Port != "" {
			cd[xpv1.ResourceCredentialsSecretPortKey] = []byte(endpoint.Port)
		}
	}

	return cd
}
