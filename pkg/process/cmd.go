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
	saveTo      string
	verbose     bool
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
		&data.saveTo, "save-to", "s", "",
		"Extract and Save Results to disk. Example: -s ./results",
	)
	cmd.Flags().BoolVarP(
		&data.verbose, "verbose", "v", false,
		"Show test details of test failures",
	)
	return cmd
}

func processResult(input *Input) error {

	cs := summary.ConsolidatedSummary{
		Provider: &summary.ResultSummary{
			Name:      summary.ResultSourceNameProvider,
			Archive:   input.archive,
			OpenShift: &summary.OpenShiftSummary{},
			Sonobuoy:  &summary.SonobuoySummary{},
			Suites: &summary.OpenshiftTestsSuites{
				OpenshiftConformance:  &summary.OpenshiftTestsSuite{Name: "openshiftConformance"},
				KubernetesConformance: &summary.OpenshiftTestsSuite{Name: "kubernetesConformance"},
			},
		},
		Baseline: &summary.ResultSummary{
			Name:      summary.ResultSourceNameBaseline,
			Archive:   input.archiveBase,
			OpenShift: &summary.OpenShiftSummary{},
			Sonobuoy:  &summary.SonobuoySummary{},
			Suites: &summary.OpenshiftTestsSuites{
				OpenshiftConformance:  &summary.OpenshiftTestsSuite{Name: "openshiftConformance"},
				KubernetesConformance: &summary.OpenshiftTestsSuite{Name: "kubernetesConformance"},
			},
		},
	}

	if err := cs.Process(); err != nil {
		return err
	}

	if err := printAggregatedSummary(&cs); err != nil {
		return err
	}

	if err := printProcessedSummary(&cs); err != nil {
		return err
	}

	if err := printErrorDetails(&cs, input.verbose); err != nil {
		return err
	}

	if input.saveTo != "" {
		if err := cs.SaveResults(input.saveTo); err != nil {
			return err
		}
	}

	return nil
}

