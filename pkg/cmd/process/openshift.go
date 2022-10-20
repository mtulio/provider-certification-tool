package process

import (
	"fmt"
	log "github.com/sirupsen/logrus"
	"os"
	"strings"
)

type OpenShiftSummary struct {
	// Infra CR
	infraPlatformType         string
	infraAPIServerURL         string
	infraAPIServerURLInternal string
	infraControlPlaneTopology string
	infraTopology             string
	infraName                 string

	// Plugin Results
	pluginResultK8sConformance *OPCTPluginSummary
	pluginResultOCPValidated   *OPCTPluginSummary

	versionOPpctSonobuoy     string
	versionOpctCli           string
	versionPluginConformance string

	// get from Sonobuoy metadata
	versionK8S string

	// CVO (results-partner/resources/cluster/config.openshift.io_v1_clusterversions.json)
	cvoStatusDesiredVersion   string
	cvoCondProgressing        string
	cvoCondProgressingMessage string

	// Cluster Operators (results-partner/resources/cluster/config.openshift.io_v1_clusteroperators.json)
	coCountAvailable   uint64
	coCountProgressing uint64
	coCountDegraded    uint64
}

func NewOpenShiftSummary() *OpenShiftSummary {
	return &OpenShiftSummary{}
}

func (os *OpenShiftSummary) setFromInfraCR(cr *OpenShiftCrInfrastructures) {
	os.infraPlatformType = cr.Items[0].Status.Platform
	os.infraAPIServerURL = cr.Items[0].Status.APIServerURL
	os.infraAPIServerURLInternal = cr.Items[0].Status.APIServerInternalURL
	os.infraControlPlaneTopology = cr.Items[0].Status.ControlPlaneTopology
	os.infraTopology = cr.Items[0].Status.InfrastructureTopology
	os.infraName = cr.Items[0].Status.InfrastructureName
}

func (os *OpenShiftSummary) setFromCvoCR(cr *OpenShiftCrCvo) {
	os.cvoStatusDesiredVersion = cr.Items[0].Status.Desired.Version
	for _, condition := range cr.Items[0].Status.Conditions {
		if condition.Type == "Progressing" {
			os.cvoCondProgressing = condition.Status
			os.cvoCondProgressingMessage = condition.Message
		}
	}
}

func (os *OpenShiftSummary) setFromCoCR(cr *OpenShiftCrCo) {

	for _, item := range cr.Items {
		for _, condition := range item.Status.Conditions {
			switch condition.Type {
			case "Available":
				if condition.Status == "True" {
					os.coCountAvailable += 1
				}
			case "Progressing":
				if condition.Status == "True" {
					os.coCountProgressing += 1
				}
			case "Degraded":
				if condition.Status == "True" {
					os.coCountDegraded += 1
				}
			}
		}
	}
}

func (os *OpenShiftSummary) setPluginResult(in *OPCTPluginSummary) {
	switch in.Name {
	case "openshift-kube-conformance":
		os.pluginResultK8sConformance = in
	case "openshift-conformance-validated":
		os.pluginResultOCPValidated = in
	default:
		fmt.Println("ERROR: plugin not found")
	}
	return
}

func (os *OpenShiftSummary) getResultOCPValidated() *OPCTPluginSummary {
	return os.pluginResultOCPValidated
}

func (os *OpenShiftSummary) getResultK8SValidated() *OPCTPluginSummary {
	return os.pluginResultK8sConformance
}

// OPCT
type OPCTPluginSummary struct {
	Name    string
	Status  string
	Total   int64
	Passed  int64
	Failed  int64
	Timeout int64
	Skipped int64

	// FailedItems is the map with details for each failure
	FailedItems map[string]*PluginFailedItem
	// FailedList is the list of tests failures on the original execution
	FailedList []string
	// FailedFilterSuite is the list of failures (A) included only in the original suite (B): A INTERSECTION B
	FailedFilterSuite []string
	// FailedFilterBaseline is the list of failures (A) excluding the baseline(B): A EXCLUDE B
	FailedFilterBaseline []string
	// FailedFilteFlaky is the list of failures with no Flakes on OpenShift CI
	FailedFilterFlaky []string
}

