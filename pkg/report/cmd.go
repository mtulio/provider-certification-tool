package report

import (
	"fmt"
	"os"
	"strings"

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

func NewCmdReport() *cobra.Command {
	data := Input{}
	cmd := &cobra.Command{
		Use:   "report archive.tar.gz",
		Short: "Create a report from results.",
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

// processResult reads the artifacts and show it as an report format.
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

	if err := showAggregatedSummary(&cs); err != nil {
		return err
	}

	if err := showProcessedSummary(&cs); err != nil {
		return err
	}

	if err := showErrorDetails(&cs, input.verbose); err != nil {
		return err
	}

	if input.saveTo != "" {
		if err := cs.SaveResults(input.saveTo); err != nil {
			return err
		}
	}

	return nil
}

func showAggregatedSummary(cs *summary.ConsolidatedSummary) error {
	fmt.Printf("\n> OPCT Summary <\n\n")

	// vars starting with p* represents the 'partner' artifact
	// vars starting with b* represents 'baseline' artifact
	pOCP := cs.GetProvider().GetOpenShift()
	pOCPCV, _ := pOCP.GetClusterVersion()
	pOCPInfra, _ := pOCP.GetInfrastructure()

	var bOCP *summary.OpenShiftSummary
	var bOCPCV *summary.SummaryClusterVersionOutput
	var bOCPInfra *summary.SummaryOpenShiftInfrastructureV1
	baselineProcessed := cs.GetBaseline().HasValidResults()
	if baselineProcessed {
		bOCP = cs.GetBaseline().GetOpenShift()
		bOCPCV, _ = bOCP.GetClusterVersion()
		bOCPInfra, _ = bOCP.GetInfrastructure()
	}

	// Provider and Baseline Cluster (archive)
	pCL := cs.GetProvider().GetSonobuoyCluster()
	bCL := cs.GetBaseline().GetSonobuoyCluster()

	newLineWithTab := "\t\t\n"
	tbWriter := tabwriter.NewWriter(os.Stdout, 0, 8, 1, '\t', tabwriter.AlignRight)

	if baselineProcessed {
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
	partnerPlatformName := string(pOCPInfra.Status.PlatformStatus.Type)
	if pOCPInfra.Status.PlatformStatus.Type == "External" {
		partnerPlatformName = fmt.Sprintf("%s (%s)", partnerPlatformName, pOCPInfra.Spec.PlatformSpec.External.PlatformName)
	}
	if baselineProcessed {
		baselinePlatformName := string(bOCPInfra.Status.PlatformStatus.Type)
		if bOCPInfra.Status.PlatformStatus.Type == "External" {
			baselinePlatformName = fmt.Sprintf("%s (%s)", baselinePlatformName, bOCPInfra.Spec.PlatformSpec.External.PlatformName)
		}
		fmt.Fprintf(tbWriter, " OCP Infrastructure:\t\t\n")
		fmt.Fprintf(tbWriter, " - PlatformType\t: %s\t: %s\n", partnerPlatformName, baselinePlatformName)
		fmt.Fprintf(tbWriter, " - Name\t: %s\t: %s\n", pOCPInfra.Status.InfrastructureName, bOCPInfra.Status.InfrastructureName)
		fmt.Fprintf(tbWriter, " - Topology\t: %s\t: %s\n", pOCPInfra.Status.InfrastructureTopology, bOCPInfra.Status.InfrastructureTopology)
		fmt.Fprintf(tbWriter, " - ControlPlaneTopology\t: %s\t: %s\n", pOCPInfra.Status.ControlPlaneTopology, bOCPInfra.Status.ControlPlaneTopology)
		fmt.Fprintf(tbWriter, " - API Server URL\t: %s\t: %s\n", pOCPInfra.Status.APIServerURL, bOCPInfra.Status.APIServerURL)
		fmt.Fprintf(tbWriter, " - API Server URL (internal)\t: %s\t: %s\n", pOCPInfra.Status.APIServerInternalURL, bOCPInfra.Status.APIServerInternalURL)
	} else {
		fmt.Fprintf(tbWriter, " OCP Infrastructure:\t\n")
		fmt.Fprintf(tbWriter, " - PlatformType\t: %s\n", partnerPlatformName)
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
	if baselineProcessed {
		plK8S = bOCP.GetResultK8SValidated()
		bOCPPluginRes := fmt.Sprintf("%s [%d/%d/%d/%d] (%d)", plK8S.Status, plK8S.Total, plK8S.Passed, plK8S.Failed, plK8S.Skipped, plK8S.Timeout)
		fmt.Fprintf(tbWriter, " - %s\t: %s\t: %s\n", name, pOCPPluginRes, bOCPPluginRes)
	} else {
		fmt.Fprintf(tbWriter, " - %s\t: %s\n", name, pOCPPluginRes)
	}

	plOCP := pOCP.GetResultOCPValidated()
	name = plOCP.Name
	pOCPPluginRes = fmt.Sprintf("%s [%d/%d/%d/%d] (%d)", plOCP.Status, plOCP.Total, plOCP.Passed, plOCP.Failed, plOCP.Skipped, plOCP.Timeout)

	if baselineProcessed {
		plOCP = bOCP.GetResultOCPValidated()
		bOCPPluginRes := fmt.Sprintf("%s [%d/%d/%d/%d] (%d)", plOCP.Status, plOCP.Total, plOCP.Passed, plOCP.Failed, plOCP.Skipped, plOCP.Timeout)
		fmt.Fprintf(tbWriter, " - %s\t: %s\t: %s\n", name, pOCPPluginRes, bOCPPluginRes)
	} else {
		fmt.Fprintf(tbWriter, " - %s\t: %s\n", name, pOCPPluginRes)
	}

	fmt.Fprint(tbWriter, newLineWithTab)
	fmt.Fprintf(tbWriter, " Health summary:\t  [A=True/P=True/D=True]\t\n")
	pOCPCO, _ := pOCP.GetClusterOperator()

	if baselineProcessed {
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
	if baselineProcessed {
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
	if baselineProcessed {
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

func showProcessedSummary(cs *summary.ConsolidatedSummary) error {

	fmt.Printf("\n> Processed Summary <\n")

	fmt.Printf("\n Total tests by conformance suites:\n")
	fmt.Printf(" - %s: %d \n", summary.SuiteNameKubernetesConformance, cs.GetProvider().GetSuites().GetTotalK8S())
	fmt.Printf(" - %s: %d \n", summary.SuiteNameOpenshiftConformance, cs.GetProvider().GetSuites().GetTotalOCP())

	fmt.Printf("\n Result Summary by conformance plugins:\n")
	bProcessed := cs.GetBaseline().HasValidResults()
	showSummaryPlugin(cs.GetProvider().GetOpenShift().GetResultK8SValidated(), bProcessed)
	showSummaryPlugin(cs.GetProvider().GetOpenShift().GetResultOCPValidated(), bProcessed)

	return nil
}

func showSummaryPlugin(p *summary.OPCTPluginSummary, bProcessed bool) {
	fmt.Printf(" - %s:\n", p.Name)
	fmt.Printf("   - Status: %s\n", p.Status)
	fmt.Printf("   - Total: %d\n", p.Total)
	fmt.Printf("   - Passed: %s\n", calcPercStr(p.Passed, p.Total))
	fmt.Printf("   - Failed: %s\n", calcPercStr(p.Failed, p.Total))
	fmt.Printf("   - Timeout: %s\n", calcPercStr(p.Timeout, p.Total))
	fmt.Printf("   - Skipped: %s\n", calcPercStr(p.Skipped, p.Total))
	fmt.Printf("   - Failed (without filters) : %s\n", calcPercStr(int64(len(p.FailedList)), p.Total))
	fmt.Printf("   - Failed (Filter SuiteOnly): %s\n", calcPercStr(int64(len(p.FailedFilterSuite)), p.Total))
	if bProcessed {
		fmt.Printf("   - Failed (Filter Baseline) : %s\n", calcPercStr(int64(len(p.FailedFilterBaseline)), p.Total))
	}
	fmt.Printf("   - Failed (Filter CI Flakes): %s\n", calcPercStr(int64(len(p.FailedFilterNotFlake)), p.Total))

	/*
		// The Final result will be hidden (pass|fail) will be hiden for a while for those reasons:
		// - OPCT was created to provide feeaback of conformance results, not a passing binary value. The numbers should be interpreted
		// - Conformance results could have flakes or runtime failures which need to be investigated by executor
		// - Force user/executor to review the results, and not only the summary.

		// checking for runtime failure
		runtimeFailed := false
		if p.Total == p.Failed {
			runtimeFailed = true
		}

		// rewrite the original status when pass on all filters and not failed on runtime
		status := p.Status
		if (len(p.FailedFilterNotFlake) == 0) && !runtimeFailed {
			status = "pass"
		}

		fmt.Printf("   - Status After Filters     : %s\n", status)
	*/
}

// showErrorDetails show details of failres for each plugin.
func showErrorDetails(cs *summary.ConsolidatedSummary, verbose bool) error {

	fmt.Printf("\n Result details by conformance plugins: \n")
	bProcessed := cs.GetBaseline().HasValidResults()
	showErrorDetailPlugin(cs.GetProvider().GetOpenShift().GetResultK8SValidated(), verbose, bProcessed)
	showErrorDetailPlugin(cs.GetProvider().GetOpenShift().GetResultOCPValidated(), verbose, bProcessed)

	return nil
}

// showErrorDetailPlugin Show failed e2e tests by filter, when verbose each filter will be shown.
func showErrorDetailPlugin(p *summary.OPCTPluginSummary, verbose bool, bProcessed bool) {

	flakeCount := len(p.FailedFilterBaseline) - len(p.FailedFilterNotFlake)

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
		if bProcessed {
			fmt.Printf("\n --> [verbose] Failed tests removing baseline (Filter Baseline):\n")
			if len(p.FailedFilterBaseline) == 0 {
				fmt.Println("<empty>")
			}
			for _, test := range p.FailedFilterBaseline {
				fmt.Println(test)
			}
		}
	} else {
		fmt.Printf("\n\n => %s: (%d failures, %d flakes)\n", p.Name, len(p.FailedFilterBaseline), flakeCount)
	}

	fmt.Printf("\n --> Failed tests to Review (without flakes) - Immediate action:\n")
	noFlakes := make(map[string]struct{})
	if len(p.FailedFilterBaseline) == flakeCount {
		fmt.Println("<empty>")
	} else { // TODO move to small functions
		testTags := NewTestTagsEmpty(len(p.FailedFilterNotFlake))
		for _, test := range p.FailedFilterNotFlake {
			noFlakes[test] = struct{}{}
			testTags.Add(&test)
		}
		// Failed tests grouped by tag (first value between '[]')
		fmt.Printf("%s\n\n", testTags.ShowSorted())
		fmt.Println(strings.Join(p.FailedFilterNotFlake[:], "\n"))
	}

	fmt.Printf("\n --> Failed flake tests - Statistic from OpenShift CI\n")
	tbWriter := tabwriter.NewWriter(os.Stdout, 0, 8, 1, '\t', tabwriter.AlignRight)

	if len(p.FailedFilterBaseline) == 0 {
		fmt.Fprintf(tbWriter, "<empty>\n")
	} else {
		testTags := NewTestTagsEmpty(len(p.FailedFilterBaseline))
		fmt.Fprintf(tbWriter, "Flakes\tPerc\t TestName\n")
		for _, test := range p.FailedFilterBaseline {
			// preventing duplication when flake tests was already listed.
			if _, ok := noFlakes[test]; ok {
				continue
			}
			// TODO: fix issues when retrieving flakes from Sippy API.
			// Fallback to '--' when has issues.
			if p.FailedItems[test].Flaky == nil {
				fmt.Fprintf(tbWriter, "--\t--\t%s\n", test)
			} else if p.FailedItems[test].Flaky.CurrentFlakes != 0 {
				fmt.Fprintf(tbWriter, "%d\t%.3f%%\t%s\n", p.FailedItems[test].Flaky.CurrentFlakes, p.FailedItems[test].Flaky.CurrentFlakePerc, test)
			}
			testTags.Add(&test)
		}
		fmt.Printf("%s\n\n", testTags.ShowSorted())
	}
	tbWriter.Flush()
}

// calcPercStr receives the numerator and denominator and return the numerator and percentage as string.
func calcPercStr(num, den int64) string {
	return fmt.Sprintf("%d (%.2f%%)", num, (float64(num)/float64(den))*100)
}
