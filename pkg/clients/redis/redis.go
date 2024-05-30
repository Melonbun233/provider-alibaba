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
	"encoding/json"
	"strconv"

	"github.com/crossplane/crossplane-runtime/pkg/password"
	"github.com/pkg/errors"

	sdkerrors "github.com/aliyun/alibaba-cloud-sdk-go/sdk/errors"
	"github.com/aliyun/alibaba-cloud-sdk-go/sdk/requests"
	aliredis "github.com/aliyun/alibaba-cloud-sdk-go/services/r-kvstore"

	"github.com/crossplane-contrib/provider-alibaba/apis/redis/v1alpha1"
)

const (
	// HTTPSScheme indicates request scheme
	HTTPSScheme = "https"

	// Errors
	errInstanceNotFound       = "DBInstanceNotFound"
	errDescribeInstanceFailed = "cannot describe instance attributes"
	errGeneratePasswordFailed = "cannot generate a password"
)

// Same server error but without requestID
type CleanedServerError struct {
	HttpStatus int
	HostId     string
	Code       string
	Message    string
	Comment    string
}

// Client defines Redis client operations
type Client interface {
	DescribeInstance(id string) (*aliredis.DBInstanceAttribute, error)
	CreateInstance(name string, parameters *v1alpha1.RedisInstanceParameters) (*CreateInstanceResponse, error)
	DeleteInstance(id string) error
	// AllocateInstancePublicConnection(id string, port int) (string, error)
	// ModifyDBInstanceConnectionString(id string, port int) (string, error)
	UpdateInstance(id string, req *ModifyRedisInstanceRequest) error
}

// DBInstance defines the instance information
type CreateInstanceResponse struct {
	// Instance ID
	Id string

	// Default password for the admin user
	Password string

	// Endpoint specifies the connection endpoint.
	Endpoint *v1alpha1.Endpoint
}

// ModifyRedisInstanceRequest defines the request info to modify DB Instance
type ModifyRedisInstanceRequest struct {
	InstanceClass string
}

type client struct {
	redisCli *aliredis.Client
}

// NewClient creates new Redis RedisClient
func NewClient(ctx context.Context, accessKeyID, accessKeySecret, region string) (Client, error) {
	redisCli, err := aliredis.NewClientWithAccessKey(region, accessKeyID, accessKeySecret)
	if err != nil {
		return nil, CleanError(err)
	}
	c := &client{redisCli: redisCli}
	return c, nil
}

func (c *client) DescribeInstance(id string) (*aliredis.DBInstanceAttribute, error) {
	request := aliredis.CreateDescribeInstanceAttributeRequest()
	request.Scheme = HTTPSScheme

	request.InstanceId = id

	response, err := c.redisCli.DescribeInstanceAttribute(request)
	if err != nil {
		return nil, errors.Wrap(CleanError(err), errDescribeInstanceFailed)
	}
	if len(response.Instances.DBInstanceAttribute) == 0 {
		return nil, errors.New(errInstanceNotFound)
	}

	attr := response.Instances.DBInstanceAttribute[0]

	return &attr, nil
}

func (c *client) CreateInstance(externalName string, p *v1alpha1.RedisInstanceParameters) (*CreateInstanceResponse, error) {
	request := aliredis.CreateCreateInstanceRequest()

	// Seems regionID will be by default from the first part ZoneID
	// request.RegionID = p.RegionID
	request.Token = p.Token
	request.InstanceName = externalName
	request.InstanceClass = p.InstanceClass
	request.ZoneId = p.ZoneID
	request.ChargeType = p.ChargeType
	request.NodeType = p.NodeType
	request.NetworkType = p.NetworkType
	request.VpcId = p.VpcID
	request.VSwitchId = p.VSwitchID
	request.Period = p.Period
	request.BusinessInfo = p.BusinessInfo
	request.CouponNo = p.CouponNo
	request.SrcDBInstanceId = p.SrcDBInstanceId
	request.BackupId = p.BackupId
	request.InstanceType = p.InstanceType
	request.EngineVersion = p.EngineVersion
	request.PrivateIpAddress = p.PrivateIpAddress
	request.AutoUseCoupon = p.AutoUseCoupon
	request.AutoRenew = p.AutoRenew
	request.AutoRenewPeriod = p.AutoRenewPeriod
	request.ResourceGroupId = p.ResourceGroupId
	request.RestoreTime = p.RestoreTime
	request.DedicatedHostGroupId = p.DedicatedHostGroupId
	request.GlobalInstanceId = p.GlobalInstanceId
	request.GlobalInstance = requests.NewBoolean(p.GlobalInstance)
	request.SecondaryZoneId = p.SecondaryZoneID
	request.GlobalSecurityGroupIds = p.GlobalSecurityGroupIds
	request.Appendonly = p.Appendonly
	request.ConnectionStringPrefix = p.ConnectionStringPrefix
	request.ParamGroupId = p.ParamGroupId
	request.ClusterBackupId = p.ClusterBackupId

	if p.Port != nil {
		request.Port = strconv.Itoa(*p.Port)
	}

	if p.Capacity != nil {
		request.Capacity = requests.NewInteger(*p.Capacity)
	}

	if p.ShardCount != nil {
		request.ShardCount = requests.NewInteger(*p.ShardCount)
	}

	if p.ReadOnlyCount != nil {
		request.ReadOnlyCount = requests.NewInteger(*p.ReadOnlyCount)
	}

	// Password might be generated or provided by user
	var pw string
	var err error
	if p.Password == "" {
		pw, err = password.Generate()
		if err != nil {
			return nil, errors.Wrap(err, errGeneratePasswordFailed)
		}
	}
	request.Password = pw

	requestTags := make([]aliredis.CreateInstanceTag, len(p.Tag))
	for _, tag := range p.Tag {
		requestTags = append(requestTags, aliredis.CreateInstanceTag{Key: tag.Key, Value: tag.Value})
	}
	request.Tag = &requestTags

	resp, err := c.redisCli.CreateInstance(request)
	if err != nil {
		return nil, CleanError(err)
	}

	return &CreateInstanceResponse{
		Id:       resp.InstanceId,
		Password: pw,
		Endpoint: &v1alpha1.Endpoint{
			Address: resp.ConnectionDomain,
			Port:    strconv.Itoa(resp.Port),
		},
	}, nil
}