func printAggregatedSummary(cs *summary.ConsolidatedSummary) error {
	fmt.Printf("\n> OpenShift Provider Certification Summary <\n\n")

	pOCP := cs.GetProvider().GetOpenShift()
	pOCPCV, _ := pOCP.GetClusterVersion()
	pOCPInfra, _ := pOCP.GetInfrastructure()

	var bOCP *summary.OpenShiftSummary
	var bOCPCV *summary.SummaryClusterVersionOutput
	var bOCPInfra *summary.SummaryOpenShiftInfrastructureV1
	baselineDefined := cs.GetBaseline().HasValidResults()
	if baselineDefined {
		bOCP = cs.GetBaseline().GetOpenShift()
		bOCPCV, _ = bOCP.GetClusterVersion()
		bOCPInfra, _ = bOCP.GetInfrastructure()
	}

	// Provider and Baseline Cluster (archive)
	pCL := cs.GetProvider().GetSonobuoyCluster()
	bCL := cs.GetBaseline().GetSonobuoyCluster()

	newLineWithTab := "\t\t\n"
	tbWriter := tabwriter.NewWriter(os.Stdout, 0, 8, 1, '\t', tabwriter.AlignRight)

	if baselineDefined {
		fmt.Fprintf(tbWriter, " Kubernetes API Server version\t: %s\t: %s\n", pCL.APIVersion, bCL.APIVersion)
		fmt.Fprintf(tbWriter, " OpenShift Container Platform version\t: %s\t: %s\n", pOCPCV.DesiredVersion, bOCPCV.DesiredVersion)
		fmt.Fprintf(tbWriter, " - Cluster Update Progressing\t: %s\t: %s\n", pOCPCV.Progressing, bOCPCV.Progressing)
		fmt.Fprintf(tbWriter, " - Cluster Target Version\t: %s\t: %s\n", pOCPCV.ProgressingMessage, bOCPCV.ProgressingMessage)
	} else {
		fmt.Fprintf(tbWriter, " Kubernetes API Server version\t: %s\n", pCL.APIVersion)
		fmt.Fprintf(tbWriter, " OpenShift Container Platform version\t: %s\n", pOCPCV.DesiredVersion)
		fmt.Fprintf(tbWriter, " - Cluster Update Progressing\t: %s\n", pOCPCV.Progressing)
		fmt.Fprintf(tbWriter, " - Cluster Target Version\t: %s\n", pOCPCV.ProgressingMessage)
	}

	fmt.Fprint(tbWriter, newLineWithTab)
	if baselineDefined {
		fmt.Fprintf(tbWriter, " OCP Infrastructure:\t\t\n")
		fmt.Fprintf(tbWriter, " - PlatformType\t: %s\t: %s\n", pOCPInfra.Status.PlatformStatus.Type, bOCPInfra.Status.PlatformStatus.Type)
		fmt.Fprintf(tbWriter, " - Name\t: %s\t: %s\n", pOCPInfra.Status.InfrastructureName, bOCPInfra.Status.InfrastructureName)
		fmt.Fprintf(tbWriter, " - Topology\t: %s\t: %s\n", pOCPInfra.Status.InfrastructureTopology, bOCPInfra.Status.InfrastructureTopology)
		fmt.Fprintf(tbWriter, " - ControlPlaneTopology\t: %s\t: %s\n", pOCPInfra.Status.ControlPlaneTopology, bOCPInfra.Status.ControlPlaneTopology)
		fmt.Fprintf(tbWriter, " - API Server URL\t: %s\t: %s\n", pOCPInfra.Status.APIServerURL, bOCPInfra.Status.APIServerURL)
		fmt.Fprintf(tbWriter, " - API Server URL (internal)\t: %s\t: %s\n", pOCPInfra.Status.APIServerInternalURL, bOCPInfra.Status.APIServerInternalURL)
	} else {
		fmt.Fprintf(tbWriter, " OCP Infrastructure:\t\n")
		fmt.Fprintf(tbWriter, " - PlatformType\t: %s\n", pOCPInfra.Status.PlatformStatus.Type)
		fmt.Fprintf(tbWriter, " - Name\t: %s\n", pOCPInfra.Status.InfrastructureName)
		fmt.Fprintf(tbWriter, " - Topology\t: %s\n", pOCPInfra.Status.InfrastructureTopology)
		fmt.Fprintf(tbWriter, " - ControlPlaneTopology\t: %s\n", pOCPInfra.Status.ControlPlaneTopology)
		fmt.Fprintf(tbWriter, " - API Server URL\t: %s\n", pOCPInfra.Status.APIServerURL)
		fmt.Fprintf(tbWriter, " - API Server URL (internal)\t: %s\n", pOCPInfra.Status.APIServerInternalURL)
	}

	fmt.Fprint(tbWriter, newLineWithTab)
	fmt.Fprintf(tbWriter, " Plugins summary by name:\t  Status [Total/Passed/Failed/Skipped] (timeout)\n")

	plK8S := pOCP.GetResultK8SValidated()
	name := plK8S.Name
	pOCPPluginRes := fmt.Sprintf("%s [%d/%d/%d/%d] (%d)", plK8S.Status, plK8S.Total, plK8S.Passed, plK8S.Failed, plK8S.Skipped, plK8S.Timeout)
	if baselineDefined {
		plK8S = bOCP.GetResultK8SValidated()
		bOCPPluginRes := fmt.Sprintf("%s [%d/%d/%d/%d] (%d)", plK8S.Status, plK8S.Total, plK8S.Passed, plK8S.Failed, plK8S.Skipped, plK8S.Timeout)
		fmt.Fprintf(tbWriter, " - %s\t: %s\t: %s\n", name, pOCPPluginRes, bOCPPluginRes)
	} else {
		fmt.Fprintf(tbWriter, " - %s\t: %s\n", name, pOCPPluginRes)
	}

	plOCP := pOCP.GetResultOCPValidated()
	name = plOCP.Name
	pOCPPluginRes = fmt.Sprintf("%s [%d/%d/%d/%d] (%d)", plOCP.Status, plOCP.Total, plOCP.Passed, plOCP.Failed, plOCP.Skipped, plOCP.Timeout)

	if baselineDefined {
		plOCP = bOCP.GetResultOCPValidated()
		bOCPPluginRes := fmt.Sprintf("%s [%d/%d/%d/%d] (%d)", plOCP.Status, plOCP.Total, plOCP.Passed, plOCP.Failed, plOCP.Skipped, plOCP.Timeout)
		fmt.Fprintf(tbWriter, " - %s\t: %s\t: %s\n", name, pOCPPluginRes, bOCPPluginRes)
	} else {
		fmt.Fprintf(tbWriter, " - %s\t: %s\n", name, pOCPPluginRes)
	}

	fmt.Fprint(tbWriter, newLineWithTab)
	fmt.Fprintf(tbWriter, " Health summary:\t  [A=True/P=True/D=True]\t\n")
	pOCPCO, _ := pOCP.GetClusterOperator()

	if baselineDefined {
		bOCPCO, _ := bOCP.GetClusterOperator()
		fmt.Fprintf(tbWriter, " - Cluster Operators\t: [%d/%d/%d]\t: [%d/%d/%d]\n",
			pOCPCO.CountAvailable, pOCPCO.CountProgressing, pOCPCO.CountDegraded,
			bOCPCO.CountAvailable, bOCPCO.CountProgressing, bOCPCO.CountDegraded,
		)
	} else {
		fmt.Fprintf(tbWriter, " - Cluster Operators\t: [%d/%d/%d]\n",
			pOCPCO.CountAvailable, pOCPCO.CountProgressing, pOCPCO.CountDegraded,
		)
	}

	pNhMessage := fmt.Sprintf("%d/%d %s", pCL.NodeHealth.Total, pCL.NodeHealth.Total, "")
	if pCL.NodeHealth.Total != 0 {
		pNhMessage = fmt.Sprintf("%s (%d%%)", pNhMessage, 100*pCL.NodeHealth.Healthy/pCL.NodeHealth.Total)
	}

	bNhMessage := fmt.Sprintf("%d/%d %s", bCL.NodeHealth.Total, bCL.NodeHealth.Total, "")
	if bCL.NodeHealth.Total != 0 {
		bNhMessage = fmt.Sprintf("%s (%d%%)", bNhMessage, 100*bCL.NodeHealth.Healthy/bCL.NodeHealth.Total)
	}
	if baselineDefined {
		fmt.Fprintf(tbWriter, " - Node health\t: %s\t: %s\n", pNhMessage, bNhMessage)
	} else {
		fmt.Fprintf(tbWriter, " - Node health\t: %s\n", pNhMessage)
	}

	pPodsHealthMsg := ""
	bPodsHealthMsg := ""
	if len(pCL.PodHealth.Details) > 0 {
		phTotal := ""
		if pCL.PodHealth.Total != 0 {
			phTotal = fmt.Sprintf(" (%d%%)", 100*pCL.PodHealth.Healthy/pCL.PodHealth.Total)
		}
		pPodsHealthMsg = fmt.Sprintf("%d/%d %s", pCL.PodHealth.Healthy, pCL.PodHealth.Total, phTotal)
	}
	if baselineDefined {
		if len(bCL.PodHealth.Details) > 0 {
			phTotal := ""
			if bCL.PodHealth.Total != 0 {
				phTotal = fmt.Sprintf(" (%d%%)", 100*bCL.PodHealth.Healthy/bCL.PodHealth.Total)
			}
			bPodsHealthMsg = fmt.Sprintf("%d/%d %s", bCL.PodHealth.Healthy, bCL.PodHealth.Total, phTotal)
		}
		fmt.Fprintf(tbWriter, " - Pods health\t: %s\t: %s\n", pPodsHealthMsg, bPodsHealthMsg)
	} else {
		fmt.Fprintf(tbWriter, " - Pods health\t: %s\n", pPodsHealthMsg)
	}

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
	fmt.Printf("   - Failed (Filter Baseline) : %d\n", len(p.FailedFilterBaseline))
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
	fmt.Printf(" - %s: %d \n", summary.SuiteNameKubernetesConformance, cs.GetProvider().GetSuites().GetTotalK8S())
	fmt.Printf(" - %s: %d \n", summary.SuiteNameOpenshiftConformance, cs.GetProvider().GetSuites().GetTotalOCP())

	fmt.Printf("\n Total Tests by Certification Layer:\n")
	printSummaryPlugin(cs.GetProvider().GetOpenShift().GetResultK8SValidated())
	printSummaryPlugin(cs.GetProvider().GetOpenShift().GetResultOCPValidated())

	return nil
}

