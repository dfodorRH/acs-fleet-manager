/*
 * Red Hat Advanced Cluster Security Service Fleet Manager
 *
 * Red Hat Advanced Cluster Security (RHACS) Service Fleet Manager is a Rest API to manage instances of ACS components.
 *
 * API version: 1.2.0
 * Generated by: OpenAPI Generator (https://openapi-generator.tech)
 */

package public

// CloudProviderList struct for CloudProviderList
type CloudProviderList struct {
	Kind  string          `json:"kind"`
	Page  int32           `json:"page"`
	Size  int32           `json:"size"`
	Items []CloudProvider `json:"items"`
}
