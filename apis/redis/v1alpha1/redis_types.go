/*


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

package v1alpha1

import (
	xpv1 "github.com/crossplane/crossplane-runtime/apis/common/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +kubebuilder:object:root=true

// RedisInstance is the Schema for the redisinstances API
// An RedisInstance is a managed resource that represents an Redis instance.
// +kubebuilder:printcolumn:name="READY",type="string",JSONPath=".status.conditions[?(@.type=='Ready')].status"
// +kubebuilder:printcolumn:name="SYNCED",type="string",JSONPath=".status.conditions[?(@.type=='Synced')].status"
// +kubebuilder:printcolumn:name="STATE",type="string",JSONPath=".status.atProvider.dbInstanceStatus"
// +kubebuilder:printcolumn:name="INSTANCE_TYPE",type="string",JSONPath=".spec.forProvider.instanceType"
// +kubebuilder:printcolumn:name="VERSION",type="string",JSONPath=".spec.forProvider.engineVersion"
// +kubebuilder:printcolumn:name="AGE",type="date",JSONPath=".metadata.creationTimestamp"
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Cluster,categories={crossplane,managed,alibaba}
type RedisInstance struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   RedisInstanceSpec   `json:"spec,omitempty"`
	Status RedisInstanceStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// RedisInstanceList contains a list of RedisInstance
type RedisInstanceList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []RedisInstance `json:"items"`
}

// RedisInstanceSpec defines the desired state of RedisInstance
type RedisInstanceSpec struct {
	xpv1.ResourceSpec `json:",inline"`
	ForProvider       RedisInstanceParameters `json:"forProvider"`
}

// Redis instance states.
const (
	// The instance is healthy and available
	RedisInstanceStateRunning = "Ready"
	// The instance is being created. The instance is inaccessible while it is being created.
	RedisInstanceStateCreating = "Creating"
	// The instance is being deleted.
	RedisInstanceStateDeleting = "Deleting"
)

// RedisInstanceStatus defines the observed state of RedisInstance
type RedisInstanceStatus struct {
	xpv1.ResourceStatus `json:",inline"`
	AtProvider          RedisInstanceObservation `json:"atProvider,omitempty"`
}

// A Tag is used to tag the Redis resources in AliCloud.
type Tag struct {
	// Key for the tag.
	Key string `json:"key"`

	// Value of the tag.
	// +optional
	Value string `json:"value,omitempty"`
}

// RedisInstanceParameters define the desired state of an Redis instance.
type RedisInstanceParameters struct {
	// The ID of the region where you want to create the instance.
	// +immutable
	// +optional
	RegionID string `json:"regionId,omitempty"`

	// The primary zone ID of the instance.
	// +optional
	ZoneID string `json:"zoneId,omitempty"`

	// The secondary zone ID of the instance.
	// The master node and replica node of the instance can be deployed in different zones and disaster recovery is implemented across zones.
	// +optional
	SecondaryZoneID string `json:"secondaryZoneId,omitempty"`

	// The ID of the virtual private cloud (VPC).
	// +immutable
	// +optional
	VpcID string `json:"vpcId,omitempty"`

	// VSwitchId is indicates VSwitch ID
	// +immutable
	// +optional
	VSwitchID string `json:"vSwitchId,omitempty"`

	// ChargeType is indicates payment type
	// ChargeType：PrePaid/PostPaid
	// +optional
	// +kubebuilder:default="PostPaid"
	ChargeType string `json:"chargeType"`

	// NetworkType is indicates service network type
	// NetworkType：CLASSIC/VPC
	// +optional
	// +immutable
	// +kubebuilder:default="VPC"
	NetworkType string `json:"networkType"`

	// Engine is the name of the database engine to be used for this instance.
	// Engine is a required field.
	// +immutable
	// +kubebuilder:validation:Enum=Redis
	InstanceType string `json:"instanceType"`

	// The instance type.
	// For example, redis.master.small.default indicates a Community Edition
	// standard master-replica instance that has 1 GB of memory.
	InstanceClass string `json:"instanceClass"`

	// Port is indicates the database service port
	// +optional
	Port int `json:"port,omitempty"`

	// EngineVersion indicates the database engine version.
	// +kubebuilder:validation:Enum="4.0";"5.0";"6.0";"7.0"
	EngineVersion string `json:"engineVersion,omitempty"`

	// The number of data shards.
	// This parameter is available only if you create a cluster instance that uses cloud disks.
	ShardCount int `json:"shardCount"`

	// The tags of the instance.
	// +optional
	Tag []Tag `json:"tag,omitempty"`

	// MasterUsername is the name for the master user.
	// Constraints:
	//    * Required for Redis.
	//    * Must be 1 to 16 letters or numbers.
	//    * First character must be a letter.
	//    * Cannot be a reserved word for the chosen database engine.
	// +immutable
	// +optional
	MasterUsername string `json:"masterUsername"`
}

// RedisInstanceObservation is the representation of the current state that is observed.
type RedisInstanceObservation struct {
	// DBInstanceStatus specifies the current state of this database.
	DBInstanceStatus string `json:"dbInstanceStatus,omitempty"`

	// DBInstanceID specifies the Redis instance ID.
	DBInstanceID string `json:"dbInstanceID"`

	// AccountReady specifies whether the initial user account (username + password) is ready
	AccountReady bool `json:"accountReady"`

	// ConnectionReady specifies whether the network connect is ready
	ConnectionReady bool `json:"connectionReady"`
}

// Endpoint is the redis endpoint
type Endpoint struct {
	// Address specifies the DNS address of the Redis instance.
	Address string `json:"address,omitempty"`

	// Port specifies the port that the database engine is listening on.
	Port string `json:"port,omitempty"`
}