func printErrorDetailPlugin(p *summary.OPCTPluginSummary, verbose bool) {

	flakeCount := len(p.FailedFilterBaseline) - len(p.FailedFilterFlaky)

	if verbose {
		fmt.Printf("\n\n => %s: (%d failures, %d failures filtered, %d flakes)\n", p.Name, len(p.FailedList), len(p.FailedFilterBaseline), flakeCount)

		fmt.Printf("\n --> [verbose] Failed tests detected on archive (without filters):\n")
		if len(p.FailedList) == 0 {
			fmt.Println("<empty>")
		}
		for _, test := range p.FailedList {
			fmt.Println(test)
		}

		fmt.Printf("\n --> [verbose] Failed tests detected on suite (Filter SuiteOnly):\n")
		if len(p.FailedFilterSuite) == 0 {
			fmt.Println("<empty>")
		}
		for _, test := range p.FailedFilterSuite {
			fmt.Println(test)
		}

		fmt.Printf("\n --> [verbose] Failed tests removing baseline (Filter Baseline):\n")
		if len(p.FailedFilterBaseline) == 0 {
			fmt.Println("<empty>")
		}
		for _, test := range p.FailedFilterBaseline {
			fmt.Println(test)
		}

	} else {
		fmt.Printf("\n\n => %s: (%d failures, %d flakes)\n", p.Name, len(p.FailedFilterBaseline), flakeCount)
	}

	fmt.Printf("\n --> Failed tests to Review (without flakes) - Immediate action:\n")
	if len(p.FailedFilterBaseline) == flakeCount {
		fmt.Println("<empty>")
	}
	for _, test := range p.FailedFilterFlaky {
		fmt.Println(test)
	}

	fmt.Printf("\n --> Failed flake tests - Statistic from OpenShift CI\n")
	tbWriter := tabwriter.NewWriter(os.Stdout, 0, 8, 1, '\t', tabwriter.AlignRight)

	if len(p.FailedFilterBaseline) == 0 {
		fmt.Fprintf(tbWriter, "<empty>\n")
	} else {
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
	}
	tbWriter.Flush()
}

func printErrorDetails(cs *summary.ConsolidatedSummary, verbose bool) error {

	fmt.Printf("\n Total Tests by Certification Layer: \n")
	printErrorDetailPlugin(cs.GetProvider().GetOpenShift().GetResultK8SValidated(), verbose)
	printErrorDetailPlugin(cs.GetProvider().GetOpenShift().GetResultOCPValidated(), verbose)

	return nil
}
