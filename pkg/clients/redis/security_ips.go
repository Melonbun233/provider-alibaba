package redis

import (
	"strings"

	aliredis "github.com/aliyun/alibaba-cloud-sdk-go/services/r-kvstore"
)

const DefaultModifyMode = "Cover"

func (c *client) ModifySecurityIps(id string, ips string) error {
	req := aliredis.CreateModifySecurityIpsRequest()

	req.InstanceId = id
	req.SecurityIps = ips
	req.ModifyMode = DefaultModifyMode

	_, err := c.redisCli.ModifySecurityIps(req)
	return CleanError(err)
}

// Check if the whitelist IPs (IPv4) in Redis parameters are different than what are actually configured
// Return true if there are differences
func SecurityIpsNeedUpdate(configuredIps string, parameterIps string) bool {
	if parameterIps == "" {
		return false
	}

	ips := strings.Split(configuredIps, ",")
	for _, ip := range ips {
		if !strings.Contains(parameterIps, ip) {
			return true
		}
	}

	return false
}
