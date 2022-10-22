package process

import (
	"fmt"
	"os"

	"github.com/pkg/errors"
	"github.com/spf13/cobra"

	"text/tabwriter"

	"github.com/redhat-openshift-ecosystem/provider-certification-tool/internal/pkg/summary"
	"github.com/vmware-tanzu/sonobuoy/pkg/errlog"
)

type Input struct {
	archive     string
	archiveBase string
	suiteOCP    string
	suiteKube   string
	saveTo      string
}

func NewCmdProcess() *cobra.Command {
	data := Input{}
	cmd := &cobra.Command{
		Use:   "process archive.tar.gz",
		Short: "Inspect plugin results.",
		Run: func(cmd *cobra.Command, args []string) {
			data.archive = args[0]
			if err := processResult(&data); err != nil {
				errlog.LogError(errors.Wrapf(err, "could not process archive: %v", args[0]))
				os.Exit(1)
			}
		},
		Args: cobra.ExactArgs(1),
	}

	cmd.Flags().StringVarP(
		&data.archiveBase, "baseline", "b", "",
		"Baseline result archive file. Example: -b file.tar.gz",
	)
	cmd.MarkFlagRequired("base")
	cmd.Flags().StringVarP(
		&data.suiteOCP, "base-suite-ocp", "o", "",
		"Base suite reference. Example: -b openshift-tests-openshift-conformance.txt",
	)
	cmd.MarkFlagRequired("base-suite-ocp")
	cmd.Flags().StringVarP(
		&data.suiteKube, "base-suite-k8s", "k", "",
		"Base suite reference. Example: -b openshift-tests-kube-conformance.txt",
	)
	cmd.MarkFlagRequired("base-suite-k8s")
	cmd.Flags().StringVarP(
		&data.saveTo, "save-to", "s", "",
		"Extract and Save Results to disk. Example: -s ./results",
	)
	return cmd
}

func processResult(input *Input) error {

	cs := summary.ConsolidatedSummary{
		Provider: &summary.ResultSummary{
			Name:      "provider",
			Archive:   input.archive,
			OpenShift: &summary.OpenShiftSummary{},
			Sonobuoy:  &summary.SonobuoySummary{},
		},
		Baseline: &summary.ResultSummary{
			Name:      "baseline",
			Archive:   input.archiveBase,
			OpenShift: &summary.OpenShiftSummary{},
			Sonobuoy:  &summary.SonobuoySummary{},
		},
		Suites: &summary.OpenshiftTestsSuites{
			OpenshiftConformance: &summary.OpenshiftTestsSuite{
				Name:      "openshiftConformance",
				InputFile: input.suiteOCP,
			},
			KubernetesConformance: &summary.OpenshiftTestsSuite{
				Name:      "kubernetesConformance",
				InputFile: input.suiteKube,
			},
		},
	}

	err := cs.Process()
	if err != nil {
		return err
	}

	err = printAggregatedSummary(&cs)
	if err != nil {
		return err
	}

	err = printProcessedSummary(&cs)
	if err != nil {
		return err
	}

	err = printErrorDetails(&cs)
	if err != nil {
		return err
	}

	if input.saveTo != "" {
		err = cs.SaveResults(input.saveTo)
		if err != nil {
			return err
		}
	}

	return err
}

