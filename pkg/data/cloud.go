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
	"bytes"
	"context"
	"errors"
	"fmt"
	"io/ioutil"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/pingcap/log"
	"go.uber.org/zap"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

type component int

// Flags for component.
const (
	TiDB component = iota
	PD
	TiKV
)

var componentToName = map[component]string{
	TiDB: "tidb",
	PD:   "pd",
	TiKV: "tikv",
}

const (
	BaseDir    = "/var/lib/"
	CommandLen = 50
	MaxRetry   = 5
	// DebugLabel is the label for debug.
	DebugLabel = "runmode"
	DebugValue = "debug"
)

// String implements fmt.Stringer interface.
func (c component) String() string {
	return componentToName[c]
}

// BataDir returns the data directory of the component.
func (c component) BataDir() string {
	return BaseDir + c.String()
}

// BackExecCmd backups cmd to the component's data directory.
// The format of directory is: version.back (e.g. 5.1.back).
func (c component) BackExecCmd(version string) string {
	dir := c.BataDir()
	backDir := fmt.Sprintf("%s/%s.bat", dir, version)
	shFile := fmt.Sprintf("%s/back_%s.sh", dir, version)

	// normal cmd: cp -rf `ls -A |grep -vE "back|space_placeholder_file"` /usr/local/bin/tidb /var/lib/tidb/5.1.back
	// it should exclude other backup directory and space_placeholder_file to decrease directory size.
	steps := []string{
		fmt.Sprintf("rm -rf %s", backDir),
		fmt.Sprintf("mkdir -p %s", backDir),
		fmt.Sprintf("cd %s;/bin/cp -rf \\`ls -A | grep -vE 'bat|space_placeholder_file'\\` %s -v", dir, backDir),
	}
	cmd := strings.Join(steps, ";")
	return fmt.Sprintf("echo \"%s\" > %s;sh %s", cmd, shFile, shFile)
}

// RestoreExecCmd restores cmd from the component's data directory.
func (c component) RestoreExecCmd(version string) string {
	dir := BaseDir + c.String()
	shFile := fmt.Sprintf("%s/restore_%s.sh", dir, version)
	backDir := fmt.Sprintf("%s/%s.bat", dir, version)
	steps := []string{
		fmt.Sprintf("cd %s;rm -rf \\`ls -A | grep -vE 'bat|space_placeholder_file' \\` -v", dir),
		fmt.Sprintf("/bin/cp -rf %s/* %s -v", backDir, dir),
	}
	cmd := strings.Join(steps, ";")
	return fmt.Sprintf("echo \"%s\" > %s;sh %s", cmd, shFile, shFile)
}

// CloudOperator is the interface for cloud operator.
type CloudOperator struct {
	client    *kubernetes.Clientset
	config    *rest.Config
	namespace string
	ctx       context.Context
}

// NewCloudOperator creates a cloud operator.
func NewCloudOperator(namespace, conf string, ctx context.Context) *CloudOperator {
	// creates the in-cluster config
	config, err := clientcmd.BuildConfigFromFlags("", conf)
	if err != nil {
		panic(err.Error())
	}
	// creates the clientset
	client, err := kubernetes.NewForConfig(config)
	if err != nil {
		log.Error("k8s load config failed", zap.Error(err))
		return nil
	}
	return &CloudOperator{
		client,
		config,
		namespace,
		ctx,
	}
}

// List returns all the backup version of the component in one cluster.
func (c *CloudOperator) List() (map[string][]string, error) {
	// k: component, v: versions
	rst := make(map[string][]string)
	for _, cp := range []component{TiKV, PD} {
		options := metav1.ListOptions{
			LabelSelector: fmt.Sprintf("app.kubernetes.io/component=%s", cp.String()),
		}
		pods, err := c.client.CoreV1().Pods(c.namespace).List(c.ctx, options)
		if err != nil {
			return nil, err
		}
		commands := []string{
			"sh",
			"-c",
			fmt.Sprintf("ls %s|grep bat", cp.BataDir()),
		}
		for _, pod := range pods.Items {
			dirs, err := c.exec(pod.Name, cp.String(), commands)
			if err != nil {
				log.Error("exec failed", zap.String("pod-name", pod.Name), zap.Any("command", commands))
				return nil, err
			}
			var versions []string
			if _, ok := rst[pod.Name]; !ok {
				versions = make([]string, 0)
			} else {
				versions = rst[pod.Name]
			}
			for _, version := range strings.Split(dirs, "\r\n") {
				if len(version) > 0 {
					versions = append(versions, strings.TrimSuffix(version, ".back"))
				}
			}
			rst[pod.Name] = versions
		}
	}
	return rst, nil
}

