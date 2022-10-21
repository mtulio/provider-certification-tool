package summary

import (
	"github.com/redhat-openshift-ecosystem/provider-certification-tool/internal/pkg/sippy"
)

const (
	CertPluginNameKubernetesConformance = "openshift-kube-conformance"
	CertPluginNameOpenshiftValidated    = "openshift-conformance-validated"
)

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
