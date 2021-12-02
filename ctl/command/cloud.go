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
package command

import (
	"context"
	"os"
	"path/filepath"
	"time"

	"go.uber.org/zap"

	"github.com/bufferflies/tinker/pkg/data"
	"github.com/spf13/cobra"
)

type CloudCommand struct {
	version   string
	namespace string
	config    string
}

var cloudCmd CloudCommand

func NewCloudCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "tc",
		Short: "data back or recovery for tidb controller",
	}
	config := filepath.Join(homeDir(), ".kube", "config")
	cmd.PersistentFlags().StringVarP(&cloudCmd.version, "version", "v", "5.2", "back or restore version")
	cmd.PersistentFlags().StringVarP(&cloudCmd.config, "kube-config", "c", config, "kube config file path")
	cmd.PersistentFlags().StringVarP(&cloudCmd.namespace, "namespace", "n", "", "kube namespace")
	cmd.AddCommand(cloudCmd.backCmd())
	cmd.AddCommand(cloudCmd.restoreCmd())
	cmd.AddCommand(cloudCmd.stopCmd())
	cmd.AddCommand(cloudCmd.startCmd())
	return cmd
}

func (c *CloudCommand) stopCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "stop",
		Short: "stop component",
		Run:   c.stop,
	}
	return cmd
}

func (c *CloudCommand) startCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "start",
		Short: "start component",
		Run:   c.start,
	}
	return cmd
}

func (c *CloudCommand) backCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "back",
		Short: "back data",
		Run:   c.back,
	}
	return cmd
}

func (c *CloudCommand) stop(cmd *cobra.Command, _ []string) {
	co := data.NewCloudOperator(c.namespace, c.config, context.Background())
	if co == nil {
		cmd.Println("init k8s client failed \n")
		return
	}
	if err := co.Stop(); err != nil {
		cmd.Printf("stop cloud operator failed:%v \n", err)
		return
	}
}

func (c *CloudCommand) start(cmd *cobra.Command, _ []string) {
	co := data.NewCloudOperator(c.namespace, c.config, context.Background())
	if co == nil {
		cmd.Println("init k8s client failed")
		return
	}
	if err := co.Start(); err != nil {
		cmd.Printf("stop cloud operator failed:%v \n", err)
		return
	}
}

func (c *CloudCommand) back(cmd *cobra.Command, _ []string) {
	ctx := context.Background()
	co := data.NewCloudOperator(c.namespace, c.config, ctx)
	if co == nil {
		cmd.Println("init k8s client failed")
		return
	}
	t := time.Now()
	cmd.Println("it will try to stop all component")
	if err := co.Stop(); err != nil {
		cmd.Printf("stop cloud operator failed:%v", err)
		return
	}
	cmd.Printf("it has stopped component, costs:%f s \n", time.Since(t).Seconds())
	cmd.Println("it will back data，it can not interrupt, please wait")
	if err := co.Back(c.version); err != nil {
		cmd.Printf("back to 5.2 failed", zap.Error(err))
		return
	}
	cmd.Printf("it restores component already, costs:%f s \n", time.Since(t).Seconds())
	if err := co.Start(); err != nil {
		cmd.Printf("pods start error:%v", err)
	}
	cmd.Println("it finished all")
	return
}

func (c *CloudCommand) restoreCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "restore",
		Short: "restore data",
		Run:   c.restore,
	}
	return cmd
}

func (c *CloudCommand) restore(cmd *cobra.Command, _ []string) {
	ctx := context.Background()
	co := data.NewCloudOperator(c.namespace, c.config, ctx)
	if co == nil {
		cmd.Println("init k8s client failed")
		return
	}
	t := time.Now()
	cmd.Println("it will try to stop all component")
	if err := co.Stop(); err != nil {
		cmd.Printf("stop cloud operator failed:%v \n", err)
		return
	}
	cmd.Printf("it has stopped component, costs:%f s \n", time.Since(t).Seconds())
	cmd.Println("it will restore data，it can not interrupt, please wait")
	if err := co.Restore(c.version); err != nil {
		cmd.Printf("restore from 5.2 failed", zap.Error(err))
		return
	}
	cmd.Printf("it restores component already, costs:%f s \n", time.Since(t).Seconds())
	if err := co.Start(); err != nil {
		cmd.Printf("pods start error:%v", err)
	}
	cmd.Println("it finished all")
	return
}

func homeDir() string {
	if h := os.Getenv("HOME"); len(h) > 0 {
		return h
	}
	return os.Getenv("USERPROFILE")
}