// Start starts all the components.
func (c *CloudOperator) Start() error {
	pods, err := c.client.CoreV1().Pods(c.namespace).List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		log.Error("get pods error", zap.Error(err))
	}
	// it will annotate all pods of runmode=debug
	for _, pod := range pods.Items {
		// annotate will not nil
		newPod := pod.DeepCopy()
		ann := newPod.ObjectMeta.Annotations
		delete(ann, DebugLabel)
		_, err = c.client.CoreV1().Pods(c.namespace).Update(c.ctx, newPod, metav1.UpdateOptions{})
		if err != nil {
			log.Error("update pods annotation error", zap.Error(err))
			return err
		}
	}
	for _, name := range []component{PD, TiKV, TiDB} {
		err = c.delete(name)
		if err != nil {
			return err
		}
	}
	return nil
}

// Stop stops all the pods of the component and will enter debug mode.
func (c *CloudOperator) Stop() error {
	pods, err := c.client.CoreV1().Pods(c.namespace).List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		log.Error("list all pods failed", zap.Error(err))
		return err
	}
	// it will annotate all pods of runmode=debug
	for _, pod := range pods.Items {
		// annotate will not nil
		newPod := pod.DeepCopy()
		ann := newPod.ObjectMeta.Annotations
		if ann == nil {
			ann = make(map[string]string)
		}
		// if ann is nil, it will create a new map
		ann[DebugLabel] = DebugValue
		_, err := c.client.CoreV1().Pods(c.namespace).Update(c.ctx, newPod, metav1.UpdateOptions{})
		if err != nil {
			log.Error("update pods annotation failed", zap.Error(err))
			return err
		}

	}
	for _, cp := range []component{TiDB, TiKV, PD} {
		err = c.kill(cp)
		if err != nil {
			log.Error("kill component failed", zap.String("component", cp.String()), zap.Error(err))
			return err
		}
		log.Info("stop result", zap.String("pod name", cp.String()))
	}
	return nil
}
func (c *CloudOperator) Check() bool {
	for _, cp := range []component{TiKV, PD, TiDB} {
		if !c.checkStatus(cp, true) {
			log.Info("check failed", zap.String("component", cp.String()))
			return false
		}
	}
	return true
}

// Back backs up all the components.
func (c *CloudOperator) Back(version string) error {
	wg := &sync.WaitGroup{}
	for _, cp := range []component{TiKV, PD} {
		if !c.checkStatus(cp, false) {
			return errors.New("check failed")
		}
		options := metav1.ListOptions{
			LabelSelector: fmt.Sprintf("app.kubernetes.io/component=%s", cp.String()),
		}
		pods, err := c.client.CoreV1().Pods(c.namespace).List(c.ctx, options)
		if err != nil {
			log.Info("list pods failed", zap.Error(err))
			return err
		}
		commands := []string{
			"sh",
			"-c",
			cp.BackExecCmd(version),
		}

		for _, pod := range pods.Items {
			wg.Add(1)
			log.Info("backup cmd", zap.String("pod name", pod.Name), zap.Any("command", commands))
			go func(podName, comp string, commands []string) {
				defer wg.Done()
				log.Info("backup up start", zap.String("pod", podName))
				_, err := c.exec(podName, comp, commands)
				if err != nil {
					log.Error("exec failed", zap.String("pod-name", podName), zap.String("component", comp), zap.Error(err))
				} else {
					log.Info("backup finished", zap.String("pod-name", podName))
				}
			}(pod.Name, cp.String(), commands)
		}
	}
	wg.Wait()
	return nil
}

// Restore restores all the components from backup directory.
func (c *CloudOperator) Restore(version string) error {
	wg := &sync.WaitGroup{}
	for _, cp := range []component{TiKV, PD} {
		if !c.check(cp, version, false) {
			return errors.New("check failed")
		}
		options := metav1.ListOptions{
			LabelSelector: fmt.Sprintf("app.kubernetes.io/component=%s", cp.String()),
		}
		pods, err := c.client.CoreV1().Pods(c.namespace).List(c.ctx, options)
		if err != nil {
			return err
		}
		commands := []string{
			"sh",
			"-c",
			cp.RestoreExecCmd(version),
		}
		for _, pod := range pods.Items {
			wg.Add(1)
			log.Info("cmd debug", zap.String("cmd", commands[2]))
			go func(podName, componentName string, commands []string) {
				defer wg.Done()
				log.Info("restore start", zap.String("pod-name", podName))
				result, err := c.exec(podName, componentName, commands)
				if err != nil {
					log.Error("exec failed", zap.String("pod-name", podName), zap.Any("command", commands))
				} else {
					log.Info("restore finished", zap.String("pod-name", podName), zap.String("result log", result))
				}
			}(pod.Name, cp.String(), commands)
		}
	}
	wg.Wait()
	return nil
}

