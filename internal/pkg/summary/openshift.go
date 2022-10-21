package summary

import (
	"fmt"
	"github.com/pkg/errors"
)

const (
	ConditionTypeProgressing = "Progressing"
	ConditionTypeAvailable   = "Available"
	ConditionTypeDegraded    = "Degraded"
	ConditionStatusTrue      = "True"
	ConditionStatusFalse     = "False"
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

func (os *OpenShiftSummary) SetFromInfraCR(cr *OpenShiftCrInfrastructures) error {
	if len(cr.Items) == 0 {
		return errors.New("Unable to find result Items to set Infrastructures")
	}
	os.InfraPlatformType = cr.Items[0].Status.Platform
	os.InfraAPIServerURL = cr.Items[0].Status.APIServerURL
	os.InfraAPIServerURLInternal = cr.Items[0].Status.APIServerInternalURL
	os.InfraControlPlaneTopology = cr.Items[0].Status.ControlPlaneTopology
	os.InfraTopology = cr.Items[0].Status.InfrastructureTopology
	os.InfraName = cr.Items[0].Status.InfrastructureName
	return nil
}

func (os *OpenShiftSummary) SetFromCvoCR(cr *OpenShiftCrCvo) error {
	if len(cr.Items) == 0 {
		return errors.New("Unable to find result Items to set Infrastructures")
	}
	os.CvoStatusDesiredVersion = cr.Items[0].Status.Desired.Version
	for _, condition := range cr.Items[0].Status.Conditions {
		if condition.Type == ConditionTypeProgressing {
			os.CvoCondProgressing = condition.Status
			os.CvoCondProgressingMessage = condition.Message
		}
	}
	return nil
}

func (os *OpenShiftSummary) SetFromCoCR(cr *OpenShiftCrCo) error {
	for _, item := range cr.Items {
		for _, condition := range item.Status.Conditions {
			switch condition.Type {
			case ConditionTypeAvailable:
				if condition.Status == ConditionStatusTrue {
					os.CoCountAvailable += 1
				}
			case ConditionTypeProgressing:
				if condition.Status == ConditionStatusTrue {
					os.CoCountProgressing += 1
				}
			case ConditionTypeDegraded:
				if condition.Status == ConditionStatusTrue {
					os.CoCountDegraded += 1
				}
			}
		}
	}
	return nil
}

func (os *OpenShiftSummary) SetPluginResult(in *OPCTPluginSummary) error {
	switch in.Name {
	case CertPluginNameKubernetesConformance:
		os.PluginResultK8sConformance = in
	case CertPluginNameOpenshiftValidated:
		os.PluginResultOCPValidated = in
	default:
		return fmt.Errorf("Unable to Set Plugin results: Plugin not found: %s", in.Name)
	}
	return nil
}

func (os *OpenShiftSummary) GetResultOCPValidated() *OPCTPluginSummary {
	return os.PluginResultOCPValidated
}

func (os *OpenShiftSummary) GetResultK8SValidated() *OPCTPluginSummary {
	return os.PluginResultK8sConformance
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