type PluginFailedItem struct {
	// Name is the name of the e2e test
	Name string
	// Failure contains the failure reason extracted from JUnit field 'item.detials.failure'
	Failure string
	// SystemOut contains the entire test stdout extracted from JUnit field 'item.detials.system-out'
	SystemOut string
	// Offset is the offset of failure from the plugin result file
	Offset int
	// Flaky contains the flaky information from OpenShift CI - scraped from Sippy API
	Flaky *SippyTestsResponse
}

type openshiftTestsSuites struct {
	kubernetesConformance *openshiftTestsSuite
	openshiftConformance  *openshiftTestsSuite
}

func (ts *openshiftTestsSuites) LoadAll() error {
	err := ts.openshiftConformance.Load()
	if err != nil {
		return err
	}
	err = ts.kubernetesConformance.Load()
	if err != nil {
		return err
	}
	return nil
}

func (ts *openshiftTestsSuites) GetTotalOCP() int {
	return ts.openshiftConformance.Count
}

func (ts *openshiftTestsSuites) GetTotalK8S() int {
	return ts.kubernetesConformance.Count
}

type openshiftTestsSuite struct {
	inputFile string
	name      string
	Count     int
	tests     []string
}

func (s *openshiftTestsSuite) Load() error {
	content, err := os.ReadFile(s.inputFile)
	if err != nil {
		log.Fatal(err)
		return err
	}

	s.tests = strings.Split(string(content), "\n")
	s.Count = len(s.tests)
	return nil
}

// OpenShift CRs

// Infrastructures

type OpenShiftCrInfrastructures struct {
	APIVersion string                      `json:"apiVersion"`
	Items      []OpenShiftCrInfrastructure `json:"items"`
}

type OpenShiftCrInfrastructure struct {
	APIVersion string                          `json:"apiVersion"`
	Status     OpenShiftCrInfrastructureStatus `json:"status"`
}

type OpenShiftCrInfrastructureStatus struct {
	APIServerInternalURL   string `json:"apiServerInternalURI"`
	APIServerURL           string `json:"apiServerURL"`
	ControlPlaneTopology   string `json:"controlPlaneTopology"`
	InfrastructureTopology string `json:"infrastructureTopology"`
	InfrastructureName     string `json:"infrastructureName"`
	Platform               string `json:"platform"`
}

// CVO

type OpenShiftCrCvo struct {
	APIVersion string         `json:"apiVersion"`
	Items      []OpenShiftCvo `json:"items"`
}

type OpenShiftCvo struct {
	APIVersion string             `json:"apiVersion"`
	Status     OpenShiftCvoStatus `json:"status"`
}

type OpenShiftCvoStatus struct {
	Desired    OpenShiftCvoStatusDesired      `json:"desired"`
	Conditions []OpenShiftCvoStatusConditions `json:"conditions"`
}

type OpenShiftCvoStatusDesired struct {
	Version string `json:"version"`
}

type OpenShiftCvoStatusConditions struct {
	LastTransitionTime string `json:"lastTransitionTime"`
	Message            string `json:"message,omitempty"`
	Status             string `json:"status"`
	Type               string `json:"type"`
}

// Cluster Operator

type OpenShiftCrCo struct {
	APIVersion string        `json:"apiVersion"`
	Items      []OpenShiftCo `json:"items"`
}

type OpenShiftCo struct {
	APIVersion string            `json:"apiVersion"`
	Status     OpenShiftCoStatus `json:"status"`
}

type OpenShiftCoStatus struct {
	Conditions []OpenShiftCoStatusConditions `json:"conditions"`
}

type OpenShiftCoStatusConditions struct {
	LastTransitionTime string `json:"lastTransitionTime"`
	Message            string `json:"message,omitempty"`
	Status             string `json:"status"`
	Type               string `json:"type"`
}