func printAggregatedSummary(cs *summary.ConsolidatedSummary) error {
	fmt.Printf("\n> OpenShift Provider Certification Summary <\n\n")

	pOCP := cs.GetProvider().GetOpenShift()
	bOCP := cs.GetBaseline().GetOpenShift()

	pOCPCV, _ := pOCP.GetClusterVersion()
	bOCPCV, _ := bOCP.GetClusterVersion()

	pOCPInfra, _ := pOCP.GetInfrastructure()
	bOCPInfra, _ := bOCP.GetInfrastructure()

	// Provider and Baseline Cluster (archive)
	pCL := cs.GetProvider().GetSonobuoyCluster()
	bCL := cs.GetBaseline().GetSonobuoyCluster()

	newLineWithTab := "\t\t\n"
	tbWriter := tabwriter.NewWriter(os.Stdout, 0, 8, 1, '\t', tabwriter.AlignRight)

	fmt.Fprintf(tbWriter, " Kubernetes API Server version\t: %s\t: %s\n", pCL.APIVersion, bCL.APIVersion)
	fmt.Fprintf(tbWriter, " OpenShift Container Platform version\t: %s\t: %s\n", pOCPCV.DesiredVersion, bOCPCV.DesiredVersion)
	fmt.Fprintf(tbWriter, " - Cluster Update Progressing\t: %s\t: %s\n", pOCPCV.Progressing, bOCPCV.Progressing)
	fmt.Fprintf(tbWriter, " - Cluster Target Version\t: %s\t: %s\n", pOCPCV.ProgressingMessage, bOCPCV.ProgressingMessage)

	fmt.Fprint(tbWriter, newLineWithTab)
	fmt.Fprintf(tbWriter, " OCP Infrastructure:\t\t\n")
	fmt.Fprintf(tbWriter, " - PlatformType\t: %s\t: %s\n", pOCPInfra.Status.PlatformStatus.Type, bOCPInfra.Status.PlatformStatus.Type)
	fmt.Fprintf(tbWriter, " - Name\t: %s\t: %s\n", pOCPInfra.Status.InfrastructureName, bOCPInfra.Status.InfrastructureName)
	fmt.Fprintf(tbWriter, " - Topology\t: %s\t: %s\n", pOCPInfra.Status.InfrastructureTopology, bOCPInfra.Status.InfrastructureTopology)
	fmt.Fprintf(tbWriter, " - ControlPlaneTopology\t: %s\t: %s\n", pOCPInfra.Status.ControlPlaneTopology, bOCPInfra.Status.ControlPlaneTopology)
	fmt.Fprintf(tbWriter, " - API Server URL\t: %s\t: %s\n", pOCPInfra.Status.APIServerURL, bOCPInfra.Status.APIServerURL)
	fmt.Fprintf(tbWriter, " - API Server URL (internal)\t: %s\t: %s\n", pOCPInfra.Status.APIServerInternalURL, bOCPInfra.Status.APIServerInternalURL)

	fmt.Fprint(tbWriter, newLineWithTab)
	fmt.Fprintf(tbWriter, " Plugins summary by name:\t  Status [Total/Passed/Failed/Skipped] (timeout)\n")

	plK8S := pOCP.GetResultK8SValidated()
	name := plK8S.Name
	pOCPPluginRes := fmt.Sprintf("%s [%d/%d/%d/%d] (%d)", plK8S.Status, plK8S.Total, plK8S.Passed, plK8S.Failed, plK8S.Skipped, plK8S.Timeout)
	plK8S = bOCP.GetResultK8SValidated()
	bOCPPluginRes := fmt.Sprintf("%s [%d/%d/%d/%d] (%d)", plK8S.Status, plK8S.Total, plK8S.Passed, plK8S.Failed, plK8S.Skipped, plK8S.Timeout)
	fmt.Fprintf(tbWriter, " - %s\t: %s\t: %s\n", name, pOCPPluginRes, bOCPPluginRes)

	plOCP := pOCP.GetResultOCPValidated()
	name = plOCP.Name
	pOCPPluginRes = fmt.Sprintf("%s [%d/%d/%d/%d] (%d)", plOCP.Status, plOCP.Total, plOCP.Passed, plOCP.Failed, plOCP.Skipped, plOCP.Timeout)
	plOCP = bOCP.GetResultOCPValidated()
	bOCPPluginRes = fmt.Sprintf("%s [%d/%d/%d/%d] (%d)", plOCP.Status, plOCP.Total, plOCP.Passed, plOCP.Failed, plOCP.Skipped, plOCP.Timeout)
	fmt.Fprintf(tbWriter, " - %s\t: %s\t: %s\n", name, pOCPPluginRes, bOCPPluginRes)

	fmt.Fprint(tbWriter, newLineWithTab)
	fmt.Fprintf(tbWriter, " Health summary:\t  [A=True/P=True/D=True]\t\n")
	pOCPCO, _ := pOCP.GetClusterOperator()
	bOCPCO, _ := bOCP.GetClusterOperator()
	fmt.Fprintf(tbWriter, " - Cluster Operators\t: [%d/%d/%d]\t: [%d/%d/%d]\n",
		pOCPCO.CountAvailable, pOCPCO.CountProgressing, pOCPCO.CountDegraded,
		bOCPCO.CountAvailable, bOCPCO.CountProgressing, bOCPCO.CountDegraded,
	)

	pNhMessage := fmt.Sprintf("%d/%d %s", pCL.NodeHealth.Total, pCL.NodeHealth.Total, "")
	if pCL.NodeHealth.Total != 0 {
		pNhMessage = fmt.Sprintf("%s (%d%%)", pNhMessage, 100*pCL.NodeHealth.Healthy/pCL.NodeHealth.Total)
	}

	bNhMessage := fmt.Sprintf("%d/%d %s", bCL.NodeHealth.Total, bCL.NodeHealth.Total, "")
	if bCL.NodeHealth.Total != 0 {
		bNhMessage = fmt.Sprintf("%s (%d%%)", bNhMessage, 100*bCL.NodeHealth.Healthy/bCL.NodeHealth.Total)
	}
	fmt.Fprintf(tbWriter, " - Node health\t: %s\t: %s\n", pNhMessage, bNhMessage)

	pPodsHealthMsg := ""
	bPodsHealthMsg := ""
	if len(pCL.PodHealth.Details) > 0 {
		phTotal := ""
		if pCL.PodHealth.Total != 0 {
			phTotal = fmt.Sprintf(" (%d%%)", 100*pCL.PodHealth.Healthy/pCL.PodHealth.Total)
		}
		pPodsHealthMsg = fmt.Sprintf("%d/%d %s", pCL.PodHealth.Healthy, pCL.PodHealth.Total, phTotal)
	}
	if len(bCL.PodHealth.Details) > 0 {
		phTotal := ""
		if bCL.PodHealth.Total != 0 {
			phTotal = fmt.Sprintf(" (%d%%)", 100*bCL.PodHealth.Healthy/bCL.PodHealth.Total)
		}
		bPodsHealthMsg = fmt.Sprintf("%d/%d %s", bCL.PodHealth.Healthy, bCL.PodHealth.Total, phTotal)
	}

	fmt.Fprintf(tbWriter, " - Pods health\t: %s\t: %s\n", pPodsHealthMsg, bPodsHealthMsg)
	tbWriter.Flush()

	return nil
}

