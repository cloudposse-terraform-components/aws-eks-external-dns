package test

import (
	"context"
	"testing"
	"fmt"
	"strings"
	"time"
	"github.com/cloudposse/test-helpers/pkg/atmos"
	"github.com/cloudposse/test-helpers/pkg/helm"
	awsHelper "github.com/cloudposse/test-helpers/pkg/aws"
	helper "github.com/cloudposse/test-helpers/pkg/atmos/component-helper"
	awsTerratest "github.com/gruntwork-io/terratest/modules/aws"
	"github.com/gruntwork-io/terratest/modules/random"

	"github.com/stretchr/testify/assert"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"k8s.io/client-go/dynamic"
)

type ComponentSuite struct {
	helper.TestSuite
}

func (s *ComponentSuite) TestBasic() {
	const component = "eks/external-dns/basic"
	const stack = "default-test"
	const awsRegion = "us-east-2"

	clusterOptions := s.GetAtmosOptions("eks/cluster", stack, nil)
	clusrerId := atmos.Output(s.T(), clusterOptions, "eks_cluster_id")
	cluster := awsHelper.GetEksCluster(s.T(), context.Background(), awsRegion, clusrerId)

	dnsDelegatedOptions := s.GetAtmosOptions("dns-delegated", stack, nil)
	delegatedDomainName := atmos.Output(s.T(), dnsDelegatedOptions, "default_domain_name")
	defaultDNSZoneId := atmos.Output(s.T(), dnsDelegatedOptions, "default_dns_zone_id")

	defer func() {
		if err := awsHelper.CleanDNSZoneID(s.T(), context.Background(), defaultDNSZoneId, awsRegion); err != nil {
			fmt.Printf("Error cleaning DNS zone %s: %v\n", defaultDNSZoneId, err)
		}
	}()

	randomID := strings.ToLower(random.UniqueId())

	namespace := fmt.Sprintf("external-dns-%s", randomID)
	dnsEndpointName := fmt.Sprintf("example-dns-record-%s", randomID)
	dnsRecordHostName := fmt.Sprintf("%s.%s", randomID, delegatedDomainName)

	inputs := map[string]interface{}{
		"kubernetes_namespace": namespace,
	}

	defer s.DestroyAtmosComponent(s.T(), component, stack, &inputs)
	options, _ := s.DeployAtmosComponent(s.T(), component, stack, &inputs)
	assert.NotNil(s.T(), options)

	metadataArray := []helm.Metadata{}

	atmos.OutputStruct(s.T(), options, "metadata", &metadataArray)

	metadata := metadataArray[0]

	assert.Equal(s.T(), metadata.AppVersion, "0.14.0")
	assert.Equal(s.T(), metadata.Chart, "external-dns")
	assert.NotNil(s.T(), metadata.FirstDeployed)
	assert.NotNil(s.T(), metadata.LastDeployed)
	assert.Equal(s.T(), metadata.Name, "external-dns")
	assert.Equal(s.T(), metadata.Namespace, namespace)
	assert.NotNil(s.T(), metadata.Values)
	assert.Equal(s.T(), metadata.Version, "6.33.0")


	config, err := awsHelper.NewK8SClientConfig(cluster)
	assert.NoError(s.T(), err)
	assert.NotNil(s.T(), config)

	dynamicClient, err := dynamic.NewForConfig(config)
	if err != nil {
		panic(fmt.Errorf("failed to create dynamic client: %v", err))
	}

	// Define the GroupVersionResource for the DNSEndpoint CRD
	dnsEndpointGVR := schema.GroupVersionResource{
		Group:    "externaldns.k8s.io",
		Version:  "v1alpha1",
		Resource: "dnsendpoints",
	}

	// Create the DNSEndpoint object as an unstructured resource
	dnsEndpoint := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "externaldns.k8s.io/v1alpha1",
			"kind":       "DNSEndpoint",
			"metadata": map[string]interface{}{
				"name":	dnsEndpointName,
				"namespace": namespace,
			},
			"spec": map[string]interface{}{
				"endpoints": []interface{}{
					map[string]interface{}{
						"dnsName": dnsRecordHostName,
						"recordTTL":  300,
						"recordType": "A",
						"targets": []interface{}{
							"127.0.0.1",
						},
					},
				},
			},
		},
	}

	// Create the DNSEndpoint resource in the "default" namespace
	_, err = dynamicClient.Resource(dnsEndpointGVR).Namespace(namespace).Create(context.Background(), dnsEndpoint, metav1.CreateOptions{})
	assert.NoError(s.T(), err)

	defer func() {
		if err := dynamicClient.Resource(dnsEndpointGVR).Namespace(namespace).Delete(context.Background(), dnsEndpointName, metav1.DeleteOptions{}); err != nil {
			fmt.Printf("Error deleting external dns %s: %v\n", dnsEndpointName, err)
		}
	}()

	// Wait for the DNS record to be created
	// We can't use informers as DNS CRD status is not updated when the record is created
	time.Sleep(2 * time.Minute)

	delegatedNSRecord := awsTerratest.GetRoute53Record(s.T(), defaultDNSZoneId, dnsRecordHostName, "A", awsRegion)
	assert.Equal(s.T(), fmt.Sprintf("%s.", dnsRecordHostName), *delegatedNSRecord.Name)

	s.DriftTest(component, stack, &inputs)
}

func (s *ComponentSuite) TestEnabledFlag() {
	const component = "eks/external-dns/disabled"
	const stack = "default-test"
	s.VerifyEnabledFlag(component, stack, nil)
}

func (s *ComponentSuite) SetupSuite() {
	s.TestSuite.InitConfig()
	s.TestSuite.Config.ComponentDestDir = "components/terraform/eks/external-dns"
	s.TestSuite.SetupSuite()
}

func TestRunSuite(t *testing.T) {
	suite := new(ComponentSuite)
	suite.AddDependency(t, "vpc", "default-test", nil)
	suite.AddDependency(t, "eks/cluster", "default-test", nil)

	subdomain := strings.ToLower(random.UniqueId())
	inputs := map[string]interface{}{
		"zone_config": []map[string]interface{}{
			{
				"subdomain": subdomain,
				"zone_name": "components.cptest.test-automation.app",
			},
		},
	}
	suite.AddDependency(t, "dns-delegated", "default-test", &inputs)

	randomID := strings.ToLower(random.UniqueId())
	domainName := fmt.Sprintf("example-%s.net", randomID)
	inputsPrimaryDns := map[string]interface{}{
		"domain_names": []string{domainName},
		"record_config": []map[string]interface{}{
			{
				"root_zone": domainName,
				"name":      "",
				"type":      "A",
				"ttl":       60,
				"records":   []string{"127.0.0.1"},
			},
			{
				"root_zone": domainName,
				"name":      "www.",
				"type":      "CNAME",
				"ttl":       60,
				"records":   []string{domainName},
			},
			{
				"root_zone": domainName,
				"name":      "123456.",
				"type":      "CNAME",
				"ttl":       120,
				"records":   []string{domainName},
			},
		},
	}
	suite.AddDependency(t, "dns-primary", "default-test", &inputsPrimaryDns)

	suite.AddDependency(t, "eks/alb-controller", "default-test", nil)
	helper.Run(t, suite)
}
