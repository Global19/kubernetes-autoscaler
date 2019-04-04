// +build ignore

/*
Copyright 2017 The Kubernetes Authors.

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
	"encoding/json"
	"flag"
	"html/template"
	"io/ioutil"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"strings"

	"github.com/aws/aws-sdk-go/aws/endpoints"
	"k8s.io/klog"
)

type response struct {
	Products map[string]product `json:"products"`
}

type product struct {
	Attributes productAttributes `json:"attributes"`
}

type productAttributes struct {
	InstanceType string `json:"instanceType"`
	VCPU         string `json:"vcpu"`
	Memory       string `json:"memory"`
	GPU          string `json:"gpu"`
}

type instanceType struct {
	InstanceType string
	VCPU         int64
	Memory       int64
	GPU          int64
}

var packageTemplate = template.Must(template.New("").Parse(`/*
Copyright The Kubernetes Authors.

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

// This file was generated by go generate; DO NOT EDIT

package aws

type instanceType struct {
	InstanceType string
	VCPU         int64
	MemoryMb     int64
	GPU          int64
}

// InstanceTypes is a map of ec2 resources
var InstanceTypes = map[string]*instanceType{
{{- range .InstanceTypes }}
	"{{ .InstanceType }}": {
		InstanceType: "{{ .InstanceType }}",
		VCPU:         {{ .VCPU }},
		MemoryMb:     {{ .Memory }},
		GPU:          {{ .GPU }},
	},
{{- end }}
}
`))

func main() {
	flag.Parse()
	defer klog.Flush()

	instanceTypes := make(map[string]*instanceType)

	resolver := endpoints.DefaultResolver()
	partitions := resolver.(endpoints.EnumPartitions).Partitions()

	for _, p := range partitions {
		for _, r := range p.Regions() {
			url := "https://pricing.us-east-1.amazonaws.com/offers/v1.0/aws/AmazonEC2/current/" + r.ID() + "/index.json"
			klog.V(1).Infof("fetching %s\n", url)
			res, err := http.Get(url)
			if err != nil {
				klog.Warningf("Error fetching %s skipping...\n", url)
				continue
			}

			defer res.Body.Close()

			body, err := ioutil.ReadAll(res.Body)
			if err != nil {
				klog.Warningf("Error parsing %s skipping...\n", url)
				continue
			}

			var unmarshalled = response{}
			err = json.Unmarshal(body, &unmarshalled)
			if err != nil {
				klog.Warningf("Error unmarshaling %s skipping...\n", url)
				continue
			}

			for _, product := range unmarshalled.Products {
				attr := product.Attributes
				if attr.InstanceType != "" {
					instanceTypes[attr.InstanceType] = &instanceType{
						InstanceType: attr.InstanceType,
					}
					if attr.Memory != "" && attr.Memory != "NA" {
						instanceTypes[attr.InstanceType].Memory = parseMemory(attr.Memory)
					}
					if attr.VCPU != "" {
						instanceTypes[attr.InstanceType].VCPU = parseCPU(attr.VCPU)
					}
					if attr.GPU != "" {
						instanceTypes[attr.InstanceType].GPU = parseCPU(attr.GPU)
					}
				}
			}
		}
	}

	f, err := os.Create("ec2_instance_types.go")
	if err != nil {
		klog.Fatal(err)
	}

	defer f.Close()

	err = packageTemplate.Execute(f, struct {
		InstanceTypes map[string]*instanceType
	}{
		InstanceTypes: instanceTypes,
	})

	if err != nil {
		klog.Fatal(err)
	}
}

func parseMemory(memory string) int64 {
	reg, err := regexp.Compile("[^0-9\\.]+")
	if err != nil {
		klog.Fatal(err)
	}

	parsed := strings.TrimSpace(reg.ReplaceAllString(memory, ""))
	mem, err := strconv.ParseFloat(parsed, 64)
	if err != nil {
		klog.Fatal(err)
	}

	return int64(mem * float64(1024))
}

func parseCPU(cpu string) int64 {
	i, err := strconv.ParseInt(cpu, 10, 64)
	if err != nil {
		klog.Fatal(err)
	}
	return i
}