func printSummaryPlugin(p *summary.OPCTPluginSummary) {
	fmt.Printf(" - %s:\n", p.Name)
	fmt.Printf("   - Status: %s\n", p.Status)
	fmt.Printf("   - Total: %d\n", p.Total)
	fmt.Printf("   - Passed: %d\n", p.Passed)
	fmt.Printf("   - Failed: %d\n", p.Failed)
	fmt.Printf("   - Timeout: %d\n", p.Timeout)
	fmt.Printf("   - Skipped: %d\n", p.Skipped)
	fmt.Printf("   - Failed (without filters) : %d\n", len(p.FailedList))
	fmt.Printf("   - Failed (Filter SuiteOnly): %d\n", len(p.FailedFilterSuite))
	fmt.Printf("   - Failed (Filter Baseline  : %d\n", len(p.FailedFilterBaseline))
	fmt.Printf("   - Failed (Filter CI Flakes): %d\n", len(p.FailedFilterFlaky))
	newStatus := p.Status
	if len(p.FailedFilterFlaky) == 0 {
		newStatus = "pass"
	}
	fmt.Printf("   - Status After Filters     : %s\n", newStatus)
}

func printProcessedSummary(cs *summary.ConsolidatedSummary) error {

	fmt.Printf("\n> Processed Summary <\n")

	fmt.Printf("\n Total Tests suites:\n")
	fmt.Printf(" - %s: %d \n", summary.SuiteNameKubernetesConformance, cs.Suites.GetTotalK8S())
	fmt.Printf(" - %s: %d \n", summary.SuiteNameOpenshiftConformance, cs.Suites.GetTotalOCP())

	fmt.Printf("\n Total Tests by Certification Layer:\n")
	printSummaryPlugin(cs.GetProvider().GetOpenShift().GetResultK8SValidated())
	printSummaryPlugin(cs.GetProvider().GetOpenShift().GetResultOCPValidated())

	return nil
}

func printErrorDetailPlugin(p *summary.OPCTPluginSummary) {

	flakeCount := len(p.FailedFilterBaseline) - len(p.FailedFilterFlaky)
	fmt.Printf("\n\n => %s: (%d failures, %d flakes)\n", p.Name, len(p.FailedFilterBaseline), flakeCount)

	fmt.Printf("\n --> Failed tests to Review (without flakes) - Immediate action:\n")
	for _, test := range p.FailedFilterFlaky {
		fmt.Println(test)
	}
	if len(p.FailedFilterBaseline) == flakeCount {
		fmt.Println("<empty>")
	}

	fmt.Printf("\n --> Failed flake tests - Statistic from OpenShift CI\n")
	tbWriter := tabwriter.NewWriter(os.Stdout, 0, 8, 1, '\t', tabwriter.AlignRight)
	fmt.Fprintf(tbWriter, "Flakes\tPerc\t TestName\n")
	for _, test := range p.FailedFilterBaseline {
		// When the was issues to create the flaky item (network connectivity with Sippy API),
		// fallback to '--' values.
		if p.FailedItems[test].Flaky == nil {
			fmt.Fprintf(tbWriter, "--\t--\t%s\n", test)
		} else if p.FailedItems[test].Flaky.CurrentFlakes != 0 {
			fmt.Fprintf(tbWriter, "%d\t%.3f%%\t%s\n", p.FailedItems[test].Flaky.CurrentFlakes, p.FailedItems[test].Flaky.CurrentFlakePerc, test)
		}
	}
	tbWriter.Flush()
}

func printErrorDetails(cs *summary.ConsolidatedSummary) error {

	fmt.Printf("\n Total Tests by Certification Layer: \n")
	printErrorDetailPlugin(cs.GetProvider().GetOpenShift().GetResultK8SValidated())
	printErrorDetailPlugin(cs.GetProvider().GetOpenShift().GetResultOCPValidated())

	return nil
}