// exec: exec command in the pod.
// container: the container name to cover multi container in single pods.
func (c *CloudOperator) exec(podName string, container string, commands []string) (string, error) {
	stdout := new(bytes.Buffer)
	stderr := new(bytes.Buffer)
	for i := 0; i < MaxRetry; i++ {
		err := exec(podName, container, c.namespace, commands, c.config, stdout, stderr)
		if err != nil {
			log.Error("cloud exec failed", zap.Error(err))
			if info, err := ioutil.ReadAll(stdout); err == nil {
				log.Error("get error info from std out", zap.String("pod-name", podName), zap.String("error", string(info)), zap.Error(err))
			}
		} else {
			if info, err := ioutil.ReadAll(stdout); err == nil {
				return string(info), nil
			}
			return "", err
		}
		log.Warn("cloud exec failed, it will retry after one minute", zap.String("pod-name", podName), zap.Int("retry", i))
		time.Sleep(time.Minute)
	}
	return "", errors.New("exec failed")
}

func (c *CloudOperator) delete(name component) error {
	options := metav1.ListOptions{
		LabelSelector: fmt.Sprintf("app.kubernetes.io/component=%s", name.String()),
	}
	pods, err := c.client.CoreV1().Pods(c.namespace).List(c.ctx, options)
	if err != nil {
		return err
	}
	for _, pod := range pods.Items {
		if pod.Status.Phase == corev1.PodRunning {
			err = c.client.CoreV1().Pods(c.namespace).Delete(c.ctx, pod.Name, metav1.DeleteOptions{})
			if err != nil {
				return err
			}
		}
	}
	return nil
}

func (c *CloudOperator) kill(name component) error {
	options := metav1.ListOptions{
		LabelSelector: fmt.Sprintf("app.kubernetes.io/component=%s", name.String()),
	}
	pods, err := c.client.CoreV1().Pods(c.namespace).List(c.ctx, options)
	if err != nil {
		log.Error("err", zap.Error(err))
		return err
	}
	for _, pod := range pods.Items {
		if pod.Status.Phase == corev1.PodRunning {
			commands := []string{
				"sh",
				"-c",
				"kill 1",
			}
			_, err = c.exec(pod.Name, name.String(), commands)
			if err != nil {
				return err
			}
		}
	}
	return nil
}

func (c *CloudOperator) check(name component, version string, status bool) bool {
	if !c.checkStatus(name, status) {
		log.Info("check status failed", zap.String("component", name.String()))
	}
	if !c.checkVersion(version) {
		log.Info("check version failed", zap.String("component", name.String()))
	}
	return true
}

func (c *CloudOperator) checkStatus(name component, status bool) bool {
	options := metav1.ListOptions{
		LabelSelector: fmt.Sprintf("app.kubernetes.io/component=%s", name.String()),
	}
	pods, err := c.client.CoreV1().Pods(c.namespace).List(c.ctx, options)
	if err != nil {
		log.Error("list all pods error", zap.Error(err))
		return false
	}
	for _, pod := range pods.Items {
		if pod.Status.Phase == corev1.PodRunning {
			commands := []string{
				"sh",
				"-c",
				"ps|awk '{print NF}'",
			}
			if name == TiKV {
				commands[2] = "ps -Cp 1|awk '{print NF}'"
			}
			result, err := c.exec(pod.Name, name.String(), commands)
			if err != nil {
				log.Error("exec failed", zap.Error(err), zap.Any("command", commands))
				return false
			}
			count, _ := strconv.Atoi(strings.Split(result, "\r\n")[1])

			if status != (count > 8) {
				log.Error("status check failed", zap.String("component", pod.Name), zap.Bool("status", status), zap.Int("count", count))
				return false
			}
		}
	}
	return true
}

func (c *CloudOperator) checkVersion(version string) bool {
	versions, err := c.List()
	if err != nil {
		log.Error("list version error", zap.Error(err))
	}
	for name, versions := range versions {
		exist := false
		for _, v := range versions {
			if v == version {
				exist = true
				break
			}
		}
		if !exist {
			log.Info("version not exist", zap.String("component", name), zap.String("version", version))
			return false
		}
	}
	return true
}
