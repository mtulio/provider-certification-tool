package summary

import (
	"fmt"
	log "github.com/sirupsen/logrus"
	"os"
	"strings"
	"github.com/redhat-openshift-ecosystem/provider-certification-tool/internal/pkg/sippy"
)

type OpenShiftSummary struct {
	// Infra CR
	InfraPlatformType         string
	InfraAPIServerURL         string
	InfraAPIServerURLInternal string
	InfraControlPlaneTopology string
	InfraTopology             string
	InfraName                 string

	// Plugin Results
	PluginResultK8sConformance *OPCTPluginSummary
	PluginResultOCPValidated   *OPCTPluginSummary

	VersionOPpctSonobuoy     string
	VersionOpctCli           string
	VersionPluginConformance string

	// get from Sonobuoy metadata
	VersionK8S string

	// CVO (results-partner/resources/cluster/config.openshift.io_v1_clusterversions.json)
	CvoStatusDesiredVersion   string
	CvoCondProgressing        string
	CvoCondProgressingMessage string

	// Cluster Operators (results-partner/resources/cluster/config.openshift.io_v1_clusteroperators.json)
	CoCountAvailable   uint64
	CoCountProgressing uint64
	CoCountDegraded    uint64
}

func NewOpenShiftSummary() *OpenShiftSummary {
	return &OpenShiftSummary{}
}

func (os *OpenShiftSummary) SetFromInfraCR(cr *OpenShiftCrInfrastructures) {
	os.InfraPlatformType = cr.Items[0].Status.Platform
	os.InfraAPIServerURL = cr.Items[0].Status.APIServerURL
	os.InfraAPIServerURLInternal = cr.Items[0].Status.APIServerInternalURL
	os.InfraControlPlaneTopology = cr.Items[0].Status.ControlPlaneTopology
	os.InfraTopology = cr.Items[0].Status.InfrastructureTopology
	os.InfraName = cr.Items[0].Status.InfrastructureName
}

func (os *OpenShiftSummary) SetFromCvoCR(cr *OpenShiftCrCvo) {
	os.CvoStatusDesiredVersion = cr.Items[0].Status.Desired.Version
	for _, condition := range cr.Items[0].Status.Conditions {
		if condition.Type == "Progressing" {
			os.CvoCondProgressing = condition.Status
			os.CvoCondProgressingMessage = condition.Message
		}
	}
}

func (os *OpenShiftSummary) SetFromCoCR(cr *OpenShiftCrCo) {

	for _, item := range cr.Items {
		for _, condition := range item.Status.Conditions {
			switch condition.Type {
			case "Available":
				if condition.Status == "True" {
					os.CoCountAvailable += 1
				}
			case "Progressing":
				if condition.Status == "True" {
					os.CoCountProgressing += 1
				}
			case "Degraded":
				if condition.Status == "True" {
					os.CoCountDegraded += 1
				}
			}
		}
	}
}

func (os *OpenShiftSummary) SetPluginResult(in *OPCTPluginSummary) {
	switch in.Name {
	case "openshift-kube-conformance":
		os.PluginResultK8sConformance = in
	case "openshift-conformance-validated":
		os.PluginResultOCPValidated = in
	default:
		fmt.Println("ERROR: plugin not found")
	}
	return
}

func (os *OpenShiftSummary) GetResultOCPValidated() *OPCTPluginSummary {
	return os.PluginResultOCPValidated
}

func (os *OpenShiftSummary) GetResultK8SValidated() *OPCTPluginSummary {
	return os.PluginResultK8sConformance
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
	Flaky *sippy.SippyTestsResponse
}

type OpenshiftTestsSuites struct {
	KubernetesConformance *OpenshiftTestsSuite
	OpenshiftConformance  *OpenshiftTestsSuite
}

func (ts *OpenshiftTestsSuites) LoadAll() error {
	err := ts.OpenshiftConformance.Load()
	if err != nil {
		return err
	}
	err = ts.KubernetesConformance.Load()
	if err != nil {
		return err
	}
	return nil
}

func (ts *OpenshiftTestsSuites) GetTotalOCP() int {
	return ts.OpenshiftConformance.Count
}

func (ts *OpenshiftTestsSuites) GetTotalK8S() int {
	return ts.KubernetesConformance.Count
}

type OpenshiftTestsSuite struct {
	InputFile string
	Name      string
	Count     int
	Tests     []string
}

func (s *OpenshiftTestsSuite) Load() error {
	content, err := os.ReadFile(s.InputFile)
	if err != nil {
		log.Fatal(err)
		return err
	}

	s.Tests = strings.Split(string(content), "\n")
	s.Count = len(s.Tests)
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
