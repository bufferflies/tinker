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
	"errors"
	"os"
	"path/filepath"
	"time"

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
	cmd.AddCommand(cloudCmd.stopCmd())
	cmd.AddCommand(cloudCmd.startCmd())
	cmd.AddCommand(cloudCmd.backCmd())
	cmd.AddCommand(cloudCmd.restoreCmd())
	cmd.AddCommand(cloudCmd.listCmd())
	cmd.AddCommand(cloudCmd.checkCmd())
	return cmd
}

func (c *CloudCommand) stopCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "stop",
		Short: "stop component",
		RunE:  c.stop,
	}
	return cmd
}

func (c *CloudCommand) startCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "start",
		Short: "start component",
		RunE:  c.start,
	}
	return cmd
}

func (c *CloudCommand) checkCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "check",
		Short: "check component",
		RunE:  c.check,
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

func (c *CloudCommand) listCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list",
		Short: "list version",
		RunE:  c.listE,
	}
	return cmd
}

func (c *CloudCommand) listE(cmd *cobra.Command, _ []string) error {
	rst, err := c.list(cmd, nil)
	if err != nil {
		return err
	}
	cmd.Printf("version list:%v\n", rst)
	return nil
}

func (c *CloudCommand) list(_ *cobra.Command, _ []string) (map[string][]string, error) {
	co := data.NewCloudOperator(c.namespace, c.config, context.Background())
	if co == nil {
		return nil, errors.New("init k8s client failed")
	}
	return co.List()
}

func (c *CloudCommand) stop(cmd *cobra.Command, _ []string) error {
	co := data.NewCloudOperator(c.namespace, c.config, context.Background())
	if co == nil {
		cmd.Println("init k8s client failed \n")
		return nil
	}
	if err := co.Stop(); err != nil {
		cmd.Printf("stop cloud operator failed:%v \n", err)
		return nil
	}
	return nil
}

func (c *CloudCommand) start(cmd *cobra.Command, _ []string) error {
	co := data.NewCloudOperator(c.namespace, c.config, context.Background())
	if co == nil {
		cmd.Println("init k8s client failed")
		return nil
	}
	if err := co.Start(); err != nil {
		cmd.Printf("stop cloud operator failed:%v \n", err)
		return err
	}
	time.Sleep(time.Second * 20)
	for i := 0; i < 5; i++ {
		if err := c.check(cmd, nil); err == nil {
			return nil
		}
		cmd.Println("waiting for pods start")
		time.Sleep(time.Second * 10)
	}
	cmd.Println("pods check exceed timeout")
	return nil
}

func (c *CloudCommand) check(cmd *cobra.Command, _ []string) error {
	co := data.NewCloudOperator(c.namespace, c.config, context.Background())
	if co == nil {
		return errors.New("init k8s client failed")
	}
	if !co.Check() {
		return errors.New("check failed")
	}
	cmd.Printf("check success \n")
	return nil
}

func (c *CloudCommand) back(cmd *cobra.Command, _ []string) {
	ctx := context.Background()
	t := time.Now()
	cmd.Println("it will try to stop all component")
	if err := c.stop(cmd, nil); err != nil {
		cmd.Printf("stop cloud operator failed:%v", err)
		return
	}
	cmd.Printf("it has stopped component, costs:%f s \n", time.Since(t).Seconds())
	time.Sleep(time.Second * 20)
	cmd.Println("it will back data，it can not interrupt, please wait")
	co := data.NewCloudOperator(c.namespace, c.config, ctx)
	if co == nil {
		cmd.Println("init k8s client failed")
		return
	}
	if err := co.Back(c.version); err != nil {
		cmd.Printf("back to %s failed:%v", c.version, err)
		return
	}
	cmd.Printf("it restores component already, costs:%f s \n", time.Since(t).Seconds())
	if err := c.start(cmd, nil); err != nil {
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

	t := time.Now()
	cmd.Println("it will try to stop all component")
	if err := c.stop(cmd, nil); err != nil {
		cmd.Printf("stop cloud operator failed:%v \n", err)
		return
	}
	cmd.Printf("it has stopped component, costs:%f s \n", time.Since(t).Seconds())
	time.Sleep(time.Second * 20)
	cmd.Println("it will restore data，it can not interrupt, please wait")
	co := data.NewCloudOperator(c.namespace, c.config, ctx)
	if co == nil {
		cmd.Println("init k8s client failed")
		return
	}
	if err := co.Restore(c.version); err != nil {
		cmd.Printf("restore from %s failed:%v\n", c.version, err)
		return
	}
	cmd.Printf("it restores component already, costs:%f s \n", time.Since(t).Seconds())
	if err := c.start(cmd, nil); err != nil {
		cmd.Printf("pods start error:%v", err)
	}
	time.Sleep(time.Minute)
	cmd.Println("it finished all")
	return
}

func homeDir() string {
	if h := os.Getenv("HOME"); len(h) > 0 {
		return h
	}
	return os.Getenv("USERPROFILE")
}
