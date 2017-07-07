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
    "errors"
    "flag"
    "fmt"
    "os"
    "time"

    "github.com/golang/glog"
    "github.com/kubernetes-incubator/external-storage/lib/controller"
    metav1 "github.com/kubernetes/apimachinery/pkg/apis/meta/v1"
    "github.com/kubernetes/apimachinery/pkg/util/wait"
    "github.com/kubernetes/client-go/kubernetes"
    "github.com/kubernetes/client-go/pkg/api/v1"
    "github.com/kubernetes/client-go/rest"
    RestClient "github.com/qeas/k8s-nexentastor-nfs/pkg/client"
    "strconv"
    "strings"
    "syscall"
)

const (
    resyncPeriod              = 15 * time.Second
    provisionerName           = "nexenta.com/nexentastor-nfs"
    exponentialBackOffOnError = false
    failedRetryThreshold      = 5
    leasePeriod               = controller.DefaultLeaseDuration
    retryPeriod               = controller.DefaultRetryPeriod
    renewDeadline             = controller.DefaultRenewDeadline
    termLimit                 = controller.DefaultTermLimit
)

type Auth struct {
    Username string `json:"username"`
    Password string `json:"password"`
}

type NexentaStorProvisioner struct {
    // Identity of this NexentaStorProvisioner, set to node's name. Used to identify
    // "this" provisioner's PVs.
    identity string
    hostname string
    port     int
    pool     string
    auth     Auth
}

func (p *NexentaStorProvisioner) Request(method, endpoint string, data map[string]interface{}) (body []byte, err error) {
    log.Debug("Issue request to Nexenta, endpoint: ", endpoint, " data: ", data, " method: ", method)
    if p.Endpoint == "" {
        log.Error("Endpoint is not set, unable to issue requests")
        err = errors.New("Unable to issue json-rpc requests without specifying Endpoint")
        return nil, err
    }
    datajson, err := json.Marshal(data)
    if (err != nil) {
        log.Error(err)
    }
    tr := &http.Transport{
        TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
    }
    client := &http.Client{Transport: tr}
    url := p.Endpoint + endpoint
    req, err := http.NewRequest(method, url, nil)
    if len(data) != 0 {
        req, err = http.NewRequest(method, url, strings.NewReader(string(datajson)))
    }
    req.Header.Set("Content-Type", "application/json")
    resp, err := client.Do(req)
    if resp.StatusCode == 401 || resp.StatusCode == 403 {
        log.Debug("No auth: ", resp.StatusCode)
        auth, err := p.https_auth()
        if err != nil {
            log.Error("Error while trying to https login: %s", err)
            return nil, err
        }
        req, err = http.NewRequest(method, url, nil)
        if len(data) != 0 {
            req, err = http.NewRequest(method, url, strings.NewReader(string(datajson)))
        }
        req.Header.Set("Content-Type", "application/json")
        req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", auth))
        resp, err = client.Do(req)
        log.Debug("With auth: ", resp.StatusCode)
    }

    if err != nil {
        log.Error("Error while handling request %s", err)
        return nil, err
    }
    p.checkError(resp)
    defer resp.Body.Close()
    body, err = ioutil.ReadAll(resp.Body)
    if (err != nil) {
        log.Error(err)
    }
    if (resp.StatusCode == 202) {
        body, err = p.resend202(body)
    }
    return body, err
}


func NewNexentaStorProvisioner() controller.Provisioner {
    nodeName := os.Getenv("NODE_NAME")
    if nodeName == "" {
        glog.Fatal("env variable NODE_NAME must be set so that this provisioner can identify itself")
    }
    hostname := os.Getenv("NEXENTA_HOSTNAME")
    if hostname == "" {
        glog.Fatal("env variable NEXENTA_HOSTNAME is required")
    }
    port := os.Getenv("NEXENTA_HOSTPORT")
    if port == "" {
        glog.Fatal("env variable NEXENTA_HOSTPORT is required")
    }
    pool := os.Getenv("NEXENTA_HOSTPOOL")
    if pool == "" {
        glog.Fatal("env variable NEXENTA_POOL is required")
    }
    username := os.Getenv("NEXENTA_USERNAME")
    if username == "" {
        glog.Fatal("env variable NEXENTA_USERNAME is required")
    }
    // HACK this should go into a secert!
    password := os.Getenv("NEXENTA_PASSWORD")
    if password == "" {
        glog.Fatal("env variable NEXENTA_PASSWORD is required")
    }
    auth := RestClient.Auth{Username: username, Password: password}
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
func (p *NexentaStorProvisioner) Provision(options controller.VolumeOptions) (*v1.PersistentVolume, error) {
    base_url := fmt.Sprintf("https://%s:%d/", p.hostname, p.port)
    rest_client := RestClient.RestClient{Auth: &p.auth, Baseurl: base_url}
    new_path := fmt.Sprintf("%v/%v", p.pool, options.PVName)
    new_network_path := fmt.Sprintf("/%v", new_path)
    var new_file_system FileSystem
    if storage_request, ok := options.PVC.Spec.Resources.Requests[v1.ResourceName(v1.ResourceStorage)]; ok {
        if size, ok := storage_request.AsInt64(); ok {
            new_file_system = FileSystem{Path: new_path,
                QuotaSize: size}
        }
    } else {
        new_file_system = FileSystem{Path: new_path}
    }
    rest_client.Post("storage/filesystems", new_file_system)
    new_nfs := NFS{FileSystem: new_path, Anon: "root"}
    rest_client.Post("nas/nfs", new_nfs)
    pv := &v1.PersistentVolume{
        ObjectMeta: metav1.ObjectMeta{
            Name: options.PVName,
            Annotations: map[string]string{
                "nexentaStorProvisionerIdentity": p.identity,
            },
        },
        Spec: v1.PersistentVolumeSpec{
            PersistentVolumeReclaimPolicy: options.PersistentVolumeReclaimPolicy,
            AccessModes:                   options.PVC.Spec.AccessModes,
            Capacity: v1.ResourceList{
                v1.ResourceName(v1.ResourceStorage): options.PVC.Spec.Resources.Requests[v1.ResourceName(v1.ResourceStorage)],
            },
            PersistentVolumeSource: v1.PersistentVolumeSource{
                NFS: &v1.NFSVolumeSource{
                    Server:   p.hostname,
                    Path:     new_network_path,
                    ReadOnly: false,
                },
            },
        },
    }

    return pv, nil
}

// Delete removes the storage asset that was created by Provision represented
// by the given PV.
func (p *NexentaStorProvisioner) Delete(volume *v1.PersistentVolume) error {
    ann, ok := volume.Annotations["nexentaStorProvisionerIdentity"]
    if !ok {
        return errors.New("identity annotation not found on PV")
    }
    if ann != p.identity {
        return &controller.IgnoredError{"identity annotation on PV does not match ours"}
    }

    base_url := fmt.Sprintf("https://%s:%d/", p.hostname, p.port)
    rest_client := RestClient.RestClient{Auth: &p.auth, Baseurl: base_url}
    rest_client.Delete(fmt.Sprintf("storage/filesystems/%v",
        strings.Replace(volume.Spec.PersistentVolumeSource.NFS.Path[1:], "/", "%2F", -1)))

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
