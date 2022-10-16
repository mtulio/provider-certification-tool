package process

import (
	"bufio"
	"fmt"
	"os"
	"sort"

	log "github.com/sirupsen/logrus"

	"github.com/pkg/errors"
	"github.com/spf13/cobra"

	"github.com/vmware-tanzu/sonobuoy/pkg/errlog"
	"text/tabwriter"
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
		&data.archiveBase, "base", "b", "",
		"Base result archive file. Example: -b file.tar.gz",
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

	cs := ConsolidatedSummary{
		provider: &ResultSummary{
			name:      "provider",
			archive:   input.archive,
			openshift: NewOpenShiftSummary(),
		},
		baseline: &ResultSummary{
			name:      "base",
			archive:   input.archiveBase,
			openshift: NewOpenShiftSummary(),
		},
		suites: &openshiftTestsSuites{
			openshiftConformance: &openshiftTestsSuite{
				name:      "openshiftConformance",
				inputFile: input.suiteOCP,
			},
			kubernetesConformance: &openshiftTestsSuite{
				name:      "kubernetesConformance",
				inputFile: input.suiteKube,
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

func printAggregatedSummary(cs *ConsolidatedSummary) error {
	fmt.Printf("\n> OpenShift Provider Certification Summary <\n\n")

	pOCP := cs.provider.openshift
	pCL := cs.provider.cluster

	bOCP := cs.baseline.openshift
	bCL := cs.baseline.cluster

	newLineWithTab := "\t\t\n"
	tbWriter := tabwriter.NewWriter(os.Stdout, 0, 8, 1, '\t', tabwriter.AlignRight)

	fmt.Fprintf(tbWriter, " Kubernetes API Server version\t: %s\t: %s\n", pCL.APIVersion, bCL.APIVersion)
	fmt.Fprintf(tbWriter, " OpenShift Container Platform version\t: %s\t: %s\n", pOCP.cvoStatusDesiredVersion, bOCP.cvoStatusDesiredVersion)
	fmt.Fprintf(tbWriter, " - Cluster Update Progressing\t: %s\t: %s\n", pOCP.cvoCondProgressing, bOCP.cvoCondProgressing)
	fmt.Fprintf(tbWriter, " - Cluster Target Version\t: %s\t: %s\n", pOCP.cvoCondProgressingMessage, bOCP.cvoCondProgressingMessage)

	fmt.Fprintf(tbWriter, newLineWithTab)
	fmt.Fprintf(tbWriter, " OCP Infrastructure:\t\t\n")
	fmt.Fprintf(tbWriter, " - PlatformType\t: %s\t: %s\n", pOCP.infraPlatformType, bOCP.infraPlatformType)
	fmt.Fprintf(tbWriter, " - Name\t: %s\t: %s\n", pOCP.infraName, bOCP.infraName)
	fmt.Fprintf(tbWriter, " - Topology\t: %s\t: %s\n", pOCP.infraTopology, bOCP.infraTopology)
	fmt.Fprintf(tbWriter, " - ControlPlaneTopology\t: %s\t: %s\n", pOCP.infraControlPlaneTopology, bOCP.infraControlPlaneTopology)
	fmt.Fprintf(tbWriter, " - API Server URL\t: %s\t: %s\n", pOCP.infraAPIServerURL, bOCP.infraAPIServerURL)
	fmt.Fprintf(tbWriter, " - API Server URL (internal)\t: %s\t: %s\n", pOCP.infraAPIServerURLInternal, bOCP.infraAPIServerURLInternal)

	fmt.Fprintf(tbWriter, newLineWithTab)
	fmt.Fprintf(tbWriter, " Plugins summary by name:\t  Status [Total/Passed/Failed/Skipped]\t\n")

	plK8S := pOCP.getResultK8SValidated()
	name := plK8S.Name
	pOCPPluginRes := fmt.Sprintf("%s [%d/%d/%d/%d] (%d)", plK8S.Status, plK8S.Total, plK8S.Passed, plK8S.Failed, plK8S.Skipped, plK8S.Timeout)
	plK8S = bOCP.getResultK8SValidated()
	bOCPPluginRes := fmt.Sprintf("%s [%d/%d/%d/%d] (%d)", plK8S.Status, plK8S.Total, plK8S.Passed, plK8S.Failed, plK8S.Skipped, plK8S.Timeout)
	fmt.Fprintf(tbWriter, " - %s\t: %s\t: %s\n", name, pOCPPluginRes, bOCPPluginRes)

	plOCP := pOCP.getResultOCPValidated()
	name = plOCP.Name
	pOCPPluginRes = fmt.Sprintf("%s [%d/%d/%d/%d] (%d)", plOCP.Status, plOCP.Total, plOCP.Passed, plOCP.Failed, plOCP.Skipped, plOCP.Timeout)
	plOCP = bOCP.getResultOCPValidated()
	bOCPPluginRes = fmt.Sprintf("%s [%d/%d/%d/%d] (%d)", plOCP.Status, plOCP.Total, plOCP.Passed, plOCP.Failed, plOCP.Skipped, plOCP.Timeout)
	fmt.Fprintf(tbWriter, " - %s\t: %s\t: %s\n", name, pOCPPluginRes, bOCPPluginRes)

	fmt.Fprintf(tbWriter, newLineWithTab)
	fmt.Fprintf(tbWriter, " Health summary:\t  [A=True/P=True/D=True]\t\n")
	fmt.Fprintf(tbWriter, " - Cluster Operators\t: [%d/%d/%d]\t: [%d/%d/%d]\n",
		pOCP.coCountAvailable, pOCP.coCountProgressing, pOCP.coCountDegraded,
		bOCP.coCountAvailable, bOCP.coCountProgressing, bOCP.coCountDegraded,
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

func printSummaryPlugin(p *OPCTPluginSummary) {
	fmt.Printf(" - %s:\n", p.Name)
	fmt.Printf("   - Status: %s\n", p.Status)
	fmt.Printf("   - Total: %d\n", p.Total)
	fmt.Printf("   - Passed: %d\n", p.Passed)
	fmt.Printf("   - Failed: %d\n", p.Failed)
	fmt.Printf("   - Timeout: %d\n", p.Timeout)
	fmt.Printf("   - Skipped: %d\n", p.Skipped)
	fmt.Printf("   - len(FailedList): %d\n", len(p.FailedList))
	fmt.Printf("   - len(FailedFilterSuite): %d\n", len(p.FailedFilterSuite))
	fmt.Printf("   - len(FailedFilterBaseline): %d\n", len(p.FailedFilterBaseline))
}

func printProcessedSummary(cs *ConsolidatedSummary) error {

	fmt.Printf("\n> Processed Summary <\n")

	fmt.Printf("\n Total Tests suites\n")
	fmt.Printf(" - kubernetes/conformance: %d \n", cs.suites.GetTotalK8S())
	fmt.Printf(" - openshift/conformance: %d \n", cs.suites.GetTotalOCP())

	fmt.Printf("\n Total Tests by Certification Layer: \n")
	printSummaryPlugin(cs.provider.openshift.getResultK8SValidated())
	printSummaryPlugin(cs.provider.openshift.getResultOCPValidated())

	return nil
}

func printErrorDetailPlugin(p *OPCTPluginSummary) {
	fmt.Printf("\n - %s: (%d failures)\n\n", p.Name, len(p.FailedFilterBaseline))
	for _, test := range p.FailedFilterBaseline {
		fmt.Println(test)
	}
}

func printErrorDetails(cs *ConsolidatedSummary) error {

	fmt.Printf("\n> Processed Summary <\n")
	fmt.Printf("\n Total Tests by Certification Layer: \n")
	printErrorDetailPlugin(cs.provider.openshift.getResultK8SValidated())
	printErrorDetailPlugin(cs.provider.openshift.getResultOCPValidated())

	return nil
}

// ConsolidatedSummary Aggregate the results of provider and baseline
type ConsolidatedSummary struct {
	provider *ResultSummary
	baseline *ResultSummary
	suites   *openshiftTestsSuites
}

func (cs *ConsolidatedSummary) Process() error {
	err := cs.provider.Populate()
	if err != nil {
		fmt.Println("ERROR processing provider results...")
		return err
	}

	err = cs.baseline.Populate()
	if err != nil {
		fmt.Println("ERROR processing baseline results...")
		return err
	}

	// Read Suites
	err = cs.suites.LoadAll()
	if err != nil {
		return err
	}

	// apply filters
	err = cs.applyFilterSuite()
	if err != nil {
		return err
	}

	err = cs.applyFilterBaseline()
	if err != nil {
		return err
	}

	return nil
}

// applyFilterSuite process the FailedList for each plugin, getting **intersection** tests
// for respective suite.
func (cs *ConsolidatedSummary) applyFilterSuite() error {
	err := cs.applyFilterSuiteToPlugin("kubernetes-conformance")
	if err != nil {
		return err
	}

	err = cs.applyFilterSuiteToPlugin("openshift-validated")
	if err != nil {
		return err
	}

	return nil
}

// applyFilterSuiteToPlugin calculates the intersection of Provider Failed AND suite
func (cs *ConsolidatedSummary) applyFilterSuiteToPlugin(plugin string) error {
	var e2eSuite []string
	var e2eFailures []string
	var e2eFailuresFiltered []string
	hashSuite := make(map[string]struct{})

	switch plugin {
	case "kubernetes-conformance":
		e2eSuite = cs.suites.kubernetesConformance.tests
		e2eFailures = cs.provider.openshift.pluginResultK8sConformance.FailedList
	case "openshift-validated":
		e2eSuite = cs.suites.openshiftConformance.tests
		e2eFailures = cs.provider.openshift.pluginResultOCPValidated.FailedList
	default:
		fmt.Println("Suite not found!\n")
	}

	for _, v := range e2eSuite {
		hashSuite[v] = struct{}{}
	}

	for _, v := range e2eFailures {
		if _, ok := hashSuite[v]; ok {
			e2eFailuresFiltered = append(e2eFailuresFiltered, v)
		}
	}
	sort.Strings(e2eFailuresFiltered)

	switch plugin {
	case "kubernetes-conformance":
		cs.provider.openshift.pluginResultK8sConformance.FailedFilterSuite = e2eFailuresFiltered
	case "openshift-validated":
		cs.provider.openshift.pluginResultOCPValidated.FailedFilterSuite = e2eFailuresFiltered
	default:
		fmt.Println("Suite not found!\n")
	}

	return nil
}

// applyFilterBaseline process the FailedFilterSuite for each plugin, **excluding** failures from
// baseline test.
func (cs *ConsolidatedSummary) applyFilterBaseline() error {
	err := cs.applyFilterBaselineForPlugin("kubernetes-conformance")
	if err != nil {
		return err
	}

	err = cs.applyFilterBaselineForPlugin("openshift-validated")
	if err != nil {
		return err
	}

	return nil
}

// applyFilterBaselineForPlugin calculates the **exclusion** tests of
// Provider Failed included on suite and Baseline failed tests.
func (cs *ConsolidatedSummary) applyFilterBaselineForPlugin(plugin string) error {
	var e2eFailuresProvider []string
	var e2eFailuresBaseline []string
	var e2eFailuresFiltered []string
	hashBaseline := make(map[string]struct{})

	switch plugin {
	case "kubernetes-conformance":
		e2eFailuresProvider = cs.provider.openshift.pluginResultK8sConformance.FailedFilterSuite
		e2eFailuresBaseline = cs.baseline.openshift.pluginResultK8sConformance.FailedList
	case "openshift-validated":
		e2eFailuresProvider = cs.provider.openshift.pluginResultOCPValidated.FailedFilterSuite
		e2eFailuresBaseline = cs.baseline.openshift.pluginResultOCPValidated.FailedList
	default:
		fmt.Println("Suite not found!\n")
	}

	for _, v := range e2eFailuresBaseline {
		hashBaseline[v] = struct{}{}
	}

	for _, v := range e2eFailuresProvider {
		if _, ok := hashBaseline[v]; !ok {
			e2eFailuresFiltered = append(e2eFailuresFiltered, v)
		}
	}
	sort.Strings(e2eFailuresFiltered)

	switch plugin {
	case "kubernetes-conformance":
		cs.provider.openshift.pluginResultK8sConformance.FailedFilterBaseline = e2eFailuresFiltered
	case "openshift-validated":
		cs.provider.openshift.pluginResultOCPValidated.FailedFilterBaseline = e2eFailuresFiltered
	default:
		fmt.Println("Suite not found!\n")
	}

	return nil
}

func (cs *ConsolidatedSummary) SaveResults(path string) error {

	if err := createDir(path); err != nil {
		return err
	}

	prefix := "tests"

	// for each plugin:
	// save provider failures
	suite := "kubernetes-conformance"
	filename := fmt.Sprintf("%s/%s_%s_provider_failures.txt", path, prefix, suite)
	if err := writeFileTestList(filename, cs.provider.openshift.getResultK8SValidated().FailedList); err != nil {
		return err
	}

	suite = "openshift-validated"
	filename = fmt.Sprintf("%s/%s_%s_provider_failures.txt", path, prefix, suite)
	if err := writeFileTestList(filename, cs.provider.openshift.getResultOCPValidated().FailedList); err != nil {
		return err
	}

	// save provider failures with filter suite
	suite = "kubernetes-conformance"
	filename = fmt.Sprintf("%s/%s_%s_provider_filter1-suite.txt", path, prefix, suite)
	if err := writeFileTestList(filename, cs.provider.openshift.getResultK8SValidated().FailedFilterSuite); err != nil {
		return err
	}

	suite = "openshift-validated"
	filename = fmt.Sprintf("%s/%s_%s_provider_filter1-suite.txt", path, prefix, suite)
	if err := writeFileTestList(filename, cs.provider.openshift.getResultOCPValidated().FailedFilterSuite); err != nil {
		return err
	}
	// save provider failures with filter baseline
	suite = "kubernetes-conformance"
	filename = fmt.Sprintf("%s/%s_%s_provider_filter2-baseline.txt", path, prefix, suite)
	if err := writeFileTestList(filename, cs.provider.openshift.getResultK8SValidated().FailedFilterBaseline); err != nil {
		return err
	}

	suite = "openshift-validated"
	filename = fmt.Sprintf("%s/%s_%s_provider_filter2-baseline.txt", path, prefix, suite)
	if err := writeFileTestList(filename, cs.provider.openshift.getResultOCPValidated().FailedFilterBaseline); err != nil {
		return err
	}

	// save baseline failures
	suite = "kubernetes-conformance"
	filename = fmt.Sprintf("%s/%s_%s_baseline_failures.txt", path, prefix, suite)
	if err := writeFileTestList(filename, cs.baseline.openshift.getResultK8SValidated().FailedList); err != nil {
		return err
	}

	suite = "openshift-validated"
	filename = fmt.Sprintf("%s/%s_%s_baseline_failures.txt", path, prefix, suite)
	if err := writeFileTestList(filename, cs.baseline.openshift.getResultOCPValidated().FailedList); err != nil {
		return err
	}

	// TODO
	// // sub-dir failures-provider-filtered, extract:
	// // - stdout
	// // - detailed
	// subdir := fmt.Sprintf("%s/failures-provider-filtered", path)
	// if _, err := os.Stat(subdir); !os.IsNotExist(err) {
	// 	log.Errorf("ERROR: directory '%s' already exists: %v\n", subdir, err)
	// 	return err
	// }
	// if err := os.Mkdir(subdir, os.ModePerm); err != nil {
	// 	log.Errorf("ERROR: unable to create directory '%s': %v\n", subdir, err)
	// 	return err
	// }
	subdir := fmt.Sprintf("%s/failures-provider-filtered", path)
	if err := createDir(subdir); err != nil {
		return err
	}

	suite = "kubernetes-conformance"
	subPrefix := fmt.Sprintf("%s/%s", subdir, suite)
	errItems := cs.provider.openshift.getResultK8SValidated().FailedItems
	errList := cs.provider.openshift.getResultK8SValidated().FailedFilterBaseline
	if err := extractTestErrors(subPrefix, errItems, errList); err != nil {
		return err
	}

	suite = "openshift-validated"
	subPrefix = fmt.Sprintf("%s/%s", subdir, suite)
	errItems = cs.provider.openshift.getResultOCPValidated().FailedItems
	errList = cs.provider.openshift.getResultOCPValidated().FailedFilterBaseline
	if err := extractTestErrors(subPrefix, errItems, errList); err != nil {
		return err
	}

	// // sub-dir failures-provider, extract:
	// // - stdout
	// // - detailed
	// subdir = fmt.Sprintf("%s/failures-provider", path)
	subdir = fmt.Sprintf("%s/failures-provider", path)
	if err := createDir(subdir); err != nil {
		return err
	}
	suite = "kubernetes-conformance"
	subPrefix = fmt.Sprintf("%s/%s", subdir, suite)
	errItems = cs.provider.openshift.getResultK8SValidated().FailedItems
	errList = cs.provider.openshift.getResultK8SValidated().FailedList
	if err := extractTestErrors(subPrefix, errItems, errList); err != nil {
		return err
	}

	suite = "openshift-validated"
	subPrefix = fmt.Sprintf("%s/%s", subdir, suite)
	errItems = cs.provider.openshift.getResultOCPValidated().FailedItems
	errList = cs.provider.openshift.getResultOCPValidated().FailedList
	if err := extractTestErrors(subPrefix, errItems, errList); err != nil {
		return err
	}

	// // sub-dir failures-baseline, extract:
	// // - stdout
	// // - detailed
	// subdir = fmt.Sprintf("%s/failures-baseline", path)
	subdir = fmt.Sprintf("%s/failures-baseline", path)
	if err := createDir(subdir); err != nil {
		return err
	}
	suite = "kubernetes-conformance"
	subPrefix = fmt.Sprintf("%s/%s", subdir, suite)
	errItems = cs.baseline.openshift.getResultK8SValidated().FailedItems
	errList = cs.baseline.openshift.getResultK8SValidated().FailedList
	if err := extractTestErrors(subPrefix, errItems, errList); err != nil {
		return err
	}

	suite = "openshift-validated"
	subPrefix = fmt.Sprintf("%s/%s", subdir, suite)
	errItems = cs.baseline.openshift.getResultOCPValidated().FailedItems
	errList = cs.baseline.openshift.getResultOCPValidated().FailedList
	if err := extractTestErrors(subPrefix, errItems, errList); err != nil {
		return err
	}

	// for each suite: save test list
	suite = "kubernetes-conformance"
	filename = fmt.Sprintf("%s/%s_%s_suite_full.txt", path, prefix, suite)
	if err := writeFileTestList(filename, cs.suites.kubernetesConformance.tests); err != nil {
		return err
	}
	suite = "openshift-validated"
	filename = fmt.Sprintf("%s/%s_%s_suite_full.txt", path, prefix, suite)
	if err := writeFileTestList(filename, cs.suites.kubernetesConformance.tests); err != nil {
		return err
	}

	fmt.Printf("\n Data Saved to directory '%s/'\n", path)
	return nil
}

func writeFileTestList(filename string, data []string) error {
	fd, err := os.OpenFile(filename, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	defer fd.Close()

	if err != nil {
		log.Fatalf("failed creating file: %s", err)
	}

	writer := bufio.NewWriter(fd)
	defer writer.Flush()

	for _, line := range data {
		_, err = writer.WriteString(line + "\n")
		if err != nil {
			return err
		}
	}

	return nil
}

func extractTestErrors(prefix string, items map[string]*PluginFailedItem, failures []string) error {

	for idx, line := range failures {
		if _, ok := items[line]; ok {
			file := fmt.Sprintf("%s_%d-failure.txt", prefix, idx+1)
			err := writeErrorToFile(file, items[line].Failure)
			if err != nil {
				log.Errorf("Error writing Failure for test: %s\n", line)
			}

			file = fmt.Sprintf("%s_%d-systemOut.txt", prefix, idx+1)
			err = writeErrorToFile(file, items[line].SystemOut)
			if err != nil {
				log.Errorf("Error writing SystemOut for test: %s\n", line)
			}
		}
	}

	return nil
}

func writeErrorToFile(file, data string) error {
	fd, err := os.OpenFile(file, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	defer fd.Close()

	if err != nil {
		log.Fatalf("failed creating file: %s", err)
	}

	writer := bufio.NewWriter(fd)
	defer writer.Flush()

	_, err = writer.WriteString(data)
	if err != nil {
		return err
	}

	return nil
}

func createDir(path string) error {
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		log.Errorf("ERROR: Directory already exists [%s]: %v", path, err)
		return err
	}

	if err := os.Mkdir(path, os.ModePerm); err != nil {
		log.Errorf("ERROR: Unable to create directory [%s]: %v", path, err)
		return err
	}
	return nil
}
