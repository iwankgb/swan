// Copyright (c) 2017 Intel Corporation
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package testhelpers

import (
	"encoding/json"
	"fmt"
	"time"

	"io/ioutil"
	"net/http"

	"github.com/intelsdi-x/swan/pkg/executor"
	cluster "github.com/intelsdi-x/swan/pkg/kubernetes"
	"k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/pkg/api"
	clientv1 "k8s.io/client-go/pkg/api/v1"
	"k8s.io/client-go/rest"
)

// KubeClient is a helper struct to communicate with K8s API. It stores
// Kubernetes client and extends it to additional functionality needed
// by integration tests.
type KubeClient struct {
	Clientset *kubernetes.Clientset
	namespace string
}

// NewKubeClient creates KubeClient object based on given KubernetesConfig
// structure. It returns error if given configuration is invalid.
func NewKubeClient(kubernetesConfig executor.KubernetesConfig) (*KubeClient, error) {
	kubectlConfig := &rest.Config{
		Host: kubernetesConfig.Address,
	}

	cli, err := kubernetes.NewForConfig(kubectlConfig)
	if err != nil {
		return nil, err
	}
	return &KubeClient{
		Clientset: cli,
		namespace: kubernetesConfig.Namespace,
	}, nil
}

// WaitForCluster is waiting for at least one node in K8s cluster is ready.
func (k *KubeClient) WaitForCluster(timeout time.Duration) error {
	readyNodesFilterFunc := func() bool {
		nodes, err := k.getReadyNodes()
		if err != nil {
			return false
		}
		return len(nodes) > 0
	}
	readyAPIServer := func() bool {
		resp, err := http.Get(fmt.Sprintf("http://%s:8080/healthz", cluster.KubernetesMasterFlag.Value()))
		if err != nil {
			return false
		}
		if resp.StatusCode != 200 {
			return false
		}
		body, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			return false
		}
		if string(body) != "ok" {
			return false
		}
		return true
	}
	return k.kubectlWait(func() bool { return readyNodesFilterFunc() && readyAPIServer() }, timeout)
}

// WaitForPod is waiting for all pods are up and running.
func (k *KubeClient) WaitForPod(timeout time.Duration) error {
	runningPodsFilterFunc := func() bool {
		runningPods, notRunningPods, err := k.GetPods()
		if err != nil {
			return false
		}
		return len(notRunningPods) == 0 && len(runningPods) > 0
	}
	return k.kubectlWait(runningPodsFilterFunc, timeout)
}

// KubectlWait run K8s request and check results for expected string in a loop every second, unless it expected substring is found or timeout expires.
func (k *KubeClient) kubectlWait(filterFunction func() bool, timeout time.Duration) error {
	requstedTimeout := time.After(timeout)
	for {
		if filterFunction() {
			return nil
		}
		select {
		case <-requstedTimeout:
			return fmt.Errorf("timeout(%s) on K8s call", timeout.String())
		default:
		}
		time.Sleep(1 * time.Second)
	}
}

// GetPods gathers running and not running pods from K8s cluster.
func (k *KubeClient) GetPods() ([]*clientv1.Pod, []*clientv1.Pod, error) {
	pods, err := k.Clientset.Pods(k.namespace).List(v1.ListOptions{})
	if err != nil {
		return nil, nil, err
	}
	var runningPods []*clientv1.Pod
	var notRunningPods []*clientv1.Pod

	for _, pod := range pods.Items {
		switch pod.Status.Phase {
		case clientv1.PodRunning:
			runningPods = append(runningPods, &pod)
		case clientv1.PodPending:
		case clientv1.PodSucceeded:
		case clientv1.PodFailed:
			notRunningPods = append(notRunningPods, &pod)
		case clientv1.PodUnknown:
			return nil, nil, fmt.Errorf("at least one of pods is in Unknown state")
		}
	}

	return runningPods, notRunningPods, nil
}

func (k *KubeClient) getReadyNodes() ([]*clientv1.Node, error) {
	nodes, err := k.Clientset.Nodes().List(v1.ListOptions{})
	if err != nil {
		return nil, err
	}

	var readyNodes []*clientv1.Node
	for _, node := range nodes.Items {
		for _, condition := range node.Status.Conditions {
			if condition.Type == "Ready" && condition.Status != "True" {
				readyNodes = append(readyNodes, &node)
			}
		}
	}

	return readyNodes, nil
}

// DeletePod with given podName.
func (k *KubeClient) DeletePod(podName string) error {
	var oneSecond int64 = 1
	return k.Clientset.Pods(k.namespace).Delete(podName, &v1.DeleteOptions{GracePeriodSeconds: &oneSecond})
}

// Node assume just one node a return it. Note panics if unavailable (this is just test helper!).
func (k *KubeClient) node() *clientv1.Node {
	nodes, err := k.Clientset.Nodes().List(v1.ListOptions{})
	if err != nil {
		panic(err)
	}
	if len(nodes.Items) != 1 {
		panic("Expected signle nodes kubernetes cluster!")
	}
	return &nodes.Items[0]
}

// TaintNode with NoSchedule taint (can panic).
func (k *KubeClient) TaintNode() {
	newTaint := api.Taint{
		Key: "hponly", Value: "true", Effect: api.TaintEffectNoSchedule,
	}
	taintsInJSON, err := json.Marshal([]api.Taint{newTaint})
	if err != nil {
		panic(err)
	}

	node := k.node()
	k.updateTaints(node, taintsInJSON)
}

func (k *KubeClient) updateTaints(node *clientv1.Node, taints []byte) {
	patchSet := clientv1.Node{}
	patchSet.Annotations = map[string]string{api.TaintsAnnotationKey: string(taints)}
	patchSetInJSON, err := json.Marshal(patchSet)
	if err != nil {
		panic(err)
	}
	_, err = k.Clientset.Nodes().Patch(node.Name, types.MergePatchType, patchSetInJSON)
	if err != nil {
		panic(err)
	}
}

// UntaintNode removes all tains for given node (can panic on failure).
func (k *KubeClient) UntaintNode() {
	taintsInJSON, err := json.Marshal([]api.Taint{})
	if err != nil {
		panic(err)
	}
	node := k.node()
	k.updateTaints(node, taintsInJSON)
}