func (c *client) DeleteInstance(id string) error {
	request := aliredis.CreateDeleteInstanceRequest()
	request.Scheme = HTTPSScheme

	request.InstanceId = id

	_, err := c.redisCli.DeleteInstance(request)
	return CleanError(err)
}

// GenerateObservation is used to produce v1alpha1.RedisInstanceObservation from
// redis.DBInstance.
func GenerateObservation(attr *aliredis.DBInstanceAttribute) v1alpha1.RedisInstanceObservation {
	return v1alpha1.RedisInstanceObservation{
		DBInstanceStatus: attr.InstanceStatus,
		Endpoint: v1alpha1.Endpoint{
			Address: attr.ConnectionDomain,
			Port:    strconv.FormatInt(attr.Port, 10),
		},
	}
}

// IsErrorNotFound helper function to test for ErrCodeDBInstanceNotFoundFault error
func IsErrorNotFound(err error) bool {
	if err == nil {
		return false
	}
	// If the instance is already removed, errors should be ignored when deleting it.
	var srverr *sdkerrors.ServerError
	if !errors.As(err, &srverr) {
		return false || errors.Is(err, errors.New(errInstanceNotFound))
	}

	return srverr.ErrorCode() == "InvalidInstanceId.NotFound"
}

// func (c *client) AllocateInstancePublicConnection(id string, port int) (string, error) {
// 	request := aliredis.CreateAllocateInstancePublicConnectionRequest()
// 	request.Scheme = HTTPSScheme
// 	request.InstanceId = id
// 	request.ConnectionStringPrefix = id + PubilConnectionDomain
// 	request.Port = strconv.Itoa(port)
// 	request.ReadTimeout = DefaultReadTime
// 	_, err := c.redisCli.AllocateInstancePublicConnection(request)
// 	if err != nil {
// 		return "", CleanError(err)
// 	}
// 	return request.ConnectionStringPrefix, err
// }

// func (c *client) ModifyDBInstanceConnectionString(id string, port int) (string, error) {
// 	request := aliredis.CreateModifyDBInstanceConnectionStringRequest()
// 	request.Scheme = HTTPSScheme
// 	request.DBInstanceId = id
// 	request.CurrentConnectionString = id + PubilConnectionDomain
// 	request.Port = strconv.Itoa(port)
// 	request.ReadTimeout = DefaultReadTime
// 	_, err := c.redisCli.ModifyDBInstanceConnectionString(request)
// 	if err != nil {
// 		return "", CleanError(err)
// 	}
// 	return request.CurrentConnectionString, err
// }

func (c *client) UpdateInstance(id string, req *ModifyRedisInstanceRequest) error {
	if req.InstanceClass == "" {
		return errors.New("modify instances spec is require")
	}
	if req.InstanceClass != "" {
		return c.modifyInstanceSpec(id, req)
	}
	return nil
}

func (c *client) modifyInstanceSpec(id string, req *ModifyRedisInstanceRequest) error {
	request := aliredis.CreateModifyInstanceSpecRequest()
	request.Scheme = HTTPSScheme
	request.InstanceId = id
	request.InstanceClass = req.InstanceClass
	_, err := c.redisCli.ModifyInstanceSpec(request)
	return CleanError(err)
}

// 2024-05-14: Henry
// Try to remove requestID from AliCloud SDK errors
// Returning error with requestID will cause Crossplane reconciler to treat the errors
// as a sequence of unique errors and insert all errors into the retry queue, which
// immediately boomed the AliCloud rate limit.
// See more details of a similar issue in AWS controller:
// https://github.com/crossplane-contrib/provider-aws/issues/69
func CleanError(err error) error {
	if err == nil {
		return err
	}

	if aliCloudErr, ok := err.(*sdkerrors.ServerError); ok {
		cleanedErr := CleanedServerError{
			HttpStatus: aliCloudErr.HttpStatus(),
			HostId:     aliCloudErr.HostId(),
			Code:       aliCloudErr.ErrorCode(),
			Message:    aliCloudErr.Message(),
			Comment:    aliCloudErr.Comment(),
		}
		strData, err := json.Marshal(cleanedErr)
		if err != nil {
			return errors.Wrap(err, "Failed to marshal cleaned error from AliCloud SDK Error.")
		}
		return sdkerrors.NewServerError(aliCloudErr.HttpStatus(), string(strData), aliCloudErr.Comment())
	}

	return err
}
