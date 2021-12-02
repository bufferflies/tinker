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
	"sync"

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

const base = "/var/lib/"
const CommandLen = 50

func (c component) String() string {
	return componentToName[c]
}

func (c component) Back(version string) string {
	dir := base + c.String()
	backDir := fmt.Sprintf("%s/%s.back", dir, version)
	cpFile := fmt.Sprintf("%scp.sh", base)

	return fmt.Sprintf("echo \"mkdir -p %s;cd %s;/bin/cp -rf \\`ls -A | grep -v back\\` %s -v\" > %s;sh %s;rm %s",
		backDir, dir, backDir, cpFile, cpFile, cpFile)
}

func (c component) Restore(version string) string {
	dir := base + c.String()
	backDir := fmt.Sprintf("%s/%s.back", dir, version)
	return fmt.Sprintf("/bin/cp -rf %s/* %s", backDir, dir)
}

const DebugLabel = "runmode"

type CloudOperator struct {
	client    *kubernetes.Clientset
	config    *rest.Config
	namespace string
	ctx       context.Context
}

func NewCloudOperator(namespace, conf string, ctx context.Context) *CloudOperator {
	// creates the in-cluster config
	config, err := clientcmd.BuildConfigFromFlags("", conf)
	if err != nil {
		panic(err.Error())
	}
	// creates the clientset
	client, err := kubernetes.NewForConfig(config)
	if err != nil {
		log.Error("err", zap.Error(err))
		return nil
	}
	return &CloudOperator{
		client,
		config,
		namespace,
		ctx,
	}
}

func (c CloudOperator) Stop() error {
	pods, err := c.client.CoreV1().Pods(c.namespace).List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		log.Error("err", zap.Error(err))
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
		ann[DebugLabel] = "debug"
		_, err = c.client.CoreV1().Pods(c.namespace).Update(c.ctx, newPod, metav1.UpdateOptions{})
		if err != nil {
			log.Error("err", zap.Error(err))
			return err
		}
	}
	for _, cp := range []component{TiDB, TiKV, PD} {
		err = c.kill(cp)
		if err != nil {
			log.Error("err", zap.Error(err))
			return err
		}
	}
	return nil
}

func (c CloudOperator) Back(version string) error {
	wg := &sync.WaitGroup{}
	for _, cp := range []component{TiKV, PD} {
		if err := c.preCheck(cp); err != nil {
			return err
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
			cp.Back(version),
		}

		for _, pod := range pods.Items {
			wg.Add(1)
			go func(podName, comp string, commands []string) {
				defer wg.Done()
				_, err = c.exec(podName, comp, commands)
				if err != nil {
					log.Error("exec failed", zap.String("pod-name", podName), zap.String("component", comp), zap.Error(err))
				}
				log.Info("back finished", zap.String("pod-name", podName))
			}(pod.Name, cp.String(), commands)
		}
	}
	wg.Wait()
	return nil
}

func (c CloudOperator) Restore(version string) error {
	wg := &sync.WaitGroup{}
	for _, cp := range []component{TiKV, PD} {
		if err := c.preCheck(cp); err != nil {
			return err
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
			cp.Restore(version),
		}
		for _, pod := range pods.Items {
			wg.Add(1)
			go func(podName, componentName string, commands []string) {
				defer wg.Done()
				_, err = c.exec(podName, componentName, commands)
				if err != nil {
					log.Error("exec failed", zap.String("pod-name", podName), zap.Any("command", commands))
				}
				log.Info("restore finished", zap.String("pod-name", podName))
			}(pod.Name, cp.String(), commands)
		}
	}
	wg.Wait()
	return nil
}

func (c CloudOperator) Start() error {
	pods, err := c.client.CoreV1().Pods(c.namespace).List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		log.Error("err", zap.Error(err))
	}
	// it will annotate all pods of runmode=debug
	for _, pod := range pods.Items {
		// annotate will not nil
		newPod := pod.DeepCopy()
		ann := newPod.ObjectMeta.Annotations
		delete(ann, DebugLabel)
		_, err = c.client.CoreV1().Pods(c.namespace).Update(c.ctx, newPod, metav1.UpdateOptions{})
		if err != nil {
			log.Error("err", zap.Error(err))
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

func (c CloudOperator) exec(podName string, container string, commands []string) (string, error) {
	stdout := new(bytes.Buffer)
	stderr := new(bytes.Buffer)
	err := exec(podName, container, c.namespace, commands, c.config, stdout, stderr)
	if err != nil {
		log.Error("err", zap.Error(err))
		if info, err := ioutil.ReadAll(stderr); err == nil {
			log.Error("get error info from std error", zap.String("error", string(info)))
		}
		if info, err := ioutil.ReadAll(stdout); err == nil {
			log.Error("get error info from std out", zap.String("error", string(info)))
		}
		return "", err
	}
	if info, err := ioutil.ReadAll(stdout); err == nil {
		return string(info), nil
	}
	return "", err
}

func (c CloudOperator) delete(name component) error {
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

func (c CloudOperator) kill(name component) error {
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

func (c CloudOperator) preCheck(name component) error {
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
				"ps 1 |sed -n '2p'|awk '{print $6}'",
			}
			result, err := c.exec(pod.Name, name.String(), commands)
			if err != nil {
				return err
			}
			if len(result) > CommandLen {
				return errors.New(fmt.Sprintf("pods does not enter debug mode, pod name:%s", pod.Name))
			}
		}
	}
	return nil
}
