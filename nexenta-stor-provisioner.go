/*
Copyright 2016 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package main

import (
    "flag"
    "os"
    "time"

    "github.com/golang/glog"
    "github.com/kubernetes-incubator/external-storage/lib/controller"
    // "github.com/kubernetes-incubator/external-storage/vendor/k8s.io/client-go/pkg/api/v1"
    "k8s.io/client-go/pkg/api/v1"
    "k8s.io/apimachinery/pkg/util/wait"
    "k8s.io/client-go/kubernetes"
    "k8s.io/client-go/rest"
    "strconv"
    "syscall"
)
// import (
//     "errors"
//     "flag"
//     "fmt"
//     "os"
//     "time"

//     "github.com/golang/glog"
//     "github.com/kubernetes-incubator/external-storage/lib/controller"
//     metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
//     "k8s.io/apimachinery/pkg/util/wait"
//     "k8s.io/client-go/kubernetes"
//     "k8s.io/api/core/v1"
//     "k8s.io/client-go/rest"
//     RestClient "github.com/qeas/k8s-nexentastor-nfs/pkg/client"
//     "strconv"
//     "strings"
//     "syscall"
// )

const (
    resyncPeriod              = 15 * time.Second
    provisionerName           = "nexenta.com/nexenta-stor"
    exponentialBackOffOnError = false
    failedRetryThreshold      = 5
    leasePeriod               = controller.DefaultLeaseDuration
    retryPeriod               = controller.DefaultRetryPeriod
    renewDeadline             = controller.DefaultRenewDeadline
    termLimit                 = controller.DefaultTermLimit
)

type NexentaStorProvisioner struct {
    // Identity of this NexentaStorProvisioner, set to node's name. Used to identify
    // "this" provisioner's PVs.
    identity string
    hostname string
    port     int
    pool     string
    auth     Auth
}

type Auth struct {
    Username string `json:"username"`
    Password string `json:"password"`
}

func NewNexentaStorProvisioner() controller.Provisioner {
    nodeName := os.Getenv("NODE_NAME")
    if nodeName == "" {
        glog.Fatal("env variable NODE_NAME must be set so that this provisioner can identify itself")
    }
    hostname := os.Getenv("NEXENTA_HOSTNAME")
    if hostname == "" {
        glog.Fatal("env variable NEXENTA_HOSTNAME must be set to know whom to talk to")
    }
    port := os.Getenv("NEXENTA_HOSTPORT")
    if port == "" {
        glog.Fatal("env variable NEXENTA_HOSTPORT must be set to know whom to talk to")
    }
    pool := os.Getenv("NEXENTA_HOSTPOOL")
    if pool == "" {
        glog.Fatal("env variable NEXENTA_HOSTPOOL must be set to know whom to talk to")
    }
    username := os.Getenv("NEXENTA_USERNAME")
    if username == "" {
        glog.Fatal("env variable NEXENTA_USERNAME must be set to know whom to talk to")
    }
    // HACK this should go into a secert!
    password := os.Getenv("NEXENTA_PASSWORD")
    if password == "" {
        glog.Fatal("env variable NEXENTA_PASSWORD must be set to know whom to talk to")
    }
    auth := Auth{Username: username, Password: password}
    port_int, _ := strconv.Atoi(port)
    return &NexentaStorProvisioner{
        identity: nodeName,
        hostname: hostname,
        port:     port_int,
        pool:     pool,
        auth:     auth,
    }
}

var _ controller.Provisioner = &NexentaStorProvisioner{}

type FileSystem struct {
    Path      string `json:"path"`
    QuotaSize int64  `json:"quotaSize"`
}

type NFS struct {
    FileSystem string `json:"filesystem"`
    Anon       string `json:"anon"`
}

// Provision creates a storage asset and returns a PV object representing it.
func (p *NexentaStorProvisioner) Provision(options controller.VolumeOptions) (v *v1.PersistentVolume, err error) {
  
    return 
}

// Delete removes the storage asset that was created by Provision represented
// by the given PV.
func (p *NexentaStorProvisioner) Delete(volume *v1.PersistentVolume) error {

    return nil
}

func main() {
    syscall.Umask(0)

    flag.Parse()
    flag.Set("logtostderr", "true")

    // Create an InClusterConfig and use it to create a client for the controller
    // to use to communicate with Kubernetes
    config, err := rest.InClusterConfig()
    if err != nil {
        glog.Fatalf("Failed to create config: %v", err)
    }
    clientset, err := kubernetes.NewForConfig(config)
    if err != nil {
        glog.Fatalf("Failed to create client: %v", err)
    }

    // The controller needs to know what the server version is because out-of-tree
    // provisioners aren't officially supported until 1.5
    serverVersion, err := clientset.Discovery().ServerVersion()
    if err != nil {
        glog.Fatalf("Error getting server version: %v", err)
    }

    // Create the provisioner: it implements the Provisioner interface expected by
    // the controller
    nexentaStorProvisioner := NewNexentaStorProvisioner()

    // Start the provision controller which will dynamically provision nexentaStor
    // PVs
    pc := controller.NewProvisionController(clientset, resyncPeriod, provisionerName, nexentaStorProvisioner, serverVersion.GitVersion, exponentialBackOffOnError, failedRetryThreshold, leasePeriod, renewDeadline, retryPeriod, termLimit)
    pc.Run(wait.NeverStop)
}
