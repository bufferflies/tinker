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
package ctl

import (
	"os"

	"github.com/bufferflies/tinker/ctl/command"

	"github.com/spf13/cobra"
)

func init() {
	cobra.EnablePrefixMatching = true
}

// GetRootCmd is exposed for integration tests. But it can be embedded into another suite, too.
func GetRootCmd() *cobra.Command {
	rootCmd := &cobra.Command{
		Use:   "regression",
		Short: "tools for regression test",
	}
	rootCmd.AddCommand(command.NewCloudCommand())
	rootCmd.SilenceErrors = true
	return rootCmd
}

// MainStart start main command
func MainStart(args []string) {
	rootCmd := GetRootCmd()

	rootCmd.SetArgs(args)
	rootCmd.ParseFlags(args)
	rootCmd.SetOutput(os.Stdout)

	if err := rootCmd.Execute(); err != nil {
		rootCmd.Println(err)
		os.Exit(1)
	}
}
