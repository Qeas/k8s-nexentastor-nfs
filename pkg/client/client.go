package client

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"github.com/golang/glog"
	"io"
	"io/ioutil"
	"net/http"
	"strings"
)

type Auth struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type RestClient struct {
	token   string
	Baseurl string
	client  *http.Client
	Auth    *Auth
}

func (rc *RestClient) rest_request(method string, url string, body interface{}) (map[string]interface{}, error) {
	var b io.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			glog.Fatal("Error encoding body: %v Error:%s", body, err)
		}
		b = bytes.NewBuffer(encoded)
	}
	real_url := []string{rc.Baseurl, url}
	real_url_string := strings.Join(real_url, "")
	fmt.Printf("(%v) to %v with %v\n", method, real_url_string, b)
	glog.Infof("(%v) to %v with %v", method, real_url_string, b)
	req, err := http.NewRequest(method, real_url_string, b)
	if b != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if rc.token != "" {
		req.Header.Add("Authorization", fmt.Sprintf("Bearer %s", rc.token))
	}
	if rc.client == nil {
		tr := &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		}
		rc.client = &http.Client{Transport: tr}
	}
	resp, err := rc.client.Do(req)
	if err != nil {
		return nil, err
	}
	var m interface{}
	bodyText, err := ioutil.ReadAll(resp.Body)
	err = json.Unmarshal(bodyText, &m)
	if err != nil {
		fmt.Printf("\nJson Error: %v\nResponse Body: %v", err, bodyText)
		return nil, err
	}
	if m != nil {
		return m.(map[string]interface{}), nil
	} else {
		return nil, nil
	}
}

func (rc *RestClient) with_authentication(method string, url string, body interface{}) (map[string]interface{}, error) {
	res, err := rc.rest_request(method, url, body)
	if message, ok := res["message"]; ok {
		if message == "Please login to continue" {
			if rc.Auth != nil {
				res, err = rc.rest_request("POST", "auth/login", rc.Auth)
				if err != nil {
					glog.Fatalf("Could not login to %v with user %s and provided password", rc.Baseurl, rc.Auth.Username)
				}
				if token, ok := res["token"]; ok {
					if rc.token, ok = token.(string); ok {
						res, err = rc.rest_request(method, url, body)
					} else {
						glog.Fatalf("Not able to extract token from rest response")
					}
				} else {
					glog.Fatalf("Not able to extract token from rest response")
				}
			} else {
				glog.Fatalf("No credentials provided for %v", rc.Baseurl)
			}
		}
	}
	if err != nil {
		glog.Infof("Error %v\n", err)
		fmt.Printf("Error %v\n", err)
	}
	glog.Infof("Got Rest response: %v\n", res)
	fmt.Printf("Got Rest response: %v\n", res)
	return res, err
}

func (rc *RestClient) Post(url string, body interface{}) (map[string]interface{}, error) {
	return rc.with_authentication("POST", url, body)
}

func (rc *RestClient) Get(url string) (map[string]interface{}, error) {
	return rc.with_authentication("GET", url, nil)
}

func (rc *RestClient) Delete(url string) (map[string]interface{}, error) {
	return rc.with_authentication("DELETE", url, nil)
}

func (rc *RestClient) Update(url string, body interface{}) (map[string]interface{}, error) {
	return rc.with_authentication("UPDATE", url, body)
}

type FileSystem struct {
	Path string `json:"path"`
}

type NFS struct {
	FileSystem string `json:"filesystem"`
}
