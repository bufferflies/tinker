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
			backCmd:    "echo \"mkdir -p /var/lib/tikv/5.2.back;cd /var/lib/tikv;/bin/cp -rf \\`ls -A | grep -vE \"back|space_placeholder_file\"\\` /var/lib/tikv/5.2.back -v\" > /var/lib/tikv/back_5.2.sh;sh /var/lib/tikv/back_5.2.sh",
			restoreCmd: "echo \"cd /var/lib/tikv;rm -rf \\`ls -A | grep -vE \"back|space_placeholder_file\" \\` /var/lib/tikv -v;/bin/cp -rf /var/lib/tikv/5.2.back/* /var/lib/tikv -v\" > /var/lib/tikv/restore_5.2.sh;sh /var/lib/tikv/restore_5.2.sh",
		},
		{
			co:         PD,
			backCmd:    "echo \"mkdir -p /var/lib/pd/5.2.back;cd /var/lib/pd;/bin/cp -rf \\`ls -A | grep -vE \"back|space_placeholder_file\"\\` /var/lib/pd/5.2.back -v\" > /var/lib/pd/back_5.2.sh;sh /var/lib/pd/back_5.2.sh",
			restoreCmd: "echo \"cd /var/lib/pd;rm -rf \\`ls -A | grep -vE \"back|space_placeholder_file\" \\` /var/lib/pd -v;/bin/cp -rf /var/lib/pd/5.2.back/* /var/lib/pd -v\" > /var/lib/pd/restore_5.2.sh;sh /var/lib/pd/restore_5.2.sh",
		},
	}
	version := "5.2"
	for _, ca := range testCases {
		cmd := ca.co.BackExecCmd(version)
		assert.Equal(t, ca.backCmd, cmd)
		cmd = ca.co.RestoreExecCmd(version)
		assert.Equal(t, ca.restoreCmd, cmd)
	}
}
