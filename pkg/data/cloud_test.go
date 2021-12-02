// Copyright 2021 TiKV Project Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
package data

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRestoreAndBack(t *testing.T) {
	testCases := []struct {
		co         component
		backCmd    string
		restoreCmd string
	}{
		{
			co:         TiKV,
			backCmd:    "echo \"mkdir -p /var/lib/tikv/5.2.back;cd /var/lib/tikv;/bin/cp -rf \\`ls -A | grep -v back\\` /var/lib/tikv/5.2.back -v\" > /var/lib/cp.sh;sh /var/lib/cp.sh;rm /var/lib/cp.sh",
			restoreCmd: "/bin/cp -rf /var/lib/tikv/5.2.back/* /var/lib/tikv",
		},
		{
			co:         PD,
			backCmd:    "echo \"mkdir -p /var/lib/pd/5.2.back;cd /var/lib/pd;/bin/cp -rf \\`ls -A | grep -v back\\` /var/lib/pd/5.2.back -v\" > /var/lib/cp.sh;sh /var/lib/cp.sh;rm /var/lib/cp.sh",
			restoreCmd: "/bin/cp -rf /var/lib/pd/5.2.back/* /var/lib/pd",
		},
	}
	version := "5.2"
	for _, ca := range testCases {
		cmd := ca.co.Back(version)
		assert.Equal(t, ca.backCmd, cmd)
		cmd = ca.co.Restore(version)
		assert.Equal(t, ca.restoreCmd, cmd)
	}
}
